# `fabric merge` — independent review, round `opus-medium-r2`

Round 2. Assignment is the three residuals the orchestrator's independent verification of round 1 left standing, plus whatever adversarial work in those regions turns up.
The CLOSED-AND-VERIFIED list from round 1 is not re-litigated.

Status: **IN PROGRESS** — this file is appended to and committed as each scenario or finding lands.

## Executive summary

(written last)

## Findings

### R1 — `MergeStart` misclassifies an empty-result merge as `AlreadyUpToDate`, leaving a live `MERGE_HEAD` fabric then abandons — BLOCKING, CONFIRMED (live, deployed binary)

`internal/gitrepo/merge.go:99-106` (the four-way classification switch).

**Mechanism.**
`MergeStart`'s success-path classification reads exactly two signals: whether `git diff --cached --quiet` reports a staged difference, and whether HEAD moved.
It never asks whether git left a live `MERGE_HEAD`.
A **real, non-fast-forward merge whose result tree happens to equal HEAD's tree** — the everyday shape produced when the same change reached both branches independently (a cherry-pick, a backport, a duplicated hand-edit, a rebase-and-also-merge) — stages nothing and moves no HEAD, yet git has genuinely started a merge and written `MERGE_HEAD`.
That case is classified `MergeAlreadyUpToDate`.

Proven at the raw-git layer first (`exp1.sh`), on a fresh repo where `side` is *not* an ancestor of HEAD but both carry identical content:

```
--- is side an ancestor of HEAD? ---   NO
--- git merge --ff --no-commit side ---
Automatic merge went well; stopped before committing as requested
exit=0
--- diff --cached --quiet ---          staged_exit=0     <- nothing staged
--- MERGE_HEAD? ---                    a6aea47a...       <- but a merge IS in progress
head_moved=no
```

So `staged=false`, `headAfter == headBefore` → `MergeAlreadyUpToDate`, with `MERGE_HEAD` live.

**Failure scenario, end to end, through the deployed binary** (`scen-empty-result.sh`, hub `hub1`, `lyx` built from `aa7fcd49`).
Warp branch `feat` and warp `main` each add `a.txt`=`X` independently; weft branch `feat-weft` and weft `master-weft` each add `b.txt`=`Y` independently. Neither source is an ancestor of its side's HEAD.

```
$ lyx fabric merge-in feat
{"already_up_to_date":true,"committed":false,"mutations":[],"ok":true,"partial":false}
```

fabric reports unqualified success and a clean no-op. What it actually left behind:

```
=== after: fabric record ===   record absent
=== after: git MERGE_HEAD ===
warp MERGE_HEAD: dbf890fc76851fc301b70ec2dfe5ebb1c370007b
weft MERGE_HEAD: 1c24ed01a54e16fe6f47ae837f03fb5bb24174f7
--- warp ---  All conflicts fixed but you are still merging.
--- weft ---  All conflicts fixed but you are still merging.
```

Four distinct harms, all reproduced in that one run:

1. **The record and git disagree** — the primary invariant of this surface. `MergeInProgress()` is false (the record was deleted on the happy path) while `git status` in *both* checkouts says a merge is in progress. `MergeIn`/`Merge` skip the conclude for an `up_to_date` side by design (`mergelifecycle.go:36`, `:53`), so nothing ever clears the state fabric itself created.
2. **The merge is silently lost.** `feat` is never recorded as merged into `main` in either history, and `RecordCorrespondence` pins the *pre-merge* SHAs as the pair's post-merge correspondence. The merge is concludable — `git commit --no-edit` on that state yields a proper two-parent merge commit (`exp2.sh` section B) — fabric just declines to.
3. **The pair is bricked for every fabric merge verb, including fabric's own recovery verb.** With no record and foreign state present, `mergeStateOrForeignErr` returns `*ErrForeignMergeState`:
   ```
   --- lyx fabric merge --abort ---    {"error":"fabricengine: git merge state exists that fabric did not start; ...","ok":false}
   --- lyx fabric merge --continue --- {"error":"fabricengine: git merge state exists that fabric did not start; ...","ok":false}
   --- lyx fabric merge-in feat ---    {"error":"fabricengine: git merge state exists that fabric did not start; ...","ok":false}
   ```
   Only plain `git merge --abort` in both checkouts recovers. fabric told the operator the state is "state fabric did not start" — it did.
4. **Blast radius outside the merge surface.** A plain `git commit -m "…"` in such a checkout produces a **merge commit** carrying whatever unrelated change the operator staged (`exp2.sh` section C: `parents: 3` — two parents — on a commit titled "unrelated sibling commit"). `Commit` itself is saved here only because it happens to also consult `foreignMergeStatePresent` (`commit.go:125`); `Pull`, `Checkout` and `Remove` consult the record only (`pull.go:221`, `checkout.go:48`, `remove.go:65`) and would proceed.

**Suggested fix.** In `MergeStart`'s success path, probe `MergeHeadPresent()` and classify a live `MERGE_HEAD` as `MergeStaged` before the HEAD-moved test — a merge git has started and can conclude is staged work, whatever the index diff says. The squash form writes no `MERGE_HEAD`, so the probe is a no-op there and squash's genuinely-nothing-to-do case keeps reporting `AlreadyUpToDate`. Fast-forward leaves no `MERGE_HEAD` either, so the `MergeFastForwarded` arm is untouched.

### R2 — `MergeAbort` discards a conclude-commit the record itself says already landed — BLOCKING, CONFIRMED (live, deployed binary)

`internal/fabricengine/mergelifecycle.go:204` (`MergeAbort` → `resetMergeSides(rec, st.WarpStart, st.WeftStart)`, unconditional).

This is the single most dangerous member of residual 2's enumerated class (see the table below): the sites where the record survives an error return *after* a conclude-commit has landed on one side.

**Mechanism.** `concludeMergeSides` lands warp first, then weft, writing each side's SHA into the record as it goes. If the weft conclude fails, it returns `*ErrMergeIncomplete` and **deliberately retains the record** — with `warp_committed` populated. `MergeAbort` then resets *both* sides to `WarpStart`/`WeftStart` unconditionally. It reads the record it is restoring from, so it has `WarpCommitted` in hand, and ignores it.

**Failure scenario** (`scen-abort-after-conclude.sh`, hub `hub2`). A refusing `pre-commit` hook in the weft checkout makes the weft conclude fail — the same shape as `commit.gpgsign=true` with no key, a full disk, or any hook-enforced policy.

```
$ lyx fabric merge-in feat
{"error":"fabricengine: merge conclude did not finish; run MergeContinue again",
 "mutations":[{"kind":"merge_staged","target":"wtest",...},
              {"kind":"merge_staged","target":"wtest-weft",...},
              {"kind":"merge_committed","target":"wtest","detail":"4eef1870..."}],
 "ok":false,"partial":true}

record: { ..., "warp_start":"42401f04...", "warp_outcome":"staged",
          "warp_committed":"4eef18708b9e53b0d047a9b58c24a46079fd0d39", "weft_committed":"" }
warp HEAD now = 4eef1870...  (moved: YES)   warp parents: 2
```

The operator, seeing a failed merge, does the obvious thing:

```
$ lyx fabric merge --abort
{"already_up_to_date":false,"committed":false,
 "mutations":[{"kind":"worktree_reset","target":"wtest","detail":"42401f04..."},
              {"kind":"worktree_reset","target":"wtest-weft","detail":"51c40e7b..."}],
 "ok":true,"partial":false}
warp HEAD after abort = 42401f04...
>>> WARP CONCLUDE COMMIT DISCARDED BY --abort
record after abort: deleted
```

`ok: true`. No warning. The conclude commit is gone and the record with it, so nothing remains to reconstruct from.
In the `merge-in`-with-conflicts flow that commit is not machine-generated content — it carries the operator's **manual conflict resolutions**, and `resetHardTo` runs with `force: true`, so there is no dirty-worktree brake either.

The prompt states the property directly: *"`MergeAbort` must never discard work that was actually committed."* It does.

**Suggested fix.** `MergeAbort` refuses when `st.landedConcludeCommit()` is true, via a new member of the closed guard-reason set, leaving `MergeContinue` — which already skips a side whose committed SHA is recorded, so it is idempotent here — as the correct recovery.
This is the exact mirror of round 1's F1 (`MergeContinue` refuses an attempt whose record shows an empty outcome, leaving `--abort` as the recovery); the two refusals together make each half-finished shape recoverable by exactly one verb, and neither verb destructive.
The closed set's same-commit rule means `mergevocab_test.go` and `mergeerrors_test.go` move with it. `doc.go` must state the escape hatch: if the underlying git failure is not fixable, plain git is the last resort.

### R3 — `mergeReasonNoMergeInProgress` is a dangling member of the closed guard-reason set — NIT, CONFIRMED (traced)

`internal/fabricengine/mergeerrors.go:24`.

```
$ grep -rn "mergeReasonNoMergeInProgress" --include=*.go .
internal/fabricengine/mergeerrors.go:24        <- declaration
internal/fabricengine/mergeerrors_test.go:117  <- pins the set's contents
internal/fabricengine/mergevocab_test.go:98    <- pins the set's contents
internal/fabricengine/mergevocab_test.go:142   <- pins the set's contents
```

No production code path produces it. The no-merge case is the typed `*ErrNoMergeInProgress` (`mergelifecycle.go:84`), and that is the right shape for it: the closed set exists for **aggregated precondition reasons** that several guards contribute to one `*MergeGuardError`, whereas "no merge in progress" is a terminal, standalone disposition with no other reason to aggregate alongside. Its `Error()` string is already the same words.

**Suggested fix.** Delete the constant from the set and from the three test pin-lists in the same commit. Producing it somewhere would mean converting `MergeContinue`/`MergeAbort`'s typed error into a guard error, which is a strictly worse API for the CLI's envelope and contradicts `mergeerrors.go`'s own doc comment.

### R4 — `Pull`/`Checkout`/`Remove` do not consult `foreignMergeStatePresent`, while `Commit` does — LOW, CONFIRMED (traced + live)

`internal/fabricengine/pull.go:221`, `checkout.go:48`, `remove.go:65` versus `commit.go:123-130`.

`Commit` refuses on *either* a fabric record or foreign git merge state. The other three refuse on the record alone.
Reproduced as part of R1's live run: with both checkouts holding a live `MERGE_HEAD` and no record, `lyx fabric commit` correctly refused (`"a merge is in progress"`), while the record-only guards would have proceeded.
`doc.go:922` describes the four as a uniform set, which is not what the code does.

The harmful direction is `Checkout`: `git checkout` with a clean index and a live `MERGE_HEAD` succeeds and silently drops `MERGE_HEAD`, abandoning the merge.

**Suggested fix.** This is the smaller, honest half of the fix: make `doc.go` state the asymmetry accurately, and note which verb consults which probe.
Broadening `Pull`/`Checkout`/`Remove` to the foreign probe is a behaviour change reaching outside the merge primitive into three sibling verbs the spec deliberately scoped as record-only, and it would make a hub carrying any human-left plain-git merge state un-checkout-able. Recorded as **doc-only this round**, with the behaviour question named explicitly.

## Enumerated class — post-record error returns (residual 2)

(appended below)

## What was tested

(appended as each command/scenario returns)
