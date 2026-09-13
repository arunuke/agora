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
	return ReconcileWith(signals, titles, p, nil)
}

// ReconcileWith adds constraints that came from the REQUEST rather than from
// anyone's profile — the occasion in "schedule a christmas marathon".
//
// They are treated as public because that is what they are: the group asked for
// them out loud. They never touch the tally, so they cannot help a private
// constraint over the anonymity threshold.
func ReconcileWith(signals []Signal, titles []store.Title, p AnonymityPolicy, requested []vocab.Constraint) Reconciled {
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
	// Requested constraints join the public set AFTER classification, so they
	// are speakable in the justification without ever being counted toward k.
	for _, c := range requested {
		r.Public = append(r.Public, c)
	}

	// --- veto filter: silent, and BEFORE scoring or Loop B ---
	for _, t := range titles {
		if vetoed(t, r.Vetoes) {
			continue
		}
		r.Candidates = append(r.Candidates, t)
	}

	// --- occasion filter: a season is a FILTER, not a preference ------------
	//
	// "Something christmassy" means only christmas films. As a weight it merely
	// nudged, and a member with strong standing preferences got two autumn
	// films and one christmas one — an answer that ignored the only thing they
	// actually asked for.
	//
	// Deliberately driven by PUBLIC constraints only. A below-threshold
	// occasion applied as a hard filter would make every title in the slate
	// christmas, which tells the whole group that somebody asked for christmas
	// — anonymity undone by inference rather than by quotation. Vetoes can
	// afford to be silent filters because their evidence is an ABSENCE; an
	// occasion filter's evidence is everything that remains.
	//
	// In the single-signal case — a member's own recommendations, scored under
	// cloud parity — everything is public and their own request filters freely.
	if wanted := occasionsIn(r.Public); len(wanted) > 0 {
		var kept []store.Title
		for _, t := range r.Candidates {
			if t.Occasion != "" && wanted[t.Occasion] {
				kept = append(kept, t)
			}
		}
		// An empty result is reported as empty rather than silently widened: a
		// christmas request answered with non-christmas films is a worse
		// failure than one answered with "nothing matches".
		r.Candidates = kept
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

// occasionsIn collects requested seasons. Only Prefer counts: "not christmas"
// is an exclusion to score against, never a filter to select by.
func occasionsIn(cs []vocab.Constraint) map[string]bool {
	out := map[string]bool{}
	for _, c := range cs {
		if c.Dim == vocab.DimOccasion && c.Polarity == vocab.Prefer {
			out[c.Value] = true
		}
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
	case vocab.DimOccasion:
		// An untagged title matches no occasion. Treating "" as a wildcard would
		// make "something christmassy" rank the whole catalogue.
		return t.Occasion != "" && t.Occasion == value
	case vocab.DimRuntimeMax:
		n, err := strconv.Atoi(value)
		if err != nil {
			return false
		}
		return t.Runtime <= n
	}
	return false
}
