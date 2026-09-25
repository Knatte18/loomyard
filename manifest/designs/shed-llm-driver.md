# shed: the LLM driver as a generic stepper and mender

## The problem

Every crucible campaign against a live-substrate module ends with more findings than it started with, and none has reached a safety pass.
The later findings are crash windows: a process killed mid-`git push`, a branch stranded on the remote, a remedy text that fails when followed verbatim.
Go code has to be perfect against these, because nothing downstream of a `lyx` command can see what it did or repair what it left behind.

Millhouse has been useful for a long time without being perfect, for two reasons:

- An LLM carries out most of what it does from instructions, and sees and repairs what the instructions and scripts get wrong.
- It runs Python, so when something goes wrong the LLM opens the script that ran, sees exactly what was done, and knows what to clean up.

This design gives `lyx` both properties without leaving Go: an LLM outer loop over `lyx shed step`, fed by a trace complete enough to repair from.

## The shape

### The seed picks the recipe; the driver knows none

`lyx shed seed <run-id> --recipe <name>` already writes the run's `seed.json`, which `internal/shedrun` owns: the recipe, the driver, and the parameters.
Every recipe already supports every shed verb (`internal/shedcli/table.go`).

`ly-drive` becomes a generic driver of `lyx shed step <run-id>`.
It knows shed's own contract vocabulary — policy words, refusal kinds, envelope fields — and never a recipe, a row name, or a recipe's file layout.
Loom, batten, and Hardener when it lands are all driven by the same loop; which one runs is a property of the seed alone.

### Every step leaves a trace complete enough to repair from

- `internal/logger`'s durable sink already writes one trace file per process under `.lyx/logs/`, opening, appending, and closing the file on every write, so no file stays held or locked between writes.
- `internal/fabricengine`'s `Mutations` already records every mutation a verb call performs, in order, but only in memory: a killed process loses it.
  Each entry is also written to the durable sink as it is appended, so a trace survives a SIGKILL up to the last completed mutation.
- Every mutating step logs at `Info`, not only at `Warn` on failure.
- The step envelope names the trace file the step wrote, and any friction location the recipe uses, so the driver reads exactly what that step did without searching.

### The driver is the mender

On any non-running envelope, the driver reads the step's trace, repairs the state, and steps again.
It escalates to a human only when it cannot repair.

Every repair is recorded as friction, so a crash window the driver heals still surfaces as data instead of disappearing.
Shed's bounce budgets already bound how often a row can come back.

### The orchestrator drives through forks

A repo-wide orchestrator session — the kind a user keeps open in the main worktree, or in prime for batten — drives runs by forking one `ly-drive` loop per run.
The fork carries the step output and trace reading; the orchestrator's own context stays clean.
A fork is a subagent inside the interactive session, so this keeps to the interactive-tmux rule in `CLAUDE.md`.

## What leaves `ly-drive`

Everything the skill knows about loom today moves out:

| Loom knowledge in `ly-drive` today | Where it goes |
|---|---|
| Step cap derived from loom's rows and bounce budgets | Dropped; shed's bounce budgets bound the graph |
| `.lyx/loom/friction/` and loom's self-report rules | The step envelope names the friction location |
| cwd requirement and `lyx loom start` as bootstrap remedy | The recipe's own refusals (`RefuseSeedAt`, `Arm`); the driver reports them |
| Loom's launch convention and the `$TMUX_PANE` self-check | `lyx loom start`'s own documentation |
| What `handback` means for loom | The policy word on the envelope, with no recipe-specific gloss |

## Consequences for the backlog

The fabric crash-state work (`fabric-pair-state-after-crash`) and the remedy-text work (`remedy-texts-followed-verbatim`) shrink to the trace requirements above:
fabric no longer has to resume every partial state itself, and a remedy no longer has to be a command that works verbatim, since the driver reads the trace and acts.

## Open

- `BootstrapVerb` in `internal/shedcli/table.go` gates which recipes may be seeded with the `llm` driver. A generic driver argues for every recipe being drivable.
- Adding the trace path to the step envelope touches the Shed Verb-Set Invariant, which pins the envelope's refusal kinds closed.
- The single-retry rule for `KindProducer` gives way to the driver's own judgment; the cap on repeated repairs of the same row is still unsettled.
- Whether friction becomes a shed-level location every recipe shares, instead of loom's own directory.
- A fork's lifetime is the orchestrator session's: a long batten run needs the orchestrator to stay up for its duration.
