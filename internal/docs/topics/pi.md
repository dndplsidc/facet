# Pi Extensions

Pi coding-agent extensions are managed inside the `ai:` block:

```yaml
ai:
  pi:
    extensions:
      - source: npm:pi-intercom
      - source: npm:@company/internal-pi-extension
        install_env:
          NPM_CONFIG_REGISTRY: https://bnpm.byted.org
```

## Behavior

Facet installs newly declared extensions with the current Pi package command:

```sh
pi install <source>
```

If a declared extension is already recorded in Facet state, normal `facet apply`
skips reinstalling it. Use `facet apply --force` to reinstall unchanged managed
Pi extensions. Optional `install_env` values are passed only to the install
command for that extension, not to other installs or removals.

Facet removes only extensions that were previously managed by Facet and are no
longer declared in the resolved `ai.pi.extensions` config:

```sh
pi remove <source>
```

Manually installed Pi extensions are left untouched.

## Stage

Pi extensions are reconciled as part of the `ai` apply stage. Use this to run
only AI configuration, including Pi extensions:

```sh
facet apply work --stages ai
```
