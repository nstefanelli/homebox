package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/pkgs/utils"
	"gocloud.dev/pubsub"
)

func TestRunJobSubscriptionStopsCleanlyOnCancellation(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() {
		log.Logger = previousLogger
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cfg := &config.Config{}
	cfg.Database.PubSubConnString = "mem://{{ .Topic }}"
	const topicName = "run-job-subscription-shutdown-test"

	handled := make(chan struct{}, 1)
	done := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		done <- runJobSubscription(ctx, cfg, topicName, func(context.Context, *pubsub.Message) {
			select {
			case handled <- struct{}{}:
			default:
			}
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Error("subscription cleanup did not complete")
		}
	})

	conn, err := utils.GenerateSubPubConn(cfg.Database.PubSubConnString, topicName)
	require.NoError(t, err)
	topic, err := pubsub.OpenTopic(context.Background(), conn)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		sendCtx, sendCancel := context.WithTimeout(context.Background(), time.Second)
		defer sendCancel()
		require.NoError(t, topic.Send(sendCtx, &pubsub.Message{Body: []byte("probe")}))

		select {
		case <-handled:
			return true
		default:
			return false
		}
	}, 3*time.Second, 20*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("subscription did not stop after cancellation")
	}

	require.NotContains(t, logs.String(), "failed to receive message from pubsub topic")
	require.NotContains(t, logs.String(), "failed to shutdown pubsub subscription")
	require.NotContains(t, logs.String(), "failed to shutdown pubsub topic")
	require.NotContains(t, logs.String(), "context canceled")
}
