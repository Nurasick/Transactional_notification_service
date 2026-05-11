package dedup

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const ttl = 24 * time.Hour

type Deduplicator struct {
	client *redis.Client
}

func New(client *redis.Client) *Deduplicator {
	return &Deduplicator{client: client}
}

func (d *Deduplicator) IsDuplicate(ctx context.Context, messageID string) (bool, error) {
	exists, err := d.client.SetNX(ctx, "dedup:"+messageID, "1", ttl).Result()
	if err != nil {
		return false, err
	}
	return !exists, nil
}
