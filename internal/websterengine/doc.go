// Package websterengine is the domain kernel behind webster, a fork-based
// implementer loop: instead of spawning a fresh reed/tmux strand per batch, one long-lived Master
// session reads the codebase and the whole plan once, then forks one implementer
// per execution batch in-session (Claude Code's Agent tool, subagent_type "fork"),
// sequentially,
// in the plan's declared card order. websterengine holds no loop itself —
// the loop is the Master session driving fat `lyx webster` verbs
// (internal/webstercli); this package provides only those verbs' logic plus
// the distillation behind them.
//
// # Plan consumption: one parser, one batcher registry
//
// webster consumes the pinned flat card-list plan format (see
// contracts/specs/loom-plan-spec.md) through internal/planparser, the SOLE
// parser of the on-disk `_lyx/plan/` tree — no code in this package or
// anywhere else re-derives that grammar; the one remaining plan-level-section
// consumer here, RenderIntegrationPrompt (the integration-suite fork's own
// prompt), reads plan.Verify only off the planparser.Plan model a caller
// (internal/webstercli) hands in. Neither RenderForkPrompt nor
// RenderRecoveryPrompt takes a *planparser.Plan at all any more — per the
// fork-context-hygiene Shared Decision, both render a card's content from its
// SourcePath pointer, never from an inlined plan-level field.
// A plan's flat, unordered Cards list is grouped into the execution units
// Master actually forks by internal/batcher: a name-keyed registry of
// Batcher implementations, selected once at config-load time via
// batcher.yaml's `active:` key (default: the identity batcher — one card,
// one batch), resolved by internal/webstercli and handed to Run via
// RunDeps.Batcher — this package loads no batcher config itself. Batching
// is a standalone step webster consumes today, and one Shed will drive as
// producer #8 once built, never the plan's (a card carries no
// batch-membership field of its own) and never an LLM's (no batchifier
// consults a fork's judgment). In v0 the identity batcher is the only
// registered entry, so batch ≡ card everywhere this package numbers or
// persists a "batch" — BatchState is keyed by execution-batch number,
// which happens to coincide with card number today; a future grouping
// batchifier changes that coincidence, not this package's contract.
//
// # Declared order now, a dead DAG seam for later
//
// Cards — and so batches — run strictly in the plan's declared order; there
// is no dependency graph, no cycle detection, and no strongly-connected-
// component merging in v0. The eventual scheduler is written with the
// conditional branch already in place even though only one arm is live:
//
//	if card.HasSymbolFields() {
//	    // Mechanism 1: DAG from plan-internal cross-matching (dead code
//	    // until scout lands and a planner starts populating symbol
//	    // fields)
//	} else {
//	    // v0: declared order
//	}
//
// HasSymbolFields() is unreachable in v0 — plan-format cards carry no
// symbol fields yet — so this costs nothing today and turns the eventual
// scout-driven rollout into "the planner starts populating fields,"
// never a webster code change.
//
// # Fork-return contract: OK/FAILED, a head SHA, an informational deviation list
//
// A fork's whole return contract is the minimal report shape report.go
// implements (Report/ParseReport/WriteReport): `status: OK` or `status:
// FAILED`, the `head_sha` the fork left the worktree at, and a `deviations`
// list of paths the fork touched beyond its cards' declared file-ops union.
// The deviation list is ALWAYS informational, never a failure condition — a
// fork returns FAILED only on a non-zero build/unit gate or a non-zero
// per-card `verify:`, never on deviation alone (plan-predicted file impact
// is frequently incomplete; treating deviation as failure would make the
// system impractically brittle). Within one fork's batch, each card lands
// as its own commit (BatchState.CardSHAs records the ordered per-card SHA
// trail — one element under the identity batcher, more once a grouping
// batchifier ships) and each card's own optional `verify:` runs and must
// pass before that card's commit; there is no batch-wide verify distinct
// from its cards' own gates, mirroring the plan-format card model
// directly.
//
// # bracket verbs, not spawn/poll
//
// Because the fork runs inside Master's own session, there is nothing for
// Go to spawn in the normal path — spawn-batch does not exist here. Go
// provides thin bracket verbs Master calls around each fork: begin-batch
// (pause/fingerprint checks, records the batch's start-SHA, idempotently
// asserts Master's model for this batch, renders and writes the fork
// prompt) immediately before forking, await-batch (a SHORT, stateless,
// foreground poll on the batch's report path, re-called in a loop) between
// the fork spawn and the record, and record-batch (incremental fork audit,
// batch-report parsing, digest distillation, state update) once the fork
// has delivered. await-batch exists because the Agent-tool fork is a
// BACKGROUNDED agent on current Claude Code (2.1.205): the fork call
// returns immediately, before the batch is done, so Master must stay
// inside its turn by re-polling await-batch until the report lands — a
// Master that ends its turn "waiting" is classified asking by the shuttle
// file contract and kills the run (found live in round fable-r1). Each
// await-batch call blocks only ~30s (DefaultAwaitWaitS), deliberately
// short: Claude Code auto-backgrounds a foreground command that runs much
// past ~2 minutes, and a backgrounded poll stops keeping Master's turn
// alive — so the poll is short and looped rather than one long block. Go's
// gates only run when Master actually calls them — the fork itself is
// Master's own un-gateable act, so enforcement is two-layer: template
// discipline (the master template pins the begin -> fork -> await -> record
// sequence, property-tested) plus fail-loud detection after the fact
// (record-batch hard-errors when a batch has no begin-batch record; the
// audit cross-checks fork-transcript count against begun-batch count). This
// is a steering guard, not a security boundary, the same class as burler's
// nested-Agent ban.
//
// A third, deterministic layer closes the fork-loop deadlock: because a fork
// inherits Master's whole prompt (the await-batch poll loop included), a fork
// that starts driving that loop itself — polling await-batch for the report
// it is meant to write — livelocks the run. A fork-context PreToolUse(Bash)
// hook in the claudeengine seam (buildSettings, gated on the same
// fork-authorized spec that enables forks) refuses any `lyx webster` command
// when it fires inside a fork (the hook payload carries a top-level agent_id,
// present only for a subagent, never a top-level Master call — the fork's
// transcript_path is NOT distinguishable, so agent_id is the load-bearing
// signal), while Master's own verb calls pass. This makes the deadlock
// deterministically impossible rather than merely template-discouraged; a
// cold recovery strand is a separate, non-fork-authorized session and never
// sees the hook.
//
// # idempotent per-batch model assertion
//
// Forks always inherit Master's current model — there is no per-fork model
// override, so webster carries no implementer/implementer_oversized fork
// roles at all; RoleMaster and RoleRecovery are its only two roles.
// begin-batch synchronously injects RoleMaster into Master's pane via
// shuttleengine's Runner.Inject before returning its envelope, asserting
// the correct model for THIS batch rather than assuming the previous
// batch's state. There is nothing to forget on a failure path that skips
// record-batch: the next batch's begin-batch call asserts afresh
// regardless of what the prior batch left behind. Note that the injection
// itself is DORMANT in the shipped flow: run launches Master with
// RoleMaster's model AND baselines State.AssertedModel to that same value
// at every entry, and begin-batch's only target is RoleMaster, so the
// idempotency check never finds a divergence without manual state
// tampering — the mechanism is the seam a future per-batch model policy
// plugs into, and its live timing is exercised only by the sandbox
// suite's tamper-armed W2 scenario.
//
// # cold recovery is the only real model escalation
//
// The one place webster spawns a genuinely separate process is
// recover-batch: a bounded, re-entrant long-poll verb that spawns a fresh
// implementer as its own shuttle/reed strand at the recovery role when a
// fork reports stuck or writes no report, rendering the SEPARATE, full
// cold-start recovery prompt (RenderRecoveryPrompt) — deliberately distinct
// from a fork's own thin RenderForkPrompt, since the recovery strand
// inherits no session context (see the fork-context-hygiene Shared
// Decision). Every call, including the first, blocks for at
// most poll_wait_s and returns either a terminal digest or a running
// snapshot; a re-entrant call finds the strand already recorded in state
// and skips straight to the bounded wait. This mirrors classify.go's
// dead/timeout/stuck classification but keeps any single Bash tool call
// bounded rather than open for the whole recovery timeout.
//
// # digest persistence carries batch context forward
//
// webster persists its distilled Digest into BatchState.Digest at terminal
// classification, because begin-batch(N+1) needs the immediately preceding
// batch's digest to render into the next fork's prompt, and a
// crash-resumed Master needs the same digests to reconstruct
// {{.progress}}. Nothing downstream ever re-Distills a report to
// reconstruct it, since the report's originating HEAD may have since moved.
//
// # engine/cli split: webster is fabric-blind
//
// websterengine is _lyx- and fabric-blind: every function here takes an
// already-resolved directory string, and all `_lyx/webster` path
// construction lives in internal/lyxcwd (WebsterDir/
// WebsterReportsDir/WebsterPromptsDir), per the Cwd Resolution Invariant.
// Every fabric commit of a webster artifact (state.json, a batch report,
// outcome.yaml, summary.md) happens in internal/webstercli, never here, at
// the same deterministic boundary points: begin-batch, record-batch,
// recover-batch (spawn and terminal), and run's exit backstop. Neither
// Master nor its forks ever touch fabric or git for webster's own
// bookkeeping — the Fabric Git Invariant's ban is on agents driving fabric git,
// not on the Go verbs the agent happens to invoke.
//
// # crash/resume: fresh Master re-drives the first unreported batch
//
// Because forks die with Master (same process), there is never an orphaned
// in-flight implementer for a normal batch the way a fresh-strand-per-batch
// loop can leave one behind — only Master's own strand and a possible
// recovery strand ever need reclaiming. Resuming after a crash is exactly
// re-running `lyx webster run`: entry-time reclaim stops any live recorded
// strand, then a fresh Master (never a provider resume) is spawned,
// hydrated from the on-disk register — the reports directory plus
// state.json rendered into the run's progress context — and re-drives the
// first batch that has no terminal record. Every card an implementer
// commits survives independently of Master's fate; only reports and state
// are fabric-committed per batch, so nothing already recorded is ever lost.
// One crash window needs a distinct resume move: a crash landing between a
// fork's report and record-batch leaves the re-driven batch with a report
// already on disk, which begin-batch refuses to overwrite — the resumed
// Master consumes it with record-batch instead (its fork audit keys on the
// bracket-opening session recorded in the batch state, never the current
// Master session, so the crashed session's fork transcript — still on
// disk — is found and policy-checked exactly as a late record would have),
// or with recover-batch's attach path for a recovery batch (found live in
// round fable-r3, where auditing the current session instead wedged that
// resume across all three verbs). This crash window resumes on the SAME
// machine only: fork transcripts live under the machine-local ~/.claude
// projects directory, while state.json and the reports are fabric-synced —
// a different machine sees the report with no transcript behind it, which
// record-batch refuses exactly as it refuses a forged report
// (ErrNoForkTranscripts names the operator recourse: move the orphan
// report aside and re-drive the batch).
//
// # Integration-suite fork + in-process bisect + terminal escalation
//
// A plan carrying a plan-level "## verify:" section (ShouldRunIntegration)
// drives one additional, dedicated integration-suite fork after every
// batch has landed done — never per-card, never per-batch, run once. Its
// prompt file is Go-rendered and Go-written by run at entry
// (RenderIntegrationPrompt into the prompts dir), exactly like a batch's
// own fork prompt, and its path is injected into the master template —
// Master may write nothing but its two contract files, so a
// Master-synthesized prompt file would itself be a parent-write audit
// violation (found live in round fable-r1: with no pre-rendered prompt the
// stage was unreachable). Master spawns the fork exactly like a batch
// fork; AwaitIntegration mirrors await-batch's own bounded long-poll idiom
// over the single fixed IntegrationReportPath rather than a per-batch
// report path, and a missing integration report at run exit is
// outcome-aware: fail-loud under Master's outcome: done (a done claim
// requires a passing suite), consistent-and-preserved under stuck (the
// fork died or the stage never started; Master's own judgment stands).
// On a FAILED integration report, bisect performs an in-process binary
// search over the accumulated per-card SHA trail (every terminal batch's
// own BatchState.CardSHAs) — checking out each candidate SHA detached and
// running the plan's verify command in-process via os/exec, never a fork
// per bisect candidate — to localize the first offending card in
// logarithmic, not linear, re-runs. BisectAndEscalate then records that
// localized finding as a terminal, non-successful entry in State.Batches
// under the reserved key -1 (RecordIntegrationFailure — never a real plan
// card number, so RenderProgress's walk over positive card numbers can
// never surface it by accident) and extends summary.md naming the
// offending card (AppendIntegrationFailure). A Master claiming outcome: done
// over a FAILED suite is then fail-loud after that escalation — symmetric
// with the missing-report-under-done case, since a done outcome requires a
// passing suite; a stuck outcome keeps its own judgment, merely sharpened by
// the localized card.
//
// # No shared substrate or parser with any other batch-implementation loop
//
// websterengine imports no other batch-implementation module's plan
// parser, report schema, or digest contract — planparser and this
// package's own Report/Digest types are the only ones in play. What
// webster DOES reuse is the provider-invariant orchestration substrate
// every `lyx` module built on top of shuttle shares (internal/reedengine,
// internal/shuttleengine and its claudeengine, internal/gitrepo) plus a
// small set of webster-LOCAL mechanism helpers with no cross-module
// import: its own plan-fingerprint hash (fingerprint.go), pause-flag
// mechanics (pause.go), git-query helpers built on gitrepo
// (gitwrap.go), archive-never-refuse primitives (archive.go), and
// recovery-classification logic (classify.go). Each of these exists as its
// own webster-scoped implementation rather than an imported one — this
// package owns its whole mechanism end to end.
package websterengine
