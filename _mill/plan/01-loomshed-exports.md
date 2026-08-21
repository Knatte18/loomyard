# Batch: loomshed constructor exports

```yaml
task: 'Shed recipe: engine registry'
batch: 'loomshed constructor exports'
number: 1
cards: 1
verify: go test ./internal/loomshed/...
depends-on: []
```

## Batch Scope

This batch makes `internal/loomshed`'s six producer constructors reachable from outside the package, which is the precondition for six of the registry's twelve entries.
It is one batch because the rename is a single atomic edit: production files, `New`'s call sites, and the seven test files that call the constructors directly all have to move together or the package does not compile.
Nothing else in the repo calls these constructors — they are unexported today — so the blast radius is exactly `internal/loomshed`.
The external interface batch 3 consumes is the six exported names, each returning `shedengine.ShedProducer`.

Batch-local decision beyond `## Shared Decisions`: the rename is one card, not two.
Splitting production and test call sites into separate cards would leave the first card's commit with a package that fails to compile under `go test`, and a broken intermediate commit is worse than a slightly larger card.

## Cards

### Card 1: Export loomshed's six producer constructors

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/loomshed/doc.go`
  - `internal/loomshed/seam_enforcement_test.go`
- **Edits:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/webster.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/stub_test.go`
  - `internal/loomshed/batchifier_test.go`
  - `internal/loomshed/discussionvalidate_test.go`
  - `internal/loomshed/loompreflight_test.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/webster_test.go`
  - `internal/loomshed/resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rename six unexported constructors to their exported forms, keeping every parameter list and every function body byte-for-byte unchanged and widening only the declared return type:
  - `newStub(name string) *stubProducer` in `internal/loomshed/stub.go` becomes `NewStub(name string) shedengine.ShedProducer`.
  - `newBatchifier(name, anchorPath string) *batchifier` in `internal/loomshed/batchifier.go` becomes `NewBatchifier(name, anchorPath string) shedengine.ShedProducer`.
  - `newDiscussionValidate(name, decisionRecordPath, supportLogPath string) *discussionValidate` in `internal/loomshed/discussionvalidate.go` becomes `NewDiscussionValidate(name, decisionRecordPath, supportLogPath string) shedengine.ShedProducer`.
  - `newLoomPreflight(name, statusPath, statusLockPath string) *loomPreflightProducer` in `internal/loomshed/loompreflight.go` becomes `NewLoomPreflight(name, statusPath, statusLockPath string) shedengine.ShedProducer`.
  - `newPlanValidate(name, anchorPath, worktreeRoot string) *planValidate` in `internal/loomshed/planvalidate.go` becomes `NewPlanValidate(name, anchorPath, worktreeRoot string) shedengine.ShedProducer`.
  - `newWebsterProducer(name, anchorPath string, run shedadapters.WebsterRunner, deps websterengine.RunDeps) *websterProducer` in `internal/loomshed/webster.go` becomes `NewWebsterProducer(name, anchorPath string, run shedadapters.WebsterRunner, deps websterengine.RunDeps) shedengine.ShedProducer`.

  The six concrete struct types — `stubProducer`, `batchifier`, `discussionValidate`, `loomPreflightProducer`, `planValidate`, `websterProducer` — stay unexported, and each file's existing `var _ shedengine.ShedProducer = (*T)(nil)` assertion stays as it is.
  Rewrite each constructor's godoc comment to open with the new exported name, and state in each that the declared return type is the seam interface so the registry in `internal/shedrecipe` can call it from outside this package while the concrete type stays package-private.
  `NewLoomPreflight`'s existing godoc paragraph beginning "The constructor is unexported:" is now false — replace that paragraph with one stating that the constructor is exported for the `internal/shedrecipe` registry and that row 2 is still built internally by `New`, never injected, unlike `Deps.Preflight`.

  In `internal/loomshed/loomshed.go`, update the six call sites inside `New`'s `producers` slice literal to the new names, changing nothing else about the slice.
  In the seven test files listed under `Edits:`, retarget every call site to the new name and change no assertion, no fixture, and no test name — a renamed call site with an unchanged assertion is the proof this rename is behaviour-neutral.
  Do not touch `internal/loomshed/sequence_test.go` or `internal/loomshed/loomshed_test.go`: both reach the producers through `New`'s assembled list rather than by constructor name, and their staying untouched is the second half of that proof.
  Do not add or remove any import in `internal/loomshed`, and do not edit `internal/loomshed/seam_enforcement_test.go` — the dependency direction is `shedrecipe` -> `loomshed`, never the reverse, so that allowlist needs no change.
- **Commit:** `refactor(loomshed): export the six producer constructors for the shedrecipe registry`

## Batch Tests

`verify: go test ./internal/loomshed/...` runs the whole `internal/loomshed` suite, which is the correct scope: the rename touches only this package, and every one of its test files either calls a renamed constructor or exercises `New`'s assembled list.
The behaviour-neutrality claim is carried by two properties this command checks together — every renamed-call-site test still passes with its assertions unchanged, and `sequence_test.go` plus `loomshed_test.go` pass with no edit at all.
