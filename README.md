# LoomYard

LoomYard is a multi-agent orchestration system for software development.
It takes a task from a written request to a reviewed change merged into the project.
You talk to one agent, the orch, which oversees many others:
each task's run brings in agents of its own to discuss, plan, implement and review the change,
and many runs go at once.
The agents are Claude Code sessions, and support for OpenAI Codex sessions is under development.

LoomYard keeps the workflow in a program around the agents, not in their prompts.
At its core is **`lym`**, one Go binary.
It decides what happens next, keeps the state, runs git and checks each agent's output files against fixed rules.
Agents do only the work that needs judgment, such as writing a plan or reviewing a diff, and each gets a narrow job.

- **Written plans.** A plan is a list of small cards that later steps read.
  `lym` rejects a card that names a function the code does not have, without saying it creates it.
- **Fresh reviewers.** Reviews run in fresh sessions, never in the session that wrote the work.
- **Resumable runs.** The steps of a run are fixed and recorded, so a crashed run resumes where it stopped.
- **Separate checkouts.** Each task works in its own git checkout.
  Tasks the orch labels as touching the same code run one after the other.
- **Tracked state.** `lym` keeps each run's state in git, and the board shows where each task stands.

Since late September 2026, LoomYard has been developed with LoomYard, and many runs have landed.
The records of every one are public in [loomyard-weft](https://github.com/Knatte18/loomyard-weft).

## From a task to a landed commit

You describe a task to the **orch**, the Claude session you talk to.
The orch files it on the **board**, the project's task list, and you release it by telling the orch.
**Dobby**, a background program, then starts a **run**: one walk through the fixed steps of the task's **recipe**.
A released run can go from start to landing without you; it stops for you when it halts.

![You describe a task to the orch, which files it on the board; when you release it, dobby starts a run. The run goes through discussion, plan, implementation, code review, landing and cleanup, with lym checking the output between steps. The decision record, the plan and the diff each pass review rounds: a fresh reviewer, a fresh fixer, then a judge who decides whether another round is needed, and a spent budget halts the run.](docs/loomyard-run.svg)

The `tapestry` recipe, for a task big enough to need a discussion and a plan:

| Step | What happens |
|---|---|
| Start | Dobby makes the task's **worktree pair**: a git checkout of the project, and beside it a checkout of the **records repository** that holds LoomYard's files. |
| Discussion | An agent turns the board entry into a **decision record**, also called a sketch: scope, decisions taken, what is left out. |
| Plan | An agent writes cards that name the code they touch by symbol. `lym` looks each symbol up in a syntax index of the real tree, so a plan naming a function that does not exist fails before any code is written. |
| Implementation | `lym` groups the cards into batches. One agent reads the plan once and forks an implementer per batch; `lym` runs each batch's tests and records its commits. |
| Code review | The run merges the parent branch into the task, so the review sees the change as it will land. The project's regression gate, if it has one, runs here. |
| Landing | An agent writes the change description, which becomes the landing commit's message. The task lands through a GitHub pull request or straight into its parent branch, one task at a time. |
| Cleanup | An agent files what was hard or broken in LoomYard's own tooling as issues against LoomYard. The worktree pair is torn down. |

The decision record, the plan and the final diff each pass **review rounds** before the run moves on.
A fresh reviewer reviews the artifact and a fresh fixer fixes what it found.
A **judge**, who reads the review and the earlier rounds but never the artifact, decides whether another round is needed.
The reviewer, the fixer and the judge are never the session that wrote the artifact.
They are Claude sessions like the writer, though, so a review can share the writer's blind spots.
Each review has a budget of rounds, by default 2 for the decision record, 3 for the plan and 1 for the code.
A run that spends its budget without settling halts, and the orch asks you.

At the end of each agent turn, `lym` checks the files the agent handed back.
A failed check goes back into the same session, and the run does not move on until it passes.
After a set number of failed attempts the run halts.

### Recipes

| Recipe | For |
|---|---|
| `tapestry` | A task big enough to need a discussion and a plan. |
| `darn` | A change one writer can make, test and commit; it replaces discussion, plan and implementation with one writer. |
| `motif` | Settling a design in a decision record, with no code. |
| `sampler` | A change you work out live with an agent. |

A task with no recipe runs the hub's default, `darn` unless the hub changed it.
The recipe files are data, in [`contracts/recipes/`](contracts/recipes/).

## Beyond the run

- **Mechanical gates.** Tests run in tiers, from fast to slow, and only for the code a change reaches.
  A project with numerical output can add a regression gate that compares against a baseline taken before the change.
  The gates support Go and .NET projects today.
- **Review lenses in forks.** A reviewer can fork one subagent per lens, a focus such as missing tests.
  A fork inherits its parent's context and prompt cache, so it starts without re-reading what the parent read.
- **Parallel tasks, beams and machines.** Tasks run in parallel.
  A project on the board can become a **beam**, a long-lived branch its tasks land into and that lands as one unit.
  A run's state is pushed with the records repository, so several machines can work on different projects at once.
- **Interactive sessions.** Every agent runs in an interactive terminal as a [Claude Code](https://claude.ai/code) session,
  with its skills, hooks and subagents; attach to any session with `lym reed attach`.
- **Messaging through hooks.** `creel`, the hub's message service, delivers messages from the orch, other agents and `lym`
  into running sessions through Claude Code's hooks.
  Delivery through Codex's hooks is under trial.
- **A long-lived orch.** A watcher clears or compacts the orch's context before the window fills.

## Measurements from its own runs

Every run leaves its transcripts, review rounds and token usage behind,
and changes to LoomYard's settings are proposed from measurements of them.
[`docs/research/`](docs/research/) holds those measurements, each made by an agent as a task of its own.
A person, not an agent, decides which recommendations to adopt.

The samples are small, often under ten runs per arm, and mostly from LoomYard itself.
In the review studies the writing agent grades each finding as real or noise, so the grades are an agent's judgment.
A gating finding below is a blocking or medium one of the kind the judge acts on.

| Question | Sample | Finding | Recommendation | Status |
|---|---|---|---|---|
| [What do later review rounds find?](docs/research/burler/review-setup-audit-2026-10-28/report.md) | 8 landed runs with discussion and plan reviews | In later discussion rounds, 92 of the 94 findings that could be traced concerned text the previous fixer had changed; 37 more could not be traced. Gating findings per round fell from 8.4 to 2.3 between discussion rounds 1 and 3, and from 5.9 to 2.1 between plan rounds 1 and 2. | Review later rounds only as the delta the fixer changed | Not yet decided |
| [Can the fixers run on Sonnet instead of Opus?](docs/research/burler/fixers-on-sonnet-2026-11-03/report.md) | 2 runs per model on one plan and one decision record | Every run, on either model, fixed all 9 plan findings. Over two runs on the decision record, Opus fixed 28 of 30 findings and Sonnet 25 of 30, a gap within noise at two runs. | Plan fixer on Sonnet; decision-record fixer stays on Opus until a larger test | Not yet decided |
| [Do larger implementation batches cost more?](docs/research/webster/batch-budget/batch-budget-trial-2026-11-01/report.md) | Runs with wider batches against earlier runs, not randomized | No sign of a higher cost: the median cost per card was $1.13 over 9 runs with wider batches against $1.15 over 51 earlier runs, and $1.13 against $1.50 on runs of 10 cards or more (6 runs against 18). Every batch finished. | Make wider batches the default | Adopted |
| [Do reviewers need a rule to write concrete findings?](docs/research/burler/concrete-review-findings-2026-11-03/report.md) | 374 findings in 44 review files | Every finding names its place and a fix; in 4 of them the fix is a choice between options. | Add no rule, and skip the planned eight-run test | Adopted |

[`docs/research/README.md`](docs/research/README.md) indexes every study by topic.

### Open data

[loomyard-weft](https://github.com/Knatte18/loomyard-weft), LoomYard's own records repository, is public.
Every run since late September 2026 can be read artifact by artifact:

- **The board.** The `main` branch is the task board, and its
  [README](https://github.com/Knatte18/loomyard-weft/blob/main/README.md) shows where each run stands.
- **Landed runs.** A [tag](https://github.com/Knatte18/loomyard-weft/tags) named for the task keeps the whole run:
  the decision record, the plan's cards, each review round and the change description.
  [`archive/driver-arbiter`](https://github.com/Knatte18/loomyard-weft/tree/archive/driver-arbiter/c1dbcb49a0f5) is one.
- **Runs in flight.** A [records branch](https://github.com/Knatte18/loomyard-weft/branches) per running task.

## Names at a glance

| Name | Meaning |
|---|---|
| hub | the directory that holds one project's worktrees and records |
| worktree | a separate git checkout; each task gets its own |
| prime | the worktree on the project's main branch |
| records repository | the second git repository, holding LoomYard's files for each worktree |
| worktree pair | a worktree and its sibling in the records repository |
| board | the hub's list of tasks, notes and projects |
| recipe | the list of steps a task's run walks |
| run | one task's walk from its board entry to a landed commit |
| decision record | what a task is to do: scope, decisions taken, what is left out; also called a sketch |
| plan cards | the units of a plan, each naming the code it touches |
| review round | a fresh reviewer and a fresh fixer on a written artifact, then the judge |
| judge | the session that decides whether another review round is needed |
| lens | one focus a reviewer hands a forked subagent |
| beam | a long-lived branch that tasks fork from and land into |
| dobby | the background program that starts and drives runs; one per beam |
| orch | the Claude session you talk to, which runs the hub with you; one per beam |
| creel | the hub's message service between sessions |
| lym | the one binary behind all of it |

Command groups such as `lym batten` and `lym reed` take their names from weaving.
Everything is driven through `lym`, whose [Cobra](https://github.com/spf13/cobra) command tree lists itself through `--help`.

## Where to read more

- [`docs/getting-started.md`](docs/getting-started.md): build `lym`, make a hub and start working.
- [`docs/overview.md`](docs/overview.md): the architecture, the full vocabulary, hub and worktree geometry, the board and the map of every package.
- [`docs/benchmarks/`](docs/benchmarks/): measurements of LoomYard's own test suite and daemons.
- [`PATTERN.md`](PATTERN.md): the structural rules of the code, most of them enforced by tests.
- [`contracts/`](contracts/): the recipes, the on-disk format specs and the prompts every agent reads.
- [`plugins/`](plugins/): the Claude Code plugin with the `board` skill.
- [scribe](https://github.com/Knatte18/scribe): the plugin with the shared writing and code conventions the agents follow.
- Each package's `doc.go`: the durable detail of that package.

## Lineage

LoomYard grew out of [Millhouse](https://github.com/Knatte18/millhouse),
a set of Claude Code skills and Python tools for running parallel Claude Code sessions,
and it has since grown well beyond it.
Millhouse drew on:

- [claude-code-plugins](https://github.com/motlin/claude-code-plugins) (Craig Motlin)
- Autoboard (Willie Tran), the largest inspiration; its repository has since been removed
- [skills](https://github.com/mattpocock/skills) (Matt Pocock)
