# AI Configuration

## Overview

The `ai:` block configures AI coding agents during `facet apply`.

```yaml
ai:
  agents:
    - claude-code
    - cursor
    - codex
    - pi
```

facet has built-in providers for `claude-code`, `cursor`, and `codex`. Skills
can also target `pi`, which is the agent name accepted by `npx skills`. If a
config names another agent without a provider, facet warns and skips that agent.

## Pi Extensions

Manage Pi coding-agent extensions under `ai.pi.extensions`:

```yaml
ai:
  pi:
    extensions:
      - source: npm:pi-intercom
      - source: npm:@company/internal-pi-extension
        install_env:
          NPM_CONFIG_REGISTRY: https://bnpm.byted.org
```

Pi extensions are reconciled during the `ai` apply stage. Facet installs declared
extensions with `pi install <source>` only when they were not already recorded in
Facet state, and removes previously Facet-managed extensions that are no longer
declared with `pi remove <source>`. Optional per-extension `install_env` values
are passed only to that extension's install command. Manually installed Pi
extensions are left untouched. Use `facet apply --force` to reinstall unchanged
managed Pi extensions. `ai.pi`
may be used without `ai.agents` when no agent-scoped permissions, skills, or MCPs
are configured.

## Permissions

Permissions are configured per agent using that agent's native terms.

```yaml
ai:
  agents:
    - claude-code
    - cursor
  permissions:
    claude-code:
      allow:
        - Read
        - Edit
      deny:
        - Bash
    cursor:
      allow:
        - Read(**)
        - Write(**)
      deny: []
```

If an agent is listed in `ai.agents` but omitted from `ai.permissions`, facet
applies an empty permission set for that agent.

## Skills

Install skills from a source, optionally scoped to specific agents:

```yaml
ai:
  agents:
    - claude-code
    - cursor
  skills:
    # All skills from this source (omit skills list)
    - source: "@anthropic/claude-code-skills"
    # Specific skills from a source
    - source: "@my-org/custom-skills"
      skills:
        - deploy-helper
      agents:
        - claude-code
```

Each skill entry has:

- `source`: package or path passed to the skills installer (see formats below)
- `skills`: optional list of skill names from that source. **If omitted, all skills
  from the source are installed** using the skills CLI wildcard (`--skill "*"`) while
  preserving facet's explicit agent list.
- `agents`: optional list limiting installation to specific agents. When omitted,
  skills are installed for `claude-code`, `cursor`, `codex`, and `pi` only (not every
  agent in `ai.agents`). To target other agents, list them explicitly. The Pi
  agent name passed to `npx skills` is `pi`.

facet reconciles skills on every `facet apply`. If a previously managed source is
removed or narrowed to fewer skills, facet removes the no-longer-declared skills
for the affected agents before writing the new state. This includes entries that
were previously installed as "all skills from this source."

Codex and Cursor read shared storage at `~/.agents/skills`. When a previously
Facet-managed skill is removed entirely, facet runs `npx skills remove <names>
-g -y` without agent filters. This removes that skill across all CLI-supported
agents, including its shared directory and lock entry, even if another detected
agent such as Cline can discover it. Other skill names are left alone.
Removal runs from an empty temporary directory to protect same-named project
skills from the CLI's fallback paths.

When a skill remains desired by Codex or Cursor, removals stay scoped to the
dropped agents and shared storage is retained. Shared agents cannot be isolated
from each other through this directory. When an agent reduction leaves only
Claude Code and/or Pi, facet removes the skill globally and reinstalls the
remaining native copies with `--copy`. Native-only installs always use `--copy`;
this flag cannot avoid shared storage for Codex or Cursor.

The skills CLI maintains its own lock at `~/.agents/.skill-lock.json`, or
`$XDG_STATE_HOME/skills/.skill-lock.json` when configured. facet never edits it.
After global removal, facet checks both the lock and `skills list -g --json`.
Failed cleanup preserves the source's existing Facet state and defers its
installs so the next apply can retry. No additional state file is created.
Legacy all-source records retain concrete retry names even if the CLI already
deleted their lock entries. Agent-scoped removals still rely on the CLI's exit
status; they do not receive the global absence check because shared copies remain.

After installation, facet records names confirmed by both the lock and actual
CLI inventory, including native copies. A stale lock entry alone is not proof
of installation. Missing names trigger a warning. If verification cannot be
read, named installs fall back to recording requested names with a warning;
all-source installs cannot infer names and warn instead. Cleanup uses the
existing `.state.json` to identify previously managed skills; unrelated lock
entries and remnants already absent from Facet state are not automatically
pruned.

### Skill Source Formats

The `source` field supports any format accepted by the skills CLI:

| Format | Example |
|--------|---------|
| GitHub shorthand | `owner/repo` |
| HTTPS URL | `https://github.com/org/repo` |
| SSH URL | `git@github.com:org/private-repo.git` |
| Local path | `./my-local-skills` or `/absolute/path` |

Private repositories work via system-level git authentication. Ensure your SSH
keys are loaded (`ssh-agent`) or your git credentials are configured
(`gh auth login`) before running `facet apply`.

### Skills Management

Check for available skill updates:

    facet ai skills check

Update all installed skills to their latest versions:

    facet ai skills update

These commands pass through to the underlying skills CLI (`npx skills`) and
operate on all globally installed skills, not just those managed by facet.

## MCP Servers

Configure MCP servers, optionally scoped to specific agents:

```yaml
ai:
  agents:
    - claude-code
    - cursor
  mcps:
    - name: filesystem
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    - name: postgres
      command: npx
      args: ["-y", "@anthropic/mcp-postgres"]
      env:
        DATABASE_URL: ${facet:db_url}
      agents:
        - claude-code
```

Each MCP entry has:

- `name`: identifier used for merge and state tracking
- `command`: server executable
- `args`: optional argument list
- `env`: optional environment variables; values support `${facet:...}`
- `agents`: optional list limiting the MCP to specific agents. When omitted,
  MCPs apply to provider-backed agents and do not target `pi`.
- `startup_timeout_sec`: optional Codex-only startup timeout in seconds

For Claude Code, MCPs are always registered at user scope
(`claude mcp add --scope user ...`) so they are available across every project
on the machine. Removal and the idempotent re-add on conflict also target user
scope. Cursor and Codex MCPs are written directly to each agent's own config
file and are user-wide by nature.

There is no separate overrides section. Per-agent permissions live directly under
`ai.permissions`.
