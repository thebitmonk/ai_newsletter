package nsqx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Idempotency provides at-most-once message handling on top of NSQ's
// at-least-once delivery. The first call for a key within the TTL window
// returns (false, nil) — handler should process. Subsequent calls return
// (true, nil) — handler should drop the message.
type Idempotency struct {
	rdb       *redis.Client
	keyPrefix string
}

// NewIdempotency wraps a Redis client. keyPrefix is prepended to every key so
// the same Redis instance can serve multiple services without collision.
func NewIdempotency(rdb *redis.Client, keyPrefix string) *Idempotency {
	return &Idempotency{rdb: rdb, keyPrefix: keyPrefix}
}

// AlreadyProcessed checks whether key has been claimed within ttl.
// Implemented via SET NX EX — atomic. Returns (false, nil) on first claim,
// (true, nil) on subsequent calls within the window.
func (i *Idempotency) AlreadyProcessed(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	full := i.keyPrefix + ":" + key
	ok, err := i.rdb.SetNX(ctx, full, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency setnx: %w", err)
	}
	return !ok, nil
}
