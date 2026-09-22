package profile

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// FacetMeta represents the facet.yaml metadata file.
type FacetMeta struct {
	MinVersion string `yaml:"min_version"`
}

// FacetConfig represents a parsed base.yaml, profile, or .local.yaml.
type FacetConfig struct {
	Extends    string                      `yaml:"extends,omitempty"`
	Vars       map[string]any              `yaml:"vars,omitempty"`
	Packages   []PackageEntry              `yaml:"packages,omitempty"`
	Configs    map[string]OSValue          `yaml:"configs,omitempty"`
	ConfigMeta map[string]ConfigProvenance `yaml:"-"`
	AI         *AIConfig                   `yaml:"ai,omitempty"`
	PreApply   []ScriptEntry               `yaml:"pre_apply,omitempty"`
	PostApply  []ScriptEntry               `yaml:"post_apply,omitempty"`
}

// ScriptEntry is a named shell command run during facet apply.
type ScriptEntry struct {
	Name    string  `yaml:"name"`
	Run     OSValue `yaml:"run"`
	WorkDir string  `yaml:"-"`
}

type ConfigProvenance struct {
	SourceRoot  string `yaml:"-"`
	Materialize bool   `yaml:"-"`
}

// PackageEntry is a package with a name, optional check command, and install command.
// If Check is set and the check command succeeds (exit 0), the install is skipped.
type PackageEntry struct {
	Name    string     `yaml:"name"`
	Check   InstallCmd `yaml:"check,omitempty"`
	Install InstallCmd `yaml:"install"`
}

// InstallCmd can be a simple string command or a per-OS map.
type InstallCmd struct {
	Command string            // non-empty if install is a plain string
	PerOS   map[string]string // non-nil if install is a per-OS map
}

// UnmarshalYAML handles both string and map forms of a command field (install or check).
func (c *InstallCmd) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		c.Command = value.Value
		return nil
	}
	if value.Kind == yaml.MappingNode {
		c.PerOS = make(map[string]string)
		return value.Decode(&c.PerOS)
	}
	return fmt.Errorf("field must be a string or a map of OS-specific commands")
}

// ForOS returns the install command for the given OS.
// Returns the command and true if available, or empty string and false if not.
func (c *InstallCmd) ForOS(os string) (string, bool) {
	if c.Command != "" {
		return c.Command, true
	}
	cmd, ok := c.PerOS[os]
	return cmd, ok
}
