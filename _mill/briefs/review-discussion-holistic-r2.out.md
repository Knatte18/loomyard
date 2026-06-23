Now I understand the full picture. `cmd/lyx board sync` → `board.RunCLI` → `b.Sync()` (board.go:172) → package `Sync(boardPath)` (sync.go:31) which reads env directly. The discussion's Decision B says production should resolve env→Config at the entry point and have `(b *Board) Sync()` thread the flags through. This is feasible but the discussion's phrasing is somewhat loose: `b.Sync()` would need to thread its `Config.SkipGit/SkipPush` through to package `Sync`, which means package `Sync` needs a new signature/options. The discussion does say "thread the flags through that call" (board.go:172) and add options to package funcs, so this is covered.

I have verified all the major claims. Let me now assess the findings. The discussion is round 2 with r1 findings incorporated. The main remaining issues I found:

1. **No existing weft-pushing test** — Workstream C testing says "keep at least one weft-pushing test on the full fixture," but all 12 Add calls use `SkipPush:true`. There is no weft-pushing test to keep; the weft-bare is never exercised as a live push target by any current test.

2. **PairedFixture.WeftBare field** — the lean variant's struct shape is unspecified.

Let me write the review.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-23
```

## Findings

### [GAP] No existing weft-pushing test to "keep on full fixture"
**Section:** Testing § Workstream C / Decision C
**Issue:** Testing says "Keep at least one weft-pushing test on the full fixture," but all 12 `Add` call sites in `internal/worktree` (add_test.go, weft_test.go, remove_test.go) pass `AddOptions{SkipPush:true}` — no test pushes the weft branch, so the weft-bare is never exercised as a live push target by any current test and there is no candidate to keep.
**Fix:** State whether the plan must author a new weft-pushing test (using the full `CopyPaired`) to cover the weft-bare/`pushWeftBranch` path, or drop the "keep one" clause and accept that weft-bare push is uncovered.

### [GAP] Lean fixture struct shape vs. PairedFixture.WeftBare unspecified
**Section:** Technical context § Fixtures (workstream C)
**Issue:** `PairedFixture` (lyxtest.go:324) exposes a `WeftBare` field and `Layout` derived from the hub; the discussion says the lean variant "omits the weft-bare" and may "skip or no-op the origin-URL rewrite on the weft-prime's origin," but does not specify what the lean fixture's `WeftBare`/`WeftPrime.origin` fields hold (empty string, absent, or a dangling URL) — leaving the weft-prime origin pointed at a non-copied path could surprise a future weft-pushing test.
**Fix:** Specify the lean variant's return type/fields (e.g. `WeftBare==""`) and that weft-prime's origin URL is left unrewritten (pointing at the template bare), confirming no in-scope `SkipPush:true` test reads those fields.

### [NOTE] Decision B entry-point resolution glosses the package-Sync seam
**Section:** Decision B / Technical context
**Issue:** Decision B says production "resolves env→Config at the entry point (cmd/lyx board sync) so internal sites read the resolved flag only," but that path is `RunCLI` (cli.go:263) → `b.Sync()` (board.go:172) → package `Sync(boardPath)` (sync.go:31), which reads env directly with no Config seam; threading `Config.SkipGit/SkipPush` requires giving package `Sync` a new options signature, which the prose only implies.
**Fix:** State explicitly that `(b *Board) Sync()` must pass its resolved flags into package `Sync` via the new options seam, and that `RunCLI`'s sync case resolves env→options once.

## Verdict
GAPS_FOUND
Weft-pushing coverage claim has no existing test; lean-fixture field shape underspecified.
MILL_REVIEW_END