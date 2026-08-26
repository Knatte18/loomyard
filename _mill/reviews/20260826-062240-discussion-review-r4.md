MILL_REVIEW_BEGIN
# Review: Fix Bouncer anchor-path and run-dir clearing

```yaml
duration_s: 249.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic Claude, Opus-class; per env metadata)
reviewed_file: /home/knatte/Code/loomyard/wts/loom-bouncer-anchor-rundir-fix/_mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:scope] Defect-2 enumeration method is file-scoped and misses doc.go
**Demoted-from:** BLOCKING
**Section:** "Stale-assertion inventory — defect 2"
**Issue:** The method is declared as "read every doc comment in `internal/shedadapters/bouncer.go` and `contracts/recipes/loom-recipe.yaml`", justified by "the set is small and file-local because the replay path is entirely private to `Bouncer.Call`" — but `internal/shedadapters/doc.go:42` ("Bouncer: Call resolves into one of four modes -- seed, re-bounce, judge, or replay") restates the same four-mode branch the fix falsifies, and it sits in the package doc's "Outcome mapping" section, not the "round-artifact convention" section Scope names.
**Fix:** Widen the method to cover the package doc (or state it as the two files plus `doc.go`'s outcome-mapping paragraph) and give `doc.go:42-48` an explicit disposition row.

### [NIT:consistency] Problem overstates defect 2's in-run reachability
**Section:** "Problem — Defect 2" / "Why now"
**Issue:** The described trigger ("a downstream row bounces back past the writer and control flows through the segment a second time") is not reachable in the shipped Discussion path: nothing after `Discussion-Bouncer` (`on_done: Plan-Write`) routes back to `Discussion-Write`/`Discussion-Validate`, so post-approval re-entry there is only cross-invocation or the crash window. Only `Plan-Revalidate` → `Plan-Write` → `Plan-Validate` → `Plan-Bouncer` reaches it in-run.
**Fix:** Say which route reaches the defect in each segment, so the "confirmed present in the shipped Discussion path" claim reads as cross-invocation rather than in-run bounce-back.

### [NIT:consistency] Scope "In" file list omits two inventoried edit sites
**Section:** "Scope — In" vs the defect-1 inventory
**Issue:** The inventory assigns "Rewrite"/"Reword" dispositions to `internal/loomcli/wiring.go:87-91` and `internal/shedrecipe/recipe.go:37-42`, but neither file appears in the "In" bullet list (recipe.go is reachable only via an aside in "Out"; loomcli/wiring.go appears nowhere but the table).
**Fix:** Add both files to the "In" list, or state once that the inventory tables extend the In list rather than restate it.

## Verdict

APPROVE
Defect-2's comment enumeration provably misses `shedadapters/doc.go`'s four-mode claim.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
