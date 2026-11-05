// Package loomshed owns loom's own producer constructors, its durable row names, its status seeder, and its own cancellation helpers.
// Loom's ordered producer list itself moved to contracts/recipes/tapestry-recipe.yaml; internal/loomrecipe is what assembles a *shedengine.Shed from it.
// It takes told absolute paths and has no direct production import of internal/lymcwd -- see PATTERN-cwd-resolution.
//
// The stops its gate rows halt on each name a way forward, as contracts/specs/refusal-spec.md defines one.
// The Webster row loads `batcher.yaml` through the layer stack under the told `configengine.Dirs` when it partitions the plan, and a load fault stops the row with the fix-and-re-step way forward.
//
// Its plan gates, and the discussion gate's citation check, resolve refs and symbols through a told planindex.Index, so the package links no tree-sitter grammar.
// The citation check parses glyphs with the cgo-free `quarry/glyph` grammar.
// It tests a loom decision record's cited paths and symbols against the worktree, and a quarry outage is a returned error, never a finding.
//
// Every loom record row's gate, `lym loom validate-discussion` and Darn-Write's post-session guard share ValidateDiscussion over a DiscussionCheck.
// It runs the section check, the decision-id and acceptance check and, when the section check is clean, the history check that keeps every `Added after Discussion` entry and appended PATTERN bullet the record's committed versions held.
// With the check's Citations set, which follows the record row's engine, it also runs the citation check and the relevant-list check.
// NewDiscussionGate maps it onto the gate contract and commits nothing.
// The plan gate and `lym loom validate-plan` share ValidatePlan, which reads webster's run record under the anchor and tells the index the cards of every batch it holds done.
// A twill Plan-Write run's gate and `lym loom validate-plan --lens-review` share ValidatePlanLensReview, which adds the presence check on `lens-review.md`.
// Those cards are history, not targets: a run moved back to the plan review after Webster executed batches has their work in the tree, so their tree-dependent checks are skipped.
// Every other card, and every card of a plan with no run record, is checked against the tree.
// A done card is also frozen: ValidatePlan appends a blocking `done-card-edited` finding for each one whose file no longer hashes to what its batch recorded at begin, naming the follow-up card as the way forward.
// A plan with no run record or no recorded hashes reports nothing, and an unreadable card file is a returned error.
//
// A split plan has three checks of its own.
// NewPartsGate maps partsparser.Validate onto the gate contract for the Parts-Write row.
// ValidatePart is the part check `lym loom validate-plan --part`, the part plan gate (NewPartPlanGate) and a part writer's Accept (NewPartAccept) share:
// the plan gate's format and context checks over one part's plan directory under a part scope, without the decision record, so the PATTERN checks that rest on the joined overview are left to the joined plan's gate, plus a blocking `part-export-missing` finding for each export of the part's contract no card of the part meets.
// JoinParts re-runs ValidatePart on every part, in the parts file's order, and joins the parts' plans through planparser.JoinParts; the part to fix and the way forward are named by each refusal.
//
// Every plan gate, and every `validate-plan` mode, also takes a PlanContext and appends blocking checks outside planparser.
// `rename-tool-missing` is one finding per symbol `Rename` pair of a card not recorded done when the context's rename tools are empty, with `lym config language` as the way forward.
// `pattern-changes-mismatch` is one finding per `## PATTERN changes` bullet that the overview and the context's decision record do not both declare.
// An absent record adds nothing, and a malformed record section is a returned error naming `lym loom validate-discussion`.
// `test-tier-unstated` is one finding per test file a go plan's card not recorded done creates as a file target whose Intent lines do not state exactly one tier of the context's tier tags, a `||` join of several, or untagged.
// `seam-test-unlisted` is one finding per card not recorded done, struct a `//lym:seam` test marks and marked test, for a go plan's card that touches the struct without listing the test.
// SeamTestReader returns the PlanContext reader that parses the markers of the worktree's package directories; a marker fault is a returned error naming the file and line.
// `doc-go-unlisted` is one finding per card not recorded done and package directory whose non-test Go code a go plan's card changes without covering the package's `doc.go`; a directory with no `doc.go` on disk is skipped.
// The zero PlanContext runs none of the first four of these checks, a nil TierTags, the state with no gate.yaml, skips test-tier-unstated, a nil SeamTests skips seam-test-unlisted, and doc-go-unlisted needs no context field.
//
// Four PATTERN checks follow them, in this order, each reading `PATTERN.md` in the worktree and the overview PlanContext.PatternBase returns for the task branch's merge base, or for HEAD with no parent recorded.
// `pattern-removes-prescribed` is one finding per removed entry that a card not recorded done fences a line for.
// `pattern-headroom` is one finding when the overview projected from every card's fenced entry lines and the removed entries grows past the headroom line.
// `pattern-card-headroom` is one finding per card not recorded done whose own step grows past the line, the bound its card gate's pattern lint applies:
// the projection starts at the worktree overview and takes those cards in plan order, each applying its fenced entry lines and dropping the removed entries it is the first to name in backticks, and a removed entry no such card names is dropped before the first.
// `pattern-line-unprescribed` runs only with a decision record path, and is one finding per added or changed entry that no card fences a line for and whose bullet equals the base's.
// A worktree with no `PATTERN.md`, or a nil PatternBase, runs none of the four and reads no base.
//
// A plan gate that passes on informational findings alone records them as a count and a file path in its log line, and writes the findings, one per line, to `<gate>-informational-findings.txt` under the run's told ephemeral scratch directory.
// A gate told no scratch directory, such as a standalone validate, and a gate whose file cannot be written keep the findings inline in the log line.
// The blocking path is unchanged: the findings go to the writer and the log line in full.
//
// The clean gate (NewCleanGate) fails while the worktree holds any uncommitted or untracked path that git does not ignore, naming every path and asking the session to commit what belongs to its change and delete the rest.
// A row lists it before its verify gate, so a writer or fixer that left a stray file is re-prompted to remove it before its session ends, not halted at a later row.
//
// The verify gate (NewVerifyGate) is told its verify command by the recipe's wiring and reads it at each arrival; it never parses the plan itself.
// The command is the row's own `lym gate test --wait --gate <key>` call, the `code-burler-fix` key at Code-Burler-Fix and the `darn-write` key at Darn-Write, so each fix row and writer runs its own key and shares no pass with Webster's end gate.
// Told a tree scope reader, it takes the darn form; otherwise it takes the round form.
// The form is a told reader, never inferred from the merge base reader's presence.
// Told a merge base, the round form first runs the headroom lint from it to HEAD, before the comment lint and any test; told none, it skips that lint.
// The round form lints the comments added since the told command's latest recorded pass when that commit is an ancestor of HEAD, else since the merge base, so a first round lints rather than skips; with neither base it runs no lint.
// It then runs the told command as it is through verifytree.Verify: the gate derives its own diff base, narrowing, fallback to whole and guard step.
// Webster's gate, Publish and Finalize keep the full verify on the tree that lands.
// A failed run's findings carry the exit code, the log path, and the failing tests and packages the log names, each with its own output, followed by `lym gate test`'s closing line of failed runs; a log that names none is quoted as its tail instead, and when the whole log holds the CPU watchdog's line that `lym gate test` prints after it quits a busy test binary, a paragraph naming the window.
// A watchdog quit of a healthy package has no code fix: the way forward is raising `busy_quit_sec` through `lym config gate` from the prime, then re-stepping the row.
// The gate runs no flake rerun.
// The told command runs the tiers its key lists and nothing else, so `tmux` and `llm` tiers run only if the hub lists them in that key.
// Publish reruns both verifies in full regardless, and stays the guard.
// Told a tree scope reader, it takes the darn form the darn recipe's `Darn-Write` and `Code-Burler-Fix` rows run, and requires the merge base reader:
// the headroom lint and then the comment lint run from the task branch's merge base with the parent to HEAD, so every attempt diffs from the merge base and is never narrowed to a stale pass.
// The tree scope reader is then read at each arrival.
// A tree-scoped diff, wholly under the hub's configured non-production trees, runs only the tree-scoped command the reader answers; an empty command, as for a docs-only diff whose trees name no target, runs nothing, logs so and passes, so an empty tree scope never falls back to the computed scope.
// Any other diff runs the told command.
// The told command is the key's own call in every case, so a pass keeps its record entry and a later arrival on the same tree and command skips.
// That form has no skip path: an empty told command, a command read error, a merge base read error and a tree scope read error are each the gate's returned error.
// An impact-scoped pass lets through a break only the whole module shows;
// Parent-Align's whole run catches it for every diff touching production code.
//
// The Plan-Write rotation archives the prior plan and appends a prior-plan block naming that archive to the respawned session's prompt.
// It first archives webster's run record into the archive's `webster` subdirectory through a told seam, so the next Webster run starts a new run over the new plan instead of refusing the old record's plan drift.
// A run holding webster's run lock refuses the archive before any plan file moves, and ArchivedPlanWebsterDirs lists the subdirectories a rotation writes the record into.
// The Plan-Review and Code-Review run directories stay out of that rotation.
//
// `_lym/plan/`'s top-level files hold exactly one plan generation.
// A retired generation lives only under the rework round that retired it (`round-<N>/prior-generation/`), or in the Plan-Write rotation's archive above.
// Only PR-Rework archives a generation into a round, and this package declares the round layout once, building every path from the existing accessors and telling webster its archive destination.
// Webster's run record and the Plan-Review and Code-Review run directories are generation-scoped: they move into the round with the plan, so the next generation starts with none.
//
// The Discussion-Write and Darn-Write producers share runAndCommit, which runs the session once and commits its outputs on a non-empty output pointer, Done or a gate-failed Stuck.
// A nil commit seam is refused by name, and a commit failure is a returned error, never Stuck.
//
// The Darn-Write row's producer (NewDarnWrite) wraps the gated darn writer session and tells it a prior-work note, empty when absent.
// The decision record Record-Write wrote is the writer's scope: a call that finds it missing, or finds the support log missing beside it, halts the run Stuck before any session, with the way forward that brings the file back.
// A record the committed history holds but the working copy lacks is restored from there, never written again from the board entry.
// The producer removes the stop marker before each session, and a marker the session wrote halts the run Stuck after the commit, Done or gate-failed alike, with the session's stop report as the reason and the way forward that records the parent's answer.
// A condition stop marker the session wrote beside the stop marker marks a stop on a condition the task's body names: the reason is then the stop report alone, whose own way forward stands, or a fixed message naming the stop marker when the report is empty, and the producer removes that marker before each session too.
// The note, written in Go, is told whenever the run's history holds a Darn-Write entry:
// it says the change is already under way on the task branch, names that entry's outcome and the row's kept gate findings file when one exists, and lets a decision recorded after the Discussion win over a finding it forbids fixing.
// It reads no halt reason, because the shed's resume write clears the status error before the row is called.
// Each Darn-Write gate entry re-prompts with DarnGateReprompt's line, which names the decision record and lets the same decisions win over a gate finding.
// On a non-empty output pointer, Done or a gate-failed Stuck, it commits the landing directory.
// A gate-failed Stuck keeps its record and gains the resume way forward.
// A failed commit is a returned error, since re-running the session cannot fix a git or filesystem fault.
// A Done session is then guarded by DarnDeps.CheckRecord, which the wiring points at ValidateDiscussion over the run's record with no citation checks:
// findings halt the run Stuck with no re-prompt, and the reason names the findings and the way forward that restores the record, which keeps every `Added after Discussion` entry the record held.
//
// The Sampler-Session row's producer (NewSamplerSession) keeps one interactive agent session in the run's worktree and waits in process, with no timeout, for the done flag `lym loom sampler-done` writes into the sampler record.
// A recorded strand that is live is reused, a recorded session id whose strand is gone is resumed, and with none recorded a session starts; the started strand is recorded each time.
// A flag counts only at the row's current entry stamp.
// Judging one stops the session, then reads the code checkout: uncommitted paths clear the flag, bring the session back and mail it the paths, a decision record with findings is Stuck with Halt and the restore way forward, and an intact record on a clean checkout is Done.
// A flag written while the run was halted at the row is judged on entry, so the resume ends the row without starting or resuming a session.
// A covered pause stops the session and the next uncovered wake resumes it.
// A strand that dies with nothing having stopped it is Stuck with Halt, and so is a start failure that is not transient, each naming `lym loom resume` and the commit-then-`sampler-done` way out for a session that can no longer be resumed.
// Wakes are at least a second apart and double to a minute while nothing changes; a Watch delivery cuts a sleep short.
// Every verdict, Stuck or error stops a live session, but a cancelled context leaves it running.
//
// OperatorWaitRows lists the rows at which a running child waits on something outside it with no agent working, Beam-Await, PR-Review and Sampler-Session, and WaitsOnOperator reads the list.
// Dobby reads a child at a listed row as a live wait, and the loop's step watchdog reads a step at one as active, so neither ends a wait on the operator.
// A wedged step at a listed row is ended only by `lym batten cancel` or the row's own halts.
//
// The Record-Write row's producer (NewRecordWrite) writes a darn run's decision record and support log from the run's board entry, with no agent.
// BoardRecord renders the record with the entry's body verbatim in one fenced block under `## Goal`, so no line of the body starts a section of the record.
// It writes only the file that is missing, commits the pair on every call, and reports a board with no entry for the run as Stuck.
//
// A raddle run's state is RaddleState, the run's `state.json`: its mode, its Raddle-Write session count, the sessions of the last passing propose and waiting notice, the approval, the count and reason of Raddle-File's returns and its notice marker.
// It is read and updated through internal/state under one lock, so a key it does not declare survives a rewrite.
// CheckRaddleProposal is the one D6 check of a proposal on disk, run over a RaddleCheck whose Board builds the context from the board, the raddle store and the profiles, and RecheckRaddleBoard is the one D8 re-check.
// A proposal that is missing or does not parse is the finding `proposal-unparsable`.
// NewRaddleGate maps the check onto the gate contract and calls its onPass on a pass.
// StoreColdProposal stores the run's first passing proposal as the cold proposal once and never overwrites it.
//
// The Raddle-Write row's producer (NewRaddleWrite) wraps the table-driven raddle session.
// The run's first call records its mode from the `raddle_auto_approve` key, which every later call and Raddle-File read from the state, and every call raises the session count.
// A fresh entry, a Raddle-File return or an operator goto, sets the last attempt's output aside once per history position: auto mode moves the proposal to the returned-proposal file, and interactive mode removes the approval record and clears the state's approval.
// On Done it commits the raddle directory.
//
// The Raddle-File row's producer (NewRaddleFile) is a Go row with no agent.
// It runs the proposal's checks, the approval match in interactive mode and the board re-check, unless the raddle store already holds the filing, and sends a proposal that fails one back to Raddle-Write by deleting its stage and part records and counting a return; the failure after two returns halts the run for its parent.
// Its steps are staging the store files, writing the part records, filing the entries in one board write, making the project a beam and releasing its parts for the beam shape, notifying the parent once, and ending the run's entry.
// A step whose outputs carry the hash of the proposal now approved is skipped, so a retry after a crash finishes the steps it missed, and a failure after the filing is Stuck with Halt naming the resume.
//
// A rework round a run recorded before PR-Rework went stays readable.
// PlanReviewSkippable classifies the live plan generation from the committed round records, and answers false while the plan amendment record exists, so the amended plan is judged and its settle records a round start.
// ArchivedWebsterDirs lists each round's archived webster run record.
//
// InterruptPolicies also names the pawl recipe's Beam-Up, Beam-Await and Beam-Down rows as reinvoke:
// each spawns no agent and is idempotent, so the driver re-steps an interrupted one with no hand-back, and the arbiter admits one past the repair cap through InterruptPolicyFor.
// The agent Drift rows, Drift-Baseline, Drift-Expect and Drift-Verify, are reinvoke too: each probes for a live chair and reattaches to it rather than spawning a second.
//
// It declares its own unexported cancellation helpers (entryErr/cancelErr in ctx.go) rather than
// reusing internal/shedadapters' identically-shaped, unexported ones: shedadapters' versions are
// unexported and Scope forbids changing that package, so every real producer written here honours
// the two obligations Shed cannot enforce -- return exactly Done, Stuck or Awaiting, and surface
// context cancellation as a non-nil error, never as Stuck -- with its own copies. The duplication is
// deliberate, not an oversight.
package loomshed
