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
	Reply string
	Tier  int
	// Suggestions is what the SYSTEM computed from the catalogue, independent of
	// how the model chose to word its reply. Exposed so callers (and tests) can
	// assert on behaviour rather than on prose.
	Suggestions []string
	Degraded    bool
}

// HandleMessage is a chat turn. It runs entirely inside one member's private
// scope: it reads their raw context, appends to it, re-derives, and replies.
func (a *Agent) HandleMessage(ctx context.Context, memberID, text string) (MessageResult, error) {
	scope := rawctx.For(a.st.DB(), memberID)
	if _, _, err := scope.Load(); err != nil {
		return MessageResult{}, err
	}
	// REFUSAL BEFORE ANYTHING ELSE. A request that reaches for another member's
	// context is declined here, in code, before the text is stored, before it is
	// extracted, and before any model sees it.
	//
	// The chat prompt already instructs the model to decline. That instruction
	// is worth keeping and worth nothing on its own: a 3B local model answered
	// this exact probe by inventing preferences and attributing them to a member
	// by name. Isolation itself never depended on the model — Loop A can only
	// load its own scope — but the REPLY did, and a confabulated answer reads
	// to a user as a leak whether or not anything leaked.
	//
	// Refusing before AppendRaw also keeps the probe out of the asker's profile,
	// where another member's name has no business being.
	if a.asksAboutAnotherMember(memberID, text) {
		name := "there"
		if m, err := a.st.Member(memberID); err == nil && m.DisplayName != "" {
			name = m.DisplayName
		}
		return MessageResult{Reply: refusalFor(name), Tier: TierKeyword}, nil
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
		return MessageResult{Reply: templateReply(ack, titles), Tier: tier,
			Suggestions: titles, Degraded: true}, nil
	}
	return MessageResult{Reply: resp.Text, Tier: tier, Suggestions: titles, Degraded: degraded}, nil
}

// refusalFor is the one sentence this system will not let a model improvise.
func refusalFor(name string) string {
	return fmt.Sprintf("I'm sorry, %s. I'm afraid I can't do that. "+
		"I only hold your own preferences, never anyone else's — not to read out, "+
		"not to summarise, and not to copy onto you. Tell me what YOU feel like "+
		"watching and I'll work from that.", name)
}

// groupProbes are requests that reach for other members without naming one.
// Deliberately narrow: each phrase asks ABOUT members, which is why none of
// them appears in an ordinary statement of taste. "Something everyone can
// watch" is a preference and must keep working.
var groupProbes = []string{
	"every member", "each member", "all members", "member by member",
	"every user", "all users", "other members", "the other member",
	"everyone else", "anyone else", "someone else", "somebody else",
	"each person", "everybody else",
	"their preferences", "their profile", "his preferences", "her preferences",
	"raw profile", "profile table", "stored preferences", "everyone's preferences",
	"which member", "who wanted", "who else", "who disagreed", "who vetoed",
}

// asksAboutAnotherMember reports whether a message reaches for someone else's
// context, either by naming a member or by asking about members generally.
//
// Names are matched on word boundaries, not substrings: a member called Cara
// must not make "caramel" a refusal, and the whole point of this check is that
// it fires on the request rather than on the vocabulary.
func (a *Agent) asksAboutAnotherMember(memberID, text string) bool {
	low := strings.ToLower(text)
	for _, p := range groupProbes {
		if strings.Contains(low, p) {
			return true
		}
	}

	me, err := a.st.Member(memberID)
	if err != nil {
		return false
	}
	others, err := a.st.Members(me.GroupID)
	if err != nil {
		return false
	}
	for _, m := range others {
		if m.MemberID == memberID {
			continue
		}
		for _, token := range []string{m.DisplayName, m.MemberID} {
			token = strings.ToLower(strings.TrimSpace(token))
			if token == "" {
				continue
			}
			if wordInText(low, token) {
				return true
			}
		}
	}
	return false
}

// wordInText is a word-boundary containment check over already-lowered text.
func wordInText(text, word string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(word)
		beforeOK := start == 0 || !isWordByte(text[start-1])
		afterOK := end == len(text) || !isWordByte(text[end])
		if beforeOK && afterOK {
			return true
		}
		i = start + 1
		if i >= len(text) {
			return false
		}
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
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
