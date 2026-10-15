// Package burlerengine runs the two halves of one review+fix round over an artifact: a review half that returns a verdict and a fix half that repairs the findings.
// It is named for burling and mending: the cloth-finishing step where a worker inspects woven fabric for defects AND repairs them.
// A round splits that into two fresh sessions, each the chair of its own one-seat table on the seats engine (internal/seatengine).
// A reviewer finds the defects and writes the review.
// A fixer, spawned only once the review is accepted, validates the findings against the code and repairs them.
// Each half is its own shuttle run, and neither waits on, messages or knows of the other.
//
// A burler half runs ONCE and exits.
// It knows nothing about round loops, caps, convergence, or progress across rounds — that is its caller's job, which composes burler.
// Today the caller is internal/shedadapters, whose review row and fix row each wrap one half and hand off to their segment's Bouncer.
// The dependency runs one way, caller -> burler -> shuttle, a strict chain: each layer knows only the one below it.
// This split is deliberate and is why burler is a separate module from its caller rather than folded into it:
// burler is LLM-heavy: one round is two sessions, and its tests are a fake-shuttle unit suite plus a handful of opt-in real-engine smoke tests.
// The loop owner is deterministic Go: the round advance and the Bouncer's judge call, whose tests use a fake burler returning scripted results, no LLM at all.
// Keeping them one module would blend those two test regimes.
//
// Told-geometry tier: burlerengine is a producer — it is told the absolute paths it operates on
// through its Geometry struct (WorktreeRoot, AnchorPath), derives none of its own, and requires
// none of the three resolution tiers, so it runs in a directory that is not a git repository.
// internal/hubgeom and internal/standalonegeom are Geometry's two sole constructors, in hub mode
// and told mode respectively.
// This property is a review obligation here, not machine-enforced — this package has no
// import-allowlist test policing the absence of internal/lyxcwd.
// See PATTERN-told-geometry.
//
// # Skills and the parent directive
//
// Both seats load `scribe:prose`, `scribe:code-quality` and `scribe:testing` through their spawn specs' skills (`burlerSkills`).
// A half whose `Profile.LanguageSkills` marks a code round loads the language skills told to `New` after those, on every seat, with repeats dropped;
// any other half loads `burlerSkills` alone.
// The seats engine renders the parent and edit directives into each seat stencil exactly once:
// the chair's parent directive names Geometry.ParentName, which hubgeom.BurlerGeometry fills from the worktree's origin record, and standalone geometry leaves it empty, which renders the no-parent variant.
// The instruction files carry no directive.
//
// # The seat tables
//
// reviewTable and fixTable each build a one-seat seatengine.Table: the review segment, RunOpts.Timeout and RunOpts.Round, not interactive, and the half's agent as the chair.
// The one seat has no name, so the strands are <shortname>:<slug>:burler-review and <shortname>:<slug>:burler-fix, formed through seatengine.SeatRole by the seats engine.
// A Profile's RolePrefix leads both roles, as <shortname>:<slug>:<prefix>-burler-review and <shortname>:<slug>:<prefix>-burler-fix; a standalone run sets none.
// New builds that seat engine itself from the told Geometry and the stencils directory, over the seatengine.Shuttle it is given.
// Each seat's stencil markers ride the seat's values: the absolute instruction-file paths and the review path, and for the fix half the review format path.
// composeReviewPrompt and composeFixPrompt fill the instruction files and return those values; the seat stencils are filled by the seats engine.
// RunOpts.Review and RunOpts.Fix pick each half's model, effort and version.
//
// # The review half
//
// Engine.Review runs one fresh reviewer session as the chair of the review table.
// The reviewer explores the target, judges it against the fasit and writes the review file;
// its write surface is the review file alone, in every fix scope, and it never edits, creates or deletes a target file.
// Its spec declares the review file as its only output file.
// Its gate is the review-parse entry (ReviewGateEntry) alone.
// It sets ForkSubagents exactly when the profile names a cluster fan.
// The review half's files are the explore step, the review step and the review-file format, written to a fresh directory under the anchor's .lyx.
// The reviewer's prompt carries the PATTERN directive; it carries no friction directive.
// The accepted review is the part of a done reviewer's end acceptReview takes: the cluster audit for a fanned round, then ParseCurrentReview on the review file.
// Its ReviewResult carries the verdict, the findings and the Digest, the hex SHA-256 of the accepted bytes.
// An unparseable review after the review gate's budget is the strict-parse error, never a gate-failed outcome.
//
// # The fix half
//
// Engine.Fix runs one fresh fixer session as the chair of the fix table, over a review whose bytes hash to the digest it is told.
// It reads the review through acceptedReview first, so a review edited after acceptance is ErrReviewChanged and never reaches a fixer, and the same check runs again after a done fixer.
// A review with findings of any severity, a NIT included, spawns the fixer.
// A review with none runs the round's gates once through checkGates:
// when they pass, Fix writes a normal fixer report saying nothing was fixed and returns a skipped OutcomeDone with no session;
// when one fails, the fixer is spawned with the failing gates' findings in a gate-findings file as its work, since a gate exhausted in a round leaves its gate-failed edits in the target and the re-review of that round may find nothing.
// A gate closure that errors is returned wrapped with nothing spawned.
// The fixer reads the review, validates each finding against the code, and fixes it.
// Its spec declares the fixer-report as its only output file, and its gate is RunOpts.Gate with every entry wrapped by repairReportBeforeGate.
// The fixer's files hold the review-file format and the fix step; it carries no rubric, fasit, PATTERN directive, focus file, prior rounds or explore step,
// so `premise false` means false against the code it reads.
// The fixer is the round's only friction-note writer.
//
// # Review before fix
//
// Review-before-fix is a hard gate, not advisory: the review is fully on disk, and accepted by Go, before the fix half spawns at all, so no target file is touched before then.
// Fixing findings as they are spotted turns the "review" into a post-hoc rationalization of edits already made, which destroys the independent judgment the whole method depends on.
// No file or message rides between the halves; what carries the acceptance is the digest the review half returns, which the caller records and hands to the fix half.
// See PATTERN-review-round and the prompt assets that state this rule to each agent every round.
// Those assets are the two seat stencils, burler-template-{reviewer,fixer}.md, and the instruction files burler-step-1-explore.md, burler-step-review.md, burler-review-format.md and burler-step-fix.md.
// The prompts ship as embedded defaults in the top-level stencils package.
// They are read from the hub's stencils directory (see fabricengine.StencilsDir) at call time via stencilstore.Read, never from a compiled-in copy — see prompt.go.
// Each half renders its instruction files per call and writes them to a fresh directory under .lyx, AnchorPath-anchored so it is a
// directory sibling of the durable _lyx tree
// (via lyxdirs.DotLyxDirName, machine-local, never committed —
// distinct from the committed _lyx), then runs its seat from its stencil, which
// names the files' absolute paths so the agent reads each step's rules when it
// reaches that step. A half never prunes these per-call directories itself —
// they accumulate under .lyx/burler in a long-lived worktree.
// This is accepted machine-local litter, not a leak: .lyx is never
// committed, so it carries no cross-machine cost, and whatever clears a
// worktree's .lyx tree wholesale (a future caller-side cleanup step, or
// manual deletion) removes it along with everything else machine-local
// there.
//
// # How a half ends
//
// concludeReview and Fix's conclusion are each the one place their half's failure rules live:
//   - A reviewer whose provider never came up is OutcomeDied with NotStarted, Review.StartError and a nil error; a fixer's likewise, with Fix.StartError.
//   - A half that dies or times out returns its outcome with a nil error.
//   - A done reviewer whose review gate failed is an error naming the gate, never a verdict.
//   - A done reviewer whose cluster audit or strict parse fails is that error, returned with the outcome and fork audit.
//   - A done fixer returns its Gate, passed through; a failing fixer gate returns the fixer's Gate with a nil error.
//   - A done fixer whose review file no longer holds the accepted bytes is ErrReviewChanged.
//
// The seats engine stops every seat before it returns, through each handle's Stop, which records the stop in the seat's run record before it removes the strand, so a later attach probe reads the seat as stopped and never halts on it.
// A seat that timed out or whose wait returned an error is stopped as well, since it may still be running.
// A seat that cannot be stopped is seatengine.ErrSeatNotStopped, returned wrapped in ErrHalfNotStopped beside it, so a caller matching either sees it; it is never retried or archived over, and its message ends with the way forward (run "lyx reed remove <guid>", then re-step the row).
// Any other start error is a pre-strand failure, returned wrapped.
//
// # Resuming a half
//
// A caller that finds a previous session's half on disk asks ProbeReview or ProbeFix what became of it, without waiting on it.
// A probe builds the half's table with the same builder and runs the seats engine's probe over it.
// A half is live when the shuttle holds a run for its seat, even when its output file is already on disk, as a reviewer repairing its review in the parse gate is.
// A half is done when it has no live run and its output file is present, and gone otherwise.
// A seat that cannot be stopped is returned wrapped in ErrHalfNotStopped.
// The LiveHalf it returns carries the half's state and, for a live half, its Handle, whose strand guid is what a caller stops the half by;
// it also holds, unexported, what the probe found, which ResumeReview or ResumeFix hands back to the seats engine.
// ResumeReview and ResumeFix attach to a live half: they start nothing, and the resumed half ends exactly as a fresh one does.
// ResumeFix first asks a live handle that can deliver a pending task prompt to do so, so a fixer whose step died between its skill-load turns and its prompt is not waited on without ever receiving its task;
// a delivery error is the resume's error and nothing is resumed.
// A LiveHalf that is not live, and one that did not come from the half's probe, is refused with nothing started or stopped;
// the caller settles it by stopping what is live through the handle's Stop and running the half fresh, since a live half is never started a second time.
//
// Every recorded finding is fixed by the fixer, all severities including LOW and NIT.
// Severity affects how a finding is reported, never whether it gets fixed.
// Leaving low-severity findings unfixed "because they're just nits" is a known failure mode:
// unfixed nits re-surface or silently vanish across rounds instead of ever closing, so round count goes up instead of down.
// Two exceptions exist.
// A finding is disputed, with cited evidence in the fixer-report's Disputed section, on one of two grounds:
// its premise is false against the code, or the premise holds but every change that addresses it makes the target worse.
// One sound change means the finding is fixed, not disputed, and a finding whose only sound change leaves the fixer's write surface is deferred.
// When the premise holds and the proposed change is wrong, the fixer fixes with its own change and marks the finding as fixed differently in the fixer-report's list of changes.
// Severity, size, cost or disagreement with the rubric never justify a dispute, and a dispute never converges a segment on its own.
// Something the round genuinely cannot do alone is named explicitly, with its reason, in the deferred section, which keeps that meaning.
// A reviewer that re-raises a disputed finding answers the dispute's evidence in that finding's Issue.
//
// # Finding class
//
// Every finding carries a required Class beside its severity, one of four: design (the design is wrong, or a decision is missing or rests on a false premise), scope (the work missed a call site, file or case), decision (a choice is left open that the author must make) and consistency (two parts disagree, or the artifact departs from its own conventions).
// Each loom rubric says what design means for its own segment.
// A recurring scope gap is raised once, as a design finding about the method that keeps missing it.
// GatingClass (design) is the one class that decides when a review loop stops, in every segment.
//
// Class is not a severity ladder.
// It decides who decides and when the loop stops, never whether a finding is fixed:
// the fixer still fixes every finding of every class and severity.
//
// The case that motivated the split: a six-round discussion-review loop reported blocking findings of 6, 5, 4, 3, 3 and 3 and never converged.
// Sorted by kind, the design findings (five in round one, two in each of rounds two to five, none in round six) converged to zero,
// while scope findings ("you missed these call sites") recurred in four of six rounds, because the author kept adding the newly named files instead of fixing the cause,
// which was that hand-enumerating about forty call sites from greps is not a reliable method.
// Those findings were enumerations of one constant's consumers, which `go build ./...` reports exhaustively, instantly and for free.
// So a recurring scope gap is raised once, as a design finding about the method, and the loop stops on a round with no design finding rather than on a flat cap.
// Scope splits into two mechanical halves, neither an LLM lens: symbol references (the compiler, or a code-index references query before a deletion, for plan sizing)
// and bare string literals with no symbol behind them, which neither sees and a literal scan such as `TestEnforcement_GeometryLiterals` covers narrowly.
// An exclusion has to be written on both sides: telling the writer not to enumerate X while the reviewer still flags a missing X recreates the non-convergent loop.
// The trap to design against is reading class as a severity ladder and filing real problems under a low class to dodge the fix-everything default,
// which is why class decides who decides and when the loop stops, never whether a finding is fixed.
//
// A single burler round never grades its own fix.
// Because the review precedes the fix within a round, the reviewer is a legitimate, independent gate exactly like a normal reviewer.
// The fix FROM round N is judged by a FRESH reviewer in round N+1, not by the same round that made it.
// That cross-round independence is the caller's discipline (it spawns a new review half each round);
// a single round only guarantees review-before-fix within itself.
//
// # Profile vs RunOpts
//
// Profile is the content contract for one round: what to review (Target),
// what to judge it against (Fasit — an empty Fasit degenerates the round
// to a pure internal-consistency check, which validateReview rejects), the
// criteria (Rubric, mapped onto the fixed Severity vocabulary), the
// write-surface discipline (FixScope), whether the round may drive the
// real substrate (ToolUse), cluster fan-out (ClusterFan), the caller-named
// output paths, and optional prior-round hydration paths.
//
// RunOpts (Review, Fix, Timeout, Round) is kept deliberately OFF the
// content Profile: run-tuning is a caller-resolved, config-driven
// selection that varies per invocation — a caller may vary each half's model/effort
// per round of the SAME artifact — while Profile describes what does not
// change about the round's content.
// Each half maps RunOpts onto its seat's shuttle Spec through its table builder, and the tables are not interactive:
// rounds are autonomous by default.
//
// # FixScope: overlay vs source
//
// FixScope selects the fixer's write-surface and git discipline — content-agnostic
// (a burler improves code, text, or any artifact; the split is never about
// file type):
//
//   - FixScopeSource: the target is the repo's own files. The fixer's write
//     surface is the working tree; it commits each fix individually
//     once green (message format
//     "<module-or-target>: fix <finding-id> — <one-line what/why>") and
//     never pushes. If the round dies mid-fix, git log shows exactly which
//     findings landed.
//   - FixScopeOverlay: the target is lyx system/orchestration state (plan,
//     discussion, review artifacts), reached through
//     the _lyx junction. The fixer's write surface is EXACTLY Target.Paths plus
//     the fixer-report, never the review file, and the round runs NO git
//     commands at all — the Fabric Git Invariant reserves committing that
//     class of file to the loop owner, never an agent.
//
// Any other FixScope value, including empty, is a validation error: the
// field selects safety-critical behavior and gets no silent default.
//
// # Fabric-blindness
//
// burlerengine never imports the fabric module and never constructs a
// _lyx/... path — the results return the review/fixer-report paths the caller supplied (resolved absolute).
// Committing them is the loop owner's job (loom's Burler review and fix rows plus the Bouncer segments), via the fabric engine in-process.
// See PATTERN-fabric-git.
// The one exception an agent DOES commit is its own code
// under FixScopeSource — that is an ordinary repo commit, not a fabric
// operation.
//
// # Cluster fan-out (fork subagents)
//
// ClusterFan names a fan from the burler.yaml lens/fan library (see
// Config/ResolveFan in config.go — a seed-only, operator-owned config
// module registered in internal/configreg). Naming a fan IS what activates
// clustering: the fan's entry count becomes the fork count, one fork per
// listed lens, in fan order (repeats allowed). An empty ClusterFan is a
// single-reviewer round — the default, since forking is never on unless a
// profile explicitly names a fan — and a fan longer than maxClusterN (16)
// entries fails validation. There is deliberately no fan named "default":
// every seeded fan is dormant until a profile names it.
// Fan and lens names resolve against burler.yaml first and, per name, against the embedded template,
// so an operator-defined name wins and an absent burler.yaml resolves wholly from the template.
//
// ClusterExclude names lenses to drop from the fan ClusterFan resolves to,
// applied inside validateReview after ResolveFan, with the survivors stored in
// clusterLenses — the single value both prompt composition and
// auditClusterRound's exact-N fork check read, so a trimmed round demands
// exactly the forks it named and ErrClusterForksMissing's fail-loud posture
// is untouched. Three edge cases split on who authored the input:
// ClusterExclude set with an empty ClusterFan is a validation error, because
// that is a Go caller's mistake; a name not present in the resolved fan is a
// no-op for that name with a warning, because an exclusion list is an
// advisory, per-call directive over a config-owned fan an operator may edit
// between rounds, so a stale name is stale rather than wrong; and an
// exclusion that would empty the fan falls back to ClusterExcludeHeld, the exclusions carried from earlier rounds of the generation.
// When that would empty the fan too the whole fan runs, because dropping to zero lenses is never what "these found nothing last round" meant and running more lenses costs tokens, never correctness.
// ClusterExcludeHeld set with an empty ClusterFan is a validation error too.
//
// A cluster round still runs its review as ONE shuttle session, the handler, in three phases.
// (1) the handler explores the target in full; (2)
// the handler spawns all N lens forks in a SINGLE message via Claude Code's
// built-in fork subagents (Agent tool, subagent_type "fork", always
// unnamed), and, while they run, performs its own HOLISTIC review —
// architecture, cross-file invariants, PATTERN-fit — the level no
// narrow lens covers; (3) the handler consolidates every fork's returned
// findings together with its own holistic findings into the ONE review
// file: dedup across lenses, an origin: frontmatter key on every kept
// finding (lens:<name> or handler), a ## Rejected prose section for false
// positives (judged with equal skepticism, never appearing in the parsed
// findings), and severity ordering.
// All three phases are part of the review, so review-before-fix is intact exactly as in a solo round:
// the consolidated review is fully written to disk, and accepted, before the fix half spawns.
//
// Fork discipline is fixed boilerplate the handler composes into every
// fork's prompt, never per-lens: read-only evidence gathering only (no
// Write/Edit/delete of any file, repo or `_lyx`), no git commands of any
// kind, no touching the round's ReviewPath/FixerReportPath, and no nested
// Agent calls (forks cannot spawn forks). Two enforcement layers back this
// discipline mechanically rather than trusting the prompt alone: a
// session-level PreToolUse(Agent) hook that allows only unnamed
// subagent_type:"fork" calls through and denies everything else (policing
// Agent calls made from inside a fork's own pane too, not just the
// handler's); and, once the run reaches shuttleengine.OutcomeDone,
// auditClusterRound in cluster.go, which reads the shuttleengine.ForkAudit
// the engine attaches to the run (per-fork AgentCalls/WriteCalls/
// BashCommands facts from shuttleengine.AuditForks — never this package's
// own knowledge of the transcript layout) and enforces the fail-loud
// posture: exactly len(clusterLenses) fork transcripts or
// ErrClusterForksMissing (naming requested vs actual — a shortfall,
// including zero, is an infrastructure defect, never a degrade-to-solo);
// any fork with AgentCalls > 0, WriteCalls > 0, or a git-mutating Bash
// command is a hard error; any named spawn is a hard error. A fork that
// ran clean but never returned a report is sloppiness no mechanism
// prevents in advance — it is collected into ReviewResult.ClusterWarnings, never
// failing the round, since the handler's own consolidation phase already
// judges each fork's output on its merits.
//
// Run mechanics: forks run IN the handler's own shuttle session, under the
// handler's own model — there is no model-per-fork axis (a Claude Code
// constraint, not a burler choice), and no separate tmux pane or window
// per fork is needed. shuttleengine.Spec.ForkSubagents authorizes this for
// the run; claudeengine sets CLAUDE_CODE_FORK_SUBAGENT=1 inline on the
// launch line itself, never on the reed server's own environment.
// Reed's server spawn scrubs the keys the engines declare (CLAUDECODE/CLAUDE_CODE_* for claude) from the server env at boot as mandatory hygiene.
// The launch line runs after that scrub, which is the only place a per-run, staged-rollout flag can ride.
//
// Version pinning: CLAUDE_CODE_FORK_SUBAGENT is a staged-rollout flag
// requiring Claude Code v2.1.117+, and forks must stay UNNAMED — named
// forks silently lose their inherited context in Claude Code releases up
// to and including 2.1.206. A CC upgrade or downgrade should be checked
// against both facts before trusting a cluster round's output.
//
// Timeout guidance: cluster rounds get no automatic timeout scaling —
// RunOpts.Timeout is the same caller-resolved, per-invocation knob a solo
// round uses (the run-tuning-off-profile decision), so a wider fan needs
// an explicitly longer timeout from the caller. Forks queue under Claude
// Code's own concurrency cap (min(16, cores−2)) rather than running
// unboundedly parallel, so a low-core host serializes a wide fan instead
// of running it all at once; that serialization never breaks the exact-N
// contract above — every fork still runs and leaves its transcript, only
// wall-time grows — so a slow host surfaces as shuttleengine.OutcomeTimeout,
// never as a fork-shortfall error.
//
// Cluster rounds weaken nothing about fabric-blindness: forks write no
// files at all — read-only evidence gathering, findings returned only as
// their final message — so the write-surface story above (this package
// never imports fabric, never constructs a _lyx/... path) is exactly as
// true for a cluster round as for a solo one.
//
// # What the halves return
//
// ReviewResult is an invariant contract regardless of what was reviewed.
// It carries a Verdict (VerdictApproved or VerdictBlocking), the parsed Findings and the Digest.
// ParseReview enforces unique, non-empty ids fail-loud, so cross-round hydration and audit can cite a finding unambiguously;
// the caller judges progress across rounds holistically via its own verdict judge, not by tracking finding-key identity.
// It also carries the resolved ReviewPath, the reviewer's SessionID/StrandGUID/LastAssistantMessage/RunDir (ReviewResult.Review, with StartError set only on a half that never started), ForkAudit, ClusterWarnings and its Gate, the reviewer's review-parse gate outcome passed through and nil when the reviewer never ran gated.
// FixResult carries the fixer's Outcome, the resolved FixerReportPath, the fixer's Half, its Gate passed through, Skipped and NotStarted.
// Each Half also carries its run's StartedAt, EndedAt and session Usage (forks included) from its terminal shuttle Result, and ReviewResult.Lenses names the lenses the reviewer ran, nil for a solo round; no decision reads them.
// Review and Fix return a nil error for every shuttleengine outcome except a hard failure:
// an invalid profile, a shuttle start failure, a half that cannot be stopped, a changed review,
// or — deliberately loud — a verdict parse failure on a done review, since a defaulted verdict could silently terminate a caller's round loop on a malformed round.
// died/timeout are normal loop events a caller branches on via the result's Outcome, with an empty Verdict.
//
// # The review-parse gate
//
// The reviewer's gate spec is the review entry (ReviewGateEntry) alone, and the caller's own entries gate the fixer.
// While the round's own review file does not parse, the entry re-prompts the reviewer in its own session with the parse error, its quoting hint and the file to rewrite.
// It re-prompts at most reviewGateAttempts times and then lets the run through, so a file still invalid after the budget fails at the strict parse in acceptReview.
// The gate (through CheckReviewFile) and that parse both reach ParseCurrentReview, the strict parse of the current round's review:
// ParseReview plus a non-empty `fix` on every finding, `verdict` and `findings` as the frontmatter's only top-level keys, and one `### [SEVERITY]` prose block per finding matching the frontmatter's count per severity.
// ParseReview stays tolerant of `fix`, so a prior round's review written before the key existed still hydrates and still feeds a Bouncer's evidence.
// `lyx burler validate-review <review-file>` is the gate's self-check verb:
// it runs CheckReviewFile read-only and needs no hub, mode or git repository.
package burlerengine
