---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:51
    summary: D2 can inherit a whole-green base from an incompatible or filtered run
    fix: In D2, require an ancestor base to have a whole-green entry for the current module and tier fingerprint with compatible unfiltered test args. Explicitly exclude guard-only and user-filtered passes from whole-suite base eligibility, and fall back to the parent merge base or whole run when no eligible entry exists. Add acceptance scenarios for a changed tier command and a filtered whole-target pass followed by an unfiltered diff request.
  - id: F2
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/decision-record.md:80
    summary: Exact args matching defeats the promised count=1 rerun override
    fix: In D4, specify that an explicit -count=1 request bypasses green-record reads on every invocation, rather than merely using a different args key. Update the acceptance criteria and the support log carry discussion to cover two consecutive identical -count=1 invocations on the same clean tree, both executing tests.
  - id: F3
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:110
    summary: D7 rejects valid filtered runs as missing-tier-tag failures
    fix: Amend D7 so zero execution is a missing-tag failure only when the effective test-selection arguments should select at least one host-eligible test for that tier. Explicitly allow zero execution when user filters exclude every eligible test, and add acceptance coverage for a correct tier command with a nonmatching -run filter alongside the missing-tag scenario.
---

### [BLOCKING] Ancestor inheritance lacks the green record's compatibility bounds

**Location:** _lyx/discussion/decision-record.md:51

**Issue:** D2 selects an ancestor using only ancestry and whole-green status for the tier. D4 stores module, fingerprint and args separately, but its same-tree carry bound does not state how these constrain D2's ancestor search. A whole-target run restricted to one test can therefore supply a base for an unfiltered diff request; likewise an old-command pass can supply a base after the tier command changes. When that base is HEAD, the diff is empty and tests outside the guard set can be skipped without ever passing under the requested conditions. The Decision introducing inherited green needs its own eligibility bound.

### [MEDIUM] The stated force mechanism only works until its first pass is cached

**Location:** _lyx/discussion/decision-record.md:80

**Issue:** D4 says nonempty args need an exact match and concludes that `-- -count=1` reruns. After the first such invocation passes, its entry contains exactly those args; a second identical invocation matches and carries. The support log's Turn 3 and rejected-alternatives section explicitly rely on this flag as the override in place of a new rerun flag. Exact args matching alone does not deliver that behavior.

### [MEDIUM] Zero execution does not establish a missing build tag

**Location:** _lyx/discussion/decision-record.md:110

**Issue:** D7 fails any package with a host-eligible tagged test but zero executed tests, and attributes that failure to the command lacking its tag. D3 preserves manual flags and D4 explicitly supports args after `--`; the existing CLI also advertises `-run` filtering in internal/gatecli/cli.go. A correctly tagged package containing only TestA legitimately executes zero tests under `-- -run '^TestB$'`. The proposed rule turns that valid request into a configuration failure. Its expected count must account for test selection.

## Rejected

- D4's clean-tree rule, end-of-run cleanliness check, same-tree fingerprint bound and local storage are stated; their mere existence does not warrant a finding. The ancestor eligibility omission is separately identified in F1.
- D1 bounds gate_key to selecting a hub-defined gate. D8 bounds entry gates to their own row, identifies goto as the operator bypass, and specifies resume and relaunch behavior. D10 confines the window force to one setting for one run and states the zero-budget Describe behavior.
- D6 states the replacement guard-marker restriction and runs out-of-scope guards under their own tier. D13 limits the transitional integration term to guard-marked files and assigns its removal to a dependent entry after deploy. The temporary exception is explicit.
- D13's first-card discovery check verifies the prescribed hub configuration was applied; it does not defer an unknown tool capability to Plan. D11 records its test2json evidence. D4's pruning policy leaves retention mechanics to implementation rather than asking downstream to research a factual premise.
- The rationale paragraphs explain the selected architecture and deliberate operator-view departures. The assumptions and risks carry operational constraints or consequences. No relocation is necessary merely because these sections exist; moving them wholesale would risk losing plan input.
- D13 files another board entry, which the rubric expressly permits. The Out list does not direct a write to this task's own entry.
