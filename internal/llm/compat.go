package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Compat is an OpenAI-compatible chat-completions client.
//
// One implementation covers Ollama, Groq, OpenRouter, Together and OpenAI
// itself, because they all expose the same /v1/chat/completions shape. That is
// what makes "swappable providers" a demonstration rather than a claim: two
// real implementations behind LLMClient, selected by configuration.
//
// It deliberately reuses promptFor() from live.go. The prompts carry the
// isolation instructions for Loop B, so a second copy would be a second place
// for those to drift.
type Compat struct {
	BaseURL string
	Model   string
	APIKey  string // optional: Ollama needs none, hosted providers do
	Client  *http.Client
}

const ollamaDefaultBase = "http://localhost:11434/v1"

// NewCompatFromEnv builds a client from AGORA_LLM_BASE / AGORA_LLM_KEY /
// AGORA_LLM_MODEL, or returns nil when no base URL is configured.
//
// Model is allowed to be empty here, because local model names are user-chosen
// and AGORA_LLM_MODEL is therefore optional. The caller MUST run EnsureModel
// before serving traffic: an empty model name is rejected by every provider
// ("model is required"), and since a failed completion descends a tier instead
// of crashing, skipping it yields a run that reports provider=ollama on
// /healthz while every answer actually came from the deterministic extractor.
func NewCompatFromEnv() *Compat {
	base := strings.TrimSpace(os.Getenv("AGORA_LLM_BASE"))
	if base == "" {
		return nil
	}
	return &Compat{
		BaseURL: strings.TrimRight(base, "/"),
		Model:   strings.TrimSpace(os.Getenv("AGORA_LLM_MODEL")),
		APIKey:  strings.TrimSpace(os.Getenv("AGORA_LLM_KEY")),
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// NewLocalDefault points a client at the conventional local Ollama endpoint,
// for when the provider was asked for by name and no base URL was supplied.
func NewLocalDefault() *Compat {
	return &Compat{
		BaseURL: ollamaDefaultBase,
		Model:   strings.TrimSpace(os.Getenv("AGORA_LLM_MODEL")),
		APIKey:  strings.TrimSpace(os.Getenv("AGORA_LLM_KEY")),
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// EnsureModel resolves the model name and proves the provider is actually
// reachable, in one round trip.
//
// It exists because both failures are otherwise INVISIBLE: a missing model
// name and an unreachable endpoint both surface as a failed completion, and a
// failed completion is indistinguishable from the deliberate chaos injection
// the degradation tiers are built to absorb. A provider named on the command
// line must therefore be verified up front, while the failure can still be
// attributed to configuration.
//
// A configured-but-absent model is an error, not a warning: quietly serving a
// different model than the one asked for is the same class of lie.
func (c *Compat) EnsureModel(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("%w: no client", ErrProvider)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	available, err := c.models(pctx)
	if err != nil {
		return fmt.Errorf("%w: %s unreachable: %v", ErrProvider, c.BaseURL, err)
	}
	if len(available) == 0 {
		return fmt.Errorf("%w: %s has no models loaded", ErrProvider, c.BaseURL)
	}
	if c.Model == "" {
		c.Model = available[0]
		return nil
	}
	for _, m := range available {
		if m == c.Model {
			return nil
		}
	}
	return fmt.Errorf("%w: model %q not available at %s (has: %s)",
		ErrProvider, c.Model, c.BaseURL, strings.Join(available, ", "))
}

// models asks the provider what it has, rather than guessing an identifier.
// Local model names are entirely user-chosen, so guessing is never right.
func (c *Compat) models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s/models", resp.StatusCode, c.BaseURL)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	return ids, nil
}

// Describe reports the provider without revealing a key.
func (c *Compat) Describe() string {
	if c == nil {
		return "none"
	}
	kind := "openai-compatible"
	if strings.Contains(c.BaseURL, "11434") {
		kind = "ollama (local)"
	}
	auth := "no key"
	if c.APIKey != "" {
		auth = fmt.Sprintf("key configured (%d chars)", len(c.APIKey))
	}
	return fmt.Sprintf("%s at %s, model=%s, %s", kind, c.BaseURL, c.Model, auth)
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Compat) Complete(ctx context.Context, req Request) (Response, error) {
	system, user, wantJSON := promptFor(req)
	if system == "" {
		return Response{}, fmt.Errorf("llm: unknown task %q", req.Task)
	}

	// temperature 0: these tasks are extraction and ranking, not generation.
	// Determinism matters more here than variety, and it reduces the schema
	// drift that smaller local models are prone to.
	body, err := json.Marshal(chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:   1024,
		Temperature: 0,
	})
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("content-type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, ErrMalformed
	}
	if resp.StatusCode != http.StatusOK {
		msg := "unknown error"
		if out.Error != nil {
			msg = out.Error.Message
		}
		return Response{}, fmt.Errorf("%w: HTTP %d — %s", ErrProvider, resp.StatusCode, msg)
	}
	if len(out.Choices) == 0 {
		return Response{}, ErrMalformed
	}

	s := strings.TrimSpace(out.Choices[0].Message.Content)
	if s == "" {
		return Response{}, ErrMalformed
	}
	if !wantJSON {
		return Response{Text: s}, nil
	}
	// Smaller models fence JSON and prepend commentary far more often than
	// large ones, so be generous about extraction — but still require a parse.
	// Unvalidated model text never reaches a justification.
	s = stripFence(s)
	if !json.Valid([]byte(s)) {
		if inner := extractJSONObject(s); inner != "" {
			s = inner
		}
	}
	if !json.Valid([]byte(s)) {
		return Response{}, ErrMalformed
	}
	return Response{JSON: json.RawMessage(s)}, nil
}

// extractJSONObject pulls the outermost {...} out of chatty output. This is a
// concession to local models, not to the design: the parse must still succeed,
// so a failure here descends a tier exactly as before.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
