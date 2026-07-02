{"status":"success","commit_sha":"7d980cb69895c131b61dd5f284260334fcd1b37f","session_id":"ea6365ec-5a23-434c-903b-6e8f314d3cb4"}

Summary of work:

- `C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry.go` — added unexported `dotLyxDirName = ".lyx"` constant next to `LyxDirName = "_lyx"`, and added `func (l *Layout) DotLyxDir() string` returning `filepath.Join(l.Cwd, dotLyxDirName)`, mirroring the existing `LyxDir()` accessor. `LyxDir`/`LyxDirName` were not modified.
- `C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry_unit_test.go` — added `TestDotLyxDir`, asserting the returned path equals `filepath.Join(Cwd, ".lyx")` and that it is distinct from `LyxDir()`.
- Committed as `7d980cb` (`feat(hubgeometry): add DotLyxDir accessor for ephemeral .lyx`) and pushed to `internal-mux`.
- Batch verify `go test ./internal/hubgeometry/...` passes (includes existing `enforcement_test.go`/`geometry_test.go` guards plus the new unit test).
- Pre-existing repo-wide `gofmt -l` drift (CRLF-related, flags nearly every file in the package including untouched ones, confirmed via `git stash` before my edits) is out of scope for this batch and was left untouched per the isolation rule.

{"status":"success","commit_sha":"7d980cb69895c131b61dd5f284260334fcd1b37f","session_id":"ea6365ec-5a23-434c-903b-6e8f314d3cb4"}
