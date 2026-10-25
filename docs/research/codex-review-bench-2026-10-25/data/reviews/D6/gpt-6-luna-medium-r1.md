---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: D5
    summary: D5 makes the run-lock holder's network-gate state a prerequisite for ending it but does not say how that state is determined.
    fix: Research how the engine can reliably identify a step waiting at the network gate, then record the observable signal and its evidence in D5; specify what happens when the state cannot be established.
  - id: F2
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: D8
    summary: D8 relies on detecting an unregistered lyx process whose working directory is a deleted directory inside the hub without establishing how those facts are read.
    fix: Research the process and filesystem readings available for identifying a deleted in-hub working directory, then state the detection method and evidence in D8; also state how the no-registered-linked-task-worktree and batten-record conditions are read.
  - id: F3
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: Constraints, no lyx run process handles SIGTERM
    summary: The claim that no lyx run process handles SIGTERM supports D4's immediate-exit and no-stop-notice behavior but has no source or evidence.
    fix: Verify the SIGTERM behavior of the affected lyx run processes and record the source or code evidence in Constraints, including the relevant version where applicable.
  - id: F4
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: D10
    summary: The sixty-minute bound is justified by the unsupported claim that it covers the longest fork seen.
    fix: Record the evidence for the longest observed fork duration that supports the sixty-minute bound, or revise the bound and rationale to an evidenced value.
  - id: F5
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/support-log.md: Rejected alternatives, process-table scan bullet
    summary: The fallback for finding unregistered orphans is cross-referenced as D7, but the process-table fallback is in D8.
    fix: Change the parenthetical reference from D7 to D8 in the process-table scan bullet.
---

### [BLOCKING] D5 leaves gate-state detection to downstream

**Location:** _lyx/discussion/decision-record.md: D5

**Issue:** D5 allows ending a run-lock holder only when it is a step waiting at the network gate, but does not specify how to determine that state. Plan or implementation would have to discover a design-critical fact later.

### [BLOCKING] D8 leaves orphan-detection readings unresolved

**Location:** _lyx/discussion/decision-record.md: D8

**Issue:** The orphan rule depends on whether a slug has a registered linked task worktree, whether its batten record reads running, and whether an unregistered process has a deleted working directory inside the hub. The record provides no method for reading these conditions, leaving research to downstream.

### [BLOCKING] D4's SIGTERM premise has no evidence

**Location:** _lyx/discussion/decision-record.md: Constraints, no lyx run process handles SIGTERM

**Issue:** D4 relies on this premise to say ended processes exit at once without writing a drive report or posting a stop notice. The constraint states it as fact but gives no evidence for it.

### [BLOCKING] D10's sixty-minute rationale is unsupported

**Location:** _lyx/discussion/decision-record.md: D10

**Issue:** The chosen bound rests on the claim that it covers the longest fork seen, but no evidence for that observation is recorded.

### [MEDIUM] Support-log cross-reference points to the wrong decision

**Location:** _lyx/discussion/support-log.md: Rejected alternatives, process-table scan bullet

**Issue:** D7 is the owned-elsewhere pause decision; the process-table fallback appears in D8, so the reference is inconsistent with the final numbering.
