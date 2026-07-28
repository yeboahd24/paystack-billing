# Go Paystack Billing Starter

A modular, embeddable Go package for adding subscriptions, rate limiting, and usage tracking to any API product with Paystack (or any payment provider).

## What's included

```
├── Switchable payment provider (Paystack default, Stripe/Flutterwave/Razorpay via interface)
├── Redis-based rate limiting (daily + per-minute caps)
├── DB-backed plan management (no hardcoded pricing)
├── Subscription lifecycle (checkout, webhook, upgrade, downgrade, cancel)
├── Usage tracking (daily counters, dashboard history)
├── Swappable email (Brevo, SendGrid, SMTP, noop)
├── Circuit breaker for external API calls
├── SQLC-generated database queries (SQLite, swappable to Postgres)
└── Background worker (subscription expiry sweep)
```

## Quick start

```bash
cp .env.example .env
# Fill in your API keys

redis-server &  # or: docker run -d -p 6379:6379 redis:7-alpine
go run ./cmd/example
```

```
$ curl http://localhost:8080/health
{"status":"ok"}

$ curl http://localhost:8080/usage
{"Daily":null,"Today":0,"Allowed":20}
```

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `BREVO_API_KEY` | No | — | Transactional email via Brevo (noop when unset) |
| `BREVO_FROM_EMAIL` | No | — | Sender email address |
| `BREVO_FROM_NAME` | No | — | Sender display name |
| `PAYSTACK_SECRET_KEY` | Yes | — | Paystack secret key (live or `sk_test_*`) |
| `PAYSTACK_CURRENCY` | No | `GHS` | Currency code (`GHS`, `NGN`, `KES`, `USD`) |
| `REDIS_URL` | No | `redis://localhost:6379/0` | Redis connection string |
| `SQLITE_PATH` | No | `./data/billing.db` | SQLite file path |

## Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| POST | `/checkout` | Initiate subscription checkout (rate-limited) |
| POST | `/webhooks/paystack` | Paystack payment webhook |
| GET | `/usage` | Usage history (rate-limited) |

## Architecture

```
                    HTTP handlers
                         │
               ┌─────────┴─────────┐
               │   Service layer   │
               │  plan, billing,   │
               │  subscription,    │
               │  usage, worker    │
               └─────────┬─────────┘
                         │
    ┌────────────────────┼────────────────────┐
    │                    │                    │
email.Sender     PaymentProvider       Repository (interface)
    │                    │                    │
    ├─ Brevo       ┌─────┴──────┐      ┌──────┴──────┐
    ├─ SendGrid    │  Paystack  │      │  SQLite     │
    └─ Noop        │  Stripe    │      │  (sqlc)     │
                   │  Flutterwave│     └─────────────┘
                   └────────────┘
```

## Adding a payment provider

Implement 3 methods:

```go
type MyProvider struct{ apiKey string }

func (p *MyProvider) InitTransaction(ctx context.Context, email string, amount int, callbackURL string, metadata map[string]any) (string, string, error) {
    // Call provider API, return checkout URL and reference
}

func (p *MyProvider) VerifySignature(body []byte, signature string) bool {
    // HMAC or other signature check
}

func (p *MyProvider) ParseWebhook(body []byte) (*billing.WebhookEvent, error) {
    // Parse provider webhook into standard event
}
```

## Adding an email provider

Implement 1 method:

```go
type MyEmailClient struct{}

func (c *MyEmailClient) Send(ctx context.Context, toEmail, toName, subject, htmlBody string) error {
    // Send via your provider
}
```

## Regenerating sqlc code

```bash
sqlc generate
```

## Production notes

- Plans are stored in the database — no code deploy needed to change pricing
- Rate limiter degrades open if Redis is down (requests pass through)
- Circuit breaker opens after 5 consecutive Paystack failures, resets after 30s
- Subscription expiry sweeps every 5 minutes (30s timeout per sweep)
- Webhooks are idempotent — duplicate deliveries are ignored
