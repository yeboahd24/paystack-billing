package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/yeboahd24/paystack-billing/billing"
	"github.com/yeboahd24/paystack-billing/email"
	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/ratelimit"
	"github.com/yeboahd24/paystack-billing/subscription"
	"github.com/yeboahd24/paystack-billing/usage"
	"github.com/yeboahd24/paystack-billing/worker"
	"github.com/yeboahd24/paystack-billing/store/sqlite"
)

func main() {
	_ = godotenv.Load()
	// ── Config ─────────────────────────────────────────────
	paystackSecret := os.Getenv("PAYSTACK_SECRET_KEY")
	paystackCurrency := os.Getenv("PAYSTACK_CURRENCY")
	if paystackCurrency == "" {
		paystackCurrency = "GHS"
	}
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	sqlitePath := os.Getenv("SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath = "./data/billing.db"
	}

	// Validate critical config at startup, not mid-request.
	if paystackSecret != "" && len(paystackSecret) < 10 {
		log.Fatal("PAYSTACK_SECRET_KEY looks invalid (too short)")
	}
	if os.Getenv("BREVO_API_KEY") != "" && len(os.Getenv("BREVO_API_KEY")) < 20 {
		log.Fatal("BREVO_API_KEY looks invalid (too short)")
	}

	// ── Store ──────────────────────────────────────────────
	db, err := sqlite.Open(sqlitePath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	planRepo := sqlite.NewPlanRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	userRepo := sqlite.NewUserRepository(db)
	usageRepo := sqlite.NewUsageRepository(db)

	// ── Redis ──────────────────────────────────────────────
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	rdb := redis.NewClient(opts)
	defer rdb.Close()

	// ── Email sender ───────────────────────────────────────
	var sender email.Sender
	if key := os.Getenv("BREVO_API_KEY"); key != "" {
		sender = email.NewBrevo(key, os.Getenv("BREVO_FROM_EMAIL"), os.Getenv("BREVO_FROM_NAME"))
		log.Println("email: using Brevo")
	} else {
		sender = email.NewNoop()
		log.Println("email: using noop (set BREVO_API_KEY for production)")
	}

	// ── Payment provider ───────────────────────────────────
	var pay billing.PaymentProvider
	if paystackSecret != "" {
		pay = billing.NewPaystack(paystackSecret, paystackCurrency)
		log.Printf("billing: using Paystack (%s)", paystackCurrency)
	} else {
		log.Println("billing: no PAYSTACK_SECRET_KEY set — checkout will fail")
	}

	// ── Services ───────────────────────────────────────────
	plans, err := plan.NewService(context.Background(), planRepo)
	if err != nil {
		log.Fatalf("plans: %v", err)
	}

	subSvc := subscription.NewService(subRepo, userRepo, pay, plans, sender)
	usageSvc := usage.NewService(usageRepo, plans)
	limiter := ratelimit.New(rdb)

	// ── Seed plans (first run) ─────────────────────────────
	if len(plans.List()) == 0 {
		log.Println("seeding default plans...")
		db.ExecContext(context.Background(),
			`INSERT INTO plans (name, display_name, amount, daily_limit, description) VALUES ('free', 'Free', 0, 20, '20 requests/day')`)
		db.ExecContext(context.Background(),
			`INSERT INTO plans (name, display_name, amount, daily_limit, description) VALUES ('starter', 'Starter', 9900, 500, '500 requests/day')`)
		db.ExecContext(context.Background(),
			`INSERT INTO plans (name, display_name, amount, daily_limit, retention_days, description) VALUES ('pro', 'Professional', 49900, 5000, 90, '5000 requests/day, 90-day retention')`)
		plans, _ = plan.NewService(context.Background(), planRepo)
	}

	// ── Rate-limited middleware ─────────────────────────────
	rateLimited := func(next http.HandlerFunc, tier string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			p := plans.Get(tier)
			if p == nil {
				http.Error(w, "unknown tier", http.StatusInternalServerError)
				return
			}
			allowed, remaining, err := limiter.Allow(r.Context(), "api:"+r.RemoteAddr, int(p.DailyLimit))
			if err != nil || !allowed {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			next(w, r)
		}
	}

	// ── HTTP routes ────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /webhooks/paystack", func(w http.ResponseWriter, r *http.Request) {
		body, err := billing.ReadBody(r.Body)
		if err != nil || len(body) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		sig := r.Header.Get("x-paystack-signature")
		if !pay.VerifySignature(body, sig) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		event, err := pay.ParseWebhook(body)
		if err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		if event.Event == "charge.success" {
			userID, ok := event.Metadata["user_id"].(float64)
			if !ok {
				http.Error(w, "bad metadata", http.StatusBadRequest)
				return
			}
			planName, ok := event.Metadata["plan"].(string)
			if !ok {
				http.Error(w, "bad metadata", http.StatusBadRequest)
				return
			}
			if err := subSvc.HandleChargeSuccess(r.Context(), int64(userID), planName, event.Reference); err != nil {
				log.Printf("webhook: charge.success failed: %v", err)
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /checkout", rateLimited(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID int64  `json:"user_id"`
			Plan   string `json:"plan"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		url, ref, err := subSvc.InitCheckout(r.Context(), req.UserID, req.Plan, "https://example.com/callback")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"url": url, "reference": ref})
	}, "starter"))

	mux.HandleFunc("GET /usage", rateLimited(func(w http.ResponseWriter, r *http.Request) {
		userID := int64(1)
		tier := "free"
		history, err := usageSvc.GetHistory(r.Context(), userID, tier, 30)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(history)
	}, "free"))

	// ── Background workers ────────────────────────────────
	bgCtx, bgCancel := context.WithCancel(context.Background())
	go worker.RunExpirySweep(bgCtx, subSvc)

	// ── HTTP server ────────────────────────────────────────
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	bgCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
