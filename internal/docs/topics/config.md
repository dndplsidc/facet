# Config Format

## Directory Layout

facet separates the config repo from the machine-local state directory.

### Config repo

```text
~/dotfiles/
├── facet.yaml
├── base.yaml
├── profiles/
│   ├── work.yaml
│   └── personal.yaml
└── configs/
    ├── .gitconfig
    ├── .zshrc
    └── work/
        └── .gitconfig
```

When `--config-dir` is not provided, facet uses the current directory and expects
to find `facet.yaml` there.

### State directory

```text
~/.facet/
├── .state.json
└── .local.yaml
```

Override it with `--state-dir` or `-s`.

## `facet.yaml`

This is metadata and the marker file for a facet config repo.

```yaml
min_version: "0.1.0"
```

## `base.yaml`

Shared configuration that profiles can extend locally, or replicate in another directory or git repo.

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

ai:
  pi:
    extensions:
      - source: npm:pi-intercom
      - source: npm:@company/internal-pi-extension
        install_env:
          NPM_CONFIG_REGISTRY: https://bnpm.byted.org
```

## `profiles/<name>.yaml`

```yaml
extends: ./base.yaml

vars:
  git_email: sarah@work.com

packages:
  - name: docker
    install: brew install docker

configs:
  ~/.gitconfig: configs/work/.gitconfig
  ~/.npmrc: configs/work/.npmrc
```

`extends` is a locator string. Supported forms include:

- `base.yaml`
- `shared/base.yaml`
- `./shared-config`
- `https://github.com/me/personal-dotfiles.git`
- `https://github.com/me/personal-dotfiles.git@main`
- `git@github.com:me/personal-dotfiles.git@v1.2.0`

## `.local.yaml`

This lives in the state directory and must exist. It usually holds machine-local
values such as secrets.

```yaml
vars:
  acme_db_url: postgres://user:secret@localhost:5432/acme
```

## Field Reference

All config layers use this schema:

| Field | Type | Description |
|-------|------|-------------|
| `extends` | string | Profile files only. Base locator. Supports local files, local directories, and git locators. |
| `vars` | map[string]any | Variables used by `${facet:...}` substitution. Supports nested maps. |
| `packages` | list of PackageEntry | Package install entries (with optional `check`). See `facet docs packages`. |
| `configs` | map[string]string | Target path to source path. See `facet docs deploy`. |
| `pre_apply` | list of ScriptEntry | Scripts run before package install. See `facet docs scripts`. |
| `post_apply` | list of ScriptEntry | Scripts run after package install. See `facet docs scripts`. |
| `ai` | AIConfig | AI agent configuration, including Pi extensions under `ai.pi`. See `facet docs ai` and `facet docs pi`. |

All fields are optional except `extends` in profile files.
