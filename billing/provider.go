// Package billing provides a payment provider abstraction with a Paystack implementation.
package billing

import "context"

// PaymentProvider is the interface for payment gateway integrations.
type PaymentProvider interface {
	InitTransaction(ctx context.Context, email string, amount int, callbackURL string, metadata map[string]any) (checkoutURL, reference string, err error)
	VerifySignature(body []byte, signature string) bool
	ParseWebhook(body []byte) (*WebhookEvent, error)
	VerifyTransaction(ctx context.Context, reference string, expectedAmount int) (bool, error)
}

// WebhookEvent represents a parsed webhook payload from a payment provider.
type WebhookEvent struct {
	Event     string
	Reference string
	Status    string
	Amount    int
	Metadata  map[string]any
}
