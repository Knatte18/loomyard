The batch declares 1 card (Card 16), and I made 1 matching commit. 1 of 1 cards committed — all complete.

Summary of work: batch `05-roadmap-and-design-doc-deletion` had a single card. I moved the completed Planned item "producers standalone: invariants and docs" to the head of `## Done` in `manifest/roadmap.md`, repointed it and the four existing producers-standalone Done entries away from the design doc toward `../CONSTRAINTS.md#told-geometry-invariant` (plus `internal/preflight`, `internal/hubgeom`/`internal/standalonegeom` package docs where relevant), dropped the now-false "the doc survives this task because …" clauses, and `git rm`'d `manifest/designs/producers-standalone.md`. Confirmed via grep that no remaining non-`_mill` reference to `producers-standalone` exists outside the expected prose mentions in `CONSTRAINTS.md:105` and `internal/buildinfo/doc.go` (both out of this card's scope). `go test ./internal/lyxcwd/...` passes. Everything committed and pushed as `acd1323a`.

1 of 1 cards committed — all complete.

{"status":"success","commit_sha":"acd1323af17f8877c6bc9c27cf0acc106a61af4e","session_id":"5754c0d3-fd4c-4ea2-9104-c37c8a322f9c","cards_done":[16]}

Relevant files:
- /home/knatte/Code/loomyard/wts/standalone-docs-and-invariants/manifest/roadmap.md
- /home/knatte/Code/loomyard/wts/standalone-docs-and-invariants/manifest/designs/producers-standalone.md (deleted)

{"status":"success","commit_sha":"acd1323af17f8877c6bc9c27cf0acc106a61af4e","session_id":"5754c0d3-fd4c-4ea2-9104-c37c8a322f9c","cards_done":[16]}
