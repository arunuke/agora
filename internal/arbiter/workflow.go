package arbiter

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/arunuke/agora/internal/clock"
	"github.com/arunuke/agora/internal/llm"
	"github.com/arunuke/agora/internal/store"
)

// Convene workflow states. Each is persisted BEFORE the call that causes the
// next one, so a crash resumes rather than restarts.
const (
	StateCreated         = "created"
	StateFanoutStarted   = "fanout_started"
	StateSignalsCollected = "signals_collected"
	StateReconciled      = "reconciled"
	StateRanked          = "ranked"
	StateDelivered       = "delivered"
)

type Arbiter struct {
	st       Store
	agent    MemberAgent
	llm      llm.LLMClient
	clk      clock.Clock
	sw       *llm.Switches
	policy   AnonymityPolicy
	deadline time.Duration

	mu sync.Mutex // serialises checkpoint writes; SQLite has one writer
}

type Config struct {
	Policy   AnonymityPolicy
	Deadline time.Duration
}

func New(st Store, ag MemberAgent, client llm.LLMClient, clk clock.Clock, sw *llm.Switches, cfg Config) *Arbiter {
	if cfg.Deadline == 0 {
		cfg.Deadline = 2 * time.Second
	}
	return &Arbiter{st: st, agent: ag, llm: client, clk: clk, sw: sw,
		policy: cfg.Policy, deadline: cfg.Deadline}
}

func (a *Arbiter) Policy() AnonymityPolicy { return a.policy }

// ---------------------------------------------------------------- fan-out --

// chaosAgent decorates MemberAgent so POST /v1/demo/chaos needs no conditional
// anywhere inside the workflow. Failing a member is a decorator, not a branch.
type chaosAgent struct {
	inner MemberAgent
	sw    *llm.Switches
	index int
}

func (c chaosAgent) Consult(ctx context.Context, req ConsultRequest) (ConsultResponse, error) {
	if c.sw != nil && c.index < c.sw.MemberFail() {
		if c.sw.MemberHang() {
			// Hang rather than error. The convene must still return within
			// budget — this is what proves the per-member deadline is real.
			<-ctx.Done()
			return ConsultResponse{}, ctx.Err()
		}
		return ConsultResponse{}, fmt.Errorf("chaos: member agent %d unavailable", c.index)
	}
	if c.sw != nil {
		if d := c.sw.MemberLatency(); d > 0 {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return ConsultResponse{}, ctx.Err()
			}
		}
	}
	return c.inner.Consult(ctx, req)
}

// fanout consults every member in parallel under a per-member deadline and
// returns an UNORDERED BAG of signals with identity stripped.
//
// This is layer 2 of anonymity. The map from member to signal exists only
// inside this function, is used only for quorum accounting, and is not
// returned — the reconciler has no identities to leak even in principle.
func (a *Arbiter) fanout(ctx context.Context, members []store.Member) ([]Signal, Quorum, bool) {
	type result struct {
		sig      Signal
		degraded bool
		err      error
	}
	results := make([]result, len(members))
	var wg sync.WaitGroup

	for i, m := range members {
		wg.Add(1)
		go func(i int, m store.Member) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, a.deadline)
			defer cancel()
			ag := chaosAgent{inner: a.agent, sw: a.sw, index: i}
			resp, err := ag.Consult(cctx, ConsultRequest{MemberID: m.MemberID, Purpose: "convene"})
			results[i] = result{sig: resp.Signal, degraded: resp.Degraded, err: err}
		}(i, m)
	}
	wg.Wait()

	var signals []Signal
	var anyDegraded bool
	for _, r := range results {
		if r.err != nil {
			continue // a member that times out does not fail the workflow
		}
		signals = append(signals, r.sig)
		anyDegraded = anyDegraded || r.degraded
	}

	// Shuffle before handing to the reconciler. Position in the slice must
	// carry no information: an ordering that tracked member index would be an
	// identity side channel that name-stripping does not close.
	mrand.Shuffle(len(signals), func(i, j int) { signals[i], signals[j] = signals[j], signals[i] })

	q := Quorum{Represented: len(signals), Of: len(members)}
	q.Provisional = q.Represented < q.Of
	return signals, q, anyDegraded
}

// --------------------------------------------------------------- convene ---

func (a *Arbiter) Convene(ctx context.Context, groupID, requestedBy string) (ConveneResult, error) {
	id := newID("cv")
	c := store.Convene{
		ConveneID: id, GroupID: groupID, State: StateCreated,
		CreatedAt: a.clk.Now(),
	}
	if err := a.checkpoint(c); err != nil {
		return ConveneResult{}, err
	}
	return a.run(ctx, c)
}

// Schedule creates a durable workflow that fires when the simulated clock
// passes fireAt. This is what makes a long-running workflow observable inside
// a five-minute review.
func (a *Arbiter) Schedule(groupID string, fireAt time.Time) (string, error) {
	id := newID("cv")
	c := store.Convene{
		ConveneID: id, GroupID: groupID, State: StateCreated,
		FireAt: &fireAt, CreatedAt: a.clk.Now(),
	}
	return id, a.checkpoint(c)
}

// RunDue advances every workflow whose time has come, plus anything left
// mid-flight by a restart. Called by POST /v1/demo/tick.
func (a *Arbiter) RunDue(ctx context.Context) ([]ConveneResult, error) {
	due, err := a.st.ConvenesDue(a.clk.Now())
	if err != nil {
		return nil, err
	}
	var out []ConveneResult
	for _, c := range due {
		res, err := a.run(ctx, c)
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

// Resume picks a workflow back up from its last checkpoint. Signals already
// collected are reused: members who already answered are NOT re-consulted.
func (a *Arbiter) Resume(ctx context.Context, conveneID string) (ConveneResult, error) {
	c, err := a.st.LoadConvene(conveneID)
	if err != nil {
		return ConveneResult{}, err
	}
	return a.run(ctx, c)
}

func (a *Arbiter) run(ctx context.Context, c store.Convene) (ConveneResult, error) {
	members, err := a.st.Members(c.GroupID)
	if err != nil {
		return ConveneResult{}, err
	}
	titles, err := a.st.Titles()
	if err != nil {
		return ConveneResult{}, err
	}

	var signals []Signal
	var q Quorum
	var degraded bool

	if len(c.Signals) > 0 && c.State != StateCreated {
		// Resume path: signals were persisted before the crash.
		var saved struct {
			Signals []Signal `json:"signals"`
			Quorum  Quorum   `json:"quorum"`
		}
		if err := json.Unmarshal(c.Signals, &saved); err == nil && len(saved.Signals) > 0 {
			signals, q = saved.Signals, saved.Quorum
		}
	}

	if signals == nil {
		c.State = StateFanoutStarted
		if err := a.checkpoint(c); err != nil {
			return ConveneResult{}, err
		}
		signals, q, degraded = a.fanout(ctx, members)

		blob, _ := json.Marshal(struct {
			Signals []Signal `json:"signals"`
			Quorum  Quorum   `json:"quorum"`
		}{signals, q})
		c.Signals = blob
		c.State = StateSignalsCollected
		if err := a.checkpoint(c); err != nil {
			return ConveneResult{}, err
		}
	}

	r := Reconcile(signals, titles, a.policy)
	c.State = StateReconciled
	if err := a.checkpoint(c); err != nil {
		return ConveneResult{}, err
	}

	slate, justification, rankDegraded := a.rank(ctx, r, q)
	res := ConveneResult{
		ConveneID:         c.ConveneID,
		Slate:             slate,
		Justification:     justification,
		Quorum:            q,
		Degraded:          degraded || rankDegraded,
		Tier:              lowestTier(signals),
		PublicConstraints: r.PublicPhrases(),
	}
	blob, _ := json.Marshal(res)
	c.Result = blob
	c.State = StateRanked
	if err := a.checkpoint(c); err != nil {
		return ConveneResult{}, err
	}

	// Notifications piggyback the next response rather than needing transport.
	if len(slate) > 0 {
		data, _ := json.Marshal(map[string]any{"slate": slate})
		for _, m := range members {
			_ = a.st.EnqueueNotification(m.MemberID, "movie_night_ready",
				"A movie night slate is ready for the family:", string(data))
		}
	}

	c.State = StateDelivered
	if err := a.checkpoint(c); err != nil {
		return ConveneResult{}, err
	}
	return res, nil
}

func (a *Arbiter) checkpoint(c store.Convene) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st.SaveConvene(c)
}

func lowestTier(signals []Signal) int {
	t := 1
	for _, s := range signals {
		if s.Tier > t {
			t = s.Tier
		}
	}
	return t
}

func newID(prefix string) string {
	b := make([]byte, 4)
	_, _ = crand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
