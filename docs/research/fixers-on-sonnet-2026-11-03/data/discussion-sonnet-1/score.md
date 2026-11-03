| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | D1 step 3 passes no `Anchor` and skips the dirt check under the adopt-only option (add.go refusals at the `opts.Anchor != "" && weftOld.live` check and the `ErrSourceDirty` branch); the beam-anchor and dirty-prime criterion is added. |
| F2 | partial | The not-live step (restore the weft from its archive tag, then `--adopt-local`) works, since a local weft branch makes `probeWeftLeftover` answer live. But the `ErrRemoteLeftover` step only says "take the remedy its text names", and that text offers "use a different slug" or deleting origin's diverged weft, neither of which recovers the run's records. A not-live weft that has no archive tag gets no step at all. |
| F4 | fixed | D1's first paragraph now makes the tolerance a run-scope option that only the resume sets, classifies the entry from its origin record and the Bolt, and keeps today's error for `DropLocal` and loom's reconciles. The criterion covers both sides. |
| F5 | fixed | D2 gives a closed permanent list (not-live, records worktree present, `ErrRemoteLeftover`) and names the transient classes, `ErrOriginUnreadable` (new) tested with `errors.Is`, and a worktree-add transient criterion. |
| F6 | fixed | The rationale matches `childReachedParent` (cancel.go): with the child status gone, only Worktree-Teardown or pastMerge returns true. The warning is limited to rows before Land, and a no-warning-at-Land criterion is added. |
| F7 | fixed | D3 drops the appended cancel from Finalize's stop and leaves the record to the adopt. Fabric remove does not warn when an owner pause stands, and a criterion pins this. |
| F8 | fixed | D6 leases on the local tip when D4's push landed, else on the step-1 tip, and adds a criterion for a pushed-then-deleted removal. |
| F9 | fixed | D6 has the sweep read the origin tip under the lock, archive a diverged tip and lease on it. A diverged-orphan sweep criterion is added. |
| F10 | partial | Both risks are moved to Constraints with a code trace: step 11 is leased (add.go), and `checkOrigin` passes a branch with no upstream (pushclassified.go). But the record says the hub's run logs were not read, and it cites no git documentation or git version for the receive-pack claim, although the fix asked for both. |
| F11 | fixed | D1 step 3 adds that under adopt-only a local weft branch covered by an archive tag answers not-live, and a criterion covers it. |
| F12 | fixed | D4 step 3 fetches a diverged origin tip by SHA before tagging it, which matches `resolveArchiveTip`'s fetch by SHA (archive.go). A criterion is added. |
| F13 | fixed | D5 reads the tip and treats a commit as present when it equals the tip or `merge-base --is-ancestor` holds. An unknown tip is unsettled, and the criterion is extended. |
| F14 | fixed | D7's Bound states the 30 s wait (`StatusPushLockWait`) against about a minute of hold (archive delays 2+4+8+16+32 s), and says a refused hand add or discard names the re-run. |
| F15 | fixed | The first D7 criterion now has `Add` wait on an exclusive hold by `Remove`, the discard or the sweep. |
| F16 | fixed | D1's Bound names the end of the flag-only `AdoptLocal` rule (add.go `AddOptions.AdoptLocal` comment and the docs of both packages) and the rule that replaces it. |

## New defects

- LOW — D1 step 3: "passes no `Parent`" is new and its justification, "no fork point is read when nothing is forked", is false. With an empty `Parent`, `Add` still reads the acting worktree (the prime) as its fork point for the empty-marker and anchor-move checks. A live pair whose warp branch is on neither side would also fork that branch from the prime's HEAD, not from the run's parent.
- LOW — `## PATTERN changes`, `PATTERN-slug-remote-order`: the entry still says the delete "is leased on the origin tip read". That no longer matches D6's new lease, origin's tip after D4's branch push, which the Scope bullet was updated to.
- LOW — D3 heading and the second Scope bullet: both still say a way forward naming `lym fabric remove` on a batten slug names `lym batten cancel` before Land. But D3's body dropped the general rule and leaves Finalize's stop (reached before batten's Land) without the cancel.
- NIT — support-log.md Question ledger Q8 and Q5: Q8 still says the trigger is "recorded under Open risks", which is now `None.`, and Q5 still names cancel in the way forward, which D3 no longer does.

TALLY fixed=13 partial=2 open=0 new=4
