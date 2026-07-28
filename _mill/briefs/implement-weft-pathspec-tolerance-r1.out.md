Both cards in this batch (2 of 2) are committed, verified, and pushed:

- **Card 13** (`5d269938`) — `internal/fabricengine/weftgit.go`: added `weftPathspecFilter`/`entryMatchesWeft`, wired into `CommitWeft` before staging, with early-return when no positive entry survives. New test file `internal/fabricengine/weftgit_pathspec_integration_test.go` covers all four predicate-clause cases.
- **Plan-extension** (`9f260465`) — `_mill/plan/04-weft-pathspec-tolerance.md`: added `weftgit_pathspec_integration_test.go` to card 14's `Edits:` list before touching it, per the batch's own file-scope discipline.
- **Card 14** (`1f96a076`) — widened `internal/fabricengine/template.yaml`'s pathspec default to `_lyx _pattern`; rewrote `internal/fabricengine/template_test.go`'s stale assertion; updated `internal/fabriccli/weft_verbs.go`'s help text and `internal/fabricengine/doc.go`'s package doc for the two accepted consequences; added the batch's core regression test (widened default + no `_pattern` content, both absent and empty-directory shapes, still commits `_lyx`).

Verify command `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/...` passed clean (all three packages `ok`). `go build ./...`, `go vet ./...`, `goimports -l`, and `golangci-lint run` on the touched packages were all clean. Working tree is clean (`git status --porcelain --untracked-files=no` empty). Card-count self-check: 2 of 2 declared cards committed.

Note: while running `go fmt ./internal/fabricengine/...` for the format-drift step, it rewrote ~45 unrelated files in that package purely due to CRLF→LF line-ending churn (no content diff, confirmed via `git diff`); I reverted all of those with `git checkout --` before staging anything, keeping the commits scoped to only the intended files.

{"status":"success","commit_sha":"1f96a076d9b528041c33d69e3b7ed9379c830287","session_id":"d810eafd-0c80-4f1f-971a-f89852809033","cards_done":[13,14]}
