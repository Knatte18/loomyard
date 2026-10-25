---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:78
    summary: The landed-work refusal can become stale while cancel waits to end the run
    fix: In D3 and D4, require the landed-work refusal to be re-evaluated after acquiring the records write locks and immediately before every End pass, with serialization that prevents a parent merge between that check and the end. Refuse without ending or clearing anything when the recheck finds landed work. Add an acceptance scenario where Finalize lands while cancel is waiting for the write lock.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:55
    summary: The offline escape hatch does not state the origin checks it bypasses
    fix: In D2, explicitly state that unreachable origin skips authoritative ownership and stopped-branch verification, that local ownership may be stale, and that the fallback requires a successful local ownership read and refuses a locally recorded foreign owner. Bound cancellation to the named local run and describe the deferred board and records publication through failed-push handling. Replace the unconditional claim that it cannot end a run owned elsewhere with the actual online and offline guarantees.
  - id: F3
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:68
    summary: A completed claim clear makes interrupted final cleanup impossible to resume
    fix: In D2 and D3, preserve a durable indication of unfinished cancellation through pause lifting and records-push scheduling, and let that indication bypass the nothing-left refusal on re-entry. Remove it only after those steps finish. Include interruption after step 7 with --remove and a live slug pause in the acceptance criteria, requiring the retry to lift the pause and schedule the records push.
  - id: F4
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:171
    summary: Tender reap cannot recover the older-binary processes the risk delegates to it
    fix: Replace the older-binary risk with the researched limits of tender reap from internal/tenderengine/processes.go and reap.go. In D4, specify a refusal before records removal and claim clearing when a positively identified unregistered process belonging to the slug survives; name manual termination and retry as its way forward. State the supported detection boundary instead of promising reap will handle arbitrary older processes, and add a kept-pair older-binary survivor scenario.
---

### [BLOCKING] Recheck landed work under the end's serialization

**Location:** `_lyx/discussion/decision-record.md:78`

**Issue:** D3 reads the landed refusal before D4 waits for the combined write locks. A running Finalize can merge into the parent during that interval, release the write lock, and then be killed by cancel using the earlier not-landed reading. D4 has no landed recheck after acquiring the locks. The subsequent path can remove its records and clear its claim despite D2's promise to refuse landed work with nothing changed. This is supported by `internal/battenshed/innerrun_owner.go`, where `FinalizePastMerge` reads the parent's current holdings, and `internal/fabricengine/merge.go`, where Merge serializes its mutation with the records write lock. Merely checking before waiting does not protect that boundary; D5's later removal refusal also cannot restore an ended landing.

### [MEDIUM] State the offline bypass and its actual bound

**Location:** `_lyx/discussion/decision-record.md:55`

**Issue:** The Warn path proceeds without an origin verdict, so it cannot discover a stopped branch or an ownership change absent from the local board. D2 names the local fallback but does not acknowledge these skipped protections, and its final bound still says a run owned elsewhere cannot be ended. `internal/battencli/resumereconcile.go` explicitly describes its analogous offline resume as proceeding without judging the run against origin. Referencing that precedent does not establish the stronger cancellation guarantee. The rubric requires the Decision introducing this bypass to state what it lets through and its bounds.

### [MEDIUM] Preserve re-entry through the final cancellation steps

**Location:** `_lyx/discussion/decision-record.md:68`

**Issue:** With `--remove`, after step 7 there can be no pair, records directory, run-form claim or registered live process. If cancellation is interrupted there, D2's nothing-left refusal rejects the retry before step 8 can lift the slug pause or spawn the records push. This directly contradicts the acceptance criterion that interruption after any completed step is recoverable. A live pause is not among D2's evidence that cancellation remains unfinished.

### [MEDIUM] The older-process recovery premise is false

**Location:** `_lyx/discussion/decision-record.md:171`

**Issue:** The risk leaves unregistered older processes to `lyx tender reap`, but `internal/tenderengine/processes.go` only includes unregistered processes with the executable's filename in a deleted working directory outside registered worktrees, and explicitly excludes deleted executables. `internal/tenderengine/reap.go` acts only on that orphan report. An older detached push in a kept pair therefore need not be reached by End, hold either checked driver/run lock, or qualify for reap. The cancel can report completion and clear its records while such a process remains alive. The claimed recovery is not merely an unresolved implementation detail; it rests on behavior the existing recovery verb does not provide.

## Rejected

- D3/D4 lock ordering is coherent when read with the existing teardown boundary: D4 step 2 acquires batten's prime-side per-slug locks; step 5 waits for the child's self-run lock. `internal/pairteardown/teardown.go` derives the latter from the task location and `shedrun.SelfRunID`. D3 can retain the former through claim clearing without waiting on its own lock. The survivor wording is broad, but the distinct contexts do not justify a deadlock finding.
- D1 bounds orch access through the prime guard and named slug. D4 explicitly permits ending live work and bounds the skipped quiet wait with process, strand and lock checks; F1 addresses the separate landed-work hole. D5 states forced removal's snapshot and landed-work bounds. D11 limits the hand-back route to the precondition prefix and typed dirty-source error. No additional generic attack-surface finding is warranted.
- D8's no-resume-line premise is supported by `internal/tenderengine/doc.go` and `resume.go`: a dead strand gets no line. D10 names the actual tracked-only probe, confirmed by `internal/fabricengine/dirtiness.go`. D5 prescribes a new claim-preserving removal mode rather than leaving its existence to downstream research; the current `RemovePair` implementation settles the claim and restricts snapshots to local mode, so implementing the specified mode must change that behavior. Those are design requirements, not unanswered research questions.
- The index.lock item states a residual risk for raw git outside the combined lock, rather than assigning a factual question to Plan. It does not warrant a deferred-research finding by itself. The older-binary recovery assertion is addressed separately in F4.
- Scope, Constraints, Auto-mode assumptions and the remaining risk prose are sufficiently distilled and carry exclusions, requirements or accepted limitations. No relocation would improve the record enough to justify a finding, and no required constraint should be moved out of the plan writer's input.
- Interview Turn 3 is historical. Turn 5 explicitly revises the offline pick, the rejected alternatives explain its rejection, and Q10 explicitly records the revision. Read as a turn-by-turn log, it does not misrepresent the final design.
