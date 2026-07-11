MILL_REVIEW_BEGIN
# Review: Build builder - the batch-implementation loop — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-11
```

## Findings

### [BLOCKING] Card 26 Context omits weftengine/hubgeometry/perchcli run.go
**Location:** Batch 7 (buildercli), Card 26
**Issue:** Requirements names `weftengine.Commit/Push/ScopedPathspec/EnvSyncOptions`, `hubgeometry.LyxDirName`, and `hubgeometry.PlanDir/BuilderDir/BuilderReportsDir`, and says to copy "perchcli's block-exit sync," but Context lists only `perchcli/cli.go` — the sync actually lives in `perchcli/run.go` (lines ~363-392), and neither `internal/weftengine` nor `internal/hubgeometry` is in Context. weft.go cannot be written from cli.go alone.
**Fix:** Add `internal/perchcli/run.go`, `internal/weftengine` (or its commit API file), and `internal/hubgeometry/hubgeometry.go` to Card 26's Context.

### [MEDIUM] Card 26 struct lacks engine + mux fields Card 28 poll needs
**Location:** Batch 7, Card 26 struct vs Card 28 poll wiring
**Issue:** Card 26's `builderCLI` struct lists only `runner`, `layout`, `cfg`, `roles`, and the derived dirs. But Card 28 wires `turnEnded` to "the claude engine instance the PreRunE constructed" and `strandLive` to "the mux engine" — and `*shuttleengine.Runner`'s `mux`/`engine` are unexported, so poll cannot reach them. The struct as specified makes Card 28 un-implementable as written.
**Fix:** Card 26 must also store `engine shuttleengine.Engine` and the mux engine (a `shuttleengine.MuxOps`) on `builderCLI`.

### [NIT] PlanBatch.Intent has two conflicting sources
**Location:** Batch 1, Cards 2 and 3
**Issue:** Card 2 parses the Batch Index line `NN — <slug> — <one-line intent>` (filling `PlanBatch.Intent`); Card 3 then says the batch file's `## Intent` first paragraph "supplies `Intent`" — two sources for one field, with no rule for which wins/overwrites.
**Fix:** Pin one source for `Intent` (e.g. the index one-liner) and, if the batch `## Intent` paragraph is also needed, give it a distinct field.

### [NIT] Card 21 Context omits stencil.go; Card 25 omits engine.go
**Location:** Batch 5 Card 21; Batch 6 Card 25
**Issue:** Card 21 Requirements calls `stencil.Fill` but `internal/stencil/stencil.go` is only in sibling Card 20's Context, not Card 21's. Card 25 maps `OutcomeDone/OutcomeAsking/OutcomeDied/OutcomeTimeout` (defined in `shuttleengine/engine.go`) but Context lists only run.go/spec.go.
**Fix:** Add `internal/stencil/stencil.go` to Card 21 Context and `internal/shuttleengine/engine.go` to Card 25 Context.

## Verdict

REQUEST_CHANGES
One blocking Context gap (Card 26 weft/geometry) plus a struct/poll consistency gap; both mechanical.
MILL_REVIEW_END
