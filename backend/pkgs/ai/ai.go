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
	Quantity      float64  `json:"quantity"`
	CategoryHints []string `json:"category_hints"`
	Confidence    float64  `json:"confidence"`
}

// Provider is implemented once per backend wire shape.
type Provider interface {
	Analyze(ctx context.Context, imageBytes []byte, mimeType string) (AnalyzeResult, error)
	// AnalyzeContents identifies every distinct item visible in a photo of an
	// open container/shelf. Empty slice = nothing identified (valid result).
	AnalyzeContents(ctx context.Context, imageBytes []byte, mimeType string) ([]AnalyzeResult, error)
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

const maxBulkCandidates = 50

const systemPromptBulk = `You are an inventory cataloging assistant. You will be shown a photo
of an OPEN CONTAINER (tote, bin, box) or shelf. List EVERY distinct physical
item you can identify from visible shapes, text, logos, and packaging. Group
identical items into one entry with a quantity. Do not guess a manufacturer
or model number if it is not visible or you are not reasonably confident -
leave the field empty rather than fabricate a plausible-looking value. Do
not invent items you cannot actually see.

Respond with ONLY a JSON array (no other text), each element:
{
  "name": string,            // short human title, e.g. "Coleman Camping Stove"
  "description": string,     // 1 sentence, visible details only
  "manufacturer": string,    // "" if not visible/certain
  "model_number": string,    // "" if not visible/certain
  "quantity": number,        // count of this identical item, default 1
  "category_hints": string[],// 0-3 short category guesses
  "confidence": number       // 0.0-1.0 for this identification
}`

const userPromptBulk = `List every item in this container.`

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

// parseAnalyzeResults decodes a model reply into a slice of AnalyzeResult,
// tolerating markdown code fences. Elements without a name are dropped; a
// non-empty reply yielding zero named elements is an error, as is a reply
// with an implausible number of candidates.
func parseAnalyzeResults(raw string) ([]AnalyzeResult, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	type probeResult struct {
		AnalyzeResult
		Confidence *float64 `json:"confidence"`
		Quantity   *float64 `json:"quantity"`
	}
	var probes []probeResult
	if err := json.Unmarshal([]byte(cleaned), &probes); err != nil {
		return nil, fmt.Errorf("model reply is not a valid JSON array: %w", err)
	}
	if len(probes) > maxBulkCandidates {
		return nil, fmt.Errorf("bulk analyze returned implausible candidate count: %d", len(probes))
	}

	results := make([]AnalyzeResult, 0, len(probes))
	for _, p := range probes {
		if p.Name == "" {
			continue
		}
		res := p.AnalyzeResult
		if p.Confidence != nil {
			res.Confidence = *p.Confidence
		} else {
			res.Confidence = 0.5
		}
		if p.Quantity != nil && *p.Quantity > 0 {
			res.Quantity = *p.Quantity
		} else {
			res.Quantity = 1
		}
		if res.CategoryHints == nil {
			res.CategoryHints = []string{}
		}
		results = append(results, res)
	}
	if len(results) == 0 && len(probes) > 0 {
		return nil, errors.New("bulk analyze returned no usable candidates (all missing name)")
	}
	return results, nil
}
