# fabric `weft-is-never-merged` diff — review report (sonnet-high-r2, safety pass)

Round tag: `sonnet-high-r2`.
Scope: the diff landed by merge commit `ab99f531` ("Add a local-only file category to weft" / `weft-local-only-files`), per `_mill/fabric-review-prompt.md`'s file list.
This is round 2, a safety pass following round `opus-high-r1` (0 BLOCKING, 4 MEDIUM, 5 LOW, 4 NIT, all 13 fixed and orchestrator-verified).

Clean-room constraint honored: this report's findings below were formed from the recovered SPEC (`_mill/discussion.md` history — see the Scope section's note on which commit is authoritative), the diff files themselves, `CONSTRAINTS.md`, the module docs, and live substrate driving — **before** reading any `_mill/fabric-review-*` material from round 1. Round 1's material was consulted only afterward, to check for regressions in its closed-and-verified items and to sanity-check the residuals list — per the prompt's clean-room section.

## Executive summary

**No new code defects found.** This safety-pass round's own independent clean-room investigation — full reads of every in-scope file, the recovered SPEC, `CONSTRAINTS.md`, the affected design docs, the full hermetic and integration test suites (including 3× concurrent runs of the fabricengine integration binary), and live driving of every item in the review prompt's high-yield focus list against a real scratch hub built with `lyx fabric clone`/`add` — turned up zero BLOCKING/MEDIUM/LOW/NIT code findings.

One **process-level observation** (not a code defect, not fixed in Job 2 — see below) is worth recording for the crucible campaign's own material: the review prompt's SPEC pointer (`4b30b14e:_mill/discussion.md`) is stale. `_mill/discussion.md` was substantively rewritten by a later commit, `4ccd610d` ("mill-start: discussion-gap-fix round 1 for weft-local-only-files"), which **explicitly rejected** the `MergeOptions.LocalOnlyPaths` per-path mechanism the `4b30b14e` version describes, in favor of the much simpler "weft is never a merge participant" design that was actually shipped. The shipped code matches the **corrected** (`4ccd610d`) SPEC precisely; it is only the review prompt's citation that is one revision behind. This did not mislead this round's findings (I recovered and read the corrected version once the discrepancy became apparent, per the "read the SPEC" requirement, which is clean-room-permitted), but a future round trusting `4b30b14e` alone at face value could wrongly conclude the whole `LocalOnlyPaths` mechanism is a "silently dropped requirement" — it is not; it was deliberately superseded during discussion, before any code was written against it.

## Scope assessment

Using the corrected SPEC (`4ccd610d:_mill/discussion.md`, "Add a local-only file category to weft" post-gap-fix):

- **In-scope items, all delivered:** `Fabric.Merge`/`MergeIn` merge the warp side only, no exported signature change (`MergeOptions` carries only `Squash`/`Message`); the raddle fold-back gate removed from `Topology.Cleanup`; `shedengine.Shed.CommitStatus` seam wired from `persist`; `loomrecipe.ShedPaths.CommitStatus` threaded through; `loomcli` fills the closure with commit-then-push; `fabricengine.PushAnchored` added beside `CommitAnchoredPaths`; `CONSTRAINTS.md`/`manifest/designs/loom.md`/`manifest/designs/shed.md` all updated in the same commit.
- **Explicitly out-of-scope items, correctly absent:** no `MergeOptions.LocalOnlyPaths`, no delete-then-restore, no forced `--no-ff`, no `landingshed.Deps.LocalOnlyPaths`/`DropLocalOnly` (confirmed via `grep -rn "LocalOnlyPaths\|DropLocalOnly"` across the whole worktree — zero hits outside historical git objects); no `internal/gitrepo` changes; no `internal/landingshed.Publish` changes; no teardown step added to a loom producer; no millhouse changes; `status.json` still tracked at `_lyx/loom/status.json`, not moved to `.lyx/`.
- No shipped-beyond-scope surface found: every touched file maps to a decision in the (corrected) discussion.md.

## Code findings

None. Zero BLOCKING / MEDIUM / LOW / NIT findings against the fabric code in this diff's scope.

## Docs & operability findings

None. Cross-checked `doc.go`'s "merge surface" section, `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant addition, `manifest/designs/loom.md`'s rewritten resume/checkpoint paragraphs, and `manifest/designs/shed.md`'s new `CommitStatus` seam paragraph against the actual code and live behavior — all accurate. `internal/fabriccli/fabric.go`'s `cleanup` command help text's flag-matrix summary omits the unmanaged-branch carve-out from its terse one-line parenthetical, but the `Long` text three paragraphs below fully and correctly documents all three carve-outs (checked-out, primary, unmanaged) — considered acceptable terseness, not a defect worth recording.

## Process observation (not a code finding — NOT-FIXED-THIS-ROUND)

**What:** `_mill/fabric-review-prompt.md`'s "Design intent (SPEC)" section points at `git show 4b30b14e:_mill/discussion.md`. That commit is the discussion.md **before** the `discussion-gap-fix round 1` rewrite (`4ccd610d`), which rejected the wiki brief's original `MergeOptions.LocalOnlyPaths` per-path design in favor of "weft is never a merge participant," rewrote the Scope/Decisions/Q&A sections accordingly, and is what the shipped diff actually implements.

**Why it matters:** a reviewer citing only `4b30b14e` would see `MergeOptions.LocalOnlyPaths`, `Deps.LocalOnlyPaths`, `Deps.DropLocalOnly`, forced `--no-ff`, and a third Durable-vs-Ephemeral category described as "in scope," none of which the shipped code has — a scope mismatch that reads as a silently-dropped requirement unless the reader also discovers and reads `4ccd610d`.

**Why NOT-FIXED-THIS-ROUND:** `_mill/fabric-review-prompt.md` is not one of this diff's in-scope files, is not fabric code, and is orchestrator-owned campaign material this round's fixer scope does not cover. Recommend the orchestrator update the prompt template/pointer for any future round of this campaign (or note it in the handoff) to cite `4ccd610d` instead of `4b30b14e`.

## What was tested

Hermetic (all green, run cold from a clean working tree):
```
go build ./...
go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/loomcli/... ./internal/shedengine/... ./internal/landingshed/... ./internal/loomrecipe/... ./cmd/lyx/...
go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/loomcli/... ./internal/shedengine/... ./internal/landingshed/... ./internal/loomrecipe/... ./cmd/lyx/...
```
All packages passed with no failures across 5 repetitions each.

Live integration (real git substrate, `-tags integration`):
```
go test -tags integration ./internal/fabricengine/... -v -count=1   # PASS, 66.05s, no FAIL/panic/race markers
go test -tags integration ./internal/fabriccli/... -v -count=1      # PASS, 6.89s
go test -tags integration ./internal/landingshed/... -v -count=1    # PASS, 0.86s
```
Plus a diagnostic amplifier: the fabricengine integration binary compiled once (`go test -tags integration -c`) and run 3× concurrently — all three exited 0, no FAIL/panic/DATA RACE marker in any log.
Plus a targeted re-run of `TestPushAnchored_*` (all 4 subtests) confirming the `ErrPushRejected` unwrapped-sentinel discrimination (both the positive and negative case) is live-substrate-covered and green.

Live driving (real `lyx fabric` CLI against a scratch hub built via `lyx fabric clone`/`add`, no launcher):
1. **Deploy:** `./deploy-dev` — built and deployed `.dev-bin/lyx` @ `dd4144ed`. Re-deployed not needed since no source changes were made (zero findings to fix).
2. **Scratch hub setup:** `git init --bare` warp/weft remotes (weft's default branch re-pointed to `refs/heads/main` to match warp), `lyx fabric clone <weft> <warp>`, `lyx fabric add task1` — produced a real `task1`/`task1-weft` pair.
3. **Combined weft guard states, without foreign merge (the "ordinary" guard set):** dirtied `task1-weft`'s tracked `_lyx/config/loom.yaml`, detached its HEAD (`git checkout --detach`), then ran `lyx fabric merge-in main` from the warp checkout with a real upstream advance to merge. Result: merge proceeded and committed (`committed: true`) purely on warp state; post-merge inspection of `task1-weft` showed the SAME dirty diff and SAME detached HEAD SHA, byte-for-byte unchanged — confirming the weft is genuinely untouched by a warp-only merge even under combined dirty+detached weft state. CONFIRMED, matches doc.go's claim ("Drive all four weft states at once and the merge still decides purely on warp state" — I combined the two "ordinary" ones; see next bullet for why the foreign-state combination is a different, still-active guard).
4. **Foreign weft merge state combined with the same dirty+detached weft:** built a real rename/delete conflict on `task1-weft` (git-level `MERGE_HEAD` live, conflicted index) while `task1-weft` was still dirty and detached, then ran `lyx fabric merge-in main`. Result: refused immediately with `ErrForeignMergeState` — this is the retained, intentional weft-reading guard (`foreignMergeStatePresent`), not a regression of the "narrowed guard ignores ordinary weft state" property; the two are documented as deliberately different (doc.go: "the weft lost its power to block a merge on ORDINARY state... not on foreign in-flight git state"). CONFIRMED consistent with design.
5. **Weft conflict list falsification, rename/delete shape (beyond the packaged `mergeweftlocal_integration_test.go` content-rewrite scenarios):** started a real fabric merge attempt with a genuine warp-side conflict (`mainfile.txt`, leaving the merge record open), then independently built a real weft-side rename/delete conflict (`git mv` on one branch, `git rm` on another, merged into the checked-out weft branch) via plain git in the weft worktree, live `MERGE_HEAD` present. Ran `lyx fabric merge --continue`: the response's `unresolved` list correctly named BOTH the warp conflict (`mainfile.txt`) and the weft conflict (`_lyx/fabric/origin-renamed.json`), refusing with `mergeReasonUnresolvedConflicts`. CONFIRMED — doc.go's "the weft conflict list is not permanently empty" claim holds under a rename/delete shape, not just content rewrites.
6. **MergeAbort restoring warp-only, weft-side partial state:** with the fabric merge record still open (warp conflicted, weft independently foreign-conflicted from the prior step), ran `lyx fabric merge --abort`. Result: warp reset cleanly to its pre-merge SHA (`worktree_reset` mutation, `git status --porcelain` empty afterward); the weft's foreign conflict (`UD _lyx/fabric/origin-renamed.json`, live `MERGE_HEAD`) was left COMPLETELY UNTOUCHED — identical before/after. CONFIRMED: no weft-side write happens during a merge attempt at all (only `f.weft.CurrentSHA()` reads), so there is no partial weft state for an abort to leave behind or need to clean up — consistent with `resetMergeSides`' own doc comment ("abort-does-not-reset-weft ... the weft was never a merge participant, so an abort has nothing to restore there").
7. **`Cleanup` carve-outs, live against a real scratch hub, not just the removed pinning unit tests:** removed `task1`'s warp worktree (`git worktree remove --force`) to orphan `task1-weft`, added an unmanaged (non-`-weft`-suffixed) branch (`legacy-unmanaged-branch`) and two other non-suffixed test branches, then ran `lyx fabric cleanup` (dry run) and `lyx fabric cleanup --apply` (no `--force`). Dry run correctly reported `task1-weft` as the sole non-protected orphan; `--apply` deleted exactly `task1-weft` and nothing else. `main-weft` (primary) stayed protected throughout; `main` (board's own weft:main, checked out at `_board`) stayed protected as a checked-out branch; every non-`-weft`-suffixed branch stayed protected as unmanaged. Re-ran with `task1-weft` still checked out at its own weft worktree (before removing that worktree) and confirmed it reports `protected: true` there too (checked-out carve-out, distinct from the orphan-but-unremoved-worktree case). CONFIRMED, all three carve-outs plus the orphan-is-deletable-without---force path hold live.
8. **Teardown/hygiene check of the live-driving scratch state:** `find` over the scratch hub for `*.lock`/`fabric-merge.json` found only ordinary flock artifacts (`board.lock`, `.gitrepo-push.lock`, `weft.write.lock`, `exclude.lyx.lock`) left un-deleted by `lock.FileLock.Release`'s own documented unlock-without-delete behavior (doc.go names this explicitly for `.gitrepo-push.lock`) — no stray `fabric-merge.json` merge-state record survived (confirms `deleteMergeState` ran cleanly after the `MergeAbort` in step 6). No stray hub or worktree outside the scratchpad directory.

## Deferred items from round 1 — re-evaluated

Per the prompt's residuals list, re-evaluated and NOT re-flagged (no new reason found to reopen either):
1. F2's remaining `git add` staging into a foreign merge's index on a lost race (`commitStatusFailureDisposition`'s own doc comment already states this plainly) — still bounded, closing it needs a lock the operator doesn't take.
2. `sideRecordedMergeGone`'s squash exemption — still pre-existing, untouched by `ab99f531`, out of this campaign's scope.

Round 1's closed-and-verified items (F1–F13) were spot-checked live where the high-yield focus list called for it (items 3, 5, 6, 7 above overlap with F9/F7's territory) and found unregressed.

## Merge bar

Correctness in the normal single-instance flow is the gate, per the prompt's stated merge bar. All normal-flow scenarios, plus the adversarial combined-state and rename/delete scenarios beyond the packaged tests, passed live. The 3× concurrent integration-suite run is the diagnostic amplifier the prompt allows for this cheap-substrate module; it found nothing.

## Merge-readiness verdict

**READY TO MERGE.** No new defects, no regressions of round 1's fixes, docs accurate, full test suite green (hermetic + integration + concurrent), and every high-yield adversarial scenario in the round 2 prompt — including two combinations (rename/delete conflict, and dirty+detached+foreign-merge together) not exercised by round 1's own live driving as far as this round's clean-room investigation could tell — reproduced the documented, intended behavior with no deviation.
