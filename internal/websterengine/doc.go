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
// batcher.yaml's `active:` key (template default: the cautious profile,
// the cost batchifier; an empty value resolves to the identity batcher —
// one card, one batch), resolved by internal/webstercli and handed to Run via
// RunDeps.Batcher — this package loads no batcher config itself. Batching
// is a standalone step webster consumes today, and one Shed will drive as
// producer #8 once built, never the plan's (a card carries no
// batch-membership field of its own) and never an LLM's (no batchifier
// consults a fork's judgment). BatchState is keyed by execution-batch
// number, not card number: a grouping batchifier puts several cards in one
// batch, and only under identity do the two numbers coincide.
//
// # Merriam's start base enters at partition time
//
// The batchifier prices each fork from Merriam's context at that point, whose start part is the start base this package computes in merriambase.go:
// the line count of what Merriam loads at start (the worktree's CLAUDE.md and CLAUDE.local.md, the Master stencil with the orchestrator PATTERN directive, and the plan's 00-overview.md) plus a fixed system-prompt-and-tools context.
// Run computes it where it forms a new partition (first init and `--fresh`), Rebaseline is told it when it batches the tail, and validate computes it for a run with no state;
// a recorded partition keeps the estimates it was formed with, so the base never regroups a run.
//
// # Batches run in the batchifier's order, asserted not derived
//
// internal/batcher owns both grouping and order: a batchifier returns its batches in plan card order, and webster runs them in that order, never reordering them.
// sequence.go's CheckBatchOrder only asserts it: it derives edges from Targets/Uses ref matching across the plan's cards — a Uses entry naming another card's Targets entry puts the producer before the consumer, and two cards writing the same Targets entry settle by declared card number — and refuses, wrapping ErrBatchOrder, when any edge runs from a batch to an earlier one.
// A cycle between batches always holds such an edge, so the same rule refuses it.
// Plan-Gate's uses-later-target refusal already holds, so the assertion fires only on a plan that bypassed or predates that gate.
// The previous-digest lookup in beginbatch.go/recoverbatch.go (predecessorDigestLine) depends on the order: it reads whichever batch sits immediately before the target batch in the execution order, not the batch one number lower.
//
// # The partition is recorded once per run
//
// The first init of a run (no state.json, or the --fresh re-init) forms the partition with the active batchifier, refuses it on CheckBatchOrder's error before saving anything, and records it in State.Partition: each batch's card ids, profile and estimate, and for a cost profile the estimate's breakdown (weights, startup, read union and per-card components), so a finished run can be fitted against its forks' measured peaks.
// Every other verb reads that record through partition.go's ExecutionBatches, which maps the recorded ids onto the plan's cards and re-asserts the order, so a size the batchifier weighed changing under the run's own commits never regroups cards mid-run.
// A recorded id the plan lacks, or a plan card in no recorded batch, is refused with ErrPartitionMismatch.
// A state written before the field existed records no partition and runs on the identity batchifier whatever profile is active, so an in-flight run keeps the grouping it started under.
// Batches run in the recorded order.
// Only a first init or a rebaseline replaces the record.
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
// fork returns FAILED on a failed card gate, card `**Verify:**` or batch gate, never on deviation alone and never on a slot-busy exit.
// Plan-predicted file impact is frequently incomplete, so treating deviation as failure would make the system impractically brittle.
// Within one fork's batch, each card lands as its own commit (BatchState.CardSHAs records the ordered per-card SHA trail — one element under the identity batcher, more once a grouping batchifier ships), and each card's own optional `verify:` runs and must pass before that card's commit.
// After the last card the fork runs the batch gate once, over the batch's packages with `-tags integration`;
// Go never runs it, and Webster's plan-level verify gate catches a fork that skips it.
// A card's own gate, which begin-batch and recover-batch render into the fork's prompt, is `lyx gate test` over the card's own package directories, one step per module (a directory inside a nested module is tested inside it with `lyx gate test -C <module>` and module-relative paths), one step per line, and ends with `lyx loom lint-comments`, the comment line-break lint.
//
// record-batch and recover-batch apply one merge-only rule when they cross-check the consumed report's `head_sha` against the worktree's HEAD,
// so a parent merge-in landing between a fork's commit and the report's consumption cannot wedge the run.
// HEAD is accepted when it equals `head_sha` or sits above it by clean parent merges alone on the first-parent chain:
// each walked commit has exactly two parents, its second parent is reachable from the parent branch in fabric's origin record, and its tree equals the conflict-free merge of the two.
// An evil merge, a hand-resolved conflict, a merge of any other branch, an octopus, or any merge in standalone mode (no parent branch) is refused,
// because the audited delta ends at `head_sha` and such a merge's own content would bypass it.
// The batch is recorded at the report's `head_sha` (CardSHAs and the delta's end), while the done-checks and drift detection read the merged tree as it stands,
// and a warning names the walked merge SHAs.
// A `head_sha` may be an abbreviation of 4 to 39 hex digits, resolved to the one commit it names before any comparison,
// and an abbreviation naming no commit or several is refused with ErrHeadSHAUnresolved.
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
// loom's plan gate takes a narrower scope from DoneCards: only the cards of a batch recorded terminal with status done, read from the batch records alone, with no batchifier.
// A run moved back to the plan review keeps its run record, and those cards are history whose work is in the tree.
// The gate reads EditedDoneCards beside DoneCards: the ids of those cards whose file no longer hashes to the CardHashes entry their batch recorded at begin, from the batch records and the card bytes alone.
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
// `rebaseline` accepts an operator's edit, so it never moves a begun card's hash, except for a card the operator names with --card whose batch is in flight or terminal failed, dead or stuck.
//
// A foreign edit an operator means to keep has its own way forward:
// `lyx webster rebaseline --card NN` (Rebaseline) accepts the on-disk plan as the new baseline without dropping any batch record, provided the edited plan's batch of each recorded number still holds exactly the cards that record names.
// With a recorded partition, Rebaseline keeps every batch up to the last begun one as recorded, profile and estimate included, and refuses a plan whose first cards are not exactly those batches' recorded cards.
// It batches every plan card after the kept prefix with the then-active batchifier, asserts the order over kept batches and tail together, and only then replaces State.Partition;
// cards added, removed or reordered after the last begun batch are accepted and regrouped.
// A state without a partition regroups with the identity batchifier and records none.
// The operator names every card the edit changed with --card: State.PlanFileHashes records a hash of every plan file, and a changed card file whose number is not named is refused.
// A named card of a batch that is terminal failed, dead or stuck is accepted even though that batch was begun:
// Rebaseline restamps that card's CardHashes entry and keeps the rest of the batch record, so a one-card fix needs no reset and no fresh run.
// A named card of an in-flight batch, begun and not terminal, is accepted the same way and is also recorded in the batch's AmendedCards (AmendedCard) with Rendered false;
// a card already there has its mark set back to false, and RebaselineResult.CardsAmended reports the accepted ones as `cards_amended`.
// The fork or recovery strand running then keeps working on the old text and nothing is stopped; recovery re-runs the batch on the edited cards.
// That re-run is forced.
// When record-batch records a fork's report, or recover-batch records a recovery's terminal digest, and the batch holds an AmendedCards entry with Rendered false, the batch first runs every normal check.
// It is then recorded failed through failBatch, whatever the report's status.
// failBatch adds a reason per unrendered entry to any failure and sets BatchFailedError.CardAmended, which both verbs' batch_failed envelopes carry as `card_amended`;
// the reason is recoverable, never an Uncheckable entry, so recover-batch never answers needs_fresh for it.
// Every recovery spawn lists each AmendedCards entry of the record it replaces in the prompt's failure digest, with an instruction to re-read the card and bring the committed work in line with it,
// and carries the entries onto the fresh record with Rendered true, so that recovery's own report is not forced failed.
// A recovery recorded done clears the entries; one that ends stuck, dead or failed keeps them, so the next spawn renders them again.
// Each amendment thus forces at most one failed record and so at most one extra recover-batch;
// Master's template re-runs recover-batch on a `card_amended` refusal even after a failed recovery.
// A done batch and a failed batch whose record lists Uncheckable entries still refuse, each with its own way forward.
// The done batch's way forward is the follow-up card landing below; the failed batch's is to restore the card or take the reset route.
//
// 00-overview.md carries the plan's integration verify, and only its Card Index section is rebaselined.
// State.PlanOverviewFrameHash is the hash of planparser.OverviewWithoutCardIndex over the overview, recorded wherever State.PlanFileHashes is.
// When the overview is among the changed plan files, Rebaseline accepts it if the file's frame hash still equals the run's recorded overview frame, so the change is confined to the Card Index.
// The index change is then held to the card-set rule, which compares NN-slug ids: only cards after the last begun batch can be added, removed or reordered, and a begun card's index line keeps its number and slug.
// What passes besides is a reworded one-line intent of a begun card in a later Master render, while that card's file stays pinned by its recorded hash.
// A state without the frame hash takes the recorded frame from the stored baseline copy of the overview until the first restamp.
// A run with neither refuses any overview change, and a change outside the Card Index refuses naming the overview outside its Card Index.
// An empty recorded frame has one of three causes, no overview hash recorded, the baseline copy absent, or a copy without a parseable Card Index;
// Rebaseline logs the cause once at Info, with the copy's path where there is one, and the no-frame refusal names both.
// Every refusal and transient of Rebaseline is keyed on RebaselineDeps.Step: empty keeps the manual verbs, and a set step, the loom's Webster row re-step, replaces them, so no text on that path tells an agent to run a verb by hand.
// A done batch's card and a changed frame refuse with the follow-up card landing as their way forward:
// add a follow-up card after the last begun batch that carries the decision, with its Card Index line, then `lyx webster rebaseline --card NN` naming it.
// The fingerprint refusals in begin-batch and run name the landing too, and for an index-only change name rebaseline with the added cards' flags.
// An operator decision taken mid-Webster reaches the plan in this order:
// `lyx loom decision add` for the record, the card edit (or a follow-up card with its index line), then `lyx webster rebaseline --card NN`.
// No refusal or prompt text names a loom verb.
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
// A fabric reference is correctness too unless its command is read-only, since an agent never touches the fabric repo for a write and the command can rewrite run state the cards' verify commands cannot detect;
// the read-only classifier is injected (RecordDeps, RunDeps and RecoverDeps carry it as ReadOnly) and sees static shape only, so a command behind `bash -c`, a variable or a substitution stays correctness, and a nil classifier accepts nothing.
// The classifier splits a `lyx`, `git` or `sed` segment into shell words, so a quoted word is its dequoted text, and a quoted writing flag is still rejected.
// An unquoted glob character is rejected, except after the `--` of `git log`, `show`, `diff` and `status`, where every word is a pathspec.
// A leading tilde is text, so a home path is admitted.
// The stderr redirections `2>&1` and `2>/dev/null` are dropped before the redirection scan, and every other redirection still fails it.
// `sed -n` with a line number or a range of two line numbers followed by `p`, then file words, is a read.
// A trailing `--json` is a help form of a `lyx` segment, since the global flag raises help before the command runs.
// A policy finding breaks a steering rule without touching correctness, such as a named spawn, a nested agent call or a read-only fabric reference.
// Each finding carries a stable identity (its Key, prefixed by the session id for a parent finding),
// and state.json's ledger dispositions it once per run, so the whole-session parent audit repeating earlier findings on every record-batch never re-judges them.
// A policy finding is recorded as a warning on the batch once the evidence holds:
// an OK report on a batch that carries policy findings first has its cards' verify commands re-run in-process (rerunCardVerifies), and a failing re-run makes those findings correctness for the batch and fails it.
// A rerun command never sees gateslot.PrebuiltLyxEnv, slotted or not, so its own `go test` builds lyx once per test binary.
// A correctness finding fails the batch on its merits instead of wedging it:
// the batch goes terminal with digest status failed and its reasons, the report is archived, and record-batch returns *BatchFailedError naming `lyx webster recover-batch`.
// recover-batch proceeds from a failed batch and hands its strand the failure digest,
// except a batch failed on a correctness finding the recovery check cannot verify (a finding with no path, or a path outside the tracked tree and the plan directory), which its record lists as Uncheckable:
// recover-batch refuses it with ErrRecoveryNeedsFresh before spawning anything,
// and the way forward is `lyx webster reset --to start`, which archives the run record, and then `lyx webster run`.
// `run --fresh` drops such a batch under the same HEAD and path rules as a pending finding.
// A pathless entry is recognised by its class prefix and never resolved as a path.
// One narrow exception keeps the batches before it (AcceptBatchFabricReference, `lyx webster accept-audit --batch NN`):
// when every Uncheckable entry is a pathless fabric reference, the batch recorded a start commit and the worktree is clean apart from the run's own state,
// the explicit call clears the entries and records each as a batch audit warning, and recover-batch then proceeds;
// the refusal names that route only for such a record.
// It has two routes.
// Either HEAD is that start, so the batch changed nothing (a batch that committed qualifies after `lyx webster reset --to batch-start --batch NN`),
// or the start is an ancestor of HEAD and every entry's recorded command is read-only (`fabricengine.IsReadOnlyCommand`), so the batch's commits are kept.
// An entry that is not read-only, or records no command, refuses the whole call with the reset-to-start and fresh-run steps.
// The evidence shows the start still lies in HEAD's history and nothing uncommitted;
// no command the classifier accepts writes tracked content, a ref, config or a remote, and `git status` may refresh the index's stat cache, which changes no content.
// It does not inspect the batch's commits.
// The fabric repo's own state it cannot show, and the caller vouches for it by running the verb.
// recover-batch runs the same call in line (RecoverSpawnOrAttach, with RecoverDeps.ReadOnly) before its refusal, when every Uncheckable entry is a pathless fabric reference whose recorded command the classifier accepts:
// on success the warnings are recorded on the batch, each naming `recover-batch`, and the recovery spawns;
// an evidence failure returns ErrAuditNotAcceptable, which recover-batch's envelope carries as `audit_not_acceptable` and whose way forward names `recover-batch` for the re-run.
// A command the classifier rejects, a nil classifier, an entry of another kind or a fabric reference recorded with a path keeps the refusal.
// record-batch on a batch already terminal as a fork batch first audits the fork transcripts it has not consumed, once and without the settle wait:
// an undispositioned correctness finding (a fork that marked its own batch done by writing state.json) replaces the terminal record with a failed one,
// and otherwise the "already terminal" refusal stands.
// It audits nothing while a later fork batch of the session is open or the verify-gate report exists, since an unseen transcript may then be that fork's.
// A batch's own bracket transcripts, the ones record-batch attributed to it since its begin-batch, count toward attribution when one of their writes is the batch's report,
// and every one of them is re-audited in full on each call, so a fork stopped and resumed across a no-report call is attributed on the next.
// begin-batch and recover-batch open an empty bracket list, so a transcript of an earlier bracket never counts.
// A report that cannot be attributed to a begun batch, or to any counted fork transcript, is archived and returned as *ReportArchivedError naming `lyx webster begin-batch`, which re-drives the batch.
// The post-batch done-checks fail the batch the same way when a card's own declared work is missing, while drift that concerns only a later card is recorded as a warning rather than blocking this batch.
// A delete-not-done finding gets one more check, planindex.Index.LaterDeleteReferences over the batch's own cards and the cards of every batch with no record:
// when an unbegun later card's Edit code still references the target, the failure's reasons name that card and the reference,
// and the BatchFailedError's way forward is the plan edit (move the delete after that card, rebaseline, then recover-batch), or the reset-to-start steps when the record also lists uncheckable entries, since recover-batch would repeat the same failure.
// PersistRecoveryTerminal fails a recovered batch the same way, and recover-batch runs the same check before spawning:
// while it fires it refuses with ErrRecoveryDeleteReferenced, which the CLI maps to the `batch_failed` flag.
// At run exit the audit cross-check drops dispositioned findings, records the rest of the policy findings as run-level warnings, appended to summary.md under "Audit warnings", and demotes Master's outcome done to stuck for an undispositioned correctness finding.
// A correctness finding stays pending in state.json until `lyx webster accept-audit` clears it, and run entry refuses with ErrPendingAuditFindings meanwhile.
// accept-audit needs evidence: it checks every suspect path against the last batch head (a plan file against the run's recorded plan hashes) and refuses with ErrAuditNotAcceptable while any path differs, cannot be checked, or a finding names no path;
// the evidence covers HEAD too, so it also refuses while HEAD carries a commit past the last batch head other than a clean parent merge, and checks the paths against that reconciled HEAD;
// the last two clear only through `lyx webster reset --to start` and then `lyx webster run`.
// The run-exit stuck reason and the pending-findings refusal name each finding once (findingsClause), an uncheckable path carrying its reason (uncheckableReason),
// and end in one ordered list: the restores, then `lyx webster accept-audit`, then exactly one re-entry step, RunDeps.ReentryStep (`lyx webster run` when empty);
// the reset route ends in that same re-entry step, since the reset archives the run record and a plain run starts over.
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
// The archive `run --fresh` performs is one function (archiveRunInPlace): state.json and the reports dir renamed with a stamp, the rendered prompts cleared.
// ArchiveRunAfterReset is the reset's entry to it, behind the same pending-findings checks (checkPendingFindings) judged against the worktree's HEAD in place of the recorded start.
// After a moving reset HEAD is the start, so the checks are the ones `run --fresh` runs there; on an archive-only reset the recorded start may be missing or not an ancestor of HEAD, and the tree the next run starts from is HEAD's.
// A HEAD-relative guard therefore never refuses toward a verb that cannot clear the state: it refuses only on an uncleared contract file, a plan path differing from the recorded plan and a suspect path differing from HEAD, each naming its clearing step and then the reset, and drops every other pending finding with a warning.
// A branch rewritten under the run passes, and the dropped findings stay readable in the archived state.
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
// The recovery prompt also carries a block of the worktree's uncommitted paths (the optional uncommitted_paths marker, `none` on a clean tree), taken from UncommittedPaths when the strand is spawned.
// Each path is grouped by the run's write evidence (loadRunWrites): "Written by this run" is the earlier attempt's unfinished work, which the strand keeps as its starting point after checking it against the card, and "Not written by this run", which includes every path the evidence cannot attribute, the strand leaves untouched and never stages or commits.
// The call that spawns the recovery strand first waits for its provider to come up (normally seconds, bounded by startup_timeout_s),
// and every call then blocks for RecoveryWaitBudget (recovery_timeout_min plus one poll tick) and returns a terminal digest:
// the budget outlasts the timeout measured from spawn, so a strand that never reports classifies dead on its timeout and the call returns.
// A re-entrant call finds the strand already recorded in state and skips straight to the wait.
// Every terminal digest (done, stuck or dead) stops the recovery strand through shuttle's stop verb, even when the call then refuses on a merge in progress or a report head mismatch;
// only a done digest also removes the run dir, which stuck and dead keep for diagnosis.
// Every reclaim of a leftover strand (entry-time, begin-batch's prior recovery strand, recover-batch's prior strand and reset) goes through that verb, which records the stop on the strand's run before it removes a live strand, so a stop that cannot be recorded fails the reclaim.
// `lyx webster reset --to start` likewise stops every recovery strand the state records before it resets,
// so no recovery agent keeps writing into the tree the reset moves;
// `--to pre-fix` removes none.
// Merriam runs the call as a backgrounded Bash command, ends its turn and acts on the completion notification.
// Only an operator's shorter --wait can return a running snapshot.
// A recovery strand's turn end with no report classifies dead/asking only when it is the strand's newest turn signal and nothing keeps it waiting (TurnEndedAfter):
// a later turn start is the strand working again, and a plain turn end counts only after it has stood a few seconds, because a background shell that finished just before the turn ended starts the next turn through its completion notification (issue #498).
// A turn end left waiting on any task, a shell of either signal or a fork, never counts, so recovery_timeout_min bounds that strand and a report present before it classifies done.
// This mirrors classify.go's dead/timeout/stuck classification.
//
// A batch's recovery spawns are counted in BatchState.Recoveries, and the spawn records HEAD as RecoveryStartSHA, read before anything is stopped or started, so a HEAD that cannot be read starts and counts nothing.
// A re-begin and a rebaseline keep the count.
// A spawn whose prompt renders an amendment no earlier spawn rendered does not count, so an amendment forces at most one uncounted re-run.
// A counted spawn is refused with ErrRecoveryExhausted once the batch has two recoveries, whoever asks, with a way forward through `reset --to batch-start` or `reset --to start`.
// A terminal dead recovery below the cap earns one more when it committed work of its own (RecoveryRetry):
// HEAD descends from its start through a first-parent range holding a non-merge commit.
// An empty start, a HEAD a reset moved off the start, a range of merge-ins only and a failed git read, which is logged at Warn, all give no retry.
// begin-batch reads the same function, so its report-present remedy names recover-batch once more for such a batch.
// recover-batch's terminal envelope and status's batch entries carry the count as recoveries and the verdict as recovery_retry, and the refusal wrapping ErrRecoveryExhausted carries recovery_exhausted.
// reset --to batch-start clears the batch's count and start; reset --to start archives the whole record.
// Merriam runs the second recovery on recovery_retry: true and stops stuck on false or on recovery_exhausted.
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
// See geometry.go for what each of Geometry's fields means; this section does not restate them.
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
// A plan edited between runs normally refuses the next run with ErrFingerprintMismatch, and `--fresh` is the escape that archives the record.
// RunOptions.AutoRebaseline, which only the loom's Webster row sets, is the second: Run rebaselines the changed plan on entry, before anything else acts on the run.
// It names every changed card file's number as Rebaseline's cards, so it accepts exactly what the operator's `--card` flags accept: a follow-up card after the last begun batch, an unbegun card's edit, and an in-flight card as an amendment recorded unrendered for the next recovery to render;
// a done card's edit, a begun batch's card-set change, a failed batch with uncheckable findings and an overview change outside its Card Index are refused.
// An accepted rebaseline is saved before the batch loop starts, so a crash after it re-enters on a matching fingerprint, and the fabric receives it with the run's next sync.
// Every error of the auto path wraps ErrAutoRebaseline, and its refusals name RunDeps.ReentryStep through Rebaseline's step-keyed texts, never `lyx webster rebaseline`; the shed adapter maps them to Stuck.
// The accepted rebaseline's warning is logged once at Warn, leads RunResult.Warnings, opens a stuck outcome's reason and every error Run returns afterwards, and the plan refusals that follow it (validation, zero batches, quarry) wrap ErrAutoRebaseline too.
// `--fresh` beside the option keeps its archive path and runs no rebaseline, and a run without the option keeps ErrFingerprintMismatch.
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
// A run that outlives the verify timeout is never rerun, since a hang is not flakiness: it fails the gate at once,
// and the report names the timeout, the log path and the log tail.
// The first failed evaluation of any kind records HEAD as the pre-fix head, so every commit a fixer makes afterwards is checked at the next arrival.
// The same moment persists it as state.json's `PreFixHead`, overwriting a value an earlier shuttle run left, so a later verb can reset to it;
// a passing evaluation clears it, and `run --fresh` archives state.json with it.
// A persist failure is returned as the gate's error.
//
// After the wait, a done run whose gate did not pass ends stuck, with a reason naming the failing identities and the attempts spent, or the `Terminal` failure's own reason.
//
// # A broken config stops Master
//
// A verb whose `webster.yaml` or `batcher.yaml` is present with content that fails to parse or validate refuses in the CLI's pre-run, before its body, with `config_invalid: true`;
// `LoadConfig` marks those failures `configengine.ErrInvalid`, and the refusal changes no state, report or plan.
// Master's failure ladder has a rung for it: a refusal carrying `config_invalid` from any verb ends the run with `outcome: stuck` quoting the refusal, without calling another verb.
// A config file that exists but cannot be read refuses with no `config_invalid` and a transient way forward, so it never stops a run through that rung.
//
// # Background shells in Master's wait
//
// Master's spawn declares one awaited shell prefix, `masterAwaitedShellPrefix` (the backgrounded recovery verb of the failure ladder);
// recovery_timeout_min already bounds that verb, so shuttle's turn-end wait treats it like a fork.
// No background shell expires: a turn end waiting on a shell of either signal ends on Master's output files, the run's deadline or the liveness check, and `background_shell_wait_min` only sets when a long-running shell is logged and shown in the wait marker.
// Every shell outstanding when the run ends comes back on shuttle's `Result.EndedShells`, whatever ends the run.
// lyx does not stop such a shell.
// What ends the shell follows the run's final outcome.
// When Master's turn ends shuttle-done (a webster done, Master's own stuck or paused, the verify gate's demotion, or a mapping error after that end), shuttle removes Master's strand as the run finishes, and the session and the shell end with it.
// When the run returns a died or timeout error, Master's strand stays alive until the next `lyx webster run` reclaims it at entry.
// Run writes one best-effort `webster-background-shell` friction note once the outcome is known, on every outcome.
// For each shell the note states its label, signal and time outstanding, that lyx did not stop the shell, what ends it and the run's final outcome.
// Each shell is also a `RunResult.Warnings` entry with the same wording on every outcome that returns a `RunResult` (done, stuck and paused), after the verify-gate demotion, so the warning and the note cannot drift;
// an error outcome returns no `RunResult`, so the note alone carries it.
// summary.md's "Background shells at the run's end" section (AppendBackgroundShells) stays done-only, as does its "Plan rebaselined" section, which names an accepted auto-rebaseline beside the "Audit warnings" section so the summary a step's Done points at carries it.
//
// # Planning a reset
//
// PlanReset decides, read-only, what a reset of the task branch may do, and refuses before fabric is ever called.
// The target is one of five commits the run recorded (ResetTargets); no raw SHA is accepted.
// `start` (ResetToStart) is the run's oldest recorded start commit, or the octopus merge-base of the recorded starts when none is the oldest of all;
// `pre-fix` (ResetToPreFix) is state.json's `PreFixHead`;
// `last-batch-head` (ResetToLastBatchHead) is the last recorded batch head, the commit accept-audit picks by git ancestry.
// `batch-start` (ResetToBatchStart) takes `--batch NN` and is batch NN's recorded start commit, refused when a later batch in the partition's order recorded a start.
// `report-head` (ResetToReportHead) takes `--batch NN` and is the `head_sha` of batch NN's report, an abbreviated one resolved to the commit it names as record-batch does.
// It is accepted only for a begun, non-terminal batch whose report parses and whose head descends from or equals the batch's start, with no later batch begun and no live recovery strand of the batch in reed.
// It is the one target allowed while the run lock is held, because Master runs it inside its run between a fork's report and record-batch.
// Its bound is that those checks rule out every writer lyx can see; a Master that spawned an in-session fork out of order stays unseen.
// `--batch` is required for `report-head` and `batch-start` and refused for the other three, before any git read.
// The refusals run in order: the `--batch` pairing, run lock held (transient, except for `report-head`), no state, a merge in progress, a checked-out branch that is not the task branch,
// no recorded target, a recorded commit missing from the repository, a target that is not an ancestor of HEAD, and a dirty tracked path outside the run's own writes.
// The own paths are the tracked paths that loadRunWrites records a successful write to, Master's and every fork's; a failed write is not evidence.
// Each refusal ends in wayForwardSteps' numbered list.
// The plan carries the SHA and the own paths, and reads no force flag.
// `start` is the one start-over verb, so it does not refuse where the start cannot be moved to:
// when no batch recorded a start, a recorded start is missing from the repository, the recorded starts share no single oldest commit and no common ancestor, or the resolved start is not an ancestor of HEAD, PlanReset returns a plan with ArchiveOnly set and the Reason.
// Those four are the closed set, since each guards only a branch move the archive-only path does not make; the refusals before the target resolves, and `pre-fix` and the other targets, keep all of theirs.
//
// # The reset verb
//
// `lyx webster reset --to start|pre-fix|report-head|last-batch-head|batch-start [--batch NN]` (internal/webstercli) performs the reset PlanReset planned, so no recovery needs the denied `git reset --hard`.
// Under the state-mutation lease it plans with the pair's branch read through the fabric handle,
// resets the pair's code checkout through fabricengine's pair-checkout reset with the plan's SHA, the parent branch from the origin record and the own paths,
// clears State.PreFixHead, saves, and fabric-syncs state.json.
// It changes no other webster state, except that a reset to start ends by calling ArchiveRunAfterReset under the same lease, after the move and the save, so a following plain `run` finds no state and starts a new run.
// When the pending-findings guard refuses after the move, the move and the save stand, the record stays unarchived and re-running the reset converges, since a reset to the commit HEAD is already on moves nothing.
// On the archive-only path the verb removes the live recovery strands as a moving reset does, runs no git that mutates and archives alone; the guard there refuses only on what its clearing step can undo, because it judges against HEAD.
// The trees differ by mode: a pair's reset discards the run's tracked changes and keeps untracked files, and the archive-only path keeps everything, which the next run starts over.
// A plain `run` does the rest for the other targets, as the way-forward texts order them.
// The reset also moves the task branch on the remote to the same commit, so a later push is not rejected as diverged; `FABRIC_SKIP_PUSH=1` skips that half.
// The bound: it can discard only commits above a run-recorded commit on the task's own branch, on the checkout and on the remote, and uncommitted tracked changes to paths the run itself wrote.
// The remote update is made only when every commit it drops is reachable from the task worktree's HEAD and under a lease on the remote tip it read;
// a remote-only commit becomes reachable only through the `git merge --strategy ours` the operator runs after reading the commits the refusal lists.
// It cannot move another branch, take a raw SHA, touch the parent branch, the records side or untracked files, and it has no `--force`.
// Fabric's own refusal (ownership, dirtiness, remote divergence, an unreachable remote) is surfaced as the verb's error with fabric's reason.
// In standalone mode, with no task pair, the verb performs the planned move itself with gitrepo's keep-reset, touching no remote.
// The call sits in internal/webstercli, since webster's engine runs no mutating git.
// PlanReset skips its foreign-dirty-path refusal there (ResetDeps.Standalone), because keep is the guard: it carries an uncommitted change across and refuses, changing nothing, over one the move would overwrite, so it never discards one.
// A standalone reset therefore leaves foreign tracked changes the move does not touch in the tree, and every other PlanReset refusal still applies.
// KeepResetRefusal builds the keep refusal: git's message and, per overwritten path, `git checkout -- <path>` for a path the run wrote or commit-or-restore for any other, then the re-run.
// A standalone reset keeps every change git carries across, the run's own included, so its `uncommitted` list holds all of them.
// The envelope carries `target`, `sha`, `mutations` (the `worktree_reset` entry, and `remote_branch_updated` when the remote moved) and `partial`, false on success.
// A reset to start, and a standalone reset to any target, also carries `uncommitted` (UncommittedPaths, read after the move) and `warnings` (the findings a reset to start's archive dropped).
// A reset to start also carries `moved` and, when `moved` is false, `reason`, with no `sha`.
// A failed UncommittedPaths read leaves `uncommitted` out and adds a warning, since the reset and any archive are already done.
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
// the head, dirtiness, a merge in progress, a commit's parents, the clean-parent-merge verdict, commit existence and ancestry, ignore rules, linked worktrees and blobs.
// Geometry.Git carries it, and nil means the real repository, so no production caller sets it and the helpers in gitwrap.go stay the one place webster runs git.
// A test that asserts on webster's own records, warnings and verdicts sets a fake and spawns nothing;
// a test whose behavior is git itself (merge commits, ignore rules, blobs, the verify gate) builds a real scratch repository under the `integration` tag.
// UncommittedPaths is the one read of the task's uncommitted work: the dirty and untracked paths minus the run's own state (webster's run directory, the plan directory and the scratch directory), sorted.
// accept-audit's clean-tree evidence and the reset's and the recovery prompt's lists of uncommitted paths all read it.
//
// # The code-index seam
//
// Every plan gate and the batch delta go through Geometry.Index, a planindex.Index (internal/planindex), so this package imports neither internal/planglyph nor quarry and links no tree-sitter.
// The CLI layer sets the real index (planglyph.NewIndex); hubgeom and standalonegeom leave it empty because they sit below cliwire.
// A call that reaches a nil Index returns an error naming the missing wiring.
// A test that drives the real resolve pass sets planglyph.NewIndex, and a test that steers the batch delta sets a fake planindex.Delta.
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
