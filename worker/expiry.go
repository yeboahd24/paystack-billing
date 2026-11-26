// Package worker implements background tasks for subscription management.
package worker

import (
	"context"
	"log"
	"time"

	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/subscription"
)

const expiryInterval = 5 * time.Minute

// RunExpirySweep periodically ends expired (and cancelled-and-expired)
// subscriptions and moves those users to the free tier. Each sweep bounds its
// own steps, so a large batch of expiries doesn't time out as a whole.
func RunExpirySweep(ctx context.Context, svc *subscription.Service) {
	ticker := time.NewTicker(expiryInterval)
	defer ticker.Stop()

	svc.SweepExpired(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			svc.SweepExpired(ctx)
		}
	}
}

// RunPlanRefresh reloads plans from the database every interval, so price and
// limit changes apply without a restart. A failed refresh keeps the old plans.
func RunPlanRefresh(ctx context.Context, plans *plan.Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := plans.Refresh(rctx); err != nil {
				log.Printf("plans: refresh: %v", err)
			}
			cancel()
		}
	}
}
