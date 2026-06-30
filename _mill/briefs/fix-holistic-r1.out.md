Working tree is clean. HEAD `0f3ae26794813550503da28148cea5366df1f050` differs from baseline `3629740c01a3ff582d6b84bd717956bd5416a8c9`.

Summary of work done:
- Found 1 NIT finding (stale `paths.X` comment qualifiers in 6 out-of-manifest files): routed to FIX.
- Extended `_mill/plan/01-code-rename.md` with Card 8a to declare the 6 new files (rule 6 compliance), committed that plan extension.
- Fixed all 6 files by replacing `paths.BoardDir`, `paths.Getwd()`, and `paths.Resolve` in prose comments with `hubgeometry.BoardDir`, `hubgeometry.Getwd()`, and `hubgeometry.Resolve` respectively. Ran `gofmt -w` to normalize line endings. 13 individual replacements across the sweep.
- Verify: `go build ./... && go test ./... && go vet -tags integration ./...` all pass.

{"status":"success","commit_sha":"0f3ae26794813550503da28148cea5366df1f050","session_id":"69886c88-d71d-489e-8fa7-05492344f81b"}
