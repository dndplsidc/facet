package profile

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Loader handles loading and parsing facet configuration files.
type Loader struct{}

// NewLoader creates a new Loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadMeta reads and parses facet.yaml from the given config directory.
func (l *Loader) LoadMeta(configDir string) (*FacetMeta, error) {
	path := filepath.Join(configDir, "facet.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read facet.yaml: %w (is this a facet config directory?)", err)
	}
	var meta FacetMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse facet.yaml: %w", err)
	}
	return &meta, nil
}

// LoadConfig reads and parses a single YAML config file (base.yaml, profile, or .local.yaml).
func (l *Loader) LoadConfig(path string) (*FacetConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", filepath.Base(path), err)
	}
	var cfg FacetConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filepath.Base(path), err)
	}
	return &cfg, nil
}

// ValidateProfile checks that a profile config has a valid extends field.
func ValidateProfile(cfg *FacetConfig) error {
	if cfg.Extends == "" {
		return fmt.Errorf("profile is missing 'extends' field")
	}
	if _, err := ParseExtends(cfg.Extends); err != nil {
		return err
	}
	return nil
}

// ValidateMergedConfig checks invariants that can only be validated after all
// profile layers have been merged together.
func ValidateMergedConfig(cfg *FacetConfig) error {
	if cfg.AI == nil {
		return nil
	}
	if cfg.AI.Pi != nil {
		for i, ext := range cfg.AI.Pi.Extensions {
			if ext.Source == "" {
				return fmt.Errorf("ai.pi.extensions[%d].source must not be empty", i)
			}
		}
	}
	needsAgents := len(cfg.AI.Permissions) > 0 || len(cfg.AI.Skills) > 0 || len(cfg.AI.MCPs) > 0
	if len(cfg.AI.Agents) == 0 {
		if needsAgents {
			return fmt.Errorf("ai.agents must not be empty when ai permissions, skills, or mcps are present")
		}
		return nil
	}

	agentSet := make(map[string]bool, len(cfg.AI.Agents))
	for _, a := range cfg.AI.Agents {
		agentSet[a] = true
	}
	for agentName := range cfg.AI.Permissions {
		if !agentSet[agentName] {
			return fmt.Errorf("ai.permissions references undeclared agent %q (declared: %v)", agentName, cfg.AI.Agents)
		}
	}
	return nil
}
