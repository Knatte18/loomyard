# landingshed — what the module must do and why

A spec of `internal/landingshed` written from the module's fix-commit history and its package doc's statement of behaviour, not from how the code is arranged.
It is meant as step 1 of a clean-room pilot: a writer holding this file, the interfaces below and nothing else should be able to rebuild the module and its tests.
Each special case names the commit whose incident or review created it, so a rebuild can tell a needed rule from an accident of the current design.

## Job

A run (a task branch in its own worktree) ends by landing its work on a parent branch.
The module owns the last three rows of every recipe:

1. **Parent-Align** brings the parent's newer commits into the task branch and proves the merged tree still passes the gate.
2. **PR-Review** (only when a pull request is wanted) opens a pull request, waits for a human or agent review of it, and ends when the review ends.
3. **Finalize** takes the parent lock, merges once more, gates the merged tree, lands the work on the parent (a local squash pushed, or a squash merge on GitHub), and marks the board task done.

Two smaller jobs sit beside them: **Describe** drafts or records the change description that becomes the pull request text and the landing commit message, and **ResolvePullRequest** decides whether a landing goes through a pull request.

## Invariants

- **I1. Only one decision of "pull request or not".** PR-Review and Finalize read the same resolver, so they cannot disagree. (`ec46eaf4`, `305312a6`)
- **I2. The parent moves only under the parent lock, and only in Finalize.** Parent-Align writes nothing in the parent pair. (`dd9084b5`)
- **I3. Nothing is merged, gated or pushed over a dirty tree.** The task worktree is clean before the merge-in, after it and after the verify; the parent's code checkout is clean before the parent-side merge. (`412a78c7`, `8a4ef901`)
- **I4. Every merge of the parent into the task is followed by the gate on the merged tree before anything leaves the worktree.** A clean-building merge can break tests. (`34426f42`)
- **I5. Finalize never sends a run back to an earlier row.** A failure it cannot fix is a halt for the orch, never a reroute. (`cb423116`, `e31c9fce`)
- **I6. A halt always names its way forward** (a command to run, ending in `lym loom resume` where the run can continue). (`b194fbc4`, `8a4ef901`)
- **I7. A change the machine does not own is not landed.** Landing, resuming and pushing run only on the run's owner machine, and the parent's owner is read the same way. (`a68d07de`)
- **I8. A landing is idempotent.** A parent that already holds the task's work takes the already-landed path; a re-entered Finalize never merges twice. (`3634810a`, `ec46eaf4`)
- **I9. Only the owner of the lock lands.** The lock record is re-read before each landing attempt; a record that no longer names the run halts. (`dd9084b5`)
- **I10. A seam that fails to report never silently changes a verdict.** A failed display mark, a failed trace write or a failed notice is logged and changes nothing; a failed read the verdict depends on is an error or a halt. (`792fea01`, `78a416ea`)
- **I11. Told geometry.** Every path is handed in; the module derives none. (`PATTERN-cwd-resolution`)

## Rule table: Parent-Align

| Rule | Why (commit) |
|---|---|
| Commit the status file, then reconcile with origin before acting; an ended entry or a stopped branch halts with its way forward; an unreachable origin logs a Warn and proceeds. | A woken machine drove runs on a stale view of origin (`8e292cf1`). |
| Merge the upstream tip read by a fetching read, never the local parent tip. | A landing another run made locally and had not pushed must not be merged in (`dd9084b5`). |
| With no upstream or under push skip, take the lock only to read the local tip, release before the merge. | The merge needs no lock; it writes nothing in the parent pair. |
| Clear a parked merge-in of this module's own earlier attempt before the clean check; stop every live conflict-session strand first; any other merge state is Stuck, never touched. | A stale conflict report refused a later merge-in (`412a78c7`, `eb042716`). |
| Run the row's own gate key over the merged tree, keyed on the tree, so a resolved conflict, new parent commits or a crash between merge and verify re-verify; a no-op merge onto a verified tree skips. | `34426f42`, `412a78c7`. |
| A gate failure writes a landing-failure record (kind, failing tier or build, HEAD, merge-in commit, failing tests, optional trace); a pass removes it. | The record is the fixer's input and the orch's evidence (`eb042716`). |
| A gate whose every failed run crashed its test runner halts and spawns no fixer. | A fixer judges no code from a runner crash (`d33b3f64`). |
| A timeout kills the verify and halts with the log path. A verify that dirties the tree halts. | A hung verify was re-run unfixed (`78a416ea`); a dirtying test is non-hermetic. |
| Parent commits brought in, or a failing gate, run the merge fixer in parent-align mode with `parent_align_fix_attempts`. A clean merge with nothing brought in and a pass is aligned with no fixer. | `dd9084b5`. |
| The fixer's whole change is judged by `LineChange(Baseline, HEAD)` against `finalize_fix_max_lines`, the test-file matcher, and gate-config and skip additions; an over-cap change halts or goes to the Code-Perch when its budget is above 0. | Turns that each stay under the cap but sum past it are over it (`f269badc`). |
| A fixer turn that leaves the tree dirty is re-prompted with the paths; only a dirty tree after the last turn ends the fixer, naming the paths. | `b194fbc4`, `b5e0ed26`. |
| Write a merge record: merged tip, task head before, parent tip last merged, aligned tip, merge commit, overlap. | The overlap is what a later fixer and Finalize's "large merge" test read (`dd9084b5`). |
| A darn run whose diff lies wholly under `tree_scope` runs the tree-scoped command only. | `2313ab5c`. |
| Read the attempt record of the row's earlier name (Merge-In) when its own is absent. | A rename must not lose spent attempts (`3b0f3b08`). |

## Rule table: PR-Review

| Rule | Why (commit) |
|---|---|
| Open or find the pull request, set title and body from the change description, regression check and credit block, record number, URL, base, head. | `ef9d580b`, `58a85abf`. |
| A resolver no closes a recorded open pull request with a comment, drops the record and ends Done. | A branch with no commit beyond the parent has no pull request to open (`305312a6`). |
| A resolver yes marked MergeAtOnce does nothing here; Finalize opens and merges. | A protected parent takes work only through a pull request (`dbd169ae`). |
| Wait in process, no timeout, wake at least a second apart, doubling to a minute while idle, returning to a second when the head moves; a change to the record file ends the sleep early. | `PATTERN-daemon-wakeups`, `46505b90`. |
| A clean, not-yet-gated head runs the `pr-review` gate; a pass pushes the head and records it gated; a failure pushes nothing, mails the reason once per head. `pr-continue` clears the mailed head so the head is gated again. A failure whose every run crashed mails nothing and halts. A head moved or tree dirtied during the gate discards the verdict. | A gate failure from outside the change deadlocked the row (`57f125eb`). |
| Read the pull request's status at most once a minute (once at the first wake that sees the continue flag); re-set title and body when the description yields other text than last set, never mid-edit. | The agent changes the text by editing the description (`8ef911f7`). |
| Verdicts: merged at the gated head with a clean tree is Done; merged elsewhere or without task commits is Stuck; continue flag at a clean gated head records the operator's ending and is Done, leaving the pull request for Finalize to merge; approved at the gated and task head is Done; a closed pull request is Done only after the operator's ending. | `ec46eaf4`. |
| `pr-findings` sends findings back to Code-Review as a reroute on a clean tree; a dirty tree halts; the next call clears the mark only when its own newest history entry is that reroute. | `8e35d709`. |
| One interactive agent session per run: reuse a live strand, else resume the session id, else start; none after the operator's ending; stopped before each read that may end the review, on every verdict, Stuck and error; resumed when the read keeps waiting; paused with the hub pause. Its shell guard allows gh reads, comments and text edits, and refuses push and every merge, close or open. | `8ef911f7`, `ec46eaf4`. |
| Ask the parent orch for a review once per pull request record, when the setting is on; never gate on the answer. | `639e4fab`. |
| A draft pull request is marked ready before the text is set. | The link, comments and history survive. |
| A transient push or GitHub failure is an error the driver re-steps; token failure, a refused push and any other failure are Stuck; the text names the token only for a token failure or 401 or 403. | `305312a6`. |

## Rule table: ResolvePullRequest

Inputs: the receiving branch's `pull_request` (`on`, `off`, `required`, `merge`), the entry's field (empty, `on`, `off`, `merge-in`), origin facts, whether the task branch has a commit beyond its parent, GitHub's branch rules, and for a beam its landing record.

- No commit beyond the parent is a no before any origin fact. (`305312a6`)
- `required` and `merge` want a pull request; `on` wants one unless the field is off; `off` only when the field is on.
- `merge-in` is a no marked TrueMerge: the beam keeps the parent's commit as an ancestor. (`262d7c50`)
- `merge` yes is marked MergeAtOnce; a beam's yes always is. (`dbd169ae`)
- A missing origin fact halts when the parent is required or the field is on, and is a no when the parent is `on` alone.
- GitHub rules requiring a pull request count as required; they are read only when every other input says no, a failed read halts a run holding a record (a no would close its pull request and land by a push GitHub refuses) and reads as no rule otherwise. (`ec46eaf4`)

## Rule table: Finalize

| Rule | Why (commit) |
|---|---|
| Parse the description, commit the status file, take the lock through the shared guard; every way out releases it; a dead owner's lock is taken over. | `dd9084b5`. |
| Reconcile with origin as Parent-Align does, then the owner checks (run and parent), then the land check for a beam's pawl run. A parent that already holds the task's work skips the owner checks. | `a68d07de`; an adopter-first landing leaves the work on the parent. |
| Open the parent pair and sync it: fast-forward a behind parent in its own pair; ahead, diverged, missing, mid-merge or tracked-dirty is Stuck; never push or rewrite the parent here; skip fetch and fast-forward under push skip. | `dd9084b5`, `8a4ef901`. |
| Ask the resolver; a no closes a recorded open pull request and lands locally; a yes never lands by a local push; a yes with no record halts unless MergeAtOnce. | `ec46eaf4`: a protected parent refuses the local push. |
| Merge the parent into the task through the resolver, for every recipe; a conflict is resolved under the lock; a "large" merge (parent's changes since the merged tip touch a file the task changed) writes the merge record with its overlap. | A reroute to Parent-Align re-entered a whole review perch and looped (`cb423116`). |
| Gate the merged tree with the `finalize` key; keyed on the tree, so a run that went right holds the lock for one merge, a skipped gate and the landing. | `34426f42`. |
| Rerun a failing gate once on the same head when the failure lists tier and guard runs alone; the failed tests alone when only Go tests are named; a pass is flaky and listed in the landing text. Timeouts, crashes, build, vet and lint failures are not rerun. | A race between tests started a fixer for a fault the change did not cause (`dea07060`). |
| A failing gate spawns the merge fixer in finalize mode, with `finalize_fix_attempts` turns, a recorded Baseline and the caps as in Parent-Align. | `dd9084b5`. |
| Free the parent lock before a fixer or a fix review starts; take it again at the front of the queue, merge anew, re-verify, and spawn on the attempts left. | One run held `main` for twelve minutes while four green runs queued (`1fbac34f`). |
| An over-cap fixer change goes to one solo fix review that reads only the fixer's commits; approval lands and records Reviewed; a rejection hands findings to the fixer in place on a fresh turn cap; `finalize_fix_review_rounds` bounds the rounds; running out halts; `circling accept` appends the open findings to the description and lands at that head. | A ten-line test lock cost a whole review fan (`f269badc`, `e31c9fce`). |
| Retry once after a merge-in-required error from the parent-side merge (parent moved by run records alone), merging, gating and landing again; never merge on GitHub twice. | `cb423116`. |
| Commit the parent pair's own run records before the parent-side merge so a sibling's uncommitted status does not trip the merge guard. | `412a78c7`. |
| Parent-side merge squashes by default, except TrueMerge. A dirty parent is Stuck naming the checkout and up to 20 tracked paths. | `8a4ef901`. |
| Re-read the lock record before each landing attempt; a record not naming the run halts. | A hand-over or a `batten lock release` releases it (`a68d07de`). |
| Land through GitHub when a pull request is wanted: push the task branch, find or open the pull request (MergeAtOnce), reopen a closed one under the operator's ending, merge squashed with the pushed head as expected head and title `<title> (#<n>)`; retry a merge GitHub reports pending for up to a minute; name the right setting for a method refusal. | `ec46eaf4`, `dbd169ae`, `067f4f02`. |
| A beam's landing reads the beam's landing record, merges a recorded open pull request as it stands, drops a spent record after the landing. | `bf59ec43`. |
| After the parent-side merge and before the push: hand the landing commit to the recorder, record the landed size, push the parent, push the run's records, write the board task done; before each, read whether another machine owns the run and hand over if so. A rejected push over a parent already holding the work archives the local merge; any other rejection is Stuck with a repair note (reset or rebase, `fabric remove`), or the protected-branch note naming `pull_request=merge`. | `8e292cf1`, `dbd169ae`, `3634810a`. |
| The landing commit is the title, the body, the regression check, then the credit block (plain lines, never trailers); the pull request carries the same. | `364c47ca`, `58a85abf`, `81e74d76`. |
| Notify the orch once about per-worktree config files the task changed, never carrying them. | A task's config edit must not reach the parent silently. |
| A Done clears the attempt record. | A reached-again entry starts at zero attempts. |

## Rule table: Describe

- A session per recipe from one stencil writes `_lym/landing/summary.md`; the validator is the Describe gate. (`3634810a`)
- A beam's description is written by the beam's orch; the module drafts a starting point (the beam's board entry and its first-parent commits oldest first, naming each landed pull request by `(#<n>)`) and records the orch's text after validation. (`112bb821`)

## Hand-back and escalate

- `HandsBack` names the halts the orch takes over: a blocked stop at Parent-Align or Finalize whose reason opens with `LandingStopPrefix`. (`e31c9fce`)
- A fixer's `escalate` halts under `EscalateStopPrefix` on a tapestry, darn or sampler run (the arbiter judges it first) and under `LandingStopPrefix` on a pawl run. (`cb423116`)

## Config

`landing.yaml` holds hub-wide keys (`squash`, caps, attempts, timeouts, `credit_*`, `pr_reviewer`, `describe`, `deploy`, `deploy_branches`), read from the board directory and never from a pair, so a task cannot weaken them. Retired keys load and are dropped by reconcile. (`d7762eac`, `7ccd0b99`)

## Interfaces the rebuild must keep

`Deps` and its seams (all told, nil meaning absent), `NewFinalize`, `NewParentAlign`, `PRReview`, `ResolvePullRequest`, `HandsBack`, `EscalateHalt`, `AcceptFixReview`, `DraftBeamDescription`, `RecordBeamDescription`, `OriginMissing`, the landing and escalate prefixes, and the attempt, merge and pull-request record shapes.
