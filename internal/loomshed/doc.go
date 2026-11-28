// Package loomshed owns loom's producer constructors and its durable row names.
// It also owns loom's status seeder and its cancellation helpers.
// Loom's ordered producer list lives in contracts/recipes/tapestry-recipe.yaml.
// internal/loomrecipe assembles a *shedengine.Shed from it.
// It takes told absolute paths and has no direct production import of internal/lymcwd -- see PATTERN-cwd-resolution.
//
// The stops its gate rows halt on each name a way forward, as contracts/specs/refusal-spec.md defines one.
// Both implementer rows, Webster's and Weaver's, run a guard first, so one run stays on one implementer.
// A row whose run holds the other implementer's begun batches stops without spawning anything.
//
// Its plan gates and the discussion gate's citation check resolve refs and symbols through a told planindex.Index,
// so the package links no tree-sitter grammar.
// A quarry outage in the citation check is a returned error, never a finding.
//
// ValidateDiscussion is shared by the following:
// every loom record row's gate, `lym loom validate-discussion` and Darn-Write's post-session guard.
// NewDiscussionGate maps it onto the gate contract and commits nothing.
// The plan gate and `lym loom validate-plan` share ValidatePlan.
// A twill Plan-Write run's gate and `lym loom validate-plan --lens-review` share ValidatePlanLensReview.
// Cards of batches the run record holds done are history, so their tree-dependent checks are skipped.
// A done card is also frozen:
// ValidatePlan appends a blocking `done-card-edited` finding when its file's hash differs from the one its batch stored.
// An unreadable card file is a returned error.
// Once a batch has begun, 00-overview.md outside its Card Index is frozen as well.
// An overview whose frame differs from the one the run recorded gets a blocking `overview-frame-edited` finding.
//
// A split plan has checks of its own.
// NewPartsGate maps partsparser.Validate onto the gate contract for the Parts-Write row.
// ValidatePart is the part check of `lym loom validate-plan --part`, and the part plan gate and Accept run it too.
// JoinParts re-runs it on every part before joining the parts' plans.
//
// Every plan gate and every `validate-plan` mode also takes a PlanContext.
// It adds the blocking checks outside planparser,
// and the PATTERN checks that read `PATTERN.md` and the base overview from PlanContext.PatternBase.
// contracts/specs/loom-plan-checks-spec.md defines each finding.
// The zero PlanContext skips the checks whose context field it leaves unset.
// A worktree with no `PATTERN.md`, or a nil PatternBase, runs no PATTERN check and reads no base.
//
// A plan gate that passes on informational findings alone writes them to a file under the run's told scratch directory.
// The file is `<gate>-informational-findings.txt`.
// The gate keeps them inline in the log line when it has no such directory or cannot write.
//
// The clean gate (NewCleanGate) fails while the worktree holds any uncommitted path, untracked and not ignored included.
// It asks the session to commit what belongs to its change and delete the rest.
// The run is not halted at a later row.
//
// The verify gate (NewVerifyGate) is told its verify command by the recipe's wiring and never parses the plan itself.
// The command is the row's own `lym gate test --wait --gate <key>` call,
// so each fix row and writer runs its own key and shares no pass with Webster's end gate.
// Told a tree scope reader, it takes the darn form; otherwise it takes the round form.
// The form is a told reader, never inferred from the merge base reader's presence.
// Both forms lint headroom and comments before any test, then run the told command through verifytree.Verify.
// The gate runs no flake rerun, and the key lists the tiers it runs.
// Publish reruns both verifies in full regardless, and stays the guard.
// A watchdog quit of a healthy package has no code fix:
// the way forward is raising `gate.busy_quit_sec` through `lym config gate` from the prime, then re-stepping the row.
// The darn form requires the merge base reader and has no skip path.
// A tree-scoped diff is one wholly under the hub's configured non-production trees.
// For one, an empty tree-scoped command runs nothing and passes, so an empty tree scope never falls back.
// An empty told command is the gate's returned error, and so is a read error of the command, merge base or tree scope.
// An impact-scoped pass lets through a break only the whole module shows;
// Parent-Align's whole run catches it for every diff touching production code.
// In the darn form a failed run naming failed tests alone keeps them, and the next arrival runs them alone first.
// A told VerifyBudget notices the run's parent once the gate has run past its limit, and stops nothing.
// Bound: the kept tests and the budget's clock live in the gate, so a restarted driver starts both afresh.
//
// The red-first gate (NewRedFirstGate) holds a darn writer's handoff on a bug fix until its test is shown red first.
// An entry without the `bug` label (gateslot.BugLabel) passes at once and runs nothing.
// So does a decision record holding an added entry whose title opens gateslot.NoTestSeamTitle.
// So does one whose title opens gateslot.NoBugFixedTitle, for a change that only adds tests pinning existing behaviour.
// A hub whose gate.scoper is neither go nor dotnet passes too, since no check can show the test red there.
// So does a C# change whose tests the check cannot select or run.
// A diff that adds or changes no test holds the run without spending an attempt.
// The hold lasts until a new commit or the parent's record.
// Failing exits name their way forward, and any other exit code, a timeout or an unjudged run fails terminally.
// A labels or record read error is the gate's returned error.
//
// The Plan-Write rotation archives the prior plan and webster's run record,
// so the next Webster run starts over the new plan instead of refusing the old record's plan drift.
// A run holding webster's run lock refuses the archive before any plan file moves.
// `_lym/plan/`'s top-level files hold exactly one plan generation.
// A retired generation lives in the rotation's archive or under `round-<N>/prior-generation/` in the rework round.
// This package declares the round layout once.
// Webster's and Weaver's run records and the Plan-Review and Code-Review run directories are generation-scoped:
// they move into the round with the plan, so the next generation starts with none.
//
// The Discussion-Write and Darn-Write producers share runAndCommit.
// A nil commit seam is refused by name, and a commit failure is a returned error, never Stuck.
//
// The Darn-Write row's producer (NewDarnWrite) wraps the gated darn writer session.
// The decision record Record-Write wrote is the writer's scope.
// A call that finds it or its support log missing halts the run Stuck before any session.
// A record the committed history holds but the working copy lacks is restored from that history.
// It is never written again from the board entry.
// A stop marker the session wrote halts the run Stuck after the commit, with the parent's answer as the way forward.
// A condition stop marker beside it marks a stop on a condition the task body names;
// the stop report's own way forward then stands instead.
// A gate-failed Stuck keeps its record and gains the resume way forward.
// Prior work told to a spawn is the latest recorded outcome, the branch's commits beyond base and the scratch entries.
// A spawn interrupted before it recorded an outcome is still named by those commits.
// Scratch entries with no commit and no outcome are listed with no claim of who wrote them.
// Every agent of the task shares the scratch directory.
// A Done session is then guarded by DarnDeps.CheckRecord, and findings halt the run Stuck with no re-prompt.
// Every restore keeps each `Added after Discussion` entry the record held.
//
// The Sampler-Session row's producer (NewSamplerSession) keeps one interactive agent session in the run's worktree.
// It waits in process, with no timeout, for the done flag `lym loom sampler-done` writes into the sampler record.
// Judging a flag stops the session, then reads the code checkout:
// uncommitted paths clear the flag and bring the session back;
// a decision record with findings is Stuck with Halt; an intact record on a clean checkout is Done.
// A strand that dies unstopped is Stuck with Halt, and so is a start failure that is not transient.
// Every verdict, Stuck or error stops a live session, but a cancelled context leaves it running.
//
// OperatorWaitRows lists the rows at which a running child waits on something outside it with no agent working.
// WaitsOnOperator reads the list.
// Dobby and the loop's step watchdog read a step at a listed row as active, so neither ends a wait on the operator.
// A wedged step there is ended only by `lym batten cancel` or the row's own halts.
//
// The Record-Write row's producer (NewRecordWrite) writes a darn run's decision record and support log with no agent.
// BoardRecord renders the record with the entry's body verbatim in one fenced block under `## Goal`.
// A board with no entry for the run is reported as Stuck.
// So is an entry body holding a CLI error envelope, which a failed verb's captured output leaves there.
//
// A raddle run's state is RaddleState, the run's `state.json`.
// It is read and updated through internal/state under one lock, so a key it does not declare survives a rewrite.
// CheckRaddleProposal is the one D6 check of a proposal on disk, and RecheckRaddleBoard is the one D8 re-check.
// NewRaddleGate maps the check onto the gate contract.
// StoreColdProposal stores the run's first passing proposal as the cold proposal once and never overwrites it.
//
// The Raddle-Write row's producer (NewRaddleWrite) wraps the table-driven raddle session.
// The run's first call records its mode, and every later call and Raddle-File read it from the state.
// The Raddle-File row's producer (NewRaddleFile) is a Go row with no agent.
// A proposal that fails its checks goes back to Raddle-Write, and the failure after two returns halts the run.
// Every filed task is draft, so a release stays the operator's.
// A step whose outputs carry the hash of the proposal now approved is skipped,
// so a retry after a crash finishes the steps it missed.
//
// PlanReviewSkippable classifies the live plan generation from the committed round records.
// It answers false while the plan amendment record exists.
// ArchivedRunRecordDirs lists each round's archived webster and weaver run records.
//
// InterruptPolicies also names the pawl recipe's Beam-Up, Beam-Await and Beam-Down rows as reinvoke:
// each spawns no agent and is idempotent.
// The agent Drift rows are reinvoke too: each probes for a live chair and reattaches to it.
//
// It declares its own unexported cancellation helpers (entryErr/cancelErr in ctx.go).
// With them every producer written here honours two obligations that Shed cannot enforce:
// it returns exactly Done, Stuck or Awaiting, and it surfaces context cancellation as a non-nil error, never as Stuck.
package loomshed
