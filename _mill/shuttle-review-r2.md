# `shuttle` — independent review, round 2 (`opus-high-r2`)

> Clean-room round-2 review of `internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`.
> Written per `_mill/shuttle-review-prompt.md`. Findings formed BEFORE reading any round-1 material.
> Merge bar: correctness in the NORMAL single-instance flow. No N×-concurrent sweep against this module.

## Substrate baseline (recorded before any driving)

- `claude` on PATH: `/home/knatte/.local/bin/claude`, version `2.1.226 (Claude Code)`, logged in.
- `tmux` on PATH: `/usr/bin/tmux`.
- `ps -eo comm | grep -cx 'tmux: server'` = **0** at start.
- `pgrep -c claude` = **20** at start — 14 `claude-desktop` (unrelated GUI app) + 6 `claude` agent sessions
  (pids 1472397, 1922201, 3145593, 3145825, 3187175, 3187599), all pre-dating this round.
  Teardown is judged against this exact pid set, not against zero.

## What was tested

(appended live, in order)

### Hermetic gates, cold

- `go build ./...` → exit 0.
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...` → exit 0.
- `go vet -tags smoke ./internal/shuttlecli/...` → exit 0.

## Findings

(appended live, provisionally, as spotted)

## Scope assessment

(pending)

## Convergence assessment

(pending)
