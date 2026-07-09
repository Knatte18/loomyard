MILL_REVIEW_BEGIN
# Review: Facilitate Linux support (Win11-side prep)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-09
```

## Findings

### [GAP] Capability probe: no mechanism for subcommand/format-var check
**Section:** Decisions → capability-probe
**Issue:** The probe is specified to "verify the required subcommands and `#{pane_*}` format vars are supported," but only the `-V` version parse has a mechanism; detecting an unsupported subcommand or format var at boot is non-trivial (typically needs a live pane, i.e. what the integration contract test already does) and no method is chosen.
**Fix:** Decide whether the boot probe is version-pin-only (delegating subcommand/format verification to the `//go:build integration` contract test) or performs a live check, and if the latter, name the mechanism (e.g. `list-commands` + a `display-message` format query against the just-ensured server).

### [NOTE] Posix pane-shell command form left as two alternatives
**Section:** Decisions → shell-abstraction
**Issue:** The posix chain is written as `<bin> "$(cat <file>)"` / direct-exec form — an unresolved "or" that is load-bearing, since it must reproduce the pwsh single-argument prompt semantics (the whole reason for `Get-Content -Raw` + `maxLaunchPromptBytes`); an unquoted or direct-exec variant changes prompt word-splitting and where the prompt file is read.
**Fix:** Pin the posix form (and confirm single-quoted-substitution single-arg semantics) in the discussion or explicitly delegate the exact form to mill-plan's TDD with the single-arg contract stated as fixed.

### [NOTE] Cross-compile "CI gate" has no home in-repo
**Section:** Testing → seamed-package verification
**Issue:** The `GOOS=linux go build ./...` gate is named as "the mechanical proof the whole tree builds for Linux," but the repo has no CI workflow, Makefile, or build script (no `.github/`, no Makefile found) — every other invariant is `go test`-enforced, and a build is not a `go test`.
**Fix:** State where the gate lives — a new workflow file, a Makefile/task target, or a test that shells `go build -o /dev/null` under `GOOS=linux`.

### [NOTE] Linux `waitProcessExit` is a no-op for non-child pids
**Section:** Technical context / Decisions → proc-tree-reaping
**Issue:** `waitProcessExit` (lifecycle.go:726-729) returns immediately for a non-child process on non-Windows, so on Linux `ensureServerGoneLocked`'s pid-wait is a no-op and the entire "confirm gone" guarantee falls on the `/proc`-polling `waitServerProcessesGone`; this Linux behavioral shift in the reap flow isn't surfaced alongside the closure/cmdline-scan seams.
**Fix:** Note in the proc-tree decision that Linux reap-confirmation relies solely on the `/proc` drain (pid-wait inert), so the drain path must be complete, not belt-and-suspenders.

## Verdict
GAPS_FOUND
One unresolved probe mechanism blocks planning; three NOTEs worth recording, none blocking.
MILL_REVIEW_END
