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

### Run the example server

```bash
git clone https://github.com/yeboahd24/paystack-billing.git
cd paystack-billing
cp .env.example .env
# Fill in PAYSTACK_SECRET_KEY (test key is fine)

redis-server &  # or: docker run -d -p 6379:6379 redis:7-alpine
go run ./cmd/example
```

```bash
$ curl http://localhost:8080/health
{"status":"ok"}

$ curl http://localhost:8080/usage
{"Daily":null,"Today":0,"Allowed":20}
```

### Import as a library in your own Go project

```bash
go get github.com/yeboahd24/paystack-billing
```

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/yeboahd24/paystack-billing/billing"
	"github.com/yeboahd24/paystack-billing/email"
	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/subscription"
	"github.com/yeboahd24/paystack-billing/worker"
	"github.com/yeboahd24/paystack-billing/store/sqlite"
)

func main() {
	// 1. Open SQLite (auto-migrates)
	db, _ := sqlite.Open("./data/billing.db")

	planRepo := sqlite.NewPlanRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	userRepo := sqlite.NewUserRepository(db)

	// 2. Load plans from DB (create 'free', 'starter', 'pro' rows)
	plans, _ := plan.NewService(context.Background(), planRepo)

	// 3. Create payment provider
	pay := billing.NewPaystack(os.Getenv("PAYSTACK_SECRET_KEY"), "GHS")

	// 4. Create email sender
	var sender email.Sender = email.NewNoop()
	if key := os.Getenv("BREVO_API_KEY"); key != "" {
		sender = email.NewBrevo(key, os.Getenv("BREVO_FROM_EMAIL"), os.Getenv("BREVO_FROM_NAME"))
	}

	// 5. Create billing service
	subSvc := subscription.NewService(subRepo, userRepo, pay, plans, sender)

	// 6. Start background expiry sweeper
	go worker.RunExpirySweep(context.Background(), subSvc)

	// 7. Handle a Paystack webhook
	// subSvc.HandleChargeSuccess(ctx, userID, "pro", "paystack_ref_123")

	// 8. Check if user's tier is good enough for a feature
	// if !plans.TierAtLeast(user.Tier, "starter") { reject }
}
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
