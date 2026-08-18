MILL_REVIEW_BEGIN
# Review: the standalone CLI path — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Cards 18/24 prescribe an unwritable reed-geometry assertion
**Location:** batch 4 card 18 (`internal/burlercli/wiring_test.go`); batch 5 card 24 (`internal/perchcli/wiring_test.go`)
**Issue:** Both cards require asserting "the reed geometry's SocketKey, SessionName, LogsDir, RepoName and HubPath match `standalonegeom.ReedGeometry(target, stateDir, hash8)`'s values," but neither receiver stores a `reedGeom` field (burlerCLI's post-card-14 inventory is `engine, stencilsDirFlag, targetDirFlag, mode, stateDir, stencilsDir`; perchCLI stores only `c.runner *shuttleengine.Runner`), `*burlerengine.Engine` exposes only `Run` (no geometry accessor), and `shuttleengine.Runner`'s fields (`reed, engine, anchorPath, worktreeRoot, cfg`) are all unexported with no accessor — verified by reading `internal/burlerengine/engine.go` and `internal/shuttleengine/run.go`. The card's own fallback ("assert through `c.mode`/`c.stateDir`/`c.stencilsDir`... rather than inventing an accessor on the engine") cannot cover these specific fields since none of those three reporting fields encode SocketKey/SessionName/LogsDir/RepoName. `webstercli`'s existing `wiring_test.go` (the file both cards are told to mirror) never asserts reed-geometry field values either, confirming this is a genuinely new, unreachable obligation rather than an established pattern.
**Fix:** Drop the reed-geometry field-value assertions from cards 18 and 24 (leaving the burler/perch-geometry and config-base assertions, which are observable), or add an explicit accessor/reporting field carrying the reed geometry so the assertion becomes writable, and say which.

### [NIT:consistency] Card 17 and card 29 contradict each other about the hermetic-git guard
**Location:** batch 4 card 17; batch 6 card 29
**Issue:** Card 17 states the Hermetic Git Test Environment guard "is token-keyed and will not catch a spawn reached indirectly through a CLI seam, so nothing else would flag the gap" — correct per `cmd/lyx/hermeticenv_test.go`'s raw-substring `gitSpawnTokens` scan (`gitexec.Run`, `exec.Command`, `gitkit.Copy*`, `hubforge.NewHub`), none of which appear anywhere in burlercli's post-batch-4 test files (mirrored from webstercli's token-free `wiring_test.go`/`cli_integration_test.go`/`testmain_test.go`). Card 29 then asks the implementer to "confirm the guard sees it" for `internal/burlercli` — but the guard's own mechanism never flags burlercli as git-spawning at all (no matching token anywhere in the package), so there is nothing for the guard to "see," contradicting card 17's own accurate description.
**Fix:** Reword card 29's bullet to say the suite passes trivially for burlercli (the guard never flags it, per card 17's documented blind spot) rather than asking the implementer to "confirm the guard sees it."

## Verdict

REQUEST_CHANGES
Cards 18/24's reed-geometry assertion is unwritable given the plan's own field inventory; fix before implementation.
MILL_REVIEW_END
