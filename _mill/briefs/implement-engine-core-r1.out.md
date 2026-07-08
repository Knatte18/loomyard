No tracked in-scope modifications remain (status output is empty). Verify passed. Both cards are committed with distinct commits.

{"status":"success","commit_sha":"782c164","session_id":"7d210445-b6ea-4212-a3e0-b3f1563e00d9"}

Summary of work:
- Created `internal/burlerengine/profile.go` and `internal/burlerengine/profile_test.go` (Card 1): `FileSet`, `FixScope`, `Profile`, `RunOpts`, `ErrClusterUnsupported`, and `(*Profile).validate` with fail-loud, ordered validation and in-place absolute path resolution mirroring `shuttleengine.Spec.validate`.
- Created `internal/burlerengine/verdict.go` and `internal/burlerengine/verdict_test.go` (Card 2): `Verdict`, `Severity`, `Finding`, and `ParseReview` — a strict `---`-delimited YAML frontmatter parser (CRLF-tolerant, unknown-key-tolerant) enforcing verdict spelling, finding-key completeness, severity vocabulary, duplicate-id rejection, and the symmetric BLOCKING/APPROVED consistency rules.
- Commits: `c243ae7` (Card 1), `782c164` (Card 2), both pushed to `origin/internal-burler`.
- Verify command `go build ./... && go test ./internal/burlerengine/...` passes; `gofmt` and `go vet` clean on the new package; working tree has no uncommitted tracked changes.

{"status":"success","commit_sha":"782c164","session_id":"7d210445-b6ea-4212-a3e0-b3f1563e00d9"}