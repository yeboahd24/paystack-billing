package email

import (
	"context"
	"log"
)

// NoopClient logs email sends without delivering them.
type NoopClient struct{}

// NewNoop creates a noop email client for development.
func NewNoop() *NoopClient {
	return &NoopClient{}
}

func (c *NoopClient) Send(ctx context.Context, toEmail, toName, subject, htmlBody string) error {
	log.Printf("email noop: to=%s <%s> subject=%q", toName, toEmail, subject)
	return nil
}
