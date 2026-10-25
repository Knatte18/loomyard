---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:102
    summary: The amendment admission has no bound on repeated arbiter re-entry
    fix: In D7 and D8, bound arbiter-authored admissions to one amendment review per Plan-Review settle episode and require a parent resume before another arbiter amendment can reopen the row; specify how the admission state is consumed and reset.
---

### [BLOCKING] Arbiter amendments can repeatedly reopen Plan-Review

**Location:** `_lyx/discussion/decision-record.md:102`

**Issue:** D7 admits a goto whenever a fresh amendment record is newer than the latest Plan-Review settle, and removes the record after a later judged round. D8 lets the arbiter create another such record and resume to the target. The stated bounds constrain each route to Plan-Review and require the plan gate and Plan-Bouncer judgment, but nothing limits repeated arbiter-created records after successive settles. The arbiter can therefore reopen the review loop repeatedly without a parent resume, leaving the new routing edge unbounded in frequency.
