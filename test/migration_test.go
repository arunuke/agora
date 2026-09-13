package test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arunuke/agora/internal/llm"
	"github.com/arunuke/agora/internal/store"
)

// A deployed host reuses its data volume across deploys, so it opens a database
// created by an older build. `create table if not exists` leaves that table
// alone, which means a column added later is simply absent and the first insert
// fails at startup — the container never becomes healthy and the deploy fails
// after the image has already been transferred.
//
// Nothing local reproduces this: `make pipeline` runs `docker compose down -v`
// and starts from an empty volume every single time. This test is the only
// place the upgrade path exists.
func TestMigration_OlderDatabaseGainsNewColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Rewind to the previous release's shape.
	for _, table := range []string{"titles", "convenes"} {
		if _, err := st.DB().Exec("alter table " + table + " drop column occasion"); err != nil {
			t.Fatalf("rewinding %s: %v", table, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening is what a deploy does. It must migrate, not fail.
	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopening an older database: %v", err)
	}
	defer st2.Close()

	for _, table := range []string{"titles", "convenes"} {
		var n int
		if err := st2.DB().QueryRow(
			`select count(*) from pragma_table_info(?) where name='occasion'`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%s.occasion missing after migration", table)
		}
	}

	// And the path that actually broke: seeding writes the new column.
	if _, err := st2.Seed(context.Background(), seedDir, llm.DeterministicEmbedder{}); err != nil {
		t.Fatalf("seeding a migrated database: %v", err)
	}
	titles, err := st2.Titles()
	if err != nil {
		t.Fatal(err)
	}
	var seasonal int
	for _, ti := range titles {
		if ti.Occasion != "" {
			seasonal++
		}
	}
	if seasonal == 0 {
		t.Error("no seasonal titles survived the migration")
	}
}
