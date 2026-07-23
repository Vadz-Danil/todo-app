package service_test

import (
	"context"
	"errors"
	"testing"

	"todo-app/internal/apperrors"
	"todo-app/internal/service"
	"todo-app/pkg/email"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// stubSender records what it was handed and returns a fixed error.
type stubSender struct {
	err  error
	sent []email.Message
}

func (s *stubSender) Name() string { return "stub" }

func (s *stubSender) Send(_ context.Context, msg email.Message) error {
	s.sent = append(s.sent, msg)
	return s.err
}

func newEmailSvc(t *testing.T, sender email.Sender) *service.EmailService {
	t.Helper()
	svc, err := service.NewEmailService(sender, zap.NewNop())
	require.NoError(t, err)
	return svc
}

func TestEmailService_ShareTasks_DeliversTheRenderedTemplate(t *testing.T) {
	sender := &stubSender{}
	svc := newEmailSvc(t, sender)

	err := svc.ShareTasks(context.Background(), "  Recipient@Example.COM ", "owner@example.com", "https://app.example.com/dashboard")
	require.NoError(t, err)

	require.Len(t, sender.sent, 1)
	msg := sender.sent[0]

	assert.Equal(t, "recipient@example.com", msg.To, "the recipient should be trimmed and lowercased")
	assert.Contains(t, msg.Subject, "owner@example.com")
	assert.Contains(t, msg.HTMLBody, "https://app.example.com/dashboard")
}

func TestEmailService_ShareTasks_ProviderFailureIsAnUpstreamError(t *testing.T) {
	// A provider refusing the message (unactivated account, bad key, outage)
	// must not surface as a fault in this server.
	providerErr := errors.New("brevo: 403 Forbidden: permission_denied: account is not activated")
	svc := newEmailSvc(t, &stubSender{err: providerErr})

	err := svc.ShareTasks(context.Background(), "recipient@example.com", "owner@example.com", "https://app.example.com/dashboard")

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrEmailDelivery, "handlers map this to 502")
	assert.ErrorIs(t, err, providerErr, "the cause must stay wrapped for the log")
}

func TestEmailService_ShareTasks_RejectsBadInputBeforeSending(t *testing.T) {
	cases := []struct {
		name         string
		recipient    string
		dashboardURL string
		wantErr      error
	}{
		{"blank recipient", "   ", "https://app.example.com", apperrors.ErrEmptyRecipient},
		{"empty recipient", "", "https://app.example.com", apperrors.ErrEmptyRecipient},
		{"blank dashboard url", "r@example.com", "  ", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := &stubSender{}
			svc := newEmailSvc(t, sender)

			err := svc.ShareTasks(context.Background(), tc.recipient, "owner@example.com", tc.dashboardURL)

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			assert.Empty(t, sender.sent, "nothing should reach the provider")
		})
	}
}

func TestEmailService_ShareTasks_HonoursACancelledContext(t *testing.T) {
	sender := &stubSender{}
	svc := newEmailSvc(t, sender)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.ShareTasks(ctx, "recipient@example.com", "owner@example.com", "https://app.example.com")

	require.Error(t, err)
	assert.Empty(t, sender.sent, "a cancelled request should not send mail")
}
