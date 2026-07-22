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

func anthropicServer(t *testing.T, handler http.HandlerFunc) *anthropicProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newAnthropicProvider(config.AIConf{
		Provider: "anthropic", BaseURL: srv.URL, APIKey: "sk-ant-test",
		Model: "claude-haiku-4-5", TimeoutSeconds: 5,
	})
}

func TestAnthropic_Success(t *testing.T) {
	var gotKey, gotVersion, gotPath string
	var gotBody map[string]any
	p := anthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		resp, _ := json.Marshal(map[string]any{
			jsonFieldContent: []map[string]any{{"type": "text", "text": goodReply}},
		})
		_, _ = w.Write(resp)
	})

	res, err := p.Analyze(context.Background(), []byte{0xFF, 0xD8}, "image/jpeg")
	require.NoError(t, err)
	assert.Equal(t, "DeWalt 20V Drill", res.Name)
	assert.Equal(t, "sk-ant-test", gotKey)
	assert.Equal(t, "2023-06-01", gotVersion)
	assert.Equal(t, "/v1/messages", gotPath)
	assert.Equal(t, "claude-haiku-4-5", gotBody["model"])
	assert.NotNil(t, gotBody["output_config"], "structured-output config must be sent")

	// image block must be base64-typed with the right media type
	msgs := gotBody["messages"].([]any)
	content := msgs[0].(map[string]any)[jsonFieldContent].([]any)
	img := content[0].(map[string]any)
	assert.Equal(t, "image", img["type"])
	src := img["source"].(map[string]any)
	assert.Equal(t, "base64", src["type"])
	assert.Equal(t, "image/jpeg", src["media_type"])
}

func TestAnthropic_HTTPErrorStatus(t *testing.T) {
	p := anthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := p.Analyze(context.Background(), []byte{1}, "image/jpeg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestAnthropic_NoTextBlockErrors(t *testing.T) {
	p := anthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp, _ := json.Marshal(map[string]any{jsonFieldContent: []map[string]any{}})
		_, _ = w.Write(resp)
	})

	_, err := p.Analyze(context.Background(), []byte{1}, "image/jpeg")
	require.Error(t, err)
}

func TestAnthropic_AnalyzeContents_Success(t *testing.T) {
	var gotBody map[string]any
	p := anthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		resp, _ := json.Marshal(map[string]any{
			jsonFieldContent: []map[string]any{{"type": "text", "text": goodBulkReply}},
		})
		_, _ = w.Write(resp)
	})

	res, err := p.AnalyzeContents(context.Background(), []byte{1}, "image/jpeg")
	require.NoError(t, err)
	require.Len(t, res, 2)

	// array-typed structured output schema sent
	oc := gotBody["output_config"].(map[string]any)
	format := oc["format"].(map[string]any)
	schema := format["schema"].(map[string]any)
	assert.Equal(t, "array", schema["type"])
}

func TestAnthropic_IdentifyKeyword_Success(t *testing.T) {
	var gotBody map[string]any
	p := anthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		resp, _ := json.Marshal(map[string]any{
			jsonFieldContent: []map[string]any{{"type": "text", "text": goodReply}},
		})
		_, _ = w.Write(resp)
	})

	res, err := p.IdentifyKeyword(context.Background(), "dewalt drill")
	require.NoError(t, err)
	assert.Equal(t, "DeWalt 20V Drill", res.Name)

	// keyword system prompt with the no-fabrication instruction
	sys := gotBody["system"].(string)
	assert.Contains(t, sys, "NEVER invent a model")

	// text-only single content block — no image block on the keyword lane
	msgs := gotBody["messages"].([]any)
	content := msgs[0].(map[string]any)[jsonFieldContent].([]any)
	require.Len(t, content, 1)
	block := content[0].(map[string]any)
	assert.Equal(t, "text", block["type"])
	assert.Contains(t, block["text"], "dewalt drill")

	// object-typed structured output schema sent
	oc := gotBody["output_config"].(map[string]any)
	format := oc["format"].(map[string]any)
	schema := format["schema"].(map[string]any)
	assert.Equal(t, "object", schema["type"])
}

func TestAnthropic_IdentifyKeyword_MalformedReplyErrors(t *testing.T) {
	p := anthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp, _ := json.Marshal(map[string]any{
			jsonFieldContent: []map[string]any{{"type": "text", "text": "no json here"}},
		})
		_, _ = w.Write(resp)
	})

	_, err := p.IdentifyKeyword(context.Background(), "dewalt drill")
	require.Error(t, err)
}
