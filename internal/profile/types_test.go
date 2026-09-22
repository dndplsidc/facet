package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPackageEntry_UnmarshalYAML_StringInstall(t *testing.T) {
	input := `
name: ripgrep
install: brew install ripgrep
`
	var pkg PackageEntry
	err := yaml.Unmarshal([]byte(input), &pkg)
	require.NoError(t, err)
	assert.Equal(t, "ripgrep", pkg.Name)
	assert.Equal(t, "brew install ripgrep", pkg.Install.Command)
	assert.Nil(t, pkg.Install.PerOS)
}

func TestPackageEntry_UnmarshalYAML_MapInstall(t *testing.T) {
	input := `
name: lazydocker
install:
  macos: brew install lazydocker
  linux: go install github.com/jesseduffield/lazydocker@latest
`
	var pkg PackageEntry
	err := yaml.Unmarshal([]byte(input), &pkg)
	require.NoError(t, err)
	assert.Equal(t, "lazydocker", pkg.Name)
	assert.Empty(t, pkg.Install.Command)
	assert.Equal(t, "brew install lazydocker", pkg.Install.PerOS["macos"])
	assert.Equal(t, "go install github.com/jesseduffield/lazydocker@latest", pkg.Install.PerOS["linux"])
}

func TestInstallCmd_ForOS(t *testing.T) {
	// Simple command — works on any OS
	simple := InstallCmd{Command: "brew install ripgrep"}
	cmd, ok := simple.ForOS("macos")
	assert.True(t, ok)
	assert.Equal(t, "brew install ripgrep", cmd)

	cmd, ok = simple.ForOS("linux")
	assert.True(t, ok)
	assert.Equal(t, "brew install ripgrep", cmd)

	// Per-OS command — only works on specified OS
	perOS := InstallCmd{PerOS: map[string]string{
		"macos": "brew install lazydocker",
	}}
	cmd, ok = perOS.ForOS("macos")
	assert.True(t, ok)
	assert.Equal(t, "brew install lazydocker", cmd)

	cmd, ok = perOS.ForOS("linux")
	assert.False(t, ok)
	assert.Empty(t, cmd)
}

func TestPackageEntry_UnmarshalYAML_StringCheck(t *testing.T) {
	input := `
name: ripgrep
check: which rg
install: brew install ripgrep
`
	var pkg PackageEntry
	err := yaml.Unmarshal([]byte(input), &pkg)
	require.NoError(t, err)
	assert.Equal(t, "ripgrep", pkg.Name)
	assert.Equal(t, "which rg", pkg.Check.Command)
	assert.Equal(t, "brew install ripgrep", pkg.Install.Command)
}

func TestPackageEntry_UnmarshalYAML_MapCheck(t *testing.T) {
	input := `
name: lazydocker
check:
  macos: which lazydocker
  linux: which lazydocker
install:
  macos: brew install lazydocker
  linux: apt install lazydocker
`
	var pkg PackageEntry
	err := yaml.Unmarshal([]byte(input), &pkg)
	require.NoError(t, err)
	assert.Equal(t, "lazydocker", pkg.Name)
	assert.Equal(t, "which lazydocker", pkg.Check.PerOS["macos"])
	assert.Equal(t, "which lazydocker", pkg.Check.PerOS["linux"])
	assert.Equal(t, "brew install lazydocker", pkg.Install.PerOS["macos"])
}

func TestPackageEntry_UnmarshalYAML_NoCheck(t *testing.T) {
	input := `
name: ripgrep
install: brew install ripgrep
`
	var pkg PackageEntry
	err := yaml.Unmarshal([]byte(input), &pkg)
	require.NoError(t, err)
	assert.Empty(t, pkg.Check.Command)
	assert.Nil(t, pkg.Check.PerOS)
}

func TestFacetConfig_UnmarshalYAML_Full(t *testing.T) {
	input := `
extends: base
vars:
  git:
    name: Sarah
    email: sarah@acme.com
  simple: hello
packages:
  - name: ripgrep
    install: brew install ripgrep
  - name: lazydocker
    install:
      macos: brew install lazydocker
      linux: go install github.com/jesseduffield/lazydocker@latest
configs:
  ~/.gitconfig: configs/.gitconfig
  ~/.zshrc: configs/.zshrc
`
	var cfg FacetConfig
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.NoError(t, err)
	assert.Equal(t, "base", cfg.Extends)
	assert.Equal(t, "hello", cfg.Vars["simple"])

	gitVars, ok := cfg.Vars["git"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Sarah", gitVars["name"])

	assert.Len(t, cfg.Packages, 2)
	assert.Equal(t, "ripgrep", cfg.Packages[0].Name)
	assert.Equal(t, "configs/.gitconfig", cfg.Configs["~/.gitconfig"].Value)
}

func TestFacetConfig_UnmarshalYAML_Scripts(t *testing.T) {
	input := `
pre_apply:
  - name: configure git
    run: git config --global user.email "test@example.com"
  - name: run setup
    run: |
      export FOO=bar
      ./scripts/setup.sh
post_apply:
  - name: cleanup
    run: echo done
`
	var cfg FacetConfig
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.NoError(t, err)
	require.Len(t, cfg.PreApply, 2)
	assert.Equal(t, "configure git", cfg.PreApply[0].Name)
	assert.Equal(t, `git config --global user.email "test@example.com"`, cfg.PreApply[0].Run.Value)
	assert.Equal(t, "run setup", cfg.PreApply[1].Name)
	assert.Contains(t, cfg.PreApply[1].Run.Value, "export FOO=bar")
	assert.Contains(t, cfg.PreApply[1].Run.Value, "./scripts/setup.sh")
	require.Len(t, cfg.PostApply, 1)
	assert.Equal(t, "cleanup", cfg.PostApply[0].Name)
	assert.Equal(t, "echo done", cfg.PostApply[0].Run.Value)
}

func TestFacetMeta_UnmarshalYAML(t *testing.T) {
	input := `min_version: "0.1.0"`
	var meta FacetMeta
	err := yaml.Unmarshal([]byte(input), &meta)
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", meta.MinVersion)
}
