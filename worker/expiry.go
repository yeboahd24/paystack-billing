package worker

import (
	"context"
	"time"

	"github.com/yeboahd24/paystack-billing/subscription"
)

const (
	expiryInterval = 5 * time.Minute
	sweepTimeout   = 30 * time.Second
)

func RunExpirySweep(ctx context.Context, svc *subscription.Service) {
	ticker := time.NewTicker(expiryInterval)
	defer ticker.Stop()

	sweep(ctx, svc)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep(ctx, svc)
		}
	}
}

func sweep(ctx context.Context, svc *subscription.Service) {
	timeoutCtx, cancel := context.WithTimeout(ctx, sweepTimeout)
	defer cancel()
	svc.SweepExpired(timeoutCtx)
}
