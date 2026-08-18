MILL_REVIEW_BEGIN
# Review: the standalone CLI path

```yaml
duration_s: 182.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] ResolveMode refuses inside a plain repo's subdirectory
**Section:** mode-trigger, part 2
**Issue:** In an ordinary downloaded repo with no lyx geometry, `readRecordedAnchor` finds no marker so `anchorRel` is `"."` (`internal/lyxcwd/lyxcwd.go:117-128`) and the cwd gate (`checkCwdAnchorGate`, `anchor.go:134-141`) returns `ErrCwdOutsideAnchor` from **any subdirectory** — so `lyx burler run` under `~/src/somerepo/pkg/` is refused with "cwd is outside the recorded fabric anchor subtree (recorded by .lyx-anchor)", naming a marker that does not exist, rather than running standalone; the decision's rationale treats `ErrCwdOutsideAnchor` as exclusively "healthy hub, wrongly entered", and line 88's "what remains is only a hub damaged precisely at `<hub>/_board/_lyx`" is therefore false.
**Fix:** Decide and record what `ResolveMode` does when `Resolve` fails `ErrCwdOutsideAnchor` at a location with no lyx geometry (e.g. discriminate on marker presence, or accept the refusal explicitly as a documented standalone limitation with an error message that does not name `.lyx-anchor`).

### [NIT:consistency] Byte-identity exception count contradicts itself
**Demoted-from:** BLOCKING
**Section:** hub mode is byte-identical / Scope / Q&A log
**Issue:** The decision says "exactly **three** intentional deviations, all named here", while Scope line 48 says "which names both exceptions" and the Q&A log line 432 says "with exactly **two** named exceptions" — and the decision's own closing rule ("any fourth deviation is a bug in this plan") is only checkable against a single agreed count.
**Fix:** Make all three sites say three, since the enumeration itself lists three.

### [NIT:consistency] Constraints/Testing sections still specify the superseded HubPresent trigger
**Demoted-from:** BLOCKING
**Section:** Constraints; Testing; Technical context
**Issue:** After the part-2 decision the trigger is `preflight.ResolveMode` and `wire` takes `mode preflight.Mode`, but Constraints line 328 still names `preflight.HubPresent` as one of "the only entry points", line 333 says `wire` is extracted "with `hubPresent` as a parameter", Testing line 352 drives `wire` with a told `(loc, hubPresent)` pair, and Technical context line 252 states "Only `HubPresent` is used here" — an implementer working from those sections would build the old trigger.
**Fix:** Repoint all four passages onto `ResolveMode`/`preflight.Mode`, and add the `Mode` type itself (constants plus its zero value used by the `(nil, 0, err)` refuse return) explicitly to the `internal/preflight` scope row.

### [NIT:decision] burlercli's new integration test spawns git with no hermetic TestMain
**Demoted-from:** BLOCKING
**Section:** Gotchas; Testing
**Issue:** The new `internal/burlercli/cli_integration_test.go` drives `RunCLIIn`, which reaches `ResolveMode` → `lyxcwd.Resolve` → a real `git rev-parse` spawn, in a package the discussion itself notes "has no `TestMain` at all"; `internal/webstercli` carries `testmain_test.go` with `gitkit.HermeticGitEnv()` for exactly this, and the discussion dispositions only the state-directory half of burlercli's isolation gap, leaving the git-config half unstated (the Hermetic Git Test Environment guard is token-keyed and will not catch an indirect spawn).
**Fix:** State whether `internal/burlercli` gains a `TestMain` calling `gitkit.HermeticGitEnv()` in this task, and add the file to the edit table if so.

### [NIT:decision] Perch's standalone nested burler geometry unnamed
**Section:** perch's three `layout` uses; geometry
**Issue:** `perchcli` builds a nested burler engine with `hubgeom.BurlerGeometry(layout)` today (`internal/perchcli/cli.go:152`); the discussion names the told `stencilsDir` reaching both consumers but never says which `burlerengine.Geometry` `wireStandalone` passes to that nested `burlerengine.New`.
**Fix:** State explicitly that perch's standalone branch builds it via `standalonegeom.BurlerGeometry(target, stateDir)`, the same values burlercli uses.

## Verdict

REQUEST_CHANGES
A refuse-path design gap plus two self-contradictions and one unstated test-isolation disposition.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
