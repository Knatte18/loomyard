MILL_REVIEW_BEGIN
# Review: Make producer engines runnable without a lyx worktree

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md → manifest/designs/producers-standalone.md + manifest/roadmap.md
date: 2026-08-17
```

## Findings

### [BLOCKING:design] T6 table omits reed/shuttle state dirs
**Section:** design doc, T6 "Every told value is pinned here" table
**Issue:** `reedengine`'s `stateDir()` is `Join(AnchorPath(), ".lyx")` (`internal/reedengine/lifecycle.go:43-44`, holding `reed.json` + `reed.lock`) and `shuttleengine.runDirRoot`'s default is `Join(AnchorPath(), ".lyx", "shuttle")` (`internal/shuttleengine/rundir.go:49-56`); with `anchorRoot` pinned to the target directory, both write a hidden `.lyx` tree into the reviewed folder — contradicting the same section's "the target directory receives only what the caller explicitly named" and its claim that the Durable-vs-Ephemeral invariant does not engage.
**Fix:** Add rows pinning reed state dir and shuttle run dir to `<state>/…`, or pin `anchorRoot` itself to `<state>` and pin `worktreeRoot` separately to the target.

### [BLOCKING:design] Per-invocation socketKey contradicts resume claim
**Section:** design doc, T6 table row "reed `socketKey`" and the `<state>/<hash8>` paragraph
**Issue:** `ReedState` persists `Socket`/`Session` (`internal/reedengine/state.go:32-36`) and reed's whole Up/Resume model reads them back, so a socket "minted per invocation, never derived from a directory" makes the persisted state unresumable and the deterministic `<basename>-<hash8>` session name decorative, while the same paragraph promises "a re-run against the same folder resumes its own state"; teardown of the per-invocation tmux server is also unstated.
**Fix:** Decide one: derive `socketKey` deterministically from the target's absolute-path hash (matching the session name and the state dir), or state explicitly that standalone runs are non-resumable and name who tears the server down.

### [BLOCKING:consistency] `--target-dir` unspecified and arguably out of scope
**Section:** design doc, T6 table row 1 vs "What is deliberately not in scope" (`lyx burler run <path>`)
**Issue:** `--target-dir` is introduced only inside a table cell with no stated semantics, is absent from T6's **Files**/**Watch** flag obligations (only `--stencils-dir` is named there), and the out-of-scope bullet rejects "a second way to say the same thing" about a directory — leaving an implementer unable to tell it apart from the profile's `target.paths`.
**Fix:** State that `--target-dir` is the relative-path resolution base for profile paths (`Profile.validate`'s `worktreeRoot`, `internal/burlerengine/profile.go:59-66`), not a review target, and add it to T6's flag/help obligations.

### [NIT:decision] T8 package placement left open
**Section:** design doc, T8 "Open for the implementer"
**Issue:** Whether the lifted preflight is a new `internal/preflight` or a `fabricengine` composite verb is deferred, with a recommendation but no decision.
**Fix:** Pin the new-package option; the rationale given already decides it.

### [NIT:consistency] Only one roadmap entry links the design doc
**Section:** `manifest/roadmap.md` producers-standalone entries; discussion §Scope ("four Planned entries pointing at that doc")
**Issue:** Only the wave-1 entry carries `See [designs/producers-standalone.md]`; the other three carry no link.
**Fix:** Either add the link to the remaining three or reword the discussion's Scope claim.

## Verdict

REQUEST_CHANGES
Standalone state locations and reed socket identity are underspecified and self-contradictory.
MILL_REVIEW_END
