package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"facet/internal/ai"
	"facet/internal/deploy"
	"facet/internal/packages"
	"facet/internal/pi"
	"facet/internal/profile"
)

// validStages returns the ordered list of stage names that --stages accepts.
func validStages() []string {
	return []string{"configs", "pre_apply", "packages", "post_apply", "ai"}
}

// ValidStagesCSV returns the valid stage names as a comma-separated string for help text.
func ValidStagesCSV() string {
	return strings.Join(validStages(), ", ")
}

// parseStages parses a comma-separated stages string into a set.
// An empty string means all stages. Returns an error for invalid stage names.
func parseStages(raw string) (map[string]bool, error) {
	all := validStages()
	if raw == "" {
		result := make(map[string]bool, len(all))
		for _, s := range all {
			result[s] = true
		}
		return result, nil
	}

	result := make(map[string]bool)
	valid := make(map[string]bool, len(all))
	for _, s := range all {
		valid[s] = true
	}

	for _, part := range strings.Split(raw, ",") {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		if !valid[s] {
			return nil, fmt.Errorf("unknown stage %q; valid stages: %s", s, strings.Join(all, ", "))
		}
		result[s] = true
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid stages specified; valid stages: %s", strings.Join(all, ", "))
	}

	return result, nil
}

// ApplyOpts holds options for the Apply operation.
type ApplyOpts struct {
	ConfigDir   string
	StateDir    string
	Force       bool
	SkipFailure bool
	DryRun      bool
	Stages      string
}

// Apply loads, merges, and applies a configuration profile.
func (a *App) Apply(profileName string, opts ApplyOpts) (applyErr error) {
	applyDone := a.reporter.ProgressStart(fmt.Sprintf("facet apply %s", profileName))
	defer func() {
		outcome := "done"
		if applyErr != nil {
			outcome = "failed"
		}
		applyDone(outcome, applyErr)
	}()

	stages, err := parseStages(opts.Stages)
	if err != nil {
		return err
	}

	// Step 1: Load facet.yaml
	var metaErr error
	metaErr = a.reporter.ProgressStep("Loading metadata", func() error {
		_, err = a.loader.LoadMeta(opts.ConfigDir)
		return err
	})
	if metaErr != nil {
		return fmt.Errorf("Not a facet config directory (facet.yaml not found). Use -c to specify the config directory, or run facet scaffold to create one.\n  detail: %w", metaErr)
	}

	// Step 2: Load profile
	profilePath := filepath.Join(opts.ConfigDir, "profiles", profileName+".yaml")
	var profileCfg *profile.FacetConfig
	if err := a.reporter.ProgressStep("Loading profile", func() error {
		var stepErr error
		profileCfg, stepErr = a.loader.LoadConfig(profilePath)
		return stepErr
	}); err != nil {
		return fmt.Errorf("cannot load profile %q: %w", profileName, err)
	}
	if err := a.reporter.ProgressStep("Validating profile", func() error {
		return profile.ValidateProfile(profileCfg)
	}); err != nil {
		return err
	}
	profile.AnnotateLayer(profileCfg, opts.ConfigDir, false)

	if a.baseResolver == nil {
		return fmt.Errorf("base resolver is not configured")
	}
	var resolvedBase *profile.ResolvedBase
	if err := a.reporter.ProgressStep("Resolving extends", func() error {
		var stepErr error
		resolvedBase, stepErr = a.baseResolver.Resolve(profileCfg.Extends, opts.ConfigDir)
		return stepErr
	}); err != nil {
		return fmt.Errorf("cannot resolve extends %q: %w", profileCfg.Extends, err)
	}
	defer func() {
		if cleanupErr := resolvedBase.Cleanup(); cleanupErr != nil {
			a.reporter.Warning(fmt.Sprintf("Failed to clean extends clone: %v", cleanupErr))
		}
	}()
	baseCfg := resolvedBase.Config

	// Step 3: Load .local.yaml
	localPath := filepath.Join(opts.StateDir, ".local.yaml")
	var localCfg *profile.FacetConfig
	if err := a.reporter.ProgressStep("Loading local config", func() error {
		var stepErr error
		localCfg, stepErr = a.loader.LoadConfig(localPath)
		return stepErr
	}); err != nil {
		return fmt.Errorf(".local.yaml is required in %s: %w", opts.StateDir, err)
	}
	profile.AnnotateLayer(localCfg, opts.ConfigDir, false)

	// Step 4: Merge layers
	var merged *profile.FacetConfig
	if err := a.reporter.ProgressStep("Merging base and profile", func() error {
		var stepErr error
		merged, stepErr = profile.Merge(baseCfg, profileCfg)
		return stepErr
	}); err != nil {
		return fmt.Errorf("merge error: %w", err)
	}
	if err := a.reporter.ProgressStep("Merging local config", func() error {
		var stepErr error
		merged, stepErr = profile.Merge(merged, localCfg)
		return stepErr
	}); err != nil {
		return fmt.Errorf("merge error with .local.yaml: %w", err)
	}
	if err := a.reporter.ProgressStep("Validating merged config", func() error {
		return profile.ValidateMergedConfig(merged)
	}); err != nil {
		return err
	}

	// Step 5: Resolve variables
	var resolved *profile.FacetConfig
	if err := a.reporter.ProgressStep("Resolving variables", func() error {
		var stepErr error
		resolved, stepErr = profile.Resolve(merged)
		return stepErr
	}); err != nil {
		return err
	}

	// Dry-run: preview what would happen without side effects
	if opts.DryRun {
		return a.printDryRun(profileName, resolved, opts)
	}

	// Step 7: Canary write to .state.json
	if err := a.reporter.ProgressStep("Creating state directory", func() error {
		return os.MkdirAll(opts.StateDir, 0o755)
	}); err != nil {
		return fmt.Errorf("cannot create state directory %s: %w", opts.StateDir, err)
	}
	if err := a.reporter.ProgressStep("Writing state canary", func() error {
		return a.stateStore.CanaryWrite(opts.StateDir)
	}); err != nil {
		return fmt.Errorf("cannot write state file: %w", err)
	}

	// Read previous state for unapply
	var prevState *ApplyState
	readErr := a.reporter.ProgressStep("Reading previous state", func() error {
		var stepErr error
		prevState, stepErr = a.stateStore.Read(opts.StateDir)
		return stepErr
	})
	if readErr != nil {
		a.reporter.Warning(fmt.Sprintf("Could not read previous state: %v", readErr))
	}

	var prevConfigs []deploy.ConfigResult
	var prevAIState *ai.AIState
	var prevPiState *pi.PiState
	currentConfigTargets := make(map[string]bool)
	preserveAllPreviousConfigs := false
	if prevState != nil {
		prevConfigs = prevState.Configs
		prevAIState = prevState.AI
		if prevState.AI != nil {
			prevPiState = prevState.AI.Pi
		}
		if prevPiState == nil {
			prevPiState = prevState.LegacyPi
		}
	}

	// Unapply previous state if needed
	if prevState != nil {
		shouldUnapply := opts.Force || prevState.Profile != profileName
		if shouldUnapply {
			willUnapplyConfigs := stages["configs"] && len(prevState.Configs) > 0
			willUnapplyAI := stages["ai"] && a.aiOrchestrator != nil && prevState.AI != nil
			willUnapplyPi := stages["ai"] && a.piManager != nil && prevPiState != nil
			if willUnapplyConfigs || willUnapplyAI || willUnapplyPi {
				done := a.reporter.ProgressStart("Unapplying previous state")
				if willUnapplyAI {
					if err := a.reporter.ProgressStep("  -> AI unapply", func() error {
						return a.aiOrchestrator.Unapply(prevState.AI)
					}); err != nil {
						a.reporter.Error(fmt.Sprintf("AI unapply failed: %v", err))
					}
					prevAIState = nil
				}
				if willUnapplyPi {
					if err := a.reporter.ProgressStep("  -> Pi unapply", func() error {
						return a.piManager.Unapply(prevPiState)
					}); err != nil {
						a.reporter.Error(fmt.Sprintf("Pi unapply failed: %v", err))
					}
					prevPiState = nil
				}
				if stages["configs"] {
					unapplyDeployer := a.deployerFactory(opts.ConfigDir, "", resolved.Vars, nil)
					if err := a.reporter.ProgressStep("  -> configs unapply", func() error {
						return unapplyDeployer.Unapply(prevState.Configs)
					}); err != nil {
						a.reporter.Warning(fmt.Sprintf("Unapply warning: %v", err))
					}
					prevConfigs = nil
				}
				done("done", nil)
			}
			if !willUnapplyConfigs && stages["configs"] {
				prevConfigs = nil
			}
		} else if stages["configs"] {
			// Same profile reapply — find orphaned configs to clean up
			for target := range resolved.Configs {
				expanded, err := deploy.ExpandPath(target)
				if err != nil {
					if opts.SkipFailure {
						preserveAllPreviousConfigs = true
						continue
					}
					return err
				}
				currentConfigTargets[expanded] = true
			}

			if !preserveAllPreviousConfigs {
				var orphans []deploy.ConfigResult
				for _, cfg := range prevState.Configs {
					if !currentConfigTargets[cfg.Target] {
						orphans = append(orphans, cfg)
					}
				}
				if len(orphans) > 0 {
					done := a.reporter.ProgressStart("Cleaning up orphaned configs")
					orphanDeployer := a.deployerFactory(opts.ConfigDir, "", nil, nil)
					if err := a.reporter.ProgressStep("  -> orphan configs unapply", func() error {
						return orphanDeployer.Unapply(orphans)
					}); err != nil {
						a.reporter.Warning(fmt.Sprintf("Orphan cleanup warning: %v", err))
					}
					done("done", nil)
				}
			}
		}
	}

	// Step 8: Deploy configs (sorted for deterministic order)
	deployer := a.deployerFactory(opts.ConfigDir, "", resolved.Vars, prevConfigs)

	if stages["configs"] && len(resolved.Configs) > 0 {
		done := a.reporter.ProgressStart("Deploying configs")
		var deployErr error

		targets := make([]string, 0, len(resolved.Configs))
		for target := range resolved.Configs {
			targets = append(targets, target)
		}
		sort.Strings(targets)

		for _, target := range targets {
			source := resolved.Configs[target]

			var sourceSpec deploy.SourceSpec
			err := a.reporter.ProgressStep("  -> "+target+" source", func() error {
				var stepErr error
				sourceSpec, stepErr = deploy.ResolveSourcePath(source, resolved.ConfigMeta[target], opts.ConfigDir)
				return stepErr
			})
			if err != nil {
				if opts.SkipFailure {
					a.reporter.Warning(fmt.Sprintf("Skipping config %s: %v", target, err))
					continue
				}
				a.rollbackDeployedConfigs(deployer)
				finalErr := fmt.Errorf("config deployment failed: %w", err)
				done("failed", finalErr)
				return finalErr
			}

			var expandedTarget string
			err = a.reporter.ProgressStep("  -> "+target+" expand", func() error {
				var stepErr error
				expandedTarget, stepErr = deploy.ExpandPath(target)
				return stepErr
			})
			if err != nil {
				if opts.SkipFailure {
					a.reporter.Warning(fmt.Sprintf("Skipping config %s: %v", target, err))
					continue
				}
				a.rollbackDeployedConfigs(deployer)
				finalErr := fmt.Errorf("config deployment failed: %w", err)
				done("failed", finalErr)
				return finalErr
			}

			err = a.reporter.ProgressStep("  -> "+target, func() error {
				_, stepErr := deployer.DeployOne(expandedTarget, sourceSpec, opts.Force)
				return stepErr
			})
			if err != nil {
				if opts.SkipFailure {
					a.reporter.Warning(fmt.Sprintf("Config deploy warning: %v", err))
					continue
				}
				deployErr = err
				break
			}
		}

		if deployErr != nil {
			a.reporter.Error(fmt.Sprintf("Config deployment failed: %v", deployErr))
			a.reporter.Warning("Rolling back deployed configs...")
			a.rollbackDeployedConfigs(deployer)
			if prevState == nil {
				os.Remove(filepath.Join(opts.StateDir, stateFile))
			}
			finalErr := fmt.Errorf("config deployment failed (rolled back): %w", deployErr)
			done("failed", finalErr)
			return finalErr
		}
		done("done", nil)
	}

	// Stage: pre_apply
	if stages["pre_apply"] {
		if len(resolved.PreApply) > 0 {
			done := a.reporter.ProgressStart("Running pre_apply scripts")
			if err := a.runScripts(resolved.PreApply, opts.ConfigDir, "pre_apply"); err != nil {
				done("failed", err)
				return err
			}
			done("done", nil)
		}
	}

	// Stage: packages
	var pkgResults []packages.PackageResult
	if stages["packages"] {
		if len(resolved.Packages) > 0 {
			done := a.reporter.ProgressStart("Installing packages")
			pkgResults = a.installer.InstallAll(resolved.Packages)
			done("done", nil)
		} else {
			pkgResults = a.installer.InstallAll(resolved.Packages)
		}
	}

	// Stage: post_apply
	if stages["post_apply"] {
		if len(resolved.PostApply) > 0 {
			done := a.reporter.ProgressStart("Running post_apply scripts")
			if err := a.runScripts(resolved.PostApply, opts.ConfigDir, "post_apply"); err != nil {
				done("failed", err)
				return err
			}
			done("done", nil)
		}
	}

	// Stage: ai
	var aiState *ai.AIState
	var piState *pi.PiState
	effectiveAI := ai.Resolve(resolved.AI)
	var effectivePi *pi.Config
	if resolved.AI != nil && resolved.AI.Pi != nil {
		effectivePi = &pi.Config{Extensions: append([]string{}, resolved.AI.Pi.Extensions...)}
	}
	if stages["ai"] && ((a.aiOrchestrator != nil && (effectiveAI != nil || prevAIState != nil)) || (a.piManager != nil && (effectivePi != nil || prevPiState != nil))) {
		done := a.reporter.ProgressStart("Applying AI configuration")
		var aiErr error
		if a.aiOrchestrator != nil && (effectiveAI != nil || prevAIState != nil) {
			aiState, aiErr = a.aiOrchestrator.Apply(effectiveAI, prevAIState)
			if aiErr != nil {
				a.reporter.Error(fmt.Sprintf("AI configuration failed: %v", aiErr))
			}
		}
		if a.piManager != nil && (effectivePi != nil || prevPiState != nil) {
			var piErr error
			piState, piErr = a.piManager.Apply(effectivePi, prevPiState, pi.ApplyOptions{Force: opts.Force})
			if piErr != nil {
				a.reporter.Error(fmt.Sprintf("Pi extensions failed: %v", piErr))
				if aiErr == nil {
					aiErr = piErr
				}
			}
		}
		if piState != nil {
			if aiState == nil {
				aiState = &ai.AIState{}
			}
			aiState.Pi = piState
		}
		if aiErr != nil {
			done("failed", aiErr)
		} else {
			done("done", nil)
		}
		if isEmptyAIState(aiState) {
			aiState = nil
		}
	}

	// Carry forward results from previous state for skipped stages
	configResults := deployer.Deployed()
	if prevState != nil && prevState.Profile == profileName && stages["configs"] {
		configResults = mergeConfigResults(prevConfigs, deployer.Deployed(), currentConfigTargets, preserveAllPreviousConfigs)
	}
	if !stages["configs"] && prevState != nil {
		configResults = prevState.Configs
	}
	if !stages["packages"] && prevState != nil {
		pkgResults = prevState.Packages
	}
	if !stages["ai"] && prevState != nil {
		aiState = prevState.AI
		if aiState == nil && prevState.LegacyPi != nil {
			aiState = &ai.AIState{Pi: prevState.LegacyPi}
		}
	}

	appliedProfile := profileName
	if prevState != nil && !stages["configs"] {
		appliedProfile = prevState.Profile
	}

	// Write final state
	applyState := &ApplyState{
		Profile:      appliedProfile,
		AppliedAt:    time.Now().UTC(),
		FacetVersion: a.version,
		Packages:     pkgResults,
		Configs:      configResults,
		AI:           aiState,
	}

	if err := a.reporter.ProgressStep("Writing state", func() error {
		return a.stateStore.Write(opts.StateDir, applyState)
	}); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	// Step 12: Report
	if err := a.reporter.ProgressStep("Rendering apply report", func() error {
		a.printApplyReport(applyState)
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (a *App) rollbackDeployedConfigs(deployer deploy.Service) {
	_ = a.reporter.ProgressStep("Rolling back deployed configs", func() error {
		return deployer.Rollback()
	})
}

// runScripts executes a list of scripts sequentially. Fails fast on first error.
func (a *App) runScripts(scripts []profile.ScriptEntry, fallbackDir, stageName string) error {
	if len(scripts) == 0 {
		return nil
	}
	if a.scriptRunner == nil {
		return fmt.Errorf("%s script runner is not configured", stageName)
	}

	for _, script := range scripts {
		dir := script.WorkDir
		if dir == "" {
			dir = fallbackDir
		}
		if err := a.reporter.ProgressStep("  -> "+script.Name, func() error {
			return a.scriptRunner.Run(script.Run, dir)
		}); err != nil {
			return fmt.Errorf("%s script %q failed: %w", stageName, script.Name, err)
		}
	}

	return nil
}

func isEmptyAIState(state *ai.AIState) bool {
	if state == nil {
		return true
	}
	return len(state.Skills) == 0 && len(state.MCPs) == 0 && len(state.Permissions) == 0 && state.Pi == nil
}

func mergeConfigResults(prevConfigs, deployedConfigs []deploy.ConfigResult, keepTargets map[string]bool, preserveAllPrevious bool) []deploy.ConfigResult {
	merged := make(map[string]deploy.ConfigResult, len(prevConfigs)+len(deployedConfigs))

	for _, cfg := range prevConfigs {
		if preserveAllPrevious || keepTargets[cfg.Target] {
			merged[cfg.Target] = cfg
		}
	}
	for _, cfg := range deployedConfigs {
		merged[cfg.Target] = cfg
	}

	targets := make([]string, 0, len(merged))
	for target := range merged {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	results := make([]deploy.ConfigResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, merged[target])
	}
	return results
}
