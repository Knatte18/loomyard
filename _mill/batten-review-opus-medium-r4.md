# batten — independent review (round 4, tag `opus-medium-r4`)

> Clean-room round: findings below were formed from the SPEC (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`), the module code, the docs, and live driving of a disposable fixture hub — before any prior round's review material was opened.

**Status: in progress.** Sections are appended as each scenario returns.

## Executive summary

_(written last)_

## Scope assessment — plan vs shipped

_(written last)_

## Findings

_(appended as they are found)_

## Docs & operability findings

_(appended as they are found)_

## What was tested

### Hermetic (all green, before any change)

- `go build ./...` — clean.
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...` — clean.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...` — all `ok`.
- `go test -tags integration -count=1 ./internal/battencli/...` — `ok`, 2.504s.

### Fixture hub

Disposable hub built outside both the loomyard tree and `$HOME/Code`, under this session's scratchpad:
`.../scratchpad/hub` holding `remotes/bfix.git` + `remotes/bfix-weft.git` (bare, hand-seeded minimal Go project: `go.mod`, `main.go`, `main_test.go`, `README.md`) and the cloned hub `bfix-LYXHUB` (`bfix` warp prime on `main`, `bfix-weft` on `main-weft`, `_board`).

- `lyx fabric clone --into <scratch> <weft.git> <warp.git>` — `ok: true`, `partial: false`, 22 mutations, warp binding recorded.
- **GitHub self-report hazard, discharged first:** `selfreport: false` committed and pushed onto `main-weft` (`f4807bd fixture: selfreport off`) BEFORE any Board task existed and before any `Worktree-Create`.

_(live scenarios appended below as they run)_
