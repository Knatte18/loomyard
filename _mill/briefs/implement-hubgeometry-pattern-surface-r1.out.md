{"status":"success","commit_sha":"3481e9c7","session_id":"ec16ed78-b26a-487f-8b3b-a64d796209bd","cards_done":[3,4,5]}

All 3 cards of batch `02-hubgeometry-pattern-surface` are committed (3 of 3 — full completion, honestly verified against `git log fcb1fb8d..HEAD`):

- Card 3 (`9ea808e4`): added `PatternDirName` const, free `PatternDir`/`PatternFile` helpers, and the four Layout accessors `WeftPatternDir`/`WeftPatternDirFor`/`HostPatternLink`/`HostPatternLinkHere` plus `PatternFileHere()`, all in `internal/hubgeometry/hubgeometry.go`; new test file `internal/hubgeometry/pattern_test.go`.
- Card 4 (`bb7aeb13`): added `HostJunctionsHere()` on `*Layout` in `internal/hubgeometry/hubgeometry.go`; new `TestHostJunctionsHere` in `internal/hubgeometry/hubgeometry_test.go`.
- Card 5 (`3481e9c7`): reserved `_pattern` in `IsReservedHubName`, added it to `TestEnforcement_GeometryLiterals`'s token list in `internal/hubgeometry/enforcement_test.go`, added `TestIsReservedHubName_Pattern` in `internal/hubgeometry/hubgeometry_test.go`, and updated `CONSTRAINTS.md` and `docs/shared-libs/hubgeometry.md`.

Verify command `go test -tags integration ./internal/hubgeometry/... ./cmd/lyx/...` passes.

One note worth flagging: an early `go fmt ./internal/hubgeometry/...` run reformatted 14 files it shouldn't have touched (pure CRLF→LF line-ending churn from Windows `core.autocrlf`, plus one real pre-existing struct-alignment fix in `internal/hubgeometry/weft_test.go` that predates this batch). I reverted all of those via `git checkout --` before staging anything, so only the batch's declared files (`internal/hubgeometry/hubgeometry.go`, `pattern_test.go`, `hubgeometry_test.go`, `enforcement_test.go`, `CONSTRAINTS.md`, `docs/shared-libs/hubgeometry.md`) ended up in the three commits — confirmed via `git diff --stat` before each `git add`. Whole-package `go fmt`/`gofmt -l`/`goimports -l` all falsely flag this repo's CRLF files as unformatted; per-file `git diff --stat` is the reliable signal here, not raw formatter output.
