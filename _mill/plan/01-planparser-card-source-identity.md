# Batch: planparser-card-source-identity

```yaml
task: 'webster: stop re-rendering already-inherited context into fork prompts'
batch: 'planparser-card-source-identity'
number: 1
cards: 1
verify: go build ./... && go test ./internal/planparser/... ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

This batch adds the per-card worktree-relative source-identity token that batch 2's prompt renderers consume. It adds a `hubgeometry.PlanDirRel()` accessor (the relative `_lyx/plan` token — keeping the `_lyx/plan` construction inside `hubgeometry` per `PlanDir`'s doc), and `planparser.Card` gains a `SourcePath` field carrying the bare worktree-relative token `_lyx/plan/NN-<slug>.md`, built during parse from `PlanDirRel()` plus planparser's own `NN-<Slug>.md` filename — independent of the absolute `Plan.Dir`. This is the sole source of the card's path token; `render.go` (batch 2) renders it verbatim. It is one batch and one card because it is a single, self-contained addition split across the `hubgeometry` accessor and its sole `planparser` consumer, gated by both packages' unit suites. **External interface batch 2 consumes:** the new exported `SourcePath` field on `planparser.Card`.

## Cards

### Card 1: planparser.Card worktree-relative source-identity field

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add an exported accessor `PlanDirRel() string` to `internal/hubgeometry/hubgeometry.go` returning the **worktree-relative** plan-directory token `_lyx/plan` (forward-slash), built as `path.Join(LyxDirName, "plan")` using the stdlib `path` package (add the `path` import if absent). Document it as the relative counterpart to `PlanDir(baseDir)` — for building worktree-relative plan-file pointers that resolve from the session cwd — and note that, like `PlanDir`, it keeps the `_lyx/plan` path construction inside `hubgeometry` per the Hub Geometry Invariant and `PlanDir`'s own "no other package may construct this path" doc. Do NOT hardcode a literal `_lyx` (reference `LyxDirName`).
  - Add an exported field `SourcePath string` to the `Card` struct in `internal/planparser/plan.go`, documented as the card's bare worktree-relative source-identity token of the form `_lyx/plan/NN-<slug>.md` (`NN` zero-padded, `slug` the card's own `Slug`). Its godoc must state: the `_lyx/plan` segment comes from `hubgeometry.PlanDirRel()` and the `NN-<slug>.md` filename from planparser's own `cardFileName`, never from the absolute `Plan.Dir` (which is `t.TempDir()` in tests) and never from a literal `_lyx`/`"plan"` string; it is the sole source of the card's path pointer, rendered verbatim by webster's prompt renderers per the `card-pointer-relative-via-hubgeometry` decision.
  - Populate `SourcePath` in `internal/planparser/parse.go` where each `Card` is constructed from its Card Index entry (the `Card{Number: entry.Number, Slug: entry.Slug, ...}` literal near line 286). Build the token with forward slashes: `path.Join(hubgeometry.PlanDirRel(), cardFileName(entry.Number, entry.Slug))` — reuse the existing `cardFileName` helper (returns `NN-<slug>.md`) and the new `hubgeometry.PlanDirRel()` accessor for the `_lyx/plan` segment. Add the `path` and `github.com/Knatte18/loomyard/internal/hubgeometry` imports to `parse.go`. Do NOT use `filepath.Join` for this token (it would emit backslashes on Windows); do NOT hardcode the literal string `"_lyx"` or `"plan"` in `parse.go` (the `_lyx/plan` construction lives in `hubgeometry.PlanDirRel()`, per the Hub Geometry Invariant, `PlanDir`'s doc, and the Planparser Sole-Parser Invariant's "resolves `_lyx/plan/` via hubgeometry, never string literals").
  - Update the `What` field's godoc in `internal/planparser/plan.go` (currently "It is what RenderForkPrompt injects into a fork/recovery prompt") to reflect the new reality: the card file is read directly by the fork/recovery strand via the `SourcePath` pointer; `What` is no longer Go-inlined into the prompt. Keep the existing note that `Intent` (index one-liner) is never a substitute for `What`.
  - TDD in `internal/planparser/parse_test.go`: parse a fixture plan through `ParsePlan` and assert each parsed card's `SourcePath` equals the expected `_lyx/plan/NN-<slug>.md` token (e.g. `_lyx/plan/01-<slug>.md`, `_lyx/plan/02-<slug>.md`). Assert the token is NOT prefixed by the absolute `Plan.Dir` and contains no `t.TempDir()` leak (assert it does not contain the temp dir path and is exactly the bare worktree-relative form). Cover both a single-card and a multi-card plan (reuse or extend the existing parse-test fixtures under `internal/planparser/testdata`).
- **Commit:** `feat(planparser): add Card.SourcePath via hubgeometry.PlanDirRel`

## Batch Tests

`verify: go build ./... && go test ./internal/planparser/... ./internal/hubgeometry/...` — the new `Card.SourcePath` field, its parse-time population, and the multi-card/single-card assertions live in `internal/planparser`; the new `PlanDirRel()` accessor lives in `internal/hubgeometry`. Both packages' unit suites are the exact scope (the hubgeometry suite also runs its `TestEnforcement_GeometryLiterals` geometry-literal guard, confirming the new accessor introduces no stray `_lyx` literal). `go build ./...` additionally confirms no downstream package (webster) broke from the struct change before batch 2 consumes the field.
