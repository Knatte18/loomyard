---
verdict: BLOCKING
findings:
  - id: F1
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md: D4
    summary: The green-record pruning policy does not define which recent whole-green commits are retained, although D2 depends on the newest retained ancestor as its diff base.
    fix: Specify in D4 an objective retention rule for whole-green commits that guarantees the intended ancestor search, and align D2's base selection and the acceptance criteria with that rule.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md: D10
    summary: The --window override introduces a routing edge without stating what it can skip or the bound on that effect in the introducing Decision.
    fix: State in D10 that --window only forces the batten.open_window lookup for the current run, allowing only Open-Window's entry gate to pass, and does not affect Seed-IDE or Open-IDE or any other row.
  - id: F3
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md: D13
    summary: D13 relies on a later first-retagging-card check to learn whether the installed gate discovered the retagged packages' tests, leaving a design-critical fact to the implementation run.
    fix: Resolve before planning, with evidence from the installed lyx version and its gate behavior, whether the proposed transitional build constraints and hub commands make those tests discoverable; record the answer and change D13's transition accordingly rather than delegating discovery to the first retagging card.
---

### [MEDIUM] Define whole-green pruning

**Location:** _lyx/discussion/decision-record.md: D4
**Issue:** D4 says pruning keeps each tier's recent whole-green commits but gives no definition of recent or retention bound. D2 then relies on the newest ancestor whole-green commit to choose a diff base. Without a retention rule, the retained records may omit the ancestor D2 expects and change the scope from the specified base.

### [MEDIUM] Bound the window override

**Location:** _lyx/discussion/decision-record.md: D10
**Issue:** D10 makes `--window` a forced setting value and thereby lets the Open-Window gate pass when the setting is off, but does not state in that Decision the exact row the override can route through or its scope. The general bound in D8 does not describe this override's effect.

### [BLOCKING] Resolve the transition's discovered-test premise

**Location:** _lyx/discussion/decision-record.md: D13
**Issue:** D13 says the installed lyx lacks D7 and makes a first retagging card's discovered-test count the only catch for vacuous passes. Whether the installed gate discovers the tests under the transitional constraints and hub commands is a fact on which this transition rests. Deferring its resolution to the implementation card violates the requirement to settle factual design premises before Plan.
