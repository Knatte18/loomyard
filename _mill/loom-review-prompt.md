# `loom` — independent review + fix (prompt template) — ROUND 2 (glyph-hardening campaign)

> Filled instance of `crucible/review-prompt-template.md` for the `loom` module, round 2 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230). See [../../crucible/README.md](../../crucible/README.md) for the loop this prompt runs inside, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for this campaign's own charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's running state (you may read the handoff note only AFTER you have your own independent findings list — see "Clean-room review constraint" below).

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
This matters MORE than usual this round: the primary live-driving activity — a real `lyx loom run` with real LLM sessions — can take tens of real minutes per attempt, so a mid-run crash without incremental commits would lose the most expensive evidence this round produces.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including round 1's own `loom-review-opus5-high-r1.md`/`loom-review-opus5-high-r1-fixer-report.md` AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment — the handoff note is the orchestrator's own private state, not a review, but it matches the pattern and is exactly as off-limits until you have your own list.
AFTER you have your own independent findings, you MAY (and should) consult round 1's material — see "What to read" below — to (a) confirm round 1's 20 fixes have not regressed and (b) understand this round's actual mission, which depends on round 1's own stated residual.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- Code — loom's own machinery: `internal/loomengine/**`, `internal/loomcli/**`, `internal/loomrecipe/**`, `internal/loomshed/**`, `internal/shedengine/**`, `internal/shedadapters/**`, `internal/shedrecipe/**`, `internal/shedbuild/**`, `internal/hubgeom/**`, `contracts/recipes/loom-recipe.yaml`, `cmd/lyx`'s loom integration.
- Code — the glyph surface loom now drives, read for how loom's Plan-Validate/Plan-Revalidate rows and Webster's begin-batch/record-batch flow *use* it (do NOT re-review these packages' own internal correctness — see "Explicitly OUT of scope"): `internal/planparser/**`, `internal/planglyph/**` (round 1 changed `handle.go`, `drift.go`, `planglyph.go`, `create.go`, `containment.go`, `scope.go` — read the CURRENT state of each, not just the design doc, since round 1's fixes are now part of "how this works"), `internal/websterengine/beginbatch.go` and `recordbatch.go` (round 1 added `completedCards`/`ValidateDispatch`/fingerprint re-baseline to both), `internal/websterengine/fingerprint.go` (round 1 added `restampFingerprint`).
- Docs: `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md` (round 1 corrected rows 8/10's Plan-Validate/Plan-Revalidate description — read the CURRENT text), `contracts/specs/loom-plan-spec.md` (round 1 corrected the Rename worked example), `contracts/stencils/loom/loom-template-plan.md` (round 1 corrected the Create declaration-head example — **this is what a real `Plan-Write` session reads to learn the grammar, so its correctness is directly on this round's critical path**), `manifest/designs/webster-parallel-execution.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- Sandbox suite: none dedicated exists for loom. `tools/sandbox/SANDBOX-CORE-SUITE.md`'s scenario S8 carries a `**Covers:** loom` tag but is fixture-only — read it for scenario IDEAS only.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` — in particular the Cwd Resolution, Told-Geometry, Fabric Git, Review Round, Shed Recipe Registry, Lyxdirs Single-Declarer, Test Tier Purity, Config Strictness, Planparser Sole-Parser, Glyph Conversion Chokepoint, Quarry CGO Requirement, and Documentation Lifecycle invariants.
  **Note the Quarry CGO Requirement Invariant explicitly**: every build/test/deploy command needs `CGO_ENABLED=1` and a C compiler on `PATH` — already the default on an ordinary dev machine.
- Round 1's own material (read AFTER your own findings list, per "Clean-room review constraint" above): `_mill/loom-review-opus5-high-r1.md` (the full review: 22 findings, live-scenario transcripts, per-scenario verdict table), `_mill/loom-review-opus5-high-r1-fixer-report.md` (what was fixed and how, incl. the one design-shaped change `c6ee9eb16`), `_mill/loom-review-HANDOFF.md` (the orchestrator's independent-verification record — 9/9 sabotage-proofs passed, all gates green cold; read this to understand exactly what has and has not been independently confirmed, not just self-reported).
- Prior (pre-glyph) loom crucible campaigns, for background only, NOT this round's business: `git show archive/loom-crucible-hardening~1:_mill/loom-review-opus5-high-r1.md` (round 1 of the OLD campaign — note the tag collision with THIS campaign's round 1; disambiguate by commit context if you look), `git show 980f2d480:_mill/loom-review-prompt.md` (OLD campaign round 2's seed).

## Mission (assess on two axes, be adversarial)
1. **Scope/integration — PRIMARY THIS ROUND.** Round 1 proved every glyph mechanism works when driven through hand-authored plans and direct bracket-verb calls (webster's standalone mode, zero LLM cost — the "standalone probe harness"). It could NOT drive a real `lyx loom run` (hub mode, real LLM sessions, a plan actually authored by a real `Plan-Write` session) because of three environment gaps, two of which are standalone-mode-specific and do NOT apply to hub mode (see "Round context" below). This round's primary job is to do what round 1 could not: run the real thing, hub mode, real sessions, and confirm the now-fixed mechanics survive contact with a real planning session and a real Webster hub run — not a rig.
2. Correctness — bugs, races, error handling, edge cases you find along the way, including in round 1's own fixes if a real run exposes something the standalone harness couldn't (e.g. real `Plan-Write` producing a card shape round 1 never tried, or the real Master/fork-transcript machinery interacting with `BindHandles`/`DetectDrift` in a way the standalone bracket-verb calls never exercised). Also re-check docs accuracy given round 1's own corrections.

## High-yield focus — this round's actual mission (drive these, do not just read them)

**PRIMARY — a real `lyx loom run`, hub mode, real LLM sessions, has never once carried a glyph scenario through the real phase machine.** Round 1 explicitly could not attempt this (see "What could NOT be verified" in its review report). Achieve it now:

- **Set up a real hub** with a Go-backed warp repository (`lyx fabric add` / `hubforge`-shaped, or whatever the current real CLI flow is — check `internal/loomcli/smoke_test.go`'s `newWiredPairFixture` for the shape a fixture needs, then do the equivalent for real, not as a Go test fixture). The warp repo needs real Go source for quarry to resolve glyphs against — an empty/non-Go repo is exactly the gap that blocked round 1.
- **Check the `lyx` your own shell resolves from PATH before trusting ANY live result.** Round 1 found the installed `lyx` on this host predates the glyph landing entirely (`manifest/designs/loom.md`'s "Agent execution" section documents this exact hazard: every agent loom spawns resolves `lyx` from its own shell PATH, and a stale one will silently rewrite the hub's stencils back to pre-glyph text mid-run). Confirm the resolved `lyx --version`/build timestamp reflects current HEAD before spawning anything; if it's stale, fix your PATH (or reinstall) BEFORE starting, not after a run produces confusing results.
- **Steer Discussion-Write toward a task that naturally produces a glyph-bearing plan**: something shaped like "add a new helper function and use it from an existing one, then rename the helper" — concrete enough that `Plan-Write` plausibly emits a `Create` card with a `plan:` handle and a `Rename` card whose `New` side is a handle, without you hand-editing the plan afterward. If a real `Plan-Write` session's output needs a nudge to hit the exact shape (see round 1's own note on this), that's fine and worth reporting — but the goal is to prove the CURRENT stencil (round 1 fixed its unparseable example, F15) actually teaches a real planner the correct grammar, so lean toward steering the task rather than hand-editing the output.
- **Drive it all the way through Webster for real** — real `begin-batch`/`record-batch` calls issued by webster's own Master session and per-batch forks, not by you calling the bracket verbs directly. Confirm: the plan canonicalizes and validates through `Plan-Validate`/`Plan-Revalidate` exactly as round 1's standalone probe showed; the Create card's handle binds correctly after its batch; a declared `Rename` card's own outcome is NOT misclassified as drift (F2's fix); `begin-batch 2` does NOT wedge on a stale fingerprint (F4's fix) or a corrupted declaration (F3/F17's fix). If the whole plan completes, push it as far as `Webster-Review`/`Finalize` as time allows.
- **Report exactly how far the real run got**, honestly. If it wedges somewhere round 1's fixes should have closed, that is this round's most valuable possible finding — it means round 1's mechanical-layer proof didn't fully transfer to the real orchestrator path, and you should dig into why (a difference between how Master invokes the bracket verbs vs. how round 1's rig called them directly is the first thing to suspect).

**SECONDARY — an adversarial sweep for anything round 1's method couldn't reach**, per the crucible README's own guidance ("derive each round's assignment from what your verification leaves standing" / "when the countable classes are closed, switch to regions nothing has ever driven"). Round 1's own standalone-probe method has a structural blind spot: it never drove a REAL Claude fork/transcript session interacting with the glyph mechanics (its runs 4–6 seeded the transcript as a fixture). If your primary mission's real run doesn't surface anything on its own, look specifically at:
- Whether a real Webster fork's own commit/report-writing interacts oddly with `BindHandles`'/`DetectDrift`'s own `RewriteRefs` calls (round 1 traced this at the code level but never watched a real fork do it).
- Whether `Plan-Review`'s rubric (a real Bouncer-judge session) correctly evaluates a plan carrying `plan:` handles — nothing has ever driven this live; round 1 didn't reach `Plan-Review` at all in its standalone rig.
- Anything in `manifest/designs/loom.md`'s pipeline description that still doesn't match what you observe, beyond what round 1 already corrected.

## Explicitly OUT of scope for `loom`
- **Round 1's 20 fixed findings (F1–F15, F17–F21) — CLOSED-AND-VERIFIED, do not re-review from scratch.** See "Round context" below for the full list and the independent-verification evidence. DO flag it immediately if your live driving happens to show one of these regressing — that would be a serious finding, not something to silently re-fix.
- **F16 and F22 — permanently out of loom's scope**, confirmed by round 1's own code-level trace AND this orchestrator's independent verification: loom never constructs `standalonegeom`/uses webster's standalone mode (grepped `internal/loomcli`/`loomengine`/`loomshed`/`loomrecipe` — zero hits), so a bug in standalone Master-start (F16) or standalone's untracked `.lyx/logs/` (F22) cannot bite loom. Do not re-investigate; do not fix them if you happen to notice them again.
- `internal/planglyph`'s and `internal/planparser`'s own internal correctness beyond how loom's Plan-Validate/Plan-Revalidate rows and Webster's begin-batch/record-batch flow use them.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Loom's general pipeline mechanics hardened by the two PRE-glyph crucible campaigns (bootstrap, crash/resume, the review segments' generic round loop, Publish/Finalize) — don't re-verify from scratch; DO flag if your real run happens to expose a regression.
- `Plan-Sweep` — does not exist, not needed.
- Windows path behavior — unreachable from this Linux host.
- Any new feature or roadmap work.

## Round context seeded from prior-round verification

**Round 1 (opus-high-r1) is CLOSED-AND-VERIFIED — independently confirmed by this orchestrator, not just self-reported.** 22 findings (13 BLOCKING, 5 MEDIUM, 3 LOW, 1 NIT); 20 fixed across 14 commits (`c2b64e001`..`9726310b6`), 2 (F16, F22) correctly left unfixed as genuinely out of loom's scope. Independent verification: 9/9 sabotage-proofs passed (production hunk reverted → regression test fails at the intended assertion → restored, empty diff) covering every BLOCKING finding's commit; all hermetic/integration/named-smoke gates re-run green cold; the one design-shaped change (`c6ee9eb16`, two new `internal/planglyph` exported functions scoping the dispatch-boundary re-resolution to pending cards) independently judged correctly scoped, not a Hard-Rule-5 oversized change; F16/F22's out-of-loom's-reach claims independently confirmed by code, not just asserted; all three doc corrections (F13, F15, F19) confirmed landed with correct text. Full detail: `_mill/loom-review-HANDOFF.md`.

**What round 1 fixed, so you don't re-litigate it:** no multi-batch plan with a `Create`/`Delete`/`Rename` card could complete through Webster (root cause: whole-plan re-resolution against the post-change tree on every batch dispatch, F5/F6/F7/F21); a `Create` handle's own declaration was corrupted by binding (F3/F17); webster's own sanctioned plan rewrites tripped its own staleness guard (F4); a declared `Rename`'s own expected outcome was misclassified as drift (F2); no method could be renamed through the glyph alphabet at all (F9); the evidence-tier drift path was dead twice over (F1, F18); `create-new-unit` never fired for a handle-declared Create (F14); plus F8, F10, F11, F12, F20 (smaller correctness fixes) and F13/F15/F19 (doc corrections).

**RESIDUAL — this round's actual mission, see "High-yield focus" above:**
1. A real `lyx loom run`, hub mode, real LLM sessions, carrying a glyph-bearing plan (ideally `Plan-Write`-authored, not hand-edited) through Webster for the first time ever.
2. Whatever the SECONDARY sweep above turns up if the primary mission leaves time.

State the **merge bar**: correctness in the NORMAL single-instance flow — the real run above reaching as far into the pipeline as a correct implementation should, with no glyph-surface defect blocking it — is the gate. Do not chase artificial concurrency stress for this module.

## Live-substrate cost declaration (loom IS an LLM-driving module)

**`LLM-DRIVING: yes.`** This round's PRIMARY activity — a real `lyx loom run` — is the most expensive thing in this declaration, separate from and in addition to the named smoke tests below. Read this whole section before running anything.

**The full-pipeline live run.** Confirmed shipped default: no `cluster-fan` key on any of the three Burler rows, and `internal/websterengine`'s per-batch loop is strictly sequential — so the worst case is **exactly one real `claude` subprocess alive at any given moment**, never concurrent. A clean pass against a genuinely minimal one-or-two-card task spawns roughly 4–8 real sessions in series (Discussion-Write, Plan-Write, up to three Bouncer-judge calls, up to three Burler-round calls if any gate bounces, one-to-a-few Webster batch/fork sessions). Each real session costs real wall-clock MINUTES — plan for 20–45 real minutes for a full clean pass, possibly more if `Plan-Write` needs a retry to hit the right card shape, or if a Burler round bounces.
- **Set up your hub and check your PATH's `lyx` BEFORE spawning anything real** — see "High-yield focus" above. Wasting a real session's cost on a stale binary is exactly the failure mode round 1's own report warns about.
- **Run at most one full-pipeline attempt at a time**, foreground, waited on to completion. Never start a second live run while one is still in flight.
- **Report the exact `lyx reed status`/`lyx reed attach` commands every time you start a live run** — the operator may be watching live.

**Named smoke tests.** All of these function names begin with the substring `Smoke` — a bare `-run Smoke` pattern matches every single one of them simultaneously and is BANNED, full stop, for this module. Always name the exact one test function you mean to run.

`internal/loomcli/smoke_test.go` — each spawns 0 real subprocesses in practice (providerless shuttle config), inside its own outer test timeout:
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
- `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` — 0 real subprocesses.

`internal/burlerengine/smoke_round_test.go`:
- `TestSmokeBurlerRoundToyFixture` — 1 real subprocess.

**EXECUTION BAN** (do NOT run these this round — loom's own recipe never configures cluster-fan on any Burler row):
- `internal/burlerengine/smoke_cluster_test.go`: `TestSmokeBurlerClusterCleanFan`, `TestSmokeBurlerClusterRogueFork` — 2 real subprocesses each.
- Reason: simultaneous real provider sessions exhaust the host's RAM — confirmed by a real prior incident (see `crucible/README.md`).

`internal/webstercli/smoke_test.go`'s `TestSmoke_*` tests are out of loom's own module scope — do not run them, but a stray bare `-run Smoke` would match them too.

- Never run more than one live-substrate (`-tags smoke`) invocation at a time, in parallel, or backgrounded.
- **The generic "N× CONCURRENT full smoke suites" gate does NOT apply to loom, full stop.** Do not run it, under any framing.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/...`
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/... ./cmd/lyx/...`
- `go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/...`

Live smoke (real substrate, behind the `smoke` build tag) — name the exact ONE test function each time:
- `go test -tags smoke ./internal/loomcli/... -run TestSmokeBootstrap_BringsUpSessionStrandAndDriver -v -count=1`
- (repeat per exact test name from the list above as needed — never a bare `-run Smoke`)

Live driving — YOU drive it directly (PRIMARY — where this round's actual mission lives):
- Deploy the current source as the dev binary under test: `deploy-dev` (POSIX) before EVERY source change you want reflected.
- Set up a real hub with a Go-backed warp repo; confirm your PATH's `lyx` is current (see "High-yield focus" above) — do this BEFORE any real session.
- Run the real CLI commands yourself, directly, foreground, waiting for each to return: `lyx loom run`, `lyx loom status`, `lyx reed status`, `lyx reed attach`, `lyx fabric` verbs as needed.
- **Report the exact `lyx reed status`/`lyx reed attach` commands to connect to whatever session you start, every time you start one.**
- "Headless" means "no human required" — NOT "no time/token cost to you." A real substrate session takes real wall-clock MINUTES. You are explicitly forbidden from writing "operator-assisted", "cost-bearing", "long-running", or "impractical" as a reason to skip live driving.

TEARDOWN DISCIPLINE (critical): if you start any substrate server/session, tear it down. At the end, confirm ZERO stray substrate processes (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: design-intent vs shipped; flag deferred-that-should-be-fixed and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2, including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round (an operator decision, a second real TTY) — say so explicitly in the fixer report's deferred section. A finding whose fix is genuinely LARGE (a subsystem addition, a cross-cutting refactor) gets marked NOT-FIXED-THIS-ROUND instead — record it fully, the orchestrator spins it into its own mill-wiki task afterward.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None open for re-evaluation. F16/F22 are permanently out of loom's scope (see "Explicitly OUT of scope" above), not an open question this round revisits.

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
1. A structured review report — Executive summary with top risks + merge-readiness opinion, and an explicit statement of how far the real `lyx loom run` got and whether it completed; Scope assessment; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including what you could NOT verify and why. Write it to `_mill/loom-review-<yourtag>.md` and commit it incrementally as described above.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Write it to `_mill/loom-review-<yourtag>-fixer-report.md` and commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict + an explicit statement of how far the real hub-mode run got). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
