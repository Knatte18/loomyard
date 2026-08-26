# `loom` — independent review + fix (prompt template)

> Filled instance of `crucible/review-prompt-template.md` for the `loom` module. See [../../crucible/README.md](../../crucible/README.md) for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/loom-crucible-hardening` (branch `loom-crucible-hardening`).
Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of `loom`'s scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux via `reed`, real interactive `claude` sessions via `shuttle`/`burler` — Discussion-Write, Plan-Write, Webster's per-card agents, and the three review segments' Bouncer-judge + Burler-round agents) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/suite check if the finding needed one),
and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `loom: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/loom-review-<yourtag>.md` and `_mill/loom-review-<yourtag>-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
If you catch yourself wanting to patch something the moment you spot it: don't. Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below — each hermetic command, each live-smoke run, each live-driving scenario — APPEND your observations to `_mill/loom-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
**COMMIT each append**, not just write it to disk — a small, frequent commit (`loom: review notes — <what you just appended>`) after each meaningful append.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` (this is round 1, so nothing should exist yet, but the rule stands for future rounds reading this same instance).
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**` (the `BurlerGeometry`/`WebsterGeometry` tellers loom's review segments and Webster ride), `internal/burlerengine/**` (the round engine every review segment's Burler row rides — read for how loom *uses* it, not to re-review burler's own internals, which have had their own separate crucible campaigns), `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- Docs: `manifest/designs/loom.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- Sandbox suite: none dedicated exists for loom yet. `tools/sandbox/SANDBOX-CORE-SUITE.md`'s scenario S8 carries a `**Covers:** loom` tag but is fixture-only (hand-writes `_lyx/loom/status.json` rather than reaching that state through any shipped verb) — read it for scenario IDEAS only, it is not real coverage and you drive everything yourself directly regardless.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` — in particular the Cwd Resolution, Told-Geometry, Fabric Git, Review Round, Shed Recipe Registry, Lyxdirs Single-Declarer, Test Tier Purity, and Documentation Lifecycle invariants; loom's producers and their reviewers all sit directly on top of these.
- Design intent (SPEC, not a review — the newest, least-proven code, recovered from git history):
  - `_mill/discussion.md` at `08508b14` (Webster-Review producer)
  - `_mill/discussion.md` at `5e923e10` (interactive Discussion-Write — the `Attach`/`AwaitOperator` machinery)
  - `_mill/discussion.md` at `aede023a` (Discussion-Burler Fabric Git Invariant fix)
  - `_mill/discussion.md` at `8cac77aa` (Bouncer anchor-path + run-dir clearing fix — the settled-marker mechanism)
  - Use `git show <sha>:_mill/discussion.md` to read each (the worktrees themselves are long torn down).

## Mission (assess on two axes, be adversarial)
1. Scope — does loom's as-built pipeline (17 producer rows, three review segments, Discussion→Plan→Webster→Publish→Finalize) deliver what the design docs above intend?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile areas below. Also assess docs accuracy and operability.

## High-yield focus — where `loom`'s real bugs live (drive these, do not just read them)
The pure/unit-tested parts are usually solid; defects concentrate in the COMPOSED, LIVE behavior the hermetic tests never exercise. Treat each as an INVARIANT you must actively verify by driving the real substrate.

- **The full 17-row pipeline, chained, has never been proven to run in one real `lyx loom run`, on any OS, ever** — every prior test drives at most one segment in isolation (unit/fixture tests) or stops at Discussion-Write's bounce loop (the loomcli smoke tests). Drive a small real task from a fresh seed all the way to `Finalize` and report exactly where it first breaks, if it does.
- **`discussion_interactive`/`Attach`/`AwaitOperator` — brand new, zero live exercise.** Its own discussion record (recovered at `5e923e10`) took six review rounds to converge on paper and was never run for real. Set `discussion_interactive: true`, drive the interview via `tmux send-keys` (see "Driving the real substrate" below), and specifically: (a) kill the driver mid-interview and confirm resume *re-attaches* to the live agent rather than respawning and discarding the operator's answers; (b) confirm an `OutcomeAsking` genuinely does not terminate the wait loop; (c) confirm two live matching runs is refused as an error, never silently picked.
- **The just-landed Bouncer settled-marker + anchor-path fix (`8cac77aa`) — only unit-tested.** Drive a real review segment (Discussion or Plan) through an APPROVED settle, force re-entry (bounce a downstream row back past the writer and into the segment a second time), and confirm the run directory actually archived-and-cleared rather than replaying the old verdict. Separately confirm `_lyx` artifact paths now resolve under a hub whose `AnchorRel` is genuinely not `"."` (a subpath-anchored hub), where `AnchorPath()` and `WorktreePath()` diverge for real — the fix's own regression tests may only prove this synthetically; prove it live.
- **`commit_seam: discussion`'s real git commit.** Confirm live that an APPROVED Discussion-Bouncer settle actually produces a real weft commit (`git log` in the weft repo), not just that the closure is wired — the Fabric Git Invariant is the entire reason this row exists.
- **Webster chained from a real `Plan-Write` output, never proven.** Webster's own smoke tests drive its fork machinery against an isolated fixture plan, never a plan `Plan-Write` itself produced. Drive a real multi-card plan through to Webster and watch batch/card sequencing for real.
- **Crash/resume ladder across every phase**, not just Discussion-Write: kill the driver mid Plan-Write, mid a review round, mid a Webster batch — confirm each resumes per `loom.md`'s documented ladder (attach-if-live, else respawn-from-output-files) with no duplicate agents and no discarded work.

## Explicitly OUT of scope for `loom`
- `Plan-Sweep` — does not exist, not needed for this campaign (confirmed with the operator).
- Windows path behavior — unreachable from this Linux host; do not reason about it as if driven.
- `burlerengine`'s own internal correctness beyond how loom's three review segments configure and call it — burler has had its own separate crucible campaigns; review loom's *use* of it, not burler itself.
- Any new feature or roadmap work. This is a hardening pass on already-shipped behavior only.

## Round context seeded from prior-round verification
**Round 1 — no prior round.** Do a genuinely independent clean-room pass. There is no residual to close and nothing CLOSED-AND-VERIFIED to avoid re-litigating yet.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow (one clean run of the full pipeline, plus each high-yield invariant above holding under a direct, deliberate attack) is the gate. Do not chase artificial concurrency stress for this module — see the cost declaration below for why.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes.`** Every test below spawns at least one real `claude` subprocess. **All of these function names begin with the substring `Smoke` — a bare `-run Smoke` pattern matches every single one of them simultaneously (~14 real `claude` subprocesses at once) and is BANNED, full stop, for this module.** Always name the exact one test function you mean to run.

`internal/loomcli/smoke_test.go` — each spawns ~1 real Discussion-Write `claude` subprocess (no fake/stub engine substitution in the fixture — confirmed by reading `newWiredPairFixture`), inside its own outer test timeout:
- `TestSmokeBootstrap_BringsUpSessionStrandAndDriver`
- `TestSmokeBootstrap_SecondInvocationDoesNotSpawnASecondDriver`
- `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`
- `TestSmokeDriveStandalone_RefusesOnNeverSeededPair`
- `TestSmokeDriveStandalone_FailureBeforeFirstPersistLeavesNonEmptyLog`
- `TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove`
- `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit`
- `TestSmokeBootstrap_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit`
- `TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver`
- `TestSmokeBootstrap_HandshakeFailureRefusesWithoutAttaching`

`internal/burlerengine/smoke_round_test.go`:
- `TestSmokeBurlerRoundToyFixture` — 1 real subprocess.

**EXECUTION BAN** (do NOT run these this round — not for extra confidence, not if there's time — unless this round's own mission is specifically the cluster-fan path):
- `internal/burlerengine/smoke_cluster_test.go`: `TestSmokeBurlerClusterCleanFan` — 2 real subprocesses (the `"clean"` fan resolves to 2 forks).
- `internal/burlerengine/smoke_cluster_test.go`: `TestSmokeBurlerClusterRogueFork` — 2 real subprocesses (the `"rogue"` fan resolves to 2 forks).
- Reason the ban exists: simultaneous real provider sessions exhaust the host's RAM — confirmed by a real prior incident (see `crucible/README.md`).

`cmd/lyx/tierpurity_test.go` carries `Smoke` in its own name-matching logic as literal test data, not as a real substrate-driving test of its own — 0 real subprocesses, but note it if a broad pattern ever matches it, so you know why it's harmless.

- Never run more than one live-substrate (`-tags smoke`) invocation at a time, in parallel, or backgrounded — one process, foreground, waited on to completion.
- The generic "N× CONCURRENT full smoke suites" gate in the orchestrator's verification protocol does **NOT** apply to loom as written — do not run it without first computing the real process count and getting the operator to sign off.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/...`
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag) — name the exact ONE test function each time, per the cost declaration above:
- `go test -tags smoke ./internal/loomcli/... -run TestSmokeBootstrap_BringsUpSessionStrandAndDriver -v -count=1`
- (repeat per exact test name as needed — never a bare `-run Smoke`)

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary under test: `deploy-dev` (POSIX) before EVERY source change you want reflected — live driving runs the deployed snapshot, not your working tree.
- Do NOT invoke any `sandbox-<module>-suite` launcher — none exists for loom anyway, and even if one did, spawning it on top of yourself would be meaningless (see `crucible/README.md`'s "Driving the real substrate" section).
- Run the real CLI commands yourself, directly, foreground, waiting for each to return: `lyx loom run`, `lyx reed status`, `lyx reed attach`, `lyx fabric` verbs as needed to seed a fresh worktree pair. This spawns real substrate underneath — real tmux panes, real interactive `claude` sessions — that is expected and required.
- **Report the exact `lyx reed status`/`lyx reed attach` commands to connect to whatever session you start, every time you start one** — the operator may be watching live and needs the socket/session name to attach from their own terminal (`lyx reed attach`, run from this same worktree, hands the operator's terminal to the tmux session in place; `lyx reed status` prints the JSON `session`/`socket`/`strands` first if you need to confirm it's up before saying so).
- The high-yield focus list above is a FLOOR — devise and run MANY more adversarial scenarios beyond it.
- "Headless" means "no human required" — NOT "no time/token cost to you." A real substrate session takes real wall-clock MINUTES. That cost is expected and budgeted for. You are explicitly forbidden from writing "operator-assisted", "cost-bearing", "long-running", or "impractical" as a reason to skip live driving — only a scenario that structurally requires a human's physical eyes (e.g. a visual render check) or a genuine environment gap is a legitimate "cannot verify headlessly".

TEARDOWN DISCIPLINE (critical): if you start any substrate server/session, tear it down. At the end, confirm ZERO stray substrate processes (`ps aux | grep -i tmux` — must show nothing of yours left running). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: design-intent vs shipped; flag deferred-that-should-be-fixed and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2, including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round (an operator decision, a second real TTY) — say so explicitly in the fixer report's deferred section. A finding whose fix is genuinely LARGE (a subsystem addition, a cross-cutting refactor) gets marked NOT-FIXED-THIS-ROUND instead — record it fully, the orchestrator spins it into its own mill-wiki task afterward.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None — round 1.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- Prefer surgical edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test that walks the failing scenario against the real substrate.
- MAKE SMOKE TESTS DETERMINISTIC — poll with a deadline on the actual state transition, never sleep a fixed amount.
- Update `manifest/designs/loom.md` (and `docs/overview.md`/`CONSTRAINTS.md` if invariants or the module table move) IN THE SAME change as the fix. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy-dev`) and re-run every live scenario yourself, directly.
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it — do NOT push unless the user explicitly asks.
- Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report — Executive summary with top risks + merge-readiness opinion; Scope assessment; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including what you could NOT verify and why. Write it to `_mill/loom-review-<yourtag>.md` and commit it incrementally as described above.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Write it to `_mill/loom-review-<yourtag>-fixer-report.md` and commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
