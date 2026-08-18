# shuttle — independent review, round 1 (`opus-medium-r1`)

Clean-room review of `internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`,
run against the worktree `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).
No prior `_mill/shuttle-review-*` or `_mill/reed-shuttle-HANDOFF.md` material was read before this findings list was complete.

Merge bar for this round: correctness in the NORMAL single-instance flow.
`LLM-DRIVING: YES` — every live scenario below spawns exactly one real `claude` process, pinned to `--model haiku`.

## What was tested

(appended live, in order)

## Findings

(appended live, in order)
