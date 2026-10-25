---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: Decisions — D1
    summary: D1 treats the linked _lyx layout as settled without recording evidence for the premise that run records are invisible to code-checkout git status
    fix: Add to D1 the concrete evidence establishing that the code checkout's _lyx is a directory link into the records checkout and that git status from the code checkout does not report _lyx/shed paths; cite the repository source or test that establishes each fact, so Plan does not have to discover whether this foundational premise holds.
---

### [BLOCKING] D1 leaves its foundational checkout-layout fact unevidenced

**Location:** _lyx/discussion/decision-record.md: Decisions — D1

**Issue:** D1 relies on the claim that the code checkout's `_lyx` is a directory link into the records checkout, and concludes from it that `_lyx/shed/**` cannot appear in the code checkout's `git status`. The decision records the conclusion but gives no source, version, or test evidence for either part of that premise. The support log's Turn 1 repeats the claim as a traced fact but likewise names no evidence. Since the no-change decision and acceptance criterion depend on this behavior, the plan writer is left to find out a fact the discussion process was required to settle.
