# `loom` (loom-step + self-report Tier 1 + Tier 2) — independent review + fix (round 2)

> Filled instance of `crucible/review-prompt-template.md` for the campaign in `_mill/loom-crucible-orchestrator-kickoff.md`. Read that kickoff file too — it is the campaign's charter and carries context this prompt does not restate. This file is rewritten each round by the orchestrator; every version that ever seeded a round stays in git history.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of three pieces that landed together — `lyx loom step` (`internal/loomcli/step.go`), self-report Tier 1 (`internal/loomengine/anomaly.go` + `internal/loomcli/selfreport.go`), and self-report Tier 2 (`internal/friction`, `internal/frictionengine`) — followed by FIXING what you find. Round 1 already drove this trio live once, found and fixed 1 BLOCKING + 4 MEDIUM + 2 LOW + 1 NIT defects, and captured the campaign's one deliberate live-fire self-report issue. This round is a genuinely independent second pass, not a rubber stamp — form your own findings before consulting round 1's material (see "Clean-room review constraint" below).
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
Also commit `_mill/loom-review-r2.md` and `_mill/loom-review-r2-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-r2.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below, APPEND your observations to `_mill/loom-review-r2.md`'s "What was tested" section immediately after each command/scenario returns. Jot each finding into the file's findings section provisionally as you spot it.
**COMMIT each append** — a small, frequent commit like `loom: review notes — <what you just appended>` after each meaningful append.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue file before you have your own list — nothing under `_mill/` matching `loom-review-*`: this covers `loom-review-r1.md`, `loom-review-r1-fixer-report.md`, and `loom-review-HANDOFF.md` (the orchestrator's own private state — off-limits by the same filename-pattern rule, not a content judgment). Reading the design SPEC and the module docs is expected and required (those are not reviews).

AFTER you have written your own independent findings, you MAY consult round 1's material (`_mill/loom-review-r1.md`, `_mill/loom-review-r1-fixer-report.md`) to (a) confirm round 1's fixes have not regressed and (b) re-evaluate the deferred/residual items listed below. Do NOT re-litigate anything in the CLOSED-AND-VERIFIED list below — the orchestrator independently sabotage-proved those fixes from a cold checkout; re-finding them is wasted round time.

## What to read
- Code: `internal/loomcli/step.go` + `step_test.go`, `internal/loomcli/selfreport.go` + `selfreport_test.go` + `selfreport_github_test.go`, `internal/loomcli/drive.go`, `internal/loomcli/bootstrap.go` + `run.go` + `sharedbootstrap.go` (round 1's F-0/F-1/F-3 fixes — `awaitRunLockHalted`, `ensureFrictionDirAfterSeed`, `ensureStatusLockDir`), `internal/loomshed/interruptpolicy.go` + `interruptpolicy_test.go`, `internal/loomengine/anomaly.go` + `anomalybody.go` (+ tests), `internal/loomengine/config.go` (the `selfreport`/`friction`/`friction_timeout_min` keys, plus the new `LoomFrictionLock`) and `template.yaml`, `internal/friction/**`, `internal/frictionengine/**`, `internal/selfreportengine/selfreport.go`, `internal/selfreportcli/**`, `internal/shedadapters/bouncer.go` (round 1's F-4 fix — `awaitLiveSeed`, `bouncerSeedRole`, the re-bounce probe — this is a GENERIC adapter shared by all three review segments; round 1 drove and fixed it only via the `Discussion-Bouncer` instance) and `internal/shedadapters/doc.go`, `internal/websterengine/strand.go` (round 1's F-5 log-level fix), the seven prompt composers that inject `{{.friction_directive}}`.
- Skill: `plugins/ly/skills/ly-supervise/SKILL.md` — updated by round 1 (D-1, F-3, F-4's "no orphan" claim). Confirm its current text matches what the code actually does.
- Docs: `manifest/designs/loom-step.md`, `manifest/designs/self-report-tier1.md`, `manifest/designs/self-report-tier2.md`, `manifest/designs/loom.md` (round 1 updated all four — confirm they still match the code), `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- `tools/sandbox/SANDBOX-CORE-SUITE.md`'s **S8** — round 1 extended it with the never-bootstrapped refusal and `interrupt_policy`. Extend it further if you surface a live/visual behavior it still doesn't cover.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
- Design intent (SPEC, not a review): the three design docs above — all say "Status: Shipped/Done"; use them as the authoritative source of intended v1 scope/behavior, cross-checked against the code as it stands after round 1's fixes.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — does `step`'s ten-key envelope, Tier 1's five anomaly kinds, and Tier 2's friction/reflection machinery actually deliver what the three design docs promise, as they stand now?
2. Correctness — bugs, races, error handling, edge cases, concentrated on the composed live behavior below, PLUS specifically re-verifying round 1's fixes hold and generalize (see "High-yield focus").

## High-yield focus — where this trio's remaining bugs live (drive these, do not just read them)
Round 1 already covered envelope fidelity across a full multi-step run, one `reinvoke` repro (`Discussion-Bouncer` seed, `Discussion-Burler` round), the `handback` row (`Webster`), the `busy` refusal, and one Tier-1 + Tier-2 live-fire off the same halt. Do not simply repeat those — extend into what round 1 itself flagged as unexercised or unresolved:

- **F-4's fix on a SECOND `Bouncer` instance.** `internal/shedadapters/bouncer.go` is a generic adapter shared by `Discussion-Bouncer`, `Plan-Bouncer`, and `Webster-Bouncer`. Round 1 reproduced and fixed the re-bounce-abandons-a-live-seed defect only via `Discussion-Bouncer`. Reproduce the same interrupted-seed-then-reinvoke scenario against `Plan-Bouncer` (or `Webster-Bouncer`) and confirm the fix generalizes — same probe, same no-abandon outcome — rather than assuming shared code implies shared correctness untested.
- **F-0's `halted` predicate, pressed harder.** The fix makes `awaitRunLock` proceed once the persisted status has left `running`, even if that happened before the driver's very first persist (an already-`blocked` resume). Round 1 flagged this as a deliberate narrowing of the refusal, not a defect, but did not drive the specific case: kill a driver process BEFORE it ever calls `Shed.Run` (i.e., before any persist at all, mid-bootstrap) while an OLD status file from a genuinely wedged prior driver still reads non-`running`, and confirm the handshake's `halted` check does not mask a real wedge in that narrow window. If it's genuinely fine, say so with the repro that proves it; if it's a real gap, it's a finding.
- **Drive to `Webster-Review`, `Publish`, and `Finalize` for real.** `dummy-r1` stopped at `Webster-Bouncer` in round 1. Continue a dummy task (new or the same fixture, your call) through `Webster-Bouncer` → `Webster-Burler` (if it bounces) → `Publish` → `Finalize`, confirming envelope fidelity holds on these terminal rows too and that a `RunDone`-triggered friction reflection (as opposed to round 1's only-observed `RunBlocked` trigger) actually fires when `shouldReflectFriction` says it should.
- **A friction note from genuine model judgment, not hand-placed.** Round 1 verified injection (real prompt carries the directive) and aggregation/reflection/filing (hand-placed notes) as two separate legs. Try to get an actual producer to write a note on its own this round — e.g., seed a dummy task with something mildly confusing or a minor inconsistency in its own board/discussion setup (not a doctored prompt — a genuine rough edge) and see whether a producer chooses to flag it. Do not force this; if it doesn't happen naturally, say so plainly rather than manufacturing it.
- **F-6, only if genuinely cheap.** The two-concurrent-drivers race is still PLAUSIBLE-only (traced, not reproduced) per round 1, and the campaign's cost declaration forbids two concurrent dummy-task drives. Do not attempt to force this reproduction — the fix and its unit tests are already sabotage-proved. If you see a genuinely cheap way to exercise it without violating the concurrency ban, note it; otherwise leave it as an accepted, documented residual.
- **Anything your own reading makes you suspicious of.** The floor above is not a ceiling — hand-roll further adversarial scenarios (e.g., a friction note written but the run never reaches Finalize or a stuck state — does it sit unflushed forever? Is that intentional?).

## Explicitly OUT of scope for this round
- `Plan-Sweep` — never built, its own Someday roadmap item. Not a finding.
- Windows-specific behavior — unreachable from this Linux host.
- The `hardener` module (DRAFT, unbuilt).
- The *content quality* of any dummy task's own discussion/plan/implementation.
- **Re-triggering the live-fire self-report path.** The campaign's one deliberate live-fire is already captured (issues #240 and #241 — see "Live-substrate cost declaration" below). Do NOT set up a scenario that could file a third GitHub issue.

## Round context seeded from prior-round verification

**Residual to close / re-evaluate — round 1 found real defects, so this is not a safety pass.** The orchestrator independently verified round 1's work from a cold checkout (rebuilt, re-vetted, re-tested at `-count=5`, reproduced the live smoke suite, and personally sabotage-proved F-0/F-1/F-3/F-4/F-6 by reverting each fix and watching its test fail at the claimed assertion, then restoring). All of it held exactly as claimed.

**CLOSED-AND-VERIFIED — do NOT re-litigate these:**
- F-0 (BLOCKING) — `run`'s handshake no longer misreads a Tier-2 reflection as a wedged spawn. Commit `713ab509a`.
- F-1 (MEDIUM) — the once-per-task friction-directory clear is now wired into `seedAndCommitBootstrap`. Commit `1d671f44c`.
- F-2 (MEDIUM) — `step`'s Tier-1/Tier-2 exemption is now documented in three places; this was a deliberate documentation-only fix (making `step` file automatically is an outward-facing design decision, not a defect). Commit `9600fc799`.
- F-3 (MEDIUM) — `status`/`pause` now create the status lock's parent directory before reading, so their own "no status file" remedies are reachable. Commit `78a407698`.
- F-4 (MEDIUM) — the Bouncer's re-bounce branch now probes for a live seed before concluding the segment is seeded. Commit `eb6af7720` — **but only verified via `Discussion-Bouncer`; see the high-yield focus item above.**
- F-5 (LOW) — Webster Master reclaim on a handback reinvoke now logs at `Warn`. Commit `6a0750a7e`.
- F-6 (LOW) — the friction reflection now takes a non-blocking lock; sabotage-proved at the unit level, but the live two-driver race it targets was never reproduced (cost-forbidden). Accepted residual, not a re-open target unless you find a genuinely cheap repro.
- F-7 (NIT), D-1, D-2 — comment/doc fixes, confirmed via diff review. Commit `6a0750a7e` / `ed9fce0d0`.

**Residual/deferred items — re-evaluate these after your own independent pass:**
1. F-4's fix generalizing to a second `Bouncer` instance (`Plan-Bouncer`/`Webster-Bouncer`) — not yet independently driven.
2. F-0's `halted` predicate under the narrower before-first-persist case described above.
3. `Publish`/`Finalize` rows, and a `RunDone`-triggered friction reflection — neither was driven in round 1.
4. A spontaneously-written Tier-2 friction note from genuine model judgment (not hand-placed).
5. F-6's live race — leave as accepted residual unless a genuinely cheap repro presents itself.

State the **merge bar**: correctness in the NORMAL single-instance flow (a serial, non-interrupted `step`/`run` sequence, plus interrupted-and-resumed repros) is the gate. There is no N×-concurrent-suite diagnostic step for this campaign.

## Live-substrate cost declaration (BLOCKING — read before running anything live)

**Existing `//go:build smoke` tests: LLM-DRIVING: NO**, unchanged from round 1 — `internal/loomcli/smoke_test.go`, `smoke_attachprobe_test.go`, and round 1's new `smoke_bootstrapwiring_test.go` all spawn zero real LLM subprocesses (providerless/stub engines throughout). The bare `-tags smoke -run Smoke` command is safe to run exactly as written.

**The campaign's OWN live-driving mission remains the expensive part.** `loom.yaml`'s shipped template defaults `discussion`/`plan`/`review`/`friction` to `opus[effort=high]` — override all four to a cheap model/effort for your dummy task(s), exactly as round 1 did (round 1 used `sonnet[effort=low]` for discussion/plan/review and `haiku` for friction; reuse or adjust as you see fit). Keep `discussion_interactive: false`. Run every live-substrate invocation one at a time, foreground, waited to completion — never two dummy-task drives concurrently.

**CRITICAL — do not re-trigger the live-fire.** The campaign's one deliberate live-fire is already captured: issue [#240](https://github.com/Knatte18/loomyard/issues/240) (Tier 1) and issue [#241](https://github.com/Knatte18/loomyard/issues/241) (Tier 2, filed by the reflection agent's own judgment off the same halt). **Set `selfreport: false` and `friction: ""` in every dummy task's `loom.yaml` this round** — you do not need either tier's live-filing path to exercise anything on this round's list (Bouncer re-bounce generalization, the `halted` predicate, Publish/Finalize, envelope fidelity). If pursuing "a spontaneous friction note" above tempts you to re-enable `friction`, you may — Tier 2 notes alone do not file anything without a run reaching a halt with `selfreport`-independent aggregation — but leave `selfreport: false` throughout this round regardless, and if a friction reflection would file a real issue on its own judgment (as it did in round 1), catch it before that call and report the near-miss instead of letting a third issue land. When genuinely unsure, disable both and note the tradeoff.

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
- Set up a dummy task in a real wired hub, real CLI, real substrate — not an in-process `RunCLI` call.
- Override `loom.yaml` per the cost declaration above — **`selfreport: false`, `friction: ""`** (or, if pursuing the spontaneous-note item, `friction` on with `selfreport` still off — see above) before driving anything live.
- Walk the "High-yield focus" list above with your own tool calls.
- The list is a FLOOR — devise more adversarial scenarios of your own if the code makes you suspicious of something.
- **"Headless" means "no human required" — NOT "no time/token cost to me."** You are explicitly forbidden from writing "operator-assisted", "cost-bearing", "long-running", "impractical", or "automated context" as a reason to skip live driving.
- The only legitimate "cannot verify" cases: (a) a scenario that structurally requires a human's physical eyes, or (b) a genuine environment gap (check FIRST). Flag those specifically rather than skipping silently.

TEARDOWN DISCIPLINE (critical): tear down every substrate server/session you start. At the end, confirm ZERO stray tmux servers (`pgrep -af tmux` / `ps aux | grep tmux`) and zero stray detached loom drivers (`pgrep -af "loom drive"`). Leave no stray state.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2 — including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires an operator decision on a real design tradeoff, or a capability you don't have — say so explicitly in the fixer report's deferred section.
**A LARGE finding is a SIZE exception, not a severity one** — record it fully, mark it NOT-FIXED-THIS-ROUND with the reason, and the orchestrator will open a proper mill-wiki task for it.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
See "Residual/deferred items" under "Round context seeded from prior-round verification" above — the five numbered items.

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
1. A structured review report at `_mill/loom-review-r2.md` (Executive summary with top risks + merge-readiness opinion; Scope assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including what you could NOT verify and why). Build it incrementally per "Log as you go" above; commit it.
2. A fixer report at `_mill/loom-review-r2-fixer-report.md`: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict, and explicit confirmation that no third GitHub issue was filed). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
