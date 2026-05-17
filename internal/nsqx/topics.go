package nsqx

import "fmt"

// Topic constructs an NSQ topic string from a domain and verb. Callers must
// use this rather than raw strings — grep for raw "source.poll" etc. should
// only ever hit this file.
func Topic(domain, verb string) string {
	return fmt.Sprintf("%s.%s", domain, verb)
}

// Channel returns a per-consumer-class channel name. NSQ delivers each message
// to one consumer per channel, so each worker class wanting to consume the
// same topic uses a distinct channel.
func Channel(consumerClass string) string {
	return consumerClass
}

// CanaryTopic is the smoke-test topic used by the canary consumer to verify
// end-to-end wiring at startup.
var CanaryTopic = Topic("canary", "echo")
