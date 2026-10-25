---
verdict: BLOCKING
findings:
  - id: F1
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:43
    summary: The proposed commit command does not commit the code checkout dirt
    fix: Replace the lyx fabric commit example in D2 with commands that actually stage and commit the named tracked paths in the parent code checkout, such as git add -- <paths> followed by git commit -m <message>. Keep the guarded goto and resume sequence. Record the CLI evidence that fabric commit commits the configured records pathspec, and require the recovery scenario to demonstrate that the parent code dirt is cleared before goto.
  - id: F2
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:75
    summary: Admitting an in-flight merge does not prevent the cleanup advice from targeting its staged paths
    fix: Amend D2 and D4 to give an in-progress merge precedence over the dirty-path cleanup remedy, including aggregated guard failures and a merge record appearing after the goto check. Require Finalize to preserve the other merge and report an in-progress remedy without recommending commit or discard of its staged paths. Add this concurrent-record scenario to Acceptance criteria and correct D4's claim that the existing guard alone guarantees the safe remedy.
  - id: F3
    severity: LOW
    class: design
    location: _lyx/discussion/decision-record.md:31
    summary: The directory-link premise lacks its supporting source in the decision record
    fix: Add a short evidence clause to D1 naming internal/fabricengine/junction.go and internal/fabricengine/junctionnames.go at the reviewed repository revision as the sources establishing the structural _lyx link into the records checkout. Connect that evidence to pairDirtyReason's code-only tracked-status read. Keep the resolved no-exclusion decision; no signatures, line citations or exhaustive dependency list are needed.
---

### [MEDIUM] Recovery commits the wrong checkout

**Location:** _lyx/discussion/decision-record.md:43

**Issue:** D2 explicitly promises a way to clear tracked dirt in the parent's code checkout, but `internal/fabriccli/weft_verbs.go` defines `lyx fabric commit` as a no-argument records commit using the configured pathspec. Its help says it stages the durable structural directory and optional configured additions. It does not stage an ordinary code path such as the support log's `docs/overview.md`. Following this example can succeed while leaving the exact dirt that D4 refuses. The named commands are not denied by the settings read; the defect is the commit command's behavior.

### [BLOCKING] The merge-record admission's safety claim is false

**Location:** _lyx/discussion/decision-record.md:75

**Issue:** D4 says Finalize's guard will report an in-progress merge without telling the operator to discard another run's staged merge. However, `internal/fabricengine/merge.go` aggregates `mergeInProgressReason` and `pairDirtyReason` in `Merge`; a recorded merge with tracked staged changes can therefore carry both reasons. Finalize's steps 6–7 select any guard error whose `WorktreeDirty()` is true, and D2 gives that arm the unconditional commit-or-discard remedy. A record appearing after Finalize's parent-record commit and before its parent-side `Merge` reaches precisely this case, even if the goto-time probe saw a clean parent. An existing record may instead stop earlier at `commitParentRecords`, whose commit refuses recorded merges, so the asserted downstream behavior is not established there either. The advisory timing bound preserves merge safety, but does not bound the destructive cleanup advice.

### [LOW] Record the evidence for the no-exclusion premise

**Location:** _lyx/discussion/decision-record.md:31

**Issue:** D1 names the dirty guard and parent-record commit as sources for those behaviors, but states the separate directory-link premise without naming its evidence. The support log repeats the assertion as an exploration result. `internal/fabricengine/junction.go` actually constructs the code-to-records junctions, and `internal/fabricengine/junctionnames.go` includes `_lyx` in the structural committed directories. The premise is supported by the source inspected in this review, but the rubric requires the load-bearing factual answer and its evidence to be recorded for Plan, which does not read the support log.

## Rejected

- D4's unreadable-parent admission and goto-time advisory bound are stated explicitly and retain Finalize's authoritative merge checks. Neither is an unbounded escape hatch.
- No goto bypass was established. `loomcli.specFor` installs the recipe guard; generic `lyx shed goto` arms loom and darn through `loomcli.ArmAt`; `shedverbs.RunGoto` passes it into `shedengine.Goto`. Tackler and arbiter validation and consume paths also receive the guard. D4 covers both Finalize and its direct routed-from row.
- The goto-then-resume sequence itself exists: Goto leaves the target paused, and the resume implementation wakes the parked driver or starts a Go driver for a halted Go-seeded run. The existing settled-guard message alone would be weak evidence, but the implementation supports the intended sequence for the normal halted run.
- D3 retains tolerant parsing explicitly, and Scope names the fabric CLI and task-side MergeIn surfaces that keep bare errors and lose the old parent section. No necessary constraint was found only in the support log.
- No relocation finding is warranted. The remaining rationale explains requirements or bounds the plan writer needs, and the optional notes carry actionable design hints rather than interview narrative.
