MILL_REVIEW_BEGIN
# Review: reed: extract Selvage-pane lifecycle

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Identifier rule flags the sanctioned survivors
**Section:** `enforcement-test-is-the-teeth` + Technical context
**Issue:** "any AST identifier whose name contains `Selvage` outside the allowlist" condemns end-state code the discussion explicitly keeps: `lifecycle.go`'s retained `upLocked`/`Resume` calls to `ensureSelvagePaneLocked` (verified at `lifecycle.go:653`/`:752`, cited as staying at discussion line 112), and `apply.go`'s `render.Params{… Selvage: …}` composite-literal key, which is an `*ast.Ident` named `Selvage` and survives by design (`narrow-seam-shapes`: "`toRenderInputs` assigns the returned value straight into `render.Params.Selvage`").
**Fix:** state the predicate's exemptions precisely — at minimum call expressions naming allowlisted-file declarations, and composite-literal field keys — or pick a rule the sanctioned end state actually satisfies.

### [BLOCKING:design] Case sensitivity of the identifier match undecided
**Section:** `enforcement-test-is-the-teeth`
**Issue:** the decision claims the predicate covers "locals and parameters", but `selvagePaneID` — the actual local/param name in `apply.go:85`, `spawn.go:149`, `reconcile.go:213` — does not contain the case-sensitive string `Selvage`; the discussion's own "Why now" note proves the two measurements differ. Conversely a case-insensitive rule flags every seam-helper call whose name starts lowercase-`selvage`, and helper names are delegated to mill-plan.
**Fix:** decide case-sensitive vs case-insensitive explicitly, and reconcile that choice with the "locals and parameters covered" claim and with the naming freedom given to mill-plan.

### [BLOCKING:consistency] Q&A log states the superseded narrow rule
**Section:** Q&A log, entry 5 (line 158)
**Issue:** it answers that the test polices "only `.SelvagePaneID` selector expressions and top-level declarations" and that "locals and parameters are not policed" — directly contradicting `enforcement-test-is-the-teeth`. Entry 12 (line 163) records the widening but nothing marks entry 5 as superseded, so a plan writer reading the log top-down implements the narrow rule the task says is insufficient.
**Fix:** mark entry 5 superseded by entry 12, or restate it to match the identifier-level decision.

### [NIT:scope] `doc.go`'s 34 Selvage lines absent from the audit table
**Section:** Problem ("Why now") / Testing ("Post-extraction audit")
**Issue:** the per-file baseline omits `doc.go`, verified at 34 case-sensitive matching lines — second only to `lifecycle.go` — while the same commit rewrites `doc.go`'s Selvage section, so the after-count will move with no before-figure to compare against.
**Fix:** add `doc.go` (comment-only) to the baseline enumeration and note that its count changes by design.

### [NIT:consistency] "~350 assertions" contradicts the stated method
**Section:** Q&A log, entry 6 (line 159)
**Issue:** "~350 existing Selvage assertions" conflicts with `pure-refactor-no-behavior-change`'s measured 276 matching lines and its explicit "*not* an assertion count" caveat, in a file that otherwise insists the method be republished with every number.
**Fix:** replace with the 276-matching-lines figure and the method name.

## Verdict

REQUEST_CHANGES
Enforcement-test predicate condemns code the task deliberately keeps; two rule statements conflict.
MILL_REVIEW_END
