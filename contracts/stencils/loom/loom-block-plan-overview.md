<!-- This is the loom-block-plan-overview plan-format block: the 00-overview.md format, glyph targets and plan: handles.
     loom-template-plan, loom-template-part-writer and loom-template-parts-chair pull it in with the bare {{template "loom-block-plan-overview"}}, with no pipeline.
     The fill hands it the including seat's value map, so it declares one required marker: {{.specs_dir}}.
     It names no step of the stencil around it; each including stencil states its own rule for the lines this block leaves out. -->

### `00-overview.md`

Scalar-only frontmatter:

```yaml
format: 5
approved: false
root: <optional worktree-relative dir>
language: go
```

`root:` is optional shorthand for a plan whose cards repeat one directory prefix: when set, every card path resolves as `<root>/<path>` — unless the path starts with `//`, which is always worktree-root-relative (root set or not).
Omit `root:` when there is no shared prefix.
Card paths are always worktree-relative and clean: never absolute, never containing `..`.

`language:` is `"go"` (the default — you may omit the key entirely), `"csharp"` or `"none"`.
Set `"csharp"` when the code the cards change is C#, read from the files' extension; `{{.specs_dir}}/loom/loom-plan-spec.md` holds the C# rules.
Otherwise leave it at `"go"` unless the task is explicitly non-Go.

**A symbol target is always spelled as a glyph, never as a bare `pkg.Symbol` string.** A glyph is `<unit>#<member>` — the package or file's own repository-relative path, a `#`, then the symbol's own name (e.g. `internal/boardcli#newListCmd`, or `internal/boardcli#` to name the whole package). A bare package-qualified symbol is a hard finding (`bare-symbol-target`) because it is the one spelling that cannot have come verbatim from a quarry answer — not because the form is uglier. A file path (`list.go`, `internal/boardcli/list.go`) stays a plain path — the parser canonicalizes it into its own file self glyph automatically; you never hand-write the `#`-suffixed form for a plain file.

### Declaring a symbol that does not exist yet: `plan:` handles

`lym quarry` can only answer with glyphs for symbols that already exist — it never invents one. When a card creates a brand-new symbol or file, `quarry` has nothing to look up, so you invent a draft placeholder spelling instead: a `plan:` handle, `plan:<unit>#<member>`, where `<unit>` is the new symbol's own repository-relative PACKAGE DIRECTORY — never the `.go` file it will live in (`plan:internal/boardcli#RowJSON`, not `plan:internal/boardcli/rowjson.go#RowJSON`; a file-unit member spelling can never resolve, and the validator refuses it as `handle-malformed`).

Write it on the `**Create:**` sub-bullet using the two-field declaration grammar, reusing the same `` `x` -> `y` `` arrow shape a `**Rename:**` pair uses:

```markdown
**Create:**
- `plan:internal/boardcli#RowJSON` -> `type RowJSON struct`
- `plan:internal/boardcli#newRowJSON` -> `func newRowJSON(r Row) RowJSON`
```

The left-hand token is the handle you just invented; the right-hand token is the declaration head — the symbol's own spelling and kind, the text a later resolve step needs to turn your handle into a real glyph. Every later card that targets this same not-yet-real symbol references it by the identical handle string, never by guessing what its eventual glyph will be.

**The declaration head must be one real, parseable Go declaration head, declaring exactly one symbol.** It is parsed as source, not read as prose, so a placeholder body is a hard finding (`handle-name-failed`), not a shorthand:

- Write `type RowJSON struct`, never `type RowJSON struct{...}` — `...` is not Go and the whole plan is blocked.
- Write the head only. A body is unnecessary; `func Foo() error` and `type Bar interface` are both complete.
- One symbol per bullet: `const A, B = 1, 2`, two funcs in one bullet, or an interface written out with its methods each declare more than one and are rejected.
- The receiver belongs to a method's head: `func (c *Cache) Get(k string) (Row, bool)`.

On a `**Rename:**` pair renaming an existing symbol, the grammar is asymmetric: the `Old` side is always a real glyph (looked up via `lym quarry`, exactly like any other target), and the `New` side is always a `plan:` handle you invent for the renamed name — never a glyph, since the symbol under its new name does not exist until the rename lands. A file-rename pair (old and new both plain file paths) is unaffected by this rule.

A card that renames a symbol also renames the tests named after it, `Test<Name>` and `Test<Name>_<case>` (`Test<Type>_<Method>` for a method), as `**Rename:**` pairs on the same card, since the implementer renames them with the symbol and a later card naming the old test would no longer match the tree.
The plan gate refuses a renamed symbol whose same-named test a card still lists under `**Edit:**` with no pair of its own (`rename-leaves-test`).

Every new member a card creates is declared as a `plan:` handle, whether or not a later card references it.
A handle referenced but never declared, or two `Create:` bullets declaring the same handle, are each hard findings (`handle-dangling`, `handle-collision`) — every handle you invent must be declared exactly once.

Always write `approved: false` — you never self-approve;
`Plan-Bouncer`'s approved settle writes it to `true`.
Body: a short task-framing paragraph, then an ordered **Card Index** (`N — <card-slug> — <one-line intent>`), then the optional plan-level sections `## Shared Decisions`, `## Rename mechanic` (required when any card is type `Rename`).