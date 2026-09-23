MILL_REVIEW_BEGIN
# Review: Shuttle guarantees a started run is past its startup gates

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Torn-down run dir makes the next Attach refuse
**Section:** Decisions "Not-ready start" (steps 2–4). **Issue:** A kept run dir whose strand was removed is untracked in reed, and `dispositionCandidate` (`internal/shuttleengine/attach.go:424–439`) routes an untracked candidate to `leftoverThenAgeVerdict` without consulting its persisted `Outcome`. That function returns `verdictError` while the dir is younger than `2 × startup_timeout_s` (`attach.go:448–456`, and the teardown's own `run.json`/capture writes refresh the dir mtime). So any `Attach`/`AttachGated` probe on the same output files inside that window hard-errors with "cannot be confirmed dead or alive": `shedadapters` burler's `probeLiveRound`, `SingleLLMProducer`, and the `bouncer.go` sites all make that call on resume. Today a startup `OutcomeDied` leaves the strand tracked and live with a terminal Outcome, which is `verdictRespawnEligible` (`attach.go:402–409`). The claim that a persisted Outcome keeps Attach from treating the record as attachable holds, but the record becomes a blocking error instead of respawn-eligible, and neither Scope nor Testing covers this interaction. **Fix:** Decide how the not-ready teardown's record reads to `dispositionCandidate`: for example, a terminal persisted Outcome on an untracked candidate short-circuits to respawn-eligible, the teardown removes the dir after all, or the regression is accepted and stated. Add an Attach-after-not-ready-start test.

### [NIT:consistency] RunGated decision names only OutcomeDied
**Section:** Decisions "RunGated preserves its OutcomeDied contract"; Q&A "What happens to a started run…". **Issue:** Both say the not-ready Result and persisted Outcome are `OutcomeDied`/`died`, but "Run deadline anchoring" later adds `OutcomeTimeout`/`timeout`. **Fix:** Reword both places to say "`OutcomeDied`, or `OutcomeTimeout` on run-deadline expiry".

### [NIT:decision] KeepPane not addressed for the not-ready teardown
**Section:** Decisions "Not-ready start". **Issue:** `Spec.KeepPane` governs strand removal only after a done outcome (`spec.go:112–114`), and the discussion never says whether the new unconditional `RemoveStrand` ignores it. **Fix:** State that the not-ready teardown ignores `KeepPane`.

## Verdict

REQUEST_CHANGES
The not-ready teardown turns the kept run dir into an Attach `verdictError` for up to 2 × startup_timeout_s.
MILL_REVIEW_END
