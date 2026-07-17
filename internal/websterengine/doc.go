// Package websterengine is the domain kernel behind webster, a fork-based
// sibling of builderengine's batch-implementation loop: instead of spawning a
// fresh mux/tmux strand per batch, one long-lived Master session reads the
// codebase and the whole plan once, then forks one implementer per batch
// in-session (Claude Code's Agent tool, subagent_type "fork"), sequentially,
// in the same order builder drives batches today. websterengine holds no loop
// itself — the loop is the Master session driving fat `lyx webster` verbs
// (internal/webstercli); this package provides only those verbs' logic plus
// the distillation behind them.
//
// # A/B contract-compatible with builder
//
// webster is deliberately kept contract-compatible with builder so both can
// run the same plan and be A/B tested, and so a future loom Builder phase
// does not care which implementation runs underneath: the plan input is
// parsed by the same builderengine.ParsePlan (there is exactly one plan
// parser in the repo), the batch-report schema forks write is the same
// shape builderengine.ParseReport already parses, and outcome.yaml is
// compatible with builderengine.ParseOutcome. webster imports builder's
// mechanism-agnostic, dir-parameterized functions directly rather than
// duplicating them — ParsePlan, Validate, Fingerprint, Distill/Digest,
// Classify/PollUntilTerminal, ParseReport, ParseOutcome/ArchiveStaleOutcome,
// chain rollback, the pause helpers, and the gitquery helpers — so drift
// between the two builders is structurally impossible. Import direction is
// websterengine -> builderengine only, never the reverse. webster defines
// only what is genuinely mechanism-specific to it: its own Role consts, its
// own Config, its own State/BatchState (fork-attribution fields in place of
// builder's strand fields), and its own sentinel errors — it never reuses
// builder's Role names, State struct, or sentinels, so errors.Is across the
// two modules can never conflate two different runtimes.
//
// # bracket verbs, not spawn/poll
//
// Because the fork returns synchronously inside Master's own turn, there is
// nothing for Go to spawn or poll in the normal path — spawn-batch and poll
// do not exist here. Instead Go provides two thin bracket verbs Master calls
// around each fork: begin-batch (pause/fingerprint checks, optional chain
// rollback, records the batch's start-SHA, idempotently asserts Master's
// model for this batch, renders and writes the fork prompt) immediately
// before forking, and record-batch (incremental fork audit, batch-report
// parsing, digest distillation, state update) immediately after the fork
// returns. Go's gates only run when Master actually calls them — the fork
// itself is Master's own un-gateable act, so enforcement is two-layer:
// template discipline (the master template pins the begin -> fork -> record
// sequence, property-tested) plus fail-loud detection after the fact
// (record-batch hard-errors when a batch has no begin-batch record; the
// audit cross-checks fork-transcript count against begun-batch count). This
// is a steering guard, not a security boundary, the same class as burler's
// nested-Agent ban.
//
// # idempotent per-batch model assertion
//
// Forks always inherit Master's current model — there is no per-fork model
// override, so webster carries no implementer/implementer_oversized fork
// roles at all. Batch-level model choice is expressed by switching Master
// itself: begin-batch synchronously injects the target role (master or
// master_oversized, whichever the batch declares) into Master's pane via
// shuttleengine's Runner.Inject before returning its envelope, asserting the
// correct model for THIS batch rather than assuming the previous batch's
// state. There is no separate de-escalation step and nothing to forget on a
// failure path that skips record-batch: the next batch's begin-batch call
// asserts afresh regardless of what the prior batch left behind.
//
// # cold recovery is the only real model escalation
//
// The one place webster spawns a genuinely separate process is
// recover-batch: a bounded, re-entrant long-poll verb that spawns a fresh
// implementer as its own shuttle/mux strand at the recovery role (reusing
// builderengine's SpawnBatch machinery by import) when a fork reports stuck
// or writes no report. Every call, including the first, blocks for at most
// poll_wait_s and returns either a terminal digest or a running snapshot; a
// re-entrant call finds the strand already recorded in state and skips
// straight to the bounded wait. This mirrors builder's dead/timeout/stuck
// classification but keeps any single Bash tool call bounded rather than
// open for the whole recovery timeout.
//
// # digest persistence carries batch context forward
//
// Builder never persisted its distilled Digest; webster must, because
// begin-batch(N+1) needs the immediately preceding batch's digest to render
// into the next fork's prompt, and a crash-resumed Master needs the same
// digests to reconstruct {{.progress}}. record-batch persists the digest into
// BatchState.Digest at terminal classification; nothing downstream ever
// re-Distills a report to reconstruct it, since the report's originating HEAD
// may have since moved.
//
// # engine/cli split: webster is weft-blind
//
// Like builderengine, websterengine is _lyx- and weft-blind: every function
// here takes an already-resolved directory string, and all `_lyx/webster`
// path construction lives in internal/hubgeometry
// (WebsterDir/WebsterReportsDir/WebsterPromptsDir), per the Hub Geometry
// Invariant. Every weft commit of a webster artifact (state.json, a batch
// report, outcome.yaml, summary.md) happens in internal/webstercli, never
// here, at the same deterministic boundary points builder established:
// begin-batch, record-batch, recover-batch (spawn and terminal), and run's
// exit backstop. Neither Master nor its forks ever touch weft or git for
// webster's own bookkeeping — the Weft Git Invariant's ban is on agents
// driving weft git, not on the Go verbs the agent happens to invoke.
//
// # crash/resume: fresh Master re-drives the first unreported batch
//
// Because forks die with Master (same process), there is never an orphaned
// in-flight implementer for a normal batch the way builder can leave one
// behind — only Master's own strand and a possible recovery strand ever need
// reclaiming. Resuming after a crash is exactly re-running `lyx webster run`:
// entry-time reclaim stops any live recorded strand, then a fresh Master
// (never a provider resume) is spawned, hydrated from the on-disk register —
// the reports directory plus state.json rendered into the run's progress
// context — and re-drives the first batch that has no terminal record. Every
// card an implementer commits survives independently of Master's fate; only
// reports and state are weft-committed per batch, so nothing already recorded
// is ever lost.
package websterengine
