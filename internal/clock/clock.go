// Package clock provides the simulated clock that makes long-running
// workflows observable inside a five-minute review (US6).
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

// Simulated advances only when Advance is called. No test in the suite calls
// time.Sleep; durability and scheduling are exercised by moving this instead.
type Simulated struct {
	mu sync.RWMutex
	t  time.Time
}

// Base is a fixed reference point so seeded event windows sit at known offsets
// and the demo is reproducible whenever it is opened.
var Base = time.Date(2026, 9, 8, 18, 0, 0, 0, time.UTC)

func NewSimulated() *Simulated { return &Simulated{t: Base} }

func (s *Simulated) Now() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.t
}

func (s *Simulated) Advance(d time.Duration) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.t = s.t.Add(d)
	return s.t
}

func (s *Simulated) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.t = Base
}
