---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: D4 steps 2 and 5
    summary: The cancel acquires and holds batten's run lock in step 2, then waits for the run lock again in step 5, so the stated sequence can deadlock on its own lock.
    fix: Clarify which lock step 5 waits for and ensure it is not batten's run lock acquired in step 2; revise the acquisition, release, and survivor rules in D3 and D4 so the sequence is executable without self-waiting.
  - id: F2
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: D2 unreachable-origin bullet and Bound
    summary: When the origin is unreachable the verb proceeds using local board ownership, which can be stale, so it can end a run owned by another machine despite the stated bound.
    fix: Define a verifiable ownership condition for proceeding while the origin is unreachable and refuse when ownership cannot be established, or change the behavior and explicitly bound the risk of ending another machine's run; make the matching D2 refusal and acceptance criterion consistent.
  - id: F3
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md: D4, D5, D8, and Open risks
    summary: Several design premises are left as risks or unsupported assertions for Plan or implementation to discover, including index.lock behavior, older-binary process registration, Lift behavior with no sessions, and removal mode leaving the claim to its caller.
    fix: Research each premise now and record the answer with source and version evidence in the relevant Decision or Constraint; resolve the index.lock and older-binary risks instead of handing their behavior downstream.
  - id: F4
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/support-log.md: Interview Turn 3 and Question ledger Q10
    summary: Interview Turn 3 records refusal on an unreachable origin as the pick, while D2 and Q10's revised answer proceed with a Warn, leaving the log's account of the final design misleading.
    fix: Amend Turn 3 to identify refusal as the initial pick and state that it was revised to proceed with a Warn, consistent with D2 and Q10.
---

### [BLOCKING] Cancel waits on a lock it already holds
**Location:** _lyx/discussion/decision-record.md: D4 steps 2 and 5
**Issue:** D3 says the cancel holds batten's driver, run, and status locks from step 4 through step 8. D4 step 2 takes those locks, then step 5 waits for the run lock after the end. As written, the cancel can wait on its own held run lock, preventing it from reaching the release in step 8.

### [BLOCKING] Unreachable origin does not establish ownership
**Location:** _lyx/discussion/decision-record.md: D2 unreachable-origin bullet and Bound
**Issue:** The verb continues when origin is unreachable and relies on local board ownership, which may be stale. That permits ending a run owned by another machine, contrary to D2's stated bound that the verb cannot end work owned elsewhere.

### [MEDIUM] Research unresolved factual premises
**Location:** _lyx/discussion/decision-record.md: D4, D5, D8, and Open risks
**Issue:** The record leaves index.lock behavior and older-binary process registration as open risks. It also asserts that Lift sends no resume line when no sessions remain and that the removal mode leaves the claim to its caller without recording the research and evidence behind those facts. The discussion stencil requires factual premises the design rests on to be settled here with source and version evidence, not left for Plan or an implementer.

### [MEDIUM] Interview log conflicts with the final origin decision
**Location:** _lyx/discussion/support-log.md: Interview Turn 3 and Question ledger Q10
**Issue:** Interview Turn 3 says the pick was to refuse the whole verb when origin is unreachable. D2 and the revised Q10 instead say the verb proceeds with a Warn. The support log therefore misstates the final design.
