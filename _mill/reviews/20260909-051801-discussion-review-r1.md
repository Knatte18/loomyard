# Review: Centralize glyph ref-shape enumeration

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [NIT:consistency] Enforcement-test scope contradicts diskPathForRef's stated location
**Demoted-from:** BLOCKING
**Section:** Decision: kind-policy-ledger / Technical context: planparser migration sites (Inventory A)
**Issue:** `kind-policy-ledger` groups `diskPathForRef` and `refKindName` together as behavior-dispatch sites that "stay as switches ... and are listed in the ledger too," but Inventory A applies "promote to the registry file" only to `refKindName`, not `diskPathForRef` (`validate.go:360-374`, no relocation noted). The same decision's grep enforcement test asserts "no `classifyRef`/`refKind` comparison ... exists outside the registry file" with no stated exemption for a ledger-listed switch that stays in place — as written, `diskPathForRef`'s own `case refKindX:` arms in `validate.go` would fail the enforcement test the design itself specifies.
**Suggested fix:** State explicitly whether `diskPathForRef` also relocates to the registry file (making Inventory A consistent with the grouped treatment in kind-policy-ledger), or add an explicit ledger-listed-switch exemption to the enforcement test's grep pattern (e.g. scope it to filter-shaped call sites only, or allowlist named behavior-dispatch functions by name).

## Verdict

APPROVE
Solid, source-verified design; one concrete self-contradiction about where diskPathForRef's switch lives needs resolving before mill-plan.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
