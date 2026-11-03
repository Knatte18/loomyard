# Fixer report

All edits are in `artifact/decision-record.md`, plus a round line in `artifact/support-log.md`. Findings were checked against the code at b194fbc4.

## Changes

- F1: D1 step 3 now says the adopt-only Add passes no Anchor and no Parent and skips the source-dirt check. Added a D1 criterion for a beam project's re-adoption and one for tracked dirt in the prime.
- F2: D2's Land way forward is rewritten by cause. For not-live, it restores the weft branch from its archive tag, then runs `lym fabric add --adopt-local`. For a records worktree still standing and for ErrRemoteLeftover, it names the step that clears the refusal. It states that `lym batten run` re-enters a failed row. The D2 Land criterion now requires the child's records to be present after the steps.
- F4: D1's first paragraph now makes the tolerance a run-scope option that only batten's resume sets, with the entry classified from its origin record and the Bolt. Every other caller keeps today's error. Added a D1 criterion for both sides.
- F5: D2 now lists a closed permanent set (not-live sentinel, records-worktree-present, ErrRemoteLeftover). Every other refusal is transient, and `ErrOriginUnreadable` (new) wraps the probe and transport failures. Added a transient-local-refusal criterion.
- F6: D3's rationale is corrected to say `childReachedParent` is true only at Worktree-Teardown or past the merge. The warning and appended cancel are restricted to rows before Land, and D3 refers to D2's way forward at Land. Added a criterion.
- F7: I dropped the appended cancel from Finalize's stop and left that stop unchanged. D3's fabric-remove warning is also not given when the owner pause stands. Added criteria.
- F8, F9: D6's lease is now origin's weft tip after D4's push, and D4 archives that tip. The cleanup sweep archives a diverged origin tip and leases on it. Added D6 criteria for both.
- F10: Both Open risks are replaced by researched entries in Constraints, and Open risks now reads "None.". The Constraints entries trace every weft-branch origin-delete path and conclude D6 and D7 cover the 2026-11-03 trigger. They also state which causes of the "missing necessary objects" rejection D4 and D7 cover.
- F11: Under the adopt-only option, a local weft branch whose tip an archive tag covers also answers not-live. Added a criterion.
- F12: D4 step 3 fetches the diverged origin tip by its SHA before tagging. Added a criterion.
- F13: D5 treats the named commit as present when it equals the remote tip or is its ancestor, and as unsettled otherwise, never absent. The D5 criterion is extended.
- F14: D7's Bound states the roughly one-minute hold against the 30 s wait. A refused hand add or discard names the re-run, and the wait is not lengthened.
- F15: The first D7 criterion now has Add waiting while Remove, the discard or the sweep holds the lock exclusive.
- F16: D1's Bound names the rule it ends: only the `--adopt-local` flag setting AdoptLocal. The replacement rule is that the flag and the adopt-only resume are the only setters.

## Fixed differently

- F7: the review offered either dropping the cancel from the stop or dropping "leaving the owner pause standing". I took the first option.
- F2: the review left the not-live step open ("restore the weft from its archive tag, or a new adopt-only add option over an archived weft"). I chose the tag restore, which adds no new Add option.

## Disputed

None.

## Deferred

- F10, part of the research: I could not read this hub's run logs for 2026-11-03, because they are outside my read surface. The trigger trace and the receive-pack cause are from code, and from my own knowledge of git's connectivity check rather than the docs at the version in use. Constraints says the trace was not reproduced.
