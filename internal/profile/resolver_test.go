package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_SimpleVar(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"name": "Sarah"},
		Packages: []PackageEntry{
			{Name: "greet", Install: InstallCmd{Command: "echo ${facet:name}"}},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "echo Sarah", resolved.Packages[0].Install.Command)
}

func TestResolve_NestedVar(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{
				"email": "sarah@acme.com",
			},
		},
		Configs: map[string]string{
			"~/.gitconfig": "configs/${facet:git.email}/gitconfig",
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "configs/sarah@acme.com/gitconfig", resolved.Configs["~/.gitconfig"])
}

func TestResolve_DeeplyNestedVar(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{
			"cloud": map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
			},
		},
		Packages: []PackageEntry{
			{Name: "aws", Install: InstallCmd{Command: "echo ${facet:cloud.aws.region}"}},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "echo us-east-1", resolved.Packages[0].Install.Command)
}

func TestResolve_UndefinedVar_Error(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"name": "Sarah"},
		Packages: []PackageEntry{
			{Name: "test", Install: InstallCmd{Command: "echo ${facet:undefined_var}"}},
		},
	}

	_, err := Resolve(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined_var")
}

func TestResolve_MultipleVarsInOneString(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{
			"first": "Sarah",
			"last":  "Chen",
		},
		Packages: []PackageEntry{
			{Name: "test", Install: InstallCmd{Command: "echo ${facet:first} ${facet:last}"}},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "echo Sarah Chen", resolved.Packages[0].Install.Command)
}

func TestResolve_NoRecursion(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{
			"a": "${facet:b}",
			"b": "actual_value",
		},
		Packages: []PackageEntry{
			{Name: "test", Install: InstallCmd{Command: "${facet:a}"}},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	// a's value is literally "${facet:b}", not "actual_value"
	assert.Equal(t, "${facet:b}", resolved.Packages[0].Install.Command)
}

func TestResolve_CheckCommand(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"tool": "rg"},
		Packages: []PackageEntry{
			{
				Name:    "ripgrep",
				Check:   InstallCmd{Command: "which ${facet:tool}"},
				Install: InstallCmd{Command: "brew install ripgrep"},
			},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "which rg", resolved.Packages[0].Check.Command)
	assert.Equal(t, "brew install ripgrep", resolved.Packages[0].Install.Command)
}

func TestResolve_CheckCommandPerOS(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"version": "22"},
		Packages: []PackageEntry{
			{
				Name: "node",
				Check: InstallCmd{
					PerOS: map[string]string{
						"macos": "node --version | grep ${facet:version}",
						"linux": "node --version | grep ${facet:version}",
					},
				},
				Install: InstallCmd{
					PerOS: map[string]string{
						"macos": "brew install node@${facet:version}",
						"linux": "apt install nodejs=${facet:version}",
					},
				},
			},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "node --version | grep 22", resolved.Packages[0].Check.PerOS["macos"])
	assert.Equal(t, "node --version | grep 22", resolved.Packages[0].Check.PerOS["linux"])
}

func TestResolve_NoCheckCommand(t *testing.T) {
	cfg := &FacetConfig{
		Packages: []PackageEntry{
			{Name: "git", Install: InstallCmd{Command: "brew install git"}},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Empty(t, resolved.Packages[0].Check.Command)
	assert.Nil(t, resolved.Packages[0].Check.PerOS)
}

func TestResolve_PerOSInstallCommand(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"version": "22"},
		Packages: []PackageEntry{
			{
				Name: "node",
				Install: InstallCmd{
					PerOS: map[string]string{
						"macos": "brew install node@${facet:version}",
						"linux": "apt install nodejs=${facet:version}",
					},
				},
			},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "brew install node@22", resolved.Packages[0].Install.PerOS["macos"])
	assert.Equal(t, "apt install nodejs=22", resolved.Packages[0].Install.PerOS["linux"])
}

func TestResolve_ConfigSourcePaths(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"env": "work"},
		Configs: map[string]string{
			"~/.gitconfig": "configs/${facet:env}/.gitconfig",
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "configs/work/.gitconfig", resolved.Configs["~/.gitconfig"])
}

func TestResolve_ConfigTargetPathsNotResolved(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"dir": "custom"},
		Configs: map[string]string{
			"~/${facet:dir}/.gitconfig": "configs/.gitconfig",
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	// Target path should NOT be resolved — ${facet:...} stays literal in keys
	_, exists := resolved.Configs["~/${facet:dir}/.gitconfig"]
	assert.True(t, exists)
}

func TestResolve_NoVarsNoError(t *testing.T) {
	cfg := &FacetConfig{
		Packages: []PackageEntry{
			{Name: "git", Install: InstallCmd{Command: "brew install git"}},
		},
		Configs: map[string]string{
			"~/.zshrc": "configs/.zshrc",
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "brew install git", resolved.Packages[0].Install.Command)
}

func TestResolve_AI_MCPEnv(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{
			"github": map[string]any{
				"token": "ghp_abc123",
			},
		},
		AI: &AIConfig{
			MCPs: []MCPEntry{
				{
					Name:    "github-mcp",
					Command: "gh-mcp",
					Env:     map[string]string{"GITHUB_TOKEN": "${facet:github.token}"},
				},
			},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	require.NotNil(t, resolved.AI)
	assert.Equal(t, "ghp_abc123", resolved.AI.MCPs[0].Env["GITHUB_TOKEN"])
}

func TestResolve_AI_MCPArgs(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"mcp_version": "v2.1.0"},
		AI: &AIConfig{
			MCPs: []MCPEntry{
				{
					Name:    "my-mcp",
					Command: "run-mcp-${facet:mcp_version}",
					Args:    []string{"--version", "${facet:mcp_version}"},
				},
			},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	require.NotNil(t, resolved.AI)
	assert.Equal(t, "run-mcp-v2.1.0", resolved.AI.MCPs[0].Command)
	assert.Equal(t, []string{"--version", "v2.1.0"}, resolved.AI.MCPs[0].Args)
}

func TestResolve_AI_UndefinedVar(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{},
		AI: &AIConfig{
			MCPs: []MCPEntry{
				{
					Name:    "secret-mcp",
					Command: "mcp-server",
					Env:     map[string]string{"TOKEN": "${facet:secret.token}"},
				},
			},
		},
	}

	_, err := Resolve(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined")
}

func TestResolve_AI_Nil(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"name": "test"},
		Packages: []PackageEntry{
			{Name: "git", Install: InstallCmd{Command: "brew install git"}},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Nil(t, resolved.AI)
}

func TestResolve_AI_PermissionsDeepCopied(t *testing.T) {
	cfg := &FacetConfig{
		AI: &AIConfig{
			Agents: []string{"claude-code"},
			Permissions: map[string]*PermissionsConfig{
				"claude-code": {
					Allow: []string{"Read"},
					Deny:  []string{"Bash"},
				},
			},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	require.NotNil(t, resolved.AI)
	require.NotNil(t, resolved.AI.Permissions["claude-code"])

	resolved.AI.Permissions["claude-code"].Allow[0] = "Write"
	resolved.AI.Permissions["claude-code"].Deny[0] = "Shell"

	assert.Equal(t, []string{"Read"}, cfg.AI.Permissions["claude-code"].Allow)
	assert.Equal(t, []string{"Bash"}, cfg.AI.Permissions["claude-code"].Deny)
}

func TestResolveAIPiExtensions_SubstitutesVariables(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"pi_ext": "pi-lens", "registry": "https://registry.example.com"},
		AI: &AIConfig{Pi: &PiConfig{Extensions: []PiExtensionEntry{
			{Source: "${facet:pi_ext}", InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "${facet:registry}"}},
			{Source: "pi-subagents"},
		}}},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	require.NotNil(t, resolved.AI)
	require.NotNil(t, resolved.AI.Pi)
	assert.Equal(t, []PiExtensionEntry{
		{Source: "pi-lens", InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"}},
		{Source: "pi-subagents"},
	}, resolved.AI.Pi.Extensions)
}

func TestResolveAIPiExtensions_UndefinedVariableErrors(t *testing.T) {
	cfg := &FacetConfig{AI: &AIConfig{Pi: &PiConfig{Extensions: []PiExtensionEntry{{Source: "${facet:missing}"}}}}}

	_, err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ai.pi.extensions[0].source")
}

func TestResolveAIPiExtensionInstallEnv_UndefinedVariableErrors(t *testing.T) {
	cfg := &FacetConfig{AI: &AIConfig{Pi: &PiConfig{Extensions: []PiExtensionEntry{{
		Source:     "pi-lens",
		InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "${facet:missing}"},
	}}}}}

	_, err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ai.pi.extensions[0].install_env.NPM_CONFIG_REGISTRY")
}

func TestResolve_PreApplyScriptVars(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{
				"email": "sarah@acme.com",
			},
		},
		PreApply: []ScriptEntry{
			{Name: "configure git", Run: `git config --global user.email "${facet:git.email}"`},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	require.Len(t, resolved.PreApply, 1)
	assert.Equal(t, "configure git", resolved.PreApply[0].Name)
	assert.Equal(t, `git config --global user.email "sarah@acme.com"`, resolved.PreApply[0].Run)
}

func TestResolve_PostApplyScriptVars(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"name": "Sarah"},
		PostApply: []ScriptEntry{
			{Name: "greet", Run: "echo ${facet:name}"},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	require.Len(t, resolved.PostApply, 1)
	assert.Equal(t, "echo Sarah", resolved.PostApply[0].Run)
}

func TestResolve_ScriptNameNotResolved(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"tool": "git"},
		PreApply: []ScriptEntry{
			{Name: "setup ${facet:tool}", Run: "echo hello"},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, "setup ${facet:tool}", resolved.PreApply[0].Name)
}

func TestResolve_ScriptUndefinedVar(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{},
		PostApply: []ScriptEntry{
			{Name: "broken", Run: "echo ${facet:missing}"},
		},
	}

	_, err := Resolve(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestResolve_ScriptDeepCopy(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"name": "Sarah"},
		PreApply: []ScriptEntry{
			{Name: "test", Run: "echo ${facet:name}"},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)

	resolved.PreApply[0].Run = "mutated"
	assert.Equal(t, "echo ${facet:name}", cfg.PreApply[0].Run)
}

func TestResolve_ConfigMetaPreserved(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"env": "work"},
		Configs: map[string]string{
			"~/.gitconfig": "configs/${facet:env}/.gitconfig",
		},
		ConfigMeta: map[string]ConfigProvenance{
			"~/.gitconfig": {SourceRoot: "/remote", Materialize: true},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	assert.Equal(t, ConfigProvenance{SourceRoot: "/remote", Materialize: true}, resolved.ConfigMeta["~/.gitconfig"])

	resolved.ConfigMeta["~/.gitconfig"] = ConfigProvenance{SourceRoot: "/mutated", Materialize: false}
	assert.Equal(t, ConfigProvenance{SourceRoot: "/remote", Materialize: true}, cfg.ConfigMeta["~/.gitconfig"])
}

func TestResolve_ScriptsPreserveWorkDir(t *testing.T) {
	cfg := &FacetConfig{
		Vars: map[string]any{"name": "Sarah"},
		PreApply: []ScriptEntry{
			{Name: "pre", Run: "echo ${facet:name}", WorkDir: "/remote"},
		},
		PostApply: []ScriptEntry{
			{Name: "post", Run: "printf '%s' ${facet:name}", WorkDir: "/local"},
		},
	}

	resolved, err := Resolve(cfg)
	require.NoError(t, err)
	require.Len(t, resolved.PreApply, 1)
	require.Len(t, resolved.PostApply, 1)
	assert.Equal(t, "echo Sarah", resolved.PreApply[0].Run)
	assert.Equal(t, "/remote", resolved.PreApply[0].WorkDir)
	assert.Equal(t, "/local", resolved.PostApply[0].WorkDir)
}
