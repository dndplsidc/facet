package profile

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// OSValue is a shared string or a map of platform-specific strings.
// Selection happens after merging, before variable or filesystem resolution.
type OSValue struct {
	Value string
	PerOS map[string]string
}

func (v *OSValue) UnmarshalYAML(node *yaml.Node) error {
	parsed := OSValue{}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return fmt.Errorf("line %d: expected a string or macos/linux map", node.Line)
		}
		parsed.Value = node.Value
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Tag != "!!str" || value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fmt.Errorf("line %d: platform entries must map OS names to strings", key.Line)
			}
		}
		if err := node.Decode(&parsed.PerOS); err != nil {
			return err
		}
	default:
		return fmt.Errorf("line %d: expected a string or macos/linux map", node.Line)
	}
	if err := parsed.Validate(); err != nil {
		return fmt.Errorf("line %d: %w", node.Line, err)
	}
	*v = parsed
	return nil
}

func (v OSValue) MarshalYAML() (any, error) {
	if v.PerOS != nil {
		return v.PerOS, nil
	}
	return v.Value, nil
}

func (v OSValue) Validate() error {
	if v.PerOS == nil {
		if strings.TrimSpace(v.Value) == "" {
			return fmt.Errorf("value must be a non-empty string or macos/linux map")
		}
		return nil
	}
	if v.Value != "" {
		return fmt.Errorf("cannot combine a shared value and platform values")
	}
	if len(v.PerOS) == 0 {
		return fmt.Errorf("platform map must not be empty")
	}
	for key, value := range v.PerOS {
		if key != "macos" && key != "linux" {
			return fmt.Errorf("unknown OS %q; use macos or linux", key)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("value for %s must not be empty; omit the OS to skip it", key)
		}
	}
	return nil
}

func (v OSValue) ForOS(osName string) (string, bool) {
	if v.PerOS == nil {
		return v.Value, true
	}
	value, ok := v.PerOS[osName]
	return value, ok
}

func (v OSValue) Clone() OSValue {
	result := OSValue{Value: v.Value}
	if v.PerOS != nil {
		result.PerOS = make(map[string]string, len(v.PerOS))
		for key, value := range v.PerOS {
			result.PerOS[key] = value
		}
	}
	return result
}

func resolveOSValue(v OSValue, vars map[string]any) (OSValue, error) {
	result := v.Clone()
	var err error
	if result.PerOS == nil {
		result.Value, err = substituteVars(result.Value, vars)
		return result, err
	}
	for key, value := range result.PerOS {
		result.PerOS[key], err = substituteVars(value, vars)
		if err != nil {
			return OSValue{}, err
		}
	}
	return result, nil
}
