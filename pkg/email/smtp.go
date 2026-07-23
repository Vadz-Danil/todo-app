package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	Timeout  time.Duration
}

// SMTPSender talks to a mail server over SMTP. Useful locally; on hosts that
// block outbound SMTP ports it will fail to dial no matter what the
// credentials are, which is what BrevoSender exists to work around.
type SMTPSender struct {
	cfg SMTPConfig
}

func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &SMTPSender{cfg: cfg}
}

func (s *SMTPSender) Name() string { return "smtp" }

func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	body, err := buildMIME(s.cfg.From, s.cfg.FromName, msg)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))

	client, err := s.dial(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	// A cancelled request should not keep the conversation going.
	if err := ctx.Err(); err != nil {
		return err
	}

	if s.cfg.Username != "" && s.cfg.Password != "" {
		ok, _ := client.Extension("AUTH")
		if !ok {
			// Continuing here would silently send unauthenticated mail and fail
			// later at RCPT with a misleading "relay denied".
			return fmt.Errorf("SMTP server at %s does not advertise AUTH but credentials were configured", addr)
		}
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err := client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("failed MAIL command: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("failed RCPT command: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed DATA command: %w", err)
	}

	_, writeErr := w.Write(body)
	closeErr := w.Close()

	if writeErr != nil {
		return fmt.Errorf("failed to write email body: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close body writer: %w", closeErr)
	}

	_ = client.Quit()

	return nil
}

func (s *SMTPSender) dial(ctx context.Context, addr string) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: s.cfg.Timeout}

	// Port 465 is implicit TLS: the connection is encrypted before the SMTP
	// greeting. Everything else starts in the clear and upgrades via STARTTLS.
	if s.cfg.Port == 465 {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.cfg.Host})
		if err != nil {
			return nil, fmt.Errorf("failed to dial SSL (port 465): %w", err)
		}
		client, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to create SMTP client: %w", err)
		}
		return client, nil
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial TCP: %w", err)
	}

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("failed StartTLS: %w", err)
		}
		return client, nil
	}

	// No STARTTLS on offer. That is normal for a local capture server such as
	// MailHog, but sending real credentials over a plaintext link is not.
	if s.cfg.Username != "" || s.cfg.Password != "" {
		_ = client.Close()
		return nil, fmt.Errorf("SMTP server at %s does not support STARTTLS; refusing to send credentials in the clear", addr)
	}

	return client, nil
}

// buildMIME renders headers plus body with CRLF line endings, as SMTP requires.
func buildMIME(from, fromName string, msg Message) ([]byte, error) {
	if strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("email: sender address is not configured")
	}

	sender := formatAddress(from, fromName)

	// Header order is fixed rather than map-ranged so the output is stable and
	// spam filters see a conventional layout.
	var b strings.Builder
	b.WriteString("From: " + sender + "\r\n")
	b.WriteString("To: " + msg.To + "\r\n")
	b.WriteString("Subject: " + encodeHeader(msg.Subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.HTMLBody)

	return []byte(b.String()), nil
}

// encodeHeader RFC 2047-encodes a header value. The share subject line is
// Ukrainian, and raw UTF-8 in a header is not valid — clients that do not
// guess the charset render it as mojibake. ASCII passes through untouched.
func encodeHeader(v string) string {
	return mime.QEncoding.Encode("UTF-8", v)
}
