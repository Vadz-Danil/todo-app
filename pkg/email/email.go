package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type Mailer struct {
	cfg Config
}

func NewMailer(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

func (m *Mailer) SendHTMLEmail(to string, subject, htmlBody string) error {
	headers := make(map[string]string)
	headers["From"] = m.cfg.From
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var message strings.Builder
	for k, v := range headers {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n" + htmlBody)

	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", m.cfg.Port))

	client, err := m.newSMTPClient(addr)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	if m.cfg.Username != "" && m.cfg.Password != "" {
		auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("SMTP auth failed: %w", err)
			}
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("failed MAIL command: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed RCPT command: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed DATA command: %w", err)
	}

	_, writeErr := w.Write([]byte(message.String()))
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

func (m *Mailer) newSMTPClient(addr string) (*smtp.Client, error) {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	if m.cfg.Port == 465 {
		tlsConfig := &tls.Config{
			ServerName: m.cfg.Host,
		}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to dial SSL (port 465): %w", err)
		}

		client, err := smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to create SMTP client: %w", err)
		}
		return client, nil
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial TCP: %w", err)
	}

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: m.cfg.Host}
		if err := client.StartTLS(config); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("failed StartTLS: %w", err)
		}
	}

	return client, nil
}
