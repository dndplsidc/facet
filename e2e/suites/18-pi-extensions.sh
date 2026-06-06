#!/bin/bash
# e2e/suites/18-pi-extensions.sh
SUITE_DIR="${SUITE_DIR:-$(cd "$(dirname "$0")" && pwd)}"
source "$SUITE_DIR/helpers.sh"

setup_basic

cat > "$HOME/dotfiles/base.yaml" << 'YAML'
vars:
  pi_extra: "@gotgenes/pi-session-tools"

packages:
  - name: pi-agent
    install: echo install-pi-agent

ai:
  pi:
    extensions:
      - source: pi-lens
      - source: pi-subagents
      - source: "${facet:pi_extra}"
        install_env:
          NPM_CONFIG_REGISTRY: https://registry.example.com
YAML

cat > "$HOME/dotfiles/profiles/work.yaml" << 'YAML'
extends: base

ai:
  pi:
    extensions:
      - source: pi-subagents
      - source: pi-interactive-shell
YAML

facet_apply work
assert_file_exists "$HOME/.mock-pi"
assert_file_contains "$HOME/.mock-pi" "pi install pi-lens"
assert_file_contains "$HOME/.mock-pi" "pi install pi-subagents"
assert_file_contains "$HOME/.mock-pi" "pi install @gotgenes/pi-session-tools"
assert_file_contains "$HOME/.mock-pi" "pi install env NPM_CONFIG_REGISTRY=https://registry.example.com"
assert_file_contains "$HOME/.mock-pi" "pi install pi-interactive-shell"
assert_json_field "$HOME/.facet/.state.json" '.ai.pi.extensions[0]' '@gotgenes/pi-session-tools'
echo "  ai.pi.extensions installed and recorded"

: > "$HOME/.mock-pi"
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
packages:
  - name: pi-agent
    install: echo install-pi-agent

ai:
  pi:
    extensions:
      - source: pi-lens
YAML
cat > "$HOME/dotfiles/profiles/work.yaml" << 'YAML'
extends: base
YAML
facet_apply work
assert_file_contains "$HOME/.mock-pi" "pi remove @gotgenes/pi-session-tools"
assert_file_contains "$HOME/.mock-pi" "pi remove pi-interactive-shell"
assert_file_contains "$HOME/.mock-pi" "pi remove pi-subagents"
assert_file_not_contains "$HOME/.mock-pi" "pi install pi-lens"
assert_json_field "$HOME/.facet/.state.json" '.ai.pi.extensions[0]' 'pi-lens'
echo "  removed only previously managed undeclared extensions and skipped unchanged install"

: > "$HOME/.mock-pi"
facet -c "$HOME/dotfiles" -s "$HOME/.facet" apply work --stages packages
if [ -s "$HOME/.mock-pi" ]; then
    echo "  ASSERT FAIL: --stages packages should not run pi package commands"
    cat "$HOME/.mock-pi"
    exit 1
fi
echo "  --stages packages skips ai pi extensions"

: > "$HOME/.mock-pi"
facet -c "$HOME/dotfiles" -s "$HOME/.facet" apply work --stages ai
if [ -s "$HOME/.mock-pi" ]; then
    echo "  ASSERT FAIL: unchanged --stages ai should not reinstall Pi extensions"
    cat "$HOME/.mock-pi"
    exit 1
fi
echo "  --stages ai skips unchanged pi extensions"

: > "$HOME/.mock-pi"
facet -c "$HOME/dotfiles" -s "$HOME/.facet" apply work --stages ai --force
assert_file_contains "$HOME/.mock-pi" "pi remove pi-lens"
assert_file_contains "$HOME/.mock-pi" "pi install pi-lens"
echo "  --force reinstalls unchanged pi extensions"

output=$(facet -c "$HOME/dotfiles" -s "$HOME/.facet" apply --dry-run work 2>&1)
echo "$output" | grep -q "AI Pi extensions" || { echo "  ASSERT FAIL: dry-run should show AI Pi extensions"; echo "$output"; exit 1; }
echo "  dry-run shows ai pi extension preview"
