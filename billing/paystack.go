package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yeboahd24/paystack-billing/circuitbreaker"
)

// defaultBaseURL is Paystack's API. Tests point baseURL at a local server.
const defaultBaseURL = "https://api.paystack.co"

// maxResponseBytes bounds what we read from Paystack.
const maxResponseBytes = 1 << 20

// PaystackProvider implements PaymentProvider for the Paystack API.
type PaystackProvider struct {
	secretKey string
	currency  string
	baseURL   string
	http      *http.Client
	cb        *circuitbreaker.Breaker
}

// NewPaystack creates a PaystackProvider with the given secret key and currency.
func NewPaystack(secretKey, currency string) *PaystackProvider {
	return &PaystackProvider{
		secretKey: secretKey,
		currency:  currency,
		baseURL:   defaultBaseURL,
		http:      &http.Client{Timeout: 15 * time.Second},
		cb:        circuitbreaker.New(5, 30*time.Second),
	}
}

func (p *PaystackProvider) InitTransaction(ctx context.Context, email string, amount int, callbackURL string, metadata map[string]any) (checkoutURL, reference string, err error) {
	if !p.cb.Allow() {
		return "", "", fmt.Errorf("paystack circuit breaker open")
	}

	body, _ := json.Marshal(map[string]any{
		"email":        email,
		"amount":       amount,
		"currency":     p.currency,
		"callback_url": callbackURL,
		"metadata":     metadata,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/transaction/initialize", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("paystack request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		p.cb.Failure()
		return "", "", fmt.Errorf("paystack init: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		p.cb.Failure()
		return "", "", fmt.Errorf("paystack: invalid secret key (401)")
	}

	var out struct {
		Status bool `json:"status"`
		Data   struct {
			AuthorizationURL string `json:"authorization_url"`
			Reference        string `json:"reference"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		p.cb.Failure()
		return "", "", fmt.Errorf("paystack decode: %w", err)
	}
	if !out.Status {
		p.cb.Failure()
		return "", "", fmt.Errorf("paystack init failed: %s", out.Message)
	}

	p.cb.Success()
	return out.Data.AuthorizationURL, out.Data.Reference, nil
}

func (p *PaystackProvider) VerifySignature(body []byte, signature string) bool {
	mac := hmac.New(sha512.New, []byte(p.secretKey))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

type paystackWebhook struct {
	Event string `json:"event"`
	Data  struct {
		Reference string         `json:"reference"`
		Status    string         `json:"status"`
		Amount    int            `json:"amount"`
		Metadata  map[string]any `json:"metadata"`
	} `json:"data"`
}

func (p *PaystackProvider) ParseWebhook(body []byte) (*WebhookEvent, error) {
	var raw paystackWebhook
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("paystack webhook parse: %w", err)
	}
	return &WebhookEvent{
		Event:     raw.Event,
		Reference: raw.Data.Reference,
		Status:    raw.Data.Status,
		Amount:    raw.Data.Amount,
		Metadata:  raw.Data.Metadata,
	}, nil
}

// ReadBody reads the entire body from a reader.
func ReadBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// VerifyTransaction asks Paystack whether a transaction succeeded for exactly
// expectedAmount in the provider's currency.
//
// It returns (false, nil) only when Paystack answered and the payment is not
// good: not found, not successful, or a different amount or currency. When
// Paystack could not be asked (network error, timeout, 5xx, 429, an
// unreadable reply), it returns an error, so callers can tell the customer to
// retry instead of telling them their payment failed.
func (p *PaystackProvider) VerifyTransaction(ctx context.Context, reference string, expectedAmount int) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.baseURL+"/transaction/verify/"+url.PathEscape(reference), nil)
	if err != nil {
		return false, fmt.Errorf("paystack verify request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("paystack verify: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest:
		// Paystack answered: no such transaction for this account.
		return false, nil
	case resp.StatusCode == http.StatusUnauthorized:
		return false, fmt.Errorf("paystack verify: invalid secret key (401)")
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return false, fmt.Errorf("paystack verify: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Status bool `json:"status"`
		Data   struct {
			Status   string `json:"status"`
			Amount   int    `json:"amount"`
			Currency string `json:"currency"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&result); err != nil {
		return false, fmt.Errorf("paystack verify decode: %w", err)
	}
	if !result.Status {
		return false, nil
	}
	if result.Data.Status != "success" {
		return false, nil
	}
	if result.Data.Amount != expectedAmount {
		return false, nil
	}
	if !strings.EqualFold(result.Data.Currency, p.currency) {
		return false, nil
	}
	return true, nil
}
