# Review (external, r2) — webster discussion.md

Overall this is in very good shape — the bracket-verb model, the audit
transcript-count-before-report-presence ordering, and the idempotent
per-batch model assertion are all sound. Three things worth pinning down
before this lands:

1. **`recover-batch-blocking-verb` may reintroduce the exact problem
   `spawn-batch`/`poll` avoided, on the exception path.** `recover-batch`
   is invoked by Master via a single Bash tool call and, per the decision,
   blocks in-process until the cold recovery strand reaches terminal
   classification — i.e. up to `recovery_timeout_min`. Builder's
   spawn-batch+poll split exists specifically so its orchestrator never
   holds one tool call open for the whole run of a long-lived process;
   collapsing that into one blocking call here means Master's own Bash
   tool invocation must survive open for however long recovery takes.
   Two concrete risks: (a) whatever caps a single Bash-tool-call's
   duration in Master's shuttle `Spec` must be raised to comfortably
   exceed `recovery_timeout_min`, or the harness kills the call before Go
   returns — leaving Master with a dead tool result while the Go process
   (and the recovery strand under it) may keep running or may itself get
   SIGKILLed depending on process-group handling; (b) even if the timeout
   is raised, a live Claude Code session sitting on one open tool call for
   tens of minutes is exactly the shape builder's poll loop was built to
   avoid, for reasons (session health, visibility into partial progress)
   that presumably still apply to Master's session. Worth an explicit
   decision either way: raise Master's Bash-tool ceiling with a stated
   reason, or give `recover-batch` an internal poll/checkpoint shape
   (still one Go verb from Master's side, but returning in bounded
   increments) rather than a single unbounded block.

2. **`fork-prompt-go-rendered`'s "when the batch depends on an earlier
   one" needs a mechanical definition.** `long-term-ideas.md` is explicit
   that today's plan format is a flat ordered list with **no DAG** — so
   there is no declared dependency edge for `begin-batch` to consult when
   deciding whether to prefix the fork prompt with a distilled prior-batch
   digest. If this is meant to simply be "every batch after the first,
   always," say so plainly (and confirm it's a Go-decided constant, not
   something Master or the plan format infers). If it's meant to be
   selective, the plan format needs a field to hang that on, which is a
   scope addition not currently listed. Recommend pinning it to the
   unconditional "always prefix from batch 2 onward" reading — cheapest,
   matches the sequential-only plan model, and avoids quietly reopening
   the no-DAG decision.

3. **The `/model`-injection sandbox scenario should also assert
   non-interference with the concurrently running subprocess, not only
   that the model switches.** The scenario as scoped checks that injected
   keys reach the TUI and take effect for subsequent calls. Worth adding
   an explicit assertion that the injected keystrokes do **not** leak into
   or get consumed by the foreground Bash tool subprocess's own stdin
   (e.g. echoed into that command's input/output, or swallowed by an
   interactive sub-command) — a keystroke race that merely fails silently
   (keys go to the wrong process) is a different failure mode than one
   that corrupts the running tool call's result, and only the second is
   dangerous enough to insist the fallback trigger on.
