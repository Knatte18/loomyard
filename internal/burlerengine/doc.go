// Package burlerengine runs the two halves of one review+fix round over an artifact:
// a review half that returns a verdict and a fix half that repairs the findings.
// The name comes from burling and mending:
// the cloth-finishing step where a worker inspects woven fabric for defects and repairs them.
// A round splits that into two fresh sessions.
// Each is the chair of its own one-seat table on the seats engine (internal/seatengine).
// A reviewer finds the defects and writes the review.
// A fixer, spawned only once the review is accepted, validates the findings against the code and repairs them.
// Each half is its own shuttle run, and neither waits on, messages or knows of the other.
//
// A burler half runs once and exits.
// It knows nothing about round loops, caps, convergence, or progress across rounds; its caller composes those.
// The dependency runs one way, caller -> burler -> shuttle: each layer knows only the one below it.
// Burler is a separate module from its caller because the two test differently.
// One burler round is two LLM sessions, tested by a fake-shuttle unit suite plus opt-in real-engine smoke tests.
// The loop owner is deterministic Go, tested with a fake burler returning scripted results and no LLM.
//
// Told-geometry tier: burlerengine is a producer.
// It is told the absolute paths it operates on through its Geometry struct (WorktreeRoot, AnchorPath).
// It derives none of its own and requires none of the three resolution tiers.
// It therefore runs in a directory that is not a git repository.
// internal/hubgeom (hub mode) and internal/standalonegeom (told mode) are Geometry's only constructors.
// This property is a review obligation, not machine-enforced:
// no import-allowlist test polices the absence of internal/lymcwd.
// See PATTERN-cwd-resolution.
//
// # Skills and the parent directive
//
// Both seats load the skills agents.yaml lists for role `burler` through their spawn specs.
// A half whose `Profile.LanguageSkills` marks a code round loads the language skills told to `New` after those.
// They load on every seat, with repeats dropped; any other half loads the role's skills alone.
// The seats engine renders the parent and edit directives into each seat stencil once.
// The chair's parent directive names Geometry.ParentName.
// hubgeom.BurlerGeometry fills it from the worktree's origin record; standalone geometry leaves it empty.
// Left empty, it renders the no-parent variant.
// The instruction files carry no directive.
//
// # The seat tables
//
// reviewTable and fixTable each build a one-seat seatengine.Table:
// the review segment, RunOpts.Timeout and RunOpts.Round, not interactive, and the half's agent as the chair.
// The one seat has no name.
// The seats engine forms the strand roles through seatengine.SeatRole:
// <shortname>:<slug>:burler-review and <shortname>:<slug>:burler-fix.
// A Profile's RolePrefix leads both roles:
// <shortname>:<slug>:<prefix>-burler-review and <shortname>:<slug>:<prefix>-burler-fix.
// A standalone run sets no prefix.
// New builds the seats engine from the told Geometry and the stencils directory.
// The engine runs over the seatengine.Shuttle New is given.
// Each seat's stencil markers ride the seat's values:
// the absolute instruction-file paths and the review path, and for the fix half the review format path.
// composeReviewPrompt and composeFixPrompt fill the instruction files and return those values;
// the seats engine fills the seat stencils.
// Both read each instruction stencil through stencilstore.ReadSet for the half's engine.
// That engine is the one its model choice names, else Geometry.DefaultEngine.
// An engine block an instruction stencil includes therefore renders that engine's text.
// RunOpts.Review and RunOpts.Fix pick each half's engine, model, effort and version.
// RoundModels' per-round lists carry each entry's engine, so a later round can run on another engine than the first.
//
// # The review half
//
// Engine.Review runs one fresh reviewer session as the chair of the review table.
// The reviewer explores the target, judges it against the fasit and writes the review file;
// its write surface is the review file alone, in every fix scope, and it never edits, creates or deletes a target file.
// Its spec declares the review file as its only output file.
// Its gate is the review-parse entry (ReviewGateEntry) alone.
// It sets ForkSubagents exactly when the profile names a cluster fan.
// The review half's files are the explore step, the review step and the review-file format.
// They are written to a fresh directory under the anchor's .lym.
// The reviewer's prompt carries the PATTERN directive; it carries no friction directive.
// acceptReview accepts a done reviewer's end:
// the cluster audit for a fanned round, then ParseCurrentReview on the review file.
// Its ReviewResult carries the verdict, the findings and the Digest, the hex SHA-256 of the accepted bytes.
// An unparseable review after the review gate's budget is the strict-parse error, never a gate-failed outcome.
//
// # The assistant reviewer
//
// RunOpts.Assist names an assistant reviewer for the round.
// A model choice with a model adds a seat named AssistSeatName to the review table, on any engine the choice names.
// Its strand role is the review role followed by -assist.
// The chair then runs burler-template-reviewer-assisted, which includes seat-directive-assistant-chair.
// Its review step renders the burler-assist-consolidation part after the cluster rules:
// the chair waits for the assistant's end marker and judges a done assistant's findings with a fork's skepticism.
// It keeps the findings that hold, each with `origin: assist:<seat name>`, and rejects false ones.
// The verdict stays the chair's.
// A round with no assistant keeps burler-template-reviewer and the review step's bytes.
// The assistant runs burler-template-assist-reviewer, which carries the engine-assist-reading block of its engine.
// It reads its own explore and review steps, composed a second time for its engine and its report path.
// It reads the same fasit, rubric, PATTERN directive, focus and prior rounds as the chair.
// RunOpts.AssistFan fans the assistant out through its engine's own subagents inside its one turn.
// burler-assist-cluster-fan and the engine-assist-fan block drive the fan.
// The seat authorizes no fork, so no lym fork audit covers the subagents.
// A fanned assistant runs burler-template-assist-reviewer-fanned in place of burler-template-assist-reviewer.
// Its write surface and seat directive name its lens reports and say its subagents are its own work.
// Its report and end marker sit under the anchor's .lym at the review path's subpath.
// They are named from the review's stem, the attempt token RunOpts.Round and the seat.
// So a retry never shares the first attempt's files.
// Its gate is the review-parse entry on its report, and RunOpts.AssistTimeout bounds its run.
// RunOpts.AssistSkip keeps the assistant from starting, as does an assistant whose files cannot be composed.
// The reason goes in its marker.
// Once the chair has ended, fresh or resumed, a done assistant's report is copied beside the review.
// AssistCopyPath names the copy.
// ReviewResult.Assists says what became of each assistant.
// It names each lens a done fanned assistant returned no report for.
// No verdict or finding reads the copy.
// ProbeReview and ResumeReview build the same table from the same RunOpts.
// So the caller passes the recorded token and assistant.
//
// # Parent-review asks
//
// A run's asks to its parent orch to review a round live in a never-tracked record that ParentReviewFilesOf locates.
// RecordParentAsk numbers each ask from 1 up in recording order.
// An ask moves only from pending-post to open and from either to an end state: done, timeout, no-parent or not-sent.
// Every move takes the record's one lock, so an answer and a timeout never both land.
// AnswerParentAsk writes the parent's report beside the record and sets the latest ask done.
// It numbers the report's findings P1 up and refuses with a ParentAnswerRefusal that names its kind.
// An ask at or past its deadline is set timeout instead.
// ExpireParentAsk sets the latest active ask timeout once its deadline has passed.
//
// # The fix half
//
// Engine.Fix runs one fresh fixer session as the chair of the fix table.
// The review's bytes must hash to the digest Fix is told.
// Fix reads the review through acceptedReview first.
// A review edited after acceptance is ErrReviewChanged and never reaches a fixer.
// The same check runs again after a done fixer.
// A review with findings of any severity, a NIT included, spawns the fixer.
// A review with none runs the round's gates once through checkGates.
// When they pass, Fix writes a normal fixer report saying nothing was fixed.
// It returns a skipped OutcomeDone with no session.
// When one fails, the fixer is spawned with the failing gates' findings in a gate-findings file as its work.
// A gate exhausted in a round leaves its gate-failed edits in the target.
// The re-review of that round may find nothing.
// A gate closure that errors is returned wrapped with nothing spawned.
// RunOpts.DecisionsRouted marks a caller that sends the review's decision-class findings to the run's parent.
// A fixer can neither ask for nor record the call.
// Fix then counts those findings as no work:
// a review holding only them is handled as one with none, its skipped report naming the routing;
// a spawned fixer's work line says they are not its own until the parent rules on one.
// A ruling that reaches the fixer during the round makes the ruled finding its work.
// The ruling arrives as parent mail or as a decision notice.
// The fixer carries it out.
// It flags a ruling that is redundant or mistaken against the code with one creel post to its parent.
// Either way the fixer-report records the ruling in its own Rulings section.
// The fixer reads the review, validates each finding against the code, and fixes it.
// Its spec declares the fixer-report as its only output file.
// Its gate is RunOpts.Gate with every entry wrapped by repairReportBeforeGate.
// The fixer's files hold the review-file format and the fix step.
// It carries no rubric, fasit, PATTERN directive, focus file, prior rounds or explore step,
// so `premise false` means false against the code it reads.
// The fixer is the round's only friction-note writer.
//
// # The fixer fan (twill fixer)
//
// Profile.FixFan names a fan from the lenses.yaml lens/fan library.
// validateFix resolves it into the fixer's lenses before any spawn, so an unknown fixer fan fails the half.
// A profile with no FixFan runs the single fixer.
// With a fan, the fixer is spawned with fork subagents authorized,
// and its fix step renders the burler-fix-self-check part before the fixer-report rule.
// Before it writes the fixer-report, the fixer reads the deployed burler-fix-check-rubric stencil.
// It spawns one read-only fork per lens over the change set it just made.
// It waits for them inside `lym shuttle await-forks`, so their results reach it in one turn.
// It edits nothing until the last fork has returned.
// It then saves each fork's final message verbatim in a lens report beside the fixer-report.
// It judges all fork findings together by the fix step's own dispositions.
// It records the result in a `## Self-check` section of the fixer-report.
// A fixer re-prompted by its gate, or resumed after its forks were lost, spawns no further forks.
// It names each lost fork as "no report".
// A forking fixer still carries its row's gates.
// A turn end with a fork outstanding is a wait, so no gate runs while a fork is live.
//
// A done twill fixer is audited by auditFixRound before its gate is looked at.
// A fork that wrote, ran a mutating git command, called the Agent tool or was spawned with a name is a hard error.
// The error carries a way forward.
// That holds whether or not the gate passed, since the fork may have corrupted the fix.
// A fork count other than the lens count, and a fork that never returned a report, are warnings only.
// The fix is on disk, and failing the half would redo a finished fix.
// FixResult carries the fixer fan's Lenses, the audited Forks count and the Warnings.
// twillEngineRefusal words the start refusal of an engine without fork support.
// It names the model key and the fan key that RunOpts.FixKey and RunOpts.FixFanKey carry.
// A fan recorded for the round that does not resolve carries the way forward: define it in lenses.yaml.
//
// # Review before fix
//
// Review-before-fix is a hard gate:
// the review is on disk, and accepted by Go, before the fix half spawns, so no target file is touched earlier.
// Fixing findings as they are spotted turns the review into a post-hoc rationalization of edits already made.
// That destroys the independent judgment the method depends on.
// No file or message rides between the halves;
// the digest the review half returns carries the acceptance, and the caller records it and hands it to the fix half.
// The fix half also refuses a review whose bytes changed since its acceptance.
// Nothing in a round is self-graded:
// a round ends on a verdict, and each fix is its own commit on the code branch, never pushed.
// A review segment converges only on a judge verdict.
// From round 2 on, a fix of a key first raised in the closing round may converge.
// Such a finding is carried to the decision record's `## Open risks`.
// The fixer holds neither the fasit nor the rubric, so neither is a dispute ground:
// the reviewer applies both before it writes, and the judge rules on a finding that breaks them.
// A finding's class decides who decides and when the loop stops, never whether it is fixed.
// The prompt assets state this rule to each agent every round.
// The assets are the seat stencils `burler-template-*`.
// They also include the instruction files `burler-step-*` and `burler-review-format`.
// The prompts ship as embedded defaults in the top-level stencils package.
// They are read at call time via stencilstore.Read from the hub's stencils directory (fabricengine.StencilsDir).
// They are never read from a compiled-in copy.
// Each half renders its instruction files per call into a fresh directory under .lym, anchored at AnchorPath.
// That directory is a sibling of the durable _lym tree (via lymdirs.DotLymDirName; machine-local, never committed).
// The half then runs its seat from its stencil.
// The stencil names the files' absolute paths, so the agent reads each step's rules on reaching that step.
// A half never prunes these per-call directories, so they accumulate under .lym/burler in a long-lived worktree.
// That is accepted machine-local litter:
// .lym is never committed, and whatever clears a worktree's .lym tree removes them.
//
// # How a half ends
//
// concludeReview and Fix's conclusion are each the one place their half's failure rules live:
//   - A reviewer whose provider never came up is OutcomeDied with NotStarted, Review.StartError and a nil error;
//     a fixer's likewise, with Fix.StartError.
//   - A half that dies or times out returns its outcome with a nil error.
//   - A done reviewer whose review gate failed is an error naming the gate, never a verdict.
//   - A done reviewer whose cluster audit or strict parse fails is that error, returned with the outcome and fork audit.
//   - A done fixer returns its Gate, passed through; a failing fixer gate returns the fixer's Gate with a nil error.
//   - A done fixer whose review file no longer holds the accepted bytes is ErrReviewChanged.
//
// The seats engine stops every seat before it returns, through each handle's Stop.
// Stop records the stop in the seat's run record before it removes the strand,
// so a later attach probe reads the seat as stopped and never halts on it.
// A seat that timed out or whose wait returned an error is stopped as well, since it may still be running.
// A seat that cannot be stopped is seatengine.ErrSeatNotStopped, returned wrapped in ErrHalfNotStopped beside it.
// A caller matching either error sees it.
// The seat is never retried or archived over.
// The message ends with the way forward: run "lym reed remove <guid>", then re-step the row.
// Any other start error is a pre-strand failure, returned wrapped.
//
// # Resuming a half
//
// A caller that finds a previous session's half on disk asks ProbeReview or ProbeFix what became of it.
// A probe does not wait on the half.
// A probe builds the half's table with the same builder and runs the seats engine's probe over it.
// A half is live when the shuttle holds a run for its seat.
// That holds even when its output file is on disk, as for a reviewer repairing its review in the parse gate.
// A half is done when it has no live run and its output file is present, and gone otherwise.
// A seat that cannot be stopped is returned wrapped in ErrHalfNotStopped.
// The LiveHalf it returns carries the half's state and, for a live half, its Handle.
// A caller stops the half by the Handle's strand guid.
// LiveHalf also holds, unexported, what the probe found, which ResumeReview or ResumeFix hands back to the seats engine.
// ResumeReview and ResumeFix attach to a live half:
// they start nothing, and the resumed half ends as a fresh one does.
// ResumeFix first asks a live handle that can deliver a pending task prompt to do so.
// Thus a fixer whose step died between its skill-load turns and its prompt still receives its task.
// A delivery error is the resume's error, and nothing is resumed.
// A LiveHalf that is not live, or one that did not come from the half's probe, is refused;
// nothing is started or stopped.
// The caller settles it by stopping what is live through the handle's Stop and running the half fresh.
// A live half is never started a second time.
//
// # What the fixer fixes
//
// Every recorded finding is fixed by the fixer, all severities including LOW and NIT.
// Severity affects how a finding is reported, never whether it gets fixed.
// Leaving low-severity findings unfixed "because they're just nits" is a known failure mode:
// unfixed nits re-surface or silently vanish across rounds instead of ever closing.
// Round count then goes up instead of down.
// Two exceptions exist.
// A finding is disputed, with cited evidence in the fixer-report's Disputed section, on one of two grounds:
// its premise is false against the code, or the premise holds but every change that addresses it makes the target worse.
// One sound change means the finding is fixed, not disputed.
// A finding whose only sound change leaves the fixer's write surface is deferred.
// When the premise holds and the proposed change is wrong, the fixer fixes with its own change.
// It marks the finding as fixed differently in the fixer-report's list of changes.
// Severity, size, cost or disagreement with the rubric never justify a dispute.
// A dispute never converges a segment on its own.
// Something the round cannot do alone is named, with its reason, in the deferred section.
// A reviewer that re-raises a disputed finding answers the dispute's evidence in that finding's Issue.
//
// # Finding class
//
// Every finding carries a required Class beside its severity, one of four.
// Design means the design is wrong, or a decision is missing or rests on a false premise.
// Scope means the work missed a call site, file or case.
// Decision means a choice is left open that the author must make.
// Consistency means two parts disagree, or the artifact departs from its own conventions.
// Each loom rubric says what design means for its own segment.
// A recurring scope gap is raised once, as a design finding about the method that keeps missing it.
// GatingClass (design) is the one class that decides when a review loop stops, in every segment.
//
// Class is not a severity ladder.
// It decides who decides and when the loop stops, never whether a finding is fixed:
// the fixer still fixes every finding of every class and severity.
//
// Class exists because a loop that counts every blocking finding does not converge.
// Design findings converge to zero.
// Scope findings ("you missed these call sites") recur.
// The author keeps adding the newly named files instead of fixing the cause:
// hand-enumerating call sites from greps is not a reliable method.
// Such findings enumerate one constant's consumers, which `go build ./...` reports exhaustively, instantly and for free.
// So the loop stops on a round with no design finding rather than on a flat cap.
// Scope splits into two mechanical halves, neither an LLM lens.
// Symbol references are the first: the compiler, or a code-index references query before a deletion, for plan sizing.
// Bare string literals with no symbol behind them are the other, which neither sees.
// A literal scan such as `TestEnforcement_GeometryLiterals` covers them narrowly.
// An exclusion has to be written on both sides:
// telling the writer not to enumerate X while the reviewer still flags a missing X recreates the non-convergent loop.
// The trap to design against is reading class as a severity ladder.
// It files real problems under a low class to dodge the fix-everything default.
//
// A single burler round never grades its own fix.
// The review precedes the fix within a round, so the reviewer is an independent gate like any other reviewer.
// The fix FROM round N is judged by a FRESH reviewer in round N+1, not by the same round that made it.
// That cross-round independence is the caller's discipline (it spawns a new review half each round);
// a single round only guarantees review-before-fix within itself.
//
// # Profile vs RunOpts
//
// Profile is the content contract for one round: what to review (Target) and what to judge it against (Fasit).
// It also carries the criteria (Rubric, mapped onto the fixed Severity vocabulary).
// Its write-surface discipline is FixScope.
// Its remaining fields say whether the round may drive the real substrate (ToolUse).
// Another field names the cluster fan (ClusterFan).
// They also give the caller-named output paths and optional prior-round hydration paths.
// An empty Fasit degenerates the round to a pure internal-consistency check, which validateReview rejects.
//
// RunOpts stays off the content Profile.
// Run-tuning is a caller-resolved, config-driven selection that varies per invocation;
// a caller may vary each half's model and effort per round of the same artifact.
// Profile describes what does not change about the round's content.
// Each half maps RunOpts onto its seat's shuttle Spec through its table builder, and the tables are not interactive:
// rounds are autonomous by default.
//
// # FixScope: overlay vs source
//
// FixScope selects the fixer's write surface and git discipline, content-agnostic:
// a burler improves code, text, or any artifact, and the split is never about file type.
//
//   - FixScopeSource: the target is the repo's own files.
//     The fixer's write surface is the working tree.
//     It commits each fix individually once green and never pushes.
//     The message format is "<module-or-target>: fix <finding-id> — <one-line what/why>".
//     If the round dies mid-fix, git log shows which findings landed.
//   - FixScopeOverlay: the target is lym system/orchestration state, such as plan, discussion and review artifacts.
//     It is reached through the _lym junction.
//     The fixer's write surface is Target.Paths plus the fixer-report, never the review file.
//     The round runs no git commands at all:
//     the Fabric Git Invariant reserves committing that class of file to the loop owner, never an agent.
//
// Any other FixScope value, including empty, is a validation error:
// the field selects safety-critical behavior and gets no silent default.
//
// # Fabric-blindness
//
// burlerengine never imports the fabric module and never constructs a _lym/... path;
// the results return the review and fixer-report paths the caller supplied, resolved absolute.
// Committing them is the loop owner's job, via the fabric engine in-process.
// See PATTERN-fabric-boundary.
// The one exception is an agent's own code under FixScopeSource, an ordinary repo commit rather than a fabric operation.
//
// # Cluster fan-out (fork subagents)
//
// ClusterFan names a fan from the lenses.yaml lens/fan library.
// The lenses package owns the library and registers it in internal/configreg.
// Naming a fan activates clustering:
// the fan's entry count becomes the fork count, one fork per listed lens, in fan order (repeats allowed).
// An empty ClusterFan is a single-reviewer round, the default, since forking is off unless a profile names a fan.
// A fan longer than lenses.MaxFanLenses entries fails validation.
// No fan is named "default": every templated fan is dormant until a profile names it.
// A fan resolves against the loaded lenses.yaml, whose load fills a missing template lens or fan,
// so an operator-defined name wins and an absent lenses.yaml resolves wholly from the template.
// The embedded template ships production fans named `review-*` and `fix-*`.
//
// lenses.FanList is the value of a module's per-round fan key: one fan name, or a list of them.
// The last entry serves every later round, and an empty entry runs its round solo or with the single fixer.
// Its decoder refuses any other shape with a lenses.FanShapeError carrying the value's line.
// lenses.ValidateFanKeys refuses an empty list and resolves every named fan through lenses.yaml.
// Each refusal names the module, the key and the entry.
// ModelSpecList is the matching value of a reviewer or fixer model key: one model-spec, or a per-round list.
// Its decoder refuses any other shape with a ModelSpecShapeError carrying the value's line.
// Its Unset reports the empty value, which a segment key reads as "take the shared list".
// ValidateModelSpecList refuses an empty list and an entry that is no model-spec.
// Each refusal names the module, the key and the entry.
//
// A fork reviewer and a fix-check fork may run git commands that only read.
// Examples are `git diff`, `git log` and `git show`.
// A fan thus reaches a subject that exists only through git.
// The fork stencils forbid every git command that changes state, and the round audits refuse a fork that ran one.
//
// ClusterExclude names lenses to drop from the fan ClusterFan resolves to.
// It is applied inside validateReview after lenses.ResolveFan, and the survivors are stored in clusterLenses.
// Both prompt composition and auditClusterRound's exact-N fork check read clusterLenses,
// so a trimmed round demands exactly the forks it named and ErrClusterForksMissing stays fail-loud.
// Three edge cases split on who authored the input.
// ClusterExclude set with an empty ClusterFan is a validation error, because that is a Go caller's mistake.
// A name not present in the resolved fan is a no-op for that name with a warning:
// an exclusion list is an advisory, per-call directive over a config-owned fan,
// and an operator may edit the fan between rounds, so a stale name is stale rather than wrong.
// An exclusion that would empty the fan falls back to ClusterExcludeHeld:
// the exclusions carried from earlier rounds of the generation.
// When that would empty the fan too, the whole fan runs.
// Dropping to zero lenses is never what "these found nothing last round" meant.
// Running more lenses costs tokens, never correctness.
// ClusterExcludeHeld set with an empty ClusterFan is a validation error too.
//
// A cluster round runs its review as one shuttle session, the handler, in three phases.
// Phase 1: the handler explores the target in full.
// Phase 2: the handler spawns all N lens forks in a single message via Claude Code's built-in fork subagents.
// The forks use the Agent tool with subagent_type "fork" and are always unnamed.
// While they run, the handler performs its own holistic review:
// architecture, cross-file invariants and PATTERN-fit, the level no narrow lens covers.
// Phase 3: the handler waits for the forks inside `lym shuttle await-forks` rather than by ending its turn,
// so it takes every fork's result in one turn instead of waking once per fork.
// The handler saves each fork's final message verbatim in a lens report.
// It does so once the last fork has returned, and never before.
// The report sits beside the review file and is named by internal/lensreport.
// It follows the twill-lens-report-spec spec under Geometry.SpecsDir.
// The handler then consolidates every fork's returned findings and its own holistic findings into the one review file.
// It dedups across lenses and orders by severity.
// Every kept finding carries an `origin:` frontmatter key, lens:<name> or handler.
// False positives go in a ## Rejected prose section, judged with equal skepticism.
// That section never appears in the parsed findings.
// All three phases are part of the review, so review-before-fix is intact as in a solo round:
// the consolidated review is written to disk, and accepted, before the fix half spawns.
//
// The prose of a cluster round is stencil text read at call time, never composed in Go.
// The explore step's optional fork_reading marker renders the burler-fork-reading stencil in a fanned round only.
// So the handler's pre-read is the forks' inherited reading.
// A solo round's explore step renders the same bytes as without the marker.
// That stencil has the handler gather its pre-read in waves of parallel calls, each wave one message.
// It does not read one item a turn.
// The review step's cluster_rules marker renders burler-cluster-solo for a solo round.
// For a fanned round it renders burler-cluster-fan, filled with the round's lens list.
// The fanned part carries the spawn mechanics, the fork rules and the consolidation rules.
//
// Fork discipline is fixed boilerplate the handler composes into every fork's prompt, never per-lens:
// read-only evidence gathering only (no Write/Edit/delete of any file, repo or `_lym`);
// git commands that only read, such as git diff, git log and git show;
// no git command that changes the repository, its index, its refs or its working tree;
// no touching the round's ReviewPath/FixerReportPath;
// and no nested Agent calls (forks cannot spawn forks).
// auditClusterRound in cluster.go enforces this discipline mechanically instead of trusting the prompt alone.
// It runs once the run reaches shuttleengine.OutcomeDone.
// It reads the per-fork AgentCalls, WriteCalls and BashCommands facts of the run's shuttleengine.ForkAudit.
// It never uses this package's own knowledge of the transcript layout.
// It enforces a fail-loud posture:
// exactly len(clusterLenses) fork transcripts or ErrClusterForksMissing, naming requested versus actual;
// a shortfall, including zero, is an infrastructure defect, never a degrade-to-solo;
// any fork with AgentCalls > 0, WriteCalls > 0, or a git-mutating Bash command is a hard error;
// and any named spawn is a hard error.
//
// A Bash command is git-mutating only when the subcommand word of one of its git invocations mutates.
// That word is the first non-option word after git's global options.
// A mutating verb named as an argument passes, as in git help cherry-pick or git log --grep merge.
// A -h or --help request passes too.
// git tag is judged by its arguments:
// it passes when bare or in list mode, as in git tag --points-at HEAD.
// No option may create, delete or sign a tag.
// git branch is judged by its arguments too.
// It passes when every option only reads and it either has no operand or is in list mode.
// Examples that pass are git branch -a --contains HEAD and git branch --show-current.
// A branch name beside -a, -r, -v or --format alone still counts, since git creates that branch.
// A redirection such as 2>/dev/null, with its target, is no word of the command, so it is never read as an operand.
//
// A mutating invocation also passes when it acts inside a repository the fork itself created earlier.
// The fork creates one with git init or with git worktree add from such a repository.
// A fork may therefore experiment on a throwaway repository in its scratchpad.
// The directory is resolved lexically from each command's cd segments and -C options.
// git push, --git-dir, --work-tree and an unresolvable directory never pass this way.
// A git init whose relative path is the starting directory, an ancestor or leaves it through .. records no repository.
// A .. climbing above a leading ~ or $ word leaves the directory unresolvable.
//
// A fork that ran clean but never returned a report is sloppiness no mechanism prevents in advance.
// It is collected into ReviewResult.ClusterWarnings and never fails the round.
// The handler's own consolidation phase already judges each fork's output on its merits.
//
// Run mechanics: forks run in the handler's own shuttle session, under the handler's own model.
// There is no model-per-fork axis, a Claude Code constraint.
// There is no separate tmux pane or window per fork either.
// shuttleengine.Spec.ForkSubagents authorizes forks for the run;
// claudeengine sets CLAUDE_CODE_FORK_SUBAGENT=1 inline on the launch line.
// It never sets it on the reed server's own environment.
// Reed's server spawn scrubs the keys the engines declare from the server environment at boot;
// for claude those are CLAUDECODE and CLAUDE_CODE_*.
// The launch line runs after that scrub, which is the only place a per-run, staged-rollout flag can ride.
//
// Version pinning: CLAUDE_CODE_FORK_SUBAGENT is a staged-rollout flag requiring Claude Code v2.1.117+.
// Forks must stay unnamed:
// named forks silently lose their inherited context in Claude Code releases up to and including 2.1.206.
// A Claude Code upgrade or downgrade should be checked against both facts before trusting a cluster round's output.
//
// Timeout guidance: cluster rounds get no automatic timeout scaling.
// RunOpts.Timeout is the same caller-resolved, per-invocation knob a solo round uses,
// so a wider fan needs an explicitly longer timeout from the caller.
// Forks queue under Claude Code's own concurrency cap (min(16, cores−2)) rather than running unboundedly parallel,
// so a low-core host serializes a wide fan.
// Every fork still runs and leaves its transcript, only wall time grows;
// the exact-N contract holds.
// A slow host surfaces as shuttleengine.OutcomeTimeout, never as a fork-shortfall error.
//
// Cluster rounds weaken nothing about fabric-blindness:
// forks write no files, returning findings only as their final message,
// so this package still never imports fabric and never constructs a _lym/... path.
//
// # What the halves return
//
// ReviewResult is an invariant contract regardless of what was reviewed.
// It carries a Verdict (VerdictApproved or VerdictBlocking), the parsed Findings and the Digest.
// ParseReview enforces unique, non-empty ids fail-loud.
// That lets cross-round hydration and audit cite a finding unambiguously.
// The caller judges progress across rounds holistically via its own verdict judge.
// It does not track finding-key identity.
// It also carries the resolved ReviewPath, ForkAudit and ClusterWarnings.
// ReviewResult.Review holds the reviewer's SessionID, StrandGUID, LastAssistantMessage and RunDir.
// Its StartError is set only on a half that never started.
// Gate is the reviewer's review-parse gate outcome passed through, nil when the reviewer never ran gated.
// SessionStates is the chair's session states passed through on every ending, the audit error included,
// so a caller can say how the chair ended.
// FixResult carries the fixer's Outcome, the resolved FixerReportPath and the fixer's Half.
// It further carries its Gate passed through, Skipped and NotStarted.
// For a twill fixer it also carries Lenses, Forks and Warnings.
// Each Half also carries its run's StartedAt, EndedAt and session Usage, forks included.
// Those come from the terminal shuttle Result.
// ReviewResult.Lenses names the lenses the reviewer ran, nil for a solo round.
// No decision reads these.
// Review and Fix return a nil error for every shuttleengine outcome except a hard failure.
// A hard failure is an invalid profile, a shuttle start failure, a half that cannot be stopped or a changed review.
// A verdict parse failure on a done review is loud too:
// a defaulted verdict could silently terminate a caller's round loop on a malformed round.
// A died or timeout outcome is a normal loop event with an empty Verdict.
// A caller branches on it via the result's Outcome.
//
// # The review-parse gate
//
// The reviewer's gate spec is the review entry (ReviewGateEntry) alone, and the caller's own entries gate the fixer.
// The entry re-prompts the reviewer in its own session while the round's review file does not parse.
// The prompt carries the parse error, its quoting hint and the file to rewrite.
// It re-prompts at most reviewGateAttempts times and then lets the run through,
// so a file still invalid after the budget fails at the strict parse in acceptReview.
// The gate (through CheckReviewFile) and that parse both reach ParseCurrentReview.
// It is the strict parse of the current round's review:
// ParseReview plus a non-empty `fix` on every finding;
// `verdict` and `findings` as the frontmatter's only top-level keys;
// and one `### [SEVERITY]` prose block per finding, matching the frontmatter's count per severity.
// ParseReview tolerates a missing `fix`, so an older round's review still hydrates and still feeds a Bouncer's evidence.
// `lym burler validate-review <review-file>` is the gate's self-check verb:
// it runs CheckReviewFile read-only and needs no hub, mode or git repository.
package burlerengine
