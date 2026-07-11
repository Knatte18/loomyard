# Batch: plan-model

```yaml
task: "Build builder - the batch-implementation loop"
batch: "plan-model"
number: 1
cards: 6
verify: go test ./internal/hubgeometry/... ./internal/builderengine/...
depends-on: []
```

## Batch Scope

Foundations: the three new `hubgeometry` helpers that own builder's `_lyx` paths, the
`internal/builderengine` package with the plan data model (parser for `00-overview.md`
frontmatter + Batch Index + per-batch files), the plan fingerprint, the six machine
validation checks from plan-format.md, and the hand-written plan fixtures every later
batch tests against. External interface consumed by later batches: `ParsePlan`,
`Validate`, `Fingerprint`, the `Plan`/`PlanBatch` types, and
`hubgeometry.PlanDir`/`BuilderDir`/`BuilderReportsDir`.

## Cards

### Card 1: hubgeometry helpers PlanDir, BuilderDir, BuilderReportsDir

- **Context:**
  - `docs/modules/plan-format.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three exported helpers directly below `PerchRunsDir`, following
  its exact godoc shape (purpose, weft-sync rationale, Hub Geometry Invariant note,
  literal `Returns filepath.Join(...)` line): `PlanDir(baseDir string) string` returning
  `filepath.Join(baseDir, LyxDirName, "plan")`; `BuilderDir(baseDir string) string`
  returning `filepath.Join(baseDir, LyxDirName, "builder")`;
  `BuilderReportsDir(baseDir string) string` returning
  `filepath.Join(BuilderDir(baseDir), "reports")`. Add table-driven tests mirroring the
  existing `PerchRunsDir` test coverage (expected suffix per platform separator).
- **Commit:** `feat(hubgeometry): add PlanDir, BuilderDir, BuilderReportsDir helpers`

### Card 2: builderengine package doc and plan overview parsing

- **Context:**
  - `docs/modules/plan-format.md`
  - `_mill/discussion.md`
  - `internal/perchengine/doc.go`
- **Creates:**
  - `internal/builderengine/doc.go`
  - `internal/builderengine/plan.go`
  - `internal/builderengine/plan_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `doc.go`: package comment for `builderengine` summarizing the
  module per the discussion — LLM orchestrator over fat Go verbs, digest-only
  consumption, no DAG, recovery by fresh escalated spawn, the advance-vs-converge
  sibling relation to perch, and the engine/cli + weft-ownership split (engine
  geometry-aware, weft-blind). `plan.go`: define `Plan` (fields: `Dir string`,
  `Format int`, `Approved bool`, `Framing string`, `Batches []PlanBatch`) and
  `PlanBatch` (fields: `Number int`, `Slug string`, `Intent string`, `File string`,
  `Oversized bool`, `VerifyDeferred bool`, `ChainEnd int` — zero when absent,
  `VerifyCommand string`, `Scope []string`, `WhereFiles []string`, `CardCount int`).
  Implement `ParsePlan(planDir string) (*Plan, error)`: read `00-overview.md`, extract
  the leading `---`-fenced YAML frontmatter, strict-decode exactly two keys `format:`
  (int) and `approved:` (bool) via `yaml.Decoder.KnownFields(true)`; parse the
  `## Batch Index` list entries of the shape `NN — <batch-slug> — <one-line intent>`
  (accept both the em-dash `—` and ASCII `-`/`--` as the separator, normalizing
  whitespace); capture the body paragraph(s) before the Batch Index as `Framing`.
  Missing overview file, missing/duplicate frontmatter keys, or an unparseable index
  line are distinct wrapped errors prefixed `builder:`. This card parses ONLY the
  overview; per-batch file parsing is card 3 (`ParsePlan` calls a stub that card 3
  fills in — structure the code so card 3 extends, not rewrites).
- **Commit:** `feat(builder): builderengine package with plan overview parser`

### Card 3: per-batch file parsing

- **Context:**
  - `docs/modules/plan-format.md`
- **Edits:**
  - `internal/builderengine/plan.go`
  - `internal/builderengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `ParsePlan` to read each `NN-<batch-slug>.md` named by the
  Batch Index. Per batch file: an OPTIONAL leading `---`-fenced YAML frontmatter
  strict-decoded against exactly three known keys — `oversized:` (bool),
  `verify:` (the literal string `deferred` only; any other frontmatter `verify:` value
  is an error), `chain-end:` (int); a `## Scope` section parsed as a bullet list of
  plain paths (prefix semantics, no globs — reject entries containing `*`); a
  `## Cards` section where each `### Card N` heading increments `CardCount` and each
  `**Where:**` line's comma-separated paths accumulate into `WhereFiles`; a
  `## verify:` section whose first non-empty line is `VerifyCommand`. A batch with
  frontmatter `verify: deferred` must have NO `## verify:` body section and vice versa
  ("one or the other, never both" — plan-format.md); violation is a parse error.
  `PlanBatch.Intent` has exactly ONE source: the Batch Index one-liner card 2 already
  parsed — the batch file's `## Intent` section is prose for the implementer and is
  NOT stored on `PlanBatch`. Keep the parser line-oriented and tolerant of
  surrounding prose everywhere except the machine fields above.
- **Commit:** `feat(builder): parse per-batch plan files`

### Card 4: plan fixtures

- **Context:**
  - `docs/modules/plan-format.md`
- **Creates:**
  - `internal/builderengine/testdata/plan-valid/00-overview.md`
  - `internal/builderengine/testdata/plan-valid/01-json-flag.md`
  - `internal/builderengine/testdata/plan-valid/02-list-tests.md`
  - `internal/builderengine/testdata/plan-valid/03-refactor-a.md`
  - `internal/builderengine/testdata/plan-valid/04-refactor-b.md`
  - `internal/builderengine/testdata/plan-valid/05-oversized.md`
  - `internal/builderengine/testdata/plan-unapproved/00-overview.md`
  - `internal/builderengine/testdata/plan-unapproved/01-only.md`
  - `internal/builderengine/testdata/plan-broken-chain/00-overview.md`
  - `internal/builderengine/testdata/plan-broken-chain/01-first.md`
  - `internal/builderengine/testdata/plan-broken-chain/02-second.md`
- **Edits:**
  - `internal/builderengine/plan_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `plan-valid/` extends plan-format.md's worked example
  (batches 01–02 copied from the doc's `--json` example) with a deferred-verify chain
  (03 declares `verify: deferred` + `chain-end: 4`; 04 carries the chain's real
  `## verify:`) and an `oversized: true` batch 05 whose Intent justifies the flag.
  Batch Index, filenames, slugs, and card `**Where:**` paths stay byte-consistent.
  Scope paths in fixtures point at files that exist inside the fixture dir itself (add
  small dummy source paths under the fixture's own tree only if validation check 6
  needs them to exist — see card 6; otherwise reference the fixture's `.md` files).
  `plan-unapproved/` sets `approved: false`. `plan-broken-chain/` has 01 declaring
  `chain-end: 3` (dangling — no batch 03) and 02 declaring `chain-end: 2` (self-deferred
  target: 02 itself carries `verify: deferred`). Extend `plan_test.go` to assert
  `ParsePlan` round-trips `plan-valid` exactly (numbers, slugs, flags, chain-ends,
  scope lists, card counts, verify commands).
- **Commit:** `test(builder): hand-written plan fixtures per plan-format v1`

### Card 5: plan fingerprint

- **Context:**
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/fingerprint.go`
  - `internal/builderengine/fingerprint_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Fingerprint(planDir string) (string, error)`: SHA-256 over the
  plan's files — sort the dir's `*.md` filenames lexically, and for each write
  `name + "\x00" + contents + "\x00"` into the hash; return lowercase hex. Tests:
  identical dirs → identical fingerprint; renaming a file, editing one byte, or
  adding a batch file each change it; non-`.md` files and subdirectories are ignored.
- **Commit:** `feat(builder): plan fingerprint for resume identity`

### Card 6: the six validation checks

- **Context:**
  - `docs/modules/plan-format.md`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/validate.go`
  - `internal/builderengine/validate_test.go`
- **Edits:**
  - `internal/builderengine/testdata/plan-valid/00-overview.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Validate(plan *Plan, worktreeRoot string, caps ValidateCaps)
  []ValidationError` where `worktreeRoot` is the base check 5 resolves Scope/Where
  file paths against, `ValidateCaps` carries `ContextCapTokens int` and `CardCap int`
  (config wiring lands in batch 2; the engine type keeps validation config-free), and
  `ValidationError` has `Check string` (a stable kebab-case name), `Batch string`
  (empty for plan-level), and `Detail string`. Implement plan-format.md's checks: (1) `format-unrecognized` (only
  `1` is known) and `plan-unapproved`; (2) `index-file-mismatch` — index ↔ files
  consistent both directions, numbering has no gaps, slugs match filenames;
  (3) `verify-missing` — every batch has `VerifyCommand` or `VerifyDeferred` with a
  `ChainEnd`; (4) `chain-end-dangling` — every `ChainEnd` names an existing batch
  number that is not itself `VerifyDeferred` (and is greater than the declaring batch's
  number); (5) `batch-oversized` — context estimate (sum of byte sizes of each batch's
  `Scope` + `WhereFiles` entries that exist on disk, resolved against the
  `worktreeRoot` parameter, divided by 4) over `ContextCapTokens`, or `CardCount`
  over `CardCap`, without `Oversized: true`; (6) `scope-malformed` — scope entry list
  well-formed (non-empty, relative, clean, no `..` escapes; existence is NOT required —
  "exist or are creatable"). Order findings deterministically (check number, then batch
  number). Test each check against the fixtures: `plan-valid` yields zero findings
  (adjust the fixture only if a check exposes a genuine fixture bug — never weaken a
  check to fit the fixture); `plan-unapproved` trips check 1; `plan-broken-chain` trips
  check 4 twice (dangling target and self-deferred target); synthetic in-test plans
  trip 2, 3, 5, 6.
- **Commit:** `feat(builder): the six plan validation checks`

## Batch Tests

`verify:` runs the hubgeometry suite (new helpers + the geometry-literal enforcement
test, which proves builderengine constructs no `_lyx` tokens) and the new builderengine
suite (parser round-trip over fixtures, fingerprint properties, all six validation
checks positive + negative).
