package profile

import "sort"

// PlatformSkip identifies a declaration with no value for the selected OS.
type PlatformSkip struct {
	Stage string
	Name  string
}

// SelectOS returns an independent configuration containing only active config
// sources and hooks. Packages without an installer remain for skip reporting,
// but their unused checks and variables are discarded.
func SelectOS(cfg *FacetConfig, osName string) (*FacetConfig, []PlatformSkip) {
	result := &FacetConfig{
		Extends:    cfg.Extends,
		Vars:       cloneVars(cfg.Vars),
		Configs:    make(map[string]OSValue, len(cfg.Configs)),
		ConfigMeta: make(map[string]ConfigProvenance, len(cfg.ConfigMeta)),
		AI:         mergeAI(nil, cfg.AI),
	}
	var skipped []PlatformSkip
	targets := make([]string, 0, len(cfg.Configs))
	for target := range cfg.Configs {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		if source, ok := cfg.Configs[target].ForOS(osName); ok {
			result.Configs[target] = OSValue{Value: source}
			if meta, exists := cfg.ConfigMeta[target]; exists {
				result.ConfigMeta[target] = meta
			}
		} else {
			skipped = append(skipped, PlatformSkip{Stage: "configs", Name: target})
		}
	}
	result.PreApply, skipped = selectScripts(cfg.PreApply, "pre_apply", osName, skipped)
	result.PostApply, skipped = selectScripts(cfg.PostApply, "post_apply", osName, skipped)
	for _, pkg := range cfg.Packages {
		selected := PackageEntry{Name: pkg.Name}
		if install, ok := pkg.Install.ForOS(osName); ok {
			selected.Install = InstallCmd{PerOS: map[string]string{osName: install}}
			if check, ok := pkg.Check.ForOS(osName); ok {
				selected.Check = InstallCmd{PerOS: map[string]string{osName: check}}
			}
		}
		result.Packages = append(result.Packages, selected)
	}
	return result, skipped
}

func selectScripts(scripts []ScriptEntry, stage, osName string, skipped []PlatformSkip) ([]ScriptEntry, []PlatformSkip) {
	var selected []ScriptEntry
	for _, script := range scripts {
		if command, ok := script.Run.ForOS(osName); ok {
			selected = append(selected, ScriptEntry{Name: script.Name, Run: OSValue{Value: command}, WorkDir: script.WorkDir})
		} else {
			skipped = append(skipped, PlatformSkip{Stage: stage, Name: script.Name})
		}
	}
	return selected, skipped
}
