package ai

import (
	"fmt"
	"strings"
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

func TestParseAnalyzeResults_ValidArray(t *testing.T) {
	raw := `[
	  {"name":"Camping Stove","description":"Green two-burner stove.","manufacturer":"Coleman","model_number":"","quantity":1,"category_hints":["camping"],"confidence":0.9},
	  {"name":"Propane Canister","description":"Small green canisters.","manufacturer":"","model_number":"","quantity":4,"category_hints":["camping","fuel"],"confidence":0.8}
	]`

	res, err := parseAnalyzeResults(raw)
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.Equal(t, "Camping Stove", res[0].Name)
	assert.InDelta(t, 4.0, res[1].Quantity, 0.001)
}

func TestParseAnalyzeResults_StripsFencesAndDefaults(t *testing.T) {
	raw := "```json\n[{\"name\":\"Rope\",\"description\":\"\",\"manufacturer\":\"\",\"model_number\":\"\",\"category_hints\":[]}]\n```"

	res, err := parseAnalyzeResults(raw)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.InDelta(t, 1.0, res[0].Quantity, 0.001)   // absent quantity -> 1
	assert.InDelta(t, 0.5, res[0].Confidence, 0.001) // absent confidence -> 0.5
}

func TestParseAnalyzeResults_DropsNamelessElements(t *testing.T) {
	raw := `[{"name":"Tarp"},{"description":"mystery blob with no name"}]`

	res, err := parseAnalyzeResults(raw)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "Tarp", res[0].Name)
}

func TestParseAnalyzeResults_AllNamelessErrors(t *testing.T) {
	_, err := parseAnalyzeResults(`[{"description":"no name"}]`)
	require.Error(t, err)
}

func TestParseAnalyzeResults_EmptyArrayOK(t *testing.T) {
	res, err := parseAnalyzeResults(`[]`)
	require.NoError(t, err)
	assert.Empty(t, res)
}

func TestParseAnalyzeResults_OverCapErrors(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 51; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"name":"item %d"}`, i)
	}
	sb.WriteString("]")

	_, err := parseAnalyzeResults(sb.String())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "implausible")
}

func TestParseAnalyzeResults_NotAnArrayErrors(t *testing.T) {
	_, err := parseAnalyzeResults(`{"name":"single object not array"}`)
	require.Error(t, err)
}
