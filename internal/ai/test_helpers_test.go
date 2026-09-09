package ai

import "strings"

// mockRunner records commands and optionally returns an error.
type mockRunner struct {
	commands            []string
	interactiveCommands []string
	err                 error
	output              []byte
}

func (m *mockRunner) Output(name string, args ...string) ([]byte, error) {
	if err := m.Run(name, args...); err != nil {
		return nil, err
	}
	if m.output == nil {
		return []byte("[]"), nil
	}
	return m.output, nil
}

func (m *mockRunner) Run(name string, args ...string) error {
	m.commands = append(m.commands, strings.Join(append([]string{name}, args...), " "))
	return m.err
}

func (m *mockRunner) RunInDir(_ string, name string, args ...string) error {
	return m.Run(name, args...)
}

func (m *mockRunner) RunInteractive(name string, args ...string) error {
	cmd := strings.Join(append([]string{name}, args...), " ")
	m.commands = append(m.commands, cmd)
	m.interactiveCommands = append(m.interactiveCommands, cmd)
	return m.err
}

// sequentialMockRunner records commands and returns errors from a pre-defined
// sequence, one per call. If the call index exceeds the errors slice, nil is returned.
type sequentialMockRunner struct {
	commands            []string
	interactiveCommands []string
	errors              []error
	callIdx             int
}

func (m *sequentialMockRunner) Output(name string, args ...string) ([]byte, error) {
	return []byte("[]"), m.Run(name, args...)
}

func (m *sequentialMockRunner) Run(name string, args ...string) error {
	m.commands = append(m.commands, strings.Join(append([]string{name}, args...), " "))
	var err error
	if m.callIdx < len(m.errors) {
		err = m.errors[m.callIdx]
	}
	m.callIdx++
	return err
}

func (m *sequentialMockRunner) RunInDir(_ string, name string, args ...string) error {
	return m.Run(name, args...)
}

func (m *sequentialMockRunner) RunInteractive(name string, args ...string) error {
	cmd := strings.Join(append([]string{name}, args...), " ")
	m.commands = append(m.commands, cmd)
	m.interactiveCommands = append(m.interactiveCommands, cmd)
	var err error
	if m.callIdx < len(m.errors) {
		err = m.errors[m.callIdx]
	}
	m.callIdx++
	return err
}
