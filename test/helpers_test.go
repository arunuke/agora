package test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/arunuke/agora/internal/app"
)

const seedDir = "../seed"

// newApp gives each test its own store file. Every test runs against
// llm.Deterministic — a gate that depends on a network call is not a gate.
//
// AGORA_K overrides the threshold so `make cloud-parity` can run the entire
// suite with anonymity descoped, verifying continuously that the seam still
// works rather than discovering it at hour six.
func newApp(t *testing.T, k int) *app.App {
	t.Helper()
	if v := os.Getenv("AGORA_K"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			k = n
		}
	}
	return newAppK(t, k)
}

// newAppK ignores AGORA_K. Used by the test that compares the two policies
// against each other, which needs both to exist regardless of the environment.
func newAppK(t *testing.T, k int) *app.App {
	t.Helper()
	a, err := app.New(context.Background(), app.Options{
		DBPath:   filepath.Join(t.TempDir(), "agora.db"),
		SeedDir:  seedDir,
		K:        k,
		Deadline: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func ctx() context.Context { return context.Background() }
