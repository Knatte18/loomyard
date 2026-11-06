```yaml
lens: code-affected-set
outcome: reported
```

Affected-set list:

- New `plan_order` cause: `BeginBatch`, `PlanConflictError.Error`, CLI envelope, conflict-record writer, shed rerouting, engine tests, package docs, and master stencil. Code consumers preserve or ignore the cause appropriately; retry instructions and external documentation are not fully covered.
- Delete-specific way forward: error rendering and new engine test rows are covered; the master’s record-batch paragraph no longer quotes the generic remedy.
- `CheckDeleteBeforeReference`: three finding producers, the dispatch classifier, and planglyph test readers use the constant. No changed wire value or uncovered production reader found.
- Plan-fixer bypass: initial master routing and plan-fix stencil header are covered; the retry branch and general Webster documentation retain unconditional routing.
- Current-tree check documentation: `planGatePass`, `callerCoverageFindings`, `ValidateDispatch`, `LaterDeleteReferences`, rebaseline, and handle exclusion support the stated distinction. No additional affected-set defect found.
- PATTERN requirements: named vocabulary, dependency direction, refusal guidance, stencil ownership, and test conventions were checked against the inherited backgrounds. No additional violation found.

Findings:

1. **MEDIUM — scope — `contracts/stencils/webster/webster-template-master.md:251`**  
   **Summary:** The post-fixer retry still sends every new plan conflict directly back to the fixer.  
   The `fixed` branch tells Merriam to retry `begin-batch`, then “run this rung again from step 1” for any `plan_conflict`. A repaired stale Edit target can expose a delete-before-reference conflict, changing the cause to `plan_order`; this instruction sends that refusal to step 2’s fixer instead of the new immediate-stuck route. Replace the unconditional retry instruction with cause dispatch: repeat the rung only for `plan_defect` or `tree_moved`, and apply the immediate-stuck instruction for `plan_order`.  
   **Evidence:** `contracts/stencils/webster/webster-template-master.md:242`, `:244`, `:247`, `:250–251`; `contracts/stencils/webster/webster-template-plan-fix.md:31–36`.

2. **MEDIUM — scope — `docs/overview.md:448`**  
   **Summary:** Webster’s public documentation still routes every plan conflict through the cold fixer.  
   The overview explicitly says every `plan_conflict` goes first to `fix-plan`, and its rerouting description assumes the fixer could not clear the conflict. The root CLI help similarly sends any begin-batch plan conflict to the fixer, and the CLI package documentation retains the unqualified route. These readers contradict the new `plan_order` bypass and the commit’s explicit request to check for remaining unconditional routes. Qualify these descriptions with `plan_defect`/`tree_moved` and describe `plan_order` as immediately stuck, retaining the conflict record for plan review.  
   **Evidence:** `docs/overview.md:448`, `:645`; `internal/webstercli/cli.go:239–240`; `internal/webstercli/doc.go:62`; `contracts/stencils/webster/webster-template-master.md:238–244`.

Tool calls:

1. Searched changed cause names, message wording, and check constants across code, contracts, and rules to enumerate readers and assertions.
2. Read retry routing, fixer help, CLI documentation, and shed conflict handling; searched repository documentation for remaining unconditional fixer routes.
3. Read root CLI help, overview routing passages, fixer scope, and tapestry review instructions to establish the documentation mismatch and verify the downstream plan-review route.
