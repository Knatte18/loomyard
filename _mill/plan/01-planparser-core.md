# Batch: planparser-core

```yaml
task: 'webster: rewrite for flat card list'
batch: planparser-core
number: 1
cards: 5
verify: go test ./internal/planparser/...
depends-on: []
```

## Batch Scope

Stand up the new `internal/planparser` package: the Go struct model of a flat
card-list plan, the readers that turn `_lyx/plan/00-overview.md` + `NN-<card-slug>.md`
files into that model, the `root:`/`//` path-normalization rule, and extraction of the
three plan-level body sections. This batch delivers parsing and normalization ONLY; the
14 validation checks are batch 2. The external interface batches 3, 7, 8, 9 consume is the
`planparser.Plan` / `planparser.Card` types and `planparser.ParsePlan(planDir string) (*Plan, error)`.
Mirror the good structural choices of the frozen v2 parser in `internal/builderengine/plan.go`
(lenient card-level parsing that records defects into fields for the validator to enumerate;
fail-loud only on document-structure errors) WITHOUT importing it and WITHOUT any `v2`/`v3`
naming. `internal/planparser` imports only stdlib + `gopkg.in/yaml.v3` + `internal/hubgeometry`
(no feature-package imports) so it stays a cycle-free leaf reusable by a future non-webster consumer.

## Cards

### Card 1: planparser package types + doc

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/plan.go`
  - `internal/builderengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/plan.go`
  - `internal/planparser/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Define the package's public struct model in `plan.go`, no version suffix in any name. `Plan` struct fields: `Dir string`; `Format int`; `Approved bool`; `Root string`; `Framing string`; `Cards []Card`; and the three extracted plan-level body sections `SharedDecisions string`, `RenameMechanic string`, `Verify string` (raw section bodies, empty when the section is absent — populated by Card 5). `Card` struct fields: `Number int`; `Slug string`; `Title string`; `Intent string` (the Card Index one-liner); `HasWhat bool`; `ContextFiles []string`; `EditsFiles []string`; `CreatesFiles []string`; `DeletesFiles []string`; `Moves []MovePair`; `MovesRaw []string`; `HasContext/HasEdits/HasCreates/HasDeletes/HasMoves/HasDependsOn bool`; `DependsOn []int`; `Commit string`; `Verify string`. Follow the frozen parser's nil-vs-empty-slice convention (nil slice = field label never seen; empty non-nil slice = present-but-`none`) and keep the `HasX` presence bools. Define `MovePair struct { Old, New string }` (normalized paths). Write `doc.go` as the package godoc: state that `internal/planparser` is the SOLE parser of the on-disk plan format (`_lyx/plan/`), that no other package may read that directory, and summarize the type model, the `root:`/`//` rule, the `none` sentinel, and that the 14 validation checks live in `validate.go`.
- **Commit:** `feat(planparser): plan/card struct model and package doc`

### Card 2: parse 00-overview.md

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/plan.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/planparser/plan.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/parse.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `parse.go` implement `ParsePlan(planDir string) (*Plan, error)` and the overview reader it calls. `ParsePlan` reads `filepath.Join(planDir, "00-overview.md")` — accept `planDir` as an explicit argument; NEVER construct the `_lyx`/`plan` tokens here (hubgeometry owns them; the caller passes `hubgeometry.PlanDir(...)`). Parse the scalar-only overview frontmatter into an unexported `overviewFrontmatter struct { Format *int; Approved *bool; Root *string }` decoded with `yaml.Decoder.KnownFields(true)` (pointer fields distinguish absent from zero, mirroring the frozen parser). Populate `Plan.Format`, `Plan.Approved`, `Plan.Root`. Parse the H1 title, the task-framing paragraph into `Plan.Framing`, and the `## Card Index` entries (`N — <card-slug> — <one-line intent>`) into per-card `Number`/`Slug`/`Intent`. Reuse helper shapes analogous to the frozen parser's `splitFrontmatter`, `extractSection`, `hasHeading` (reimplement locally; do not import builderengine). Fail-loud (return a wrapped `planparser:`-prefixed error) on document-structure errors: missing/undecodable frontmatter, unparseable index line. Then read each `NN-<card-slug>.md` file (Card 3 fills the card-body parse). Set `Plan.Dir = planDir`.
- **Commit:** `feat(planparser): parse 00-overview.md frontmatter, framing, and Card Index`

### Card 3: parse NN-<card-slug>.md card bodies

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/plan.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/parse.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `parse.go` with the per-card body reader. For each `NN-<card-slug>.md`, parse the ordered fields into the `Card` struct: `# Card N — <name>` heading (set `Title`, cross-check `Number`); `**What:**` prose (set `HasWhat`, discard the text like the frozen parser); the five typed file-op fields `**Context:**`/`**Edits:**`/`**Creates:**`/`**Deletes:**`/`**Moves:**` (set the `HasX` bools and the slices; `none` → present-but-empty non-nil slice; missing label → nil slice); `**Depends-on:**` (set `HasDependsOn`, parse the plain integer list or `none` into `DependsOn`); optional `**Commit:**` (set `Commit`); optional `**verify:**` (set `Verify`). Implement local `parseFileOpField` and `parseMovesField` helpers analogous to the frozen parser: `Moves:` sub-bullets are the two-path `` `old` -> `new` `` grammar → `MovePair`, with the raw bullet strings also captured in `MovesRaw` for the validator. Keep parsing LENIENT at card level (record malformed bullets into `MovesRaw`/leave slices as parsed; do NOT fail the parse) so `Validate` (batch 2) can enumerate findings; fail-loud only on document-structure errors (unparseable heading line, inline value on a field label that must be a bullet list). Do NOT normalize paths here — Card 4 does that as a distinct pass.
- **Commit:** `feat(planparser): parse card bodies with typed file-op and depends-on fields`

### Card 4: path normalization (root: / // resolution)

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/plan.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/parse.go`
- **Creates:**
  - `internal/planparser/normalize.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `normalize.go` implement `normalizeCardPath(root, raw string) string` reproducing the exact three-case rule the spec pins (mirror the frozen `normalizeCardPath`): a `//`-prefixed path is ALWAYS worktree-root-relative (strip the `//`, ignore `root`); otherwise the path resolves as `<root>/<path>`; a degenerate `root: "."` yields the raw path unchanged (not `./<raw>`). Normalization runs exactly ONCE, at parse time — call it from `parse.go` on every card file-op path (all five fields) and both endpoints of every `MovePair`, so downstream consumers and the validator only ever see plain, clean, forward-slash worktree-relative paths. Do NOT reject malformed forms here (a single-`/` prefix or a `..` segment); leave the raw-but-normalized value in place and let batch 2's `card-path-malformed` check flag them (keep the lenient/validator split). Add a `cleanPosixPath` helper as needed (`filepath.ToSlash` + `path.Clean`, preserving the malformed markers the validator keys on).
- **Commit:** `feat(planparser): root:/// card-path normalization at parse time`

### Card 5: plan-level section extraction + golden fixture

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/planparser/parse.go`
- **Creates:**
  - `internal/planparser/sections.go`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/normalize_test.go`
  - `internal/planparser/sections_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `sections.go` implement extraction of the three plan-level body sections from `00-overview.md` — `## Shared Decisions`, `## Rename mechanic`, `## verify:` — into `Plan.SharedDecisions`, `Plan.RenameMechanic`, `Plan.Verify` (raw section body text; empty string when the section is absent, since all three are optional). Wire the extraction into `ParsePlan`. For `## verify:` capture the single command line the section carries. Materialize the plan-format-v3.md **Worked example** (its `## Worked example`, the complete `00-overview.md` + four card files) VERBATIM as a golden happy-path fixture under `internal/planparser/testdata/goodplan/` (files `00-overview.md`, `01-…md` … `04-…md` matching the example's slugs). Write Tier-1 table-driven tests (no git, no `TestMain` needed — `t.TempDir()` only): `parse_test.go` asserts the golden fixture parses to the expected `Plan`/`Card` values (numbers, slugs, intents, all five file-op slices with `none`→empty and nil-vs-empty distinctions, `DependsOn`, `Commit`, `Verify`); `normalize_test.go` covers `//`, `<root>/<path>`, `root: "."`, malformed single-`/`, and `..`-escape inputs against `normalizeCardPath`; `sections_test.go` asserts the three plan-level sections are exposed from the golden fixture and are empty when absent. Follow `golang:golang-testing` conventions.
- **Commit:** `test(planparser): plan-level sections, golden fixture, parse/normalize tests`

## Batch Tests

`verify: go test ./internal/planparser/...` runs the Tier-1 table-driven tests for parsing,
path normalization, and plan-level section extraction. All hermetic: fixtures live under
`internal/planparser/testdata/`, existence-independent parsing needs no real repo, so no git
is spawned and no `//go:build integration` tag or `TestMain` is required for this package. The
14 validation checks and their existence-dependent fixtures arrive in batch 2.
