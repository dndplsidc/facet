package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"facet/internal/ai"
	"facet/internal/deploy"
	"facet/internal/packages"
	"facet/internal/pi"
	"facet/internal/profile"
)

type mockLoader struct {
	meta    *profile.FacetMeta
	configs map[string]*profile.FacetConfig
	err     error
}

func (m *mockLoader) LoadMeta(configDir string) (*profile.FacetMeta, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.meta, nil
}

func (m *mockLoader) LoadConfig(path string) (*profile.FacetConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	cfg, ok := m.configs[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return cfg, nil
}

type mockInstaller struct {
	results []packages.PackageResult
}

func (m *mockInstaller) InstallAll(pkgs []profile.PackageEntry) []packages.PackageResult {
	return m.results
}

type mockDeployer struct {
	deployed []deploy.ConfigResult
	sources  []deploy.SourceSpec
	unapply  [][]deploy.ConfigResult
	rollback int
	err      error
}

func (m *mockDeployer) DeployOne(targetPath string, source deploy.SourceSpec, force bool) (deploy.ConfigResult, error) {
	m.sources = append(m.sources, source)
	strategy := deploy.StrategySymlink
	if source.Materialize {
		strategy = deploy.StrategyCopy
	}
	r := deploy.ConfigResult{
		Target:     targetPath,
		Source:     source.DisplaySource,
		SourcePath: source.ResolvedPath,
		Strategy:   strategy,
	}
	if m.err != nil {
		return r, m.err
	}
	m.deployed = append(m.deployed, r)
	return r, nil
}

func (m *mockDeployer) Unapply(configs []deploy.ConfigResult) error {
	m.unapply = append(m.unapply, configs)
	return nil
}
func (m *mockDeployer) Rollback() error {
	m.rollback++
	return nil
}
func (m *mockDeployer) Deployed() []deploy.ConfigResult { return m.deployed }

type mockBaseResolver struct {
	result     *profile.ResolvedBase
	err        error
	rawExtends []string
	configDirs []string
	cleanedUp  bool
}

func (m *mockBaseResolver) Resolve(rawExtends string, localConfigDir string) (*profile.ResolvedBase, error) {
	m.rawExtends = append(m.rawExtends, rawExtends)
	m.configDirs = append(m.configDirs, localConfigDir)
	if m.err != nil {
		return nil, m.err
	}
	if m.result == nil {
		return &profile.ResolvedBase{
			Config:  &profile.FacetConfig{},
			Cleanup: func() error { return nil },
		}, nil
	}
	if m.result.Cleanup == nil {
		m.result.Cleanup = func() error {
			m.cleanedUp = true
			return nil
		}
	}
	return m.result, nil
}

func newStaticBaseResolver(cfg *profile.FacetConfig) *mockBaseResolver {
	return &mockBaseResolver{
		result: &profile.ResolvedBase{
			Config: cfg,
			Cleanup: func() error {
				return nil
			},
		},
	}
}

func TestApply_DryRun_NoSideEffects(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	// Create config source files so DetectStrategy works
	os.MkdirAll(filepath.Join(cfgDir, "configs"), 0o755)
	os.WriteFile(filepath.Join(cfgDir, "configs", ".zshrc"), []byte("# zshrc"), 0o644)

	// Create .local.yaml
	os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644)

	r := &mockReporter{}
	baseCfg := &profile.FacetConfig{
		Packages: []profile.PackageEntry{
			{Name: "ripgrep", Install: profile.InstallCmd{Command: "brew install ripgrep"}},
		},
		Configs: map[string]string{
			filepath.Join(stateDir, ".zshrc"): "configs/.zshrc",
		},
	}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	installerCalled := false
	mockInst := &mockInstaller{}
	origInstall := mockInst.InstallAll
	_ = origInstall
	// Use a tracking installer to prove it's never called
	trackingInstaller := &trackingInstaller{called: &installerCalled}

	mockDep := &mockDeployer{}
	a := New(Deps{
		Reporter:     r,
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    trackingInstaller,
		StateStore:   &mockStateStore{},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
		DryRun:    true,
	})
	assert.NoError(t, err)

	// Installer should never be called
	assert.False(t, installerCalled, "installer should not be called during dry run")

	// Deployer should never be called
	assert.Empty(t, mockDep.deployed, "deployer should not be called during dry run")

	// Reporter should show dry-run header
	foundDryRun := false
	for _, msg := range r.messages {
		if contains(msg, "Dry run") {
			foundDryRun = true
		}
	}
	assert.True(t, foundDryRun, "should print dry-run header")
}

type trackingInstaller struct {
	called *bool
}

func (t *trackingInstaller) InstallAll(pkgs []profile.PackageEntry) []packages.PackageResult {
	*t.called = true
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestApply_MissingFacetYaml(t *testing.T) {
	r := &mockReporter{}
	loader := &mockLoader{err: fmt.Errorf("file not found")}

	a := New(Deps{
		Reporter: r,
		Loader:   loader,
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: "/fake", StateDir: t.TempDir()})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "facet config directory")
}

type mockAIOrchestrator struct {
	applyCalled   bool
	applyConfig   ai.EffectiveAIConfig
	applyPrev     *ai.AIState
	applyResult   *ai.AIState
	applyErr      error
	unapplyCalled bool
	unapplyPrev   *ai.AIState
	unapplyErr    error
}

func (m *mockAIOrchestrator) Apply(config ai.EffectiveAIConfig, previousState *ai.AIState) (*ai.AIState, error) {
	m.applyCalled = true
	m.applyConfig = config
	m.applyPrev = previousState
	return m.applyResult, m.applyErr
}

func (m *mockAIOrchestrator) Unapply(previousState *ai.AIState) error {
	m.unapplyCalled = true
	m.unapplyPrev = previousState
	return m.unapplyErr
}

type mockPiManager struct {
	applyCalled   bool
	applyConfig   *pi.Config
	applyPrev     *pi.PiState
	applyResult   *pi.PiState
	applyErr      error
	applyOpts     pi.ApplyOptions
	unapplyCalled bool
	unapplyPrev   *pi.PiState
	unapplyErr    error
}

func (m *mockPiManager) Apply(config *pi.Config, previousState *pi.PiState, opts pi.ApplyOptions) (*pi.PiState, error) {
	m.applyCalled = true
	m.applyConfig = config
	m.applyPrev = previousState
	m.applyOpts = opts
	return m.applyResult, m.applyErr
}

func (m *mockPiManager) Unapply(previousState *pi.PiState) error {
	m.unapplyCalled = true
	m.unapplyPrev = previousState
	return m.unapplyErr
}

func TestApply_WithPiExtensions(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	piMgr := &mockPiManager{applyResult: &pi.PiState{Extensions: []string{"pi-lens"}}}
	stateStore := &mockStateStore{}
	baseCfg := &profile.FacetConfig{}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				AI: &profile.AIConfig{Pi: &profile.PiConfig{Extensions: []profile.PiExtensionEntry{{
					Source:     "pi-lens",
					InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"},
				}}}},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   stateStore,
		PiManager:    piMgr,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return &mockDeployer{}
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir})
	require.NoError(t, err)
	assert.True(t, piMgr.applyCalled)
	require.NotNil(t, piMgr.applyConfig)
	assert.Equal(t, []pi.ExtensionEntry{{
		Source:     "pi-lens",
		InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"},
	}}, piMgr.applyConfig.Extensions)
	require.NotNil(t, stateStore.written.AI)
	require.NotNil(t, stateStore.written.AI.Pi)
	assert.Equal(t, []string{"pi-lens"}, stateStore.written.AI.Pi.Extensions)
	assert.False(t, piMgr.applyOpts.Force)
}

func TestApply_ForcePassesPiForceOption(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	piMgr := &mockPiManager{applyResult: &pi.PiState{Extensions: []string{"pi-lens"}}}
	stateStore := &mockStateStore{}
	baseCfg := &profile.FacetConfig{}
	loader := &mockLoader{meta: &profile.FacetMeta{}, configs: map[string]*profile.FacetConfig{
		filepath.Join(cfgDir, "base.yaml"): baseCfg,
		filepath.Join(cfgDir, "profiles", "work.yaml"): {
			Extends: "base",
			AI:      &profile.AIConfig{Pi: &profile.PiConfig{Extensions: []profile.PiExtensionEntry{{Source: "pi-lens"}}}},
		},
		filepath.Join(stateDir, ".local.yaml"): {},
	}}

	a := New(Deps{Reporter: &mockReporter{}, Loader: loader, BaseResolver: newStaticBaseResolver(baseCfg), Installer: &mockInstaller{}, StateStore: stateStore, PiManager: piMgr, DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
		return &mockDeployer{}
	}, OSName: "macos"})

	require.NoError(t, a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir, Force: true}))
	assert.True(t, piMgr.applyCalled)
	assert.True(t, piMgr.applyOpts.Force)
}

func TestApply_SameProfileRemovalOfPiSectionReconcilesPreviousState(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	piMgr := &mockPiManager{}
	stateStore := &mockStateStore{state: &ApplyState{Profile: "work", AI: &ai.AIState{Pi: &pi.PiState{Extensions: []string{"pi-lens"}}}}}
	baseCfg := &profile.FacetConfig{}
	loader := &mockLoader{meta: &profile.FacetMeta{}, configs: map[string]*profile.FacetConfig{
		filepath.Join(cfgDir, "base.yaml"):             baseCfg,
		filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "base"},
		filepath.Join(stateDir, ".local.yaml"):         {},
	}}

	a := New(Deps{Reporter: &mockReporter{}, Loader: loader, BaseResolver: newStaticBaseResolver(baseCfg), Installer: &mockInstaller{}, StateStore: stateStore, PiManager: piMgr, DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
		return &mockDeployer{}
	}, OSName: "macos"})
	require.NoError(t, a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir}))
	assert.True(t, piMgr.applyCalled)
	assert.Nil(t, piMgr.applyConfig)
	assert.Nil(t, stateStore.written.AI)
}

func TestApply_ProfileSwitchTriggersPiUnapply(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	prevPi := &pi.PiState{Extensions: []string{"pi-lens"}}
	piMgr := &mockPiManager{}
	stateStore := &mockStateStore{state: &ApplyState{Profile: "work", AI: &ai.AIState{Pi: prevPi}}}
	baseCfg := &profile.FacetConfig{}
	loader := &mockLoader{meta: &profile.FacetMeta{}, configs: map[string]*profile.FacetConfig{
		filepath.Join(cfgDir, "base.yaml"): baseCfg,
		filepath.Join(cfgDir, "profiles", "personal.yaml"): {
			Extends: "base",
			AI:      &profile.AIConfig{Pi: &profile.PiConfig{Extensions: []profile.PiExtensionEntry{{Source: "pi-subagents"}}}},
		},
		filepath.Join(stateDir, ".local.yaml"): {},
	}}

	a := New(Deps{Reporter: &mockReporter{}, Loader: loader, BaseResolver: newStaticBaseResolver(baseCfg), Installer: &mockInstaller{}, StateStore: stateStore, PiManager: piMgr, DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
		return &mockDeployer{}
	}, OSName: "macos"})
	require.NoError(t, a.Apply("personal", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir}))
	assert.True(t, piMgr.unapplyCalled)
	assert.Equal(t, prevPi, piMgr.unapplyPrev)
	assert.True(t, piMgr.applyCalled)
	assert.Nil(t, piMgr.applyPrev)
}

func TestApply_StagesPackagesPreservesPreviousPiState(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	prevPi := &pi.PiState{Extensions: []string{"pi-lens"}}
	piMgr := &mockPiManager{}
	stateStore := &mockStateStore{state: &ApplyState{Profile: "work", AI: &ai.AIState{Pi: prevPi}}}
	baseCfg := &profile.FacetConfig{Packages: []profile.PackageEntry{{Name: "git", Install: profile.InstallCmd{Command: "true"}}}}
	loader := &mockLoader{meta: &profile.FacetMeta{}, configs: map[string]*profile.FacetConfig{
		filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "base"},
		filepath.Join(stateDir, ".local.yaml"):         {},
	}}

	installerCalled := false
	a := New(Deps{Reporter: &mockReporter{}, Loader: loader, BaseResolver: newStaticBaseResolver(baseCfg), Installer: &trackingInstaller{called: &installerCalled}, StateStore: stateStore, PiManager: piMgr, DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
		return &mockDeployer{}
	}, OSName: "macos"})
	require.NoError(t, a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir, Stages: "packages"}))
	assert.True(t, installerCalled)
	assert.False(t, piMgr.applyCalled)
	require.NotNil(t, stateStore.written.AI)
	assert.Equal(t, prevPi, stateStore.written.AI.Pi)
}

func TestApply_WithAI(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	// Create .local.yaml
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	r := &mockReporter{}
	aiOrch := &mockAIOrchestrator{
		applyResult: &ai.AIState{
			Permissions: map[string]ai.PermissionState{
				"claude-code": {Allow: []string{"Read"}, Deny: []string{"Execute"}},
			},
		},
	}
	mockDep := &mockDeployer{}
	stateStore := &mockStateStore{}
	baseCfg := &profile.FacetConfig{}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				AI: &profile.AIConfig{
					Agents: []string{"claude-code"},
					Permissions: map[string]*profile.PermissionsConfig{
						"claude-code": {
							Allow: []string{"Read"},
							Deny:  []string{"Execute"},
						},
					},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:       r,
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &mockInstaller{},
		StateStore:     stateStore,
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)

	// AI orchestrator should have been called
	assert.True(t, aiOrch.applyCalled, "AI orchestrator Apply should be called")
	assert.Contains(t, aiOrch.applyConfig, "claude-code")

	// State should include AI data
	require.NotNil(t, stateStore.written)
	require.NotNil(t, stateStore.written.AI)
	assert.Equal(t, []string{"Read"}, stateStore.written.AI.Permissions["claude-code"].Allow)
}

func TestApply_WithInheritedAIAgentsFromBase(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	aiOrch := &mockAIOrchestrator{}
	stateStore := &mockStateStore{}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{
		AI: &profile.AIConfig{
			Agents: []string{"claude-code"},
			Permissions: map[string]*profile.PermissionsConfig{
				"claude-code": {
					Allow: []string{"Read"},
				},
			},
		},
	}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				AI: &profile.AIConfig{
					Permissions: map[string]*profile.PermissionsConfig{
						"claude-code": {
							Allow: []string{"Read", "Edit"},
						},
					},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:       &mockReporter{},
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &mockInstaller{},
		StateStore:     stateStore,
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)
	assert.True(t, aiOrch.applyCalled, "AI orchestrator should be called for inherited agents")
}

func TestApply_WithoutAI(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	// Create .local.yaml
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	r := &mockReporter{}
	aiOrch := &mockAIOrchestrator{}
	mockDep := &mockDeployer{}
	stateStore := &mockStateStore{}
	baseCfg := &profile.FacetConfig{}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"):             baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "base"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:       r,
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &mockInstaller{},
		StateStore:     stateStore,
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)

	// AI orchestrator should NOT have been called (no AI config)
	assert.False(t, aiOrch.applyCalled, "AI orchestrator Apply should not be called when no AI config")

	// State should not include AI data
	require.NotNil(t, stateStore.written)
	assert.Nil(t, stateStore.written.AI)
}

func TestApply_SameProfileRemovalOfAISectionReconcilesPreviousState(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	aiOrch := &mockAIOrchestrator{}
	stateStore := &mockStateStore{
		state: &ApplyState{
			Profile: "work",
			AI: &ai.AIState{
				Permissions: map[string]ai.PermissionState{
					"claude-code": {Allow: []string{"Read"}},
				},
				Skills: []ai.SkillState{
					{Source: "@org/skills", Name: "code-review", Agents: []string{"claude-code"}},
				},
				MCPs: []ai.MCPState{
					{Name: "playwright", Agents: []string{"claude-code"}},
				},
			},
		},
	}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"):             baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "base"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:       &mockReporter{},
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &mockInstaller{},
		StateStore:     stateStore,
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)
	assert.True(t, aiOrch.applyCalled, "AI orchestrator should reconcile previous AI state on same-profile removal")
	assert.Nil(t, aiOrch.applyConfig, "current AI config should be nil when AI section is removed")
	require.NotNil(t, stateStore.written)
	assert.Nil(t, stateStore.written.AI, "written state should omit AI after full removal")
}

func TestApply_ProfileSwitchTriggersAIUnapply(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	prevAIState := &ai.AIState{
		Permissions: map[string]ai.PermissionState{
			"claude-code": {Allow: []string{"Read"}, Deny: []string{"Execute"}},
		},
		Skills: []ai.SkillState{
			{Source: "@org/skills", Name: "code-review", Agents: []string{"claude-code"}},
		},
		MCPs: []ai.MCPState{
			{Name: "playwright", Agents: []string{"claude-code"}},
		},
	}

	aiOrch := &mockAIOrchestrator{}
	stateStore := &mockStateStore{
		state: &ApplyState{
			Profile: "work",
			AI:      prevAIState,
		},
	}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "personal.yaml"): {
				Extends: "base",
				AI: &profile.AIConfig{
					Agents: []string{"claude-code"},
					Permissions: map[string]*profile.PermissionsConfig{
						"claude-code": {
							Allow: []string{"Read"},
						},
					},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:       &mockReporter{},
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &mockInstaller{},
		StateStore:     stateStore,
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("personal", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)

	assert.True(t, aiOrch.unapplyCalled, "AI orchestrator Unapply should be called on profile switch")
	assert.Equal(t, prevAIState, aiOrch.unapplyPrev, "Unapply should receive the previous AI state")
	assert.True(t, aiOrch.applyCalled, "AI orchestrator Apply should still be called for the new profile")
	assert.Nil(t, aiOrch.applyPrev, "Apply should not diff against previous AI state after full unapply")

	// If unapply fails, reconciliation must retain the old ownership records.
	stateStore.state = &ApplyState{Profile: "work", AI: prevAIState}
	aiOrch.unapplyErr = fmt.Errorf("skill cleanup failed")
	require.NoError(t, a.Apply("personal", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir}))
	assert.Equal(t, prevAIState, aiOrch.applyPrev, "failed unapply must remain available for cleanup retry")
}

func TestApply_ForceTriggersAIUnapply(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	prevAIState := &ai.AIState{
		Permissions: map[string]ai.PermissionState{
			"claude-code": {Allow: []string{"Read"}, Deny: []string{"Execute"}},
		},
		Skills: []ai.SkillState{
			{Source: "@org/skills", Name: "code-review", Agents: []string{"claude-code"}},
		},
		MCPs: []ai.MCPState{
			{Name: "playwright", Agents: []string{"claude-code"}},
		},
	}

	aiOrch := &mockAIOrchestrator{}
	stateStore := &mockStateStore{
		state: &ApplyState{
			Profile: "work",
			AI:      prevAIState,
		},
	}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				AI: &profile.AIConfig{
					Agents: []string{"claude-code"},
					Permissions: map[string]*profile.PermissionsConfig{
						"claude-code": {
							Allow: []string{"Read"},
						},
					},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:       &mockReporter{},
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &mockInstaller{},
		StateStore:     stateStore,
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
		Force:     true,
	})
	require.NoError(t, err)

	assert.True(t, aiOrch.unapplyCalled, "AI orchestrator Unapply should be called on force apply")
	assert.True(t, aiOrch.applyCalled, "AI orchestrator Apply should still be called after force unapply")
	assert.Nil(t, aiOrch.applyPrev, "Apply should not diff against previous AI state after force unapply")
}

type mockScriptRunner struct {
	commands []string
	dirs     []string
	failOn   map[string]error
}

func (m *mockScriptRunner) Run(command string, dir string) error {
	m.commands = append(m.commands, command)
	m.dirs = append(m.dirs, dir)
	if err, ok := m.failOn[command]; ok {
		return err
	}
	return nil
}

func TestApply_RunsPreAndPostApplyScripts(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	runner := &mockScriptRunner{}
	mockDep := &mockDeployer{}
	stateStore := &mockStateStore{}
	baseCfg := &profile.FacetConfig{
		PreApply: []profile.ScriptEntry{
			{Name: "base-pre", Run: "echo base-pre"},
		},
		PostApply: []profile.ScriptEntry{
			{Name: "base-post", Run: "echo base-post"},
		},
	}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				PreApply: []profile.ScriptEntry{
					{Name: "profile-pre", Run: "echo profile-pre"},
				},
				PostApply: []profile.ScriptEntry{
					{Name: "profile-post", Run: "echo profile-post"},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   stateStore,
		ScriptRunner: runner,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)

	require.Len(t, runner.commands, 4)
	assert.Equal(t, "echo base-pre", runner.commands[0])
	assert.Equal(t, "echo profile-pre", runner.commands[1])
	assert.Equal(t, "echo base-post", runner.commands[2])
	assert.Equal(t, "echo profile-post", runner.commands[3])
}

func TestApply_PreApplyScriptFailureHaltsApply(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	runner := &mockScriptRunner{
		failOn: map[string]error{
			"echo fail": fmt.Errorf("exit status 1"),
		},
	}
	installerCalled := false
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{
		PreApply: []profile.ScriptEntry{
			{Name: "will-fail", Run: "echo fail"},
		},
	}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"):             baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "base"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &trackingInstaller{called: &installerCalled},
		StateStore:   &mockStateStore{},
		ScriptRunner: runner,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "will-fail")
	assert.False(t, installerCalled, "installer should not run after pre_apply failure")
}

func TestApply_StagesFiltering(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	runner := &mockScriptRunner{}
	installerCalled := false
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{
		PreApply: []profile.ScriptEntry{
			{Name: "pre", Run: "echo pre"},
		},
		PostApply: []profile.ScriptEntry{
			{Name: "post", Run: "echo post"},
		},
		Packages: []profile.PackageEntry{
			{Name: "git", Install: profile.InstallCmd{Command: "brew install git"}},
		},
	}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"):             baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "base"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &trackingInstaller{called: &installerCalled},
		StateStore:   &mockStateStore{},
		ScriptRunner: runner,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
		Stages:    "packages",
	})
	require.NoError(t, err)

	assert.True(t, installerCalled, "installer should run when packages stage is selected")
	assert.Empty(t, runner.commands, "scripts should not run when not in stages")
	assert.Empty(t, mockDep.deployed, "configs should not deploy when not in stages")
}

func TestApply_StagesPackages_DoesNotUnapplyOrCarryStaleState(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	prevConfigs := []deploy.ConfigResult{{Target: filepath.Join(t.TempDir(), ".gitconfig"), Source: "configs/.gitconfig", Strategy: deploy.StrategySymlink}}
	prevAIState := &ai.AIState{
		Permissions: map[string]ai.PermissionState{
			"claude-code": {Allow: []string{"Read"}},
		},
	}
	mockDep := &mockDeployer{}
	stateStore := &mockStateStore{
		state: &ApplyState{
			Profile: "other",
			Configs: prevConfigs,
			AI:      prevAIState,
		},
	}
	aiOrch := &mockAIOrchestrator{}
	baseCfg := &profile.FacetConfig{
		Packages: []profile.PackageEntry{
			{Name: "git", Install: profile.InstallCmd{Command: "brew install git"}},
		},
	}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "file:///tmp/base.git"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	installerCalled := false
	a := New(Deps{
		Reporter:       &mockReporter{},
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &trackingInstaller{called: &installerCalled},
		StateStore:     stateStore,
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir, Stages: "packages"})
	require.NoError(t, err)

	assert.True(t, installerCalled)
	assert.Empty(t, mockDep.unapply)
	assert.False(t, aiOrch.unapplyCalled)
	assert.Equal(t, "other", stateStore.written.Profile)
	assert.Equal(t, prevConfigs, stateStore.written.Configs)
	assert.Equal(t, prevAIState, stateStore.written.AI)
}

func TestApply_SameProfileStagesPackages_DoesNotCleanupOrphans(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	prevConfigs := []deploy.ConfigResult{{Target: filepath.Join(t.TempDir(), ".gitconfig"), Source: "configs/.gitconfig", Strategy: deploy.StrategySymlink}}
	mockDep := &mockDeployer{}
	stateStore := &mockStateStore{
		state: &ApplyState{
			Profile: "work",
			Configs: prevConfigs,
		},
	}
	baseCfg := &profile.FacetConfig{
		Packages: []profile.PackageEntry{
			{Name: "git", Install: profile.InstallCmd{Command: "brew install git"}},
		},
	}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "file:///tmp/base.git"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	installerCalled := false
	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &trackingInstaller{called: &installerCalled},
		StateStore:   stateStore,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir, Stages: "packages"})
	require.NoError(t, err)

	assert.True(t, installerCalled)
	assert.Empty(t, mockDep.unapply)
	assert.Equal(t, "work", stateStore.written.Profile)
	assert.Equal(t, prevConfigs, stateStore.written.Configs)
}

func TestApply_SameProfileSkipFailure_PreservesPreviousConfigState(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	target := filepath.Join(t.TempDir(), ".gitconfig")
	prevConfigs := []deploy.ConfigResult{{
		Target:     target,
		Source:     "configs/.gitconfig",
		SourcePath: filepath.Join(cfgDir, "configs", ".gitconfig"),
		Strategy:   deploy.StrategySymlink,
	}}
	mockDep := &mockDeployer{err: fmt.Errorf("deploy failed")}
	stateStore := &mockStateStore{
		state: &ApplyState{
			Profile: "work",
			Configs: prevConfigs,
		},
	}
	baseCfg := &profile.FacetConfig{
		Configs: map[string]string{
			target: "configs/.gitconfig",
		},
	}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "file:///tmp/base.git"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   stateStore,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir, SkipFailure: true})
	require.NoError(t, err)
	assert.Equal(t, prevConfigs, stateStore.written.Configs)
}

func TestApply_EmitsRollbackTiming_OnConfigFailure(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	target := filepath.Join(stateDir, ".gitconfig")
	r := &mockReporter{}
	mockDep := &mockDeployer{err: fmt.Errorf("deploy failed")}
	baseCfg := &profile.FacetConfig{
		Configs: map[string]string{
			target: "configs/.gitconfig",
		},
	}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "file:///tmp/base.git"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:     r,
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   &mockStateStore{},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir})
	require.Error(t, err)

	progressOutput := strings.Join(r.messages, "\n")
	assert.Contains(t, progressOutput, "progress:   -> "+target+" ... failed")
	assert.Contains(t, progressOutput, "progress: Rolling back deployed configs ... ok")
	assert.Equal(t, 1, mockDep.rollback)
}

func TestApply_InvalidStageReturnsError(t *testing.T) {
	a := New(Deps{
		Reporter: &mockReporter{},
		Loader: &mockLoader{
			meta: &profile.FacetMeta{},
		},
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: "/fake",
		StateDir:  "/fake",
		Stages:    "invalid_stage",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown stage")
}

func TestApply_ScriptsResolveVariables(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	runner := &mockScriptRunner{}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{
		Vars: map[string]any{
			"git": map[string]any{"email": "sarah@acme.com"},
		},
		PreApply: []profile.ScriptEntry{
			{Name: "setup", Run: `echo "${facet:git.email}"`},
		},
	}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"):             baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "base"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   &mockStateStore{},
		ScriptRunner: runner,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)

	require.Len(t, runner.commands, 1)
	assert.Equal(t, `echo "sarah@acme.com"`, runner.commands[0])
}

func TestApply_DryRunSkipsAIOrchestrator(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	aiOrch := &mockAIOrchestrator{}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{}

	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				AI: &profile.AIConfig{
					Agents: []string{"claude-code"},
					Permissions: map[string]*profile.PermissionsConfig{
						"claude-code": {
							Allow: []string{"Read", "Edit"},
							Deny:  []string{"Execute"},
						},
					},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:       &mockReporter{},
		Loader:         loader,
		BaseResolver:   newStaticBaseResolver(baseCfg),
		Installer:      &mockInstaller{},
		StateStore:     &mockStateStore{},
		AIOrchestrator: aiOrch,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
		DryRun:    true,
	})
	require.NoError(t, err)

	assert.False(t, aiOrch.applyCalled, "AI orchestrator Apply should not be called during dry run")
	assert.False(t, aiOrch.unapplyCalled, "AI orchestrator Unapply should not be called during dry run")
}

func TestApply_CleansUpResolvedBase(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	resolver := &mockBaseResolver{}
	resolver.result = &profile.ResolvedBase{
		Config: &profile.FacetConfig{},
		Cleanup: func() error {
			resolver.cleanedUp = true
			return nil
		},
	}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "file:///tmp/base.git"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: resolver,
		Installer:    &mockInstaller{},
		StateStore:   &mockStateStore{},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return &mockDeployer{}
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir})
	require.NoError(t, err)
	assert.True(t, resolver.cleanedUp)
}

func TestApply_UsesRemoteConfigSourceAndMaterializes(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	remoteRoot := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	baseCfg := &profile.FacetConfig{
		Configs: map[string]string{
			"~/.gitconfig": "configs/.gitconfig",
		},
		ConfigMeta: map[string]profile.ConfigProvenance{
			"~/.gitconfig": {
				SourceRoot:  remoteRoot,
				Materialize: true,
			},
		},
	}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {Extends: "file:///tmp/base.git"},
			filepath.Join(stateDir, ".local.yaml"):         {},
		},
	}
	mockDep := &mockDeployer{}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   &mockStateStore{},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir})
	require.NoError(t, err)
	require.Len(t, mockDep.sources, 1)
	assert.Equal(t, "configs/.gitconfig", mockDep.sources[0].DisplaySource)
	assert.Equal(t, filepath.Join(remoteRoot, "configs", ".gitconfig"), mockDep.sources[0].ResolvedPath)
	assert.True(t, mockDep.sources[0].Materialize)
	require.Len(t, mockDep.deployed, 1)
	assert.Equal(t, deploy.StrategyCopy, mockDep.deployed[0].Strategy)
}

func TestApply_RunsRemoteScriptsFromRemoteClone(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	baseCfg := &profile.FacetConfig{
		PreApply: []profile.ScriptEntry{
			{Name: "remote-pre", Run: "echo remote-pre", WorkDir: "/remote-root"},
		},
		PostApply: []profile.ScriptEntry{
			{Name: "remote-post", Run: "echo remote-post", WorkDir: "/remote-root"},
		},
	}
	runner := &mockScriptRunner{}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "file:///tmp/base.git",
				PreApply: []profile.ScriptEntry{
					{Name: "profile-pre", Run: "echo profile-pre"},
				},
				PostApply: []profile.ScriptEntry{
					{Name: "profile-post", Run: "echo profile-post"},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   &mockStateStore{},
		ScriptRunner: runner,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return &mockDeployer{}
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir})
	require.NoError(t, err)
	require.Len(t, runner.commands, 4)
	assert.Equal(t, []string{"/remote-root", cfgDir, "/remote-root", cfgDir}, runner.dirs)
}

func TestApply_SameProfileInvalidTargetDoesNotTriggerOrphanCleanup(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	prevTarget := filepath.Join(t.TempDir(), "existing-config")

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	baseCfg := &profile.FacetConfig{}
	mockDep := &mockDeployer{}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "file:///tmp/base.git",
				Configs: map[string]string{
					"$UNDEFINED_TARGET/.gitconfig": "configs/.gitconfig",
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     &mockReporter{},
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   &mockStateStore{state: &ApplyState{Profile: "work", Configs: []deploy.ConfigResult{{Target: prevTarget, Source: "configs/.gitconfig", Strategy: deploy.StrategySymlink}}}},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{ConfigDir: cfgDir, StateDir: stateDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undefined environment variable")
	assert.Empty(t, mockDep.unapply)
}

func TestApply_EmitsProgressMessages(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "configs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "configs", ".zshrc"), []byte("# zshrc"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	r := &mockReporter{}
	mockDep := &mockDeployer{}
	stateStore := &mockStateStore{}
	baseCfg := &profile.FacetConfig{}

	targetPath := filepath.Join(stateDir, ".zshrc")
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				Configs: map[string]string{
					targetPath: "configs/.zshrc",
				},
				Packages: []profile.PackageEntry{
					{Name: "git", Install: profile.InstallCmd{Command: "brew install git"}},
				},
				PreApply: []profile.ScriptEntry{
					{Name: "setup", Run: "echo setup"},
				},
				PostApply: []profile.ScriptEntry{
					{Name: "teardown", Run: "echo teardown"},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     r,
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   stateStore,
		ScriptRunner: &mockScriptRunner{},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)

	var progressMessages []string
	for _, msg := range r.messages {
		if strings.HasPrefix(msg, "progress: ") {
			progressMessages = append(progressMessages, msg)
		}
	}

	progressOutput := strings.Join(progressMessages, "\n")
	assert.Contains(t, progressOutput, "progress: Loading profile ... ok")
	assert.Contains(t, progressOutput, "progress: Resolving extends ... ok")
	assert.Contains(t, progressOutput, "progress: Merging base and profile ... ok")
	assert.Contains(t, progressOutput, "progress: Deploying configs ... start")
	assert.Contains(t, progressOutput, "progress:   -> "+targetPath+" ... ok")
	assert.Contains(t, progressOutput, "progress: Installing packages ... start")
	assert.NotContains(t, progressMessages, "progress:   -> git")
	assert.Contains(t, progressOutput, "progress: Running pre_apply scripts ... start")
	assert.Contains(t, progressOutput, "progress:   -> setup ... ok")
	assert.Contains(t, progressOutput, "progress: Running post_apply scripts ... start")
	assert.Contains(t, progressOutput, "progress:   -> teardown ... ok")
	assert.Contains(t, progressOutput, "progress: Writing state ... ok")
	assert.Contains(t, progressOutput, "progress: facet apply work ... done")
}

func TestApply_EmitsUnapplyProgress_OnForce(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "configs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "configs", ".zshrc"), []byte("# zshrc"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	prevTarget := filepath.Join(stateDir, ".zshrc")
	r := &mockReporter{}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				Configs: map[string]string{prevTarget: "configs/.zshrc"},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	stateStore := &mockStateStore{
		state: &ApplyState{
			Profile: "work",
			Configs: []deploy.ConfigResult{
				{Target: prevTarget, Source: "configs/.zshrc", Strategy: deploy.StrategySymlink},
			},
		},
	}

	a := New(Deps{
		Reporter:     r,
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   stateStore,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
		Force:     true,
	})
	require.NoError(t, err)

	var progressMessages []string
	for _, msg := range r.messages {
		if strings.HasPrefix(msg, "progress: ") {
			progressMessages = append(progressMessages, msg)
		}
	}

	progressOutput := strings.Join(progressMessages, "\n")
	assert.Contains(t, progressOutput, "progress: Unapplying previous state ... start")
	assert.Contains(t, progressOutput, "progress:   -> configs unapply ... ok")
	assert.Contains(t, progressOutput, "progress: Unapplying previous state ... done")
}

func TestApply_ForceWithNonUnapplyStages_SkipsUnapplyProgress(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	r := &mockReporter{}
	baseCfg := &profile.FacetConfig{}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				Packages: []profile.PackageEntry{
					{Name: "git", Install: profile.InstallCmd{Command: "brew install git"}},
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     r,
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore: &mockStateStore{state: &ApplyState{
			Profile: "work",
			Configs: []deploy.ConfigResult{
				{Target: filepath.Join(stateDir, ".zshrc"), Source: "configs/.zshrc", Strategy: deploy.StrategySymlink},
			},
		}},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return &mockDeployer{}
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
		Force:     true,
		Stages:    "packages",
	})
	require.NoError(t, err)
	assert.NotContains(t, r.messages, "progress: Unapplying previous state")
}

func TestApply_EmitsOrphanCleanupProgress(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "configs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "configs", ".zshrc"), []byte("# zshrc"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	orphanTarget := filepath.Join(stateDir, ".oldrc")
	activeTarget := filepath.Join(stateDir, ".zshrc")
	r := &mockReporter{}
	mockDep := &mockDeployer{}
	baseCfg := &profile.FacetConfig{}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
				Configs: map[string]string{
					activeTarget: "configs/.zshrc",
				},
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     r,
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore: &mockStateStore{state: &ApplyState{
			Profile: "work",
			Configs: []deploy.ConfigResult{
				{Target: orphanTarget, Source: "configs/.oldrc", Strategy: deploy.StrategySymlink},
			},
		}},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return mockDep
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)
	progressOutput := strings.Join(r.messages, "\n")
	assert.Contains(t, progressOutput, "progress: Cleaning up orphaned configs ... start")
	assert.Contains(t, progressOutput, "progress:   -> orphan configs unapply ... ok")
	assert.Contains(t, progressOutput, "progress: Cleaning up orphaned configs ... done")
	require.Len(t, mockDep.unapply, 1)
	assert.Equal(t, orphanTarget, mockDep.unapply[0][0].Target)
}

func TestApply_SkipsEmptyStageProgressMessages(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".local.yaml"), []byte(""), 0o644))

	r := &mockReporter{}
	baseCfg := &profile.FacetConfig{}
	loader := &mockLoader{
		meta: &profile.FacetMeta{},
		configs: map[string]*profile.FacetConfig{
			filepath.Join(cfgDir, "base.yaml"): baseCfg,
			filepath.Join(cfgDir, "profiles", "work.yaml"): {
				Extends: "base",
			},
			filepath.Join(stateDir, ".local.yaml"): {},
		},
	}

	a := New(Deps{
		Reporter:     r,
		Loader:       loader,
		BaseResolver: newStaticBaseResolver(baseCfg),
		Installer:    &mockInstaller{},
		StateStore:   &mockStateStore{},
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, ownedConfigs []deploy.ConfigResult) deploy.Service {
			return &mockDeployer{}
		},
		OSName: "macos",
	})

	err := a.Apply("work", ApplyOpts{
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	})
	require.NoError(t, err)
	assert.NotContains(t, r.messages, "progress: Deploying configs")
	assert.NotContains(t, r.messages, "progress: Installing packages")
	assert.NotContains(t, r.messages, "progress: Applying AI configuration")
}
