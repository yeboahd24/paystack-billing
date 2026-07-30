// Package circuitbreaker implements a simple circuit breaker for external API resilience.
package circuitbreaker

import (
	"sync/atomic"
	"time"
)

// Breaker tracks failures and opens the circuit when a threshold is exceeded.
type Breaker struct {
	failures      atomic.Int32
	threshold     int32
	openUntil     atomic.Int64
	resetDuration time.Duration
}

// New creates a Breaker that opens after threshold failures and resets after resetDuration.
func New(threshold int32, resetDuration time.Duration) *Breaker {
	return &Breaker{
		threshold:     threshold,
		resetDuration: resetDuration,
	}
}

// Allow returns true if the circuit is closed or half-open and ready to test.
func (b *Breaker) Allow() bool {
	openUntil := b.openUntil.Load()
	if openUntil == 0 {
		return true
	}
	if time.Now().UnixNano() < openUntil {
		return false
	}
	b.openUntil.Store(0)
	b.failures.Store(0)
	return true
}

// Success resets the failure count when a request succeeds.
func (b *Breaker) Success() {
	b.failures.Store(0)
}

// Failure increments the failure count; opens the circuit if threshold is reached.
func (b *Breaker) Failure() {
	f := b.failures.Add(1)
	if f >= b.threshold {
		b.openUntil.Store(time.Now().Add(b.resetDuration).UnixNano())
	}
}
