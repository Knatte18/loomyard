// Package websterengine is the domain kernel behind webster, a fork-based implementer loop.
// One long-lived Master session reads the plan's overview and the repo's rules once, and never the code.
// It then forks one implementer per execution batch in-session (Claude Code's Agent tool, subagent_type "fork").
// Each fork reads the code its cards need itself.
// Forks run sequentially, one at a time in one worktree, in an order that follows the cards' declared dependencies.
// websterengine holds no loop itself;
// the loop is the Master session driving fat `lym webster` verbs (internal/webstercli).
// This package provides only those verbs' logic plus the distillation behind them.
//
// # Plan consumption: one parser, one batcher registry
//
// webster consumes the pinned flat card-list plan format (see contracts/specs/loom-plan-spec.md).
// It reads it through internal/planparser, the sole parser of the on-disk `_lym/plan/` tree.
// No code in this package or anywhere else re-derives that grammar.
// RenderForkPrompt and RenderRecoveryPrompt take no *planparser.Plan.
// Both render a card's content from its SourcePath pointer, never from an inlined plan-level field.
// This follows the fork-context-hygiene Shared Decision.
// internal/batcher groups a plan's flat, unordered Cards list into the execution units Master forks.
// It holds a name-keyed registry of Batcher implementations.
// The registry is selected once at config-load time via batcher.yaml's `active:` key.
// The template default is the wide profile, the cost batchifier;
// an empty value resolves to the identity batcher, one card per batch.
// internal/webstercli resolves the batcher and hands it to Run via RunDeps.Batcher.
// This package loads no batcher config itself.
// Batching is a standalone step.
// A card carries no batch-membership field, and no batchifier consults a fork's judgment.
// BatchState is keyed by execution-batch number, not card number:
// a grouping batchifier puts several cards in one batch, and only under identity do the two numbers coincide.
//
// # Merriam's start base enters at partition time
//
// The batchifier prices each fork from Merriam's context at that point.
// Its start part is the start base this package computes in merriambase.go:
// the line count of what Merriam loads at start, plus a fixed system-prompt-and-tools context.
// What Merriam loads at start is the worktree's instruction files that Master's engine names.
// It also loads the Master stencil with the orchestrator PATTERN directive and the plan's 00-overview.md.
// Run computes the base where it forms a new partition (first init and `--fresh`).
// Rebaseline is told it when it batches the tail, and validate computes it for a run with no state;
// a recorded partition keeps the estimates it was formed with, so the base never regroups a run.
//
// # Batches run in the batchifier's order, asserted not derived
//
// internal/batcher owns both grouping and order: a batchifier returns its batches in plan card order.
// webster runs them in that order and never reorders them.
// sequence.go's CheckBatchOrder only asserts the order.
// It derives edges from Targets/Uses ref matching across the plan's cards:
// a Uses entry naming another card's Targets entry puts the producer before the consumer.
// Two cards writing the same Targets entry settle by declared card number.
// It refuses, wrapping ErrBatchOrder, when any edge runs from a batch to an earlier one.
// planparser.DependencyEdge refines each shared ref:
// two Extends on one ref make no edge unless both cards fence a verbatim PATTERN.md line for one entry ID.
// A cycle between batches always holds such an edge, so the same rule refuses it.
// Plan-Gate's uses-later-target refusal normally prevents such an edge;
// the assertion fires only on a plan that bypassed or predates that gate.
// The previous-digest lookup in beginbatch.go/recoverbatch.go (predecessorDigestLine) depends on the order:
// it reads the batch immediately before the target in execution order, not the batch one number lower.
//
// # The partition is recorded once per run
//
// The first init of a run (no state.json, or the --fresh re-init) forms the partition with the active batchifier.
// It refuses the partition on CheckBatchOrder's error before saving anything, and records it in State.Partition:
// each batch's card ids, profile and estimate;
// for a cost profile also the estimate's breakdown (weights, startup, read union and per-card components).
// A finished run can then be fitted against its forks' measured peaks.
// Every other verb reads that record through partition.go's ExecutionBatches.
// It maps the recorded ids onto the plan's cards and re-asserts the order,
// so a size the batchifier weighed changing under the run's own commits never regroups cards mid-run.
// A recorded id the plan lacks, or a plan card in no recorded batch, is refused with ErrPartitionMismatch.
// A state with no recorded partition runs on the identity batchifier whatever profile is active,
// so an in-flight run keeps the grouping it started under.
// Batches run in the recorded order.
// Only a first init or a rebaseline replaces the record.
//
// # Fork-return contract: OK/FAILED and a head SHA
//
// A fork's whole return contract is the minimal report shape report.go implements (Report/ParseReport/WriteReport).
// It carries `status: OK`, `status: FAILED` or `status: NEEDS_PARENT` with its `need`.
// It also carries the `head_sha` the fork left the worktree at.
// The plan's glyph names are the fork's contract.
// A fork renames a planned symbol only when its name collides with an existing symbol or breaks a language or lint rule.
// It lists each such rename under `renames`, as an `old` glyph, a `new` glyph and a `reason`.
// ParseReport refuses an entry missing `old` or `new` or naming one glyph twice; the reason is never enforced.
// The digest carries the list as `renames`, so record-batch and recover-batch both hand it to drift detection.
// Drift detection rewrites every pending card still naming an `old` glyph through the exact tier's repair path.
// The rewrite is under the amendment tier `reported`.
// A reported `new` glyph that does not resolve is a later-card drift warning, refused at that card's begin-batch.
// An older fork's `deviations` key is accepted and ignored.
// The digest's `deviations` list is the glyph scope guard's file list over the batch delta:
// the files changed outside the batch's own targets.
// Record-batch and recover-batch fill it.
// The list is informational, never a failure condition.
// The digest's `departures` list holds one `<subject>: <value>` line per `Pattern-Departure:` trailer.
// Trailers come from the batch's non-merge commits, from its start head to its recorded head, oldest first.
// Record-batch and recover-batch fill it, a recovery over the whole bracket;
// a parent merge-in's commits are never counted.
// It is informational like `deviations`,
// and the prior-batch line and the resume trail carry it to the next fork and a resumed Master.
// A fork returns FAILED on a failed card gate, card `**Verify:**` or batch gate.
// Deviation alone and a slot-busy exit never make it return FAILED.
// Plan-predicted file impact is frequently incomplete, so treating deviation as failure would make webster brittle.
// Within one fork's batch, each card lands as its own commit.
// BatchState.CardSHAs records the ordered SHA trail of those commits.
// A card that declares a `**Verify:**` line has `lym gate card-verify` run for it, which must pass before the next card.
// After the last card the fork runs the batch gate once;
// Go never runs it, and Webster's plan-level `webster` gate catches a fork that skips it.
// The fork runs every card gate step, `lym gate card-verify` call and the batch gate in the foreground.
// It follows the gate rule in its system prompt and never backgrounds a call, so its only turn end is its final report.
// The card-gate fixer paragraph is `webster-fragment-card-fixer`.
// The fork and recovery prompts render it into the body's optional `card_gate_fixer` marker right after the card gates.
// They do so only when the hub's `card` gate key carries a `fixer` and the worktree has a pair slug.
// The paragraph says a failed card gate call has already been through a cold fixer whose reply the report carries.
// The strand reads the uncommitted diff the fixer left before it commits,
// and it fixes what is left under its self-fix cap.
// With the fixer off the marker renders nothing.
// A card's own gate, which begin-batch and recover-batch render into the fork's prompt, is the hub's `card` gate call.
// The prompt carries it as one step, `lym gate test --gate card --card <card id>`, from `gateslot.Config.AgentGateCall`.
// The call carries no `--wait` since an agent runs it.
// The call itself runs the lint steps the hub's `card` key lists:
// the template's are the comment line-break lint and the PATTERN headroom lint.
// The gate call computes the card's packages and runs the guard step itself.
// Go therefore renders no package list and scans for no guard test.
// The batch gate is the hub's `batch` gate call, `lym gate test --gate batch`.
// It names every card of the batch, `--card <card id>` once per card.
// Both calls are rendered from `BeginDeps.Gate` and `RecoverDeps.Gate`.
// `webstercli` loads them from the hub's `gate.yaml`.
// begin-batch and recover-batch refuse before they mutate anything when the config gives no `card` or `batch` list.
// The refusal carries the config's way forward.
// The implementer body has a fork fix a guard step failure on a file outside the card's list under the card's commit.
// It moves a pin only when the pin itself must move.
//
// record-batch and recover-batch cross-check the consumed report's `head_sha` against the worktree's HEAD.
// The check applies one merge-only rule.
// A parent merge-in landing between a fork's commit and the report's consumption therefore cannot wedge the run.
// HEAD is accepted when it equals `head_sha` or sits above it by clean parent merges alone on the first-parent chain:
// each walked commit has exactly two parents, and its tree equals the conflict-free merge of the two;
// its second parent is reachable from the parent branch in fabric's origin record.
// An evil merge, a hand-resolved conflict, a merge of any other branch, or an octopus is refused;
// so is any merge in standalone mode (no parent branch).
// Such a merge's own content would bypass the audited delta, which ends at `head_sha`.
// The batch is recorded at the report's `head_sha` (CardSHAs and the delta's end);
// the done-checks and drift detection read the merged tree as it stands, and a warning names the walked merge SHAs.
// A `head_sha` may be an abbreviation of 4 to 39 hex digits, resolved to the one commit it names before any comparison.
// An abbreviation naming no commit or several is refused with ErrHeadSHAUnresolved.
// Any non-merge movement — a plain commit, a fast-forward onto non-merge commits — is refused,
// and so is any call made while a git merge is in progress, leaving the batch non-terminal and retryable.
//
// # every terminal batch runs the same mechanical pass
//
// A batch reaches terminal down one of two paths:
// record-batch after an ordinary in-session fork, or recover-batch after a cold recovery strand.
// Both run the identical post-batch mechanical pass (postBatchChecks):
// the card done-checks, the re-sign rebind, one shared delta and handle binding.
// It also runs the informational glyph scope guard and drift detection with its exact-tier auto-repair.
// Each rewrite among them also requires the plan-staleness re-baseline.
// The guard's result lands in the batch digest's deviation list, never in the warnings.
//
// The re-sign rebind runs right after the done-checks and before the delta.
// It carries a recorded C# re-sign arrow of the batch's cards into the plan:
// every later card naming the member by its old glyph then names the minted id.
// An arrow whose edit is not in the tree fails the batch as resign-not-done.
// When the rebind rewrote the plan, the pass restamps the plan fingerprint and re-parses the plan.
// It re-reads the batch's cards from the re-parsed plan.
// The guard and drift detection then read the re-read plan.
// The guard's targets are the cards as recorded and as re-read,
// so a re-signed member's old glyph and its minted id both count as the batch's own.
// A batch done through a recovery is as done as one done through a fork.
// Master's failure ladder treats a terminal recovery digest as "move on to the next batch".
// A recovery path that skipped the pass would leave the recovered card's plan: handles unbound for the plan's life.
// Drift detection would also miss them, since its reference index keys on the ref as the card spells it.
//
// The pass traces every plan: handle the batch's cards own, so the bind rate is countable by grep.
// Each handle gets one Info line, `webster: handle bind`.
// Its keys are batch, card, handle, outcome (`bound` or `failed`), and glyph on a bound line or reason on a failed one.
// The reasons are:
//   - no-delta-match: the batch's delta created nothing the handle expects;
//   - sibling-unmatched: the handle matched, but another handle of its card did not, so the card bound none;
//   - card-not-done: the done-checks reported findings for the handle's card;
//   - batch-not-done: the done-checks failed another card of the batch, so the pass never bound this one;
//   - delta-unavailable: quarry could not answer for the delta, which replaces whatever the empty delta computed;
//   - language-none: the plan opts out of glyphs, which replaces any other reason.
//
// A pass logs no bind line when the verb fails before binding:
// no index wired, a done-checks error, a delta error other than quarry being unavailable, or a bind error.
// The trace adds no finding, no refusal and no state.
//
// A recovery BatchState inherits the stuck fork's own StartSHA rather than re-capturing the head at recovery-spawn time:
// the pass computes its delta from that SHA, and a fork frequently commits part of its work before getting stuck.
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
// A plan describes intended change.
// Re-resolving a card whose work may already have landed reports the plan working as designed as a defect.
// begin-batch, run entry and `lym webster validate` therefore all take their scope from DispatchScope:
// begun is every card of a batch begin-batch recorded, terminal or not, and those cards are not resolved;
// forthcoming is the cards of every begun batch whose record is not terminal.
// A forthcoming card's Create targets and Rename New sides are excluded from the status check,
// so a later card that Uses one passes while the fork has landed nothing yet.
// A terminal batch's cards are never forthcoming: its done-checks proved its targets present.
// With no begun card, validate runs the whole-plan check set, approval gate included.
// begin-batch's blocking refusal is a PlanConflictError, the `plan_conflict` class.
// Its cause is `plan_order` when every blocking finding is delete-before-reference.
// Otherwise it is `plan_defect` when begun is empty and `tree_moved` when it is not.
// A delete-before-reference conflict can first appear mid-run.
// The reference may be code an earlier card wrote, which no plan gate could read.
// Every begin-batch checks all pending cards, so the first begin-batch after that code lands reports it.
// A rebaseline needs no check of its own: the next begin-batch runs the same one before any fork.
//
// The bound: drift in a begun, non-terminal card's own targets is reported by none of the three sites.
// That card was validated at its first begin-batch, and its bytes stay pinned by its recorded CardHashes.
// Record-batch and recover-batch still run its done-checks and drift detection.
// A forthcoming target that never lands keeps the run from outcome done.
// The run-exit check requires a terminal done record for every batch.
//
// loom's plan gate takes a narrower scope from DoneCards:
// only the cards of a batch recorded terminal with status done, read from the batch records alone, with no batchifier.
// A run moved back to the plan review keeps its run record, and those cards are history whose work is in the tree.
// The gate reads EditedDoneCards beside DoneCards:
// the ids of those cards whose file no longer hashes to the CardHashes entry their batch recorded at begin.
// It reads the batch records and the card bytes alone.
//
// # the plan-staleness guard re-baselines at the rewrite, not at the return
//
// begin-batch compares the plan directory's fingerprint against the one state.json records,
// so a plan edited from outside the run between two batches is refused.
// The run itself also rewrites the plan:
// handle canonicalization inside begin-batch's ValidateDispatch;
// handle binding and exact-tier drift repair inside record-batch.
// The guard cannot tell those apart from a foreign edit.
// Every sanctioned rewrite is therefore re-baselined immediately after the call that performed it.
// The re-baseline comes before any finding or error that call also reports is examined;
// the caller persists that re-baseline even when the call then fails.
//
// Both halves are load-bearing.
// A call can rewrite the plan and also block.
// Re-baselining after the refusals would then leave state.json describing the pre-rewrite bytes.
// That happens routinely.
// BindHandles' substitutions come from the delta's created symbols;
// its bind-count-mismatch comes from a card that matched nothing.
// DetectDrift's repair and its plan-references-deleted-symbol finding likewise read different parts of one delta.
// Persisting only on success would drop the re-baseline for the same reason.
// Either way every later begin-batch would fail ErrFingerprintMismatch on webster's own edit.
// The advised `--fresh` recourse would then refuse the run over the cards that had already landed.
// Only a hand-edit of state.json would recover it.
// Comparing against the fingerprint read immediately before the call keeps the persist narrow:
// an unchanged fingerprint writes nothing, so a genuine foreign edit still fails.
//
// The restamp also moves the recorded CardHashes of every begun card the rewrite changed,
// so webster's own rewrite of a begun card is not later refused as an operator edit that `rebaseline` cannot accept.
// It moves a hash only when the recorded hash equals the pre-rewrite plan file hash for that card's file;
// a card an earlier untracked rewrite already moved keeps its old hash and stays refused.
// Each restamp site runs after a foreign-edit check passed in the same call,
// so an edit on disk when the call starts is refused, never adopted.
// The check precedes the rewrite and is not atomic with the restamp,
// so an edit landing between the two in one call is adopted with the rewrite.
// `rebaseline` accepts an operator's edit, so it never moves a begun card's hash.
// The exception is a card the operator names with --card whose batch is in flight or terminal failed, dead or stuck.
//
// A foreign edit an operator means to keep has its own way forward:
// `lym webster rebaseline --card NN` (Rebaseline) accepts the on-disk plan as the new baseline.
// It drops no batch record, provided each recorded batch still holds exactly the cards its record names.
// With a recorded partition, Rebaseline keeps every batch up to the last begun one as recorded.
// The profile and estimate are kept; a plan whose first cards are not those batches' recorded cards is refused.
// It batches every plan card after the kept prefix with the active batchifier.
// It asserts the order over kept batches and tail together, and only then replaces State.Partition.
// Cards added, removed or reordered after the last begun batch are accepted and regrouped.
// A state without a partition regroups with the identity batchifier and records none.
// The operator names every card the edit changed with --card:
// State.PlanFileHashes records a hash of every plan file, and a changed card file whose number is not named is refused.
// A named card of a batch that is terminal failed, dead or stuck is accepted even though that batch was begun:
// Rebaseline restamps that card's CardHashes entry and keeps the rest of the batch record,
// so a one-card fix needs no reset and no fresh run.
// A named card of an in-flight batch, begun and not terminal, is accepted the same way.
// It is also recorded in the batch's AmendedCards (AmendedCard) with Rendered false;
// a card already there has its mark set back to false.
// RebaselineResult.CardsAmended reports the accepted ones as `cards_amended`.
// The fork or recovery strand running then keeps working on the old text and nothing is stopped;
// recovery re-runs the batch on the edited cards.
// That re-run is forced.
// Suppose record-batch is recording a fork's report, or recover-batch a recovery's terminal digest.
// If the batch holds an AmendedCards entry with Rendered false, it first runs every normal check.
// It is then recorded failed through failBatch, whatever the report's status.
// failBatch adds a reason per unrendered entry to any failure and sets BatchFailedError.CardAmended;
// both verbs' batch_failed envelopes carry it as `card_amended`.
// The reason is recoverable, never an Uncheckable entry, so recover-batch never answers needs_fresh for it.
// Every recovery spawn lists each AmendedCards entry of the record it replaces in the prompt's failure digest.
// The prompt tells the strand to re-read the card and bring the committed work in line with it.
// The spawn carries the entries onto the fresh record with Rendered true.
// That recovery's own report is therefore not forced failed.
// A recovery recorded done clears the entries;
// one that ends stuck, dead or failed keeps them, so the next spawn renders them again.
// Each amendment thus forces at most one failed record and so at most one extra recover-batch;
// Master's template re-runs recover-batch on a `card_amended` refusal even after a failed recovery.
// A done batch and a failed batch whose record lists Uncheckable entries still refuse, each with its own way forward.
// The done batch's way forward is the follow-up card landing below;
// the failed batch's is to restore the card or take the reset route.
//
// 00-overview.md carries the plan's integration verify, and only its Card Index section is rebaselined.
// State.PlanOverviewFrameHash is the hash of planparser.OverviewWithoutCardIndex over the overview.
// It is recorded wherever State.PlanFileHashes is.
// Rebaseline accepts a changed overview if its frame hash still equals the run's recorded overview frame.
// That confines the change to the Card Index.
// The index change is then held to the card-set rule, which compares NN-slug ids:
// only cards after the last begun batch can be added, removed or reordered;
// a begun card's index line keeps its number and slug.
// What passes besides is a reworded one-line intent of a begun card in a later Master render;
// that card's file stays pinned by its recorded hash.
// A state without the frame hash takes the frame from the stored baseline copy of the overview until the first restamp.
// A run with neither refuses any overview change.
// A change outside the Card Index refuses naming the overview outside its Card Index.
// An empty recorded frame has one of three causes:
// no overview hash recorded, the baseline copy absent, or a copy without a parseable Card Index.
// Rebaseline logs the cause once at Info, with the copy's path where there is one, and the no-frame refusal names both.
// Every refusal and transient of Rebaseline is keyed on RebaselineDeps.Step.
// An empty step keeps the manual verbs, and a set step, the loom's Webster row re-step, replaces them.
// No text on that path then tells an agent to run a verb by hand.
// A done batch's card and a changed frame refuse with the follow-up card landing as their way forward:
// add a follow-up card that carries the decision, with its Card Index line, after the last begun batch;
// then run `lym webster rebaseline --card NN` naming it.
// The fingerprint refusals in begin-batch and run name the landing too.
// For an index-only change they name rebaseline with the added cards' flags.
// An operator decision taken mid-Webster reaches the plan in this order:
// `lym loom decision add` for the record;
// the card edit (or a follow-up card with its index line);
// then `lym webster rebaseline --card NN`.
// The verb's notice reaches Master with the decision's text inline, since Master reads no decision record;
// Master hands it on through the forks it spawns afterwards.
// Their implementer and verify-fix bodies let a parent decision win over a card.
// With no batch left, Master tells its parent the decision reaches the code only through a gate fixer or plan amendment.
// The notice reaches only a Master already running.
// Run therefore renders every `Added after Discussion` entry of the decision record verbatim into the master prompt.
// The entries land in the `parent_decisions` marker (`none` without one):
// a Master that a re-step starts after its parent answered an earlier stop is told that answer, and never asks it again.
// No refusal or prompt text names a loom verb.
//
// validate, record-batch and recovery refuse a plan that changed before their own rewrites, instead of adopting it:
// each checks the plan against the recorded fingerprint (PlanEditError) before its first rewrite,
// and record-batch and recovery also check that no card of the batch changed since it was begun.
// Their restamps exist to adopt webster's own rewrites, so a difference seen at that point is someone else's edit.
// A refusal there mutates nothing;
// validate still lints but skips its restamp.
// Every plan-hash restamp and run initialisation also stores the hashed content under `<WebsterDir>/plan-baseline/`:
// one file per content, named by its SHA-256.
// The fabric sync carries the store with state.json.
// `lym webster restore-plan` (RestorePlan) restores each plan file that differs from the recorded plan from the store.
// It also removes a plan file the run never recorded;
// it never touches state.json, and refuses with ErrPlanBaselineMissing, changing nothing, when a copy is missing.
// Each batch record carries the card set it was begun with (BatchState.Cards).
// The check that no card of the batch changed since it was begun compares against that set.
// It also carries each card file's content hash (BatchState.CardHashes),
// so a begun card whose body changed while its file name stayed is refused too, not only a changed id;
// a record with no hashes compares ids only.
//
// # audit findings: correctness fails the batch, policy warns once
//
// The fork and parent audits classify each finding (ClassifyViolation) as correctness or policy.
// A correctness finding means the delta or the run's own state may be wrong.
// A fork writing one of Master's two contract files is a correctness finding (fork-state-write);
// so is a fork writing anything under the plan directory, or anything under webster's run directory but its own report.
// A parent write is a correctness finding when it lands in one of these places:
// under the run's `_lym` directory;
// in the worktree's tracked (not git-ignored) content;
// under the run's `.lym` state directory (webster's locks, another module's lock or pause flag, a reed launch script);
// in another worktree of the task repository.
// A fabric reference is correctness too unless its command is read-only:
// an agent never touches the fabric repo for a write,
// and the command can rewrite run state the cards' verify commands cannot detect.
// The read-only classifier is injected: RecordDeps, RunDeps and RecoverDeps carry it as ReadOnly.
// It sees static shape only, so a command behind `bash -c`, a variable or a substitution stays correctness.
// A nil classifier accepts nothing.
// The classifier splits a `lym`, `git` or `sed` segment into shell words, so a quoted word is its dequoted text.
// A quoted writing flag is still rejected.
// An unquoted glob character is rejected, except after the `--` of `git log`, `show`, `diff` and `status`.
// There every word is a pathspec.
// A leading tilde is text, so a home path is admitted.
// The stderr redirections `2>&1` and `2>/dev/null` are dropped before the redirection scan;
// every other redirection still fails it.
// A `sed` segment is a read when it is made of these parts in order:
// the quiet flag (`-n`, `--quiet` or `--silent`);
// print commands joined by `;`, each with a line-number or `/RE/` address or a range of two;
// file words.
// A trailing `--json` is a help form of a `lym` segment, since the global flag raises help before the command runs.
// A policy finding breaks a steering rule without touching correctness.
// Examples are a named spawn, a nested agent call and a read-only fabric reference.
// Each finding carries a stable identity: its Key, prefixed by the session id for a parent finding.
// The ledger in state.json dispositions each finding once per run.
// The whole-session parent audit repeats earlier findings on every record-batch.
// The ledger keeps the repeats from being re-judged.
// A policy finding is recorded as a warning on the batch once the evidence holds.
// The evidence is a re-run of the cards' verify commands (rerunCardVerifies), in-process.
// It runs on an OK report that carries policy findings.
// A failing re-run makes those findings correctness for the batch and fails it.
// A rerun command never sees gateslot.PrebuiltLymEnv, slotted or not.
// Its own `go test` therefore builds lym once per test binary.
// A correctness finding fails the batch on its merits instead of wedging it.
// The batch goes terminal with digest status failed and its reasons.
// The report is archived, and record-batch returns *BatchFailedError naming `lym webster recover-batch`.
// recover-batch proceeds from a failed batch and hands its strand the failure digest.
// The exception is a batch failed on a correctness finding the recovery check cannot verify.
// Such a finding has no path or a path outside the tracked tree and the plan directory.
// The batch's record lists it as Uncheckable.
// recover-batch refuses that batch with ErrRecoveryNeedsFresh before spawning anything.
// The way forward is `lym webster reset --to start`, which archives the run record, and then `lym webster run`.
// `run --fresh` drops such a batch under the same HEAD and path rules as a pending finding.
// A pathless entry is recognised by its class prefix and never resolved as a path.
// One narrow exception keeps the batches before it: AcceptBatchFabricReference, `lym webster accept-audit --batch NN`.
// It applies when every Uncheckable entry is a pathless fabric reference and the batch recorded a start commit.
// The worktree must also be clean apart from the run's own state.
// The explicit call then clears the entries and records each as a batch audit warning, and recover-batch proceeds.
// The refusal names that route only for a record the evidence check admits.
// The check runs without mutating anything.
// When the evidence holds, the refusal names `accept-audit --batch NN` then `recover-batch NN`.
// Otherwise it names the check's own reason and steps, which end by re-running both, beside the reset-to-start route.
// The explicit call admits a batch by one of two routes.
// In the first, HEAD is the batch's start commit, so the batch changed nothing.
// A batch that committed qualifies after `lym webster reset --to batch-start --batch NN`.
// In the second, the start is an ancestor of HEAD, and the batch's commits are kept.
// Every entry's recorded command must be read-only (`fabricengine.IsReadOnlyCommand`).
// An entry that is not read-only, or records no command, refuses the whole call.
// The refusal names the reset-to-start and fresh-run steps.
// The evidence shows that the start still lies in HEAD's history and that nothing is uncommitted.
// No command the classifier accepts writes tracked content, a ref, config or a remote.
// `git status` may refresh the index's stat cache, which changes no content.
// The evidence does not inspect the batch's commits.
// It cannot show the fabric repo's own state, and the caller vouches for that by running the verb.
// recover-batch runs the same call in line before its refusal (RecoverSpawnOrAttach, with RecoverDeps.ReadOnly).
// It does so when every Uncheckable entry is a pathless fabric reference with a command the classifier accepts.
// On success the warnings are recorded on the batch, each naming `recover-batch`, and the recovery spawns.
// An evidence failure returns ErrAuditNotAcceptable.
// recover-batch's envelope carries it as `audit_not_acceptable`.
// Its way forward names `recover-batch` for the re-run.
// The refusal stands for a command the classifier rejects, a nil classifier or an entry of another kind.
// It also stands for a fabric reference recorded with a path.
// record-batch on a batch already terminal as a fork batch first audits the fork transcripts it has not consumed.
// The audit runs once and without the settle wait.
// An undispositioned correctness finding replaces the terminal record with a failed one.
// An example is a fork that marked its own batch done by writing state.json.
// Otherwise the "already terminal" refusal stands.
// It audits nothing while a later fork batch of the session is open or the verify-gate report exists.
// An unseen transcript may then be that fork's.
// A batch's own bracket transcripts are the ones record-batch attributed to it since its begin-batch.
// They count toward attribution when one of their writes is the batch's report.
// Every one of them is re-audited in full on each call.
// A fork stopped and resumed across a no-report call is thus attributed on the next.
// begin-batch and recover-batch open an empty bracket list, so a transcript of an earlier bracket never counts.
// A report that cannot be attributed to a begun batch, or to any counted fork transcript, is archived.
// It is returned as *ReportArchivedError naming `lym webster begin-batch`, which re-drives the batch.
// The post-batch done-checks fail the batch the same way when a card's own declared work is missing.
// Drift that concerns only a later card is recorded as a warning and does not block this batch.
// One kind of later-card drift is first handed back to the fork, while the fork still holds the batch's context.
// It is a symbol the delta deleted that a later card targets or uses (planindex.Delta.RemovedTargets).
// The symbol has no exact rename and no rename in the report.
// No Delete target or Rename old side on the batch's own cards declares it either.
// On an OK report, record-batch checks it after the shared delta and before handle binding.
// The only plan rewrite it can follow is the re-sign rebind.
// It returns *LaterTargetRemovedError, the CLI's `later_target_removed` flag.
// The batch stays open with its report in place and BatchState.LaterTargetsReturned set.
// Master sends the refusal to the fork, which restores the name or reports the rename and rewrites its report.
// record-batch is then called again.
// The hand-back comes once per bracket.
// The next record-batch records the batch with the later-card warning even when the fork could not be reached.
// recover-batch never hands back, since a recovery strand has ended when its report is read.
// A delete-not-done finding gets one more check, planindex.Index.LaterDeleteReferences.
// It runs over the batch's own cards and the cards of every batch with no record.
// An unbegun later card's Edit code may still reference the target.
// The failure's reasons then name that card and the reference.
// The BatchFailedError's way forward is the plan edit: move the delete after that card, rebaseline, then recover-batch.
// With uncheckable entries in the record the way forward is the reset-to-start steps instead.
// recover-batch would repeat the same failure.
// Only a reference resolved to the deleted member counts, which is the blocking finding;
// a method matched by name alone is informational, adds no reason and changes no way forward.
// A glyph-rejected finding is a card target quarry rejects before resolving it.
// Its way forward is the same plan edit, with the card's target amended instead of the delete moved.
// Recovery re-runs the same check on the same plan.
// A create-not-done or rename-not-done finding offers that card amendment beside recover-batch.
// A signature-mismatch finding offers the amendment alone.
// So does a create-not-done or rename-not-done finding a recovery has already failed.
// recover-batch thus never names itself as the way past its own repeat.
// The delete-reference, glyph-rejected and create ways forward take the exhausted-batch steps.
// They do so once the batch has used its counted recoveries and holds no amendment a recovery has yet to render.
// PersistRecoveryTerminal fails a recovered batch the same way.
// recover-batch runs the same check before spawning.
// While a blocking reference fires, recover-batch refuses with ErrRecoveryDeleteReferenced.
// The CLI maps it to the `batch_failed` flag.
// At run exit the audit cross-check drops dispositioned findings.
// It records the remaining policy findings as run-level warnings, appended to summary.md under "Audit warnings".
// It demotes Master's outcome done to stuck for an undispositioned correctness finding.
// A correctness finding stays pending in state.json until `lym webster accept-audit` clears it.
// Run entry refuses with ErrPendingAuditFindings meanwhile.
// accept-audit needs evidence.
// It checks every suspect path against the last batch head, and a plan file against the run's recorded plan hashes.
// It refuses with ErrAuditNotAcceptable while any path differs, cannot be checked, or a finding names no path.
// The evidence covers HEAD too.
// accept-audit also refuses while HEAD carries a commit past the last batch head other than a clean parent merge.
// A clean parent merge is reconciled, and the paths are checked against the resulting HEAD.
// The last two clear only through `lym webster reset --to start` and then `lym webster run`.
// The run-exit stuck reason and the pending-findings refusal name each finding once (findingsClause).
// An uncheckable path carries its reason (uncheckableReason).
// Both end in one ordered list: the restores, then `lym webster accept-audit`, then one re-entry step.
// The re-entry step is RunDeps.ReentryStep, `lym webster run` when empty.
// The reset route ends in that same re-entry step.
// The reset archives the run record, so a plain run starts over.
// A suspect path that is one of the run's two contract files, outcome.yaml or summary.md, is the exception.
// It lies outside the tracked tree and has no blob to compare.
// contractFileStatus clears it when the file is absent.
// It also clears it when the latest successful Master write to it (from RunWrites) is later than every other write.
// The other writes are every fork write and every recovery or verify-fixer write to it.
// An uncleared path is one a fork, recovery or verify-fixer session wrote last, and the refusal names which.
// A Master write whose result failed is not evidence, and an acknowledgement never clears it.
// Every other path under `_lym`, `.lym` or the scratch directory stays uncheckable.
// These sites consult contractFileStatus before checkSuspectPaths:
// AcceptPendingAudit, RecoverSpawnOrAttach, pendingPathsWayForward and `run --fresh`.
// A cleared contract path is resolved, so at run exit `lym webster accept-audit` clears the finding directly;
// an uncleared one refuses with `rm <path>` as the first step, then the verb to re-run.
// Absence clears the finding only.
// accept-audit on an absent file adds a `next` step to its envelope.
// The run still needs a Master that writes both files.
// `run --fresh` drops pending findings, with one warning per finding, even on an unchanged plan.
// It refuses with ErrPendingAuditFindings and archives nothing in two cases.
// In one, a pending suspect path outside the plan still differs from the run's start commit.
// In the other, HEAD is not the start commit.
// It also refuses while a pending plan path differs from the plan the run recorded.
// That applies when `lym webster restore-plan` can undo the difference.
// It can when the recorded copy is stored or the file was never recorded.
// A differing plan path whose recorded copy is missing is dropped with the archived state, and its warning says so.
// Every way forward for a differing plan path names restore-plan or `rebaseline --card`, never a git checkout.
// The archive `run --fresh` performs is archiveRunInPlace.
// It renames state.json and the reports dir with a stamp and clears the rendered prompts.
// ArchiveRunAfterReset is the reset's entry to that archive.
// It first runs the pending-findings checks (checkPendingFindings).
// They are judged against the worktree's HEAD in place of the recorded start.
// After a moving reset HEAD is the start, so the checks are the ones `run --fresh` runs there.
// On an archive-only reset the recorded start may be missing or not an ancestor of HEAD.
// The tree the next run starts from is HEAD's.
// A HEAD-relative guard therefore never refuses toward a verb that cannot clear the state.
// The guard refuses only on an uncleared contract file.
// It also refuses on a plan path differing from the recorded plan and on a suspect path differing from HEAD.
// Each refusal names its clearing step and then the reset.
// The guard drops every other pending finding with a warning.
// A branch rewritten under the run passes, and the dropped findings stay readable in the archived state.
//
// Every refusal this package returns ends in a trailing `way forward:` clause.
// contracts/specs/refusal-spec.md defines that clause.
//
// # bracket verbs, not spawn/poll
//
// Because the fork runs inside Master's own session, there is nothing for Go to spawn in the normal path.
// Go provides thin bracket verbs Master calls around each fork.
// begin-batch runs immediately before forking.
// It checks the fingerprint, records the batch's start-SHA, and renders and writes the fork prompt.
// record-batch runs once the fork has delivered.
// It audits the fork incrementally, parses the batch report, distills the digest and updates state.
// The Agent-tool fork is a backgrounded agent: the fork call returns immediately, before the batch is done.
// Master therefore ends its turn after spawning it.
// It calls record-batch when the fork's completion notification starts its next turn.
// That turn end does not end the run.
// shuttle reads a turn that ends with a background agent still running as EventWaiting.
// Its wait loop treats that as still running, so Master spends no turns while a fork works.
// await-batch is a verb an operator can call: a stateless, bounded wait on a batch's report path.
// Master's loop does not use it.
// Go's gates only run when Master calls them, and the fork itself is Master's own un-gateable act,
// so enforcement has two layers: template discipline and fail-loud detection after the fact.
// The master template pins the begin -> fork -> notification -> record sequence, and a property test checks it.
// record-batch archives the report and refuses when a batch has no begin-batch record.
// The refusal names begin-batch as the way forward.
// The audit also cross-checks the fork-transcript count against the begun-batch count.
// This is a steering guard, not a security boundary, the same class as burler's nested-Agent ban.
//
// A third, deterministic layer closes the fork-loop deadlock.
// A fork inherits Master's whole prompt, the batch loop included.
// A fork that drives that loop itself livelocks the run.
// It does so by calling the bracket verbs or waiting for the report it is meant to write.
// A fork-context PreToolUse(Bash) hook in the claudeengine seam refuses any `lym webster` command inside a fork.
// Master's own verb calls pass.
// buildSettings installs it only for the fork-authorized spec that enables forks.
// The hook payload carries a top-level agent_id only for a subagent, never for a top-level Master call;
// the fork's transcript_path is not distinguishable, so agent_id is the signal the hook relies on.
// A cold recovery strand is a separate, non-fork-authorized session and never sees the hook.
//
// # one model per run
//
// Forks inherit Master's current model, so webster has no per-fork model override.
// It has no implementer or oversized-implementer fork role either.
// RoleMaster and RoleRecovery are its only roles.
// The cold strands, recovery and verify-gate fixer alike, run at RoleRecovery.
// run launches Master with RoleMaster's model, and begin-batch never reads or changes it;
// a Master pane an operator moves with /model stays there for the rest of the run.
//
// # cold strands are the only real model escalation
//
// Webster spawns a separate process in two places: recover-batch and the cold verify-gate fixer.
// The fixer has its own section below.
// recover-batch is a blocking, re-entrant verb.
// It spawns a fresh implementer as its own shuttle/reed strand at the recovery role.
// It does so when a fork reports stuck or writes no report.
// The recovery prompt is the full cold-start prompt (RenderRecoveryPrompt), not a fork's thin RenderForkPrompt.
// The recovery strand inherits no session context (see the fork-context-hygiene Shared Decision).
// The master and the recovery strand both load `scribe:prose`, `scribe:code-quality` and `scribe:testing`.
// They load them through their spawn spec's skills (`roleSkills`).
// The hub's language skills follow them.
// `RunDeps.LanguageSkills` and `RecoverDeps.LanguageSkills` carry them, loaded by their wiring.
// The fork and recovery prompts render the hub's tiers and gate lists.
// `BeginDeps.GateTiers` and `RecoverDeps.GateTiers` carry the rendered `gate.yaml` lines, loaded by their wiring.
// The prompts render `none` when the hub declares no tier.
// They also fill the implementer body's required `rename_tools` marker.
// `BeginDeps.RenameTools` and `RecoverDeps.RenameTools` supply it from the hub's `language.yaml`.
// A standalone run leaves them empty, and both prompts render `none` for an empty value.
// The prompts render no plan `## verify:` command, an in-flight plan's included.
// Their opening prompts render the parent directive (internal/parentdirective) from Geometry.ParentName.
// hubgeom.WebsterGeometry fills it from the worktree's origin record.
// The master and recovery prompts also render the task's declared PATTERN changes.
// They read them from Geometry.DecisionRecordPath; standalone mode leaves it empty and renders none.
// Forks get no skills and no directive: they inherit the master's context.
// The recovery prompt also carries the worktree's uncommitted paths in the optional uncommitted_paths marker.
// The block reads `none` on a clean tree and is taken from UncommittedPaths when the strand is spawned.
// Each path is grouped by the run's write evidence (loadRunWrites).
// That evidence covers Master, every fork, every recovery session and every cold verify-gate fixer session.
// "Written by this run" is the earlier attempt's unfinished work.
// The strand keeps it as its starting point after checking it against the card.
// "Not written by this run" includes every path the evidence cannot attribute.
// The strand leaves those paths untouched and never stages or commits them.
// The call that spawns the recovery strand first waits for its provider to come up.
// That wait is normally seconds and is bounded by startup_timeout_s.
// Every call then blocks for RecoveryWaitBudget, which is recovery_timeout_min plus one poll tick.
// It returns a terminal digest.
// The budget outlasts the timeout measured from spawn, so a strand that never reports classifies dead on its timeout.
// The call then returns.
// A re-entrant call finds the strand already recorded in state and skips straight to the wait.
// Every terminal digest (done, stuck or dead) stops the recovery strand through shuttle's stop verb.
// It does so even when the call then refuses on a merge in progress or a report head mismatch.
// Only a done digest also removes the run dir, which stuck and dead keep for diagnosis.
// Every reclaim of a leftover strand goes through that verb.
// The verb records the stop on the strand's run before it removes a live strand.
// A stop that cannot be recorded therefore fails the reclaim.
// `lym webster reset --to start` likewise stops every recovery strand the state records before it resets,
// so no recovery agent keeps writing into the tree the reset moves;
// `--to pre-fix` removes none.
// Merriam runs the call as a backgrounded Bash command, ends its turn and acts on the completion notification.
// Only an operator's shorter --wait can return a running snapshot.
// A recovery strand's turn end with no report classifies dead/asking only when it is the strand's newest turn signal.
// Nothing may keep the strand waiting (gatefixer's TurnEndedAfter).
// A later turn start is the strand working again.
// A plain turn end counts only after it has stood a few seconds.
// A background shell that finished shortly before the turn ended starts the next turn through its notification.
// A turn end left waiting on any task, a shell of either signal or a fork, never counts.
// recovery_timeout_min therefore bounds that strand, and a report present before it classifies done.
// This mirrors classify.go's dead/timeout/stuck classification.
//
// A batch's recovery spawns are counted in BatchState.Recoveries.
// BatchState.RecoverySessions records every recovery strand's session id in spawn order.
// A session's engine is recorded beside its session id, because a shuttle run record does not outlive its run.
// State.MasterEngine sits beside MasterSessionID and BatchState.Engine beside SessionID.
// BatchState.RecoveryEngines, keyed by session id, sits beside RecoverySessions.
// State without these keys reads them as empty.
// Webster is told an AuditEngines lookup, which the engine set satisfies.
// It reads each session through the engine its own entry records.
// A change to shuttle.yaml's engine therefore never alters how a recorded session is read.
// The run-write history and the reset's own paths audit each recorded session through its engine.
// record-batch audits a batch's session through that batch's Engine.
// recover-batch reads the live recovery strand's turn ends through the engine its last RecoveryEngines entry records.
// An engine the set does not hold is refused naming the session and the engine.
// An entry with no engine reads through the default engine, and is refused when that default cannot audit forks.
// The bound is the single registered engine.
// An engine-less entry read through a default that did not run it either fails that engine's audit or audits nothing.
// Each spawn checks its role's engine first.
// Run checks the master and recovery roles as an entry gate before it takes the state lock.
// A refusal therefore stops no strand and writes nothing.
// RecoverSpawnOrAttach checks the recovery role again once it has found no live recovery to attach to.
// The key can change mid-run.
// A role whose engine is not held or cannot audit forks is refused naming its webster.yaml key;
// the attach, await and terminal paths check no key.
// The default engine is checked only when a role's choice resolves to it.
// The spawn records HEAD as RecoveryStartSHA, read before anything is stopped or started.
// A HEAD that cannot be read starts and counts nothing.
// A re-begin and a rebaseline keep the count.
// A spawn whose prompt renders an amendment no earlier spawn rendered does not count.
// An amendment thus forces at most one uncounted re-run.
// A counted spawn is refused with ErrRecoveryExhausted once the batch has two recoveries, whoever asks.
// The way forward goes through `reset --to batch-start` or `reset --to start`.
// A terminal dead recovery at no counted recovery earns one more.
// The batch's first counted recovery is still owed.
// A batch-start reset leaves that state (RecoveryRetry).
// Below the cap it otherwise earns one more only when it committed work of its own:
// HEAD descends from its start through a first-parent range holding a non-merge commit.
// An empty start, a HEAD a reset moved off the start and a range of merge-ins only all give no retry.
// So does a failed git read, which is logged at Warn.
// begin-batch reads the same function, so its report-present remedy names recover-batch once more for such a batch.
// Its refusal over a batch at the cap wraps ErrRecoveryExhausted and carries recovery_exhausted.
// recover-batch's terminal envelope and status's batch entries carry the count as recoveries.
// They carry the verdict as recovery_retry.
// reset --to batch-start clears the batch's count and start, so the reset batch reads recovery_retry: true;
// reset --to start archives the whole record.
// Merriam runs another recovery on recovery_retry: true and stops stuck on false or on recovery_exhausted.
// A dead or stuck envelope also carries recovery_session, the session of the strand the verdict is about.
// Its message names that session.
// Two consecutive refusals over different strands, such as either side of a batch-start reset, are thus told apart.
// Run refuses on entry, before it spawns Master, while a recorded batch is at a dead end.
// A dead end is a batch that is terminal and not done with its recoveries spent, or dead with no retry.
// In either case the batch holds no amendment a spawn has not rendered.
// A git read that fails while judging a dead recovery refuses with a plain error, never as a dead end.
// The error names the re-run and the batch-start reset.
// The refusal wraps ErrBatchDeadEnd, and its text depends only on the batch's record and the tree.
// Its way forward is the batch-start reset then the re-entry step, or the reset to start.
// A re-entry of a run halted on the dead end therefore halts again with the same text and spends no Master session.
//
// # A batch waiting on its parent
//
// A step only the run's parent can take is no card failure, since no fix and no recovery strand can take it.
// Examples are a hub setting in `gate.yaml`, a board write, and a mutating `lym fabric` verb refused to a strand.
// The implementer body tells a fork or recovery strand that meets one to stop and report `status: NEEDS_PARENT`.
// The report carries a one-line `need` naming the step.
// record-batch records such a report as a waiting batch (waiting.go).
// recover-batch does so through PersistRecoveryTerminal.
// `Terminal` stays false, `Status` is `waiting`, and the digest carries the need.
// The report is archived, and neither the post-batch pass nor the suspect check runs.
// The wait spends no recovery.
// The recovery count and start commit are unchanged.
// recover-batch never attaches to a waiting record, never refuses it as exhausted and counts no spawn that resumes it.
// Merriam tells its parent the need by message and ends its turn.
// Once the parent says the step is taken, Merriam resumes the batch where it stood.
// It resumes through begin-batch's re-begin for a fork batch and through recover-batch for a recovery batch.
// record-batch on a waiting batch refuses naming that resume verb.
// The progress trail lists a waiting batch with its need and the resume verb, so a fresh Master re-drives it.
// recover-batch exits 0 on a waiting digest.
//
// The waiting rung and a decision with no batch left are the only times Merriam messages its parent.
// Every other dead end of the run is an `outcome: stuck` with no message.
// The stop thus reaches the parent on one channel.
// The loom driver hands a blocked Webster stop to the arbiter before any tackler.
// Only a question the arbiter hands on reaches the parent.
//
// # digest persistence carries batch context forward
//
// webster persists its distilled Digest into BatchState.Digest at terminal classification.
// begin-batch(N+1) needs the preceding batch's digest to render into the next fork's prompt,
// and a crash-resumed Master needs the same digests to reconstruct {{.progress}}.
// Nothing downstream re-Distills a report to reconstruct a digest, since the report's originating HEAD may have moved.
//
// # engine/cli split: webster is fabric-blind
//
// websterengine is _lym- and fabric-blind: every function here takes an already-resolved directory string.
// It declares its own `_lym/webster` and `.lym/webster` subpaths through the accessors in state.go.
// Those are Dir, ReportsDir, ScratchDir and PromptsDir.
// The caller always supplies the anchor root they are joined onto, per the Cwd Resolution Invariant.
// No production file in this package imports internal/fabricengine.
// The engine declares RefMatcher, an interface the caller supplies, for the fork-audit's fabric-reference class.
// internal/lymcwd is likewise absent from this package's production imports.
// Every path this package consumes arrives already resolved, through a Geometry value (geometry.go).
// None arrives through a *lymcwd.Location.
// internal/hubgeom's WebsterGeometry and internal/standalonegeom are the tellers that build a Geometry.
// One builds it from a resolved hub *lymcwd.Location, the other from a standalone state tree.
// The dependency direction between them and this package is one-way.
// They import websterengine to build the struct it declares, and websterengine never imports either back.
// See geometry.go for what each of Geometry's fields means.
// Every fabric commit of a webster artifact happens in internal/webstercli, never here.
// The artifacts are state.json, a batch report, outcome.yaml and summary.md.
// The commits fall at deterministic boundary points.
// Those are begin-batch, record-batch, recover-batch (spawn and terminal), and run's exit backstop.
// Neither Master nor its forks touch fabric or git for webster's own bookkeeping.
// The Fabric Git Invariant bans agents from driving fabric git, not the Go verbs an agent invokes.
//
// # crash/resume: fresh Master re-drives the first unreported batch
//
// Forks die with Master (same process), so a normal batch never leaves an orphaned in-flight implementer;
// only Master's own strand and a possible recovery strand ever need reclaiming.
// Resuming after a crash is re-running `lym webster run`:
// entry-time reclaim stops any live recorded strand, and a fresh Master (never a provider resume) is spawned.
// That Master is hydrated from the on-disk register.
// The register is the reports directory plus state.json, rendered into the progress context.
// It re-drives the first batch with no terminal record.
// Every card an implementer commits survives independently of Master's fate;
// only reports and state are fabric-committed per batch, so nothing already recorded is lost.
//
// One crash window needs a distinct resume move.
// A crash between a fork's report and record-batch leaves the re-driven batch with a report on disk.
// begin-batch refuses to overwrite it when state.json records the batch.
// The resumed Master consumes that report with record-batch instead.
// For a recovery batch it consumes the report with recover-batch's attach path.
// The fork audit keys on the bracket-opening session recorded in the batch state, never the current Master session,
// so the crashed session's fork transcript, still on disk, is found and policy-checked as a late record would be.
// This crash window resumes on the same machine only:
// fork transcripts live under the machine-local ~/.claude projects directory.
// The state.json file and the reports are fabric-synced.
// A different machine sees the report with no transcript behind it;
// record-batch treats that as it treats a forged report:
// it archives the report and returns a *ReportArchivedError naming `lym webster begin-batch`, which re-drives the batch.
// A report with no begin-batch record is archived by begin-batch itself.
// It proceeds and returns the archive path as BeginResult.ArchivedReport;
// only a batch with no record is archived this way, and a recorded batch's report is never archived by begin-batch.
//
// A plan edited between runs normally refuses the next run with ErrFingerprintMismatch,
// and `--fresh` is the escape that archives the record.
// RunOptions.AutoRebaseline, which only the loom's Webster row sets, is the second:
// Run rebaselines the changed plan on entry, before anything else acts on the run.
// It names every changed card file's number as Rebaseline's cards,
// so it accepts exactly what the operator's `--card` flags accept:
// a follow-up card after the last begun batch;
// an unbegun card's edit;
// an in-flight card as an amendment recorded unrendered for the next recovery to render.
// A done card's edit and a begun batch's card-set change are refused.
// So are a failed batch with uncheckable findings and an overview change outside its Card Index.
// An accepted rebaseline is saved before the batch loop starts,
// so a crash after it re-enters on a matching fingerprint,
// and the fabric receives it with the run's next sync.
// Every error of the auto path wraps ErrAutoRebaseline,
// and its refusals name RunDeps.ReentryStep through Rebaseline's step-keyed texts, never `lym webster rebaseline`;
// the shed adapter maps them to Stuck.
// The accepted rebaseline's warning is logged once at Warn and leads RunResult.Warnings.
// It also opens a stuck outcome's reason and every error Run returns afterwards,
// and the plan refusals that follow it (validation, zero batches, quarry) wrap ErrAutoRebaseline too.
// `--fresh` beside the option keeps its archive path and runs no rebaseline,
// and a run without the option keeps ErrFingerprintMismatch.
//
// # The plan-conflict record
//
// begin-batch leaves two never-tracked files in the scratch directory (`.lym/webster/`), never under `_lym`,
// so no sync carries them:
// the conflict record `plan-conflict.md`, naming the batch, the cause and each blocking finding;
// the last-routed conflict `plan-conflict-routed.md`, holding the findings of the conflict last routed to plan review.
// Their paths are ConflictRecordPath and RoutedConflictPath.
// ConflictRecordRel is the record's anchor-relative path for recipe text.
// The record is bounded to one Webster call:
// Run removes it on entry once it holds run.lock, and begin-batch removes it as its first step.
// It then writes it only when it refuses with a PlanConflictError and the plan-fingerprint re-baseline persisted.
// So a record exists only when the latest begin-batch of the call refused with `plan_conflict`.
// A begun batch removes the routed file under the state-mutation lease.
// A failed removal or write is logged at Warn and changes no refusal, class or outcome.
// Under loom, the Webster producer claims the record when Master ends stuck (ClaimConflictRoute) and decides on its findings:
// findings that differ, as a set, from the routed file's are new,
// so the claim rewrites the routed file and the stuck outcome reroutes to plan review;
// findings equal to the routed file's are a recurrence,
// and the stuck outcome halts for the plan amendment route:
// a parent's `--amends-plan` decision on the named pending cards and a goto to Plan-Burler-Review, not a hand edit;
// no record, or one naming no finding line, claims nothing.
// A read or write error is returned, and the producer then halts rather than reroutes.
//
// # The verify-gate fixer fork
//
// A gate failure reaches Merriam as a `Gate findings recorded at …` message.
// The message names the verify-gate report (VerifyGateReportPath).
// The gate routes each failure to one of two fixers, FixerFork or FixerCold (see The verify gate below).
// It records the choice as the report's `fixer`.
// For a fork failure, Merriam spawns one fixer fork in the background with a prompt Run rendered at entry.
// RenderVerifyFixPrompt writes it into the prompts dir as verify-fix.md.
// Merriam then ends its turn.
// On the fork's notification it rewrites its outcome and summary files, which re-arrives at the gate.
// The gate's re-prompt line and its findings both open with the way forward to the routed fixer.
// It names the fixer-fork step and the prompt file's path, or the backgrounded `lym webster fix-verify` call,
// so the rule stands where Merriam reads the failure.
// The waiting turn end in between is no arrival.
// Both fixers' prompts share one body, webster-body-verify-fix, behind a prefix of their own.
// The fixer fixes the cause in source and never deletes, skips or weakens a test.
// It never touches the plan directory or `_lym`, and it does not run the plan-level verify.
// It commits each fix as `fix: <summary>`.
// Before it reports it runs the hub's `webster-fix` gate call, `lym gate test --gate webster-fix`.
// Run and the cold fixer's spawn render that call from the told Geometry.GateConfig into the prompt's `fix_gate` marker.
// Its commits skip record-batch's done-checks, drift detection and glyph scope guard.
// They are bounded by the gate's commit check, the run-exit fork audit, the gate's own verify and the Code-Review perch.
// That perch follows Parent-Align and reviews the committed range.
// The plan directory and `_lym` sit outside every code commit,
// so the run-exit fork audit is what catches a fixer write there:
// forkOwnReport maps a fork transcript outside every batch bracket to the verify-gate report,
// and CheckFork audits the fixer fork as it audits a batch fork.
//
// # The cold verify-gate fixer
//
// For a cold failure, Merriam runs `lym webster fix-verify` as a backgrounded Bash command.
// Master's spawn declares that command an awaited shell prefix beside recover-batch.
// Merriam ends its turn.
// Whatever its exit, on the command's completion notification it rewrites its outcome and summary files as after a fork.
// internal/gatefixer owns the cold fixer's spawn, wait and runproc registration.
// Webster adapts it through a store over State.VerifyFixers.
// FixVerifySpawnOrAttach spawns the cold fixer as its own strand through it, with role `verify-fixer`.
// The model is RoleRecovery's choice, and the prompt is gatefixer.RenderPrompt's:
// the shared body behind the shared cold frame.
// The frame carries the parent, edit, PATTERN and friction directives and the orientation steps.
// The strand is registered with runproc under the worktree's pair slug when the run has one,
// so the run's end verbs stop it.
// It refuses and spawns nothing in these cases:
// no verify-gate report is on disk;
// a recovery role's engine cannot audit forks;
// a gate config has no `webster-fix` key.
// The fixer's final action is its reply file in the scratch dir (VerifyFixReplyPath).
// That file is the strand's shuttle output file, and a stale one is removed at spawn.
// Each spawn appends a VerifyFixerState to State.VerifyFixers, which the verb saves and fabric-commits at once;
// a call that finds the last record not terminal attaches to it instead of spawning a second.
// FixVerifyAwait blocks for RecoveryWaitBudget and classifies the fixer done on its reply file.
// It classifies it dead when its newest turn ended without one, recovery_timeout_min passed or its strand is gone;
// every terminal stops the strand through the stopper, and only done reads the reply and removes the run dir.
// The verb records the terminal status in a reloaded state and fabric-commits it.
// It returns the status, with the reply on done and the dead reason on dead, and exits non-zero on dead.
// The gate, not the verb, judges the fix: a dead fixer leaves the failure for the next evaluation to count.
// Bound: the verb spawns whenever a verify-gate report exists, a fork-routed failure's included, and counts no spawn;
// the gate messages bound how many cold fixers a run gets, and the attempt budget caps those messages.
// The Master template also says to run it only when a message names it.
// The run-exit audit reads the forks of Master's session only,
// so a cold fixer's own writes are checked only through the contract-file evidence that loadRunWrites collects.
// A recovery strand's writes are checked the same way.
// Each fixer session joins the run's write evidence (loadRunWrites) beside the recovery sessions.
// Run entry stops a fixer recorded not terminal and saves its record dead/died,
// so the next fix-verify spawns a fresh fixer instead of attaching to the stopped strand.
// `reset --to start` stops every recorded fixer strand.
//
// # The plan fixer
//
// A `plan_conflict` refusal of begin-batch with the cause `plan_defect` or `tree_moved` goes to a cold plan fixer.
// Master ends stuck only after it.
// A refusal with the cause `plan_order` goes to no fixer, since moving a delete reshapes cards.
// Master ends stuck at once quoting the refusal.
// For the fixer, Merriam runs `lym webster fix-plan` backgrounded and ends its turn.
// On the verb's completion it calls begin-batch again for the same batch on verdict `fixed`.
// Otherwise it writes `outcome: stuck` quoting the begin-batch refusal,
// so a tapestry run still reroutes to plan review through the conflict record.
// Nothing but the next begin-batch removes that record.
// A `plan_drifted` refusal gets no fixer.
// FixPlanSpawnOrAttach reads the conflict record (ReadConflictRecord) and spawns the fixer as its own strand.
// The role is `plan-fixer`, the model is RoleMaster's choice, and the timeout is recovery_timeout_min.
// The prompt, from RenderPlanFixPrompt, carries the parent, edit and friction directives.
// The fixer edits only the target names, paths and glyphs the findings name in the cards they name.
// It runs `lym webster rebaseline --card NN` for them,
// and it runs `lym webster restore-plan` when rebaseline refuses or a finding would change what a card does.
// It writes its reply (PlanFixReplyPath), whose first line is `verdict: fixed` or `verdict: plan_writer`.
// Each spawn appends a PlanFixerState to State.PlanFixers, which the verb saves and fabric-commits at once.
// The state holds the conflict's batch and findings and the worktree's HEAD and uncommitted paths;
// a call that finds the last record not terminal attaches to it.
// FixPlanAwait waits as FixVerifyAwait does and reads the verdict from the reply;
// a reply without a known verdict reads `plan_writer`.
// Bound: the verb refuses when an earlier plan fixer of the run was given the same findings, compared as a set.
// It also refuses once the conflict's batch had planFixersPerBatch fixers.
// So a fixer that cannot clear a conflict leaves it to the plan writer after at most that many extra begin-batch calls.
// The fixer's session is not in the run's write evidence:
// its one legitimate write, to the plan directory, is a correctness violation for every session there;
// instead the verb overrules a `fixed` verdict to `plan_writer` with a warning.
// That happens when the fixer moved HEAD or left an uncommitted change on a path that was clean at the spawn.
// The fixer's plan edits are bounded by rebaseline's acceptance rules and begin-batch's re-resolution.
// Run entry stops a plan fixer recorded not terminal and saves its record dead/died,
// and `reset --to start` stops every recorded plan fixer strand.
//
// # The verify gate
//
// Run hands Merriam's spawn the hub's `webster` gate as one must-pass gate entry named `verify` (NewVerifyGate).
// Any entry the caller's RunDeps.Gate carries comes beside it;
// a RunDeps.Gate that already names `verify` is refused, so a recipe row cannot add a second entry.
// Its attempt budget is Config.VerifyGateAttempts,
// and the count and the pre-fix head live in the closure's memory for one shuttle run,
// so an attach restarts them from zero.
// The shipped Webster row supplies no gates of its own.
// StartMaster is Merriam's only start,
// so every Merriam carries the spec.
//
// At each gated Done arrival the closure passes without verifying when outcome.yaml names an outcome other than done,
// so a stuck outcome ends the run.
// Otherwise it fails with the dirty paths when the tree is not clean, and runs no verify.
// With a pre-fix head recorded, fixCommitRejection (gitwrap.go) checks each commit from it to HEAD.
// The check follows the first-parent chain.
// Each must be a non-merge commit or a clean parent merge;
// any other fails the gate `Terminal`, which ends the run at once with the commit and the reason.
// At each evaluation the gate's notes record the non-merge commits in that range.
// They also record the commits' `Pattern-Departure:` trailers.
// Run appends them after the wait as a `## Verify gate fix commits` section of summary.md (AppendVerifyGateFixCommits):
// one bullet per `fix:` commit, then one per departure.
// Otherwise the closure renders `lym gate test --wait --gate webster` for the `webster` gate key.
// It reads the told Geometry.GateConfig at each arrival.
// verifytree.Verify runs that call under the site label `webster gate`,
// and it is told Geometry.VerifyHold so a pause covering the run holds the first run and the rerun until the resume;
// a pass records the tree, keyed by the rendered call.
// A config that cannot be loaded, or a `webster` key the config refuses, is the gate's returned error, never a pass.
// The error names `lym config gate`.
// A failure is parsed from the log and rerun once:
// a rerun pass passes the gate, and the identities that failed once are flaky.
// Run reports them after the wait as a warning, a summary.md section and a friction note (VerifyGateNotes.Apply).
// A failure that survives the rerun returns findings.
// Each failure goes to a fixer.
// The cold fixer takes the failure that reaches the attempt budget;
// it also takes one whose previous fixer added no commit above the pre-fix head;
// and it takes every failure after one that went cold.
// The fixer fork takes the rest.
// The routing lives in the closure for one shuttle run, like the count.
// VerifyGateNotes carries it to the re-prompt line.
// Bound: a re-prompt shuttle could not deliver is evaluated again, which counts one more failure,
// so the routing can go cold one attempt early.
// A run that outlives the verify timeout is never rerun, since a hang is not flakiness: it fails the gate at once,
// and the report names the timeout, the log path and the log tail, and says the step was not re-run.
// A run whose every failed tier run crashed its test runner with no failing test judged no code:
// on the first run or the rerun it fails the gate at once and terminally, naming the crashed runs.
// It uses no fixer, spends no attempt, records no pre-fix head and writes no report.
// A failed run that hung is not re-run either: classifyHang reads the log and exit code,
// and the first failure and the rerun's failure are both checked.
// The markers are checked in this order:
// the CPU watchdog line of `lym gate test`;
// go test's `panic: test timed out after`;
// the runtime's `SIGQUIT: quit` dump header;
// and a log line starting `signal: `.
// Without a marker, an exit code of 128 or above is a hang, and so is -1 with no `Detail`.
// `verifyrun` returns -1 for a shell killed by a signal.
// A shell that could not start and an ordinary non-zero exit below 128 are no hang.
// Bound: the markers are matched as text anywhere in the log,
// so a test that prints one, or a test binary the OOM killer ends, also reads as a hang,
// and any exit code of 128 or above reads as one whatever caused it.
// A false match skips only the one rerun, and a flaky failure in the same run reaches the fixer without its retry.
// The hang report carries `Hang` (the marker or `exit code <n>` matched) and `HangStacks`.
// It carries `WatchdogWindow` after a watchdog quit.
// It also carries the failing identities the log parses to, the log path and the log tail.
// Its findings say the step was not re-run and whether goroutine stacks are in the log.
// After a watchdog quit they name the window.
// They also say that a package that is CPU-bound but healthy needs `gateslot.BusyQuitWayForward`, the operator's to do.
// The first failed evaluation of any kind records HEAD as the pre-fix head,
// so every commit a fixer makes afterwards is checked at the next arrival.
// The same moment persists it as state.json's `PreFixHead`, overwriting a value an earlier shuttle run left,
// so a later verb can reset to it;
// a passing evaluation clears it, and `run --fresh` archives state.json with it.
// A persist failure is returned as the gate's error.
//
// Every other entry of RunDeps.Gate, such as the `drift` entry, is wrapped by wrapRunLevelEntry,
// so its failure takes the verify entry's paths:
// a failed result writes the verify-gate report with the entry's `entry` name, `findings` and failure count.
// It returns the rendered report as the findings,
// and it re-prompts with the verify entry's line, which names the fixer fork or `lym webster fix-verify`.
// The wrap changes how a failure is reported, never whether the entry passes.
// A Terminal result passes through unreported.
//
// After the wait, a done run whose gate did not pass ends stuck.
// The reason names the failing identities and the attempts spent, or is the `Terminal` failure's own reason.
//
// # A broken config stops Master
//
// A verb refuses in the CLI's pre-run, before its body, with `config_invalid: true`.
// That happens when a present `webster.yaml`, `batcher.yaml` or hub `language.yaml` fails to parse or validate;
// `LoadConfig` marks those failures `configengine.ErrInvalid`, and the refusal changes no state, report or plan.
// Master's failure ladder has a rung for it:
// a refusal carrying `config_invalid` from any verb ends the run with `outcome: stuck` quoting the refusal.
// Master calls no other verb after it.
// A config file that exists but cannot be read refuses with no `config_invalid` and a transient way forward,
// so it never stops a run through that rung.
//
// # Background shells in Master's wait
//
// Master's spawn declares its awaited shell prefixes, `masterAwaitedShellPrefixes`:
// the backgrounded recovery verb of the failure ladder;
// the backgrounded cold fixer verb of the gate-failure section;
// and the backgrounded plan fixer verb of the plan-conflict rung.
// recovery_timeout_min already bounds each verb, so the prefixes keep them out of the background-shell wait marker.
// The prefixes are also what keep them holding a gated turn end.
// A gated turn end waiting only on non-awaited shells is held when one of Master's output files is missing.
// With every output file present it is not done while such a shell runs.
// The shell may hold a commit or a gate run whose result Master never read:
// shuttle tells Master once per shell to wait for it and read its result.
// It counts the turn end done once every such shell has been outstanding for `background_shell_wait_min`.
// A gated turn end waiting on an awaited verb or on a fork is never an arrival, and neither expires.
// The run ends on the next turn end with nothing outstanding, the run deadline, the liveness check or a stop;
// a deadline still ends it done through shuttle's satisfied file contract with the gate run once at finalize,
// and the shell's result is then never read.
// `background_shell_wait_min` sets when a long-running shell is logged and shown in the wait marker.
// Every shell outstanding when the run ends comes back on shuttle's `Result.EndedShells`, whatever ends the run.
// lym does not stop such a shell.
// What ends the shell follows the run's final outcome.
// When Master's turn ends shuttle-done, shuttle removes Master's strand as the run finishes.
// The session and the shell end with it.
// Shuttle-done covers a webster done, Master's own stuck, the verify gate's demotion, or a mapping error after that end.
// When the run returns a died or timeout error, Master's strand stays alive until the next `lym webster run` reclaims it at entry.
// Run writes one best-effort `webster-background-shell` friction note once the outcome is known, on every outcome.
// For each shell the note states its label, signal and time outstanding, and that the run never read its result.
// It also states that lym did not stop the shell, what ends it and the run's final outcome.
// Each shell is also a `RunResult.Warnings` entry with the same wording, added after the verify-gate demotion.
// That holds on every outcome that returns a `RunResult` (done and stuck), so the warning and the note cannot drift;
// an error outcome returns no `RunResult`, so the note alone carries it.
// Every `RunResult` also carries `RunResult.Gate`, Merriam's shuttle gate outcome.
// The outcome passes through with the verify gate's entry among its entries, demotion to stuck included.
// summary.md's "Background shells at the run's end" section (AppendBackgroundShells) stays done-only.
// So does its "Plan rebaselined" section.
// It names an accepted auto-rebaseline beside the "Audit warnings" section,
// so the summary a step's Done points at carries it.
//
// # Planning a reset
//
// PlanReset decides, read-only, what a reset of the task branch may do, and refuses before fabric is ever called.
// The target is one of five commits the run recorded (ResetTargets); no raw SHA is accepted.
// `start` (ResetToStart) is the run's oldest recorded start commit;
// when none is the oldest of all, it is the octopus merge-base of the recorded starts;
// `pre-fix` (ResetToPreFix) is state.json's `PreFixHead`;
// `last-batch-head` (ResetToLastBatchHead) is the last recorded batch head.
// accept-audit picks that commit by git ancestry.
// `batch-start` (ResetToBatchStart) takes `--batch NN` and is batch NN's recorded start commit.
// It is refused when a later batch in the partition's order recorded a start.
// `report-head` (ResetToReportHead) takes `--batch NN` and is the `head_sha` of batch NN's report.
// An abbreviated one is resolved to the commit it names, as record-batch does.
// It is accepted only for a begun, non-terminal batch whose report parses.
// The report's head must descend from or equal the batch's start.
// No later batch may be begun, and no live recovery strand of the batch may be in reed.
// It is the one target allowed while the run lock is held.
// Master runs it inside its run between a fork's report and record-batch.
// Its bound is that those checks rule out every writer lym can see;
// a Master that spawned an in-session fork out of order stays unseen.
// `--batch` is required for `report-head` and `batch-start` and refused for the other three, before any git read.
// The refusals run in order:
// the `--batch` pairing; run lock held (transient, except for `report-head`); no state; a merge in progress;
// a checked-out branch that is not the task branch; no recorded target; a recorded commit missing from the repository;
// a target that is not an ancestor of HEAD; and a dirty tracked path outside the run's own writes.
// The run-lock refusal's way forward has the run's parent re-run the reset once the run has stopped.
// A Webster session inside the run cannot.
// The own paths are the tracked paths that loadRunWrites records a successful write to.
// The writers are Master, every fork, every recovery session and every cold verify-gate fixer session;
// a failed write is not evidence.
// Each refusal ends in wayForwardSteps' numbered list.
// The plan carries the SHA and the own paths, and reads no force flag.
// `start` is the one start-over verb, so it does not refuse where the start cannot be moved to:
// PlanReset returns a plan with ArchiveOnly set and the Reason in four cases:
// when no batch recorded a start; when a recorded start is missing from the repository;
// when the recorded starts share no single oldest commit and no common ancestor;
// or when the resolved start is not an ancestor of HEAD.
// Those four are the closed set, since each guards only a branch move the archive-only path does not make.
// The refusals before the target resolves, and `pre-fix` and the other targets, keep all of theirs.
//
// # The reset verb
//
// `lym webster reset --to <target> [--batch NN]` (internal/webstercli) performs the reset PlanReset planned,
// so no recovery needs the denied `git reset --hard`.
// Under the state-mutation lease it plans with the pair's branch read through the fabric handle.
// It resets the pair's code checkout through fabricengine's pair-checkout reset.
// That reset takes the plan's SHA, the parent branch from the origin record and the own paths.
// It then clears State.PreFixHead, saves, and fabric-syncs state.json.
// It changes no other webster state.
// A reset to start ends by calling ArchiveRunAfterReset under the same lease, after the move and the save,
// so a following plain `run` finds no state and starts a new run.
// When the pending-findings guard refuses after the move, the move and the save stand and the record stays unarchived.
// Re-running the reset converges, since a reset to the commit HEAD is already on moves nothing.
// On the archive-only path the verb removes the live recovery strands as a moving reset does.
// It runs no git that mutates and archives alone;
// the guard there refuses only on what its clearing step can undo, because it judges against HEAD.
// The trees differ by mode:
// a pair's reset discards the run's tracked changes and keeps untracked files,
// and the archive-only path keeps everything, which the next run starts over.
// A plain `run` does the rest for the other targets, as the way-forward texts order them.
// The reset also moves the task branch on the remote to the same commit,
// so a later push is not rejected as diverged; `FABRIC_SKIP_PUSH=1` skips that half.
// The bound: it can discard only these:
// commits above a run-recorded commit on the task's own branch, on the checkout and on the remote;
// and uncommitted tracked changes to paths the run itself wrote.
// The remote update is made only when every commit it drops is reachable from the task worktree's HEAD,
// and it is made under a lease on the remote tip it read;
// a remote-only commit becomes reachable only through a `git merge --strategy ours`.
// The operator runs it after reading the commits the refusal lists.
// It cannot move another branch, take a raw SHA, touch the parent branch, the records side or untracked files,
// and it has no `--force`.
// Fabric's own refusal (ownership, dirtiness, remote divergence, an unreachable remote) is the verb's error.
// The error carries fabric's reason.
// In standalone mode, with no task pair, the verb performs the planned move itself with gitrepo's keep-reset.
// It touches no remote.
// The call sits in internal/webstercli, since webster's engine runs no mutating git.
// PlanReset skips its foreign-dirty-path refusal there (ResetDeps.Standalone), because keep is the guard:
// it carries an uncommitted change across and refuses, changing nothing, over one the move would overwrite,
// so it never discards one.
// A standalone reset therefore leaves foreign tracked changes the move does not touch in the tree,
// and every other PlanReset refusal still applies.
// KeepResetRefusal builds the keep refusal from git's message, a step per overwritten path and the re-run.
// The step is `git checkout -- <path>` for a path the run wrote and commit-or-restore for any other.
// A standalone reset keeps every change git carries across, the run's own included,
// so its `uncommitted` list holds all of them.
// The envelope carries `target`, `sha`, `mutations` and `partial`, false on success.
// `mutations` holds the `worktree_reset` entry, and `remote_branch_updated` when the remote moved.
// A reset to start, and a standalone reset to any target, also carries `uncommitted` and `warnings`.
// `uncommitted` is UncommittedPaths read after the move.
// `warnings` holds the findings a reset to start's archive dropped.
// A reset to start also carries `moved` and, when `moved` is false, `reason`, with no `sha`.
// A failed UncommittedPaths read leaves `uncommitted` out and adds a warning.
// The reset and any archive are already done by then.
// A refusal before the remote update is a bare error envelope.
// A checkout rewrite that fails after the remote moved is an error envelope carrying `mutations` and `partial: true`.
// Re-running the reset converges.
//
// # The verify-gate report and findings
//
// Every failed evaluation writes the verify-gate report (VerifyGateReportPath).
// The report is `verify-gate.yaml` in the reports directory.
// It returns renderVerifyGateFindings of the report as the findings Merriam reads.
// The report carries the attempt and the cap, the failing identities or the dirty paths, and the verify log's path.
// It also carries the commits the fixer made since the pre-fix head and a hint:
// the cards, in trail order, whose commits changed a file directly in a failing package's directory.
// cardHint computes it over accumulatedCardSHAs.
// The hint claims only that a card touched a failing package;
// Merriam judges it with the plan in hand.
// The gate runs no bisect, no baseline run and no detached checkout of the live worktree.
// Run removes a stale report at entry.
//
// # The Git seam
//
// Every repository question of the bracket verbs and the run-level checks goes through the Git interface (git.go):
// the head, dirtiness, a merge in progress, a commit's parents, the clean-parent-merge verdict;
// commit existence and ancestry, ignore rules, linked worktrees and blobs.
// Geometry.Git carries it, and nil means the real repository,
// so no production caller sets it and the helpers in gitwrap.go stay the one place webster runs git.
// A test that asserts on webster's own records, warnings and verdicts sets a fake and spawns nothing;
// a test whose behavior is git itself builds a real scratch repository under the `integration` tag.
// Such behavior includes merge commits, ignore rules, blobs and the verify gate.
// UncommittedPaths is the one read of the task's uncommitted work, sorted.
// It holds the dirty and untracked paths, minus the run's own state.
// That state is webster's run directory, the plan directory and the scratch directory.
// accept-audit's clean-tree evidence and the reset's and the recovery prompt's lists of uncommitted paths all read it.
//
// # The code-index seam
//
// Every plan gate and the batch delta go through Geometry.Index, a planindex.Index (internal/planindex),
// so this package imports neither internal/planglyph nor quarry and links no tree-sitter.
// The CLI layer sets the real index (planglyph.NewIndex).
// hubgeom and standalonegeom leave it empty because they sit below cliwire.
// A call that reaches a nil Index returns an error naming the missing wiring.
// A test that drives the real resolve pass sets planglyph.NewIndex.
// A test that steers the batch delta sets a fake planindex.Delta.
//
// # No shared substrate or parser with any other batch-implementation loop
//
// websterengine imports no other batch-implementation module's plan parser, report schema, or digest contract.
// Only planparser and this package's own Report and Digest types are in play.
// It reuses the provider-invariant orchestration substrate that every `lym` module built on shuttle shares:
// internal/reedengine, internal/shuttleengine and its claudeengine, and internal/gitrepo.
// It owns the rest of its mechanism, with no cross-module import.
// The plan-fingerprint hash is in fingerprint.go.
// The git-query helpers built on gitrepo are in gitwrap.go and are reached through the Git seam.
// The archive-never-refuse primitives are in archive.go, and the recovery classification is in classify.go.
//
// # Why cards run one at a time
//
// Cards run strictly in sequence inside one working tree, and the plan carries no scheduling DAG.
// Git's index is a single shared file per working tree, so two forks committing at once race on the same lock.
// That holds even when their files are disjoint.
// A declared-disjoint pair that overlaps through a deviation would corrupt the tree, not merely misreport it.
// Nothing isolates forks on disk, so a fork's code-index queries would see other forks' broken, uncommitted edits.
// Letting forks edit in parallel and serializing only the add, commit and verify step through a mutex is no remedy.
// It would still need file-disjointness enforced strictly, not merely the absence of a dependency edge.
// None of this is built.
//
// A shape that avoids the shared index is not covered by that reasoning:
// each independent group in its own `fabric`-spawned worktree, with its own index and HEAD.
// The groups merge back through fabric's merge machinery, gated on a build and test of the merged result.
// The ready set would be recomputed wave by wave, so a group that depends on another's output is not ready yet.
// Batten's run-id addressing already carries concurrent runs side by side.
// What remains is a wave scheduler and a plan group-filter.
// It also needs a variant that spawns its agents into the lane worktree's own reed session.
// That replaces forking inside the Master's context.
// The board holds this as `webster-parallel-execution`.
//
// The case for it is weaker than the idea suggests.
// A card-level dependency analysis of a large plan showed a shallower card DAG than a linear chain.
// File conflicts barely bind when a plan creates and then extends.
// Most conflicting pairs are already in different waves.
// The tail sets the ceiling: a funnel near the end collapses the wave width whatever the fork budget.
// A wave lasts as long as its slowest card, so the wall-clock gain is a small multiple of what a card count suggests.
// A routine small plan has little fan-out headroom and would gain close to nothing.
// The planner already emits true per-card dependencies instead of an over-constrained batch line.
// Reopening the executor should rest on the same width analysis run across several real, typical-size plans:
// wide waves, a short critical path and few file conflicts would pay off,
// and a narrow plan makes the sequential design the complete design.
package websterengine
