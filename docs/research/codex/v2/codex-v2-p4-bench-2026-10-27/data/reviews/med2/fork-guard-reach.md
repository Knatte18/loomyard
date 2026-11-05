Guard reachability list:

- Card 1 hostname refusal: reachable through `Name`/`FromHostname`; named table covers empty names.
- Cards 2–3 payload owner refusal: reachable after field allowlisting; replay bypass is intentional.
- Card 3 task takeover guard: reachable after the locked pull.
- Card 3 prime takeover guard: absent; a stale first-act claim can overwrite an existing prime owner.
- Cards 5–6 reconcile owner guard: reachable for recorded batten pairs; absent for existing loom runs without a prime-side batten status.
- Card 8 start, claim, resume, step and window refusals: reachable on uncovered, otherwise valid runs; its offline fallback is obstructed by the earlier prime claim when the prime is unowned locally.
- Card 9 existing-run refusal: reachable after `refuseCovered` for an uncovered run with batten records; the same check silently passes existing loom runs omitted by `recordedPairs`.
- Cards 10–11 sleep, hold-end, spawn-refusal and owner-pause lock-release branches: reachable through the named seams; Finalize’s running/transient carve-out has stated bounds.
- Card 12 ownership halts: reachable after the parent-lock guard with a stable parent; past-merge skip and error halts are specified.
- Card 13 wake pauses: reachable after successful wake reconciliation; a shared-branch stop retains the existing hub pause upstream.
- Card 13 orch pause: requires care with installation timing; `Command` is constructed before root pre-run, so a construction-time copy would be nil.
- Card 15 window reopening: reachable after a successful resume preflight.
- Card 16 adoption refusals: reachable with the named local owner, branch and live-pair states; the hub-pause exception is expressly bounded.

Findings:

1. **BLOCKING — decision — `_lyx/plan/05-reconcile-owner-outcome.md:36` — Ownership is checked only for batten-recorded pairs, leaving existing direct loom runs unchecked.**  
   The card explicitly supplies `recorded` pairs to the owner reader and classifier.
   `recordedPairs` includes a pair only when its prime-side batten status exists (`internal/fabricengine/originreconcilemoves.go:435–454`).
   Existing `loom start` independently seeds its own run through `seedAndCommitBootstrap` (`internal/loomcli/start.go:510–524`), without requiring a batten run.
   Therefore a local pair with an existing loom status and a foreign owner on Bolt, but no batten status at the prime, receives no owned-elsewhere entry.
   Card 9’s report-based check then allows its start/resume, and card 12 can likewise lack its run-owner verdict.
   This violates D3 and the declared `PATTERN-owner-machine`.
   The owner verdict must cover the requested run and relevant local pairs independently of whether they have batten bookkeeping; preserve the existing ended-record filtering separately.

2. **MEDIUM — decision — `_lyx/plan/03-board-owner-writes.md:47–48` — The non-takeover guard protects task slugs but not the prime claim.**  
   `SetOwners` is prescribed to write the prime whenever `Prime` is set, while its post-pull guard is stated only for a slug.
   Card 8 decides to claim from the local prime-owner read before resume reconciliation (`_lyx/plan/08-batten-owner-check.md:50`).
   A machine whose stale Bolt has no prime owner can therefore request a first claim; `PullThenCommitWritten` first brings in another machine’s established prime owner, then invokes the write (`internal/fabricengine/bolt.go:153–165`).
   The prescribed write overwrites it, and no competing replay is needed to make it lose.
   D2 requires the first claimant to stand, and D5 reserves explicit takeover for adopt.
   Apply the post-pull non-takeover rule to `Prime` as well, and test a stale ownerless local Bolt whose origin already has another prime owner.

3. **MEDIUM — decision — `_lyx/plan/08-batten-owner-check.md:50–52` — Claiming an absent prime owner before reconciliation prevents the promised offline legacy-run resume.**  
   For an older hub with no prime owner and an existing unowned run, the prescribed resume first claims the prime.
   Card 3 makes that setter return `ErrOwnerOriginUnreachable` on a failed pull (`_lyx/plan/03-board-owner-writes.md:49`).
   The existing locked pull returns its fetch-failed skip before invoking any write (`internal/fabricengine/bolt.go:153–160`, `207–214`).
   Consequently the later unreachable-reconcile branch that should warn and resume without writing cannot handle this input.
   D3 explicitly allows an unreachable-origin resume of an unowned run-held task with no write.
   Determine the unreachable outcome before requiring the first-act prime claim, and name a test combining no prime owner, an unowned existing run, and unreachable origin.

Calls made:

1. Read batten arm/pre-run/step/window and loom start/resume bodies to trace upstream checks and mutation order absent from the manifest.
2. Read InnerRun Call/check/wait, loom pause refusal, network gate, wake sequencing and orch start support to trace pause and guard reachability.
3. Located pause-network helpers and read board write/upsert boundaries to check earlier guards; two guessed paths did not exist.
4. Read Bolt pull/write and push semantics, batten claim ordering, network pause behavior and parent-guard references to establish real write sequencing.
5. Read pause-slug selection, locked pull implementation, recorded-pair filtering and run-scope target selection to distinguish entry omission from upstream refusal.
6. Searched direct loom-start tests and obtained precise plan locations for reconcile, resume and orch wiring; one guessed test filename did not exist.
7. Located direct bootstrap callers and obtained precise setter/reconcile/resume plan locations to substantiate the retained findings.
