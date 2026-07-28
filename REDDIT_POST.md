We built a Paystack billing system in Go — here's the architecture we ended up with

For the past few months we've been running two Go APIs (GhanaPostGPS + verify.afrihex.com) that needed Paystack subscriptions with rate limiting, tier management, and usage tracking.

The Paystack Go SDKs handle API calls but left a lot of common billing infrastructure to rebuild each time. Here's what we built:

- **Paystack subscriptions** with idempotent webhook handling (ref-based dedup)
- **Redis rate limiting** (daily limit per tier + per-minute secondary cap)
- **Plans stored in the DB** — no hardcoded pricing, no code deploys for price changes
- **Usage tracking** with daily counters and history
- **Email with a swappable Sender interface** (Brevo by default, implement SendGrid/SMTP by writing 1 method)
- **Circuit breaker** for external API calls (5 failures → 30s cooldown → half-open probe)
- **Background subscription expiry sweeper** (5 min interval, 30s timeout per sweep)
- **SQLC-generated queries** (SQLite by default, swap to Postgres by implementing the repository interfaces)

The interface pattern means you can swap Paystack for Stripe, Flutterwave, or Razorpay by implementing 3 methods. Email swap is 1 method.

We extracted this into a standalone library in case anyone else is building similar billing infrastructure in Go:

https://github.com/yeboahd24/paystack-billing

Curious what other approaches people have taken for Paystack billing in Go — especially around tier gating and idempotent webhook processing.
