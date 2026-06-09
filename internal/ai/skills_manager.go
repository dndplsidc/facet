package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
)

// NPXSkillsManager implements SkillsManager using the npx skills CLI.
type NPXSkillsManager struct {
	runner        CommandRunner
	skillLockPath string
	npxOnce       sync.Once
	npxError      error
}

// NewNPXSkillsManager constructs an NPXSkillsManager with the given CommandRunner.
func NewNPXSkillsManager(runner CommandRunner, skillLockPath string) *NPXSkillsManager {
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

	if err := m.runner.Run(parts[0], parts[1:]...); err != nil {
		return fmt.Errorf("skills install: %w", err)
	}
	return nil
}

// Remove runs: npx skills remove <s1> <s2> -a <a1> -a <a2> -y
func (m *NPXSkillsManager) Remove(skills []string, agents []string) error {
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

	if err := m.runner.Run(parts[0], parts[1:]...); err != nil {
		return fmt.Errorf("skills remove: %w", err)
	}
	return nil
}

// InstalledForSource returns the globally installed skill names tracked for the
// given source in the skills lock file. Missing lock files are treated as empty.
func (m *NPXSkillsManager) InstalledForSource(source string) ([]string, error) {
	data, err := os.ReadFile(m.skillLockPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skill lock: %w", err)
	}

	var lock struct {
		Skills map[string]struct {
			Source    string `json:"source"`
			SourceURL string `json:"sourceUrl"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse skill lock: %w", err)
	}

	skills := make([]string, 0, len(lock.Skills))
	for name, entry := range lock.Skills {
		if entry.Source == source || entry.SourceURL == source {
			skills = append(skills, name)
		}
	}
	sort.Strings(skills)
	if len(skills) == 0 {
		return nil, nil
	}
	return skills, nil
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
