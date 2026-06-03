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
      - pi-interactive-shell
      - pi-lens
      - pi-subagents
```

Pi extensions are reconciled during the `ai` apply stage. Facet installs declared
extensions with `pi extension install <name>` only when they were not already
recorded in Facet state, and removes only previously Facet-managed extensions
that are no longer declared. Manually installed Pi extensions are left untouched.
Use `facet apply --force` to reinstall unchanged managed Pi extensions. `ai.pi`
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
  from the source are installed** (equivalent to `npx skills add <source> --all`).
- `agents`: optional list limiting installation to specific agents. When omitted,
  skills are installed for `claude-code`, `cursor`, `codex`, and `pi` only (not every
  agent in `ai.agents`). To target other agents, list them explicitly. The Pi
  agent name passed to `npx skills` is `pi`.

facet reconciles skills on every `facet apply`. If a previously managed source is
removed or narrowed to fewer skills, facet removes the no-longer-declared skills
for the affected agents before writing the new state. This includes entries that
were previously installed as "all skills from this source."

After installing named skills, facet verifies each one against the skill lock
(`~/.agents/.skill-lock.json`). Skills that are absent from the lock after
install are not recorded in state and trigger a warning; they may not exist in
the source. If the lock file is unreadable, facet records the requested names as
a fallback and warns.

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
