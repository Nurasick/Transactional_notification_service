package ratelimit_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Nurasick/transactional-notification-service/internal/ratelimit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLimiter(t *testing.T) (*ratelimit.Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return ratelimit.New(client), mr
}

func TestAllow_UnderLimit(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()

	for i := range 10 {
		got, err := l.Allow(ctx, "user-1")
		require.NoError(t, err)
		assert.True(t, got, "request %d should be allowed", i+1)
	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()

	for range 10 {
		l.Allow(ctx, "user-2")
	}

	got, err := l.Allow(ctx, "user-2")
	require.NoError(t, err)
	assert.False(t, got, "11th request should be rate-limited")
}

func TestAllow_DifferentUsers_IndependentCounters(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()

	for range 10 {
		l.Allow(ctx, "user-a")
	}

	got, err := l.Allow(ctx, "user-b")
	require.NoError(t, err)
	assert.True(t, got, "different user should not be affected")
}

func TestAllow_ResetsAfterWindow(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()

	for range 10 {
		l.Allow(ctx, "user-reset")
	}
	denied, _ := l.Allow(ctx, "user-reset")
	assert.False(t, denied)

	mr.FastForward(3601 * 1e9) // advance past 1-hour window

	got, err := l.Allow(ctx, "user-reset")
	require.NoError(t, err)
	assert.True(t, got, "counter should reset after window expires")
}

// TestAllow_HighLoad sends 200 concurrent requests for the same user and
// verifies exactly 10 are allowed (the window limit).
func TestAllow_HighLoad(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()

	const total = 200
	var allowed atomic.Int64
	var wg sync.WaitGroup

	for range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := l.Allow(ctx, "load-user")
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(10), allowed.Load(), "only 10 requests should be allowed under load")
}

// BenchmarkAllow measures throughput of the rate limiter.
func BenchmarkAllow(b *testing.B) {
	mr := miniredis.RunT(b)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := ratelimit.New(client)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			l.Allow(ctx, "bench-user")
			i++
		}
	})
}
