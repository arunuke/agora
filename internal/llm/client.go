// Package llm holds the two provider interfaces and their implementations.
//
// LLMClient and Embedder are SEPARATE interfaces on purpose. Completion and
// embedding endpoints fail independently, and that independence is what makes
// tier 2 of the degradation ladder a real operating mode rather than a
// theoretical one:
//
//	tier 1  LLM extraction          — negation, idiom, nuance
//	tier 2  embedding nearest-neighbour to the canonical vocabulary
//	tier 3  keyword match over the vocabulary
package llm

import (
	"context"
	"encoding/json"
	"errors"
)

// Task names the shape of work being asked for. A generic completion API with
// a task hint keeps the interface gRPC-shaped while letting the deterministic
// provider produce structured output.
const (
	TaskExtract = "extract" // free text -> closed-vocabulary constraints
	TaskChat    = "chat"    // free text -> a reply to one member
	TaskRank    = "rank"    // candidates + public constraints -> ranked slate
)

type Request struct {
	Task    string         `json:"task"`
	Input   string         `json:"input,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type Response struct {
	Text string          `json:"text,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
}

type LLMClient interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

var (
	// ErrProvider is returned by the chaos decorator and by the live provider
	// on transport failure. Callers treat it as "descend one tier".
	ErrProvider = errors.New("llm: provider unavailable")
	// ErrMalformed signals output that failed a typed unmarshal. Raw model
	// text is NEVER passed through on this path — that rule is a privacy
	// control before it is a robustness one.
	ErrMalformed = errors.New("llm: malformed provider output")
)
