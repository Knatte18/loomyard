MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Commit-routing decision cites the wrong call site
**Section:** `### PATTERN content joins _lyx commit routing — intended`
**Issue:** The rationale claims "every round-loop caller commits with `ScopedPathspec(l.AnchorRel, ["_lyx"])` (`internal/fabriccli/weft_verbs.go:102`)", but that line passes `fabricengine.PathspecNames(...)` — today `{_lyx, _pattern}` — so the one cited caller is the one that already sweeps `_pattern`; `internal/fabricengine/doc.go:53-55` states outright that `PATTERN.md` written through the `_pattern` junction "is staged and committed alongside `_lyx` by the same `commitWeft` call".
**Fix:** Re-cite the genuine `["_lyx"]` callers (`internal/webstercli/sync.go:22`, `internal/buildercli/sync.go:25`, `internal/perchcli/run.go:334`), and restate the premise as "the round-loop callers could not sweep PATTERN; `lyx fabric sync` already did" so the "Consequence to expect" paragraph is re-derived from the true baseline.

### [GAP] `fabricengine/doc.go`'s pathspec passage is under-scoped as "comments only"
**Section:** `## Technical context` → `internal/fabricengine` (line 250) and line 249
**Issue:** Only `doc.go:84` and the "today just `_pattern`" clause are enumerated, but `doc.go:53-92` is a ~40-line passage asserting the default is `_lyx _pattern`, that PATTERN.md is staged alongside `_lyx`, the whole "narrow-pathspec asymmetry" gap (`:57-73`, "every already-initialised worktree stays on `pathspec: _lyx` forever and never persists `_pattern` content"), and the "append a name to `pathspec:` to wire a future optional module" worked example — all false or subject-less once the default is empty.
**Fix:** Enumerate `doc.go:53-92` explicitly as a rewrite (not a token substitution), and state what the narrow-pathspec-asymmetry passage becomes when the default names nothing.

### [GAP] Q&A log still carries the reversed `raddle_guard_test.go` answer
**Section:** `## Q&A log` line 408 vs line 426 and `### _raddle un-reservation keeps a positive guard test`
**Issue:** Line 408 answers "Keep, repurposed as a positive guard that `_raddle` is not reserved" — exactly the action the r4 decision forbids as import-cycle-illegal and destructive of a live tree-scan guard; two contradictory answers to the same question sit in the same log with no marker on the stale one.
**Fix:** Strike or annotate line 408 as superseded so the plan writer cannot read the repurpose instruction as live.

## Verdict

GAPS_FOUND
Commit-routing rationale rests on a misread call site; `fabricengine/doc.go` under-scoped; stale Q&A entry.
MILL_REVIEW_END
