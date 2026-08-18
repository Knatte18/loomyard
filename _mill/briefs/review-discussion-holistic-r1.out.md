MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] worktree_root renders `<state>` in standalone
**Section:** `worktree-root-token-value-preserved` + `the-two-roots`
**Issue:** `{{.worktree_root}}` is where the implementer runs `go build`/tests and commits (`contracts/stencils/webster/webster-body-implementer.md:28,31`) and where the integration fork runs verify; filling it from `anchorRoot` is today's hub value, but in standalone `anchorRoot` is `<state>`, so both prompts would point the agent at the hidden state directory instead of `--target-dir`.
**Fix:** state that the preserve-today's-value ruling binds hub mode only, and that standalone fills the token from `WorktreeRoot`, with the standalone case pinned by a test.

### [BLOCKING:design] Standalone panes are spawned in `<state>`, not the target
**Section:** `new-package-internal-standalonegeom` / `standalone-has-no-fabric`
**Issue:** `reedengine` spawns every pane with `-c geom.AnchorPath` (`internal/reedengine/lifecycle.go:294,489`) and the pinned standalone `AnchorPath` is `<state>`, so Master and every fork start with cwd `<state>`; the same value is the audit workdir (`recordbatch.go:102`). The discussion never says how a standalone fork reaches the directory it is meant to edit.
**Fix:** decide and record whether standalone reed `AnchorPath` stays `<state>` (and the prompt/`cd` mechanism that gets the agent to the target) or the pane cwd is told separately.

### [BLOCKING:consistency] Master prompt carries no `worktree_root`
**Section:** `worktree-root-token-value-preserved`
**Issue:** The decision says the token is preserved "in the fork, recovery and Master prompts", but `RenderMasterPrompt` (`render.go:230-251`) renders no `worktree_root` key at all — the discussion's own Technical Context correctly lists only `149` and `183`.
**Fix:** drop Master from that sentence so the plan does not add a token to `webster-template-master`.

### [BLOCKING:consistency] Mode decision sits outside the function its test drives
**Section:** `mode-selection-and-the-extracted-wiring-function` vs Testing
**Issue:** The decision keeps "the mode decision" in `PersistentPreRunE` and gives the extracted function "the resolved-or-not state as a parameter", yet Testing requires the untagged unit test over that extracted function to cover the four-row mode-selection truth table — which it cannot if the decision is made upstream of it.
**Fix:** state explicitly that the extracted function computes (and returns) the mode from told predicate results, leaving only `CwdFrom` + predicate calls in the pre-run.

### [BLOCKING:design] `_board` discriminator has no Location to derive from
**Section:** `mode-selection-and-the-extracted-wiring-function`
**Issue:** `preflight.Wired` returns `(nil, false)` on both the unwired and unresolvable branches (`internal/preflight/predicates.go:30-40`), so `fabricengine.BoardDir(filepath.Dir(worktreeRoot))` has no `worktreeRoot` available; and if `worktreeRoot` is webster's hub value `AnchorPath()`, `filepath.Dir` is not `HubPath` in a subpath-anchored hub. `preflight.HubPresent` already answers exactly this question.
**Fix:** name `preflight.HubPresent(cwd)` (or an explicit second `lyxcwd.Resolve`) as the discriminator instead of hand-derived hub arithmetic.

### [NIT:consistency] Only `run` emits `fabricCommitted`
**Section:** `standalone-has-no-fabric`
**Issue:** "each verb's `fabricCommitted` output field reports `false`" — only `run.go:106` has that field; `beginbatch.go:136`, `recordbatch.go:135`, `recoverbatch.go:155,189` discard the bool.
**Fix:** scope the sentence to `run` and say the other three simply skip the call.

### [NIT:consistency] Geometry field count stated three ways
**Section:** `webster-geometry-struct`
**Issue:** "seven told values" is followed by seven names "plus `PlanDir`" (eight), while the rationale says "the same six values".
**Fix:** fix the count to the eight fields actually listed.

### [NIT:scope] Largest Location-fixture test file unnamed
**Section:** Testing → `internal/websterengine`
**Issue:** `template_test.go` holds `testLayout`/`patternActiveLayout`/`patternActiveMissingPatternStencilsLayout` and is the main consumer of the render functions' `*lyxcwd.Location` parameter, but only `webstergeom_test.go` is named for rewrite.
**Fix:** name `template_test.go` alongside it so the render-fixture conversion is inventoried.

## Verdict

REQUEST_CHANGES
Standalone geometry misroutes the agent's working directory; three specification contradictions need resolving first.
MILL_REVIEW_END
