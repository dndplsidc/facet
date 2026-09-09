package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"sync"
)

// SkillsCommandRunner supports machine-readable CLI inventory as well as commands.
type SkillsCommandRunner interface {
	CommandRunner
	Output(name string, args ...string) ([]byte, error)
	RunInDir(dir string, name string, args ...string) error
}

// Claude Code and Pi have native skill directories. Other supported agents
// consume shared storage; unknown agents conservatively keep that storage.
func nativeSkillAgent(agent string) bool { return agent == "claude-code" || agent == "pi" }

type skillLockEntry struct {
	Source    string `json:"source"`
	SourceURL string `json:"sourceUrl"`
}

// NPXSkillsManager implements SkillsManager using the npx skills CLI.
type NPXSkillsManager struct {
	runner        SkillsCommandRunner
	skillLockPath string
	npxOnce       sync.Once
	npxError      error
}

// NewNPXSkillsManager constructs an NPXSkillsManager with the given CommandRunner.
func NewNPXSkillsManager(runner SkillsCommandRunner, skillLockPath string) *NPXSkillsManager {
	return &NPXSkillsManager{
		runner:        runner,
		skillLockPath: skillLockPath,
	}
}

// checkNPX verifies that npx is available on PATH (called lazily via sync.Once).
func (m *NPXSkillsManager) checkNPX() error {
	m.npxOnce.Do(func() {
		if err := m.runner.Run("npx", "--version"); err != nil {
			m.npxError = fmt.Errorf("npx not found on PATH: %w", err)
		}
	})
	return m.npxError
}

// Install runs: npx skills add <source> --skill <s1> --skill <s2> -a <a1> -a <a2> -y
// When skills is empty, passes --skill * to request all skills while preserving explicit agent scoping.
func (m *NPXSkillsManager) Install(source string, skills []string, agents []string) error {
	if err := m.checkNPX(); err != nil {
		return err
	}

	var parts []string
	parts = append(parts, "npx", "skills", "add", source)
	if len(skills) == 0 {
		parts = append(parts, "--skill", "*")
	} else {
		for _, s := range skills {
			parts = append(parts, "--skill", s)
		}
	}
	for _, a := range agents {
		parts = append(parts, "-a", a)
	}
	parts = append(parts, "-g", "-y")
	if len(agents) > 0 {
		nativeOnly := true
		for _, agent := range agents {
			if !nativeSkillAgent(agent) {
				nativeOnly = false
			}
		}
		if nativeOnly {
			parts = append(parts, "--copy")
		}
	}

	if err := m.runner.Run(parts[0], parts[1:]...); err != nil {
		return fmt.Errorf("skills install: %w", err)
	}
	return nil
}

// Remove runs global removal, omitting -a to remove a skill across all agents.
// An isolated working directory protects project-local skills from CLI fallback
// paths used by agents that have no global skill directory.
func (m *NPXSkillsManager) Remove(skills []string, agents []string) error {
	if len(skills) == 0 {
		return nil
	}
	if err := m.checkNPX(); err != nil {
		return err
	}

	var parts []string
	parts = append(parts, "npx", "skills", "remove")
	parts = append(parts, skills...)
	for _, a := range agents {
		parts = append(parts, "-a", a)
	}
	parts = append(parts, "-g", "-y")

	dir, err := os.MkdirTemp("", "facet-skills-remove-")
	if err != nil {
		return fmt.Errorf("create skills removal directory: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := m.runner.RunInDir(dir, parts[0], parts[1:]...); err != nil {
		return fmt.Errorf("skills remove: %w", err)
	}
	if len(agents) == 0 {
		lock, err := m.readLock()
		if err != nil {
			return err
		}
		installed, err := m.installedNames()
		if err != nil {
			return err
		}
		for _, name := range skills {
			if _, exists := lock[name]; exists {
				return fmt.Errorf("skills remove: %q remains in the skill lock", name)
			}
			if slices.Contains(installed, name) {
				return fmt.Errorf("skills remove: %q remains installed", name)
			}
		}
	}
	return nil
}

// InstalledForSource returns the globally installed skill names tracked for the
// given source in the skills lock file. Missing lock files are treated as empty.
func (m *NPXSkillsManager) InstalledForSource(source string) ([]string, error) {
	names, err := m.TrackedForSource(source)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	installed, err := m.installedNames()
	if err != nil {
		return nil, err
	}
	var result []string
	for _, name := range names {
		if slices.Contains(installed, name) {
			result = append(result, name)
		}
	}
	return result, nil
}

// TrackedForSource includes lock-only entries so legacy all-source state can
// still clean up stale metadata through the CLI.
func (m *NPXSkillsManager) TrackedForSource(source string) ([]string, error) {
	lock, err := m.readLock()
	if err != nil {
		return nil, err
	}
	var names []string
	for name, entry := range lock {
		if entry.Source == source || entry.SourceURL == source {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)
	return names, nil
}

func (m *NPXSkillsManager) installedNames() ([]string, error) {
	if err := m.checkNPX(); err != nil {
		return nil, err
	}
	data, err := m.runner.Output("npx", "skills", "list", "-g", "--json")
	if err != nil {
		return nil, fmt.Errorf("list installed skills: %w", err)
	}
	var inventory []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		return nil, fmt.Errorf("parse installed skills: %w", err)
	}
	names := make([]string, 0, len(inventory))
	for _, skill := range inventory {
		names = append(names, skill.Name)
	}
	return names, nil
}

func (m *NPXSkillsManager) readLock() (map[string]skillLockEntry, error) {
	data, err := os.ReadFile(m.skillLockPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skill lock: %w", err)
	}

	var lock struct {
		Skills map[string]skillLockEntry `json:"skills"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse skill lock: %w", err)
	}

	return lock.Skills, nil
}

// Check runs: npx skills check (interactive, streams output to terminal).
func (m *NPXSkillsManager) Check() error {
	if err := m.checkNPX(); err != nil {
		return err
	}
	if err := m.runner.RunInteractive("npx", "skills", "check"); err != nil {
		return fmt.Errorf("skills check: %w", err)
	}
	return nil
}

// Update runs: npx skills update (interactive, streams output to terminal).
func (m *NPXSkillsManager) Update() error {
	if err := m.checkNPX(); err != nil {
		return err
	}
	if err := m.runner.RunInteractive("npx", "skills", "update"); err != nil {
		return fmt.Errorf("skills update: %w", err)
	}
	return nil
}
