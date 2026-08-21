# fabric merge surface — fixer report (round 9, tag `opus-high-r9`)

Companion to `_mill/fabric-merge-review-opus-high-r9.md`.
All four recorded findings are fixed, each committed on its own as it landed green. Nothing pushed.

## Summary

| Finding | Severity | Status | Commit |
|---|---|---|---|
| F1 — `concludeMergeSides` commits whatever git merge is live, with no evidence it is this merge's | MEDIUM | FIXED | `8dfa1464` |
| F2 — `MergeContinue`'s unresolved-conflicts refusal names no path | LOW | FIXED | `0398bac0` |
| F3 — three runtime remedies name engine methods no shipped CLI verb offers | LOW | FIXED | `7cb9f3fd` |
| F4 — `resolveMergeSources` godoc claims a disjointness the code does not have | NIT | FIXED | `b4b6c1b7` |

Deferred / not-fixed: none. Every finding was fixable inside this round's scope.

## F1 — the conclude arm now demands evidence

**Change.** `MergeContinue` gained a fourth aggregated precondition, `checkout no longer carries the recorded merge` (`recordedMergeGoneReason`/`sideRecordedMergeGone` in `mergeguards.go`, wired into `MergeContinue`'s existing reason set). A side pending a conclude passes it three ways, and the three-way shape is what makes the fix precise rather than blunt:

1. `MERGE_HEAD` is exactly the recorded per-side source SHA and nothing else — the ordinary case.
2. the recorded source is already an ancestor of that side's HEAD — the merge landed, whatever else has happened since.
3. no merge is live AND the checkout is clean — `git commit` fails on its own there, so the honest `*ErrMergeIncomplete` already comes back.

Exit 2 is what keeps the precondition from wedging states `MergeAbort` also refuses (the adoption crash, a conclude with a second merge on top, a source merged onto a wrong base). Exit 3 is what keeps it from **blinding** the four adoption-evidence tests, every one of which builds exactly that state — the first draft of this fix omitted it, and all four went from proving the first-parent / second-parent / exact-arity clauses to being decided by the new precondition before they reached them. That is the "test that stays green when the mechanism it claims to guard is sabotaged" failure mode arriving from the other direction, and it is why the predicate is shaped this way. The same state WITH tracked dirt is deliberately not exempt: `git commit` succeeds there and lands an ordinary commit of whatever the operator staged.

**New gitrepo primitive.** `Repo.MergeHeads()` reads `MERGE_HEAD` itself, at the path `git rev-parse --git-path MERGE_HEAD` resolves (correct for a linked worktree), rather than rev-parsing it. That is load-bearing, not stylistic: `MERGE_HEAD` is multi-valued for an octopus and every rev-parsing spelling truncates it to the first entry. Verified directly on git 2.53 — for a two-head `MERGE_HEAD`, `rev-parse --verify --quiet MERGE_HEAD` and `rev-list --no-walk MERGE_HEAD` each print one SHA, and `for-each-ref MERGE_HEAD` / `show-ref MERGE_HEAD` print nothing. A first-entry-only read would accept `git merge --no-commit <recorded source> <decoy>`.

**Pinned lists updated in the same commit:** `MergeHeads` added to `CONSTRAINTS.md`'s gitrepo Client Boundary list and to `cmd/lyx/gitrepoboundary_test.go`'s `gitrepoPinnedRunBoundMethods`; `mergeReasonRecordedMergeGone` added to `mergevocab_test.go`'s `pinnedMergeReasons`.

**Tests added (5).**
- `internal/gitrepo`: `TestMergeHeads_EnumeratesEveryHeadOfAnOctopus` (asserts BOTH the full list and the truncated rev-parse answer for the same state, so rewriting onto rev-parse fails here) and `TestMergeHeads_NoMergeLiveReturnsEmptyNeverNil`.
- `internal/fabricengine`: `TestMergeContinue_DifferentMergeLiveAtConcludeTime_IsNeverCommitted`, `TestMergeContinue_UncommittedOctopusCarryingTheSource_IsNeverCommitted`, `TestMergeContinue_StagedContentWithNoLiveMergeAtConcludeTime_IsNeverCommitted`.

**Sabotage proofs of the new tests** (all in the isolated tree copy):

| Sabotage | Result |
|---|---|
| Remove `recordedMergeGoneReason` from `MergeContinue` entirely | all three fabricengine tests FAIL |
| Rewrite `MergeHeads` onto the truncating `rev-parse --verify --quiet MERGE_HEAD` | `TestMergeHeads_EnumeratesEveryHeadOfAnOctopus` FAILs, and so does `TestMergeContinue_UncommittedOctopusCarryingTheSource_IsNeverCommitted` |
| Neuter the third arm (`return dirty` → `return false`) | `TestMergeContinue_StagedContentWithNoLiveMergeAtConcludeTime_IsNeverCommitted` FAILs |
| Remove the source-already-landed exemption | `TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead` FAILs |

Every clause of the predicate is guarded by a test that fails without it.

**Live re-drive (lab3, fresh hub, redeployed binary).** The exact scenario that produced the silent false success now yields:

```
lyx fabric merge --continue
{"error":"fabricengine: merge preconditions failed: checkout no longer carries the recorded merge","mutations":[],"ok":false,"partial":false}
```

with no commit on either side, the record still on disk, and neither half falsely merged (`is feat merged into main? NO`, `is feat-weft merged into main-weft? NO`). The escape route is confirmed open: `lyx fabric merge --abort` immediately afterwards returned `ok:true` with two `worktree_reset` entries, leaving both checkouts clean and the record gone.

**Stated residual.** A squash record is exempt, because `git merge --squash` writes no `MERGE_HEAD` and there is no evidence to demand — refusing every squash `--continue` would break the ordinary flow. So is a record with an empty recorded source SHA, written by a binary predating them. Both are named in `doc.go` rather than papered over, and both mirror exemptions the adoption arm already carries for the same reason.

**Docs:** `internal/fabricengine/doc.go`'s merge-surface section gained "The commit arm demands evidence too, and for a while it did not", stating the three exits and the squash residual. `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F20 gained the uncommitted half of its adoption matrix — the three uncommitted shapes (different merge live, uncommitted octopus, staged content with no merge) plus the abort-still-works check.

## F2 — the unresolved-conflicts refusal names the paths

**Change.** `MergeContinue` maps the paths still unmerged on each side through the same `unifyConflictPaths` the conflict result uses (`Fabric.unifiedRemainingConflicts`) and returns them on the refusal result; `fabriccli`'s `setMergeExit` reports them under `unresolved`.

**Key choice.** `unresolved`, never `conflicts`. `merge-in`'s own `Long` promises that a conflict RESULT, and only a conflict result, carries a `conflicts` array — that is how a script tells a conflict apart from a hard failure when both exit 1 with `ok:false`. Reusing the key would have broken exactly that test while adding nothing the separate key does not carry.

**Best-effort by design.** A geometry read that fails, or a remaining path that maps nowhere in the visible tree, yields no list rather than a partial one — a list silently omitting a path the operator must still resolve would mislead them, which is the same judgment `unifyConflictPaths`' own unmappable arm makes. Both cases log via `logger.Warn` rather than passing silently, and neither ever replaces the precondition failure with an unrelated error.

**Test added:** `TestRunCLI_MergeContinuePartialStagingListsTheRemainingPaths` — both sides conflicted, warp path staged, weft path left outstanding; asserts the `unresolved` array names exactly the weft path AND that no `conflicts` key is present. Sabotage-proven twice: dropping the engine-side list, and reporting the list under `conflicts` instead, each fail it.

**Help text** updated on both `merge-in` and `merge`. **Doc** updated in the merge-surface section's resolve-flow paragraph.

**Live (lab4/lab5, redeployed):**

```
{"error":"fabricengine: merge preconditions failed: unresolved conflicts remain","mutations":[],"ok":false,"partial":false,"unresolved":["_lyx/note.md"]}
```

**Scope note.** The SPEC records "surfacing merge-in-progress state in `lyx fabric status` output" as an explicit out-of-scope follow-up, and this fix does not touch `status` — it closes only the part inside the merge surface, where the verb had already read the answer and was discarding it.

## F3 — runtime remedies name the shipped verb

`ErrMergeInRequired`, `ErrMergeIncomplete` and `ErrMergeInProgress` now read:

```
run "lyx fabric merge-in" in the source branch's own worktree first, then retry
run "lyx fabric merge --continue" again
run "lyx fabric merge --continue" or "lyx fabric merge --abort" first
```

`ErrMergeInRequired`'s godoc states the rule and why it is not a lapse into CLI vocabulary inside an engine: these strings are read verbatim by an operator or agent out of the fabric envelope and out of `landingshed`'s stuck message, `*ErrForeignMergeState` already names plain git, and `errConflictsWithRecord` already names `lyx fabric merge-stage`. Go callers are served by the godoc, which still names the method.

**Every pinned copy updated in the same commit:** `mergeerrors_test.go` (3 rows), `merge_cli_integration_test.go`'s verbatim text assertion, the two in-code comments quoting them (`commit.go`, `mergesiblings_integration_test.go`), and six `SANDBOX-FABRIC-SUITE.md` quotations. Historical `_mill/` review reports are records of what past rounds observed and are deliberately left alone.

**Live (lab1/lab4, redeployed):** both `merge` over a conflicting source and the sibling refusals (`pull`, `checkout`) now emit the CLI spellings.

## F4 — doc accuracy

`resolveMergeSources`' godoc no longer claims the two source-resolution reasons "stay disjoint". It now states the actual asymmetry: the *weft* arm's `source-not-found` is gated on `weftManaged`, so a resolvable-but-unmanaged source reports one reason; the *warp* arm's is ungated, so a source resolving nowhere reports both — deliberately, because an operator told only "not fabric-managed" for a mistyped name would go looking for a weft counterpart to create instead of at the name they got wrong. Names the two places that pin the dual answer (`TestRunCLI_MergeNonexistentBranchReportsAggregatedGuardError`, `SANDBOX-FABRIC-SUITE` F19) so a later reader does not "fix" the code to match the old sentence. Documentation only, no behaviour change, both pinning tests untouched and still green.

## Gates — final state

- `go build ./...` — OK.
- `go vet ./...` (whole repo) — OK.
- `go test -count=1 ./...` (whole repo, hermetic) — PASS.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok.
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — all ok (37.1s / 2.9s / 1.5s).
- `golangci-lint run` and `--build-tags integration` over the three packages — the merge surface is clean. Remaining hits are all outside this round's scope and pre-existing: `internal/gitrepo/push.go:123` (errcheck), three errcheck hits in `internal/gitrepo/push_test.go`, one `unused` in `internal/fabricengine/livestate_verbs_test.go`. Left alone deliberately — `push.go` is not the merge surface.
- `./deploy-dev` re-run after every source change; live scenarios re-driven on fresh hubs each time.

**Final live re-drive (lab5, fresh hub end to end):** conflicting `merge-in` on both sides → partial stage → refusal naming `_lyx/note.md` → stage the rest → `--continue` concludes with two-parent merges on both sides, `feat` and `feat-weft` both merged, clean status, no `MERGE_HEAD`, no record; `--continue`/`--abort` afterwards both answer `no merge in progress`; a `--squash -m` merge lands a one-parent commit with `committed:true`.

## Teardown

Every scratch hub (`lab1`–`lab5`, the octopus probe repo, the isolated sabotage tree) lives under the session scratch directory, outside the repo. `git status` in the worktree is clean apart from this round's own seven commits; `git status --ignored` shows only pre-existing ignored entries (`.dev-bin/`, `.vscode/`, `.millhouse/`, `.active`, `.portals`, `.wiki`).

## Merge readiness

Ready. Four findings, all fixed, all verified against the real substrate; every new test sabotage-proven; every gate green; no residual left unstated.
