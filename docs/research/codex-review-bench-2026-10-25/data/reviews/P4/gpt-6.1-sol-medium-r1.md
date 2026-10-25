---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: scope
    location: _lyx/plan/05-reconcile-owner-outcome.md:36
    summary: Owner judgment excludes pairs without a prime-side batten run record
    fix: In card 5's Intent, derive owner candidates from all local pairs in the selected scope, including the run-scope slug even when it has no prime-side batten status. Pass their owner outcomes to the mover and emit owned_elsewhere entries for such pairs without broadening the existing ended-run teardown candidates. Extend the already-targeted TestReconcileOrigin_Moves with a pair owned elsewhere that has no prime-side batten record, covering both judge-only run scope and a moving hub reconcile. In card 9's existing TestRunOwnerVerdict description, include an existing task-worktree shed status with no prime-side batten status and require the other owner's verdict.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/plan/16-adopt-module.md:16
    summary: The adopt card bundles three separate exported module surfaces into its CLI implementation
    fix: Use lyx loom insert-card to introduce prerequisite cards before adopt-module for battencli.ResumeInWindow, fabricengine.DropLocal with DropLocalResult, and hubforge.CloneSecond. Move each surface's implementation intent and its associated coverage into its own prerequisite card; keep DropLocal's destroy.go header, mutation-result contract, destruction-guard registrations and TestDropLocal together. Keep adopt's private request, outcome, dependency and orchestration helpers with its CLI surface. Update the Card Index and adopt's references through the insertion workflow so the consumer follows all three prerequisites.
---

### [BLOCKING] Owner checks silently disappear without a batten record

**Location:** _lyx/plan/05-reconcile-owner-outcome.md:36

**Issue:** Card 5 explicitly reads owners only over the recorded pairs. In the current implementation, `recordedPairs` in `internal/fabricengine/originreconcilemoves.go` includes a pair only when `shedrun.StatusFile(prime, slug)` exists; `classifyEntries` likewise iterates that restricted list. A task worktree can have an existing loom/darn status without that separate prime-side batten record, and D5 explicitly permits owning a task without a batten run. With origin reachable and Bolt naming another owner, card 9's judge-only reconcile then returns no owned_elsewhere entry, so `runOwnerVerdict` permits the shed resume. The same omission leaves that pair outside card 5's no-replay map and card 16's report-based local-commit stop. D3's existing-shed resume guard and D4/D5's ownership rules do not have a batten-record prerequisite. This is one candidate-enumeration gap affecting all those consumers.

### [MEDIUM] Separate the exported prerequisites from the adopt CLI

**Location:** _lyx/plan/16-adopt-module.md:16

**Issue:** The card creates the adopt module while also introducing public APIs in battencli, fabricengine and hubforge. In particular, DropLocal is a destructive fabric primitive with its own mutation contract, guard registrations and integration test, independently reviewable from adopt's selection, ownership writes and resume flow. ResumeInWindow and CloneSecond are also exported across package boundaries. The rubric and the plan spec's Card granularity section require such public surfaces to have their own cards even when this CLI is their only initial consumer; these are not private supporting helpers inseparable from adopt's type.

## Rejected

- Card order: the owner reader, OwnerRequest, owner-pause label, PauseOwnedElsewhere and InstallOwnerPause are introduced before their named consumers. No additional ordering defect was established beyond checks already owned by the mechanical gate.
- Attack surface: Takeover explicitly permits adoption over another owner and is reserved to adopt; the Finalize carve-out names the parent-holds-work bound and InnerRun's status/restep limits. The nil seam rules describe their compatibility behavior and production wiring. No separate missing-bound finding is warranted.
- Guard reachability: the prescribed tests name owned-elsewhere inputs, unreachable-origin inputs, pause transitions and seam failures that reach the intended branches. The missing owner candidate in F1 is the concrete exception; no additional upstream rejection was established.
- Fidelity and PATTERN: cards 15 and 16 implement D8 and D5. Card 17's background text, shortening of the overview, byte-neutral edits and trailer are licensed by the added-after-Discussion decision. No own-board-entry edit or unsupported scope addition was established.
- Test economy and signatures: card 7 extends existing tests; cards 10 and 14 cover new owner behavior, and card 16 adds takeover and destructive-operation coverage. No existing test was found to cover those same owner behaviors already. The signature changes reviewed do not establish another missing re-sign arrow, and private supporting types remain with their surfaces.

Review used file reads only. No target files were changed, and no tests or plan validator were run. F1 changes intent and existing coverage descriptions only; F2 requires card insertion, which the rubric exempts from overlay validation.
