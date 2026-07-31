All 7 cards in the `empty-commit-rule` batch are committed (7 of 7 — verified by matching each commit subject in `git log adb534eb..HEAD --oneline` against the batch file's `Commit:` messages for cards 15-21). The batch's `verify:` command passed for both target package trees (`internal/fabricengine` and `cmd/lyx`, integration tag, with the two documented pre-existing-failure skips) - confirmed by building each package's test binary separately and running it with `-test.skip` and `-test.count=1` (a self-contained `-flag=value` token form was required to route around a Cobra `SetArgs(nil)`-falls-back-to-`os.Args` quirk that any stray raw argv token — including Go's own `-test.*` flags passed as separate tokens — can trip in `cmd/lyx`'s "no args" test cases; this is pre-existing tooling behavior unrelated to any of this batch's edits, since `cmd/lyx` source was untouched).

Key files touched:
- `internal/fabricengine/commit.go` — widened `Commit`'s `weftSide` predicate; reworded `PartialCommitError`'s doc/`Error()` for the no-warp-commit case.
- `internal/fabricengine/weftgit.go` — added `commitEmptySnapshot`; wired the empty-commit fall-through into `commitWeftLocked`'s three early-return points; rewrote `commitWeftLocked`/`CommitWeft` godoc.
- `internal/fabricengine/index.go` — expanded `RebuildIndex`'s comment on the now-routine correspondence overwrite.
- `internal/fabricengine/commit_integration_test.go` — inverted the warp-only-tags test, added the unchanged-content regression, and 11 more cases covering every triggering path and both exceptions.
- `internal/fabricengine/snapshot_integration_test.go` — correspondence-overwrite and dangling-baseline tests.
- `internal/fabricengine/doc.go` — package-doc section documenting the empty-commit rule.

One item explicitly documented rather than tested: the "did not match any files" `StageAndCommit` tolerance path plus tags could not be reached independently of `weftPathspecFilter`'s own pre-check within a single `commitWeftLocked` call (both use the identical anchor/pathspec semantics, synchronously, with no window for divergence) — recorded as a comment in `commit_integration_test.go` per the card's explicit escape hatch rather than shipped as a contrived/flaky test.

{"status":"success","commit_sha":"58cb0c1816b411feaf1eea89809c458499dfccd3","session_id":"f68ab44c-b9a2-4d4c-b5cd-f814b1f84317","cards_done":[15,16,17,18,19,20,21]}
