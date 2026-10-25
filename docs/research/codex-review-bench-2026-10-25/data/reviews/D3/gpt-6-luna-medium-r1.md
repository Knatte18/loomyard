---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: Open risks
    summary: The MTP unmatched-filter behavior is left as an unresolved risk instead of being researched and recorded as a design fact.
    fix: Research the relevant Microsoft.Testing.Platform and dotnet test behavior for an unmatched filter at the supported versions, then replace the Open risks entry with the result and its source/version evidence in Decisions or Constraints.
  - id: F2
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: Auto-mode assumptions
    summary: The VSTest behavior for a gated project that skips discovery is treated as an assumption without recorded research evidence.
    fix: Verify the Models gated-project behavior and its VSTest exit code at the relevant versions, then record the result with source/version evidence in Decisions or Constraints and remove it from Auto-mode assumptions.
  - id: F3
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md: Decision 8
    summary: The carry false guarantee has an explicit-target exception but gives no bound on how explicit targets can narrow or skip the timing work.
    fix: State in Decision 8 what explicit-target invocations can omit and how that exception is bounded, or remove the exception so carry false runs the tier whole for explicit-target scopes too.
---

### [BLOCKING] Research unmatched-filter behavior before accepting MTP risk

**Location:** _lyx/discussion/decision-record.md: Open risks

**Issue:** The entry says MTP may fail when a narrowed filter matches no tests and leaves that behavior as an open risk. The discussion stencil requires factual questions the design rests on to be settled during Discussion, with evidence named by source and version. The support log records VSTest filter grammar research, but no research of unmatched-filter behavior under MTP. This matters to whether a narrowed test project can fail merely because it has no matching test.

### [BLOCKING] Verify gated-project VSTest exit behavior

**Location:** _lyx/discussion/decision-record.md: Auto-mode assumptions

**Issue:** The assumption says Models gated projects skip discovery and exit 0 under VSTest, and Decision 4 relies on this to characterize narrowed runs. The record supplies no evidence/version for this fact, and the support log's research summary does not cover it. The stencil requires this kind of fact to be researched and recorded rather than left as an assumption.

### [MEDIUM] Bound the explicit-target exception to carry false

**Location:** _lyx/discussion/decision-record.md: Decision 8

**Issue:** Decision 8 says a carry-false tier runs whole for every scope form except explicit targets. For the timing tier, that exception allows an explicit-target invocation to avoid the whole-work guarantee, but the Decision does not say what it can skip or how that path is bounded. The stated bound only concerns raw builds outside lyx gate test.
