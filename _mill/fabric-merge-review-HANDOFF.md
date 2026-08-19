# fabric merge — crucible campaign HANDOFF (orchestrator-only)

Off-limits to round agents: this file matches the `fabric-merge-review-*` pattern the round prompt declares unreadable.

**Last refreshed:** during round 2's Job 1, at commit `7e251055`. Written to disk, NOT committed — a round is live, so Hard Rule 3 keeps the orchestrator off `git add`/`git commit`. Commit this refresh once r2 finishes.

## What this campaign is

Mill task `fabric-merge-crucible-hardening` (wiki id 85), worktree `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-hardening`, branch of the same name.
Crucible (`crucible/README.md`) run by hand against **only** the merge primitive shipped by `a2bf44e2` — `MergeIn` / `Merge` / `MergeContinue` / `MergeAbort` / `MergeInProgress` on `internal/fabricengine`, the `internal/gitrepo/merge.go` layer under it, and the `lyx fabric merge-in` / `lyx fabric merge` CLI surface.
Explicitly **not** the rest of fabricengine — the `crucible: follow-ups` slices 12–15 already hardened that.

Orchestrator-driven, not mill-go. The task's mill phase stays `discussing`; nothing in this campaign advances it.

## Operator's round plan (given up front, 2026-08-19)

Up to four rounds in the first instalment, model + effort fixed in advance:

| Round | Model | Effort | Tag | Status |
|---:|---|---|---|---|
| r1 | Opus | medium | `opus-medium-r1` | **done, verified** — 9 findings, 9 fixed; verification left 3 residuals |
| r2 | Opus | medium | `opus-medium-r2` | **running** — Job 1 in progress, 4 findings recorded so far |
| r3 | Fable | medium | `fable-medium-r3` | not started |
| r4 | Opus | high | `opus-high-r4` | not started |

Hard Rule 2 (explicit effort pick required before every spawn) is satisfied for all four by that instruction.
Do not deviate from this list without the operator saying so.

## Operator norms observed so far

- **The operator pauses and resumes rounds on purpose.** Round 1 was stopped mid-Job-2 and restarted by hand, with an explicit "don't you worry about it". Hard Rule 6 applies: a `killed`/`stopped by user` notification is never a crash, never something to recover from, never something to report back as a problem. Note the state and keep waiting. The same round may notify several more times before it finishes for real.
- **Keep this handoff current at all times, not just after a verification.** The operator called this out directly. Refreshing the *file on disk* mid-round is required; only the `git add`/`git commit` of it waits for a clean tree. A refresh that only exists in orchestrator working context is exactly what a compaction destroys — which is the failure this file exists to prevent.

## Campaign-specific facts worth not rediscovering

- **fabric's live tier is `-tags integration`, not `-tags smoke`.** There are zero `//go:build smoke` files under `internal/fabricengine`, `internal/fabriccli`, `internal/gitrepo`. Any round claiming a "smoke" run for fabric has run nothing — reject that claim.
- **Not LLM-driving.** The substrate is real git repos. The generic protocol's N-concurrent amplifier is safe here and there is no EXECUTION BAN list. The real cost is wall-clock (~30 s per full `-tags integration` fabricengine run), not RAM.
- **No `manifest/designs/fabric.md`.** fabric's module doc is `internal/fabricengine/doc.go`; its "# The merge surface" section (~line 846) is the authoritative prose contract. Documentation Lifecycle updates land there.
- **SPEC lives only in git history.** `git show 3b800bc8:_mill/discussion.md` and `3b800bc8:_mill/plan/0*.md` (six batches); rejected alternatives at `967916ea:_mill/discussion-meta.md`. The working tree's `_mill/` was cleaned on merge.
- **`./deploy-dev`** is the POSIX deploy script (not `.cmd`) on this host.

## Baseline the orchestrator established before round 1 (all green, committed tree at `9115020a`)

- `go build ./...`
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`
- `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...`
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — fabricengine 25.2 s, fabriccli 2.1 s, gitrepo 1.3 s

Green baseline is the starting condition, not evidence. The whole point of this campaign is that these gates pass and the surface is still not trusted under crash and concurrency.

## Pre-count

`_mill/fabric-merge-review-PRECOUNT.md`, written before round 1 was spawned, never shown to a round.
Six counted classes, each with its blind spots named. Expect to be corrected by a round; that is the round working.
**Not yet checked against r1's numbers** — do that during verification, and read r1's enumeration table (report line ~297) against pre-count classes 3 and 6 in particular.

## Round 1 (`opus-medium-r1`) — COMPLETE, and independently verified

Seed commit `74ef8089`. Round commits `125757f4` → `a6a88502`.
It survived one deliberate operator pause/restart and one genuine watchdog stall (recovered by resuming the same agent from its transcript — see the stall note above).

Job 1: twelve live scenarios (L1–L12) on ~20 purpose-built scratch hubs, the mutating-entry-point enumeration, and the plan-vs-shipped scope pass. Report committed incrementally, before the first production edit — the Sequencing rule held.
Job 2: nine findings, nine fixed, nothing deferred. It also repaired four pre-existing integration tests that were pinning F3's defect as correct.

Self-verdict was READY TO MERGE. That verdict is not the gate; what follows is.

### What the orchestrator verified itself

**Gates, from cold on the committed tree — all green, tags named:**
`go build ./...`; `go vet` on the three packages; `go test -count=5` hermetic across `fabricengine`/`fabriccli`/`gitrepo`/`cmd/lyx`; `go test -tags integration -count=1` across the three packages (fabricengine 44.0 s, fabriccli 4.3 s, gitrepo 2.3 s).

**Sabotage proofs — every new test watched to fail at its intended assertion, then the code restored to an empty diff.** Eight new tests, seven mechanisms:

| Mechanism sabotaged | Test | Result |
|---|---|---|
| F1 `mergeAttemptIncompleteReason` → never fires | `TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides` | failed correctly |
| F2 `detachedHeadReason` → returns nil | `TestMergeCrucible_DetachedHeadRefused` (both subtests) | failed correctly |
| F3 `landedConcludeCommit` → `true` | `TestMergeCrucible_ResultFlagsDescribeWhatHappened` | failed correctly |
| F3 `bothSidesAlreadyUpToDate` → `false` | — | **STAYED GREEN — see residual 1** |
| F4 `--ff` pin removed | `TestMergeStart_HostileMergeFFConfig` (both subtests) | failed correctly |
| F5 `mergeSourceInFlight` → `false` | `TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming` | failed correctly |
| F7 `Conflicts: mergeNoConflicts` dropped | `TestMergeCrucible_ConflictsIsEmptyNeverNil` | failed correctly |
| F8 `-m`+`--abort` rejection disabled | `TestRunCLI_MergeRejectsFlagsItWouldOtherwiseIgnore` | failed correctly |
| F8 rejection broadened to `--continue` | `TestRunCLI_MergeContinueAcceptsMessage` | failed correctly |

The first sabotage attempt on F2 (`if warpDetached || weftDetached` → `if false`) broke the build on unused variables and was inconclusive; it was redone by neutering the return instead. A build break is not a proof — redo the sabotage rather than counting it.

**Both BLOCKING fixes re-driven live**, on a purpose-built hub, against the freshly deployed binary:
- F2: warp detached, weft detached, and `merge --squash` as the strongest mode — all three refused with `checkout is not on a branch`, empty mutation record, no record written, neither branch moved.
- F1: the crash window reconstructed by hand (`fabric-merge.json` with `warp_outcome:"staged"`, `weft_outcome:""`). `--continue` refused with `merge attempt did not reach both sides`, zero mutations, nothing landed; `--abort` then restored both sides to the recorded pre-merge SHAs, deleted the record, and left a clean worktree. The reconstruction turned out stricter than intended — git had actually fast-forwarded where the record claimed `staged`, and the guard still fired on the empty weft outcome.

**Pre-count reconciliation:** r1's enumeration reported 27 mutating entry points, 4 guarded, 1 genuine gap. Pre-count class 6 had the spec's 4 guarded verbs and predicted that a round enumerating every entry point would exceed 4 — it did, and F5 is that excess. The round moved in the correct direction and the orchestrator's number needed no correction. Classes 1, 2, 4 and 5 were not contested.

### RESIDUAL — three things verification left standing, now seeded for r2

1. **`bothSidesAlreadyUpToDate()` has zero test coverage.** Hardwired to `return false`, the entire suite — hermetic and `-tags integration`, both packages — stays green. And the subtest that looks like it covers the flag (`…/SecondCallReportsAlreadyUpToDateNotCommitted`) is caught by the **pre-lock** probe at `merge.go:122`, a different return site with a hardcoded `true`. Textbook refinement 5: a green run that never entered the code path. The fix itself reads correct; it is the proof that is missing.
2. **The post-record error-return class was never enumerated.** r1 brushed against it repeatedly (F1 lives in it) but never counted it. Pre-count class 3 has the orchestrator's own hand-read: 27 sites, of which 12 leave the record live *after* a conclude-commit already landed — where `MergeAbort` restores from pre-merge SHAs and would discard committed work. That class is countable and unclosed, which is exactly what refinement 2 says to switch a round to.
3. **`mergeReasonNoMergeInProgress` is dangling** — declared in the closed reason set at `mergeerrors.go:24`, referenced only by the vocabulary tests that pin the set, produced by no code path. This was noted in pre-count class 4 and deliberately withheld; r1 did not find it.

### DEFERRED — carried into r2's prompt for re-evaluation

- The unlocked guard window between `mergeRecordExists()` and `saveMergeState`. r1 reasoned about it, could not reproduce it, and correctly declined to record it.
- `CheckoutDetached`/`RestoreBranch` abandoning a merge already in progress. r1 logged it as webster's problem on the argument that F2 closes the harmful direction. r2 is asked to re-examine that argument, since F2 stops a merge *starting* while detached but not a detach *during* a merge.

### Honest limits of this verification

- The two verification hubs under the session scratchpad could not be deleted — `rm -rf` is refused by this session's sandbox. They are outside the repo, in an ephemeral session directory, and `git status` is clean. Not cleaned, and said so rather than worked around.
- One stray `lyx reed header --blocking` process is running, and it belongs to the **`reed-shuttle-crucible-hardening`** worktree, not this one. Left alone under worktree isolation. Do not reap it.
- Windows path behaviour in `weftPathVisible`/`unifyConflictPaths` remains unexecuted, by both the round and the orchestrator. Out of scope, Linux host, reasoned about rather than driven — the same named gap the previous fabric campaign carried to the end.
- The N-way concurrent amplifier was not run. The merge bar is single-instance correctness and the surface is not tmux-shaped; this is a deliberate omission, not an oversight.

## Round 2 (`opus-medium-r2`) — Job 1 COMPLETE and committed; Job 2 now live

Re-seed commit `a5700c41`. Round commits: `aa7fcd49` (opened report) -> `7e251055` (findings R1-R4) -> `75b8b6b8` (review COMPLETE: residual-2 enumeration, residual-1 route, race non-repro, R4 withdrawn, R5 added).
Report: `_mill/fabric-merge-review-opus-medium-r2.md`, 381 lines. Sequencing rule held — the whole review was committed before the first production edit.
As of now `internal/gitrepo/merge.go` is modified-uncommitted: that is Job 2 starting (R1's classifier probe). Orchestrator stays off the tree.

**Self-verdict: NOT READY as reviewed** — two BLOCKING data-integrity defects, both reproduced live through the deployed binary. That verdict is the round's own and is not the gate.

### R2's findings, as claimed (none independently verified yet)

- **R1 — BLOCKING.** `gitrepo.MergeStart` (`internal/gitrepo/merge.go:99-106`) classifies on two signals only: staged-diff and HEAD-moved. A real non-fast-forward merge whose result tree equals HEAD's tree stages nothing and moves no HEAD, yet git writes `MERGE_HEAD`. That is classified `AlreadyUpToDate`. fabric reports `ok:true, already_up_to_date:true`, deletes its record, and abandons a live `MERGE_HEAD` in **both** checkouts. Consequences reproduced in one run on `hub1`: record and git disagree; the merge is silently lost though `git commit --no-edit` would land it properly; every fabric merge verb — `--abort` included — then returns `ErrForeignMergeState`, telling the operator this is state fabric did not start, which it did; and a plain `git commit` in that checkout silently produces a two-parent merge commit carrying an unrelated staged change. Proposed fix: probe `MergeHeadPresent()` and classify a live `MERGE_HEAD` as `MergeStaged`, ahead of the HEAD-moved test. Squash writes no `MERGE_HEAD`, so the squash arm is untouched.
- **R2 — BLOCKING.** `MergeAbort` (`mergelifecycle.go:204`) resets both sides to the recorded pre-merge SHAs unconditionally, discarding a conclude-commit the record it is reading says already landed — with `force: true`, `ok:true`, no warning, and the record deleted afterwards so nothing remains to reconstruct from. In the `merge-in`-with-conflicts flow that commit carries the operator's manual conflict resolutions. Reproduced on `hub2` with a refusing weft `pre-commit` hook (same shape as `commit.gpgsign` with no key, or a full disk). Proposed fix: a new closed-set guard reason refusing the abort, leaving `MergeContinue` (already idempotent on a recorded side) as the recovery — the exact mirror of r1's F1.
- **R3 — NIT.** `mergeReasonNoMergeInProgress` is dangling: declared at `mergeerrors.go:24`, referenced only by the three test pin-lists, produced by nothing. This is the item the pre-count named and deliberately withheld; r2 found it unaided. Its argument for deleting rather than producing it is sound — the closed set is for aggregated precondition reasons, and no-merge-in-progress is a terminal standalone disposition.
- **R4 — WITHDRAWN on evidence** by the round itself, after re-reading `doc.go:920-923` and finding the doc already states the `Commit` asymmetry it had claimed was undocumented. Recorded rather than deleted, so the trail shows the correction.
- **R5 — LOW.** `doc.go:938-940` justifies the `CheckoutDetached`/`RestoreBranch` exemption by appealing to F2's attached-HEAD precondition. That is r1's deferred item, and r2 says the conclusion holds but the reason is wrong: F2 closes starting a merge while detached, not detaching during one. What actually closes the long conflicted window is git itself refusing `checkout --detach` with unmerged index entries. The narrow resolved-but-not-concluded window stays open and belongs to webster. Doc-only fix.

### Residual 2 — the enumeration r1 never did

Reproducible method (`enumerate.sh` resolves each function's extent from source rather than hardcoding lines). Counts: 94 raw error-returning statements across the surface, 45 in the post-record region, **41 in class**, of which **17 can have a landed conclude-commit** — 13 visible in the record (`*_committed` set), **4 invisible** (`CurrentSHA`/`saveMergeState` failing immediately after a successful `git commit`).

Against pre-count class 3 (orchestrator's hand-read: 27 sites, 21 excluding `deleteMergeState`, 12 post-conclude): **r2 corrects the orchestrator upward on both numbers, 41 vs 27 and 17 vs 12.** A round correcting the orchestrator is the round working, and the pre-count's own blind spots were written down next to it. The correction still needs checking — specifically whether r2's 41 counts helper-internal sites in a way the hand-read did not (r2 says it attributes `selfAbortMergeAttempt` x4, `concludeMergeSides` x3, `resetMergeSides` x4 call sites to the helper's internal sites once each).

The 4 invisible sites are why r2's R2 fix cannot key on `landedConcludeCommit()` alone. Its field-free predicate: a conclude may have landed on a side iff its recorded committed SHA is non-empty, **or** its recorded outcome is `staged`/`conflicted` and its HEAD is no longer at its recorded start SHA. Verify that reasoning independently — it is the load-bearing claim of the whole fix.

### Residual 1 — the proof gap, and r2's route to it

r2 reproduces the diagnosis exactly (pre-lock probe at `merge.go:132`/`:340` short-circuits every ordinary second call from a different return site with a hardcoded `true`), then finds a **race-free, seam-free, deterministic** route to the derived field: `merge --squash` of a source that is *not* an ancestor of HEAD (so the pre-lock probe cannot early-return) whose squash result tree equals HEAD's tree on both sides (so both post-lock outcomes are `up_to_date`). Confirmed live on `hub3`: `already_up_to_date:true` with neither source an ancestor is only producible by the derived field. It survives R1's fix because squash writes no `MERGE_HEAD`.

That is a better answer than the concurrency seam r1's residual assumed would be needed. It is also the thing to sabotage-prove hardest.

### r1's other deferred item — the unlocked guard window

Attempted and **not reproduced**: 75 attempts across three arms (25 interleaved on `hubA`, 25 strictly-sequential control on `hubA`, 25 interleaved on independent `hubB`), 0/75. Notably r2 discarded two earlier detector versions because they fired on the sequential control, and only trusted the third after the control read 0 while the detector still had a live path to a positive. That is the right shape for a negative result — a clean run from a probe never shown able to detect is worth nothing.

## Next action

**While Job 2 runs:** stay off the tree, no `git add`/`git commit`, keep refreshing this file on disk. Do not read the round's raw transcript.

**When r2 finishes,** verification protocol — the protocol, not the round, is what caught r1's residual:

1. Commit this handoff refresh once the tree is the orchestrator's again.
2. Read r2's fixer report and its "What was tested" in full before characterising anything.
3. Gates from cold: `go build ./...`; `go vet` on the three packages; `go test -count=5` hermetic across `fabricengine`/`fabriccli`/`gitrepo`/`cmd/lyx`; `go test -tags integration -count=1 -timeout 40m` across the three. Name the tier in your own record; never accept a green claim that does not name its tag.
4. **Sabotage-prove every new test.** The three that matter most:
   - hardwire `bothSidesAlreadyUpToDate` to `return false` — the new squash-route test **must** fail. If it stays green, r2 reproduced r1's mistake and residual 1 is still open. This is the single decisive check of the round.
   - revert R1's `MergeHeadPresent()` probe in the classifier — the empty-result-merge test must fail at the intended assertion.
   - revert R2's abort guard — the abort-after-conclude test must fail by observing the discarded commit, not merely by an error-string mismatch.
   A build break is not a proof; redo the sabotage a different way (r1's F2 needed this).
5. Re-drive both BLOCKING fixes live on a freshly built hub against a freshly deployed binary. Hub recipe: bare warp + bare weft, exporting `GIT_CONFIG_GLOBAL` with `init.defaultBranch = main` **before** the first `git init` (otherwise the weft defaults to `master` and `lyx fabric add` fails on an invalid `main-weft` reference); seed and push warp `main`; `lyx fabric clone <weft-bare> <warp-bare>`; `lyx fabric add task1`. Use `env -C <dir> <cmd>` — this sandbox refuses `cd <dir> && ...`, and `rm -rf` too, so scratch hubs cannot be cleaned and that must be stated rather than worked around.
   For R1 specifically, re-drive the *bricking* claim, not just the misclassification: confirm that after the fix `--abort` and `--continue` no longer return `ErrForeignMergeState` on that pair.
6. Re-check r2's 41/17 against the pre-count's 27/12 by hand on the diff, deciding whether the delta is method or substance.
7. Re-seed from whatever verification leaves standing — derived from the residue, never "review it again" — and spawn r3 as **Fable / medium**, tag `fable-medium-r3`.

**Convergence bar:** a safety pass that finds nothing, the orchestrator's own gates, and the sabotage proofs all agreeing, across rotated models. State the limits in the verdict rather than claiming more: Windows path behaviour (never executed by any round or the orchestrator), the N-way concurrent amplifier (deliberately not run — the merge bar is single-instance correctness), and any class whose zero came from a method never shown able to detect.
