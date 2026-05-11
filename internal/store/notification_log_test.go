package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Nurasick/transactional-notification-service/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test — requires a running Postgres.
// Set POSTGRES_DSN env var or the test is skipped.
func openTestDB(t *testing.T) *store.NotificationLog {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set — skipping integration test")
	}
	db, err := store.New(dsn)
	require.NoError(t, err)
	return db
}

func TestInsert_And_List(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id := "test-" + time.Now().Format("20060102150405.000")
	rec := store.NotificationRecord{
		MessageID: id,
		UserID:    "u-integ",
		EventType: "order.placed",
		Status:    "sent",
		SentAt:    time.Now().UTC(),
	}

	require.NoError(t, db.Insert(ctx, rec))

	records, err := db.List(ctx, store.ListFilter{UserID: "u-integ", Limit: 10})
	require.NoError(t, err)

	found := false
	for _, r := range records {
		if r.MessageID == id {
			found = true
			assert.Equal(t, "sent", r.Status)
		}
	}
	assert.True(t, found, "inserted record should appear in List results")
}

func TestInsert_ConflictOnDuplicate_NoError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id := "dup-" + time.Now().Format("20060102150405.000")
	rec := store.NotificationRecord{
		MessageID: id,
		UserID:    "u-dup",
		EventType: "order.placed",
		Status:    "sent",
		SentAt:    time.Now().UTC(),
	}

	require.NoError(t, db.Insert(ctx, rec))
	// Second insert with the same message_id must not return an error (ON CONFLICT DO NOTHING).
	require.NoError(t, db.Insert(ctx, rec), "duplicate insert must be silently ignored")
}

func TestList_FilterByStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	records, err := db.List(ctx, store.ListFilter{Status: "sent", Limit: 5})
	require.NoError(t, err)
	for _, r := range records {
		assert.Equal(t, "sent", r.Status)
	}
}

func TestList_FilterByEventType(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	records, err := db.List(ctx, store.ListFilter{EventType: "order.placed", Limit: 5})
	require.NoError(t, err)
	for _, r := range records {
		assert.Equal(t, "order.placed", r.EventType)
	}
}

func TestList_DefaultLimit(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	records, err := db.List(ctx, store.ListFilter{}) // limit=0 → defaults to 50
	require.NoError(t, err)
	assert.LessOrEqual(t, len(records), 50)
}
