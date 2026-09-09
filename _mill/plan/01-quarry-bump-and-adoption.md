# Batch: quarry-bump-and-adoption

```yaml
task: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()
batch: quarry-bump-and-adoption
number: 1
cards: 4
verify: CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./... && CGO_ENABLED=1 go test -tags integration ./...
depends-on: []
```

## Batch Scope

This batch delivers the whole task: it moves `go.mod`'s `github.com/Knatte18/quarry` pin from `v0.1.0` to `v0.2.0`, replaces the two hand-rolled vocabulary guards in `internal/planglyph` with the primitives v0.2.0 ships (`quarry.Status.Known()` and `quarry.ResolveResult.Rejected()`), adds the drift-guard test that turns a future widening of quarry's status vocabulary into a build failure, and updates the prose that still recites lyx's own copy of the four-value enumeration.
It is one batch because every card lands in `internal/planglyph` against the same handful of files, and because the two adoption cards do not compile until card 1's bump is in place — splitting them would buy no isolation and would make each batch re-read the same context.

There is no external interface for a later batch to consume: this is the only batch.

Batch-local decision, differing from nothing in `## Shared Decisions`: card 4's new test file is `status_completeness_test.go`, a sibling of the package's existing `status_enforcement_test.go` and `chokepoint_enforcement_test.go`, rather than an addition to `donecheck_test.go`.
The package already keeps one enforcement concern per file, and this test's subject is quarry's vocabulary rather than the done rules.

## Cards

### Card 1: Bump the quarry dependency to v0.2.0

- **Context:**
  - `CLAUDE.md`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** From the repository root, run `go get github.com/Knatte18/quarry@v0.2.0` and then `go mod tidy`, so that `go.mod`'s `require` block names `github.com/Knatte18/quarry v0.2.0` and `go.sum` carries the matching hashes.
  Do not hand-edit either file — hand-editing `go.mod` leaves `go.sum` stale and the build broken.
  Both commands, and the `CGO_ENABLED=1 go build ./...` that must follow, need `CGO_ENABLED=1` and a C compiler on `PATH`, because quarry's engine links tree-sitter's C grammars and its own cgoguard fails the build outright under `CGO_ENABLED=0`.
  No `.go` file changes in this card.
  The v0.1.0 to v0.2.0 diff was verified purely additive during discussion — `Statuses`, `Status.Known()` and `ResolveResult.Rejected()` added, no exported identifier removed, no signature changed — so if `CGO_ENABLED=1 go build ./...` reports an error in any package after the bump, that contradicts the additive-only premise: stop and report it rather than editing call sites to accommodate it.
- **Commit:** `build(deps): bump github.com/Knatte18/quarry to v0.2.0`

### Card 2: Guard doneCheckVerdicts with quarry.Status.Known()

- **Context:**
  - `internal/planglyph/repo.go`
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/donecheck_test.go`
  - `internal/planglyph/status_enforcement_test.go`
- **Edits:**
  - `internal/planglyph/donecheck.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `doneCheckVerdicts` in `internal/planglyph/donecheck.go`, replace the four-case vocabulary switch on `r.Status` — the one whose single `case` lists `quarry.StatusFound`, `quarry.StatusMultipart`, `quarry.StatusAmbiguous`, `quarry.StatusNotFound` with an empty body and whose `default` arm appends the `glyph-rejected` finding and continues — with a single `if !r.Status.Known() { ... }` block.
  The block's body is the old `default` arm's body character-for-character: the same `findings = append(...)` call with the same four struct field expressions (`Check` set to `glyph-rejected`, the same `Card` expression, `Detail` from `unreadableStatusDetail` with the same three arguments, `Severity` set to `SeverityBlocking`), followed by the same `continue`.
  The guard keeps its position: it stays after the `index` lookup and its `ErrQuarryUnavailable` return, and before the `resolved` and `stillExists` assignments.
  Leave the `resolved` and `stillExists` assignments and the whole `switch e.checkID` below them exactly as they are — this card changes the guard and nothing else.
  Rewrite the guard's own doc comment (the block immediately above it, which currently opens "Fail closed on an answer outside quarry's four-value vocabulary") so it names `quarry.Status.Known()` as the predicate and `quarry.Statuses` as the vocabulary's owner, instead of asserting lyx's own four-value list.
  Keep the comment's existing substance: why the guard runs before either boolean, that the old single boolean folded a pre-resolution rejection into "the target is gone" and failed OPEN for the delete and rename-old directions, and the two crucible-round citations (`opus-high-r9`'s R9-6 and `fable-high-r10`'s F1).
  The `("donecheck.go", "doneCheckVerdicts")` pair in `allowedStatusConsumers` stays live and unedited: `r.Status.Known()` still reads a `Status` selector inside that function, so the tripwire still hits it and still needs the entry.
- **Commit:** `refactor(planglyph): guard done-check verdicts with quarry Status.Known()`

### Card 3: Spell unreadableStatusDetail's rejection test as Rejected()

- **Context:**
  - `internal/planglyph/donecheck.go`
  - `internal/planglyph/create.go`
  - `internal/planglyph/containment.go`
  - `internal/planglyph/handle.go`
  - `internal/planglyph/status_enforcement_test.go`
  - `internal/quarrycli/resolve.go`
- **Edits:**
  - `internal/planglyph/resolve.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `unreadableStatusDetail` in `internal/planglyph/resolve.go`, replace the condition `r.Status == ""` with `r.Rejected()`.
  Both `fmt.Sprintf` format strings and all of their arguments stay exactly as they are, including the second branch's trailing `r.Status` argument.
  Update the function's own doc comment so the sentence that currently reads "an absent Status is quarry's pre-resolution rejection" names `quarry.ResolveResult.Rejected` as the predicate that asks the question, keeping the rest of the comment — the noun-parameter explanation and the one-renderer-per-package rationale — unchanged.
  The `("resolve.go", "unreadableStatusDetail")` pair in `allowedStatusConsumers` stays live and unedited: the second branch still reads `r.Status`, so the tripwire still hits this function.
  The function's four callers — `statusFindings` and `createFindings` and `resolveContainment` and `doneCheckVerdicts` — are unchanged by this card; the swap is inside the callee only.
  `internal/planglyph/handle.go` is not involved: its `CanonicalizeHandles` tests `res.Error != ""` on a `quarry.NameResult`, a different type with no `Rejected` method, and its `renameDeclSource` compares against `quarry.StatusFound` by design rather than guarding a vocabulary.
  This card's commit message carries a body, not just the subject line below.
  The body must be one paragraph recording the follow-up this task deliberately does not fix, naming the file, the line, and the one-line change: that `describeRejectedResolve` in `internal/quarrycli/resolve.go` (line 80) spells its pre-resolution-rejection test as `r.Error != ""` where `r.Rejected()` is now the correct spelling, that quarry's own `Rejected` doc comment names the `Error != ""` spelling as the rejected alternative because it reads false for a rejection whose message happens to be empty while `Status == ""` still holds, that the consequence today is `describeRejectedResolve` falling through and printing an empty status in that case, and that the fix is the same one-line swap this commit makes one package over — left out here because `internal/quarrycli` renders quarry's answer verbatim to the operator rather than gating a plan decision on it, so widening a bump-and-adopt commit past its stated surface is the operator's call.
  The paragraph exists because `_mill/` never reaches `main`, so the commit body is the only durable place this follow-up survives the merge.
- **Commit:** `refactor(planglyph): spell the resolve rejection test as Rejected()`

### Card 4: Add the vocabulary-completeness drift guard

- **Context:**
  - `internal/planglyph/donecheck.go`
  - `internal/planglyph/donecheck_test.go`
  - `internal/planglyph/repo.go`
  - `internal/planglyph/resolve.go`
  - `internal/planglyph/testmain_test.go`
- **Edits:**
  - `internal/planglyph/status_enforcement_test.go`
- **Creates:**
  - `internal/planglyph/status_completeness_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/planglyph/status_completeness_test.go`, an untagged test file in package `planglyph`, carrying a file doc comment that states what the file guards and why: `quarry.Status.Known()` moved the definition of "readable" from lyx to quarry, so a status quarry adds later would pass the guard in `doneCheckVerdicts` and land on the `resolved` and `stillExists` booleans, which derive only from `quarry.StatusFound`, `quarry.StatusMultipart` and `quarry.StatusNotFound` — a fifth status would read as not-resolved-but-still-existing and produce an ordinary-looking verdict rather than the honest `glyph-rejected` diagnostic.
  The file declares a package-level expectation table keyed by `quarry.Status`, one entry per status a human has consciously taught `doneCheckVerdicts` to handle, whose value names the expected outcome for all four of the `checkID` arms `doneCheckVerdicts` switches on — `create-not-done`, `delete-not-done`, `rename-not-done-old` and `rename-not-done-new` — with today's four entries as follows.

| status | `create-not-done` | `delete-not-done` | `rename-not-done-old` | `rename-not-done-new` |
| --- | --- | --- | --- | --- |
| `quarry.StatusFound` | no finding | fires | fires | no finding |
| `quarry.StatusMultipart` | no finding | fires | fires | no finding |
| `quarry.StatusAmbiguous` | fires | fires | fires | fires |
| `quarry.StatusNotFound` | fires | no finding | no finding | fires |

  Write three assertions, in this order.
  First and most important, the coverage assertion, which is the whole reason the file exists: the table's key set and `quarry.Statuses` cover each other exactly in both directions — every element of `quarry.Statuses` has a table entry, so a status quarry adds fails here, and every table key appears in `quarry.Statuses`, so a status quarry removes fails here too.
  Both directions must be genuine set membership tests.
  A length comparison is not acceptable, because it passes a same-size swap of one status for another, and sizing a range loop off the length of `quarry.Statuses` asserts nothing at all.
  Second, per status, drive `doneCheckVerdicts` through its existing seam — a hand-built `index` of type `map[string]quarry.ResolveResult` holding a synthetic result carrying that status, and one `doneCheckEntry` per `checkID` — and compare the findings against that status's table row.
  The two rename arms both emit a `Finding` whose `Check` field is `rename-not-done`, so an assertion on `Check` alone cannot tell them apart: drive each arm with its own separate `doneCheckEntry` and assert on the entry that produced the finding or on the finding's `Detail`, not on `Check` alone.
  Third, and separately from the table, assert that the zero-value status — the empty string, with `Error` and `Reason` populated, which is quarry's pre-resolution rejection shape — and a synthetic out-of-vocabulary status string each produce exactly one blocking `glyph-rejected` finding for every one of the four arms and no create, delete or rename verdict, and that the two cases render the two different `Detail` strings `unreadableStatusDetail` produces for them: the rejected-before-resolution wording for the zero value and the unrecognized-resolve-status wording for the synthetic string.
  Do not assert that every value in `quarry.Statuses` avoids `glyph-rejected`.
  That would hold for a hypothetical fifth status too, since `Known()` would admit it, so it cannot detect the drift this file exists to catch — say so in a comment beside the coverage assertion so a later author does not add it.
  The file spawns no process and touches no repository, which is what keeps it untagged under the Test Tier Purity Invariant; the package's `TestMain` already wires the hermetic git environment and needs no change.
  This test is green against today's four statuses both before and after card 2's swap.
  That is intended — it is a drift guard designed to fail on a future quarry release, and it must not be reshaped into a failing-first test.
  Separately, update the file doc comment of `internal/planglyph/status_enforcement_test.go` so that where it recites the closed four-value vocabulary and where it instructs a future author to write "a switch with a default arm, or a boolean derived only after a vocabulary guard", it names `quarry.Status.Known()` as the canonical spelling of that guard and `quarry.Statuses` as the vocabulary's owner.
  Leave `allowedStatusConsumers` and every one of its six pairs exactly as they are — this task adds and renames no `.Status` consumer function — and leave the file's three test functions and `statusHitsIn` unchanged.
- **Commit:** `test(planglyph): guard quarry's status vocabulary against silent widening`

## Batch Tests

`verify:` is `CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./... && CGO_ENABLED=1 go test -tags integration ./...`.

**Why the unbounded whole-repository suite rather than a scope narrowed to `./internal/planglyph/...`.**
Card 1 is a dependency bump, which is a cross-cutting change by construction: it moves the pin every package importing quarry compiles against — `internal/planglyph`, `internal/planparser`, `internal/quarrycli` and `cmd/lyx`.
The whole point of the discussion's additive-only API finding is that those packages compile and pass *untouched* at v0.2.0, and a scoped verify cannot demonstrate that.
The `go build ./...` leg is what catches a removed or re-signatured identifier anywhere in the module, and the two `go test` legs are what catch a behavioural change behind an unchanged signature.
`CGO_ENABLED=1` is mandatory on every leg per the Quarry CGO Requirement Invariant: quarry's cgoguard fails the build outright without it.

**Why the tagged leg is present as well.**
`internal/planglyph/donecheck_integration_test.go` is `integration`-tagged and exercises `DoneChecks`, whose pure half is exactly what card 2 edits; a plain `go test ./...` would not even compile it.
The task's own regression list names that file's scenarios among the behaviour that must stay green, and the hub's `pipeline.done_gate` already runs the same pair of legs, so this matches the gate the task must pass at handoff anyway rather than deferring the tagged tier to it.

**What the legs cover for each card.**
Card 1 is verified by `go build ./...` alone — no `.go` file changes, so a green build is the additive-only claim holding.
Cards 2 and 3 are verified by the existing untagged tests in `internal/planglyph/donecheck_test.go` passing **unedited**: `TestDoneCheckVerdicts_Rules` pins every readable status against every check direction, including F1's ambiguous-blocks-a-deletion rows, and `TestDoneCheckVerdicts_UnreadableStatusFailsClosed` pins both unreadable shapes to `glyph-rejected` across all four directions.
An edit to either test is the signal that a swap changed behaviour and is wrong.
`TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass` covers the `ErrQuarryUnavailable` path that sits above the guard.
Card 4 adds `internal/planglyph/status_completeness_test.go` and is verified by that file plus the two AST tripwires that must keep passing without allowlist edits: `TestStatusEnforcement_NoOutOfAllowlistConsumer` in `internal/planglyph/status_enforcement_test.go` and the pins in `internal/planglyph/chokepoint_enforcement_test.go`.
