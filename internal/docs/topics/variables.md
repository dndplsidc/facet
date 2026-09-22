# Variables

## Syntax

```text
${facet:var_name}
```

Variables come from the merged `vars:` blocks across the resolved base from
`extends`, the selected profile, and `.local.yaml`.

## Nested Variables

Nested maps use dot notation:

```yaml
vars:
  git:
    name: Sarah Chen
    email: sarah@work.com
  aws:
    region: us-east-1
```

Examples:

- `${facet:git.email}`
- `${facet:aws.region}`

## Where Variables Are Resolved

- Package install and check commands
- Config source paths on the right side of `configs:`
- Templated config file contents
- Script `run` fields (in `pre_apply` and `post_apply`)
- MCP `command`, `args`, and `env` values

## Where Variables Are Not Resolved

- Config target paths on the left side of `configs:`. Those use `~`, `$VAR`, and `${VAR}` environment expansion instead.
- Config source paths are substituted first, then resolved relative to the source root of the layer that defined them
- Variable values themselves. facet does not do recursive substitution.

## Undefined Variables

Referencing an undefined variable in the selected configuration is a fatal error.
OS selection happens after merging and before substitution: inactive config sources,
hook commands, and package commands do not require their variables to be defined.
If a package has no installer for the current OS, its check is also inactive.

```text
undefined variable: ${facet:db_url} — define it in .local.yaml or your profile's vars
```

## Rules

- No recursive resolution inside `vars`
- Merge order is resolved base then profile then `.local.yaml`
- Later layers win on conflicts
- Keep secrets in `.local.yaml`
