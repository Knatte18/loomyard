# Batch: pull-non-fatal-weft

```yaml
task: "Add a local-only file category to weft"
batch: "pull-non-fatal-weft"
number: 3
cards: 4
verify: go build ./cmd/lyx && go test ./internal/fabricengine/... ./internal/fabriccli/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [2]
```

## Batch Scope

Once a status push may be rejected and warned past, a locally diverged weft becomes routine — and `Fabric.Pull` pulls the weft first and returns immediately on its error, so one rejected push would block the operator's own resume verb for the whole pair.
This batch makes `Pull`'s weft arm non-fatal: a failed weft `git pull --ff-only` warns, `PullResult` reports the weft unpulled, and the warp fetch/reconcile proceeds.
That reshapes `PartialPullError`, whose doc comment, `WeftPulled` field semantics and hardcoded `Error()` text all currently assert that a weft-side failure can never produce one.
`lyx fabric pull`'s envelope gains no key, but `weft_pulled` can now report `false` inside a success envelope — an observable CLI change this batch accepts and documents rather than hides.

Batch-local decision beyond `## Shared Decisions`: the weft **upstream probe** (`weftHasUpstream`) becomes non-fatal too, not only the pull itself.
The `pull-does-not-stall-on-weft` Decision says "`Fabric.Pull`'s weft arm becomes non-fatal", and a probe error today returns before the warp fetch through a different door than the pull error does.
Leaving it fatal would preserve the exact stall this batch exists to remove, reachable by a different failure.
The recovery stays a named manual step — `git -C <weft> reset --hard origin/<branch>` — and is never automated, because a push rejection means another machine advanced the same FSM state.

## Cards

### Card 14: Pull's weft arm warns and continues

- **Context:**
  - `internal/gitrepo/pull.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/fabric.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Fabric.Pull` (`internal/fabricengine/pull.go`):
  (a) replace the `weftHasUpstream()` error return `return PullResult{}, fmt.Errorf("fabricengine: weft pull: %w", err)` with a `logger.Warn` call and a fall-through that leaves `result.WeftPulled` false and proceeds to the warp side;
  (b) replace the `f.PullWeft(opts)` error return with the same disposition — `logger.Warn` naming the weft path and the error, `result.WeftPulled` left false, warp work proceeds;
  (c) set `result.WeftPulled = true` only on the two success paths — a completed `PullWeft`, and the no-upstream vacuous no-op — rather than unconditionally after the block, so the field is a faithful report rather than a constant;
  (d) leave the before/after `weftSHAOrEmpty` sampling and its `rec.Append(KindRepoAdvanced, f.weftPath, ...)` exactly as written, and keep it on the success path only;
  (e) change every `&PartialPullError{WeftPulled: true, ...}` construction in this function — seventeen of them — to `&PartialPullError{WeftPulled: result.WeftPulled, ...}`, so a partial error stops asserting a weft success it can no longer guarantee;
  (f) rewrite `Pull`'s own doc comment: it currently says a warp-side failure reports rather than unwinding the weft pull, which stays true, and must additionally say the weft arm is non-fatal, that the warp fetch/reconcile runs regardless, and that reconciling a diverged weft is a named manual operator step rather than something `Pull` resolves by rewriting history;
  (g) rewrite `PullResult.WeftPulled`'s field doc, deleting the sentence "Every field below is only ever populated once this is true — see Fabric.Pull's weft-first ordering", which stops holding the moment warp pulls with the weft unpulled.
  Import `internal/logger` in this file if it is not already imported.
  Leave `PullWeft`, `weftSHAOrEmpty` and `weftHasUpstream`'s own bodies unchanged.
- **Commit:** `fix(fabricengine): a weft-side pull failure no longer stalls the warp pull`

### Card 15: reshape PartialPullError's contract

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/pull.go`, rewrite `PartialPullError`'s doc comment and `Error()` method.
  The doc comment currently asserts "`WeftPulled` is always true for this type: a weft-side failure never produces a `*PartialPullError` at all, since Fabric.Pull returns immediately on that path" — that claim is false after card 14 and is replaced by the new contract: the type reports a `Fabric.Pull` call whose warp-side work did not complete, and `WeftPulled` now faithfully reports whether the weft arm completed, which may be false.
  Record the direction that has NOT changed, so a reader is not left to infer it: this type still never reports a weft-side failure on its own, because a weft-side failure alone is no longer an error at all.
  Rewrite `Error()` so it stops hardcoding "weft pull succeeded": branch on `e.WeftPulled` and emit the existing `"fabricengine: weft pull succeeded, warp %s failed: %v"` wording when it is true, and a second, distinct wording naming both failures when it is false.
  Keep the struct's field set, the `Stage` field's meaning, and `Unwrap()` exactly as written.
  Correct the file-header comment's "with the two sides' roles swapped to match Pull's weft-first ordering" sentence, which describes the superseded report-not-rollback shape.
- **Commit:** `fix(fabricengine): PartialPullError reports a weft that did not complete`

### Card 16: correct the pull contract in the package narrative and the CLI envelope

- **Context:**
  - `internal/fabricengine/pull.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`, correct the sentence around `internal/fabricengine/doc.go:40` describing "the full flow and the `*PartialPullError` weft-succeeded/warp-failed contract", so it names the reshaped contract instead.
  If the file carries a fuller pull narrative elsewhere, correct that too — grep the file for `PartialPullError`, `WeftPulled` and `weft-first` and fix every claim that a weft-side failure stops the pair.
  In `internal/fabriccli/weft_verbs.go`, amend `pullResultMap`'s doc comment with one sentence recording the observable change: `weft_pulled` can now be `false` inside a **success** envelope, meaning the warp side pulled while the weft did not, and the operator's remedy is to reconcile the weft by hand.
  Leave `pullResultMap`'s body, the envelope's key set and every other verb in that file unchanged.
- **Commit:** `docs(fabricengine): document the non-fatal weft pull arm`

### Card 17: integration coverage for the non-fatal weft pull

- **Context:**
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/merge_target_integration_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/pull_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/pull_integration_test.go`, rewrite the test at `internal/fabricengine/pull_integration_test.go:635` that asserts `result.WeftPulled` is false alongside "a weft-side failure must report the zero result", since a weft-side failure no longer produces a zero result.
  It must now assert that the warp side still fetched and advanced, that `result.WeftPulled` is false, and that the returned error is nil.
  Keep the test at `internal/fabricengine/pull_integration_test.go:476` — the no-upstream vacuous-success path — asserting `WeftPulled` true, unchanged.
  Add three new test functions to the same file:
  (1) a weft genuinely diverged from its own upstream, where `git pull --ff-only` hard-refuses — assert the warp branch still advanced to the fetched upstream tip, `WeftPulled` is false, the error is nil, and the weft HEAD is exactly where it was before the call;
  (2) the same diverged weft combined with a warp-side failure — assert the returned error is a `*PartialPullError` whose `WeftPulled` is false and whose `Error()` string does not claim the weft pull succeeded;
  (3) a healthy pair where both sides pull cleanly — assert `WeftPulled` true and the weft HEAD advanced, so the ordinary path is pinned against a regression that simply stops pulling the weft.
  Reuse the file's existing fixture helpers rather than adding new ones.
- **Commit:** `test(fabricengine): pin the non-fatal weft pull arm`

## Batch Tests

`verify:` runs `go build ./cmd/lyx`, then the untagged tier over `./internal/fabricengine/...` and `./internal/fabriccli/...`, then the `integration` tier over the same two packages.

- `./internal/fabriccli/...` is included because card 16 edits `weft_verbs.go`;
  that package's `envelopecontract_integration_test.go` and `cli_test.go` are the surface a botched envelope edit would break.
- The `integration` tier is chained separately because card 17 edits `pull_integration_test.go`, which carries `//go:build integration`.
- Card 17's rewrite of the existing weft-failure test is this batch's primary proof: it asserts the opposite disposition today, so a batch that left `Pull`'s weft arm fatal fails it.
- The scope stays two packages;
  `pipeline.done_gate` runs the repo-wide sweep at the end of the run.
