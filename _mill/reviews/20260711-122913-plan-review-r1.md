MILL_REVIEW_BEGIN
# Review: Build builder - the batch-implementation loop — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-11
```

## Findings

### [BLOCKING] strandLive cannot read liveness from persisted mux state
**Location:** Batch 4, Card 18 (`strandLive`)
**Issue:** The card resolves pane liveness via `muxengine.LoadState` + "scanning `st.Strands` for the guid's `Live` field", but persisted `muxengine.Strand` (state.go) has **no `Live` field** — liveness is derived only by `muxengine.(*Engine).Status()` from a live `list-panes` query (`StrandStatus.Live = aliveIDs[PaneID]`). As written the `died` branch can never fire.
**Fix:** Source liveness from the mux engine's `Status()` seam (threaded like shuttleengine's `MuxOps`), not `LoadState`; the gatherer needs a live mux handle, not a dir path.

### [BLOCKING] turnEnded re-parses Claude event grammar in a provider-invariant package
**Location:** Batch 4, Card 18 (`turnEnded`)
**Issue:** The card says to detect the Stop-hook line by "mirror[ing] how `shuttleengine/wait.go` recognizes its Stop event line" — but `wait.go` does **not** parse Stop lines itself; `pollEventsTick` delegates to `run.runner.engine.ParseEvents` (claudeengine). Hand-parsing `events.jsonl` Stop-hook shape inside `builderengine` embeds Claude specifics, violating the Shuttle Provider-Seam Invariant (semantic half is a review obligation; the import-graph test won't catch it).
**Fix:** Route turn-end detection through the shuttle engine's `ParseEvents` seam (the discussion's sanctioned "reconstruct via FindRun and reuse shuttle's classification" path), keeping event grammar in claudeengine.

### [BLOCKING] SpawnBatch has no way to obtain the shuttle run dir
**Location:** Batch 5, Cards 21 & 22 (`ShuttleRunDir` / `SpawnResult.RunDir`)
**Issue:** Card 21 records `run dir` in `BatchState` and Card 22 exposes `SpawnResult.RunDir`, but `*shuttleengine.Run` exposes only `StrandGUID()` — there is no `RunDir()` accessor, and `shuttleengine` is not in the plan's touched-files set. `SpawnDeps` carries no `shuttleengine.Config`/`Layout` to reach `FindRun`, and the `Starter` seam returns only `(*Run, error)`.
**Fix:** Resolve the run dir via `shuttleengine.FindRun(cfg, layout, guid)` (add rundir.go to Card 21 Context and thread cfg/layout into `SpawnDeps`), or add a shuttle run-dir accessor as an explicit scoped change.

### [NIT] Card 10 references builderengine.ConfigTemplate without listing its file
**Location:** Batch 2, Card 10
**Issue:** Requirements name `builderengine.ConfigTemplate` (defined in template.go, Card 8) while `Context: none` and Edits list only configreg files — a strict Context-completeness gap, though the identifier is fully specified and the identical pattern is visible in the edited configreg.go.
**Fix:** Add `internal/builderengine/template.go` to Card 10 Context, or note it as an intra-batch dependency on Card 8.

## Verdict

REQUEST_CHANGES
Three cross-process poll/spawn mechanics (liveness, turn-end, run-dir) are not realizable as specified against the shuttle/mux seams.
MILL_REVIEW_END
