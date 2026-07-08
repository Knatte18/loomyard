All 5 cards committed, verify passes, working tree clean (untracked-only remnant is the brief file itself, which is not an in-scope tracked modification).

Summary of work in this batch (`cli-docs`, batch 5 of the perch task):

- `internal/perchcli/cli.go`, `cli_test.go` — cobra command tree, `PersistentPreRunE` wiring chain (mirrors `burlercli`), `perchCLI` struct storing resolved ingredients rather than a constructed engine.
- `internal/perchcli/run.go`, `run_test.go` — `run` verb: strict `decodeProfile` (kebab-case profile YAML incl. `gate`/`round-caps`), run-tuning flag overrides, run identity (`ProfileHash`/`DeriveRunID`), per-invocation `perchengine.New` with the pause seam, weft commit+push at block exit.
- `internal/perchcli/pause.go` — `pause` verb writing the round-boundary pause flag; fails loud on a missing run dir, idempotent re-pause.
- `cmd/lyx/main.go`, `cmd/lyx/helptree_test.go` — registered `perchcli.Command()` in `newRoot()`, extended pinned help-tree sets.
- `internal/perchengine/doc.go` — expanded into the durable design header (two-exit gate + PAUSED, milestone ladder, holistic verdict-judge model, pluggable gate, non-done handling, pause, weft/geometry-blindness, config rule).
- Deleted `docs/modules/perch.md`; retargeted every inbound reference in `docs/overview.md`, `docs/roadmap.md`, `docs/reviews/README.md`, `docs/modules/README.md`, `docs/modules/loom.md`, `docs/modules/hardener.md`.
- `tools/sandbox/SANDBOX-BURLER-SUITE.md` — added scenario S4 (`**Covers:** perch`) covering convergence, run-dir/weft inspection, and pause/resume.

Verify command `go test ./cmd/lyx/ ./internal/perchcli/ ./internal/perchengine/` passes; `go build ./...` and `go vet ./...` are clean.

{"status":"success","commit_sha":"03a8ed09b161a593bbfd4261ebca45f9ff19e8bf","session_id":"bd81d30d-931c-40fb-8ba0-6c8ef52bc435"}
