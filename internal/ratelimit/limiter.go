package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	limit  = 10
	window = time.Hour
)

type Limiter struct {
	client *redis.Client
}

func New(client *redis.Client) *Limiter {
	return &Limiter{client: client}
}

func (l *Limiter) Allow(ctx context.Context, userID string) (bool, error) {
	key := "ratelimit:" + userID
	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := l.client.Expire(ctx, key, window).Err(); err != nil {
			return false, err
		}
	}
	return count <= limit, nil

}
