---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:100
    summary: Failure of a later amendment deletes earlier pending amendments
    fix: In D7, replace unconditional amendment-record removal on append or commit failure with restoration of the pre-call record, including its pending titles and write time; remove the file only when none existed before the call. State that the failed call also restores its decision-record append, consistently with the promised all-or-nothing outcome. Add acceptance scenarios where amendment A succeeds and amendment B fails during append or commit, preserving A and its routing eligibility while leaving no B entry.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:101
    summary: Whole-second timestamps cannot order an amendment after a settle in the same second
    fix: Replace D7's whole-second ordering with an explicit causal identity. Record the current Plan-Review settle generation and round with each pending amendment, admit it against that settle, and consume it only after a later review round has judged that amendment. Retain timestamps for diagnostics. Update the acceptance criteria to cover a settle, amendment write and subsequent judged round occurring within one wall-clock second, plus refusal to reuse an amendment against a later settled round.
  - id: F3
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:80
    summary: The wrong-label bound incorrectly guarantees detection by merge or verification
    fix: Rewrite D4's bound to name D6's plan-review check as the present protection against an incorrect Extends label, retain the current sequential-execution bound, and state that a missed label can produce a clean but semantically wrong merge under future parallel execution. Remove the never-silently guarantee and record that residual risk under Open risks. Add an acceptance criterion that plan review flags a card labelled Extends whose intent rewrites, reorders or removes existing content.
---

### [BLOCKING] A failed second amendment erases the first amendment's route

**Location:** _lyx/discussion/decision-record.md:100

**Issue:** D7 explicitly removes the amendment record when an append or commit fails, but the next sentence makes that record an accumulator of all pending amendments. After a successful amendment A, a failed amendment B therefore removes A's pending title and write time as well. A remains committed in the decision record, but the settled-segment guard has lost the admission needed to send it to the fixer. This silently loses an already accepted amendment from the owned route. The acceptance criterion that a failed commit leaves neither also needs to distinguish the failing call from previously committed amendments. The existing verb's commit-failure path in `internal/loomcli/decision.go` leaves the decision append in place, so the promised transaction must explicitly cover that restoration too.

### [MEDIUM] Same-second events make the amendment route refuse valid work

**Location:** _lyx/discussion/decision-record.md:101

**Issue:** D7 stores whole-second UTC and requires the amendment to be strictly after the settle. The existing Bouncer also stores `SettledAt` and `JudgeAt` with `time.RFC3339` in `internal/shedadapters/bouncer.go`. A settle followed by an amendment within the same second compares equal, so the new admission refuses a legitimately later amendment. Retrying the goto cannot change that stored ordering. The consumption rule has the same ambiguity if the judged round follows the amendment in that second. Simply admitting equality would instead admit events that preceded a settle in the same second; a causal round identity avoids both errors.

### [MEDIUM] A clean merge and passing verification do not prove an additive label was sound

**Location:** _lyx/discussion/decision-record.md:80

**Issue:** D4 says an incorrectly labelled dependency will surface as a merge conflict or verify failure, never silently. Two cards can change different lines of a shared function or document, merge cleanly, and leave an inconsistent behavior or statement that verification does not exercise. D6 supplies a useful review check, but neither that judgment nor merged-tree verification guarantees detection. The attack-surface bound must identify the actual protection and its residual risk rather than rest on this false guarantee. This does not require building parallel batches in this task.

## Rejected

- D7/D8 do state bounds on the new routing edge: only Plan-Review is reopened, existing review budget remains spent, done cards remain guarded, and the arbiter hands on scope widening or an unresolved operator choice. No additional finding asserts that this route is wholly unbounded.
- D1 and D2 agree that fenced entry lines count as prescriptions. D2 explicitly identifies its changed-bullet exemption and the remaining per-card and verify guards for undeclared prose edits; those escapes are stated rather than omitted.
- D3/D4 give an outcome for Extends against Edit, Uses, Create and deletion labels. No separate pair-enumeration finding is warranted.
- D9 specifies the location-to-package mapping, the matching rule and the pinned quarry API source/version. It does not instruct Plan or the implementer to research those facts later. Exact signatures and exhaustive call-site lists are excluded by writer/reviewer symmetry.
- D5's estimate explains the decision to order measurement before implementation without gating the label build. That is decision rationale. The Open risks retain operational constraints or limitations the plan writer needs; relocation would not improve the actionable record without preserving those same requirements.
