package llm

import (
	"context"
	"sync"
	"time"
)

// Switches holds injected-failure state for the whole process. It is shared by
// the LLM decorator, the embedder decorator and the MemberAgent decorator, so
// POST /v1/demo/chaos flips one struct and no conditional appears anywhere
// inside the workflow.
type Switches struct {
	mu            sync.RWMutex
	llmDown       bool
	embedDown     bool
	malformed     bool
	memberFail    int  // number of member agents to fail
	memberHang    bool // hang rather than error, to prove deadlines work
	memberLatency time.Duration
}

func NewSwitches() *Switches { return &Switches{} }

func (s *Switches) Set(f func(*Switches)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

func (s *Switches) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Fields individually — assigning *s = Switches{} would zero the mutex
	// while it is held, and the deferred Unlock then panics.
	s.llmDown, s.embedDown, s.malformed = false, false, false
	s.memberFail, s.memberHang, s.memberLatency = 0, false, 0
}

func (s *Switches) SetLLMDown(v bool)    { s.mu.Lock(); s.llmDown = v; s.mu.Unlock() }
func (s *Switches) SetEmbedDown(v bool)  { s.mu.Lock(); s.embedDown = v; s.mu.Unlock() }
func (s *Switches) SetMalformed(v bool)  { s.mu.Lock(); s.malformed = v; s.mu.Unlock() }
func (s *Switches) SetMemberFail(n int)  { s.mu.Lock(); s.memberFail = n; s.mu.Unlock() }
func (s *Switches) SetMemberHang(v bool) { s.mu.Lock(); s.memberHang = v; s.mu.Unlock() }
func (s *Switches) SetMemberLatency(d time.Duration) {
	s.mu.Lock()
	s.memberLatency = d
	s.mu.Unlock()
}

func (s *Switches) LLMDown() bool   { s.mu.RLock(); defer s.mu.RUnlock(); return s.llmDown }
func (s *Switches) EmbedDown() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.embedDown }
func (s *Switches) Malformed() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.malformed }
func (s *Switches) MemberFail() int { s.mu.RLock(); defer s.mu.RUnlock(); return s.memberFail }
func (s *Switches) MemberHang() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memberHang
}
func (s *Switches) MemberLatency() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memberLatency
}

// ChaosLLM decorates LLMClient. Note it decorates the completion interface
// ONLY — embeddings have their own decorator, so failing completions while
// leaving embeddings healthy is exactly the tier-2 demonstration.
type ChaosLLM struct {
	Inner LLMClient
	Sw    *Switches
}

func (c ChaosLLM) Complete(ctx context.Context, req Request) (Response, error) {
	if c.Sw.LLMDown() {
		return Response{}, ErrProvider
	}
	if c.Sw.Malformed() {
		return Response{JSON: []byte(`{"constraints": "this is not the right shape"`)}, nil
	}
	return c.Inner.Complete(ctx, req)
}

type ChaosEmbedder struct {
	Inner Embedder
	Sw    *Switches
}

func (c ChaosEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if c.Sw.EmbedDown() {
		return nil, ErrProvider
	}
	return c.Inner.Embed(ctx, texts)
}
