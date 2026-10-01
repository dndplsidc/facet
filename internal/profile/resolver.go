package profile

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var facetVarPattern = regexp.MustCompile(`\$\{facet:([a-zA-Z0-9_.]+)(?:\|([a-zA-Z0-9_]+))?\}`)

// Resolve substitutes all ${facet:var.name} references in the config.
// Config target paths (map keys in Configs) are NOT resolved.
// Returns a new FacetConfig with all references substituted.
func Resolve(cfg *FacetConfig) (*FacetConfig, error) {
	result := &FacetConfig{
		Vars:       cloneVars(cfg.Vars), // vars themselves are not resolved (no recursion)
		Configs:    make(map[string]OSValue, len(cfg.Configs)),
		ConfigMeta: mergeConfigMeta(nil, cfg.ConfigMeta),
	}

	// Resolve packages
	for _, pkg := range cfg.Packages {
		resolved, err := resolvePackageEntry(pkg, cfg.Vars)
		if err != nil {
			return nil, err
		}
		result.Packages = append(result.Packages, resolved)
	}

	// Resolve config source paths (values), but NOT target paths (keys)
	for target, source := range cfg.Configs {
		resolvedSource, err := resolveOSValue(source, cfg.Vars)
		if err != nil {
			return nil, err
		}
		result.Configs[target] = resolvedSource
	}

	resolvedAI, err := resolveAI(cfg.AI, cfg.Vars)
	if err != nil {
		return nil, err
	}
	result.AI = resolvedAI

	if cfg.PreApply != nil {
		result.PreApply = make([]ScriptEntry, len(cfg.PreApply))
		for i, script := range cfg.PreApply {
			resolvedRun, err := resolveOSValue(script.Run, cfg.Vars)
			if err != nil {
				return nil, fmt.Errorf("pre_apply[%d] %q: %w", i, script.Name, err)
			}
			result.PreApply[i] = ScriptEntry{
				Name:    script.Name,
				Run:     resolvedRun,
				WorkDir: script.WorkDir,
			}
		}
	}

	if cfg.PostApply != nil {
		result.PostApply = make([]ScriptEntry, len(cfg.PostApply))
		for i, script := range cfg.PostApply {
			resolvedRun, err := resolveOSValue(script.Run, cfg.Vars)
			if err != nil {
				return nil, fmt.Errorf("post_apply[%d] %q: %w", i, script.Name, err)
			}
			result.PostApply[i] = ScriptEntry{
				Name:    script.Name,
				Run:     resolvedRun,
				WorkDir: script.WorkDir,
			}
		}
	}

	return result, nil
}

func resolvePackageEntry(pkg PackageEntry, vars map[string]any) (PackageEntry, error) {
	result := PackageEntry{Name: pkg.Name}

	// Resolve check command
	resolved, err := resolveInstallCmd(pkg.Check, vars)
	if err != nil {
		return result, err
	}
	result.Check = resolved

	// Resolve install command
	resolved, err = resolveInstallCmd(pkg.Install, vars)
	if err != nil {
		return result, err
	}
	result.Install = resolved

	return result, nil
}

func resolveInstallCmd(cmd InstallCmd, vars map[string]any) (InstallCmd, error) {
	var result InstallCmd

	if cmd.Command != "" {
		resolved, err := substituteVars(cmd.Command, vars)
		if err != nil {
			return result, err
		}
		result.Command = resolved
	}

	if cmd.PerOS != nil {
		result.PerOS = make(map[string]string, len(cmd.PerOS))
		for os, c := range cmd.PerOS {
			resolved, err := substituteVars(c, vars)
			if err != nil {
				return result, err
			}
			result.PerOS[os] = resolved
		}
	}

	return result, nil
}

// SubstituteVars replaces all ${facet:var.name} references in a string.
// It is exported so the deploy package can reuse the same substitution logic.
func SubstituteVars(s string, vars map[string]any) (string, error) {
	return substituteVars(s, vars)
}

// substituteVars replaces all ${facet:var.name} references in a string.
func substituteVars(s string, vars map[string]any) (string, error) {
	var resolveErr error

	result := facetVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		if resolveErr != nil {
			return match
		}
		// Extract the variable name from ${facet:var.name}
		submatches := facetVarPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		key := submatches[1]
		filter := submatches[2]
		if filter != "" {
			if filter != "yaml" {
				resolveErr = fmt.Errorf("unsupported variable filter %q in %s", filter, match)
				return match
			}
			value, err := lookupVarValue(vars, key)
			if err != nil {
				resolveErr = err
				return match
			}
			rendered, err := renderYAMLValue(value)
			if err != nil {
				resolveErr = fmt.Errorf("variable %s: YAML rendering failed: %w", match, err)
				return match
			}
			return rendered
		}

		value, err := lookupVar(vars, key)
		if err != nil {
			resolveErr = err
			return match
		}
		return value
	})

	if resolveErr != nil {
		return "", resolveErr
	}
	return result, nil
}

// resolveAI substitutes ${facet:...} references inside an AIConfig.
// It deep-copies mutable fields so the input is not modified.
func resolveAI(ai *AIConfig, vars map[string]any) (*AIConfig, error) {
	if ai == nil {
		return nil, nil
	}

	result := &AIConfig{}

	// Deep-copy Agents slice
	if ai.Agents != nil {
		result.Agents = make([]string, len(ai.Agents))
		copy(result.Agents, ai.Agents)
	}

	// Deep-copy Permissions map and nested slices.
	if ai.Permissions != nil {
		result.Permissions = make(map[string]*PermissionsConfig, len(ai.Permissions))
		for agent, perms := range ai.Permissions {
			if perms == nil {
				result.Permissions[agent] = nil
				continue
			}
			result.Permissions[agent] = &PermissionsConfig{
				Allow: append([]string{}, perms.Allow...),
				Deny:  append([]string{}, perms.Deny...),
			}
		}
	}

	// Deep-copy Skills slice (including inner slices to prevent aliasing)
	if ai.Skills != nil {
		result.Skills = make([]SkillEntry, len(ai.Skills))
		for i, entry := range ai.Skills {
			result.Skills[i] = SkillEntry{
				Source: entry.Source,
				Skills: append([]string{}, entry.Skills...),
				Agents: append([]string{}, entry.Agents...),
			}
		}
	}

	// Resolve Pi extensions
	resolvedPi, err := resolvePi(ai.Pi, vars)
	if err != nil {
		return nil, err
	}
	result.Pi = resolvedPi

	// Resolve MCPs
	if ai.MCPs != nil {
		result.MCPs = make([]MCPEntry, len(ai.MCPs))
		for i, mcp := range ai.MCPs {
			resolved, err := resolveMCPEntry(i, mcp, vars)
			if err != nil {
				return nil, err
			}
			result.MCPs[i] = resolved
		}
	}

	return result, nil
}

func resolvePi(piCfg *PiConfig, vars map[string]any) (*PiConfig, error) {
	if piCfg == nil {
		return nil, nil
	}
	result := &PiConfig{}
	if piCfg.Extensions != nil {
		result.Extensions = make([]PiExtensionEntry, len(piCfg.Extensions))
		for i, ext := range piCfg.Extensions {
			resolvedSource, err := substituteVars(ext.Source, vars)
			if err != nil {
				return nil, fmt.Errorf("ai.pi.extensions[%d].source: %w", i, err)
			}
			resolved := PiExtensionEntry{Source: resolvedSource}
			if ext.InstallEnv != nil {
				resolved.InstallEnv = make(map[string]string, len(ext.InstallEnv))
				for key, value := range ext.InstallEnv {
					resolvedValue, err := substituteVars(value, vars)
					if err != nil {
						return nil, fmt.Errorf("ai.pi.extensions[%d].install_env.%s: %w", i, key, err)
					}
					resolved.InstallEnv[key] = resolvedValue
				}
			}
			result.Extensions[i] = resolved
		}
	}
	return result, nil
}

// resolveMCPEntry substitutes ${facet:...} references in a single MCPEntry.
func resolveMCPEntry(i int, mcp MCPEntry, vars map[string]any) (MCPEntry, error) {
	result := MCPEntry{
		Name:   mcp.Name,
		Agents: append([]string{}, mcp.Agents...),
	}

	// Resolve Command
	if mcp.Command != "" {
		resolved, err := substituteVars(mcp.Command, vars)
		if err != nil {
			return result, fmt.Errorf("ai.mcps[%d].command: %w", i, err)
		}
		result.Command = resolved
	}

	// Resolve Args
	if mcp.Args != nil {
		result.Args = make([]string, len(mcp.Args))
		for j, arg := range mcp.Args {
			resolved, err := substituteVars(arg, vars)
			if err != nil {
				return result, fmt.Errorf("ai.mcps[%d].args[%d]: %w", i, j, err)
			}
			result.Args[j] = resolved
		}
	}

	// Resolve Env values
	if mcp.Env != nil {
		result.Env = make(map[string]string, len(mcp.Env))
		for k, v := range mcp.Env {
			resolved, err := substituteVars(v, vars)
			if err != nil {
				return result, fmt.Errorf("ai.mcps[%d].env[%s]: %w", i, k, err)
			}
			result.Env[k] = resolved
		}
	}

	return result, nil
}

// lookupVar resolves a dot-notation key against a nested vars map.
// For example, "git.email" looks up vars["git"]["email"].
func lookupVar(vars map[string]any, key string) (string, error) {
	value, err := lookupVarValue(vars, key)
	if err != nil {
		return "", err
	}
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("variable ${facet:%s} resolves to a non-string value — use a more specific path or the |yaml filter", key)
	}
	return s, nil
}

// lookupVarValue preserves the variable's type for structured rendering.
func lookupVarValue(vars map[string]any, key string) (any, error) {
	parts := strings.Split(key, ".")
	current := vars

	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			return "", fmt.Errorf("undefined variable: ${facet:%s} — define it in .local.yaml or your profile's vars", key)
		}

		if i == len(parts)-1 {
			return val, nil
		}

		// Intermediate — must be a map
		m, ok := toStringAnyMap(val)
		if !ok {
			return "", fmt.Errorf("variable ${facet:%s}: '%s' is not a map", key, part)
		}
		current = m
	}

	return "", fmt.Errorf("undefined variable: ${facet:%s}", key)
}

// renderYAMLValue emits one flow-style YAML value, independent of the template's
// indentation. Quoted strings preserve scalar types and escape embedded newlines.
func renderYAMLValue(value any) (string, error) {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return "", err
	}
	setInlineYAMLStyle(&node)
	rendered, err := yaml.Marshal(&node)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(rendered), "\n"), nil
}

func setInlineYAMLStyle(node *yaml.Node) {
	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		node.Style = yaml.FlowStyle
	case yaml.ScalarNode:
		if node.Tag == "!!str" {
			node.Style = yaml.DoubleQuotedStyle
		}
	}
	for _, child := range node.Content {
		setInlineYAMLStyle(child)
	}
}
