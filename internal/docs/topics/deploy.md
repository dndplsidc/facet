# Config File Deployment

## The `configs` Block

`configs:` maps target paths on your machine to source paths owned by the layer that defines them.

```yaml
configs:
  ~/.gitconfig: configs/.gitconfig
  ~/.config/nvim: configs/nvim
  ~/.zshrc: configs/.zshrc
```

## Platform-Specific Configs

Each source can be a shared string or a map keyed by `macos` and `linux`:

```yaml
configs:
  ~/.gitconfig: configs/git/shared
  ~/.facet-profiles/zsh/conf.d/platform.sh:
    macos: configs/zsh/macos.sh
    linux: configs/zsh/ubuntu.sh
  ~/.aerospace.toml:
    macos: configs/aerospace/config.toml
  "~/Library/Application Support/Code/User/settings.json":
    macos: configs/vscode/settings.json
  ~/.config/Code/User/settings.json:
    linux: configs/vscode/settings.json
```

Facet uses `linux` for Ubuntu and other Linux distributions; it does not select by
distribution or architecture. Missing OS entries are explicitly reported as skipped
in apply and dry-run. Skipped entries do not require their source files, variables,
or target environment variables to exist. Config sources and OS values must be
non-empty strings; unknown OS keys, empty maps, and null values are errors.

Layers merge before OS selection. A later declaration for the same destination
replaces the entire source value, including any OS map. Different destinations
remain separate declarations, so scope each destination to its intended OS.

Only selected configs are recorded in deployment state. If a previously managed
config becomes inactive, a subsequent apply of the configs stage removes it using
the normal ownership checks. Dry-run lists that target as a removal candidate without
changing it. Applying only other stages leaves it untouched.

## Target Path Expansion

Target paths support environment expansion:

- `~` expands to your home directory
- `$VAR` and `${VAR}` expand from the OS environment

After expansion, the target path must be absolute.

## Source Path Rules

Source paths are resolved relative to the source root of the layer that defines them.
For local profiles that is usually the config repo. For a local base file it is the
directory containing that file. For a git-based base it is the temporary clone root.

Absolute paths are allowed after variable substitution for local layers. Git-based
bases must use relative source paths rooted inside the cloned base. Relative paths
that escape the owning source root with `../` are rejected.

Source paths can contain `${facet:...}` variables, which are resolved before use.

## Deploy Strategy

facet decides the strategy automatically:

| Source | Contains `${facet:` | Strategy |
|--------|---------------------|----------|
| File | No | Symlink |
| File | Yes | Template |
| Directory | N/A | Symlink |
| File or directory from a git-based base without `${facet:` | N/A | Copy |

You do not configure the strategy manually.

## Existing Target Behavior

1. Correct symlink already exists: no-op
2. Wrong symlink exists: replace it only if facet still owns that symlink or `--force` is set
3. Existing file or directory managed by facet: replace it unless the on-disk type has drifted in a way facet treats as unsafe
4. Existing file or directory not managed by facet: return an error unless `--force` is set
5. Missing target: create parent directories and deploy it

facet will also refuse to remove repointed symlinks or unexpected type changes during
unapply. Use `--force` only when you intentionally want facet to replace a conflicting
target.

## Template Behavior

Templated files are rendered with `${facet:...}` substitution and written as regular
files. They are rewritten on each apply.

Files and directories inherited from a git-based base are materialized into place so
they still work after the temporary clone is cleaned up.

## Profile Switching

When switching profiles, facet removes configs that were managed by the previous
profile but are no longer part of the new one.
