# `fabric merge` — crucible round fable-medium-r3 fixer report

All four findings from `_mill/fabric-merge-review-fable-medium-r3.md` are closed: B1 and A1/A2 by code, C1 by record (no code exists to fix, and the prior round's report is off-limits to edit).
Nothing was deferred for size; nothing needs an operator decision.

## B1 — MergeContinue adopts a landed-but-unrecorded conclude (commit `fabric: fix B1`)

**Changed files:** `internal/fabricengine/mergelifecycle.go`, `internal/fabricengine/doc.go`, `internal/fabricengine/mergein_recovery_integration_test.go`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`.

**What was implemented.** `concludeMergeSides` gained a per-side adoption arm: when a side's recorded conclude SHA is empty but its HEAD has moved off the recorded pre-merge start with no live `MERGE_HEAD`, the landed commit is adopted into the record (and reported as `KindMergeCommitted`) instead of re-running `git commit` on a clean tree.
The adopt predicate is deliberately the same reading as `sideConcludeMayHaveLanded`'s HEAD-moved clause (minus states a live `MERGE_HEAD` still makes concludable), so `MergeContinue` can now finish exactly the states `MergeAbort` refuses — the mirror is total again, and the `ErrMergeIncomplete` message's "run MergeContinue again" is true in all four invisible states.
`doc.go` now documents the adoption arm and replaces the false "plain git in the two checkouts is the last resort" claim with the recovery that actually works (hand-commit with git, then `MergeContinue` adopts and clears the record); `SANDBOX-FABRIC-SUITE.md` F20 gained the invisible-conclude arm (no coverage-guard test references that file, so none needed updating).

**New test:** `TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking` (`//go:build integration`), both sides invisible: conflicted `MergeIn` on warp and weft, both resolved and hand-committed with plain git (byte-identical to a kill between conclude-commit and record re-save), record asserted `committed:""` on both sides, `MergeAbort` asserted to refuse, then a fresh-handle `MergeContinue` asserted to succeed with `Committed:true`, both HEADs unmoved (no second commit), both adopted SHAs present as `KindMergeCommitted` details, record deleted.

**False-green proof.** Sabotaged `sideConcludeAlreadyLanded` to always report not-landed: the test fails at its intended assertion (`MergeContinue() ... error = fabricengine: merge conclude did not finish; run MergeContinue again; want adoption to finish the merge`). Restored; diff back to the fix only.

**Determinism.** `-tags integration -count=5 -run TestMergeContinue_InvisibleLandedConclude` green (the scenario is fully synchronous real-git; no sleeps, no polling needed — every step waits on a completed subprocess).

**Live re-drive.** Re-deployed (`./deploy-dev`), then re-drove the wedged hub from the review's residual-B scenario: `lyx fabric merge --continue` returned `ok:true, committed:true` with exactly one `merge_committed` mutation carrying the hand-landed SHA `0d831d8...`, warp HEAD unmoved, record gone, `merge --abort` now `no merge in progress`, and the sibling `pull` no longer refuses with the merge guard.

## A1/A2 — the closure test now reads the const block (commit `fabric: fix A1/A2`)

**Changed files:** `internal/fabricengine/mergevocab_test.go`, `internal/fabricengine/mergeerrors_test.go`, `internal/fabricengine/mergeerrors.go` (const-block comment only).

**What was implemented.** One pinned name→value map (`pinnedMergeReasons`) is now the single test-side copy of the closed set; the new `TestMergeVocabulary_GuardReasonSetMatchesConstBlock` parses `mergeerrors.go` with `go/ast` (the `cmd/lyx/registration_test.go` precedent) and asserts two-way equality — names and verbatim values — between the pinned map and the const block actually declared.
`TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree` and `TestMergeVocabulary_ErrorsAreSideFree` now iterate the pinned map, and `TestMergeErrors_NoVocabularyLeakInReasons` does too — closing A2, where its hand-copied subset had silently drifted to 7 of 9 members.
The const block's comment now names the mechanism instead of claiming an assertion that did not exist.

**Detection proof (the finding's own bar).** Added `mergeReasonSabotageTenth = "sabotage tenth member"` to the const block — before the fix this left the entire hermetic tier green (reproduced in the review); after the fix the tier FAILS at the intended assertion: `mergeerrors.go declares mergeReasonSabotageTenth = "sabotage tenth member", which pinnedMergeReasons does not pin`. Removed the sabotage; tier green; diff contains only the intended changes.

## C1 — recorded, not fixable in code

Round 2's row-24 "else concludes a half-reset pair" arm is unreachable (all four `selfAbortMergeAttempt` call sites leave the failing side's outcome empty, so F1 always refuses). The inaccuracy lives in a prior round's report, which this round must not edit; the correction is recorded in this round's review report. No production or test code is wrong.

## Gates (final tree, tags named)

- `go build ./...` — OK.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK.
- Hermetic (no tag): `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok.
- Integration (`-tags integration`): `go test -tags integration -count=1 -timeout 30m` across fabricengine (42.1s) / fabriccli (4.1s) / gitrepo (2.2s) — all ok.
- `golangci-lint run ./internal/fabricengine/...` — clean.

## Teardown

All scratch hubs and bares removed (scratchpad empty), no stray git processes, `git status` in the worktree clean apart from the orchestrator's own `_mill/fabric-merge-review-HANDOFF.md` modification, which this round did not touch.
