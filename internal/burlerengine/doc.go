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
// real substrate (ToolUse), cluster fan-out (ClusterN), the caller-named
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
// # Cluster fan-out (not yet)
//
// ClusterN selects how many extra cross-checking reviewers step A spawns
// alongside the round's own review. Only ClusterN == 0 is supported today;
// a positive value fails validate with ErrClusterUnsupported. Cluster
// reviewers need their own switchable psmux window (mux's own-window
// anchoring), which does not exist yet — this gates only the cluster
// feature, not the rest of the shuttle -> burler -> perch -> loom spine,
// which ships on ClusterN == 0. See the roadmap's own-window-anchoring
// milestone.
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
