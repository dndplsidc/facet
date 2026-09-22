package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"facet/internal/deploy"
	"facet/internal/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func platformApp(t *testing.T, osName, base, overlay string) (*App, ApplyOpts, *mockReporter, *mockScriptRunner, *mockStateStore) {
	t.Helper()
	dir, state, home := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "profiles"), 0755))
	for path, content := range map[string]string{
		"facet.yaml": "min_version: '0.1.0'", "base.yaml": base,
		"profiles/test.yaml": "extends: base\n" + overlay,
		"shared":             "shared", "macos": "macos", "linux": "linux",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(content), 0644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(state, ".local.yaml"), []byte("{}"), 0644))
	loader, reporter, runner, store := profile.NewLoader(), &mockReporter{}, &mockScriptRunner{}, &mockStateStore{}
	a := New(Deps{Loader: loader, BaseResolver: profile.NewBaseResolver(loader, nil), Reporter: reporter,
		Installer: &mockInstaller{}, ScriptRunner: runner, StateStore: store, OSName: osName,
		DeployerFactory: func(configDir, homeDir string, vars map[string]any, owned []deploy.ConfigResult) deploy.Service {
			return deploy.NewDeployer(configDir, homeDir, vars, owned)
		},
	})
	return a, ApplyOpts{ConfigDir: dir, StateDir: state}, reporter, runner, store
}

func TestApply_PlatformSelection(t *testing.T) {
	for _, osName := range []string{"macos", "linux"} {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry=%v", osName, dryRun), func(t *testing.T) {
				other := "macos"
				if osName == other {
					other = "linux"
				}
				base := fmt.Sprintf(`vars:
  selected: %s
configs:
  ~/.shared: shared
  ~/.platform:
    %s: ${facet:selected}
    %s: ${facet:missing}
  $FACET_UNSET_PLATFORM_PATH/file:
    %s: missing-source
pre_apply:
  - name: setup
    run:
      %s: echo ${facet:selected}
      %s: echo ${facet:missing}
post_apply:
  - name: inactive
    run:
      %s: echo ${facet:missing}
  - name: shared
    run: echo shared
packages:
  - name: conditional
    check:
      %s: echo ${facet:missing}
    install:
      %s: echo ${facet:missing}
`, osName, osName, other, other, osName, other, other, other, other)
				a, opts, reporter, runner, store := platformApp(t, osName, base, "")
				opts.DryRun = dryRun
				require.NoError(t, a.Apply("test", opts))
				output := strings.Join(reporter.messages, "\n")
				assert.Contains(t, output, "not configured for "+osName)
				assert.NotContains(t, output, "undefined variable")
				if dryRun {
					assert.Empty(t, runner.commands)
					assert.Nil(t, store.written)
					_, err := os.Lstat(filepath.Join(os.Getenv("HOME"), ".platform"))
					require.ErrorIs(t, err, os.ErrNotExist)
				} else {
					content, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".platform"))
					require.NoError(t, err)
					assert.Equal(t, osName, string(content))
					assert.Equal(t, []string{"echo " + osName, "echo shared"}, runner.commands)
					assert.Equal(t, []string{opts.ConfigDir, opts.ConfigDir}, runner.dirs)
					require.NotNil(t, store.written)
					assert.Len(t, store.written.Configs, 2)
				}
			})
		}
	}
}

func TestApply_PlatformOverrideAndCleanup(t *testing.T) {
	a, opts, reporter, runner, store := platformApp(t, "linux", `configs:
  ~/.platform:
    macos: macos
    linux: linux
pre_apply:
  - name: same
    run: echo base
`, `pre_apply:
  - name: same
    run:
      linux: echo overlay
`)
	require.NoError(t, a.Apply("test", opts))
	assert.Equal(t, []string{"echo base", "echo overlay"}, runner.commands)
	store.state = store.written
	require.NoError(t, os.WriteFile(filepath.Join(opts.StateDir, ".local.yaml"), []byte(`configs:
  ~/.platform:
    macos: macos
`), 0644))
	opts.DryRun = true
	reporter.messages = nil
	require.NoError(t, a.Apply("test", opts))
	output := strings.Join(reporter.messages, "\n")
	assert.Contains(t, output, "Configs to remove")
	assert.Contains(t, output, filepath.Join(os.Getenv("HOME"), ".platform"))
	_, statErr := os.Lstat(filepath.Join(os.Getenv("HOME"), ".platform"))
	require.NoError(t, statErr, "dry-run must not remove the inactive managed config")
	opts.Stages = "pre_apply"
	reporter.messages = nil
	require.NoError(t, a.Apply("test", opts))
	assert.NotContains(t, strings.Join(reporter.messages, "\n"), "Configs to remove")
	opts.DryRun = false
	require.NoError(t, a.Apply("test", opts))
	_, err := os.Lstat(filepath.Join(os.Getenv("HOME"), ".platform"))
	require.NoError(t, err)
	opts.Stages = "configs"
	require.NoError(t, a.Apply("test", opts))
	_, err = os.Lstat(filepath.Join(os.Getenv("HOME"), ".platform"))
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Empty(t, store.written.Configs)
}

func TestApply_PlatformHookFailure(t *testing.T) {
	a, opts, _, runner, _ := platformApp(t, "linux", `pre_apply:
  - name: failing
    run:
      linux: fail-selected
  - name: later
    run: echo later
`, "")
	runner.failOn = map[string]error{"fail-selected": fmt.Errorf("selected command failed")}
	require.ErrorContains(t, a.Apply("test", opts), "selected command failed")
	assert.Equal(t, []string{"fail-selected"}, runner.commands)
}

func TestApply_PlatformDryRunStages(t *testing.T) {
	a, opts, reporter, runner, store := platformApp(t, "linux", `configs:
  ~/.selected:
    linux: linux
  ~/.inactive:
    macos: macos
pre_apply:
  - name: chosen-hook
    run:
      linux: echo chosen
  - name: inactive-pre-hook
    run:
      macos: echo unused
post_apply:
  - name: unrequested-hook
    run: echo unrequested
packages:
  - name: unrequested-package
    install: echo unrequested
`, "")
	opts.DryRun, opts.Stages = true, "pre_apply"
	require.NoError(t, a.Apply("test", opts))
	output := strings.Join(reporter.messages, "\n")
	assert.Contains(t, output, "chosen-hook")
	assert.Contains(t, output, "inactive-pre-hook: skipped (not configured for linux)")
	assert.NotContains(t, output, "~/.inactive")
	assert.NotContains(t, output, "Configs to deploy")
	assert.NotContains(t, output, "unrequested")
	assert.Empty(t, runner.commands)
	assert.Nil(t, store.written)
}
