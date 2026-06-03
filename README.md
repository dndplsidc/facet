# facet

**One command to set up any machine. Every time.**

facet is a developer environment manager that keeps your dotfiles, packages, and configs in sync across every machine you touch. Describe your setup once in YAML, switch between profiles instantly — work laptop, personal desktop, CI runner, whatever.

No more "let me spend an afternoon setting up my new laptop." No more "which machine has my latest .zshrc?" No more brittle install scripts that break after six months.

```sh
facet apply work    # packages installed, configs deployed, done
```

## Why facet?

**Existing tools solve pieces of the puzzle.** GNU Stow symlinks dotfiles but doesn't install packages. Homebrew Bundle installs packages but doesn't manage configs. Chezmoi manages dotfiles but has its own templating language to learn.

**facet solves the whole thing:**

| Feature | Stow | chezmoi | Homebrew Bundle | **facet** |
|---------|------|---------|-----------------|-----------|
| Dotfile deployment | ✅ | ✅ | — | ✅ |
| Template variables | — | ✅ (Go templates) | — | ✅ (simple `${facet:var}`) |
| Package installation | — | — | ✅ (macOS only) | ✅ (any OS) |
| Per-machine profiles | — | partial | — | ✅ |
| Profile switching | — | — | — | ✅ |
| Secret management | — | partial | — | ✅ (`.local.yaml`) |
| Single config format | ✅ | ❌ (Go templates) | ✅ | ✅ (plain YAML) |

**It's just YAML.** No DSL to learn. No Go template syntax. No scripting language. Write what you want, apply it.

## Quick Start

```sh
# Install
go install facet@latest

# Scaffold a new config repo
mkdir ~/dotfiles && cd ~/dotfiles
facet scaffold

# Edit base.yaml — your shared foundation
# Create profiles/work.yaml — your work machine
# Drop config files in configs/

# Apply it
facet apply work

# Check what's deployed
facet status
```

## How It Works

### Three layers, one merge

```
resolved base from extends →  shared packages, configs, vars
  + profiles/work.yaml     →  work-specific additions & overrides
  + ~/.facet/.local.yaml   →  machine secrets (never committed)
  ─────────────────────────
  = your fully resolved environment
```

Each layer can define **packages**, **configs**, and **variables**. Layers merge predictably: maps merge deeply, packages union by name (overlay wins on conflict), configs shallow-merge (same target → overlay wins).

### Your config repo

```
~/dotfiles/
├── facet.yaml                 # marker (facet finds your repo by this)
├── base.yaml                  # shared foundation
├── profiles/
│   ├── work.yaml              # extends: ./base.yaml
│   └── personal.yaml          # extends: git@github.com:me/dotfiles.git@main
├── configs/
│   ├── .zshrc                 # shared dotfile → symlinked
│   ├── .gitconfig             # has ${facet:...} → templated
│   └── work/
│       ├── .gitconfig         # work-specific override
│       └── .npmrc
```

```
~/.facet/                      # machine-local, never committed
├── .local.yaml                # secrets & per-machine vars
└── .state.json                # tracks what's applied
```

## Profiles

Each profile points `extends` at a base locator. Supported forms include:

- `base.yaml`
- `shared/base.yaml`
- `./shared-config`
- `https://github.com/me/personal-dotfiles.git`
- `https://github.com/me/personal-dotfiles.git@main`
- `git@github.com:me/personal-dotfiles.git@v1.2.0`

Merge order is:

1. resolved base from `extends`
2. selected profile
3. `~/.facet/.local.yaml`

When the base comes from git, facet clones it fresh for each `apply` and removes the clone after the run. Configs inherited from that git base are materialized into place instead of symlinked so they keep working after cleanup.

```yaml
# profiles/work.yaml
extends: ./base.yaml

vars:
  git:
    email: you@company.com
    signingkey: ABC123

packages:
  - name: docker
    install: brew install docker
  - name: node
    install: brew install node

configs:
  ~/.gitconfig: configs/work/.gitconfig
  ~/.npmrc: configs/work/.npmrc
```

## Variables

Use `${facet:var.name}` in config files and install commands. Variables resolve from the merged layer stack.

```yaml
# base.yaml
vars:
  git:
    name: Your Name
  editor: nvim
```

```
# configs/.gitconfig — facet auto-detects this needs templating
[user]
    name = ${facet:git.name}
    email = ${facet:git.email}
[core]
    editor = ${facet:editor}
```

Keep secrets out of git with `.local.yaml`:

```yaml
# ~/.facet/.local.yaml — gitignored, per-machine
vars:
  git:
    signingkey: DEADBEEF
  db_url: postgres://user:secret@localhost/dev
```

Undefined variables are a fatal error — no silent failures.

## Config Deployment

facet auto-detects the right strategy for each file:

| Source file | Strategy | Result |
|---|---|---|
| No `${facet:...}` | **Symlink** | `~/.zshrc → ~/dotfiles/configs/.zshrc` |
| Has `${facet:...}` | **Template** | Variables resolved, written as regular file |
| Directory | **Symlink** | Entire directory symlinked |
| Remote git base file or directory without `${facet:...}` | **Copy** | Materialized regular file or directory |

Config source paths are resolved relative to the layer that defined them. Local profiles still behave like today. Git-based bases are cloned to a temporary directory for the duration of `facet apply`, so inherited configs from that clone are copied or templated rather than symlinked.

When switching profiles, facet **cleans up the old profile first** — orphaned configs are removed, new ones deployed. It's always a clean slate.

## Package Installation

Packages are evaluated on every apply. Failures are **non-fatal** — facet warns and continues so one broken package doesn't block your entire setup.

Add a `check` command to skip installation when a package is already present — speeds up repeated applies:

```yaml
packages:
  # Same command everywhere
  - name: ripgrep
    check: which rg
    install: brew install ripgrep

  # Different command per OS
  - name: curl
    check:
      macos: which curl
      linux: which curl
    install:
      macos: brew install curl
      linux: sudo apt-get install -y curl
```

If `check` exits 0, the install is skipped. If `check` is omitted, the install always runs.

## Pi Extensions

facet manages Pi coding-agent extensions under the AI configuration:

```yaml
ai:
  pi:
    extensions:
      - pi-interactive-shell
      - pi-lens
      - pi-subagents
```

During the `ai` apply stage, Facet installs declared extensions that were not
already recorded in Facet state and removes only those previously managed by
Facet that are no longer declared. Manually installed Pi extensions are not
touched. Use `facet apply --force` to reinstall unchanged managed Pi extensions.

## AI Configuration

facet configures AI coding agents — permissions, skills, and MCP servers.

```yaml
# base.yaml
ai:
  agents: [claude-code, cursor, codex, pi]
  permissions:
    claude-code:
      allow: [Read, Edit, Bash]
  skills:
    - source: "owner/repo"              # all skills from this source
    - source: "other/repo"
      skills: [code-review]             # specific skills only
  mcps:
    - name: playwright
      command: npx
      args: ["@anthropic/mcp-playwright"]
```

Skills without an explicit `agents` list default to `claude-code`, `cursor`, `codex`, and `pi`; the agent name passed to `npx skills` for Pi is `pi`. Skills are always installed globally (`-g`). Source formats include GitHub repos, SSH URLs for private repos, and local paths. After named-skill installs, facet records only skills confirmed in `~/.agents/.skill-lock.json` and warns for requested names that are missing from the lock; if the lock cannot be read, facet warns and records the requested names as a fallback. When a skill entry is removed or narrowed on a later `facet apply`, facet also removes the no-longer-declared skills for that source and agent scope, including entries originally installed with `--all`. MCP servers are registered for Claude Code at user scope (`claude mcp add --scope user`), so they are available across every project on the machine; Cursor and Codex MCPs are written to each agent's own config file; unscoped MCPs do not target `pi`. See `facet docs ai` for details.

Manage installed skills with:

```sh
facet ai skills check    # check for available updates
facet ai skills update   # update all skills to latest
```

## Commands

### `facet scaffold`

Scaffold a new config repo. Creates the directory structure, marker file, and state directory.

```sh
facet scaffold
```

### `facet apply <profile>`

The main event. Resolves the base from `extends`, merges layers, deploys configs, runs scripts, installs packages, and records state.

```sh
facet apply work
facet apply work --dry-run         # preview what would happen, no side effects
facet apply work --force           # overwrite conflicts and reinstall unchanged Pi extensions
facet apply work --verbose         # stream stage, item, and duration diagnostics
facet apply work --skip-failure    # warn on deploy errors instead of rolling back
facet apply work --stages configs,packages  # run only specific stages
```

Switching profiles is just another apply:

```sh
facet apply personal    # old configs cleaned up, new ones deployed
```

### `facet status`

See what's active — profile, packages, configs, and whether everything is still valid.

```sh
facet status
```

### Global Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--config-dir` | `-c` | current directory | Path to facet config repo |
| `--state-dir` | `-s` | `~/.facet` | Machine-local state directory |
| `--verbose` | `-v` | false | Stream stage, item, and duration diagnostics during apply |

## Design Principles

- **Predictable** — YAML in, deterministic output. No magic, no surprises.
- **Composable** — Base + profile + local. Three layers, clear precedence.
- **Safe** — Undefined vars fail loudly. Unmanaged files aren't touched. Rollback on failure.
- **Minimal** — One binary, three dependencies (Cobra, yaml.v3, testify). No daemon, no background process.

## License

MIT
