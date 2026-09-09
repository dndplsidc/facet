# facet v1 — Design Spec

This document captures all design decisions made during the brainstorming phase. It supersedes any conflicting details in `facet-requirements.md` and `facet-implementation-plan.md`.

---

## 1. Scope

### In scope (v1)
- Config loading, merging, variable resolution
- Config deployment (symlink / template / copy) with unapply/apply model
- Package installation (user-provided commands)
- AI tool configuration (permissions, skills, MCPs for supported agents)
- CLI commands: `facet init`, `facet apply <profile>`, `facet status`
- Colored, structured console output
- Comprehensive unit tests

### Deferred
- `facet diff` command
- `facet doctor` command
- `.facet.d/` shell snippet directory — removed entirely, user manages their own shell sourcing

---

## 2. Directory Architecture

facet separates the **config repo** (portable, git-managed) from the **state directory** (machine-local, never committed).

### Config directory (the git repo)

Location: wherever the user clones or creates it. Detected via `facet.yaml` in the current directory, or specified with `--config-dir` / `-c`.

```
~/dotfiles/                     # or any path
├── facet.yaml                  # metadata — marker file + version
├── base.yaml                   # shared foundation config
├── profiles/
│   ├── work.yaml
│   └── personal.yaml
└── configs/
    ├── .gitconfig              # may contain ${facet:...} variables
    ├── .zshrc
    └── work/
        └── .gitconfig
```

### State directory (machine-local)

Location: `~/.facet/` by default, overridden with `--state-dir` / `-s`.

```
~/.facet/
├── .state.json                 # written by facet apply
└── .local.yaml                 # machine-specific secrets/vars — must exist
```

### Config directory detection

When `--config-dir` is not provided, facet checks if the current working directory contains `facet.yaml`. If not found, it exits with:

> `"Not a facet config directory (facet.yaml not found). Use -c to specify the config directory, or run facet init to create one."`

---

## 3. File Formats

### 3.1 facet.yaml

Metadata only. Serves as the config directory marker.

```yaml
min_version: "0.1.0"
```

Future metadata fields will be added here as needs arise.

### 3.2 base.yaml

The shared foundation config for a local repo. Profiles may also extend an equivalent base from a git repo, a local directory, or another local base file. Same schema as profile files, minus the `extends` field.

```yaml
vars:
  git_name: Sarah Chen

packages:
  - name: git
    install: brew install git
  - name: ripgrep
    install: brew install ripgrep

configs:
  ~/.gitconfig: configs/.gitconfig
  ~/.zshrc: configs/.zshrc
```

### 3.3 Profile files (profiles/*.yaml)

```yaml
extends: ./base.yaml
# or:
# extends: https://github.com/me/personal-dotfiles.git
# extends: https://github.com/me/personal-dotfiles.git@main
# extends: git@github.com:me/personal-dotfiles.git@v1.2.0
# extends: /Users/me/personal-dotfiles
# extends: /Users/me/personal-dotfiles/base.yaml

vars:
  git_email: sarah@acme.com

packages:
  - name: docker
    install: brew install docker

configs:
  ~/.gitconfig: configs/work/.gitconfig   # overrides base's .gitconfig
  ~/.npmrc: configs/work/.npmrc           # adds new config
```

`extends` is a locator string. Supported forms in v1 are HTTPS git, SSH git, local directory, and local base file. Git locators may include an optional `@ref` suffix for branch, tag, or commit. Omitting the ref uses the remote default branch.

### 3.4 .local.yaml

Same schema as base.yaml. Lives in the state directory. Must exist — missing `.local.yaml` is a fatal error.

Typically contains secrets:

```yaml
vars:
  acme_db_url: postgres://user:secret@localhost:5432/acme
```

### 3.5 .state.json

Written by `facet apply`, read by `facet status`.

```json
{
  "profile": "acme",
  "applied_at": "2025-03-15T10:30:00Z",
  "facet_version": "0.1.0",
  "packages": [
    {"name": "ripgrep", "install": "brew install ripgrep", "status": "ok"},
    {"name": "lazydocker", "install": "brew install lazydocker", "status": "failed", "error": "..."}
  ],
  "configs": [
    {"target": "~/.gitconfig", "source": "configs/work/.gitconfig", "strategy": "template"},
    {"target": "~/.zshrc", "source": "configs/.zshrc", "strategy": "symlink"}
  ]
}
```

---

## 4. Variable System

### Syntax

`${facet:var_name}` — no relation to Go's `text/template`. Pure string substitution.

### Nested vars with dot notation

Vars support nested maps, referenced with dot notation:

```yaml
vars:
  git:
    name: Sarah Chen
    email: sarah@acme.com
  aws:
    region: us-east-1
```

Referenced as `${facet:git.email}`, `${facet:aws.region}`.

Arbitrary depth is allowed. Recommended to keep it to 2-3 levels.

### Resolution scope

`${facet:...}` is resolved in **all** string values across the merged config:
- Config file contents (templates)
- Package `check` and `install` commands
- Any future string field

Config source paths (right side of `configs:`) are also resolved.
Config target paths (left side of `configs:`) are NOT resolved by `${facet:...}` — they use environment variable expansion instead (see Section 6).

### No recursion

If a var's value contains `${facet:...}`, it is treated as a literal string. No recursive resolution.

### Undefined variables

Any `${facet:var_name}` referencing an undefined variable is a **fatal error**. The error message must name the undefined variable and suggest where to define it.

---

## 5. Merge Rules

Three layers are merged in order: resolved base from `extends` → `profiles/<name>.yaml` → `.local.yaml`. Later layers win on conflicts.

### 5.1 vars — deep merge, last writer wins per leaf key

```yaml
# base.yaml
vars:
  git:
    name: Sarah
    editor: nvim

# profile
vars:
  git:
    email: sarah@acme.com

# result
vars:
  git:
    name: Sarah
    editor: nvim
    email: sarah@acme.com
```

**Type conflict is a fatal error.** If base defines `git` as a map and profile defines `git` as a string (or vice versa), facet exits with a clear error.

### 5.2 packages — union by name, last writer wins on conflict

```yaml
# base
packages:
  - name: git
    install: brew install git

# profile (same name, different install)
packages:
  - name: git
    install: sudo apt-get install -y git

# result: profile's install wins
```

### 5.3 configs — shallow merge, last writer wins on same target

```yaml
# base
configs:
  ~/.gitconfig: configs/.gitconfig

# profile
configs:
  ~/.gitconfig: configs/work/.gitconfig   # overrides base

# result: profile's source wins for ~/.gitconfig
```

---

## 6. Config Deployment

### Target path expansion

Config target paths are absolute and support environment variable expansion:

- `~` → expands to `$HOME`
- `$VAR_NAME` or `${VAR_NAME}` → expands from OS environment variables
- After expansion, the path must be absolute — otherwise fatal error

This is **environment variable expansion**, not `${facet:...}` substitution. The two systems are separate.

### Source path constraints

Source paths are resolved relative to the owning source root for the layer that defined them. Local profiles use the config directory. Local base files use the directory containing that file. Git-based bases use the temporary clone root and must use relative source paths rooted inside that clone. Relative paths that escape the owning source root are rejected with a fatal error. Absolute paths are allowed after variable substitution only for local layers.

### Deploy strategy (auto-detected)

| Source type | Contains `${facet:` | Strategy |
|---|---|---|
| File | No | Symlink target → source |
| File | Yes | Substitute variables, write rendered content as regular file |
| Directory | N/A | Symlink target → source |
| File or directory from a git-based base without `${facet:` | N/A | Copy target contents into place |

Facet reads every source file to check for `${facet:`. Directories are symlinked for local sources and copied for git-based bases.

### Symlink behavior

1. Symlink exists, points to correct source → no-op
2. Symlink exists, points to wrong source → replace it only when facet can verify ownership or when `--force` is set
3. Regular file exists at target → prompt user to replace. With `--force`, replace without asking.
4. Target doesn't exist → create symlink, creating parent directories as needed (`mkdir -p`)

### Template behavior

Templated files are always rewritten on every apply (vars may have changed). No staleness check.

---

## 7. Package Installation

### Package entry format

Every package is an object with `name` (required) and `install` (required). An optional `check` field can skip installation when the package is already present. No plain string shorthand. No `version` field. No `packages_gui`.

```yaml
packages:
  # Same command on all OSes
  - name: ripgrep
    check: which rg
    install: brew install ripgrep

  # Per-OS commands
  - name: lazydocker
    check:
      macos: which lazydocker
      linux: which lazydocker
    install:
      macos: brew install lazydocker
      linux: go install github.com/jesseduffield/lazydocker@latest

  # Single OS only — skipped with warning on other OSes
  - name: xcode-tools
    install:
      macos: xcode-select --install
```

`check` supports the same formats as `install`: a plain string (runs on all OSes) or a per-OS map. If the check command exits 0, the package is marked `already_installed` and the install command is skipped.

### Execution behavior

- If `check` is defined and succeeds (exit 0), the install is skipped.
- If `check` is omitted, the install command runs every time `facet apply` is called.
- Install commands must be idempotent. Most package managers handle this natively.
- Failed installs are **non-fatal** — facet logs the failure and continues with other steps.
- No automatic package manager detection. No built-in Homebrew/apt/pacman backends.

### No package uninstallation

facet is additive only. It never uninstalls packages.

---

## 8. Apply Model

### The unapply/apply cycle

Profile switching is: **unapply the current profile, then apply the new one.**

**Unapply** reads `.state.json` and removes everything it recorded:
- Deletes symlinks created by facet
- Deletes templated files created by facet
- Does NOT uninstall packages

**Apply** deploys the new profile:
- Resolves the base from `extends`
- Runs all package install commands
- Deploys all config files (symlink, template, or copy)
- Writes new `.state.json`

With `--verbose`, each apply step and selected stage item reports an outcome and elapsed duration; non-verbose output remains the final summary only.

### Same-profile reapply

Running `facet apply <same-profile>` just applies — overwrites configs to converge to the declared state. No unapply needed (the configs are the same targets).

### --force flag

`--force` = unapply + apply, even for the same profile. Gives a clean slate. Also skips user prompts for conflicting files (replaces without asking, but still logs). For managed Pi extensions, `--force` also reinstalls declared extensions that normal apply would skip as already managed.

### --dry-run flag

`--dry-run` runs the full load → merge → resolve pipeline (steps 1–7), catching any YAML, profile, or variable errors, then prints what would happen without making any changes:

- Packages that would be installed (with per-OS command resolution)
- Configs that would be deployed (with auto-detected strategy — symlink, template, or copy)
- Configs that would be removed (if switching profiles or using `--force`)

No side effects — no package installs, no symlinks, no file writes, no state changes. Steps 8–11 are skipped entirely.

### --skip-failure flag

Changes config deployment failure behavior from rollback to warn-and-continue (same as package install behavior).

### Error handling during apply

| Step | Failure behavior |
|---|---|
| 1. Find config dir | Fatal |
| 2. Load facet.yaml | Fatal |
| 3. Load profiles/<name>.yaml | Fatal |
| 4. Resolve the base from `extends` | Fatal |
| 5. Load .local.yaml | Fatal if missing |
| 6. Merge layers | Fatal on type conflicts |
| 7. Resolve ${facet:...} | Fatal on undefined var |
| 8. Write canary to .state.json | Fatal (early permission/disk check) |
| 9. Install packages | Non-fatal, log failures, continue |
| 10. Deploy configs | Default: rollback on failure. With --skip-failure: warn and continue |
| 11. Write final .state.json | Fatal |

### Rollback on config deploy failure

If a config deployment fails (and `--skip-failure` is not set), facet rolls back configs it already deployed during this run. The rollback list is maintained in-memory during the apply — the same data structure that eventually gets written to `.state.json`.

---

## 9. CLI Commands

### Global flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--config-dir` | `-c` | Current working directory | Path to the facet config repo |
| `--state-dir` | `-s` | `~/.facet/` | Path to the machine-local state directory |
| `--verbose` | `-v` | false | Stream stage, item, and duration diagnostics during apply |

### facet init

Creates a new config repo in the current directory and initializes the state directory.

**In current directory (config repo):**
- `facet.yaml` — with `min_version`
- `base.yaml` — commented scaffold with examples
- `profiles/` — empty directory
- `configs/` — empty directory

**In state directory (`~/.facet/` or `--state-dir`):**
- `.local.yaml` — created only if it doesn't exist (empty vars scaffold)

Does NOT run `git init`. The user manages their own git repo.

Error if `facet.yaml` already exists in the current directory.

### facet apply \<profile\>

Applies a profile. See Section 8 for the full apply model. The base from `extends` is resolved before merge, and git-based bases are cloned fresh for each apply then cleaned up afterward.

**Flags:**
- `--dry-run` — preview what would happen without making changes
- `--force` — unapply + apply, skip prompts
- `--skip-failure` — warn on config deploy failure instead of rollback
- `--verbose` / `-v` — stream stage, item, and duration diagnostics as apply runs

**Exit codes:**
- 0: success
- 1: fatal error

### facet status

Reads `.state.json` and displays the current state with validity checks.

**Output includes:**
- Active profile name and when it was last applied
- List of packages with their install commands and status (ok/failed)
- List of configs with target path, source path, strategy (symlink/template/copy), and current validity (symlink still valid, file or directory still exists with the expected type)

**Validity checks** (always run, but cleanly encapsulated for future refactoring):
- Are all symlinks still pointing to the correct source?
- Do all templated files still exist at the target path?
- Do copied remote-base files still exist at the target path?

If no profile has been applied (no `.state.json`), print a message suggesting `facet apply <profile>`.

---

## 10. Console Output

All CLI output uses:
- **Colored output** — green for success, yellow for warnings, red for errors
- **Structured formatting** — tables for lists (packages, configs), clear section headers
- **Status indicators** — checkmarks, crosses, warning symbols

Falls back to plain text if the terminal doesn't support colors.

---

## 11. Pi Extensions

`facet apply` reconciles Pi coding-agent extensions from `ai.pi.extensions`.
Each extension entry is an object with a required `source` and optional
`install_env` map. Newly declared extensions are installed with `pi install
<source>`. If `install_env` is present, those environment values are passed only
to that extension's install command. Declared extensions already recorded in
Facet state are not reinstalled during a normal apply; `facet apply --force`
reinstalls them. Previously managed extensions that are no longer declared are
removed with `pi remove <source>`.

Removal is state-scoped: Facet only removes extensions recorded in its own
`.state.json`; manually installed Pi extensions are left untouched. Pi extension
reconciliation belongs to the `ai` stage, so `facet apply <profile> --stages ai`
applies AI permissions, skills, MCPs, and Pi extensions together.

## 12. AI Configuration

`facet apply` reconciles AI configuration for supported agents. Permissions and
MCP registrations are applied per agent. Skill entries may either list explicit
skill names or omit the `skills` list to mean "all skills from this source."
All-skills installs use the skills CLI wildcard (`--skill "*"`) instead of the
CLI `--all` shortcut so facet's explicit agent scope is preserved. When a skill
entry omits `agents`, facet targets the default skill agents: `claude-code`,
`cursor`, `codex`, and `pi`. The Pi skill agent name passed to `npx skills` is
`pi`; Pi does not add permissions or MCP behavior. Unscoped MCP entries do not
target `pi`.

For Claude Code, MCP servers are registered at user scope via
`claude mcp add --scope user` (and removed via
`claude mcp remove <name> --scope user`). This makes them available across every
project on the machine and keeps add/remove operating on the same scope so
orphan cleanup works correctly. Cursor and Codex MCPs are written to each
agent's own user-wide config file.

AI skill reconciliation is stateful: when a previously managed source is removed
or narrowed on a later apply, facet removes only the no-longer-declared skills
for the affected agents before recording the new state.

Codex and Cursor consume shared storage at `~/.agents/skills`. Fully removed
Facet-managed skills are grouped by source and removed through `npx skills
remove <names> -g -y` without agent filters. Removal applies to those skill names
across all agents supported by the CLI, including consumers outside the profile
such as Cline. This lets the CLI remove both shared files and lock metadata.
Unrelated skill names are not pruned. Existing Facet state identifies ownership;
the global lock alone cannot establish which skills Facet manages.
Every removal command runs in an empty temporary directory, preventing CLI
fallback paths for agents without global directories from deleting same-named
project-local skills in the caller's working directory.

If a skill remains desired by a shared agent, removal uses the complete dropped
agent set and retains shared storage. Unknown agent types conservatively retain
shared storage as well. An agent reduction leaving only Claude Code and/or Pi
uses a global removal followed by native-only installation with `--copy`.
Native-only installs always use copies; shared-agent installs cannot avoid
`.agents/skills` via `--copy`.

facet reads the CLI's global lock at `~/.agents/.skill-lock.json`, or
`$XDG_STATE_HOME/skills/.skill-lock.json`. Only the CLI writes this file. Global
cleanup succeeds only when both the lock entry and the CLI installed inventory
are absent. Failed cleanup keeps the source's previous records in the existing
Facet state and defers its installs for retry; failed profile-switch unapply
also retains previous AI state for reconciliation. No extra ownership store is
introduced. Legacy all-source state resolves cleanup candidates from the lock,
including stale entries, and preserves concrete failed names in existing state
even if the lock entry disappears. Failed unapply expands legacy skill records
in its supplied previous state so the caller can retry them. Agent-scoped
removals still rely on CLI exit status; the global absence check does not apply
while a shared copy remains desired.

Post-install verification intersects source-matching lock entries with `skills
list -g --json`, which also recognizes native copies. Stale lock-only entries
are excluded from new Facet state. Missing names warn; unreadable verification
falls back to requested names for named installs and warns without inferring
names for all-source installs. Historical remnants already absent from Facet
state require explicit cleanup and are not globally pruned.

The hermetic mock E2E suites cover reconciliation and retry behavior. A separate
`make test-skills-integration` gate runs the actual Facet binary with the pinned
skills CLI 1.5.25, an isolated home, and a local Git fixture accessed through URL
rewriting. It verifies shared-directory and lock deletion with Cline detected,
native-copy transitions, dangling links, stale metadata, unrelated-skill
preservation, repeated apply/update, and XDG lock paths. CI runs it on macOS and
Linux; set `FACET_SKILLS_CLI` to the installed package's `bin/cli.mjs` to run locally.

---

## 13. Non-Requirements (v1)

| Feature | Why excluded |
|---|---|
| `facet diff` | Unapply/apply model makes preview less critical |
| `facet doctor` | Deferred |
| `.facet.d/` shell snippets | Removed — user manages their own shell sourcing |
| `facet init --from <repo>` | User clones their own repo |
| Package version field | User puts version in their install command |
| `packages_gui` | No distinction needed — user writes full install command |
| Auto-detect package manager | User provides install commands explicitly |
| Plain string package entries | `name` + `install` always required |
| Default profile | Profile argument always required for `facet apply` |
| Template logic (if/else/loops) | Just `${facet:...}` substitution |
| Recursive var resolution | Var values containing `${facet:...}` are literal |
| Multi-level extends | Single base locator only; no chained inheritance |
| Windows support | macOS and Linux only |
