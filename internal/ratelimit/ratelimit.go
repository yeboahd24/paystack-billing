package ratelimit

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	rdb           *redis.Client
	redisDown     atomic.Bool
	lastRedisFail atomic.Int64
}

func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

func (l *Limiter) Allow(ctx context.Context, key string, dailyLimit int) (allowed bool, remaining int, err error) {
	l.checkRecovered()

	count, err := l.rdb.Incr(ctx, dailyKey(key)).Result()
	if err != nil {
		l.recordFailure()
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

func (l *Limiter) AllowPerMinute(ctx context.Context, key string, perMinute int) (allowed bool, err error) {
	l.checkRecovered()

	count, err := l.rdb.Incr(ctx, perMinKey(key)).Result()
	if err != nil {
		l.recordFailure()
		return true, nil
	}
	l.redisDown.Store(false)
	if count == 1 {
		l.rdb.Expire(ctx, perMinKey(key), 70*time.Second)
	}
	return int(count) <= perMinute, nil
}

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
