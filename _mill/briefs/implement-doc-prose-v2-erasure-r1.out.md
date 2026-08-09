4 of 5 cards produced commits (cards 4, 5, 6, 7); card 8 is a `Commit: none` verification-only card whose grep gates were all run and passed this turn. All 5 cards declared in this batch are complete.

```json
{"status":"success","commit_sha":"1aa15333068553de073f236b6ba8428833d3f1e5","session_id":"c037f54e-d272-4ba7-8fd6-765078fc6b2c","cards_done":[4,5,6,7,8]}
```

Summary of work:
- `docs/reference/plan-format.md` (commit `b72d054a`): deleted the retired-v2 blockquote pair.
- `docs/reference/plan-format.md` (commit `bac65da0`): erased every plan-format-v2 reference and bare-v3 label — rewrote 8 sites to state properties directly, hyphenated the H1 to `Plan-format`, deleted the "Dropped from v2, and why" block. Confirmed `grep -ni 'v3' docs/reference/plan-format.md` returns nothing.
- `manifest/designs/loom.md` (commit `9cdfb4d9`): rewrote the self-referential Plan-producer sentence to state the live format plainly with a single link.
- `manifest/roadmap.md` (commit `1aa15333`): deleted the stale "v3 is the live plan format..." sentence from the Done item, and hand-rewrote line 18's task-breakdown parenthetical to describe the sweep instead of naming both filenames, keeping the `plan-format-drop-v3-suffix` slug intact.
- Card 8 (verification-only, no commit): confirmed all grep gates return zero — `v3` in `plan-format.md`, `\bv2\b` across all three files, the six-pattern sweep grep repo-wide (with and without the roadmap:18 filter), and the preserved task slug on `manifest/roadmap.md:18`. Working tree is clean and `go build ./...` passes.

`git rev-parse HEAD` = `1aa15333068553de073f236b6ba8428833d3f1e5`.