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

const defaultMailjetBaseURL = "https://api.mailjet.com/v3.1"

type MailjetConfig struct {
	// Mailjet issues a pair: the public key identifies the account, the
	// private one authenticates it. Both go in HTTP Basic auth.
	APIKey    string
	SecretKey string
	BaseURL   string
	From      string
	FromName  string
	Timeout   time.Duration
}

// MailjetSender delivers mail through Mailjet's Send API v3.1 over HTTPS.
type MailjetSender struct {
	cfg    MailjetConfig
	client *http.Client
}

func NewMailjetSender(cfg MailjetConfig) *MailjetSender {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultMailjetBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}

	return &MailjetSender{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

func (m *MailjetSender) Name() string { return "mailjet" }

type mailjetContact struct {
	Email string `json:"Email"`
	Name  string `json:"Name,omitempty"`
}

type mailjetMessage struct {
	From     mailjetContact   `json:"From"`
	To       []mailjetContact `json:"To"`
	Subject  string           `json:"Subject"`
	HTMLPart string           `json:"HTMLPart"`
}

type mailjetRequest struct {
	Messages []mailjetMessage `json:"Messages"`
}

type mailjetResponse struct {
	Messages []struct {
		Status string `json:"Status"`
		Errors []struct {
			ErrorCode    string `json:"ErrorCode"`
			ErrorMessage string `json:"ErrorMessage"`
		} `json:"Errors"`
	} `json:"Messages"`
	// Returned instead of Messages when the whole request is rejected.
	ErrorMessage string `json:"ErrorMessage"`
}

func (m *MailjetSender) Send(ctx context.Context, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(m.cfg.APIKey) == "" || strings.TrimSpace(m.cfg.SecretKey) == "" {
		return fmt.Errorf("mailjet: both the API key and the secret key are required")
	}
	if strings.TrimSpace(m.cfg.From) == "" {
		return fmt.Errorf("mailjet: sender address is not configured")
	}

	payload, err := json.Marshal(mailjetRequest{
		Messages: []mailjetMessage{{
			From:     mailjetContact{Email: m.cfg.From, Name: m.cfg.FromName},
			To:       []mailjetContact{{Email: msg.To}},
			Subject:  msg.Subject,
			HTMLPart: msg.HTMLBody,
		}},
	})
	if err != nil {
		return fmt.Errorf("mailjet: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.BaseURL+"/send", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("mailjet: build request: %w", err)
	}
	req.SetBasicAuth(m.cfg.APIKey, m.cfg.SecretKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mailjet: send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, brevoMaxErrorBody))

	var parsed mailjetResponse
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("mailjet: %s: %s", resp.Status, mailjetErrorMessage(parsed, body))
	}

	// v3.1 reports per-message outcomes inside a 200, so the status code alone
	// does not tell us the mail was accepted.
	for _, m := range parsed.Messages {
		if !strings.EqualFold(m.Status, "success") {
			return fmt.Errorf("mailjet: message rejected: %s", mailjetErrorMessage(parsed, body))
		}
	}
	if len(parsed.Messages) == 0 {
		return fmt.Errorf("mailjet: unexpected response: %s", truncateBody(body))
	}

	return nil
}

// mailjetErrorMessage digs the most specific message out of a failure, falling
// back to the raw body when the shape is unfamiliar.
func mailjetErrorMessage(parsed mailjetResponse, body []byte) string {
	for _, m := range parsed.Messages {
		for _, e := range m.Errors {
			if e.ErrorMessage != "" {
				if e.ErrorCode != "" {
					return e.ErrorCode + ": " + e.ErrorMessage
				}
				return e.ErrorMessage
			}
		}
	}
	if parsed.ErrorMessage != "" {
		return parsed.ErrorMessage
	}
	return truncateBody(body)
}
