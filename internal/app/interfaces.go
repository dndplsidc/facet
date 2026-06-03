package app

import (
	"time"

	"facet/internal/ai"
	"facet/internal/deploy"
	"facet/internal/packages"
	"facet/internal/pi"
	"facet/internal/profile"
)

// ProfileLoader loads and parses facet configuration files.
type ProfileLoader interface {
	LoadMeta(configDir string) (*profile.FacetMeta, error)
	LoadConfig(path string) (*profile.FacetConfig, error)
}

type BaseResolver interface {
	Resolve(rawExtends string, localConfigDir string) (*profile.ResolvedBase, error)
}

// Reporter handles formatted terminal output.
type Reporter interface {
	Success(msg string)
	Warning(msg string)
	Error(msg string)
	Header(msg string)
	PrintLine(msg string)
	Dim(text string) string
	Progress(msg string)
	ProgressDuration(label, outcome string, elapsed time.Duration, err error)
	ProgressStart(label string) func(outcome string, err error)
	ProgressStep(label string, fn func() error) error
}

// StateStore handles reading and writing apply state.
type StateStore interface {
	Read(stateDir string) (*ApplyState, error)
	Write(stateDir string, s *ApplyState) error
	CanaryWrite(stateDir string) error
}

// Installer handles package installation.
type Installer interface {
	InstallAll(pkgs []profile.PackageEntry) []packages.PackageResult
}

// ScriptRunner executes shell commands for pre_apply and post_apply scripts.
type ScriptRunner interface {
	Run(command string, dir string) error
}

// DeployerFactory creates a deploy.Service for a given configuration.
type DeployerFactory func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service

// AIOrchestrator handles AI agent configuration lifecycle.
type AIOrchestrator interface {
	Apply(config ai.EffectiveAIConfig, previousState *ai.AIState) (*ai.AIState, error)
	Unapply(previousState *ai.AIState) error
}

// PiManager handles Pi extension lifecycle.
type PiManager interface {
	Apply(config *pi.Config, previousState *pi.PiState, opts pi.ApplyOptions) (*pi.PiState, error)
	Unapply(previousState *pi.PiState) error
}

// SkillsManager handles interactive skill check and update operations.
type SkillsManager interface {
	Check() error
	Update() error
}
