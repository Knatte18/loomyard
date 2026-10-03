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
// Known limit: the integration stage's bisect over earlier CardSHAs runs on pre-merge trees.
// Known limit: bisect and triage attribute a regression introduced by a mid-run parent merge to the first card after the merge.
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
// `rebaseline` accepts an operator's edit, so it never moves a begun card's hash.
//
// A foreign edit an operator means to keep has its own way forward:
// `lyx webster rebaseline --card NN` (Rebaseline) accepts the on-disk plan as the new baseline without dropping any batch record, provided the edited plan's batch of each recorded number still holds exactly the cards that record names.
// The operator names every card the edit changed with --card: State.PlanFileHashes records a hash of every plan file, and a changed card file whose number is not named is refused.
// An edit to 00-overview.md, which carries the plan's integration verify, is never accepted; the way forward is to restore it or to reset the branch and run `lyx webster run --fresh`.
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
// At run exit the audit cross-check drops dispositioned findings, records the rest of the policy findings as run-level warnings, appended to summary.md under "Audit warnings", and demotes Master's outcome done to stuck for an undispositioned correctness finding.
// A correctness finding stays pending in state.json until `lyx webster accept-audit` clears it, and run entry refuses with ErrPendingAuditFindings meanwhile.
// accept-audit needs evidence: it checks every suspect path against the last batch head (a plan file against the run's recorded plan hashes) and refuses with ErrAuditNotAcceptable while any path differs, cannot be checked, or a finding names no path;
// the evidence covers HEAD too, so it also refuses while HEAD carries a commit past the last batch head other than a clean parent merge, and checks the paths against that reconciled HEAD;
// the last two clear only through `lyx webster run --fresh` after resetting the branch to the run's start commit.
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
// Because the fork runs inside Master's own session, there is nothing for
// Go to spawn in the normal path — spawn-batch does not exist here. Go
// provides thin bracket verbs Master calls around each fork: begin-batch
// (pause/fingerprint checks, records the batch's start-SHA, idempotently
// asserts Master's model for this batch, renders and writes the fork
// prompt) immediately before forking, and record-batch (incremental fork
// audit, batch-report parsing, digest distillation, state update) once the
// fork has delivered. The Agent-tool fork is a BACKGROUNDED agent: the fork
// call returns immediately, before the batch is done, so Master ends its
// turn right after spawning it and calls record-batch when the fork's
// completion notification starts its next turn. That turn end does not end
// the run: shuttle reads a turn that ends with a background agent still
// running as EventWaiting, which its wait loop treats as still running,
// so Master spends no turns while a fork works. await-batch (a stateless,
// bounded wait on a batch's report path) remains as a verb an operator can
// call, but no longer sits in Master's loop. Go's
// gates only run when Master actually calls them — the fork itself is
// Master's own un-gateable act, so enforcement is two-layer: template
// discipline (the master template pins the begin -> fork -> notification ->
// record sequence, property-tested) plus fail-loud detection after the fact
// (record-batch archives the report and refuses when a batch has no begin-batch record, naming begin-batch as the way forward;
// the audit cross-checks fork-transcript count against begun-batch count). This
// is a steering guard, not a security boundary, the same class as burler's
// nested-Agent ban.
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
// recover-batch: a blocking, re-entrant verb that spawns a fresh
// implementer as its own shuttle/reed strand at the recovery role when a
// fork reports stuck or writes no report, rendering the SEPARATE, full
// cold-start recovery prompt (RenderRecoveryPrompt) — deliberately distinct
// from a fork's own thin RenderForkPrompt, since the recovery strand
// inherits no session context (see the fork-context-hygiene Shared
// Decision). The call that spawns the recovery strand first waits for its
// provider to come up (normally seconds, bounded by startup_timeout_s), and
// every call then blocks for RecoveryWaitBudget (recovery_timeout_min plus
// one poll tick) and returns a terminal digest: the budget outlasts the
// timeout measured from spawn, so a strand that never reports classifies dead
// on its timeout and the call returns. A re-entrant call finds the strand
// already recorded in state and skips straight to the wait. Merriam runs the
// call as a backgrounded Bash command, ends its turn and acts on the
// completion notification; only an operator's shorter --wait can return a
// running snapshot. This mirrors classify.go's dead/timeout/stuck
// classification.
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
// caller supplies instead — RefMatcher for the fork-audit's fabric-reference violation class, and
// FabricBisector, reached through RunDeps.OpenBisector, for the integration-suite bisect.
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
// record-batch treats exactly as it treats a forged report:
// it archives the report and returns a *ReportArchivedError naming `lyx webster begin-batch`, which re-drives the batch.
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
// fork and waits for its completion notification the same way;
// AwaitIntegration, Go's own bounded wait at run exit, mirrors await-batch's
// idiom over the single fixed IntegrationReportPath rather than a per-batch
// report path, and a missing integration report at run exit is
// outcome-aware: fail-loud under Master's outcome: done (a done claim
// requires a passing suite), consistent-and-preserved under stuck (the
// fork died or the stage never started; Master's own judgment stands).
// The fork redirects the verify command's output to a scratch log (IntegrationLogPath),
// so Go can name every failing test and its output tail (parseVerifyFailures) without trusting the fork's own account.
// On a FAILED integration report, webster reruns the verify once at head and compares the remaining failures against the plan's starting commit,
// classifying each failing identity flaky (the rerun passes), pre-existing (it also fails at the baseline), or regression.
// The baseline is the earliest of the batches' recorded start commits by ancestry, since begin-batch does not enforce execution order;
// start commits with no single earliest member leave nothing excused as pre-existing.
// A test identity is the deepest failing test or subtest path go test prints, so a failing TestX/a at baseline never excuses a new TestX/b.
// Triage excuses only what it can attribute to a named test: a package identity (a build or setup failure, a panic or timeout, a TestMain failure, a test binary that exited before reporting) or an opaque one is never pre-existing,
// though it is still flaky when the rerun passes cleanly.
// For the same reason the verify command must be a plain "&&" chain whose rerun reached its last step (a marker echoed before that step proves it),
// and must not run with -failfast;
// otherwise a non-test step's failure, or a step left unrun behind a pre-existing failure, could hide behind it, and every failure is a regression.
// A step that runs go test inside a script is outside this reach: its own non-test failures surface only as the step's exit status.
// The report carries the result in its optional failures and triage fields.
// A regression demotes a Master outcome: done to stuck;
// flaky and pre-existing failures keep the outcome and are recorded as RunResult warnings, an "## Integration suite triage" section in summary.md (AppendIntegrationTriage), and a friction note.
// A regression is then localized: bisect performs an in-process binary search over the accumulated per-card SHA trail (every terminal batch's own BatchState.CardSHAs) — checking out each candidate SHA detached and running the plan's verify command in-process via os/exec, never a fork per bisect candidate — to find the first offending card in logarithmic, not linear, re-runs.
// A candidate passes when none of the regressing identities fails there, not when the whole command exits zero,
// so an unrelated flaky or pre-existing failure cannot misdirect the search.
// BisectAndEscalate then records that localized finding as a terminal, non-successful entry in State.Batches under the reserved key -1 (RecordIntegrationFailure — never a real plan card number, so RenderProgress's walk over batch numbers, which are equally positive, can never surface it by accident)
// and extends summary.md naming the offending card and the regressing identities with their tails (AppendIntegrationFailure).
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
// The first failed evaluation of any kind records HEAD as the pre-fix head, so every commit a fixer makes afterwards is checked at the next arrival.
//
// After the wait, a done run whose gate did not pass ends stuck, with a reason naming the failing identities and the attempts spent, or the `Terminal` failure's own reason.
//
// # The verify-gate report and findings
//
// Every failed evaluation writes the verify-gate report (VerifyGateReportPath, `verify-gate.yaml` in the reports directory) and returns renderVerifyGateFindings of it as the findings Merriam reads.
// The report carries the attempt and the cap, the failing identities or the dirty paths, the verify log's path, the commits the fixer made since the pre-fix head, and a hint:
// the cards, in trail order, whose commits changed a file directly in a failing package's directory (cardHint over accumulatedCardSHAs).
// The hint claims only that a card touched a failing package; Merriam judges it with the plan in hand.
// The gate runs no bisect, no baseline run and no detached checkout of the live worktree.
// Run removes a stale report at entry.
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
