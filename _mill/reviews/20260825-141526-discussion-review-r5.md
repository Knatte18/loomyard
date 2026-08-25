MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write

```yaml
duration_s: 212.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Reed Status error has no attach disposition
**Section:** `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`
**Issue:** The decision enumerates "reed's three answers" (tracked+live, not-tracked, binding-cleared) plus the state-file absent/unreadable pair, but liveness necessarily comes from `ReedOps.Status()`, whose `requireSessionLocked` (`internal/reedengine/lifecycle.go:1144`) returns an error for a torn-down/foreign tmux session, and `listPanes` for a tmux fault — a fourth answer with no stated disposition, reachable exactly on the reboot-crash resume this feature exists to serve.
**Fix:** State the disposition for a `Status()` error (and for a `Status()` that succeeds but disagrees with the `LoadState` table), and say explicitly which of the two reed reads answers "tracked" versus "absent".

### [NIT:consistency] Q&A log's final entry contradicts the decisions
**Demoted-from:** BLOCKING
**Section:** Q&A log, last entry vs `resume-discriminates-on-live-agent-evidence-only` and `leftover-run-dir-from-a-completed-run`
**Issue:** The Q&A says ladder step 1 "buys nothing — a run whose output files are complete already reached `Done` and cleaned itself up", which the decision body explicitly calls the wrong earlier phrasing (step 1 buys the completed-crash window) and which `Run.finalize`'s `KeepPane` skip plus best-effort `RemoveStrand`/`RemoveAll` (`wait.go:405-413`) falsify.
**Fix:** Rewrite that Q&A answer to match the decision — step 1 is dropped because the bounce ping-pong outweighs the narrow window it would rescue, not because it buys nothing.

### [NIT:consistency] Absent reed.json errors forever, unlike untracked
**Section:** `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`
**Issue:** The untracked/binding-cleared answers get an age escape precisely because "erroring unconditionally deadlocks resume permanently", yet the absent-state-file answer is an unconditional error at any age — the same deadlock, with no in-band escape named (the real one, `lyx reed up` repopulating the table so the age rule applies, is unstated).
**Fix:** Reconcile the asymmetry in one sentence, naming `lyx reed up` as the in-band recovery, or give the absent case the same age rule.

### [NIT:decision] Negative-Timeout check disposition left as "may"
**Section:** `attach-normalizes-the-spec-it-matches-on`
**Issue:** "Its negative-`Timeout` rejection is worth keeping in spirit; the plan may reuse it" leaves a named `Spec.validate` check (`spec.go:150-152`) without a decided disposition for the attach path.
**Fix:** Decide it — either `Attach` rejects a negative `Timeout` or it does not.

## Verdict

REQUEST_CHANGES
One undispositioned reed answer and one superseded Q&A claim need closing before plan writing.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
