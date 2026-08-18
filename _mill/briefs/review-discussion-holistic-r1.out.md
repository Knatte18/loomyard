MILL_REVIEW_BEGIN
# Review: the standalone CLI path

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] standalone stencils dir has no owner
**Section:** "stencils bootstrap" + "geometry"
**Issue:** The decision prescribes `stencilstore.Reconcile(geom.StencilsDir, ...)`, but `burlerengine.Geometry` is `{WorktreeRoot, AnchorPath}` and `perchengine.Geometry` is `{GateDir, AnchorPath}` — neither has a `StencilsDir` field (verified `internal/burlerengine/geometry.go:12`, `internal/perchengine/geometry.go:10`); the webster snippet works only because `websterengine.Geometry` carries one. Nothing in the discussion says who constructs the `<state>/_lyx/stencils` default for burler/perch.
**Fix:** Decide and name the single construction site (e.g. a `standalonegeom.StencilsDir(stateDir)` helper vs. a per-CLI local using `lyxdirs.LyxDirName`), and correct the Reconcile snippet to a told `stencilsDir` string rather than `geom.StencilsDir`.

### [BLOCKING:scope] two existing untagged tests flip into standalone
**Section:** "Testing" → regression coverage
**Issue:** `internal/burlercli/cli_test.go:81` (`TestRunCLI_Run_MissingProfile`) and `internal/perchcli/cli_test.go:99` (`TestRunCLI_Pause_MissingRunID`) are untagged tests that `t.Chdir(t.TempDir())` and drive a real subcommand, so their pre-run currently aborts; after this change they enter `wireStandalone`, call `standalonestate.Derive` against the operator's live `XDG_STATE_HOME`/`HOME` and Reconcile a stencils tree there. Webster avoided this because its equivalent chdir tests only hit the group guard. Neither test appears in the regression list nor in the `Derive`-redirect gotcha.
**Fix:** Name both tests explicitly and state their disposition (redirect the state root via `t.Setenv`, or restate the expected output shape), since their doc comments' "double-failure shape" rationale also goes stale.

### [BLOCKING:consistency] envelope fields contradict "hub mode byte-identical"
**Section:** "operator visibility" vs. "hub mode is byte-identical" / Scope "Out"
**Issue:** Scope-Out says "Any change to hub-mode behaviour. Hub mode must stay byte-identical," and the byte-identical decision names only one intentional exception (the plain-git-repo case), yet `mode`/`stateDir`/`stencilsDir` are added to *both* run verbs' success envelopes in both modes — an observable hub-mode output change and a second exception.
**Fix:** State whether the three fields appear in hub mode too, and record it as the second named exception to byte-identity (or scope the fields to standalone only).

### [NIT:consistency] burlercli receiver contents contradict envelope plumbing
**Section:** "burler's stack" vs. "operator visibility"
**Issue:** "`burlercli` stores only `c.engine *burlerengine.Engine`" cannot hold while `resultEnvelope` takes `mode`/`stateDir`/`stencilsDir` values that `run.go` can only read off the receiver; the file table already says burlercli's `cli.go` gains "receiver fields".
**Fix:** Restate the burler decision as "no `openFabric`, no `anchorRel`, no `Location`" and list the three reporting fields the wiring branches set.

## Verdict

REQUEST_CHANGES
Three unresolved points: stencils-dir ownership, two flipping untagged tests, and hub-envelope scope.
MILL_REVIEW_END
