package arbiter

import "github.com/arunuke/agora/internal/vocab"

// This file is the ENTIRE anonymity surface.
//
// No package outside this file may reference K or the public/private
// classification. That invariant is what keeps descoping anonymity a
// one-line change rather than a refactor at hour six.
//
// The cut line runs between layers 2 and 3:
//
//	layer 1  content de-identification  (agent, at emission)   — ISOLATION, never cut
//	layer 2  identity stripping         (workflow, at fan-out) — ISOLATION, never cut
//	layer 3  k-threshold                (here)                 — ANONYMITY, cut by K=1
//
// At K=1 every constraint becomes speakable, but member names still never
// appear because layer 2 stripped identity upstream. Descoping therefore lands
// on exactly cloud-domain behaviour, which is the correct floor.
type AnonymityPolicy struct {
	// K is the domain-translation knob, not a magic number.
	//   K = 1  cloud parity   — attribution acceptable, every constraint speakable
	//   K = 2  family default
	//   K = n  total anonymity
	K int `json:"k"`

	// GateJustifications enforces that nothing below threshold is spoken.
	GateJustifications bool `json:"gate_justifications"`

	// RevealVetoes governs the SILENCE of the veto filter, not the filter
	// itself. The filter is a correctness feature and applies under every
	// policy; only whether it may be explained is an anonymity question.
	RevealVetoes bool `json:"reveal_vetoes"`
}

func FamilyDefault() AnonymityPolicy {
	return AnonymityPolicy{K: 2, GateJustifications: true, RevealVetoes: false}
}

// CloudParity reproduces the source domain, where it is acceptable for Storage
// to know how Networking is configured. Verified continuously by `make
// cloud-parity` so the descope seam never rots.
func CloudParity() AnonymityPolicy {
	return AnonymityPolicy{K: 1, GateJustifications: false, RevealVetoes: true}
}

func PolicyForK(k int) AnonymityPolicy {
	if k <= 1 {
		return CloudParity()
	}
	p := FamilyDefault()
	p.K = k
	return p
}

// Tally is one constraint and how many signals held it. Count is the number of
// distinct signals, never a list of which — the reconciler receives an
// unordered bag and has no member identities to record even if it wanted to.
type Tally struct {
	Constraint vocab.Constraint `json:"constraint"`
	Count      int              `json:"count"`
	Weight     float64          `json:"weight"`
	Public     bool             `json:"public"`
}

// Classify marks each tally public or private.
//
// K is absolute, not relative to the number of responders. That gives a safety
// property for free: if only one member's agent responds, nothing can reach
// the threshold and the justification goes fully generic — which is correct,
// because with one responder any named constraint is attributable to them.
func (p AnonymityPolicy) Classify(tallies []Tally) []Tally {
	out := make([]Tally, len(tallies))
	copy(out, tallies)
	for i := range out {
		out[i].Public = out[i].Count >= p.K
	}
	return out
}

// SpeakableVetoes returns the vetoes that may appear in an explanation.
//
// Under the family policy this is always empty: a veto is held by one member,
// so it is permanently below threshold. It must still be honoured absolutely,
// which is why the FILTER runs regardless and only its explanation is gated.
// The model cannot describe the absence of horror films because it is never
// shown any.
func (p AnonymityPolicy) SpeakableVetoes(v []vocab.Veto) []vocab.Veto {
	if !p.RevealVetoes {
		return nil
	}
	return v
}
