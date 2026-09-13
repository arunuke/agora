// Package rawctx is the compiler-enforced private zone.
//
// Go's visibility rules make a package under internal/agent/internal/
// importable ONLY by packages rooted at internal/agent/. A member's raw
// preference text and profile vector are reachable through this package and
// nowhere else, so the arbiter CANNOT COMPILE if it tries to reach them.
//
// The isolation guarantee is therefore a build error rather than a
// code-review convention.
package rawctx

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/arunuke/agora/internal/vocab"
)

// Derived is the reduced, closed-vocabulary form of a member's context. It is
// what a sealed signal is built from; the raw text never leaves this package.
type Derived struct {
	Constraints []vocab.Constraint `json:"constraints"`
	Vetoes      []vocab.Veto       `json:"vetoes"`
	Tier        int                `json:"tier"`
}

// Scope binds every read and write to exactly one member. There is no API here
// that takes a member id at call time, so a scope cannot be pointed at someone
// else after construction.
type Scope struct {
	db       *sql.DB
	memberID string
}

func For(db *sql.DB, memberID string) *Scope { return &Scope{db: db, memberID: memberID} }

func (s *Scope) MemberID() string { return s.memberID }

var ErrNoProfile = errors.New("rawctx: no profile for member")

func (s *Scope) Load() ([]string, Derived, error) {
	var rawJSON, derivedJSON string
	err := s.db.QueryRow(
		`select raw_context, derived from profiles where member_id=?`, s.memberID).
		Scan(&rawJSON, &derivedJSON)
	if err == sql.ErrNoRows {
		return nil, Derived{}, ErrNoProfile
	}
	if err != nil {
		return nil, Derived{}, err
	}
	var raw []string
	_ = json.Unmarshal([]byte(rawJSON), &raw)
	var d Derived
	_ = json.Unmarshal([]byte(derivedJSON), &d)
	return raw, d, nil
}

func (s *Scope) AppendRaw(text string) error {
	raw, _, err := s.Load()
	if err != nil {
		return err
	}
	raw = append(raw, text)
	b, _ := json.Marshal(raw)
	_, err = s.db.Exec(`update profiles set raw_context=?, updated_at=? where member_id=?`,
		string(b), time.Now().UTC().Format(time.RFC3339), s.memberID)
	return err
}

func (s *Scope) SaveDerived(d Derived) error {
	b, _ := json.Marshal(d)
	_, err := s.db.Exec(`update profiles set derived=?, updated_at=? where member_id=?`,
		string(b), time.Now().UTC().Format(time.RFC3339), s.memberID)
	return err
}

// VectorWriter is satisfied by *store.Store. Taking an interface rather than
// the concrete store keeps this package free of an import cycle while still
// making the profile vector reachable only from here.
type VectorWriter interface {
	UpsertProfileVector(id string, v []float32) error
}

func (s *Scope) SaveVector(w VectorWriter, v []float32) error {
	return w.UpsertProfileVector(s.memberID, v)
}
