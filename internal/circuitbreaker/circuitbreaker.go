package circuitbreaker

import (
	"sync/atomic"
	"time"
)

type Breaker struct {
	failures      atomic.Int32
	threshold     int32
	openUntil     atomic.Int64
	resetDuration time.Duration
}

func New(threshold int32, resetDuration time.Duration) *Breaker {
	return &Breaker{
		threshold:     threshold,
		resetDuration: resetDuration,
	}
}

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

func (b *Breaker) Success() {
	b.failures.Store(0)
}

func (b *Breaker) Failure() {
	f := b.failures.Add(1)
	if f >= b.threshold {
		b.openUntil.Store(time.Now().Add(b.resetDuration).UnixNano())
	}
}
