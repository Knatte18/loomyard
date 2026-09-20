# Producer gates — mechanical accept-gates on LLM-running producers

**Status:** a settled direction distilled from a design discussion (2026-09-20).
Ready to be broken into a task, but not a row-level spec — validate details against the code before implementing from it.

## The concept

A `ShedProducer` that runs an LLM (via shuttle) can declare a **mechanical gate**: a validator the producer runs on the artifact *while the agent session is still alive*, holding the handoff until the gate passes.
On failure the producer injects the findings back into the same live session as a re-prompt — the warm session fixes mechanical findings for a re-prompt's cost, instead of a fresh spawn rebuilding context.

The concept applies to LLM-running producers only.
A pure-Go producer just codes the check inline in its own control flow, where it cannot be skipped;
it is the LLM that makes "was the instruction followed" a question, so the gate and its injectable-findings contract exist exactly there.
Webster already embodies the pattern: its per-batch loop runs the plan's verify command in-process before moving on.

The gate never judges quality.
"Is the plan *valid*" is the gate's question (a parser's verdict, milliseconds, free);
"is the plan *good*" stays the perch segments' question (an LLM judge with a rubric).
The gate is perch in miniature — judge → findings → repair → re-judge → budget — boiled down to Go and one live session.

## The gate contract

```go
type GateResult struct {
    Passed   bool
    Findings string   // detailed WHAT-is-wrong; empty when Passed
    // or: FindingsPath string — written to a file, linked in the re-prompt
}
type Gate func(artifactDir string) (GateResult, error)
```

- **`error` is not "not passed".**
  `Passed: false` means the artifact is invalid and the findings go to the LLM;
  `error` means the validator itself failed (I/O, config), and the row fails as an ordinary producer error — never burning gate attempts on a problem that is not the LLM's.
  Mixing them spends the budget on infrastructure faults and makes the Stuck message lie about the cause.
- **Findings inline for a line or two, in a file for everything else.**
  The re-prompt stays short ("the gate failed — read `_lyx/plan/gate-findings.md`"), the agent reads the file itself with warm context — the same short-notification/content-in-file split the rest of the system uses.
- **An explicit attempt counter bounds the loop** — the same idiom as `max_bounces`, recorded in the row's envelope/history so the status file shows it.
  Exhausted means ordinary `Stuck`, never another attempt: neither a stubborn session nor a validator bug can loop forever, and a validator that always says no surfaces as three absurd findings in the log — immediately recognizable as a validator bug.

## When the gate runs

The trigger is not an event the producer must recognize — it is a fixed point in the producer's own control flow, which is why the agent cannot skip it:

1. Compose the prompt, start the session via shuttle.
2. Wait for handoff — the agent *believes* it is done.
3. **Run the gate on the artifact.**
4. Passed → seams fire, row returns `Done` — as today.
5. Not passed, attempts left → re-prompt the *same* session with the findings, back to 2.
6. Budget exhausted → `Stuck`.

The agent never knows when the gate runs and has no say in it — it only experiences that a "done" handoff is sometimes answered with another prompt instead of silence.
Which validator a row gets is declared in the row's config/deps, read by Go — exactly as `commit_seam`/`approve_seam` are declared today.

For a Burler fix step the same loop sits one level in: the fix step is a shuttle session with a handoff point, and the gate runs there — after the fix's handoff, before the round reports back to the Bouncer.
The round can then never hand back an artifact it made invalid itself.

## What "done" means, per attempt

Shuttle's spawn-level done is the file contract (the agent writes its declared output files), backed by the engine's completion-edge classifier and `run.json`'s outcome — and **idle alone is never proof of done**: the file contract outranks pane evidence precisely because an agent waiting on something looks idle.

Re-prompt attempts need their own done-definition, because the output file already exists and will be edited in several passes before the agent is done again:

- **The turn boundary is the per-attempt done-signal.**
  An interactive session's rhythm is mechanical: prompt in → the agent works (editing as many times as it likes) → the turn ends, the provider goes idle.
  Mid-turn edits are invisible and irrelevant: the gate runs only when the agent no longer has its hands on the wheel.
- **But turn-idle alone is not enough** — an agent that launched an async subagent ends its turn and sits idle while work is still pending.
  The quiescence definition is therefore compound:

  ```
  done-candidate = turn idle (completion edge)
                 ∧ no child processes under the pane's process (reed's proctree machinery)
                 ∧ the pending-work ledger is empty (hook-maintained)
  ```

  Background shell commands and monitors are real child processes — the existing `proctree` machinery answers that mechanically.
  In-process async subagents have no OS footprint;
  the reliable source is the provider itself, via injected hooks maintaining a pending-work ledger file (the AskUserQuestion recording hook is the precedent for the channel).
  This is provider-specific what-does-done-mean knowledge and so belongs in the engine adapter, per the Shuttle Provider-Seam Invariant: shuttle asks "quiescent?", the engine answers from its sources.
- **Caveat to verify at implementation time:** the hook vocabulary must actually cover start and completion of async work.
  If it does not, the fallback is a per-attempt receipt file named in the re-prompt — used only as done-evidence, never validity-evidence (the gate still validates the artifact mechanically), so a forgotten receipt degrades to a timeout and a burned attempt, never to an invalid artifact slipping through.

## What the gates replace

Four gate sites, two validators:
`Discussion-Write` and Discussion-Burler's fix step share the discussion validator;
`Plan-Write` and Plan-Burler's fix step share the plan validator.

With every LLM mutation of an artifact gated at its source, the standalone `Discussion-Validate`, `Plan-Validate`, and `Plan-Revalidate` rows are removed from the recipe in the same task — no path remains where an invalid artifact leaves a row as `Done`.
Keeping them would keep a heavy, masking recovery path (a fresh-writer respawn) for what is by definition a bug in the gate;
removing the check entirely would move first detection downstream to Batchifier/Webster breaking on a malformed plan.
Removing the rows *because the check moved into the producers* is the clean resolution: the check cannot be skipped where it now lives, and a bug in it is an ordinary producer bug, guarded by tests like any other.

The plan approval flag never needed a row: it is written by Plan-Bouncer's Go approve seam, which fails loudly on its own — there was never LLM uncertainty there.

The escalation that `Plan-Revalidate`'s old `on_stuck: Plan-Write` edge provided ("this cannot be patched — fresh writer, full rewrite") moves to where the decision is actually made: the fixer's gate exhausting its budget fails the round back to the Bouncer, and the segment's bounce budget escalates from there.

## Relation to perch

| | perch (Bouncer ⇄ Burler) | producer gate |
|---|---|---|
| lives | between rows, in the FSM | inside one row, in producer code |
| judge | LLM with a rubric | Go validator |
| question | is the artifact *good*? | is the artifact *valid*? |
| the session | the writer's is long gone — rounds are their own spawns | still alive — the entire point |
| cost per round | a full review generation | milliseconds |

Same concept, two levels — and the implementation should share vocabulary (findings format, attempt counter, `Stuck` on exhaustion) so the status file reads the same at both.

## Existing machinery this binds to

- `planparser`'s checks (the `PlanValidate` engine's own validator) and the discussion validator — the gate *is* these functions, called from a new site.
- Re-prompting a live session: the reinvoke/handback wiring from the loom-step work; sessions are interactive reed strands by construction.
- `proctree` in reed — child-process detection, built for reap/kill.
- Hook injection into spawned sessions — the AskUserQuestion recording hook is the precedent.
- Webster's in-process verify command — the pattern's existing embodiment.
