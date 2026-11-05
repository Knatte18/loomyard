---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/plan/00-overview.md:63
    summary: The retag cards use Custom despite an applicable Edit representation
    fix: In cards 24 through 32, change Custom to Edit, retaining the existing package self targets without adding member or file targets beside them, and add an ImpactSummary describing the affected packages and their test compilation boundaries. Rewrite retag-rules to remove the claim that header edits require Custom. Remove the Custom-based rationale from scan-after-last-retag; carry the D5 scan with the last retagging operation when restructuring that boundary through the prescribed card insertion/removal tools.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/plan/04-verify-gate-site-key.md:19
    summary: Two planned function result changes omit their re-sign arrows
    fix: Give internal/loomcli#darnTreeScope in card 4 the arrow head func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error). Give internal/gatecli#guardRunOf in card 14 the arrow head func guardRunOf(plan []tierRun) []tierRun, matching the intent to return every guard run. Keep these as changes to the existing member bullets.
---

### [BLOCKING] Retagging does not require the Custom escape hatch

**Location:** _lyx/plan/00-overview.md, Shared Decisions retag-rules and scan-after-last-retag; cards 24–32.

**Issue:** The stated reason for Custom is that build constraints have no member to name and `edit-test-file` refuses test file edits. The format explicitly provides a header-edit representation: its `edit-test-file` row says to name every member of the file. It also explicitly limits that check to test-file self glyphs, excluding extensionless package targets such as those these cards already use. Thus this is ordinary editing with an available representation, not work for which none of the seven other types fits. The number of files does not satisfy the rubric's last-resort condition. Retaining the existing package targets while changing the label avoids both test-file self targets and package/member redundancy. This also removes the stated justification for separating card 33 from the last retag card, whereas D5 requires the scan to land with that last operation.

### [MEDIUM] Signature changes are not declared consistently

**Location:** _lyx/plan/04-verify-gate-site-key.md:19 and _lyx/plan/14-guard-step-per-tier.md:24.

**Issue:** Card 4 changes the tree-scope reader into a keyed function and explicitly says `darnTreeScope` takes the key. The existing declaration at `internal/loomcli/darnwiring.go:76` returns `func() (string, bool, error)`, so the planned keyed closure changes its result type. Card 14 says `guardRunOf` returns every guard run, whereas `internal/gatecli/run.go:202` returns one `*tierRun`. Both member bullets omit the required re-sign arrow. These are ordinary functions, not the exempt interface methods or struct fields. This repeated signature-enumeration gap is reported together.

## Rejected

- Cards 24–32 do not prescribe raw test execution. Card 24's Verify inspects resolved configuration; the remaining retag cards rely on the gates. The command's occurrence of `go test` is an awk match, not an executed test command.
- Cards 1–6 introduce the configuration keys, then the flag, then its rendering and per-row wiring. Card 1 explicitly retains the old rendering until card 3. Retagging follows cards 11, 12, 14, 15 and 16.
- Card 23's shortened PATTERN lines are licensed by the Added after Discussion entries. Card 33's board follow-up is licensed by D13 and writes another entry, not this task's body or scope. Card 24 expressly checks for retained covers entries.
- The new one-tag scan enforces a different property from the existing tier-purity and LLM reachability guards; their shared repository traversal alone does not make it redundant.
- The focused flag, carry, empty-tier and entry-gate changes state their bounds and name reaching coverage. No research halt or own-board-entry edit was found in their intents.
