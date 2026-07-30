# `burler` — independent review + fix (single-round prompt-split verification)

> Instantiated from [`review-prompt-template.md`](review-prompt-template.md). See [README.md](README.md) for the loop this prompt runs inside.
>
> **Scope note (operator directive, 2026-07-29):** this is a deliberately NARROW, single-round crucible invocation — not a full multi-round hardening campaign to convergence. The operator asked for exactly one round (`opus`, `high`) to answer one question: **does the new prompt setup actually work?** "The new prompt setup" = the `split-round-prompt` change that replaced burler's one monolithic combined-prompt string with a thin orchestrator (`round-orchestrator-template.md`) that NAMES the absolute paths of three separate instruction files (`instruction-{1-explore,2-review,3-fix}-template.md`), materialized to disk under `.lyx/burler/<round>` by `Engine.Run`, and read by the agent ONE AT A TIME as it reaches each step — instead of the whole thing being inlined up front. The existing `engine_test.go`/`prompt_test.go` suite only exercises this through a `fakeShuttle` test double; it has never been driven by a REAL claude session actually navigating from the orchestrator to the three files. That gap is exactly what this round exists to close. Do the full review+fix job below, but weight your time and attention toward the "High-yield focus" section — that is the actual point of this round.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `burler` module in the loomyard repo, followed by FIXING what you find. Work in the worktree at `/home/knatte/Code/loomyard/wts/burler-prompt-split` (branch `burler-prompt-split`). This worktree already has `main` merged in and PR #112 (the split-round-prompt change) is currently pr-pending — you are reviewing the merged, current state of that branch, not a hypothetical.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of burler's prompt-split correctness (and, secondarily, its general scope/correctness). Hunt for bugs by reading the code AND by driving the real substrate (real tmux + a real logged-in `claude` on PATH, both confirmed present in this environment) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding. Commit message format: `burler: fix <finding-id> — <one-line what/why>`. Do not commit `.scratch/` (gitignored; your review and fixer reports never belong in a commit regardless).

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `.scratch/burler-review-opus-high-r1.md` on disk — before you touch (edit, create, or delete) a single production or test file. Do not fix findings as you go, even ones that look small and obviously right. Write findings down, keep reading/driving, finish the review, save the file, THEN start Job 2.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review under `.scratch/` before you have your own list (there should be none yet — this is round 1 — but the rule stands for any future re-run of this file). Reading the design docs, code, and `_mill/` history (see "What to read" below) is expected and required — that is not a review.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
This round's first attempt (2026-07-29, same tag `opus-high-r1`) was killed mid-session by a host crash before it had written a single commit, `.scratch/` file, or review content — only a stray uncommitted scratch test file (`internal/burlerengine/zz_scratch_cli_smoke_test.go`) was left behind, which has since been removed. That left the orchestrator with zero evidence of what, if anything, had been tried. Do not repeat that: as you work through "What to TEST" below — each hermetic command, each live-smoke run, each live-driving scenario — APPEND your observations to `.scratch/burler-review-opus-high-r1.md`'s "What was tested" section immediately after each command/scenario returns, rather than holding results in your own working context to write out in one pass at the end. Do the same for findings as you form them: jot each one into the file's findings section provisionally as you spot it. Only the executive summary and final severity ordering need to wait until you have the full picture. This file lives under `.scratch/` (gitignored), so writing to it during Job 1 does not conflict with the Sequencing rule above. A round that dies at 95% this time should leave a 95%-complete account on disk, not nothing.

## What to read
- Code: `internal/burlerengine/**` (especially `engine.go`, `prompt.go`, `template.go`, `template.yaml`, `round-orchestrator-template.md`, `instruction-1-explore-template.md`, `instruction-2-review-template.md`, `instruction-3-fix-template.md`, and their `_test.go` counterparts), `internal/burlercli/**`.
- The module doc: `internal/burlerengine/doc.go`'s package comment — it is the as-built source of truth for the split (the "A/B round" and "Weft-blindness" sections in particular).
- `docs/overview.md` (burler's module-table line + the perch→burler→shuttle stack description), `CONSTRAINTS.md` (the **Review Round Invariant** and **Sandbox Suite Coverage** sections specifically), `tools/sandbox/SANDBOX-BURLER-SUITE.md` (scenarios S1–S3 — for scenario ideas AND because they are the closest existing black-box proof of the split; you run them yourself, never via a launcher).
- Design intent for the split itself: this worktree's own git history — `git log --oneline -- internal/burlerengine` and, if still present, `_mill/plan/01-split-round-prompt.md` / `_mill/discussion.md` via `git log --all --oneline -- '_mill/*'` (the task's own worktree state has since been cleaned up by mill-finalize, so these may only be recoverable from an earlier commit — that is expected, not a defect to report).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full.

## Mission (assess on two axes, be adversarial)
1. **Scope / omfang (secondary this round):** does the split match the design intent recorded in `doc.go` and the git history above? Any gap between what the split promises (lazy, step-scoped reads; orchestrator as single source of truth for ordering) and what's actually implemented?
2. **Correctness (PRIMARY this round — the whole reason for this crucible invocation):** does a REAL claude agent, handed only the orchestrator string (never the instruction bodies inline), actually navigate to and read each instruction file at the right step, in order, and complete a full A-then-B round correctly? Concentrate on the historically-fragile area below.

## High-yield focus — where THIS change's real bugs live (drive these, do not just read them)
The hermetic `fakeShuttle`-based tests in `engine_test.go`/`prompt_test.go` are almost certainly solid on their own terms (they were reviewed and passed CI) — but a fake shuttle can never prove an agent *actually* reads a file it's merely told the path of. Treat each of these as an INVARIANT you must actively verify by driving the real substrate:
- **INVARIANT 1 — the agent finds and reads instruction-1 (explore) from the orchestrator's bare path reference**, before doing anything else, and does not hallucinate its content or skip straight to reviewing.
- **INVARIANT 2 — the agent finds and reads instruction-2 (review) only when it reaches the review step**, writes the FULL review to `review_path` on disk BEFORE touching any target/fasit file (the A-before-B gate — this is the Review Round Invariant in `CONSTRAINTS.md`, and the split makes it easier to violate by accident if the orchestrator's sequencing language is unclear standing alone without the instruction bodies inline for context).
- **INVARIANT 3 — the agent finds and reads instruction-3 (fix) only when it reaches the fix step**, fixes ALL recorded findings (including NIT/LOW, not just BLOCKING), and never pushes.
- **INVARIANT 4 — a partial-materialization / stale-path failure mode**: if any of the three instruction files were missing or unreadable at the path the orchestrator names (verify `Engine.Run`'s "write every instruction file before the shuttle starts" claim in `doc.go`/`engine.go` actually holds under a real run — e.g. does a real agent ever get confused, retry, or silently give up if a path briefly doesn't resolve?).
- **INVARIANT 5 — the orchestrator itself does not leak instruction-2/3 body content** (per `TestTemplate_OrchestratorExcludesDownstreamBodies` in `template_test.go`) in a way that changes real agent behavior versus the pre-split monolithic prompt — i.e. confirm the split is behavior-neutral for a real session, not just neutral in the unit-tested marker substitution.

## Explicitly OUT of scope for this round
- `perch` (the round-loop layer above burler) — not touched by this change, not this round's job.
- Cluster fan-out (`ClusterFan`/fork subagents) — orthogonal to the prompt-split fix; touch it only if you have time left after the solo-round invariants above are fully verified, and treat any finding there as a bonus, not a requirement.
- A full multi-round hardening campaign — per the operator's scope note at the top, this is one round only. If you find yourself wanting a second round to converge, say so explicitly in your merge-readiness verdict instead of spawning one yourself.

## Round context seeded from prior-round verification
This is round 1 — there is no prior round and no CLOSED-AND-VERIFIED list yet. There is also no known residual to seed: treat this as a first genuine independent pass, not a safety pass. State the merge bar so you calibrate: correctness of the split in the NORMAL single-instance A-then-B flow (INVARIANTS 1–3 above) is the gate; INVARIANTS 4–5 and the concurrent/cluster axes are diagnostic, not blocking, for this narrow round.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/burlerengine/... ./internal/burlercli/...`
- `go test -count=5 ./internal/burlerengine/... ./internal/burlercli/... ./cmd/lyx/...`

Live smoke — REAL claude, REAL tmux (this is the decisive evidence for this round):
- `go test -tags smoke ./internal/burlerengine/... -run TestSmokeBurlerRoundToyFixture -v -count=1` — this existing opt-in test already drives one full A-then-B round through the real stack (reedengine + claudeengine + shuttleengine.Runner + burlerengine.Engine) over a toy chair/table fixture. Note: `smoke_round_test.go`'s teardown helper (`deferHubRelease`/`hubHolders`) shells out to a Windows-only `pwsh.exe` path that does not exist on this Linux host — `hubHolders` degrades gracefully (returns nil on exec failure) so the test's actual assertions are unaffected, but the cleanup grace/escalation loop may run its full timeout before falling through. That is an environment artifact of the test's teardown probe, not a burler defect — do not report it as one, but DO note in your report if it meaningfully slows things down.
- Read `smoke_round_test.go` in full before running it, so you understand exactly what it does and does not prove (it proves the A→B machinery + file contract + verdict parse against a real engine — never review quality, per its own doc comment).
- Watch the test's real tmux pane if you can (or inspect the kept `RunDir`/session artifacts on a failure) to directly confirm the agent actually opened and read each of the three instruction files at the right step — this is the one thing no assertion in the test itself checks; it only checks the OUTCOME (verdict/findings/file-contents), not that the agent got there via genuinely reading the split files rather than guessing.

Live driving — YOU also drive `lyx burler run` directly, no launcher (this exercises `burlercli`'s wiring, a different code path than the smoke test's direct engine construction):
- Build the dev binary: `./deploy-dev` (resolves `.dev-bin`; re-run after any source change before testing that change).
- Set up a scratch fixture worktree of your own (do NOT reuse this source worktree as the "hub" under test) — the existing `internal/lyxtest.CopyPaired`/`CopyPairedLocal` helpers show the exact shape (a git-initialized hub dir with `shuttle`/`reed` config seeded) if you want to replicate it by hand outside Go, or you can drive it via a small throwaway Go test in a `_scratch` file using those helpers directly and just `t.Log` your observations instead of asserting — either approach is fine, pick whichever gets you to a real `lyx burler run` invocation fastest.
- Run S1 (BLOCKING path — toy chair/table color mismatch) and S2 (APPROVED path — colors already match) from `tools/sandbox/SANDBOX-BURLER-SUITE.md` verbatim against your dev binary. These are the closest existing black-box proof that the new split works end-to-end through the CLI's JSON envelope, not just through direct Go engine construction.
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real substrate session takes real wall-clock minutes. That is expected and budgeted for. Do not write "operator-assisted", "cost-bearing", or "impractical" as a reason to skip live driving — none of S1/S2 structurally need a human.
- TEARDOWN DISCIPLINE: run `lyx reed down` (or equivalent) after any substrate session you start; confirm zero stray tmux servers with `tmux ls` (expect "no server running" or equivalent) before you finish.

## How to judge each finding
For each finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced live) vs PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT. The only legitimate reason to leave something unfixed is that it genuinely requires an operator decision or a capability you don't have this round — say so explicitly in the fixer report's deferred section.

## Deferred items from the prior round — RE-EVALUATE these
None — this is round 1.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the `/code-quality` skill AND `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect (e.g. an agent genuinely getting lost between instruction files under some condition), extend `smoke_round_test.go` or add a new `//go:build smoke` test, following its existing pattern (opt-in, skip when no claude on PATH, poll-with-deadline never fixed sleeps).
- Keep `go build`/`vet`/`test` green after every change. Re-run the live smoke test and your live-driving scenarios after each fix that touches the prompt/template/engine code, to confirm the fix actually holds against a real session, not just hermetically.
- Update `internal/burlerengine/doc.go`'s package comment (and `docs/overview.md`/`CONSTRAINTS.md` if invariants or the module table move) IN THE SAME change as any fix that changes behavior or the split's contract. Do NOT add notes to `manifest/roadmap.md`.
- If `tools/sandbox/SANDBOX-BURLER-SUITE.md` needs a new scenario to cover something this round found (e.g. a specific split-navigation failure mode), extend it — keep `sandbox_coverage_test.go` green.
- Tear down all substrate state; confirm zero stray tmux processes. COMMIT each fix as you finish it — do NOT push. Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report → `.scratch/burler-review-opus-high-r1.md` (Executive summary with top risks + an explicit answer to "does the new prompt split work against a real LLM call?" + merge-readiness opinion; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; What-was-tested with exact commands + observed results, including anything you could NOT verify and why).
2. A fixer report → `.scratch/burler-review-opus-high-r1-fixer-report.md` (what you implemented, what you deliberately deferred and why, exact test commands + results, changed files).
3. In your final chat message: a concise executive summary + counts by severity + the two report paths + an explicit, direct answer to the operator's actual question — does the new orchestrator+three-instruction-file prompt split work when driven by a real LLM, yes or no, with the evidence — plus a merge-readiness verdict. Do not paste the whole reports.

Begin with the clean-room review (read the docs + code, then drive the real substrate per "What to TEST" above), produce your independent findings, save the review report, THEN implement and verify the fixes.
