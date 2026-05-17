package nsqx

import (
	"log"

	"github.com/nsqio/go-nsq"
)

// CanaryHandler is a HandlerFunc that logs every received message. Subscribe
// it to CanaryTopic at startup to smoke-test end-to-end wiring.
func CanaryHandler(msg *nsq.Message) error {
	log.Printf("nsqx canary: topic=%s id=%s body=%q", CanaryTopic, string(msg.ID[:]), string(msg.Body))
	return nil
}
