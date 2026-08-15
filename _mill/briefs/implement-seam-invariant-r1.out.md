Both cards (23 and 24) are committed, working tree is clean, and verify (`go test ./internal/shedengine/...`) passed. Card-count self-check: 2 of 2 cards committed — all complete.

Summary of changes:
- `/home/knatte/Code/loomyard/wts/shed/internal/shedengine/seam_enforcement_test.go` (new) — allowlist-based `TestProducerSeamInvariant_AllowlistOnly`, restricting `internal/shedengine` production imports to stdlib, `internal/state`, and `internal/lock`.
- `/home/knatte/Code/loomyard/wts/shed/CONSTRAINTS.md` — new `## Shed Producer-Seam Invariant` section inserted between the Treadle Runner-Seam and Tokenvocab Leaf Invariant sections.

{"status":"success","commit_sha":"bc9221cf8aa0dc94c83273ade4bc97f8077345a7","session_id":"c69444eb-29d0-4f7a-aa04-d2f5567f3a7b","cards_done":[23,24]}
