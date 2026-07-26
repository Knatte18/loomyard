MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-26
```

## Findings

### [GAP] fabriccli/fabric.go Short/Long stale + dangling fabric.md ref
**Section:** Scope (parallel-build cleanup) / Technical context §fabric.md inbound links
**Issue:** `internal/fabriccli/fabric.go` Command `Short` (`:40` "parallel build alongside warp/weft") and `Long` (`:40-54`) describe fabric as coexisting "during the parallel-build period, not yet the default" and cite the deleted `manifest/designs/fabric.md` (`:54`); this file is named in no cleanup item, and the nine-link repoint list omits it — so the "zero dangling" claim (Q&A r2) is false and the CLI/Cobra help-accuracy obligation is unmet.
**Fix:** Add `internal/fabriccli/fabric.go` to Batch D: rewrite its `Short`/`Long` to describe fabric as the sole git-coordination module and repoint/remove the fabric.md reference.

### [GAP] Parallel-build comment rot in fabric production code unscoped
**Section:** Scope / Testing §grep-clean gate Tier 2
**Issue:** The parallel-build prose cleanup is scoped only to `fabricengine/doc.go` + FABRIC-SUITE + `tools/sandbox/main.go`, but "parallel-build period" comments describing coexistence with the warp/weft modules survive in `internal/fabricengine/{fabric.go:96,cleanup.go:14/63/158,weftgit.go:8/27,hook.go:9}` and `internal/fabriccli/{clone.go:23,fabric.go:12/218}`; none match the Tier-2 grep `internal/(warp|weft)(cli|engine)` nor the enumerated bare-name sweep list (perch/webster/reed/lyxtest), so they fall through every defined gate.
**Fix:** Extend Batch D's comment sweep (or Tier-2 scope) to include `internal/fabricengine` + `internal/fabriccli` "parallel-build period" comments that reference warp/weft as live modules.

### [NOTE] Inbound-link count inconsistent (7 vs 9) after r2 fix
**Section:** Decisions §batch DAG (`:106`) / §fabric.md deletion (`:150`)
**Issue:** The r2 resolution updated the count to nine (`:50,:263`), but Batch D still says "repoint 7 links" and the fabric.md-deletion Decision still says "seven other docs", so a plan writer reading those sections could repoint only seven.
**Fix:** Update `:106` and `:150` to "nine" to match the authoritative enumerated list at `:263`.

## Verdict

GAPS_FOUND
Fabric's own CLI help and in-code parallel-build comments are stale/unscoped, leaving a dangling fabric.md ref.
MILL_REVIEW_END
