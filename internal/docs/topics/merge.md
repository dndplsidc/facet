# Merge Rules

## Layer Order

facet merges three layers in order. Later layers win on conflicts.

```text
1. resolved base from `extends`
2. profiles/<name>.yaml
3. .local.yaml
```

## `vars`

`vars` deep-merge by leaf key.

```yaml
# base.yaml
vars:
  git:
    name: Sarah
    editor: nvim

# profile
vars:
  git:
    email: sarah@work.com
```

Result:

```yaml
vars:
  git:
    name: Sarah
    editor: nvim
    email: sarah@work.com
```

Type conflicts are fatal.

## `packages`

Packages are unioned by `name`. Later layers replace earlier entries with the same
name.

## `configs`

`configs` are shallow-merged by target path. If two layers define the same target,
the later layer wins. The entire source string or OS map is replaced; platform
branches are not deep-merged. OS selection happens after all layers merge.

## `pre_apply` / `post_apply`

Scripts are concatenated: base scripts first, then profile scripts appended at the
end. No deduplication by name. `.local.yaml` can also contribute scripts.

## `ai.agents`

Last writer wins. A later layer replaces the entire list.

## `ai.permissions`

Permissions merge by agent name. For the same agent, the later layer replaces that
agent's entire allow/deny block.

## `ai.skills`

Skills are unioned by the tuple `(source, skill name)`. If the same pair appears in
multiple layers, the later layer wins, including its agent scoping.

An entry with an empty `skills` list means "all skills from this source." It is
treated as an atomic unit during merge:

- "All" in a later layer replaces all individual skill entries for the same source
  from earlier layers.
- Specific skills in a later layer replace an "all" entry for the same source from
  earlier layers.

## `ai.mcps`

MCP entries are unioned by `name`. Same name means the later layer replaces the
whole entry.
