MILL_REVIEW_BEGIN
# Review: loom: pin the spawn/handover status schema + discussion-format.md

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Inbound links live in Go godoc, colliding with spec-only
**Section:** Scope (relocate) / Testing (link integrity)
**Issue:** `docs/modules/plan-format.md` / `builder-contract.md` are referenced from ~8 Go files (`internal/builderengine/{doc,report,validate,spawn,plan,chain}.go`, `template_test.go`, `buildercli/cli.go`, `hubgeometry.go:223,247`) and `internal/builderengine/implementer-template.md` + `tools/sandbox/SANDBOX-BUILDER-SUITE.md`; "update every inbound link" + verify "zero stale references" contradicts "No Go code / no go-test gate applies."
**Fix:** State explicitly whether godoc/template/sandbox path fixups are in scope (comment-only .go edits) or whether the link-integrity verify is scoped to `docs/` only — leaving Go references stale.

### [GAP] README scope item references links that don't exist
**Section:** Scope ("Update docs/modules/README.md")
**Issue:** The scope says README's "table links plan-format.md/builder-contract.md," but `docs/modules/README.md`'s table (lines 10-17) contains no link to either doc — the plan writer will find nothing to update.
**Fix:** Correct or drop the README item; if a mention exists elsewhere in README it is not in the table — cite the real location, else remove the claim.

### [GAP] Pinned `preflight` phase enum contradicts loom.md/overview.md "Setup"
**Section:** Decisions: status-field-set / loom-md-reconciliation
**Issue:** The schema pins `phase: preflight|…|raddle|…`, but loom.md's phase machine (lines 54, 63, 239) and `overview.md:273` still say "Setup" (overview also omits Raddle); `loom-md-reconciliation` only scopes the "State & contracts" section, leaving the phase-machine label and overview blurb divergent from the fixed vocabulary.
**Fix:** Extend reconciliation scope to align the phase-machine labels (Setup→Preflight, add Raddle) in loom.md and overview.md, or state they are deliberately out of scope.

## Verdict

GAPS_FOUND
Three scope/reconciliation gaps: Go-godoc inbound links, a nonexistent README link, and a Setup/Preflight vocabulary divergence.
MILL_REVIEW_END
