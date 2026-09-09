#!/bin/bash
# Cleanup verification, retry, and stale lock inventory behavior.
SUITE_DIR="${SUITE_DIR:-$(cd "$(dirname "$0")" && pwd)}"
source "$SUITE_DIR/helpers.sh"
setup_basic
mkdir -p "$HOME/.agents"
cat > "$HOME/dotfiles/profiles/work.yaml" << 'YAML'
extends: base
YAML
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code, codex, cursor, pi]
  skills:
    - source: org/repo
      skills: [example]
YAML
cat > "$HOME/.agents/.skill-lock.json" << 'JSON'
{"version":3,"skills":{"example":{"source":"org/repo"},"unrelated":{"source":"other/repo"}}}
JSON
facet_apply work
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[0].name' 'example'

cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai: {}
YAML
# The CLI can exit zero without removing anything. Keep ownership for retry.
touch "$HOME/.mock-skills-retain"
facet_apply work
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[0].name' 'example'
assert_file_contains "$HOME/.mock-ai" 'npx skills remove example -g -y'
rm "$HOME/.mock-skills-retain"
facet_apply work
assert_json_field "$HOME/.facet/.state.json" '.ai.skills' 'null'
assert_json_field "$HOME/.agents/.skill-lock.json" '.skills | keys | join(",")' 'unrelated'
: > "$HOME/.mock-ai"
facet_apply work
assert_file_not_contains "$HOME/.mock-ai" 'npx skills add'
assert_file_not_contains "$HOME/.mock-ai" 'npx skills remove'

# Lock metadata alone does not establish an installed skill or Facet ownership.
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code, codex, cursor, pi]
  skills:
    - source: other/repo
YAML
echo '[]' > "$HOME/.mock-skills-inventory.json"
facet_apply work
assert_json_field "$HOME/.facet/.state.json" '.ai.skills' 'null'
assert_json_field "$HOME/.agents/.skill-lock.json" '.skills | keys | join(",")' 'unrelated'
echo '  verified cleanup, retry, unrelated-skill preservation, and stale lock filtering'
