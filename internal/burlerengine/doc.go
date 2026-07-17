// Package burlerengine runs one review+fix round over an artifact and
// returns a verdict. It is named for burling and mending: the
// cloth-finishing step where a worker inspects woven fabric for defects
// AND repairs them in one pass. That is exactly what a burler does — A:
// review (find the defects), then B: fix (repair them) — in a single
// agent, one shuttle run.
//
// A burler runs ONE round and exits. It knows nothing about round loops,
// caps, convergence, or progress across rounds — that is perch's job
// (unbuilt), which composes burler. The dependency runs one way,
// perch -> burler -> shuttle, a strict chain: each layer knows only the
// one below it. This split is deliberate and is why burler is a separate
// module from perch rather than folded into it: burler is LLM-heavy (one
// round is a shuttle run; its tests are a fake-shuttle unit suite plus a
// handful of opt-in real-engine smoke tests), while perch is deterministic
// Go (the loop, the milestone cap ladder, and the progress judge; its
// tests use a fake burler returning scripted verdicts, no LLM at all).
// Keeping them one module would blend those two test regimes.
//
// # The A/B round
//
// A-before-B is a hard gate, not advisory: job A must be complete, with
// the review fully written to disk, before the round touches a single
// target file. Fixing findings as they are spotted turns the "review"
// into a post-hoc rationalization of edits already made, which destroys
// the independent judgment the whole method depends on — see the Review
// Round Invariant in CONSTRAINTS.md and the embedded prompt template
// (review-prompt-template.md via template.go) that states this rule to
// the agent every round.
//
// Every recorded finding is fixed in B, all severities including LOW and
// NIT: severity affects how a finding is reported, never whether it gets
// fixed. Leaving low-severity findings unfixed "because they're just
// nits" is a known failure mode — unfixed nits re-surface or silently
// vanish across rounds instead of ever closing, so round count goes up
// instead of down. The only legitimate exception is something the round
// genuinely cannot do alone; even then it must be named explicitly, with
// its reason, in the fixer-report's deferred section.
//
// A single burler round never grades its own fix. Because A precedes B
// within a round, A is a legitimate, independent gate exactly like a
// normal reviewer — but the fix FROM round N is judged by a FRESH
// burler's A in round N+1, not by the same round that made it. That
// cross-round independence is perch's discipline (it spawns a new burler
// each round); a single Engine.Run call only guarantees A-before-B within
// its own round.
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
// RunOpts (Model, Effort, Timeout, Round) is kept deliberately OFF the
// content Profile: run-tuning is a caller-resolved, config-driven
// selection that varies per invocation — perch will vary model/effort per
// round of the SAME artifact — while Profile describes what does not
// change about the round's content. Run maps RunOpts 1:1 onto the
// shuttle Spec and leaves Interactive/Parent/Display/KeepPane at their
// zero values: rounds are autonomous by default.
//
// # FixScope: overlay vs source
//
// FixScope selects B's write-surface and git discipline — content-agnostic
// (a burler improves code, text, or any artifact; the split is never about
// file type):
//
//   - FixScopeSource: the target is the host repo's own files. B's write
//     surface is the host working tree; it commits each fix individually
//     once green (message format
//     "<module-or-target>: fix <finding-id> — <one-line what/why>") and
//     never pushes. If the round dies mid-fix, git log shows exactly which
//     findings landed.
//   - FixScopeOverlay: the target is lyx system/orchestration state (plan,
//     discussion, review artifacts — typically weft-side, reached through
//     the _lyx junction). B's write surface is EXACTLY Target.Paths plus
//     the two output files, nothing else, and the round runs NO git
//     commands at all — the Weft Git Invariant reserves committing that
//     class of file to the loop owner, never an agent.
//
// Any other FixScope value, including empty, is a validate error: the
// field selects safety-critical behavior and gets no silent default.
//
// # Weft-blindness
//
// burlerengine never imports the weft module and never constructs a
// _lyx/... path — Result returns the review/fixer-report paths the
// caller supplied (resolved absolute), and committing them to the weft
// is the loop owner's job (perch's CLI standalone, or loom once it
// exists), via the weft engine in-process. See the Weft Git Invariant in
// CONSTRAINTS.md. The one exception an agent DOES commit is its own code
// under FixScopeSource — that is an ordinary host-repo commit, not a weft
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
// A cluster round still runs as ONE shuttle session — the handler — inside
// job A, in three phases: (1) the handler explores the target in full; (2)
// the handler spawns all N lens forks in a SINGLE message via Claude Code's
// built-in fork subagents (Agent tool, subagent_type "fork", always
// unnamed), and, while they run, performs its own HOLISTIC review —
// architecture, cross-file invariants, CONSTRAINTS-fit — the level no
// narrow lens covers; (3) the handler consolidates every fork's returned
// findings together with its own holistic findings into the ONE review
// file: dedup across lenses, an origin: frontmatter key on every kept
// finding (lens:<name> or handler), a ## Rejected prose section for false
// positives (judged with equal skepticism, never appearing in the parsed
// findings), and severity ordering. All three phases are part of job A —
// A-before-B is intact exactly as in a solo round, since the consolidated
// review is fully written to disk before the round's fix phase (B) touches
// a single target file.
//
// Fork discipline is fixed boilerplate the handler composes into every
// fork's prompt, never per-lens: read-only evidence gathering only (no
// Write/Edit/delete of any file, host or weft), no git commands of any
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
// launch line itself, never on the mux server's own environment, because
// muxengine.CleanClaudeEnv scrubs CLAUDECODE/CLAUDE_CODE_* from the server
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
// Cluster rounds weaken nothing about weft-blindness: forks write no
// files at all — read-only evidence gathering, findings returned only as
// their final message — so the write-surface story above (this package
// never imports weft, never constructs a _lyx/... path) is exactly as
// true for a cluster round as for a solo one.
//
// # What a round returns
//
// Result is an invariant contract regardless of what was reviewed: a
// Verdict (VerdictApproved or VerdictBlocking), the parsed Findings
// (ParseReview enforces unique, non-empty ids fail-loud, so cross-round
// hydration and audit can cite a finding unambiguously — perch judges
// progress across rounds holistically via a verdict judge, not by tracking
// finding-key identity), the resolved ReviewPath/FixerReportPath, and the
// shuttle run's SessionID/StrandGUID/LastAssistantMessage/RunDir. Run returns
// a nil error for every shuttleengine outcome except a hard failure
// (invalid profile, shuttle start/run failure, or — deliberately loud — a
// verdict parse failure on a done run, since a defaulted verdict could
// silently terminate a caller's round loop on a malformed round).
// asking/died/timeout are normal loop events a caller branches on via
// Result.Outcome, with an empty Verdict.
package burlerengine
