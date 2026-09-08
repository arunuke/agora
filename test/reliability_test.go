package test

import (
	"strings"
	"testing"
	"time"

	"github.com/arunuke/agora/internal/agent"
)

// Table-driven reliability scenarios (US5). Each asserts the response SHAPE
// and that the system neither hangs nor fabricates.

func TestReliability_PartialMemberFailure(t *testing.T) {
	cases := []struct {
		name string
		fail int
	}{
		{"one member fails", 1},
		{"two members fail", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newApp(t, 2)
			ids, _ := a.MemberIDs()
			a.Switch.SetMemberFail(tc.fail)

			res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
			if err != nil {
				t.Fatalf("convene must complete despite member failure: %v", err)
			}
			if got, want := res.Quorum.Represented, len(ids)-tc.fail; got != want {
				t.Errorf("quorum represented = %d, want %d", got, want)
			}
			if !res.Quorum.Provisional {
				t.Error("a reduced-quorum result must be labelled provisional")
			}
			if !strings.Contains(res.Justification, "Provisional") {
				t.Errorf("the response must SAY how many were represented: %q", res.Justification)
			}
			if len(res.Slate) == 0 {
				t.Error("a degraded convene must still produce a slate, not an error")
			}
		})
	}
}

func TestReliability_AllMembersFail(t *testing.T) {
	a := newApp(t, 2)
	ids, _ := a.MemberIDs()
	a.Switch.SetMemberFail(len(ids))

	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatalf("convene must complete: %v", err)
	}
	if res.Quorum.Represented != 0 {
		t.Errorf("represented = %d, want 0", res.Quorum.Represented)
	}
	if len(res.Slate) == 0 {
		t.Error("with no signals the system should return unpersonalised titles, clearly labelled")
	}
	if len(res.PublicConstraints) != 0 {
		t.Error("no signals means no public constraints")
	}
}

// The most valuable test in the suite. A missing context deadline passes every
// other test here and only shows up as a coordinator that hangs at 3am.
func TestReliability_HangingMemberCannotHangTheCoordinator(t *testing.T) {
	a := newApp(t, 2)
	a.Switch.SetMemberFail(1)
	a.Switch.SetMemberHang(true)

	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		if _, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben"); err != nil {
			t.Errorf("convene errored: %v", err)
		}
	}()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("convene took %v — the per-member deadline is not bounding the fan-out", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("convene never returned: a hanging member agent hung the coordinator")
	}
}

func TestReliability_DegradationLadder(t *testing.T) {
	cases := []struct {
		name      string
		llmDown   bool
		embedDown bool
		wantTier  int
	}{
		{"tier 1 — everything healthy", false, false, agent.TierLLM},
		{"tier 2 — completions down, embeddings up", true, false, agent.TierEmbed},
		{"tier 3 — both down", true, true, agent.TierKeyword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newApp(t, 2)
			a.Switch.SetLLMDown(tc.llmDown)
			a.Switch.SetEmbedDown(tc.embedDown)

			res, err := a.Agent.HandleMessage(ctx(), "cruz", "I want something funny and short")
			if err != nil {
				t.Fatalf("must not error: %v", err)
			}
			if res.Tier != tc.wantTier {
				t.Errorf("tier = %d, want %d", res.Tier, tc.wantTier)
			}
			if tc.wantTier != agent.TierLLM && !res.Degraded {
				t.Error("a degraded response must be labelled degraded")
			}
			if strings.TrimSpace(res.Reply) == "" {
				t.Error("degraded must still answer — never a hang, never an empty reply")
			}
		})
	}
}

// Malformed model output is the likeliest accidental leak path, so the rule is
// a privacy control before it is a robustness one: retry once, then fall back,
// and never pass unparsed model text into a justification.
func TestReliability_MalformedOutputNeverReachesTheJustification(t *testing.T) {
	a := newApp(t, 2)
	a.Switch.SetMalformed(true)

	res, err := a.Arbiter.Convene(ctx(), a.GroupID, "ben")
	if err != nil {
		t.Fatalf("malformed output must not fail the convene: %v", err)
	}
	if !res.Degraded {
		t.Error("a fallback-ranked result must be labelled degraded")
	}
	if strings.Contains(res.Justification, "this is not the right shape") {
		t.Errorf("raw model text reached the justification: %q", res.Justification)
	}
	if len(res.Slate) == 0 {
		t.Error("fallback must still produce a slate")
	}
}

// Startup degradation: the process boots even when the vector extension is
// unavailable, and simply never offers tier 2.
func TestReliability_StoreReportsVectorAvailability(t *testing.T) {
	a := newApp(t, 2)
	if note := a.Store.VectorNote(); note == "" {
		t.Error("the store must report vector availability so startup degradation is visible")
	}
}
