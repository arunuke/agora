package test

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/arunuke/agora/internal/arbiter"
	"github.com/arunuke/agora/internal/store"
	"github.com/arunuke/agora/internal/vocab"
)

// Reconciliation is deterministic and vector-free, which makes it
// property-testable. These two properties are cheap to write and catch a bug
// class that reading the code does not.

func testSignals() []arbiter.Signal {
	return []arbiter.Signal{
		{SignalID: "s1", Constraints: []vocab.Constraint{
			{Dim: vocab.DimGenre, Value: "comedy", Polarity: vocab.Prefer, Weight: 0.6},
			{Dim: vocab.DimRuntimeMax, Value: "90", Polarity: vocab.Prefer, Weight: 0.8},
		}},
		{SignalID: "s2", Constraints: []vocab.Constraint{
			{Dim: vocab.DimGenre, Value: "comedy", Polarity: vocab.Prefer, Weight: 0.9},
			{Dim: vocab.DimTone, Value: "intense", Polarity: vocab.Prefer, Weight: 0.7},
		}},
		{SignalID: "s3", Constraints: []vocab.Constraint{
			{Dim: vocab.DimGenre, Value: "documentary", Polarity: vocab.Prefer, Weight: 0.9},
		}, Vetoes: []vocab.Veto{{Dim: vocab.DimGenre, Value: "horror"}}},
		{SignalID: "s4", Constraints: []vocab.Constraint{
			{Dim: vocab.DimRuntimeMax, Value: "90", Polarity: vocab.Prefer, Weight: 0.5},
		}},
	}
}

func testTitles() []store.Title {
	return []store.Title{
		{TitleID: "a", Title: "A", Genre: "comedy", Runtime: 85, Tone: "light"},
		{TitleID: "b", Title: "B", Genre: "horror", Runtime: 88, Tone: "intense"},
		{TitleID: "c", Title: "C", Genre: "documentary", Runtime: 95, Tone: "cozy"},
		{TitleID: "d", Title: "D", Genre: "comedy", Runtime: 140, Tone: "intense"},
	}
}

func TestDeterminism_SameInputSameOutput(t *testing.T) {
	p := arbiter.FamilyDefault()
	first := arbiter.Reconcile(testSignals(), testTitles(), p)
	for i := 0; i < 20; i++ {
		got := arbiter.Reconcile(testSignals(), testTitles(), p)
		if !reflect.DeepEqual(first.Public, got.Public) {
			t.Fatalf("public constraints not deterministic:\n %v\n %v", first.Public, got.Public)
		}
		if !reflect.DeepEqual(first.Scores, got.Scores) {
			t.Fatalf("scores not deterministic")
		}
		if !reflect.DeepEqual(ids(first.Candidates), ids(got.Candidates)) {
			t.Fatalf("candidate order not deterministic")
		}
	}
}

// SHUFFLE-INVARIANCE does double duty: it is a determinism test AND an
// anonymity test. If output depended on signal order, and order correlated
// with member index, the system would have an ordering side channel that
// name-stripping does nothing to close.
func TestDeterminism_ShuffleInvariance(t *testing.T) {
	p := arbiter.FamilyDefault()
	base := arbiter.Reconcile(testSignals(), testTitles(), p)

	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 50; i++ {
		s := testSignals()
		rng.Shuffle(len(s), func(a, b int) { s[a], s[b] = s[b], s[a] })
		got := arbiter.Reconcile(s, testTitles(), p)

		if !reflect.DeepEqual(base.Public, got.Public) {
			t.Fatalf("permutation %d changed the public set:\n base %v\n got  %v",
				i, base.Public, got.Public)
		}
		if !reflect.DeepEqual(base.Scores, got.Scores) {
			t.Fatalf("permutation %d changed scores", i)
		}
		if !reflect.DeepEqual(ids(base.Candidates), ids(got.Candidates)) {
			t.Fatalf("permutation %d changed candidate order:\n base %v\n got  %v",
				i, ids(base.Candidates), ids(got.Candidates))
		}
	}
}

func TestReconcile_ThresholdClassification(t *testing.T) {
	r := arbiter.Reconcile(testSignals(), testTitles(), arbiter.FamilyDefault())

	public := map[string]bool{}
	for _, c := range r.Public {
		public[c.Key()] = true
	}
	// comedy: 2 signals -> public. runtime 90: 2 signals -> public.
	if !public["genre:comedy:prefer"] {
		t.Error("comedy held by 2 signals must be public at k=2")
	}
	if !public["runtime_max_min:90:prefer"] {
		t.Error("runtime<=90 held by 2 signals must be public at k=2")
	}
	// documentary and intense: 1 signal each -> private.
	for _, k := range []string{"genre:documentary:prefer", "tone:intense:prefer"} {
		if public[k] {
			t.Errorf("%s is held by one signal and must stay private", k)
		}
	}
}

func TestReconcile_VetoFiltersBeforeScoring(t *testing.T) {
	r := arbiter.Reconcile(testSignals(), testTitles(), arbiter.FamilyDefault())
	for _, c := range r.Candidates {
		if c.Genre == "horror" {
			t.Errorf("vetoed genre reached the candidate set: %s", c.TitleID)
		}
	}
	if _, ok := r.Scores["b"]; ok {
		t.Error("a vetoed title must not even be scored — it is removed before Loop B")
	}
}

// Private constraints still influence ranking. Suppressing a constraint from
// the EXPLANATION must not suppress it from the DECISION — that is the whole
// bargain the k-threshold makes.
func TestReconcile_PrivateConstraintsStillInfluenceRanking(t *testing.T) {
	r := arbiter.Reconcile(testSignals(), testTitles(), arbiter.FamilyDefault())
	if r.Scores["c"] <= 0 {
		t.Errorf("documentary is private but must still score: got %v", r.Scores["c"])
	}
	found := false
	for _, c := range r.Private {
		if c.Key() == "genre:documentary:prefer" {
			found = true
		}
	}
	if !found {
		t.Error("documentary should be classified private, not dropped")
	}
}

func TestReconcile_CloudParityMakesEverythingSpeakable(t *testing.T) {
	fam := arbiter.Reconcile(testSignals(), testTitles(), arbiter.FamilyDefault())
	cloud := arbiter.Reconcile(testSignals(), testTitles(), arbiter.CloudParity())
	if len(cloud.Public) <= len(fam.Public) {
		t.Errorf("k=1 must make strictly more speakable: %d vs %d",
			len(cloud.Public), len(fam.Public))
	}
	if len(cloud.Private) != 0 {
		t.Errorf("at k=1 nothing is below threshold, got %d private", len(cloud.Private))
	}
	// The veto filter is a correctness feature and applies under every policy.
	for _, c := range cloud.Candidates {
		if c.Genre == "horror" {
			t.Error("the veto filter must apply at k=1 too — only its silence is an anonymity feature")
		}
	}
}

func ids(ts []store.Title) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.TitleID
	}
	return out
}
