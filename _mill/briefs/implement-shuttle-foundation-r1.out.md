All four cards are committed, verify passes, and the working tree is clean.

{"status":"success","commit_sha":"b3ee703","session_id":"8d98482c-aff5-4557-ad82-7ae844009d55"}

Summary of work done for batch `02-shuttle-foundation`:

- `C:\Code\loomyard\wts\internal-shuttle\internal\shuttleengine\doc.go` — package header folding in the durable design from `docs/modules/shuttle.md`.
- `C:\Code\loomyard\wts\internal-shuttle\internal\shuttleengine\template.yaml`, `template.go`, `config.go`, `config_test.go` — shuttle.yaml config module mirroring muxengine's trio.
- `C:\Code\loomyard\wts\internal-shuttle\internal\configreg\configreg.go`, `configreg_test.go` — registered `shuttle` module alphabetically between `mux` and `warp`.
- `C:\Code\loomyard\wts\internal-shuttle\internal\shuttleengine\spec.go`, `spec_test.go` — `Spec` type + `validate` (mandatory `OutputFiles`, relative→absolute resolution, `Timeout`/`Display.Anchor` defaulting).
- `C:\Code\loomyard\wts\internal-shuttle\internal\shuttleengine\rundir.go`, `rundir_test.go`, `posix.go`, `posix_test.go` — run-dir lifecycle (`newRunID`/`createRunDir`/`runDirRoot`), `RunState`/`saveRunState`/`loadRunState`, `findRunByStrand`, age-guarded `sweepOrphans`, and `PosixPath`.

Verify command `go test ./internal/shuttleengine/... ./internal/configreg/...` passes. All 4 cards committed as separate commits (`c6f2a8d`, `150aa01`, `b26d6bc`, `b3ee703`), pushed to `origin/internal-shuttle`. Working tree is clean.
