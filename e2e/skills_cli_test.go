//go:build skillsintegration

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRealSkillsLifecycle exercises the Facet binary and published skills CLI.
// Install skills@1.5.25 outside the checkout and set FACET_SKILLS_CLI to its
// bin/cli.mjs. Git URL rewriting supplies a local repository, so the test needs
// no network, credentials, existing skills, or host configuration.
func TestRealSkillsLifecycle(t *testing.T) {
	cli := os.Getenv("FACET_SKILLS_CLI")
	require.NotEmpty(t, cli, "set FACET_SKILLS_CLI to skills@1.5.25/bin/cli.mjs")
	cli, err := filepath.Abs(cli)
	require.NoError(t, err)
	_, err = os.Stat(cli)
	require.NoError(t, err)
	binary := filepath.Join(t.TempDir(), "facet")
	build := exec.Command("go", "build", "-o", binary, "..")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "%s", out)

	for _, scenario := range []string{"full removal", "native copies", "native install", "shared retained", "lock only", "xdg lock"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			repo := filepath.Join(home, "source")
			bin := filepath.Join(home, "bin")
			write := func(path, text string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
			}
			projectDir := filepath.Join(home, "project")
			projectSkill := filepath.Join(projectDir, ".agents", "skills", "example", "SKILL.md")
			projectContent := "---\nname: example\ndescription: Independent project skill\n---\nKeep this project skill.\n"
			write(projectSkill, projectContent)
			for _, name := range []string{"example", "unrelated"} {
				write(filepath.Join(repo, name, "SKILL.md"), fmt.Sprintf("---\nname: %s\ndescription: Isolated cleanup test fixture\n---\nTest fixture.\n", name))
			}
			write(filepath.Join(bin, "npx"), "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then exec node --version; fi\n[ \"$1\" = skills ] || exit 1\nshift\nexec node \"$FACET_SKILLS_CLI\" \"$@\"\n")
			require.NoError(t, os.Chmod(filepath.Join(bin, "npx"), 0o755))
			// A whitelist prevents inherited agent/config variables from pointing
			// at the real home. Git accepts only the rewritten local file URL.
			env := []string{
				"HOME=" + home, "PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TMPDIR=" + home, "FACET_SKILLS_CLI=" + cli,
				"DISABLE_TELEMETRY=1", "DO_NOT_TRACK=1", "CI=1", "GIT_CONFIG_NOSYSTEM=1",
				"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=url.file://" + repo + ".insteadOf",
				"GIT_CONFIG_VALUE_0=https://skills.invalid/org/repo.git", "GIT_ALLOW_PROTOCOL=file",
				"GIT_AUTHOR_NAME=Facet Test", "GIT_AUTHOR_EMAIL=facet@example.invalid",
				"GIT_COMMITTER_NAME=Facet Test", "GIT_COMMITTER_EMAIL=facet@example.invalid",
			}
			lockPath := filepath.Join(home, ".agents", ".skill-lock.json")
			if scenario == "xdg lock" {
				env = append(env, "XDG_STATE_HOME="+filepath.Join(home, "state"))
				lockPath = filepath.Join(home, "state", "skills", ".skill-lock.json")
			}
			run := func(name string, args ...string) string {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, name, args...)
				cmd.Env, cmd.Dir = env, projectDir
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s %v:\n%s", name, args, out)
				return string(out)
			}
			require.Equal(t, "1.5.25", strings.TrimSpace(run("node", cli, "--version")))
			run("git", "-C", repo, "init", "-q")
			run("git", "-C", repo, "add", ".")
			run("git", "-C", repo, "commit", "-qm", "fixture")
			source := "https://skills.invalid/org/repo.git"
			// Cline discovers shared skills merely by being installed.
			require.NoError(t, os.MkdirAll(filepath.Join(home, ".cline"), 0o755))
			run("node", cli, "add", source, "--skill", "unrelated", "-a", "codex", "-g", "-y")
			configDir, stateDir := filepath.Join(home, "profiles"), filepath.Join(home, ".facet")
			write(filepath.Join(configDir, "facet.yaml"), "min_version: 0.1.0\n")
			write(filepath.Join(configDir, "profiles", "work.yaml"), "extends: base\n")
			write(filepath.Join(stateDir, ".local.yaml"), "{}\n")
			configure := func(agents string) {
				content := "ai: {}\n"
				if agents != "" {
					content = fmt.Sprintf("ai:\n  agents: [%s]\n  skills:\n    - source: %s\n      skills: [example]\n", agents, source)
				}
				write(filepath.Join(configDir, "base.yaml"), content)
			}
			apply := func() { run(binary, "-c", configDir, "-s", stateDir, "apply", "work") }
			lock := func() map[string]json.RawMessage {
				data, err := os.ReadFile(lockPath)
				require.NoError(t, err)
				var value struct {
					Skills map[string]json.RawMessage `json:"skills"`
				}
				require.NoError(t, json.Unmarshal(data, &value))
				return value.Skills
			}
			canonical := filepath.Join(home, ".agents", "skills", "example")
			configure("claude-code, codex, cursor, pi")
			if scenario == "native install" {
				configure("claude-code, pi")
			}
			apply()
			if scenario != "native install" {
				require.FileExists(t, filepath.Join(canonical, "SKILL.md"))
			}
			require.Contains(t, lock(), "example")
			if scenario == "native copies" || scenario == "native install" {
				configure("claude-code, pi")
				apply()
				require.NoDirExists(t, canonical)
				for _, path := range []string{".claude/skills/example", ".pi/agent/skills/example"} {
					info, err := os.Lstat(filepath.Join(home, path))
					require.NoError(t, err)
					require.True(t, info.IsDir(), "native skill must be a directory, not a symlink")
				}
				require.Contains(t, lock(), "example", "valid native copies retain update metadata")
				state, err := os.ReadFile(filepath.Join(stateDir, ".state.json"))
				require.NoError(t, err)
				require.Contains(t, string(state), `"example"`, "inventory must recognize native copies")
			}
			if scenario == "shared retained" {
				configure("cursor")
				apply()
				require.FileExists(t, filepath.Join(canonical, "SKILL.md"))
				require.Contains(t, lock(), "example")
				for _, path := range []string{".claude/skills/example", ".pi/agent/skills/example"} {
					_, err := os.Lstat(filepath.Join(home, path))
					require.ErrorIs(t, err, os.ErrNotExist)
				}
			}
			if scenario == "lock only" {
				// Simulate missing files while leaving CLI metadata and dangling links.
				require.NoError(t, os.Rename(canonical, filepath.Join(home, "removed-example")))
			}
			configure("")
			apply()
			projectData, err := os.ReadFile(projectSkill)
			require.NoError(t, err, "global removal must preserve same-name project skills")
			require.Equal(t, projectContent, string(projectData))
			require.NoDirExists(t, canonical)
			for _, path := range []string{".claude/skills/example", ".pi/agent/skills/example"} {
				_, err := os.Lstat(filepath.Join(home, path))
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			require.NotContains(t, lock(), "example")
			require.Contains(t, lock(), "unrelated")
			require.FileExists(t, filepath.Join(home, ".agents", "skills", "unrelated", "SKILL.md"))
			apply()
			run("node", cli, "update", "-g", "-y")
			require.NoDirExists(t, canonical)
			require.NotContains(t, lock(), "example")
		})
	}
}
