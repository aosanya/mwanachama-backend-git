# Document store settings

Board row G23 (member app decisions 280, 281, 398). An agency's document
store carries two settings, declared through shared's S64 engine in
`git.blueprint.json`'s `settings` block.

| Setting | Type | Default | Meaning |
| --- | --- | --- | --- |
| `repositories` | json, a list of names | `[]` | The names of the agency's repositories, in the order the agency reads them. |
| `tag_patterns` | json, a list of patterns | `[]` | The patterns a tag name must match, in Go's `path.Match` form (`v*`, `release-*`). None means any name. |

## How they resolve

`spec.Migrate` (called by `Provision`) creates the settings definition,
override and change-log tables in this module's own table set, because the
blueprint declares settings. Each read goes through
`spec.ResolveStored`: the override layer wins, then the definition layer the
host stored with `spec.PutDefinitionSettings`, then the module default above.
Nothing is cached in the manager, so a stored override is seen on the next
call.

The manager exposes both read-only through `DocumentStoreSettings`
(`Repositories(ctx)`, `TagPatterns(ctx)`), which `gitManager` implements.
It is a separate interface rather than two new `GitManager` methods so no
consumer's fake has to grow. Writing either setting is the host's, through
the engine's `StoreOverride`/`StoreClearOverride`; this module declares no
address for it.

`ParseStringList` holds a stored value to a list of non-empty strings and
refuses anything else by the setting's name.

## Where tag patterns apply

`CreateTag` is the one place a tag is minted by name, so it is the one place
the patterns are checked: after the repository is found and before the
duplicate check. With patterns declared, a name matching none of them is
refused with `ErrTagNameNotAllowed` (400). With none declared the behaviour
is exactly what it was. A malformed pattern refuses the create with the
pattern named, rather than letting every name through.

`repositories` constrains nothing yet: no repository create or list reads
it. What it should constrain is not decided.

## Open, not decided here

- **A stale override after a republish.** When a republished definition
  changes a setting an operator has overridden, whether the override still
  stands is an open owner question. This module reads whatever the engine
  resolves and decides nothing about it.
- Whether `repositories` should gate `InitRepo` or filter listings.
