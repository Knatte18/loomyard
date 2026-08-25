All checks pass, working tree clean, HEAD differs from baseline `a3ebf5ecc3a12bf1e9d2d39f88d1007e821980c1`.

Both findings from the holistic review were NIT:consistency, verified factually accurate, no harm from fixing -- both were FIXed:

1. `internal/loomrecipe/shape_test.go` -- stale "thirteen rows"/"rows 12 and 13" comment corrected to "fourteen rows"/"rows 13 and 14" (commit `fddbbd0e`).
2. `manifest/designs/shed-recipe.md` -- stale "thirteen-row" prose (two occurrences) corrected to "fourteen-row"; since this file wasn't in any batch's `Edits:` list, first extended batch 6 (`_mill/plan/06-docs.md`) with a new Card 25 scoping the file (commit `d897714a`), then applied the fix (commit `bfd76196`).

All six batch `verify:` commands ran clean, plus the overview's `go build ./...`.

{"status":"success","commit_sha":"bfd76196305434490c7f63e165367d68bae0efb1","session_id":"6926c50d-90f0-4aaa-85f0-631f2d860470"}
