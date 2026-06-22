MILL_REVIEW_BEGIN
# Review: Extract internal/vscode; keep ide IDE-generic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-22
```

Scope, decisions, and constraints are all verifiable against source. Confirmed: only `cmd/lyx/main.go` imports `internal/ide`; no moved symbol (`writeVSCodeConfig`, `pickColor`, `launchCode`, `ErrIDEUnsupported`, `palette`, `mainColor`) is referenced outside `internal/ide`. `mainColor` is consumed only by `color_test.go:36,83` (verified) — `pickColor` skips green via `palette[1]` and never reads `mainColor`, so it correctly moves as test-visible state in a non-`_test.go` file. `cli_test.go` and `menu_test.go` both carry `//go:build integration` (line 1 each) and reference neither moved symbols nor the stub by moved name; `spawn_test.go` stubs `codeLauncher` only. Build tags on `launch_windows.go`/`launch_other.go` and the `ErrUnsupported` build-tag-neutral placement requirement are correctly flagged. Module path `github.com/Knatte18/loomyard` confirmed in `go.mod`. Exported signatures match current source. Path-invariant compliance holds — no moved code calls `os.Getwd` or shells to git. The r1 GAP (mandatory gate skips integration-tagged picker/dispatch) is resolved by an explicit, documented operator decision keeping `-tags integration` optional, justified by `spawn_test.go` covering the rewired `vscode.*` path under the mandatory gate.

## Verdict

APPROVE
Discussion is source-accurate, fully scoped, and ready for plan writing.
MILL_REVIEW_END