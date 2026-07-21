package service

import (
	"context"
	"fmt"
	"strings"

	"todo-app/internal/apperrors"
	"todo-app/pkg/email"

	"go.uber.org/zap"
)

type EmailService struct {
	mailer *email.Mailer
	logger *zap.Logger
}

func NewEmailService(mailer *email.Mailer, logger *zap.Logger) *EmailService {
	return &EmailService{
		mailer: mailer,
		logger: logger,
	}
}

func (s *EmailService) ShareTasks(ctx context.Context, recipientEmail string, taskTitles []string) error {
	recipientEmail = strings.ToLower(strings.TrimSpace(recipientEmail))
	if recipientEmail == "" {
		return apperrors.ErrEmptyRecipient
	}

	if len(taskTitles) == 0 {
		return apperrors.ErrEmptyTaskList
	}

	var bodyBuilder strings.Builder
	bodyBuilder.WriteString("<h2>Список задач, якими з вами поділилися:</h2><ul>")
	for _, title := range taskTitles {
		bodyBuilder.WriteString(fmt.Sprintf("<li>%s</li>", title))
	}
	bodyBuilder.WriteString("</ul>")

	subject := "Вам поширили список задач"
	err := s.mailer.SendHTMLEmail(recipientEmail, subject, bodyBuilder.String())
	if err != nil {
		s.logger.Error("failed to send task list email", zap.Error(err), zap.String("to", recipientEmail))
		return err
	}

	return nil
}
