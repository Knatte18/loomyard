| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | D1 step 3 passes no `Anchor` and the adopt-only option skips the source-dirt check (add.go:239-246, 397-401); the beam/dirt acceptance criterion is added. |
| F2 | partial | Land way forward now goes by refusal class, and the criterion requires that the records are back. But the main post-removal case (origin weft branch deleted, only `archive/<slug>/*` tags left, which `probeWeftLeftover` never reads when tip=="") is described as "an archive tag covering its tip on origin". With a reused slug's several tags, nothing says which tag `git branch` restores from. |
| F4 | fixed | D1 para 1 makes the tolerance an option that only the resume sets; `originTargets` (originreconcile.go:633), `DropLocal` and loom's reconciles keep the error, and the criterion covers both cases. |
| F5 | fixed | D2 lists the three permanent classes (not-live sentinel, records-dir sentinel, `ErrRemoteLeftover`) and the transient ones, wraps probes in a probe sentinel, and adds a worktree-add-failure criterion. |
| F6 | fixed | The rationale now matches `childReachedParent` (cancel.go:691-708, false with !childFound at Land); the cancel and the warning are limited to before Land, and a Land criterion is added. |
| F7 | fixed | The Finalize stop names no cancel and keeps the owner pause, citing `liftSlugPauses` and `cancelOwnedElsewhere`; the criterion is changed to match. |
| F8 | fixed | The D6 lease is origin's tip after D4's push (local tip if it landed, else the step-1 tip); "or at the local tip it is an ancestor of" matches Add's descend-cover rule. A D6 criterion is added. |
| F9 | fixed | The sweep reads origin's tip under the lock, fetches and archives a diverged tip, and leases the delete on it; a sweep criterion is added (cleanup.go:243-258 archives only via `archiveWeftTip` today). |
| F10 | fixed | Both risks are researched into Constraints and Open risks reads None. Claims checked: originTargets/judgeBranch gone rule, checkOrigin upstream-only, Add step 11 leased (add.go:628-641), git 2.53.0, no gone lines in hub logs. |
| F11 | fixed | Under the option, a local weft branch whose tip an archive tag covers answers not-live; the bound says "locally or on origin" and a criterion is added. |
| F12 | fixed | D4 step 3 fetches the diverged origin tip by SHA before tagging it (`tagAndPushArchiveTip` runs `git tag` locally, archive.go:120). |
| F13 | fixed | D5 uses ls-remote tip plus `merge-base --is-ancestor`, and an unknown tip reads as unsettled, never absent; the criterion is extended. |
| F14 | fixed | D7's bound states the hold of about a minute (`archiveTagPushDelays` 2+4+8+16+32 s plus the branch push) past the teardown wait, and that a refused hand add or discard names the re-run. |
| F15 | fixed | The first D7 criterion now has Add wait on an exclusive hold by Remove, the discard or the sweep. |
| F16 | fixed | D1's bound names the end of the flag-only `AdoptLocal` rule (add.go:50, both doc.go files) and the rule that replaces it. |

## New defects

- LOW — decision-record.md D2, the line after the Land way forward: the record cites `haltedOverDoneChild` in `internal/battencli/arm.go` as evidence that `lym batten run` resumes a failed status at its row. That function only builds a status-envelope flag when the child's status is read as done. With the task worktree gone, `readChildStatus` returns found=false and no flag is built. The resume behaviour lives in shedengine's run loop (run.go step 3b), so the citation is wrong evidence.
- LOW — decision-record.md D2, Land way forward, the "Not-live with no weft branch and no archive tag" bullet: the failed stop names no way forward (against `PATTERN-refusal-way-forward`, which F2 invoked). It hands the run to the board note `landed-run-left-paused`, but that note covers paused landed runs, not a `failed` batten status.

TALLY fixed=14 partial=1 open=0 new=2
