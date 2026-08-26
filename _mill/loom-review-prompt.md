# `loom` — independent review + fix (prompt template) — ROUND 2

> Filled instance of `crucible/review-prompt-template.md` for the `loom` module, round 2. See [../../crucible/README.md](../../crucible/README.md) for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2` (branch `loom-crucible-hardening-round2`).
Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of `loom`'s scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux via `reed`, real interactive `claude` sessions via `shuttle`/`burler`/`webster` — Discussion-Write, Plan-Write, Webster's per-card agent(s), and the three review segments' Bouncer-judge + Burler-round agents) — this is where the defects hide.
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
This matters MORE than usual this round: the primary live-driving activity (a full pipeline run) can take tens of real minutes per attempt, so a mid-run crash without incremental commits would lose the most expensive evidence this round produces.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree (nothing should exist yet from this round).
The round-1 reports referenced below under "Design intent" are an explicit, deliberate EXCEPTION to this rule — you are told to read them as SPEC/history, not as a review to avoid — because round 1 already diagnosed exactly what this round exists to verify (see "Round context" below). Read them for that purpose; still form your own independent judgment of the LIVE behavior you observe, rather than assuming round 1's reasoning still holds.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**` (the `BurlerGeometry`/`WebsterGeometry` tellers loom's review segments and Webster ride), `internal/burlerengine/**` (the round engine every review segment's Burler row rides — read for how loom *uses* it, not to re-review burler's own internals, which have had their own separate crucible campaigns), `internal/websterengine/**` (same: read for how loom's `Webster` row uses it, not to re-review websterengine's own internals), `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- Docs: `manifest/designs/loom.md`, `manifest/designs/webster-parallel-execution.md` (states today's Webster execution is strictly SEQUENTIAL, one card at a time — read this before assuming any parallelism exists to test), `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- Sandbox suite: none dedicated exists for loom yet. `tools/sandbox/SANDBOX-CORE-SUITE.md`'s scenario S8 carries a `**Covers:** loom` tag but is fixture-only — read it for scenario IDEAS only, it is not real coverage and you drive everything yourself directly regardless.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` — in particular the Cwd Resolution, Told-Geometry, Fabric Git, Review Round, Shed Recipe Registry, Lyxdirs Single-Declarer, Test Tier Purity, Config Strictness, and Documentation Lifecycle invariants; loom's producers and their reviewers all sit directly on top of these.
- Design intent (SPEC + history — recovered from git, since these worktrees are long torn down):
  - Round 1's full review report and fixer report — `git show archive/loom-crucible-hardening~1:_mill/loom-review-opus5-high-r1.md` and `git show 1f549606:_mill/loom-review-opus5-high-r1-fixer-report.md`. Read the whole review report, not just the F7 section — it names every invariant round 1 DID manage to drive live (interactive Discussion-Write, the Bouncer settled-marker/anchor-path fix, `commit_seam: discussion`) so you know what NOT to re-litigate, and its own "What was NOT verified" section is effectively this round's starting mission statement (see "Round context" below).
  - `_mill/discussion.md` at `08508b14` (Webster-Review producer), `5e923e10` (interactive Discussion-Write — the `Attach`/`AwaitOperator` machinery), `aede023a` (Discussion-Burler Fabric Git Invariant fix), `8cac77aa` (Bouncer anchor-path + run-dir clearing fix) — the same pre-round-1 design history, still relevant since round 2 reviews the same shipped code plus round 1's fixes on top.
  - F7/F3/F12's own fix commits (round 1's deferred findings, now merged): `a5f16660` (loom: Plan-Write/Plan-Validate approval deadlock — the fix that unblocks this entire round), `a426ba48` (fabric: clone doesn't commit written module configs), `8b14f32b` (reed: attach doesn't reconcile session geometry with the terminal). Use `git show <sha>` to read each fix's own diff and commit message.
  - Use `git show <sha>:<path>` to read any of the above (the worktrees themselves are long torn down; `archive/loom-crucible-hardening` is a permanent tag kept specifically so this material survives worktree teardown).

## Mission (assess on two axes, be adversarial)
1. Scope — does loom's as-built pipeline (17 producer rows, three review segments, Discussion→Plan→Webster→Publish→Finalize) deliver what the design docs above intend, now that it can actually run to completion?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile areas below, with special weight on the LATE rows (Plan-Write onward) that no live run has ever reached before. Also assess docs accuracy and operability.

## High-yield focus — where `loom`'s real bugs live (drive these, do not just read them)
The pure/unit-tested parts are usually solid; defects concentrate in the COMPOSED, LIVE behavior the hermetic tests never exercise. Treat each as an INVARIANT you must actively verify by driving the real substrate.

- **PRIMARY MISSION — the full 17-row pipeline, chained, has never once completed in a real `lyx loom run`, on any OS, ever.** Round 1 named this as its own top invariant and could not achieve it: F7 (the `Plan-Write`/`Plan-Validate` approval deadlock) made every attempt bounce forever at row 8, burning a real LLM session on a plan that was never wrong. F7 is now fixed (`a5f16660`). Drive a small real task from a fresh seed all the way to `Finalize` and report exactly where it first breaks, if it does — and if it doesn't break, that clean run IS the primary deliverable of this round. Use a genuinely small task description (see the "Minimal real task" note in the cost declaration below) so the real Webster phase this unblocks stays small — do not feed it a large, sprawling feature request.
- **Webster chained from a real `Plan-Write` output — never proven, explicitly named by round 1 as blocked and "owed to a later round".** Webster's own smoke tests (in `internal/webstercli`, out of loom's own module scope for hermetic gates — see "What to read" — but relevant here as background) drive its fork machinery against an isolated fixture plan, never a plan `Plan-Write` itself produced. Now that Plan-Write/Plan-Validate can actually pass, drive a real multi-card-or-single-card plan through to Webster and watch batch/card sequencing for real, against `manifest/designs/webster-parallel-execution.md`'s documented sequential-execution shape.
- **Crash mid-`Plan-Write`, live, for the first time.** Round 1's own report states it could not stage this scenario because F7 made `Plan-Write` bounce before a crash window was ever interesting. Kill the driver mid-`Plan-Write` now that the row can genuinely run, and confirm the crash/resume ladder (`loom.md`'s documented attach-if-live-else-respawn) holds here exactly as round 1 proved it holds for Discussion-Write.
- **The `approve_seam: plan` / `require_approved: true` mechanism itself (F7's actual fix), live.** Confirm live that `Plan-Bouncer`'s approved settle really does flip `approved: true` via the new seam, that `Plan-Validate` (pre-review) tolerates `approved: false` while `Plan-Revalidate` (post-review) genuinely enforces it, and that a REJECTED Plan-Bouncer verdict correctly leaves `approved: false` and routes back through `Plan-Write` rather than false-passing forward.
- **Crash/resume ladder for the rows past Plan** (a Webster batch, a Webster-Burler round) — round 1 proved this ladder for Discussion-Write and Plan-Write's precursor rows; it has never been driven for anything at or past Webster because nothing ever reached Webster live before. Kill the driver mid a Webster batch and confirm no duplicate agents, no discarded work.
- **End-to-end docs accuracy** — now that a full run is actually observable, check `manifest/designs/loom.md`'s description of the full pipeline against what you actually watched happen; round 1 could only verify this up to row 7.

## Explicitly OUT of scope for `loom`
- `Plan-Sweep` — does not exist, not needed for this campaign.
- Windows path behavior — unreachable from this Linux host; do not reason about it as if driven.
- `burlerengine`'s own internal correctness beyond how loom's three review segments configure and call it — burler has had its own separate crucible campaigns; review loom's *use* of it, not burler itself.
- `websterengine`'s own internal correctness beyond how loom's `Webster` row configures and calls it — same reasoning, one level up the stack.
- Any new feature or roadmap work, including `webster-parallel-execution.md`'s own (currently unshipped) parallel-card design — that is a separate future roadmap item, not something this round builds or reviews for readiness.
- Any new feature or roadmap work in general. This is a hardening pass on already-shipped behavior only.

## Round context seeded from prior-round verification

**Round 2. Round 1 converged and was independently verified — see below — but round 1's own review explicitly named the items in "PRIMARY MISSION" above as blocked and unverified, not as closed.** This round's job is BOTH a residual-closing round for those named-but-unreachable items AND a genuine adversarial pass over the now-newly-reachable late rows, since nothing has ever exercised them live before.

**CLOSED-AND-VERIFIED — do not re-open or re-litigate these:**
- All 17 round-1 findings (F1–F17). 14 fixed and committed one-per-finding directly in round 1 (squash-merged to `main` as `a0612b30`, "Crucible hardening: loom"). The 3 deferred as separate mill-wiki tasks are also now fixed and merged: **F7** (`a5f16660`), **F3** (`a426ba48`, belonged to fabric), **F12** (`8b14f32b`, belonged to reed).
- Independently re-verified on `main` before this round was seeded (not trusting any task's own merge-ready claim): `go build ./...` clean, `go vet ./...` clean, `go test ./...` — 78 packages, all `ok`, zero `FAIL`.
- Round 1 DID successfully drive live and confirm working: interactive Discussion-Write's `Attach`/`AwaitOperator` machinery (kill-mid-interview re-attach, `OutcomeAsking` non-termination, dual-live-run refusal); the Bouncer settled-marker + anchor-path fix (`8cac77aa`) under a real APPROVED settle and forced re-entry, including on a subpath-anchored hub; `commit_seam: discussion`'s real weft commit on an APPROVED Discussion-Bouncer settle; the crash/resume ladder for Discussion-Write and the rows immediately after it.
- The roadmap hygiene cleanup that landed alongside F12 (consolidating scattered `reed daemon` Someday entries) — unrelated to loom's own correctness, mentioned here only so you don't flag stale roadmap wording as a loom finding.

**RESIDUAL / this round's actual mission — see "High-yield focus" above for the full detail:**
1. A full live `lyx loom run`, Preflight through Finalize, has never completed even once. Achieve it (or find precisely why it still can't) as the primary deliverable.
2. Webster driven from a real `Plan-Write` output — never proven.
3. Crash-mid-`Plan-Write` and crash-mid-Webster-batch — never proven live (F7 made the first structurally untestable; nothing has ever reached the second).
4. Live confirmation of F7's actual `approve_seam`/`require_approved` mechanism under both an APPROVED and a REJECTED Plan-Bouncer verdict.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow (one clean run of the full pipeline, plus each high-yield invariant above holding under a direct, deliberate attack) is the gate. Do not chase artificial concurrency stress for this module — see the cost declaration below for why, which is even more strongly true this round than it was for round 1.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes.`** This round's PRIMARY activity — a full live `lyx loom run` — is itself the most expensive thing in this declaration, separate from and in addition to the named smoke tests below. Read this whole section before running anything.

**The full-pipeline live run.** A clean pass against a genuinely minimal one-card task spawns roughly 3–6 real `claude` subprocesses in SERIES (never concurrent — the shed pipeline is a sequential state machine): one Discussion-Write session, one Plan-Write session, Bouncer judge calls at each of the three review segments (cheap, still real sessions), one Webster session per plan card (today's Webster is strictly sequential — see `manifest/designs/webster-parallel-execution.md`), and a Burler-round session only if a Bouncer ever rejects and routes back. Each real session costs real wall-clock MINUTES, not seconds (per `crucible/README.md`) — a full clean pass against a trivial task is plausibly 15–40 real minutes end to end, not the sub-second bootstrap the existing `internal/loomcli` smoke tests report (those tests never reach a real spawn at all — every one of their fixtures leaves `SingleLLMProducer`'s own precondition/spec validation failing before `shuttleengine.Run` is ever called, which is exactly why this round's live-driving is genuinely new territory, not a repeat of anything already covered).
- **Use a genuinely minimal real task** — e.g. clone/adapt the `loom-smoke-task`-shaped fixture pair (`hubforge.AddPair`, see `internal/loomcli/smoke_test.go`'s `newWiredPairFixture`) and feed Discussion-Write a small, concrete task description so `Plan-Write` plausibly produces a single-card (or very few-card) plan, keeping Webster's real cost bounded. Do not feed it an open-ended or large feature request.
- **Run at most one full-pipeline attempt at a time**, foreground, waited on to completion. Never start a second live run while one is still in flight.
- **Report the exact `lyx reed status`/`lyx reed attach` commands every time you start a live run** — the operator may be watching live and needs the socket/session name to attach from their own terminal.

**Named smoke tests.** All of these function names begin with the substring `Smoke` — a bare `-run Smoke` pattern matches every single one of them simultaneously and is BANNED, full stop, for this module. Always name the exact one test function you mean to run.

`internal/loomcli/smoke_test.go` — each spawns 0 real subprocesses in practice (every fixture bounces before a real spawn — see above), inside its own outer test timeout:
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

**EXECUTION BAN** (do NOT run these this round — not for extra confidence, not if there's time — loom's own recipe never configures cluster-fan on any Burler row, so these are burlerengine's own tests, entirely out of this round's mission):
- `internal/burlerengine/smoke_cluster_test.go`: `TestSmokeBurlerClusterCleanFan` — 2 real subprocesses.
- `internal/burlerengine/smoke_cluster_test.go`: `TestSmokeBurlerClusterRogueFork` — 2 real subprocesses.
- Reason the ban exists: simultaneous real provider sessions exhaust the host's RAM — confirmed by a real prior incident (see `crucible/README.md`).

`cmd/lyx/tierpurity_test.go` carries `Smoke` in its own name-matching logic as literal test data, not as a real substrate-driving test of its own — 0 real subprocesses, but note it if a broad pattern ever matches it, so you know why it's harmless.

`internal/webstercli/smoke_test.go`'s `TestSmoke_*` tests are websterengine's own module tests, out of loom's own module scope (see "Explicitly OUT of scope" above) — do not run them as part of this campaign's own gates, but a stray bare `-run Smoke` anywhere in the repo would match them too, which is one more reason the bare-pattern ban is absolute, not just scoped to loom's own packages.

- Never run more than one live-substrate (`-tags smoke`) invocation at a time, in parallel, or backgrounded — one process, foreground, waited on to completion. This applies equally to the primary full-pipeline live run above.
- **The generic "N× CONCURRENT full smoke suites" gate in the orchestrator's verification protocol does NOT apply to loom, full stop — do not run it this round, under any framing, with or without operator sign-off.** Even a SINGLE full-pipeline live run already spawns several real sessions serially over tens of minutes; N concurrent copies would multiply that by N simultaneous real `claude` processes, which is a materially worse version of the exact incident `crucible/README.md` documents. This is stronger than round 1's already-cautious stance: round 1 never got far enough to observe the true per-run cost, and now that it's known (3–6 real sessions per attempt), concurrent copies are unambiguously unsafe, not merely uncosted.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/...`
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag) — name the exact ONE test function each time, per the cost declaration above:
- `go test -tags smoke ./internal/loomcli/... -run TestSmokeBootstrap_BringsUpSessionStrandAndDriver -v -count=1`
- (repeat per exact test name as needed — never a bare `-run Smoke`)

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface, and where this round's actual mission lives):
- Deploy the current source as the dev binary under test: `deploy-dev` (POSIX) before EVERY source change you want reflected — live driving runs the deployed snapshot, not your working tree.
- Do NOT invoke any `sandbox-<module>-suite` launcher — none exists for loom anyway.
- Run the real CLI commands yourself, directly, foreground, waiting for each to return: `lyx loom run`, `lyx loom status`, `lyx reed status`, `lyx reed attach`, `lyx fabric` verbs as needed to seed a fresh worktree pair with a minimal real task. This spawns real substrate underneath — real tmux panes, real interactive `claude` sessions — that is expected and required.
- **Report the exact `lyx reed status`/`lyx reed attach` commands to connect to whatever session you start, every time you start one** — the operator may be watching live.
- The high-yield focus list above is a FLOOR — devise and run MORE adversarial scenarios beyond it if the primary full-pipeline mission leaves you time, but do not let extra scenarios crowd out actually completing (or definitively failing to complete) at least one full clean pipeline run — that is the one thing this round exists to establish that no prior round ever did.
- "Headless" means "no human required" — NOT "no time/token cost to you." A real substrate session takes real wall-clock MINUTES. That cost is expected and budgeted for. You are explicitly forbidden from writing "operator-assisted", "cost-bearing", "long-running", or "impractical" as a reason to skip live driving — only a scenario that structurally requires a human's physical eyes (e.g. a visual render check) or a genuine environment gap is a legitimate "cannot verify headlessly".

TEARDOWN DISCIPLINE (critical): if you start any substrate server/session, tear it down. At the end, confirm ZERO stray substrate processes (`ps aux | grep -i tmux` — must show nothing of yours left running, and no orphaned `claude`/driver processes either). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: design-intent vs shipped; flag deferred-that-should-be-fixed and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2, including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round (an operator decision, a second real TTY) — say so explicitly in the fixer report's deferred section. A finding whose fix is genuinely LARGE (a subsystem addition, a cross-cutting refactor) gets marked NOT-FIXED-THIS-ROUND instead — record it fully, the orchestrator spins it into its own mill-wiki task afterward.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- Round 1's own live crash-mid-`Plan-Write` reproduction — explicitly deferred as "owed to a later round" because F7 made the row bounce before a crash window was interesting. Now genuinely drivable — see "High-yield focus" above.

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
1. A structured review report — Executive summary with top risks + merge-readiness opinion, and an explicit statement of whether a full live pipeline run ever completed; Scope assessment; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including what you could NOT verify and why. Write it to `_mill/loom-review-<yourtag>.md` and commit it incrementally as described above.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Write it to `_mill/loom-review-<yourtag>-fixer-report.md` and commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict + an explicit yes/no on "did a full live pipeline run complete this round"). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
