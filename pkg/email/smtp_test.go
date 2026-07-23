package email

import (
	"mime"
	"strings"
	"testing"
)

func TestBuildMIME_EncodesNonASCIISubjects(t *testing.T) {
	subject := "✓ TodoApp: user@example.com поширив(-ла) вам свій список задач"

	raw, err := buildMIME("sender@example.com", "TaskFlow", Message{
		To:       "recipient@example.com",
		Subject:  subject,
		HTMLBody: "<p>hi</p>",
	})
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}

	got := string(raw)
	line := headerValue(t, got, "Subject")

	// Raw UTF-8 in a header is not valid; clients that do not guess the charset
	// render it as mojibake.
	if line == subject {
		t.Fatal("subject was emitted as raw UTF-8 instead of being RFC 2047 encoded")
	}

	decoded, err := new(mime.WordDecoder).DecodeHeader(line)
	if err != nil {
		t.Fatalf("the encoded subject does not decode: %v", err)
	}
	if decoded != subject {
		t.Errorf("decoded subject = %q, want %q", decoded, subject)
	}
}

func TestBuildMIME_LeavesASCIISubjectsAlone(t *testing.T) {
	raw, err := buildMIME("sender@example.com", "", Message{
		To:       "recipient@example.com",
		Subject:  "Plain ASCII subject",
		HTMLBody: "<p>hi</p>",
	})
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}

	if got := headerValue(t, string(raw), "Subject"); got != "Plain ASCII subject" {
		t.Errorf("Subject = %q, want it untouched", got)
	}
}

func TestBuildMIME_FormatsTheFromHeader(t *testing.T) {
	t.Run("with a display name", func(t *testing.T) {
		raw, err := buildMIME("sender@example.com", "TaskFlow", testMessage())
		if err != nil {
			t.Fatalf("buildMIME: %v", err)
		}
		if got := headerValue(t, string(raw), "From"); got != `"TaskFlow" <sender@example.com>` {
			t.Errorf("From = %q", got)
		}
	})

	t.Run("without one", func(t *testing.T) {
		raw, err := buildMIME("sender@example.com", "", testMessage())
		if err != nil {
			t.Fatalf("buildMIME: %v", err)
		}
		if got := headerValue(t, string(raw), "From"); got != "sender@example.com" {
			t.Errorf("From = %q", got)
		}
	})
}

func TestBuildMIME_UsesCRLFAndSeparatesTheBody(t *testing.T) {
	raw, err := buildMIME("sender@example.com", "", Message{
		To:       "recipient@example.com",
		Subject:  "s",
		HTMLBody: "<p>body</p>",
	})
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}

	got := string(raw)
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Error("found a bare LF; SMTP requires CRLF line endings")
	}

	headers, body, found := strings.Cut(got, "\r\n\r\n")
	if !found {
		t.Fatal("no blank line separating headers from body")
	}
	if body != "<p>body</p>" {
		t.Errorf("body = %q", body)
	}
	if !strings.Contains(headers, "Content-Type: text/html; charset=UTF-8") {
		t.Errorf("missing HTML content type in:\n%s", headers)
	}
}

func TestBuildMIME_RequiresASender(t *testing.T) {
	if _, err := buildMIME("  ", "TaskFlow", testMessage()); err == nil {
		t.Fatal("expected an error when the sender address is blank")
	}
}

// headerValue reads a single header out of a rendered message.
func headerValue(t *testing.T, raw, name string) string {
	t.Helper()

	headers, _, _ := strings.Cut(raw, "\r\n\r\n")
	for _, line := range strings.Split(headers, "\r\n") {
		if after, ok := strings.CutPrefix(line, name+": "); ok {
			return after
		}
	}

	t.Fatalf("header %q not found in:\n%s", name, headers)
	return ""
}
