# Batch: planparser-checks

```yaml
task: 'webster: rewrite for flat card list'
batch: planparser-checks
number: 2
cards: 4
verify: go test ./internal/planparser/...
depends-on: [1]
```

## Batch Scope

Add the format's 14 validation checks to `internal/planparser` as `Validate(plan *Plan,
worktreeRoot string) []ValidationError`, mirroring the frozen v2 validator's structure
(`internal/builderengine/validate.go`: one free function per check, results appended in fixed
order, a single `ValidationError{Check, Card, Detail}` finding type — NOT the v2 batch-keyed
one) while dropping every v2-only check and adding the v3-only `depends-on-order`. This batch
delivers the checks and their hermetic fixtures; it does not touch parsing (batch 1). The
external interface batches 7 and 9 consume is `Validate` + `ValidationError`.

## Cards

### Card 6: validation types + structural/format checks

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/validate.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/validate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `validate.go` define `ValidationError struct { Check string; Card string; Detail string }` with `Error() string` rendering `"check[/card]: detail"` (`Check` = stable kebab-case name matching the spec; `Card` = card number/slug identifier, replacing v2's batch key). Define `Validate(plan *Plan, worktreeRoot string) []ValidationError` that runs the 14 checks in the spec's fixed order and returns the concatenated findings (no `ValidateCaps` — the v2 oversized cap is dropped). Implement the format/structure checks in this card: `format-unrecognized`/`plan-unapproved` (`Format` recognized and `Approved == true`); `index-file-mismatch` (Card Index ↔ card files agree on numbering/slugs, no gaps, no orphaned file on disk — this absorbs v2's dropped `card-count-mismatch`); `card-numbering` (flat `N` runs 1..M with no gaps/dupes, file `NN` prefix equals heading `N`); `card-missing-field` (each card has `What:`/`Context:`/`Edits:`/`Creates:`/`Deletes:`/`Moves:`/`Depends-on:` present — now including `Depends-on:`); `card-field-overlap` (no path in more than one of a single card's Context/Edits/Creates/Deletes or Moves endpoints — per-card only). Do NOT port the dropped v2 checks `verify-missing`, `chain-end-dangling`, `batch-oversized`, `card-outside-scope`, `card-count-mismatch`.
- **Commit:** `feat(planparser): validation types and format/structure checks`

### Card 7: card-path and Moves grammar checks

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/validate.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the path/move-grammar checks to `validate.go`: `card-path-malformed` (every card path — all five fields, both `Moves:` endpoints, after normalization — is non-empty, relative, clean, no `..` escape; keys on the malformed markers Card 4 left in place); `move-format` (every non-`none` `Moves:` sub-bullet matches the `` `src` -> `dst` `` grammar — validate against `Card.MovesRaw`); `move-redundant` (a path is both a `Moves:` endpoint and in `Creates:`/`Deletes:` anywhere in the plan); `move-mechanic-missing` (plan has ≥1 `Moves:` pair but `Plan.RenameMechanic` is empty — now plan-level, using the section batch 1 extracted). Reuse local helpers analogous to the frozen `createsUnion(plan)` and `movesTargetsUnion(plan)`; reimplement, do not import builderengine.
- **Commit:** `feat(planparser): card-path and Moves grammar checks`

### Card 8: existence-dependent + depends-on checks

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/validate.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the remaining checks to `validate.go`, all keyed off `worktreeRoot` for on-disk existence: `move-source-missing` (a `Moves:` source neither exists under `worktreeRoot` nor is a `Creates:` target or `Moves:` destination of any card); `move-target-collision` (a `Moves:` target already exists on disk, is targeted by more than one card, or collides with a different card's `Creates:`); `path-missing` (an `Edits:`/`Deletes:`/`Context:` path — a `Moves:` source is `move-source-missing`'s job — that does not exist under `worktreeRoot` and is not a `Creates:` target or `Moves:` destination of any card); `commit-subject-mismatch` (a present `Commit:` value not starting with the card's own `N: ` prefix); `depends-on-order` (a card's `Depends-on:` names a card at/after its own position, or an id referencing no existing card). Add a local `pathExistsOnDisk(worktreeRoot, p string) bool` helper (join under `worktreeRoot`, `os.Stat`). This is the sole place `worktreeRoot` is consulted, matching the frozen validator's design.
- **Commit:** `feat(planparser): existence-dependent checks and depends-on-order`

### Card 9: validation tests with hermetic on-disk fixtures

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/builderengine/validate_test.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/parse.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/validate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write Tier-1 table-driven tests in `validate_test.go` covering EACH of the 14 checks with at least one triggering and one clean case: `format-unrecognized`/`plan-unapproved`, `index-file-mismatch`, `card-path-malformed`, `move-format`, `move-redundant`, `move-source-missing`, `move-target-collision`, `move-mechanic-missing`, `card-missing-field`, `card-field-overlap`, `card-numbering`, `path-missing`, `commit-subject-mismatch`, `depends-on-order`. Use the batch-1 golden fixture as the clean happy-path (zero findings). For existence-dependent checks (`move-source-missing`, `move-target-collision`, `path-missing`) build the plan model in a `t.TempDir()` and, for the clean/zero-findings happy path, MATERIALIZE under that tempdir the actual target files the golden fixture's cards reference — every `Edits:`/`Context:` path and every `Moves:` SOURCE (e.g. `internal/boardcli/list.go`, `internal/boardengine/rows.go`, `internal/boardcli/list_test.go`, the `//`-escaped `internal/output/envelope.go` / `cmd/lyx/helptree_test.go`) — while deliberately NOT creating the `Moves:` DESTINATION (`rowsjson.go`) or any `Creates:` target, so `path-missing`/`move-source-missing`/`move-target-collision` all pass and the happy path yields zero findings. For each triggering case, omit or add exactly the one file that flips the check. Pass that tempdir as `worktreeRoot` — so the on-disk-vs-plan-declared distinction is exercised hermetically without touching the actual repo tree and WITHOUT spawning git (still Tier-1, no `TestMain`). Assert on `ValidationError.Check` names and cardinality. Additional per-check malformed fixtures may be materialized under `internal/planparser/testdata/` as needed. Follow `golang:golang-testing` conventions.
- **Commit:** `test(planparser): table-driven coverage for all 14 validation checks`

## Batch Tests

`verify: go test ./internal/planparser/...` runs Tier-1 table tests for all 14 checks. The
three existence-dependent checks use `t.TempDir()` fixtures (real files, no git), so the whole
package stays untagged and hermetic — no `//go:build integration`, no `TestMain`.
