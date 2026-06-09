package ai

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Reporter is the interface that the Orchestrator uses for user-facing output.
// It is defined here (at the consumer) per Go convention. The concrete
// implementation lives in common/reporter and is wired in main.go.
type Reporter interface {
	Success(msg string)
	Warning(msg string)
	Error(msg string)
	Header(msg string)
	PrintLine(msg string)
	Dim(text string) string
	ProgressDuration(label, outcome string, elapsed time.Duration, err error)
	ProgressStart(label string) func(outcome string, err error)
	ProgressStep(label string, fn func() error) error
}

// Orchestrator coordinates AI configuration across all agent providers.
type Orchestrator struct {
	providers     map[string]AgentProvider
	skillsManager SkillsManager
	reporter      Reporter
}

// NewOrchestrator constructs an Orchestrator with the given dependencies.
func NewOrchestrator(
	providers map[string]AgentProvider,
	skillsManager SkillsManager,
	reporter Reporter,
) *Orchestrator {
	return &Orchestrator{
		providers:     providers,
		skillsManager: skillsManager,
		reporter:      reporter,
	}
}

// Apply applies the effective AI configuration to all agents and returns the
// resulting state. Individual failures are non-fatal: they are logged and
// skipped. The function never returns an error.
func (o *Orchestrator) Apply(config EffectiveAIConfig, previousState *AIState) (*AIState, error) {
	state := &AIState{
		Permissions: make(map[string]PermissionState),
	}

	// 1. Apply permissions for each agent.
	o.applyPermissions(config, previousState, state)

	// 2. Apply skills with orphan removal.
	o.applySkills(config, previousState, state)

	// 3. Apply MCPs with orphan removal.
	o.applyMCPs(config, previousState, state)

	return state, nil
}

// Unapply removes all AI configuration tracked in previousState. Order is
// reverse of apply: MCPs, then skills, then permissions.
func (o *Orchestrator) Unapply(previousState *AIState) error {
	if previousState == nil {
		return nil
	}

	// 1. Remove MCPs.
	for _, mcp := range previousState.MCPs {
		for _, agent := range mcp.Agents {
			provider, ok := o.providers[agent]
			if !ok {
				o.reporter.Warning(fmt.Sprintf("no provider for agent %q, skipping MCP removal for %q", agent, mcp.Name))
				continue
			}
			label := fmt.Sprintf("  -> mcp remove %s %s", mcp.Name, agent)
			if err := o.progressStarted(label, func() error {
				return provider.RemoveMCP(mcp.Name)
			}); err != nil {
				o.reporter.Warning(fmt.Sprintf("failed to remove MCP %q from %q: %v", mcp.Name, agent, err))
			}
		}
	}

	// 2. Remove skills.
	for _, skill := range previousState.Skills {
		skillsToRemove := []string{skill.Name}
		if skill.Name == "" {
			label := fmt.Sprintf("  -> skills detect %s", skill.Source)
			if err := o.progressStarted(label, func() error {
				var err error
				skillsToRemove, err = o.skillsManager.InstalledForSource(skill.Source)
				return err
			}); err != nil {
				o.reporter.Warning(fmt.Sprintf("failed to resolve skills for source %q: %v", skill.Source, err))
				continue
			}
			if len(skillsToRemove) == 0 {
				continue
			}
		}
		label := fmt.Sprintf("  -> skills remove %s %s", strings.Join(skillsToRemove, ","), strings.Join(skill.Agents, ","))
		if err := o.progressStarted(label, func() error {
			return o.skillsManager.Remove(skillsToRemove, skill.Agents)
		}); err != nil {
			o.reporter.Warning(fmt.Sprintf("failed to remove skills for source %q: %v", skill.Source, err))
		}
	}

	// 3. Remove permissions.
	for agent, ps := range previousState.Permissions {
		provider, ok := o.providers[agent]
		if !ok {
			o.reporter.Warning(fmt.Sprintf("no provider for agent %q, skipping permission removal", agent))
			continue
		}
		perms := ResolvedPermissions{Allow: ps.Allow, Deny: ps.Deny}
		label := fmt.Sprintf("  -> permissions remove %s", agent)
		if err := o.progressStarted(label, func() error {
			return provider.RemovePermissions(perms)
		}); err != nil {
			o.reporter.Warning(fmt.Sprintf("failed to remove permissions for %q: %v", agent, err))
		}
	}

	return nil
}

func (o *Orchestrator) progressStarted(label string, fn func() error) error {
	done := o.reporter.ProgressStart(label)
	err := fn()
	outcome := "ok"
	if err != nil {
		outcome = "failed"
	}
	done(outcome, err)
	return err
}

// applyPermissions removes permissions for dropped agents, then applies
// native permissions directly for each current agent.
func (o *Orchestrator) applyPermissions(config EffectiveAIConfig, previousState *AIState, state *AIState) {
	done := o.reporter.ProgressStart("AI permissions")
	defer done("done", nil)

	if previousState != nil {
		for agent, ps := range previousState.Permissions {
			if _, exists := config[agent]; exists {
				continue
			}

			provider, ok := o.providers[agent]
			if !ok {
				o.reporter.Warning(fmt.Sprintf("no provider for agent %q, skipping permission removal", agent))
				continue
			}

			perms := ResolvedPermissions{Allow: ps.Allow, Deny: ps.Deny}
			if err := o.reporter.ProgressStep("  -> permissions remove "+agent, func() error {
				return provider.RemovePermissions(perms)
			}); err != nil {
				o.reporter.Warning(fmt.Sprintf("failed to remove permissions for dropped agent %q: %v", agent, err))
			} else {
				o.reporter.Success(fmt.Sprintf("removed permissions for %s", agent))
			}
		}
	}

	// Sort agent names for deterministic iteration order.
	agents := sortedKeys(config)

	for _, agent := range agents {
		agentCfg := config[agent]

		perms := agentCfg.Permissions
		provider, ok := o.providers[agent]
		if !ok {
			if len(perms.Allow) == 0 && len(perms.Deny) == 0 {
				continue
			}
			o.reporter.Warning(fmt.Sprintf("no provider for agent %q, skipping permissions", agent))
			continue
		}

		if err := o.reporter.ProgressStep("  -> permissions "+agent, func() error {
			return provider.ApplyPermissions(perms)
		}); err != nil {
			o.reporter.Error(fmt.Sprintf("failed to apply permissions for %q: %v", agent, err))
			continue
		}

		state.Permissions[agent] = PermissionState{
			Allow: perms.Allow,
			Deny:  perms.Deny,
		}
		o.reporter.Success(fmt.Sprintf("applied permissions for %s", agent))
	}
}

// skillGroupKey uniquely identifies a group of skills that share the same
// source and agent set. Skills in the same group can be installed in a single
// Install call.
type skillGroupKey struct {
	source string
	agents string // sorted, comma-joined agent names
}

type skillID struct {
	source string
	name   string
}

// applySkills installs current skills and removes orphans from previousState.
func (o *Orchestrator) applySkills(config EffectiveAIConfig, previousState *AIState, state *AIState) {
	done := o.reporter.ProgressStart("AI skills")
	defer done("done", nil)

	currentSkills := make(map[skillID]map[string]struct{})
	// Track which sources have an "all" entry per agent.
	currentAllSources := make(map[string]map[string]struct{}) // source → set of agents

	for agent, agentCfg := range config {
		for _, skill := range agentCfg.Skills {
			key := skillID{source: skill.Source, name: skill.Name}
			if _, exists := currentSkills[key]; !exists {
				currentSkills[key] = make(map[string]struct{})
			}
			currentSkills[key][agent] = struct{}{}

			if skill.Name == "" {
				if _, exists := currentAllSources[skill.Source]; !exists {
					currentAllSources[skill.Source] = make(map[string]struct{})
				}
				currentAllSources[skill.Source][agent] = struct{}{}
			}
		}
	}

	if previousState != nil {
		for _, prevSkill := range previousState.Skills {
			for _, agent := range prevSkill.Agents {
				if prevSkill.Name == "" {
					var skillsToRemove []string
					err := o.reporter.ProgressStep("  -> skills detect orphans "+prevSkill.Source+" "+agent, func() error {
						var stepErr error
						skillsToRemove, stepErr = o.sourceSkillsToRemove(prevSkill.Source, agent, currentSkills, currentAllSources)
						return stepErr
					})
					if err != nil {
						o.reporter.Warning(fmt.Sprintf("failed to resolve orphan skills from %q for %q: %v", prevSkill.Source, agent, err))
						continue
					}
					if len(skillsToRemove) == 0 {
						continue
					}
					if err := o.reporter.ProgressStep("  -> skills remove "+prevSkill.Source+" "+agent, func() error {
						return o.skillsManager.Remove(skillsToRemove, []string{agent})
					}); err != nil {
						o.reporter.Warning(fmt.Sprintf("failed to remove orphan skills from %q for %q: %v", prevSkill.Source, agent, err))
					} else {
						o.reporter.Success(fmt.Sprintf("removed orphan skills %v from %s", skillsToRemove, agent))
					}
					continue
				}

				currentAgents := currentSkills[skillID{source: prevSkill.Source, name: prevSkill.Name}]
				if _, exists := currentAgents[agent]; exists {
					continue
				}
				// Skip removal if current config has "all" for this source+agent.
				if allAgents, hasAll := currentAllSources[prevSkill.Source]; hasAll {
					if _, covered := allAgents[agent]; covered {
						continue
					}
				}
				if err := o.reporter.ProgressStep("  -> skills remove "+prevSkill.Name+" "+agent, func() error {
					return o.skillsManager.Remove([]string{prevSkill.Name}, []string{agent})
				}); err != nil {
					o.reporter.Warning(fmt.Sprintf("failed to remove orphan skill %q from %q: %v", prevSkill.Name, agent, err))
				} else {
					o.reporter.Success(fmt.Sprintf("removed orphan skill %q from %s", prevSkill.Name, agent))
				}
			}
		}
	}

	// Group skills by (source, sorted agents) for batched Install calls.
	groups := make(map[skillGroupKey][]string)
	// Track which groups are "all" (contain the empty-name sentinel).
	allGroups := make(map[skillGroupKey]bool)

	for id, agentSet := range currentSkills {
		agents := sortedSetKeys(agentSet)
		key := skillGroupKey{
			source: id.source,
			agents: strings.Join(agents, ","),
		}
		groups[key] = append(groups[key], id.name)
		if id.name == "" {
			allGroups[key] = true
		}
	}

	groupKeys := make([]skillGroupKey, 0, len(groups))
	for gk := range groups {
		groupKeys = append(groupKeys, gk)
	}
	sort.Slice(groupKeys, func(i, j int) bool {
		if groupKeys[i].source != groupKeys[j].source {
			return groupKeys[i].source < groupKeys[j].source
		}
		return groupKeys[i].agents < groupKeys[j].agents
	})

	for _, gk := range groupKeys {
		skills := groups[gk]
		agents := strings.Split(gk.agents, ",")

		var installSkills []string
		if allGroups[gk] {
			// "All" group — pass nil to request all skills while preserving explicit agent scope.
			installSkills = nil
		} else {
			sort.Strings(skills)
			installSkills = skills
		}

		label := fmt.Sprintf("  -> skills install %s [%s]", gk.source, strings.Join(agents, ","))
		if err := o.reporter.ProgressStep(label, func() error {
			return o.skillsManager.Install(gk.source, installSkills, agents)
		}); err != nil {
			o.reporter.Warning(fmt.Sprintf("failed to install skills from %q: %v", gk.source, err))
			continue
		}

		recordedSkills := skills
		verifiedViaLock := true
		if allGroups[gk] {
			var resolvedSkills []string
			verifyErr := o.reporter.ProgressStep("  -> skills verify "+gk.source, func() error {
				var stepErr error
				resolvedSkills, stepErr = o.skillsManager.InstalledForSource(gk.source)
				return stepErr
			})
			if verifyErr != nil {
				o.reporter.Warning(fmt.Sprintf("installed all skills from %q, but failed to resolve concrete skill names: %v", gk.source, verifyErr))
				recordedSkills = nil
			} else if len(resolvedSkills) == 0 {
				o.reporter.Warning(fmt.Sprintf("installed all skills from %q, but could not resolve concrete skill names from the skill lock; future cleanup will be skipped until the source is re-applied with a readable lock", gk.source))
				recordedSkills = nil
			} else {
				recordedSkills = resolvedSkills
			}
		} else {
			var verified []string
			verifyErr := o.reporter.ProgressStep("  -> skills verify "+gk.source, func() error {
				var stepErr error
				verified, stepErr = o.skillsManager.InstalledForSource(gk.source)
				return stepErr
			})
			if verifyErr != nil {
				o.reporter.Warning(fmt.Sprintf("installed skills from %q but could not verify via skill lock: %v - recording requested names", gk.source, verifyErr))
				verifiedViaLock = false
			} else {
				verifiedSet := make(map[string]struct{}, len(verified))
				for _, s := range verified {
					verifiedSet[s] = struct{}{}
				}
				var confirmed, missing []string
				for _, s := range skills {
					if _, ok := verifiedSet[s]; ok {
						confirmed = append(confirmed, s)
					} else {
						missing = append(missing, s)
					}
				}
				if len(missing) > 0 {
					o.reporter.Warning(fmt.Sprintf("skills %v from %q were not found in the skill lock after install - they may not exist in the source", missing, gk.source))
				}
				recordedSkills = confirmed
			}
		}

		// Record each managed skill in state.
		for _, skillName := range recordedSkills {
			state.Skills = append(state.Skills, SkillState{
				Source: gk.source,
				Name:   skillName,
				Agents: agents,
			})
		}
		if allGroups[gk] {
			o.reporter.Success(fmt.Sprintf("installed all skills from %s", gk.source))
		} else if len(recordedSkills) > 0 {
			sort.Strings(recordedSkills)
			if verifiedViaLock {
				o.reporter.Success(fmt.Sprintf("installed skills %v from %s", recordedSkills, gk.source))
			} else {
				o.reporter.Success(fmt.Sprintf("installed skills %v from %s (unverified)", recordedSkills, gk.source))
			}
		}
	}
}

func (o *Orchestrator) sourceSkillsToRemove(
	source string,
	agent string,
	currentSkills map[skillID]map[string]struct{},
	currentAllSources map[string]map[string]struct{},
) ([]string, error) {
	if allAgents, hasAll := currentAllSources[source]; hasAll {
		if _, covered := allAgents[agent]; covered {
			return nil, nil
		}
	}

	installed, err := o.skillsManager.InstalledForSource(source)
	if err != nil {
		return nil, err
	}
	if len(installed) == 0 {
		return nil, nil
	}

	currentSkillNames := make(map[string]struct{})
	for id, agents := range currentSkills {
		if id.source != source || id.name == "" {
			continue
		}
		if _, exists := agents[agent]; exists {
			currentSkillNames[id.name] = struct{}{}
		}
	}

	remove := make([]string, 0, len(installed))
	for _, skillName := range installed {
		if _, exists := currentSkillNames[skillName]; exists {
			continue
		}
		remove = append(remove, skillName)
	}
	if len(remove) == 0 {
		return nil, nil
	}
	return remove, nil
}

// applyMCPs registers current MCPs and removes orphans from previousState.
func (o *Orchestrator) applyMCPs(config EffectiveAIConfig, previousState *AIState, state *AIState) {
	done := o.reporter.ProgressStart("AI MCPs")
	defer done("done", nil)

	currentMCPs := make(map[string]map[string]struct{})
	mcpConfigs := make(map[string]ResolvedMCP)

	for agent, agentCfg := range config {
		for _, mcp := range agentCfg.MCPs {
			if _, exists := currentMCPs[mcp.Name]; !exists {
				currentMCPs[mcp.Name] = make(map[string]struct{})
			}
			currentMCPs[mcp.Name][agent] = struct{}{}
			if _, exists := mcpConfigs[mcp.Name]; !exists {
				mcpConfigs[mcp.Name] = mcp
			}
		}
	}
	if previousState != nil {
		for _, prevMCP := range previousState.MCPs {
			currentAgents := currentMCPs[prevMCP.Name]
			for _, agent := range prevMCP.Agents {
				if _, exists := currentAgents[agent]; exists {
					continue
				}
				provider, ok := o.providers[agent]
				if !ok {
					o.reporter.Warning(fmt.Sprintf("no provider for agent %q, skipping orphan MCP removal for %q", agent, prevMCP.Name))
					continue
				}
				if err := o.reporter.ProgressStep("  -> mcp remove "+prevMCP.Name+" "+agent, func() error {
					return provider.RemoveMCP(prevMCP.Name)
				}); err != nil {
					o.reporter.Warning(fmt.Sprintf("failed to remove orphan MCP %q from %q: %v", prevMCP.Name, agent, err))
				} else {
					o.reporter.Success(fmt.Sprintf("removed orphan MCP %q from %s", prevMCP.Name, agent))
				}
			}
		}
	}

	// Register current MCPs. Track which agents succeed per MCP.
	mcpSuccessAgents := make(map[string][]string)

	// Sort MCP names for deterministic ordering.
	mcpNames := make([]string, 0, len(currentMCPs))
	for name := range currentMCPs {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)

	for _, mcpName := range mcpNames {
		agents := sortedSetKeys(currentMCPs[mcpName])
		mcpCfg := mcpConfigs[mcpName]

		for _, agent := range agents {
			provider, ok := o.providers[agent]
			if !ok {
				o.reporter.Warning(fmt.Sprintf("no provider for agent %q, skipping MCP registration for %q", agent, mcpName))
				continue
			}
			if err := o.reporter.ProgressStep("  -> mcp register "+mcpName+" "+agent, func() error {
				return provider.RegisterMCP(mcpCfg)
			}); err != nil {
				o.reporter.Warning(fmt.Sprintf("failed to register MCP %q for %q: %v", mcpName, agent, err))
				continue
			}
			mcpSuccessAgents[mcpName] = append(mcpSuccessAgents[mcpName], agent)
		}
	}

	// Record successful MCPs in state.
	for _, mcpName := range mcpNames {
		agents, ok := mcpSuccessAgents[mcpName]
		if !ok || len(agents) == 0 {
			continue
		}
		state.MCPs = append(state.MCPs, MCPState{
			Name:   mcpName,
			Agents: agents,
		})
	}
}

// sortedKeys returns the keys of an EffectiveAIConfig map sorted alphabetically.
func sortedKeys(m EffectiveAIConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSetKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
