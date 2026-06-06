package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestLoadMeta_Valid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "facet.yaml"), `min_version: "0.1.0"`)

	loader := NewLoader()
	meta, err := loader.LoadMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", meta.MinVersion)
}

func TestLoadMeta_Missing(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader()
	_, err := loader.LoadMeta(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "facet.yaml")
}

func TestLoadConfig_Base(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base.yaml"), `
vars:
  git_name: Sarah
packages:
  - name: git
    install: brew install git
configs:
  ~/.gitconfig: configs/.gitconfig
`)

	loader := NewLoader()
	cfg, err := loader.LoadConfig(filepath.Join(dir, "base.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "Sarah", cfg.Vars["git_name"])
	assert.Len(t, cfg.Packages, 1)
	assert.Equal(t, "git", cfg.Packages[0].Name)
	assert.Equal(t, "configs/.gitconfig", cfg.Configs["~/.gitconfig"])
}

func TestLoadConfig_WithAIPiExtensions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "base.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`ai:
  pi:
    extensions:
      - source: pi-interactive-shell
      - source: "@gotgenes/pi-session-tools"
`), 0o644))

	cfg, err := NewLoader().LoadConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg.AI)
	require.NotNil(t, cfg.AI.Pi)
	assert.Equal(t, []PiExtensionEntry{{Source: "pi-interactive-shell"}, {Source: "@gotgenes/pi-session-tools"}}, cfg.AI.Pi.Extensions)
}

func TestLoadConfig_Profile_ExtendsBase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profiles", "work.yaml"), `
extends: base
vars:
  git_email: sarah@acme.com
`)

	loader := NewLoader()
	cfg, err := loader.LoadConfig(filepath.Join(dir, "profiles", "work.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "base", cfg.Extends)
	assert.Equal(t, "sarah@acme.com", cfg.Vars["git_email"])
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.yaml"), `{{{invalid yaml`)

	loader := NewLoader()
	_, err := loader.LoadConfig(filepath.Join(dir, "bad.yaml"))
	assert.Error(t, err)
}

func TestLoadConfig_NestedVars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base.yaml"), `
vars:
  git:
    name: Sarah
    email: sarah@hey.com
  aws:
    region: us-east-1
`)

	loader := NewLoader()
	cfg, err := loader.LoadConfig(filepath.Join(dir, "base.yaml"))
	require.NoError(t, err)

	gitVars, ok := cfg.Vars["git"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Sarah", gitVars["name"])
	assert.Equal(t, "sarah@hey.com", gitVars["email"])
}

func TestValidateProfile_ValidGitExtends(t *testing.T) {
	cfg := &FacetConfig{Extends: "git@github.com:me/personal-dotfiles.git@main"}
	err := ValidateProfile(cfg)
	assert.NoError(t, err)
}

func TestValidateProfile_ValidLocalFileExtends(t *testing.T) {
	cfg := &FacetConfig{Extends: "shared/base.yaml"}
	err := ValidateProfile(cfg)
	assert.NoError(t, err)
}

func TestValidateProfile_InvalidExtends(t *testing.T) {
	cfg := &FacetConfig{Extends: "@not-a-locator"}
	err := ValidateProfile(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "extends")
}

func TestValidateProfile_InvalidBareTokenExtends(t *testing.T) {
	cfg := &FacetConfig{Extends: "something_else"}
	err := ValidateProfile(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "extends")
}

func TestValidateProfile_MissingExtends(t *testing.T) {
	cfg := &FacetConfig{}
	err := ValidateProfile(cfg)
	assert.Error(t, err)
}

func TestValidateProfile_AIEmptyAgentsWithAgentScopedConfig(t *testing.T) {
	cfg := &FacetConfig{
		Extends: "base",
		AI: &AIConfig{
			Agents: []string{},
			Permissions: map[string]*PermissionsConfig{
				"claude-code": {Allow: []string{"Read"}},
			},
		},
	}
	err := ValidateMergedConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agents")
}

func TestValidateProfile_AIPiWithoutAgents(t *testing.T) {
	cfg := &FacetConfig{
		Extends: "base",
		AI:      &AIConfig{Pi: &PiConfig{Extensions: []PiExtensionEntry{{Source: "pi-lens"}}}},
	}
	err := ValidateMergedConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateMergedConfig_AIPiExtensionRequiresSource(t *testing.T) {
	cfg := &FacetConfig{
		Extends: "base",
		AI:      &AIConfig{Pi: &PiConfig{Extensions: []PiExtensionEntry{{InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"}}}}},
	}
	err := ValidateMergedConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ai.pi.extensions[0].source")
}

func TestValidateProfile_AIPermissionsUnknownAgent(t *testing.T) {
	cfg := &FacetConfig{
		Extends: "base",
		AI: &AIConfig{
			Agents: []string{"claude"},
			Permissions: map[string]*PermissionsConfig{
				"unknown-agent": {Allow: []string{"Read"}},
			},
		},
	}
	err := ValidateMergedConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown-agent")
}

func TestValidateProfile_AINoSection(t *testing.T) {
	cfg := &FacetConfig{Extends: "base"}
	err := ValidateProfile(cfg)
	assert.NoError(t, err)
}
