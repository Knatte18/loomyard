MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5, tool-use/agentic mode)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:scope] Card 15's `unbindHub` has no committing mechanism in Context
**Location:** batch 5 / card 15 (`newClonedHubFixture`/`unbindHub`)
**Issue:** `unbindHub` must "delete the board's binding file and commit that deletion" inside the *already-materialized* board worktree so `reconcileWarpBinding`'s clean-board gate (card 13) passes. The only committing helper in card 15's `Context:` is `commitFileOnBranch`, which commits via a throwaway scratch clone pushed to the *bare remote* — it never touches an existing local worktree. The actual mechanism for a local commit inside an existing board worktree is `Bolt`/`NewBolt` (`bolt.go`), which is absent from card 15's `Context:`. Note also that `fabricengine.CloneHub` itself (called with both URLs) leaves `.lyx-anchor` and the freshly-seeded `fabric.yaml` uncommitted in the board worktree (CloneHub never commits — only the CLI's `Bolt.Commit` does), so whatever `unbindHub` uses must also be a stage-all commit to leave the board clean for the `recorded`-outcome tests, or those tests get `deferred` instead.
**Fix:** Add `internal/fabricengine/bolt.go` to card 15's `Context:` and state explicitly that `unbindHub` commits locally via `NewBolt(boardDir).Commit(...)` (stage-all, no push), which also sweeps up the uncommitted anchor/config from fixture setup.

### [NIT:consistency] Card 5's usage string names a flag not yet registered
**Location:** batch 2 / card 5
**Issue:** Card 5 has `runCloneWithReset`'s new usage-error text read `... [--force-bootstrap] <weft-url> [<warp-url>]`, but batch 2's own scope note says this batch "does NOT add the --force-bootstrap flag ... that is batch 3's job." For the span of batch 2 the error string advertises a flag `cloneCmd` does not register yet.
**Fix:** Either defer `--force-bootstrap` in the usage string to card 8 (batch 3), or note explicitly that the premature mention is intentional and harmless because all batches squash to one commit on merge.

### [NIT:consistency] Card 14 tells the implementer to leave a nonexistent `runStatus` alone
**Location:** batch 5 / card 14
**Issue:** "Edit `runReconcile` only — leave `runStatus`, `runPruneWithFlag`, and the clone command declaration alone." `internal/fabriccli/fabric.go` has no function named `runStatus`; the function backing `lyx fabric pairs` (which calls `top.Status`) is `runPairs`.
**Fix:** Rename the reference to `runPairs`.

### [NIT:consistency] Card 10's rationale for `--force-bootstrap` misreads its own fixture
**Location:** batch 3 / card 10
**Issue:** Card 10 says the CLI's weft bare fixtures (`makeCLICloneWeftBare`) "carry a seeded commit and no anchor, so the old-order guard would refuse them," justifying `--force-bootstrap` on the two end-to-end tests. `makeCLICloneWeftBare` actually creates a genuinely empty bare repo (`git init --bare`, zero commits) — per card 3's own probe spec this is the unborn-HEAD case, `WeftLooksLikeWeft: true`, which never trips the guard regardless of `ForceBootstrap`. The flag ends up redundant-but-harmless, not required.
**Fix:** Correct the rationale (or drop the now-unneeded flag), since a future reader may mistrust the guard's unborn-HEAD carve-out based on this misstatement.

## Verdict

REQUEST_CHANGES
Fix card 15's missing `Bolt` context for `unbindHub`; the three NIT consistency slips are minor and separable.
MILL_REVIEW_END
