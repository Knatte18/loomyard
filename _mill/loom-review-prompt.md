# `loom` (loom-step + self-report Tier 1 + Tier 2) — independent review + fix (round 3 — SAFETY PASS)

> Filled instance of `crucible/review-prompt-template.md` for the campaign in `_mill/loom-crucible-orchestrator-kickoff.md`. Read that kickoff file too — it is the campaign's charter and carries context this prompt does not restate. This file is rewritten each round by the orchestrator; every version that ever seeded a round stays in git history.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of three pieces that landed together — `lyx loom step` (`internal/loomcli/step.go`), self-report Tier 1 (`internal/loomengine/anomaly.go` + `internal/loomcli/selfreport.go`), and self-report Tier 2 (`internal/friction`, `internal/frictionengine`) — followed by FIXING what you find.

**This round is explicitly a SAFETY PASS.** Round 1 found and fixed 1 BLOCKING + 4 MEDIUM + 2 LOW + 1 NIT. Round 2 found and fixed 1 MEDIUM + 2 LOW + 1 NIT, and closed four of round 1's five residual items with real live repros. Severity is shrinking round over round, but per this method's own track record (see `crucible/README.md`'s reed and fabric worked examples), the round that self-reports "ready" right before the genuinely clean one is *always* wrong — so this round's job is to find what two independent prior rounds, across two different models, both missed, or to honestly confirm there is nothing left to find. A safety pass that finds nothing is a valuable, expected outcome — do not invent findings to justify the round.
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
Also commit `_mill/loom-review-r3.md` and `_mill/loom-review-r3-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-r3.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below, APPEND your observations to `_mill/loom-review-r3.md`'s "What was tested" section immediately after each command/scenario returns. Jot each finding into the file's findings section provisionally as you spot it.
**COMMIT each append** — a small, frequent commit like `loom: review notes — <what you just appended>` after each meaningful append.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue file before you have your own list — nothing under `_mill/` matching `loom-review-*`: this covers `loom-review-r1.md`/`-r1-fixer-report.md`, `loom-review-r2.md`/`-r2-fixer-report.md`, and `loom-review-HANDOFF.md` (the orchestrator's own private state — off-limits by the same filename-pattern rule, not a content judgment). Reading the design SPEC and the module docs is expected and required (those are not reviews).

AFTER you have written your own independent findings, you MAY consult rounds 1 and 2's material to (a) confirm their fixes have not regressed and (b) re-evaluate the deferred/residual items listed below. Do NOT re-litigate anything in the CLOSED-AND-VERIFIED list below — the orchestrator independently sabotage-proved every fix in it from a cold checkout; re-finding them is wasted round time.

## What to read
- Code: `internal/loomcli/step.go` + `step_test.go` (+ `stephandoff_test.go`), `internal/loomcli/selfreport.go` + `selfreport_test.go` + `selfreport_github_test.go`, `internal/loomcli/drive.go`, `internal/loomcli/bootstrap.go` + `run.go` + `sharedbootstrap.go` (rounds 1/2's F-0/F-1/F-3/R2-F1/R2-F3 fixes — `awaitRunLockHalted`, `ensureFrictionDirAfterSeed`, `ensureStatusLockDir`, the step-handoff marker, the halted-disposition log line), `internal/loomshed/interruptpolicy.go` + `interruptpolicy_test.go`, `internal/loomengine/anomaly.go` + `anomalybody.go` (+ tests, incl. `CleanStepHandoff`), `internal/loomengine/config.go` (`selfreport`/`friction`/`friction_timeout_min`/`LoomFrictionLock`/`LoomStepHandoff`) and `template.yaml`, `internal/friction/**`, `internal/frictionengine/**`, `internal/selfreportengine/selfreport.go`, `internal/selfreportcli/**`, `internal/shedadapters/bouncer.go` (F-4's re-bounce probe, now confirmed on both `Discussion-Bouncer` and `Plan-Bouncer`) and `internal/shedadapters/doc.go`, `internal/websterengine/strand.go`, the seven prompt composers that inject `{{.friction_directive}}`.
- Skill: `plugins/ly/skills/ly-supervise/SKILL.md` — updated by both prior rounds. Confirm its current text matches what the code actually does.
- Docs: `manifest/designs/loom-step.md`, `manifest/designs/self-report-tier1.md`, `manifest/designs/self-report-tier2.md`, `manifest/designs/loom.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- `tools/sandbox/SANDBOX-CORE-SUITE.md`'s **S8** — extended by round 1. Extend it further if you surface a live/visual behavior it still doesn't cover.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
- Design intent (SPEC, not a review): the three design docs above — all say "Status: Shipped/Done"; use them as the authoritative source of intended v1 scope/behavior, cross-checked against the code as it stands after both prior rounds' fixes.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — does `step`'s ten-key envelope, Tier 1's five anomaly kinds, and Tier 2's friction/reflection machinery actually deliver what the three design docs promise, as they stand now?
2. Correctness — bugs, races, error handling, edge cases. As a safety pass, weight your effort toward regions NEITHER prior round drove deeply (see "High-yield focus"), not toward re-walking the same envelope-fidelity ground both rounds already covered thoroughly.

## High-yield focus — where a safety pass should look
Both prior rounds walked envelope fidelity across full multi-step runs repeatedly and thoroughly — that ground is well-covered; don't just repeat it. A safety pass earns its keep by reaching what neither prior round's *method* could reach:

- **The `RunDone`-triggered friction path, on a REAL end-to-end run (not a hand-built done-state fixture).** Round 2 verified `RunDone` fires a reflection, but only on a hand-written done-state status file, never on a task that actually walked all the way to `Finalize` through the real phase machine. If you can afford a full walk to a genuine `done` state (your own cost call, per the declaration below), do it and watch the transition happen naturally rather than staging the terminal state.
- **`Publish`'s merged-PR resume and `Finalize`'s live merge-back.** Neither round drove this — round 1 stopped short, round 2 was blocked by a real external PR your predecessor could not merge (still open at `Knatte18/lyx-test#2` as of this writing — check whether the operator has since resolved it before assuming it's still blocking; if it's merged or closed, you may have a fresh path to drive this for real). `internal/landingshed` carries standing hermetic coverage for both branches — read it and confirm the composed reality still matches what those hermetic tests assume, rather than only trusting the unit tests in isolation.
- **A second, INDEPENDENT interrupted-and-resumed repro on a row neither prior round killed mid-agent.** Rounds 1/2 covered `Discussion-Bouncer` seed, `Discussion-Burler` round, `Plan-Bouncer` seed, and `Webster`'s handback row. Try a `*-Burler` round on a segment other than Discussion (e.g. `Plan-Burler` or `Webster-Burler`), or a judge-pass kill (`n > 0 && judged(n)` in `bouncer.go`) rather than another seed-pass kill — the judge-pass probe is a DIFFERENT code path (`awaitLiveJudge`, not `awaitLiveSeed`) that neither round has independently pressure-tested with a real kill.
- **F-6's live race, only if you find a genuinely cheap way in.** Still PLAUSIBLE-only after two rounds. Do not force it — the campaign's cost declaration still forbids two concurrent dummy-task drives. If a cheap angle presents itself (e.g. a way to make the reflection agent's own runtime deterministically long via a stub rather than a real timeout, so a second driver's overlap window is trivial to hit), take it; otherwise state plainly that it remains an accepted, twice-confirmed-unreproduced residual.
- **A spontaneous Tier-2 friction note, one more honest attempt.** Two rounds have now tried and failed to observe a producer choosing to write one unprompted. A third failure is itself informative (three independent models agreeing the bar for "worth a note" is higher than ordinary task friction) — report it as such rather than treating it as an open item forever.
- **Re-audit the whole trio's doc/skill claims against the code as it now stands**, since two rounds of fixes have touched `manifest/designs/loom-step.md`, `self-report-tier1.md`, `self-report-tier2.md`, `loom.md`, and `SKILL.md` — confirm nothing drifted out of sync across two rounds' worth of edits.
- **Anything your own reading makes you suspicious of.** This list is a floor, not a ceiling — a genuinely independent pass should surface things this list cannot anticipate.

## Explicitly OUT of scope for this round
- `Plan-Sweep` — never built, its own Someday roadmap item. Not a finding.
- Windows-specific behavior — unreachable from this Linux host.
- The `hardener` module (DRAFT, unbuilt).
- The *content quality* of any dummy task's own discussion/plan/implementation.
- **Re-triggering the live-fire self-report path.** The campaign's one deliberate live-fire is already captured (issues #240 and #241). Do NOT set up a scenario that could file a third GitHub issue.
- **The `deploy-specs-like-stencils` mill-wiki task's subject matter.** A stencil citing a cross-repo-unreachable spec/design-doc path is now a tracked, separate backlog item — do not fix it inline this round even if you notice it again; it is explicitly out of scope for a crucible round (too large, per hard rule 5).

## Round context seeded from prior-round verification

**This IS the safety pass.** No known residual is being carried forward as "must fix" — everything both prior rounds found has been independently verified fixed by the orchestrator. Your job is to find what both prior rounds' methods, across three different models by the end of this round (Opus, Fable, now Sonnet), still could not reach — or to honestly confirm merge-readiness.

**CLOSED-AND-VERIFIED — do NOT re-litigate these (orchestrator-sabotage-proved from a cold checkout unless noted):**
- F-0 (BLOCKING, r1) — `run`'s handshake no longer misreads a Tier-2 reflection as a wedged spawn. `713ab509a`.
- F-1 (MEDIUM, r1) — once-per-task friction-directory clear wired into `seedAndCommitBootstrap`. `1d671f44c`.
- F-2 (MEDIUM, r1) — `step`'s Tier-1/Tier-2 exemption documented in three places (deliberate documentation-only fix). `9600fc799`.
- F-3 (MEDIUM, r1) — `status`/`pause` create the status lock's parent directory before reading. `78a407698`.
- F-4 (MEDIUM, r1+r2) — the Bouncer's re-bounce branch probes for a live seed. Verified on BOTH `Discussion-Bouncer` (r1) and `Plan-Bouncer` (r2, independently reproduced). `eb6af7720`.
- F-5 (LOW, r1) — Webster Master reclaim on a handback reinvoke logs at `Warn`. `6a0750a7e`.
- F-6 (LOW, r1) — friction reflection takes a non-blocking lock; unit-sabotage-proved twice over (r1, then re-confirmed present in r2). The live two-driver race stays unreproduced after two rounds — see High-yield focus.
- F-7 (NIT), D-1, D-2 (r1) — comment/doc fixes, diff-reviewed. `6a0750a7e` / `ed9fce0d0`.
- R2-F1 (MEDIUM, r2) — the clean-handoff marker prevents a spurious crash-resume filing on an ordinary step-to-driver handoff. Orchestrator-sabotage-proved BOTH halves (the detector exclusion and the step-side wiring) independently. `aaddede3e`.
- R2-F2 (LOW, r2) — supervisor skill names the friction-note directory; `loom-step.md` records the completes-under-step drop case. `c27f70bf5`.
- R2-F3 (LOW, r2) — the handshake's halted disposition logs a breadcrumb. `f90fe0d5c`.
- R2-F4 (NIT, r2) — `reflectFriction`'s mkdir-failure warning names the right directory. `760be56bd`.

**Genuinely open, not yet closed by anyone — the safety pass's real targets:**
1. `Publish`'s merged-PR resume and `Finalize`'s live merge-back — never driven by any round. Hermetically covered; never composed-and-observed live.
2. F-6's live two-driver race — PLAUSIBLE only, twice.
3. A spontaneous (non-hand-placed) Tier-2 friction note — attempted and not observed, twice.
4. A `RunDone` reflection off a REAL walked-to-completion run (not a hand-built fixture).
5. An interrupted-and-resumed repro on a judge-pass kill or a non-Discussion `*-Burler` round — no round has independently driven these specific code paths.

State the **merge bar**: correctness in the NORMAL single-instance flow (a serial, non-interrupted `step`/`run` sequence, plus interrupted-and-resumed repros) is the gate. There is no N×-concurrent-suite diagnostic step for this campaign.

## Live-substrate cost declaration (BLOCKING — read before running anything live)

**Existing `//go:build smoke` tests: LLM-DRIVING: NO** — `internal/loomcli/smoke_test.go`, `smoke_attachprobe_test.go`, and `smoke_bootstrapwiring_test.go` all spawn zero real LLM subprocesses (providerless/stub engines throughout). The bare `-tags smoke -run Smoke` command is safe to run exactly as written.

**Build your OWN disposable fixture hub — do NOT reuse the operator's `~/Code/lyx-test-HUB`.** Round 2 drove live against that existing, operator-owned sandbox (its own live `lyx-test` tmux session had to be carefully avoided) and left real side effects behind that are still awaiting the operator's own decision: an open PR (`Knatte18/lyx-test#2`), a swapped `~/go/bin/lyx` binary, and two leftover dummy task pairs. Round 1's approach — build a throwaway hub from scratch via `hubforge.NewHub`-style bare remotes (see `internal/loomcli/smoke_test.go`'s `newWiredPairFixture` for the pattern, or round 1's own review report for the manual bare-remote recipe) — is the one to follow this round, so this safety pass does not add a THIRD round of real-world leftovers on top of two still pending resolution. If driving `Publish` for real requires a genuine external git remote to open a PR against, use a fresh disposable repo you create for this purpose, never the operator's existing `lyx-test` sandbox, and tear it down (or clearly flag it for deletion) at the end.

`loom.yaml`'s shipped template defaults `discussion`/`plan`/`review`/`friction` to `opus[effort=high]` — override all four to a cheap model/effort for your dummy task(s) (both prior rounds used `sonnet[effort=low]` for discussion/plan/review and `haiku` for friction; reuse or adjust). Keep `discussion_interactive: false`. Run every live-substrate invocation one at a time, foreground, waited to completion — never two dummy-task drives concurrently.

**CRITICAL — do not re-trigger the live-fire.** Issues [#240](https://github.com/Knatte18/loomyard/issues/240) and [#241](https://github.com/Knatte18/loomyard/issues/241) are the campaign's captured proof. **Set `selfreport: false` and `friction: ""` in every dummy task's `loom.yaml` this round.** If pursuing the spontaneous-friction-note item tempts you to re-enable `friction`, you may, but leave `selfreport: false` throughout regardless, and if a reflection would file a real issue on its own judgment, catch it before that call and report the near-miss instead of letting a third issue land.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/loomcli/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/friction/... ./internal/frictionengine/... ./internal/selfreportengine/... ./internal/selfreportcli/... ./internal/shedadapters/... ./internal/websterengine/...`
- `go test -count=5 ./internal/loomcli/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/friction/... ./internal/frictionengine/... ./internal/selfreportengine/... ./internal/selfreportcli/... ./internal/shedadapters/... ./internal/websterengine/... ./cmd/lyx/...`
- `go test ./...` (whole repo, confirm no regressions)

Live smoke (real substrate, behind the `smoke` build tag — safe as written, see the cost declaration above):
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1`
- tmux resolved via PATH (Linux host).

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary under test: `deploy-dev` (POSIX). **FOOTGUN:** re-run after EVERY source change.
- Build your OWN disposable fixture hub (see the cost declaration above) — never the operator's `lyx-test-HUB`.
- Override `loom.yaml` per the cost declaration above — **`selfreport: false`, `friction: ""`** (or `friction` on with `selfreport` still off, if pursuing the spontaneous-note item).
- Walk the "High-yield focus" list above with your own tool calls.
- The list is a FLOOR — devise more adversarial scenarios of your own if the code makes you suspicious of something.
- **"Headless" means "no human required" — NOT "no time/token cost to me."** You are explicitly forbidden from writing "operator-assisted", "cost-bearing", "long-running", "impractical", or "automated context" as a reason to skip live driving.
- The only legitimate "cannot verify" cases: (a) a scenario that structurally requires a human's physical eyes, or (b) a genuine environment gap (check FIRST). Flag those specifically rather than skipping silently.

TEARDOWN DISCIPLINE (critical): tear down every substrate server/session you start, INCLUDING your own disposable fixture hub and any external repo you created for a Publish repro. At the end, confirm ZERO stray tmux servers (`pgrep -af tmux` / `ps aux | grep tmux`) and zero stray detached loom drivers (`pgrep -af "loom drive"`). Leave no stray state — and leave the operator's existing `lyx-test-HUB` and `~/go/bin/lyx` exactly as you found them, since neither is yours to touch this round.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2 — including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires an operator decision on a real design tradeoff, or a capability you don't have — say so explicitly in the fixer report's deferred section.
**A LARGE finding is a SIZE exception, not a severity one** — record it fully, mark it NOT-FIXED-THIS-ROUND with the reason, and the orchestrator will open a proper mill-wiki task for it (as already happened once this campaign — see "Explicitly OUT of scope" above).

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
See "Genuinely open, not yet closed by anyone" under "Round context seeded from prior-round verification" above — the five numbered items.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND the Go-specific skills (`golang:golang-build`, `golang:golang-testing`, `golang:golang-comments`) before editing. Prefer surgical edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test walking the failing scenario against the real substrate, following the existing providerless/stub patterns.
- MAKE SMOKE TESTS DETERMINISTIC — poll on real state transitions with a deadline, never sleep a fixed amount. Prove determinism by running the new test several times.
- Extend `tools/sandbox/SANDBOX-CORE-SUITE.md`'s S8 if your review surfaces a live/visual behavior it doesn't cover; keep `sandbox_coverage_test.go` green.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy-dev`) and re-run every live scenario yourself, directly.
- Update the module docs / `docs/overview.md` / `CONSTRAINTS.md` (if invariants or the module table move) IN THE SAME change as the fix they document. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it — do NOT push unless the user explicitly asks.
- Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report at `_mill/loom-review-r3.md` (Executive summary with top risks + merge-readiness opinion; Scope assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including what you could NOT verify and why). Build it incrementally per "Log as you go" above; commit it.
2. A fixer report at `_mill/loom-review-r3-fixer-report.md`: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict, and explicit confirmation that no third GitHub issue was filed, and that your fixture hub was your own, not the operator's `lyx-test-HUB`). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
