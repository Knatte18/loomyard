MILL_REVIEW_BEGIN
# Review: loom: phase-machine scaffolding

```yaml
duration_s: 239.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (per runtime environment; not independently verifiable from inside the session)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [NIT:decision] Shed run-lock path has no declarer
**Demoted-from:** BLOCKING
**Section:** Technical context → Paths; `explicit-deps-struct` (`Deps.LockPath`)
**Issue:** Only `LoomStatusFile`/`LoomStatusLock` exist in `internal/loomengine/config.go`; no accessor for Shed's *run* lock exists anywhere in the repo, yet `Deps.LockPath` is told and the Durable-vs-Ephemeral Invariant requires a module scratch accessor beside its durable path (and `cmd/lyx/constructoranchoring_test.go` / `notransients_test.go` pin every such accessor).
**Fix:** State who declares the run-lock path (e.g. a new `loomengine.LoomRunLock` under `.lyx/loom/`) and add it to Scope and to the constructor-anchoring test set, or say explicitly that it is deferred to `loom: session bootstrap` and why Tier-1 temp paths suffice here.

### [BLOCKING:design] Webster wrapper's `batcher.Active` failure is unmapped
**Section:** `batchifier-is-a-gate`
**Issue:** Error mapping is pinned for row 9 only ("every error from `batcher.Active` maps to `Stuck`"); the row-10 wrapper resolves `batcher.Active` lazily inside its own `Call`, and a failure there has no stated disposition — `Stuck` (row 10 has `OnStuck: ""` → `RunBlocked`) and a returned error (`StateFailed`, `Run` returns the error) are materially different outcomes in `run.go`.
**Fix:** Pin the wrapper's mapping for a `batcher.Active` error explicitly, and add the corresponding assertion to the Testing bullet that already covers the two independent resolutions.

### [NIT:consistency] `BaseDir` and `AnchorPath` are the same value
**Section:** `explicit-deps-struct` (`Deps`)
**Issue:** `batcher.Active` → `configengine.LoadOrTemplate` → `FindBaseDir` stats `<baseDir>/_lyx`, and `planparser.PlanDir` joins `<anchorPath>/_lyx/plan`; the two fields are the same directory by construction, so two fields invite silent divergence.
**Fix:** Either drop `BaseDir` and feed `AnchorPath` to `batcher.Active`, or state why they are deliberately kept separable.

### [NIT:design] `Seed`'s write mechanics unaddressed
**Section:** `loomshed-owns-seed`
**Issue:** `state.WriteJSON` MkdirAlls the *status file's* parent but not the *lock file's* — the exact gap `preflight.go:101-113` documents and works around — and a stat-then-write refusal is a TOCTOU that `state.UpdateJSON`'s `found` guard would close atomically.
**Fix:** Say `Seed` creates the lock parent and makes the refuse-if-exists decision under the held lock (e.g. via `UpdateJSON` erroring on `found`), rather than stat-then-`WriteJSON`.

### [NIT:scope] Cancellation helpers are unexported in `shedadapters`
**Section:** Technical context → "Two obligations `Shed` cannot enforce"; Testing → Cancellation
**Issue:** `entryErr`/`cancelErr` (`internal/shedadapters/ctx.go`) are unexported and Scope forbids changing that package, so every real `loomshed` producer must re-implement the entry/exit ctx checks; the discussion does not say so.
**Fix:** Note that the pattern is duplicated in `loomshed` by design, so a plan writer does not propose exporting the helpers.

### [NIT:consistency] Read type after the schema migration is implicit
**Section:** `rewrite-loom-status-in-place`
**Issue:** Check 4 reads via `state.ReadJSONStrict[Status]` (unknown-field-rejecting); if `loomengine.Status` becomes the thin `{slug,parent,start_sha}` product struct, the read must be re-instantiated on `shedengine.Status` (a new `loomengine` → `shedengine` import) or it hard-fails on every Shed field.
**Fix:** State that check 4 decodes `shedengine.Status` and then the product payload, and that `loomengine` gains that import.

## Verdict

REQUEST_CHANGES
Two unstated dispositions: the Shed run-lock declarer and row 10's batcher-error mapping.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
