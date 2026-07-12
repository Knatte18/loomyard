# Batch: card-checks

```yaml
task: "Add typed file-ops to lyx's plan-format"
batch: "card-checks"
number: 3
cards: 3
verify: go test ./internal/builderengine/...
depends-on: [2]
```

## Rename mechanic

Not applicable — this batch has no `Moves:` entries.

## Batch Scope

This batch completes the v2 validator: the per-card structural checks
(`card-missing-field`, `card-field-overlap`, the `scope-malformed` extension to card
paths), the numbering checks (`card-numbering`, `card-count-mismatch`), and the
cross-referencing checks (`path-missing`, `card-outside-scope`,
`commit-subject-mismatch`). After this batch the full v2 check set is live and
`docs/modules/plan-format.md`'s "Validation checks" list (batch 5) can be written
against final names. Depends on batch 2 because both batches edit `validate.go` /
`validate_test.go` (ordered, no parallel overlap) and reuse its plan-wide
suppression-set helper.

Batch-local decision: `card-outside-scope` reuses `pathCovers` from `digest.go`
(same package, boundary-aware prefix semantics) — no new path-matching code.

## Cards

### Card 10: card-missing-field, card-field-overlap, scope-malformed on card paths

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/validate.go`
  - `internal/builderengine/validate_test.go`
  - `internal/builderengine/plan.go`
  - `internal/builderengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `checkCardMissingField` (check name `card-missing-field`): one finding per
    absent field per card — `HasWhat`, `HasContext`, `HasEdits`, `HasCreates`,
    `HasDeletes`, `HasMoves` each false yields a finding whose Detail names
    `card NN.C` and the missing label (`What:`, `Context:`, ...). Optional
    `Commit:`/`verify:` are never flagged.
  - `checkCardFieldOverlap` (check name `card-field-overlap`): per card, a path
    appearing in more than one of `ContextFiles`/`EditsFiles`/`CreatesFiles`/
    `DeletesFiles` or as a `Moves` endpoint yields one finding per duplicated
    path, Detail naming the card and both fields. Cross-card overlap within a
    batch is NOT flagged (discussion `typed-card-fields`: Creates-then-Edits
    across cards is legitimate; only `Moves:` endpoints are batch-level, via
    `move-redundant`).
  - Extend `checkScopeMalformed` to also run `scopeEntryMalformedReason` over
    every card's five normalized field lists and both `Moves` sides, emitting the
    existing `scope-malformed` check name with the card cited in Detail
    (discussion `validator-check-set`: scope-malformed is reused, no new name).
  - Wire into `Validate`; tests (synthetic plans, existing style): each missing
    field flagged individually; `none`-sentinel (empty non-nil) NOT flagged;
    same path in Edits+Creates of one card flagged; Creates in card A + Edits in
    card B of the same batch NOT flagged; a `..`/absolute card path flagged as
    `scope-malformed` citing the card; `plan-valid` still zero findings.
- **Commit:** `03.1: card-missing-field, card-field-overlap, card-path scope-malformed`

### Card 11: card-numbering + card-count-mismatch

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
  - `checkCardNumbering` (check name `card-numbering`): per batch, (a) every
    card's `BatchPrefix` must equal the batch's own `Number` (Detail names the
    heading's `NN.C` and the expected `NN`); (b) card `Number`s must run 1..M
    sequentially in file order — one finding per violation (gap, duplicate, or
    wrong start), mirroring `checkIndexFileConsistency`'s sequence-check style.
  - `checkCardCountMismatch` (check name `card-count-mismatch`): per batch, one
    finding when `IndexCardCount != len(Cards)`, Detail giving both numbers.
  - Wire into `Validate`; tests: wrong batch prefix; duplicate C; gap in C;
    index says 3 cards but file has 2; `plan-valid` still zero findings.
- **Commit:** `03.2: card-numbering and card-count-mismatch checks`

### Card 12: path-missing, card-outside-scope, commit-subject-mismatch

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/plan.go`
  - `internal/builderengine/digest.go`
- **Edits:**
  - `internal/builderengine/validate.go`
  - `internal/builderengine/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `checkPathMissing` (check name `path-missing`): for every card path in
    `ContextFiles`, `EditsFiles`, and `DeletesFiles` (NOT `CreatesFiles` — those
    are new by definition; NOT `Moves` sources — `move-source-missing` owns them):
    if `os.Stat(filepath.Join(worktreeRoot, p))` fails AND `p` is in neither the
    batch-2 `createsUnion` nor `movesTargets` plan-wide sets, emit one finding
    citing the card (mill's `non-existent-path`, adapted per discussion
    `validator-check-set` item 9; reuse the batch-2 suppression-set helper).
  - `checkCardOutsideScope` (check name `card-outside-scope`): for every card path
    in `EditsFiles`/`CreatesFiles`/`DeletesFiles` and every `Moves` endpoint
    (`ContextFiles` exempt — reading outside scope is legitimate): if no batch
    `Scope` entry covers it under `pathCovers`'s boundary semantics, emit one
    finding citing the card. A batch with an empty `Scope` yields no findings from
    this check (nothing declared, nothing to contradict — Scope presence itself is
    v1-status-quo territory).
  - `checkCommitSubjectMismatch` (check name `commit-subject-mismatch`): for every
    card with non-empty `Commit`, the value must start with the exact prefix
    `fmt.Sprintf("%02d.%d: ", batch.Number, card.Number)`; otherwise one finding
    with Detail quoting the value and the expected prefix.
  - Wire all three into `Validate`; final order of the full check list inside
    `Validate` and the banner comment updated to enumerate every v2 check. Tests:
    missing Edits path flagged; Edits path satisfied by another batch's Creates
    suppressed (t.TempDir + in-memory sets); Edits outside Scope flagged while the
    same path in Context is not; `internal/foo` scope must NOT cover
    `internal/foobar` (boundary case); Commit `02.3: x` on card 3 of batch 2
    clean, wrong prefix flagged; `plan-valid` still zero findings.
- **Commit:** `03.3: path-missing, card-outside-scope, commit-subject-mismatch`

## Batch Tests

`verify:` runs `go test ./internal/builderengine/...` — `validate_test.go` gains
positive+negative coverage per check as specified in each card, and the `plan-valid`
zero-findings anchor now proves the fixture passes the COMPLETE v2 check set.
Package-scoped per `package-scoped-verify`.
