{"status":"success","commit_sha":"5e9a9af65ce258b5a9ec9d73afc94aa1209d9440","session_id":"f3b40cd5-55d8-4bef-a7c0-1a64b612fa87","cards_done":[13,14]}

Card-count self-check: 2 of 2 cards in this batch completed — Card 13 (committed as `5e9a9af6`) and Card 14 (Commit: none, verification-only, performed this turn).

Summary:

**Card 13** — added three override-note blocks to `manifest/designs/shed-followups.md`, all opening with `**Override recorded 2026-08-09 (task B, as landed).**`, matching the existing task-A precedent's formatting/indentation:
- Block 1 (section B `### Scope`, after "This task changes paths and names only, never prose."): 8 numbered departures, including a freshly-verified count (`grep -c 'plan-format-v3\.md' manifest/designs/shed-followups.md` → **5**, not the discussion's "four"), and the corrected `gopkg.in/yaml.v3` file count (**32**, not "ten").
- Block 2 (section C, item 6): records that task B already discharged the "Coexistence, not replacement" rewrite by deleting the surviving blockquote.
- Block 3 (section E, two notes): records that `loom.md:29` was rewritten in full rather than left self-contradicting, and both of task B's roadmap.md edits.

Commit: `5e9a9af6` — `docs(shed-followups): record task B's overrides for tasks C and E`.

**Card 14** — ran the full acceptance gate (zero-diff, `Commit: none`):
- Gates 1, 2, 4, 5, 6, 8, 9, 10, 11 — all passed as specified.
- Gate 7 (`go build ./... && go test ./...`) initially failed: `internal/lyxcwd`'s `TestEnforcement/tree-scan` flagged `.scratch/sweep/main.go` for a raw `os.Getwd()` call. That file is the untracked, temporary sweeper from batch 1, whose own doc comment states "It is not part of the shipped build and is deleted once the sweep lands." Since the sweep already landed and the file is never staged/committed (`.gitignore` already ignores `.scratch/`), I deleted `/home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix/.scratch/sweep/main.go` as the self-fix; re-run of `go build ./... && go test ./...` is now fully green.
- Gate 3 (`\bv2\b` repo grep) surfaces hits in `manifest/designs/shed-followups.md` that are not on the batch's "deliberately-untouched list." These are the same category of historical scoping-time citations that Block 1's override note and gate 10 already establish as this file's permanent, by-design exemption — and batch 2's own card 8 explicitly pre-states, in near-identical language, that this file's v2 citations are "not a regression, and not to be filed as one." I did not edit these citations (doing so would rewrite a historical record and contradict the batch's own explicit instruction "Do not 'fix' them"), and I did not alter the gate's grep or scope to hide the hits — I'm flagging this plainly for review rather than silently treating it as a pass or a fail, since gate 3's own file list simply doesn't enumerate an exemption that the rest of the batch already establishes for this exact file.

Files touched: `/home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix/manifest/designs/shed-followups.md` (edited, committed), `/home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix/.scratch/sweep/main.go` (deleted, untracked, never part of any commit).

{"status":"success","commit_sha":"5e9a9af65ce258b5a9ec9d73afc94aa1209d9440","session_id":"f3b40cd5-55d8-4bef-a7c0-1a64b612fa87","cards_done":[13,14]}