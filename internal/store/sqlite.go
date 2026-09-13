// Package store is the single file-backed data store: SQLite for relational
// data and sqlite-vec for context vectors, in one file.
//
// Note what is NOT here: any accessor for a member's raw context or profile
// vector. Those live in internal/agent/internal/rawctx, which Go's visibility
// rules make importable only by packages under internal/agent/. The arbiter
// cannot compile if it reaches for them.
package store

import (
	"database/sql"
	"fmt"
	"time"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"

	"github.com/arunuke/agora/internal/llm"
)

type Member struct {
	MemberID    string `json:"member_id"`
	DisplayName string `json:"display_name"`
	GroupID     string `json:"group_id"`
}

type Title struct {
	TitleID      string `json:"title_id"`
	Title        string `json:"title"`
	Genre        string `json:"genre"`
	Era          string `json:"era"`
	Runtime      int    `json:"runtime"`
	Language     string `json:"language"`
	Maturity     string `json:"maturity"`
	Tone         string `json:"tone"`
	Availability string `json:"availability"`
	// Occasion is empty for the great majority of titles: most films are not
	// seasonal, and an empty value matches no occasion constraint rather than
	// matching all of them.
	Occasion string `json:"occasion"`
	Synopsis string `json:"synopsis"`
}

type Event struct {
	EventID     string    `json:"event_id"`
	TitleID     string    `json:"title_id"`
	Label       string    `json:"label"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

type Convene struct {
	ConveneID string
	GroupID   string
	State     string
	FireAt    *time.Time
	Signals   []byte
	Result    []byte
	CreatedAt time.Time
}

type Notification struct {
	ID       int64          `json:"-"`
	MemberID string         `json:"-"`
	Type     string         `json:"type"`
	Message  string         `json:"message"`
	Data     map[string]any `json:"data,omitempty"`
}

type Store struct {
	db      *sql.DB
	vecOK   bool
	vecNote string
}

// Open initialises the store. If the sqlite-vec extension cannot be loaded the
// store still opens with vector search disabled: the process logs it, tier 2
// is never offered, and tiers 1 and 3 carry on. Startup degradation is part of
// the reliability story, not an excuse to refuse to boot.
func Open(path string) (*Store, error) {
	sqlitevec.Auto()
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer; the prototype is not contention-bound
	s := &Store{db: db}

	var v string
	if err := db.QueryRow("select vec_version()").Scan(&v); err != nil {
		s.vecOK, s.vecNote = false, "sqlite-vec unavailable: "+err.Error()
	} else {
		s.vecOK, s.vecNote = true, "sqlite-vec "+v
	}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error       { return s.db.Close() }
func (s *Store) DB() *sql.DB        { return s.db }
func (s *Store) VectorsOK() bool    { return s.vecOK }
func (s *Store) VectorNote() string { return s.vecNote }

func (s *Store) migrate() error {
	stmts := []string{
		`create table if not exists members(
			member_id text primary key, display_name text not null, group_id text not null)`,
		`create table if not exists profiles(
			member_id text primary key, raw_context text not null default '[]',
			derived text not null default '{}', updated_at text not null)`,
		`create table if not exists titles(
			title_id text primary key, title text, genre text, era text, runtime integer,
			language text, maturity text, tone text, availability text,
			occasion text not null default '', synopsis text)`,
		`create table if not exists events(
			event_id text primary key, title_id text, label text,
			window_start text, window_end text)`,
		`create table if not exists convenes(
			convene_id text primary key, group_id text, state text, fire_at text,
			signals text, result text, created_at text, requested_by text)`,
		`create table if not exists notifications(
			id integer primary key autoincrement, member_id text, type text,
			message text, data text, delivered integer not null default 0)`,
	}
	if s.vecOK {
		stmts = append(stmts,
			fmt.Sprintf(`create virtual table if not exists profile_vectors using vec0(
				id text primary key, embedding float[%d])`, llm.Dims),
			fmt.Sprintf(`create virtual table if not exists catalog_vectors using vec0(
				id text primary key, embedding float[%d])`, llm.Dims),
			fmt.Sprintf(`create virtual table if not exists vocab_vectors using vec0(
				id text primary key, embedding float[%d])`, llm.Dims),
		)
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w (%s)", err, q)
		}
	}
	return nil
}

// ---------- members & catalog ----------

func (s *Store) Members(groupID string) ([]Member, error) {
	rows, err := s.db.Query(
		`select member_id, display_name, group_id from members where group_id=? order by member_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.MemberID, &m.DisplayName, &m.GroupID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Member(id string) (Member, error) {
	var m Member
	err := s.db.QueryRow(
		`select member_id, display_name, group_id from members where member_id=?`, id).
		Scan(&m.MemberID, &m.DisplayName, &m.GroupID)
	return m, err
}

func (s *Store) Titles() ([]Title, error) {
	rows, err := s.db.Query(`select title_id,title,genre,era,runtime,language,maturity,
		tone,availability,occasion,synopsis from titles order by title_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Title
	for rows.Next() {
		var t Title
		if err := rows.Scan(&t.TitleID, &t.Title, &t.Genre, &t.Era, &t.Runtime, &t.Language,
			&t.Maturity, &t.Tone, &t.Availability, &t.Occasion, &t.Synopsis); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) EventsInWindow(now time.Time, horizon time.Duration) ([]Event, error) {
	rows, err := s.db.Query(`select event_id,title_id,label,window_start,window_end from events`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ws, we string
		if err := rows.Scan(&e.EventID, &e.TitleID, &e.Label, &ws, &we); err != nil {
			return nil, err
		}
		e.WindowStart, _ = time.Parse(time.RFC3339, ws)
		e.WindowEnd, _ = time.Parse(time.RFC3339, we)
		if e.WindowEnd.After(now) && e.WindowStart.Before(now.Add(horizon)) {
			out = append(out, e)
		}
	}
	return out, rows.Err()
}

// ---------- convene workflow persistence ----------

func (s *Store) SaveConvene(c Convene) error {
	var fireAt any
	if c.FireAt != nil {
		fireAt = c.FireAt.Format(time.RFC3339)
	}
	_, err := s.db.Exec(`insert into convenes(convene_id,group_id,state,fire_at,signals,result,created_at)
		values(?,?,?,?,?,?,?)
		on conflict(convene_id) do update set state=excluded.state, fire_at=excluded.fire_at,
		signals=excluded.signals, result=excluded.result`,
		c.ConveneID, c.GroupID, c.State, fireAt, string(c.Signals), string(c.Result),
		c.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) LoadConvene(id string) (Convene, error) {
	var c Convene
	var fireAt sql.NullString
	var sig, res, created string
	err := s.db.QueryRow(`select convene_id,group_id,state,fire_at,signals,result,created_at
		from convenes where convene_id=?`, id).
		Scan(&c.ConveneID, &c.GroupID, &c.State, &fireAt, &sig, &res, &created)
	if err != nil {
		return c, err
	}
	if fireAt.Valid {
		t, _ := time.Parse(time.RFC3339, fireAt.String)
		c.FireAt = &t
	}
	c.Signals, c.Result = []byte(sig), []byte(res)
	c.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return c, nil
}

// ConvenesDue returns scheduled workflows whose fire time has arrived on the
// simulated clock, plus any workflow left mid-flight by a restart.
func (s *Store) ConvenesDue(now time.Time) ([]Convene, error) {
	rows, err := s.db.Query(`select convene_id from convenes
		where state not in ('delivered','failed')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Convene
	for _, id := range ids {
		c, err := s.LoadConvene(id)
		if err != nil {
			return nil, err
		}
		if c.FireAt == nil || !c.FireAt.After(now) {
			out = append(out, c)
		}
	}
	return out, nil
}

// ---------- notifications (piggybacked onto the next response) ----------

func (s *Store) EnqueueNotification(memberID, typ, message, dataJSON string) error {
	_, err := s.db.Exec(
		`insert into notifications(member_id,type,message,data) values(?,?,?,?)`,
		memberID, typ, message, dataJSON)
	return err
}

func (s *Store) PopNotifications(memberID string) ([]map[string]any, error) {
	rows, err := s.db.Query(
		`select id,type,message,data from notifications where member_id=? and delivered=0 order by id`,
		memberID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	var out []map[string]any
	for rows.Next() {
		var id int64
		var typ, msg, data string
		if err := rows.Scan(&id, &typ, &msg, &data); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
		n := map[string]any{"type": typ, "message": msg}
		if data != "" {
			n["data"] = jsonRaw(data)
		}
		out = append(out, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := s.db.Exec(`update notifications set delivered=1 where id=?`, id); err != nil {
			return nil, err
		}
	}
	return out, nil
}
