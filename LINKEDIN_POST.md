We've been using this internally for some months. Today we're open-sourcing it.

Every time we needed to add billing to a Go API for an African market, we had to rebuild the same plumbing — rate limiting, tier management, webhook idempotency, subscription expiry, usage tracking. Paystack SDKs handle API calls. They don't handle any of that.

So we built it once, ran it in production across two APIs (GhanaPostGPS + https://verify.afrihex.com), and now we're extracting it so nobody else has to rebuild it.

https://github.com/yeboahd24/paystack-billing

What's inside:
• Paystack subscriptions — idempotent webhooks, tested in production
• Redis rate limiting — daily + per-minute, degrades gracefully when Redis is down
• DB-backed plan management — change pricing without code deploys
• Usage tracking — daily counters, dashboard history
• Swappable email — Brevo today, SendGrid or SMTP tomorrow (1 interface)
• Circuit breaker — 5 failures, 30s cooldown, half-open probe
• Subscription expiry worker — sweeps every 5 minutes, 30s timeout
• SQLC-generated queries — SQLite by default, Postgres when you outgrow it

Swap Paystack for Stripe, Flutterwave, or Razorpay by implementing 3 methods. Swap email by implementing 1.

Built for production. Battle-tested. Finally open.

#Go #Paystack #Africa #OpenSource #Billing
