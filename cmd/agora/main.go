// Command agora runs the whole prototype: one binary, one process, one port.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/arunuke/agora/internal/app"
	"github.com/arunuke/agora/internal/httpapi"
)

func main() {
	var (
		addr     = flag.String("addr", envOr("AGORA_ADDR", ":8080"), "listen address")
		db       = flag.String("db", envOr("AGORA_DB", "/data/agora.db"), "sqlite file")
		seedDir  = flag.String("seed", envOr("AGORA_SEED", "seed"), "seed directory")
		k        = flag.Int("k", 2, "anonymity threshold: 1 = cloud parity (anonymity off), 2 = family default")
		deadline = flag.Duration("member-deadline", 2*time.Second, "per-member consult deadline")
	)
	flag.Parse()

	ctx := context.Background()
	a, err := app.New(ctx, app.Options{
		DBPath: *db, SeedDir: *seedDir, K: *k, Deadline: *deadline,
	})
	if err != nil {
		log.Fatalf("agora: startup failed: %v", err)
	}
	defer a.Close()

	// Startup degradation is part of the reliability story: if the sqlite-vec
	// extension is unavailable the process still boots, logs it, and runs on
	// tiers 1 and 3 with tier 2 simply never offered.
	log.Printf("agora: store=%s vectors=%v (%s)", *db, a.Store.VectorsOK(), a.Store.VectorNote())
	log.Printf("agora: anonymity policy %+v", a.Arbiter.Policy())
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

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
