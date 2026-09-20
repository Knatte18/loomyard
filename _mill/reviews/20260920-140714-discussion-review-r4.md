MILL_REVIEW_BEGIN
# Review: Producer gates: mechanical gates before session release

```yaml
duration_s: 179.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 4.x-class model (self-assessment; exact ID not independently verifiable from inside the session)
reviewed_file: /home/knatte/Code/loomyard/wts/producer-gates/_mill/discussion.md
date: 2026-09-20
```

## Findings

### [BLOCKING:design] Writer-row gate exhaustion halts the whole run
**Section:** "Exhaustion is reported in `Result`" / "Row removal and resume"
**Issue:** `Discussion-Write` and `Plan-Write` carry **no `on_stuck`** (`loom-recipe.yaml:40-42,101-103`, header lines 13-17), and `shedengine/run.go:234` maps `Stuck` with an empty `OnStuck` to `StateBlocked` — "stuck with no OnStuck target". So the proposed `Stuck` mapping turns today's cheap `Plan-Validate → on_stuck: Plan-Write` respawn into a run-halting human escalation, and makes `Plan-Write`'s Stuck reachable for the first time; the discussion states the mapping but never states this consequence or decides it is wanted.
**Fix:** Add an explicit decision on post-exhaustion recovery for the two writer rows (halt-for-human vs. an `on_stuck` self-edge bounded by `max_bounces`), and record it in the recipe header's escalate-set paragraph.

### [BLOCKING:design] `gate_attempts` budget scope across resume is undefined
**Section:** "Attempt budget: a row-config key, default 3" / "A Done with no live session still runs the gate"
**Issue:** `Attempts` is defined as "re-prompts actually sent on this run", i.e. per `Wait` invocation and in-memory. Every gated row carries `InterruptPolicyReinvoke` (`internal/loomshed/interruptpolicy.go:45,48,49,52`), whose whole premise is that a supervisor re-invokes `lyx loom step` and the adapter re-attaches to the live agent — which re-enters `Wait` with a fresh budget. `gate_attempts` therefore bounds nothing across re-invocations or across the `Attach`/`probeLiveRound` resume hop the discussion is otherwise careful to gate.
**Fix:** State whether the budget is deliberately per-`Wait` (and why that is acceptable given the reinvoke policy), or specify a durable per-row/per-episode carrier.

### [BLOCKING:design] The two write-row commit decorators are never mentioned
**Section:** "The four sites and how each gets its gate"
**Issue:** `Discussion-Write` and `Plan-Write` are not bare `SingleLLMProducer`s: both are wrapped by commit decorators (`entries_discussionwrite.go:40`, `entries_planwrite.go:54`) that fire the commit seam **only** on `Done` (`discussionwrite.go:47`, `planwrite.go:66`). A gate-failed `Stuck` therefore skips the commit, leaving the artifact uncommitted and the weft dirty — directly against the rationale recorded at `discussionwrite.go:41-44` ("the commit keeps the working tree clean and the artifact durable, it does not certify it"). The discussion never names these decorators or decides the interaction.
**Fix:** Decide and record whether a gate-failed writer row still commits, and reconcile with (or deliberately supersede) the decorator's recorded rationale.

### [NIT:design] Gate `Send` into an interactive `Discussion-Write` pane
**Section:** "Per-attempt done-signal" (Known limitation)
**Issue:** The recorded limitation covers only the early-Done from the `AskUserQuestion` marker hook; it does not cover the new act of `Send`ing gate re-prompt text into a pane a human operator is actively conversing in (`DiscussionSpec` sets `Interactive`/`AwaitOperator` to `!autonomous`). That is behaviour the gate introduces, not behaviour it inherits.
**Fix:** Record the interactive-mode `Send` behaviour explicitly, or state that gating is skipped/deferred when `spec.Interactive` is set.

### [NIT:consistency] Stale line references
**Section:** "The four sites…" and Technical context
**Issue:** `probeLiveRound`'s `p.attach.Attach(spec)` is at `internal/shedadapters/burler.go:471`, not `:447`; other cited offsets may have drifted similarly.
**Fix:** Re-verify the cited line numbers or cite by symbol name only.

## Verdict

REQUEST_CHANGES
Three unstated design consequences: writer-row halt semantics, budget scope on resume, commit decorators.
MILL_REVIEW_END
