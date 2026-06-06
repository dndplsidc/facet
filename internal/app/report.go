package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"facet/internal/ai"
	"facet/internal/deploy"
	"facet/internal/packages"
	"facet/internal/pi"
	"facet/internal/profile"
)

// ValidityCheck represents the result of checking a deployed config.
type ValidityCheck struct {
	Target string
	Valid  bool
	Error  string
}

func (a *App) printApplyReport(s *ApplyState) {
	a.reporter.Header(fmt.Sprintf("Applied profile: %s", s.Profile))

	if len(s.Packages) > 0 {
		a.reporter.Header("Packages")
		for _, pkg := range s.Packages {
			switch pkg.Status {
			case packages.StatusOK:
				a.reporter.Success(fmt.Sprintf("%-20s %s", pkg.Name, a.reporter.Dim(pkg.Install)))
			case packages.StatusAlreadyInstalled:
				a.reporter.Success(fmt.Sprintf("%-20s %s", pkg.Name, a.reporter.Dim("already installed")))
			case packages.StatusFailed:
				a.reporter.Error(fmt.Sprintf("%-20s %s — failed: %s", pkg.Name, pkg.Install, pkg.Error))
			case packages.StatusSkipped:
				a.reporter.Warning(fmt.Sprintf("%-20s %s", pkg.Name, pkg.Error))
			}
		}
	}

	if len(s.Configs) > 0 {
		a.reporter.Header("Configs")
		for _, cfg := range s.Configs {
			a.reporter.Success(fmt.Sprintf("%-30s → %-30s (%s)", cfg.Target, cfg.Source, cfg.Strategy))
		}
	}

	if s.AI != nil {
		a.printAIState(s.AI)
	} else if s.LegacyPi != nil {
		a.printAIState(&ai.AIState{Pi: s.LegacyPi})
	}
}

func (a *App) printStatus(s *ApplyState, checks []ValidityCheck) {
	a.reporter.Header(fmt.Sprintf("Profile: %s", s.Profile))
	a.reporter.PrintLine(fmt.Sprintf("  Applied: %s (%s ago)", s.AppliedAt.Format(time.RFC3339), timeSince(s.AppliedAt)))

	if len(s.Packages) > 0 {
		a.reporter.Header("Packages")
		for _, pkg := range s.Packages {
			switch pkg.Status {
			case packages.StatusOK:
				a.reporter.Success(fmt.Sprintf("%-20s %s", pkg.Name, a.reporter.Dim(pkg.Install)))
			case packages.StatusAlreadyInstalled:
				a.reporter.Success(fmt.Sprintf("%-20s %s", pkg.Name, a.reporter.Dim("already installed")))
			case packages.StatusFailed:
				a.reporter.Error(fmt.Sprintf("%-20s %s", pkg.Name, pkg.Error))
			case packages.StatusSkipped:
				a.reporter.Warning(fmt.Sprintf("%-20s %s", pkg.Name, pkg.Error))
			}
		}
	}

	if len(s.Configs) > 0 {
		a.reporter.Header("Configs")
		checkMap := make(map[string]ValidityCheck)
		for _, c := range checks {
			checkMap[c.Target] = c
		}

		for _, cfg := range s.Configs {
			check, hasCheck := checkMap[cfg.Target]
			if hasCheck && !check.Valid {
				a.reporter.Error(fmt.Sprintf("%-30s → %-30s (%s) (%s)", cfg.Target, cfg.Source, cfg.Strategy, check.Error))
			} else {
				a.reporter.Success(fmt.Sprintf("%-30s → %-30s (%s)", cfg.Target, cfg.Source, cfg.Strategy))
			}
		}
	}

	if s.AI != nil {
		a.printAIState(s.AI)
	} else if s.LegacyPi != nil {
		a.printAIState(&ai.AIState{Pi: s.LegacyPi})
	}
}

func (a *App) printNoState() {
	a.reporter.PrintLine("No profile has been applied yet.")
	a.reporter.PrintLine("Run: facet apply <profile>")
}

func (a *App) printDryRun(profileName string, resolved *profile.FacetConfig, opts ApplyOpts) error {
	a.reporter.Header(fmt.Sprintf("Dry run: %s", profileName))

	// Configs: show what would be deployed
	if len(resolved.Configs) > 0 {
		a.reporter.Header("Configs to deploy")

		targets := make([]string, 0, len(resolved.Configs))
		for target := range resolved.Configs {
			targets = append(targets, target)
		}
		sort.Strings(targets)

		for _, target := range targets {
			source := resolved.Configs[target]

			sourceSpec, err := deploy.ResolveSourcePath(source, resolved.ConfigMeta[target], opts.ConfigDir)
			if err != nil {
				a.reporter.Error(fmt.Sprintf("%-30s %s", target, err))
				continue
			}

			expandedTarget, err := deploy.ExpandPath(target)
			if err != nil {
				a.reporter.Error(fmt.Sprintf("%-30s %s", target, err))
				continue
			}

			strategy, err := deploy.DetectStrategy(sourceSpec.ResolvedPath, sourceSpec.Materialize)
			if err != nil {
				a.reporter.Error(fmt.Sprintf("%-30s %s", target, err))
				continue
			}

			a.reporter.Success(fmt.Sprintf("%-30s → %-30s (%s)", expandedTarget, sourceSpec.DisplaySource, strategy))
		}
	}

	// Pre-apply scripts
	if len(resolved.PreApply) > 0 {
		a.reporter.Header("Pre-apply scripts to run")
		for _, script := range resolved.PreApply {
			a.reporter.Success(fmt.Sprintf("%-20s %s", script.Name, a.reporter.Dim(script.Run)))
		}
	}

	// Packages: show what would be installed
	if len(resolved.Packages) > 0 {
		a.reporter.Header("Packages to install")
		for _, pkg := range resolved.Packages {
			cmd, skip := packages.GetInstallCommand(pkg, a.osName)
			if skip {
				a.reporter.Warning(fmt.Sprintf("%-20s %s", pkg.Name, fmt.Sprintf("no install command for OS %q", a.osName)))
			} else {
				checkCmd, hasCheck := packages.GetCheckCommand(pkg, a.osName)
				if hasCheck {
					a.reporter.Success(fmt.Sprintf("%-20s %s %s", pkg.Name, a.reporter.Dim(cmd), a.reporter.Dim(fmt.Sprintf("(check: %s)", checkCmd))))
				} else {
					a.reporter.Success(fmt.Sprintf("%-20s %s", pkg.Name, a.reporter.Dim(cmd)))
				}
			}
		}
	}

	// Pi extensions
	if resolved.AI != nil && resolved.AI.Pi != nil && len(resolved.AI.Pi.Extensions) > 0 {
		a.reporter.Header("AI Pi extensions to install")
		for _, ext := range resolved.AI.Pi.Extensions {
			a.reporter.Success(ext.Source)
		}
	}

	// Post-apply scripts
	if len(resolved.PostApply) > 0 {
		a.reporter.Header("Post-apply scripts to run")
		for _, script := range resolved.PostApply {
			a.reporter.Success(fmt.Sprintf("%-20s %s", script.Name, a.reporter.Dim(script.Run)))
		}
	}

	// Show what would be unapplied
	prevState, err := a.stateStore.Read(opts.StateDir)
	if err == nil && prevState != nil {
		shouldUnapply := opts.Force || prevState.Profile != profileName
		if shouldUnapply && len(prevState.Configs) > 0 {
			a.reporter.Header("Configs to remove (from previous profile)")
			for _, cfg := range prevState.Configs {
				a.reporter.Warning(fmt.Sprintf("%-30s (%s)", cfg.Target, cfg.Strategy))
			}
		}
	}

	// AI: show what would be configured
	if resolved.AI != nil {
		effectiveAI := ai.Resolve(resolved.AI)
		a.printAIDryRun(effectiveAI)
	}

	a.reporter.PrintLine("")
	a.reporter.PrintLine("No changes were made. Run without --dry-run to apply.")

	return nil
}

func (a *App) printAIDryRun(config ai.EffectiveAIConfig) {
	a.reporter.Header("AI configuration to apply")

	agents := make([]string, 0, len(config))
	for agent := range config {
		agents = append(agents, agent)
	}
	sort.Strings(agents)

	for _, agent := range agents {
		agentCfg := config[agent]
		a.reporter.PrintLine(fmt.Sprintf("  Agent: %s", agent))

		if len(agentCfg.Permissions.Allow) > 0 || len(agentCfg.Permissions.Deny) > 0 {
			if len(agentCfg.Permissions.Allow) > 0 {
				a.reporter.Success(fmt.Sprintf("    Permissions allow: %s", strings.Join(agentCfg.Permissions.Allow, ", ")))
			}
			if len(agentCfg.Permissions.Deny) > 0 {
				a.reporter.Warning(fmt.Sprintf("    Permissions deny:  %s", strings.Join(agentCfg.Permissions.Deny, ", ")))
			}
		}

		for _, skill := range agentCfg.Skills {
			name := skill.Name
			if name == "" {
				name = "all skills"
			}
			a.reporter.Success(fmt.Sprintf("    Skill: %s %s", name, a.reporter.Dim(fmt.Sprintf("(%s)", skill.Source))))
		}

		for _, mcp := range agentCfg.MCPs {
			a.reporter.Success(fmt.Sprintf("    MCP: %s %s", mcp.Name, a.reporter.Dim(mcp.Command)))
		}
	}
}

func (a *App) printPiState(piState *pi.PiState) {
	a.reporter.Header("Pi")
	for _, ext := range piState.Extensions {
		a.reporter.Success(fmt.Sprintf("Extension: %s", ext))
	}
}

func (a *App) printAIState(aiState *ai.AIState) {
	a.reporter.Header("AI")

	if aiState.Pi != nil {
		a.printPiState(aiState.Pi)
	}

	if len(aiState.Permissions) > 0 {
		agents := make([]string, 0, len(aiState.Permissions))
		for agent := range aiState.Permissions {
			agents = append(agents, agent)
		}
		sort.Strings(agents)

		for _, agent := range agents {
			ps := aiState.Permissions[agent]
			if len(ps.Allow) > 0 {
				a.reporter.Success(fmt.Sprintf("%-20s permissions allow: %s", agent, strings.Join(ps.Allow, ", ")))
			}
			if len(ps.Deny) > 0 {
				a.reporter.Warning(fmt.Sprintf("%-20s permissions deny:  %s", agent, strings.Join(ps.Deny, ", ")))
			}
		}
	}

	for _, skill := range aiState.Skills {
		name := skill.Name
		if name == "" {
			name = "all skills"
		}
		a.reporter.Success(fmt.Sprintf("Skill: %-20s agents: %s %s", name, strings.Join(skill.Agents, ", "), a.reporter.Dim(fmt.Sprintf("(%s)", skill.Source))))
	}

	for _, mcp := range aiState.MCPs {
		a.reporter.Success(fmt.Sprintf("MCP:   %-20s agents: %s", mcp.Name, strings.Join(mcp.Agents, ", ")))
	}
}

func timeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
