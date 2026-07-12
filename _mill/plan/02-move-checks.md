# Batch: move-checks

```yaml
task: "Add typed file-ops to lyx's plan-format"
batch: "move-checks"
number: 2
cards: 4
verify: go test ./internal/builderengine/...
depends-on: [1]
```

## Rename mechanic

Not applicable — this batch has no `Moves:` entries.

## Batch Scope

This batch delivers the five mill-derived `move-*` validation checks in
`internal/builderengine/validate.go`, plus the one parser addition they need
(`PlanBatch.HasRenameMechanic`). After this batch, a plan that declares renames
wrongly — bad pair grammar, endpoint duplicated into `Creates:`/`Deletes:`, missing
source, colliding target, or a missing `## Rename mechanic` section — fails
validation loudly with a named check. Interface consumed by batch 3: nothing new
(batch 3 adds sibling checks in the same findings model); batch 5 documents the
check names pinned here.

Batch-local decision: suppression sets (`creates_union`, `moves_targets`) are
plan-wide unions, exactly mill's semantics — chained renames (batch A moves X->Y,
batch B moves Y->Z) must not false-positive.

## Cards

### Card 6: HasRenameMechanic parser flag

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/plan.go`
  - `internal/builderengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `parseBatchFile` (`plan.go`), set a new `PlanBatch.HasRenameMechanic bool`:
    true when the batch body contains a `## Rename mechanic` heading (trimmed-line
    equality on the `## `-prefixed heading, consistent with `extractSection`'s
    matching; the heading's presence is all that matters, not its body).
  - Add `plan_test.go` coverage: a batch with the section sets the flag; a batch
    without it does not; the `plan-valid` fixture's Moves batch has it set.
- **Commit:** `02.1: parse ## Rename mechanic presence into PlanBatch`

### Card 7: move-format + move-redundant checks

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/plan.go`
- **Edits:**
  - `internal/builderengine/validate.go`
  - `internal/builderengine/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `checkMoveFormat` (check name `move-format`): for every card, every
    `MovesRaw` entry yields one finding — Detail quotes the raw sub-bullet, names
    `card NN.C`, and states the expected `` `src` -> `dst` `` grammar (per the
    `findings-shape-unchanged` Shared Decision, the card citation lives in Detail).
  - `checkMoveRedundant` (check name `move-redundant`): batch-level, mill
    semantics — collect every `Moves` endpoint (Old and New) across the batch's
    cards; intersect with the union of the batch's `CreatesFiles` and
    `DeletesFiles`; one finding per conflicting path, sorted, Detail naming the
    path and saying "use Moves: or Creates:/Deletes:, not both".
  - Wire both into `Validate` after the existing six checks, and extend
    `validate.go`'s banner comment listing.
  - Tests in `validate_test.go` via synthetic in-memory `Plan` literals (existing
    style): malformed raw entry -> one `move-format` finding citing the card;
    well-formed moves -> none; endpoint duplicated in Creates -> `move-redundant`;
    rename-plus-extraction (Moves pair + a DIFFERENT Creates path) -> no finding;
    `plan-valid` fixture still zero findings.
- **Commit:** `02.2: move-format and move-redundant validation checks`

### Card 8: move-source-missing + move-target-collision checks

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/plan.go`
- **Edits:**
  - `internal/builderengine/validate.go`
  - `internal/builderengine/validate_test.go`
  - `internal/builderengine/runlevel_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Build two plan-wide suppression sets once (helper on `*Plan` or local to
    `Validate`): `createsUnion` (every card's `CreatesFiles` across all batches)
    and `movesTargets` (every `MovePair.New` across all batches).
  - `checkMoveSourceMissing` (check name `move-source-missing`): for every
    `MovePair`, if `os.Stat(filepath.Join(worktreeRoot, pair.Old))` fails AND
    `pair.Old` is in neither `createsUnion` nor `movesTargets`, emit a finding
    (Detail: source does not exist on disk and is not created or relocated by
    another batch). Mill's plan-wide-union semantics, so chained renames pass.
  - `checkMoveTargetCollision` (check name `move-target-collision`): three OR'd
    conditions per `MovePair.New`, first match wins per target: (1) target exists
    on disk (`os.Stat` against worktreeRoot); (2) more than one batch names the
    same target (count targets per batch across the plan); (3) the target appears
    in a DIFFERENT batch's `CreatesFiles` (same-batch overlap is
    `move-redundant`'s job — skip it here, mirroring mill).
  - Wire into `Validate`; tests use `t.TempDir()` as worktreeRoot with real files
    for the on-disk conditions, plus in-memory plans for the set logic: missing
    source flagged; source satisfied by another batch's Creates -> suppressed;
    chained rename (A: X->Y, B: Y->Z) -> suppressed; existing target flagged;
    two batches targeting one path -> flagged; cross-batch Creates collision ->
    flagged; `plan-valid` fixture still zero findings.
  - `runlevel_test.go`'s `newRunFixture` sets `Deps.WorktreeRoot` to a bare
    `t.TempDir()` unrelated to the copied plan-valid fixture; per the
    fixture-self-reference decision the fixture's Moves: source
    (`03-refactor-a.md`) only exists relative to the fixture dir itself, so
    the new on-disk checks (move-source-missing) now fail every `Run()` test
    that exercises the automatic validation gate. Fix: point
    `WorktreeRoot` at the same copied `planDir` `newRunFixture` already
    builds (its only consumer is the `Validate` call inside `Run`, so this is
    a pure test-fixture correction, not a behavior change).
- **Commit:** `02.3: move-source-missing and move-target-collision checks`

### Card 9: move-mechanic-missing check

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/plan.go`
- **Edits:**
  - `internal/builderengine/validate.go`
  - `internal/builderengine/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `checkMoveMechanicMissing` (check name `move-mechanic-missing`): one finding
    per batch that has at least one parsed `MovePair` across its cards but
    `HasRenameMechanic == false`. A batch whose every `Moves:` field is `none`
    (zero pairs) is skipped — `MovesRaw`-only batches are also skipped (their
    defect is `move-format`'s finding; requiring the section too would
    double-report).
  - Wire into `Validate`; tests: moves without the section -> flagged; moves with
    it -> clean; `none`-only batch without the section -> clean; `plan-valid`
    still zero findings.
- **Commit:** `02.4: move-mechanic-missing check`

## Batch Tests

`verify:` runs `go test ./internal/builderengine/...` — `validate_test.go` grows one
focused positive+negative test group per new check (synthetic plans + `t.TempDir()`
for disk-dependent conditions), and the `plan-valid` zero-findings anchor proves the
fixture survives all five new checks. Package-scoped per `package-scoped-verify`.
