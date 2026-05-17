package nsqx

import (
	"fmt"
	"time"

	"github.com/nsqio/go-nsq"
)

// HandlerFunc processes a single NSQ message. Returning a non-nil error causes
// NSQ to requeue the message according to its retry policy. Returning nil acks.
type HandlerFunc func(msg *nsq.Message) error

// ConsumerOpts controls consumer behaviour at subscription time.
type ConsumerOpts struct {
	MaxInFlight         int           // default 1
	MaxAttempts         uint16        // default 5
	LookupdPollInterval time.Duration // default 5s (go-nsq's own default is 60s, far too slow)
}

// Consumer wraps a single *nsq.Consumer subscribed to one (topic, channel).
type Consumer struct {
	inner *nsq.Consumer
}

// Subscribe connects to nsqlookupd at lookupdAddr ("host:http_port"),
// subscribes to (topic, channel), and routes messages to handler. The
// consumer runs in the background; call Stop to shut it down.
func Subscribe(lookupdAddr, topic, channel string, handler HandlerFunc, opts ConsumerOpts) (*Consumer, error) {
	cfg := nsq.NewConfig()
	if opts.MaxInFlight > 0 {
		cfg.MaxInFlight = opts.MaxInFlight
	}
	if opts.MaxAttempts > 0 {
		cfg.MaxAttempts = opts.MaxAttempts
	}
	if opts.LookupdPollInterval > 0 {
		cfg.LookupdPollInterval = opts.LookupdPollInterval
	} else {
		cfg.LookupdPollInterval = 5 * time.Second
	}

	c, err := nsq.NewConsumer(topic, channel, cfg)
	if err != nil {
		return nil, fmt.Errorf("new consumer %s/%s: %w", topic, channel, err)
	}
	c.SetLogger(noopLogger{}, nsq.LogLevelError)
	c.AddHandler(nsq.HandlerFunc(func(m *nsq.Message) error { return handler(m) }))

	if err := c.ConnectToNSQLookupd(lookupdAddr); err != nil {
		return nil, fmt.Errorf("connect to nsqlookupd at %s: %w", lookupdAddr, err)
	}
	return &Consumer{inner: c}, nil
}

// Stop initiates a graceful shutdown and waits for in-flight messages.
func (c *Consumer) Stop() {
	c.inner.Stop()
	<-c.inner.StopChan
}
