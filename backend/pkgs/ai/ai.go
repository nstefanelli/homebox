// Package ai provides pluggable vision-LLM providers that identify a
// physical item from a single photo, for the add-by-photo flow.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

var ErrAIDisabled = errors.New("ai provider not configured")

// AnalyzeResult is the provider-agnostic shape every adapter must produce.
type AnalyzeResult struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Manufacturer  string   `json:"manufacturer"`
	ModelNumber   string   `json:"model_number"`
	CategoryHints []string `json:"category_hints"`
	Confidence    float64  `json:"confidence"`
}

// Provider is implemented once per backend wire shape.
type Provider interface {
	Analyze(ctx context.Context, imageBytes []byte, mimeType string) (AnalyzeResult, error)
}

func NewProvider(conf config.AIConf) (Provider, error) {
	switch conf.Provider {
	case "openai_compatible":
		return newOpenAICompatibleProvider(conf), nil
	case "anthropic":
		return newAnthropicProvider(conf), nil
	case "":
		return nil, ErrAIDisabled
	default:
		return nil, fmt.Errorf("unknown AI provider: %s", conf.Provider)
	}
}

const systemPrompt = `You are an inventory cataloging assistant. You will be shown a photo
of a single physical item. Identify it as precisely as possible from visible
text, logos, and form factor. Do not guess a manufacturer or model number if
it is not visible or you are not reasonably confident - leave the field empty
rather than fabricate a plausible-looking value.

Respond with ONLY a JSON object matching this shape, no other text:
{
  "name": string,            // short human title, e.g. "DeWalt 20V Cordless Drill"
  "description": string,     // 1-2 sentences, visible details only
  "manufacturer": string,    // "" if not visible/certain
  "model_number": string,    // "" if not visible/certain
  "category_hints": string[],// 0-3 short category guesses, e.g. ["power tool", "cordless drill"]
  "confidence": number       // 0.0-1.0, your confidence in the identification overall
}`

const userPrompt = `Identify this item.`

// parseAnalyzeResult decodes a model reply into AnalyzeResult, tolerating
// markdown code fences and defaulting an absent confidence to 0.5.
func parseAnalyzeResult(raw string) (AnalyzeResult, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var probe struct {
		AnalyzeResult
		Confidence *float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(cleaned), &probe); err != nil {
		return AnalyzeResult{}, fmt.Errorf("model reply is not valid JSON: %w", err)
	}
	if probe.Name == "" {
		return AnalyzeResult{}, errors.New("model reply missing required field: name")
	}

	res := probe.AnalyzeResult
	if probe.Confidence != nil {
		res.Confidence = *probe.Confidence
	} else {
		res.Confidence = 0.5
	}
	if res.CategoryHints == nil {
		res.CategoryHints = []string{}
	}
	return res, nil
}
