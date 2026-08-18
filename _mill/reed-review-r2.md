# reed — independent review, round 2 (`opus-high-r2`)

> Clean-room safety pass over `internal/reedengine` + `internal/reedcli` + `internal/hubgeom`, per `_mill/reed-review-prompt.md`.
> Written before any production or test file was touched (the prompt's BLOCKING sequencing rule).

## Scope of this round

- `internal/reedengine/**` (told-geometry seam `geometry.go`, plus `lifecycle.go`, `lock.go`, `header.go`, `strand.go`, `spawn.go`, `apply.go`, `reconcile.go`, `overlay.go`, `proctree*.go`, `server.go`, `probe.go`, `serverlog.go`, `state.go`, `io.go`, `config.go`, `name.go`, `headerpane.go`)
- `internal/reedcli/**` (all eight verbs + the `PersistentPreRunE` construction seam)
- `internal/hubgeom/**` (the one-way told-geometry adapter)
- `cmd/lyx` integration, plus the four other `hubgeom.ReedGeometry` call sites (`burlercli`, `shuttlecli`, `perchcli`, `webstercli`) as construction-seam consumers only
- Docs: `docs/overview.md` reed bullet, `CONSTRAINTS.md`, `internal/reedengine/doc.go`, `manifest/designs/producers-standalone.md`

Out of scope this round (per prompt): `internal/shuttleengine`/`internal/shuttlecli` behaviour, hubgeom's unbuilt wave-3 siblings, Windows-specific tmux/path behaviour.

## Environment

- Linux host, `tmux 3.6` at `/usr/bin/tmux` (PATH-resolved).
- Worktree `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening`, branch `reed-shuttle-crucible-hardening`, clean at start (HEAD `61cead10`).
- Dev binary deployed via `./deploy-dev` → `.dev-bin/lyx`.

## What was tested

(appended as each command/scenario returned)

### Hermetic gates — baseline, before any edit

| command | result |
| --- | --- |
| `go build ./...` | PASS (rc 0) |
| `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | PASS (rc 0) |
| `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | PASS (all 5 packages ok) |

### Import-direction check (hubgeom one-way told direction)

`go list -deps ./internal/reedengine | grep -c hubgeom` → `0`.
The full internal dependency list of `reedengine` is `envsource, proc, gitexec, lyxcwd, lyxdirs, logger, yamlengine, configengine, lock, reedengine/render, shell, fsx, state, stencil, tokenvocab` — no `hubgeom`, no `fabricengine`.
`lyxcwd` is present transitively via `logger`, exactly as `doc.go` already states honestly.
**Told direction holds.**

## Findings

(recorded provisionally as spotted; severities finalised at the end)

## Merge-readiness verdict

(filled at the end of Job 1)
