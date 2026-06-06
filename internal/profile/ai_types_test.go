package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAIConfig_UnmarshalYAML_Full(t *testing.T) {
	input := `
agents:
  - claude-code
  - cursor
permissions:
  claude-code:
    allow:
      - Read
      - Edit
      - Bash
    deny: []
  cursor:
    allow:
      - "Read(**)"
      - "Write(**)"
    deny:
      - "Shell(*)"
skills:
  - source: github.com/org/skills
    skills:
      - commit
      - review-pr
    agents:
      - claude-code
  - source: github.com/org/other-skills
    skills:
      - deploy
mcps:
  - name: playwright
    command: npx
    args:
      - "@playwright/mcp"
    env:
      DISPLAY: ":0"
    agents:
      - claude-code
  - name: filesystem
    command: npx
    args:
      - "@modelcontextprotocol/server-filesystem"
      - /tmp
pi:
  extensions:
    - source: pi-lens
      install_env:
        NPM_CONFIG_REGISTRY: https://registry.example.com
`
	var cfg AIConfig
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.NoError(t, err)

	assert.Equal(t, []string{"claude-code", "cursor"}, cfg.Agents)

	require.Len(t, cfg.Permissions, 2)
	claude := cfg.Permissions["claude-code"]
	require.NotNil(t, claude)
	assert.Equal(t, []string{"Read", "Edit", "Bash"}, claude.Allow)
	assert.Equal(t, []string{}, claude.Deny)

	cursor := cfg.Permissions["cursor"]
	require.NotNil(t, cursor)
	assert.Equal(t, []string{"Read(**)", "Write(**)"}, cursor.Allow)
	assert.Equal(t, []string{"Shell(*)"}, cursor.Deny)

	require.Len(t, cfg.Skills, 2)
	assert.Equal(t, "github.com/org/skills", cfg.Skills[0].Source)
	assert.Equal(t, []string{"commit", "review-pr"}, cfg.Skills[0].Skills)
	assert.Equal(t, []string{"claude-code"}, cfg.Skills[0].Agents)

	require.Len(t, cfg.MCPs, 2)
	assert.Equal(t, "playwright", cfg.MCPs[0].Name)
	require.NotNil(t, cfg.Pi)
	assert.Equal(t, []PiExtensionEntry{{
		Source:     "pi-lens",
		InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"},
	}}, cfg.Pi.Extensions)
}

func TestAIConfig_UnmarshalYAML_PiExtensionStringIsInvalid(t *testing.T) {
	input := `
pi:
  extensions:
    - pi-lens
`
	var cfg AIConfig
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unmarshal")
}

func TestAIConfig_UnmarshalYAML_Empty(t *testing.T) {
	input := `
agents:
  - claude-code
`
	var cfg AIConfig
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.NoError(t, err)

	assert.Equal(t, []string{"claude-code"}, cfg.Agents)
	assert.Nil(t, cfg.Permissions)
	assert.Nil(t, cfg.Skills)
	assert.Nil(t, cfg.MCPs)
}

func TestFacetConfig_WithAI(t *testing.T) {
	input := `
extends: base
vars:
  editor: nvim
packages:
  - name: ripgrep
    install: brew install ripgrep
configs:
  ~/.zshrc: configs/.zshrc
ai:
  agents:
    - claude-code
  permissions:
    claude-code:
      allow:
        - Bash
      deny: []
  mcps:
    - name: filesystem
      command: npx
      args:
        - "@modelcontextprotocol/server-filesystem"
`
	var cfg FacetConfig
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.NoError(t, err)

	assert.Equal(t, "base", cfg.Extends)
	require.NotNil(t, cfg.AI)
	assert.Equal(t, []string{"claude-code"}, cfg.AI.Agents)
	require.NotNil(t, cfg.AI.Permissions["claude-code"])
	assert.Equal(t, []string{"Bash"}, cfg.AI.Permissions["claude-code"].Allow)
	assert.Equal(t, []string{}, cfg.AI.Permissions["claude-code"].Deny)
}

func TestFacetConfig_WithoutAI(t *testing.T) {
	input := `
extends: base
vars:
  editor: nvim
packages:
  - name: ripgrep
    install: brew install ripgrep
configs:
  ~/.zshrc: configs/.zshrc
`
	var cfg FacetConfig
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.NoError(t, err)

	assert.Equal(t, "base", cfg.Extends)
	assert.Nil(t, cfg.AI)
}
