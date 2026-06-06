package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge_VarsDeepMerge(t *testing.T) {
	base := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{
				"name":   "Sarah",
				"editor": "nvim",
			},
			"simple": "hello",
		},
	}
	profile := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{
				"email": "sarah@acme.com",
			},
			"new_var": "world",
		},
	}

	result, err := Merge(base, profile)
	require.NoError(t, err)

	gitVars, ok := result.Vars["git"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Sarah", gitVars["name"])
	assert.Equal(t, "nvim", gitVars["editor"])
	assert.Equal(t, "sarah@acme.com", gitVars["email"])
	assert.Equal(t, "hello", result.Vars["simple"])
	assert.Equal(t, "world", result.Vars["new_var"])
}

func TestMerge_VarsTypeConflict(t *testing.T) {
	base := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{"name": "Sarah"},
		},
	}
	profile := &FacetConfig{
		Vars: map[string]any{
			"git": "just a string",
		},
	}

	_, err := Merge(base, profile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type conflict")
	assert.Contains(t, err.Error(), "git")
}

func TestMerge_VarsLastWriterWins(t *testing.T) {
	base := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{
				"name": "Sarah",
			},
		},
	}
	profile := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{
				"name": "Sarah Chen",
			},
		},
	}

	result, err := Merge(base, profile)
	require.NoError(t, err)
	gitVars := result.Vars["git"].(map[string]any)
	assert.Equal(t, "Sarah Chen", gitVars["name"])
}

func TestMerge_PackagesUnion(t *testing.T) {
	base := &FacetConfig{
		Packages: []PackageEntry{
			{Name: "git", Install: InstallCmd{Command: "brew install git"}},
			{Name: "ripgrep", Install: InstallCmd{Command: "brew install ripgrep"}},
		},
	}
	profile := &FacetConfig{
		Packages: []PackageEntry{
			{Name: "docker", Install: InstallCmd{Command: "brew install docker"}},
		},
	}

	result, err := Merge(base, profile)
	require.NoError(t, err)
	assert.Len(t, result.Packages, 3)

	names := make([]string, len(result.Packages))
	for i, p := range result.Packages {
		names[i] = p.Name
	}
	assert.Contains(t, names, "git")
	assert.Contains(t, names, "ripgrep")
	assert.Contains(t, names, "docker")
}

func TestMerge_PackagesOverride(t *testing.T) {
	base := &FacetConfig{
		Packages: []PackageEntry{
			{Name: "git", Install: InstallCmd{Command: "brew install git"}},
		},
	}
	profile := &FacetConfig{
		Packages: []PackageEntry{
			{Name: "git", Install: InstallCmd{Command: "sudo apt-get install -y git"}},
		},
	}

	result, err := Merge(base, profile)
	require.NoError(t, err)
	assert.Len(t, result.Packages, 1)
	assert.Equal(t, "sudo apt-get install -y git", result.Packages[0].Install.Command)
}

func TestMerge_ConfigsShallowMerge(t *testing.T) {
	base := &FacetConfig{
		Configs: map[string]string{
			"~/.gitconfig": "configs/.gitconfig",
			"~/.zshrc":     "configs/.zshrc",
		},
	}
	profile := &FacetConfig{
		Configs: map[string]string{
			"~/.gitconfig": "configs/work/.gitconfig",
			"~/.npmrc":     "configs/work/.npmrc",
		},
	}

	result, err := Merge(base, profile)
	require.NoError(t, err)
	assert.Equal(t, "configs/work/.gitconfig", result.Configs["~/.gitconfig"])
	assert.Equal(t, "configs/.zshrc", result.Configs["~/.zshrc"])
	assert.Equal(t, "configs/work/.npmrc", result.Configs["~/.npmrc"])
}

func TestMerge_ThreeLayers(t *testing.T) {
	base := &FacetConfig{
		Vars:     map[string]any{"name": "Sarah"},
		Packages: []PackageEntry{{Name: "git", Install: InstallCmd{Command: "brew install git"}}},
		Configs:  map[string]string{"~/.gitconfig": "configs/.gitconfig"},
	}
	profile := &FacetConfig{
		Vars:     map[string]any{"email": "sarah@acme.com"},
		Packages: []PackageEntry{{Name: "docker", Install: InstallCmd{Command: "brew install docker"}}},
		Configs:  map[string]string{"~/.npmrc": "configs/.npmrc"},
	}
	local := &FacetConfig{
		Vars: map[string]any{"secret": "s3cret"},
	}

	// Merge base + profile first
	merged, err := Merge(base, profile)
	require.NoError(t, err)

	// Then merge with local
	result, err := Merge(merged, local)
	require.NoError(t, err)

	assert.Equal(t, "Sarah", result.Vars["name"])
	assert.Equal(t, "sarah@acme.com", result.Vars["email"])
	assert.Equal(t, "s3cret", result.Vars["secret"])
	assert.Len(t, result.Packages, 2)
	assert.Len(t, result.Configs, 2)
}

func TestMerge_NilInputs(t *testing.T) {
	base := &FacetConfig{}
	profile := &FacetConfig{
		Vars: map[string]any{"key": "value"},
	}

	result, err := Merge(base, profile)
	require.NoError(t, err)
	assert.Equal(t, "value", result.Vars["key"])
}

func TestMerge_PreApplyConcatenation(t *testing.T) {
	base := &FacetConfig{
		PreApply: []ScriptEntry{
			{Name: "base-script-1", Run: "echo base1"},
			{Name: "base-script-2", Run: "echo base2"},
		},
	}
	overlay := &FacetConfig{
		PreApply: []ScriptEntry{
			{Name: "overlay-script-1", Run: "echo overlay1"},
		},
	}

	result, err := Merge(base, overlay)
	require.NoError(t, err)
	require.Len(t, result.PreApply, 3)
	assert.Equal(t, "base-script-1", result.PreApply[0].Name)
	assert.Equal(t, "echo base1", result.PreApply[0].Run)
	assert.Equal(t, "base-script-2", result.PreApply[1].Name)
	assert.Equal(t, "echo base2", result.PreApply[1].Run)
	assert.Equal(t, "overlay-script-1", result.PreApply[2].Name)
	assert.Equal(t, "echo overlay1", result.PreApply[2].Run)
}

func TestMerge_PostApplyConcatenation(t *testing.T) {
	base := &FacetConfig{
		PostApply: []ScriptEntry{
			{Name: "base-script", Run: "echo base"},
		},
	}
	overlay := &FacetConfig{
		PostApply: []ScriptEntry{
			{Name: "overlay-script", Run: "echo overlay"},
		},
	}

	result, err := Merge(base, overlay)
	require.NoError(t, err)
	require.Len(t, result.PostApply, 2)
	assert.Equal(t, "base-script", result.PostApply[0].Name)
	assert.Equal(t, "overlay-script", result.PostApply[1].Name)
}

func TestMerge_PreApplyBaseOnly(t *testing.T) {
	base := &FacetConfig{
		PreApply: []ScriptEntry{
			{Name: "base-only", Run: "echo base"},
		},
	}
	overlay := &FacetConfig{}

	result, err := Merge(base, overlay)
	require.NoError(t, err)
	require.Len(t, result.PreApply, 1)
	assert.Equal(t, "base-only", result.PreApply[0].Name)
}

func TestMerge_PostApplyOverlayOnly(t *testing.T) {
	base := &FacetConfig{}
	overlay := &FacetConfig{
		PostApply: []ScriptEntry{
			{Name: "overlay-only", Run: "echo overlay"},
		},
	}

	result, err := Merge(base, overlay)
	require.NoError(t, err)
	require.Len(t, result.PostApply, 1)
	assert.Equal(t, "overlay-only", result.PostApply[0].Name)
}

func TestMerge_ScriptsBothEmpty(t *testing.T) {
	base := &FacetConfig{}
	overlay := &FacetConfig{}

	result, err := Merge(base, overlay)
	require.NoError(t, err)
	assert.Nil(t, result.PreApply)
	assert.Nil(t, result.PostApply)
}

func TestMerge_ScriptsDeepCopy(t *testing.T) {
	base := &FacetConfig{
		PreApply: []ScriptEntry{
			{Name: "base-script", Run: "echo base"},
		},
	}
	overlay := &FacetConfig{
		PreApply: []ScriptEntry{
			{Name: "overlay-script", Run: "echo overlay"},
		},
	}

	result, err := Merge(base, overlay)
	require.NoError(t, err)

	result.PreApply[0].Run = "mutated"
	assert.Equal(t, "echo base", base.PreApply[0].Run)
	assert.Equal(t, "echo overlay", overlay.PreApply[0].Run)
}

func TestMergeAIPiExtensions_UnionsBySourceAndOverlayReplacesEntry(t *testing.T) {
	base := &FacetConfig{AI: &AIConfig{Pi: &PiConfig{Extensions: []PiExtensionEntry{
		{Source: "pi-lens"},
		{Source: "pi-subagents", InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"}},
	}}}}
	overlay := &FacetConfig{AI: &AIConfig{Pi: &PiConfig{Extensions: []PiExtensionEntry{
		{Source: "pi-subagents"},
		{Source: "@gotgenes/pi-session-tools"},
	}}}}

	merged, err := Merge(base, overlay)
	require.NoError(t, err)
	require.NotNil(t, merged.AI)
	require.NotNil(t, merged.AI.Pi)
	assert.Equal(t, []PiExtensionEntry{
		{Source: "pi-lens"},
		{Source: "pi-subagents"},
		{Source: "@gotgenes/pi-session-tools"},
	}, merged.AI.Pi.Extensions)
}

func TestMergeAIPi_NilWhenAbsent(t *testing.T) {
	merged, err := Merge(&FacetConfig{}, &FacetConfig{})
	require.NoError(t, err)
	assert.Nil(t, merged.AI)
}

func TestAnnotateLayer_SetsConfigMetaAndScriptWorkDirs(t *testing.T) {
	cfg := &FacetConfig{
		Configs: map[string]string{
			"~/.gitconfig": "configs/.gitconfig",
		},
		PreApply:  []ScriptEntry{{Name: "pre", Run: "echo pre"}},
		PostApply: []ScriptEntry{{Name: "post", Run: "echo post"}},
	}

	AnnotateLayer(cfg, "/tmp/source-root", true)

	assert.Equal(t, ConfigProvenance{
		SourceRoot:  "/tmp/source-root",
		Materialize: true,
	}, cfg.ConfigMeta["~/.gitconfig"])
	assert.Equal(t, "/tmp/source-root", cfg.PreApply[0].WorkDir)
	assert.Equal(t, "/tmp/source-root", cfg.PostApply[0].WorkDir)
}

func TestMerge_ConfigMetaOverlayWins(t *testing.T) {
	base := &FacetConfig{
		Configs: map[string]string{
			"~/.gitconfig": "configs/base/.gitconfig",
			"~/.zshrc":     "configs/.zshrc",
		},
		ConfigMeta: map[string]ConfigProvenance{
			"~/.gitconfig": {SourceRoot: "/base", Materialize: true},
			"~/.zshrc":     {SourceRoot: "/base", Materialize: true},
		},
	}
	overlay := &FacetConfig{
		Configs: map[string]string{
			"~/.gitconfig": "configs/overlay/.gitconfig",
		},
		ConfigMeta: map[string]ConfigProvenance{
			"~/.gitconfig": {SourceRoot: "/overlay", Materialize: false},
		},
	}

	result, err := Merge(base, overlay)
	require.NoError(t, err)
	assert.Equal(t, ConfigProvenance{SourceRoot: "/overlay", Materialize: false}, result.ConfigMeta["~/.gitconfig"])
	assert.Equal(t, ConfigProvenance{SourceRoot: "/base", Materialize: true}, result.ConfigMeta["~/.zshrc"])
}

func TestMerge_DeepCopiesNestedVars(t *testing.T) {
	base := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{
				"name": "Sarah",
			},
		},
	}

	result, err := Merge(base, &FacetConfig{})
	require.NoError(t, err)

	resultGit := result.Vars["git"].(map[string]any)
	resultGit["name"] = "Changed"

	baseGit := base.Vars["git"].(map[string]any)
	assert.Equal(t, "Sarah", baseGit["name"])
}

func TestMerge_ScriptsPreserveWorkDir(t *testing.T) {
	base := &FacetConfig{
		PreApply: []ScriptEntry{{Name: "base", Run: "echo base", WorkDir: "/base"}},
	}
	overlay := &FacetConfig{
		PreApply: []ScriptEntry{{Name: "overlay", Run: "echo overlay", WorkDir: "/overlay"}},
	}

	result, err := Merge(base, overlay)
	require.NoError(t, err)
	require.Len(t, result.PreApply, 2)
	assert.Equal(t, "/base", result.PreApply[0].WorkDir)
	assert.Equal(t, "/overlay", result.PreApply[1].WorkDir)
}
