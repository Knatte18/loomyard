MILL_REVIEW_BEGIN
# Review: Master Builder: new, parallel fork-based implementation module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Prior-batch digest has no persistence home
**Section:** state-schema vs fork-prompt-go-rendered
**Issue:** `begin-batch` must render "the immediately preceding batch's recorded digest line" into batch N+1's fork prompt, but `begin-batch` and `record-batch` are separate CLI processes and `BatchState` lists no digest field (builder never persists `Digest` either — it lives only in the JSON envelope returned to the agent). Nothing on disk carries digest N forward to begin-batch(N+1).
**Fix:** Decide the mechanism explicitly — add a persisted digest/digest-line field to `BatchState`, or state that `begin-batch` re-parses the preceding report and re-`Distill`s it (and note the gitquery `changed/scope/dirty` re-derivation that implies).

### [GAP] Per-batch audit assumes fork transcript is on-disk at record-batch time
**Section:** fork-audit-policy / audit-forks-extension
**Issue:** The incremental audit and its load-bearing "zero new fork transcripts = hard error" run inside `record-batch`, immediately after the Agent fork returns. Today's audit runs only at `OutcomeDone` (`wait.go:329-335`), long after forks; moving it per-batch introduces a new, unstated assumption that Claude Code has flushed `subagents/<id>.jsonl` to disk by the instant the tool call returns. A not-yet-flushed transcript would false-fire the hard error on a legitimate batch.
**Fix:** State the flush-timing assumption and how it is de-risked (a bounded retry/settle, or a pinned sandbox check like the `/model` one), rather than leaving it implicit.

### [NOTE] Fork-prompt return shape left as path-or-inline
**Section:** fork-prompt-go-rendered
**Issue:** `begin-batch` "returns it (path or inline in the envelope)" leaves the delivery form undecided; a plan writer could pick either and the Master template contract depends on which.
**Fix:** Pick one (path vs inline) or state it is an implementation detail deliberately deferred.

## Verdict

GAPS_FOUND
Two load-bearing mechanisms (digest carry-forward, transcript flush timing) are unspecified.
MILL_REVIEW_END
