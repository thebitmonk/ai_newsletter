package nsqx_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsqio/go-nsq"
	"github.com/redis/go-redis/v9"

	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func setup(t *testing.T) (producer *nsqx.Producer, lookupd string, idem *nsqx.Idempotency) {
	t.Helper()
	// Project-local NSQ runs on +1000 ports to avoid colliding with any
	// system-wide install; see docker-compose.yml for rationale.
	nsqdAddr := env("TEST_NSQD_TCP_ADDR", "localhost:5150")
	lookupd = env("TEST_NSQLOOKUPD_HTTP_ADDR", "localhost:5161")
	redisAddr := env("TEST_REDIS_ADDR", "localhost:6380")

	prod, err := nsqx.NewProducer(nsqdAddr)
	if err != nil {
		t.Skipf("SKIP: cannot create nsq producer at %s: %v", nsqdAddr, err)
	}
	if err := prod.Publish("__healthcheck__", []byte("ping")); err != nil {
		t.Skipf("SKIP: cannot publish to nsqd at %s: %v", nsqdAddr, err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("SKIP: cannot reach redis at %s: %v", redisAddr, err)
	}
	return prod, lookupd, nsqx.NewIdempotency(rdb, "test")
}

func TestProducerConsumer_Roundtrip(t *testing.T) {
	prod, lookupd, _ := setup(t)
	t.Cleanup(prod.Stop)

	topic := fmt.Sprintf("test.roundtrip.%d", time.Now().UnixNano())
	channel := "roundtrip"

	var received atomic.Int32
	done := make(chan struct{}, 1)
	consumer, err := nsqx.Subscribe(lookupd, topic, channel, func(m *nsq.Message) error {
		received.Add(1)
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	}, nsqx.ConsumerOpts{MaxInFlight: 1, LookupdPollInterval: time.Second})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(consumer.Stop)

	// Create the topic by publishing first so lookupd discovers it; then the
	// next lookupd poll lets the consumer connect to nsqd.
	if err := prod.Publish(topic, []byte("hello")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("did not receive message within timeout (received=%d)", received.Load())
	}
	if received.Load() != 1 {
		t.Fatalf("expected 1 received, got %d", received.Load())
	}
}

func TestIdempotency_FirstThenDuplicate(t *testing.T) {
	prod, _, idem := setup(t)
	t.Cleanup(prod.Stop)

	ctx := context.Background()
	key := fmt.Sprintf("idem-%d", time.Now().UnixNano())

	already, err := idem.AlreadyProcessed(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if already {
		t.Fatal("first call should return false")
	}
	already, err = idem.AlreadyProcessed(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !already {
		t.Fatal("second call should return true")
	}
}

func TestProducerConsumer_DedupViaIdempotency(t *testing.T) {
	prod, lookupd, idem := setup(t)
	t.Cleanup(prod.Stop)

	topic := fmt.Sprintf("test.dedup.%d", time.Now().UnixNano())
	channel := "dedup"
	idemKey := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	var processed atomic.Int32
	ctx := context.Background()
	consumer, err := nsqx.Subscribe(lookupd, topic, channel, func(m *nsq.Message) error {
		already, err := idem.AlreadyProcessed(ctx, idemKey, time.Minute)
		if err != nil {
			return err
		}
		if already {
			return nil // drop dup silently
		}
		processed.Add(1)
		return nil
	}, nsqx.ConsumerOpts{MaxInFlight: 1, LookupdPollInterval: time.Second})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(consumer.Stop)

	for range 3 {
		if err := prod.Publish(topic, []byte("duplicate-body")); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	// Wait long enough for lookupd poll + 3 deliveries.
	time.Sleep(4 * time.Second)
	if processed.Load() != 1 {
		t.Fatalf("expected 1 processed (rest dedup'd), got %d", processed.Load())
	}
}
