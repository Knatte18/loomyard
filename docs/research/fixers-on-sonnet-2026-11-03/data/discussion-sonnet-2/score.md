| finding | verdict | evidence |
|---|---|---|
| F1 | fixed | D1 step 3 now passes no Anchor and skips the source-dirt check under the adopt-only option (matches add.go's anchor-on-live-weft and ErrSourceDirty refusals); the beam/dirty-prime criterion is added. |
| F2 | partial | The Land way forward drops `--adopt-local` and names a per-refusal step, the failed-status resume and the records-present criterion. The restore step needs "the archive tag the stop names", but after a normal teardown origin's weft is deleted, so probeWeftLeftover returns the zero answer with no tag. The record never says how the stop finds the tag, or which of a reused slug's several archive/<slug>/* tags to pick. |
| F4 | fixed | D1 now makes the absence tolerance a resume-only run-scope option that classifies the entry from its origin record and the Bolt; DropLocal and loom's reconciles keep originTargets' error, and a criterion covers both cases. |
| F5 | fixed | D2 lists the permanent and transient classes, adds a transport sentinel (new) tested with errors.Is, and adds a transient-local-refusal criterion. |
| F6 | fixed | The rationale now matches childReachedParent (true only at Worktree-Teardown or pastMerge when the child status is gone). The cancel and the warning are restricted to records before Land, Land points at D2, and a criterion is added. |
| F7 | fixed | The Finalize stop keeps its text, with no cancel (finalize_owner.go still says to leave the owner pause standing). The reason is stated, and the criterion is changed. |
| F8 | partial | The lease is now "the local tip when that push landed or origin already held it". When origin is ahead of the local tip, origin already holds it but origin's tip is not the local tip, so the remover's own lease fails and the branch is kept. Also, "D4 archives exactly that tip" is false when the push fails and origin is behind: D4 then tags only the local tip. |
| F9 | fixed | D6 has the sweep read origin's tip under the lock, archive it when diverged (fetched by SHA) and lease on it; the sweep criterion is added. |
| F10 | partial | Both Open risks lines are dropped and the traces moved to Constraints, but no evidence is cited: no git docs or version for receive-pack's connectivity check, and no run logs. "So the new pair's weft branch was deleted on origin after its first push" is stated as a traced fact. |
| F11 | fixed | The D1 bound now says a leftover local weft branch is re-adopted whatever tag covers it (matches probeWeftLeftover reading tags only with no local branch), and a criterion is added. |
| F12 | fixed | D4 step 3 now fetches the diverged origin tip by SHA first, as resolveArchiveTip does. |
| F13 | fixed | D5 reads the remote tip, treats equal or ancestor (merge-base --is-ancestor) as present and anything else as unsettled, never absent; the criterion is extended and opens with `Review:`. |
| F14 | fixed | D7 Bound states about a minute of hold against the teardown wait, and that a hand add or discard refused this way names the re-run. |
| F15 | fixed | The first D7 criterion now has Add wait on Remove, the discard or the sweep holding the lock exclusive. |
| F16 | fixed | D1 now names the end of the flag-only AdoptLocal rule in add.go, fabricengine/doc.go and fabriccli/doc.go (all exist at b194fbc4) and the rule that replaces it. |

## New defects

- LOW — decision-record.md D3, first two sentences: the rule that every stop naming `lym fabric remove` for a record standing before Land also names the cancel conflicts with the next sentence. That sentence keeps Finalize's stop, which names `lym fabric remove` while batten stands at Run-Shed, before Land, free of the cancel, and the rule names no exception for it.
- NIT — support-log.md Rejected alternatives: "Leasing the weft delete on the local tip" is still listed as rejected, while the new D6 leases on the local tip after D4's push.

TALLY fixed=12 partial=3 open=0 new=2
