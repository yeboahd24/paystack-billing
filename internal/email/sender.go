package email

import "context"

type Sender interface {
	Send(ctx context.Context, toEmail, toName, subject, htmlBody string) error
}

type Address struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}
