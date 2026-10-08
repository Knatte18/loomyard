// Package burlerengine runs one review+fix round over an artifact and returns a verdict.
// It is named for burling and mending: the cloth-finishing step where a worker inspects woven fabric for defects AND repairs them.
// A round splits that into two agents started together.
// A reviewer finds the defects and writes the review.
// A fixer orients while the review is written, waits for lyx to accept it, then validates the findings and repairs them.
// Each half is its own shuttle run.
//
// A burler runs ONE round and exits. It knows nothing about round loops,
// caps, convergence, or progress across rounds — that is its caller's job,
// which composes burler. Today the caller is
// internal/shedadapters.BurlerProducer, the Shed row that wraps one round
// and hands off to its segment's Bouncer. The dependency runs one way,
// caller -> burler -> shuttle, a strict chain: each layer knows only the one
// below it. This split is deliberate and is why burler is a separate module
// from its caller rather than folded into it: burler is LLM-heavy (one
// round is two shuttle runs; its tests are a fake-shuttle unit suite plus a
// handful of opt-in real-engine smoke tests), while the loop owner is
// deterministic Go (the round advance and the Bouncer's judge call; its
// tests use a fake burler returning scripted verdicts, no LLM at all).
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
// Both halves load `scribe:prose`, `scribe:code-quality` and `scribe:testing` through their spawn specs' skills (`burlerSkills`).
// Each orchestrator stencil renders the parent directive (internal/parentdirective) from Geometry.ParentName, which hubgeom.BurlerGeometry fills from the worktree's origin record;
// standalone geometry leaves it empty, which renders the no-parent variant.
// The instruction files carry no directive.
//
// # The two halves
//
// The reviewer's role is burler-review and the fixer's is burler-fix.
// The strands are therefore <shortname>:<slug>:burler-review and <shortname>:<slug>:burler-fix.
// The reviewer explores the target, judges it against the fasit and writes the review file;
// its write surface is the review file alone, in every fix scope, and it never edits, creates or deletes a target file.
// Its spec declares the review file as its only output file.
// Its gate is the review-parse entry (ReviewGateEntry) alone.
// It sets ForkSubagents exactly when the profile names a cluster fan.
// The fixer explores the target while the review is written.
// It then runs `lyx burler await-review` on the ready marker until it reports ready, reads the review, validates each finding against the code and the fasit, and fixes it.
// Its spec declares the fixer-report as its only output file.
// Its gate is RunOpts.Gate, every entry wrapped by repairReportBeforeGate.
// The fixer is the round's only friction-note writer.
// RunOpts.Review and RunOpts.Fix pick each half's model, effort and version.
//
// # The handoff
//
// Review-before-fix is a hard gate, not advisory: the review is fully on disk, and accepted by Go, before the round touches a single target file.
// Fixing findings as they are spotted turns the "review" into a post-hoc rationalization of edits already made, which destroys the independent judgment the whole method depends on.
// See PATTERN-review-round and the round-prompt assets that state this rule to each agent every round.
// Those assets are the two orchestrators, burler-template-{review,fix}-orchestrator.md, and the instruction files burler-step-{1-explore,2-review,3-fix}.md, the explore step rendered once per half.
// The prompts ship as embedded defaults in the top-level stencils package.
// They are read from the hub's stencils directory (see fabricengine.StencilsDir) at call time via stencilstore.Read, never from a compiled-in copy — see prompt.go.
// Engine.Run renders the instruction files
// per round and writes them to a fresh directory under .lyx, AnchorPath-anchored so it is a
// directory sibling of the durable _lyx tree
// (via lyxdirs.DotLyxDirName, machine-local, never committed —
// distinct from the committed _lyx), then hands each half's shuttle run only its orchestrator, which
// names the files' absolute paths so the agent reads each step's rules when it
// reaches that step. Run never prunes these per-round directories itself —
// they accumulate under .lyx/burler across rounds in a long-lived worktree.
// This is accepted machine-local litter, not a leak: .lyx is never
// committed, so it carries no cross-machine cost, and whatever clears a
// worktree's .lyx tree wholesale (a future caller-side cleanup step, or
// manual deletion) removes it along with everything else machine-local
// there.
//
// The edge that releases the fixer is the ready marker, a file whose path the caller tells the engine on Profile.ReadyMarkerPath.
// The caller derives it with burlermarker.Path (the engine derives no path), and only lyx code writes it.
// Run removes any stale marker before either half starts.
// reviewReady writes it only after the current attempt's reviewer reached done, passed the fork audit (cluster rounds) and parsed strictly.
// An unparseable review after the review gate's budget is the strict-parse error, never a gate-failed outcome.
// The engine cannot see a fixer that creates the marker itself, edits the target before it, names another existing file to the wait verb, or writes a round file other than its fixer-report.
// Those rules are prompt-enforced and caught by review alone.
// What the engine does check is the review file: after the fixer is done it compares the file with the bytes it parsed, and a difference is an error.
//
// # How the round starts
//
// RunOpts.FixStart chooses when the fixer starts.
// FixStartParallel, the default and the empty value, starts the reviewer and then the fixer at once, and the fixer waits for the ready marker.
// FixStartAfterReview starts the reviewer alone, waits for it, and starts the fixer only once the reviewer's review was accepted and the marker written, so the fixer's first await returns at once.
// A reviewer that ends without an accepted review starts no fixer, and the round reports the reviewer's outcome or error.
// Both orders skip no gate, and the reviewer-before-fixer ordering and the marker handoff are the same.
// An unknown value is an error from Run.
//
// # How the round ends
//
// decideRound is the one place the round's failure rules live, reached from join in the parallel order and from Run directly in the after-review order.
// join waits on both halves, and the first half to end decides which rules apply:
//   - The reviewer ends in anything but done, or its handoff fails: the fixer is stopped and the round returns the reviewer's outcome (died, timeout) with a nil error, or the audit or strict-parse error.
//   - The fixer dies or times out while the reviewer runs: the reviewer is stopped and the round returns the fixer's outcome.
//   - The fixer reaches done before the marker was released: the reviewer is stopped and the round returns an error naming the skipped handoff.
//     Whether the marker was released is the reviewer goroutine's recorded fact, set as the marker write begins, never the order the two results arrive in.
//   - Otherwise the round completes with the fixer's run.
//     A failing fixer gate returns the fixer's Gate with Verdict and Findings empty.
//     A done fixer with a passing or absent gate compares the review file with the parsed bytes before Verdict and Findings are set.
//
// A half is stopped through the told StrandRemover (RemoveStrandIfLive on its strand guid).
// The round returns the failing half's outcome, never the stopped half's consequential died outcome.
// A failed stop is ErrHalfNotStopped, never retried or archived over, whose message ends with the way forward (run "lyx reed remove <guid>", then re-step the row).
// A half that timed out is stopped as well, since shuttle keeps a timed-out run's strand live and a retry would otherwise run beside it, and so is a half whose wait returned an error, since it may still be running.
// join returns only after both waiting goroutines have returned, so no half is left live.
// The one exception is the half it failed to stop, which it reports without waiting for.
// A half whose provider never came up is that half's OutcomeDied with NotStarted set and a nil error.
// StartGated reports only the error there, so the half carries its StartError text and no identity.
// A reviewer that fails to start leaves the fixer never started, and a fixer that fails to start stops the already-started reviewer first.
// Any other start error is a pre-strand failure, returned wrapped, with the started reviewer stopped first.
//
// # Resuming a round
//
// A caller that finds a previous session's round on disk asks ProbeRound what became of each half, without waiting on either.
// A half is live when the shuttle holds a run for its one output file, even when that file is already on disk, as a reviewer repairing its review in the parse gate is.
// A half is done when it has no live run and its output file is present, and gone otherwise.
// The LiveRound it returns carries each half's state and, for a live half, its Handle, whose strand guid is what a caller stops the half by.
// Resume attaches to a round whose halves are both live: it starts nothing, removes the ready marker it finds (a finished reviewer's run is never live, so a marker is stale),
// and hands both handles to join, so a resumed round ends exactly as a fresh one does.
// Any other combination is the caller's to settle by stopping what is live through the same StrandRemover and running a fresh round;
// Resume refuses it, since a round never ends with one half live and a live half is never started a second time.
//
// Every recorded finding is fixed by the fixer, all severities including LOW and NIT.
// Severity affects how a finding is reported, never whether it gets fixed.
// Leaving low-severity findings unfixed "because they're just nits" is a known failure mode:
// unfixed nits re-surface or silently vanish across rounds instead of ever closing, so round count goes up instead of down.
// Two exceptions exist.
// A finding whose premise the fixer shows false against the code or the fasit is disputed with evidence in the fixer-report's Disputed section.
// Severity, size, cost or disagreement with the rubric never justify a dispute, and a dispute never converges a segment on its own.
// Something the round genuinely cannot do alone is named explicitly, with its reason, in the deferred section.
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
// That cross-round independence is the caller's discipline (it spawns a new burler each round);
// a single Engine.Run call only guarantees review-before-fix within its own round.
//
// # Profile vs RunOpts
//
// Profile is the content contract for one round: what to review (Target),
// what to judge it against (Fasit — an empty Fasit degenerates the round
// to a pure internal-consistency check, which validate rejects), the
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
// Run maps RunOpts onto each half's shuttle Spec and leaves Interactive/Parent/Display/KeepPane at their zero values:
// rounds are autonomous by default.
// Profile.ReadyMarkerPath is told with the round's other paths and required.
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
// Any other FixScope value, including empty, is a validate error: the
// field selects safety-critical behavior and gets no silent default.
//
// # Fabric-blindness
//
// burlerengine never imports the fabric module and never constructs a
// _lyx/... path — Result returns the review/fixer-report paths the
// caller supplied (resolved absolute), and committing them
// is the loop owner's job (loom's
// Burler-round-producer-plus-Bouncer segments), via the
// fabric engine in-process. See PATTERN-fabric-git.
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
// entries fails validate. There is deliberately no fan named "default":
// every seeded fan is dormant until a profile names it.
//
// ClusterExclude names lenses to drop from the fan ClusterFan resolves to,
// applied inside validate after ResolveFan, with the survivors stored in
// clusterLenses — the single value both prompt composition and
// auditClusterRound's exact-N fork check read, so a trimmed round demands
// exactly the forks it named and ErrClusterForksMissing's fail-loud posture
// is untouched. Three edge cases split on who authored the input:
// ClusterExclude set with an empty ClusterFan is a validate error, because
// that is a Go caller's mistake; a name not present in the resolved fan is a
// no-op for that name with a warning, because an exclusion list is an
// advisory, per-call directive over a config-owned fan an operator may edit
// between rounds, so a stale name is stale rather than wrong; and an
// exclusion that would empty the fan drops the whole exclusion and keeps the
// fan intact, because dropping to zero lenses is never what "these found
// nothing last round" meant and re-running the full fan costs tokens, never
// correctness.
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
// the consolidated review is fully written to disk, and accepted, before the fixer touches a single target file.
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
// prevents in advance — it is collected into Result.ClusterWarnings, never
// failing the round, since the handler's own consolidation phase already
// judges each fork's output on its merits.
//
// Run mechanics: forks run IN the handler's own shuttle session, under the
// handler's own model — there is no model-per-fork axis (a Claude Code
// constraint, not a burler choice), and no separate tmux pane or window
// per fork is needed. shuttleengine.Spec.ForkSubagents authorizes this for
// the run; claudeengine sets CLAUDE_CODE_FORK_SUBAGENT=1 inline on the
// launch line itself, never on the reed server's own environment, because
// reedengine.CleanClaudeEnv scrubs CLAUDECODE/CLAUDE_CODE_* from the server
// env at boot as mandatory hygiene — the launch line runs after that
// scrub, which is the only place a per-run, staged-rollout flag can ride.
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
// # What a round returns
//
// Result is an invariant contract regardless of what was reviewed.
// It carries a Verdict (VerdictApproved or VerdictBlocking) and the parsed Findings.
// ParseReview enforces unique, non-empty ids fail-loud, so cross-round hydration and audit can cite a finding unambiguously;
// the caller judges progress across rounds holistically via its own verdict judge, not by tracking finding-key identity.
// It also carries the resolved ReviewPath/FixerReportPath, and each half's SessionID/StrandGUID/LastAssistantMessage/RunDir (Result.Review and Result.Fix, with StartError set only on a half that never started).
// Outcome is the deciding half's, the fixer's Gate is passed through, and ForkAudit and ClusterWarnings come from the reviewer.
// Run returns a nil error for every shuttleengine outcome except a hard failure:
// an invalid profile, a shuttle start failure, a half that cannot be stopped, a skipped handoff, a changed review,
// or — deliberately loud — a verdict parse failure on a done review, since a defaulted verdict could silently terminate a caller's round loop on a malformed round.
// died/timeout are normal loop events a caller branches on via Result.Outcome, with an empty Verdict.
//
// # The review-parse gate
//
// The reviewer's gate spec is the review entry (ReviewGateEntry) alone, and the caller's own entries gate the fixer.
// While the round's own review file does not parse, the entry re-prompts the reviewer in its own session with the parse error, its quoting hint and the file to rewrite.
// It re-prompts at most reviewGateAttempts times and then lets the run through, so a file still invalid after the budget fails at the strict parse in the handoff.
// The gate (through CheckReviewFile) and that parse both reach ParseReview.
// `lyx burler validate-review <review-file>` is the gate's self-check verb:
// it runs CheckReviewFile read-only and needs no hub, mode or git repository.
package burlerengine
