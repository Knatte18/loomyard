// Package websterengine is the domain kernel behind webster, a fork-based
// implementer loop: instead of spawning a fresh reed/tmux strand per batch, one long-lived Master
// session reads the codebase and the whole plan once, then forks one implementer
// per execution batch in-session (Claude Code's Agent tool, subagent_type "fork"),
// sequentially — one fork at a time, one worktree — in an order sequence.go derives from the
// cards' own declared dependencies, not the plan's declared order. websterengine holds no loop itself —
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
// consumer here, the verify gate's command read (NewVerifyGate), reads plan.Verify
// only off the planparser.Plan that planparser.ParsePlan returns. Neither RenderForkPrompt nor
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
// # Execution order is derived, not declared
//
// sequence.go's SequenceBatches derives edges from Targets/Uses ref matching across the plan's
// cards — a Uses entry naming another card's Targets entry orders the producer before the
// consumer, and two cards writing the same Targets entry settle by declared card number — then
// condenses every strongly-connected component it finds and returns a deterministic topological
// order plus the cycles it condensed. A cycle is reported, never fatal: SequenceBatches keeps
// every cycle's member batches together, in declared order, and hands the caller both the
// reordered slice and the []Cycle it condensed. An already dependency-correct plan sequences to
// exactly its declared order, so this is a strict superset of the old declared-order behavior, not
// a divergent one. Sequencing is unconditional — there is no config key and no opt-in — which is
// why every batch-computation site must sequence: Run plus each of the four internal/webstercli
// bracket verbs (begin-batch, await-batch, record-batch, recover-batch) call SequenceBatches over
// the batchifier's own output before doing anything else with the result, so all five agree on one
// order by construction. The previous-digest lookup in beginbatch.go/recoverbatch.go
// (predecessorDigestLine) depends on that ordering: it reads whichever batch actually sits
// immediately before the target batch in the sequenced slice, not the batch one number lower.
// internal/batcher still owns grouping (which cards land in the same batch, per the Batcher
// Registry+Config Invariant); this package owns only the sequencing of the batches a batchifier
// already returned — SequenceBatches reorders, never regroups.
//
// # Fork-return contract: OK/FAILED, a head SHA, an informational deviation list
//
// A fork's whole return contract is the minimal report shape report.go
// implements (Report/ParseReport/WriteReport): `status: OK` or `status:
// FAILED`, the `head_sha` the fork left the worktree at, and a `deviations`
// list of paths the fork touched beyond its cards' own deviation union —
// every path-shaped target entry across the batch's cards, plus the files
// holding every symbol-shaped target entry, with `Uses:` excluded because it
// is read rather than written.
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
// record-batch and recover-batch apply one merge-only rule when they cross-check the consumed report's `head_sha` against the worktree's HEAD,
// so a parent merge-in landing between a fork's commit and the report's consumption cannot wedge the run.
// HEAD is accepted when it equals `head_sha` or sits above it by clean parent merges alone on the first-parent chain:
// each walked commit has exactly two parents, its second parent is reachable from the parent branch in fabric's origin record, and its tree equals the conflict-free merge of the two.
// An evil merge, a hand-resolved conflict, a merge of any other branch, an octopus, or any merge in standalone mode (no parent branch) is refused,
// because the audited delta ends at `head_sha` and such a merge's own content would bypass it.
// The batch is recorded at the report's `head_sha` (CardSHAs and the delta's end), while the done-checks and drift detection read the merged tree as it stands,
// and a warning names the walked merge SHAs.
// Any non-merge movement — a plain commit, a fast-forward onto non-merge commits — is refused,
// and so is any call made while a git merge is in progress, leaving the batch non-terminal and retryable.
//
// # every terminal batch runs the same mechanical pass
//
// A batch reaches terminal down one of two paths — record-batch after an
// ordinary in-session fork, or recover-batch after a cold recovery strand —
// and both run the identical post-batch mechanical pass (postBatchChecks):
// the card done-checks, one shared delta, handle binding, the informational
// glyph scope guard, and drift detection with its exact-tier auto-repair,
// plus the plan-staleness re-baseline each rewrite among them requires.
// A batch that reached done through a recovery is exactly as done as one
// that reached it through a fork, and Master's own failure ladder treats a
// terminal recovery digest as "move on to the next batch" — so a recovery
// path that skipped the pass left a recovered card's plan: handles unbound
// for the rest of the plan's life, invisible to drift detection too, since
// its reference index keys on the ref as the card spells it.
//
// This is also why a recovery BatchState inherits the stuck fork's own
// StartSHA rather than re-capturing the head at recovery-spawn time: the
// pass computes its delta from that SHA, and a fork frequently commits part
// of its work before getting stuck.
//
// A batch failed on a correctness finding records its suspect paths with the blob each held when it first failed,
// and recover-batch records the batch done only once each of them was reverted or re-derived and committed.
// A plan file must match the run's recorded hashes, a tracked path must not differ from the report's head,
// and a tracked path must not still hold the flagged blob unless the start commit held it too.
// A strand whose re-derivation is byte-identical to the flagged content is failed again;
// the way forward is to revert the path and edit its card.
//
// # one dispatch scope for begin-batch, run entry and validate
//
// A plan describes intended change, so re-resolving a card whose work may already have landed reports the plan working as designed as a defect.
// begin-batch, run entry and `lyx webster validate` therefore all take their scope from DispatchScope:
// begun is every card of a batch begin-batch recorded, terminal or not, and those cards are not resolved;
// forthcoming is the cards of every begun batch whose record is not terminal.
// A forthcoming card's Create targets and Rename New sides are excluded from the status check, so a later card that Uses one passes while the fork has landed nothing yet.
// A terminal batch's cards are never forthcoming: its done-checks proved its targets present.
// With no begun card, validate runs the whole-plan check set, approval gate included.
//
// The bound: drift in a begun, non-terminal card's own targets is reported by none of the three sites.
// That card was fully validated at its first begin-batch, its bytes stay pinned by its recorded CardHashes, and record-batch and recover-batch still run its done-checks and drift detection.
// A forthcoming target that never lands keeps the run from outcome done, because the run-exit check requires a terminal done record for every batch.
//
// # the plan-staleness guard re-baselines at the rewrite, not at the return
//
// begin-batch compares the plan directory's fingerprint against the one
// state.json records, so a plan edited from outside the run between two
// batches is refused. The run itself also rewrites the plan, though —
// handle canonicalization inside begin-batch's own ValidateDispatch, handle
// binding and exact-tier drift repair inside record-batch — and the guard
// cannot tell those apart from a foreign edit. The rule that resolves it:
// EVERY sanctioned rewrite is re-baselined immediately after the call that
// performed it, before any finding or error that call also reports is
// examined, and the caller persists that re-baseline even when the call
// then fails.
//
// Both halves are load-bearing, and each was a real wedge on its own.
// Re-baselining after the refusals meant a call that rewrote the plan AND
// blocked left state.json describing the pre-rewrite bytes — routine, since
// BindHandles' substitutions come from the delta's created symbols while
// its bind-count-mismatch comes from a card that matched nothing, and
// DetectDrift's repair and its plan-references-deleted-symbol finding read
// different parts of one delta. Persisting only on success dropped the
// re-baseline for the same reason. Either way every later begin-batch
// failed ErrFingerprintMismatch on webster's own edit, and the advised
// `--fresh` recourse then refused the run outright over the cards that had
// already landed — unrecoverable without hand-editing state.json.
// Comparing against the fingerprint read immediately before the call is
// what keeps the persist narrow: an unchanged fingerprint writes nothing,
// so a genuine foreign edit still fails exactly as it did.
//
// The restamp also moves the recorded CardHashes of every begun card the rewrite changed,
// so webster's own rewrite of a begun card is not later refused as an operator edit that `rebaseline` cannot accept.
// It moves a hash only when the recorded hash equals the pre-rewrite plan file hash for that card's file;
// a card an earlier untracked rewrite already moved keeps its old hash and stays refused.
// Each restamp site runs after a foreign-edit check passed in the same call, so an edit on disk when the call starts is refused, never adopted.
// The check precedes the rewrite rather than being atomic with the restamp, so an edit landing between the two in one call is adopted with the rewrite.
// `rebaseline` accepts an operator's edit, so it never moves a begun card's hash, except for a card the operator names with --card whose batch is terminal failed, dead or stuck.
//
// A foreign edit an operator means to keep has its own way forward:
// `lyx webster rebaseline --card NN` (Rebaseline) accepts the on-disk plan as the new baseline without dropping any batch record, provided the edited plan's batch of each recorded number still holds exactly the cards that record names.
// The operator names every card the edit changed with --card: State.PlanFileHashes records a hash of every plan file, and a changed card file whose number is not named is refused.
// A named card of a batch that is terminal failed, dead or stuck is accepted even though that batch was begun:
// Rebaseline restamps that card's CardHashes entry and keeps the rest of the batch record, so a one-card fix needs no reset and no fresh run.
// A done batch, an unfinished batch and a failed batch whose record lists Uncheckable entries still refuse, each with its own way forward.
// An edit to 00-overview.md, which carries the plan's integration verify, is never accepted; the way forward is to restore it or to run `lyx webster reset --to start` and then `lyx webster run --fresh`.
// The fingerprint refusals in begin-batch and run name it.
//
// validate, record-batch and recovery refuse a plan that changed before their own rewrites, instead of adopting it:
// each checks the plan against the recorded fingerprint (PlanEditError) before its first rewrite,
// and record-batch and recovery also check that no card of the batch changed since it was begun.
// Their restamps exist to adopt webster's own rewrites, so a difference seen at that point is someone else's edit.
// A refusal there mutates nothing;
// validate still lints but skips its restamp.
// Every plan-hash restamp and run initialisation also stores the hashed content under `<WebsterDir>/plan-baseline/`, one file per content named by its SHA-256, so the fabric sync carries it with state.json.
// `lyx webster restore-plan` (RestorePlan) writes every plan file that differs from the recorded plan back from that store and removes a plan file the run never recorded;
// it never touches state.json, and refuses with ErrPlanBaselineMissing, changing nothing, when a copy is missing.
// Each batch record carries the card set it was begun with (BatchState.Cards) so that check has something to compare against.
// It also carries each card file's content hash (BatchState.CardHashes), so a begun card whose body changed while its file name stayed is refused too, not only a changed id;
// a record written before the hashes existed compares ids only.
//
// # audit findings: correctness fails the batch, policy warns once
//
// The fork and parent audits classify each finding (ClassifyViolation) as correctness or policy.
// A correctness finding means the delta or the run's own state may be wrong.
// It is a fork writing one of Master's two contract files, anything under the plan directory, or anything under webster's run directory but its own report (fork-state-write), or a parent write under the run's `_lyx` directory, into the worktree's tracked (not git-ignored) content, under the run's `.lyx` state directory (webster's pause flag and locks, another module's lock or pause flag, a reed launch script), or into another worktree of the task repository.
// Every fabric reference is correctness too, whatever its command, since an agent never touches the fabric repo and the command can rewrite run state the cards' verify commands cannot detect;
// a policy finding breaks a steering rule without touching correctness, such as a named spawn or a nested agent call.
// Each finding carries a stable identity (its Key, prefixed by the session id for a parent finding),
// and state.json's ledger dispositions it once per run, so the whole-session parent audit repeating earlier findings on every record-batch never re-judges them.
// A policy finding is recorded as a warning on the batch once the evidence holds:
// an OK report on a batch that carries policy findings first has its cards' verify commands re-run in-process (rerunCardVerifies), and a failing re-run makes those findings correctness for the batch and fails it.
// A correctness finding fails the batch on its merits instead of wedging it:
// the batch goes terminal with digest status failed and its reasons, the report is archived, and record-batch returns *BatchFailedError naming `lyx webster recover-batch`.
// recover-batch proceeds from a failed batch and hands its strand the failure digest,
// except a batch failed on a correctness finding the recovery check cannot verify (a finding with no path, or a path outside the tracked tree and the plan directory), which its record lists as Uncheckable:
// recover-batch refuses it with ErrRecoveryNeedsFresh before spawning anything,
// and the way forward is `lyx webster run --fresh` after resetting the branch to the run's start commit.
// `run --fresh` drops such a batch under the same HEAD and path rules as a pending finding.
// record-batch on a batch already terminal as a fork batch first audits the fork transcripts it has not consumed, once and without the settle wait:
// an undispositioned correctness finding (a fork that marked its own batch done by writing state.json) replaces the terminal record with a failed one,
// and otherwise the "already terminal" refusal stands.
// It audits nothing while a later fork batch of the session is open or the verify-gate report exists, since an unseen transcript may then be that fork's.
// A report that cannot be attributed to a begun batch, or to any fork transcript, is archived and returned as *ReportArchivedError naming `lyx webster begin-batch`, which re-drives the batch.
// The post-batch done-checks fail the batch the same way when a card's own declared work is missing, while drift that concerns only a later card is recorded as a warning rather than blocking this batch.
// A delete-not-done finding gets one more check, planglyph.LaterDeleteReferences over the batch's own cards and the cards of every batch with no record:
// when an unbegun later card's Edit code still references the target, the failure's reasons name that card and the reference,
// and the BatchFailedError's way forward is the plan edit (move the delete after that card, rebaseline, then recover-batch), or the `--fresh` steps when the record also lists uncheckable entries, since recover-batch would repeat the same failure.
// PersistRecoveryTerminal fails a recovered batch the same way, and recover-batch runs the same check before spawning:
// while it fires it refuses with ErrRecoveryDeleteReferenced, which the CLI maps to the `batch_failed` flag.
// At run exit the audit cross-check drops dispositioned findings, records the rest of the policy findings as run-level warnings, appended to summary.md under "Audit warnings", and demotes Master's outcome done to stuck for an undispositioned correctness finding.
// A correctness finding stays pending in state.json until `lyx webster accept-audit` clears it, and run entry refuses with ErrPendingAuditFindings meanwhile.
// accept-audit needs evidence: it checks every suspect path against the last batch head (a plan file against the run's recorded plan hashes) and refuses with ErrAuditNotAcceptable while any path differs, cannot be checked, or a finding names no path;
// the evidence covers HEAD too, so it also refuses while HEAD carries a commit past the last batch head other than a clean parent merge, and checks the paths against that reconciled HEAD;
// the last two clear only through `lyx webster reset --to start` and then `lyx webster run --fresh`.
// The run-exit stuck reason and the pending-findings refusal name each finding once (findingsClause), an uncheckable path carrying its reason (uncheckableReason),
// and end in one ordered list: the restores, then `lyx webster accept-audit`, then exactly one re-entry step, RunDeps.ReentryStep (`lyx webster run` when empty);
// the reset route ends in `lyx webster run --fresh` instead, which the shed adapter never runs itself.
// A suspect path that is one of the run's two contract files, outcome.yaml or summary.md, is the exception, since it lies outside the tracked tree and has no blob to compare:
// contractFileStatus clears it on evidence, when the file is absent or the latest successful Master write to it (from RunWrites) is later than every fork write to it.
// A Master write whose result failed is not evidence, and an acknowledgement never clears it.
// Every other path under `_lyx`, `.lyx` or the scratch directory stays uncheckable.
// Four sites consult it before checkSuspectPaths: AcceptPendingAudit, RecoverSpawnOrAttach, the way forward of a pending finding (pendingPathsWayForward) and `run --fresh`.
// A cleared contract path is resolved, so at run exit `lyx webster accept-audit` clears the finding directly;
// an uncleared one refuses with `rm <path>` as the first step, then the verb to re-run.
// Absence clears the finding only: accept-audit on an absent file adds a `next` step to its envelope, since the run still needs a Master that writes both files.
// `run --fresh` drops pending findings, with one warning per finding, even on an unchanged plan, and refuses with ErrPendingAuditFindings, archiving nothing, while a pending suspect path outside the plan still differs from the run's start commit or HEAD is not the start commit.
// It also refuses while a pending plan path differs from the plan the run recorded and `lyx webster restore-plan` can undo that (the recorded copy is stored, or the file was never recorded);
// a differing plan path whose recorded copy is missing is dropped with the archived state,
// and its warning says so.
// Every way forward for a differing plan path names restore-plan or `rebaseline --card`, never a git checkout.
//
// Every refusal this package can return, and the way forward from it, is tabulated in contracts/specs/refusal-spec.md;
// this documentation links that table rather than restating its rows.
//
// # bracket verbs, not spawn/poll
//
// Because the fork runs inside Master's own session, there is nothing for Go to spawn in the normal path — spawn-batch does not exist here.
// Go provides thin bracket verbs Master calls around each fork: begin-batch (pause/fingerprint checks, records the batch's start-SHA, renders and writes the fork prompt) immediately before forking, and record-batch (incremental fork audit, batch-report parsing, digest distillation, state update) once the fork has delivered.
// The Agent-tool fork is a BACKGROUNDED agent: the fork call returns immediately, before the batch is done,
// so Master ends its turn right after spawning it and calls record-batch when the fork's completion notification starts its next turn.
// That turn end does not end the run: shuttle reads a turn that ends with a background agent still running as EventWaiting, which its wait loop treats as still running,
// so Master spends no turns while a fork works.
// await-batch (a stateless, bounded wait on a batch's report path) remains as a verb an operator can call, but no longer sits in Master's loop.
// Go's gates only run when Master actually calls them — the fork itself is Master's own un-gateable act,
// so enforcement is two-layer: template discipline (the master template pins the begin -> fork -> notification -> record sequence, property-tested) plus fail-loud detection after the fact
// (record-batch archives the report and refuses when a batch has no begin-batch record, naming begin-batch as the way forward;
// the audit cross-checks fork-transcript count against begun-batch count).
// This is a steering guard, not a security boundary, the same class as burler's nested-Agent ban.
//
// A third, deterministic layer closes the fork-loop deadlock: because a fork
// inherits Master's whole prompt (the batch loop included), a fork that
// starts driving that loop itself — calling the bracket verbs or waiting for
// the report it is meant to write — livelocks the run. A fork-context PreToolUse(Bash)
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
// # one model per run
//
// Forks always inherit Master's current model — there is no per-fork model
// override, so webster carries no implementer/implementer_oversized fork
// roles at all; RoleMaster and RoleRecovery are its only two roles.
// run launches Master with RoleMaster's model, and nothing in begin-batch reads or changes it afterwards; a Master pane an operator moves with /model stays there for the rest of the run.
//
// # cold recovery is the only real model escalation
//
// The one place webster spawns a genuinely separate process is
// recover-batch: a blocking, re-entrant verb that spawns a fresh
// implementer as its own shuttle/reed strand at the recovery role when a
// fork reports stuck or writes no report, rendering the SEPARATE, full
// cold-start recovery prompt (RenderRecoveryPrompt) — deliberately distinct
// from a fork's own thin RenderForkPrompt, since the recovery strand
// inherits no session context (see the fork-context-hygiene Shared
// Decision).
// The master and the recovery strand both load `scribe:prose`, `scribe:code-quality` and `scribe:testing` through their spawn spec's skills (`roleSkills`), and their opening prompts render the parent directive (internal/parentdirective) from Geometry.ParentName, which hubgeom.WebsterGeometry fills from the worktree's origin record.
// Forks get no skills and no directive: they inherit the master's context.
// The call that spawns the recovery strand first waits for its provider to come up (normally seconds, bounded by startup_timeout_s),
// and every call then blocks for RecoveryWaitBudget (recovery_timeout_min plus one poll tick) and returns a terminal digest:
// the budget outlasts the timeout measured from spawn, so a strand that never reports classifies dead on its timeout and the call returns.
// A re-entrant call finds the strand already recorded in state and skips straight to the wait.
// Every terminal digest (done, stuck or dead) removes the recovery strand if it is still live, even when the call then refuses on a merge in progress or a report head mismatch;
// only a done digest also removes the run dir, which stuck and dead keep for diagnosis.
// `lyx webster reset --to start` likewise removes every live recovery strand the state records before it resets,
// so no recovery agent keeps writing into the tree the reset moves;
// `--to pre-fix` removes none.
// Merriam runs the call as a backgrounded Bash command, ends its turn and acts on the completion notification.
// Only an operator's shorter --wait can return a running snapshot.
// This mirrors classify.go's dead/timeout/stuck classification.
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
// already-resolved directory string, and websterengine itself declares its own
// `_lyx/webster` and `.lyx/webster` subpaths through the four told accessors in
// state.go (Dir/ReportsDir/ScratchDir/PromptsDir) — the anchor root they are joined
// onto is always supplied by the caller, per the Cwd Resolution Invariant.
// This claim is now literally true, not aspirational: no production file in this package imports
// internal/fabricengine, and the two seams that used to reach it are engine-declared interfaces the
// caller supplies instead — RefMatcher for the fork-audit's fabric-reference violation class.
// internal/lyxcwd is likewise absent from this package's production imports: every path this package
// consumes arrives already resolved, through a Geometry value (geometry.go), never through a
// *lyxcwd.Location.
// internal/hubgeom's WebsterGeometry and internal/standalonegeom are the two tellers that build a
// Geometry — one from a resolved hub *lyxcwd.Location, the other from a standalone state tree — and
// the dependency direction between them and this package is one-way: they import websterengine to
// build the struct it declares, and websterengine never imports either back.
// See geometry.go for what each of Geometry's eight fields means; this section does not restate them.
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
// One crash window needs a distinct resume move: a crash landing between a fork's report and record-batch leaves the re-driven batch with a report already on disk, which begin-batch refuses to overwrite when state.json records the batch — the resumed Master consumes it with record-batch instead (its fork audit keys on the bracket-opening session recorded in the batch state, never the current Master session, so the crashed session's fork transcript — still on disk — is found and policy-checked exactly as a late record would have), or with recover-batch's attach path for a recovery batch (found live in round fable-r3, where auditing the current session instead wedged that resume across all three verbs).
// This crash window resumes on the SAME machine only: fork transcripts live under the machine-local ~/.claude projects directory, while state.json and the reports are fabric-synced —
// a different machine sees the report with no transcript behind it, which record-batch treats exactly as it treats a forged report:
// it archives the report and returns a *ReportArchivedError naming `lyx webster begin-batch`, which re-drives the batch.
// A report with no begin-batch record is archived by begin-batch itself, which proceeds and returns the archive path as BeginResult.ArchivedReport;
// only a batch with no record is archived this way, and a recorded batch's report is never archived by begin-batch.
//
// # The verify-gate fixer fork
//
// A gate failure reaches Merriam as a `Gate findings recorded at …` message naming the verify-gate report (VerifyGateReportPath).
// Merriam spawns one fixer fork in the background with the prompt Run rendered at entry (RenderVerifyFixPrompt, into the prompts dir as verify-fix.md), ends its turn, and on the fork's notification rewrites its outcome and summary files, which re-arrives at the gate.
// The waiting turn end in between is no arrival.
// The fixer fixes the cause in source, never deletes, skips or weakens a test, never touches the plan directory or `_lyx`, commits each fix as `fix: <summary>` and does not run the plan-level verify.
// Its commits skip record-batch's done-checks, drift detection and glyph scope guard.
// They are bounded by the gate's commit check, the run-exit fork audit, the gate's own verify and Webster-Review, which reviews the committed range.
// The plan directory and `_lyx` sit outside every code commit, so the run-exit fork audit is what catches a fixer write there:
// forkOwnReport maps a fork transcript outside every batch bracket to the verify-gate report, and CheckFork audits the fixer fork as it audits a batch fork.
//
// # The verify gate
//
// Run hands Merriam's spawn the plan-level verify as one must-pass gate entry named `verify` (NewVerifyGate), beside any entry the caller's RunDeps.Gate carries;
// a RunDeps.Gate that already names `verify` is refused, so a recipe row cannot add a second entry.
// Its attempt budget is Config.VerifyGateAttempts, and the count and the pre-fix head live in the closure's memory for one shuttle run, so an attach restarts them from zero.
// The shipped Webster row supplies no gates of its own, and StartMaster is Merriam's only start, so every Merriam carries the spec.
//
// At each gated Done arrival the closure passes without verifying when outcome.yaml names an outcome other than done, so a stuck or paused outcome ends the run as before.
// Otherwise it fails with the dirty paths when the tree is not clean, and runs no verify.
// Once a pre-fix head is recorded, every commit from it to HEAD on the first-parent chain must be a non-merge commit or a clean parent merge (fixCommitRejection, in gitwrap.go);
// any other fails the gate `Terminal`, which ends the run at once with the commit and the reason.
// A plan with no `## verify:` section passes with a warning.
// Otherwise verifytree.Verify runs the command under the site label `webster gate`, and a pass records the tree.
// A failure is parsed from the log and rerun once: a rerun pass passes the gate, and the identities that failed once are flaky, which Run reports after the wait as a warning, a summary.md section and a friction note (VerifyGateNotes.Apply).
// A failure that survives the rerun returns findings.
// A run that outlives the verify timeout is never rerun, since a hang is not flakiness: it fails the gate at once, and the report names the timeout, the log path and the log tail.
// The first failed evaluation of any kind records HEAD as the pre-fix head, so every commit a fixer makes afterwards is checked at the next arrival.
// The same moment persists it as state.json's `PreFixHead`, overwriting a value an earlier shuttle run left, so a later verb can reset to it;
// a passing evaluation clears it, and `run --fresh` archives state.json with it.
// A persist failure is returned as the gate's error.
//
// After the wait, a done run whose gate did not pass ends stuck, with a reason naming the failing identities and the attempts spent, or the `Terminal` failure's own reason.
//
// # Background shells in Master's wait
//
// Master's spawn declares one awaited shell prefix, `masterAwaitedShellPrefix` (the backgrounded recovery verb of the failure ladder);
// recovery_timeout_min already bounds that verb, so shuttle's turn-end wait treats it like a fork.
// Every other background shell is waited out after `background_shell_wait_min`, and the labels come back on shuttle's `Result.ExpiredShells`.
// lyx does not stop such a shell: the wait only stops waiting and counts Master's turn end.
// What ends it follows the run's outcome.
// When Master's turn ends shuttle-done (a webster done, Master's own stuck or paused, the verify gate's demotion, or a mapping error after that end), shuttle removes Master's strand as the run finishes, and the session and the shell end with it.
// When the run returns an asking, died or timeout error, Master's strand stays alive until the next `lyx webster run` reclaims it at entry.
// Run writes one best-effort `webster-background-shell` friction note once the outcome is known, on every outcome.
// For each label the note states the bound, that the turn end was counted, that lyx did not stop the shell, what ends it and the run's outcome after that turn end.
// Each label is also a `RunResult.Warnings` entry with the same wording on every outcome that returns a `RunResult` (done, stuck and paused), after the verify-gate demotion, so the warning and the note cannot drift;
// an error outcome returns no `RunResult`, so the note alone carries it.
// summary.md's "Background shells waited out" section (AppendBackgroundShells) stays done-only.
//
// # Planning a reset
//
// PlanReset decides, read-only, what a reset of the task branch may do, and refuses before fabric is ever called.
// The target is `start` (ResetToStart) or `pre-fix` (ResetToPreFix); no raw SHA is accepted.
// `start` is the run's oldest recorded start commit, or the octopus merge-base of the recorded starts when none is the oldest of all;
// `pre-fix` is state.json's `PreFixHead`.
// The refusals run in order: run lock held (transient), no state, a merge in progress, a checked-out branch that is not the task branch,
// no recorded target, a recorded commit missing from the repository, a target that is not an ancestor of HEAD, and a dirty tracked path outside the run's own writes.
// The own paths are the tracked paths that loadRunWrites records a successful write to, Master's and every fork's; a failed write is not evidence.
// Each refusal ends in wayForwardSteps' numbered list.
// The plan carries the SHA and the own paths, and reads no force flag.
//
// # The reset verb
//
// `lyx webster reset --to start|pre-fix` (internal/webstercli) performs the reset PlanReset planned, so no recovery needs the denied `git reset --hard`.
// Under the state-mutation lease it plans with the pair's branch read through the fabric handle,
// resets the pair's code checkout through fabricengine's pair-checkout reset with the plan's SHA, the parent branch from the origin record and the own paths,
// clears State.PreFixHead, saves, and fabric-syncs state.json.
// It changes no other webster state; `run --fresh` or a plain `run` does the rest, as the way-forward texts order them.
// The reset also moves the task branch on the remote to the same commit, so a later push is not rejected as diverged; `WEFT_SKIP_PUSH=1` skips that half.
// The bound: it can discard only commits above a run-recorded commit on the task's own branch, on the checkout and on the remote, and uncommitted tracked changes to paths the run itself wrote.
// The remote update is made only when every commit it drops is reachable from the task worktree's HEAD and under a lease on the remote tip it read;
// a remote-only commit becomes reachable only through the `git merge --strategy ours` the operator runs after reading the commits the refusal lists.
// It cannot move another branch, take a raw SHA, touch the parent branch, the records side or untracked files, and it has no `--force`.
// Fabric's own refusal (ownership, dirtiness, remote divergence, an unreachable remote) is surfaced as the verb's error with fabric's reason.
// In standalone mode the verb plans, then refuses naming `git reset --keep <sha>`, since standalone has no pair for the fabric gate to guard.
// The envelope carries `target`, `sha`, `mutations` (the `worktree_reset` entry, and `remote_branch_updated` when the remote moved) and `partial`, false on success.
// A refusal before the remote update is a bare error envelope.
// A checkout rewrite that fails after the remote moved is an error envelope carrying `mutations` and `partial: true`, and re-running the reset converges.
// Each refusal has a row in contracts/specs/refusal-spec.md.
//
// # The verify-gate report and findings
//
// Every failed evaluation writes the verify-gate report (VerifyGateReportPath, `verify-gate.yaml` in the reports directory) and returns renderVerifyGateFindings of it as the findings Merriam reads.
// The report carries the attempt and the cap, the failing identities or the dirty paths, the verify log's path, the commits the fixer made since the pre-fix head, and a hint:
// the cards, in trail order, whose commits changed a file directly in a failing package's directory (cardHint over accumulatedCardSHAs).
// The hint claims only that a card touched a failing package;
// Merriam judges it with the plan in hand.
// The gate runs no bisect, no baseline run and no detached checkout of the live worktree.
// Run removes a stale report at entry.
//
// # The Git seam
//
// Every question the bracket verbs and the run-level checks put to a worktree's repository goes through the Git interface (git.go):
// the head, dirtiness, a merge in progress, a commit's parents, the clean-parent-merge verdict, commit existence and ancestry, ignore rules, linked worktrees, blobs, and the delta of a commit range.
// Geometry.Git carries it, and nil means the real repository, so no production caller sets it and the helpers in gitwrap.go stay the one place webster runs git.
// A test that asserts on webster's own records, warnings and verdicts sets a fake and spawns nothing;
// a test whose behavior is git itself (merge commits, ignore rules, blobs, the quarry delta, the verify gate) builds a real scratch repository under the `integration` tag.
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
// (gitwrap.go, reached through the Git seam below), archive-never-refuse primitives (archive.go), and
// recovery-classification logic (classify.go). Each of these exists as its
// own webster-scoped implementation rather than an imported one — this
// package owns its whole mechanism end to end.
//
// # Why cards run one at a time
//
// Cards run strictly in sequence inside one working tree, and the plan carries no scheduling DAG.
// Git's index is a single shared file per working tree, so two forks committing at once race on the same lock even when their files are disjoint.
// A declared-disjoint pair that turns out through a deviation to overlap would be a live corruption risk, not a bookkeeping error to fix afterwards,
// and a fork's code-index queries would see other forks' uncommitted, possibly broken, in-flight edits, since nothing isolates them on disk.
// Letting forks edit in parallel and serializing only the add, commit and verify step through a mutex would still need file-disjointness enforced strictly, not merely the absence of a dependency edge.
// None of this is built.
//
// A shape that avoids the shared index is not covered by that reasoning: each independent group in its own `fabric`-spawned worktree, with its own index and HEAD,
// merged back through fabric's existing merge machinery and gated on a build and test of the merged result rather than of the group alone.
// The ready set would be recomputed wave by wave rather than precomputed, so a group that proves to depend on another's output is simply not ready yet.
// Batten's run-id addressing already carries concurrent runs side by side, so what remains webster's is the wave scheduler, a plan group-filter,
// and a variant that spawns its agents into the lane worktree's own reed session instead of forking inside the Master's context.
// The board holds this as `webster-parallel-execution`.
//
// The measured case for it is weaker than the idea suggests.
// A card-level dependency analysis of the 42-card plan that built webster overturned the assumption of a linear chain:
// that plan ran as nine sequential batches over a card DAG of depth seven, with a peak wave width of ten and wave widths of 10, 9, 7, 7, 6, 2 and 1;
// 35 of the 42 cards were off the critical path, and about 26 cards in the first three waves could have run as three parallel waves instead of four sequential batches.
// File conflicts barely bind when a plan creates and then extends, since nearly every conflicting pair is already ordered into different waves.
// The tail is the ceiling rather than the dependencies: a funnel near the end (final registration, then sandbox validation) collapses the wave width whatever the fork budget.
// A wave lasts as long as its slowest card, so the honest estimate is a 2–3× wall-clock speedup, not the 3–5× a naive count gives,
// and 42 cards is an outlier: a routine 5–10-card plan has little fan-out headroom and would gain close to nothing.
// The cheap part of the insight is already taken, because the planner emits true per-card dependencies instead of an over-constrained batch line.
// Reopening the executor should rest on the same width analysis run across several real, typical-size plans:
// wide waves, a short critical path and few file conflicts would pay off, and a narrow plan makes the sequential design the complete design rather than the minimum one.
package websterengine
