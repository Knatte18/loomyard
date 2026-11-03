| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | D1 step 3 now passes no anchor and skips the `ErrSourceDirty` check under the adopt-only option (add.go:239-246, 397-401); beam-anchor and prime-dirt criteria added. |
| F2 | partial | The `--from-archive` option, the per-refusal Land ways forward, the `checkRunStatus` resume (arm.go:575, failed resumes) and the criterion are all added. But the records-worktree route says D1 "re-adopts the surviving local weft branch", while Remove archives the local tip before any teardown (remove.go:267-269), so D1's new local-cover rule answers not-live. That not-live way forward's `--from-archive` then refuses because the local weft branch exists, a dead end. |
| F4 | fixed | D1 now makes the tolerance a `ReconcileScope` option set only by the resume, classifies the entry without a pair worktree, and keeps the error for `DropLocal`, loomcli/ownercheck.go and loomcli/landingdeps.go, with a criterion added. |
| F5 | fixed | D2 lists the permanent sentinels and the transient sentinels and classes, with new sentinels where Add returns plain wraps; a worktree-add-failure criterion is added. |
| F6 | fixed | The rationale now matches `childReachedParent` (cancel.go:691-708). The cancel is restricted to before Land, Land names D2's way forward, and the Land criterion is added. |
| F7 | fixed | D3 drops the cancel from Finalize's stop (finalize_owner.go:160 is kept as is), citing `liftSlugPauses` and the owned-elsewhere refusal; criterion added. |
| F8 | fixed | D4 and D6 now name the lease as origin's tip after the branch push (the local tip when it landed, else the step-1 tip); the D6 push-then-delete criterion is added. |
| F9 | fixed | D6 now has the sweep read the origin tip under the lock, fetch and archive a diverged tip, and lease on it (cleanup.go:243-258 archived only the local tip); sweep criterion added. |
| F10 | fixed | Open risks is now `None.`. Constraints record the trace-log evidence (the 08:18:44 gone refusal is present in the hub's loomyard/.lym/logs), the two code routes (`originTargets`/`judgeBranch`, `checkOrigin`/`ErrBranchGone`, the step-11 lease at add.go:628-643), new D8, and the receive-pack analysis. |
| F11 | fixed | The adopt-only option now reads a local weft branch covered by an archive tag as not live; the bound says "locally or on origin"; criterion added. |
| F12 | fixed | D4 step 3 fetches the diverged origin tip by SHA, as `resolveArchiveTip` does (archive.go:140-175). |
| F13 | fixed | D5 reads the tip with ls-remote, accepts equal or `merge-base --is-ancestor`, and calls anything else unsettled, never absent; criterion extended. |
| F14 | fixed | D7's Bound states the 30 s wait (`StatusPushLockWait`, pushanchored.go:20) against 62 s of retry delays (archive.go:22) and that a refused hand add or discard names the re-run. |
| F15 | fixed | The D7 criterion now has Add wait on Remove, the discard or the sweep holding the lock exclusive. |
| F16 | fixed | D1's Bound now names the ended flag-only rule (fabriccli/doc.go:35, add.go:50) and the rule that replaces it. |

## New defects

- MEDIUM, D1 step 3 / D2 Land ways forward: these contradict each other, leaving a dead end. D1 now reads a surviving local weft branch whose tip an archive tag covers as not live. The records-worktree-present way forward claims D1 re-adopts such a branch, which Remove has always archived before teardown. The not-live way forward's `--from-archive` refuses an existing local weft branch, so a landed run whose local weft branch survived has no way forward that works.
- LOW, Constraints "missing necessary objects": the claimed sole cause, a concurrent deletion of the advertised weft branch, is unsupported. Deleting a ref does not remove its objects from the server's store before gc, so receive-pack's connectivity walk would still find them.
- LOW, D3 `lym fabric remove` warning: the cases cover only "before Land" and "at Land". A record standing at Worktree-Teardown, which the original rule covered as "not done", now gets no stated warning.
- NIT, D2 Land acceptance criterion: it names "the slug's last archive tag", while the Decision names the tag whose target descends from every other tag's target and has the operator pick among non-linear tags.
- NIT, D8: it says Add deletes the stale tracking ref "before its first mutation", but that deletion is itself Add's first mutation.

TALLY fixed=14 partial=1 open=0 new=5
