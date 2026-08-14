All 3 cards (29, 30, 31) committed — 3 of 3 cards complete, matching the batch's declared card count. Verify passed cleanly.

Summary of work:
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/gitrepo/blobread.go` — new file adding `FileAtRevision` (go-git blob read at a revision, with `ErrPathNotAtRevision` sentinel) and `PathRevisions` (newest-first path-history walk with optional limit), both go-git only, no `gitexec` calls.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/fabricengine/stencilhistory.go` — new file adding `StencilBaseByStamp(hub, name, stampHash)`, the board-scoped read-only accessor that walks a stencil's history and matches the stripped-and-LF-normalised body hash to find the forked-from default.
- `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/gitrepo/blobread_integration_test.go` and `/home/knatte/Code/loomyard/wts/stencils-directory-reorg/internal/fabricengine/stencilhistory_integration_test.go` — new integration-tagged test files covering both new gitrepo verbs and the stamp-keyed base recovery, including the CRLF/LF hash-normalisation case.

Commits (all pushed to `stencils-directory-reorg`): `3b61b344`, `e62298ee`, `4364a9cd`.

{"status":"success","commit_sha":"4364a9cdcc4e151e9025e348ffdbc33255de68bc","session_id":"77fbd99c-765b-4979-808c-4d74ea6fc264","cards_done":[29,30,31]}
