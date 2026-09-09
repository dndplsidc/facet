#!/bin/bash
# e2e/suites/10-ai-config.sh — AI configuration E2E tests
SUITE_DIR="${SUITE_DIR:-$(cd "$(dirname "$0")" && pwd)}"
source "$SUITE_DIR/helpers.sh"

setup_basic
bash "$FIXTURE_DIR/setup-ai.sh"

mkdir -p "$HOME/.agents"
cat > "$HOME/.agents/.skill-lock.json" << 'JSON'
{
  "version": 3,
  "skills": {
    "frontend-design": {"source": "@vercel-labs/agent-skills", "sourceUrl": "https://github.com/vercel-labs/agent-skills.git"}
  }
}
JSON

# Test 1: Apply with AI config
facet_apply work
echo "  apply with AI config exited cleanly"

assert_file_exists "$HOME/.facet/.state.json"
assert_json_field "$HOME/.facet/.state.json" '.profile' 'work'
assert_json_field "$HOME/.facet/.state.json" '.ai.permissions."claude-code".allow[0]' 'Read'
assert_json_field "$HOME/.facet/.state.json" '.ai.permissions."claude-code".allow[1]' 'Edit'
assert_json_field "$HOME/.facet/.state.json" '.ai.permissions."claude-code".allow[2]' 'Bash'
echo "  state.json has correct AI data"

assert_file_exists "$HOME/.claude/settings.json"
assert_file_contains "$HOME/.claude/settings.json" '"permissions"'
assert_file_contains "$HOME/.claude/settings.json" '"allow"'
assert_file_contains "$HOME/.claude/settings.json" '"Read"'
assert_file_contains "$HOME/.claude/settings.json" '"Edit"'
assert_file_contains "$HOME/.claude/settings.json" '"Bash"'
echo "  Claude Code permissions written correctly"

assert_file_exists "$HOME/.cursor/cli-config.json"
assert_file_contains "$HOME/.cursor/cli-config.json" '"permissions"'
assert_file_contains "$HOME/.cursor/cli-config.json" '"Read(**)"'
assert_file_contains "$HOME/.cursor/cli-config.json" '"Write(**)"'
assert_file_not_contains "$HOME/.cursor/cli-config.json" '"Shell(*)"'
echo "  Cursor permissions written correctly"

assert_file_exists "$HOME/.mock-ai"
assert_file_contains "$HOME/.mock-ai" "npx skills add @vercel-labs/agent-skills"
assert_file_contains "$HOME/.mock-ai" "frontend-design"
echo "  skills installed via npx"

assert_file_contains "$HOME/.mock-ai" "claude mcp add playwright --scope user"
assert_file_contains "$HOME/.mock-ai" "claude mcp add github --scope user"
echo "  Claude Code MCPs registered via CLI at user scope"

assert_file_exists "$HOME/.cursor/mcp.json"
assert_file_contains "$HOME/.cursor/mcp.json" '"playwright"'
assert_file_not_contains "$HOME/.cursor/mcp.json" '"github"'
echo "  Cursor MCP file has playwright only (github scoped to claude-code)"

assert_file_contains "$HOME/.mock-ai" "GITHUB_TOKEN=s3cret"
echo "  MCP env var resolved from .local.yaml"

# Test 2: Reapply same profile (idempotent)
: > "$HOME/.mock-ai"
facet_apply work
echo "  reapply exited cleanly"

assert_file_contains "$HOME/.claude/settings.json" '"Read"'
assert_file_contains "$HOME/.claude/settings.json" '"Edit"'
assert_file_contains "$HOME/.claude/settings.json" '"Bash"'
echo "  idempotent reapply: claude-code permissions still correct"

assert_file_contains "$HOME/.cursor/cli-config.json" '"Read(**)"'
assert_file_contains "$HOME/.cursor/cli-config.json" '"Write(**)"'
echo "  idempotent reapply: cursor permissions still correct"

assert_file_contains "$HOME/.cursor/mcp.json" '"playwright"'
echo "  idempotent reapply: cursor MCPs still correct"

# Test 3: Edit same profile to narrow AI scope to claude-code only
cat > "$HOME/dotfiles/profiles/work.yaml" << 'YAML'
extends: base

vars:
  git:
    email: sarah@acme.com

packages:
  - name: docker
    install: brew install docker
  - name: node
    install:
      macos: brew install node
      linux: sudo apt-get install -y nodejs

configs:
  ~/.gitconfig: configs/work/.gitconfig
  ~/.npmrc: configs/work/.npmrc

ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read]
      deny: []
YAML

: > "$HOME/.mock-ai"
facet_apply work
echo "  same-profile AI narrowing exited cleanly"

assert_file_contains "$HOME/.mock-ai" "npx skills remove frontend-design -g -y"
echo "  cursor skill removed on same-profile narrowing"

assert_file_contains "$HOME/.cursor/cli-config.json" '"allow": []'
assert_file_not_contains "$HOME/.cursor/cli-config.json" '"Read(**)"'
echo "  cursor permissions cleared on same-profile narrowing"

assert_file_not_contains "$HOME/.cursor/mcp.json" '"playwright"'
echo "  cursor MCP removed on same-profile narrowing"

# Test 4: Switch to minimal profile — skills should be orphan-removed
: > "$HOME/.mock-ai"
facet_apply minimal
echo "  apply minimal profile exited cleanly"

assert_file_contains "$HOME/.mock-ai" "npx skills remove frontend-design"
echo "  orphan skill removed on profile switch"

assert_file_contains "$HOME/.mock-ai" "claude mcp remove playwright --scope user"
assert_file_contains "$HOME/.mock-ai" "claude mcp remove github --scope user"
echo "  orphan MCPs removed on profile switch at user scope"

assert_json_field "$HOME/.facet/.state.json" '.profile' 'minimal'
echo "  state shows minimal profile"

# Test 5: Dry-run with AI — should show preview without side effects
: > "$HOME/.mock-ai"
facet -c "$HOME/dotfiles" -s "$HOME/.facet" apply --dry-run minimal > "$HOME/.dryrun-output" 2>&1
assert_file_contains "$HOME/.dryrun-output" "AI configuration"
assert_file_contains "$HOME/.dryrun-output" "No changes were made"
if [ -s "$HOME/.mock-ai" ]; then
    echo "  ASSERT FAIL: dry-run should not execute AI commands"
    cat "$HOME/.mock-ai"
    exit 1
fi
echo "  dry-run shows AI preview without side effects"

# Test 6: Force apply triggers full unapply+reapply
: > "$HOME/.mock-ai"
facet -c "$HOME/dotfiles" -s "$HOME/.facet" apply --force minimal
echo "  force apply exited cleanly"
assert_json_field "$HOME/.facet/.state.json" '.profile' 'minimal'
echo "  force reapply succeeded"

# Test 7: Status shows AI information
facet_status > "$HOME/.status-output" 2>&1
assert_file_contains "$HOME/.status-output" "AI"
assert_file_contains "$HOME/.status-output" "claude-code"
echo "  status shows AI section"

# Test 8: Deny permissions
cat > "$HOME/dotfiles/profiles/deny-test.yaml" << 'YAML'
extends: base

ai:
  agents: [claude-code]
  permissions:
    claude-code:
      allow: [Read, Edit]
      deny: [Bash]
YAML

: > "$HOME/.mock-ai"
facet_apply deny-test
assert_file_exists "$HOME/.claude/settings.json"
assert_json_field "$HOME/.facet/.state.json" '.ai.permissions."claude-code".deny[0]' 'Bash'
echo "  deny permissions applied correctly"

# Test 9: Codex agent receives MCPs
cat > "$HOME/dotfiles/profiles/codex-test.yaml" << 'YAML'
extends: base

ai:
  agents: [codex]
  mcps:
    - name: playwright
      command: npx
      args: ["@anthropic/mcp-playwright"]
YAML

: > "$HOME/.mock-ai"
facet_apply codex-test
assert_file_exists "$HOME/.codex/config.toml"
assert_file_contains "$HOME/.codex/config.toml" "[mcp_servers.playwright]"
echo "  codex agent MCP registered in config.toml"
