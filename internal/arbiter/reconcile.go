package arbiter

import (
	"sort"
	"strconv"

	"github.com/arunuke/agora/internal/store"
	"github.com/arunuke/agora/internal/vocab"
)

// Reconciliation is DETERMINISTIC and contains no LLM call and no vector
// search. That is deliberate: the privacy-critical step is the one step that
// is not a model call, so its behaviour is testable and its output stable.
//
// Vectors carry free text as far as the vocabulary (in the agent, tier 2);
// from here on everything is enums, because the k-threshold needs countable,
// EQUAL constraints and nearest-neighbour similarity does not give that.

type Reconciled struct {
	Tallies     []Tally
	Public      []vocab.Constraint
	Private     []vocab.Constraint
	Vetoes      []vocab.Veto
	Candidates  []store.Title
	Scores      map[string]float64
	SignalCount int
}

// Reconcile takes an UNORDERED BAG of signals with no identities attached.
func Reconcile(signals []Signal, titles []store.Title, p AnonymityPolicy) Reconciled {
	r := Reconciled{Scores: map[string]float64{}, SignalCount: len(signals)}

	// --- vetoes: collected first, applied as a silent hard filter ---
	vetoSeen := map[string]bool{}
	for _, s := range signals {
		for _, v := range s.Vetoes {
			if !vetoSeen[v.Key()] {
				vetoSeen[v.Key()] = true
				r.Vetoes = append(r.Vetoes, v)
			}
		}
	}
	sort.Slice(r.Vetoes, func(i, j int) bool { return r.Vetoes[i].Key() < r.Vetoes[j].Key() })

	// --- tally by (dim, value, polarity), counting DISTINCT signals ---
	idx := map[string]int{}
	var tallies []Tally
	for _, s := range signals {
		perSignal := map[string]bool{}
		for _, c := range s.Constraints {
			k := c.Key()
			if perSignal[k] {
				continue // one signal counts once toward the threshold
			}
			perSignal[k] = true
			if i, ok := idx[k]; ok {
				tallies[i].Count++
				tallies[i].Weight += c.Weight
			} else {
				// The representative constraint is CANONICAL: dim, value and
				// polarity only. Its own Weight is deliberately zeroed here and
				// replaced by the aggregate below.
				//
				// Keeping the first-seen signal's weight would make the output
				// depend on arrival order — and since arrival order can
				// correlate with member index, that is an identity side
				// channel that stripping names does nothing to close. The
				// shuffle-invariance property test exists to catch exactly
				// this, and did.
				canonical := vocab.Constraint{Dim: c.Dim, Value: c.Value, Polarity: c.Polarity}
				idx[k] = len(tallies)
				tallies = append(tallies, Tally{Constraint: canonical, Count: 1, Weight: c.Weight})
			}
		}
	}
	for i := range tallies {
		tallies[i].Constraint.Weight = tallies[i].Weight
	}
	// Stable ordering so the output does not depend on map iteration or on the
	// order signals arrived in. Shuffle-invariance is both a determinism
	// property and an anonymity one: an ordering that tracked member index
	// would be an identity side channel.
	sort.SliceStable(tallies, func(i, j int) bool {
		return tallies[i].Constraint.Key() < tallies[j].Constraint.Key()
	})

	r.Tallies = p.Classify(tallies)
	for _, t := range r.Tallies {
		if t.Public {
			r.Public = append(r.Public, t.Constraint)
		} else {
			r.Private = append(r.Private, t.Constraint)
		}
	}

	// --- veto filter: silent, and BEFORE scoring or Loop B ---
	for _, t := range titles {
		if vetoed(t, r.Vetoes) {
			continue
		}
		r.Candidates = append(r.Candidates, t)
	}

	// --- score using ALL constraints, public and private alike ---
	for _, t := range r.Candidates {
		var score float64
		for _, tl := range r.Tallies {
			if !matches(t, tl.Constraint.Dim, tl.Constraint.Value) {
				continue
			}
			if tl.Constraint.Polarity == vocab.Exclude {
				score -= tl.Weight
			} else {
				score += tl.Weight
			}
		}
		r.Scores[t.TitleID] = score
	}
	sort.SliceStable(r.Candidates, func(i, j int) bool {
		a, b := r.Candidates[i], r.Candidates[j]
		if r.Scores[a.TitleID] != r.Scores[b.TitleID] {
			return r.Scores[a.TitleID] > r.Scores[b.TitleID]
		}
		return a.TitleID < b.TitleID
	})
	return r
}

// PublicPhrases is the ONLY constraint material that reaches Loop B.
func (r Reconciled) PublicPhrases() []string {
	var out []string
	for _, c := range r.Public {
		out = append(out, c.Human())
	}
	return out
}

func vetoed(t store.Title, vs []vocab.Veto) bool {
	for _, v := range vs {
		if matches(t, v.Dim, v.Value) {
			return true
		}
	}
	return false
}

func matches(t store.Title, d vocab.Dim, value string) bool {
	switch d {
	case vocab.DimGenre:
		return t.Genre == value
	case vocab.DimEra:
		return t.Era == value
	case vocab.DimTone:
		return t.Tone == value
	case vocab.DimLanguage:
		return t.Language == value
	case vocab.DimMaturity:
		return t.Maturity == value
	case vocab.DimAvailability:
		return t.Availability == value
	case vocab.DimRuntimeMax:
		n, err := strconv.Atoi(value)
		if err != nil {
			return false
		}
		return t.Runtime <= n
	}
	return false
}
