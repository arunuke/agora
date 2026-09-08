package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/arunuke/agora/internal/clock"
	"github.com/arunuke/agora/internal/llm"
	"github.com/arunuke/agora/internal/vocab"
)

type seedMember struct {
	MemberID    string   `json:"member_id"`
	DisplayName string   `json:"display_name"`
	RawContext  []string `json:"raw_context"`
}

type seedMembersFile struct {
	GroupID string       `json:"group_id"`
	Members []seedMember `json:"members"`
}

type seedCatalogFile struct {
	Titles []Title `json:"titles"`
}

type seedEvent struct {
	EventID      string `json:"event_id"`
	TitleID      string `json:"title_id"`
	Label        string `json:"label"`
	StartOffsetH int    `json:"start_offset_h"`
	EndOffsetH   int    `json:"end_offset_h"`
}

type seedEventsFile struct {
	Events []seedEvent `json:"events"`
}

// SeedResult reports what a reset produced, so the walkthrough transcript can
// state the ground truth a reviewer is about to test against.
type SeedResult struct {
	GroupID string   `json:"group_id"`
	Members []string `json:"members"`
	Titles  int      `json:"titles"`
	Events  int      `json:"events"`
	Vectors bool     `json:"vectors_enabled"`
}

// Seed wipes and reloads all state. It is the implementation of both `make
// data` and POST /v1/demo/reset.
func (s *Store) Seed(ctx context.Context, dir string, emb llm.Embedder) (SeedResult, error) {
	var res SeedResult

	for _, t := range []string{"members", "profiles", "titles", "events", "convenes", "notifications"} {
		if _, err := s.db.Exec("delete from " + t); err != nil {
			return res, err
		}
	}
	if s.vecOK {
		for _, t := range []string{"profile_vectors", "catalog_vectors", "vocab_vectors"} {
			if _, err := s.db.Exec("delete from " + t); err != nil {
				return res, err
			}
		}
	}

	var mf seedMembersFile
	if err := readJSON(filepath.Join(dir, "members.json"), &mf); err != nil {
		return res, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, m := range mf.Members {
		if _, err := s.db.Exec(
			`insert into members(member_id,display_name,group_id) values(?,?,?)`,
			m.MemberID, m.DisplayName, mf.GroupID); err != nil {
			return res, err
		}
		raw, _ := json.Marshal(m.RawContext)
		if _, err := s.db.Exec(
			`insert into profiles(member_id,raw_context,derived,updated_at) values(?,?,'{}',?)`,
			m.MemberID, string(raw), now); err != nil {
			return res, err
		}
		res.Members = append(res.Members, m.MemberID)
	}
	res.GroupID = mf.GroupID

	var cf seedCatalogFile
	if err := readJSON(filepath.Join(dir, "catalog.json"), &cf); err != nil {
		return res, err
	}
	for _, t := range cf.Titles {
		if _, err := s.db.Exec(`insert into titles(title_id,title,genre,era,runtime,language,
			maturity,tone,availability,synopsis) values(?,?,?,?,?,?,?,?,?,?)`,
			t.TitleID, t.Title, t.Genre, t.Era, t.Runtime, t.Language, t.Maturity,
			t.Tone, t.Availability, t.Synopsis); err != nil {
			return res, err
		}
	}
	res.Titles = len(cf.Titles)

	var ef seedEventsFile
	if err := readJSON(filepath.Join(dir, "events.json"), &ef); err != nil {
		return res, err
	}
	for _, e := range ef.Events {
		ws := clock.Base.Add(time.Duration(e.StartOffsetH) * time.Hour)
		we := clock.Base.Add(time.Duration(e.EndOffsetH) * time.Hour)
		if _, err := s.db.Exec(
			`insert into events(event_id,title_id,label,window_start,window_end) values(?,?,?,?,?)`,
			e.EventID, e.TitleID, e.Label, ws.Format(time.RFC3339), we.Format(time.RFC3339)); err != nil {
			return res, err
		}
	}
	res.Events = len(ef.Events)

	// Embed the closed vocabulary once. This is what makes tier 2 of the
	// degradation ladder possible: a phrase can be mapped onto the vocabulary
	// by nearest neighbour with no completion call.
	if s.vecOK && emb != nil {
		terms := vocab.Terms()
		texts := make([]string, len(terms))
		for i, t := range terms {
			texts[i] = t.EmbedText()
		}
		vecs, err := emb.Embed(ctx, texts)
		if err == nil {
			for i, t := range terms {
				if err := s.UpsertVocabVector(string(t.Dim)+":"+t.Value, vecs[i]); err != nil {
					return res, err
				}
			}
		}
		// Catalogue vectors power semantic search inside Loop A.
		ctexts := make([]string, len(cf.Titles))
		for i, t := range cf.Titles {
			ctexts[i] = fmt.Sprintf("%s %s %s %s %s", t.Title, t.Genre, t.Tone, t.Era, t.Synopsis)
		}
		cvecs, err := emb.Embed(ctx, ctexts)
		if err == nil {
			for i, t := range cf.Titles {
				if err := s.UpsertCatalogVector(t.TitleID, cvecs[i]); err != nil {
					return res, err
				}
			}
		}
	}
	res.Vectors = s.vecOK
	return res, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return json.Unmarshal(b, v)
}
