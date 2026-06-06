package pi

// Config is the effective Pi configuration to apply.
type Config struct {
	Extensions []ExtensionEntry
}

// ExtensionEntry describes one Pi package managed by Facet.
type ExtensionEntry struct {
	Source     string
	InstallEnv map[string]string
}

// PiState records Pi extensions managed by Facet.
type PiState struct {
	Extensions []string `json:"extensions,omitempty"`
}
