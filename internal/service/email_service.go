package service

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"strings"

	"todo-app/internal/apperrors"
	"todo-app/pkg/email"

	"go.uber.org/zap"
)

type Email interface {
	ShareTasks(ctx context.Context, recipientEmail, senderEmail, dashboardURL string) error
}

//go:embed templates/*
var templateFS embed.FS

type EmailService struct {
	mailer *email.Mailer
	logger *zap.Logger
	tmpl   *template.Template
}

type ShareTasksEmailData struct {
	SenderEmail  string
	DashboardURL string
	AppName      string
}

func NewEmailService(mailer *email.Mailer, logger *zap.Logger) (*EmailService, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/share_dashboard.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse email template: %w", err)
	}

	return &EmailService{
		mailer: mailer,
		logger: logger,
		tmpl:   tmpl,
	}, nil
}

func (s *EmailService) ShareTasks(ctx context.Context, recipientEmail, senderEmail, dashboardURL string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	recipientEmail = strings.ToLower(strings.TrimSpace(recipientEmail))
	if recipientEmail == "" {
		return apperrors.ErrEmptyRecipient
	}

	dashboardURL = strings.TrimSpace(dashboardURL)
	if dashboardURL == "" {
		return fmt.Errorf("dashboard URL cannot be empty")
	}

	data := ShareTasksEmailData{
		SenderEmail:  senderEmail,
		DashboardURL: dashboardURL,
		AppName:      "TodoApp",
	}

	var bodyBuffer bytes.Buffer
	if err := s.tmpl.Execute(&bodyBuffer, data); err != nil {
		s.logger.Error("failed to execute email template", zap.Error(err), zap.String("recipient", recipientEmail))
		return fmt.Errorf("failed to generate email body: %w", err)
	}

	subject := fmt.Sprintf("✓ TodoApp: %s поширив(-ла) вам свій список задач", senderEmail)

	if err := s.mailer.SendHTMLEmail(recipientEmail, subject, bodyBuffer.String()); err != nil {
		s.logger.Error("failed to send dashboard link email", zap.Error(err), zap.String("to", recipientEmail))
		return err
	}

	s.logger.Info("share tasks email sent successfully",
		zap.String("to", recipientEmail),
		zap.String("from", senderEmail),
	)

	return nil
}
