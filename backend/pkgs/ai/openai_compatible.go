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

// openaiCompatibleProvider talks to any /chat/completions-shaped backend:
// Ollama, OpenAI, OpenRouter, LM Studio, vLLM.
type openaiCompatibleProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func newOpenAICompatibleProvider(conf config.AIConf) *openaiCompatibleProvider {
	timeout := conf.TimeoutSeconds
	if timeout <= 0 {
		timeout = 120
	}
	return &openaiCompatibleProvider{
		baseURL: strings.TrimSuffix(conf.BaseURL, "/"),
		apiKey:  conf.APIKey,
		model:   conf.Model,
		client:  &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

type oaiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func (p *openaiCompatibleProvider) Analyze(ctx context.Context, imageBytes []byte, mimeType string) (AnalyzeResult, error) {
	dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(imageBytes)

	messages := []oaiMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: []map[string]any{
			{"type": "text", "text": userPrompt},
			{"type": "image_url", "image_url": map[string]string{"url": dataURI}},
		}},
	}

	reply, err := p.complete(ctx, messages)
	if err != nil {
		return AnalyzeResult{}, err
	}

	res, parseErr := parseAnalyzeResult(reply)
	if parseErr == nil {
		return res, nil
	}

	// One repair retry: feed the bad reply and the parse error back.
	messages = append(messages,
		oaiMessage{Role: "assistant", Content: reply},
		oaiMessage{Role: "user", Content: fmt.Sprintf(
			"Your previous response was not the required JSON object (%v). Respond again with ONLY the JSON object, no other text.", parseErr)},
	)
	reply, err = p.complete(ctx, messages)
	if err != nil {
		return AnalyzeResult{}, err
	}
	return parseAnalyzeResult(reply)
}

func (p *openaiCompatibleProvider) complete(ctx context.Context, messages []oaiMessage) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":           p.model,
		"messages":        messages,
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vision provider request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("vision provider returned status code: %d: %s", resp.StatusCode, snippet)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("vision provider returned unparseable body: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("vision provider returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
