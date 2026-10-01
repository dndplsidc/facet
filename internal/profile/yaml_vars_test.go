package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSubstituteVars_YAML(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"models", map[string]any{"claude-opus-5-5-high": []any{
			map[string]any{"local_agent_id": "first", "managed_agent_id": "11785"},
			map[string]any{"local_agent_id": "second", "managed_agent_id": "00123"},
		}, "grok-4.7-high": []any{}}},
		{"empty map", map[string]any{}},
		{"empty list", []any{}},
		{"scalar types", []any{true, 5, nil, "true", "123", "null"}},
		{"special strings", []any{"line\nbreak", "quote\" # comment: value", "你好", "${facet:missing}"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]any{"wagent": map[string]any{"models": tc.value}, "label": "unchanged"}
			rendered, err := SubstituteVars("agents:\n  models: ${facet:wagent.models|yaml}\nlabel: ${facet:label}\n", vars)
			require.NoError(t, err)
			assert.Len(t, strings.Split(strings.TrimSuffix(rendered, "\n"), "\n"), 3)
			var got map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &got))
			assert.Equal(t, tc.value, got["agents"].(map[string]any)["models"])
			assert.Equal(t, "unchanged", got["label"])
		})
	}
}

func TestSubstituteVars_YAMLErrors(t *testing.T) {
	for _, tc := range []struct {
		input   string
		message string
	}{
		{"${facet:missing|yaml}", "undefined variable"},
		{"${facet:models.missing|yaml}", "undefined variable"},
		{"${facet:models|json}", "unsupported variable filter"},
		{"${facet:models}", "use a more specific path"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			_, err := SubstituteVars(tc.input, map[string]any{"models": map[string]any{}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}
