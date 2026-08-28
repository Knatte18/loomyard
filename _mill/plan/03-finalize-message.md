# Batch: finalize-message

```yaml
task: "Producer-agnostic final-summary artifact + wire Finalize"
batch: "finalize-message"
number: 3
cards: 2
verify: go test ./internal/landingshed/... && go test -tags integration ./internal/landingshed/...
depends-on: [2]
```

## Batch Scope

This batch closes the unset-`Message` gap: `Finalize` parses the final-summary artifact at the top of `Call` and sets `fabricengine.MergeOptions.Message` to the composed commit message, unconditionally.
It also adds the construction-time rejection of an empty `Deps.FinalSummaryPath` to both `NewFinalize` and `NewPublish`.

Both changes tighten preconditions that existing fixtures did not satisfy, so card 7 carries the fixture repairs alongside the production change and leaves the suite green at its own commit; card 8 then adds the new assertions.
No `fabricengine` change is required — `MergeOptions.Message` is already plumbed end to end, stored into the merge-state record and applied ahead of git's own prepared `MERGE_MSG`/`SQUASH_MSG`.

This is the batch that makes the artifact mandatory on task-to-task landings where nothing read it before.
That is intentional and is settled by the **finalize-parse-fails-loud** Decision in `_mill/discussion.md`: `Finalize` is the last row of every landing, pull-request-gated or not, and a landing commit with no composed message is the gap this task exists to close.

## Cards

### Card 7: Parse the artifact and set MergeOptions.Message

- **Context:**
  - `internal/summaryparser/summary.go`
  - `internal/landingshed/deps.go`
  - `internal/landingshed/commitstatus_test.go`
- **Edits:**
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize_test.go`
  - `internal/landingshed/finalize_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/landingshed/finalize.go`, insert the artifact parse into `Call` immediately after the `entryErr(ctx, finalizeName)` check and before step 1b's `fz.deps.CommitStatus()` block: `summary, err := summaryparser.Parse(fz.deps.FinalSummaryPath)`, returning `fmt.Errorf("landingshed: %s: parse summary artifact: %w", finalizeName, err)` on failure.
  A missing or malformed artifact is a returned error, never `Stuck` and never a silent fallback to an unset `Message`.
  The placement is load-bearing: the failure must land before any commit, any catch-up merge-in, and any parent-side mutation, so a broken run never half-lands.
  Renumber or reword the surrounding step comments only as far as the new step requires.

  Change step 4's `mergeOpts := fabricengine.MergeOptions{Squash: fz.deps.Config.Squash}` to also carry `Message: summary.CommitMessage()`.
  Set it whether or not `Config.Squash` is true — `opts.Message` is the conclude-commit message for both merge shapes, so gating on `Squash` would leave the non-squash landing commit with today's unset message.
  Step 5's retry reuses the same `mergeOpts` value, so no second assignment is needed; do not introduce one.
  Add the `internal/summaryparser` import.

  In `NewFinalize`, reject an empty `deps.FinalSummaryPath` with `fmt.Errorf("landingshed: NewFinalize: Deps.FinalSummaryPath must not be empty")`, placed alongside the existing nil-closure rejections and before the `deps.OpenFabric()` call.
  In `internal/landingshed/publish.go`'s `NewPublish`, add the equivalent rejection naming `NewPublish`, so the two constructors carry a distinct error each.
  Validate only this one field: `Deps`' other string fields are unvalidated in these constructors today and stay that way — widening that check is out of scope.

  In `internal/landingshed/finalize_test.go`, change the shared `newFinalizeDeps(t)` helper to write a well-formed artifact into a `t.TempDir()` path and assign that path to `FinalSummaryPath`.
  That one helper is what every in-package `&Finalize{...}` literal in `finalize_test.go` and in `internal/landingshed/commitstatus_test.go` builds its `Deps` from, so fixing it there covers all of them.
  Do not patch those call sites individually.

  In `internal/landingshed/finalize_integration_test.go`, the one in-package site that builds a `landingshed.Deps` literal directly rather than through `newFinalizeDeps`, write a real artifact to disk and set `FinalSummaryPath` to it.
  A non-empty path string alone is not enough there: that test calls a real `fz.Call(ctx)` and would fail the new top-of-`Call` parse.
  This file is `//go:build integration`-tagged.
- **Commit:** `feat(landingshed): compose the landing commit message from the final-summary artifact`

### Card 8: Assert the composed message and the new rejections

- **Context:**
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/deps.go`
  - `internal/summaryparser/summary.go`
- **Edits:**
  - `internal/landingshed/finalize_test.go`
  - `internal/landingshed/publish_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add coverage to `internal/landingshed/finalize_test.go` using the existing in-package `recordingParentMerger` fake, which already records every call's `fabricengine.MergeOptions`.
  Every test added here is untagged Tier 1: no `gitexec.Run`, no `exec.Command`, no `gitkit.Copy*`, no `hubforge.NewHub`.

  Assert that a successful merge passes `MergeOptions` carrying both the composed message and the configured `Squash` value, with one case for `Squash: true` and one for `Squash: false` — the message must be set either way.
  The expected message is the artifact's title, a blank line, and its trimmed body, matching `(*summaryparser.Summary).CommitMessage`.

  Assert that a missing artifact and a malformed artifact each make `Call` return an error with no merge attempted and no status commit performed.
  Override `newFinalizeDeps`'s `FinalSummaryPath` locally in these two cases rather than changing the helper.
  Use the injected `CommitStatus` closure and the `recordingParentMerger`'s own call list as the two recorders that prove the parse runs before either — `len(merger.calls)` must be 0 and the commit closure must never have fired.

  Assert the step-5 retry path: script the `recordingParentMerger` to return a `*fabricengine.ErrMergeInRequired` on the first call and succeed on the second, then check the retry call's `MergeOptions` carries the same composed message as the first.

  Assert `NewFinalize` rejects an empty `FinalSummaryPath`, alongside the existing `TestNewFinalize_RejectsNilOpenFabric` and `TestNewFinalize_RejectsNilOpenParentFabric` tests.
  Add the matching `NewPublish` rejection test to `internal/landingshed/publish_test.go`, beside its own existing nil-closure rejection tests.
- **Commit:** `test(landingshed): assert the composed merge message and the empty-path rejections`

## Batch Tests

`verify` runs `internal/landingshed`'s untagged Tier 1 suite and then its `//go:build integration` tier, because this batch changes both: `finalize_test.go`, `commitstatus_test.go`, and `publish_test.go` are untagged, while `finalize_integration_test.go` and `publish_integration_test.go` are tagged and both build a `landingshed.Deps` the new constructor gate and the new top-of-`Call` parse now police.
Running only the untagged half would leave the two fixture repairs card 7 makes unexecuted.

The scope is exactly `internal/landingshed` — it is the only package whose code or fixtures this batch touches.
Per the **Not covered** section of `_mill/discussion.md`'s Testing plan, no integration-tier assertion that the real squash commit's message equals the composed string is added: `fabricengine`'s own suite already covers `opts.Message` reaching the conclude commit, and asserting that the producer passes the right `MergeOptions` is the boundary this task owns.
`internal/landingshed/finalize_integration_test.go` is nonetheless the natural site for anyone who later wants that end-to-end assertion, since it is the one place a real squash commit is produced.
