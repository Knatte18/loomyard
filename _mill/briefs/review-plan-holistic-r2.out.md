MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (per system-provided model info)
reviewed_file: plan/
date: 2026-09-06
```

## Findings

### [BLOCKING:design] Informational findings still bounce Plan-Validate/Plan-Revalidate
**Location:** batch 5, card 23 (also touches card 18, batch 4) **Issue:** Card 18's `create-new-unit` finding is explicitly informational (fires on every `Create` card whose unit is genuinely new — i.e. every task that adds a brand-new package, including this task's own `internal/planglyph`/`internal/quarrycli`). Card 23 keeps `loomshed/planvalidate.go`'s existing `if len(findings) > 0 { ... Stuck }` gate unchanged (verified in `internal/loomshed/planvalidate.go`) and its own text states this deliberately: "an informational-only findings set still produces Stuck — the gate's own contract is that any finding bounces." Since nothing about creating a new package is fixable by Plan-Write, every such plan bounces to Plan-Write, resubmits unchanged, and bounces again until the segment's bounce budget escalates to a human — for a condition that was never wrong. This directly contradicts card 32's own handling of the identical `[]planglyph.Finding` type at `begin-batch`, which correctly lets only blocking findings return `ErrPlanDrifted` and carries informational ones on `BeginResult.Advisories` with no block. The plan introduces `Finding.Severity` specifically to draw this distinction (cards 16, 23's own log-rendering change) but then ignores it at the one gate that runs before any human ever sees the plan. **Fix:** Change `planValidate.Call` (and the standalone `validate-plan`/`webster validate` verbs, card 24) to map to `Stuck`/error-envelope only when at least one `Severity: blocking` finding is present; an informational-only set should map to `Done`/a clean envelope, with the informational findings still surfaced (e.g. in the log line or as an envelope field) for visibility.

### [NIT:consistency] Batch 7 scope narrative claims DoneChecks shares the DeltaGit delta; card 33's signature doesn't
**Location:** batch 7 overview / card 33 **Issue:** The batch's "Batch Scope" section states the one `DeltaGit` call "serv[es] handle binding, done-checks and the scope guard at once," but card 33's `DoneChecks(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error)` takes no `quarry.GitDeltaAnswer` parameter at all — it runs its own independent batched `Resolve`, unlike card 34 (`BindHandles`) and card 35 (`ScopeGuard`), which both explicitly take `delta quarry.GitDeltaAnswer`. The card-level signatures are unambiguous and correct on their own; only the batch-level rationale is imprecise. **Fix:** Reword the Batch Scope sentence to say the delta serves handle binding and the scope guard (and drift detection), with done-checks running its own separate `Resolve` call within the same `record-batch` invocation.

## Verdict

REQUEST_CHANGES
Fix the informational-severity/blocking-gate inconsistency at Plan-Validate/Plan-Revalidate before this plan proceeds.
MILL_REVIEW_END
