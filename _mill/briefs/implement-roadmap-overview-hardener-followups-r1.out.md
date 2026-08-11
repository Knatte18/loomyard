All 7 cards in this batch are complete: cards 12-17 committed as separate content commits (docs-only edits to `manifest/roadmap.md`, `docs/overview.md`, `manifest/designs/hardener.md`, `manifest/designs/shed-followups.md`), and card 18 (Commit: none) was completed as a verification-only pass — I ran the full acceptance grep set from the batch's Requirements over `manifest/`, `docs/`, and `README.md` (excluding the two permanently-exempt files) and confirmed all eight checks pass with zero in-scope hits, plus manually confirmed `shed.md` lines 7/19 have no dangling "below" references and that `manifest/roadmap.md` lines 57-61 agree in substance and terminology with `shed.md`'s carve-out text. Verify (`go test ./internal/lyxcwd`) passed. Working tree is clean (`git status --porcelain --untracked-files=no` empty).

Relevant files touched:
- `/home/knatte/Code/loomyard/wts/shed-producer-typology-sweep/manifest/roadmap.md`
- `/home/knatte/Code/loomyard/wts/shed-producer-typology-sweep/docs/overview.md`
- `/home/knatte/Code/loomyard/wts/shed-producer-typology-sweep/manifest/designs/hardener.md`
- `/home/knatte/Code/loomyard/wts/shed-producer-typology-sweep/manifest/designs/shed-followups.md`

{"status":"success","commit_sha":"739131efd6264d9b31443d061ec269add70fac09","session_id":"d8338de8-1a3c-438b-9d8e-0d3a17e6f570","cards_done":[12,13,14,15,16,17,18]}
