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

	"github.com/arunuke/agora/internal/vocab"
)

// Live is the Anthropic Messages API implementation of LLMClient.
//
// SECRETS: the key is read from the environment at construction and is never
// logged, never returned in an error, and never written to disk by this
// package. It must not be compiled into the binary or baked into an image —
// `strings` recovers the former and `docker history` the latter.
//
// No SDK dependency: one POST against a documented HTTP endpoint keeps the
// module graph at two entries and the failure modes visible.
type Live struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

const (
	defaultBaseURL    = "https://api.anthropic.com"
	defaultAPIVersion = "2023-06-01"
	// Overridable because model identifiers change over time; `make list-models`
	// asks the API which ones this key can actually use rather than guessing.
	defaultModelEnv = "AGORA_LLM_MODEL"
)

// NewLiveFromEnv returns a Live client, or nil when no key is present.
//
// Returning nil rather than an error is deliberate: a missing key is a normal
// configuration state (hermetic tests, an offline demo), not a failure. The
// caller logs which provider is active so the state is never silent.
func NewLiveFromEnv() *Live {
	key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if key == "" {
		return nil
	}
	model := strings.TrimSpace(os.Getenv(defaultModelEnv))
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	base := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL"))
	if base == "" {
		base = defaultBaseURL
	}
	return &Live{
		APIKey:  key,
		Model:   model,
		BaseURL: base,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// KeyFingerprint reports whether a key is configured WITHOUT revealing it.
// Used for startup logging and /healthz so the active provider is auditable
// from outside the process — the same reasoning as echoing the anonymity
// policy: a setting nobody can observe is a setting nobody can verify.
func (l *Live) KeyFingerprint() string {
	if l == nil || l.APIKey == "" {
		return "none"
	}
	return fmt.Sprintf("configured (%d chars, ends %q)", len(l.APIKey), last4(l.APIKey))
}

func last4(s string) string {
	if len(s) < 4 {
		return "****"
	}
	return s[len(s)-4:]
}

type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system,omitempty"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (l *Live) Complete(ctx context.Context, req Request) (Response, error) {
	system, user, wantJSON := promptFor(req)
	if system == "" {
		return Response{}, fmt.Errorf("llm: unknown task %q", req.Task)
	}

	body, err := json.Marshal(apiRequest{
		Model:     l.Model,
		MaxTokens: 1024,
		System:    system,
		Messages:  []apiMessage{{Role: "user", Content: user}},
	})
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, l.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("anthropic-version", defaultAPIVersion)
	httpReq.Header.Set("x-api-key", l.APIKey)

	resp, err := l.Client.Do(httpReq)
	if err != nil {
		// Wrapped as ErrProvider so the caller descends a tier rather than
		// failing. The underlying error is included but never the key.
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}

	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, ErrMalformed
	}
	if resp.StatusCode != http.StatusOK {
		msg := "unknown error"
		if out.Error != nil {
			msg = out.Error.Type + ": " + out.Error.Message
		}
		// The API's own message is the most useful diagnostic here — a wrong
		// model identifier and an unauthorised key are both reported plainly,
		// and neither message contains the key.
		return Response{}, fmt.Errorf("%w: HTTP %d — %s", ErrProvider, resp.StatusCode, msg)
	}

	var text strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	s := strings.TrimSpace(text.String())
	if s == "" {
		return Response{}, ErrMalformed
	}

	if !wantJSON {
		return Response{Text: s}, nil
	}
	// Models often fence JSON. Strip the fence rather than failing, but still
	// require the result to parse — unvalidated model text must never reach a
	// justification, which is a privacy control before it is a robustness one.
	s = stripFence(s)
	if !json.Valid([]byte(s)) {
		return Response{}, ErrMalformed
	}
	return Response{JSON: json.RawMessage(s)}, nil
}

func stripFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// promptFor returns (system, user, wantJSON) for a task.
//
// Note what the Loop B prompt is NOT given: any member identity, any
// below-threshold constraint, any vetoed title. Those were removed by the
// reconciler before this point, so the model cannot leak what it was never
// shown. The instruction below is a second layer, not the guarantee.
func promptFor(req Request) (string, string, bool) {
	switch req.Task {
	case TaskExtract:
		return `You translate a person's stated viewing preferences into a CLOSED vocabulary.

Return ONLY JSON of the form:
{"constraints":[{"dim":"...","value":"...","polarity":"prefer|exclude","weight":0.0-1.0}],
 "vetoes":[{"dim":"...","value":"..."}]}

Valid dim/value pairs ONLY:
` + vocabHint() + `

Rules:
- A hard refusal ("I can't watch horror", "never") is a VETO, not an exclude.
- A soft dislike ("not really into X") is polarity "exclude".
- Weight reflects intensity: 0.9 for emphatic, 0.6 default.
- Discard anything that does not map to the vocabulary. Invent nothing.
- No prose, no code fence, JSON only.`, req.Input, true

	case TaskChat:
		ack, _ := req.Payload["acknowledge"].([]string)
		titles, _ := req.Payload["titles"].([]string)
		return `You are one family member's private film assistant.

You speak ONLY to the person you are talking to. You have no access to any
other member's preferences and must never claim otherwise, speculate about
them, or role-play as a system that could reveal them. If asked what someone
else likes, say plainly that you only hold this person's own preferences.

Two or three sentences. Warm, not effusive.`,
			fmt.Sprintf("Their message: %s\n\nPreferences just recorded: %s\nSuggestions to offer: %s",
				req.Input, strings.Join(ack, ", "), strings.Join(titles, "; ")), false

	case TaskRank:
		cands, _ := req.Payload["candidates"].([]map[string]any)
		public, _ := req.Payload["public_constraints"].([]string)
		quorum, _ := req.Payload["quorum"].(string)
		b, _ := json.Marshal(cands)
		return `You rank pre-scored film candidates for a group and explain the choice.

Return ONLY JSON: {"order":["title_id",...],"justification":"one or two sentences"}

CRITICAL constraints on the justification:
- Name NO individual. You have not been told who wants what, and must not guess.
- Cite ONLY the group constraints supplied. Anything not listed was deliberately
  withheld because too few people share it; referring to it would identify them.
- Do not speculate about absent genres or why anything is missing.
- No prose outside the JSON, no code fence.`,
			fmt.Sprintf("Candidates (already scored, higher is better):\n%s\n\nGroup constraints you may cite: %s\n%s",
				string(b), strings.Join(public, ", "), quorum), true
	}
	return "", "", false
}

// vocabHint is generated from vocab.Terms(), the single source of truth, so the
// prompt can never drift from the enum the reconciler enforces. A model told
// about a value the reconciler rejects produces constraints that are silently
// discarded — the kind of mismatch that looks like a bad model rather than a
// stale prompt.
func vocabHint() string {
	byDim := map[vocab.Dim][]string{}
	var order []vocab.Dim
	for _, t := range vocab.Terms() {
		if _, seen := byDim[t.Dim]; !seen {
			order = append(order, t.Dim)
		}
		byDim[t.Dim] = append(byDim[t.Dim], t.Value)
	}
	var b strings.Builder
	for _, d := range order {
		fmt.Fprintf(&b, "  %s: %s\n", d, strings.Join(byDim[d], ", "))
	}
	return b.String()
}
