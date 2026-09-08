package arbiter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/arunuke/agora/internal/llm"
)

// Loop B is the PUBLIC trust level.
//
// It receives veto-filtered candidates that are already scored, plus the
// constraints that met the k-threshold. It receives NOTHING else — no member
// context, no identities, no below-threshold constraints, and no vetoed
// titles. A member cannot extract another member's preferences from this loop
// because they were never present in it.
//
// The tool surface IS the isolation boundary, which is why it is enumerable:
// list_candidates and get_public_constraints, and nothing that could reach a
// member's context even if the model asked for it.

const slateSize = 5
const candidateWindow = 12

func (a *Arbiter) rank(ctx context.Context, r Reconciled, q Quorum) ([]SlateItem, string, bool) {
	top := r.Candidates
	if len(top) > candidateWindow {
		top = top[:candidateWindow]
	}

	payload := []map[string]any{}
	for _, t := range top {
		payload = append(payload, map[string]any{
			"id": t.TitleID, "title": t.Title, "score": r.Scores[t.TitleID],
		})
	}

	quorumNote := ""
	if q.Provisional {
		quorumNote = fmt.Sprintf("Provisional: %d of %d family members were represented.",
			q.Represented, q.Of)
	}

	build := func(order []string) []SlateItem {
		byID := map[string]int{}
		for i, t := range top {
			byID[t.TitleID] = i
		}
		var out []SlateItem
		for _, id := range order {
			i, ok := byID[id]
			if !ok {
				continue
			}
			t := top[i]
			out = append(out, SlateItem{
				TitleID: t.TitleID, Title: t.Title, Availability: t.Availability,
				Runtime: t.Runtime, Score: r.Scores[t.TitleID],
			})
			if len(out) == slateSize {
				break
			}
		}
		return out
	}

	// Deterministic fallback, prepared first so no failure path can hang or
	// fabricate. Candidates are already ranked by the reconciler.
	fallbackOrder := make([]string, 0, len(top))
	for _, t := range top {
		fallbackOrder = append(fallbackOrder, t.TitleID)
	}
	fallback := build(fallbackOrder)
	fallbackJustification := templateJustification(r.PublicPhrases(), quorumNote)

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := a.llm.Complete(ctx, llm.Request{
			Task: llm.TaskRank,
			Payload: map[string]any{
				"candidates":         payload,
				"public_constraints": r.PublicPhrases(),
				"quorum":             quorumNote,
			},
		})
		if err != nil {
			break // provider down: use the deterministic path
		}
		var out llm.RankResult
		if err := json.Unmarshal(resp.JSON, &out); err != nil {
			// One retry, then fall back. Raw model text is NEVER passed into a
			// justification — that rule is a privacy control before it is a
			// robustness one, because unparsed text is the likeliest leak path.
			continue
		}
		slate := build(out.Order)
		if len(slate) == 0 {
			break
		}
		return slate, out.Justification, false
	}
	return fallback, fallbackJustification + " (Degraded: ranked without the language model.)", true
}

func templateJustification(public []string, quorumNote string) string {
	var s string
	if len(public) == 0 {
		s = "Picked on overall group fit. Nothing was shared widely enough across the " +
			"family to name without pointing at someone, so this leans on the catalogue."
	} else {
		s = "The family leans toward " + joinHuman(public) + ", and these fit that best."
	}
	if quorumNote != "" {
		s += " " + quorumNote
	}
	return s
}

func joinHuman(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	case 2:
		return xs[0] + " and " + xs[1]
	default:
		out := ""
		for i, x := range xs {
			switch {
			case i == len(xs)-1:
				out += " and " + x
			case i == 0:
				out += x
			default:
				out += ", " + x
			}
		}
		return out
	}
}
