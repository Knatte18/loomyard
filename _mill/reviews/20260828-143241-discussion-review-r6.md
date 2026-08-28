MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
duration_s: 119.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] "Permanent" premise fails under rename-back
**Section:** `### the-watch-loop-terminates-when-its-anchor-is-gone` **Issue:** The parking rationale rests on "the condition is permanent … every subsequent tick would refuse identically", but the sandbox workflow this task is built around is *rename away → test refusal → rename back*: once the directory returns, the frozen `WorktreeRoot` is valid again, yet the watcher has already parked on `ctx.Done()` for the header pane's remaining life, leaving that session with no resize self-heal and no recovery short of restarting the pane (`ensureHeaderPaneLocked` only splits a pane that is absent). **Fix:** State the disposition explicitly — either accept it and say so (naming the recovery: the session's watchdog is dead until the header pane is restarted, and the `Warn` is the only notice), or specify a re-check/unpark rule — rather than asserting a permanence the rename-back case contradicts.

### [NIT:consistency] Q&A still claims standalone target "always present"
**Demoted-from:** BLOCKING
**Section:** `## Q&A log`, the "Gate on `AnchorPath` or `WorktreeRoot`?" entry **Issue:** It says `WorktreeRoot` is "the operator's real target repo in standalone mode (always present)", which the later Decision (`### the-predicate-is-worktreeroot-exists-not-anchorpath-exists`) and the later Q&A entry both flatly reject as false — verified: `standalonegeom.ReedGeometry` sets `WorktreeRoot = target` with no `Stat` anywhere upstream. **Fix:** Strike the superseded "(always present)" claim from that Q&A entry so the log does not contradict the Decision that supersedes it.

## Verdict

REQUEST_CHANGES
Watchdog-parking permanence premise is wrong under rename-back; one superseded Q&A claim remains.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
