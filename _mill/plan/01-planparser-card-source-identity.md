# Batch: planparser-card-source-identity

```yaml
task: 'webster: stop re-rendering already-inherited context into fork prompts'
batch: 'planparser-card-source-identity'
number: 1
cards: 1
verify: go build ./... && go test ./internal/planparser/...
depends-on: []
```

## Batch Scope

This batch adds the per-card worktree-relative source-identity token that batch 2's prompt renderers consume. `planparser.Card` gains a field carrying the bare worktree-relative token `_lyx/plan/NN-<slug>.md`, built during parse from `hubgeometry.LyxDirName` and the card's own `NN-<Slug>.md` filename — independent of the absolute `Plan.Dir`. This is the sole source of the card's path token; `render.go` (batch 2) renders it verbatim. It is one batch because it is a single, self-contained addition inside `planparser`, gated by the planparser unit suite alone. **External interface batch 2 consumes:** the new exported field on `planparser.Card`.

## Cards

### Card 1: planparser.Card worktree-relative source-identity field

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add an exported field `SourcePath string` to the `Card` struct in `internal/planparser/plan.go`, documented as the card's bare worktree-relative source-identity token of the form `_lyx/plan/NN-<slug>.md` (`NN` zero-padded, `slug` the card's own `Slug`). Its godoc must state: it is built from `hubgeometry.LyxDirName`, never from the absolute `Plan.Dir` (which is `t.TempDir()` in tests) and never from a literal `_lyx` string; it is the sole source of the card's path pointer, rendered verbatim by webster's prompt renderers per the `card-pointer-relative-via-hubgeometry` decision.
  - Populate `SourcePath` in `internal/planparser/parse.go` where each `Card` is constructed from its Card Index entry (the `Card{Number: entry.Number, Slug: entry.Slug, ...}` literal near line 286). Build the token with forward slashes using the stdlib `path` package: `path.Join(hubgeometry.LyxDirName, "plan", cardFileName(entry.Number, entry.Slug))` — reuse the existing `cardFileName` helper (returns `NN-<slug>.md`). Add the `path` and `github.com/Knatte18/loomyard/internal/hubgeometry` imports to `parse.go`. Do NOT use `filepath.Join` for this token (it would emit backslashes on Windows); do NOT hardcode the literal string `"_lyx"` (Hub Geometry Invariant bans the geometry token outside `hubgeometry` as a `filepath.Join` arg, `+` operand, or `const` — reference `hubgeometry.LyxDirName` instead).
  - Update the `What` field's godoc in `internal/planparser/plan.go` (currently "It is what RenderForkPrompt injects into a fork/recovery prompt") to reflect the new reality: the card file is read directly by the fork/recovery strand via the `SourcePath` pointer; `What` is no longer Go-inlined into the prompt. Keep the existing note that `Intent` (index one-liner) is never a substitute for `What`.
  - TDD in `internal/planparser/parse_test.go`: parse a fixture plan through `ParsePlan` and assert each parsed card's `SourcePath` equals the expected `_lyx/plan/NN-<slug>.md` token (e.g. `_lyx/plan/01-<slug>.md`, `_lyx/plan/02-<slug>.md`). Assert the token is NOT prefixed by the absolute `Plan.Dir` and contains no `t.TempDir()` leak (assert it does not contain the temp dir path and is exactly the bare worktree-relative form). Cover both a single-card and a multi-card plan (reuse or extend the existing parse-test fixtures under `internal/planparser/testdata`).
- **Commit:** `feat(planparser): add worktree-relative Card.SourcePath token`

## Batch Tests

`verify: go build ./... && go test ./internal/planparser/...` — the new field, its parse-time population, and the multi-card/single-card assertions all live in `internal/planparser`, so the planparser unit suite is the exact scope. `go build ./...` additionally confirms no downstream package (webster) broke from the struct change before batch 2 consumes the field.
