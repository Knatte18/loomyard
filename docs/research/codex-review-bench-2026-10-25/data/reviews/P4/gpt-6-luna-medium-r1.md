---
verdict: BLOCKING
findings:
  - id: F1
    severity: MEDIUM
    class: design
    location: _lyx/plan/03-board-owner-writes.md
    summary: OwnerRequest.Takeover lets any SetOwners caller bypass the effective-owner refusal, but the plan does not bound this escape hatch to lyx adopt.
    fix: In card 3 Intent, constrain Takeover to the adopt flow and specify that ordinary boardengine callers cannot set it; enforce that boundary in SetOwners rather than relying on callers to choose false.
---

### [MEDIUM] Takeover is not bounded to adopt

**Location:** _lyx/plan/03-board-owner-writes.md, OwnerRequest and SetOwners Intent

**Issue:** The plan says that `OwnerRequest` carries a `Takeover` flag and that setting it bypasses the rule that skips a slug owned by another machine. It then claims only `lyx adopt` moves an owner, but the exported `boardengine.SetOwners` API accepts this flag and the plan does not enforce or otherwise bound which callers may use it. Any future caller can therefore take ownership without the adopt flow's reconcile, local-commit decision, and pause handling. State and enforce the boundary for this bypass in the plan.
