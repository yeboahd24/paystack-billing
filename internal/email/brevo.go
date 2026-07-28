package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type BrevoClient struct {
	apiKey string
	http   *http.Client
	from   Address
}

func NewBrevo(apiKey, fromEmail, fromName string) *BrevoClient {
	return &BrevoClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 10 * time.Second},
		from:   Address{Email: fromEmail, Name: fromName},
	}
}

func (c *BrevoClient) Send(ctx context.Context, toEmail, toName, subject, htmlBody string) error {
	payload := map[string]any{
		"sender":      c.from,
		"to":          []Address{{Email: toEmail, Name: toName}},
		"subject":     subject,
		"htmlContent": htmlBody,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("brevo send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("brevo send returned %d", resp.StatusCode)
	}
	return nil
}
