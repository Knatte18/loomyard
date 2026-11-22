// Package burlerengine runs the two halves of one review+fix round over an artifact:
// a review half that returns a verdict and a fix half that repairs the findings.
// Each half is a fresh session, the chair of its own one-seat table on the seats engine (internal/seatengine).
// Each half is its own shuttle run, and neither waits on, messages or knows of the other.
//
// A burler half runs once and exits.
// It knows nothing about round loops, caps, convergence, or progress across rounds; its caller composes those.
// The dependency runs one way, caller -> burler -> shuttle.
//
// Told-geometry tier: burlerengine is a producer.
// It is told the absolute paths it operates on through its Geometry struct (WorktreeRoot, AnchorPath).
// It derives none of its own and requires none of the three resolution tiers, so it runs outside a git repository.
// internal/hubgeom (hub mode) and internal/standalonegeom (told mode) are Geometry's only constructors.
// This is a review obligation, not machine-enforced: no import-allowlist test polices the absence of internal/lymcwd.
// See PATTERN-cwd-resolution.
//
// # Skills and the parent directive
//
// Both seats load the skills agents.yaml lists for role `burler`.
// A half whose `Profile.LanguageSkills` marks a code round also loads the language skills told to `New`.
// The chair's parent directive names Geometry.ParentName.
// hubgeom.BurlerGeometry fills it from the worktree's origin record.
// Standalone geometry leaves it empty, which renders the no-parent variant.
//
// # The review half
//
// Engine.Review runs one fresh reviewer session.
// The reviewer explores the target, judges it against the fasit and writes the review file;
// its write surface is the review file alone, in every fix scope.
// Its gate is the review-parse entry (ReviewGateEntry), then the round-check entry (RoundCheckGateEntry).
// A done reviewer's end is accepted by `acceptReview`:
// the cluster audit for a fanned round, then ParseCurrentReview on the review file.
// ReviewResult carries the verdict, the findings and the Digest, the hex SHA-256 of the accepted bytes.
// An unparseable review after the gate's budget is the strict-parse error, never a gate-failed outcome.
//
// # The assistant reviewer
//
// RunOpts.Assist adds an assistant reviewer seat to the review table, on any engine its model choice names.
// The chair waits for the assistant and judges its findings with a fork's skepticism.
// It keeps those that hold, each with `origin: assist:<seat name>`.
// The verdict stays the chair's.
// RunOpts.AssistFan fans the assistant out through its engine's own subagents; no lym fork audit covers them.
// RunOpts.AssistTimeout bounds its run, and RunOpts.AssistSkip keeps it from starting.
// ReviewResult.Assists says what became of each assistant.
// A done assistant's report is copied beside the review at AssistCopyPath; no verdict or finding reads the copy.
// ProbeReview and ResumeReview build the same table from the same RunOpts,
// so the caller passes the recorded token and assistant.
//
// # Parent-review asks
//
// RunOpts.ParentAsk names the round's ask to its parent orch.
// Asks live in a never-tracked record that ParentReviewFilesOf locates.
// An ask moves only from pending-post to open and from either to an end state: done, timeout, no-parent or not-sent.
// Every move takes the record's one lock, so an answer and a timeout never both land.
// The chair keeps every finding of a done parent report, tagged `origin: parent` (ParentOrigin).
// It lists the report ids it covers in `parent_ids` (Finding.ParentIDs).
// ParentCoverageGateEntry fails naming each report id no parent finding covers, and never passes on its cap.
// It first sets a still-open ask timeout, so a late answer is refused once the chair is done.
//
// # The fix half
//
// Engine.Fix runs one fresh fixer session.
// The review's bytes must hash to the digest Fix is told;
// a review edited after acceptance is ErrReviewChanged and never reaches a fixer.
// A review with findings of any severity spawns the fixer.
// A review with none runs the round's gates once.
// When they pass, Fix writes a fixer report with `spawned: false` and returns a skipped OutcomeDone with no session.
// When a gate fails, the fixer is spawned with the failing gates' findings as its work.
// The fixer's write surface is the fixer-report alone beyond the target scope (see FixScope).
// Its gate is RunOpts.Gate, then the fixer-report entry.
//
// RunOpts.Rerun marks a fix that runs again over a review an earlier fixer session worked.
// The fixer's work then says it runs again, not for new work, why, and where the earlier report was archived.
//
// RunOpts.DecisionsRouted marks a caller that sends decision-class findings to the run's parent.
// Fix then counts those findings as no work until the parent rules on one.
// A ruling that reaches the fixer makes the ruled finding its work, and the fixer-report records it under Rulings.
//
// The fixer-report opens with `---`-delimited YAML frontmatter over its prose.
// `dispositions` holds one entry per finding the fixer owns:
// fixed, fixed-differently, disputed, deferred or ruling.
// ParseFixerReport is the sole parser of that frontmatter and refuses any top-level key but `dispositions` and `spawned`.
// Only the engine writes `spawned`.
// CheckFixerReport judges a fixer-written report against the accepted review's findings.
// FixerReportGateEntry lets the round through on its cap,
// so a slip in the frontmatter never fails the fix half, archives a round or spends a bounce.
//
// The fixer carries no rubric, fasit, PATTERN directive, focus file or prior rounds,
// so `premise false` means false against the code it reads.
// The fixer is the round's only friction-note writer.
//
// # The fixer fan (twill fixer)
//
// Profile.FixFan names a fan from the lenses.yaml library; an unknown fan fails the half before any spawn.
// With a fan, the fixer spawns one read-only fork per lens over its change set before it writes the fixer-report.
// It records the judged result in a `## Self-check` section.
// A done twill fixer is audited by `auditFixRound` before its gate is looked at.
// A fork that wrote, ran a mutating git command, called the Agent tool or was spawned with a name is a hard error,
// whether or not the gate passed.
// Its way forward says when the audit found no target change: no fork wrote a file or changed the target's repository.
// A wrong fork count or a fork that returned no report is a warning only, since the fix is on disk.
// Subagents the fixer starts to make its fixes are not audited; only the self-check forks are held to the read-only rule.
//
// # Review before fix
//
// Review-before-fix is a hard gate:
// the review is on disk, and accepted by Go, before the fix half spawns, so no target file is touched earlier.
// Fixing findings as they are spotted turns the review into a post-hoc rationalization of edits already made.
// No file or message rides between the halves;
// the digest the review half returns carries the acceptance, and the caller records it and hands it to the fix half.
// Nothing in a round is self-graded:
// a round ends on a verdict, and each fix is its own commit on the code branch, never pushed.
// A review segment converges only on a judge verdict.
// The fixer holds neither the fasit nor the rubric, so neither is a dispute ground:
// the reviewer applies both before it writes, and the judge rules on a finding that breaks them.
// The fix from round N is judged by a fresh reviewer in round N+1.
// That cross-round independence is the caller's discipline;
// a single round guarantees review-before-fix only within itself.
//
// The prompt assets are the seat stencils `burler-template-*`.
// They also include the instruction files `burler-step-*` and `burler-review-format`.
// They are read at call time via stencilstore.Read from the hub's stencils directory (fabricengine.StencilsDir).
// No compiled-in copy is used.
// Each half renders its instruction files per call into a fresh directory under `.lym` at AnchorPath.
// Nothing prunes those directories; `.lym` is machine-local and never committed.
//
// # How a half ends
//
// A half returns a nil error for every shuttleengine outcome except a hard failure.
// A hard failure is an invalid profile, a shuttle start failure, a half that cannot be stopped or a changed review.
//   - A half whose provider never came up is OutcomeDied with NotStarted, a StartError and a nil error.
//   - A half that dies or times out returns its outcome with a nil error.
//   - A done reviewer whose review gate failed is an error naming the gate, never a verdict.
//   - A done reviewer whose cluster audit or strict parse fails is that error, returned with the outcome and fork audit.
//   - A done fixer returns its Gate, passed through; a failing fixer gate returns it with a nil error.
//
// A defaulted verdict could silently terminate a caller's round loop on a malformed round,
// so a verdict parse failure on a done review is loud.
//
// The seats engine stops every seat before it returns, including a timed-out or errored one.
// A seat that cannot be stopped is seatengine.ErrSeatNotStopped, returned wrapped in ErrHalfNotStopped beside it.
// The seat is never retried or archived over.
// The message ends with the way forward: run "lym reed remove <guid>", then re-step the row.
//
// # Resuming a half
//
// A caller that finds a previous session's half on disk asks ProbeReview or ProbeFix what became of it.
// A probe does not wait on the half.
// A half is live when the shuttle holds a run for its seat, even when its output file is on disk.
// It is done when it has no live run and its output file is present, and gone otherwise.
// A caller stops a live half by the LiveHalf Handle's strand guid.
// ResumeReview and ResumeFix attach to a live half; they start nothing, and the resumed half ends as a fresh one does.
// A LiveHalf that is not live, or did not come from the half's probe, is refused.
// A live half is never started a second time.
//
// # What the fixer fixes
//
// Every recorded finding is fixed by the fixer, all severities including LOW and NIT.
// Severity affects how a finding is reported, never whether it gets fixed;
// unfixed nits re-surface or silently vanish across rounds instead of ever closing.
// Two exceptions exist.
// A finding is disputed, with cited evidence in the fixer-report's Disputed section, on one of two grounds:
// its premise is false against the code, or the premise holds but every change that addresses it makes the target worse.
// Severity, size, cost or disagreement with the rubric never justify a dispute.
// A dispute never converges a segment on its own.
// A finding whose only sound change leaves the fixer's write surface is deferred, with its reason.
// When the premise holds and the proposed change is wrong, the fixer fixes with its own change.
// It marks the finding fixed differently.
//
// # Finding class
//
// Every finding carries a required Class beside its severity:
// design (the design is wrong or a decision is missing);
// scope (the work missed a call site, file or case);
// decision (a choice is left open that the author must make);
// or consistency (two parts disagree).
// GatingClass (design) is the one class that decides when a review loop stops, in every segment.
// Class decides who decides and when the loop stops, never whether a finding is fixed.
// Filing a real problem under a low class to dodge the fix-everything default is the trap to design against.
//
// A loop that counts every blocking finding does not converge: scope findings recur.
// They recur because the author keeps adding the newly named files instead of fixing the cause.
// So the loop stops on a round with no design finding rather than on a flat cap.
// An exclusion has to be written on both sides:
// telling the writer not to enumerate X while the reviewer still flags a missing X recreates the non-convergent loop.
//
// # Profile vs RunOpts
//
// Profile is the content contract for one round: what to review (Target) and what to judge it against (Fasit).
// An empty Fasit degenerates the round to a pure internal-consistency check, which validateReview rejects.
// RunOpts holds run-tuning that a caller resolves from config and may vary per invocation.
// An example is each half's model and effort per round of the same artifact.
//
// # FixScope: overlay vs source
//
// FixScope selects the fixer's write surface and git discipline, content-agnostic:
// the split is never about file type.
//
//   - FixScopeSource: the target is the repo's own files.
//     The fixer's write surface is the working tree.
//     It commits each finding's fix on its own once green and never pushes.
//     The message format is "<module-or-target>: fix <finding-id> — <one-line what/why>".
//     Findings whose changes touch a file in common are fixed and committed together, the subject naming every id.
//   - FixScopeOverlay: the target is lym system state, such as plan, discussion and review artifacts,
//     reached through the _lym junction.
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
// ClusterFan names a fan from the lenses.yaml library, owned by the lenses package.
// The fan's entry count becomes the fork count, one fork per listed lens.
// An empty ClusterFan is a single-reviewer round; a fan longer than lenses.MaxFanLenses entries fails validation.
//
// ClusterExclude drops lenses from the resolved fan.
// The survivors are what prompt composition and the audit's exact-N fork check both read,
// so a trimmed round demands exactly the forks it named and ErrClusterForksMissing stays fail-loud.
// ClusterExclude or ClusterExcludeHeld with an empty ClusterFan is a validation error, a Go caller's mistake.
// A name not in the resolved fan is a no-op with a warning, since an operator may edit the fan between rounds.
// An exclusion that would empty the fan falls back to ClusterExcludeHeld.
// When that would empty it too, the whole fan runs: running more lenses costs tokens, never correctness.
//
// A cluster round runs as one shuttle session, the handler, in three phases:
// it explores the target;
// it spawns all N unnamed lens forks in one message while it reviews holistically;
// then waits inside `lym shuttle await-forks` and consolidates the forks' findings and its own into the one review file.
// Each fork's final message is saved verbatim as a lens report (internal/lensreport, twill-lens-report-spec).
// Every kept finding carries an `origin:` key, lens:<name> or handler.
// False positives go in a ## Rejected prose section, which never appears in the parsed findings.
// The consolidated review is accepted before the fix half spawns, so review-before-fix holds as in a solo round.
//
// Fork discipline is read-only evidence gathering:
// no write of any file;
// no git command that changes the repository, index, refs or working tree;
// no touching the round's ReviewPath or FixerReportPath, and no nested Agent calls.
// `auditClusterRound` enforces it once the run reaches shuttleengine.OutcomeDone, rather than trusting the prompt.
// It fails loud: a fork transcript count other than len(clusterLenses) is ErrClusterForksMissing;
// that is never a degrade-to-solo.
// Any fork with Agent calls, writes, a git-mutating Bash command or a name is a hard error.
// A fork that ran clean but returned no report is a ReviewResult.ClusterWarnings entry and never fails the round.
//
// A Bash command is git-mutating when the subcommand word of one of its git invocations mutates.
// A mutating verb named as an argument passes, and so does a -h or --help request.
// Bare or list-mode `git tag` and read-only `git branch` forms pass too.
// A mutating invocation also passes inside a repository the fork created by git init, git clone or git worktree add.
// So a fork may experiment on a throwaway repository in its scratchpad.
// --git-dir, --work-tree and an unresolvable directory never pass that way.
// A `git push` passes when its destination leads into such a repository:
// a remote that git remote add, git remote set-url or git clone pointed into one, or a path into one.
// A push with no destination or after a config key that may redirect it, a remote., url. or branch. key, never passes.
//
// Forks run in the handler's own session under the handler's own model; there is no model-per-fork axis.
// Named forks silently lose their inherited context in Claude Code releases up to and including 2.1.206.
// Forks must therefore stay unnamed; fork subagents need Claude Code v2.1.117+.
// Cluster rounds get no automatic timeout scaling: a wider fan needs a longer RunOpts.Timeout from the caller.
// A slow host surfaces as shuttleengine.OutcomeTimeout, never as a fork-shortfall error.
//
// # What the halves return
//
// ReviewResult is an invariant contract regardless of what was reviewed.
// It carries a Verdict (VerdictApproved or VerdictBlocking), the parsed Findings and the Digest.
// ParseReview enforces unique, non-empty ids fail-loud, so cross-round hydration can cite a finding unambiguously.
// Gate is the review-parse gate outcome passed through, nil when the reviewer never ran gated.
// SessionStates is the chair's session states on every ending, the audit error included.
// No decision reads the usage, timing or lens fields the results carry.
// A died or timeout outcome is a normal loop event with an empty Verdict; a caller branches on the result's Outcome.
//
// # The review-parse gate
//
// The entry re-prompts the reviewer in its own session while the round's review file does not parse.
// It lets the run through after its attempt budget, so a still-invalid file fails at the strict parse in acceptReview.
// Both reach ParseCurrentReview, the strict parse of the current round's review:
// ParseReview plus a non-empty `fix` on every finding,
// `verdict`, `findings` and `prior` as the frontmatter's only top-level keys,
// and one `### [SEVERITY]` prose block per finding, matching the frontmatter's count per severity.
// ParseReview tolerates a missing `fix`, so an older round's review still hydrates.
// `lym burler validate-review <review-file>` runs CheckReviewFile read-only as the gate's self-check.
//
// # The round checks
//
// A review after the first round answers the findings of the round before it.
// RoundContext holds those findings, each with the fixer's disposition, and the files the round's fixes changed.
// The review's `prior` list gives each earlier finding a status: resolved, open, outdated or withdrawn;
// a finding links the earlier one it carries forward with `carries`.
// CheckReviewRound judges the review against the context alone, so the package reads no format of its caller.
// RoundCheckGateEntry lets the round through on its cap.
// A review that passes on the cap keeps its out-of-bound findings, which the fixer works.
// A plan card file that an entry of RoundContext.AmendingTexts names is exempt from the changed-file rule.
// The match is the file's stem, with or without its leading number, as a whole token of the entry.
// It applies to a stem of 4 characters or more.
// Profile.HaltReason adds a halt-reason block to the review prompt.
// RoundContext.HaltReason lets the finding it asks for sit outside the changed files.
// That finding is tagged `origin: webster` (WebsterOrigin), and it must still be BLOCKING in an incremental round.
package burlerengine
