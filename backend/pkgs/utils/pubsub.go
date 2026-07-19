package utils

import (
	"context"
	"net/url"
	"strings"

	"gocloud.dev/pubsub"
)

// ShutdownPubSubTopic releases a topic unless it came from Go CDK's mem://
// URL opener. mempubsub caches one portable Topic per name, so shutting down a
// short-lived publisher's handle also poisons every later open of that topic.
// Memory topics live only for the process lifetime and need no explicit close.
func ShutdownPubSubTopic(ctx context.Context, conn string, topic *pubsub.Topic) error {
	parsed, err := url.Parse(conn)
	if err == nil && strings.EqualFold(parsed.Scheme, "mem") {
		return nil
	}

	return topic.Shutdown(ctx)
}
