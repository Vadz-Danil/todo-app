package email

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// SMTP2GO
// ---------------------------------------------------------------------------

func TestSMTP2GOSender_SendsTheExpectedRequest(t *testing.T) {
	var (
		gotPath string
		gotKey  string
		gotBody smtp2goRequest
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Smtp2go-Api-Key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"request_id":"r1","data":{"email_id":"e1","succeeded":1,"failed":0,"failures":[]}}`))
	}))
	defer srv.Close()

	sender := NewSMTP2GOSender(SMTP2GOConfig{
		APIKey: "api-abc", BaseURL: srv.URL, From: "sender@example.com", FromName: "TaskFlow",
	})

	if err := sender.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotPath != "/email/send" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "api-abc" {
		t.Errorf("api key header = %q", gotKey)
	}
	if gotBody.Sender != `"TaskFlow" <sender@example.com>` {
		t.Errorf("sender = %q, want the Name <addr> form", gotBody.Sender)
	}
	if len(gotBody.To) != 1 || gotBody.To[0] != "recipient@example.com" {
		t.Errorf("to = %v", gotBody.To)
	}
	if gotBody.Subject != "✓ TaskFlow: звіт" {
		t.Errorf("subject = %q, want raw UTF-8", gotBody.Subject)
	}
}

func TestSMTP2GOSender_TreatsAFailedRecipientAsAnError(t *testing.T) {
	// A rejected recipient arrives inside an HTTP 200, so the status code alone
	// would report success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"request_id":"r1","data":{"succeeded":0,"failed":1,"failures":["recipient@example.com: rejected"]}}`))
	}))
	defer srv.Close()

	sender := NewSMTP2GOSender(SMTP2GOConfig{APIKey: "k", BaseURL: srv.URL, From: "s@example.com"})

	err := sender.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("a 200 with failed=1 must not be reported as success")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("error = %v", err)
	}
}

func TestSMTP2GOSender_SurfacesAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"request_id":"r1","data":{"error_code":"E_ApiResponseCodes.NON_VALIDATING_IN_PAYLOAD","error":"sender is not verified"}}`))
	}))
	defer srv.Close()

	sender := NewSMTP2GOSender(SMTP2GOConfig{APIKey: "k", BaseURL: srv.URL, From: "s@example.com"})

	err := sender.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("expected an error for a 400 response")
	}
	if !strings.Contains(err.Error(), "sender is not verified") {
		t.Errorf("error = %v, want the provider message", err)
	}
}

func TestSMTP2GOSender_RejectsIncompleteConfiguration(t *testing.T) {
	if err := NewSMTP2GOSender(SMTP2GOConfig{From: "s@example.com"}).Send(context.Background(), testMessage()); err == nil {
		t.Error("expected an error without an API key")
	}
	if err := NewSMTP2GOSender(SMTP2GOConfig{APIKey: "k"}).Send(context.Background(), testMessage()); err == nil {
		t.Error("expected an error without a sender")
	}
}

func TestSMTP2GOSender_Name(t *testing.T) {
	if got := NewSMTP2GOSender(SMTP2GOConfig{}).Name(); got != "smtp2go" {
		t.Errorf("Name() = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Mailjet
// ---------------------------------------------------------------------------

func TestMailjetSender_SendsTheExpectedRequest(t *testing.T) {
	var (
		gotPath string
		gotAuth string
		gotBody mailjetRequest
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Messages":[{"Status":"success","To":[{"Email":"recipient@example.com"}]}]}`))
	}))
	defer srv.Close()

	sender := NewMailjetSender(MailjetConfig{
		APIKey: "public-key", SecretKey: "private-key",
		BaseURL: srv.URL, From: "sender@example.com", FromName: "TaskFlow",
	})

	if err := sender.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotPath != "/send" {
		t.Errorf("path = %q", gotPath)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("public-key:private-key"))
	if gotAuth != want {
		t.Errorf("Authorization = %q, want basic auth over the key pair", gotAuth)
	}

	if len(gotBody.Messages) != 1 {
		t.Fatalf("Messages = %v", gotBody.Messages)
	}
	m := gotBody.Messages[0]
	if m.From.Email != "sender@example.com" || m.From.Name != "TaskFlow" {
		t.Errorf("From = %+v", m.From)
	}
	if len(m.To) != 1 || m.To[0].Email != "recipient@example.com" {
		t.Errorf("To = %+v", m.To)
	}
	if m.HTMLPart != "<p>hello</p>" {
		t.Errorf("HTMLPart = %q", m.HTMLPart)
	}
}

func TestMailjetSender_TreatsAPerMessageErrorAsAFailure(t *testing.T) {
	// v3.1 reports per-message outcomes inside the body; a 200 is not enough.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Messages":[{"Status":"error","Errors":[{"ErrorCode":"mj-0013","ErrorMessage":"\"sender@example.com\" is an invalid email address."}]}]}`))
	}))
	defer srv.Close()

	sender := NewMailjetSender(MailjetConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL, From: "sender@example.com"})

	err := sender.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("a 200 carrying Status=error must not be reported as success")
	}
	if !strings.Contains(err.Error(), "mj-0013") || !strings.Contains(err.Error(), "invalid email address") {
		t.Errorf("error = %v, want the per-message code and text", err)
	}
}

func TestMailjetSender_SurfacesTransportErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ErrorMessage":"API key authentication/authorization failure."}`))
	}))
	defer srv.Close()

	sender := NewMailjetSender(MailjetConfig{APIKey: "bad", SecretKey: "bad", BaseURL: srv.URL, From: "s@example.com"})

	err := sender.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error = %v", err)
	}
}

func TestMailjetSender_RejectsAnEmptyMessagesArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	sender := NewMailjetSender(MailjetConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL, From: "s@example.com"})

	if err := sender.Send(context.Background(), testMessage()); err == nil {
		t.Fatal("a response with no per-message outcome must not count as success")
	}
}

func TestMailjetSender_RequiresBothKeys(t *testing.T) {
	cases := map[string]MailjetConfig{
		"no secret": {APIKey: "k", From: "s@example.com"},
		"no key":    {SecretKey: "s", From: "s@example.com"},
		"no sender": {APIKey: "k", SecretKey: "s"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if err := NewMailjetSender(cfg).Send(context.Background(), testMessage()); err == nil {
				t.Fatal("expected a configuration error")
			}
		})
	}
}

func TestMailjetSender_Name(t *testing.T) {
	if got := NewMailjetSender(MailjetConfig{}).Name(); got != "mailjet" {
		t.Errorf("Name() = %q", got)
	}
}
