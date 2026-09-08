// Package agent is Loop A — the PRIVATE trust level.
//
// Every LLM session started here sees exactly ONE member's raw context and
// never any other member's. That is the load-bearing property: a member's raw
// context is never present in any context window producing output shown to
// another member. Not filtered out — never present.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/arunuke/agora/internal/agent/internal/rawctx"
	"github.com/arunuke/agora/internal/arbiter"
	"github.com/arunuke/agora/internal/llm"
	"github.com/arunuke/agora/internal/store"
	"github.com/arunuke/agora/internal/vocab"
)

const (
	TierLLM     = 1 // completion extraction: negation, idiom, nuance
	TierEmbed   = 2 // embedding nearest-neighbour to the canonical vocabulary
	TierKeyword = 3 // exact synonym match only
)

type Agent struct {
	st  *store.Store
	llm llm.LLMClient
	emb llm.Embedder
}

func New(st *store.Store, client llm.LLMClient, emb llm.Embedder) *Agent {
	return &Agent{st: st, llm: client, emb: emb}
}

type MessageResult struct {
	Reply    string
	Tier     int
	Degraded bool
}

// HandleMessage is a chat turn. It runs entirely inside one member's private
// scope: it reads their raw context, appends to it, re-derives, and replies.
func (a *Agent) HandleMessage(ctx context.Context, memberID, text string) (MessageResult, error) {
	scope := rawctx.For(a.st.DB(), memberID)
	if _, _, err := scope.Load(); err != nil {
		return MessageResult{}, err
	}
	if strings.TrimSpace(text) != "" {
		if err := scope.AppendRaw(text); err != nil {
			return MessageResult{}, err
		}
	}
	derived, tier, degraded, err := a.rederive(ctx, scope)
	if err != nil {
		return MessageResult{}, err
	}

	var ack []string
	for _, c := range extractFrom(ctx, a, text, tier) {
		ack = append(ack, c.Human())
	}
	titles, err := a.recommendFor(ctx, derived, 3)
	if err != nil {
		return MessageResult{}, err
	}

	payload := map[string]any{"acknowledge": ack, "titles": titles}
	resp, err := a.llm.Complete(ctx, llm.Request{Task: llm.TaskChat, Input: text, Payload: payload})
	if err != nil {
		// Degraded chat: a template, never a fabrication and never a hang.
		return MessageResult{Reply: templateReply(ack, titles), Tier: tier, Degraded: true}, nil
	}
	return MessageResult{Reply: resp.Text, Tier: tier, Degraded: degraded}, nil
}

// Consult implements arbiter.MemberAgent. It returns a SEALED SIGNAL: derived,
// closed-vocabulary, no identity, no free text. The arbiter never needs this
// member's full state, which is exactly why it can proceed when this call
// times out.
func (a *Agent) Consult(ctx context.Context, req arbiter.ConsultRequest) (arbiter.ConsultResponse, error) {
	scope := rawctx.For(a.st.DB(), req.MemberID)
	derived, tier, degraded, err := a.rederive(ctx, scope)
	if err != nil {
		return arbiter.ConsultResponse{}, err
	}
	sig := arbiter.Signal{
		SignalID:    newID("sig"),
		Constraints: derived.Constraints,
		Vetoes:      derived.Vetoes,
		Tier:        tier,
	}
	if sig.Constraints == nil {
		sig.Constraints = []vocab.Constraint{}
	}
	if sig.Vetoes == nil {
		sig.Vetoes = []vocab.Veto{}
	}
	return arbiter.ConsultResponse{Signal: sig, Degraded: degraded, Tier: tier}, nil
}

// rederive rebuilds the closed-vocabulary form of a member's context from
// their raw text, descending the fidelity ladder as providers fail.
func (a *Agent) rederive(ctx context.Context, scope *rawctx.Scope) (rawctx.Derived, int, bool, error) {
	raw, _, err := scope.Load()
	if err != nil {
		return rawctx.Derived{}, 0, false, err
	}
	joined := strings.Join(raw, ". ")

	cs, vs, tier := a.extract(ctx, joined)
	d := rawctx.Derived{Constraints: cs, Vetoes: vs, Tier: tier}
	if err := scope.SaveDerived(d); err != nil {
		return d, tier, tier != TierLLM, err
	}
	// The profile vector is written through the scope, never directly. It is
	// private data: derived, but re-identifying and partially invertible, so
	// it obeys the same boundary as the raw text.
	if a.emb != nil {
		if vecs, err := a.emb.Embed(ctx, []string{joined}); err == nil && len(vecs) == 1 {
			_ = scope.SaveVector(a.st, vecs[0])
		}
	}
	return d, tier, tier != TierLLM, nil
}

// extract is the degradation ladder.
func (a *Agent) extract(ctx context.Context, text string) ([]vocab.Constraint, []vocab.Veto, int) {
	// --- tier 1: completion ---
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := a.llm.Complete(ctx, llm.Request{Task: llm.TaskExtract, Input: text})
		if err != nil {
			break
		}
		var res llm.ExtractResult
		if err := json.Unmarshal(resp.JSON, &res); err != nil {
			continue // one retry, then descend. Raw text is never passed through.
		}
		cs := sanitize(res.Constraints)
		return cs, sanitizeVetoes(res.Vetoes), TierLLM
	}

	// --- tier 2: embedding nearest-neighbour to the closed vocabulary ---
	// Lower fidelity by construction: no negation handling, so a veto stated
	// in prose is downgraded to a preference. That is a real, documented loss.
	if a.emb != nil && a.st.VectorsOK() {
		clauses := vocab.SplitClauses(text)
		if vecs, err := a.emb.Embed(ctx, clauses); err == nil {
			var cs []vocab.Constraint
			seen := map[string]bool{}
			for _, v := range vecs {
				ns, err := a.st.VocabKNN(v, 1)
				if err != nil || len(ns) == 0 {
					continue
				}
				dim, val, ok := splitKey(ns[0].ID)
				if !ok || !vocab.Valid(dim, val) {
					continue
				}
				c := vocab.Constraint{Dim: dim, Value: val, Polarity: vocab.Prefer, Weight: 0.5}
				if !seen[c.Key()] {
					seen[c.Key()] = true
					cs = append(cs, c)
				}
			}
			if len(cs) > 0 {
				return cs, nil, TierEmbed
			}
		}
	}

	// --- tier 3: keyword match over the vocabulary ---
	var cs []vocab.Constraint
	for _, t := range vocab.MatchTerms(text) {
		cs = append(cs, vocab.Constraint{Dim: t.Dim, Value: t.Value, Polarity: vocab.Prefer, Weight: 0.5})
	}
	return cs, nil, TierKeyword
}

func (a *Agent) recommendFor(ctx context.Context, d rawctx.Derived, n int) ([]string, error) {
	titles, err := a.st.Titles()
	if err != nil {
		return nil, err
	}
	sigs := []arbiter.Signal{{Constraints: d.Constraints, Vetoes: d.Vetoes}}
	// Scoring one member against their own constraints. K=1 here is not an
	// anonymity decision: with a single signal there is no group to protect.
	r := arbiter.Reconcile(sigs, titles, arbiter.CloudParity())
	var out []string
	for i, t := range r.Candidates {
		if i >= n {
			break
		}
		out = append(out, fmt.Sprintf("%s (%d min, %s)", t.Title, t.Runtime, t.Availability))
	}
	return out, nil
}

func templateReply(ack, titles []string) string {
	var sb strings.Builder
	sb.WriteString("Recorded")
	if len(ack) > 0 {
		sb.WriteString(": " + strings.Join(ack, ", "))
	}
	sb.WriteString(".")
	if len(titles) > 0 {
		sb.WriteString(" You might like: " + strings.Join(titles, "; ") + ".")
	}
	sb.WriteString(" (Degraded mode — running without the language model.)")
	return sb.String()
}

func extractFrom(ctx context.Context, a *Agent, text string, tier int) []vocab.Constraint {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	cs, _, _ := a.extract(ctx, text)
	return cs
}

func sanitize(cs []vocab.Constraint) []vocab.Constraint {
	var out []vocab.Constraint
	for _, c := range cs {
		if !vocab.Valid(c.Dim, c.Value) {
			continue // anything outside the closed vocabulary is discarded
		}
		if c.Polarity != vocab.Prefer && c.Polarity != vocab.Exclude {
			c.Polarity = vocab.Prefer
		}
		if c.Weight <= 0 || c.Weight > 1 {
			c.Weight = 0.6
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func sanitizeVetoes(vs []vocab.Veto) []vocab.Veto {
	var out []vocab.Veto
	for _, v := range vs {
		if vocab.Valid(v.Dim, v.Value) {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func splitKey(k string) (vocab.Dim, string, bool) {
	i := strings.Index(k, ":")
	if i < 0 {
		return "", "", false
	}
	return vocab.Dim(k[:i]), k[i+1:], true
}

func newID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
