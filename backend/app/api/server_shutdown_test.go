package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShutdownHTTPServerCompletesGracefully(t *testing.T) {
	t.Parallel()

	server, _, serveDone := startShutdownTestServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	require.NoError(t, shutdownHTTPServer(server, time.Second))
	require.ErrorIs(t, <-serveDone, http.ErrServerClosed)
}

func TestShutdownHTTPServerForcesCloseAfterDeadline(t *testing.T) {
	t.Parallel()

	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})
	server, listener, serveDone := startShutdownTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		<-r.Context().Done()
		close(handlerDone)
	}))

	requestDone := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String()) //nolint:gosec // loopback-only test server
		if response != nil {
			_ = response.Body.Close()
		}
		requestDone <- err
	}()

	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach the test handler")
	}

	started := time.Now()
	err := shutdownHTTPServer(server, 25*time.Millisecond)
	elapsed := time.Since(started)

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, time.Second)
	require.ErrorIs(t, <-serveDone, http.ErrServerClosed)

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("forced close did not cancel the active handler")
	}
	require.Error(t, <-requestDone)
}

func startShutdownTestServer(t *testing.T, handler http.Handler) (*http.Server, net.Listener, <-chan error) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: time.Second,
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	return server, listener, serveDone
}
