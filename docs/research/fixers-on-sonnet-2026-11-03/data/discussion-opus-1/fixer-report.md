# Fixer report — discussion-opus-1

I checked every finding's premise against the code at b194fbc4. Every premise held, and every finding is fixed in `artifact/decision-record.md`. The support log's Rejected alternatives and Question ledger (Q11–Q13) and `artifact/relevant.md` were updated to match.

## Changes

- F1 (fixed as proposed): D1 step 3 now passes no anchor. Under the adopt-only option, `Add` skips the source-dirt check, citing add.go:239-246 and 397-401. Added D1 criteria for a beam project's run and for a prime with tracked dirt.
- F2 (fixed as proposed): D2's Land way forward is now set by refusal class:
  - Not-live with archive tags: a new `lym fabric add <slug> --adopt-local --from-archive <tag>` option, with its refusals and bound stated. It makes the pair over the weft restored from the tag, which keeps the child's records.
  - Not-live with no archive tag: the error says no copy remains and names the hand finish.
  - Records-worktree-present: remove that worktree, then re-run.
  - `ErrRemoteLeftover`: its own way forward, then re-run.
  - D2 now states that `lym batten run` resumes a `failed` status at its persisted row (arm.go `checkRunStatus`). The Land criterion now requires that following the steps resumes Land with the child's records present.
- F4 (fixed as proposed): D1's first paragraph rewritten. The tolerance is now a `ReconcileScope` option that only batten's resume sets. Under it the entry is classified from its origin record and the Bolt. `DropLocal` and loom's reconciles keep today's error. Added a criterion for a caller without the option.
- F5 (fixed as proposed): D2 lists `Add`'s refusal classes, sorted by sentinel or type with `errors.Is` and `errors.As`.
  - Permanent: not-live, records-worktree-present, `ErrRemoteLeftover`.
  - Transient: a probe or transport failure wrapped in a new sentinel, the D7 lock wait, a worktree-add failure, the anchor-move stop, and any other refusal.
  - Added a criterion that a worktree-add failure leaves the status `running`.
- F6 (fixed as proposed): D3's rationale corrected: `childReachedParent` returns true only at Worktree-Teardown or past the merge. D3's appended cancel and fabric remove's warning are limited to a record before Land. At Land they name D2's Land way forward. Added the Land criterion.
- F7 (fixed with the review's first option): the cancel is dropped from Finalize's stop, which keeps leaving the owner pause standing. The record is left to the `lym adopt` that clears the pause. When an owner pause stands, fabric remove's warning names `lym adopt` instead of cancel. Criteria were added for both. The other option is recorded under Rejected alternatives.
- F8 (fixed as proposed): D4 and D6 name the lease as origin's weft tip after D4's branch push: the local tip when the push landed, else the step-1 tip. D4 archives exactly that tip. Added the D6 criterion.
- F9 (fixed as proposed): D6's cleanup sweep reads origin's tip under the lock, archives it when it diverges (fetching by SHA), and leases the delete on it. Added the sweep criterion.
- F10 (fixed as proposed): I researched both risks and recorded the answers in Constraints, then dropped both Open risks lines (the section now reads "None.").
  - 2026-11-03:
    - The hub's trace logs show the 08:14:57 pause at Seed-Child and the 08:18:44 gone refusal for `shed-sub-run-block`, with nothing logged about the weft branch in between.
    - From the code, a gone verdict has two routes: a deletion on origin, which D6, D7 and step 11's lease cover, and a stale tracking ref. A stale ref arises because `DeleteRemoteBranch` treats an absent remote ref as success and leaves the local ref behind.
    - A new D8 makes `Add` drop a stale tracking ref of a branch it forks that origin lacks, with a bound and two criteria.
  - "missing necessary objects": cited git-receive-pack(1) and git-push(1) at git 2.53. The rejection needs a concurrent deletion of a ref the server advertised. Constraints now says D4's retry and D7 cover it and the branch push first does not.
- F11 (fixed as proposed): under the adopt-only option, a local weft branch whose tip an archive tag covers now answers not-live. The bound and a criterion were updated.
- F12 (fixed as proposed): D4 step 3 fetches the diverged origin tip by SHA before tagging, as `resolveArchiveTip` does. The D4 criterion was extended.
- F13 (fixed as proposed): D5 now counts a commit as present when it equals the remote tip or is an ancestor of it (`merge-base --is-ancestor`), and as unsettled otherwise, never absent. The criterion was extended.
- F14 (fixed as proposed): D7's Bound compares the hold D4 can reach (a branch push plus 62 s of tag-retry delays) with the 30 s `StatusPushLockWait`. It states that a hand add or discard refused this way names the running removal and the re-run, and that D1 reads the refusal as transient.
- F15 (fixed as proposed): the first D7 criterion was replaced. `Add` now waits while `Remove`, the discard or the sweep holds the lock exclusive.
- F16 (fixed as proposed): D1's Bound states that it ends the rule that only the `--adopt-local` flag sets `AdoptLocal`, and gives the rule that replaces it.

## Disputed

None.

## Deferred

None.
