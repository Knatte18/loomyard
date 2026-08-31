## Summary

All six cards' code is committed in a single combined commit (`f8f85a991`, per the plan's own "all six land in one file/commit" note), and I additionally committed a design-corrected fix (`6999b53d0`) that retargets the resize-trigger control to fire against reed's own socket/session rather than the harness window, plus a documentation commit (`a0a9bd078`) recording this round's diagnosis in the plan file. Two of the batch's three new tests pass (`TestSmokeDotFillFloorIsCleanOnASettledAttach`, `TestSmokeDotFillCrossClientControl`). `TestSmokeDotFillResizeControl` still fails after two distinct self-fix attempts this round:

1. Retargeted the trigger from the harness window to `resize-window -t <reedSession>` on `h.reedSocket` (per the plan's own already-recorded correction) — confirmed via `#{window_width}x#{window_height}` readback that the resize genuinely takes effect (140x42 -> 80x24 -> 160x50), but no dot-fill appears.
2. Widened the shrink/grow magnitudes to 10x5 -> 300x100 (still within Card 5's "distinctly smaller"/"grow back past the original" wording) — a tight capture-pane loop with no sleep (927 samples over 2s, far finer than the production 100ms poll) found zero hits either way.

I confirmed this is in-scope, not pre-existing: `git show main:internal/reedcli/smoke_dotfill_test.go` shows the file doesn't exist on `main` at all. Per the Test Integrity Guardrail I did not touch `dotRunFloor`, the glyph list, or the poll cadence to force a pass.

Files touched:
- `/home/knatte/Code/loomyard/wts/reed-attach-dotfill-artifact/internal/reedcli/smoke_dotfill_test.go` (resize-target fix, committed at `6999b53d0`)
- `/home/knatte/Code/loomyard/wts/reed-attach-dotfill-artifact/_mill/plan/01-dotfill-repro-harness.md` (round-2 diagnosis appended, committed at `a0a9bd078`)

Card-count honesty check: all 6 cards' code exists in the tree and is committed (5 via the pre-existing `f8f85a991`, which its own commit message states covers all six), but the batch as a whole does not pass its declared `verify:` command, so this is not a clean "all complete" success.

{"status":"stuck","stuck_type":"verify","reason":"TestSmokeDotFillResizeControl still cannot be made to hit in this environment after 2 self-fix attempts (retarget resize to reed's own socket/session per plan correction; widen shrink/grow magnitudes to 10x5->300x100) - confirmed non-pre-existing via git show main; tight-loop diagnostics (927 samples/2s) show the resize genuinely takes effect but produces zero dot-fill of any duration","commit_sha":"a0a9bd0780f1f4e696c61608bf80093603b35f8e","session_id":"0ecf36bc-358d-4ff5-8dbf-2a1d7ba90c92"}
