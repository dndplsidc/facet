package execrunner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes commands directly without going through a shell.
type Runner struct{}

// New constructs a Runner.
func New() *Runner {
	return &Runner{}
}

// Output captures stdout for machine-readable commands and keeps stderr separate.
func (r *Runner) Output(name string, args ...string) ([]byte, error) {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return output, nil
}

// Run executes the given command and argv.
func (r *Runner) Run(name string, args ...string) error {
	return r.RunWithEnv(nil, name, args...)
}

// RunInDir executes a command with an explicit working directory.
func (r *Runner) RunInDir(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return run(cmd)
}

// RunWithEnv executes the given command and argv with extra environment values.
func (r *Runner) RunWithEnv(env map[string]string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if len(env) > 0 {
		cmd.Env = mergedEnv(env)
	}
	return run(cmd)
}

func run(cmd *exec.Cmd) error {
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}

func mergedEnv(overrides map[string]string) []string {
	base := os.Environ()
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, overridden := overrides[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

// RunInteractive executes the given command with stdout and stderr connected
// directly to the parent process, allowing real-time streaming output.
func (r *Runner) RunInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
