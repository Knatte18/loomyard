# Darn cost measure

The scripts and tables behind [report.md](report.md): the cost of darn runs by phase, and a darn family against tapestry runs,
read from the runs that landed since 2026-10-25.
The tables record one run on one machine and are not claiming to stay true.

## Rerunning

Run each script from any working directory; the repo root is found from the script's location.
Environment:

- `WEFT` names the records repo clone (default `<repo root>/../loomyard-weft`).
- `CODE` names the code repo clone (default `<repo root>/../loomyard`).
- `SINCE` (`collect.sh`) names the first UTC date (default 2026-10-25).
- `MAXLINES`, `EXCLUDE`, `EXCLUDE_DARN` and `EXCLUDE_TAPESTRY` (`tables.sh`, `family.sh`, `sonnet-writer.sh`)
  are named in each script's header.

The scripts need `bash`, `git`, `jq` and `awk`.
Everything is read-only apart from the tables.

## Order and inputs

1. `go run ./tools/tokencount -since 2026-10-25 -out <file>` writes the transcript report,
   and `parse-tokencount.sh <file>` splits it into `parts.tsv` and `producers.tsv`.
2. `lym report timing --since 2026-10-25 --format json`, run from the hub's prime worktree and saved to a file,
   feeds `run-table.sh <file>`, which writes `run-table.tsv`.
3. `collect.sh` writes `runs.tsv` and `sessions.tsv` from the usage records at the archive tags.
4. `cost-table.sh` joins them into `cost-table.tsv`, one row per run.
5. `tables.sh`, `family.sh` and `sonnet-writer.sh` print the report's tables from `cost-table.tsv`, `family-slugs.txt`,
   `family-pre.tsv` and `sessions.tsv`.

`family-slugs.txt` lists the parts of the `fabric-test-speed` raddle proposal.
`family-pre.tsv` holds the Motif and raddle runs of that slug, read by `tokencount -no-cutoff fabric-test-speed`.
