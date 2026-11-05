# LoomYard

LoomYard is a multi-agent orchestration system for software development.
It takes a task from a written request to a reviewed change merged into the project.
You talk to one agent, the orch, which oversees many others:
each task's run brings in agents of its own to discuss, plan, implement and review the change, and many runs go at once.
The agents are Claude Code sessions, and support for OpenAI Codex sessions is under development.

LoomYard keeps the workflow in a program around the agents, not in their prompts.
At its core is **`lym`**, one Go binary that decides what happens next, keeps the state, runs git and checks each agent's output files against fixed rules.
Agents do only the work that needs judgment, such as writing a plan or reviewing a diff, and each gets a narrow job.
The design rests on a few choices:

- **Written plans.** The plan is written down as small cards that later steps read.
  `lym` checks each card, and rejects one that names a function the code does not have, without saying it creates it.
- **Fresh reviewers.** Reviews run in fresh sessions, never in the session that wrote the work.
- **Resumable runs.** The steps of a run are fixed and recorded, so a crashed run resumes at the step where it stopped.
- **Separate checkouts.** Each task works in its own git checkout, and tasks the orch labels as touching the same code run one after the other.
- **Tracked state.** `lym` keeps each run's state in git, and the board shows where each task stands.

Since late September 2026, LoomYard has been developed with LoomYard, and more than 700 runs have landed.
The records of every one are public in [loomyard-weft](https://github.com/Knatte18/loomyard-weft).
Proposed changes to LoomYard's settings come from measurements of those runs, and a person decides on each (see [Measurements from its own runs](#measurements-from-its-own-runs)).

## From a task to a landed commit

You describe a task to the **orch**, the Claude session you talk to.
The orch files the task as a draft on the **board**, the project's task list.
When you release the task, by telling the orch or by clearing its draft status yourself, **dobby**, a background program, starts a **run** for it.
A run is one walk through a fixed list of steps, and the task's **recipe** names that list.
Once released, a run can go from start to landing without you.
It stops for you when it halts, or where its recipe asks for you.

Each project lives in a **hub**, a directory LoomYard makes for it.
Each task works in its own git worktree,
and LoomYard keeps its own files for the task in a separate git repository, the **records repository**.
A task lands into its **parent branch**, normally the main branch.
[Names at a glance](#names-at-a-glance) lists every term this README uses.

![You describe a task to the orch, which files it on the board; when you release it, dobby starts a run. The run goes through discussion, plan, implementation, code review, landing and cleanup, with lym checking the output between steps. The decision record, the plan and the diff each pass review rounds: a fresh reviewer, a fresh fixer, then a judge who decides whether another round is needed, and a spent budget halts the run.](docs/loomyard-run.svg)

The figure and the steps below show the `tapestry` recipe, for a task big enough to need a discussion and a plan.

1. **Dobby starts the run.**
   The run makes the task's **worktree pair**: a git checkout of the project for the code, and a checkout of the records repository beside it for LoomYard's files.
   Everything after that happens there, so other tasks cannot see it.
2. **Discussion.**
   An agent turns the board entry into a **decision record**, also called a sketch: the scope, the decisions taken and what is out of scope.
   The decision record then passes its review rounds.
3. **Plan.**
   An agent writes a plan as a list of cards, small units of work, each naming the code it touches by its symbols.
   Each symbol is looked up in a syntax index of the real tree, so a plan that names a function that does not exist fails before any code is written.
   A large task's plan can first be split into parts.
   The plan then passes its review rounds.
4. **Implementation.**
   `lym` groups the cards into batches by their estimated size.
   The implementation agent reads the plan once.
   For each batch it forks an implementer, a subagent that inherits what the agent has read.
   `lym` runs each batch's tests and records its commits.
5. **Code review.**
   The run merges the parent branch into the task, so the review sees the change as it will land.
   The whole diff then passes its review rounds.
   The project's regression gate, if it has one, runs here as well.
6. **Landing.**
   An agent writes the change description, which becomes the landing commit's message.
   Depending on the hub's settings, the task lands through a GitHub pull request or straight into its parent branch.
   You can discuss a pull request with an agent session in the task's worktree.
   Only one task at a time lands on a parent branch.
7. **Cleanup.**
   An agent reflects on what was hard or broken in LoomYard's own tooling and files it as issues against LoomYard.
   The task's worktree pair is then torn down and the board entry is marked done.

A `darn` run, for a smaller change, replaces steps 2 to 4 with one writer, who writes a short decision record and the code;
the code review, landing and cleanup are the same.

The decision record, the plan and the final diff each pass **review rounds** before the run moves on.
In each round, a fresh reviewer session reviews the artifact and a fresh fixer session fixes what it found.
A **judge** then reads the review and the record of earlier rounds, never the artifact itself, and decides whether another round is needed.
A fix the judge rates as mechanical is accepted without another review;
otherwise the next round's fresh reviewer reviews the fixed artifact.
The reviewer, the fixer and the judge are never the session that wrote the artifact.
They are Claude sessions like the writer, though, so a review can share the writer's blind spots.
Each review has a budget of rounds, by default 2 for the decision record, 3 for the plan and 1 for the code.
A run that spends its budget without settling halts, and the orch asks you, instead of looping.

At the end of each agent turn, `lym` checks the files the agent handed back.
When they fail the check, `lym` sends the findings back into the same session, and the run does not move on until the check passes;
after a set number of failed attempts the run halts.

A run is carried by three kinds of process.
Dobby, a `lym` daemon with one instance per beam, decides when each run moves.
It runs `lym batten`, the part of `lym` that walks a recipe, in short passes:
each pass takes the run as far as it can, such as starting an agent or checking its output, then exits, and dobby starts the next pass when something changes.
The agents run in terminals of their own, which the run's steps start, and the run's state lives in the task's records.
The orch steps in only when a run halts: dobby tells it, and it fixes what it can and asks you about the rest.
`lym batten status <slug>` shows where a run stands.

## Beyond the run

1. **Mechanical gates.**
   Tests run in tiers, from fast to slow, and only for the code a change reaches.
   A project with numerical output can add a regression gate that compares its results with a baseline taken before the change.
   The gates support Go and .NET projects today;
   another language needs a reader for its test runner's output and a rule for which tests a change reaches.
2. **Review lenses in forks.**
   A reviewer can fork one subagent per lens, a focus such as missing tests or callers the change breaks.
   A fork inherits its parent's context and prompt cache, so it starts without re-reading what the parent read.
3. **Parallel tasks, projects and machines.**
   Tasks run in parallel, except that the orch makes tasks whose labels overlap run one after the other (see [The board](#the-board)).
   A project on the board can become a beam, a long-lived branch its tasks land into, which then lands into the main branch as one unit.
   A run's state is pushed with the records repository, so several machines can work on different projects at the same time.
4. **Interactive agent sessions.**
   Every agent runs in an interactive terminal as a [Claude Code](https://claude.ai/code) session, with its skills, hooks and subagents, and you can attach to any of them with `lym reed attach` and watch.
5. **Messaging through hooks.**
   `creel`, the hub's message service, delivers messages into running agent sessions.
   It uses Claude Code's hooks, commands Claude Code runs at fixed points such as after each tool call, and Codex's hooks in the same way for a Codex run.
   The senders are the orch, other agents and `lym` itself.
6. **A long-lived orch.**
   A watcher clears or compacts the orch's context before the window fills, so the orch session continues past one context window.

Under trial: `creel` messages to Codex sessions through Codex's hooks, as part of the Codex support.
Planned: a Slack integration for `creel`.

## Measurements from its own runs

Every run leaves its transcripts, review rounds and token usage behind, and changes to LoomYard's settings are proposed from measurements of them.
[`docs/research/`](docs/research/) holds those measurements, each made by an agent as a task of its own.
Each report states its question, sample and method, and ends in a recommendation.
A person, not an agent, decides which recommendations to adopt.
Most reports keep their scripts and tables beside them, so the measurement can be rerun.

The studies measure one project, mostly LoomYard itself.
Their samples are small, often under ten runs per arm, and each report lists its own limits.
In the review studies, the agent writing the report grades each finding as real or noise against the code, so the grades are an agent's judgment, not a person's.

Reviewers rate each finding from blocking through medium and low to nit.
A gating finding below is a blocking or medium one of the kind the judge acts on.

| Question | Sample | Finding | Recommendation | Status |
|---|---|---|---|---|
| [What do later review rounds find?](docs/research/burler/review-setup-audit-2026-10-28/report.md) | 8 landed runs with discussion and plan reviews | In later discussion rounds, 92 of the 94 findings that could be traced concerned text the previous fixer had changed; 37 more could not be traced. Gating findings per round fell from 8.4 to 2.3 between discussion rounds 1 and 3, and from 5.9 to 2.1 between plan rounds 1 and 2. | Review later rounds only as the delta the fixer changed | Not yet decided |
| [Can the fixers run on Sonnet instead of Opus?](docs/research/burler/fixers-on-sonnet-2026-11-03/report.md) | 2 runs per model on one plan and one decision record | Every run, on either model, fixed all 9 plan findings. Over two runs on the decision record, Opus fixed 28 of 30 findings and Sonnet 25 of 30, a gap within noise at two runs. | Plan fixer on Sonnet; decision-record fixer stays on Opus until a larger test | Not yet decided |
| [Do larger implementation batches cost more?](docs/research/webster/batch-budget/batch-budget-trial-2026-11-01/report.md) | Runs with wider batches against earlier runs, not randomized | No sign of a higher cost: the median cost per card was $1.13 over 9 runs with wider batches against $1.15 over 51 earlier runs, and $1.13 against $1.50 on runs of 10 cards or more (6 runs against 18). Every batch finished. | Make wider batches the default | Adopted |
| [Do reviewers need a rule to write concrete findings?](docs/research/burler/concrete-review-findings-2026-11-03/report.md) | 374 findings in 44 review files | Every finding names its place and a fix; in 4 of them the fix is a choice between options. | Add no rule, and skip the planned eight-run test | Adopted |

The index [`docs/research/README.md`](docs/research/README.md) lists every study by topic.
The other studies measure token use and cost per role, the cost of forking a review over lenses, how many turns and lookups the writers spend, and the effect of the hooks that steer them.

### Open data

LoomYard's own records repository, [loomyard-weft](https://github.com/Knatte18/loomyard-weft), is public, so every LoomYard run since late September 2026 can be read artifact by artifact.

- **The board.** The `main` branch is the task board.
  Its [README](https://github.com/Knatte18/loomyard-weft/blob/main/README.md) shows running, ready and blocked tasks and where each run stands.
- **Landed runs.** When a task lands, its records branch is deleted and a [tag](https://github.com/Knatte18/loomyard-weft/tags) named after the task and the run's last commit keeps its whole run: the decision record, the plan's cards, each review round and the change description.
  [`archive/driver-arbiter`](https://github.com/Knatte18/loomyard-weft/tree/archive/driver-arbiter/c1dbcb49a0f5) is one such run.
- **Runs in flight.** A [records branch](https://github.com/Knatte18/loomyard-weft/branches) per running task.

## Under the hood

### Hub and worktrees

A **hub** holds everything LoomYard works with for one project.
Inside it, each line of work lives in its own git **worktree**, a separate checkout of your repository, so tasks do not share a working directory.
The first worktree, on your main branch, is the **prime**.

Every worktree has a sibling in a second git repository, the **records repository**, which rides piggyback on the project's.
The sibling holds LoomYard's own files for that worktree: configuration, decision records, plans, review rounds and run status.
A worktree and its sibling form a **worktree pair**.
The two repositories branch together, so a task's files and run status are versioned and can be picked up on another machine.
The project's commits hold only the code changes, and its `.gitignore` stays free of LoomYard.

```
<hub>/
  ├── <project>/             your repository, on its main branch (the prime)
  ├── <project>-<records>/   its sibling in the records repository (<records> is a suffix lym sets)
  ├── <task>/                a task's worktree, on the task's branch
  ├── <task>-<records>/      its sibling in the records repository
  └── _bolt/                the records repository's main branch, which holds the board
```

### The board

The **board** is kept in the records repository and rendered as a README there.
An entry is a **task**, a **note** or a **project**.
A task is work to do, and it can depend on other tasks.
A note is an idea that is not ready to be done, and it is promoted to a task when it is.
A project groups entries.

Each entry carries labels for the parts of the code it touches.
The orch gives tasks whose labels overlap a dependency on each other, so they run one after the other;
the rest run in parallel.

A task carries a title, a body that says what is wanted, labels and a recipe:

- `darn`, for a change that one writer can make, test and commit.
- `tapestry`, for a task big enough to need a discussion and a plan.
- `motif`, for settling a design in a decision record, with no code.
- `sampler`, for a change you work out live with an agent.

The recipe files are data, in [`contracts/recipes/`](contracts/recipes/).
A task with no recipe runs the hub's default recipe, `darn` unless the hub changed it.

### Beams

A **beam** is a long-lived branch that tasks fork from and land back into, and the prime is the first one.
A project on the board can become a beam of its own, and its tasks then fork from the beam's branch and land into it.
The beam lands into its parent when you flag it ready.

Each beam has its own dobby and orch.
Dobby starts a released task once the tasks it depends on are done, and drives at most a set number of runs at once, set per beam.

### `lym`

Everything above is driven through `lym`.
Its commands form a [Cobra](https://github.com/spf13/cobra) command tree, so the orch and every agent can list the commands and their flags through `--help`.

### Names at a glance

| Name | Meaning |
|---|---|
| hub | the directory that holds one project's worktrees and records |
| worktree | a separate git checkout; each task gets its own |
| prime | the worktree on the project's main branch |
| records repository | the second git repository, holding LoomYard's files for each worktree |
| worktree pair | a worktree and its sibling in the records repository |
| board | the hub's list of tasks, notes and projects |
| recipe | the list of steps a task's run walks: `darn`, `tapestry`, `motif` or `sampler` |
| run | one task's walk from its board entry to a landed commit |
| decision record | what a task is to do: its scope, the decisions taken and what is left out; also called a sketch |
| plan cards | the units of a plan, each naming the code it touches |
| review round | a fresh reviewer and a fresh fixer on a written artifact, then the judge |
| judge | the session that decides whether another review round is needed |
| lens | one focus a reviewer hands a forked subagent, such as missing tests |
| beam | a long-lived branch that tasks fork from and land into |
| dobby | the background program that starts and drives runs |
| orch | the Claude session you talk to, which runs the hub with you |
| creel | the hub's message service between sessions |
| lym | the one binary behind all of it |

Command groups such as `lym batten` and `lym reed` take their names from weaving;
[`docs/overview.md`](docs/overview.md) holds the full vocabulary and a detailed architecture figure.

## Where to read more

- [`docs/getting-started.md`](docs/getting-started.md): how to build `lym`, make a hub and start working.
- [`docs/overview.md`](docs/overview.md): the architecture, the naming and the map of every package.
- [`docs/benchmarks/`](docs/benchmarks/): measurements of LoomYard's own test suite and daemons.
- [`PATTERN.md`](PATTERN.md): the structural rules of the code, most of them enforced by tests.
- [`contracts/`](contracts/): the recipes, the on-disk format specs and the prompts every agent reads.
- [`plugins/`](plugins/): the Claude Code plugin with the `board` skill.
- [scribe](https://github.com/Knatte18/scribe): the plugin with the shared writing and code conventions the agents follow.
- Each package's `doc.go`: the durable detail of that package.

## Lineage

LoomYard grew out of [Millhouse](https://github.com/Knatte18/millhouse), a set of Claude Code skills and Python tools for running parallel Claude Code sessions.
Through Millhouse it builds on ideas from [claude-code-plugins](https://github.com/motlin/claude-code-plugins) (Craig Motlin), [autoboard](https://github.com/willietran/autoboard) (Willie Tran) and [skills](https://github.com/mattpocock/skills) (Matt Pocock).
