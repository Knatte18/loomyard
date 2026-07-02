MILL_REVIEW_BEGIN
# Review: Build internal/mux: the window to the world (overlay + strands + render) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-02
```

## Findings

### [NIT] Card 9 Context omits checksum.go
**Location:** batch 3 / card 9
**Issue:** `rules_test.go` requirement asserts `checksum prefix == layoutChecksum(body)`, but `checksum.go` (which defines `layoutChecksum`) is not in Context (only via `wrapLayout` in the listed `layout.go`).
**Fix:** Add `internal/muxengine/render/checksum.go` to card 9 Context.

### [NIT] Card 21 leaves `baseDir` for LoadConfig unspecified
**Location:** batch 6 / card 21
**Issue:** PreRunE says `hubgeometry.Resolve(cwd) -> muxengine.LoadConfig(baseDir, "mux")` but never names which Layout-derived path is `baseDir` (config lives in `_lyx`, i.e. `layout.Cwd`), unlike the explicit `weftBaseDir` in the weftcli pattern.
**Fix:** Name the value, e.g. `baseDir = layout.Cwd`, so the implementer needs no cold-start exploration.

### [NIT] Card 29 comment misstates registration order
**Location:** batch 7 / card 29
**Issue:** `wantSubs: {up, add, remove, status, attach, resume, down}` is annotated "matching the Command() registration order", but the actual AddCommand order across cards 22-27 is up, down, add, remove, status, resume, attach; harmless because helptree is a `strings.Contains` set check.
**Fix:** Drop/correct the "matching registration order" note (order is irrelevant for the superset check).

## Verdict

APPROVE
Plan is constraint-clean, DAG-valid, source-accurate; only cosmetic context/wording nits.
MILL_REVIEW_END
