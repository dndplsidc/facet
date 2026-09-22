package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOSValueYAML(t *testing.T) {
	for _, input := range []string{`shared`, `{macos: apple, linux: penguin}`, `{linux: penguin}`} {
		var value OSValue
		require.NoError(t, yaml.Unmarshal([]byte(input), &value))
		encoded, err := yaml.Marshal(value)
		require.NoError(t, err)
		var roundtrip OSValue
		require.NoError(t, yaml.Unmarshal(encoded, &roundtrip))
		assert.Equal(t, value, roundtrip)
	}
	for _, input := range []string{`''`, `42`, `true`, `[]`, `{}`, `{ubuntu: source}`, `{linux: ''}`, `{linux: true}`, `{linux: 42}`, `{linux: null}`, `{linux: [source]}`, `{linux: one, linux: two}`} {
		t.Run(input, func(t *testing.T) {
			var cfg FacetConfig
			err := yaml.Unmarshal([]byte("configs:\n  ~/.example: "+input), &cfg)
			require.Error(t, err)
		})
	}
	for _, input := range []string{"configs:\n  ~/.example: null", "pre_apply:\n  - name: missing", "post_apply:\n  - name: missing\n    run: null"} {
		var cfg FacetConfig
		require.NoError(t, yaml.Unmarshal([]byte(input), &cfg))
		require.Error(t, ValidateMergedConfig(&cfg))
	}
}

func TestPlatformMergeAndProvenance(t *testing.T) {
	var base, overlay FacetConfig
	require.NoError(t, yaml.Unmarshal([]byte(`configs:
  ~/.shared: shared
  ~/.override:
    macos: original-macos
    linux: original-linux
pre_apply:
  - name: same
    run:
      macos: echo remote-macos
      linux: echo remote-linux
`), &base))
	require.NoError(t, yaml.Unmarshal([]byte(`configs:
  ~/.override:
    macos: override-macos
pre_apply:
  - name: same
    run: echo local
`), &overlay))
	AnnotateLayer(&base, "/remote", true)
	AnnotateLayer(&overlay, "/local", false)
	merged, err := Merge(&base, &overlay)
	require.NoError(t, err)
	selected, skipped := SelectOS(merged, "linux")
	assert.Equal(t, []PlatformSkip{{Stage: "configs", Name: "~/.override"}}, skipped)
	assert.Equal(t, ConfigProvenance{SourceRoot: "/remote", Materialize: true}, selected.ConfigMeta["~/.shared"])
	assert.NotContains(t, selected.ConfigMeta, "~/.override")
	require.Len(t, selected.PreApply, 2)
	assert.Equal(t, "echo remote-linux", selected.PreApply[0].Run.Value)
	assert.Equal(t, "/remote", selected.PreApply[0].WorkDir)
	assert.Equal(t, "/local", selected.PreApply[1].WorkDir)
	macos, _ := SelectOS(merged, "macos")
	assert.Equal(t, "override-macos", macos.Configs["~/.override"].Value)
	assert.Equal(t, ConfigProvenance{SourceRoot: "/local"}, macos.ConfigMeta["~/.override"])
	// A scalar overlay replaces the entire platform map, too.
	scalar, err := Merge(merged, &FacetConfig{Configs: map[string]OSValue{"~/.override": {Value: "shared-again"}}})
	require.NoError(t, err)
	linux, _ := SelectOS(scalar, "linux")
	assert.Equal(t, "shared-again", linux.Configs["~/.override"].Value)
	// Mutating merged or selected values cannot change either input layer.
	merged.Configs["~/.override"].PerOS["macos"] = "changed"
	merged.PreApply[0].Run.PerOS["linux"] = "changed"
	selected.PreApply[0].Run.Value = "changed"
	assert.Equal(t, "override-macos", overlay.Configs["~/.override"].PerOS["macos"])
	assert.Equal(t, "echo remote-linux", base.PreApply[0].Run.PerOS["linux"])
}

func TestPlatformSelectBeforeResolve(t *testing.T) {
	var cfg FacetConfig
	require.NoError(t, yaml.Unmarshal([]byte(`vars:
  source: linux-file
configs:
  ~/.example:
    linux: ${facet:source}
    macos: ${facet:undefined}
pre_apply:
  - name: setup
    run:
      macos: ${facet:undefined}
packages:
  - name: available
    install:
      linux: echo ${facet:source}
      macos: ${facet:undefined}
    check:
      macos: ${facet:undefined}
  - name: unavailable
    check: ${facet:undefined}
    install:
      macos: ${facet:undefined}
`), &cfg))
	selected, _ := SelectOS(&cfg, "linux")
	resolved, err := Resolve(selected)
	require.NoError(t, err)
	assert.Equal(t, "linux-file", resolved.Configs["~/.example"].Value)
	command, ok := resolved.Packages[0].Install.ForOS("linux")
	assert.True(t, ok)
	assert.Equal(t, "echo linux-file", command)
	_, ok = resolved.Packages[1].Install.ForOS("linux")
	assert.False(t, ok)
	assert.Empty(t, resolved.PreApply)
	// Selected branches still validate variables.
	selected, _ = SelectOS(&cfg, "macos")
	_, err = Resolve(selected)
	require.ErrorContains(t, err, "undefined variable")
	resolved.Vars["source"] = "changed"
	assert.Equal(t, "linux-file", cfg.Vars["source"])
}
