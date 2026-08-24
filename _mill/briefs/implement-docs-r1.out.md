Card-count self-check: 4 of 4 cards committed (cards 13, 14, 15, 16), matching the batch file's declared card count of 4. All committed, verify passed, working tree clean.

{"status":"success","commit_sha":"9db50307c611d26852bafc461ed4326e87811a9","session_id":"61b364df-6446-46ff-8a77-b35ec5f7b7d8","cards_done":[13,14,15,16]}

Summary of work:

- contracts/specs/loom-plan-spec.md — corrected the "Plan vs. schedule" and "Deferred / forward-compat" sections' now-false scheduling claims (commit 0a5183f4).
- docs/overview.md — added the DAG-derived batch-sequencing clause to webster's module bullet (commit 7c52996e).
- manifest/roadmap.md — moved webster: DAG-derived card sequencing from Wave 3 to Done with a shipped-behavior rewrite, corrected the now-stale HasSymbolFields() claim in the planparser Done entry, and repointed the Someday worktree-per-card item's dependency at the Done entry (commit 744bbce1).
- internal/planparser/validate.go — corrected the file banner's stale "cards still execute in strict declared plan order" justification to point at internal/websterengine/sequence.go (commit 9db50307).

Verify command `go test ./internal/lyxcwd/... ./internal/planparser/...` passed. Working tree is clean (no uncommitted tracked changes), gofmt -l reports no drift on the one Go file touched.

{"status":"success","commit_sha":"9db50307c611d26852bafc461ed4326e87811a9","session_id":"61b364df-6446-46ff-8a77-b35ec5f7b7d8","cards_done":[13,14,15,16]}
