// Package email delivers transactional mail through a pluggable transport.
//
// Two transports exist because they solve different problems: SMTP is the
// obvious choice locally, but most PaaS providers (Railway among them) block
// outbound SMTP ports on their cheaper plans to stop spam, so a deployed build
// has to reach the mail provider over ordinary HTTPS instead.
package email

import (
	"context"
	"fmt"
	"strings"
)

// Message is one outbound HTML email.
type Message struct {
	To       string
	Subject  string
	HTMLBody string
}

// Validate rejects the mistakes a transport cannot recover from, so both
// senders fail the same way rather than each inventing its own error.
func (m Message) Validate() error {
	if strings.TrimSpace(m.To) == "" {
		return fmt.Errorf("email: recipient is required")
	}
	if strings.TrimSpace(m.Subject) == "" {
		return fmt.Errorf("email: subject is required")
	}
	if strings.TrimSpace(m.HTMLBody) == "" {
		return fmt.Errorf("email: body is required")
	}
	return nil
}

// Sender is the transport seam. Implementations must respect ctx cancellation
// so a request that gives up does not leave a socket dangling.
type Sender interface {
	Send(ctx context.Context, msg Message) error
	// Name identifies the transport in logs and health output.
	Name() string
}
