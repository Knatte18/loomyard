# fabric merge surface — independent review (fable-high-r5)

Round 5 of the fabric-merge crucible campaign (second instalment).
Clean-room review by a fresh agent; no prior-round review/fixer material read before this findings list was complete.

## Scope reviewed

- `internal/fabricengine`: merge.go, mergelifecycle.go, mergeerrors.go, mergeguards.go, mergestate.go, mergestage.go, mergepaths.go + tests
- `internal/gitrepo/merge.go` + tests
- `internal/fabriccli/merge_verbs.go`, `cmd/lyx` wiring
- Docs: `internal/fabricengine/doc.go` "# The merge surface", `docs/overview.md`, `CONSTRAINTS.md`, `README.md`
- SPEC: `git show 3b800bc8:_mill/discussion.md` (+ plan batch headings)

## What was tested

### Hermetic gates (all green)

- `go build ./...` — OK
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok
- `go test -tags integration -count=1 -timeout 30m` over the three packages — all ok (fabricengine 46.7s, fabriccli 4.3s, gitrepo 2.4s)

### Code reading pass

- Read the full SPEC discussion (decisions: recorded merge, two verbs, no-new-commit-until-clean, unified conflict paths, SHA-not-branch merges, aggregated side-free guards, conclude-never-rolls-back, weft-gated reset, correspondence, CLI mirror).
- Read all seven fabricengine merge production files, gitrepo/merge.go, fabriccli/merge_verbs.go, destroy.go's resetMergeSides, commit.go's sibling guard, doc.go lines 846–1046, mergevocab_test.go.

### Live driving (dev binary `.dev-bin/lyx`, deployed via `go run ./tools/deploy -dev`, hub recipe per prompt)

- Hub 2 (`$SCRATCH/hub2`): bare warp/weft (`-b main`), seeded warp `main` (root.txt, src/app.go), `lyx fabric clone` — ok.
- `lyx fabric add task-a` — pair created, both branches pushed.
- Scenario: both-sides divergent conflict. task-a: warp edit root.txt + weft `_lyx/note.md` via `lyx fabric commit`; prime: same paths, different content. `lyx fabric merge-in task-a` → `conflicts:["_lyx/note.md","root.txt"]`, exit envelope `ok:false, partial:false`, markers labelled `>>>>>>> <SHA>` on BOTH sides (no `-weft` token anywhere). Resolved both (weft path staged via `cd warp/_lyx && git add note.md` through the junction), `lyx fabric merge --continue` → `committed:true`, two-parent merge commits on both sides, subjects name SHAs, `fabric-merge.json` gone (find over the hub returns nothing). PASS.
- Scenario: non-ASCII conflicted path. Probe at the git layer first: `git diff --name-only --diff-filter=U` on a conflicted `ä-file.txt` emits the C-quoted form `"\303\244-file.txt"`, while `-z` emits raw bytes. Then live end-to-end: pair task-b, add/add conflict on `_lyx/ä-note.md` (both sides `lyx fabric commit`), `lyx fabric merge-in task-b` → **`fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required`** (ErrUnmergeableState), both sides reset. The conflict is squarely INSIDE the fabric-managed tree; the quoted path `"_lyx/ä-note.md"` fails `weftPathVisible`'s prefix test because it begins with a `"` character. CONFIRMED defect — see F1.

## Provisional findings (to be confirmed)

- F1 (MEDIUM, CONFIRMED live): `gitrepo.ConflictedFiles` returns C-quoted paths for non-ASCII names (`core.quotepath` default); a wired weft conflict is then spuriously classified unmappable → `ErrUnmergeableState` self-abort, making the merge impossible through fabric; a warp-side non-ASCII conflict is reported as a garbled literal the operator cannot resolve against; `MergeStageResolved` cannot match the real path. Fix: `-z` + NUL-split in `ConflictedFiles`.
- F2 (MEDIUM, CONFIRMED by trace): lifecycle guard reads run OUTSIDE the write lock (TOCTOU). Sharpest shape: `MergeAbort` evaluates `concludeLandedReason` and loads the record BEFORE acquiring the weft write lock; a concurrent `MergeContinue` that concludes and deletes the record between guard-eval and lock-acquire leaves `MergeAbort` to `resetMergeSides` (force:true) the freshly landed conclude commits — exactly the destruction `concludeLandedReason` exists to prevent — and `deleteMergeState` tolerates the record's absence, so nothing notices. `MergeContinue` has the mirror shape (stale record adopted/resurrected after another process concluded), and `MergeIn`/`Merge`'s record-existence guard has the same window (a second `MergeIn` during another's resolution window overwrites the live record with a new source). Same family as R4-F3 (mutation-before-mechanism). Fix: re-load + re-validate the record (and the landed guard) after acquiring the lock, refusing on any change.
- F3 (LOW, CONFIRMED by reading): `doc.go`'s merge-surface paragraph "The rest of the mutating surface is deliberately unguarded ... for stated reasons rather than by omission" enumerates every unguarded mutating verb EXCEPT `MergeStageResolved`, which is a mutating, unlocked, unguarded merge-surface verb (its own godoc explains why, but the doc.go enumeration's completeness claim is now false by omission).
- F4 (NIT, CONFIRMED by reading): genuine-failure wrap messages inside the public merge verbs name sides ("fabricengine: resolve warp HEAD", "sync weft before merge", "check warp head attachment", etc.). These are unexpected-infra-error paths, consistent with package-wide practice and outside the named-error/side-free-result contract, but they do cross to non-owner-set callers on infrastructure failures, where two scenarios differing only in side produce different error text. (Assess: normalize the merge-surface wrap texts to side-free forms, keeping the side in internal logs.)

### Live driving, continued (all on hub2 unless noted)

- Already-up-to-date: `merge-in task-a` after it already landed → `{"already_up_to_date":true,"committed":false,"mutations":[]}` — no lock, no record, empty record. PASS.
- Target verb clean squash: task-c (non-conflicting divergence both sides), `lyx fabric merge task-c --squash -m "land task-c"` from prime → single-parent squash commits on both sides with the message, content landed through the junction. PASS.
- Target verb conflict: task-d conflicts with prime on root.txt; `lyx fabric merge task-d` → `ErrMergeInRequired` fixed message, both sides restored to exact pre-merge SHAs (asserted), worktrees clean, no record. PASS.
- Sibling dispositions during a live conflicted record (prime mid-merge on task-d): `commit` and `pull` refuse with the fixed `ErrMergeInProgress` message; `status` succeeds; `remove task-d` (the SOURCE pair, via `mergeSourceInFlight`) refuses. `merge --abort` then restores both sides exactly, record gone. PASS.
- Foreign plain-git merge state (conflicted `git merge main` in task-d's warp checkout): `merge-in` and `merge --abort` refuse with the foreign-state message and leave the state untouched; `commit` refuses (see F5 for its message's misdirection); after plain `git merge --abort`, `merge --abort` correctly reports "no merge in progress". PASS (with F5 noted).
- Detached HEAD: warp-side detach → `checkout is not on a branch`; weft-side detach (task-g-weft) → byte-identical refusal. PASS, both directions live.
- Dirty guards: weft-only-dirty and warp-only-dirty produce byte-identical `worktree dirty` refusals. Aggregation live: dirty + nonexistent source → `source branch is not fabric-managed; source branch not found; worktree dirty` (sorted, deduped); warp-only branch → `source branch is not fabric-managed`. PASS.
- CLI pre-flights: `--continue --squash` and `--abort -m` rejected with usage errors; `--continue --abort` rejected by cobra group; positional + `--continue` rejected. PASS.
- Half-concluded conclude (weft `pre-commit` hook `exit 1`): fixture first degraded to a double fast-forward (prime hadn't advanced past the fork — caught by asserting divergence with `merge-base --is-ancestor` before trusting the scenario; the degraded run incidentally confirmed `committed:false` on a double-ff merge, the derived-flag behavior). Rebuilt with genuine two-sided divergence: `merge-in task-f` → warp conclude landed, weft conclude failed → `ErrMergeIncomplete`, record retained; `merge --abort` refused (`merge conclude already landed`); hook removed, `merge --continue` → concluded weft ONLY (warp HEAD unmoved), record gone. PASS.
- Adoption, legitimate direction: conflicted `merge-in task-g`, resolved, warp conclude landed BY HAND (`git commit --no-edit` with `MERGE_HEAD` live); `merge --abort` refused; `merge --continue` adopted the hand-landed SHA verbatim (merge_committed detail == hand SHA, HEAD unmoved), concluded weft, record gone, `committed:true`. PASS.

### Sabotage checks (run after this findings list was saved, before fixes — results recorded in the fixer report)

Planned: (S1) a `mergeReason*` constant declared in a third file (mergestate.go) must fail both vocab closure tests; (S2) dropping the `parents[0] == start` clause must fail `TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted`; (S3) skipping `weftPathVisible` must fail `TestMergeIn_UnmappablePathConflict_SelfAbortsBothSides`; (S4) moving `Merge`'s lock acquisition after the sync must fail `TestMerge_PreMergeSyncRunsInsideTheWriteLock`.

Results:

- S1: `mergeReasonSabotageProbe` declared in mergestate.go (a THIRD file, not the one round 4 sabotaged) → BOTH `TestMergeVocabulary_GuardReasonSetMatchesConstBlock` and `TestMergeVocabulary_GuardReasonSetIsDeclaredInOneFile` failed, each naming the file. DETECTS. Reverted.
- S2: **FAILED TO DETECT — new finding F7.** With `parents[0] != start` deleted from `sideConcludeAlreadyLanded`, `TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted` still passes (its fixture is a ONE-parent unrelated commit, refused by `len(parents) < 2` alone), and the ENTIRE fabricengine integration suite stays green (43.8s, ok). The first-parent evidence clause — which doc.go describes at length as load-bearing ("a conclude landed on top of some other commit is a merge of a different base and is not adopted") — is guarded by no test. Reverted.
- S3: `weftPathVisible` check bypassed → hermetic `TestMergePaths_UnifyConflictPaths` AND integration `TestMergeIn_UnmappablePathConflict_SelfAbortsBothSides` both failed. DETECTS at both tiers. Reverted.
- S4: lock acquisition moved after the sync step in `Merge` → `TestMerge_PreMergeSyncRunsInsideTheWriteLock` failed. DETECTS. Reverted; working tree confirmed clean.

## Findings

### F1 — MEDIUM, CONFIRMED (live): `ConflictedFiles` returns C-quoted paths; a legitimate wired-tree conflict on a non-ASCII path spuriously aborts as unmergeable

`internal/gitrepo/merge.go:148` (`ConflictedFiles`, `git diff --name-only --diff-filter=U` without `-z`).
Failure scenario (reproduced end-to-end on hub2): add/add conflict on `_lyx/ä-note.md` → git emits `"\303\244..."` C-quoted with surrounding quotes (`core.quotepath` default) → `weftPathVisible` sees a leading `"` instead of `_lyx/` → `unifyConflictPaths` reports unmappable → `MergeIn` self-aborts with `ErrUnmergeableState` ("conflicts outside the fabric-managed tree") for a conflict squarely INSIDE the fabric-managed tree. The merge is impossible through fabric for any non-ASCII conflicted path, and the Fabric Git Invariant forbids the plain-git workaround. A warp-side non-ASCII conflict is reported as the garbled quoted literal, which is not a real worktree path; `MergeStageResolved` can never match the caller's real path against the quoted set.
Fix: use `git diff --name-only --diff-filter=U -z` and split on NUL (probed: emits raw bytes, unquoted). Add an integration test with a non-ASCII conflicted path on both sides.

### F2 — MEDIUM, CONFIRMED (trace): lifecycle guard evaluation runs outside the write lock, so the guard cannot see what lands between guard-eval and lock-acquire

`internal/fabricengine/mergelifecycle.go:297–321` (`MergeAbort`: `loadMergeState` + `concludeLandedReason` before `AcquireWriteLock`), same shape in `MergeContinue` (`mergelifecycle.go:221–254`), and `MergeIn`/`Merge`'s record-existence guard (`merge.go:72–93`, `merge.go:268–294`).
Sharpest failure scenario: P1 `MergeContinue` and P2 `MergeAbort` race on a resolved merge. P2 evaluates `concludeLandedReason` (nothing landed yet) and blocks on the lock; P1 acquires first, concludes both sides, deletes the record, releases; P2 then acquires and `resetMergeSides` (force: true) both sides to the recorded pre-merge SHAs — destroying the just-landed conclude commits, including the operator's hand-written resolutions — exactly the destruction `concludeLandedReason` exists to prevent, and `deleteMergeState` tolerates the record's absence so nothing notices. Mirror shapes: a raced `MergeContinue` acting on a stale record (adoption arm resurrects a deleted record mid-call, reports `committed:true` where a sequential run reports `ErrNoMergeInProgress`); a second `MergeIn` overwriting another merge's live record during its resolution window (its `MergeStart` then classifies the FIRST merge's conflicts as its own — a false-attribution record whose conclude would land the other merge's content under this record's source).
Single-instance flows are unaffected (the merge bar), but the write lock is the mechanism that exists to serialize cross-process writers, and these are its blind windows — the same family as R4-F3's mutation-before-lock.
Fix: `MergeContinue`/`MergeAbort` acquire the lock FIRST and evaluate record + guards under it; `MergeIn`/`Merge` re-verify no record exists after acquiring the lock (refusing with the aggregated `merge already in progress` guard reason). Deterministic in-package integration test: hold the lock, launch the verb (blocks pre-guard), mutate the state it would have trusted, release, assert the verb now answers what a sequential run answers.

### F3 — LOW, CONFIRMED (read): doc.go's unguarded-mutating-surface enumeration omits `MergeStageResolved`

`internal/fabricengine/doc.go:1008` claims "The rest of the mutating surface is deliberately unguarded and safe for stated reasons rather than by omission" and enumerates every such verb — except `MergeStageResolved`, a mutating, lockless, guardless merge-surface verb (its own godoc in mergestage.go explains both omissions, but the doc.go completeness claim is false as written, and doc.go's merge-surface section never mentions the verb at all).
Fix: add `MergeStageResolved` to the enumeration with its one-line reason.

### F4 — NIT, CONFIRMED (read): fabric's own error-wrap prefixes on the merge surface name sides

`internal/fabricengine/merge.go:118,122,126,130,342,345,353,357,361,365`, `mergeguards.go:103,107,143,147`, `mergelifecycle.go:262,266` — genuine-failure wraps whose fabric-authored prefix text names a side ("resolve warp HEAD", "sync weft before merge", "check warp dirtiness", "check warp head attachment", …). The SPEC's settling test binds error messages; the SPEC accepts the wrapped git CAUSE varying (selfAbortMergeAttempt's documented tradeoff), but these side-naming words are fabric's own, and two scenarios differing only in which side's probe failed produce differently-worded fabric text. Infra-failure-only paths; four prior rounds did not flag them; graded NIT.
Fix: make fabric's own prefix words side-free ("resolve checkout HEAD", "sync checkout before merge", …), keeping the wrapped cause (which may still carry a repo path, the SPEC-accepted variation) and the side detail in internal logs where present.

### F5 — NIT, CONFIRMED (live): `Commit`'s foreign-merge-state refusal misdirects the operator

`internal/fabricengine/commit.go:125–131`: with foreign plain-git merge state and no record, `Commit` refuses with `ErrMergeInProgress` ("a merge is in progress; run MergeContinue or MergeAbort first") — but fabric has NO merge in progress (`MergeInProgress` reports false), and following the advice yields `ErrForeignMergeState` ("conclude or abort it with plain git"), a two-hop redirect. Driven live on hub2.
Fix: return `&ErrForeignMergeState{}` for the foreign branch (the same typed, side-free error every merge verb gives that state), keeping `ErrMergeInProgress` for the recorded case; update the sibling test and doc.go's one-line mention.

### F6 — NIT, CONFIRMED (read): `pickMergeSourceSHA` silently swallows `IsAncestor` failures, degrading the freshness rule with no trace

`internal/fabricengine/mergeguards.go:89–93`: `isAncestor, err := repo.IsAncestor(localSHA, remoteSHA); if err == nil && isAncestor { … }` — an IsAncestor infra failure silently falls back to the local SHA, so a stale local source can be merged with no log line, and the godoc does not state the tolerance. Every other best-effort step in this surface (both Fetch sites) logs its tolerated failure.
Fix: `logger.Warn` on the error + state the best-effort rule in the godoc.

### F7 — MEDIUM, CONFIRMED (sabotage): the adoption arm's first-parent evidence clause is guarded by no test

`internal/fabricengine/mergelifecycle.go:158` (`parents[0] != start` in `sideConcludeAlreadyLanded`) and the same line's source-SHA membership loop.
Deleting `parents[0] != start` leaves the whole hermetic AND integration suite green: `TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted`'s fixture is a one-parent commit, refused by `len(parents) < 2` alone, so neither the first-parent clause nor (by the same argument) the source-SHA-membership clause has a test that fails when it is removed. This is the campaign's recurring highest-yield shape: an invariant asserted by a mechanism that cannot detect its violation. The doc (doc.go ~934) states the clause as load-bearing.
Failure scenario the missing test encodes: operator plain-git-aborts a conflicted recorded merge, lands an unrelated commit X, then `git merge <recorded source SHA>` and commits — HEAD is now a TWO-parent merge of the recorded source whose first parent is X, not the recorded start. Without the clause, `MergeContinue` adopts it, reports `committed:true`, records correspondence against a base the paired side never saw, and deletes the record.
Fix: integration tests driving both refusal directions through public `MergeContinue`: (a) parents `[X, source]` (wrong base, right source) → not adopted; (b) parents `[start, Y≠source]` (right base, wrong source) → not adopted. Both must fail when their clause is sabotaged (verified as part of the fix).

## Re-evaluation of the seeded judgement call: `parents[0] == start` adoption evidence

Verdict: KEEP, unchanged. (a) The refused shape — operator lands extra commits on the branch mid-recovery, then hand-merges the source — is a merge of a DIFFERENT base; adopting it would record a conclude whose first parent the paired side's conclude does not correspond to, silently breaking pair correspondence. Refusal is recoverable and documented in doc.go (reset to recorded start, re-merge recorded source). (b) The legitimate recovery flows all pass: crash-after-commit adoption (drove live — hand-landed conclude adopted verbatim); plain-git `merge --abort` + re-merge of the recorded source lands parents `[start, source]` and is adopted, correctly, because it IS semantically this merge's conclude. (c) False evidence requires deliberately fabricating a commit with exactly the recorded parent pair (`git commit-tree`), which is operator self-sabotage outside any threat model this layer owns. (d) The refused-flow frequency argument: the window is crash-during-conclude, already rare; an operator advancing the branch mid-recovery while every sibling verb refuses is rarer still. The safe direction costs the right party.

## Deferred items re-evaluated

- **Windows path behaviour** (`weftPathVisible`/`unifyConflictPaths`): NOT executed this round either — Linux host, no Windows environment reachable headlessly. Stated plainly: still never driven in this campaign.
- **`parents[0] == start` ergonomics**: re-examined above — keep.
- **Four stuck `MergeContinue` states** (first instalment r2 rows 27/28/30/31): F2's fix touches `MergeContinue`'s locking, so the adoption-adjacent integration tests (`TestMergeContinue_InvisibleLandedConclude*`, `*_UnrelatedCommit*`, `*_SquashConclude*`) will be re-run and must stay green post-fix; no behavioral change to those states intended.
- **45-row post-record error-return table**: not re-walked (not required; not touched).

## Scope verdict

The as-built surface delivers the SPEC: two verbs, recorded merge, lifecycle quartet, unified side-free conflict reporting, SHA-labelled markers, aggregated guards, sibling refusals, gated two-sided resets, correspondence recording, CLI mirror with envelope/exit parity. `MergeStageResolved` is a post-SPEC addition serving `mergeresolve`, deliberately narrow. No silently-dropped SPEC requirement found. Over-reach: none found.

