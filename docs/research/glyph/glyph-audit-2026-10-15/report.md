# Glyph use audit 3: pack-file layout (1e39afc10) vs read-all-first forks

Written by the orch from the audit agent's returned report (2026-10-15); the agent's own Write of this file was refused.
Every command is in `COMMANDS.md`.

## Samples

- `pf`: 24 forks, 10 runs, 93 cards, `shed-tackler` batches 1, 4, 7 and 13 included.
- `raf`: 31 forks, 14 runs, 134 cards (the old report said 29; the old `extract2.jq` gives 31 over the same files, unexplained).
- Forks were classified by prompt content: `## Start here` for `pf`, the inline pack plus the read-all-first phrase for `raf`.
- Find: `find . -path './-home-knatte-Code-loomyard-LYXHUB-*/*/subagents/agent-*.jsonl' -newermt '2026-10-12 00:00:00'` from `~/.claude/projects`; `-newermt '2026-10-13 16:42:00'` gives the same 24 `pf` forks.
- No fork ran with `kickstart_pack_file: false`, and none ran the `d8d1c0956`-only layout.

## Do forks use the pack?

All 24 `pf` forks opened the pack file, in assistant turn 2, before any edit.
They do not read all cards and spans in one parallel turn as the file asks.

| Metric | `pf` | `raf` |
| --- | --- | --- |
| Spans reached before the first edit (median per fork) | 24% | 17% |
| Spans reached before the first edit (pooled) | 27% | 20% |
| Spans reached in the first work turn (pooled) | 4% | 1% |
| Forks reaching at least half the spans before the first edit | 6 of 24 | 3 of 31 |

## `pf` against `raf` (medians per card)

| Metric | `pf` | `raf` |
| --- | --- | --- |
| First work turn with 3 or more parallel calls | 7 of 24 (29%) | 16 of 31 (52%) |
| Calls before the first edit (net of the pack-file read) | 8.5 | 8 |
| Exploration calls (net) | 4.8 | 5.8 |
| Whole-file reads (net of the pack-file read) | 2.8 | 2.2 |
| Turns | 19.5 | 15 |
| Output tokens | 16.0k | 12.6k |
| Cache read | 3.14M | 2.50M |
| Quarry lookups | 0 | 0 |

Pooled exploration is 5.6 against 5.5, with overlapping quartile ranges.
Edits outside the listed spans are 27% in both arms.
`pf` forks did about 25% more edits per card and about 14% more tool calls (median), so most of the cost gap is probably task size, not layout.

## Verdict

Forks open the pack file but do not read its spans in one turn; `## Start here` did not improve first-turn parallelism (29% against 52%).
Exploration per card is no different, and the cost gap is probably workload, not separable here.
Too small to tell: cost per card, the first-turn share, and anything per run type (10 `pf` runs, one with 6 forks; no significance test).

## Pack correctness

Spans of each `pf` pack were checked against the tree at the batch's `startSha`, taken from the records repo's `state.json` and matched by spawn time; trees from `git show` in the shared object store.

- 885 span entries; 823 checked in 21 forks.
- Correct: 801; correct, declaration only: 22 (grouped const/var blocks, test vars, fields).
- Stale, wrong or missing file: 0 each; no package or directory mismatch.
- Unresolved: 388, all `plan:` handles for members a card creates; none failed to resolve.
- Not checkable: 62 entries in 3 forks (a tender fork at 14:55Z and a twill-fork0 fork at 05:51Z have no matching batch in the records repo; the `reed-window-order` `startSha` object is gone).
- Control: the batch 4, 7 and 13 packs of `shed-tackler` checked against the batch 1 tree give 18 of 96 entries stale or wrong, so the check catches drift; for example `battenCLI.battenPostRun` listed at `arm.go:690-704` sits 43 lines earlier, and `loomcli#driveEnvelopePath` is absent from `drivewiring.go`.
- Limit: this shows the resolver is right at batch start; drift during a batch was not measured.
- Missed code: in the 21 checkable forks, 327 of 1774 edits (18%) went to 167 files that existed at batch start and had no span, mostly md, yaml and tests; Go source was 19 files and 39 edits (e.g. `internal/shedverbs/spec.go` 5, `internal/loomcli/tackler.go` 4), which the cards named as bare paths.

## Do plan writers use glyphs?

Plans from the records repo (`git show <weft commit>:_lyx/plan/...`).

| Metric | `pf` plans | `raf` plans |
| --- | --- | --- |
| Plans (runs) | 10 | 15 |
| Cards | 89 | 138 |
| References | 1516 | 1873 |
| Glyphs of existing members | 842 | 1560 |
| `plan:` handles | 392 | 2 |
| Bare files | 282 | 311 |
| Bare directories | 0 | 0 |
| Cards with no glyph | 1 of 89 | 0 of 138 |

Glyphs and `plan:` handles are 81% (`pf`) and 83% (`raf`) of references; almost every card also names some bare file.
All 842 `pf` glyphs of existing members resolved.
Bare paths where a glyph was natural: `shed-tackler` card 02 lists 9 whole `internal/loomcli` and `arbiterstop` Go files under Edit; `overview-seats` card 17 lists `shedadapters/shuttle.go`, `gateattempts.go` and `archive.go`; `tender` card 07 lists `proc_linux.go` and `proc_windows.go`.

| Arm | Writers | Quarry calls | Greps | Reads |
| --- | --- | --- | --- | --- |
| `pf` | 9 | 85 | 807 | 764 |
| `raf` | 14 | 145 | 1359 | 1190 |

Quarry is about 10% of plan-writer lookups in both arms; `pf` used `glyphs` 77 and `resolve` 8 times, `raf` `glyphs` 119 and `resolve` 26; `expand` and `toc` were never called.
No writer session was found for `webster-pack-layout-switch`.

## Could not reconstruct

The start tree for the 3 forks above, mid-batch drift of pack line numbers, and the `raf` runs' pack trees (not checked).
