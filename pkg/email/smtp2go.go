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

const defaultSMTP2GOBaseURL = "https://api.smtp2go.com/v3"

type SMTP2GOConfig struct {
	APIKey   string
	BaseURL  string
	From     string
	FromName string
	Timeout  time.Duration
}

// SMTP2GOSender delivers mail through SMTP2GO's HTTPS API.
type SMTP2GOSender struct {
	cfg    SMTP2GOConfig
	client *http.Client
}

func NewSMTP2GOSender(cfg SMTP2GOConfig) *SMTP2GOSender {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultSMTP2GOBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}

	return &SMTP2GOSender{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

func (s *SMTP2GOSender) Name() string { return "smtp2go" }

type smtp2goRequest struct {
	Sender   string   `json:"sender"`
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	HTMLBody string   `json:"html_body"`
}

type smtp2goResponse struct {
	RequestID string `json:"request_id"`
	Data      struct {
		EmailID   string `json:"email_id"`
		Succeeded int    `json:"succeeded"`
		Failed    int    `json:"failed"`
		Failures  []any  `json:"failures"`
		ErrorCode string `json:"error_code"`
		Error     string `json:"error"`
	} `json:"data"`
}

func (s *SMTP2GOSender) Send(ctx context.Context, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.cfg.APIKey) == "" {
		return fmt.Errorf("smtp2go: API key is not configured")
	}
	if strings.TrimSpace(s.cfg.From) == "" {
		return fmt.Errorf("smtp2go: sender address is not configured")
	}

	payload, err := json.Marshal(smtp2goRequest{
		Sender:   formatAddress(s.cfg.From, s.cfg.FromName),
		To:       []string{msg.To},
		Subject:  msg.Subject,
		HTMLBody: msg.HTMLBody,
	})
	if err != nil {
		return fmt.Errorf("smtp2go: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL+"/email/send", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("smtp2go: build request: %w", err)
	}
	req.Header.Set("X-Smtp2go-Api-Key", s.cfg.APIKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("smtp2go: send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, brevoMaxErrorBody))

	var parsed smtp2goResponse
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode/100 != 2 {
		if parsed.Data.Error != "" {
			return fmt.Errorf("smtp2go: %s: %s: %s", resp.Status, parsed.Data.ErrorCode, parsed.Data.Error)
		}
		return fmt.Errorf("smtp2go: %s: %s", resp.Status, truncateBody(body))
	}

	// A 200 does not by itself mean the message was accepted: a rejected
	// recipient comes back as failed>0 in an otherwise successful response.
	if parsed.Data.Failed > 0 || parsed.Data.Succeeded == 0 {
		if parsed.Data.Error != "" {
			return fmt.Errorf("smtp2go: message rejected: %s: %s", parsed.Data.ErrorCode, parsed.Data.Error)
		}
		return fmt.Errorf("smtp2go: message rejected: %s", truncateBody(body))
	}

	return nil
}
