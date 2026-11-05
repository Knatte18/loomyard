# Fixer report — discussion-opus-2

All 15 findings were fixed in `artifact/decision-record.md`. `artifact/support-log.md` (Q8) and `artifact/relevant.md` got consistency edits. Every premise held against the code at b194fbc.

## Changes

- F1 (fixed as proposed): D1 step 3 now passes no `Anchor`, and the adopt-only option skips the source-dirt check. Added a D1 criterion: a beam-project resume and a resume with tracked dirt in the prime both re-adopt.
- F2 (fixed as proposed): D2's Land way forward now names a step for each permanent refusal class:
  - Not-live, local weft branch gone: restore it with `git branch <weft branch> <tag>`, then `lym fabric add --adopt-local`. A plain add reads a local weft branch as live and keeps its records.
  - Not-live, local weft branch present: the add alone.
  - Not-live with nothing left: no step; the entry goes to the board note `landed-run-left-paused`.
  - Records worktree present: `git worktree remove`.
  - `ErrRemoteLeftover`: archive origin's tip, then the delete its own text names.

  D2 now states that `lym batten run` resumes a failed status at the row it failed on (`haltedOverDoneChild`, `internal/battencli/arm.go`). The Land criterion now requires that following the steps resumes Land with the child's records present.
- F4 (fixed as proposed): D1's first paragraph now makes the tolerance a run-scope option that only batten's resume sets. Every other caller keeps the `originTargets` error. Step 1 drops "stopped", since no branch is judged. The absent-pair criterion now covers both with and without the option.
- F5 (fixed as proposed): D2 lists the permanent classes: the not-live sentinel, the records-worktree-present refusal (which gains a sentinel) and `ErrRemoteLeftover`. It lists the transient ones: probe and transport failures (wrapped in a new probe sentinel), the D7 lock wait, worktree-add failures, the anchor-move stop, and any other refusal. Added a criterion: a worktree-add failure leaves the status `running`.
- F6 (fixed as proposed): D3's rationale is corrected: `childReachedParent` is true only at Worktree-Teardown or past the parent-side merge. The appended cancel and the `fabric remove` warning now apply only before Land; at Land they name D2's Land way forward. Added a D3 criterion for Land.
- F7 (fixed as proposed, first option): Finalize's stop names no cancel and keeps the owner pause standing. The record is left to the adopt that clears the pause, after which D1/D2 apply. The D3 Finalize criterion was changed to match.
- F8 (fixed as proposed): D6's removal lease is origin's tip after D4's push: the local tip when the push landed, else the step-1 tip. One deviation: the record says D4 archives that tip "by a tag at it or at the local tip it is an ancestor of" instead of "exactly". When the branch push fails and the origin tip is an ancestor of the local tip, D4 tags only the local tip, and Add's coverage rule (`remoteleftover.go`) counts that as covering. Added a D6 criterion.
- F9 (fixed as proposed): D6 now has the sweep read origin's tip under the lock, fetch and archive it when it diverges, and lease the delete on it. Added a sweep criterion.
- F10 (fixed as proposed): both Open risks lines are gone; the section reads "None." Two Constraints bullets record the research:
  - **2026-11-03 trace:** a weft branch reads gone only when it has a tracking ref or upstream and origin lacks the branch (`originTargets`, `judgeBranch`, `checkOrigin`). A fresh weft has neither until its push lands, and `git push --delete` drops the tracking ref under the records repo's default `+refs/heads/*:refs/remotes/origin/*` refspec. So the trigger is a delete of origin's branch after the push. The only deleters are the removal, its origin-only path, the sweep and Add's step 11; D6 and D7 cover all of them. A grep of the hub's `.lym/logs` found no gone or `ErrBranchGone` line.
  - **Missing objects:** pinned to receive-pack's connectivity check and the quarantine environment (`git help receive-pack`, git 2.53.0). The record states which cause the branch-first push and the retry cover, and that a remote store fault is not covered.

  Support-log Q8 now points at Constraints.
- F11 (fixed as proposed): under the adopt-only option, a local weft branch whose tip an archive tag covers answers not-live. The D1 bound now says "locally or on origin". Added a D1 criterion.
- F12 (fixed as proposed): D4 step 3 fetches the diverged origin tip by SHA before tagging it.
- F13 (fixed as proposed): D5's rule now reads the remote tip with `git ls-remote`. A commit equal to the tip, or an ancestor of it by `git merge-base --is-ancestor`, counts as present; when the tip is not known locally the answer is unsettled, never absent. The D5 criterion was extended.
- F14 (fixed as proposed, second option): D7's bound states that the hold can reach one branch push plus the archive-tag retries, about a minute (`archiveTagPushDelays`), which is past the teardown wait. A hand add or a discard refused this way names the re-run, and D2 reads a batten pass's refusal as transient.
- F15 (fixed as proposed): the first D7 criterion was replaced with Add waiting while Remove, the discard or the sweep holds the lock exclusive.
- F16 (fixed as proposed): D1's bound now names the end of the flag-only `AdoptLocal` rule (`add.go`, `fabricengine/doc.go`, `fabriccli/doc.go`) and the rule that replaces it.

`relevant.md` gained `arm.go` `haltedOverDoneChild`, `pushclassified.go` `checkOrigin`, and the `liftSlugPauses` member on `cancelcmd.go`.

No gate or validator was run: `lym` commands are out of scope for this round.

## Disputed

None.

## Deferred

None.
