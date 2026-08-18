MILL_REVIEW_BEGIN
# Review: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
duration_s: 211.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] Scope bullet still names the rejected seed gate
**Demoted-from:** BLOCKING
**Section:** Scope → In, bullet on `cmd/lyx/stencilseed.go` ("gates `seedStencils` on the tier-1-AND-tier-2 wiring predicate")
**Issue:** The `seed-gate-is-tier1-plus-Ready-not-the-full-tier-2-report` Decision explicitly rejects the *wiring* predicate (`Wired` = tier 1 + `fabricengine.Ready`) for the seed and mandates `HubPresent`; the Scope line still carries the brief's superseded wording, and Scope is the authoritative in/out list a plan writer reads first.
**Fix:** Reword the Scope bullet to "gates `seedStencils` on `preflight.HubPresent`", with the `Wired`-is-not-the-seed-gate note inline.

### [NIT:consistency] `export_test.go` both forbidden and prescribed
**Demoted-from:** BLOCKING
**Section:** `standalonestate-is-pure-derivation-with-an-injectable-seam` + Technical context → Existing helpers to reuse
**Issue:** The Decision states "No `export_test.go`" and "**This task adds no `export_test.go` anywhere**", yet its own seam paragraph closes with "This is the same `export_test.go` idiom `internal/loomengine/export_test.go` already uses for `CheckResolvedForTest`", and Technical context lists that file as the shim idiom "for both `preflight` and `standalonestate`".
**Fix:** Delete the two stale references (or restate them as "reference only, not replicated") so exactly one disposition survives.

### [NIT:scope] preflight listed as shared infrastructure
**Section:** Scope → docs bullet; Technical context → "Docs to touch"
**Issue:** `docs/shared-libs/README.md:7-9` states a shared lib "does one mechanical thing … carries *no* domain logic"; `internal/preflight` carries orchestrator precondition policy, and the Scope bullet argues only the `.md`-per-entry contract, not this one.
**Fix:** Say explicitly why `preflight` still belongs there (or place only `buildinfo`/`standalonestate` there and give `preflight` a `docs/overview.md` tree row alone).

### [NIT:consistency] Stale line cite for `looksLikeHub`
**Section:** `seed-gate-is-tier1-plus-Ready-not-the-full-tier-2-report` rationale
**Issue:** Cites `clone.go:641`; the function is at `internal/fabricengine/clone.go:645` (641 is the start of its comment block). Every other cite checked in this round (`ready.go:17`, `warpclean.go:20`, `worktreelist.go:86`, `drift.go:52`, `junctionnames.go:126`, `stencilstore.go:135/142`, `stencilseed.go:24,29,74`, `tools/deploy/main.go:60,62`, `lyxcwd.go:150-163`) is accurate.
**Fix:** Correct to `clone.go:645`.

## Verdict

APPROVE
Two internal contradictions; one would reinstate the `_board` seed regression round 2 caught.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
