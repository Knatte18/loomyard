// Package perchengine is the deterministic gate loop over burler rounds: it
// spawns a fresh burlerengine round each iteration, reads its verdict, and
// decides APPROVED or STUCK via a milestone-capped round ladder and an
// ephemeral progress judge — never by trusting a burler's own self-grading.
// It is named for perching: the cloth-finishing station where woven fabric
// is draped over a frame under light and judged pass or fail. That is
// perch's role — it does not do the mending itself (that is burler, see
// the internal/burlerengine package documentation); it runs burler rounds
// and decides when the cloth passes.
//
// perch is what loom (unbuilt) puts between every phase, and it also runs
// standalone (`lyx perch run`). One engine serves ALL text-based review —
// discussion-review, plan-review, webster-review, and ad-hoc "review this
// file / this PR" are just different call-sites with different profiles;
// forking a copy per phase would lose the point (Profile — rubric, fasit,
// gate, caps — is data, never code). The heavier, behavior-based hardening
// of live-substrate modules is a separate module, hardener (see
// manifest/designs/hardener.md), which shares only the burler round discipline
// and runs post-loom, off this spine.
//
// # The gate — a black box with two judgment exits, plus pause
//
// From a caller's view a perch block is a black box with two judgment
// exits — Outcome APPROVED or STUCK — plus a third, operational exit,
// PAUSED (resumable, not judged; see Pause below). What happens inside is
// not the caller's concern; the block is not finished until the artifact is
// approved or it is definitively stuck. Inside, Engine.Run drives a round
// loop in plain Go — no standing orchestrator agent:
//
//  1. Run spawns a fresh burlerengine round (one round: A-review, B-fix).
//  2. Control returns to Run, which reads the round's verdict and review
//     file.
//  3. If the round did not converge, Run spawns a NEW burler for the next
//     round, hydrated from every prior round's review/fixer-report files
//     (and any failing gate-command output — see Pluggable gate below).
//
// Convergence is loop-until-dry, not resolve-one-round: the block reaches
// APPROVED only when a fresh round's review comes back clean — zero
// blocking findings — ON TOP OF the previous round's fixes. Because every
// round's fix is judged by the NEXT round's fresh burler A (never the same
// round that made the fix — no self-grading), termination on APPROVED
// always carries an independent confirmation; resolving one round's
// findings is not itself convergence.
//
// # The milestone ladder — round_caps
//
// RoundCaps (config key round_caps, profile key round-caps) is an array of
// strictly increasing round numbers, e.g. [5, 8, 10], replacing an earlier
// design's separate K / K_max pair: the operator writes the escalation
// ladder directly instead of perch inventing raises. Every entry except the
// last is a MILESTONE RUNG: reaching that round still BLOCKING triggers a
// mandatory judge-gated continuation check (see Verdict-judge model below).
// The LAST entry is the HARD CAP: reaching it still BLOCKING is
// unconditional STUCK (StuckHardCap) — no judge call, no escalation. A
// one-element array degenerates to a plain hard cap. The built-in default
// is [5, 8, 10] when neither the profile nor perch.yaml sets one
// (resolution order: profile > perch.yaml > built-in default).
//
// # Verdict-judge model — holistic, not key-canonicalized
//
// An earlier design tracked finding IDENTITY across rounds via canonical
// keys, with Go doing cycle detection over the key history ("key X
// recurred in rounds 1, 3, 5 → circling"). That is NOT what shipped: the
// key machinery bought a deterministic stuck decision at real complexity
// cost, while the hard cap already bounds the damage of a wrong verdict.
// What ships instead is a holistic verdict judge — an ephemeral LLM
// (default Haiku, config key judge_model / profile key judge-model) spawned
// via shuttle that reads the judge's bounded read-set (below) and writes a
// verdict file: strict YAML frontmatter (a verdict enum plus a rationale
// citing concrete finding evidence) over unconstrained prose. The prose
// carries a human-facing THEMES OVERVIEW — what kinds of findings keep
// recurring — so an operator can eyeball cross-round overlap; parsing is
// fail-loud, mirroring burlerengine.ParseReview.
//
// The judge's read-set is bounded, not unbounded: {the latest VALID
// judge-maintained handoff + the reviews of every completed round its
// covers_rounds does not already absorb} plus the current round's fresh
// review — never every prior round's review file. The SAME judge call that
// renders the verdict also maintains the handoff, writing a fresh
// round-<token>-handoff.md alongside the verdict file: strict YAML
// frontmatter (a lossless per-finding ledger plus covers_rounds) over a
// distilled prose narrative, carrying forward every ledger entry from the
// previous handoff (open or resolved, never dropped) so circling-detection
// never silently loses a recurring finding. state.json's per-round record
// carries this in an optional handoffPath field (additive, omitempty — see
// the state-json-compatibility shared decision), set only when that
// round's judge call both succeeded AND produced a handoff file that reads
// and parses cleanly. A round with no judge call at all (round 1; the
// round right after an APPROVED verdict; any round a judge call failed on)
// carries no handoffPath and therefore never appears in a later handoff's
// covers_rounds, so its review is always fed to the next judge call — this
// is what closes the "judge-gap" hole a bounded read-set would otherwise
// open. A missing or unparseable handoff file is the same fail-safe Warn
// posture described below, never an error and never STUCK: the next judge
// call simply falls back to the next older valid handoff, or — with no
// valid handoff at all — degrades to exactly the pre-handoff all-reviews
// behavior. This bounding was a genuine efficiency fix to perch's own
// shipped behavior (unbounded O(N) judge context growth as rounds
// accumulate), landed as part of extracting the loop into treadleengine
// (below) — it does not change what a burler ROUND reads (still every
// prior review/fixer-report/failed-gate output; see collectPriorHydration
// in treadleengine), only what the JUDGE reads.
//
// The same judge, the same template, serves two framings selected by a mode
// value:
//
//   - Per-round circling check — runs after every BLOCKING round that has a
//     prior round to compare against (never round 1, never after an
//     APPROVED round): verdict PROGRESSING | CIRCLING | UNCERTAIN. CIRCLING
//     (which the prompt rubric requires clear evidence for) means STUCK
//     immediately (StuckCircling), on any round. PROGRESSING/UNCERTAIN
//     continue the loop.
//   - Milestone continuation gate — runs INSTEAD OF (not in addition to)
//     the circling check on a milestone-rung round still BLOCKING: verdict
//     CONTINUE | STOP | UNCERTAIN ("does the trajectory justify
//     continuing?"). STOP means STUCK (StuckMilestoneStop); CONTINUE and
//     UNCERTAIN continue.
//
// Both triggers are burler-VERDICT-based, never convergence-based: in every
// gate mode, a round whose burler verdict is APPROVED runs no judge call at
// all, even in command/both gate modes where an APPROVED verdict alone does
// not converge the block (see Pluggable gate below) — the judge's material
// is blocking findings, and an APPROVED round has none to compare.
//
// Fail-safe posture: a validly-parsed UNCERTAIN verdict is a normal
// outcome of the judge call — the loop simply continues, with no Warn
// logged. ANY judge infrastructure failure (spawn error, a non-done
// shuttle outcome — which now includes a call that wrote its verdict but
// not the handoff, since both are required output files of the same spawn
// — or an unparseable verdict file) ALSO degrades to
// "progressing"/CONTINUE, but is additionally logged via internal/logger's
// Warn, carrying the round and cause (plus a human-facing label naming
// which call failed). A judge failure is NEVER a perchengine error and
// NEVER STUCK; the hard cap is what bounds the damage of a wrong or
// missing verdict. This mirrors the same fail-safe posture asking-triage
// uses below — the two ephemeral LLM utility calls are the ONLY fail-safe
// surface in the engine; every machine-read parse elsewhere is fail-loud.
// The fail-safe default is never persisted as if it were a genuine verdict:
// a round's state.json record carries JudgePath/JudgeVerdict only when the
// judge actually produced a parsed verdict, so an operator can always tell
// a real PROGRESSING/CONTINUE apart from a judge that never answered (the
// Warn log is the only trace of the latter).
//
// # Pluggable gate — verdict vs. command, and why it runs in perch
//
// A profile's Gate selects what "clean round" means (GateMode:
// GateLLMVerdict | GateCommand | GateBoth):
//
//   - llm-verdict (the default for text review): clean means a fresh
//     burler A returned VerdictApproved. No command runs.
//   - command: after each round's B-fix, Run itself executes Gate.Command
//     (argv, no shell — portable, quoting-safe) with cwd = the WORKTREE
//     ROOT, killing it after Gate.Timeout; a zero exit is clean. A timed-out
//     command is an ordinary FAILING gate (its partial output plus a timeout
//     note are recorded and fed forward — a hang is most plausibly the
//     round's own fix deadlocking the command, an artifact signal, not
//     machinery). The gate CALL itself is bounded even when the command
//     leaves a child holding its output pipe (test binaries, build workers,
//     a server the fix started): a short grace after exit or kill, the pipe
//     is abandoned and the recorded exit status decides pass/fail — an
//     orphaned grandchild may outlive the gate, but can never hang the
//     loop. Only a command that cannot START at all is a hard error,
//     and even then the completed round's record is persisted first so a
//     resume does not re-buy the round — and if that round was the hard cap
//     itself, the resume finalizes the block STUCK (StuckHardCap) rather than
//     running rounds past the ladder, so the guaranteed-termination invariant
//     survives the persist-then-error path. The burler verdict does NOT decide
//     convergence in this mode — the review still drives what B fixes, but
//     only the observed command result decides whether the block is done.
//   - both: both signals must agree (VerdictApproved AND a zero exit).
//
// The gate command runs in PERCH, never inside the burler's own A phase —
// because the whole point of independent verification is that the decider
// does not trust the worker. On a failing command, Run writes the combined
// stdout+stderr to round-N-gate.md and includes that file in the NEXT
// round's burler hydration (alongside prior reviews) so the next round
// starts already knowing what failed, rather than rediscovering it.
//
// # Non-done burler outcomes: deterministic retry vs. LLM-triage
//
// A round's burler attempt can finish in one of shuttle's four outcomes.
// done is the expected path (Verdict/Findings are read, the round proceeds
// to the gate). The other three are handled deliberately differently from
// each other and from STUCK — an infrastructure failure is never modeled
// as STUCK, because STUCK means "the artifact won't converge," a semantic
// judgment, while a dead or asking pane is a machinery event:
//
//   - died / timeout: retried once, deterministically, with a fresh burler
//     over the SAME hydration (the round number is not advanced — no
//     review was produced to hydrate forward). A SECOND consecutive
//     non-done attempt for the same round is a hard ERROR (not STUCK),
//     naming the round, the shuttle SessionID, and the kept shuttle run dir
//     (shuttle deliberately keeps died/timeout run dirs for inspection) —
//     deaths are nearly always environmental, so the first response is
//     cheap and deterministic rather than spending an LLM call interpreting
//     nothing.
//   - asking: the agent stopped mid-round asking a question instead of
//     finishing. Because this outcome carries interpretable text
//     (LastAssistantMessage), an ephemeral triage call (same judge-model
//     config) reads it and returns RETRY or GIVE_UP plus a reason, via the
//     same strict file contract the judge uses. RETRY re-attempts once
//     (then the second-consecutive rule above applies); GIVE_UP is a hard
//     ERROR surfacing the agent's question, SessionID, and run dir. Triage
//     infrastructure failure is fail-safe RETRY, exactly like the judge's
//     own fail-safe posture above — never an error, never STUCK.
//
// # Pause — a callback seam checked only at the round boundary
//
// Engine accepts Options.PauseRequested (func() bool), checked ONLY between
// rounds — never mid-round, never mid-aggregation — so a long-running block
// always pauses at a clean round start. A true result persists state and
// returns Outcome PAUSED — an OPERATIONAL exit, resumable, not judged;
// callers must branch on Outcome before ever reading StuckReason, which is
// set only alongside STUCK. Standalone, `lyx perch pause --run-id <id>`
// writes a flag file (PauseFlagPath) inside the run dir; Engine.Run clears
// that flag at its own entry, so resuming a paused block never instantly
// re-pauses on the flag that requested the very pause being resumed from.
// The same flag is also cleared whenever Run reaches any TERMINAL,
// non-PAUSED outcome (APPROVED or STUCK): a pause requested while the last
// round was still in flight can lose the race — that round settles on its
// own before the next round boundary (where the flag would have been
// observed) ever arrives — and the flag must not linger in a finished
// block's run dir. Loom will wire its own status-file check into the same
// seam later.
//
// # Run-dir mutual exclusion
//
// Engine.Run holds an exclusive, non-blocking OS file lock (run.lock, inside
// runDir) for its entire call duration. A second Run call against the SAME
// run dir — a duplicate invocation from two terminals, or a re-entrant loom
// caller — fails FAST with a named "already running" error rather than
// blocking until the first call finishes or, worse, silently interleaving
// rounds into the same state.json and artifact paths. That refusal wraps the
// ErrBlockBusy sentinel so a loop owner can errors.Is-match it and skip its
// block-exit bookkeeping — the losing call touched nothing on disk, and e.g.
// perchcli's fabric sync must not commit the winner's in-flight state under
// the loser's name. The lock is an OS
// advisory lock, released automatically if the holding process dies, so a
// crashed run never bricks the run dir for a later resume.
//
// # Fabric-blindness and geometry-blindness
//
// perchengine never imports fabricengine and never constructs a
// _lyx/... path itself — Engine operates on a caller-supplied absolute
// runDir and is told its geometry as a Geometry value, whose GateDir
// resolves the gate command's working directory and whose AnchorPath is
// carried for the caller's RunsDir/ScratchDir base but never read by Engine
// itself. Committing the run dir's artifacts to
// fabric is the loop OWNER's job — perchcli's standalone CLI today, loom
// once it exists — exactly once per block, at block exit (APPROVED, STUCK,
// or PAUSED), via fabricengine directly; see the Fabric Git Invariant in
// CONSTRAINTS.md. This is the identical split burlerengine already enforces
// one layer down.
//
// # Configuration: engine-general rails vs. phase knowledge
//
// perch.yaml (config keys judge_model, round_caps) holds only engine-general
// defaults and NEVER learns a phase name. Per-phase knowledge — which
// rubric, which fasit, a phase's own round-cap ladder, gate mode — is
// supplied per invocation via the profile, and for loom's phases that
// profile data will live in loom's OWN per-phase config, not perch.yaml.
// This is a recurring design rule worth stating once: knowledge of phases
// and lifecycle collects in loom; the engines beneath it (perch, burler)
// stay phase-agnostic. Resolution order for every perch-owned tunable is
// uniformly profile > perch.yaml > built-in default, applied once at block
// CREATION: a block's identity hash covers the profile as supplied (not the
// resolved values), and its resolved round-caps ladder is stamped into
// state.json, so a later perch.yaml change neither alters nor invalidates
// the resume of an in-flight block.
//
// judge_model (and a profile's judge-model/model keys, see perchcli) is a
// model-spec string, not a bare model name — contracts/specs/llm-model-spec.md's
// notation: a registry alias with an optional [effort=...] bracket, the
// escape form for a model not (yet) in the registry, or a bare alias that
// picks up its default effort from the operator-owned models.yaml. Effort
// is the ONLY bracket param perch accepts — a spec setting any other param
// (e.g. version) fails loud, since perch's Config/Profile carry no field to
// put it in. Resolution (modelspec.Parse, then Registry.Resolve against a
// models.yaml loaded once per invocation) runs exactly once, at config/
// profile load, unpacking the resolved (model, effort) pair into Config's
// and Profile's unchanged JudgeModel/JudgeEffort and Model/Effort fields —
// never re-resolved later, and never threaded as a spec string past load
// time. A pre-migration perch.yaml or profile file still carrying the old
// split judge_effort/effort keys fails strict validation loud rather than
// silently dropping the value (a deliberate fail-loud breaking change to
// config FILES only; perch's own Go API is unchanged — see "Treadle" below).
//
// # Treadle — where the round loop actually lives now
//
// The round loop this document describes — the round-caps ladder, the
// verdict-judge model, the pluggable gate, the non-done-outcome retry/
// triage machinery, pause, and run-dir locking — no longer lives in this
// package's own code. It was extracted into internal/treadleengine, the
// generalized round-loop engine a second future consumer (Tenter, see
// manifest/designs/hardener.md) can also drive. perchengine is now the thin
// configuration layer: it resolves perch.yaml/profile data (this file's
// resolution-order and identity rules above), adapts burlerengine into
// treadleengine's RoundRunner seam (adapter.go), and delegates to
// treadleengine.Engine.Run (engine.go) — every invariant documented above
// still holds exactly as described; only where the code lives has changed.
//
// Two deliberate diagnostics deltas, both cosmetic and both recorded here so
// a later reader does not mistake them for drift. First, hard round errors
// keep their "perch: " prefix but their BODY text is now runner-agnostic —
// "round N attempt run: ..." instead of the old "round N burler run: ...",
// and "kept run dir" instead of "kept shuttle run dir" — because
// treadleengine cannot name burler or shuttle without violating the Treadle
// Runner-Seam Invariant. No caller matches on these bodies (ErrBlockBusy is
// the only sentinel). Second, treadleengine's EXPORTED fail-loud parsers
// (ParseJudgeVerdict, ParseTriageVerdict, ParseHandoff) are package-level
// functions with no engine in scope and keep a fixed "treadle: " prefix
// where perch's own parsers said "perch: ". Those strings never reach a
// caller as an engine error — a perch operator sees them only as the cause=
// field of a "perch: "-prefixed fail-safe Warn.
//
// perch's own exported Go API is unchanged with exactly two recorded
// exceptions, neither of which any caller in this repo depended on.
// ParseJudgeVerdict and ParseTriageVerdict moved to treadleengine and are
// not re-exported (see identity.go): ParseJudgeVerdict was never callable
// from outside this package anyway, since its judgeFraming parameter type
// was already unexported, but ParseTriageVerdict took only []byte and
// genuinely was — so its removal is a real, deliberate narrowing rather than
// a no-op. And perchengine.ErrBlockBusy is now an alias of treadleengine's
// sentinel, whose own message text is the un-prefixed "block is already
// running": the composed error a caller actually sees is byte-identical
// (errf applies "perch: " at wrap time) and errors.Is still matches, but
// ErrBlockBusy.Error() on its own reads differently than it did
// pre-extraction.
//
// One treadleengine capability this package deliberately does not exercise:
// pre-round targeting (treadleengine.Profile.PreRoundTargeting). It exists
// for a future consumer whose rounds benefit from dynamically retargeted
// focus; perch's own rounds keep re-using a fixed rubric, so Engine.Run
// never sets that field on the treadleengine.Profile it builds, and it
// stays at its zero value (off). See the internal/treadleengine package
// documentation for that capability and for the full mechanics of the
// bounded judge handoff read-set summarized above.
package perchengine
