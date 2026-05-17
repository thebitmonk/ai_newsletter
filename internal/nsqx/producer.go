package nsqx

import (
	"fmt"
	"time"

	"github.com/nsqio/go-nsq"
)

// Producer is a thin wrapper around *nsq.Producer that exposes the operations
// the rest of the app needs. One Producer should be shared across the process.
type Producer struct {
	inner *nsq.Producer
}

// NewProducer constructs a Producer connected (lazily) to nsqd at addr.
// addr is "host:tcp_port", e.g. "localhost:4150".
func NewProducer(addr string) (*Producer, error) {
	cfg := nsq.NewConfig()
	cfg.WriteTimeout = 5 * time.Second
	p, err := nsq.NewProducer(addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("new producer at %s: %w", addr, err)
	}
	// Silence the default chatty stderr logger. go-nsq's logger interface is
	// unexported but only requires Output(int, string) error.
	p.SetLogger(noopLogger{}, nsq.LogLevelError)
	return &Producer{inner: p}, nil
}

// Publish sends body on topic synchronously. Returns when nsqd has acked.
func (p *Producer) Publish(topic string, body []byte) error {
	return p.inner.Publish(topic, body)
}

// PublishDeferred sends body on topic to be delivered after delay.
func (p *Producer) PublishDeferred(topic string, delay time.Duration, body []byte) error {
	return p.inner.DeferredPublish(topic, delay, body)
}

// Stop closes the connection. Safe to call multiple times.
func (p *Producer) Stop() {
	p.inner.Stop()
}

type noopLogger struct{}

func (noopLogger) Output(_ int, _ string) error { return nil }
