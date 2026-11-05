# Webster fork read audit, 2026-10-13

This directory keeps the scripts and results of the second glyph use audit, which compared Webster implementer forks before and after the read-all-first rule (commit `793c6a8ee`).
`report.md` is the result and is copied verbatim from the scratch directory `.scratch/glyph-audit-2/`.
The scripts and `table.tsv` are byte-identical copies too, so paths in them, and the name "previous audit's `extract.jq`", refer to scratch paths that existed at the time and are gone.

## What it measures

Input is the Claude Code transcripts of every Webster implementer fork (model `claude-sonnet-5-5`) from 2026-10-08 to 2026-10-13.
Per fork it counts tool calls by class (whole-file reads, partial reads, greps, member greps, quarry lookups), calls before the first edit, exploration calls between a commit and the next edit, turns, and token usage.
It reads the kick-start pack from the fork prompt (`## Kick-start pack`): resolved and unresolved entries, and the files that have a span.
It checks whether the fork read its pack spans in one parallel first work turn, and how many edits went to files with no listed span.
Forks are sorted into four arms by what the prompt contains: `base_off` (no pack), `base_on` (pack before `113593990`), `mid` (per-card pack without read-all-first), `raf` (prompt holds the read-all-first rule).
The main outcomes are exploration calls per card and the share of forks with 3 or more parallel calls in the first work turn.

## Files

| File | Role |
| --- | --- |
| `extract2.jq` | Per-transcript extractor, one JSON object per fork |
| `stats-base.jq` | Pack-on versus pack-off medians and means, from the first audit |
| `stats2.jq` | Per-arm table of medians and pooled values, produces `table.tsv` |
| `stats3.jq` | Per-arm pack-span hit share and edits outside the listed spans |
| `table.tsv` | Output of `stats2.jq`, sample sizes per arm |
| `turns.tsv` | Per fork `arm` and `{pre_turns, first_par, par}`, the first-turn parallelism data |
| `report.md` | Findings and verdict |

`raw.jsonl`, `raw-pack.jsonl` and `files.txt` (the list of 1089 transcript paths) were not kept, being bulky.

## Re-running

1. Collect the transcripts.
   `files.txt` held relative paths of the form `./-home-knatte-Code-loomyard-LYXHUB-<run>/<session-id>/subagents/agent-<id>.jsonl`, so they sit under a per-project directory of the Claude Code projects store (presumably `~/.claude/projects/`) and the working directory was that store.
   The `/subagents/` path part is what marks a fork.
   How `files.txt` was built (the find command and its date filter) is not recorded.
2. Extract one record per transcript.
   `extract2.jq` starts with `. as $all` and uses `input_filename`, so it must see all lines of one file as one array: `jq -s -c -f extract2.jq <file>` for each path in `files.txt`, output concatenated.
   It emits nothing for a transcript without a `# Webster implementer job` prompt.
   The exact command and the split between `raw.jsonl` and `raw-pack.jsonl` are not recorded.
   The records in `raw-pack.jsonl` carry the extra fields `nsf` and `pre_hit` that `stats3.jq` needs, and the `raw.jsonl` records seen lack them, so `raw-pack.jsonl` looks like an extraction run restricted to pack forks, or an earlier output of a different version of the script.
   Which is not recorded.
3. Build the arm table.
   `stats2.jq` expects the records as one array: `jq -s -r -f stats2.jq raw.jsonl > table.tsv`.
   Which of the two raw files fed the medians in `report.md` is not recorded, nor is the exact command.
4. Pack-span hit and edits outside spans.
   `stats3.jq` also expects one array: `jq -s -r -f stats3.jq raw-pack.jsonl`.
   It groups by arm and prints hit share, edits outside the listed spans and writes.
5. Pack on versus off.
   `stats-base.jq` takes `--arg from <ISO timestamp>` and the array form: `jq -s -r --arg from <timestamp> -f stats-base.jq raw.jsonl`.
   The `$from` value used is not recorded, and `report.md` does not cite its output.
6. First-turn parallelism.
   `turns.tsv` holds `arm`, tab, JSON with `pre_turns`, `first_par` and `par` (tool calls per assistant turn before the first edit).
   The script that produced it is not among the files and not recorded; `report.md` calls it the "turns.tsv recipe".
   The report's first-turn figures (median 3, 15 of 29 forks with 3 or more) come from it.

The pack-quality figures and the 96 percent `plan:` share in `report.md` were taken from the fork prompts, and the command is not recorded either.
