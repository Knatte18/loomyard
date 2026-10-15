# Webster pack use and correctness audit, 2026-10-15

This directory keeps the scripts and result tables of the third glyph use audit, a follow-up to `../glyph-audit-2026-10-13/`.
The scratch directory it came from, `.scratch/glyph-audit-3/`, is git-ignored and gone from task worktrees.
Scripts and `COMMANDS.md` may still name `.scratch/glyph-audit-3/` paths (`COMMANDS.md` writes it as `$A`), and the scripts read transcripts from `~/.claude/projects/` and the records repo `loomyard-weft` at fixed absolute paths; none of that is fixed in the copies.

## What it measures

Input is the Claude Code transcripts of Webster implementer forks, from 2026-10-12 on.
- Pack-file layout forks (`pf`, 24 forks): forks after commit `1e39afc10`, whose prompt holds `## Start here` and reads the pack from a file.
- Read-all-first forks (`raf`, 31 forks): the 2026-10-13 audit's arm, whose prompt holds the inline `## Kick-start pack` and the read-all-first phrase.
- Per fork it counts tool calls by class, calls before the first edit, turns and token usage, and compares the two arms (`table.tsv`, `perfork-pf.tsv`, `quartiles.tsv`).
- Pack correctness at batch start: each span entry in a `pf` fork's pack is checked against the repository tree at the batch's `startSha`, resolved through the records repo (`batches.tsv`, `pack-check-perfork.tsv`).
  A negative control checks packs of batches 4, 7 and 13 against batch 1's tree (`control-batches.tsv`, `control-check.tsv`).
  Files a fork edited or read that the pack gave no span for are in `missed-files.tsv`.
- Plan writers' glyph use: the plan cards of each run are read from the records repo and their references counted (`plan-runs.tsv`, `plan-stats.tsv`), and the plan-writing sessions' tool use is counted (`plan-writers.tsv`, `plan-writers-summary.tsv`).

## Provenance

`report.md` was written by the orch from the audit agent's returned text, because the agent's own Write of it was refused.
`COMMANDS.md` records every command in order, the numbered steps that reproduce each table in `report.md` and a list of read-only orientation commands.

## Files

| File | Role |
| --- | --- |
| `report.md` | Findings and verdict |
| `COMMANDS.md` | Every command, in order |
| `run-extract.sh` | Finds fork transcripts, prefilters on `# Webster implementer job`, runs `extract-pf.jq` per file into `raw.jsonl` |
| `run-old-extract.sh` | Cross-check: runs the previous audit's `extract2.jq` over the same transcripts into `raw-old.jsonl` |
| `extract-pf.jq`, `extract2.jq` | Per-transcript extractors; `extract2.jq`, `stats2.jq`, `stats3.jq` and `stats-base.jq` are the previous audit's scripts copied unchanged |
| `stats-pf.jq`, `perfork-pf.jq`, `quartiles.jq` | Produce `table.tsv`, `perfork-pf.tsv`, `quartiles.tsv` from `raw.jsonl` |
| `resolve-batches.sh` | Maps each `pf` fork to its Webster batch and `startSha` through the records repo, from `pf-forks.tsv`, into `batches.tsv` |
| `pack-entries.jq`, `touched-files.jq` | Per-transcript extractors for pack entries and for files a fork touched |
| `check-pack.sh` | Checks each pack span entry against the tree at `startSha` (`pack-check.tsv`, `pack-unres.tsv`); `BATCHES` and `OUT` select the control run |
| `missed-files.sh` | Produces `missed-files.tsv` |
| `plan-refs.sh`, `plan-stats.jq` | Plan reference census into `plan-runs.tsv`, `plan-refs.tsv`, then `plan-stats.tsv` |
| `plan-writer.sh`, `plan-writer.jq` | Tool use of the plan-writing sessions into `plan-writer.tsv`; `plan-writers.tsv` is derived from it in `COMMANDS.md` section 4 |
| `summaries.sh` | Reshapes the raw tables into `summaries.txt` and `plan-writers-summary.tsv` |
| `table.tsv`, `quartiles.tsv`, `perfork-pf.tsv`, `pf-forks.tsv`, `batches.tsv`, `control-batches.tsv`, `pack-check-perfork.tsv`, `control-check.tsv`, `missed-files.tsv`, `plan-runs.tsv`, `plan-stats.tsv`, `plan-writers.tsv`, `plan-writers-summary.tsv`, `summaries.txt` | Result tables |

## Left out

These were bulky raw data and are not kept.
- `raw.jsonl` and `raw-old.jsonl`: the per-fork records of the new and old extractors.
- `cand-all.txt` and `webster-files.txt`: the candidate and Webster transcript path lists, 794 and 101 paths.
- `pack-check.tsv` and `pack-unres.tsv`: the per-entry pack check; `pack-check-perfork.tsv` and `summaries.txt` hold their counts.
- `plan-refs.tsv` and `plan-writer.tsv`: the per-reference and per-session plan data; `plan-stats.tsv` and `plan-writers.tsv` hold their summaries.
- `.runs.txt` and `.spans.txt`: intermediate files the scripts rewrite.

## Re-running

The steps are in `COMMANDS.md`; this is how they depend on each other.
1. The scripts write into their own directory (`here="$(cd "$(dirname "$0")" && pwd)"`), so run them from a directory that holds the scripts and the tables they read.
   `run-extract.sh` and `run-old-extract.sh` change to `~/.claude/projects` and use a find window starting 2026-10-12 00:00.
   `check-pack.sh`, `missed-files.sh` and `resolve-batches.sh` hard-code `/home/knatte/Code/loomyard-LYXHUB/loomyard`; `resolve-batches.sh` and `plan-refs.sh` also hard-code the records repo `/home/knatte/Code/loomyard-LYXHUB/loomyard-weft`.
2. `run-extract.sh` builds `cand-all.txt`, `webster-files.txt` and `raw.jsonl`; `run-old-extract.sh` needs `webster-files.txt`.
3. `jq -s -r -f stats-pf.jq raw.jsonl > table.tsv`, and likewise `perfork-pf.jq` and `quartiles.jq`.
4. `pf-forks.tsv` is built with the `jq | awk | sort` pipeline in `COMMANDS.md` section 3, then `resolve-batches.sh`, `check-pack.sh` and `missed-files.sh` in that order.
   The control run overwrites `pack-unres.tsv`, so `COMMANDS.md` re-runs `check-pack.sh` afterwards.
   `pack-check-perfork.tsv` comes from an `awk` one-liner in `COMMANDS.md`.
5. `plan-refs.sh` needs `batches.tsv` and `raw.jsonl`; `plan-writer.sh` needs `plan-runs.tsv`.
   `plan-stats.tsv` and `plan-writers.tsv` come from the commands in `COMMANDS.md` section 4.
6. `summaries.sh` runs last, as it reads the outputs of the earlier steps.

`check-pack.sh` and `missed-files.sh` need the transcripts and the `startSha` objects to still exist; a batch whose start tree is not reconstructable is classed `notree`.
Section 0 of `COMMANDS.md` copies the previous audit's four scripts from `docs/research/glyph-audit-2026-10-13/` into the scratch directory; they are also kept here.
