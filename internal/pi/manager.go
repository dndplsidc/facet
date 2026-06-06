package pi

import (
	"fmt"
	"sort"
)

// CommandRunner executes commands directly without a shell.
type CommandRunner interface {
	Run(name string, args ...string) error
	RunInteractive(name string, args ...string) error
}

// Reporter emits user-facing status messages.
type Reporter interface {
	Success(msg string)
	Warning(msg string)
}

// ApplyOptions controls Pi extension reconciliation behavior.
type ApplyOptions struct {
	Force bool
}

// Manager reconciles Pi extension state.
type Manager struct {
	runner   CommandRunner
	reporter Reporter
}

func NewManager(runner CommandRunner, reporter Reporter) *Manager {
	return &Manager{runner: runner, reporter: reporter}
}

func (m *Manager) Apply(config *Config, previousState *PiState, opts ApplyOptions) (*PiState, error) {
	current := make(map[string]struct{})
	if config != nil {
		for _, ext := range config.Extensions {
			if ext == "" {
				continue
			}
			current[ext] = struct{}{}
		}
	}

	previous := make(map[string]struct{})
	if previousState != nil {
		for _, ext := range previousState.Extensions {
			previous[ext] = struct{}{}
			if _, keep := current[ext]; keep {
				continue
			}
			if err := m.runner.Run("pi", "remove", ext); err != nil {
				m.reporter.Warning(fmt.Sprintf("failed to remove Pi extension %q: %v", ext, err))
			} else {
				m.reporter.Success(fmt.Sprintf("removed Pi extension %s", ext))
			}
		}
	}

	if len(current) == 0 {
		return nil, nil
	}

	extensions := sortedKeys(current)
	state := &PiState{}
	for _, ext := range extensions {
		if _, alreadyManaged := previous[ext]; alreadyManaged && !opts.Force {
			state.Extensions = append(state.Extensions, ext)
			continue
		}
		if err := m.runner.Run("pi", "install", ext); err != nil {
			m.reporter.Warning(fmt.Sprintf("failed to install Pi extension %q: %v", ext, err))
			continue
		}
		m.reporter.Success(fmt.Sprintf("installed Pi extension %s", ext))
		state.Extensions = append(state.Extensions, ext)
	}
	if len(state.Extensions) == 0 {
		return nil, nil
	}
	return state, nil
}

func (m *Manager) Unapply(previousState *PiState) error {
	if previousState == nil {
		return nil
	}
	for _, ext := range previousState.Extensions {
		if err := m.runner.Run("pi", "remove", ext); err != nil {
			m.reporter.Warning(fmt.Sprintf("failed to remove Pi extension %q: %v", ext, err))
			continue
		}
		m.reporter.Success(fmt.Sprintf("removed Pi extension %s", ext))
	}
	return nil
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
