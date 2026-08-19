# `fabric merge` — independent review, round `opus-medium-r2`

Round 2. Assignment is the three residuals the orchestrator's independent verification of round 1 left standing, plus whatever adversarial work in those regions turns up.
The CLOSED-AND-VERIFIED list from round 1 is not re-litigated; nothing I drove contradicts any of it.

## Executive summary

Two BLOCKING defects, both CONFIRMED live against real git through the deployed binary, both found by driving the regions residuals 1 and 2 point at.

- **R1** — `gitrepo.MergeStart` misclassifies a real merge whose result tree equals HEAD's tree as `AlreadyUpToDate`. fabric then reports unqualified success, deletes its own record, and walks away leaving a live `MERGE_HEAD` in *both* checkouts. The merge is silently lost, the record and git disagree, and the pair is bricked for every fabric merge verb — including `merge --abort`, which now reports the state as one "fabric did not start". Only plain git recovers.
- **R2** — `MergeAbort` unconditionally resets both sides to the recorded pre-merge SHAs, discarding a conclude-commit the record it is reading says already landed. In the `merge-in`-with-conflicts flow that commit carries the operator's manual conflict resolutions, and the reset runs `force: true`. `ok: true`, no warning, and the record is deleted so nothing remains to reconstruct from. This is a direct violation of the property the campaign names as central.

The other three residual items are real but small: a dangling closed-set member (R3), and a doc justification that is not accurate (R5). R4 was raised and then **withdrawn** on evidence — see below.

Residual 1 is a genuine proof gap, and I found a **race-free, seam-free, fully deterministic route** to the derived `AlreadyUpToDate` field, confirmed live: a `merge --squash` whose source is not an ancestor but whose squash result tree is empty on both sides. It survives R1's fix, because squash writes no `MERGE_HEAD`.

**Merge readiness: NOT READY as reviewed.** R1 and R2 are both data-integrity defects reachable from ordinary operator workflows, not stress artefacts. With both fixed and pinned by sabotage-proven integration tests, I judge the surface mergeable.

## Findings

### R1 — `MergeStart` misclassifies an empty-result merge as `AlreadyUpToDate`, leaving a live `MERGE_HEAD` fabric then abandons — BLOCKING, CONFIRMED (live, deployed binary)

`internal/gitrepo/merge.go:99-106` (the four-way classification switch).

**Mechanism.**
`MergeStart`'s success-path classification reads exactly two signals: whether `git diff --cached --quiet` reports a staged difference, and whether HEAD moved.
It never asks whether git left a live `MERGE_HEAD`.
A **real, non-fast-forward merge whose result tree happens to equal HEAD's tree** — the everyday shape produced when the same change reached both branches independently (a cherry-pick, a backport, a duplicated hand-edit, a revert-and-reapply) — stages nothing and moves no HEAD, yet git has genuinely started a merge and written `MERGE_HEAD`.
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
2. **The merge is silently lost.** `feat` is never recorded as merged into `main` in either history, and `RecordCorrespondence` pins the *pre-merge* SHAs as the pair's post-merge correspondence. The merge is perfectly concludable — `git commit --no-edit` on that state yields a proper two-parent merge commit (`exp2.sh` section B) — fabric just declines to.
3. **The pair is bricked for every fabric merge verb, including fabric's own recovery verb.** With no record and foreign state present, `mergeStateOrForeignErr` returns `*ErrForeignMergeState`:
   ```
   --- lyx fabric merge --abort ---    {"error":"fabricengine: git merge state exists that fabric did not start; ...","ok":false}
   --- lyx fabric merge --continue --- {"error":"fabricengine: git merge state exists that fabric did not start; ...","ok":false}
   --- lyx fabric merge-in feat ---    {"error":"fabricengine: git merge state exists that fabric did not start; ...","ok":false}
   ```
   Only plain `git merge --abort` in both checkouts recovers. fabric told the operator this is state fabric did not start. It did.
4. **Blast radius outside the merge surface.** A plain `git commit -m "…"` in such a checkout produces a **merge commit** carrying whatever unrelated change the operator staged (`exp2.sh` section C: two parents on a commit titled "unrelated sibling commit"). `Commit` is saved here only because it also consults `foreignMergeStatePresent` (`commit.go:125`).

**Suggested fix.** In `MergeStart`'s success path, probe `MergeHeadPresent()` and classify a live `MERGE_HEAD` as `MergeStaged`, ahead of the HEAD-moved test — a merge git has started and can conclude is staged work, whatever the index diff says. The squash form writes no `MERGE_HEAD` (confirmed, `exp2.sh` section A), so the probe is a no-op there and squash's genuinely-nothing-to-do case keeps reporting `AlreadyUpToDate`. A fast-forward leaves no `MERGE_HEAD` either, so the `MergeFastForwarded` arm is untouched.

### R2 — `MergeAbort` discards a conclude-commit the record itself says already landed — BLOCKING, CONFIRMED (live, deployed binary)

`internal/fabricengine/mergelifecycle.go:204` (`MergeAbort` → `resetMergeSides(rec, st.WarpStart, st.WeftStart)`, unconditional).

This is the most dangerous member of residual 2's enumerated class (table below): the sites where the record survives an error return *after* a conclude-commit has landed on one side.

**Mechanism.** `concludeMergeSides` lands warp first, then weft, writing each side's SHA into the record as it goes. If the weft conclude fails, it returns `*ErrMergeIncomplete` and **deliberately retains the record** — with `warp_committed` populated. `MergeAbort` then resets *both* sides to `WarpStart`/`WeftStart` unconditionally. It reads the very record that records the landed commit, and ignores that field.

**Failure scenario** (`scen-abort-after-conclude.sh`, hub `hub2`). A refusing `pre-commit` hook in the weft checkout makes the weft conclude fail — the same shape as `commit.gpgsign=true` with no key, a full disk, or any hook-enforced commit policy.

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
In the `merge-in`-with-conflicts flow that commit is not machine-generated content — it carries the operator's **manual conflict resolutions** — and `resetHardTo` runs with `force: true`, so there is no dirty-worktree brake either.

The prompt states the property directly: *"`MergeAbort` must never discard work that was actually committed."* It does.

**Suggested fix.** `MergeAbort` refuses when a conclude-commit may already have landed on either side, via a new member of the closed guard-reason set, leaving `MergeContinue` — which already skips a side whose committed SHA is recorded, so it is idempotent here — as the correct recovery.
This is the exact mirror of round 1's F1 (`MergeContinue` refuses an attempt whose record shows an empty outcome, leaving `--abort` as the recovery). Together the two refusals make each half-finished shape recoverable by exactly one verb, and neither verb destructive.

The predicate must be wider than `st.landedConcludeCommit()` alone, because the table below turns up four sites where the commit landed and the record does **not** say so (the `CurrentSHA`/`saveMergeState` failures immediately after a successful `git commit`). A field-free predicate closes all of them with no schema change:

> a conclude may have landed on a side iff its recorded committed SHA is non-empty, **or** its recorded outcome is `staged`/`conflicted` and its HEAD is no longer at its recorded start SHA.

That is exact: an `up_to_date` side is never concluded and cannot move; a `fast_forwarded` side moved legitimately and abort is documented to reset it; an empty-outcome side never started. Only a `staged`/`conflicted` side can have had a commit put on it, and the only thing that puts one there is the conclude.
The failure direction is safe — an unreadable HEAD or an unexpected move over-refuses rather than destroying.

The closed set's same-commit rule means `mergevocab_test.go` and `mergeerrors_test.go` move with it, and `doc.go` must name the escape hatch: if the underlying git failure cannot be fixed, plain git is the last resort.

### R3 — `mergeReasonNoMergeInProgress` is a dangling member of the closed guard-reason set — NIT, CONFIRMED (traced)

`internal/fabricengine/mergeerrors.go:24`.

```
$ grep -rn "mergeReasonNoMergeInProgress" --include=*.go .
internal/fabricengine/mergeerrors.go:24        <- declaration
internal/fabricengine/mergeerrors_test.go:117  <- pins the set's contents
internal/fabricengine/mergevocab_test.go:98    <- pins the set's contents
internal/fabricengine/mergevocab_test.go:142   <- pins the set's contents
```

No production code path produces it. The no-merge case is the typed `*ErrNoMergeInProgress` (`mergelifecycle.go:84`), and that is the right shape: the closed set exists for **aggregated precondition reasons** several guards contribute to one `*MergeGuardError`, whereas "no merge in progress" is a terminal, standalone disposition with nothing to aggregate alongside. Its `Error()` string is already the same words, so nothing is lost.

**Suggested fix.** Delete the constant from the set and from the three test pin-lists in the same commit. Producing it instead would mean converting `MergeContinue`/`MergeAbort`'s typed error into a guard error — strictly worse for the CLI envelope, and contrary to `mergeerrors.go`'s own doc comment.

### R4 — WITHDRAWN on evidence

I initially recorded a finding that `doc.go` describes the four sibling guards as a uniform set while `Commit` alone also consults `foreignMergeStatePresent` (`commit.go:125`, versus `pull.go:221` / `checkout.go:48` / `remove.go:65`).
Reading `doc.go:920-923` properly, it already says so explicitly: *"`Commit`, `Pull`, `Topology.Checkout` and `Topology.Remove` all return `*ErrMergeInProgress` while a merge record exists (`Commit` additionally refuses foreign git-level merge state with no record)"*.
The doc is accurate; my first reading of it was not. Recorded here rather than deleted, so the log-as-you-go trail shows the correction.

The asymmetry itself stays adjudicated-and-accepted: the record-only guards are correct for fabric-started merges, and after R1's fix fabric no longer manufactures foreign state of its own. What remains is genuinely human-left plain-git state, which the spec scopes out. Broadening the three verbs would make a hub carrying any human-left merge state un-checkout-able, which is worse.

### R5 — `doc.go`'s justification for the `CheckoutDetached`/`RestoreBranch` exemption is not accurate — LOW, CONFIRMED (traced)

`internal/fabricengine/doc.go:938-940`:

> `CheckoutDetached`/`RestoreBranch` are the one knowing exception — raw primitives driven only by webster's integration bisect, **whose harmful direction is closed instead by the attached-HEAD precondition above.**

This is the argument round 1 accepted, and the prompt asks whether it was convenient. It was.
F2's attached-HEAD precondition closes *starting a merge while detached*. It does nothing about the opposite order: `CheckoutDetached` **detaching a checkout that is already mid-merge**, which is exactly what `internal/websterengine/integration.go:133` does on the warp side with no merge probe of any kind.

What actually happens is better than the doc's argument suggests, but for a different reason: git itself refuses `checkout --detach` while unmerged index entries exist ("you need to resolve your current index first"), so the conflicted window — the long one, where an operator sits for minutes — is closed by git, not by F2. The window that stays open is the narrow resolved-but-not-concluded one (index clean, `MERGE_HEAD` live, record live), where `checkout --detach` succeeds and silently drops `MERGE_HEAD`, stranding the record.

I agree with round 1's *conclusion* — this belongs to webster's surface, and putting a merge-record probe into a primitive `doc.go` deliberately classes as raw would be scope creep — but the stated *reason* is wrong and should not be left in the module doc for a future round to re-derive.

**Suggested fix.** Doc-only: replace the F2 appeal with what actually holds, and name the residual narrow window as a known, accepted hazard belonging to the caller.

## Enumerated class — post-record error returns (residual 2)

**Class definition.** Every error return that executes at a point where the merge-state record exists on disk, and which does not delete the record before returning.

**Enumeration method (reproducible).** `enumerate.sh` resolves each function's extent from the source rather than hardcoding line numbers, then lists every error-returning statement inside it:

```sh
ext() { awk -v fn="$2" '$0 ~ "^func .*\\) "fn"\\(" || $0 ~ "^func "fn"\\(" {start=NR}
                        start && $0=="}" && NR>=start {print start" "NR; exit}' "$1"; }
# for each of: merge.go{MergeIn,Merge,syncSideBeforeMerge,selfAbortMergeAttempt},
#              mergelifecycle.go{concludeMergeSides,MergeContinue,MergeAbort},
#              destroy.go{resetMergeSides}
awk -v s=$START -v e=$END 'NR>=s && NR<=e &&
  /return .*err|return .*&Err|return .*newMergeGuardError|return .*selfAbortMergeAttempt|return .*mergeStateOrForeignErr/' FILE
```

**Counts.**

| | count |
|---|---|
| Error-returning statements across the whole merge surface (the awk sweep's raw output) | 94 |
| …of those, sites lying in the post-record region and therefore adjudicated below | 45 |
| **…of those, IN CLASS (record exists and is not deleted)** | **41** |
| …of the in-class rows, sites where a conclude-commit may already have landed | 17 |
| …of those 17, sites where the record makes the landed commit **visible** (`*_committed` set) | 13 |
| …of those 17, sites where the landed commit is **invisible** to the record | 4 |

The awk sweep alone gives 94, which over-counts (it includes the whole pre-record guard stage) and simultaneously under-counts the class, because five sites are `return`s of a *helper* whose own internal returns are the real class members (`selfAbortMergeAttempt` ×4 call sites, `concludeMergeSides` ×3 call sites, `resetMergeSides` ×4 call sites). The 41 below counts the helper's internal sites once each and attributes them, rather than counting each call site.

**The table.** `next continue` / `next abort` answer the question that matters: with the record left exactly in that state, what does the operator's next recovery verb do?

#### `MergeIn` (record first written at `merge.go:155`)

| # | site | state left | in class | next `MergeContinue` | next `MergeAbort` |
|---|---|---|---|---|---|
| 1 | `merge.go:156` | `saveMergeState` failed; `state.WriteJSON` is atomic-replace so nothing is on disk | **no** | `ErrNoMergeInProgress` | `ErrNoMergeInProgress` — correct, nothing happened |
| 2 | `merge.go:165` | record written, `WarpOutcome` save failed; warp already merged in the worktree | yes | refuses, `merge attempt did not reach both sides` (F1) | resets both to start — correct |
| 3 | `merge.go:177` | record has warp outcome, weft outcome save failed; both sides merged | yes | refuses (F1) | resets both — correct |
| 4 | `merge.go:188` | warp `ConflictedFiles` failed mid-conflict-report | yes | refuses, `unresolved conflicts remain` | resets both — correct |
| 5 | `merge.go:194` | weft `ConflictedFiles` failed | yes | refuses, `unresolved conflicts remain` | resets both — correct |
| 6 | `merge.go:203` | unmappable-conflict self-abort: `resetMergeSides` failed, record retained | yes | refuses, `unresolved conflicts remain` | retries the reset — correct, this is the intended recovery |
| 7 | `merge.go:206` | unmappable-conflict self-abort: reset done, `deleteMergeState` failed | yes | conclude on reset repos → `ErrMergeIncomplete` (git has nothing to commit) — **noisy but not destructive** | re-resets to the same SHAs, deletes record — idempotent, correct |
| 8 | `merge.go:208` | `ErrUnmergeableState`; record deleted at :205 | **no** | `ErrNoMergeInProgress` | `ErrNoMergeInProgress` — correct |
| 9 | `merge.go:217` | delegates to `concludeMergeSides` — see rows 22-27 | (see 22-27) | | |
| 10 | `merge.go:222` | both concludes landed & recorded; warp `CurrentSHA` failed | yes | idempotent: both sides skipped, correspondence recorded, record deleted — **correct** | **DESTRUCTIVE — discards both landed conclude commits** |
| 11 | `merge.go:226` | as row 10, weft `CurrentSHA` failed | yes | idempotent, correct | **DESTRUCTIVE** |
| 12 | `merge.go:229` | both landed; `RecordCorrespondence` failed | yes | idempotent, correct | **DESTRUCTIVE** |
| 13 | `merge.go:232` | both landed & correspondence recorded; `deleteMergeState` failed | yes | idempotent, correct | **DESTRUCTIVE** |
| — | `merge.go:161`, `:173` | delegate to `selfAbortMergeAttempt` — see rows 20-21 | (see 20-21) | | |

#### `Merge` (record first written at `merge.go:365`)

| # | site | state left | in class | next `MergeContinue` | next `MergeAbort` |
|---|---|---|---|---|---|
| 14 | `merge.go:366` | atomic-replace failure, nothing on disk | **no** | `ErrNoMergeInProgress` | `ErrNoMergeInProgress` — correct |
| 15 | `merge.go:375` | warp outcome save failed | yes | refuses (F1) | resets both — correct |
| 16 | `merge.go:387` | weft outcome save failed | yes | refuses (F1) | resets both — correct |
| 17 | `merge.go:399` | conflict self-abort: `resetMergeSides` failed | yes | refuses, `unresolved conflicts remain` | retries the reset — correct |
| 18 | `merge.go:402` | conflict self-abort: reset done, delete failed | yes | `ErrMergeIncomplete` — noisy | idempotent re-reset — correct |
| 19 | `merge.go:404` | `ErrMergeInRequired`; record deleted at :401 | **no** | `ErrNoMergeInProgress` | `ErrNoMergeInProgress` — correct |
| 20-21 | `merge.go:416`, `:420` | both landed; `CurrentSHA` failed | yes ×2 | idempotent, correct | **DESTRUCTIVE ×2** |
| 22 | `merge.go:423` | both landed; `RecordCorrespondence` failed | yes | idempotent, correct | **DESTRUCTIVE** |
| 23 | `merge.go:426` | both landed; `deleteMergeState` failed | yes | idempotent, correct | **DESTRUCTIVE** |

#### `selfAbortMergeAttempt` (reached from `merge.go:161`, `:173`, `:371`, `:383`)

| # | site | state left | in class | next `MergeContinue` | next `MergeAbort` |
|---|---|---|---|---|---|
| 24 | `merge.go:500` | reset failed, record **deliberately retained** (documented) | yes | refuses (F1) if an outcome is empty, else concludes a half-reset pair | retries the reset — correct, and the doc's stated intent |
| 25 | `merge.go:503` | reset succeeded, `deleteMergeState` failed | yes | conclude on reset repos → `ErrMergeIncomplete` — noisy | idempotent re-reset — correct |

**Adjudication of row 24 against the doc comment**, as residual 2 requires rather than assuming the comment settles it: retention here is right. The pair is in a half-reset state that only `MergeAbort` can finish, and deleting the record would make `MergeAbort` return `ErrNoMergeInProgress` on a pair that genuinely needs one. The doc's reasoning holds.

#### `concludeMergeSides` (reached from `merge.go:217`, `:411`, `mergelifecycle.go:155`)

| # | site | state left | in class | next `MergeContinue` | next `MergeAbort` |
|---|---|---|---|---|---|
| 26 | `mergelifecycle.go:39` | warp `git commit` failed → **nothing landed** (a non-zero `git commit` does not create a commit) | yes | retries the warp conclude — correct | resets both — correct, nothing to lose |
| 27 | `mergelifecycle.go:44` | warp commit **landed**, `CurrentSHA` failed → record says `warp_committed:""` | yes | `MergeConclude` on a concluded warp → fails again → `ErrMergeIncomplete`. **Stuck** | **DESTRUCTIVE — and the record does not even reveal it** |
| 28 | `mergelifecycle.go:48` | warp commit **landed**, `saveMergeState` failed → record says `warp_committed:""` | yes | as row 27, **stuck** | **DESTRUCTIVE — invisible** |
| 29 | `mergelifecycle.go:56` | warp landed **and recorded**, weft `git commit` failed. `ErrMergeIncomplete`, **deliberate retention** (documented) | yes | skips warp, retries weft — **correct, and the intended recovery** | **DESTRUCTIVE — this is R2's live reproduction** |
| 30 | `mergelifecycle.go:61` | weft commit **landed**, `CurrentSHA` failed → `weft_committed:""`; warp landed & recorded | yes | retries weft conclude → fails → **stuck** | **DESTRUCTIVE — invisible on the weft half** |
| 31 | `mergelifecycle.go:65` | weft landed, `saveMergeState` failed | yes | as row 30, **stuck** | **DESTRUCTIVE — invisible** |

**Adjudication of row 29 against the doc comment.** Retention is right — `MergeContinue` is genuinely the correct recovery and is idempotent. What is *not* right is that the doc treats retention as the end of the story: retaining a record whose warp half is committed leaves `MergeAbort` armed and destructive against it. Retention is correct; leaving `MergeAbort` unguarded against it is R2.

Rows 27/28/30/31 are the four **invisible** sites and are why R2's fix cannot key on `landedConcludeCommit()` alone.
They also independently make `MergeContinue` stuck, since it will retry a conclude git has already performed. That is a strictly better failure than silent destruction, and it is closed for the operator by R2's predicate (which refuses the abort and leaves plain git as the documented last resort), so I am not adding a second mechanism for it.

#### `MergeContinue` (record exists for every row; `:121` and `:124` are pre-record and excluded)

| # | site | state left | in class | next `MergeContinue` | next `MergeAbort` |
|---|---|---|---|---|---|
| 32-33 | `mergelifecycle.go:129`, `:133` | `ConflictedFiles` probe failed; nothing done | yes ×2 | retries — correct | resets both — correct |
| 34 | `mergelifecycle.go:141` | guard refusal (`unresolved conflicts remain` / F1's `attempt did not reach both sides`) | yes | refuses again — correct, this is the point of the guard | resets both — correct, and F1's documented recovery |
| 35 | `mergelifecycle.go:146` | `ensureWeftLockDir` failed | yes | retries — correct | resets both — correct |
| 36 | `mergelifecycle.go:150` | lock acquisition failed | yes | retries — correct | resets both — correct |
| 37 | `mergelifecycle.go:155` | delegates to `concludeMergeSides` — rows 26-31 | (see 26-31) | | |
| 38-39 | `mergelifecycle.go:160`, `:164` | both landed; `CurrentSHA` failed | yes ×2 | idempotent, correct | **DESTRUCTIVE ×2** |
| 40 | `mergelifecycle.go:167` | both landed; `RecordCorrespondence` failed | yes | idempotent, correct | **DESTRUCTIVE** |
| 41 | `mergelifecycle.go:170` | both landed; `deleteMergeState` failed | yes | idempotent, correct | **DESTRUCTIVE** |

#### `MergeAbort` (`:188`, `:191` are pre-record and excluded)

| # | site | state left | in class | next `MergeContinue` | next `MergeAbort` |
|---|---|---|---|---|---|
| 42 | `mergelifecycle.go:196` | `ensureWeftLockDir` failed; nothing done | yes | per the record's own shape | retries — correct |
| 43 | `mergelifecycle.go:200` | lock acquisition failed; nothing done | yes | per the record's shape | retries — correct |
| 44 | `mergelifecycle.go:205` | `resetMergeSides` failed, possibly **half-reset** (warp reset, weft not — `destroy.go:1206` returns before the weft request) | yes | on a half-reset pair, concludes garbage or `ErrMergeIncomplete` | retries the reset — correct, and the only right recovery |
| 45 | `mergelifecycle.go:208` | both reset, `deleteMergeState` failed | yes | `ErrMergeIncomplete` on reset repos — noisy | idempotent re-reset — correct |

#### `resetMergeSides` (`destroy.go:1206`, `:1217`)

Both are pure propagation into rows 6, 17, 24, 44 and carry no record disposition of their own; counted within those rows rather than separately.

#### `syncSideBeforeMerge` (`merge.go:447`, `:459`, `:467`, `:475`, `:482`)

All five run **before** the record is written (`Merge` syncs at `:314-319`, saves at `:365`) and are therefore out of class. Rows retained for completeness: each returns to `merge.go:315`/`:318`, which returns with no record on disk, so both recovery verbs correctly report `ErrNoMergeInProgress`. Note the sync's `KindRepoAdvanced` mutation is honestly retained in the record on that error path — checked against focus item 8 and correct.

**Summary of the class.** 41 in-class sites. 24 of them leave a state both recovery verbs handle correctly. **17 leave a state where `MergeAbort` destroys a landed conclude-commit** — 13 visibly, 4 invisibly. All 17 are closed by R2's single guard.

## Residual 1 — reaching `bothSidesAlreadyUpToDate` honestly

The proof gap is real and I reproduce the diagnosis: `merge.go:236` and `:430` are the only consumers, and the pre-lock probe at `merge.go:132`/`:340` short-circuits every ordinary second call with a hardcoded `AlreadyUpToDate: true` from a different return site, so `TestMergeCrucible_ResultFlagsDescribeWhatHappened/SecondCallReportsAlreadyUpToDateNotCommitted` never reads the derived field.

**Do the two values ever disagree in a way that matters?** They disagree exactly when both sides' `MergeStart` outcomes come back `up_to_date` while the pre-lock probe said otherwise. Two ways to get there:

1. **The concurrency-loser case** the doc describes — another process lands the same merge between this call's probe and its lock. Real, but not deterministically constructible from outside the process without a production test seam.
2. **A race-free route I found and confirmed live**: `merge --squash` of a source that is *not* an ancestor of HEAD (so `IsAncestor` is false and the pre-lock probe cannot early-return) but whose squash result tree equals HEAD's tree on both sides (so both post-lock `MergeStart` outcomes are `up_to_date`).

Route 2 is fully deterministic, needs no seam, no goroutines, and no sleeps. Confirmed through the deployed binary (`scen-squash-utd.sh`, hub `hub3`):

```
warp: feat ancestor of HEAD? NO
weft: feat-weft ancestor of HEAD? NO
$ lyx fabric merge feat --squash
{"already_up_to_date":true,"committed":false,"mutations":[],"ok":true,"partial":false}
warp HEAD moved: no    weft HEAD moved: no
warp MERGE_HEAD: none  weft MERGE_HEAD: none    record: absent
```

`already_up_to_date: true` with neither source an ancestor is only producible by the derived field.
It survives R1's fix because `git merge --squash` writes no `MERGE_HEAD` (confirmed, `exp2.sh` section A), so the new probe does not reclassify it — and reporting `AlreadyUpToDate` here is the honest answer: nothing to squash, nothing committed, HEAD unmoved.

So the answer to "can the hardcoded and derived values disagree in a way that matters" is **yes, and route 2 is a supported single-process workflow, not just a race**. The derived field is the correct one; the finding is only that it was never asserted.

## Re-evaluated deferred items from round 1

**A sibling verb slipping through the unlocked guard window** — attempted, **not reproduced**, agree with round 1's decision not to record it.
Harmful shape targeted: `fabric commit` landing a weft commit after `MergeIn` captured `weftStart` (`merge.go:122`) and before it took the write lock (`merge.go:140`), making `weftStart` stale so a later `MergeAbort` resets past it.
75 attempts: 25 interleaved on hub `hubA`, 25 strictly-sequential control of the identical sequence on `hubA`, 25 interleaved on the independent hub `hubB`. **0 losses in all three arms.**
Two earlier versions of the detector were unsound — both fired on the sequential control, which is what an unsound detector looks like, and both were discarded rather than reported. The third version requires `"committed":true` in the `fabric commit` envelope before it treats an iteration as a candidate, and checks reachability of the committed path from the weft branch tip both before and after the abort. Not a finding.

**`CheckoutDetached`/`RestoreBranch` abandoning a merge in progress** — re-examined; the conclusion holds but the stated reason does not. Recorded as **R5**.

## Docs & operability

- `doc.go`'s "What the result flags mean" paragraph already describes the derived-flag contract correctly, including the concurrency-loser case. It just was not tested. No doc change needed for residual 1 beyond noting the squash route, which R1's fix makes worth stating.
- `doc.go:938-940` — R5.
- `doc.go`'s merge-abort paragraph ("resets both sides unconditionally — including a fast-forwarded side and a side that never moved") becomes wrong the moment R2 lands and must move in the same commit.
- `internal/gitrepo/doc.go`'s `MergeStart` description of the four-way classification must move with R1.
- `SANDBOX-FABRIC-SUITE.md` covers no merge scenario of either shape; both R1 and R2 are live behaviours it should carry.

## What was tested

Every command below was run from `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-hardening` unless a hub path is named. `lyx` is the deployed dev binary at `.dev-bin/lyx`, rebuilt via `./deploy-dev` from `aa7fcd49` before the first live scenario.

**Hermetic tier (no build tag)** — baseline before any edit:

```
go build ./...                                                                    -> clean
go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... -> clean
go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... \
        ./internal/gitrepo/... ./cmd/lyx/...
  ok internal/fabricengine 0.114s   ok internal/fabriccli 0.006s
  ok internal/gitrepo 0.004s        ok cmd/lyx 0.304s
```

**Integration tier (`-tags integration`)** — baseline before any edit:

```
go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... \
        ./internal/fabriccli/... ./internal/gitrepo/...
  ok internal/fabricengine 43.814s  ok internal/fabriccli 4.340s  ok internal/gitrepo 2.371s
```

**Raw-git probes (no lyx, no build tag — establishing what git actually does):**

| script | what it established |
|---|---|
| `exp1.sh` | A non-ancestor merge with an identical result tree exits 0, stages nothing, moves no HEAD, and **leaves `MERGE_HEAD`** — the exact input `MergeStart`'s classifier misreads. Root cause of R1. |
| `exp2.sh` A | The **squash** form of the same input leaves **no** `MERGE_HEAD` and writes `SQUASH_MSG`. This is what makes R1's fix safe for squash and what makes residual 1's route 2 work. |
| `exp2.sh` B | That state is concludable: `git commit --no-edit` yields a proper two-parent merge commit. So R1 loses a merge fabric could have landed — it is not a harmless no-op. |
| `exp2.sh` C | A plain `git commit -m` in that state produces a two-parent **merge commit** carrying an unrelated staged change. R1's blast radius. |

**Live driving through the deployed binary, on real hubs built from local bare repos (`mkhub.sh`):**

| hub | script | result |
|---|---|---|
| `hub1` | `scen-empty-result.sh` | **R1 reproduced end to end.** `merge-in` reports `ok:true, already_up_to_date:true`, deletes its record, leaves `MERGE_HEAD` in both checkouts; `--abort`, `--continue` and a retried `merge-in` all then return `ErrForeignMergeState`. |
| `hub2` | `scen-abort-after-conclude.sh` | **R2 reproduced end to end.** Warp conclude lands and is recorded; weft conclude fails on a `pre-commit` hook; `merge --abort` returns `ok:true` having discarded the landed warp conclude commit and deleted the record. |
| `hub3` | `scen-squash-utd.sh` | Residual 1's route 2 confirmed: `merge --squash` of a non-ancestor source reports `already_up_to_date:true` from the derived field, with no `MERGE_HEAD`, no record, and no HEAD movement. |
| `hubA`, `hubB` | `scen-race-sibling.sh` | Round 1's deferred race: 25 interleaved + 25 sequential control on `hubA`, 25 interleaved on independent `hubB`. 0/75. Not a finding. |

**Sabotage proofs behind the clean results.** The clean results in this review are the two *negative* ones — the race non-reproduction, and the baseline green gates. The race probe was validated in the opposite direction: two successive detector versions were rejected precisely because they fired on the strictly-sequential control, and the accepted version was only trusted after the control read 0 while the detector still had a live path to a positive (it fires on any iteration where `fabric commit` reports `"committed":true` and the path is then unreachable). The sabotage proofs for the *fix* side — hardwiring `bothSidesAlreadyUpToDate` to `false`, reverting R1's classifier probe, reverting R2's guard — belong to Job 2 and are recorded in the fixer report.

**What I could NOT verify, and why.**

- **Windows path behaviour** — explicitly out of scope; this host is Linux. Nothing in R1/R2/R3/R5 is path-shaped, so I record no Windows-specific concern.
- **The concurrency-loser route to `bothSidesAlreadyUpToDate`** (route 1) — I did not construct it, because route 2 reaches the same code deterministically and route 1 could only be made reliable with a production test seam. This is a deliberate choice for a better test, not an unverified gap: the code path is the same two lines.
- Nothing was skipped for cost, operator-assistance, or wall-clock reasons.

## Post-fix status (appended after Job 2)

All five recorded findings were fixed in-round; nothing deferred. See
`_mill/fabric-merge-review-opus-medium-r2-fixer-report.md` for the implementation, the false-green
proof behind each new test, the determinism runs, and the live re-drive on the redeployed binary.

The review's "NOT READY as reviewed" verdict above stands as the judgment of the tree at
`a5700c41`. **As of `1e4f8d3b` the verdict is READY**: R1 and R2 are fixed and each pinned by a
sabotage-proven integration test, residual 1's proof gap is closed by a test that is the only thing
in the whole `-tags integration` `fabricengine` tier to fail under the orchestrator's own sabotage,
residual 3 is deleted with the membership rule written down, and both gates are green including a
whole-repo `go test ./...` sweep.
