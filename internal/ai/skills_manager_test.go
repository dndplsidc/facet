package ai

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"facet/internal/common/execrunner"
)

func TestNPXSkillsManager_Install_ChecksNPX(t *testing.T) {
	runner := &mockRunner{}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Install("@my-org/skills", []string{"skill-a", "skill-b"}, []string{"claude-code", "codex"})
	if err != nil {
		t.Fatalf("Install returned unexpected error: %v", err)
	}

	// First command is npx --version (lazy check), second is the actual install.
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}
	if runner.commands[0] != "npx --version" {
		t.Errorf("expected npx --version check, got %q", runner.commands[0])
	}

	want := "npx skills add @my-org/skills --skill skill-a --skill skill-b -a claude-code -a codex -g -y"
	if runner.commands[1] != want {
		t.Errorf("unexpected install command:\n  got:  %q\n  want: %q", runner.commands[1], want)
	}
}

func TestNPXSkillsManager_Install_NPXNotFound(t *testing.T) {
	runner := &mockRunner{err: errors.New("command not found")}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Install("@my-org/skills", []string{"skill-a"}, []string{"claude-code"})
	if err == nil {
		t.Fatal("expected error when npx not found, got nil")
	}
	// Should only have the npx --version check, no install attempt.
	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command (npx check only), got %d: %v", len(runner.commands), runner.commands)
	}
}

func TestNPXSkillsManager_Install_NPXCheckRunsOnce(t *testing.T) {
	runner := &mockRunner{}
	mgr := NewNPXSkillsManager(runner, "")

	_ = mgr.Install("@org/a", []string{"s1"}, []string{"claude-code"})
	_ = mgr.Install("@org/b", []string{"s2"}, []string{"cursor"})

	// npx --version should appear only once (first call), then 2 install commands.
	if len(runner.commands) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(runner.commands), runner.commands)
	}
	if runner.commands[0] != "npx --version" {
		t.Errorf("first command should be npx check, got %q", runner.commands[0])
	}
	// Second and third should be install commands, no extra npx --version.
	for _, cmd := range runner.commands[1:] {
		if cmd == "npx --version" {
			t.Error("npx --version should only run once")
		}
	}
}

func TestNPXSkillsManager_Remove(t *testing.T) {
	runner := &mockRunner{}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Remove([]string{"skill-a", "skill-b"}, []string{"claude-code", "codex"})
	if err != nil {
		t.Fatalf("Remove returned unexpected error: %v", err)
	}

	// First command is npx --version (lazy check), second is the remove.
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}
	if runner.commands[0] != "npx --version" {
		t.Errorf("expected npx --version check, got %q", runner.commands[0])
	}

	want := "npx skills remove skill-a skill-b -a claude-code -a codex -g -y"
	if runner.commands[1] != want {
		t.Errorf("unexpected remove command:\n  got:  %q\n  want: %q", runner.commands[1], want)
	}
}

func TestNPXSkillsManager_Remove_NPXNotFound(t *testing.T) {
	runner := &mockRunner{err: errors.New("command not found")}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Remove([]string{"skill-a"}, []string{"claude-code"})
	if err == nil {
		t.Fatal("expected error when npx not found, got nil")
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command (npx check only), got %d: %v", len(runner.commands), runner.commands)
	}
}

func TestNPXSkillsManager_Install_PreservesArgumentBoundaries(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "npx-args.log")
	binDir := filepath.Join(dir, "bin")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"  exit 0\n" +
		"fi\n" +
		"for arg in \"$@\"; do\n" +
		"  printf '%s\\n' \"$arg\" >> \"$LOGFILE\"\n" +
		"done\n"
	if err := os.WriteFile(filepath.Join(binDir, "npx"), []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write npx stub: %v", err)
	}

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("LOGFILE", logPath)

	mgr := NewNPXSkillsManager(execrunner.New(), "")

	if err := mgr.Install("/tmp/skills repo", []string{"skill-a"}, []string{"claude-code"}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read arg log: %v", err)
	}

	got := splitNonEmptyLines(string(data))
	want := []string{
		"skills",
		"add",
		"/tmp/skills repo",
		"--skill",
		"skill-a",
		"-a",
		"claude-code",
		"-g",
		"-y",
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected arg count:\n  got:  %#v\n  want: %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d]: got %q, want %q\nfull got: %#v", i, got[i], want[i], got)
		}
	}
}

func TestNPXSkillsManager_Install_NPXCheckOK_InstallFails(t *testing.T) {
	runner := &sequentialMockRunner{
		errors: []error{nil, errors.New("install failed")},
	}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Install("@my-org/skills", []string{"skill-a"}, []string{"claude-code"})
	if err == nil {
		t.Fatal("expected error when install fails, got nil")
	}
	if !contains(err.Error(), "skills install") {
		t.Errorf("expected error to contain 'skills install', got %q", err.Error())
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}
}

func TestNPXSkillsManager_Check(t *testing.T) {
	runner := &mockRunner{}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Check()
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}

	// First command is npx --version (lazy check), second is the check command.
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}
	if runner.commands[0] != "npx --version" {
		t.Errorf("expected npx --version check, got %q", runner.commands[0])
	}

	want := "npx skills check"
	if runner.commands[1] != want {
		t.Errorf("unexpected check command:\n  got:  %q\n  want: %q", runner.commands[1], want)
	}

	// Check uses RunInteractive, so the check command should appear in interactiveCommands.
	if len(runner.interactiveCommands) != 1 {
		t.Fatalf("expected 1 interactive command, got %d: %v", len(runner.interactiveCommands), runner.interactiveCommands)
	}
	if runner.interactiveCommands[0] != want {
		t.Errorf("unexpected interactive command:\n  got:  %q\n  want: %q", runner.interactiveCommands[0], want)
	}
}

func TestNPXSkillsManager_Check_NPXCheckOK_CheckFails(t *testing.T) {
	runner := &sequentialMockRunner{
		errors: []error{nil, errors.New("check failed")},
	}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Check()
	if err == nil {
		t.Fatal("expected error when check fails, got nil")
	}
	if !contains(err.Error(), "skills check") {
		t.Errorf("expected error to contain 'skills check', got %q", err.Error())
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}

	// The check command should have been called via RunInteractive.
	if len(runner.interactiveCommands) != 1 {
		t.Fatalf("expected 1 interactive command, got %d: %v", len(runner.interactiveCommands), runner.interactiveCommands)
	}
	if runner.interactiveCommands[0] != "npx skills check" {
		t.Errorf("unexpected interactive command: %q", runner.interactiveCommands[0])
	}
}

func TestNPXSkillsManager_Check_NPXNotFound(t *testing.T) {
	runner := &mockRunner{err: errors.New("command not found")}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Check()
	if err == nil {
		t.Fatal("expected error when npx not found, got nil")
	}
	// Should only have the npx --version check, no check attempt.
	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command (npx check only), got %d: %v", len(runner.commands), runner.commands)
	}
}

func TestNPXSkillsManager_Update(t *testing.T) {
	runner := &mockRunner{}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Update()
	if err != nil {
		t.Fatalf("Update returned unexpected error: %v", err)
	}

	// First command is npx --version (lazy check), second is the update command.
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}
	if runner.commands[0] != "npx --version" {
		t.Errorf("expected npx --version check, got %q", runner.commands[0])
	}

	want := "npx skills update"
	if runner.commands[1] != want {
		t.Errorf("unexpected update command:\n  got:  %q\n  want: %q", runner.commands[1], want)
	}

	// Update uses RunInteractive, so the update command should appear in interactiveCommands.
	if len(runner.interactiveCommands) != 1 {
		t.Fatalf("expected 1 interactive command, got %d: %v", len(runner.interactiveCommands), runner.interactiveCommands)
	}
	if runner.interactiveCommands[0] != want {
		t.Errorf("unexpected interactive command:\n  got:  %q\n  want: %q", runner.interactiveCommands[0], want)
	}
}

func TestNPXSkillsManager_Update_NPXCheckOK_UpdateFails(t *testing.T) {
	runner := &sequentialMockRunner{
		errors: []error{nil, errors.New("update failed")},
	}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Update()
	if err == nil {
		t.Fatal("expected error when update fails, got nil")
	}
	if !contains(err.Error(), "skills update") {
		t.Errorf("expected error to contain 'skills update', got %q", err.Error())
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}

	// The update command should have been called via RunInteractive.
	if len(runner.interactiveCommands) != 1 {
		t.Fatalf("expected 1 interactive command, got %d: %v", len(runner.interactiveCommands), runner.interactiveCommands)
	}
	if runner.interactiveCommands[0] != "npx skills update" {
		t.Errorf("unexpected interactive command: %q", runner.interactiveCommands[0])
	}
}

func TestNPXSkillsManager_Update_NPXNotFound(t *testing.T) {
	runner := &mockRunner{err: errors.New("command not found")}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Update()
	if err == nil {
		t.Fatal("expected error when npx not found, got nil")
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command (npx check only), got %d: %v", len(runner.commands), runner.commands)
	}
}

func TestNPXSkillsManager_Install_AllSkillsUsesSkillWildcardWithoutAllAgentShortcut(t *testing.T) {
	runner := &mockRunner{}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Install("@my-org/skills", nil, []string{"claude-code", "cursor", "pi"})
	if err != nil {
		t.Fatalf("Install returned unexpected error: %v", err)
	}

	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}

	want := "npx skills add @my-org/skills --skill * -a claude-code -a cursor -a pi -g -y"
	if runner.commands[1] != want {
		t.Errorf("unexpected install command:\n  got:  %q\n  want: %q", runner.commands[1], want)
	}
}

func TestNPXSkillsManager_Install_AllSkillsEmptySliceUsesSkillWildcard(t *testing.T) {
	runner := &mockRunner{}
	mgr := NewNPXSkillsManager(runner, "")

	err := mgr.Install("@my-org/skills", []string{}, []string{"claude-code"})
	if err != nil {
		t.Fatalf("Install returned unexpected error: %v", err)
	}

	want := "npx skills add @my-org/skills --skill * -a claude-code -g -y"
	if runner.commands[1] != want {
		t.Errorf("unexpected install command:\n  got:  %q\n  want: %q", runner.commands[1], want)
	}
}

func TestNPXSkillsManager_InstalledForSource(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".skill-lock.json")
	if err := os.WriteFile(lockPath, []byte(`{
		"version": 3,
		"skills": {
			"skill-a": {"source": "@org/skills", "sourceUrl": "git@github.com:org/skills.git"},
			"skill-b": {"source": "@org/skills", "sourceUrl": "git@github.com:org/skills.git"},
			"other-skill": {"source": "@org/other"}
		}
	}`), 0o644); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}

	mgr := NewNPXSkillsManager(&mockRunner{}, lockPath)

	got, err := mgr.InstalledForSource("@org/skills")
	if err != nil {
		t.Fatalf("InstalledForSource returned unexpected error: %v", err)
	}
	want := []string{"skill-a", "skill-b"}
	if len(got) != len(want) {
		t.Fatalf("unexpected skills:\n got:  %#v\n want: %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("skill[%d]: got %q, want %q", i, got[i], want[i])
		}
	}

	got, err = mgr.InstalledForSource("git@github.com:org/skills.git")
	if err != nil {
		t.Fatalf("InstalledForSource by sourceUrl returned unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected skills by sourceUrl:\n got:  %#v\n want: %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sourceUrl skill[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNPXSkillsManager_InstalledForSource_MissingLockFile(t *testing.T) {
	mgr := NewNPXSkillsManager(&mockRunner{}, filepath.Join(t.TempDir(), "missing.json"))

	got, err := mgr.InstalledForSource("@org/skills")
	if err != nil {
		t.Fatalf("InstalledForSource returned unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil skills for missing lock file, got %#v", got)
	}
}
