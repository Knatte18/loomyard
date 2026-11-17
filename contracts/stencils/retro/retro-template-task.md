## Goal

Audit the landed runs below from their records and transcripts,
and propose changes to lym's agent environment, most severe first.
Write one report and its line in the research index, and change nothing else: no production code, no test, no stencil and nothing under Bolt.
File nothing on the board and no issue; the operator picks which proposals become board notes.

## The runs

The runs landed after {{.from}} and up to {{.until}}:

{{.runs}}

## 1. The figures

Run each of these once from the repository root, before you read anything else:

- `{{.tools_dir}}/retro.sh {{.report_dir}} {{.run_slugs}}`
  writes `figures.md` and the tables behind it into the report directory.
  Its header names each table and column.
- `tools/tokencount/tokencount.sh -out {{.report_dir}}/tokencount.md {{.run_slugs}}`
  writes each run's cost per segment and part.
- `tools/forkaudit/forkaudit.sh --runs {{.run_list}} --out {{.report_dir}}/forkaudit.md --work .scratch/forkaudit`
  writes what the Webster implementer forks read; a run with no Webster fork matches none.

Keep every file they write beside the report, and remove `.scratch/forkaudit` after.
When a script fails, say so in the report with its error, and work from the figures the others gave.

## 2. The reading

Read what the figures point at, and nothing they do not.
Read at most {{.read_cap}} moments in all.
A moment is one span of at most 30 turns of one session's transcript,
or one stencil, heddle passage or section of `CLAUDE.local.md` that a span showed an agent acting on.
A session's transcript is `<session>.jsonl` under the run's directory in `~/.claude/projects`,
and a fork's lies under `<session>/subagents/`; read a span with `jq`, never the whole file.

Answer each question for the runs, with the moment that shows the answer:

- Which producer's turns or cost stand out against its median, and what in the session made it so?
- Which serial single lookups could have shared a turn, and which instruction failed to batch them?
- Which commands did a guard refuse, and what led the agent to try them?
- Which verify gate failures went to a fixer or back to the writer, and could a check have caught the fault earlier?
- Which review findings recur across runs or rounds, and of which kind?
- Which instruction had no effect: a sentence the sessions broke, or one they never needed?
- Where did an agent search for something it should have been told: a path, a command, a format?

## 3. The proposals

Route each proposal by the fault it fixes:

- A mechanical fault becomes a check: a shell guard row, a lint, a gate or a test.
- A judgement call becomes a reviewer rubric line.
- An instruction with no effect becomes a deletion.
- A new prompt sentence is the last resort: propose one only when no check, rubric line or deletion fits, and say why.

Each proposal names the file it changes, the route, and the moment it traces to:
the run's slug, the session's first eight characters and the turn.

## 4. The report

Write `{{.report_dir}}/report.md`, and keep the directory's name as it is:
the next `lym retro` reads its stamp from it.
The report holds:

- the runs and the window above;
- the figures that matter to a proposal, cut from `figures.md`, which stays beside the report whole;
- the proposals, most severe first, each with its severity, route, file and moment;
- the questions above that the runs gave no answer to, one line each.

Add one line for the report under a `retro` section of `{{.research_index}}`,
creating the section when it has none.
Every report under `{{.retro_dir}}` is listed there.
