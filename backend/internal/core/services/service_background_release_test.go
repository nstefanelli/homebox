package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackgroundServiceGetLatestGithubRelease(t *testing.T) {
	t.Parallel()

	var gotUserAgent string
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"tag_name":"v0.27.0-fork.1","published_at":"2026-07-18T12:34:56Z"}`))
		if err != nil {
			t.Errorf("write release response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	svc := &BackgroundService{
		releaseURL: server.URL,
		httpClient: &http.Client{Timeout: time.Second},
	}

	require.NoError(t, svc.GetLatestGithubRelease(context.Background()))
	assert.Equal(t, "Homebox-Fork-Version-Checker", gotUserAgent)
	assert.Equal(t, "application/vnd.github+json", gotAccept)
	assert.Equal(t, Latest{
		Version: "v0.27.0-fork.1",
		Date:    "2026-07-18 12:34:56 +0000 UTC",
	}, svc.GetLatestVersion())
}

func TestBackgroundServiceGetLatestGithubReleaseNoPublishedRelease(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	svc := &BackgroundService{
		latest:     Latest{Version: "stale"},
		releaseURL: server.URL,
		httpClient: &http.Client{Timeout: time.Second},
	}

	require.NoError(t, svc.GetLatestGithubRelease(context.Background()))
	assert.Equal(t, Latest{}, svc.GetLatestVersion())
}

func TestBackgroundServiceGetLatestGithubReleaseHonorsHTTPTimeout(t *testing.T) {
	t.Parallel()

	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(requestCanceled)
	}))
	t.Cleanup(server.Close)

	svc := &BackgroundService{
		releaseURL: server.URL,
		httpClient: &http.Client{Timeout: 25 * time.Millisecond},
	}

	started := time.Now()
	err := svc.GetLatestGithubRelease(context.Background())
	elapsed := time.Since(started)

	require.Error(t, err)
	require.ErrorContains(t, err, "failed to make latest version request")
	assert.Less(t, elapsed, time.Second)
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("request context was not canceled after the HTTP timeout")
	}
}

func TestBackgroundServiceLatestVersionConcurrentAccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"tag_name":"v0.27.0-fork.1","published_at":"2026-07-18T12:34:56Z"}`))
		if err != nil {
			t.Errorf("write release response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	svc := &BackgroundService{
		releaseURL: server.URL,
		httpClient: &http.Client{Timeout: time.Second},
	}

	var readers sync.WaitGroup
	for range 20 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 50 {
				_ = svc.GetLatestVersion()
			}
		}()
	}

	for range 10 {
		require.NoError(t, svc.GetLatestGithubRelease(context.Background()))
	}
	readers.Wait()
}
