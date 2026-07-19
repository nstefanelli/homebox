package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/pkgs/utils"
	"gocloud.dev/pubsub"
)

func TestPublishExportJobCanReuseMemoryTopic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	service := *tSvc.Exports
	service.pubSubConn = "mem://export-reuse-" + uuid.NewString() + "-{{ .Topic }}"

	conn, err := utils.GenerateSubPubConn(service.pubSubConn, TopicCollectionExport)
	require.NoError(t, err)
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

	exportIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for _, exportID := range exportIDs {
		require.NoError(t, service.publishExportJob(ctx, tGroup.ID, exportID))
	}

	received := make(map[string]struct{}, len(exportIDs))
	for range exportIDs {
		message, receiveErr := subscription.Receive(ctx)
		require.NoError(t, receiveErr)
		require.Equal(t, tGroup.ID.String(), message.Metadata["group_id"])
		received[message.Metadata["export_id"]] = struct{}{}
		message.Ack()
	}
	for _, exportID := range exportIDs {
		require.Contains(t, received, exportID.String())
	}
}
