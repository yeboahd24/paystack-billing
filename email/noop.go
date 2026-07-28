package email

import (
	"context"
	"log"
)

type NoopClient struct{}

func NewNoop() *NoopClient {
	return &NoopClient{}
}

func (c *NoopClient) Send(ctx context.Context, toEmail, toName, subject, htmlBody string) error {
	log.Printf("email noop: to=%s <%s> subject=%q", toName, toEmail, subject)
	return nil
}
