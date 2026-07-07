# Batch: stencil-leaf

```yaml
task: 'Build internal/stencil: fill markdown prompt templates'
batch: 'stencil-leaf'
number: 1
cards: 3
verify: go test ./internal/stencil/
depends-on: []
```

## Batch Scope

This batch delivers the entire `internal/stencil` leaf in one Sonnet-sized unit: the
implementation (`stencil.go`), its deep table-driven tests (`stencil_test.go`), and the
doc updates that the CLAUDE.md Documentation Lifecycle requires in the same task. The three
cards share a single small context (the stencil package plus its shared-lib doc), so they
are one batch. The external interface this establishes for future consumers
(`burler`/`perch`/`loom`/`hardener`) is the single exported function
`Fill(template []byte, values map[string]string) ([]byte, error)`; those consumers are out
of scope here. No batch-local decisions differ from `## Shared Decisions`.

## Cards

### Card 1: Implement the `Fill` renderer and its guard

- **Context:**
  - `internal/yamlengine/resolve.go`
- **Edits:** none
- **Creates:**
  - `internal/stencil/stencil.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create package `stencil` in `internal/stencil/stencil.go`. Import only the standard
    library: `bytes`, `fmt`, `sort`, `strings`, `text/template`, `text/template/parse`.
  - Add the sole exported function
    `func Fill(template []byte, values map[string]string) ([]byte, error)`. Mirror the godoc
    density and error style of `yamlengine.Resolve` in the Context file — a package-header
    comment plus a doc comment on `Fill` that states the load-bearing guarantee.
  - Implement the pinned ordering exactly:
    1. **Strip a leading comment.** Add an unexported helper
       `stripLeadingComment(text string) string` that drops a `<!-- … -->` block only when
       the text (after `strings.TrimLeft` of leading whitespace) begins with `<!--`: find
       the first `-->`, and return everything after it with leading `\r`/`\n` trimmed. If
       there is no leading `<!--` or no closing `-->`, return the text unchanged. Mid-template
       comments must be left untouched. Apply this to `string(template)` before parsing.
    2. **Parse** the stripped text with
       `template.New("stencil").Option("missingkey=error").Parse(stripped)`. On error return
       `fmt.Errorf("parse template: %w", err)`.
    3. **Top-level walk.** Iterate `tmpl.Tree.Root.Nodes` (depth-0 only — do NOT descend into
       `*parse.IfNode`/`*parse.WithNode`/`*parse.RangeNode` bodies). For each
       `*parse.ActionNode` whose `Pipe` is a single command (`len(Pipe.Cmds) == 1`) whose
       single argument is a bare `*parse.FieldNode` with `len(Ident) >= 1` (a `{{.X}}`
       substitution), take the field name `FieldNode.Ident[0]`. Collect it as an offender
       when `strings.TrimSpace(values[name]) == ""` (an absent key reads as the zero value
       `""`, so this one check covers both the absent-key and the empty/whitespace-only
       cases). Guard defensively against both a nil `tmpl.Tree` and a nil `tmpl.Tree.Root`
       before ranging `.Nodes` (a template that is empty or comment-only parses to a tree
       with no action nodes → no offenders); skip the walk cleanly in that case.
    4. **Fail before executing.** If any offenders were collected, dedup them, `sort.Strings`
       the unique names, and return a plain `fmt.Errorf` naming every offender in the sorted,
       joined list (e.g. `stencil: unfilled top-level marker(s): Fasit, Target`) — do NOT
       execute the template. This is why top-level batching reports all offenders at once.
    5. **Execute** only when the top level is clean: render into a `bytes.Buffer` via
       `tmpl.Execute(&buf, values)`. A branch-internal reached-but-absent marker surfaces
       here as a `missingkey=error` execution error (which halts at the first miss) — wrap it
       as `fmt.Errorf("execute template: %w", err)`. Return `buf.Bytes(), nil` on success.
  - Use `text/template`, never `html/template` — output must not be HTML-escaped.
  - Godoc must state the guarantee precisely without over-promising: top-level markers that
    are absent-or-empty are all collected and reported sorted in one error; branch-internal
    reached-but-absent markers are caught incrementally (one per call) via `missingkey=error`;
    the leading `<!-- … -->` comment is stripped before parsing.
  - Godoc must also state the caller contract that follows from top-level-only
    empty-checking: because a present-but-empty value is guarded only at depth-0, a required
    marker (like `fasit`/`target`) must be placed at the template top level, never inside a
    conditional branch — a branch-internal present-but-empty value would render a silent
    blank (only branch-internal *absent* keys are caught, via `missingkey=error`).
- **Commit:** `feat(stencil): add Fill renderer with unfilled-marker guard`

### Card 2: Table-driven tests for the contract and the guard

- **Context:**
  - `internal/yamlengine/resolve_test.go`
  - `internal/output/output_test.go`
- **Edits:** none
- **Creates:**
  - `internal/stencil/stencil_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `internal/stencil/stencil_test.go` in black-box `package stencil_test`, importing
    the package under test as `github.com/Knatte18/loomyard/internal/stencil` (match the
    import style in `internal/output/output_test.go`). Pure, table-driven; no substrate.
  - Cover every scenario from the discussion's Testing section, each exercising `stencil.Fill`
    (do not prescribe exact assertion strings — assert behaviour):
    1. **Happy path** — several top-level `{{.X}}` markers, all present and non-empty →
       correct substituted output, nil error.
    2. **Missing top-level marker** — a referenced top-level marker absent from `values` →
       error whose message names that marker.
    3. **Empty value** — a top-level marker present as `""` → error; and present as
       whitespace-only (`"   "`) → error (the empty-`fasit` guard).
    4. **Multiple top-level offenders collected & sorted** — two or more unfilled top-level
       markers → a single error listing all of them in deterministic sorted order.
    5. **Branch-internal miss caught incrementally** — a taken branch references an absent
       marker → error naming it; and a mix (absent top-level marker + absent in-branch marker)
       reports the top-level offender and returns before execution, so the in-branch name is
       NOT in the same error.
    6. **Malformed template** — an unparseable template (e.g. unclosed `{{if}}`) → a
       non-nil error wrapping the parse failure (`"parse template:"`), never a panic.
    7. **Conditional taken** — `{{if eq .Type "Cluster"}}…{{end}}` with `Type: "Cluster"` →
       section present and its inner markers substituted.
    8. **Conditional not taken** — same template with a non-matching `Type` → section absent,
       and markers living only inside that branch are NOT required (no error though absent).
    9. **Forgotten discriminator** — template references `{{if eq .Type …}}` but `values` has
       no `Type` key → error (the condition is always evaluated).
    10. **Unused values ignored** — `values` carries keys the template never references → no
        error, output unaffected.
    11. **Leading-comment strip** — a leading `<!-- … -->` is dropped (a `{{…}}`/marker inside
        it is neither substituted nor checked → no error); a mid-template comment is preserved
        verbatim; a comment-only template → empty output.
    12. **Empty / whitespace-only template** → empty output, nil error.
    13. **Idempotence / determinism** — same template + values → byte-identical output across
        repeated calls; the multi-offender error message is stable (sorted).
    14. **No HTML escaping** — a value containing `<`, `>`, `&`, or quotes passes through
        verbatim (confirms `text/template`, not `html/template`).
- **Commit:** `test(stencil): cover Fill happy path, guard, conditionals, comment strip`

### Card 3: Update the shared-lib docs to the built contract

- **Context:**
  - `docs/overview.md`
- **Edits:**
  - `docs/shared-libs/stencil.md`
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `docs/shared-libs/stencil.md`:
    - Update the status blockquote at the top (currently
      "**Status: Design — not built.**") to state the leaf is now built, keeping the
      existing note that this doc pins the contract agreed during the review-engine design.
    - In "## The contract", change the signature from
      `Fill(template []byte, values map[string]any) ([]byte, error)` to
      `Fill(template []byte, values map[string]string) ([]byte, error)`.
    - In "## The contract" → "Marker syntax", replace the "an implementation choice" hedge and
      the `<PLACEHOLDER>` alternative with the pinned grammar: stdlib `text/template`
      (`text/`, not `html/template`), `{{.X}}` substitution plus `{{if eq .Type "…"}}`
      equality conditionals (variadic `eq`, and the `and`/`or`/`not`/comparison operators come
      free); note the leading `<!-- … -->` comment is stripped before parsing.
    - In "## The one load-bearing guarantee", refine the wording to the built scoping:
      top-level absent-or-empty markers are all collected and reported sorted in one error;
      branch-internal reached-but-absent markers are caught incrementally via
      `missingkey=error`. Do not over-promise "every hole in one error" for branch-internal
      markers.
  - In `docs/shared-libs/README.md`: update the `stencil.md` bullet under "## Libraries" —
    remove the `🚧 design — not built` marker so it reads as a built lib in the same style as
    the sibling entries (e.g. `internal/stencil`: fill marker fields in a markdown template →
    prompt (fails on an unfilled marker)).
  - `docs/overview.md` needs no change — verify it references `shared-libs/README.md` rather
    than enumerating individual shared libs, so nothing there lists stencil. Make no edit to
    it if that holds.
- **Commit:** `docs(stencil): mark leaf built, pin signature and grammar`

## Batch Tests

`verify: go test ./internal/stencil/` runs the package's own table-driven suite
(`stencil_test.go`), which is the entire runnable surface of this batch — Cards 1 and 2
create it, Card 3 is docs-only with no runnable surface. Scope is the single new package, so
the command is naturally fast; no broader `go test ./...` is needed (stencil is imported by
nothing yet, and the geometry/leaf-enforcement tests in other packages are unaffected by a
stdlib-only leaf). This is a Go project, so the `verify:` command uses the native `go test`
runner with no `PYTHONPATH=` prefix, and runs from the git root (non-nested layout).
