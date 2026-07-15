All 3 of 3 cards committed and pushed, matching all three `Commit:` messages declared in the batch file. Verify (`go test ./cmd/lyx/ -run TestSandboxCoverage_AllModulesCoveredOrExcluded`) passes, and `git status --porcelain --untracked-files=no` is clean.

Summary of work:
- Card 13: `docs/modules/loom.md` — migrated the `lyx loom status` strand's documented display from `anchor:top` to `below-parent` + `ShrinkWhenWaitingOnChild` in both the module table row and the `lyx loom run` bootstrap pseudo-code, replacing the stale `height:fixed(1)` note with the dynamic shrink description and a note referencing the `childless-full-height-is-acceptable` decision. Commit `6c242e9b`.
- Card 14: `docs/reviews/mux-review-prompt.md` — removed the two TOP-BAND LEGIBILITY bullets, narrowed the anchor vocabulary line to `below-parent|hidden`, and dropped `top_band_rows` from the psmux-normalization bullet and the F1/F11 live-finding narrative. Commit `347e8b26`.
- Card 15: `tools/sandbox/SANDBOX-MUX-SUITE.md` — retired M6 to a one-line tombstone (kept heading/number), rewrote M12 as a below-parent-only three-strand scenario, added M18 (operator-run below-parent root mother + Claude Code child scenario), extended the session-log list and the `ref` range note to include M18, and left M2's `**Covers:** mux` tag untouched. Commit `c941f30f`.

Only in-scope files were touched; no plan-extension was needed (all edits landed in files already listed in the batch's `Edits:`/`Context:` lists).

{"status":"success","commit_sha":"c941f30f","session_id":"f44b15cc-3f0f-424d-932e-85aadf979eb3"}
