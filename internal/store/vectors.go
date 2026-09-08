package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Vector tables are split by TRUST LEVEL, not by convenience.
//
//	profile_vectors  PRIVATE    — one member's derived taste vector
//	catalog_vectors  world-state
//	vocab_vectors    world-state
//
// A vector is not anonymized merely because it is not text: a profile
// embedding is derived but re-identifying and partially invertible, so it
// obeys the same boundary as raw context. Sealed signals therefore never carry
// embeddings — shipping one across the boundary would both re-identify the
// member and collapse the countability argument the k-threshold rests on.
//
// UpsertProfileVector is exported but is only ever reached through the
// member-scoped accessor in internal/agent/internal/rawctx.

func vecLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (s *Store) upsertVector(table, id string, v []float32) error {
	if !s.vecOK {
		return nil
	}
	if _, err := s.db.Exec(fmt.Sprintf(`delete from %s where id=?`, table), id); err != nil {
		return err
	}
	_, err := s.db.Exec(
		fmt.Sprintf(`insert into %s(id, embedding) values(?, vec_f32(?))`, table), id, vecLiteral(v))
	return err
}

type Neighbour struct {
	ID       string
	Distance float64
}

func (s *Store) knn(table string, v []float32, k int) ([]Neighbour, error) {
	if !s.vecOK {
		return nil, nil
	}
	rows, err := s.db.Query(
		fmt.Sprintf(`select id, distance from %s where embedding match vec_f32(?) and k = ?`, table),
		vecLiteral(v), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Neighbour
	for rows.Next() {
		var n Neighbour
		if err := rows.Scan(&n.ID, &n.Distance); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) UpsertCatalogVector(id string, v []float32) error {
	return s.upsertVector("catalog_vectors", id, v)
}
func (s *Store) UpsertVocabVector(id string, v []float32) error {
	return s.upsertVector("vocab_vectors", id, v)
}
func (s *Store) UpsertProfileVector(id string, v []float32) error {
	return s.upsertVector("profile_vectors", id, v)
}

// CatalogKNN powers semantic catalog search inside Loop A — "something cozy
// and autumnal" matches no genre enum but does match synopsis embeddings.
func (s *Store) CatalogKNN(v []float32, k int) ([]Neighbour, error) {
	return s.knn("catalog_vectors", v, k)
}

// VocabKNN powers tier 2 of the degradation ladder: map a phrase onto the
// closed vocabulary without a completion call.
func (s *Store) VocabKNN(v []float32, k int) ([]Neighbour, error) {
	return s.knn("vocab_vectors", v, k)
}

func jsonRaw(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}
