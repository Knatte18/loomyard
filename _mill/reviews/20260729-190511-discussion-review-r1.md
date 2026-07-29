MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] marketplace `source` value unresolved for version subdir
**Section:** plugin-placement-and-marketplace / Technical context
**Issue:** The plugin is placed at `plugins/prowler/1.0.0/` (so `.claude-plugin/plugin.json` lives there), but the cited template shape is `source: ./plugins/<name>`; the discussion never states the actual `source` value, and `./plugins/prowler` would not point at the plugin dir and would break install.
**Fix:** State explicitly that `marketplace.json`'s `plugins[].source` is `./plugins/prowler/1.0.0` (the dir containing `.claude-plugin/plugin.json`), and confirm Claude Code resolves `source` to that exact path.

### [GAP] stdout path-capture vs wrapper build diagnostics
**Section:** build-on-first-run / output-to-unique-scratch-file / skill-contract
**Issue:** The skill captures `path=$(run.sh <urls>)`, but the wrapper also runs `go build` and emits a toolchain-missing message; any of that reaching stdout is captured as the "path" and corrupts the read.
**Fix:** Require all wrapper diagnostics (build progress/errors, "Go toolchain not found") to go to stderr, and mandate the skill check the wrapper exit code before reading the path.

## Findings (NOTE)

### [NOTE] "mirrors Millhouse `<version>/` layout" is factually wrong
**Section:** plugin-placement-and-marketplace (Rationale)
**Issue:** Millhouse's actual repo layout is flat (`plugins/weblens/`, `plugins/mill/`, etc. — verified), with marketplace `source: ./plugins/<name>`; it does NOT use a `<version>/` subdir. The `<version>/` cache path exists only under `~/.claude/plugins/cache/`, which is the install cache, not repo source.
**Fix:** Correct the rationale — the version subdir is a deliberate divergence from Millhouse convention (for future versions), not a mirror of it.

### [NOTE] Windows wrapper selection in skill contract unspecified
**Section:** skill-contract / Cross-platform note
**Issue:** The skill shows only `run.sh`, but a `run.ps1`/`run.cmd` is required on Windows; how the skill selects the correct wrapper per-OS is left open (weblens sidesteps this by invoking `bash …/run.sh`).
**Fix:** State whether Windows uses git-bash `bash run.sh` (single wrapper) or a separate PowerShell invocation, and pin the skill's invocation line accordingly.

### [NOTE] Cross-platform lock mechanism undefined
**Section:** build-on-first-run
**Issue:** "under a lockfile" is asserted but the mechanism (e.g. `flock` POSIX vs a Windows equivalent) is unspecified, and races/stale-lock recovery are not addressed.
**Fix:** Name the concrete locking primitive per platform and the stale-lock/failed-build recovery behavior.

## Verdict

GAPS_FOUND
Two install/capture-breaking gaps and a false layout-rationale need resolving before plan writing.
MILL_REVIEW_END
