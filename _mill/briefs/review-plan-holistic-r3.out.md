MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-31
```

## Findings

### [BLOCKING:design] "curl absent from PATH" scenario has no described isolation mechanism
**Location:** Batch 2, Card 13 (sixth raw-path scenario), Card 12
**Issue:** Card 12 creates both `testdata/github-read/bin/gh` and `bin/curl` in the same directory, and every `run_scenario` call (per Card 13) puts that one directory on PATH. No card describes how the "curl absent from PATH" scenario constructs a PATH where the stub `gh` resolves but no `curl` (stub or real system one) does — unlike the tree harness's `gh`-missing test (test 18), which has a clean trick (`PATH=""` + absolute interpreter) that doesn't apply here since `gh` must still be found.
**Fix:** Add a requirement (Card 12 or 13) for a second, `curl`-free scratch/stub directory (e.g. a runtime-constructed dir containing only a copy/symlink of the stub `gh`) that this one scenario points PATH at instead, mirroring the runtime-generation idiom Card 4 already establishes for `gen_tree_body`.

### [NIT:consistency] Duplicate Context entry in batch 1 card 6
**Location:** Batch 1, Card 6 — `## Cards` → Card 6 `Context:` list
**Issue:** `plugins/prowler/scripts/testdata/github-tree/bin/gh` is listed twice in the card's `Context:` bullets.
**Fix:** Remove the duplicate line.

## Verdict

REQUEST_CHANGES
Card 12/13's curl-absence test needs a stated PATH-isolation mechanism; one duplicate Context entry to clean up.
MILL_REVIEW_END
