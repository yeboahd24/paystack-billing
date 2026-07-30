// Package email provides a notification abstraction with Brevo and noop implementations.
package email

import "context"

// Sender sends transactional emails.
type Sender interface {
	Send(ctx context.Context, toEmail, toName, subject, htmlBody string) error
}

// Address represents an email recipient or sender.
type Address struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}
