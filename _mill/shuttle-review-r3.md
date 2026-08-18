# shuttle — independent review, round 3 (`opus-medium-r3`)

Reviewer: Opus 5, medium effort. Clean-room: findings below were formed before reading any
`_mill/shuttle-review-*` file or `_mill/reed-shuttle-HANDOFF.md`.

Primary mandate this round: **reed×shuttle joint adversarial testing** — real live shuttle runs
(`--model haiku`) with reed-side hardened failure modes triggered MID-FLIGHT.

## Environment / baseline

- Host: Linux, tmux 3.6 (`/usr/bin/tmux`), `claude` at `/home/knatte/.local/bin/claude`.
- Worktree: `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening`, `AnchorRel = "."`.
- Baseline BEFORE any driving: `tmux: server` processes = **0**; `pgrep -xc claude` = **4**
  (four unrelated long-lived agent sessions, none tmux-hosted).
- Build: `./deploy-dev` @ `06719ef2`, `go build ./...` green.

## What was tested

(appended live, in order)

## Findings

(appended as spotted)
