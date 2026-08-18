# `shuttle` — independent review + fix (round 1)

> Filled instance of `crucible/review-prompt-template.md` for this crucible campaign's second module (reed's six-round campaign converged and closed — see `_mill/reed-shuttle-HANDOFF.md`).
> See `crucible/README.md` for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `shuttle` module (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`) in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).

## Your two jobs, in order
1. REVIEW: form your own independent judgment of shuttle's scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux + a real, logged-in `claude` CLI, both resolved on `PATH`) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `shuttle: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/shuttle-review-r1.md` and `_mill/shuttle-review-r1-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

This module has a documented history worth knowing: an earlier shuttle crucible campaign (in a now-torn-down worktree; only the lessons survived, folded into `crucible/review-prompt-template.md` itself) had a round killed mid-fix with an uncommitted monolithic diff, and a separate round that interleaved review and fix (modifying files before finishing its review report). Both failure modes are exactly what the rules in this prompt exist to prevent — follow them precisely, not loosely.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/shuttle-review-r1.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/shuttle-review-r1.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`shuttle: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `shuttle-review-*` (a filename pattern, not a content judgment — this is round 1 so nothing should exist yet under that pattern, but if it does, it is off-limits until your own findings list is complete). Also do not open `_mill/reed-shuttle-HANDOFF.md` — the orchestrator's own running state note, off-limits the same way.
Reading the design SPEC and the module docs is expected and required (those are not reviews). Reading reed's own docs/`CONSTRAINTS.md`/`doc.go` (as opposed to reed's *review* history under `_mill/reed-review-*`) is expected and required too — shuttle is built directly on reed's API, and understanding reed's current, hardened contract is part of understanding shuttle correctly. The `_mill/reed-review-*` files are off-limits by the same filename-pattern rule as shuttle's own; reed's current CODE and its `doc.go`/`CONSTRAINTS.md` entries are not reviews and are expected reading.
AFTER your own findings are written, you MAY consult prior `shuttle-review-*` material (none exists yet for round 1) to confirm previously-fixed behaviors have not regressed.

## What to read
- Code: `internal/shuttleengine/**` (`run.go`'s `Runner`/`Start`/`Run`/`Interrupt`/`Send`/`Inject`, `rundir.go`'s run-directory lifecycle + orphan sweep, `wait.go`'s completion-classification loop + fork audit, `spec.go`'s validation, `config.go`, `forkaudit.go`'s value types, `posix.go`'s Windows-to-POSIX path helper), `internal/shuttleengine/claudeengine/**` (the one concrete provider engine), `internal/shuttlecli/**` (`cli.go`'s wiring — this is where `hubgeom.ReedGeometry` feeds `NewRunner`, `run.go`/`interrupt.go`/`send.go`), `cmd/lyx` integration.
- The wave-1 commit that changed shuttle's construction seam: `git show b98ee2ba -- internal/shuttleengine internal/shuttlecli` — read it in full. This is the diff that motivated this campaign; understand it before judging what it might have broken. The shape: `Runner` used to hold a `*lyxcwd.Location` and derive `AnchorPath()`/`WorktreePath()` from it on demand; it now holds two plain strings, `anchorPath` and `worktreeRoot`, told once at `NewRunner` construction and never re-derived. `internal/lyxcwd` is now completely absent from `shuttleengine`'s production imports (per `doc.go`'s own new sentence). Unlike reed's `Geometry` (a 7-field struct with a named type and a validating pre-flight `validateToldTmuxIdentity`), shuttle's told pair is two bare `string` parameters with NO structural type distinction between them and NO validation at all — a caller passing them in the wrong order compiles cleanly.
- Docs: `docs/overview.md`'s shuttle bullet (~line 282, search for "**shuttle**") and the execution-stack section (~line 320-370, search for "internal/shuttle" and "proc → reed → shuttle"), `manifest/roadmap.md`, `CONSTRAINTS.md` — especially the **Shuttle Provider-Seam Invariant** (provider specifics live ONLY under `internal/shuttleengine/claudeengine`; `shuttleengine` never imports `claudeengine`, the reverse only; enforced by `seam_enforcement_test.go`'s `TestProviderSeamImportRule`) and the **Durable-vs-Ephemeral State Invariant** (shuttle's run directory lives under the ephemeral `.lyx` tree, never `_lyx`; no engine derives its own `.lyx` path — confirm shuttle's `runDirRoot` honors this), `README.md`.
- Reed's CURRENT (post-six-round-campaign) contract, since shuttle drives reed directly through the `ReedOps` seam: `internal/reedengine/doc.go`, and specifically what changed behaviorally during reed's own hardening that a caller must now handle correctly — `AddStrand`/`RemoveStrand`/`Status` error shapes, the new foreign-session refusal on `up`/`resume` (a worktree rename or a stale/copied `.lyx` now makes reed REFUSE rather than silently double-launch or cross-contaminate — see reed's `refuseLiveForeignSessionLocked`), `Down`'s new `AbandonedSession` field in its result envelope, and `LoadState`'s new actionable-vs-opaque error shapes for a corrupt `reed.json`. Does `shuttleengine`'s `sweepOrphansOpportunistic` (which calls `reedengine.LoadState` directly) or any other reed-calling path in shuttle assume an OLDER, simpler reed behavior that reed's hardening campaign changed underneath it?
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
- Design intent: no dedicated `manifest/designs/shuttle.md` exists — `docs/overview.md`'s shuttle bullet plus `internal/shuttleengine/doc.go`'s package doc are the authoritative scope statement. Read both.
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` — for SCENARIO IDEAS only (a `**Covers:** shuttle` tag already satisfies the Sandbox Suite Coverage invariant; you do not need to extend the launcher machinery, only run scenarios yourself directly).

## Mission (assess on two axes, be adversarial)
1. Scope — is shuttle's scope right post wave-1? Did the told-string refactor (`NewRunner(reed, engine, layout, cfg)` replacing `NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)`) silently drop or change any observable behavior? Does shuttle correctly compose with reed's now-much-more-defensive contract (see "What to read" above)?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the areas below. Also assess docs accuracy and operability.

## High-yield focus — where shuttle's real bugs live post wave-1 (drive these, do not just read them)
This campaign exists because wave-1 (commit `b98ee2ba`) changed `Runner`'s construction from a single `*lyxcwd.Location` (which derived `AnchorPath()`/`WorktreePath()` consistently by construction) to two independently-passed plain strings. A green `go test` proves the mechanical field-splitting compiles and unit-passes, not that the new construction seam is safe under real use.

- **Told-string field integrity, no type safety.** `anchorPath` and `worktreeRoot` are both bare `string` — nothing at the type level stops a caller from swapping them, and `shuttlecli/cli.go`'s wiring (`hubgeom.ReedGeometry(layout)` → `reedGeom.AnchorPath, reedGeom.WorktreeRoot`) is the ONE production call site that must get the order right, with no compiler or runtime check backing it up. Trace every consumer of `r.anchorPath` (`runDirRoot`, `sweepOrphansOpportunistic`'s `LoadState` call, `wait.go`'s `AuditForks` workdir) and every consumer of `r.worktreeRoot` (`spec.validate`) and confirm each is semantically the right one for what it does — a swap would likely still compile and might even pass hermetic tests if a test fixture happens to give both the same value (check whether any existing test fixture accidentally does this, which would mask a real swap bug). `run_test.go`'s own `newTestRunner` helper deliberately uses DISTINCT temp dirs for the two "so a swapped NewRunner argument pair fails a test rather than passing" — confirm that guarantee is actually load-bearing in every test that could catch a swap, not just the one helper's own tests.
- **Shuttle's orphan sweep vs. reed's hardened `LoadState`.** `sweepOrphansOpportunistic` calls `reedengine.LoadState` directly (not through reed's CLI or `ReedOps` interface) and, on ANY error, logs and skips the sweep entirely ("to avoid sweeping kept diagnosis dirs over an unrelated I/O problem"). Reed's own hardening campaign changed what `LoadState`/`ReedState.UnmarshalJSON` do for a corrupt or `null` `reed.json` (see reed's R5-F1) — confirm shuttle's skip-on-any-error behavior is still the right call given reed's CURRENT error shapes, not the shape reed had before its hardening. Also: does the orphan sweep ever race a live reed operation (both processes touching the same `.lyx` tree)?
- **`Interrupt`/`Send`/`Inject` against reed's new foreign-session refusal.** These three ops call `FindRun` (pure `run.json` lookup, no reed involvement) then `requireReadyAgentPane`/`requireLiveStrand` (which DOES call into reed). If the worktree shuttle is running in has been renamed, or its `.lyx` has been copied/corrupted — the exact scenarios reed's own campaign hardened against — what does an in-flight shuttle run's `Interrupt`/`Send` actually do now? Does it get reed's new, more informative refusal, or does it fail some other, more confusing way because shuttle wraps the error?
- **Fork audit correctness (`ForkSubagents` spec option).** `AuditForks` is called only on `OutcomeDone` when `spec.ForkSubagents` is true, using `run.runner.anchorPath` as the workdir. `ForkAudit`/`ForkReport` are policy-free value types (the doc comment says so explicitly) — confirm no policy has crept into `shuttleengine` itself (that would violate the "caller interprets the counts" contract the doc promises) and that a caller with `ForkSubagents: false` genuinely never triggers a transcript scan (cost: this is real work against a real transcript file).
- **The `Stop`-hook completion classification (`done`/`asking`/`died`/`timeout`) and the live `AskUserQuestion` marker-hook detection** — drive both paths live: a run that finishes cleanly, a run that asks a question mid-flight (via the non-denying marker hook, not just the timeout path), a run whose pane dies unexpectedly, and a run that genuinely times out. Confirm each is classified correctly and that `asking`'s real-time detection actually fires before the timeout would have, not just eventually agreeing with it.
- **The `PreToolUse` guardrails** — the in-process `Agent` tool must be denied ALWAYS (fork-spawning is shuttle's own job, not the running agent's); `AskUserQuestion` must be denied only when the run is autonomous (`Interactive: false`, the default) and allowed when `Interactive: true`. Drive both guardrail edges live, not just by reading the hook wiring.
- **Interrupt-then-continue.** `TestSmokeInterruptSendContinues` names this shape in its own doc comment — drive it yourself with a fresh scenario, not just trusting the existing smoke test's fixture: interrupt a run mid-flight, send it new text, confirm it actually continues and the eventual result reflects the injected text, not a stale pre-interrupt state.
- **Windows-to-POSIX path translation (`posix.go`).** This is the one piece of shuttle with OS-conditional logic — read it for correctness even though you can only live-drive the POSIX side on this host; name the Windows gap explicitly rather than silently skipping it.
- **Every invariant a caller of shuttle depends on must hold under the new construction path** — a corrupt/partial `run.json`, a `run.json` for a strand reed no longer has (post-crash, post-rename — the exact shapes reed's own campaign spent two rounds on), a `Start` that fails after `AddStrand` succeeds (confirm the strand is genuinely cleaned up, not leaked), an `Inject` racing a `Wait` goroutine.

## Explicitly OUT of scope for this round
- `internal/reedengine`/`internal/reedcli`/`internal/hubgeom` — that campaign is CLOSED (six rounds, `_mill/reed-review-r{1..6}.md`, converged 2026-08-18). Do not re-review reed's own code; read its CURRENT contract as context for shuttle's correctness (per "What to read" above), but do not open its review history and do not propose reed changes — if you find a genuine reed defect while reviewing shuttle, name it explicitly as an OUT-OF-CAMPAIGN finding in your report rather than fixing it, and say why it's reed's problem not shuttle's.
- `internal/burlerengine`/`internal/burlercli`, `internal/perchengine`, `internal/loomengine` — modules built ON TOP of shuttle, out of scope; do not review code that merely calls shuttle from outside this module.
- Windows-specific live driving — this host is Linux; a Windows verification gap is expected for `posix.go` and should be named, not driven.
- Non-Claude engines — `claudeengine` is the only v1 engine; do not invent scope for a hypothetical Gemini engine.

## Round context seeded from prior-round verification
This is round 1 of a fresh campaign against shuttle — no prior round exists.
There is no residual to close and nothing CLOSED-AND-VERIFIED yet.
Do a full clean-room review + fix, focused on the High-yield focus list above.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the gate; an N×-concurrent stress suite (if the orchestrator later asks for one) is a diagnostic amplifier, not a merge blocker on its own. Given shuttle spawns real `claude` subprocesses (see the cost declaration below), the orchestrator will NOT ask for an N-concurrent sweep on this module the way reed got one — do not run one yourself either.

## Live-substrate cost declaration (BLOCKING)
`LLM-DRIVING: YES.` Every `//go:build smoke` test in `internal/shuttlecli` spawns a real `claude` subprocess — shuttle's whole job is running one LLM agent, so this is unavoidable and expected, not a shortcut to minimize.

The four smoke test functions, each spawning exactly ONE real `claude` process per invocation (none of them are a fan/cluster shape — `shuttleengine.Runner` runs one agent per `Start` call, full stop):
- `TestSmokeShuttleRunWritesOutputAndCleans` (`smoke_run_test.go`)
- `TestSmokeInterruptSendContinues` (`smoke_interrupt_test.go`)
- `TestSmokeGuardrailDeniesAgentTool` (`smoke_guardrail_test.go`)
- `TestSmokeGuardrailAskingSurfacesQuestion` (`smoke_guardrail_test.go`)

**EXECUTION BAN: none.** No fan/cluster test exists in this package today — every smoke test here spawns at most one real `claude` process. If you write a NEW smoke test, it must keep this shape (one real `claude` process per invocation) unless a specific finding genuinely requires more, in which case name the ban explicitly for it and get the orchestrator's sign-off before running it more than once.

- **You MUST pass `--model haiku` (or the cheapest available model) for every real `claude` process — every smoke test's own launch AND any ad-hoc `lyx shuttle run` you drive yourself — this is an explicit operator instruction for this whole campaign (reed and shuttle both), not a suggestion.** Only deviate if a specific finding genuinely requires testing model-specific behavior, and say so explicitly if you do. Check whether the existing smoke tests already pin a cheap model (reed's own `TestSmokeClaudeResumeRecallsCodeword` pinned a `smokeClaudeModel` constant — shuttle's smoke tests should follow the same pattern; if any of the four above does NOT already pin `haiku`, that omission is itself a finding).
- Run each smoke test by exact name (`-run <ExactTestName>`), never a bare `-run Smoke` sweep — even though nothing here is a fan/cluster test, running four real-claude tests back-to-back in one bare sweep is still real, serial wall-clock/token cost with no benefit over naming them.
- Never run more than one live-substrate (`-tags smoke`) invocation at a time, in parallel, or backgrounded — one process, foreground, waited on to completion.
- Do NOT run an N×-concurrent sweep against this module (see "merge bar" above) — that would multiply real `claude` subprocess count by N with no operator sign-off.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...`
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/shuttlecli/... -run <ExactTestName> -v -count=1` — one at a time, by exact name, per the cost declaration above.
- tmux is resolved via `PATH` on this host (`/usr/bin/tmux`, 3.6); `claude` must be a real, logged-in CLI on `PATH` — check for this FIRST (the smoke tests' own `claudeBinaryPath(t)` helper self-skips if absent; confirm it is present before assuming a skip means something is broken).

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary: `./deploy-dev` (POSIX script at repo root). **FOOTGUN:** live driving runs the DEPLOYED snapshot — re-run `./deploy-dev` after EVERY source change or you validate a stale binary.
- Do NOT invoke `sandbox-shuttle-suite.cmd`. Run the real `lyx shuttle run|interrupt|send` commands yourself, directly, foreground, waiting for each to return — walk the High-yield focus list above (and `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`'s scenarios for extra ideas).
- The list above is a FLOOR — devise and run MORE adversarial scenarios of your own, especially around the told-string field-integrity invariant (genuinely novel post wave-1) and shuttle's composition with reed's hardened contract.
- "Headless" means "no human required", not "no time/token cost to me." A real tmux/claude scenario takes real wall-clock time (a real agent run can take real MINUTES) — that is expected and budgeted for. Do not write "cost-bearing" or "long-running" as a reason to skip a scenario — only a genuine environment gap (no `claude` on PATH, not logged in) is a legitimate skip, and check for that FIRST.

TEARDOWN DISCIPLINE (critical): if you start any tmux server/session, tear it down. At the end, confirm ZERO stray tmux processes AND zero stray `claude` processes: `ps -eo comm | grep -cx 'tmux: server'` must be 0 (NOT `pgrep -x tmux` alone — a tmux server's `comm` is literally `tmux: server`, not `tmux`, so `pgrep -x tmux` falsely reads clean while a server runs; this was discovered during reed's own campaign, round 4), and check for any orphaned `claude` process by name. Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT.
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round — say so explicitly in the fixer report's deferred section.

## Deferred items from the prior round — RE-EVALUATE these
None — this is round 1.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `golang:golang-build`/`golang:golang-testing`/`golang:golang-comments` before editing — ALL of them, not code-quality alone. (This is exactly the step an earlier shuttle round skipped, per the note at the top of this prompt — do not repeat that.)
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test — remember the one-real-`claude`-process-per-invocation discipline above if the new test drives a live agent.
- MAKE SMOKE TESTS DETERMINISTIC — poll on actual state transitions with a deadline, never sleep a fixed amount.
- If `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` needs extending because a review surfaces a live/visual behavior it doesn't cover, extend it (keep `sandbox_coverage_test.go` green). Otherwise note the new scenario in your fixer report.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`./deploy-dev`) and re-run every live scenario yourself, directly.
- Update `docs/overview.md` (shuttle's bullet) / `internal/shuttleengine/doc.go` / `CONSTRAINTS.md` IN THE SAME change if invariants or scope move. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all tmux/claude state; confirm zero stray processes. COMMIT each fix as you finish it. Do NOT push.
- Report the changed files and how you verified each fix.

## Deliverables
1. Structured review report → `_mill/shuttle-review-r1.md`, committed incrementally per "Log as you go" above.
2. Fixer report → `_mill/shuttle-review-r1-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict. Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
