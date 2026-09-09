package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"facet/cmd"
	"facet/internal/ai"
	"facet/internal/app"
	"facet/internal/common/execrunner"
	"facet/internal/common/reporter"
	"facet/internal/deploy"
	"facet/internal/packages"
	"facet/internal/pi"
	"facet/internal/profile"
)

type shellScriptRunner struct{}

func (r *shellScriptRunner) Run(command string, dir string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	// Create concrete implementations
	loader := profile.NewLoader()
	r := reporter.NewDefault()
	runner := packages.NewShellRunner()
	scriptRunner := &shellScriptRunner{}
	osName := packages.DetectOS()
	installer := packages.NewInstallerWithProgress(runner, osName, r)
	stateStore := app.NewFileStateStore()
	deployerFactory := func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
		return deploy.NewDeployer(configDir, homeDir, vars, ownedConfigs)
	}
	baseResolver := profile.NewBaseResolver(loader, execrunner.New())

	// Create AI dependencies
	aiRunner := execrunner.New()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	skillLockPath := filepath.Join(homeDir, ".agents", ".skill-lock.json")
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		skillLockPath = filepath.Join(stateHome, "skills", ".skill-lock.json")
	}
	skillsMgr := ai.NewNPXSkillsManager(
		aiRunner,
		skillLockPath,
	)
	providers := map[string]ai.AgentProvider{
		"claude-code": ai.NewClaudeCodeProvider(
			filepath.Join(homeDir, ".claude", "settings.json"),
			aiRunner,
		),
		"cursor": ai.NewCursorProvider(
			filepath.Join(homeDir, ".cursor", "cli-config.json"),
			filepath.Join(homeDir, ".cursor", "mcp.json"),
		),
		"codex": ai.NewCodexProvider(
			filepath.Join(homeDir, ".codex", "config.toml"),
			filepath.Join(homeDir, ".codex", "config.toml"),
		),
	}
	aiOrchestrator := ai.NewOrchestrator(providers, skillsMgr, r)
	piManager := pi.NewManager(aiRunner, r)

	// Create app with all dependencies
	application := app.New(app.Deps{
		Loader:          loader,
		BaseResolver:    baseResolver,
		Installer:       installer,
		Reporter:        r,
		StateStore:      stateStore,
		DeployerFactory: deployerFactory,
		AIOrchestrator:  aiOrchestrator,
		PiManager:       piManager,
		ScriptRunner:    scriptRunner,
		SkillsManager:   skillsMgr,
		Version:         "0.1.0",
		OSName:          osName,
	})

	// Build command tree and execute
	rootCmd := cmd.NewRootCmd(application, r)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
