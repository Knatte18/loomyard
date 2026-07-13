# Plan: Restore the Tier 1 floor: guards + perchengine

```yaml
task: 'Restore the Tier 1 floor: guards + perchengine'
slug: restore-tier1-floor
approved: true
started: '20260713-090820'
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: code-fixes
    file: 01-code-fixes.md
    depends-on: []
    verify: go test ./internal/clihelp ./internal/perchengine ./internal/boardengine/boardtest ./cmd/lyx -count=1 && go test -tags integration ./internal/perchengine -run TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay -count=1 -v
  - number: 2
    name: benchmarks-block
    file: 02-benchmarks-block.md
    depends-on: [1]
    verify: go test ./... -count=1
```

## Shared Decisions

### Decision: measurement-driven-scope

- **Decision:** This task's scope was set by discussion-phase measurement, not the
  wiki proposal. The two levers are (a) disabling cobra's Windows mousetrap check
  at the `internal/clihelp` seam and (b) moving the one real-time perchengine test
  to Tier 2 — plus (c) one bounded, conditional-keep attempt at shrinking the
  boardtest concurrency test's writer-iteration volume (operator-approved
  extension; kept only if it wins ≥ ~1 s of isolated package time, reverted with
  a doc-comment note otherwise — see card 3 and the `boardtest-bounded-shrink`
  decision in `_mill/discussion.md`). The proposal's "shared parse-pass for
  cmd/lyx guards" is refuted (guards cost ~0.25 s combined) and is NOT built.
  See `_mill/discussion.md` for the full evidence trail.
- **Rationale:** Measured: Tier 1 wall ~37 s baseline → ~23 s with mousetrap
  disabled → ~11.7 s with both fixes (simulated in this worktree).
- **Applies to:** all batches

### Decision: no-behavior-change-outside-the-levers

- **Decision:** No test assertion is dropped or weakened anywhere. The perchengine
  test moves verbatim (file relocation only); the boardtest shrink changes only
  iteration constants; no guard test's enforcement semantics, allowlists, or
  banned-token sets are modified.
- **Rationale:** The equivalence guardrail from the discussion: re-tiering and
  volume reduction, never coverage loss.
- **Applies to:** all batches

### Decision: append-only-benchmarks-discipline

- **Decision:** `docs/benchmarks/test-suite-timing.md` is append-only: the new
  dated block becomes "Current best times"; the superseded 2026-07-13 block moves
  under History retitled and frozen, never edited in content. The wrong causal
  attributions in older blocks are corrected by supersession notes in the new
  block, not by rewriting history.
- **Rationale:** The doc's own stated discipline ("Append-only: each block is the
  state at that revision and is frozen").
- **Applies to:** benchmarks-block

### Decision: go-native-verify-commands

- **Decision:** `verify:` commands use the native Go toolchain directly (no
  `PYTHONPATH= ` prefix — this is a Go repo). Batch 1 scopes verify to the four
  touched/guarding packages plus a targeted `-tags integration` run proving the
  relocated test compiles and passes in Tier 2; batch 2 (docs terminal batch) runs
  the repo-wide untagged suite as the final gate.
- **Rationale:** Per-batch verify runs after every implementer/fixer round, so it
  must stay in the tens-of-seconds range; both commands do.
- **Applies to:** all batches

## All Files Touched

- `docs/benchmarks/test-suite-timing.md`
- `internal/boardengine/boardtest/concurrency_test.go`
- `internal/clihelp/exec.go`
- `internal/perchengine/gate_lingering_test.go`
- `internal/perchengine/gate_test.go`
