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

### [BLOCKING:design] Nothing ever clears `pause_requested`
**Section:** `field-ownership-split` + `state-and-error-fields` + Testing (`pause_requested: true mid-list`)
**Issue:** `pause_requested` is external-writer-owned and only carried forward, and pause writes `state: "paused"` leaving `current_producer` unchanged — so the next `Run` re-reads a still-true flag at step 3 and pauses again immediately, forever; `internal/treadleengine/run.go:132` (`clearPauseFlag`) exists precisely because a resumed run must not re-pause on the flag it is resuming from.
**Fix:** Decide and record who clears the flag — Shed clearing it on the resuming iteration (making it a fourth Shed-written field), or the product's resume verb — and add the resume-after-pause loop test.

### [BLOCKING:design] Persist-failure fault injection never reaches the persist
**Section:** Testing, "Persist failure"
**Issue:** With `StatusPath = x/status.json` where `x` is a regular file, step 1's `state.ReadJSONStrict` calls `os.ReadFile` first; `ENOTDIR` is not `os.IsNotExist`, so it returns `ErrRead` (`internal/state/state.go:147-153`) and `Run` hard-errors before any producer runs — and no last-good status file can exist at that path to assert against.
**Fix:** Name an injection method that fails only the write (e.g. make `StatusPath`'s directory tree hostile only after the first successful read, or an unwritable `StatusLockPath` parent), or drop the byte-identical last-good assertion as unstageable.

### [BLOCKING:design] Lock-file parent directories: no stated owner
**Section:** `run-lock`, `two-lock-paths-never-the-same-file`, Scope ("No `_lyx` path construction")
**Issue:** `gofrs/flock` opens the lock with `O_CREATE` but never creates parents (`internal/lock/lock.go:22-23`), which is why `internal/loomengine/preflight.go:151` does an explicit `MkdirAll` before locking; the discussion says Shed creates nothing and touches nothing on disk, so a first run against a not-yet-existing `.lyx/loom/` fails with a lock-acquire error rather than `ErrShedBusy` or a clean result. The claim that lock acquisition "touch[es] nothing on disk" is also false — the lock file itself is created.
**Fix:** State the disposition — Shed `MkdirAll`s each lock path's parent before acquiring (treadle's own precedent, `run.go:102-119`), or the caller must, documented on the two lock fields — and correct the "nothing on disk" wording.

### [BLOCKING:consistency] Q&A log keeps the superseded sole-writer answer
**Section:** Q&A log, "Who fills `activity`?"
**Issue:** That entry answers "Shed, mechanically — **it is the file's only writer, so nothing else can**", which the later `field-ownership-split` decision and the "Is Shed really the status file's only writer?" entry explicitly reverse; a plan writer reading top-down encodes the sole-writer claim the whole re-read-and-merge design exists to kill.
**Fix:** Reword that Q&A answer to the surviving rationale (Shed owns the data `activity` is composed from), with no sole-writer clause.

### [BLOCKING:design] `current_producer`'s value on completion is unstated
**Section:** `activity-mechanical-fill`, `run-entrypoint-result`, Testing ("Happy path")
**Issue:** shed.md step 6 says "advance to the next entry; past the last entry → write `state: "done"`", never saying what `current_producer` holds afterwards — yet `activity.now` is "`current_producer`'s name" and `Result.HaltedProducer` is "the producer `current_producer` named when `Run` returned", so both are undefined at the happy-path terminal the tests assert on.
**Fix:** Pin one value (last producer's name, or `""`) and state the corresponding `activity.now`/`HaltedProducer` values for `RunDone`.

### [NIT:consistency] "byte-identically" is not what a `json.RawMessage` round-trip guarantees
**Section:** Testing (`product` passthrough; status-file round-trip)
**Issue:** Persist goes through `state.WriteJSON`'s `json.MarshalIndent` (`internal/state/state.go:48`), which re-indents an embedded `json.RawMessage`, so a payload written with different whitespace survives semantically but not byte-for-byte.
**Fix:** Say the assertion is semantic equality (`json.Marshal`-normalised compare), not byte identity.

## Verdict

REQUEST_CHANGES
Pause-clear gap, unworkable persist-failure injection, lock-dir ownership, and a stale sole-writer answer.
MILL_REVIEW_END
