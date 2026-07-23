package email

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testMessage() Message {
	return Message{
		To:       "recipient@example.com",
		Subject:  "✓ TaskFlow: звіт",
		HTMLBody: "<p>hello</p>",
	}
}

func TestBrevoSender_SendsTheExpectedRequest(t *testing.T) {
	var (
		gotPath   string
		gotAPIKey string
		gotBody   brevoRequest
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("api-key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<abc@brevo>"}`))
	}))
	defer srv.Close()

	sender := NewBrevoSender(BrevoConfig{
		APIKey:   "secret-key",
		BaseURL:  srv.URL,
		From:     "sender@example.com",
		FromName: "TaskFlow",
	})

	if err := sender.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotPath != "/smtp/email" {
		t.Errorf("path = %q, want /smtp/email", gotPath)
	}
	if gotAPIKey != "secret-key" {
		t.Errorf("api-key header = %q", gotAPIKey)
	}
	if gotBody.Sender.Email != "sender@example.com" || gotBody.Sender.Name != "TaskFlow" {
		t.Errorf("sender = %+v", gotBody.Sender)
	}
	if len(gotBody.To) != 1 || gotBody.To[0].Email != "recipient@example.com" {
		t.Errorf("to = %+v", gotBody.To)
	}
	// The subject travels as JSON, so it needs no MIME encoding — a Q-encoded
	// value here would reach the inbox as literal "=?UTF-8?q?..." text.
	if gotBody.Subject != "✓ TaskFlow: звіт" {
		t.Errorf("subject = %q, want the raw UTF-8 value", gotBody.Subject)
	}
	if gotBody.HTMLContent != "<p>hello</p>" {
		t.Errorf("htmlContent = %q", gotBody.HTMLContent)
	}
}

func TestBrevoSender_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sender := NewBrevoSender(BrevoConfig{APIKey: "k", BaseURL: srv.URL + "/", From: "s@example.com"})
	if err := sender.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/smtp/email" {
		t.Errorf("path = %q, want /smtp/email (no double slash)", gotPath)
	}
}

func TestBrevoSender_SurfacesAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized","message":"Key not found"}`))
	}))
	defer srv.Close()

	sender := NewBrevoSender(BrevoConfig{APIKey: "bad", BaseURL: srv.URL, From: "s@example.com"})

	err := sender.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
	if !strings.Contains(err.Error(), "unauthorized") || !strings.Contains(err.Error(), "Key not found") {
		t.Errorf("error = %v, want the provider code and message", err)
	}
	// The key itself must never end up in a log line.
	if strings.Contains(err.Error(), "bad") && strings.Contains(err.Error(), "api-key") {
		t.Errorf("error leaks credentials: %v", err)
	}
}

func TestBrevoSender_RejectsIncompleteConfiguration(t *testing.T) {
	t.Run("missing api key", func(t *testing.T) {
		sender := NewBrevoSender(BrevoConfig{From: "s@example.com"})
		if err := sender.Send(context.Background(), testMessage()); err == nil {
			t.Fatal("expected an error when the API key is absent")
		}
	})

	t.Run("missing sender", func(t *testing.T) {
		sender := NewBrevoSender(BrevoConfig{APIKey: "k"})
		if err := sender.Send(context.Background(), testMessage()); err == nil {
			t.Fatal("expected an error when From is absent")
		}
	})
}

func TestBrevoSender_ValidatesTheMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("an invalid message must not reach the provider")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sender := NewBrevoSender(BrevoConfig{APIKey: "k", BaseURL: srv.URL, From: "s@example.com"})

	cases := map[string]Message{
		"no recipient": {Subject: "s", HTMLBody: "b"},
		"no subject":   {To: "r@example.com", HTMLBody: "b"},
		"no body":      {To: "r@example.com", Subject: "s"},
		"blank body":   {To: "r@example.com", Subject: "s", HTMLBody: "   "},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			if err := sender.Send(context.Background(), msg); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestBrevoSender_HonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sender := NewBrevoSender(BrevoConfig{APIKey: "k", BaseURL: srv.URL, From: "s@example.com"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := sender.Send(ctx, testMessage()); err == nil {
		t.Fatal("expected the cancelled context to abort the send")
	}
}

func TestBrevoSender_Name(t *testing.T) {
	if got := NewBrevoSender(BrevoConfig{}).Name(); got != "brevo" {
		t.Errorf("Name() = %q", got)
	}
}
