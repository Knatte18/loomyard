# Darn writer anatomy measure

The scripts and tables behind [report.md](report.md): what the darn writer, its lookup subagents and the cold gate fixer cost,
by token kind and activity, read from the session transcripts of the darn runs listed in `slugs.txt`.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run from any working directory with the output directory as an argument.
`extract-turns.sh` needs `jq` and the transcripts under `PROJECTS` (default `~/.claude/projects`);
the other scripts need `bash` and `gawk`.
Everything is read-only apart from the tables.

1. `extract-turns.sh slugs.txt <dir>` writes `sessions.tsv`, `turns.tsv`, `calls.tsv` and `prompts.tsv`.
2. `turn-costs.sh <dir>` writes `turn-costs.tsv`.
3. `gate-rounds.sh <dir>` writes `fixer-sessions.tsv` and `gate-prompts.tsv`.
4. `per-darn.sh <dir> [cost-table.tsv]` writes `per-darn.tsv`; the optional table is the darn cost measure's, for changed lines.
5. `summary.sh <dir>` prints the tables of the report.
