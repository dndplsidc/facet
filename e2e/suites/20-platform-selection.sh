#!/bin/bash
set -euo pipefail
SUITE_DIR="${SUITE_DIR:-$(cd "$(dirname "$0")" && pwd)}"
source "$SUITE_DIR/helpers.sh"

case "$(uname -s)" in
  Darwin) active=macos; inactive=linux ;;
  Linux) active=linux; inactive=macos ;;
  *) echo 'Unsupported test platform'; exit 1 ;;
esac
mkdir -p "$HOME/dotfiles/profiles" "$HOME/.facet"
printf '%s\n' 'min_version: "0.1.0"' > "$HOME/dotfiles/facet.yaml"
printf '%s\n' '{}' > "$HOME/.facet/.local.yaml"
printf '%s\n' 'shared' > "$HOME/dotfiles/shared"
printf '%s\n' "$active" > "$HOME/dotfiles/selected"
cat > "$HOME/dotfiles/base.yaml" <<YAML
vars:
  source: selected
configs:
  ~/.shared: shared
  ~/.platform:
    $active: \${facet:source}
    $inactive: \${facet:undefined}
  \$FACET_MISSING_PLATFORM_PATH/file:
    $inactive: nonexistent-source
pre_apply:
  - name: duplicate
    run:
      $active: cat shared > "\$HOME/base-source"
      $inactive: \${facet:undefined}
post_apply:
  - name: inactive-hook
    run:
      $inactive: touch "\$HOME/must-not-run"
  - name: shared-hook
    run: touch "\$HOME/shared-hook"
packages:
  - name: platform-package
    check:
      $inactive: \${facet:undefined}
    install:
      $active: touch "\$HOME/package-ran"
      $inactive: \${facet:undefined}
  - name: unavailable-package
    check: \${facet:undefined}
    install:
      $inactive: \${facet:undefined}
YAML
cat > "$HOME/dotfiles/profiles/test.yaml" <<YAML
extends: base
pre_apply:
  - name: duplicate
    run:
      $active: touch "\$HOME/overlay-hook"
YAML

facet_apply test --dry-run > "$HOME/preview"
assert_file_contains "$HOME/preview" "not configured for $active"
assert_file_not_exists "$HOME/.platform"
assert_file_not_exists "$HOME/package-ran"
assert_file_not_exists "$HOME/base-source"
assert_file_not_exists "$HOME/.facet/.state.json"

facet_apply test > "$HOME/applied"
assert_file_contains "$HOME/applied" "not configured for $active"
assert_file_contains "$HOME/.platform" "$active"
assert_file_contains "$HOME/.shared" shared
assert_file_contains "$HOME/base-source" shared
assert_file_exists "$HOME/overlay-hook"
assert_file_exists "$HOME/shared-hook"
assert_file_exists "$HOME/package-ran"
assert_file_not_exists "$HOME/must-not-run"
assert_json_field "$HOME/.facet/.state.json" '.configs | length' '2'
assert_json_field "$HOME/.facet/.state.json" '.packages[] | select(.name == "unavailable-package") | .status' skipped

# Whole-entry override removes the inherited active branch. Stage-limited
# applies preserve deployed configs; a subsequent configs apply cleans orphans.
cat > "$HOME/.facet/.local.yaml" <<YAML
configs:
  ~/.platform:
    $inactive: shared
YAML
facet_apply test --dry-run --stages configs > "$HOME/cleanup-preview"
assert_file_contains "$HOME/cleanup-preview" 'Configs to remove'
managed_target="$(jq -r '.configs[] | select(.source == "selected") | .target' "$HOME/.facet/.state.json")"
assert_file_contains "$HOME/cleanup-preview" "$managed_target"
assert_file_exists "$HOME/.platform"
facet_apply test --stages pre_apply
assert_file_exists "$HOME/.platform"
facet_apply test --stages configs
assert_file_not_exists "$HOME/.platform"
assert_file_exists "$HOME/.shared"
assert_json_field "$HOME/.facet/.state.json" '.configs | length' '1'

# A selected hook failure is still fatal and stops subsequent hooks.
cat > "$HOME/dotfiles/profiles/test.yaml" <<YAML
extends: base
pre_apply:
  - name: selected-failure
    run:
      $active: 'false'
  - name: after-failure
    run: touch "\$HOME/after-failure"
YAML
assert_exit_code 1 facet_apply test --stages pre_apply
assert_file_not_exists "$HOME/after-failure"
echo '  platform selection, skip reporting, overrides, cleanup, and failures work'
