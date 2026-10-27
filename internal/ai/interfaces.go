package ai

// AgentProvider handles agent-specific file I/O for permissions and MCPs.
type AgentProvider interface {
	Name() string
	ApplyPermissions(permissions ResolvedPermissions) error
	RemovePermissions(previousPermissions ResolvedPermissions) error
	RegisterMCP(mcp ResolvedMCP) error
	RemoveMCP(name string) error
	SettingsFilePath() string
}

// SkillInstallResult contains the current source inventory only when an
// all-source install reported complete, authoritative results. For all-source
// installs, nil Names means the CLI does not support an authoritative inventory.
type SkillInstallResult struct {
	Names []string
}

// SkillsManager manages skill installation/removal via external CLI.
type SkillsManager interface {
	Install(source string, skills []string, agents []string) (SkillInstallResult, error)
	Remove(skills []string, agents []string) error
	InstalledForSource(source string) ([]string, error)
	TrackedForSource(source string) ([]string, error)
	Check() error
	Update() error
}

// CommandRunner executes a command name with a stable argv vector. It is
// defined here (not imported from packages/) to avoid cross-domain coupling.
type CommandRunner interface {
	Run(name string, args ...string) error
	RunInteractive(name string, args ...string) error
}
