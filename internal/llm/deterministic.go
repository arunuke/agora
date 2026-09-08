package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/arunuke/agora/internal/vocab"
)

// Deterministic is both the hermetic test double and the real tier-3-capable
// provider. Every test in the suite runs against it: a gate that depends on a
// network call is not a gate.
//
// Its extraction is negation-aware and weight-aware, which is what makes it a
// genuine tier 1 rather than a stub — and what makes the fidelity drop to
// tier 2 (no negation) and tier 3 (exact matches only) observable offline.
type Deterministic struct{}

func (d Deterministic) Complete(ctx context.Context, req Request) (Response, error) {
	switch req.Task {
	case TaskExtract:
		return d.extract(req.Input)
	case TaskChat:
		return d.chat(req)
	case TaskRank:
		return d.rank(req)
	default:
		return Response{}, fmt.Errorf("llm: unknown task %q", req.Task)
	}
}

type ExtractResult struct {
	Constraints []vocab.Constraint `json:"constraints"`
	Vetoes      []vocab.Veto       `json:"vetoes"`
}

func (Deterministic) extract(input string) (Response, error) {
	res := ExtractResult{Constraints: []vocab.Constraint{}, Vetoes: []vocab.Veto{}}
	seenC := map[string]bool{}
	seenV := map[string]bool{}

	for _, clause := range vocab.SplitClauses(input) {
		matched := vocab.MatchTerms(clause)
		if len(matched) == 0 {
			continue
		}
		strong := vocab.HasStrongNegation(clause)
		soft := vocab.HasSoftNegation(clause)
		intense := vocab.HasIntensifier(clause)

		for _, t := range matched {
			switch {
			case strong:
				// A hard "can't" becomes a veto: a filter, not a weight.
				v := vocab.Veto{Dim: t.Dim, Value: t.Value}
				if !seenV[v.Key()] {
					seenV[v.Key()] = true
					res.Vetoes = append(res.Vetoes, v)
				}
			case soft:
				c := vocab.Constraint{Dim: t.Dim, Value: t.Value, Polarity: vocab.Exclude, Weight: 0.7}
				if !seenC[c.Key()] {
					seenC[c.Key()] = true
					res.Constraints = append(res.Constraints, c)
				}
			default:
				w := 0.6
				if intense {
					w = 0.9
				}
				c := vocab.Constraint{Dim: t.Dim, Value: t.Value, Polarity: vocab.Prefer, Weight: w}
				if !seenC[c.Key()] {
					seenC[c.Key()] = true
					res.Constraints = append(res.Constraints, c)
				}
			}
		}
	}
	b, err := json.Marshal(res)
	if err != nil {
		return Response{}, ErrMalformed
	}
	return Response{JSON: b}, nil
}

func (Deterministic) chat(req Request) (Response, error) {
	ack, _ := req.Payload["acknowledge"].([]string)
	titles, _ := req.Payload["titles"].([]string)

	var sb strings.Builder
	switch {
	case len(ack) > 0 && len(titles) == 0:
		sb.WriteString("Noted — I've recorded that you're after ")
		sb.WriteString(joinHuman(ack))
		sb.WriteString(". I'll keep that in mind for you and for family movie night.")
	case len(titles) > 0:
		if len(ack) > 0 {
			sb.WriteString("Got it — ")
			sb.WriteString(joinHuman(ack))
			sb.WriteString(". ")
		}
		sb.WriteString("Based on what you've told me, you might like: ")
		sb.WriteString(strings.Join(titles, "; "))
		sb.WriteString(".")
	default:
		sb.WriteString("I can take your preferences, recommend something for you, " +
			"or convene the family for a movie night. I only ever speak to your own " +
			"preferences — I can't tell you what anyone else has shared with their agent.")
	}
	return Response{Text: sb.String()}, nil
}

type RankResult struct {
	Order         []string `json:"order"`
	Justification string   `json:"justification"`
}

// rank sees ONLY what the reconciler hands it: veto-filtered candidates that
// are already scored, plus the constraints that met the k-threshold. It has no
// access to member context, so it cannot leak or attribute one.
func (Deterministic) rank(req Request) (Response, error) {
	type cand struct {
		ID    string
		Title string
		Score float64
	}
	var cands []cand
	raw, _ := req.Payload["candidates"].([]map[string]any)
	for _, m := range raw {
		id, _ := m["id"].(string)
		title, _ := m["title"].(string)
		score, _ := m["score"].(float64)
		cands = append(cands, cand{id, title, score})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		return cands[i].ID < cands[j].ID
	})

	res := RankResult{Order: []string{}}
	for _, c := range cands {
		res.Order = append(res.Order, c.ID)
	}

	public, _ := req.Payload["public_constraints"].([]string)
	quorum, _ := req.Payload["quorum"].(string)
	switch {
	case len(public) == 0:
		res.Justification = "Picked on overall group fit. Nothing was shared widely enough " +
			"across the family to call out without pointing at someone, so this leans on the catalogue."
	default:
		res.Justification = "The family leans toward " + joinHuman(public) +
			", and these fit that best."
	}
	if quorum != "" {
		res.Justification += " " + quorum
	}
	b, err := json.Marshal(res)
	if err != nil {
		return Response{}, ErrMalformed
	}
	return Response{JSON: b}, nil
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
		return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
	}
}
