package pi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRunner struct {
	commands []string
	envs     []map[string]string
	fail     map[string]error
}

func (m *mockRunner) Run(name string, args ...string) error {
	return m.RunWithEnv(nil, name, args...)
}

func (m *mockRunner) RunWithEnv(env map[string]string, name string, args ...string) error {
	cmd := name
	for _, arg := range args {
		cmd += " " + arg
	}
	m.commands = append(m.commands, cmd)
	m.envs = append(m.envs, env)
	if err, ok := m.fail[cmd]; ok {
		return err
	}
	return nil
}

func (m *mockRunner) RunInteractive(name string, args ...string) error {
	return m.Run(name, args...)
}

type mockReporter struct {
	warnings []string
	success  []string
}

func (m *mockReporter) Success(msg string) { m.success = append(m.success, msg) }
func (m *mockReporter) Warning(msg string) { m.warnings = append(m.warnings, msg) }

func TestManagerApply_InstallsNewCurrentAndRemovesOrphans(t *testing.T) {
	runner := &mockRunner{fail: map[string]error{}}
	mgr := NewManager(runner, &mockReporter{})

	state, err := mgr.Apply(&Config{Extensions: []ExtensionEntry{{Source: "pi-lens"}, {Source: "pi-subagents"}}}, &PiState{Extensions: []string{"pi-lens", "old-ext"}}, ApplyOptions{})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"pi remove old-ext",
		"pi install pi-subagents",
	}, runner.commands)
	assert.Equal(t, []string{"pi-lens", "pi-subagents"}, state.Extensions)
}

func TestManagerApply_SkipsUnchangedExtensionsAndPreservesState(t *testing.T) {
	runner := &mockRunner{fail: map[string]error{}}
	mgr := NewManager(runner, &mockReporter{})

	state, err := mgr.Apply(&Config{Extensions: []ExtensionEntry{{Source: "pi-lens"}, {Source: "pi-subagents"}}}, &PiState{Extensions: []string{"pi-lens", "pi-subagents"}}, ApplyOptions{})
	require.NoError(t, err)

	assert.Empty(t, runner.commands)
	require.NotNil(t, state)
	assert.Equal(t, []string{"pi-lens", "pi-subagents"}, state.Extensions)
}

func TestManagerApply_ForceReinstallsUnchangedExtensions(t *testing.T) {
	runner := &mockRunner{fail: map[string]error{}}
	mgr := NewManager(runner, &mockReporter{})

	state, err := mgr.Apply(&Config{Extensions: []ExtensionEntry{{Source: "pi-lens"}, {Source: "pi-subagents"}}}, &PiState{Extensions: []string{"pi-lens"}}, ApplyOptions{Force: true})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"pi install pi-lens",
		"pi install pi-subagents",
	}, runner.commands)
	assert.Equal(t, []string{"pi-lens", "pi-subagents"}, state.Extensions)
}

func TestManagerApply_RecordsOnlySuccessfulInstalls(t *testing.T) {
	runner := &mockRunner{fail: map[string]error{
		"pi install broken-ext": fmt.Errorf("boom"),
	}}
	reporter := &mockReporter{}
	mgr := NewManager(runner, reporter)

	state, err := mgr.Apply(&Config{Extensions: []ExtensionEntry{{Source: "ok-ext"}, {Source: "broken-ext"}}}, nil, ApplyOptions{})
	require.NoError(t, err)

	assert.Equal(t, []string{"ok-ext"}, state.Extensions)
	assert.Len(t, reporter.warnings, 1)
}

func TestManagerApply_PassesInstallEnvOnlyToInstallCommands(t *testing.T) {
	runner := &mockRunner{fail: map[string]error{}}
	mgr := NewManager(runner, &mockReporter{})

	state, err := mgr.Apply(&Config{Extensions: []ExtensionEntry{
		{Source: "private-ext", InstallEnv: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"}},
	}}, &PiState{Extensions: []string{"old-ext"}}, ApplyOptions{})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"pi remove old-ext",
		"pi install private-ext",
	}, runner.commands)
	assert.Nil(t, runner.envs[0])
	assert.Equal(t, map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.example.com"}, runner.envs[1])
	assert.Equal(t, []string{"private-ext"}, state.Extensions)
}

func TestManagerApply_NilConfigRemovesPreviousManagedExtensions(t *testing.T) {
	runner := &mockRunner{fail: map[string]error{}}
	mgr := NewManager(runner, &mockReporter{})

	state, err := mgr.Apply(nil, &PiState{Extensions: []string{"pi-lens"}}, ApplyOptions{})
	require.NoError(t, err)

	assert.Equal(t, []string{"pi remove pi-lens"}, runner.commands)
	assert.Nil(t, state)
}

func TestManagerUnapply_RemovesPreviousManagedExtensions(t *testing.T) {
	runner := &mockRunner{fail: map[string]error{}}
	mgr := NewManager(runner, &mockReporter{})

	require.NoError(t, mgr.Unapply(&PiState{Extensions: []string{"pi-lens", "pi-subagents"}}))
	assert.Equal(t, []string{
		"pi remove pi-lens",
		"pi remove pi-subagents",
	}, runner.commands)
}
