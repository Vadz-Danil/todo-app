package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrTransient marks a failure that was retried and stayed broken:
	// network trouble, 429 or 5xx.
	ErrTransient = errors.New("gemini: provider temporarily unavailable")
	// ErrBadResponse marks a 2xx the caller cannot use: no candidate,
	// empty text, or text that is not valid JSON.
	ErrBadResponse = errors.New("gemini: unusable provider response")
)

const (
	defaultBaseURL     = "https://generativelanguage.googleapis.com/v1beta"
	defaultModel       = "gemini-3.1-flash-lite"
	defaultTimeout     = 90 * time.Second
	defaultMaxRetries  = 2
	defaultTemperature = 0.3

	initialBackoff  = 500 * time.Millisecond
	maxErrBodyBytes = 4 << 10
)

type Config struct {
	APIKey      string
	Model       string
	BaseURL     string
	Timeout     time.Duration
	MaxRetries  int
	Temperature float64
}

type Request struct {
	System          string
	Prompt          string
	Schema          map[string]any
	Temperature     *float64
	MaxOutputTokens int
}

type Client interface {
	GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error)
	Model() string
}

type HTTPClient struct {
	apiKey      string
	model       string
	endpoint    string
	path        string
	maxRetries  int
	temperature float64
	httpClient  *http.Client
}

var _ Client = (*HTTPClient)(nil)

// New applies the documented defaults to every zero-valued field. A zero
// Temperature therefore means "unset"; pin a request to 0 via Request.Temperature.
func New(cfg Config) *HTTPClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	temperature := cfg.Temperature
	if temperature <= 0 {
		temperature = defaultTemperature
	}

	suffix := "/models/" + model + ":generateContent"
	path := suffix
	if u, err := url.Parse(baseURL); err == nil {
		path = strings.TrimRight(u.Path, "/") + suffix
	}

	return &HTTPClient{
		apiKey:      strings.TrimSpace(cfg.APIKey),
		model:       model,
		endpoint:    baseURL + suffix,
		path:        path,
		maxRetries:  maxRetries,
		temperature: temperature,
		httpClient:  &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Model() string { return c.model }

func (c *HTTPClient) GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	body, err := json.Marshal(c.payload(req))
	if err != nil {
		return nil, fmt.Errorf("gemini: encode request: %w", err)
	}

	backoff := initialBackoff
	for attempt := 0; ; attempt++ {
		raw, err := c.do(ctx, body)
		if err == nil {
			return raw, nil
		}
		if !errors.Is(err, ErrTransient) || attempt >= c.maxRetries {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

func (c *HTTPClient) do(ctx context.Context, body []byte) (json.RawMessage, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"?key="+url.QueryEscape(c.apiKey), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: build request for %s: %w", c.path, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: %s: %s", ErrTransient, c.path, c.scrub(networkCause(err)))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
		detail := fmt.Sprintf("%s: status %d: %s", c.path, resp.StatusCode,
			c.scrub(strings.TrimSpace(string(snippet))))
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: %s", ErrTransient, detail)
		}
		return nil, fmt.Errorf("gemini: %s", detail)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: read body: %s", ErrTransient, c.path, c.scrub(networkCause(err)))
	}

	var parsed generateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: %s: decode envelope: %v", ErrBadResponse, c.path, err)
	}
	if len(parsed.Candidates) == 0 {
		return nil, fmt.Errorf("%w: %s: no candidates", ErrBadResponse, c.path)
	}

	candidate := parsed.Candidates[0]
	var text strings.Builder
	for _, p := range candidate.Content.Parts {
		text.WriteString(p.Text)
	}
	out := stripFence(strings.TrimSpace(text.String()))
	if out == "" {
		return nil, fmt.Errorf("%w: %s: empty text (finishReason %q)", ErrBadResponse, c.path, candidate.FinishReason)
	}
	if !json.Valid([]byte(out)) {
		return nil, fmt.Errorf("%w: %s: text is not valid JSON (finishReason %q)", ErrBadResponse, c.path, candidate.FinishReason)
	}

	return json.RawMessage(out), nil
}

func (c *HTTPClient) payload(req Request) generateRequest {
	temperature := c.temperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	out := generateRequest{
		Contents: []content{{Role: "user", Parts: []part{{Text: req.Prompt}}}},
		GenerationConfig: generationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   req.Schema,
			Temperature:      temperature,
		},
	}
	if strings.TrimSpace(req.System) != "" {
		out.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}
	if req.MaxOutputTokens > 0 {
		out.GenerationConfig.MaxOutputTokens = req.MaxOutputTokens
	}
	return out
}

// scrub keeps the API key out of errors and logs; it travels in the query string,
// so transport errors quote it back verbatim.
func (c *HTTPClient) scrub(s string) string {
	if c.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, c.apiKey, "[REDACTED]")
}

func networkCause(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

func stripFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 && isLangTag(s[:i]) {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func isLangTag(s string) bool {
	for _, r := range strings.TrimSpace(s) {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

type generateRequest struct {
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	Contents          []content        `json:"contents"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generationConfig struct {
	ResponseMimeType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
	Temperature      float64        `json:"temperature"`
	MaxOutputTokens  int            `json:"maxOutputTokens,omitempty"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

func Object(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{"type": "OBJECT", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func Array(items map[string]any) map[string]any {
	return map[string]any{"type": "ARRAY", "items": items}
}

func Str(description string) map[string]any { return scalar("STRING", description) }

func Num(description string) map[string]any { return scalar("NUMBER", description) }

func Bool(description string) map[string]any { return scalar("BOOLEAN", description) }

func Enum(description string, values ...string) map[string]any {
	schema := scalar("STRING", description)
	schema["enum"] = values
	return schema
}

func scalar(kind, description string) map[string]any {
	schema := map[string]any{"type": kind}
	if description != "" {
		schema["description"] = description
	}
	return schema
}
