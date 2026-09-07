# `loom` — independent review + fix (prompt template) — ROUND 3 (glyph-hardening campaign) — TWO MISSIONS

> Filled instance of `crucible/review-prompt-template.md` for round 3 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230). See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **This round has TWO missions, both required, per the operator's explicit instruction — see "Mission" below.** Mission A is new: a `#004` mill task (`standalonegeom-webster-run-and-log-hygiene`), itself a product of this campaign's own round-1 findings (F16/F22), just squash-merged its fix INTO this branch (commit `d7c52df6c`) — and that fix has never been through a crucible round. Mission B continues loom's own glyph-surface hardening from round 2's residual.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module AND the newly-merged webster-standalone-mode fix in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening` (branch `crucible-loom-glyph-hardening`).
Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of both missions' scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux via `reed`, real interactive `claude` sessions via `shuttle`/`burler`/`webster` — Discussion-Write, Plan-Write, Webster's per-card agent(s), the three review segments' Bouncer-judge + Burler-round agents, AND (Mission A) a real standalone `lyx webster run`) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/suite check if the finding needed one),
and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `loom: fix <finding-id> — <one-line what/why>` (use a `standalonegeom:`/`shuttleengine:` prefix instead of `loom:` for a Mission-A-only finding, so the commit log itself distinguishes the two missions).
Also commit `_mill/loom-review-<yourtag>.md` and `_mill/loom-review-<yourtag>-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
If you catch yourself wanting to patch something the moment you spot it: don't. Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below — each hermetic command, each live-smoke run, each live-driving scenario — APPEND your observations to `_mill/loom-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
**COMMIT each append**, not just write it to disk — a small, frequent commit (`loom: review notes — <what you just appended>`) after each meaningful append.
This round has TWO real-LLM live-driving activities (Mission A's standalone run, Mission B's hub-mode work if you attempt the Rename gap) — either can take tens of real minutes, so incremental commits matter as much as ever.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including rounds 1/2's own review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment — the handoff note is the orchestrator's own private state, not a review, but it matches the pattern and is exactly as off-limits until you have your own list.
AFTER you have your own independent findings, you MAY (and should) consult rounds 1/2's material and the handoff — see "What to read" below — to (a) confirm rounds 1/2's 27 fixes have not regressed and (b) understand the `#004` mill task's own history (its own plan/discussion/review artifacts, recovered from git history — see below).
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read

**Mission A — the newly-merged fix (`d7c52df6c`), never reviewed by crucible:**
- The full diff: `git show d7c52df6c` (18 files, 879 insertions). Key files: `internal/shuttleengine/run.go` (new `NewDetachedRunner` — a runner whose anchor is deliberately outside its worktree root, constructed ONLY from standalone CLI wiring; `NewRunner`'s own containment assertion, which FOUR other callers — `burlercli`, `webstercli`, `shuttlecli`, `loomcli` — rely on for safety, is claimed unchanged), `internal/shuttleengine/wait.go`, `internal/standalonegeom/logsdir.go` (new — `LogsDir`, the sole declarer of standalone's trace-log directory, the F22 fix), `internal/standalonegeom/doc.go`, `internal/webstercli/wiring.go`, `internal/burlercli/wiring.go`, `internal/logger/sink.go`.
- `CONSTRAINTS.md`'s new invariant line (under Hub Containment Invariant, roughly): "A `shuttleengine` runner whose anchor is deliberately outside its worktree root is constructed only through `shuttleengine.NewDetachedRunner`, only from a standalone CLI's own wiring, and `NewRunner`'s containment assertion is never relaxed to accommodate it." Verify this claim against the actual diff — is `NewRunner` truly untouched for the hub-mode path, or does the fix's plumbing introduce any shared code between the two constructors that could regress hub-mode safety?
- The `#004` task's own history, recovered from git (this crucible branch's own log, since the task's worktree may be gone): `git log --oneline --all | grep -i standalonegeom` finds its plan/discussion/review-round commits (`mill-plan: planned for standalonegeom-webster-run-and-log-hygiene`, `mill-go: holistic approve standalonegeom-webster-run-and-log-hygiene`, etc.) — read a few for context on what the task's own plan/review process already checked, so you don't waste time re-deriving what it already covered, but still form your OWN adversarial judgment of the SHIPPED code — a mill task's own review is not a crucible round and doesn't carry the same bar.
- Round 1's own investigation of F16/F22 (`_mill/loom-review-opus5-high-r1.md`, findings F16/F22) — this campaign's own orchestrator did real research into why F16 specifically crosses a module boundary (four `NewRunner` callers) before deciding it warranted its own task; read that reasoning so you understand what the fix needed to solve.

**Mission B — loom's own glyph-surface hardening, continuing from round 2:**
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- The glyph surface: `internal/planparser/**`, `internal/planglyph/**` (rounds 1+2 changed `handle.go`, `drift.go`, `planglyph.go`, `create.go`, `containment.go`, `scope.go`, `doc.go`, `validate.go` — read CURRENT state), `internal/websterengine/beginbatch.go`/`recordbatch.go`/`fingerprint.go`.
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`, `contracts/specs/loom-plan-spec.md` (rounds 1+2 corrected multiple examples and added the same-plan-Rename constraint), `contracts/stencils/loom/loom-template-plan.md`, `contracts/stencils/loom/loom-rubric-plan-review.md` (round 2 corrected stale counts + terminology), `contracts/stencils/webster/webster-template-master.md` (round 2 added the plan-drift-refusal section), `manifest/designs/webster-parallel-execution.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- Round 1/2's own material (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the orchestrator's independent-verification record for both rounds, plus the real-PR/real-hub verification for round 2 — read this to see exactly what a real hub-mode run already proved, and what remains open: a Rename card executing through a real Webster batch, `Finalize`, `DetectDrift`'s exact-tier auto-repair via a real fork).

- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` — in particular Cwd Resolution, Told-Geometry, Hub Containment, Fabric Git, Review Round, Shed Recipe Registry, Lyxdirs Single-Declarer, Test Tier Purity, Config Strictness, Planparser Sole-Parser, Glyph Conversion Chokepoint, Quarry CGO Requirement, and Documentation Lifecycle invariants — plus the new invariant line `d7c52df6c` added.
  **Quarry CGO Requirement Invariant**: every build/test/deploy command needs `CGO_ENABLED=1` and a C compiler on `PATH` — already the default on this dev machine.

## Mission (assess on two axes, be adversarial, for BOTH missions)

**Mission A — the newly-merged standalone-mode fix.** Scope: does `NewDetachedRunner` actually solve F16 (standalone `lyx webster run` can start Master) and does `LogsDir` actually solve F22 (no untracked `.lyx/logs/` in the target repo) without weakening `NewRunner`'s shared containment safety for hub-mode callers? Correctness: this is UNREVIEWED code from a different task's own pipeline — read it as skeptically as you would a stranger's PR, not as "already checked, just confirm."

**Mission B — loom's glyph integration.** Scope: rounds 1+2 closed 27 findings and proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`. Correctness: this round is a genuine safety pass — find what two prior rounds, across two different models, both missed — PLUS an attempt to close the one interesting remaining live gap: a declared `Rename` card actually executing through a real Webster batch (never yet observed live; round 2's real planner avoided it because the specific task given couldn't be expressed that way).

## High-yield focus

### Mission A — drive the real standalone webster run

- **Set up a plain git repo with real Go source** (not a fabric hub — that's the whole point of standalone mode) and run `lyx webster run --target-dir <repo> --plan-dir <plan>` for real. Confirm Master actually starts now (round 1 observed the pre-fix refusal: `"shuttle: NewRunner was told an anchor path ... outside its worktree root ..."`). This is a REAL LLM session — budget for it like any other (see cost declaration below).
- Confirm F22's fix live: after a `begin-batch`/`record-batch` cycle in standalone mode, `git status` in the target repo should show `.lyx/logs/trace-*.log` as excluded (not untracked) — read `LogsDir`'s own mechanism (likely a `.git/info/exclude` seed, mirroring hub mode's `fabricengine` approach) and confirm it's actually invoked at the right point.
- **Confirm hub mode is genuinely unaffected.** Round 1's own investigation found `NewRunner`'s containment check has FOUR callers; this fix's job was to leave that check untouched for all of them. Re-run loom's own hub-mode smoke tests (see "What to TEST") and, if time allows, a quick hub-mode `lyx loom run` sanity check (does not need to carry a glyph scenario — just confirm nothing about Master-spawning broke for the path loom itself uses).
- Look for edge cases the mill task's own review might have missed: what happens if `--target-dir` IS a hub worktree (misuse, not the documented use case) — does `NewDetachedRunner` do something surprising, or does earlier validation correctly reject it? What if `LogsDir`'s exclude-seeding races with a concurrent standalone invocation on the same target repo?

### Mission B — close what round 2 left open, then hunt for anything new

- **A declared `Rename` card executing through a real Webster batch.** Seed a live task where the board/discussion task explicitly gives `Plan-Write` a symbol that ALREADY EXISTS to rename (branch the sandbox hub off a state where the symbol is already present, e.g. build on round 2's own `glyph-demo-greet` result if the sandbox hub still has it, or seed a fresh repo with the target symbol already committed) — round 2's own gap was specifically that its planner had nothing pre-existing to rename. Confirm live: the `Rename` card's `plan:` handle to-side canonicalizes and binds correctly (round 1 proved this in the standalone rig; this closes the real-orchestrator gap), gate one in `DetectDrift` correctly recognizes the card's own outcome as not-drift (F2's fix, live through a real Webster batch for the first time).
- Independently re-verify rounds 1+2's fixes have not regressed under this round's own live driving (you don't need to re-prove each one from scratch — the handoff has the evidence — but flag immediately if your live driving happens to show one behaving wrong).
- General adversarial sweep: anything in `manifest/designs/loom.md` or the stencils that still doesn't match what you observe; anything a THIRD independent model/pass might catch that two prior ones (Opus, Sonnet) didn't.

## Explicitly OUT of scope for this round
- **`internal/planglyph`'s and `internal/planparser`'s own internal correctness** beyond how loom's rows and Webster's begin-batch/record-batch use them (Mission B) — unchanged from prior rounds' framing.
- **`internal/shuttleengine`'s pre-existing code beyond what `d7c52df6c` changed** (Mission A) — review the NEW fix adversarially, not a full re-audit of shuttleengine's entire pre-existing surface (that's a different campaign's job if ever warranted).
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Loom's general pipeline mechanics from the two PRE-glyph crucible campaigns (bootstrap, crash/resume, the review segments' generic round loop, Publish/Finalize) — don't re-verify from scratch.
- `Plan-Sweep` — does not exist, not needed.
- Windows path behavior — unreachable from this Linux host.
- Any new feature or roadmap work.

**Corrected from prior rounds' seeds: F16/F22 are NO LONGER "permanently out of loom's scope."** Rounds 1/2 said this because the fix hadn't landed yet and lived on a code path loom itself never takes. It has now landed in THIS branch (Mission A, above) — it is squarely in scope for this round, precisely because this branch is what merges to `main` next and everything in it needs this campaign's bar applied before that happens.

## Round context seeded from prior-round verification

**Rounds 1 (opus-high-r1) and 2 (sonnet-xhigh-r2) are BOTH CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

**Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: no multi-batch plan with a `Create`/`Delete`/`Rename` card could complete through Webster (whole-plan re-resolution against the post-change tree). Fixed via a scoped dispatch-boundary change (`ValidateDispatch`/`PendingPlan`). 9/9 sabotage-proofs independently passed.

**Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run`, real LLM sessions, a `Plan-Write`-authored glyph-bearing plan, carries cleanly through `Webster-Review` — independently confirmed genuine via the actual GitHub PR and actual git commits in the sandbox hub, not just narrative. 2/2 sabotage-proofs passed.

**`#004` (a separate mill task, this campaign's own spinoff from F16/F22):** completed its own mill flow, squash-merged to THIS branch as `d7c52df6c` — see "Mission A" above. This is NEW, UNREVIEWED-BY-CRUCIBLE material this round must cover.

**RESIDUAL — this round's actual mission, both required:**
1. Mission A: adversarial review + live verification of `d7c52df6c` (F16/F22's fix).
2. Mission B: safety pass over loom's glyph surface + attempt to close the live Rename-through-Webster gap.

State the **merge bar**: correctness in the NORMAL single-instance flow for BOTH missions — Mission A's fix genuinely working live without regressing hub-mode safety, Mission B's glyph surface holding under a third independent pass — is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module; so is Mission A's standalone webster run)

**`LLM-DRIVING: yes`, for BOTH missions.** Read this whole section before running anything.

**Mission A's standalone run.** A real standalone `lyx webster run` spawns Master exactly like hub mode does — one real `claude` subprocess for Master, plus one per fork per batch, strictly sequential (same shipped defaults as hub mode — no cluster-fan). Budget similarly: real wall-clock minutes per session, 15-30 min for a small demo task through a couple of batches.

**Mission B's hub-mode run (only if you attempt the Rename gap).** Same shape as round 2's: roughly one real session at a time, 4-8 sessions in series for a full pipeline pass, 20-45 min. If the sandbox hub from round 2 (`/home/knatte/Code/lyx-test-HUB`) still exists in a reusable state, building on it (rather than a fresh clone) may save setup time — check first.

- **Run at most one full-pipeline attempt at a time, per mission**, foreground, waited on to completion. Never start two live runs concurrently, even across missions.
- **Check your PATH's `lyx` before either mission's live driving** — round 2 found the installed binaries stale; confirm `which lyx`/`lyx --version` reflects current HEAD (`d7c52df6c`) before trusting any live result; re-deploy/reinstall if not.
- **Report the exact `lyx reed status`/`lyx reed attach` commands every time you start a live run.**

**Named smoke tests.** All of these function names begin with the substring `Smoke` — a bare `-run Smoke` pattern matches every single one of them simultaneously and is BANNED, full stop. Always name the exact one test function you mean to run.

`internal/loomcli/smoke_test.go` — 0 real subprocesses (providerless shuttle config):
`TestSmokeBootstrap_BringsUpSessionStrandAndDriver`, `TestSmokeBootstrap_SecondInvocationDoesNotSpawnASecondDriver`, `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`, `TestSmokeDriveStandalone_RefusesOnNeverSeededPair`, `TestSmokeDriveStandalone_FailureBeforeFirstPersistLeavesNonEmptyLog`, `TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove`, `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit`, `TestSmokeBootstrap_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit`, `TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver`, `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`.

`internal/loomcli/smoke_attachprobe_test.go`: `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` — 0 real subprocesses.

`internal/burlerengine/smoke_round_test.go`: `TestSmokeBurlerRoundToyFixture` — 1 real subprocess.

Check `internal/webstercli/*smoke*.go` and `internal/shuttleengine/*smoke*.go`/`internal/burlercli/*smoke*.go` for any smoke test `d7c52df6c` may have added (it touched `webstercli`/`burlercli`/`shuttleengine`) — name each exactly, check its own subprocess cost before running, same discipline as above.

**EXECUTION BAN**: `internal/burlerengine/smoke_cluster_test.go`'s `TestSmokeBurlerClusterCleanFan`/`TestSmokeBurlerClusterRogueFork` — 2 real subprocesses each, no cluster-fan configured anywhere in this campaign's scope. Reason: simultaneous real provider sessions exhaust the host's RAM.

- Never run more than one live-substrate (`-tags smoke`) invocation at a time, in parallel, or backgrounded.
- **The generic "N× CONCURRENT full smoke suites" gate does NOT apply to loom or to standalone webster, full stop.**

## What to TEST — do not just read, EXERCISE it

Hermetic (must stay green throughout) — NOTE the expanded package set for Mission A:
- `CGO_ENABLED=1 go build ./...`
- `CGO_ENABLED=1 go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/... ./internal/shuttleengine/... ./internal/standalonegeom/... ./internal/webstercli/... ./internal/burlercli/... ./internal/logger/...`
- `CGO_ENABLED=1 go test -count=5` over the same package sets + `./cmd/lyx/...`
- `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...`
- `CGO_ENABLED=1 go test ./...` (whole repo)

Live smoke — name the exact ONE test function each time (see cost declaration above for the full list).

Live driving — YOU drive it directly, for BOTH missions:
- Deploy the current source: `CGO_ENABLED=1 go run ./tools/deploy` (installs to `go env GOBIN`) before EVERY source change you want reflected in a live run, and confirm both PATH-resolvable `lyx` locations agree (round 2's own fix for the stale-binary hazard).
- Mission A: a plain git repo with real Go source, `lyx webster run --target-dir <repo> --plan-dir <plan>` directly (no hub, no fabric — that's the point).
- Mission B (if attempting the Rename gap): a real fabric hub with a pre-existing symbol to rename.
- **Report the exact `lyx reed status`/`lyx reed attach` commands to connect to whatever session you start, every time you start one.**
- "Headless" means "no human required" — NOT "no time/token cost to you." Forbidden reasons to skip: "operator-assisted", "cost-bearing", "long-running", "impractical".

TEARDOWN DISCIPLINE (critical): if you start any substrate server/session, tear it down. At the end, confirm ZERO stray substrate processes (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started, for BOTH missions' substrate). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). Tag each finding with its mission (A or B) in its heading. For scope: design-intent vs shipped; flag deferred-that-should-be-fixed and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2, including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round (an operator decision, a second real TTY) — say so explicitly in the fixer report's deferred section. A finding whose fix is genuinely LARGE (a subsystem addition, a cross-cutting refactor) gets marked NOT-FIXED-THIS-ROUND instead — record it fully, the orchestrator spins it into its own mill-wiki task afterward, exactly as happened for F16/F22 themselves.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None open. F16/F22 are resolved by `d7c52df6c` (Mission A verifies this, doesn't re-litigate whether they should be fixed). Round 2's "Rename through a real Webster batch" gap is Mission B's own residual, not a deferred item to merely re-evaluate — actively attempt to close it.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT, for BOTH missions.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- Prefer surgical edits; match existing style and the file-level doc-comment convention (note: Mission A's files belong to a different task's own style history — match THEIR conventions, not loom's, when editing `shuttleengine`/`standalonegeom`/`webstercli`/`burlercli`/`logger`).
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test.
- MAKE SMOKE TESTS DETERMINISTIC — poll with a deadline, never sleep a fixed amount.
- Update the relevant docs in the SAME change as the fix: `manifest/designs/loom.md`/`docs/overview.md`/`CONSTRAINTS.md` for Mission B; whatever doc governs Mission A's packages (check for a `standalonegeom`/`shuttleengine` design doc, or note in the fixer report if none exists and none is warranted for a narrow fix). Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY and re-run every live scenario yourself, directly.
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it — do NOT push unless the user explicitly asks.
- Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report — Executive summary with top risks + merge-readiness opinion for BOTH missions separately AND combined; explicit statements of (a) whether Mission A's fix is confirmed working live and hub-mode-safe, and (b) how far Mission B's live attempts got, including the Rename-through-Webster gap specifically; Scope assessment; Code findings severity-ranked and mission-tagged with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results. Write it to `_mill/loom-review-<yourtag>.md` and commit it incrementally.
2. A fixer report: what you implemented (mission-tagged), what you deliberately deferred, exact test commands + results, changed files. Write it to `_mill/loom-review-<yourtag>-fixer-report.md` and commit it.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict for both missions + an explicit yes/no on the Rename-through-Webster gap closing). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs for BOTH missions, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
