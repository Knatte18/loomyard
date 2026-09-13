# `loom` (loom-step + self-report Tier 1 + Tier 2) — independent review + fix (round 1)

> Filled instance of `crucible/review-prompt-template.md` for the campaign in `_mill/loom-crucible-orchestrator-kickoff.md`. Read that kickoff file too — it is the campaign's charter and carries context this prompt does not restate. This file is rewritten each round by the orchestrator; every version that ever seeded a round stays in git history.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of three pieces that landed together but have never been driven live through this method: `lyx loom step` (`internal/loomcli/step.go`), self-report Tier 1 (`internal/loomengine/anomaly.go` + `internal/loomcli/selfreport.go`), and self-report Tier 2 (`internal/friction`, `internal/frictionengine`) — followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/crucible-loom-step-selfreport-hardening` (branch `crucible-loom-step-selfreport-hardening`).
Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate — real tmux, real `claude` provider sessions via shuttle, a real dummy task driven through loom's real phase machine. This is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/drive check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `loom: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/loom-review-r1.md` and `_mill/loom-review-r1-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-r1.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below, APPEND your observations to `_mill/loom-review-r1.md`'s "What was tested" section immediately after each command/scenario returns. Jot each finding into the file's findings section provisionally as you spot it.
**COMMIT each append** — a small, frequent commit like `loom: review notes — <what you just appended>` after each meaningful append.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have your own list — nothing under `_mill/` matching `loom-review-*` (this is round 1, so nothing should exist yet besides this prompt and the handoff note; if you find anything else matching that pattern, do not read it, and mention the anomaly in your report).
Reading the design SPEC and the module docs is expected and required (those are not reviews).
Since this is round 1 there is no prior round's material to consult after your own pass.

## What to read
- Code: `internal/loomcli/step.go` + `step_test.go`, `internal/loomcli/selfreport.go` + `selfreport_test.go` + `selfreport_github_test.go`, `internal/loomcli/drive.go` (the `reflectFriction`/`detectAndFileAnomalies` call sites and their ordering relative to each other and to `Finalize`), `internal/loomshed/interruptpolicy.go` + `interruptpolicy_test.go`, `internal/loomengine/anomaly.go` + `anomalybody.go` (+ tests), `internal/loomengine/config.go` (the `selfreport`/`friction`/`friction_timeout_min` keys) and `template.yaml`, `internal/friction/**` (leaf: `Directive`, `WarnIfMarkerAbsent`, `NotePath`, `EnsureDir`), `internal/frictionengine/**` (`Reflect`, `Deps`, `spec.go`), `internal/selfreportengine/selfreport.go` (`CreateIssue`, `targetRepo`, `NewGitHubClient` seam), `internal/selfreportcli/**`, `internal/shedadapters/**` (Bouncer/Burler probe-before-spawn behavior, since step/interrupt-policy correctness rides on it), the seven prompt composers that inject `{{.friction_directive}}` (grep `friction.MarkerName`/`friction.Directive` across `internal/loomengine`, `internal/websterengine`, `internal/burlerengine` to find all seven and confirm the marker is actually present in each composer's stencil).
- Skill: `plugins/ly/skills/ly-supervise/SKILL.md` — the external-supervisor contract that consumes `step`'s envelope and `next_interrupt_policy`. Its instructions must match what the code actually does; a supervisor skill that assumes step-level enforcement of the handback policy, when the code provides none, is a documentation defect worth a finding.
- Docs: `manifest/designs/loom-step.md`, `manifest/designs/self-report-tier1.md`, `manifest/designs/self-report-tier2.md`, `manifest/designs/loom.md` (the full phase machine, crash-recovery/attach section, the review-segment/Webster black-box framing), `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md` (the Friction Leaf Invariant at minimum, plus Hub Geometry / CLI-Cobra / gitkit Leaf / hubforge Fabric-Fixture / Sandbox Suite Coverage / Documentation Lifecycle), `README.md`.
- `tools/sandbox/SANDBOX-CORE-SUITE.md`'s **S8** ("Loom status and pause over a seeded fixture") — for scenario ideas only. It is fixture-only (hand-written `status.json`, never reaches a seeded state through a shipped verb) and does not touch `step`, self-report, or friction at all — that gap is exactly what this campaign exists to close. You run every scenario yourself, directly, with your own tool calls; you do NOT invoke any `sandbox-*-suite.cmd` launcher.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
  A change that ships behaviour without updating the module doc / invariants in the SAME change is incomplete.
- Design intent (SPEC, not a review): `manifest/designs/loom-step.md` and `manifest/designs/self-report-tier1.md`/`self-report-tier2.md` — all three say "Status: Shipped/Done" and describe as-built behavior; use them as the authoritative source of intended v1 scope/behavior, cross-checked against the code.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — does `step`'s ten-key envelope, Tier 1's five anomaly kinds, and Tier 2's friction/reflection machinery actually deliver what the three design docs promise? Gaps, over-reach, silently-dropped requirements.
2. Correctness — bugs, races, error handling, edge cases, concentrated on the composed live behavior below. Also assess docs accuracy and operability (does the `ly-supervise` skill's advice match what `step` actually does?).

## High-yield focus — where this trio's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (envelope field mapping, the five-value kind vocabulary, the anomaly-detector table tests) are already solid — `step_test.go` and `anomaly_test.go` exhaustively cover the pure logic. **Nothing in the existing test suite drives `lyx loom step` repeatedly through a real multi-producer sequence, and nothing drives an interrupted-and-resumed run against it.** That composed, live gap is where this campaign's bugs live:

- **Envelope fidelity across a real multi-step run.** Drive a dummy task through `lyx loom step` called repeatedly (discussion → plan → webster → review, not just one call), and at *every* step confirm `next`/`continue`/`state`/`next_interrupt_policy` are internally consistent with the real persisted status file — not just correct at Finalize. Repro: script a loop calling `lyx loom step` and diff each envelope against `_lyx/loom/status.json`'s own state after that call.
- **Interrupted-and-resumed reinvoke rows.** Kill the process running `lyx loom step` mid-producer on at least one `reinvoke`-policy row (e.g. mid `Discussion-Write` or mid a `Burler` round), then re-invoke `step`. Confirm the row's adapter attaches to the still-live agent (per `internal/shedadapters`' "probe for a live agent first" contract in `manifest/designs/loom.md`'s crash-recovery section) rather than double-spawning — check for two agents, two commits, or a duplicated round artifact.
- **The one `handback`-only row, `NameWebster`, and what "handback" actually means operationally.** `internal/loomshed/interruptpolicy.go`'s own doc comment says re-invoking an interrupted Webster step "kills the in-flight Master and restarts the batch run from state.json" — i.e. `step`/`run` themselves do NOT refuse or block a reinvocation; the handback policy is advisory metadata for an external supervisor to *choose* not to reinvoke, not an enforced guardrail in the code. Verify this understanding is actually correct by killing mid-`Webster` and reinvoking `lyx loom step` yourself: does it silently restart the batch run exactly as documented, with no code-level protection? If `next_interrupt_policy` claims `"handback"` but the machine reinvokes anyway with no warning or refusal, confirm the skill (`ly-supervise`) is the only thing standing between an automated caller and that expensive restart, and that its instructions say so clearly enough. A supervisor skill or doc that implies the machine itself enforces the handback is a real finding.
- **A genuine Tier-1 anomaly trigger, live.** Force at least one of the five kinds for real (a bounce-budget-exhausted halt is probably cheapest to engineer deliberately — bounce a review segment past its `max_bounces` with a rigged always-reject reviewer, or force a crash-resume by killing a driver mid-run and observing the next `drive`/`step`'s entry). Confirm `DetectAnomalies` fires, the marker records the title, and — per the kickoff's explicit scope addition — for **exactly one** of these across the whole campaign, let it file a **real** GitHub issue against `Knatte18/loomyard` via the unmodified `selfreport: true` path (not the `selfreportengine.NewGitHubClient` test seam — that seam is for rehearsal/regression tests only, never for the one deliberate live-fire). Record the resulting issue number/URL in your report.
- **A genuine Tier-2 friction note + reflection cycle, live.** Get at least one real producer (Discussion-Write is the cheapest target) to write a friction note under `.lyx/loom/friction/` during a live run, then drive to a natural end point (Finalize or a `stuck` escalation) and confirm exactly one reflection agent spawns over the aggregated dossier, writes `reflection-report.md`, and the directory gets archived on a clean return. If this run is also the one chosen for the Tier-1 live-fire, watch for whether Tier 1 and Tier 2 can both trigger off the *same* halt and, if so, whether they interact safely (no double-filing, no race over the same status file).
- **`step`'s early busy-refusal against a genuinely live driver.** Start a real `lyx loom run` (which spawns a detached driver), then call `lyx loom step` against the same task while the driver is mid-producer. Confirm `step` refuses with `kind: busy` rather than racing the driver for the run lock.
- **Cost-control config doesn't mask a real defect.** The dummy task's `loom.yaml` should override `discussion`/`plan`/`review`/`friction` away from the shipped `opus[effort=high]` default to a cheap model/effort (see the Live-substrate cost declaration below) to control real token spend across a full live phase-machine run — confirm nothing you observe is an artifact of that override (e.g., a cheap model producing a discussion/plan too thin for `Discussion-Validate`/`Plan-Validate` to accept is a fixture problem, not a loom defect; don't file it as one, but do note if the validators themselves seem too lenient).

## Explicitly OUT of scope for this round
- `Plan-Sweep` (row 6 of `loom.md`'s producer table) — never built, its own Someday roadmap item, not a recipe row at all. Its absence is correct, not a finding.
- Windows-specific behavior — unreachable from this Linux host, out of scope for live driving (reasoning about it from code is fine, but do not claim to have verified it).
- The `hardener` module (DRAFT, unbuilt) — not in scope.
- The *content quality* of the dummy task's own discussion/plan/implementation (its cheap model may produce a mediocre plan) — the phase-machine mechanics around it are what this round is grading, not whether the dummy task itself is well-designed software.
- Anything already covered and unrelated to this trio (e.g. general `websterengine`/`burlerengine`/`shedengine` correctness outside how loom-step/self-report/friction touch them) — flag only if you find something that concretely breaks one of this round's three subjects.

## Round context seeded from prior-round verification
**This is round 1 — there is no residual to close.** The seed is the campaign mission itself, from `_mill/loom-crucible-orchestrator-kickoff.md`:

1. Drive a **dummy task** through loom's real phase machine via repeated `lyx loom step` calls (discussion → plan → implement → review), confirming the envelope's `next`/`continue`/`state` fields track real Shed state at every step, not just at Finalize.
2. Drive at least one **interrupted-and-resumed** run: kill the driving process mid-producer, restart, confirm `reinvoke` rows re-attach rather than double-spawn, and confirm the `handback`-only row (`NameWebster`) behaves as documented above.
3. Trigger a real condition that trips one of the 5 Tier-1 anomaly kinds, **and** at least one scenario producing a Tier-2 friction note worth aggregating.
4. **Live-fire self-report exactly once, deliberately.** Leave `selfreport: true` and `friction: <a real model spec>` as-is in the dummy task's `loom.yaml` (do NOT set them to `false`/`""` to suppress filing) so that at least one genuinely-triggered anomaly or friction note files a **real** GitHub issue in `Knatte18/loomyard`, verifying the full detection-to-filing path end to end. Record the resulting issue number/URL in your review report and fixer report both. **Do not close it yourself** — campaign wrap-up (the orchestrator, once this round's live-fire is verified) closes it, clearly labeled as a deliberate crucible test-fire.

No CLOSED-AND-VERIFIED items exist yet — nothing to avoid re-litigating.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow (a serial, non-interrupted `step`/`run` sequence, plus the one interrupted-and-resumed repro above) is the gate. There is no N×-concurrent-suite diagnostic step for this campaign — an LLM-driving module's concurrency risk is cost, not a stress amplifier worth running here (see the cost declaration below); concurrency claims are out of scope for this round unless you find one incidentally.

## Live-substrate cost declaration (BLOCKING — read before running anything live)

**Existing `//go:build smoke` tests in this trio's packages: LLM-DRIVING: NO.**
`internal/loomcli/smoke_test.go` and `internal/loomcli/smoke_attachprobe_test.go` are the only `//go:build smoke` files under `internal/loomcli`, `internal/loomengine`, `internal/friction`, `internal/frictionengine`, or `internal/selfreportengine` (confirmed via `grep -rl "go:build smoke" internal/ cmd/`). Every test in both files deliberately spawns **zero** real LLM subprocesses:
- `smoke_test.go` wires `providerlessShuttleConfig()` — the shuttle provider binary path is set to a nonexistent path and the startup timeout cut to 2s, so every launch fails fast before any real process starts (see that function's own doc comment for the history of why — a crucible round once measured a real `claude` subprocess alive for 30s in this exact file before the fix).
- `smoke_attachprobe_test.go` wires a `shellLaunchEngine` stub that launches a plain shell script instead of any provider.

So the existing bare `-tags smoke -run Smoke` command below is safe to run exactly as written for this trio — no per-test exact-name restriction and no EXECUTION BAN list needed for the *existing* smoke suite.

**The campaign's OWN live-driving mission is a different, expensive animal — read this before touching it.**
`loom.yaml`'s shipped template (`internal/loomengine/template.yaml`) defaults **all four** LLM-role keys — `discussion`, `plan`, `review`, `friction` — to `opus[effort=high]`. A full, undoctored live run through discussion → plan → Webster → Webster-Review, with a friction reflection pass at the end, spawns a **real Discussion-Write session, a real Plan-Write session, a real Burler round per review segment entered (discussion + plan + webster, at least one round each, more on any bounce), at least one real Webster per-batch implementer fork plus its integration-suite fork, and a real friction-reflection agent** — on the order of six-plus separate real, expensive Opus-high sessions for one clean pass, more if any segment bounces.

Before driving live:
- **Override the dummy task's `loom.yaml`** — set `discussion`, `plan`, `review`, and `friction` to a cheap model/effort (not the `opus[effort=high]` default) for the whole live-driving mission. The mechanics under test (envelope fidelity, reinvoke/handback, anomaly/friction detection and filing) do not depend on the *quality* of the dummy task's own discussion/plan/code — only on a real provider process genuinely running and genuinely finishing or being genuinely interruptible. Keep `discussion_interactive: false` (the shipped default) so no human is required.
- **Keep the dummy task's own plan to exactly one trivial card** (a one-file, easily-reviewable change) so `Webster` spawns exactly one batch — one implementer fork, one integration-suite fork — rather than fanning out across several cards. A dummy task with more than one card multiplies real-session cost for no additional coverage this round's mission needs.
- **Run every live-substrate invocation one at a time, in the foreground, waited to completion.** Never background a `lyx loom step`/`run`/`drive` call, never run two dummy-task drives concurrently, and never invoke a fan/cluster-shaped review (a `Burler` round configured for more than one lens) for this campaign — nothing in this trio's mission needs one, and it would multiply real provider-process count for no coverage this round needs.
- **The interrupted-and-resume scenario needs exactly one real agent alive at the moment you kill it** — do not let a kill-and-resume repro race against a second, unrelated live agent from an earlier scenario you forgot to tear down. Confirm the target task's own state before killing anything.
- The generic "N× CONCURRENT full smoke suites" gate that appears in `crucible/README.md`/`orchestrator-prompt.md`'s general verification protocol does **not** apply to this campaign at all — do not run it.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/loomcli/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/friction/... ./internal/frictionengine/... ./internal/selfreportengine/... ./internal/selfreportcli/...`
- `go test -count=5 ./internal/loomcli/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/friction/... ./internal/frictionengine/... ./internal/selfreportengine/... ./internal/selfreportcli/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag — safe as written, see the cost declaration above):
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1`
- tmux resolved via PATH (Linux host); no override env needed unless PATH resolution fails.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary under test: `deploy-dev` (POSIX). **FOOTGUN:** re-run `deploy-dev` after EVERY source change or you validate a stale binary. Deploy first, always.
- Set up a dummy task in a real wired hub (`hubforge.NewHub`-shaped, or a real `lyx fabric add` pair — your choice of fixture mechanism, as long as it's the real CLI/real substrate, not an in-process `RunCLI` call, since `lyx loom run`'s driver spawn resolves `os.Executable()` and needs a genuine binary).
- Override its `loom.yaml` per the cost declaration above before driving anything live.
- Walk the "High-yield focus" list above with your own tool calls: repeated `lyx loom step`, the interrupted-and-resumed repro, the Tier-1 trigger, the Tier-2 friction note + reflection cycle, the one deliberate live-fire, the busy-refusal check.
- The list is a FLOOR — devise more adversarial scenarios of your own beyond it if the code makes you suspicious of something (e.g. what happens if a friction note is written but the run never reaches Finalize or a stuck state — does it just sit unflushed forever? Is that documented as intentional?).
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real discussion/plan/implement/review cycle takes real wall-clock minutes even on a cheap model. That cost is EXPECTED and BUDGETED FOR. You are explicitly forbidden from writing "operator-assisted", "cost-bearing", "long-running", "impractical", or "automated context" as a reason to skip live driving.
- The only legitimate "cannot verify" cases: (a) a scenario that structurally requires a human's physical eyes, or (b) a genuine environment gap (no GitHub token resolvable for the live-fire, no tmux, no `claude` binary — check for these FIRST). Flag those specifically rather than skipping silently.

TEARDOWN DISCIPLINE (critical): tear down every substrate server/session you start. At the end, confirm ZERO stray tmux servers (`tasklist`/`pgrep tmux` equivalent — on Linux, `pgrep -af tmux` or `ps aux | grep tmux`) and zero stray detached loom drivers (`pgrep -af "loom drive"` or similar). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2 — including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires an operator decision on a real design tradeoff, or a capability you don't have — say so explicitly in the fixer report's deferred section.
**A LARGE finding (a genuine subsystem/feature addition, a cross-cutting refactor reaching outside this trio) is a SIZE exception, not a severity one** — record it fully, mark it explicitly NOT-FIXED-THIS-ROUND with the reason, and the orchestrator will open a proper mill-wiki task for it. When genuinely unsure whether something is "large," say so in the report rather than guessing.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None — this is round 1.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND the Go-specific skills (`golang:golang-build`, `golang:golang-testing`, `golang:golang-comments`) before editing. Prefer surgical edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test walking the failing scenario against the real substrate — following `smoke_test.go`'s/`smoke_attachprobe_test.go`'s existing providerless/stub patterns so the new test does NOT become a real-LLM-spawning smoke test by accident (unless the defect is genuinely only reproducible with a real provider in the loop, in which case say so explicitly and keep it to exactly one real subprocess, never a fan/cluster shape).
- MAKE SMOKE TESTS DETERMINISTIC — poll on real state transitions with a deadline, never sleep a fixed amount. Prove determinism by running the new test several times.
- `tools/sandbox/SANDBOX-CORE-SUITE.md`'s S8 does not cover `step`/self-report/friction — if your review surfaces a live/visual behavior worth adding there, extend it (matching the existing scenario shape, keeping `sandbox_coverage_test.go` green); if not, just note the gap in your fixer report.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy-dev`) and re-run every live scenario yourself, directly.
- Update `manifest/designs/loom-step.md` / `manifest/designs/self-report-tier1.md` / `manifest/designs/self-report-tier2.md` / `manifest/designs/loom.md` (and `docs/overview.md` / `CONSTRAINTS.md` if invariants or the module table move) IN THE SAME change as the fix they document. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it — do NOT push unless the user explicitly asks.
- Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report at `_mill/loom-review-r1.md` (Executive summary with top risks + merge-readiness opinion; Scope assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including the live-fire issue number/URL and what you could NOT verify and why). Build it incrementally per "Log as you go" above; commit it.
2. A fixer report at `_mill/loom-review-r1-fixer-report.md`: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + the live-fire issue number/URL + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
