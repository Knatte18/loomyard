MILL_REVIEW_BEGIN
# Review: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
duration_s: 161.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class; exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:design] PerchProducer breaks on its second Call
**Section:** "Perch cancellation bridge" / Technical context ("told ... its `runDir`/`scratchDir`/`stencilsDir`")
**Issue:** `runDir` is told once at construction, but `treadleengine.loadOrInitState` refuses a terminal run dir outright — `if existing.Outcome != "" { return ... "%s: this block already finished (%s)" }` (`internal/treadleengine/state.go:126-128`) — so after the gate returns APPROVED or STUCK, Shed's unconditional re-call (resume, or the `OnStuck` bounce-back this adapter exists to serve) makes `perchengine.Engine.Run` return an error, which Shed records as `state: "failed"` instead of re-running the gate. This is the exact stale-state hazard the discussion solved for `SingleLLMProducer` (archive-then-respawn) and for Webster (which self-archives, `runlevel.go:440-443`), left unsolved for the one adapter whose bounce loop is the task's motivating use case.
**Fix:** Add a Decision fixing perch's per-Call run identity — a told run-dir source evaluated per `Call` (mirroring the `Spec` source), an archive/clear of the terminal run dir, or an explicit accepted-limitation statement — plus the matching second-Call test.

### [NIT:decision] Asking-path detail has no stated disposition
**Demoted-from:** BLOCKING
**Section:** "`StuckReason` surfaces through the log" / "`SingleLLMProducer` outcome mapping" / "Webster adapter"
**Issue:** The log-channel Decision covers only perch's and Webster's `StuckReason`, but both `asking→Stuck` paths carry their own detail that the mapping silently discards: `shuttleengine.Result.LastAssistantMessage` (set only for `OutcomeAsking`, `run.go:41,47`) and `*websterengine.MasterAskingError`'s `Message`/`SessionID`/`RunDir` (`runlevel.go:186-194`). Since the pointer is empty and the error is nil on those paths, nothing reaches the operator unless the discussion says so — the same gap r1 closed for `StuckReason`.
**Fix:** State explicitly whether the asking message/session/run-dir is logged (and by which adapter) or deliberately dropped, with the reason.

### [NIT:consistency] "Exact" doc set misses three claims it falsifies
**Demoted-from:** BLOCKING
**Section:** "Doc set — the exact edits, named line by line"
**Issue:** The enumeration is presented as exhaustive, and its own rationale is that naming every line prevents leaving a claim false the moment the package ships — yet three surviving claims become false: `docs/overview.md:294` ("the three engine adapters ... remain Planned"), `manifest/designs/shed.md:38` ("perch, Webster ... own their own error taxonomies and are not designed yet"), and `manifest/roadmap.md:16`'s "wired via the `perch` adapter above", whose "above" dangles once Planned item 1 moves to Done.
**Fix:** Add those three lines to the doc-set Decision (or state why each survives unchanged).

## Verdict

REQUEST_CHANGES
Perch's terminal run dir, asking-path detail, and three unnamed stale doc claims need decisions.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
