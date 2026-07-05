package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

const anthropicDefaultBaseURL = "https://api.anthropic.com"

const jsonSchemaTypeString = "string"

// anthropicProvider talks to Anthropic's /v1/messages API directly
// (image content blocks, output_config structured outputs, x-api-key auth).
type anthropicProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func newAnthropicProvider(conf config.AIConf) *anthropicProvider {
	timeout := conf.TimeoutSeconds
	if timeout <= 0 {
		timeout = 120
	}
	baseURL := strings.TrimSuffix(conf.BaseURL, "/")
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	return &anthropicProvider{
		baseURL: baseURL,
		apiKey:  conf.APIKey,
		model:   conf.Model,
		client:  &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

var analyzeResultSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"name":           map[string]any{"type": jsonSchemaTypeString},
		"description":    map[string]any{"type": jsonSchemaTypeString},
		"manufacturer":   map[string]any{"type": jsonSchemaTypeString},
		"model_number":   map[string]any{"type": jsonSchemaTypeString},
		"category_hints": map[string]any{"type": "array", "items": map[string]any{"type": jsonSchemaTypeString}},
		"confidence":     map[string]any{"type": "number"},
	},
	"required":             []string{"name", "description", "manufacturer", "model_number", "category_hints", "confidence"},
	"additionalProperties": false,
}

func (p *anthropicProvider) Analyze(ctx context.Context, imageBytes []byte, mimeType string) (AnalyzeResult, error) {
	body, err := json.Marshal(map[string]any{
		"model":      p.model,
		"max_tokens": 1024,
		"system":     systemPrompt,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "image", "source": map[string]string{
					"type": "base64", "media_type": mimeType,
					"data": base64.StdEncoding.EncodeToString(imageBytes),
				}},
				{"type": "text", "text": userPrompt},
			},
		}},
		"output_config": map[string]any{
			"format": map[string]any{"type": "json_schema", "schema": analyzeResultSchema},
		},
	})
	if err != nil {
		return AnalyzeResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return AnalyzeResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return AnalyzeResult{}, fmt.Errorf("vision provider request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return AnalyzeResult{}, fmt.Errorf("vision provider returned status code: %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AnalyzeResult{}, err
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return AnalyzeResult{}, fmt.Errorf("vision provider returned unparseable body: %w", err)
	}
	for _, block := range parsed.Content {
		if block.Type == "text" {
			// Schema is strictly enforced server-side; parse without repair-retry.
			return parseAnalyzeResult(block.Text)
		}
	}
	return AnalyzeResult{}, fmt.Errorf("vision provider returned no text content")
}
