package test

import (
	"strings"
	"testing"

	"github.com/arunuke/agora/internal/vocab"
)

// ANONYMITY GATE — guarded on policy.GateJustifications.
//
// Separate file from the isolation gate on purpose: this one is skipped when
// anonymity is descoped (K=1), and the isolation gate is not. Sharing a file
// would mean cutting anonymity broke isolation too, and the descope would stop
// being a one-line change.

func TestAnonymityGate_JustificationNamesNoMember(t *testing.T) {
	a := newApp(t, 2)
	if !a.Arbiter.Policy().GateJustifications {
		t.Skip("anonymity descoped (K=1): justification gating is off by policy")
	}
	names, err := a.DisplayNames()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if strings.Contains(strings.ToLower(res.Justification), strings.ToLower(n)) {
			t.Errorf("justification attributed a constraint to %s: %q", n, res.Justification)
		}
	}
}

// The inference attack the k-threshold exists to defeat.
//
// Stripping names does not stop a member reasoning "the slate mentions Korean
// horror, I didn't ask for it, and I know the others — that's Priya." Rarity
// does the identifying, so nothing below threshold may be spoken.
//
// Derived from the vocabulary rather than a hand-written list of expected
// singletons: an earlier version hardcoded the list and flagged a legitimately
// public constraint. The assertion has to follow the policy's classification.
func TestAnonymityGate_NoBelowThresholdConstraintIsSpoken(t *testing.T) {
	a := newApp(t, 2)
	if !a.Arbiter.Policy().GateJustifications {
		t.Skip("anonymity descoped (K=1)")
	}
	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	public := map[string]bool{}
	for _, p := range res.PublicConstraints {
		public[strings.ToLower(p)] = true
	}
	for _, term := range vocab.Terms() {
		if public[strings.ToLower(term.Phrase)] {
			continue
		}
		if strings.Contains(strings.ToLower(res.Justification), strings.ToLower(term.Phrase)) {
			t.Errorf("justification spoke a below-threshold constraint %q\n  justification: %q\n  public: %v",
				term.Phrase, res.Justification, res.PublicConstraints)
		}
	}
}

// A veto is held by one member, so it is permanently below threshold and can
// never be spoken — yet must be honoured absolutely. Enforcement is total and
// explanation is impossible, because the vetoed titles are removed before
// Loop B is given the candidate set.
func TestAnonymityGate_VetoIsHonouredAndUnexplainable(t *testing.T) {
	a := newApp(t, 2)
	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	titles, err := a.Store.Titles()
	if err != nil {
		t.Fatal(err)
	}
	genre := map[string]string{}
	for _, ti := range titles {
		genre[ti.TitleID] = ti.Genre
	}
	for _, item := range res.Slate {
		if genre[item.TitleID] == "horror" {
			t.Errorf("veto not honoured: %q is horror and reached the slate", item.Title)
		}
	}
	if a.Arbiter.Policy().RevealVetoes {
		return
	}
	for _, w := range []string{"horror", "veto", "excluded", "can't watch", "cannot watch"} {
		if strings.Contains(strings.ToLower(res.Justification), w) {
			t.Errorf("veto was explained (%q), which re-identifies the member who holds it: %q",
				w, res.Justification)
		}
	}
}

// With fewer than k responders nothing can reach the threshold, so the
// justification must go fully generic. This falls out of k being ABSOLUTE
// rather than relative to responders, and it is the correct behaviour: with
// one responder any named constraint is attributable to them.
func TestAnonymityGate_SingleResponderYieldsGenericJustification(t *testing.T) {
	a := newApp(t, 2)
	if !a.Arbiter.Policy().GateJustifications {
		// At K=1 a lone responder's constraints ARE all speakable. That is
		// cloud parity working as intended, not a regression: in the source
		// domain attribution is acceptable.
		t.Skip("anonymity descoped (K=1)")
	}
	ids, err := a.MemberIDs()
	if err != nil {
		t.Fatal(err)
	}
	a.Switch.SetMemberFail(len(ids) - 1) // only the last member answers
	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if res.Quorum.Represented != 1 {
		t.Fatalf("expected 1 responder, got %d", res.Quorum.Represented)
	}
	if len(res.PublicConstraints) != 0 {
		t.Errorf("with a single responder no constraint may be public, got %v",
			res.PublicConstraints)
	}
}

// The descope seam: at K=1 the system must still work, still not name members,
// and simply speak more. `make cloud-parity` runs the whole suite this way so
// the seam is verified continuously rather than discovered at hour six.
func TestAnonymityGate_CloudParityDescopeIsClean(t *testing.T) {
	a := newAppK(t, 1)
	if a.Arbiter.Policy().GateJustifications {
		t.Fatal("K=1 must disable justification gating")
	}
	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Slate) == 0 {
		t.Fatal("cloud parity produced no slate")
	}
	// Layer 2 (identity stripping) is NOT part of the cut, so names must still
	// never appear even with anonymity descoped.
	names, _ := a.DisplayNames()
	for _, n := range names {
		if strings.Contains(strings.ToLower(res.Justification), strings.ToLower(n)) {
			t.Errorf("K=1 must not attribute: %q named in %q", n, res.Justification)
		}
	}
	// And it should be strictly more talkative than the family policy.
	fam := newAppK(t, 2)
	famRes, err := fam.Arbiter.Convene(ctx(), fam.GroupID, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.PublicConstraints) < len(famRes.PublicConstraints) {
		t.Errorf("K=1 should speak at least as much as K=2: %d vs %d",
			len(res.PublicConstraints), len(famRes.PublicConstraints))
	}
}
