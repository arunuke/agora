// Package app wires the whole prototype together: one store, one agent
// package, one arbiter, one clock, one set of chaos switches.
//
// Agent and arbiter are packages in a single binary and a single process.
// There is no supervisor and no wire between them — the boundary that matters
// is the arbiter-owned MemberAgent interface, not a process boundary.
package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arunuke/agora/internal/agent"
	"github.com/arunuke/agora/internal/arbiter"
	"github.com/arunuke/agora/internal/clock"
	"github.com/arunuke/agora/internal/llm"
	"github.com/arunuke/agora/internal/store"
)

type App struct {
	Store   *store.Store
	Agent   *agent.Agent
	Arbiter *arbiter.Arbiter
	Clock   *clock.Simulated
	Switch  *llm.Switches
	Emb     llm.Embedder
	SeedDir string
	GroupID string
	// Provider names the active completion provider for /healthz. Never the key.
	Provider string
	Model    string
}

type Options struct {
	DBPath   string
	SeedDir  string
	K        int
	Deadline time.Duration
	// Live, when non-nil, replaces the deterministic completion provider.
	Live llm.LLMClient
}

func New(ctx context.Context, o Options) (*App, error) {
	if o.Deadline == 0 {
		o.Deadline = 2 * time.Second
	}
	st, err := store.Open(o.DBPath)
	if err != nil {
		return nil, err
	}

	sw := llm.NewSwitches()

	var base llm.LLMClient = llm.Deterministic{}
	provider, model := "deterministic", "n/a"
	if o.Live != nil {
		base = o.Live
		switch p := o.Live.(type) {
		case *llm.Live:
			provider, model = "anthropic", p.Model
		case *llm.Compat:
			provider, model = "openai-compatible", p.Model
			if strings.Contains(p.BaseURL, "11434") {
				provider = "ollama"
			}
		default:
			provider = "custom"
		}
	}
	// Two decorators over two SEPARATE interfaces. That separation is what
	// lets chaos fail completions while leaving embeddings healthy, which is
	// precisely the tier-2 demonstration.
	client := llm.ChaosLLM{Inner: base, Sw: sw}
	emb := llm.ChaosEmbedder{Inner: llm.DeterministicEmbedder{}, Sw: sw}

	ag := agent.New(st, client, emb)
	clk := clock.NewSimulated()
	arb := arbiter.New(st, ag, client, clk, sw, arbiter.Config{
		Policy:   arbiter.PolicyForK(o.K),
		Deadline: o.Deadline,
	})

	a := &App{
		Store: st, Agent: ag, Arbiter: arb, Clock: clk, Switch: sw,
		Emb: emb, SeedDir: o.SeedDir, Provider: provider, Model: model,
	}
	if _, err := a.Reset(ctx); err != nil {
		st.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) Close() error { return a.Store.Close() }

// Reset restores seed state and rewinds the simulated clock. It backs both
// `make data` and POST /v1/demo/reset.
func (a *App) Reset(ctx context.Context) (store.SeedResult, error) {
	a.Switch.Reset()
	a.Clock.Reset()
	res, err := a.Store.Seed(ctx, a.SeedDir, a.Emb)
	if err != nil {
		return res, err
	}
	a.GroupID = res.GroupID
	return res, nil
}

// SeedCanaries returns each member's raw preference strings straight from the
// seed FILE — not from the private store.
//
// The isolation gate needs ground truth to match responses against, and taking
// it from the file rather than from rawctx keeps the private zone closed even
// to the test that verifies it.
func (a *App) SeedCanaries() (map[string][]string, error) {
	b, err := os.ReadFile(filepath.Join(a.SeedDir, "members.json"))
	if err != nil {
		return nil, err
	}
	var f struct {
		Members []struct {
			MemberID   string   `json:"member_id"`
			RawContext []string `json:"raw_context"`
		} `json:"members"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, m := range f.Members {
		out[m.MemberID] = m.RawContext
	}
	return out, nil
}

func (a *App) MemberIDs() ([]string, error) {
	ms, err := a.Store.Members(a.GroupID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range ms {
		out = append(out, m.MemberID)
	}
	return out, nil
}

func (a *App) DisplayNames() ([]string, error) {
	ms, err := a.Store.Members(a.GroupID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range ms {
		out = append(out, m.DisplayName)
	}
	return out, nil
}
