#!/bin/bash
set -euo pipefail
SUITE_DIR="${SUITE_DIR:-$(cd "$(dirname "$0")" && pwd)}"
source "$SUITE_DIR/helpers.sh"

mkdir -p "$HOME/dotfiles/profiles" "$HOME/dotfiles/configs" "$HOME/.facet"
cat > "$HOME/dotfiles/facet.yaml" <<'YAML'
min_version: "0.1.0"
YAML
cat > "$HOME/dotfiles/base.yaml" <<'YAML'
vars:
  wagent:
    models: {}
YAML
cat > "$HOME/dotfiles/profiles/work.yaml" <<'YAML'
extends: base
configs:
  ~/.wagent.yaml: configs/wagent.yaml
YAML
cat > "$HOME/dotfiles/configs/wagent.yaml" <<'YAML'
agents:
  tmates_cursor:
    models: ${facet:wagent.models|yaml}
YAML
cat > "$HOME/.facet/.local.yaml" <<'YAML'
vars:
  wagent:
    models:
      claude-opus-5-5-high:
        - local_agent_id: first
          managed_agent_id: "11785"
        - local_agent_id: second
          managed_agent_id: "00123"
      grok-4.7-high: []
YAML

facet_apply work --stages configs
assert_not_symlink "$HOME/.wagent.yaml"
assert_file_contains "$HOME/.wagent.yaml" '"local_agent_id": "first"'
assert_file_contains "$HOME/.wagent.yaml" '"local_agent_id": "second"'
assert_file_contains "$HOME/.wagent.yaml" '"managed_agent_id": "00123"'
assert_file_contains "$HOME/.wagent.yaml" '"grok-4.7-high": []'
assert_file_not_contains "$HOME/.wagent.yaml" '${facet:'

# Lists are replaced by the local layer, including an explicitly empty list.
cat > "$HOME/.facet/.local.yaml" <<'YAML'
vars:
  wagent:
    models:
      claude-opus-5-5-high: []
YAML
facet_apply work --stages configs
assert_file_contains "$HOME/.wagent.yaml" '"claude-opus-5-5-high": []'
assert_file_not_contains "$HOME/.wagent.yaml" '"first"'
assert_file_not_contains "$HOME/.wagent.yaml" '"grok-4.7-high"'

# No local model configuration uses the profile's empty mapping.
cat > "$HOME/.facet/.local.yaml" <<'YAML'
vars: {}
YAML
facet_apply work --stages configs
assert_file_contains "$HOME/.wagent.yaml" 'models: {}'

cat > "$HOME/dotfiles/configs/wagent.yaml" <<'YAML'
models: ${facet:missing|yaml}
YAML
assert_exit_code 1 facet_apply work --stages configs
cat > "$HOME/dotfiles/configs/wagent.yaml" <<'YAML'
models: ${facet:wagent.models|json}
YAML
assert_exit_code 1 facet_apply work --stages configs
echo "  structured YAML variables: local lists, empty collections, and errors"
