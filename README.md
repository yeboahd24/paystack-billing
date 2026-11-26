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
	"time"

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

	// 5. Create billing service (db is also the transaction provider)
	subSvc := subscription.NewService(subRepo, userRepo, db, pay, plans, sender)

	// 6. Background workers: expiry sweep, and plan reload so price changes
	//    apply without a restart
	go worker.RunExpirySweep(context.Background(), subSvc)
	go worker.RunPlanRefresh(context.Background(), plans, 10*time.Minute)

	// 7. Checkout records a pending subscription with the amount charged
	// url, ref, err := subSvc.InitCheckout(ctx, userID, "pro", callbackURL)

	// 8. In the Paystack webhook (see cmd/example for the full handler):
	// err := subSvc.HandleChargeSuccess(ctx, 0, "", event.Reference)   // charge.success
	// err := subSvc.HandleRefund(ctx, event.Reference, int64(event.Amount)) // refund.processed
	// if err != nil && !subscription.IsPermanent(err) { return 503 } // Paystack retries

	// 9. Check if user's tier is good enough for a feature
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
| POST | `/checkout` | Initiate subscription checkout (rate-limited; needs auth) |
| POST | `/subscription/cancel` | Stop renewal; access runs to the period end (needs auth) |
| POST | `/webhooks/paystack` | Paystack webhook: `charge.success`, `refund.processed` (disputes are logged) |

The example has no real authentication. Replace `authenticatedUserID` with your
own; for a local try-out, `EXAMPLE_INSECURE_DEMO_AUTH=1` trusts an
`X-Demo-User-ID` header.
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

Implement 4 methods:

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

func (p *MyProvider) VerifyTransaction(ctx context.Context, reference string, expectedAmount int) (bool, error) {
    // (false, nil) only when the provider says the payment is not good;
    // an error when it couldn't be asked (so the webhook is retried)
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

## Subscription lifecycle

Each payment is one row in `subscriptions`, keyed by its Paystack reference. A
user has at most one **current** row (enforced by a unique index):

| Status | Meaning |
|---|---|
| `pending` | Checkout started; records the amount charged |
| `active` | Paid; ends at `current_period_end` |
| `cancelled` | Paid, won't renew; access continues until `current_period_end` |
| `superseded` | Replaced by a renewal or plan change |
| `expired` | Period ended; user moved to free |
| `refunded` | Refunded; its time removed |

- **Renewal** (same plan): the new month starts where the current one ends, so
  paying early loses nothing.
- **Plan change**: starts now for a month, plus the unused time of the old plan
  converted at the price ratio (10 days of a plan half the price → 5 extra days).
- **Price changes** don't break payments already in flight: a payment is checked
  against the amount recorded at checkout.
- **Months** are calendar months clamped to the month's end (31 Jan → 28 Feb).
- **Refunds** (`refund.processed`) take the refunded month off; with no paid time
  left the user moves to free. Partial refunds change nothing and are logged for
  review. Disputes are logged; accepting one makes Paystack send a refund.

## Webhook retries

Paystack redelivers a webhook until it gets a 2xx. Answer **200** when the event
was applied or can never apply (`subscription.IsPermanent(err)`: rejected
payment, unknown plan or reference, partial refund); answer **5xx** for anything
else (database or Paystack trouble) so it is retried. Handlers are idempotent by
reference, so redeliveries are safe.

## Production notes

- Plans are stored in the database — no code deploy needed to change pricing (`worker.RunPlanRefresh` reloads them)
- Rate limiter **fails open** by default: if Redis is down, requests pass through unlimited until it recovers. Use `ratelimit.New(rdb, ratelimit.WithFailClosed())` to refuse them instead (`ErrUnavailable`)
- Rate-limit by client IP or user ID, never `RemoteAddr` (it includes the port, so every connection gets a fresh counter)
- Circuit breaker opens after 5 consecutive Paystack failures, resets after 30s
- Subscription expiry sweeps every 5 minutes, in batches with a timeout per step; expiry emails don't hold up the database work
- Webhook bodies are capped at 1 MB (`billing.ReadBody`); metadata is parsed leniently (Paystack echoes numbers as strings)
- Webhooks are idempotent — duplicate deliveries are ignored
