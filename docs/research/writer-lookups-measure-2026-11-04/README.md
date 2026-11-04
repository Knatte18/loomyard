# Writer lookups measure

The scripts and tables behind [report.md](report.md): how the Plan-Write and Discussion-Write sessions of landed tapestry runs explore the code and use lookup subagents.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run `run-all.sh [out-dir]` from any working directory; the repo root is found from the script's location.
`out-dir` defaults to this folder.

Environment:

- `PROJECTS` names the transcript root; it defaults to `~/.claude/projects`.
- `SCRATCH` names the work directory; it defaults to `<repo root>/.scratch/writer-lookups-measure`.
- `CUTOFF` names the first session date; it defaults to 2026-11-02.
- `LINE_TOKENS` names the context line of the "over" figures; it defaults to 200000.

The scripts need `bash`, `jq` and `awk`.
Everything is read-only apart from the tables and the work directory.

## Adding a run

Add the slug to `slugs.txt`; its transcripts must be under `$PROJECTS/-home-knatte-Code-loomyard-LYMHUB-<slug>/`.
Then rerun `run-all.sh` and update the report's tables.

## Scripts

- `run-all.sh` extracts turns and calls with a copy of the turn audit's `extract-turns.sh` and runs the rest.
- `writers.sh` writes `writer-turns.tsv` (every writer turn, with its kind and cost), `writer-sessions.tsv` (one row per writer session and a total) and `writer-chains.tsv` (every chain of exploration turns).
- `subagents.sh` writes `writer-subagents.tsv`: every Agent call of a writer session, with its batch, kind, model, turns and cost.
- `turns.tsv` and `calls.tsv` hold the writer sessions' rows of the turn audit's extraction.
