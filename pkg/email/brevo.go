package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBrevoBaseURL = "https://api.brevo.com/v3"
	// Brevo error bodies are small; cap the read so a misrouted response
	// cannot pull an unbounded amount of data into an error string.
	brevoMaxErrorBody = 8 << 10
)

type BrevoConfig struct {
	APIKey   string
	BaseURL  string
	From     string
	FromName string
	Timeout  time.Duration
}

// BrevoSender delivers mail through Brevo's HTTPS API, which works from hosts
// that block outbound SMTP.
type BrevoSender struct {
	cfg    BrevoConfig
	client *http.Client
}

func NewBrevoSender(cfg BrevoConfig) *BrevoSender {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultBrevoBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}

	return &BrevoSender{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (b *BrevoSender) Name() string { return "brevo" }

type brevoContact struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoRequest struct {
	Sender      brevoContact   `json:"sender"`
	To          []brevoContact `json:"to"`
	Subject     string         `json:"subject"`
	HTMLContent string         `json:"htmlContent"`
}

func (b *BrevoSender) Send(ctx context.Context, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(b.cfg.APIKey) == "" {
		return fmt.Errorf("brevo: API key is not configured")
	}
	if strings.TrimSpace(b.cfg.From) == "" {
		return fmt.Errorf("brevo: sender address is not configured")
	}

	payload, err := json.Marshal(brevoRequest{
		Sender:      brevoContact{Email: b.cfg.From, Name: b.cfg.FromName},
		To:          []brevoContact{{Email: msg.To}},
		Subject:     msg.Subject,
		HTMLContent: msg.HTMLBody,
	})
	if err != nil {
		return fmt.Errorf("brevo: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.cfg.BaseURL+"/smtp/email", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("brevo: build request: %w", err)
	}
	req.Header.Set("api-key", b.cfg.APIKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("brevo: send request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, brevoMaxErrorBody))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode/100 == 2 {
		return nil
	}

	detail, _ := io.ReadAll(io.LimitReader(resp.Body, brevoMaxErrorBody))
	return fmt.Errorf("brevo: %s: %s", resp.Status, brevoErrorMessage(detail))
}

// brevoErrorMessage pulls the human-readable part out of Brevo's error body,
// falling back to the raw payload when the shape is unfamiliar.
func brevoErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "no response body"
	}

	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Message != "" {
		if parsed.Code != "" {
			return parsed.Code + ": " + parsed.Message
		}
		return parsed.Message
	}

	return trimmed
}
