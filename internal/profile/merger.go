package profile

import (
	"fmt"
)

// Merge combines two config layers. The overlay wins on conflicts.
// Call multiple times to merge three layers: Merge(Merge(base, profile), local).
func Merge(base, overlay *FacetConfig) (*FacetConfig, error) {
	result := &FacetConfig{}

	// Merge vars (deep merge)
	merged, err := deepMergeVars(base.Vars, overlay.Vars)
	if err != nil {
		return nil, err
	}
	result.Vars = merged

	// Merge packages (union by name, overlay wins on conflict)
	result.Packages = mergePackages(base.Packages, overlay.Packages)

	// Merge configs (shallow merge, overlay wins on same target)
	result.Configs = mergeConfigs(base.Configs, overlay.Configs)
	result.ConfigMeta = mergeConfigMeta(base.ConfigMeta, overlay.ConfigMeta)

	// Merge AI config
	result.AI = mergeAI(base.AI, overlay.AI)

	// Merge pre_apply scripts (concatenation — base first, then overlay)
	result.PreApply = mergeScripts(base.PreApply, overlay.PreApply)

	// Merge post_apply scripts (concatenation — base first, then overlay)
	result.PostApply = mergeScripts(base.PostApply, overlay.PostApply)

	return result, nil
}

// deepMergeVars recursively merges two vars maps.
// Both maps must have compatible types at each key — a map and a scalar at the same key is a type conflict error.
func deepMergeVars(base, overlay map[string]any) (map[string]any, error) {
	if base == nil && overlay == nil {
		return nil, nil
	}
	result := make(map[string]any)

	// Copy base
	for k, v := range base {
		result[k] = cloneValue(v)
	}

	// Merge overlay
	for k, v := range overlay {
		existing, exists := result[k]
		if !exists {
			result[k] = cloneValue(v)
			continue
		}

		existingMap, existingIsMap := toStringAnyMap(existing)
		overlayMap, overlayIsMap := toStringAnyMap(v)

		if existingIsMap && overlayIsMap {
			merged, err := deepMergeVars(existingMap, overlayMap)
			if err != nil {
				return nil, err
			}
			result[k] = merged
		} else if existingIsMap != overlayIsMap {
			return nil, fmt.Errorf("type conflict for var '%s': cannot merge map and scalar", k)
		} else {
			// Both are scalars — overlay wins
			result[k] = cloneValue(v)
		}
	}

	return result, nil
}

// toStringAnyMap attempts to convert a value to map[string]any.
// YAML v3 may decode as map[string]any or map[any]any.
func toStringAnyMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

// mergePackages unions two package lists by name. Overlay wins on name conflict.
func mergePackages(base, overlay []PackageEntry) []PackageEntry {
	byName := make(map[string]PackageEntry)
	var order []string

	for _, p := range base {
		byName[p.Name] = clonePackageEntry(p)
		order = append(order, p.Name)
	}
	for _, p := range overlay {
		if _, exists := byName[p.Name]; !exists {
			order = append(order, p.Name)
		}
		byName[p.Name] = clonePackageEntry(p)
	}

	result := make([]PackageEntry, 0, len(order))
	for _, name := range order {
		result = append(result, byName[name])
	}
	return result
}

// mergeConfigs shallow-merges two config maps. Overlay wins on same target key.
func mergeConfigs(base, overlay map[string]string) map[string]string {
	if base == nil && overlay == nil {
		return nil
	}
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

func mergeConfigMeta(base, overlay map[string]ConfigProvenance) map[string]ConfigProvenance {
	if base == nil && overlay == nil {
		return nil
	}
	result := make(map[string]ConfigProvenance)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// mergeScripts concatenates base scripts followed by overlay scripts.
// No deduplication — if both layers define a script with the same name, both run.
func mergePi(base, overlay *PiConfig) *PiConfig {
	if base == nil && overlay == nil {
		return nil
	}
	result := &PiConfig{}
	seen := make(map[string]int)
	for _, ext := range appendPiExtensions(nil, base) {
		if ext.Source == "" {
			continue
		}
		if _, ok := seen[ext.Source]; ok {
			continue
		}
		seen[ext.Source] = len(result.Extensions)
		result.Extensions = append(result.Extensions, clonePiExtensionEntry(ext))
	}
	for _, ext := range appendPiExtensions(nil, overlay) {
		if ext.Source == "" {
			continue
		}
		if idx, ok := seen[ext.Source]; ok {
			result.Extensions[idx] = clonePiExtensionEntry(ext)
			continue
		}
		seen[ext.Source] = len(result.Extensions)
		result.Extensions = append(result.Extensions, clonePiExtensionEntry(ext))
	}
	return result
}

func appendPiExtensions(dst []PiExtensionEntry, cfg *PiConfig) []PiExtensionEntry {
	if cfg == nil {
		return dst
	}
	return append(dst, cfg.Extensions...)
}

func clonePiExtensionEntry(src PiExtensionEntry) PiExtensionEntry {
	result := PiExtensionEntry{Source: src.Source}
	if src.InstallEnv != nil {
		result.InstallEnv = make(map[string]string, len(src.InstallEnv))
		for k, v := range src.InstallEnv {
			result.InstallEnv[k] = v
		}
	}
	return result
}

func mergeScripts(base, overlay []ScriptEntry) []ScriptEntry {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	result := make([]ScriptEntry, 0, len(base)+len(overlay))
	for _, script := range base {
		result = append(result, ScriptEntry{Name: script.Name, Run: script.Run, WorkDir: script.WorkDir})
	}
	for _, script := range overlay {
		result = append(result, ScriptEntry{Name: script.Name, Run: script.Run, WorkDir: script.WorkDir})
	}
	return result
}

func AnnotateLayer(cfg *FacetConfig, sourceRoot string, materialize bool) {
	if cfg == nil {
		return
	}
	if cfg.ConfigMeta == nil {
		cfg.ConfigMeta = make(map[string]ConfigProvenance, len(cfg.Configs))
	}
	for target := range cfg.Configs {
		cfg.ConfigMeta[target] = ConfigProvenance{
			SourceRoot:  sourceRoot,
			Materialize: materialize,
		}
	}
	for i := range cfg.PreApply {
		cfg.PreApply[i].WorkDir = sourceRoot
	}
	for i := range cfg.PostApply {
		cfg.PostApply[i].WorkDir = sourceRoot
	}
}

func clonePackageEntry(p PackageEntry) PackageEntry {
	return PackageEntry{
		Name:    p.Name,
		Check:   cloneInstallCmd(p.Check),
		Install: cloneInstallCmd(p.Install),
	}
}

func cloneInstallCmd(cmd InstallCmd) InstallCmd {
	result := InstallCmd{Command: cmd.Command}
	if cmd.PerOS != nil {
		result.PerOS = make(map[string]string, len(cmd.PerOS))
		for k, v := range cmd.PerOS {
			result.PerOS[k] = v
		}
	}
	return result
}

func cloneValue(v any) any {
	switch value := v.(type) {
	case nil:
		return nil
	case map[string]any:
		result := make(map[string]any, len(value))
		for k, inner := range value {
			result[k] = cloneValue(inner)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, inner := range value {
			result[i] = cloneValue(inner)
		}
		return result
	default:
		return value
	}
}

func cloneVars(vars map[string]any) map[string]any {
	if vars == nil {
		return nil
	}
	cloned, _ := cloneValue(vars).(map[string]any)
	return cloned
}
