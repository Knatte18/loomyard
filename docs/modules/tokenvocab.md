# tokenvocab: the shared token vocabulary

> **Status: ✅ Implemented — a leaf, not a phased module.** Unlike the other files in this
> folder, `tokenvocab` has no `lyx <module>` CLI of its own and nothing to phase through
> Setup/Discussion/Plan/Builder. Per the [documentation
> lifecycle](../overview.md#documentation-lifecycle) this doc stays only as long as it earns
> its keep over the package godoc; today it is the fuller design reference, cross-linked from
> [overview.md](../overview.md) and [CONSTRAINTS.md](../../CONSTRAINTS.md).

## What it is

`internal/tokenvocab` is the general/shared token vocabulary consumed by every lyx feature
that fills a text template from worktree/Hub context: mux's header text pipeline today, and
loom's prompt templates later. It owns exactly two things:

1. **The token registry** — the flat, always-resolvable set of named tokens (currently
   `repo`, `hub`) and the values they resolve to from a `hubgeometry.Layout`.
2. **`Render`** — the single reusable compose over `internal/stencil` that every consumer
   calls to fill a template with the vocabulary.

It is deliberately thin: `tokenvocab` has no opinion about *what* consumes a rendered
template (a mux pane, a loom prompt) or *where* the template text comes from (a config file,
an embedded default). Those decisions live in the consumer.

## API

```go
// Ctx carries what a Token.Resolve needs. A struct, not a bare *hubgeometry.Layout, so a
// future field (e.g. a task slug) can be added without changing every Resolve signature.
type Ctx struct {
    Layout *hubgeometry.Layout
}

// Token is one named, resolvable vocabulary entry.
type Token struct {
    Name    string
    Resolve func(Ctx) string
}

// Build resolves every registry token against c into a flat map keyed by token name — the
// shape internal/stencil.Fill consumes as its values argument.
func Build(c Ctx) map[string]string

// Render fills template with Build(c), delegating to stencil.Fill. It surfaces
// stencil.Fill's unfilled-top-level-marker error unchanged for an unknown or empty token —
// a loud, early error, never a silently blank field.
func Render(template []byte, c Ctx) ([]byte, error)
```

Today's registry has two entries, both resolved straight from `hubgeometry.Layout`:

| Token    | Resolves to        |
|----------|---------------------|
| `repo`   | `Layout.Repo`       |
| `hub`    | `Layout.Hub`        |

A `slug` token (a task identifier) is a known future addition, deliberately deferred until a
consumer needs it — `Ctx` is already shaped to carry it without a signature break.

## Adding a token

Adding a token is exactly **one entry** in the unexported `registry` slice in
`tokenvocab.go` — `{Name: "...", Resolve: func(c Ctx) string { ... }}`. `Build` and `Render`
both iterate `registry` rather than switching on token names, so neither needs to change when
a token is added. This "one registry entry" property is itself covered by a test case in
`tokenvocab_test.go` (`TestRegistry_AddingATokenIsOneEntry`).

## Leaf invariant

`tokenvocab` imports only the standard library, `internal/hubgeometry`, and
`internal/stencil` — never a feature package (`mux`, `loom`, or any other module). This
mirrors `internal/modelspec`'s leaf discipline: any future consumer can depend on
`tokenvocab` with zero risk of an import cycle, because `tokenvocab` never imports back.

- **Enforced by** `internal/tokenvocab/leaf_enforcement_test.go`
  (`TestLeafInvariant_AllowlistOnly`), an allowlist-only import-graph walk over the package's
  production (non-test) `.go` files.
- Recorded as the "Tokenvocab Leaf Invariant" in
  [CONSTRAINTS.md](../../CONSTRAINTS.md#tokenvocab-leaf-invariant).

## Why `tokenvocab` and not inside `stencil`

`internal/stencil` stays a pure stdlib leaf — a generic marker-fill engine with no opinion
about what markers mean. The vocabulary (what `repo` or `hub` *is*, and how it is derived
from worktree/Hub geometry) is deliberately a separate, slightly-less-leaf module one layer
up, so `stencil` never needs to know about `hubgeometry` and a consumer that only needs raw
template-filling (with its own values map) never needs to know about the vocabulary.

## Consumers

- **mux's header text pipeline** ([modules/loom.md](loom.md) references the broader header
  design) calls `tokenvocab.Render` to fill the header template with the vocabulary before
  handing the result to the header pane.
- **loom** (design, not yet built) is expected to reuse the same `Render` compose for its
  prompt templates once it lands — the reason `tokenvocab` was built general/shared from day
  one rather than embedded inside `mux`.
