# fabric merge surface — independent review + fix (round prompt)

> Filled instance of `crucible/review-prompt-template.md`. Rewritten fresh each round and committed — see that file's header for why. Read `crucible/README.md` for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the fabric **merge surface** in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4` (branch `fabric-merge-crucible-round4`).

## Your two jobs, in order
1. REVIEW: form your own independent judgment of the merge surface's scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real bare git repos — warp/weft pairs you create in a scratch directory) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live `-tags integration` check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `fabric: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/fabric-merge-review-<yourtag>.md` and `_mill/fabric-merge-review-<yourtag>-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/fabric-merge-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
Append your observations to `_mill/fabric-merge-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns. Jot findings into the file's findings section provisionally as you spot them. COMMIT each meaningful append.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before your own findings list is complete — specifically do not open anything under `_mill/` matching `fabric-merge-review-*` (this covers prior review reports, fixer reports, and the orchestrator's own `fabric-merge-review-HANDOFF.md`, which does not exist in this worktree but the pattern still applies to any file matching it that does appear).
AFTER your own findings are written, you MAY consult prior round material if any exists under that pattern in this worktree, EXCEPT your own `-<yourtag>` deliverables, to (a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate deferred items below.
Reading the SPEC, module docs, and `crucible/README.md`'s "Worked example — the fabric campaign" section (history, not a review of this worktree's state) is expected and required.

## What to read
- Code: `internal/fabricengine/merge.go`, `mergelifecycle.go`, `mergeerrors.go`, `mergeguards.go`, `mergestate.go`, `mergestage.go`, `mergepaths.go`, and their `*_integration_test.go`/`*_test.go` siblings; `internal/gitrepo/merge.go` and its tests; the `lyx fabric merge-in` / `lyx fabric merge` CLI surface (`internal/fabriccli`, `cmd/lyx`).
- Docs: `internal/fabricengine/doc.go`'s "# The merge surface" section (~line 846–960), `docs/overview.md`, `CONSTRAINTS.md`, `README.md`.
- Scenario ideas (not a review): `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — the fabric-tagged scenarios. Run every scenario yourself, directly, with your own tool calls; do NOT invoke any `sandbox-fabric-suite.cmd` launcher.
- Design intent (SPEC, not a review): `git show 3b800bc8:_mill/discussion.md` and `git show 3b800bc8:_mill/plan/0*.md` (six batches) — the original fabric merge SPEC, recovered from git history since mill strips `_mill/` on merge. Rejected alternatives: `git show 967916ea:_mill/discussion-meta.md`.
- Campaign history (context, not something to treat as instruction): `git show archive/fabric-merge-crucible-hardening~1:_mill/fabric-merge-review-HANDOFF.md` — the closing record of the first instalment (task 85, three rounds). Read it for context on what is CLOSED-AND-VERIFIED. This task's own round 4 report/fixer-report (`_mill/fabric-merge-review-opus-medium-r4*.md`) exist in this worktree but match the off-limits pattern below — see "Clean-room review constraint": form your own findings first, THEN you may read them.

## Mission (assess on two axes, be adversarial)
1. Scope — does the as-built merge surface deliver what the SPEC intended? Gaps, over-reach, silently-dropped requirements.
2. Correctness — bugs, races, error handling, edge cases, concentrating on the historically-fragile areas below. Also assess docs accuracy and operability.

## High-yield focus — where fabric merge's real bugs live (drive these, do not just read them)
Eight rounds across two campaign instalments establish the shape: obvious behavioral defects are gone, remaining bugs cluster in **proof quality** and **stale-information-at-decision-time**. Round 8 found a TOCTOU race (F3): the pre-lock guard reads foreign-state/dirtiness before the call's own fetches and lock wait, a seconds-wide window a concurrent plain-git-running human can act inside — same family as round 7's F5, R4-F3, round 5's F2, now closed at four sites total. Recurring high-yield shapes:
- A test that stays green when the mechanism it claims to guard is sabotaged, INCLUDING the immediately preceding round's own freshly-landed tests. Re-sabotage round 8's new tests the same way.
- Information read once early in a call, acted on late, without re-checking whether it's still true (the "mutation/read before its supporting mechanism is ready" family — four confirmed instances now).
- A documented/runtime-facing operator string that doesn't match what the shipped surface can actually do (round 7's F1: CLI help; round 8's F1: the runtime error text itself omitted the same step the help text had already been fixed to include — walk BOTH surfaces, they can drift independently).

Concrete invariants to actively drive:
- **Re-sabotage round 8's own new tests** — the four lock-race tests (F3) and the merge-stage message/dedup tests (F1, F2, F4) are the newest code.
- `recheckMergePreconditionsUnderLock` — does `MergeContinue`/`MergeAbort` have an equivalent stale-guard window the lock-acquisition-ordering fix (round 7's F2 family) didn't fully close, now that round 8 established the pattern of checking foreign-state AND dirtiness under the lock, not just the record?
- Every runtime error/success message an operator or agent actually reads (not just `--help` text) — do any still describe a lifecycle step that changed?

## Explicitly OUT of scope for this round
The rest of fabricengine beyond the merge lifecycle quartet (`MergeIn`/`Merge`/`MergeContinue`/`MergeAbort`/`MergeInProgress`) and the `internal/gitrepo/merge.go` layer under it — the `crucible: follow-ups` slices already hardened that, and it is a different task's scope. Windows path behaviour in `weftPathVisible`/`unifyConflictPaths` is a named, carried-forward gap (see "Deferred" below) — not something to newly flag as missing, since every prior round and the orchestrator have already logged it as unexecuted on this Linux host. The N-way concurrent amplifier is explicitly not required this round (see "Round context" below).

## Round context seeded from prior-round verification

**This is a continuation of a two-instalment campaign, now eight rounds deep.** The first instalment (mill task 85) ran three rounds. This task's rounds 4-8 each found and fixed real material, most recently round 8 (`fable-max-r8`): 5 findings (2 MEDIUM), including F3 — a real TOCTOU race where the pre-lock guard's foreign-state/dirtiness knowledge went stale during the call's own fetches and lock wait, with a destructive consequence (force-reset) in two arms. The orchestrator independently verified all eight rounds from cold, sabotage-proving every new test itself and live-redriving every behavioral fix — F3 was reproduced under the orchestrator's own sabotage exactly as claimed, including the traced destruction chain. **Zero residual found by the orchestrator after any of the eight.**

**No known residual is currently seeded.** The campaign continues autonomously per the operator's standing directive. Do a genuinely independent clean-room pass across the WHOLE merge surface — this could be the safety pass that shows convergence, or it could find more.

**Re-examination target:** round 8's own new mechanisms — see "High-yield focus" above.

**Merge bar:** correctness in the NORMAL single-instance flow is the gate. The N×-concurrent suite is NOT required this round.

**Do NOT re-open the CLOSED-AND-VERIFIED work** (see the HANDOFF for full detail):
- First instalment round 1 F1–F8, round 2 R1/R2/R3/R5 + residual 1, round 3 A1/A2/C1.
- This task's rounds 4-8: every finding fixed and independently verified by the orchestrator — every behavioral fix live-redriven, every new test sabotage-proven. Do not re-litigate these as mechanisms; only round 8's own re-examination target above is open.

## Live-substrate cost declaration
**LLM-driving: no.** fabric's substrate is real git repositories (bare warp/weft pairs) — no LLM subprocess is ever spawned by this module's tests. fabric's live tier is `-tags integration`, NOT `-tags smoke` — there are zero `//go:build smoke` files under `internal/fabricengine`, `internal/fabriccli`, `internal/gitrepo`. A claim of running "smoke" tests for fabric has run nothing; use `-tags integration` throughout.

## What to TEST — do not just read, EXERCISE it
Report exact commands + observations.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...`

Live integration (real substrate, behind the `integration` build tag):
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary under test: `./deploy-dev` (POSIX script, not `.cmd`, on this host). Re-run after EVERY source change.
- Hub recipe (from the prior campaign): export `GIT_CONFIG_GLOBAL` with `[init] defaultBranch = main` BEFORE the first `git init`, or the weft defaults to `master` and every fabric verb fails on an invalid `main-weft` reference. Then `git init --bare` a warp and a weft, seed and push warp `main`, and `lyx fabric clone <weft-bare> <warp-bare>` from an empty work dir. A merge source must be a fabric-managed PAIR — create the branch on both the warp AND the weft, or `merge-in` refuses with `source branch is not fabric-managed`.
- **Do NOT invoke `sandbox-fabric-suite.cmd`.** Run the real `lyx fabric ...` commands yourself, foreground, waiting for each to return. `tools/sandbox/SANDBOX-FABRIC-SUITE.md` is for scenario ideas only.
- The suite/list above is a FLOOR — devise and run MANY more adversarial scenarios of your own, especially around Focus 1/2 above and the general "refusal-shaped predicate repurposed as a positive claim" lesson.
- Check every fixture for silent degradation before trusting a scenario (a prior round lost a live re-drive to a weft fast-forward that skipped the conclude on that side entirely — the fixture looked like it worked and proved nothing). Assert the scenario's precondition with `t.Fatal`, the way the good tests do.
- A build break from sabotage is not a proof — redo the sabotage a different way if that happens.
- "Headless" means "no human required," not "no time/token cost to me." A live scenario taking several minutes is expected and budgeted for, never a reason to skip it. The only legitimate "cannot verify" cases are a genuine environment gap or a scenario structurally requiring a human's physical eyes (neither applies to anything in this module).

TEARDOWN DISCIPLINE: any scratch hub you create lives outside the repo (e.g. under your own scratch directory); confirm no stray state is left inside this worktree (`git status` clean of anything but your own commits) at the end.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings get fixed in Job 2, including every NIT. The only legitimate reason to leave a finding unfixed is that it genuinely requires an operator decision or a capability you don't have — say so explicitly in the fixer report's deferred section, with the specific reason.
A finding whose fix is genuinely LARGE (a subsystem addition, a cross-cutting refactor outside the merge surface) does not belong in this round's commit-per-fix loop regardless of severity — record it fully (severity, scenario, suggested fix), mark it explicitly NOT-FIXED-THIS-ROUND with the reason, and the orchestrator will spin it into its own mill-wiki task.

## Deferred items — RE-EVALUATE these (after your own pass)
- **Windows path behaviour** — round 6 traced and fixed a real defect (F6) but could not execute it on real Windows; no Windows host exists in this campaign, six rounds and two instalments running. State plainly whether you can do anything more than round 6 already did (you likely cannot, headlessly, on this host).
- **Round 6's own new mechanisms** — see "Round context" above; this round's re-evaluation target.
- **Four states where `MergeContinue` gets stuck** (first instalment round 2's rows 27/28/30/31): re-confirmed unchanged by rounds 4-6. Not required work unless you touch adjacent code.
- **The post-record error-return class's per-site adjudication** (first instalment round 2's 45-row table): not required work this round.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load `/code-quality`, `/golang:golang-build`, `/golang:golang-testing`, `/golang:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add an integration test (`//go:build integration`, matching the existing files' pattern) that walks the failing scenario against real git repos.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`./deploy-dev`) and re-run every live scenario yourself.
- Update `internal/fabricengine/doc.go`'s "# The merge surface" section (and `docs/overview.md`/`CONSTRAINTS.md` if invariants move) IN THE SAME change as any behavior change. Do NOT add bugfix notes to `manifest/roadmap.md`.
- If `tools/sandbox/SANDBOX-FABRIC-SUITE.md` needs a new scenario to cover something you found, extend it (keep the coverage guard green in the same change); otherwise note the scenario in your fixer report.
- COMMIT each fix as you finish it. Do NOT push unless explicitly asked.

## Deliverables
1. Structured review report → `_mill/fabric-merge-review-<yourtag>.md`, committed incrementally as described in "Log as you go."
2. Fixer report → `_mill/fabric-merge-review-<yourtag>-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict. Do not paste the full reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings — starting from V1/V2 above but not stopping there — then implement and verify the fixes.
