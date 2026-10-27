#!/bin/bash
# Reconcile a changed source without adopting unrelated installed skills.
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
YAML
cat > "$HOME/.agents/.skill-lock.json" << 'JSON'
{"version":3,"skills":{"current":{"source":"org/repo"},"deleted":{"source":"org/repo"},"unrelated":{"source":"other/repo"}}}
JSON
facet_apply work
assert_json_field "$HOME/.facet/.state.json" '.ai.skills | map(.name) | sort | join(",")' 'current,deleted'

# The source changes, while the installed lock still contains the deleted skill.
cat > "$HOME/.mock-skills-upstream.json" << 'JSON'
{"skills":{"current":{"source":"org/repo"}}}
JSON

# Partial or unreadable installation results must not authorize deletion.
for result in 'not JSON' '[]' '[{"name":"current","status":"failed"}]'; do
    printf '%s\n' "$result" > "$HOME/.mock-skills-install-result.json"
    : > "$HOME/.mock-ai"
    facet_apply work
    assert_json_field "$HOME/.facet/.state.json" '.ai.skills | map(.name) | sort | join(",")' 'current,deleted'
    assert_json_field "$HOME/.agents/.skill-lock.json" '.skills | has("deleted")' 'true'
    assert_file_not_contains "$HOME/.mock-ai" 'npx skills remove'
done
rm "$HOME/.mock-skills-install-result.json"

# A failed cleanup keeps ownership, then a subsequent apply retries it.
touch "$HOME/.mock-skills-retain"
facet_apply work
assert_json_field "$HOME/.facet/.state.json" '.ai.skills | map(.name) | unique | sort | join(",")' 'current,deleted'
rm "$HOME/.mock-skills-retain"
facet_apply work
assert_json_field "$HOME/.facet/.state.json" '.ai.skills | map(.name) | sort | join(",")' 'current'
assert_json_field "$HOME/.agents/.skill-lock.json" '.skills | keys | join(",")' 'current,unrelated'

# An unchanged source requires no additional removal.
: > "$HOME/.mock-ai"
facet_apply work
assert_file_not_contains "$HOME/.mock-ai" 'npx skills remove'

# Explicit selections keep their existing behavior, even if missing upstream.
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code, codex, cursor, pi]
  skills:
    - source: org/repo
      skills: [current]
YAML
echo '{"skills":{}}' > "$HOME/.mock-skills-upstream.json"
: > "$HOME/.mock-ai"
facet_apply work
assert_file_not_contains "$HOME/.mock-ai" 'npx skills remove'
assert_json_field "$HOME/.facet/.state.json" '.ai.skills | map(.name) | join(",")' 'current'
echo '  upstream deletion, conservative failure handling, retry, and explicit selection verified'

# Reducing the agent set to Pi retains shared storage instead of a global reset.
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [pi]
  skills:
    - source: org/repo
      skills: [current]
YAML
: > "$HOME/.mock-ai"
facet_apply work
assert_file_not_contains "$HOME/.mock-ai" 'npx skills remove current -g -y'
assert_file_not_contains "$HOME/.mock-ai" '--copy'
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[0].agents | join(",")' 'pi'
assert_json_field "$HOME/.agents/.skill-lock.json" '.skills | has("current")' 'true'
