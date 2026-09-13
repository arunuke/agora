package arbiter

import (
	"context"
	"time"

	"github.com/arunuke/agora/internal/store"
	"github.com/arunuke/agora/internal/vocab"
)

// Signal is what a member's agent emits: derived, closed-vocabulary, and
// carrying NO member identity and NO free text.
//
// Layer 1 of anonymity (content de-identification) is enforced by this type
// existing at all — there is nowhere in it to put a raw preference string.
type Signal struct {
	SignalID    string             `json:"signal_id"`
	Constraints []vocab.Constraint `json:"constraints"`
	Vetoes      []vocab.Veto       `json:"vetoes"`
	Tier        int                `json:"tier"`
}

type ConsultRequest struct {
	MemberID string `json:"member_id"`
	Purpose  string `json:"purpose"`
}

type ConsultResponse struct {
	Signal   Signal `json:"signal"`
	Degraded bool   `json:"degraded"`
	Tier     int    `json:"tier"`
}

// MemberAgent is what the arbiter needs from a member's agent. It is declared
// HERE, by the consumer, per Go convention — which is what makes the eventual
// gRPC split a second implementation of an existing contract rather than a
// refactor. Implemented in-process today; over the wire in production.
type MemberAgent interface {
	Consult(ctx context.Context, req ConsultRequest) (ConsultResponse, error)
}

// Store is the arbiter's deliberately narrow view of persistence.
//
// Note what is absent: any method returning a member's raw context or profile
// vector. Those exist only behind internal/agent/internal/rawctx, which this
// package cannot import. Isolation is enforced twice — once by this interface,
// once by the compiler.
type Store interface {
	Members(groupID string) ([]store.Member, error)
	Member(id string) (store.Member, error)
	Titles() ([]store.Title, error)
	EventsInWindow(now time.Time, horizon time.Duration) ([]store.Event, error)
	SaveConvene(c store.Convene) error
	LoadConvene(id string) (store.Convene, error)
	ConvenesDue(now time.Time) ([]store.Convene, error)
	EnqueueNotification(memberID, typ, message, dataJSON string) error
}

type SlateItem struct {
	TitleID      string `json:"title_id"`
	Title        string `json:"title"`
	Availability string `json:"availability"`
	Runtime      int    `json:"runtime"`
	// NOT serialised. The score is the sum of ALL constraint weights, public and
	// private alike — that is what makes private constraints influence the
	// ranking without being speakable. Published to members it becomes a
	// numeric side channel around the entire anonymity layer: subtract the
	// contribution of the public constraints and the residual is the weight of
	// the below-threshold ones. In the seeded family, Run Lola Run (a thriller)
	// outscored a comedy on a slate whose public constraints were "comedies"
	// and "something short" — the surplus WAS the private constraints, in a
	// number anyone could read.
	//
	// The field stays for ordering and for tests. Members get the order, which
	// is the part that carries meaning; the arithmetic behind it is not theirs
	// to reconstruct.
	Score float64 `json:"-"`
}

type Quorum struct {
	Represented int  `json:"represented"`
	Of          int  `json:"of"`
	Provisional bool `json:"provisional"`
}

type ConveneResult struct {
	ConveneID         string      `json:"convene_id"`
	Slate             []SlateItem `json:"slate"`
	Justification     string      `json:"justification"`
	Quorum            Quorum      `json:"quorum"`
	Degraded          bool        `json:"degraded"`
	Tier              int         `json:"tier"`
	PublicConstraints []string    `json:"public_constraints"`
}
