package utils

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/mempubsub"
)

func TestShutdownPubSubTopicKeepsMemoryTopicReusable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := "mem://shutdown-topic-reuse-" + uuid.NewString()
	topic, err := pubsub.OpenTopic(ctx, conn)
	require.NoError(t, err)
	subscription, err := pubsub.OpenSubscription(ctx, conn)
	require.NoError(t, err)
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		_ = subscription.Shutdown(shutdownCtx)
		_ = topic.Shutdown(shutdownCtx)
	})

	require.NoError(t, topic.Send(ctx, &pubsub.Message{Body: []byte("first")}))
	require.NoError(t, ShutdownPubSubTopic(ctx, conn, topic))

	reopened, err := pubsub.OpenTopic(ctx, conn)
	require.NoError(t, err)
	require.NoError(t, reopened.Send(ctx, &pubsub.Message{Body: []byte("second")}))

	bodies := make(map[string]struct{}, 2)
	for range 2 {
		message, receiveErr := subscription.Receive(ctx)
		require.NoError(t, receiveErr)
		bodies[string(message.Body)] = struct{}{}
		message.Ack()
	}
	require.Contains(t, bodies, "first")
	require.Contains(t, bodies, "second")
}

func TestShutdownPubSubTopicClosesNonMemoryTopic(t *testing.T) {
	ctx := context.Background()
	topic := mempubsub.NewTopic()

	require.NoError(t, ShutdownPubSubTopic(ctx, "kafka://broker/topic", topic))

	err := topic.Send(ctx, &pubsub.Message{Body: []byte("closed")})
	require.Error(t, err)
	require.Equal(t, gcerrors.FailedPrecondition, gcerrors.Code(err))
}
