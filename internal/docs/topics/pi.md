# Pi Extensions

Pi coding-agent extensions are managed inside the `ai:` block:

```yaml
ai:
  pi:
    extensions:
      - pi-interactive-shell
      - pi-lens
      - pi-subagents
      - "@juicesharp/rpiv-btw"
      - "@gotgenes/pi-session-tools"
```

## Behavior

Facet installs newly declared extensions with the current Pi package command:

```sh
pi install <name>
```

If a declared extension is already recorded in Facet state, normal `facet apply`
skips reinstalling it. Use `facet apply --force` to reinstall unchanged managed
Pi extensions.

Facet removes only extensions that were previously managed by Facet and are no
longer declared in the resolved `ai.pi.extensions` config:

```sh
pi remove <name>
```

Manually installed Pi extensions are left untouched.

## Stage

Pi extensions are reconciled as part of the `ai` apply stage. Use this to run
only AI configuration, including Pi extensions:

```sh
facet apply work --stages ai
```
