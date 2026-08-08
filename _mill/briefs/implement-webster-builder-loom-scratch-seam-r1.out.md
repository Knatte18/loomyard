All 8 cards are committed and the batch's `verify:` command passes in full (unit tests, integration-tagged tests, and the smoke-tagged vet). The working tree has no uncommitted tracked changes.

Card-count self-check: 8 of 8 cards committed (cards 15-22), confirmed by matching each `### Card N:` heading's `Commit:` message against `git log fbf7354001a1146b46b9a5308cd1eecbf3ca5cfd..HEAD --oneline` above, plus two supplementary `plan:` commits extending card 22's `Edits:` scope for two real bugs the negative-half tests surfaced (a `LoadState` MkdirAll gap in both engines' `state.go`, and a stale `buildercli/run_test.go` fixture from card 19).

Key files touched, all under `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene`:
- `internal/websterengine/state.go`, `pause.go`, `runlevel.go`, `beginbatch.go` (+ their `_test.go` files)
- `internal/builderengine/state.go`, `pause.go`, `runlevel.go`, `spawn.go` (+ their `_test.go` files)
- `internal/webstercli/cli.go`, `run.go`, `beginbatch.go`, `recordbatch.go`, `recoverbatch.go`, `awaitbatch.go`, `status.go`, `pause.go`, `ownership.go` (+ `cli_test.go`, `verbs_test.go`)
- `internal/buildercli/cli.go`, `run.go`, `spawnbatch.go`, `poll.go`, `status.go`, `pause.go` (+ their `_test.go` files, `run_test.go`, `smoke_test.go`)
- `internal/loomengine/config.go`, `preflight.go` (+ `loomstatus_test.go`, `preflight_integration_test.go`)
- `cmd/lyx/constructoranchoring_test.go`
- `_mill/plan/03-webster-builder-loom-scratch-seam.md` (two scope-extension edits)

Bare JSON status line follows:

{"status":"success","commit_sha":"d5d69f67424097e14b9d7a3198bf1d814382b073","session_id":"4482d91c-5d98-4f53-8d12-7c244e3c164e","cards_done":[15,16,17,18,19,20,21,22]}
