#!/bin/bash
# e2e/suites/14-all-skills-from-source.sh — "all skills from source" E2E tests
SUITE_DIR="${SUITE_DIR:-$(cd "$(dirname "$0")" && pwd)}"
source "$SUITE_DIR/helpers.sh"

setup_basic
bash "$FIXTURE_DIR/setup-ai.sh"

# Simplify work profile to avoid template errors when base.yaml is overwritten.
cat > "$HOME/dotfiles/profiles/work.yaml" << 'YAML'
extends: base
YAML

mkdir -p "$HOME/.agents"
cat > "$HOME/.agents/.skill-lock.json" << 'JSON'
{
  "version": 3,
  "skills": {
    "skill-a": {"source": "@vercel-labs/agent-skills", "sourceUrl": "https://github.com/vercel-labs/agent-skills.git"},
    "skill-b": {"source": "@vercel-labs/agent-skills", "sourceUrl": "https://github.com/vercel-labs/agent-skills.git"},
    "other-skill": {"source": "@org/other-skills"}
  }
}
JSON

# Test 1: Apply with "all skills" entry (no skills list) — should pass --skill '*' without --all
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
  skills:
    - source: "https://github.com/vercel-labs/agent-skills.git"
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills add https://github.com/vercel-labs/agent-skills.git --skill * -a claude-code -g -y"
assert_file_not_contains "$HOME/.mock-ai" "--all"
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[0].source' 'https://github.com/vercel-labs/agent-skills.git'
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[0].name' 'skill-a'
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[1].name' 'skill-b'
echo "  all-skills entry passes wildcard skill flag and state records resolved skills"

# Test 2: Mixed all and specific entries — correct commands for each
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
  skills:
    - source: "@vercel-labs/agent-skills"
    - source: "@org/specific-skills"
      skills: [my-skill]
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills add @vercel-labs/agent-skills --skill * -a claude-code -g -y"
assert_file_not_contains "$HOME/.mock-ai" "--all"
assert_file_contains "$HOME/.mock-ai" "npx skills add @org/specific-skills --skill my-skill"
echo "  mixed all and specific entries produce correct commands"

# Test 3: "all" scoped to specific agents
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code, cursor]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
    cursor:
      allow: ["Read(**)"]
      deny: []
  skills:
    - source: "@vercel-labs/agent-skills"
      agents: [claude-code]
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills add @vercel-labs/agent-skills --skill * -a claude-code -g -y"
assert_file_not_contains "$HOME/.mock-ai" "--all"
assert_file_not_contains "$HOME/.mock-ai" "-a cursor"
echo "  all-skills respects agent scoping"

# Test 4: Unscoped all-skills entries include Pi when Pi is declared in ai.agents
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code, cursor, codex, pi]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
  skills:
    - source: "@vercel-labs/agent-skills"
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills add @vercel-labs/agent-skills --skill * -a claude-code -a codex -a cursor -a pi -g -y"
assert_file_not_contains "$HOME/.mock-ai" "--all"
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[0].agents | length' '4'
assert_json_field "$HOME/.facet/.state.json" '.ai.skills[0].agents[3]' 'pi'
echo "  all-skills includes Pi when Pi is declared"

# Test 5: Transition from specific to all — no orphan removal
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
  skills:
    - source: "@vercel-labs/agent-skills"
      skills: [frontend-design]
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills remove skill-a skill-b -a claude-code -a codex -a cursor -a pi -g -y"
assert_file_not_contains "$HOME/.mock-ai" "npx skills remove skill-a -a claude-code"
assert_file_not_contains "$HOME/.mock-ai" "npx skills remove skill-b -a claude-code"
echo "  applied specific skills"

# Now switch to "all"
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
  skills:
    - source: "@vercel-labs/agent-skills"
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_not_contains "$HOME/.mock-ai" "npx skills remove"
assert_file_contains "$HOME/.mock-ai" "npx skills add @vercel-labs/agent-skills --skill * -a claude-code -g -y"
assert_file_not_contains "$HOME/.mock-ai" "--all"
echo "  specific to all transition: no orphan removal, installs with wildcard skill flag"

# Test 6: Transition from all to specific
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
  skills:
    - source: "@vercel-labs/agent-skills"
      skills: [skill-a]
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills remove skill-b -a claude-code -g -y"
assert_file_contains "$HOME/.mock-ai" "npx skills add @vercel-labs/agent-skills --skill skill-a"
assert_file_not_contains "$HOME/.mock-ai" "other-skill"
echo "  all to specific transition: no orphan removal, installs specific skills"

# Test 7: Transition from all to nothing removes only skills from that source
cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
  skills:
    - source: "@vercel-labs/agent-skills"
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills add @vercel-labs/agent-skills --skill * -a claude-code -g -y"
assert_file_not_contains "$HOME/.mock-ai" "--all"
echo "  restored all-skills state before removal"

cat > "$HOME/dotfiles/base.yaml" << 'YAML'
ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
YAML

: > "$HOME/.mock-ai"
facet_apply work
assert_file_contains "$HOME/.mock-ai" "npx skills remove skill-a skill-b -a claude-code -g -y"
assert_file_not_contains "$HOME/.mock-ai" "other-skill"
echo "  all to nothing transition removes only resolved source skills"

# Test 8: State tracking for "all" entries
assert_file_exists "$HOME/.facet/.state.json"
assert_json_field "$HOME/.facet/.state.json" '.ai.skills' 'null'
echo "  state tracks skills correctly after transitions"
