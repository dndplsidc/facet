package execrunner

import (
	"errors"
	"os/exec"
	"testing"
)

func TestOutput_PreservesFailureResponse(t *testing.T) {
	output, err := New().Output("sh", "-c", `printf '[{"status":"failed"}]'; printf 'diagnostic' >&2; exit 1`)
	if string(output) != `[{"status":"failed"}]` {
		t.Fatalf("expected structured stdout without stderr, got %q", output)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("expected preserved exit status 1, got %v", err)
	}
}

func TestRunInteractive_Success(t *testing.T) {
	r := New()
	err := r.RunInteractive("echo", "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunInteractive_Failure(t *testing.T) {
	r := New()
	err := r.RunInteractive("false")
	if err == nil {
		t.Fatal("expected error for 'false' command, got nil")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("expected *exec.ExitError, got %T: %v", err, err)
	}
}

func TestRunInteractive_CommandNotFound(t *testing.T) {
	r := New()
	err := r.RunInteractive("nonexistent-command-that-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
}
