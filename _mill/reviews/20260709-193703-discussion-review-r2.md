I have verified the key claims. All technical-context line references and file states are accurate. I found one structural gap in the shell-abstraction decision plus a couple of robustness notes.

MILL_REVIEW_BEGIN
# Review: Facilitate Linux support (Win11-side prep)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-09
```

## Findings

### [GAP] PosixPath folds into GOOS-selected posix shell, but is Windows-active
**Section:** Decisions › shell-abstraction
**Issue:** The decision folds `PosixPath` "into the posix shell impl (a no-op where paths are already POSIX)" while the shell family is selected by `runtime.GOOS` — but `PosixPath` is called unconditionally on Windows (`claudeengine.go:97`, `Prepare`) to convert the events path for the git-bash hook in `settings.json`; on Windows GOOS selects the pwsh family, so a GOOS-keyed posix impl would not be reachable there and the hook-path conversion regresses.
**Fix:** Distinguish the GOOS-keyed pane-shell family (pwsh on Windows) from the hook interpreter (git-bash/posix on Windows regardless); state where the Windows-active hook-path conversion lives and that it stays reachable when the pane shell is pwsh.

### [NOTE] /proc/<pid>/stat comm-field parsing pitfall not covered
**Section:** Testing › /proc descendant-closure; Decisions › proc-tree-reaping
**Issue:** PPID is "field 4," but field 2 (comm) is parenthesized and may contain spaces/parens, so a naive whitespace split reads the wrong PPID and corrupts the closure — the exact "no stray process" guarantee at stake; the enumerated fixtures omit this case.
**Fix:** Specify parsing from the last `)` and add a fixture with a comm like `(a) b`.

### [NOTE] Linux serverProcessesOnSocket assumes server argv retains `-L <socket>`
**Section:** Decisions › proc-tree-reaping (`serverProcessesOnSocket` Linux)
**Issue:** The `/proc/*/cmdline` scan matches argv containing the tmux binary + `-L <socket>`, but tmux's server often rewrites its process title (e.g. `tmux: server`) and may not retain the `-L` token, so the stray-process backstop could silently miss the real server; live validation is deferred so the fixture may encode a wrong assumption.
**Fix:** Note this as an assumption to confirm in the deferred follow-up; liveness still rests on the CLI absence signal.

## Verdict
GAPS_FOUND
Shell-abstraction decision misplaces the Windows-active git-bash hook-path conversion under a GOOS-keyed posix selector.
MILL_REVIEW_END
