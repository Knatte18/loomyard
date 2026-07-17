MILL_REVIEW_BEGIN
# Review: Spike: structured Go reference/call-graph lookup (go/packages / gopls) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-17
```

## Findings

No findings. Verification performed:

- `tools/codeintel-poc/*` (all 5 files) and `.lsp.json` are absent (Glob confirms), matching
  batch 4's revert. `go.mod`/`go.sum` carry no `golang.org/x/tools` entry, matching Card 8.
- Only tracked product is `docs/research/codeintel-spike.md`; no `docs/modules/`,
  `docs/overview.md`, or `docs/roadmap.md` edits present, per the Documentation Lifecycle and
  the plan's explicit "do not touch" instruction (spike is not a milestone).
- Spot-checked ~10 load-bearing citations in the findings doc against live source and all
  resolved exactly: `internal/state/state.go:23,49` (`WriteJSON`/`ReadJSON` generic
  signatures), `internal/hubgeometry/hubgeometry.go:101,481` (`Resolve`, `Layout.WeftWorktree`),
  `internal/shuttleengine/engine.go:148` (`Engine.Prepare`), `internal/output/output.go:20,32`
  (`Ok`/`Err`), `internal/shuttlecli/cli.go:103`, `internal/buildercli/cli.go:204`,
  `internal/burlercli/cli.go:121`, `internal/perchcli/cli.go:140` (all four `claudeengine.New()`
  interface-boxing sites), `internal/warpengine/prune.go:127` (`WeftHostSlug` call site),
  `cmd/lyx/main.go:38` (`main.main`), `internal/clihelp/exec.go:160`
  (`RunRoot`→`cmd.ExecuteContext`). No fabricated or misattributed line numbers found.
- Doc's internal consistency holds: the precision table's `shuttleengine.Engine.Prepare` row
  (0 false neg/pos) and the CHA/RTA/VTA divergence table showing three genuinely different
  caller-set sizes across three unrelated symbols are exactly the expected outcomes if Card 7's
  two edits (interface-method `resolveSymbol` fix; `transitiveCallers` `Origin()` seed-condition
  fix) were correctly applied before the harness was measured and later deleted — consistent
  with the r1 blocking findings being resolved, not merely claimed.
- Shared Decisions (`throwaway-discipline`, `no-production-module-conventions`,
  `network-prerequisites`, `measurement-artifacts-to-scratch`, `findings-doc-is-the-deliverable`)
  are all reflected in the final state: no Cobra/registration artifacts, `.gitignore` carries
  `**/.scratch/`, the doc contains the adopt/defer verdict, cost table, precision table,
  CHA/RTA/VTA divergence + roots, and a runnable how-to recipe for the adopt-now arm.
- No constraint violations found (CLI/Cobra Invariant, Hub Geometry Invariant, Test Tier
  Purity — none apply/are all avoided as designed; no other module's files were touched).

## Verdict

APPROVE
Doc-only end state verified against source; all load-bearing citations check out; shared decisions and constraints satisfied.
MILL_REVIEW_END
