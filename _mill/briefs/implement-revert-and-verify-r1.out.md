All checks pass. Summary of work:

Card count: 1 of 1 committed (Card 8). Two commits: primary card-8 commit d4dcb31c (deleted tools/codeintel-poc/ and .lsp.json, ran go mod tidy), plus follow-up fix f75f0e32 — go mod tidy left golang.org/x/sys bumped to v0.47.0; pinned back to main's v0.45.0 and re-tidied so go.mod/go.sum are byte-identical to main.

Final verification:
- `git diff main...HEAD --name-only -- ':(exclude)_mill/'` -> exactly docs/research/codeintel-spike.md (doc-only assertion passes)
- `go build ./...` -> clean (batch verify)
- `go vet ./...` -> clean
- `git status --porcelain --untracked-files=no` -> clean
- gofmt flagged 3 pre-existing files unrelated to scope, left untouched.

Deleted: tools/codeintel-poc/{main,gopackages,callers,gopls,callgraph}.go, .lsp.json. Sole remaining product diff vs main: docs/research/codeintel-spike.md.

{"status":"success","commit_sha":"f75f0e32","session_id":"619a4beb-9d0c-4d9f-84b8-5f6290357b7a"}
