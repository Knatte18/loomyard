MILL_REVIEW_BEGIN
# Review: Facilitate Linux support (Win11-side prep) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-10
```

## Findings

### [NIT] Card 3 Context omits config.go for `e.cfg.Psmux`
**Location:** Batch 1 / Card 3
**Issue:** Requirements write new code calling `matchSocketCmdlines(procs, e.cfg.Psmux, e.Socket())`, but `Config.Psmux` is defined in `internal/muxengine/config.go`, which is not in the card's `Context:` (lock.go/overlay.go/proctree.go); card 6 correctly lists config.go for the same field access.
**Fix:** Add `internal/muxengine/config.go` to Card 3's `Context:` for parity with Card 6.

### [NIT] Card 6 leaves psmux `list-commands` support unverified on host
**Location:** Batch 2 / Card 6
**Issue:** Card 5 tells the implementer to run `psmux -V`, but nothing verifies the on-box psmux actually supports `list-commands`; if it does not, the real probe (Card 7) would fail every `mux up` on Windows, and the batch-2 verify never boots a server so it would not catch this.
**Fix:** Have Card 5/6 also confirm `psmux list-commands` output shape against the dev-box binary, or note the probe's command-surface check is validated only via the Card 10 integration test.

## Verdict

APPROVE
Plan is complete, correctly sequenced, and source-grounded; only two minor Context/verification nits.
MILL_REVIEW_END
