MILL_REVIEW_BEGIN
# Review: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
duration_s: 138.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:design] Producer identity is required but never told
**Section:** §Decisions "StuckReason surfaces through the log" + §Testing (`SingleLLMProducer`)
**Issue:** The log call is specified as carrying "the producer identity" and the error text is asserted to "name the outcome and the producer", but `ShedProducer.Call(ctx)` carries no name (`internal/shedengine/producer.go:30-32`; `ProducerDef.Name` is Shed's, not the adapter's), and the "Told, never derived" inventory lists no name/label parameter for any of the three constructors.
**Fix:** State whether each `New...` takes a producer/instance name (and what it is used for — log fields and error text), or drop the "names the producer" requirement from both the StuckReason decision and the test assertions.

### [BLOCKING:design] Archive timestamp source unpinned; collision test not deterministic
**Section:** §Decisions "Stale output files — archive, then respawn" + §Testing (archive rows)
**Issue:** Both cited precedents take an injectable clock (`archiveStaleOutcome(websterDir string, now func() time.Time)`, `ArchiveStaleSummary(...)` — `outcome.go:77`, `summary.go:77`), and `firstFreeArchivePath` is unexported so the helper must be re-implemented locally, yet no Decision says whether `SingleLLMProducer` takes a `now func() time.Time` seam — without one the specified "second archive in the same timestamp second" test can only hope both calls land in one clock second.
**Fix:** Decide and record the timestamp source (injected clock seam vs. direct `time.Now().UTC()`), and align the collision test's stated mechanism with it.

### [NIT:decision] Two further shed.md overclaims have no disposition
**Demoted-from:** BLOCKING
**Section:** §Decisions "Reattach out of scope" / §Q&A "Which docs land in this commit"
**Issue:** Only `shed.md:255` and the `:3` banner are scoped, but `shed.md:261` ("Crash-recovery of live-session state (reattach vs. respawn) — inside `SingleLLMProducer`/`perch`/`Webster`'s own `Call()`") states the same reattach claim, and `shed.md:278` describes `SingleLLMProducer` as "parameterized by an Input-format pointer, an Output-format pointer, and one instruction file", which the caller-supplied-`Spec`-source Decision supersedes.
**Fix:** Name both lines explicitly as in-scope corrections (or state why each stays as written).

### [NIT:consistency] Roadmap Done entry contradicts the Planned-item move
**Demoted-from:** BLOCKING
**Section:** §Scope "Docs in the same commit" / §Q&A doc list
**Issue:** Only "the roadmap move of Planned item 1 to Done" is scoped, but the already-Done Shed skeleton entry (`manifest/roadmap.md:196-199`) asserts the three adapters "remain their own Planned item above" and justifies shed.md's survival by that Planned item — both become false in this same commit.
**Fix:** Add the `roadmap.md:196-199` edit (and shed.md's Documentation-Lifecycle survival rationale) to the in-commit doc list.

### [NIT:scope] Webster error mapping enumerates only five errors
**Section:** §Decisions "Webster adapter — `Fresh` fixed false"
**Issue:** `websterengine.Run` also returns non-sentinel errors (plan-validation refusal `runlevel.go:335`, zero-batches `:347`, `MkdirAll`/run-lock failures `:309-321`) that no mapping row covers.
**Fix:** State the default rule once — any error other than `*MasterAskingError` is a non-nil error.

### [NIT:decision] `OutputPointer` on the shuttle `asking`→`Stuck` path unstated
**Section:** §Decisions "`SingleLLMProducer` outcome mapping" / §Testing
**Issue:** Perch's `Stuck` pointer is explicitly pinned empty, but `SingleLLMProducer`'s `asking` row says only `(Stuck, nil error)`; Shed persists `history[].output` on that branch too.
**Fix:** Pin the pointer value on the `asking` path explicitly (empty, since no output file exists).

## Verdict

REQUEST_CHANGES
Four gaps: adapter identity, archive clock seam, two shed.md overclaims, roadmap Done-entry contradiction.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
