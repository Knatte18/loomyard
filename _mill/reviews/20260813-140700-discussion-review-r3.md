MILL_REVIEW_BEGIN
# Review: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
duration_s: 244.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build unverifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:design] RunRoot change left as two alternatives, and contradicts main.go
**Section:** Scope (`clihelp.ExecuteIn` bullet) + Decision `runcli-gains-a-sibling-rather-than-changing`
**Issue:** Scope says `RunRoot` "gains a parent-context parameter (or a `RunRootCtx` sibling)" — an unchosen alternative — and the parameter option contradicts the decision's rationale "`cmd/lyx/main.go` needs no change": `main.go:42` and `main.go:54` are the only two `clihelp.RunRoot` call sites in the repo and both would have to change.
**Fix:** Pick one shape explicitly, and state in commit 1 whether `cmd/lyx/main.go`'s two `RunRoot` call sites are touched.

### [BLOCKING:design] Injected cwd does not govern relative path args in scoutcli
**Section:** Scope (`RunCLIIn` on the 10 modules) + Decision `clone-gets-an-explicit-destination`
**Issue:** The discussion states the relative-base trap for clone's `--into` only, but `scoutcli` resolves relative flag/positional values against the *process* cwd independently of `lyxcwd.Getwd()` — `cli.go:446` (`filepath.Abs(targetDir)`), `:688-695` (`filterWithin`, whose own comment says "`filepath.Abs` resolves whatever remains against the process's actual working directory"), and `:784`/`:800` (`parseQuery`/`--in-file`). Swapping its 4 `Getwd()` sites to `CwdFrom` therefore yields a `scoutcli.RunCLIIn` that honours the injected cwd only for lookup, not for a relative `--target-dir`/`--within`/`file:line:col`.
**Fix:** State the seam's contract — what `RunCLIIn`'s cwd does and does not govern — and give `scoutcli`'s relative bases an explicit disposition (rebase on the seam cwd, or documented as out of scope).

### [NIT:decision] Contract of the injected cwd value is unstated
**Section:** Decisions `seam-mechanism-is-context-carried-cwd`, `runcli-gains-a-sibling-rather-than-changing`
**Issue:** `RunCLI` delegating with `cwd == ""` makes empty a sentinel, but the discussion never says whether `WithCwd(ctx, "")` is legal or whether the cwd must be absolute; `lyxcwd.Resolve` passes cwd to `gitexec.RunGit` as `cmd.Dir` and gates on an `EvalSymlinks`-normalised absolute comparison (`anchor.go:103-140`), so a relative value would resolve against the process cwd and likely trip `ErrCwdOutsideAnchor`.
**Fix:** State that the injected cwd must be absolute and that `""` alone means "read the process cwd".

### [NIT:consistency] Line/verb citations drift by one in the configcli notes
**Section:** Decision `nested-cli-calls-thread-the-callers-cwd`; Technical context per-call-site notes
**Issue:** The nested call is at `configcli.go:383` (not `:384`); the injected closure is at `configcli_integration_test.go:72-74` and invokes `fabriccli.RunCLI(w, []string{"commit"})`, not `"sync"`; `dispatch` is called at `:78`, not `:80`.
**Fix:** Correct the three citations so the plan writer's grep lands on the right lines and verb.

## Verdict

REQUEST_CHANGES
Two open design points: the unchosen `RunRoot` shape, and scoutcli's process-cwd relative-path bases.
MILL_REVIEW_END
