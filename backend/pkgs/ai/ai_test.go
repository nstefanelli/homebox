package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

func TestParseAnalyzeResult_Valid(t *testing.T) {
	raw := `{"name":"DeWalt 20V Drill","description":"Yellow cordless drill.","manufacturer":"DeWalt","model_number":"DCD771","category_hints":["power tool","cordless drill"],"confidence":0.9}`

	res, err := parseAnalyzeResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "DeWalt 20V Drill", res.Name)
	assert.Equal(t, "DeWalt", res.Manufacturer)
	assert.Equal(t, "DCD771", res.ModelNumber)
	assert.Equal(t, []string{"power tool", "cordless drill"}, res.CategoryHints)
	assert.InDelta(t, 0.9, res.Confidence, 0.001)
}

func TestParseAnalyzeResult_StripsCodeFences(t *testing.T) {
	raw := "```json\n{\"name\":\"Mug\",\"description\":\"A mug.\",\"manufacturer\":\"\",\"model_number\":\"\",\"category_hints\":[],\"confidence\":0.7}\n```"

	res, err := parseAnalyzeResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "Mug", res.Name)
}

func TestParseAnalyzeResult_DefaultsMissingConfidence(t *testing.T) {
	raw := `{"name":"Mug","description":"A mug.","manufacturer":"","model_number":"","category_hints":[]}`

	res, err := parseAnalyzeResult(raw)
	require.NoError(t, err)
	assert.InDelta(t, 0.5, res.Confidence, 0.001)
}

func TestParseAnalyzeResult_GarbageErrors(t *testing.T) {
	_, err := parseAnalyzeResult("I think this is a drill!")
	require.Error(t, err)
}

func TestParseAnalyzeResult_MissingNameErrors(t *testing.T) {
	_, err := parseAnalyzeResult(`{"description":"something","manufacturer":"","model_number":"","category_hints":[],"confidence":0.5}`)
	require.Error(t, err)
}

func TestNewProvider_Selection(t *testing.T) {
	p, err := NewProvider(config.AIConf{Provider: "openai_compatible", BaseURL: "http://x/v1", Model: "m", TimeoutSeconds: 120})
	require.NoError(t, err)
	assert.IsType(t, &openaiCompatibleProvider{}, p)

	p2, err := NewProvider(config.AIConf{Provider: "anthropic", APIKey: "k", Model: "m", TimeoutSeconds: 120})
	require.NoError(t, err)
	assert.IsType(t, &anthropicProvider{}, p2)

	_, err = NewProvider(config.AIConf{Provider: ""})
	require.ErrorIs(t, err, ErrAIDisabled)

	_, err = NewProvider(config.AIConf{Provider: "bogus"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrAIDisabled)
}
