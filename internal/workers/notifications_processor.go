// internal/workers/notification_processor.go
package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/smtp"

	"github.com/ammerola/resell-be/internal/pkg/config"
	"github.com/hibiken/asynq"
)

// NotificationProcessor handles email notifications
type NotificationProcessor struct {
	config *config.Config
	logger *slog.Logger
}

// NewNotificationProcessor creates a new notification processor
func NewNotificationProcessor(config *config.Config, logger *slog.Logger) *NotificationProcessor {
	return &NotificationProcessor{
		config: config,
		logger: logger.With(slog.String("processor", "notification")),
	}
}

// SendEmail sends email notifications
func (p *NotificationProcessor) SendEmail(ctx context.Context, t *asynq.Task) error {
	var payload map[string]interface{}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	to := payload["to"].(string)
	subject := payload["subject"].(string)
	body := payload["body"].(string)

	p.logger.InfoContext(ctx, "sending email",
		slog.String("to", to),
		slog.String("subject", subject))

	// In development, just log the email
	if p.config.App.Environment == "development" {
		p.logger.InfoContext(ctx, "email would be sent",
			slog.String("to", to),
			slog.String("subject", subject),
			slog.String("body", body))
		return nil
	}

	// Production email sending
	// This is a simplified version - in production you'd use a service like SendGrid
	from := "noreply@resell.com"
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		from, to, subject, body,
	))

	// Send via SMTP (configure your SMTP settings)
	auth := smtp.PlainAuth("", "", "", "smtp.example.com")
	err := smtp.SendMail("smtp.example.com:587", auth, from, []string{to}, msg)

	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	p.logger.InfoContext(ctx, "email sent successfully")
	return nil
}
