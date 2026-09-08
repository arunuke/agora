package test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/arunuke/agora/internal/arbiter"
	"github.com/arunuke/agora/internal/clock"
	"github.com/arunuke/agora/internal/llm"
	"github.com/arunuke/agora/internal/store"
)

// recordingAgent counts Consult calls per member. It is how the resume tests
// prove that a restart does not re-consult members who already answered — the
// difference between "resumable" and "restartable".
type recordingAgent struct {
	mu    sync.Mutex
	calls map[string]int
	inner arbiter.MemberAgent
}

func (r *recordingAgent) Consult(ctx context.Context, req arbiter.ConsultRequest) (arbiter.ConsultResponse, error) {
	r.mu.Lock()
	if r.calls == nil {
		r.calls = map[string]int{}
	}
	r.calls[req.MemberID]++
	r.mu.Unlock()
	return r.inner.Consult(ctx, req)
}

func (r *recordingAgent) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, v := range r.calls {
		n += v
	}
	return n
}

func TestDurability_ResumeDoesNotReconsultAnsweredMembers(t *testing.T) {
	a := newApp(t, 2)
	rec := &recordingAgent{inner: a.Agent}
	arb := arbiter.New(a.Store, rec, llm.Deterministic{}, a.Clock, a.Switch,
		arbiter.Config{Policy: arbiter.FamilyDefault(), Deadline: 500 * time.Millisecond})

	res, err := arb.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := a.MemberIDs()
	if got := rec.total(); got != len(ids) {
		t.Fatalf("first run consulted %d times, want %d", got, len(ids))
	}

	// Simulate a process restart picking the workflow back up.
	if _, err := arb.Resume(ctx(), res.ConveneID); err != nil {
		t.Fatal(err)
	}
	if got := rec.total(); got != len(ids) {
		t.Errorf("resume re-consulted members: %d calls total, want %d. "+
			"Persisted signals must be reused, or a restart becomes a restart-from-scratch",
			got, len(ids))
	}
}

func TestDurability_CheckpointsProgressThroughStates(t *testing.T) {
	a := newApp(t, 2)
	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	c, err := a.Store.LoadConvene(res.ConveneID)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != arbiter.StateDelivered {
		t.Errorf("final state = %q, want %q", c.State, arbiter.StateDelivered)
	}
	if len(c.Signals) == 0 {
		t.Error("signals must be persisted before the workflow can be resumed")
	}
	if len(c.Result) == 0 {
		t.Error("result must be persisted")
	}
}

// A workflow left mid-flight by a crash is swept forward rather than stranded.
func TestDurability_InterruptedWorkflowIsSweptForward(t *testing.T) {
	a := newApp(t, 2)
	stranded := store.Convene{
		ConveneID: "cv_stranded", GroupID: a.GroupID,
		State: arbiter.StateFanoutStarted, CreatedAt: a.Clock.Now(),
	}
	if err := a.Store.SaveConvene(stranded); err != nil {
		t.Fatal(err)
	}
	ran, err := a.Arbiter.RunDue(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) == 0 {
		t.Fatal("a workflow stuck in fanout_started must be picked up")
	}
	c, err := a.Store.LoadConvene("cv_stranded")
	if err != nil {
		t.Fatal(err)
	}
	if c.State != arbiter.StateDelivered {
		t.Errorf("stranded workflow state = %q, want delivered", c.State)
	}
}

// US6 — a long-running workflow made observable inside a five-minute review.
func TestDurability_ScheduledConveneFiresOnSimulatedClock(t *testing.T) {
	a := newApp(t, 2)
	fireAt := a.Clock.Now().Add(48 * time.Hour)
	id, err := a.Arbiter.Schedule(a.GroupID, fireAt)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := a.Arbiter.RunDue(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 0 {
		t.Fatalf("a scheduled convene must not fire early, ran %d", len(ran))
	}

	a.Clock.Advance(72 * time.Hour)
	ran, err = a.Arbiter.RunDue(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) == 0 {
		t.Fatal("advancing past fire_at must run the workflow")
	}
	c, _ := a.Store.LoadConvene(id)
	if c.State != arbiter.StateDelivered {
		t.Errorf("scheduled workflow state = %q, want delivered", c.State)
	}
}

func TestDurability_NotificationsPiggybackTheNextResponse(t *testing.T) {
	a := newApp(t, 2)
	if _, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben"); err != nil {
		t.Fatal(err)
	}
	notes, err := a.Store.PopNotifications("eli")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) == 0 {
		t.Fatal("a completed convene must leave a notification for each member")
	}
	// Delivered exactly once.
	again, err := a.Store.PopNotifications("eli")
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("notifications must be delivered once, got %d on the second poll", len(again))
	}
}

var _ = clock.Base
