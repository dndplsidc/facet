# Packages

## Format

Each package entry needs `name` and `install`. An optional `check` command can skip
installation when the package is already present.

```yaml
packages:
  - name: ripgrep
    install: brew install ripgrep

  - name: lazydocker
    install:
      macos: brew install lazydocker
      linux: go install github.com/jesseduffield/lazydocker@latest

  - name: xcode-tools
    install:
      macos: xcode-select --install
```

## Check Command

Add a `check` field to skip installation when a package is already installed.
If the check command exits 0, facet marks the package as `already_installed` and
skips the install command. This speeds up repeated applies.

```yaml
packages:
  - name: ripgrep
    check: which rg
    install: brew install ripgrep

  - name: lazydocker
    check:
      macos: which lazydocker
      linux: which lazydocker
    install:
      macos: brew install lazydocker
      linux: go install github.com/jesseduffield/lazydocker@latest
```

`check` supports the same formats as `install`: a plain string (runs on all OSes)
or a per-OS map. If `check` is omitted, the install command always runs.

## Behavior

- facet evaluates package entries on every `facet apply`
- If a `check` command is defined and succeeds (exit 0), the install is skipped
- Commands should be idempotent
- Failed installs are recorded but do not stop later installs
- If a package has no install command for the current OS, it is marked as skipped
- `${facet:...}` variables are resolved in both `check` and `install` before execution

## Rules

- facet is additive only; it never uninstalls packages
- There is no package manager detection; you provide the full command
- Base and profile packages are unioned by `name`
- If the same package name appears in multiple layers, the later layer wins

## Platform Selection and Variables

During apply and dry-run, only the current OS's install and check commands undergo
variable substitution. A package without an installer for that OS is skipped before
its check is resolved. Package skips do not implicitly skip configs or hooks;
use their own OS maps to declare platform-specific behavior.
