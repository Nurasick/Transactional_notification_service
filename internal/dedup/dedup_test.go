package dedup_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Nurasick/transactional-notification-service/internal/dedup"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDedup(t *testing.T) (*dedup.Deduplicator, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return dedup.New(client), mr
}

func TestIsDuplicate_FirstCall_NotDuplicate(t *testing.T) {
	d, _ := newDedup(t)
	got, err := d.IsDuplicate(context.Background(), "msg-1")
	require.NoError(t, err)
	assert.False(t, got)
}

func TestIsDuplicate_SecondCall_IsDuplicate(t *testing.T) {
	d, _ := newDedup(t)
	ctx := context.Background()

	_, _ = d.IsDuplicate(ctx, "msg-1")
	got, err := d.IsDuplicate(ctx, "msg-1")
	require.NoError(t, err)
	assert.True(t, got)
}

func TestIsDuplicate_DifferentIDs_NotDuplicate(t *testing.T) {
	d, _ := newDedup(t)
	ctx := context.Background()

	_, _ = d.IsDuplicate(ctx, "msg-a")
	got, err := d.IsDuplicate(ctx, "msg-b")
	require.NoError(t, err)
	assert.False(t, got)
}

func TestIsDuplicate_TTLExpiry_NotDuplicateAfterExpiry(t *testing.T) {
	d, mr := newDedup(t)
	ctx := context.Background()

	_, _ = d.IsDuplicate(ctx, "msg-ttl")

	// Fast-forward miniredis clock past the 24h TTL
	mr.FastForward(25 * 3600 * 1e9) // nanoseconds

	got, err := d.IsDuplicate(ctx, "msg-ttl")
	require.NoError(t, err)
	assert.False(t, got, "key should have expired")
}

// TestIsDuplicate_Concurrent verifies no race condition under parallel calls
// for the same message ID.
func TestIsDuplicate_Concurrent(t *testing.T) {
	d, _ := newDedup(t)
	ctx := context.Background()

	const goroutines = 50
	results := make([]bool, goroutines)
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dup, _ := d.IsDuplicate(ctx, "concurrent-msg")
			results[i] = dup
		}(i)
	}
	wg.Wait()

	trueCount := 0
	for _, r := range results {
		if r {
			trueCount++
		}
	}
	// Exactly one goroutine should win the SetNX race; the rest see a duplicate.
	assert.Equal(t, goroutines-1, trueCount, "all but first call should be duplicates")
}
