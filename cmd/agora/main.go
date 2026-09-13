// Command agora runs the whole prototype: one binary, one process, one port.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/arunuke/agora/internal/app"
	"github.com/arunuke/agora/internal/arbiter"
	"github.com/arunuke/agora/internal/httpapi"
	"github.com/arunuke/agora/internal/llm"
)

func main() {
	var (
		addr    = flag.String("addr", envOr("AGORA_ADDR", ":8080"), "listen address")
		db      = flag.String("db", envOr("AGORA_DB", "/data/agora.db"), "sqlite file")
		seedDir = flag.String("seed", envOr("AGORA_SEED", "seed"), "seed directory")
		// k MUST honour the environment like every other setting. It is the
		// anonymity threshold, so a silently-ignored AGORA_K is a silently
		// wrong privacy posture that looks like it worked.
		k        = flag.Int("k", envOrInt("AGORA_K", 2), "anonymity threshold: 1 = cloud parity (anonymity off), 2 = family default")
		deadline = flag.Duration("member-deadline", arbiter.DefaultMemberDeadline, "per-member consult deadline")
	)
	flag.Parse()

	ctx := context.Background()
	// Provider selection is a CHAIN, tried in order:
	//
	//   1. a local model  (Ollama on :11434, or any AGORA_LLM_BASE) — free
	//   2. ANTHROPIC_API_KEY                                        — billed
	//   3. the deterministic extractor                              — always there
	//
	// One image then behaves correctly in both places it runs, with no
	// per-environment configuration: a developer box ships neither a model nor
	// a key and lands on the simulated extractor; the deployed host has a key
	// and no Ollama, so it lands on Anthropic. Nothing silently costs money on
	// a laptop, and nothing silently runs rule-based extraction in production.
	//
	// AGORA_LLM_PREFER overrides the chain and pins one rung. A pinned provider
	// that cannot be used does NOT walk down the chain — it logs loudly and
	// serves the extractor, because quietly answering from a different provider
	// than the one demanded is how a run comes back green and meaningless.
	//
	// Every outcome is logged and served on /healthz, because "is this actually
	// talking to a model?" must be answerable from outside the process.
	var llmClient llm.LLMClient
	var providerDesc string
	prefer := strings.ToLower(strings.TrimSpace(envOr("AGORA_LLM_PREFER", "")))
	// Why we ended up on the extractor, in the words of the path actually
	// taken. A fixed sentence here would claim "no ANTHROPIC_API_KEY" to
	// someone who has one and asked us not to use it.
	whySimulated := "no local model on :11434, no AGORA_LLM_BASE, and no ANTHROPIC_API_KEY"

	switch prefer {
	case "anthropic":
		if live := llm.NewLiveFromEnv(); live != nil {
			llmClient = live
			providerDesc = fmt.Sprintf("anthropic model=%s key=%s", live.Model, live.KeyFingerprint())
		} else {
			log.Printf("agora: AGORA_LLM_PREFER=anthropic but ANTHROPIC_API_KEY is " +
				"not set. Serving the deterministic extractor — this run proves " +
				"nothing about the Anthropic path.")
			whySimulated = "AGORA_LLM_PREFER=anthropic could not be honoured"
		}

	case "ollama", "local", "compat", "openai":
		if cp := localClient(ctx, true); cp != nil {
			llmClient = cp
			providerDesc = cp.Describe()
		} else {
			log.Printf("agora: AGORA_LLM_PREFER=%s but no local model is usable. "+
				"Serving the deterministic extractor — this run proves nothing "+
				"about the local-model path.", prefer)
			whySimulated = "AGORA_LLM_PREFER=" + prefer + " could not be honoured"
		}

	case "simulated", "deterministic", "none", "off":
		whySimulated = "requested explicitly via AGORA_LLM_PREFER=" + prefer

	default:
		if prefer != "" {
			log.Printf("agora: AGORA_LLM_PREFER=%q is not recognised (want: ollama, "+
				"anthropic, simulated). Falling back to the normal chain.", prefer)
		}
		// Rung 1: a local model, if one is actually answering.
		if cp := localClient(ctx, false); cp != nil {
			llmClient = cp
			providerDesc = cp.Describe()
		} else if live := llm.NewLiveFromEnv(); live != nil {
			// Rung 2.
			llmClient = live
			providerDesc = fmt.Sprintf("anthropic model=%s key=%s", live.Model, live.KeyFingerprint())
		}
		// Rung 3 needs no construction: app.New installs the extractor.
	}

	if _, isCompat := llmClient.(*llm.Compat); isCompat && os.Getenv("ANTHROPIC_API_KEY") != "" {
		providerDesc += "  [ANTHROPIC_API_KEY present but unused — " +
			"a local model is cheaper; set AGORA_LLM_PREFER=anthropic to override]"
	}

	a, err := app.New(ctx, app.Options{
		DBPath: *db, SeedDir: *seedDir, K: *k, Deadline: *deadline, Live: llmClient,
	})
	if err != nil {
		log.Fatalf("agora: startup failed: %v", err)
	}
	defer a.Close()

	// Startup degradation is part of the reliability story: if the sqlite-vec
	// extension is unavailable the process still boots, logs it, and runs on
	// tiers 1 and 3 with tier 2 simply never offered.
	log.Printf("agora: store=%s vectors=%v (%s)", *db, a.Store.VectorsOK(), a.Store.VectorNote())
	if llmClient != nil {
		log.Printf("agora: LLM provider=%s", providerDesc)
	} else {
		log.Printf("agora: LLM provider=deterministic (simulated) — %s. "+
			"Tier 1 extraction is rule-based, not a model.", whySimulated)
	}
	// Echoed loudly, and also served on /healthz. See the comment there.
	log.Printf("agora: ANONYMITY POLICY %+v  (k=%d from --k/AGORA_K)", a.Arbiter.Policy(), *k)
	log.Printf("agora: listening on %s", *addr)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(a),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// localClient returns a usable local/compat client, or nil.
//
// The probe budget differs by intent, and that difference matters. An
// AGORA_LLM_BASE somebody typed is worth waiting on and worth complaining
// about when it fails. The unconfigured probe, by contrast, runs on EVERY
// startup in every environment — including a deployed host that will never
// have Ollama — so it must cost approximately nothing and stay silent.
func localClient(ctx context.Context, pinned bool) *llm.Compat {
	cp := llm.NewCompatFromEnv()
	configured := cp != nil
	budget := 400 * time.Millisecond
	if configured {
		budget = 5 * time.Second
	} else {
		cp = llm.NewLocalDefault()
	}

	pctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// Resolves the model name AND proves reachability. Both must happen here:
	// past this point a failure is indistinguishable from the chaos injection
	// the degradation tiers exist to absorb.
	if err := cp.EnsureModel(pctx); err != nil {
		if configured || pinned {
			log.Printf("agora: local provider at %s is unusable: %v", cp.BaseURL, err)
		}
		return nil
	}
	return cp
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envOrInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("agora: %s=%q is not an integer, using %d", k, v, def)
	}
	return def
}
