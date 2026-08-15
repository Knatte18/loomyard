MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-15
```

## Findings

### [BLOCKING:design] One LockPath serves two distinct locks
**Section:** §Decisions/`run-lock`, `told-never-derived-paths` **Issue:** `Shed` has a single `LockPath`, described as the run lock held across the whole `Run`, but `state.ReadJSONStrict`/`state.WriteJSON` each also take a `lockPath` and `WriteJSON` calls the *blocking* `lock.AcquireWriteLock` — `internal/state/state.go:108-109` states plainly that nesting one inside a held lock on the same path "hangs rather than failing", and `internal/treadleengine/run.go:34-42` keeps `run.lock` and `state.json.lock` deliberately distinct; the discussion never says which path Shed passes to `state`. **Fix:** decide and record the two lock paths explicitly (a second field, or `StatusPath + ".lock"` as the write lock beside the run `LockPath`), and state that they must never be the same file.

### [BLOCKING:design] "Shed is the file's only writer" is false; pause is never re-read
**Section:** §Decisions/`activity-mechanical-fill`, `product-field-passthrough`; `ctx-cancellation-as-pause` **Issue:** `pause_requested` is kept **in-status** and set by an outside actor (`docs/reference/status-schema.md:69`), and the seed is written by a spawn-time command — yet the loop reads the status file once (step 1) and routes back to step 2, so a pause requested during a long producer call is never observed, and step 5's whole-file rewrite from Shed's in-memory copy silently clobbers it (and any external `product` update). **Fix:** decide whether the status file is re-read at the top of every iteration (or persisted via a read-modify-write under one lock), and state which fields are external-writer-owned rather than asserting Shed is the sole writer.

### [BLOCKING:consistency] Persist-failure test method does not fail
**Section:** §Testing, "Persist failure" **Issue:** the proposed trigger "a `StatusPath` whose parent does not exist" will not fail — `state.WriteJSON` does `os.MkdirAll(filepath.Dir(path))` before locking (`internal/state/state.go:29-32`); the alternative "unwritable directory" is a no-op under root and on Windows, so the test would silently assert nothing. **Fix:** name a fault-injection method that actually fails on the target platforms (e.g. a `StatusPath` whose parent path component is an existing regular file), or drop the claim.

### [NIT:consistency] `product` passthrough enumerates only part of loom's schema
**Section:** §Decisions/`product-field-passthrough` **Issue:** the rationale names `slug`, `parent`, `start_sha`, `next_action`, but `status-schema.md:61-67,90-95` also mandates `phase`, `stage`, `narration` and a `{phase,outcome,bounced_to,ts}` history shape, so a Shed-written loom status file would still fail `checkCoherence` — the passthrough does not buy the compatibility the rationale implies. **Fix:** state that `product` carries no compatibility claim for loom's shipped schema, and keep that wording out of the package-doc divergence note.

### [NIT:design] Timestamp source and `Result.History` scope unstated
**Section:** §Decisions, §Testing **Issue:** nothing says what writes `history[].at` (a `time.Now` call vs. an injectable clock) or its format, while `status-schema.md:77-78` pins RFC3339 UTC; nor whether `Result.History` is the whole persisted history or only this run's entries — the crash-recovery test asserts against it either way. **Fix:** pin the timestamp format/source and the `Result.History` scope in one line each.

## Verdict

REQUEST_CHANGES
Locking, pause-flag ownership, and one unworkable test method need resolving before planning.
MILL_REVIEW_END
