# Fixer report

All edits are in `artifact/decision-record.md`, plus three line edits in `artifact/support-log.md` that keep it consistent with the record.
I ran no gate or validator.

## Changed

- F1: D1 step 3 passes no Anchor and states that the adopt-only option skips the source-dirt check. Added two D1 criteria: a beam project's resume re-adopts at its recorded anchor, and so does one with prime dirt.
- F2: D2's Land way forward now names a step per cause, then `lym batten run`. A not-live answer restores the weft branch from the archive tag with `git branch`. A records-worktree-present refusal removes the records worktree directory. ErrRemoteLeftover follows the step the refusal names. It also says a failed status resumes from its persisted producer, which `internal/battencli/run_test.go` (case "failed") confirms. The D2 Land criterion now requires that following the steps resumes Land with the child's records present.
- F4: D1's first paragraph makes the tolerance a run-scope option that only batten's resume sets. Every other run-scope caller keeps today's error. Reworded the absent-pair criterion and added an "error stands without the option" clause.
- F5: D2 lists the permanent and transient classes. It adds a transport sentinel (new) that `Add` wraps probe and transport failures in, tested with `errors.Is`. Any unlisted refusal is transient. Added a criterion for a transient local refusal leaving the status `running`.
- F6: D3 is restricted to a record standing before Land. The rationale is corrected for `childReachedParent`. Added a criterion that a record at Land gets no cancel from `fabric remove`'s warning.
- F7: Took the first option of the finding's fix. Finalize's stop is left unchanged, with the reason in D3. Criterion replaced.
- F8: D6's lease is origin's weft tip after D4's branch push, and D4 archives exactly that tip. Added a criterion for a removal whose weft tip origin lacked.
- F9: D6 extends D4's archive rule to the cleanup sweep, with a leased delete on the origin tip. Added a sweep criterion for a diverged origin tip.
- F10: Researched both from the code and moved the results to Constraints, and Open risks now reads "None." For 2026-11-03, the push check and the reconcile read a branch with an upstream that origin lacks as gone. Only two deletes can remove that branch, and D6 and D7 cover both. For "missing necessary objects", I recorded receive-pack's connectivity check and the cause D4 covers. Updated two support-log lines that pointed at Open risks.
- F11: D1's bound now says a local weft branch a removal left behind reads live and is re-adopted. Added a criterion.
- F12: D4 step 3 fetches the diverged origin tip by SHA before tagging it.
- F13: D5's rule reads the remote tip and treats an equal or ancestor commit as present and an unsettled one as never absent. Extended the criterion, marked Review.
- F14: D7's Bound states the hold, about a minute, and that a refused hand add or discard names the re-run.
- F15: Replaced the first D7 criterion with an Add that waits while Remove, the discard or the sweep holds the lock.
- F16: D1's Bound names the rule it ends, with the three doc paths, and its replacement.

## Fixed differently

- F7: the finding offered two options and I chose the first. Finalize's stop drops the appended cancel instead of keeping it and dropping the "leaving the owner pause standing" text.
- F2: its "new adopt-only add option over an archived weft" alternative was not taken. The restore through a local weft branch needs no new code, and F11's bound covers it.

## Disputed

None.

## Deferred

None. One gap in F10: I could not read this hub's run logs, so the 2026-11-03 trigger is a code-derived trace and no log confirms it. I also did not check the git version in use, so the receive-pack account is stated from behaviour and not cited to a version.
