# `loom` — independent review + fix (prompt template) — ROUND 1 (glyph-hardening campaign)

> Filled instance of `crucible/review-prompt-template.md` for the `loom` module, round 1 of a NEW campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230). See [../../crucible/README.md](../../crucible/README.md) for the loop this prompt runs inside, and [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for this campaign's own charter.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `loom` module in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening` (branch `crucible-loom-glyph-hardening`).
Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of `loom`'s scope and correctness against the new glyph plan-format surface.
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
This matters MORE than usual this round: the primary live-driving activity (a real task carrying glyph mechanics through the full pipeline) can take tens of real minutes per attempt, so a mid-run crash without incremental commits would lose the most expensive evidence this round produces.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree (nothing should exist yet from this round).
The prior-campaign material referenced below under "What to read" is an explicit, deliberate EXCEPTION — you are told to read it as SPEC/history, not as a review to avoid — because it tells you what loom's *general* pipeline mechanics already proved out, so you don't re-spend this round's budget re-discovering that. Read it for that purpose; still form your own independent judgment of the LIVE behavior you observe on the NEW glyph surface, rather than assuming anything about the glyph mechanics specifically has ever been checked before — it hasn't.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- Code — the new glyph surface loom now drives, read for how loom's Plan-Validate/Plan-Revalidate rows and Webster's begin-batch/record-batch flow *use* it (do NOT re-review these packages' own internal correctness — see "Explicitly OUT of scope"):
  - `internal/planparser/**` — the plan format's sole parser/writer: `handle.go` (the `plan:` handle grammar), `classify.go` (the five-rule shape classifier: handle / glyph / self-glyph / path / bare symbol), `containment.go` (the syntactic `containment-unit-overlap` tier), `rewrite.go` (`RewriteRefs`), `amendment.go` (`AppendAmendment`).
  - `internal/planglyph/**` — the resolve-backed layer: `handle.go` (`CanonicalizeHandles`, called from `planglyph.Validate`/`ValidateFormat`, which is how loom's Plan-Validate/Plan-Revalidate rows reach it — see `manifest/designs/loom.md`'s "Plan-Validate detail"), `create.go` (the Create-inversion policy), `containment.go` (the resolve-backed `containment-file-overlap` tier), `resolve.go` (the resolve status policy: `found`/`multipart` pass, `ambiguous`/`not_found`/rejection block), `drift.go` (`DetectDrift` — the two gates, exact-tier auto-repair, evidence-tier logging-only), `donecheck.go`, `repo.go` (`ErrQuarryUnavailable`).
  - `internal/websterengine/beginbatch.go` (calls `planglyph.ValidateFormat` at `BeginBatch`'s top, blocking on `SeverityBlocking` findings) and `internal/websterengine/recordbatch.go` (calls `planglyph.DoneChecks`, then `planglyph.Delta`, then `planglyph.BindHandles`, `planglyph.ScopeGuard`, `planglyph.DetectDrift` in that order, inside `RecordBatch`) — this is the exact wiring the kickoff calls out as never yet exercised through loom's real phase machine.
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md` (the as-built mechanics spec for everything above — READ THIS FIRST, it is short and it is the authoritative source of intended behavior for this round), `manifest/designs/loom.md`, `contracts/specs/loom-plan-spec.md` (card grammar: `## Rename and Move`, `## Plan: handles` sections — the exact `plan:` handle / Create-declaration / Rename-pair syntax you'll need to author real cards with), `manifest/designs/webster-parallel-execution.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- Sandbox suite: none dedicated exists for loom. `tools/sandbox/SANDBOX-CORE-SUITE.md`'s scenario S8 carries a `**Covers:** loom` tag but is fixture-only — read it for scenario IDEAS only, it is not real coverage and you drive everything yourself directly regardless.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` — in particular the Cwd Resolution, Told-Geometry, Fabric Git, Review Round, Shed Recipe Registry, Lyxdirs Single-Declarer, Test Tier Purity, Config Strictness, Planparser Sole-Parser, Glyph Conversion Chokepoint, Quarry CGO Requirement, and Documentation Lifecycle invariants.
  **Note the Quarry CGO Requirement Invariant explicitly**: `lyx` links quarry's tree-sitter grammars via cgo, so every build/test/deploy command below needs `CGO_ENABLED=1` and a C compiler on `PATH` — this already defaults correctly on an ordinary dev machine with gcc/clang installed, so you don't need to set anything, but if any hermetic gate below fails with a cgo/linker error rather than a real logic error, check this first before treating it as a finding.
- Design intent (SPEC + history — recovered from git, since the worktrees themselves are long torn down):
  - `manifest/designs/quarry-glyph-plan-alphabet.md`'s own "Related" section points at GitHub issue #226 (full glyph-alphabet proposal) if you need more background than the doc itself carries.
  - Prior loom crucible campaigns (both merged to `main`, both scoped to loom's *general* pipeline mechanics, NEITHER ever touched the glyph surface — it didn't exist yet): round 1, `git show archive/loom-crucible-hardening~1:_mill/loom-review-opus5-high-r1.md`; round 2, `git show 980f2d480:_mill/loom-review-prompt.md` (the seed) and `git log --oneline | grep b48059972` (the merge commit) for round 2's own review/fixer reports under that commit's parent history. Read these for what NOT to re-litigate (see "Round context" below) — not as a review of anything glyph-related, since they predate PR #230 entirely.
  - PR #230 itself (`quarry-glyph-plan-alphabet`, merged to `main`) — `git show <merge-sha>` or `git log --oneline main | grep -i glyph` to find its exact commits if you want the as-built diff rather than just the design doc's prose summary.

## Mission (assess on two axes, be adversarial)
1. Scope — does loom's as-built pipeline correctly carry the new glyph plan-format through Plan-Write, Plan-Validate/Plan-Revalidate, Plan-Review, and Webster's begin-batch/record-batch, end to end, for real? The glyph mechanics themselves (resolve policy, Create inversion, containment tiers, drift detection) already have their own unit/integration suite from the task that built them — this round's job is the INTEGRATION surface: does loom's phase machine actually reach and correctly gate on every one of those mechanics when driven live, not whether the mechanics are individually correct in isolation.
2. Correctness — bugs, races, error handling, edge cases in that integration surface; concentrate on the four scenarios in "High-yield focus" below. Also assess docs accuracy (does `manifest/designs/loom.md` correctly describe how the pipeline now behaves with glyph targets?) and operability.

## High-yield focus — where the NEW glyph-surface bugs live (drive these, do not just read them)
Loom's general pipeline mechanics (bootstrap, crash/resume, the three review segments' round loop, Publish/Finalize) were already hardened across two prior crucible campaigns — see "Round context" below for what's CLOSED-AND-VERIFIED there. This round's mission is narrower and specific: drive a REAL dummy task through loom's real phase machine that exercises the glyph surface PR #230 landed, which nothing has ever driven live before. Treat each of these four as an INVARIANT you must actively verify by driving the real substrate — a green `go test` proves nothing here, since the glyph task's own unit/integration suite already covers the mechanics hermetically; what's unproven is loom actually wiring into them correctly.

- **1. A `Create` card using a `plan:` placeholder handle, through canonicalization and binding after the creating card lands.**
  Author a plan (via a real `Plan-Write` session, or if that proves too unreliable to steer toward a specific card shape, by hand-editing the `_lyx/plan/` files a real `Plan-Write` session produced, then re-running `Plan-Validate`/the real pipeline from there) containing a `Create` card whose sub-bullet is the two-field declaration grammar: `` `plan:<unit>#<NewSymbol>` -> `<declaration head>` `` (see `contracts/specs/loom-plan-spec.md`'s "Plan: handles" section for the exact grammar, `internal/planparser/handle_test.go` for worked examples like `` `plan:internal/foo#NewThing` -> `func NewThing() *Thing` ``).
  Confirm live: `Plan-Validate` (row 8, format-only mode) does NOT choke on the unresolvable handle (it's a draft, not yet a real glyph). `Plan-Revalidate` (row 10, approval-enforcing mode, post `Plan-Review`) or a standalone `planglyph.Validate` call canonicalizes it via `CanonicalizeHandles` — confirm the handle rewrites plan-wide to its canonical `plan:<expected-glyph>` form via `planparser.RewriteRefs`, and that every card referencing the draft handle (not just the declaring card) gets the rewrite. Then, once Webster actually creates the symbol in that card's batch, confirm `RecordBatch`'s `BindHandles` call (via the batch's `Delta`) binds the handle to its real glyph and that a later card's `Uses`/target referencing the same handle now resolves correctly against the real symbol.
- **2. A `Rename` card whose to-side is a `plan:` handle, hitting the exact-tier auto-bind path.**
  Author a `Rename` card: `Old` side a real, resolvable glyph (something already in the repo, or created by an earlier card in the same plan); `New` side a `plan:` handle (required — a symbol `Rename`'s `New` side classifies as `rename-to-not-handle` if it's anything else). Remember the plan-level `## Rename mechanic` section is required in `00-overview.md` whenever any card is type `Rename` (`rename-mechanic-missing` check). Drive the batch that performs the rename for real through Webster, then confirm live that `RecordBatch`'s delta correctly reports the rename, that gate one in `DetectDrift` (`renameCardPairs`) recognizes it as the card's OWN expected outcome (no drift finding, no auto-repair — it's not drift, it's the plan working as designed) — this is the exact-tier "auto-bind" the kickoff names, distinct from the exact-tier auto-REPAIR in scenario 3 below, which fires on an rename NOT matching a declared pair.
- **3. A deliberate drift scenario — a symbol referenced elsewhere in the plan gets renamed/deleted mid-campaign — to exercise the auto-repair (exact-tier) and review-surfaced (evidence-tier) paths.**
  Two sub-scenarios, both live:
  - **Exact-tier auto-repair**: after a plan is approved and Webster has begun executing it, have a batch's own agent (or you, standing in for one, editing the warp source directly then letting the batch's own commit/delta capture it) rename a symbol some OTHER, not-yet-executed card in the same plan still references — via quarry's own AST-exact rename detection (a real Go rename, not a delete+recreate that merely looks similar). Confirm live: `DetectDrift`'s gate one does NOT match (no declared `Rename` card pairs this), gate two DOES match (some other card still references the old glyph) — so it queues a `driftRepair`, calls `planparser.RewriteRefs` plan-wide, revalidates via one batched resolve, and appends exactly one `planparser.Amendment` with `Tier: "exact"` to `amendments.md`. Confirm the later card's own `Uses`/targets now resolve against the NEW glyph without any human intervention.
  - **Evidence-tier, review-surfaced**: engineer a rename quarry's delta engine classifies as a `RenameCandidate` rather than an exact `Renamed` entry (e.g. change enough of the body that `body_token_similarity` drops below whatever threshold makes it inexact, per quarry's own signals — you may need to read quarry's delta-classification logic, or just try a few real edits and observe which bucket the delta actually puts them in). Confirm live: `DetectDrift` produces an INFORMATIONAL `rename-candidate` finding (never auto-repairs, never calls `RewriteRefs`/`AppendAmendment` for this tier) and that finding actually surfaces somewhere a human/reviewer sees it (trace it through to `Webster-Review` or wherever `RecordBatch`'s findings ultimately get reported — confirm this end-to-end path exists and isn't silently dropped).
  - Also confirm the plain-blocking case: a genuinely deleted symbol (no rename, no candidate) that some other card still references produces the blocking `plan-references-deleted-symbol` finding and actually blocks (`RecordBatch`/`BeginBatch` refuses to proceed), rather than silently passing.
- **4. The Create inversion policy and the two containment tiers, live.**
  - Create inversion: author a `Create` card targeting a glyph that ALREADY exists (`found`/`multipart`) — confirm it's correctly blocked (`create-already-exists`), live, not just in a unit test. Author a `Create` card targeting a genuinely new symbol in an EXISTING package (`not_found` with `unit: found`) — confirm it passes with no finding. Author a `Create` card targeting a symbol in a brand-new package/directory (`not_found` with `unit: not_found`) — confirm it passes but with the informational `create-new-unit` finding, and that finding names the new unit correctly.
  - Containment tiers: author two cards in the same plan where one targets a member glyph and the other targets the file self-glyph of the file that member lives in (or will live in, for a `Create`) — confirm `containment-unit-overlap` (syntactic, `planparser`) or `containment-file-overlap` (resolve-backed, `planglyph`, reading `ResolveResult.Symbols[].File`) actually fires live and actually prevents whatever blind-parallel-dispatch/merge-conflict scenario it exists to prevent — note per `manifest/designs/webster-parallel-execution.md` that Webster's execution today is strictly sequential, so "prevents blind parallel dispatch" may currently only be provable as "the finding fires correctly", not as "two cards were prevented from actually racing" — say so explicitly if that's what you find, don't overclaim a live race you couldn't actually construct.

Beyond these four, this list is a FLOOR — if time remains, look for anything else the glyph surface's landing might have missed: an infrastructure error (`ErrQuarryUnavailable`) from a mid-batch `quarry.Open`/`Resolve` failure surfacing as a plan finding instead of a gate/infrastructure failure (the exact disaster `manifest/designs/quarry-glyph-plan-alphabet.md`'s "infrastructure-error disposition" section says this distinction exists to prevent — see if you can actually trigger it, e.g. by breaking quarry's resolve mid-run), a `language: "none"` plan (opts out of the glyph alphabet entirely) still working correctly through the same pipeline rows, or any doc drift between `manifest/designs/loom.md`'s pipeline description and what you actually observe with glyph targets in play.

## Explicitly OUT of scope for `loom`
- `internal/planglyph`'s and `internal/planparser`'s own internal correctness beyond how loom's Plan-Validate/Plan-Revalidate rows and Webster's begin-batch/record-batch flow use them — these packages shipped with their own unit/integration suite (PR #230) and have not had a dedicated crucible campaign of their own; if you find something that looks like a genuine bug in their own internals (not just in how loom wires into them), record it as a finding and note in the fixer report whether fixing it belongs to this round or to a future planglyph/planparser-scoped campaign — use judgment, but don't silently skip it.
- `quarry`'s own resolve/delta engine correctness (the tree-sitter-backed symbol resolution, the AST-exact rename classification, the `body_token_similarity` threshold) — that's a separate, external module (`github.com/Knatte18/quarry`); treat its answers as ground truth for this campaign's purposes.
- Loom's general pipeline mechanics already hardened by the two prior campaigns (bootstrap, crash/resume, the review segments' generic round loop, Publish/Finalize) — see "Round context" below for the CLOSED-AND-VERIFIED list. Don't re-verify these from scratch; DO re-verify them opportunistically if your glyph-driving happens to exercise them (e.g. a crash mid a glyph-carrying Webster batch is fair game — see scenario 3 above) and something looks off.
- `Plan-Sweep` — does not exist, not needed for this campaign.
- Windows path behavior — unreachable from this Linux host; do not reason about it as if driven.
- Any new feature or roadmap work. This is a hardening pass on already-shipped behavior only.

## Round context seeded from prior-round verification

**Round 1 of THIS campaign.** There is no prior verification within this campaign to report a residual from — instead, this round is seeded directly with the campaign's own mission, per the orchestrator kickoff: drive the four glyph-mechanics scenarios in "High-yield focus" above through loom's real phase machine, for the first time ever.

**CLOSED-AND-VERIFIED from PRIOR loom campaigns — do not re-litigate these (they predate the glyph surface and are unrelated to it):**
- All of loom-crucible-hardening round 1's 17 findings (merged `a0612b30e`) and round 2's residual-closing findings (merged `b48059972`) — general pipeline structure, bootstrap, crash/resume ladder for Discussion-Write/Plan-Write, the `approve_seam`/`require_approved` mechanism's own plumbing (not its NEW interaction with glyph canonicalization, which IS this round's business), the three review segments' generic round loop.
- Round 2's own primary achievement: a full live `lyx loom run`, Preflight through Finalize, completing at least once on a minimal non-glyph task, with Webster chained from a real `Plan-Write` output.
- Independently re-verify `go build ./...` / `go vet ./...` / `go test ./...` clean on the current `main`-derived tree before you start (not trusting any prior task's own merge-ready claim) — record the exact counts in your review report; if anything is unexpectedly red, that itself is worth a finding before you even get to the glyph scenarios.

**RESIDUAL / this round's actual mission — see "High-yield focus" above for the full detail:**
1. Create-card `plan:` handle through canonicalization and post-Create binding.
2. Rename-card `plan:` handle to-side, exact-tier auto-bind.
3. Drift: exact-tier auto-repair, evidence-tier review-surfaced, plain-blocking deleted-and-referenced.
4. Create inversion (both directions) and both containment tiers.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow — each of the four scenarios above actually working live, end to end, at least once — is the gate. Do not chase artificial concurrency stress for this module; see the cost declaration below for why.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes.`** This round's PRIMARY activity — driving real glyph scenarios through loom's real phase machine — is itself the most expensive thing in this declaration, separate from and in addition to the named smoke tests below. Read this whole section before running anything.

**The full-pipeline live run(s).** Per the confirmed shipped default (loom's recipe configures NO `cluster-fan` key on any of the three Burler rows — `contracts/recipes/loom-recipe.yaml`'s Webster-Burler row comment says so explicitly, and the other two omit it identically), and per `internal/websterengine`'s per-batch loop being strictly sequential (no goroutine fan-out spawning concurrent LLM sessions), the WORST CASE for one full dummy task end to end is **exactly one real `claude` subprocess alive at any given moment** — every phase (Discussion-Write, Plan-Write, each Bouncer-judge/Burler-round call, Webster's per-batch Master session) runs strictly one session at a time, in series, never concurrently. A clean pass against a genuinely minimal one-or-two-card task spawns roughly 4–8 real sessions in series across the whole pipeline (Discussion-Write, Plan-Write, up to three Bouncer-judge calls, up to three Burler-round calls if any gate bounces, one-to-a-few Webster batch sessions). Each real session costs real wall-clock MINUTES, not seconds (per `crucible/README.md`) — plan for 20–45 real minutes per full clean pass, likely MORE than round 2's estimate since you'll need multiple passes (or one plan carrying multiple cards) to hit all four scenarios above, and some scenarios (drift, containment) require deliberately engineering an out-of-band code change mid-campaign, which costs extra real time to set up correctly.
- **Use a genuinely minimal real task**, but craft its discussion/plan content deliberately toward the four scenarios above rather than letting `Plan-Write` free-associate — you may need to steer the Discussion-Write session's task description explicitly toward "create a new symbol, then rename it, then reference it from a second card" shaped work so `Plan-Write` has a natural reason to emit a `Create` card with a `plan:` handle and a `Rename` card whose `New` side is a handle. If a real `Plan-Write` session won't reliably produce the exact card shapes you need even with a steered task description, it is acceptable to hand-edit `_lyx/plan/` after a real `Plan-Write` run to engineer the precise scenario, then re-run `Plan-Validate`/the rest of the pipeline for real from there — say explicitly in your report which cards were LLM-authored vs. hand-engineered, since that's a material fact about how much of the glyph surface you actually proved was reachable from a real planning session versus only from the mechanical layer.
- **Run at most one full-pipeline attempt at a time**, foreground, waited on to completion. Never start a second live run while one is still in flight.
- **Report the exact `lyx reed status`/`lyx reed attach` commands every time you start a live run** — the operator may be watching live and needs the socket/session name to attach from their own terminal.

**Named smoke tests.** All of these function names begin with the substring `Smoke` — a bare `-run Smoke` pattern matches every single one of them simultaneously and is BANNED, full stop, for this module. Always name the exact one test function you mean to run.

`internal/loomcli/smoke_test.go` — each spawns 0 real subprocesses in practice (the shuttle config is deliberately providerless — `providerlessShuttleConfig()` points at a nonexistent claude binary so no fixture in this file can ever launch a real provider), inside its own outer test timeout:
- `TestSmokeBootstrap_BringsUpSessionStrandAndDriver`
- `TestSmokeBootstrap_SecondInvocationDoesNotSpawnASecondDriver`
- `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`
- `TestSmokeDriveStandalone_RefusesOnNeverSeededPair`
- `TestSmokeDriveStandalone_FailureBeforeFirstPersistLeavesNonEmptyLog`
- `TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove`
- `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit`
- `TestSmokeBootstrap_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit`
- `TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver`
- `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`

`internal/loomcli/smoke_attachprobe_test.go`:
- `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` — 0 real subprocesses (a stub `shellLaunchEngine` stands in for the provider entirely).

`internal/burlerengine/smoke_round_test.go`:
- `TestSmokeBurlerRoundToyFixture` — 1 real subprocess.

**EXECUTION BAN** (do NOT run these this round — not for extra confidence, not if there's time — loom's own recipe never configures cluster-fan on any Burler row, so these are burlerengine's own tests, entirely out of this round's mission):
- `internal/burlerengine/smoke_cluster_test.go`: `TestSmokeBurlerClusterCleanFan` — 2 real subprocesses (one handler spawning one real fork subagent).
- `internal/burlerengine/smoke_cluster_test.go`: `TestSmokeBurlerClusterRogueFork` — 2 real subprocesses, same shape.
- Reason the ban exists: simultaneous real provider sessions exhaust the host's RAM — confirmed by a real prior incident (see `crucible/README.md`).

`internal/webstercli/smoke_test.go`'s `TestSmoke_*` tests are websterengine's own module tests, out of loom's own module scope (see "Explicitly OUT of scope" above) — do not run them as part of this campaign's own gates, but a stray bare `-run Smoke` anywhere in the repo would match them too (`TestSmoke_ForkGuardHookDeniesForkPayload`, `TestSmoke_ForkContextGuardDeniesLiveFork`, `TestSmoke_ForkTranscriptAuditCountsOneNoNestedAgent`, `TestSmoke_RecordBatchConsumesCrashedSessionReport`, `TestSmoke_AwaitBatchSeesForkWrittenReport`), which is one more reason the bare-pattern ban is absolute, not just scoped to loom's own packages.

- Never run more than one live-substrate (`-tags smoke`) invocation at a time, in parallel, or backgrounded — one process, foreground, waited on to completion. This applies equally to the primary full-pipeline live run(s) above.
- **The generic "N× CONCURRENT full smoke suites" gate in the orchestrator's verification protocol does NOT apply to loom, full stop — do not run it this round, under any framing, with or without operator sign-off.** Even a SINGLE full-pipeline live run already spawns several real sessions serially over tens of minutes; N concurrent copies would multiply that by N simultaneous real `claude` processes, which is a materially worse version of the exact incident `crucible/README.md` documents.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/...`
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag) — name the exact ONE test function each time, per the cost declaration above:
- `go test -tags smoke ./internal/loomcli/... -run TestSmokeBootstrap_BringsUpSessionStrandAndDriver -v -count=1`
- (repeat per exact test name from the list above as needed — never a bare `-run Smoke`)

Live driving — YOU drive it directly, no launcher (PRIMARY — where this round's actual mission lives):
- Deploy the current source as the dev binary under test: `deploy-dev` (POSIX, this repo's script at the worktree root) before EVERY source change you want reflected — live driving runs the deployed snapshot, not your working tree.
- Do NOT invoke any `sandbox-<module>-suite` launcher — none exists for loom anyway.
- Run the real CLI commands yourself, directly, foreground, waiting for each to return: `lyx loom run`, `lyx loom status`, `lyx reed status`, `lyx reed attach`, `lyx fabric` verbs as needed to seed a fresh worktree pair with a minimal real task steered toward the four glyph scenarios. This spawns real substrate underneath — real tmux panes, real interactive `claude` sessions — that is expected and required.
- **Report the exact `lyx reed status`/`lyx reed attach` commands to connect to whatever session you start, every time you start one** — the operator may be watching live.
- The high-yield focus list above is the actual mission, not a floor to skim past — do not let a partial pass on scenario 1 crowd out ever attempting scenarios 2–4. If you genuinely run out of time, report exactly which of the four you completed, which you attempted and hit a blocker on (name the blocker), and which you never attempted at all — do not silently omit one.
- "Headless" means "no human required" — NOT "no time/token cost to you." A real substrate session takes real wall-clock MINUTES. That cost is expected and budgeted for. You are explicitly forbidden from writing "operator-assisted", "cost-bearing", "long-running", or "impractical" as a reason to skip live driving — only a scenario that structurally requires a human's physical eyes (e.g. a visual render check) or a genuine environment gap is a legitimate "cannot verify headlessly".

TEARDOWN DISCIPLINE (critical): if you start any substrate server/session, tear it down. At the end, confirm ZERO stray substrate processes (`ps aux | grep -i tmux` — must show nothing of yours left running, and no orphaned `claude`/driver processes either). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: design-intent vs shipped; flag deferred-that-should-be-fixed and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2, including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round (an operator decision, a second real TTY) — say so explicitly in the fixer report's deferred section. A finding whose fix is genuinely LARGE (a subsystem addition, a cross-cutting refactor) gets marked NOT-FIXED-THIS-ROUND instead — record it fully, the orchestrator spins it into its own mill-wiki task afterward.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None — this is round 1 of this campaign.

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
1. A structured review report — Executive summary with top risks + merge-readiness opinion, and an explicit statement of which of the four glyph scenarios were actually driven live to completion; Scope assessment; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including what you could NOT verify and why. Write it to `_mill/loom-review-<yourtag>.md` and commit it incrementally as described above.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Write it to `_mill/loom-review-<yourtag>-fixer-report.md` and commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict + an explicit per-scenario yes/no on "did this glyph scenario get driven live to completion"). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
