# Commands

## Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--config-dir` | `-c` | Current directory | Path to the facet config repo |
| `--state-dir` | `-s` | `~/.facet` | Path to the machine-local state directory |
| `--verbose` | `-v` | false | Stream stage, item, and duration diagnostics during apply |

## `facet scaffold`

Create a new config repo in the current directory and initialize the state directory.

```bash
facet scaffold
```

Creates `facet.yaml`, `base.yaml`, `profiles/`, and `configs/` in the config repo.
Creates `.local.yaml` in the state directory if it does not already exist.

## `facet apply <profile>`

Apply a configuration profile.

```bash
facet apply work
facet apply work --dry-run
facet apply work --force
facet apply work --verbose
facet apply work --skip-failure
facet apply work --stages configs,packages
```

Flags:

- `--dry-run`: preview the resolved actions without writing changes
- `--force`: replace conflicting non-facet files and unapply previous state first when needed
- `--skip-failure`: warn on deploy failures instead of rolling back immediately
- `--stages`: comma-separated list of stages to run (default: all)
- `--verbose` / `-v`: stream stage, item, and duration diagnostics as apply runs

Valid stages (in execution order):

| Stage | Description |
|-------|-------------|
| `configs` | Deploy symlinks, templates, and copied remote-base configs |
| `pre_apply` | Run pre_apply scripts |
| `packages` | Install packages |
| `post_apply` | Run post_apply scripts |
| `ai` | Apply AI configuration, including managed Pi extensions |

What it does:

1. Loads `facet.yaml`, the selected profile, resolves its base from `extends`, and loads `.local.yaml`
2. Merges the three layers
3. Resolves `${facet:...}` variables
4. Unapplies previous state if switching profiles or using `--force`
5. Deploys config files (if `configs` stage)
6. Runs pre_apply scripts (if `pre_apply` stage)
7. Installs packages (if `packages` stage)
8. Runs post_apply scripts (if `post_apply` stage)
9. Applies AI configuration, including managed Pi extensions (if `ai` stage)
10. Writes `.state.json`

Apply and dry-run select `macos` or `linux` after merging, before resolving variables
or paths. Both report platform skips for requested stages; dry-run lists only the
requested stages. See `facet docs deploy` and `facet docs scripts` for OS maps.

## `facet status`

Show the current applied state.

```bash
facet status
```

This reads `.state.json` and reports the active profile, package results, deployed
configs, and validity checks.

## `facet docs [topic]`

Show embedded documentation.

```bash
facet docs
facet docs config
```

Run `facet docs` to list the available topics.

## `facet ai skills check`

Check for available skill updates.

    facet ai skills check

Runs `npx skills check` and streams the output. Shows which installed skills
have newer versions available.

## `facet ai skills update`

Update all installed skills to their latest versions.

    facet ai skills update

Runs `npx skills update` and streams the output. Re-installs any skills that
have updates available.

## Exit Codes

- `0`: success
- `1`: command failed
