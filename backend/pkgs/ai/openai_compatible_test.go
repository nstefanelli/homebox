package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

const goodReply = `{"name":"DeWalt 20V Drill","description":"Yellow cordless drill.","manufacturer":"DeWalt","model_number":"DCD771","category_hints":["power tool"],"confidence":0.9}`

func oaiServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *openaiCompatibleProvider) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := newOpenAICompatibleProvider(config.AIConf{
		Provider: "openai_compatible", BaseURL: srv.URL, APIKey: "test-key",
		Model: "test-model", TimeoutSeconds: 5,
	})
	return srv, p
}

func oaiChatResponse(content string) []byte {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{jsonFieldContent: content}}},
	})
	return b
}

func TestOpenAICompatible_Success(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	_, p := oaiServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = w.Write(oaiChatResponse(goodReply))
	})

	res, err := p.Analyze(context.Background(), []byte{0x89, 0x50}, "image/png")
	require.NoError(t, err)
	assert.Equal(t, "DeWalt 20V Drill", res.Name)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "/chat/completions", gotPath)
	assert.Equal(t, "test-model", gotBody["model"])
	assert.Equal(t, map[string]any{"type": "json_object"}, gotBody["response_format"])
}

func TestOpenAICompatible_NoAuthHeaderWhenNoKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write(oaiChatResponse(goodReply))
	}))
	t.Cleanup(srv.Close)
	p := newOpenAICompatibleProvider(config.AIConf{BaseURL: srv.URL, Model: "m", TimeoutSeconds: 5})

	_, err := p.Analyze(context.Background(), []byte{1}, "image/jpeg")
	require.NoError(t, err)
	assert.Empty(t, gotAuth)
}

func TestOpenAICompatible_RepairRetryRecovers(t *testing.T) {
	calls := 0
	_, p := oaiServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write(oaiChatResponse("Sure! It looks like a drill."))
			return
		}
		_, _ = w.Write(oaiChatResponse(goodReply))
	})

	res, err := p.Analyze(context.Background(), []byte{1}, "image/jpeg")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Equal(t, "DeWalt 20V Drill", res.Name)
}

func TestOpenAICompatible_RepairRetryExhaustedErrors(t *testing.T) {
	calls := 0
	_, p := oaiServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write(oaiChatResponse("still not json"))
	})

	_, err := p.Analyze(context.Background(), []byte{1}, "image/jpeg")
	require.Error(t, err)
	assert.Equal(t, 2, calls)
}

func TestOpenAICompatible_HTTPErrorStatus(t *testing.T) {
	_, p := oaiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := p.Analyze(context.Background(), []byte{1}, "image/jpeg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

const goodBulkReply = `[{"name":"Camping Stove","description":"Green stove.","manufacturer":"Coleman","model_number":"","quantity":1,"category_hints":["camping"],"confidence":0.9},{"name":"Rope","description":"Coiled rope.","manufacturer":"","model_number":"","quantity":2,"category_hints":[],"confidence":0.7}]`

func TestOpenAICompatible_AnalyzeContents_Success(t *testing.T) {
	var gotBody map[string]any
	_, p := oaiServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = w.Write(oaiChatResponse(goodBulkReply))
	})

	res, err := p.AnalyzeContents(context.Background(), []byte{1}, "image/jpeg")
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.Equal(t, "Camping Stove", res[0].Name)
	assert.InDelta(t, 2.0, res[1].Quantity, 0.001)

	// bulk prompt, not the single-item prompt
	msgs := gotBody["messages"].([]any)
	sys := msgs[0].(map[string]any)
	assert.Contains(t, sys[jsonFieldContent], "OPEN CONTAINER")
}

func TestOpenAICompatible_AnalyzeContents_RepairRetryRecovers(t *testing.T) {
	calls := 0
	_, p := oaiServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write(oaiChatResponse("I can see a stove and some rope!"))
			return
		}
		_, _ = w.Write(oaiChatResponse(goodBulkReply))
	})

	res, err := p.AnalyzeContents(context.Background(), []byte{1}, "image/jpeg")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Len(t, res, 2)
}
