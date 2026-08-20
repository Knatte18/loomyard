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
- Campaign history (context, not something to treat as instruction): `git show archive/fabric-merge-crucible-hardening~1:_mill/fabric-merge-review-HANDOFF.md` — the closing record of the prior campaign (task 85), which ran three rounds and stopped with two open findings (named below as this round's residual). Read it for context on what is CLOSED-AND-VERIFIED; do not re-litigate that material.

## Mission (assess on two axes, be adversarial)
1. Scope — does the as-built merge surface deliver what the SPEC intended? Gaps, over-reach, silently-dropped requirements.
2. Correctness — bugs, races, error handling, edge cases, concentrating on the historically-fragile areas below. Also assess docs accuracy and operability.

## High-yield focus — where fabric merge's real bugs live (drive these, do not just read them)
The prior campaign's three rounds establish the shape: the obvious behavioral defects are gone, and what remains concentrates in **proof quality** — tests that pass for the wrong reason, invariants asserted by mechanisms that cannot detect a violation, doc claims the code does not back. The three highest-yield shapes, each found by a distinct prior round:
- A test that stays green when the mechanism it claims to guard is sabotaged (residual A, residual V2 — the AST closure test scoped to one file).
- A fix driven live in only its happy direction, never its adversarial one (V1 — the adoption arm).
- A refusal-shaped predicate reused to justify a positive claim instead of a refusal — same ambiguous read, opposite risk direction (V1's core lesson: `concludeLandedReason` uses HEAD-moved-with-no-MERGE_HEAD to *refuse* an abort safely; `sideConcludeAlreadyLanded` uses the identical read to *claim* a successful adopt). Look for other places in the merge surface where a refusal-shaped check has been repurposed to justify a positive/successful outcome.

Concrete invariants to actively drive:
- Crash-recovery idempotency — `MergeContinue` resumed after a crash between `git commit` and the record save must adopt the landed commit, never re-commit and never silently adopt an unrelated commit (see Focus 1 below).
- The closed guard-reason set must be genuinely closed — a `mergeReason*` constant declared ANYWHERE in the package, not just in `mergeerrors.go`, must be caught (see Focus 2 below).
- `MergeAbort`'s refusal to destroy a landed conclude vs. `MergeContinue`'s willingness to adopt one — both read the same ambiguous HEAD-moved signal; audit every other site in the merge surface reading that same signal for which way it resolves the ambiguity.
- Both checkouts must be on a branch before a merge verb proceeds (`checkout is not on a branch`) — still correct? still tested live?
- `MergeResult.Committed`/`AlreadyUpToDate` must be read off the record's own fields, never hardcoded per return site — still true after any of this round's own edits?

## Explicitly OUT of scope for this round
The rest of fabricengine beyond the merge lifecycle quartet (`MergeIn`/`Merge`/`MergeContinue`/`MergeAbort`/`MergeInProgress`) and the `internal/gitrepo/merge.go` layer under it — the `crucible: follow-ups` slices already hardened that, and it is a different task's scope. Windows path behaviour in `weftPathVisible`/`unifyConflictPaths` is a named, carried-forward gap (see "Deferred" below) — not something to newly flag as missing, since every prior round and the orchestrator have already logged it as unexecuted on this Linux host. The N-way concurrent amplifier is explicitly not required this round (see "Round context" below).

## Round context seeded from prior-round verification

**This is a continuation of a stopped campaign (mill task 85, `fabric-merge-crucible-hardening`), tagged onward from its round 3.** Rounds 1–3 ran (`opus-medium-r1`, `opus-medium-r2`, `fable-medium-r3`); the orchestrator independently verified every round; two findings from round 3's own verification were never closed before the operator stopped the campaign. This round (`opus-medium-r4`) starts by closing those two, then continues the crucible loop as an ordinary round — review the WHOLE merge surface adversarially, not just these two items; they are the confirmed floor, not the ceiling.

**Residual to close — V1 (BLOCKING, confirmed still present in the current tree):**
`sideConcludeAlreadyLanded` (`internal/fabricengine/mergelifecycle.go:105-121`) detects "HEAD moved off the recorded pre-merge start, with no live `MERGE_HEAD`" and adopts whatever commit HEAD now points to as this merge's conclude — clearing the merge record and reporting `committed:true`. This predicate cannot distinguish the real conclude-commit from ANY other commit landed on that checkout while a merge record is live (e.g. an operator's `git merge --abort` followed by one unrelated commit). Verified live in the prior campaign (`ahub2`, see the HANDOFF's V1 section): the adversarial sequence returns `{"already_up_to_date":false,"committed":true,...}` naming the unrelated commit, deletes the record, and leaves the actual merge source un-merged with no record left to inspect — a silent false success. Confirmed a regression, not a pre-existing hole: disabling the adoption arm and re-driving the same sequence instead returns an honest `merge conclude did not finish; run MergeContinue again` with the record retained.
The discriminating evidence the prior campaign named but the fix never used: a genuine non-squash conclude is a two-parent merge commit whose second parent is the merge source; the falsely-adopted commit is not. A squash conclude has one parent, so it has no such evidence and needs its own explicit decision (most likely: refuse to adopt for a squash merge and stay honest-but-stuck, matching the pre-fix behavior) rather than silently inheriting the non-squash predicate.
Acceptance bar (behavioral, not structural):
- The adversarial scenario above must stop returning `ok:true`/`committed:true` for the unrelated commit, and must not delete the record.
- The legitimate crash-recovery shape must still adopt correctly: HEAD unmoved except by the real conclude, no second commit fabricated, the merge source really an ancestor of the result, the operator's resolution intact, the record cleared, sibling verbs released.
- Both directions proven by sabotage (neuter the mechanism, watch the new test fail at its intended assertion, restore to an empty diff) AND re-driven live on a fresh hub against a freshly deployed binary — including the squash shape, which the prior campaign only reasoned about and never drove.
- Audit `concludeLandedReason` (`MergeAbort`'s guard) and every other reader of the same "HEAD moved, no live MERGE_HEAD" signal for the same asymmetry: a refusal-shaped reading is safe when ambiguous; a claim-shaped reading is not.

**Residual to close — V2 (MEDIUM, confirmed still present in the current tree):**
`TestMergeVocabulary_GuardReasonSetMatchesConstBlock` (`internal/fabricengine/mergevocab_test.go:50`, `parser.ParseFile(fset, "mergeerrors.go", ...)`) parses `mergeerrors.go` only. A `mergeReason*` constant declared anywhere else in the package — `mergeguards.go` is the natural place, next to the guard that would consume it — escapes both the pinned-map equality check and every vocabulary/leak assertion the pinned map drives. Confirmed still true by inspection of the current source. Fix: parse the package's non-test files rather than one hardcoded filename, or add an explicit assertion that no `mergeReason*` constant is declared outside `mergeerrors.go`. Prove detection with the same sabotage the prior round used (declare one in `mergeguards.go`, use it in production code, confirm the hermetic tier now fails where it previously stayed green).

**Merge bar:** correctness in the NORMAL single-instance flow is the gate. The N×-concurrent suite is NOT required this round — fabric's merge surface is not tmux-shaped and the merge bar is single-instance correctness (see `crucible/README.md`'s note on this, carried forward from the prior campaign's deliberate scoping).

**Do NOT re-open the CLOSED-AND-VERIFIED work** (see the HANDOFF for full detail — do not re-litigate any of this):
- Round 1 F1–F8, all sabotage-proven and live-redriven.
- Round 2 R1 (`MergeStart` misclassifying an empty-result merge as staged), R2 (`concludeLandedReason` refusing `MergeAbort` when a conclude may have landed), R3, R5, and residual 1 (`bothSidesAlreadyUpToDate` coverage).
- Round 3's A1/A2 (the original AST-backed closed-set test, for the single-file scope it does cover) and C1.

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

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- **Windows path behaviour** in `weftPathVisible`/`unifyConflictPaths` — never executed by any round or the orchestrator, on any campaign. Linux host throughout. State plainly whether you touched it (you likely cannot, headlessly, on this host) rather than letting silence imply coverage.
- **The squash shape of V1** — reasoned about in the prior campaign, never driven live. This round's V1 fix must drive it (see Focus 1's acceptance bar above).
- **Four states where `MergeContinue` gets stuck** (round 2's rows 27/28/30/31): the conclude lands but `CurrentSHA`/`saveMergeState` fails, so the record never learns and plain git becomes the only recovery. Round 2 deliberately declined to make the conclude idempotent there; the guard (round 2's R2) at least stops `MergeAbort` from destroying them. Re-evaluate whether this round's V1 fix changes that calculus at all (it shouldn't — different failure shape — but confirm).
- **Round 2's 45-row per-site adjudication of the post-record error-return class** was reproduced at the raw-count level by round 3 but never re-checked row by row. Not required work this round; note if you happen to touch it.

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
