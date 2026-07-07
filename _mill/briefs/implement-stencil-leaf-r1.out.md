All three cards for batch `01-stencil-leaf` are implemented, committed, and pushed:

- `C:\Code\loomyard\wts\internal-stencil\internal\stencil\stencil.go` — Card 1: `Fill(template []byte, values map[string]string) ([]byte, error)`, stdlib-only, with `stripLeadingComment` and the top-level unfilled-marker guard (batched, sorted, deduped) plus incremental `missingkey=error` guard for branch-internal misses.
- `C:\Code\loomyard\wts\internal-stencil\internal\stencil\stencil_test.go` — Card 2: black-box, table-driven tests covering all 14 scenarios from the plan (happy path, missing/empty markers, multi-offender sort+dedup, branch-internal vs top-level priority, malformed templates, conditional taken/not-taken, forgotten discriminator, unused values, leading-comment strip, empty/whitespace templates, idempotence, no-HTML-escaping).
- `C:\Code\loomyard\wts\internal-stencil\docs\shared-libs\stencil.md` and `C:\Code\loomyard\wts\internal-stencil\docs\shared-libs\README.md` — Card 3: status flipped to built, signature and grammar pinned, guarantee wording refined to the built scoping; `docs/overview.md` left unchanged (it doesn't enumerate individual shared libs exhaustively, confirmed).

`verify: go test ./internal/stencil/` passes. `git status --porcelain --untracked-files=no` is clean (no in-scope tracked modifications outstanding).

{"status":"success","commit_sha":"24d5ebdc5427183fdc89660d7eed8582731a3666","session_id":"22288423-8acf-415c-8035-3b3911356b8d"}