# A8 audit data: Plan writer cost, rounds, wall time, and relevant.md coverage

Measured 2026-10-24 from existing data, read-only.
Cost is API list price from `tools/tokencount` (relative weights, not a bill).

## Q1: Plan writer per loom run

Writer = the Plan-Write session; forks = its twill-plan lens forks; wall time = the Plan-Write producer's elapsed time from tokencount's producer table (the writer and its forks together, including the wait for forks).
Rounds = Plan reviewer sessions (runcost.tsv column 18 where the run is in it, else tokencount).
"Plan phase" = the whole Plan phase (writer, review, fix) from `lyx report timing`.
Runs started 2026-10-20 or later, plus ubuntu-2404-support (seed 10-19, Plan ran across the cut).
Runs after c62274be5 (2026-10-22 15:38 +0200, five lenses) are marked C; the forks column confirms it: 9 fork sessions = full fan, 6 = five lenses.

| run | seed start (UTC) | grp | writer $ | forks $ | writer+forks $ | fork sessions | Plan-Write wall min | rounds | Plan phase total $ | Plan phase wall |
|---|---|---|---|---|---|---|---|---|---|---|
| ubuntu-2404-support | 10-19 14:47 | B? | 8.19 | 6.02 | 14.21 | 12 | 31.9 | 4 | 42.32 | 2h02m |
| machine-handover | 10-22 05:52 | B | 22.96 | 11.43 | 34.39 | 9 | 28.3 | 3 | 53.37 | 1h07m |
| darn-verify-fixes | 10-22 05:52 | B | 7.24 | 5.45 | 12.69 | 9 | 22.0 | 1 | 17.81 | 30m |
| quarry-csharp | 10-22 05:59 | B | 7.95 | 7.68 | 15.63 | 9 | 20.6 | 2 | 29.19 | 58m |
| lenses-language-sets | 10-22 06:47 | B | 4.62 | 5.36 | 9.98 | 9 | 16.2 | 1 | 17.86 | 35m |
| tender-pause-holds-reprompt | 10-22 07:34 | B | 12.00 | 10.25 | 22.25 | 9 | 33.5 | 2 | 31.50 | 52m |
| finalize-dirty-names-paths | 10-22 10:35 | B | 5.24 | 4.66 | 9.90 | 9 | 13.4 | 1 | 14.81 | 21m |
| run-process-ownership | 10-22 10:35 | B | 13.37 | 8.80 | 22.17 | 9 | 33.0 | 4 | 46.72 | 1h35m |
| orch-strand-cycle | 10-22 14:43 | C | 7.96 | 5.97 | 13.93 | 6 | 19.8 | 2 | 28.88 | 51m |
| batten-cancel-verb | 10-22 17:22 | C | 8.17 | 7.15 | 15.32 | 6 | 26.4 | 1 | 24.03 | no timing row |
| plan-card-rules | 10-22 17:22 | C | 11.59 | 9.09 | 20.68 | 6 | 25.4 | 1 | 26.86 | no timing row |

Not in the table: loom-tail-review (landed 10-23, no usable transcripts, timing shows 20h of sleep) and gate-runs-once (in flight, Plan phase 1h28m so far).
No loom run was seeded on 10-20 or 10-21 apart from these; the Plan-Write sample since 10-20 is 10 runs (7 full fan, 3 five-lens).

### Group means (q1-summary.tsv)

| group | runs | Plan phase $ | writer $ | forks $ | writer+forks $ | fork sessions | Plan-Write wall min (elapsed) | Plan rounds |
|---|---|---|---|---|---|---|---|---|
| A1 before 10-15 (solo writer) | 28 | 9.08 | 8.50 | 0.58 | 9.08 | 1.1 | 40.1 (inflated by 3 respawned bugfix runs; active 16.4) | 1.32 (most had no review perch) |
| A2 10-15 to 10-19 (early fan) | 41 | 16.01 | 5.53 | 3.86 | 9.39 | 6.1 | 18.5 | 1.88 |
| B 10-20 to c62274be5 (full fan) | 7 | 30.18 | 10.48 | 7.66 | 18.14 | 9.0 | 23.9 | 2.00 |
| C after c62274be5 (five lenses) | 3 | 26.59 | 9.24 | 7.40 | 16.64 | 6.0 | 23.9 | 1.33 |

B and C runs are larger tasks than A (median about 3,300 changed lines since 10-18), so the cost rise is not only the fan.
tokencount misses some reviewer sessions of the oldest runs (Plan total below runcost.tsv's), so the A1 Plan phase total is a floor; writer figures are unaffected.

## Q2: share of Plan-Write reads that relevant.md named

Method (reads.sh): the transcript with `customTitle` `ly:<slug>:plan`; repo file paths from Read `file_path` and from Bash commands using cat, sed, grep, rg, head, tail, awk, wc, nl, diff; `_lyx/`, `.lyx/`, absolute and non-repo paths dropped.
Named = the first backticked path of each `relevant.md` bullet at the run's archive tag (`_lyx/discussion/relevant.md`); a named directory covers files under it.
Not counted: directory-wide reads (`grep -r dir`, `ls`, globs like `internal/lenses/*`) and `lyx quarry` reads, so the read set is a lower bound, and "named files read" is a lower bound too.
"exdocs" drops PATTERN.md, CLAUDE.md, pattern/ and docs/ from the read set.

See q2-table.tsv for all 32 runs: the loom runs seeded 2026-10-15 to 2026-10-22 that have a relevant.md at their archive tag.
Of the 57 loom runs tried from 10-15 on, 22 have no relevant.md (the Discussion list came in during the window), 2 have no Plan-Write transcript (webster-pack-end-of-prompt, loom-tail-review), and quarry-csharp is a duplicate.
Pooled over the 32 runs: 1,799 repo files read, 510 named (28%); 603 files named, 510 read (85%); excluding docs: 29%.
Mean of per-run shares: 36% of read files named, 85% of named files read.

Representative rows (read / read and named / share; named / named and read / share):

| run | read | read named | % | named | named read | % |
|---|---|---|---|---|---|---|
| machine-handover | 128 | 13 | 10% | 16 | 13 | 81% |
| plan-card-rules | 112 | 25 | 22% | 28 | 25 | 89% |
| run-process-ownership | 104 | 20 | 19% | 25 | 20 | 80% |
| creel-notices | 96 | 30 | 31% | 30 | 30 | 100% |
| tender-pause-holds-reprompt | 86 | 29 | 34% | 31 | 29 | 94% |
| quarry-csharp | 69 | 16 | 23% | 19 | 16 | 84% |
| darn-verify-fixes | 70 | 20 | 29% | 22 | 20 | 91% |
| finalize-dirty-names-paths | 52 | 14 | 27% | 15 | 14 | 93% |
| lenses-language-sets | 34 | 12 | 35% | 13 | 12 | 92% |
| orch-strand-cycle | 65 | 25 | 38% | 33 | 25 | 76% |

## Commands and scripts (all in this directory, run from the prime)

- `go build -o .scratch/a8/tokencount-bin ./tools/tokencount`; `run-all.sh`, `run-priority.sh`: `tokencount-bin -detail -out .scratch/a8/tc/<slug>.md <slug>` per loom slug (196 slugs: runcost.tsv loom rows plus `extra-loom.tsv`, loom archive tags not in runcost.tsv).
- `lyx report timing --since 2026-10-08 --format markdown` saved as `timing-since-2026-10-08.md`; `timing-plan-phase.tsv` is its loom `plan` column.
- `q1-table.sh` then `q1-summary.sh`: `q1-table.tsv`, `q1-summary.tsv`.
- `reads.sh <slug> <archive-tag>` (loop over `sample-input.tsv`), then `q2-table.sh`: `reads/<slug>.{read,named,relevant.md,calls,summary}`, `q2-table.tsv`.
- Archive tags: `git -C ../loomyard-weft tag -l 'archive/*'`; seed and relevant.md via `git -C ../loomyard-weft show <tag>:_lyx/shed/<slug>/seed.json` and `:_lyx/discussion/relevant.md`.
