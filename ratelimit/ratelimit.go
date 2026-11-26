// Package ratelimit provides Redis-based rate limiting with graceful degradation.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrUnavailable is returned by a fail-closed limiter when Redis can't be reached.
var ErrUnavailable = errors.New("rate limiter unavailable")

// Limiter enforces daily and per-minute rate limits backed by Redis.
//
// By default it fails open: if Redis is down, every request is allowed (and
// err is nil) until it recovers, so an outage of Redis is not an outage of your
// API, but limits are not enforced meanwhile. Use WithFailClosed to refuse
// requests instead (allowed=false, err=ErrUnavailable), e.g. for endpoints that
// cost you money per call.
type Limiter struct {
	rdb           *redis.Client
	failClosed    bool
	redisDown     atomic.Bool
	lastRedisFail atomic.Int64
}

// Option configures a Limiter.
type Option func(*Limiter)

// WithFailClosed makes the limiter refuse requests while Redis is unreachable.
func WithFailClosed() Option { return func(l *Limiter) { l.failClosed = true } }

// New creates a rate limiter backed by the given Redis client.
func New(rdb *redis.Client, opts ...Option) *Limiter {
	l := &Limiter{rdb: rdb}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Degraded reports whether Redis failed recently (limits may not be enforced).
func (l *Limiter) Degraded() bool { return l.redisDown.Load() }

// Allow checks the daily rate limit for the given key.
func (l *Limiter) Allow(ctx context.Context, key string, dailyLimit int) (allowed bool, remaining int, err error) {
	l.checkRecovered()

	count, err := l.rdb.Incr(ctx, dailyKey(key)).Result()
	if err != nil {
		l.recordFailure()
		if l.failClosed {
			return false, 0, ErrUnavailable
		}
		return true, dailyLimit, nil
	}
	l.redisDown.Store(false)
	if count == 1 {
		l.rdb.Expire(ctx, dailyKey(key), 25*time.Hour)
	}

	if int(count) > dailyLimit {
		return false, 0, nil
	}
	return true, dailyLimit - int(count), nil
}

// AllowPerMinute checks the per-minute rate limit for the given key.
func (l *Limiter) AllowPerMinute(ctx context.Context, key string, perMinute int) (allowed bool, err error) {
	l.checkRecovered()

	count, err := l.rdb.Incr(ctx, perMinKey(key)).Result()
	if err != nil {
		l.recordFailure()
		if l.failClosed {
			return false, ErrUnavailable
		}
		return true, nil
	}
	l.redisDown.Store(false)
	if count == 1 {
		l.rdb.Expire(ctx, perMinKey(key), 70*time.Second)
	}
	return int(count) <= perMinute, nil
}

// Peek returns current daily usage without incrementing the counter.
func (l *Limiter) Peek(ctx context.Context, key string, dailyLimit int) (used, remaining int, err error) {
	l.checkRecovered()

	count, err := l.rdb.Get(ctx, dailyKey(key)).Int()
	if err != nil && err != redis.Nil {
		l.recordFailure()
		return 0, dailyLimit, nil
	}
	l.redisDown.Store(false)
	remaining = max(dailyLimit-count, 0)
	return count, remaining, nil
}

func (l *Limiter) recordFailure() {
	l.lastRedisFail.Store(time.Now().Unix())
	l.redisDown.Store(true)
}

func (l *Limiter) checkRecovered() {
	if l.redisDown.Load() {
		if time.Since(time.Unix(l.lastRedisFail.Load(), 0)) > 30*time.Second {
			l.redisDown.Store(false)
		}
	}
}

func dailyKey(key string) string {
	return fmt.Sprintf("ratelimit:%s:%s", key, time.Now().UTC().Format("2006-01-02"))
}

func perMinKey(key string) string {
	return fmt.Sprintf("ratelimit:permin:%s:%s", key, time.Now().UTC().Format("2006-01-02T15:04"))
}
