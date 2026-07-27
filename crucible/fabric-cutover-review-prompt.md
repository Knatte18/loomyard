# `fabric-cutover` — independent review + fix (round agent prompt)

> Instantiated from [`review-prompt-template.md`](review-prompt-template.md) for the **consumer
> side** of the fabric cutover (commit `9d0c4935`, "fabric: cutover -- rewire consumers onto
> fabric, delete warp/weft"). See [README.md](README.md) for the loop this prompt runs inside.
>
> **Why this is a separate instance from `fabric-review-prompt.md`.** That file documents fabric's
> own internal hardening campaign (5 rounds, Opus/Fable-rotated, already converged and merged) —
> fabric's own git-coordination correctness is **already validated** and is explicitly OUT OF SCOPE
> here (see below). This campaign instead reviews whether the modules the cutover **rewired onto**
> fabric — `buildercli`/`webstercli`'s weft-commit helpers and `configcli`/`configreg`'s config-sync
> wiring — still behave correctly now that they call `fabricengine`/`fabriccli` instead of the
> deleted `warpengine`/`weftengine`/`weftcli`. Do not reuse or consult
> `fabric-review-prompt.md`/its `.scratch/fabric-review-*` deliverables — different scope, different
> module tag (`fabric-cutover-review-<tag>.md`, not `fabric-review-<tag>.md`).

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the fabric
**cutover's consumer integration** in the loomyard repo, followed by FIXING what you find. Work in
the worktree at `C:\Code\loomyard\wts\crucible-fabric-cutover` (branch
`crucible-fabric-cutover`). Adjust that path/branch if the task lives elsewhere now.

**Hub geometry note:** live driving of builder/webster weft commits and `lyx config` sync rides
the same dedicated fabric test hub fabric's own campaign used — the two dedicated GitHub repos
`Knatte18/lyx-fabric-test` (host) and `Knatte18/lyx-fabric-test-weft` (weft) — never the shared
`Knatte18/lyx-test`/`Knatte18/lyx-test-weft` repos. `lyx fabric clone` materializes this dedicated
hub; see `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s Pre-conditions for the exact clone/reuse
semantics (`lyx-fabric-test-HUB` is reused idempotently if it already exists on this machine, never
reset).

## Your two jobs, in order
1. REVIEW: form your own independent judgment of the cutover's consumer-side correctness. Hunt for
   bugs by reading the code AND by driving the real substrate (real git repos/worktrees over real
   filesystem junctions, real `lyx builder`/`lyx webster`/`lyx config` invocations) — this is where
   the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do
   NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live
smoke/driving check if the finding needed one), and its doc update (if any) is included, COMMIT it
— on the current branch, no push — before starting the next finding. Commit message format:
`fabric-cutover: fix <finding-id> — <one-line what/why>` (e.g. `fabric-cutover: fix F2 — restore
builder weftCommit SkipGit short-circuit parity`). Do not commit `.scratch/` (gitignored; your
review and fixer reports never belong in a commit regardless).

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/fabric-cutover-review-<yourtag>.md` on disk — before you touch (edit, create, or delete)
a single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. Write it down as a finding, keep reading, finish the review, save the file, THEN
start Job 2.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have
your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior reviews
`fabric-cutover-review-*.md` and `*-fixer-report.md` — and unrelated prior campaigns'
`fabric-review-*.md`, out of scope regardless). Reading the design SPEC and the module docs is
expected and required (those are not reviews). AFTER you have written your own independent
findings, you MAY consult the prior rounds' `.scratch/fabric-cutover-review-*` material —
regardless of which model produced it (rounds alternate Fable/Opus; the most recent prior round is
whichever `fabric-cutover-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to
(a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at
the bottom.

## What to read
- Code under review: `internal/buildercli/weft.go`, `internal/webstercli/weft.go`,
  `internal/configcli/configcli.go` (specifically `runConfig`'s `realSync` closure and
  `dispatch`), `internal/configreg/configreg.go`.
- The API surface these consumers now call: `internal/fabricengine/fabric.go` (`New`,
  `EnvSyncOptions`, `SyncOptions.SkipGit`/`SkipPush`), `internal/fabricengine/weftgit.go`
  (`CommitWeft`, `PushWeft`, `PushWeftAt` — note each independently re-checks
  `SkipGit`/`SkipPush`), `internal/fabricengine/syncweft.go`, `internal/fabriccli/fabric.go` +
  `internal/fabriccli/weft_verbs.go` (the `sync` subcommand `configcli` now shells into via
  `fabriccli.RunCLI`).
- `internal/hubgeometry` — `Layout.WeftWorktree()`/`WorktreeRoot`/`RelPath`, the fields both
  `weftCommit` helpers depend on.
- The exact diff that created this task, for the authoritative "what should have changed and what
  shouldn't have": `git show 9d0c4935 -- internal/buildercli/weft.go internal/webstercli/weft.go
  internal/configcli/configcli.go internal/configreg/configreg.go`. Treat any BEHAVIORAL delta
  beyond "route through fabricengine/fabriccli instead of warpengine/weftengine/weftcli" as
  suspect until proven intentional and correct.
- Docs: `internal/builderengine/doc.go` and `internal/websterengine/doc.go` (the "weft-blind"
  sections — `weft` as a concept term is intentionally retained post-cutover; only the
  *implementation* moved to fabric), `docs/overview.md`, `CONSTRAINTS.md` (Weft Git Invariant, Hub
  Geometry Invariant), `README.md`.
- Scenario ideas (not a review): `tools/sandbox/SANDBOX-BUILDER-SUITE.md`,
  `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`. You run every
  scenario yourself, directly, with your own tool calls; you do NOT invoke any
  `sandbox-<module>-suite.cmd` launcher (that spawns a SEPARATE, context-free interactive `claude`
  session for a human operator's own dogfooding — meaningless for you to spawn on top of yourself).
- Existing smoke coverage to extend, not duplicate: `internal/buildercli/smoke_test.go`,
  `internal/webstercli/smoke_test.go` (both `//go:build smoke`).
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry, Weft Git, CLI/Cobra, lyxtest Leaf, Sandbox Suite Coverage, Documentation
  Lifecycle). A change that ships behaviour without updating the module doc / invariants in the
  SAME change is incomplete.
- Design intent (SPEC, not a review): the cutover commit message and diff itself
  (`git show 9d0c4935`) is the authoritative statement of intended behavior — a mechanical
  rewire onto fabric with **zero** observable behavior change for builder/webster weft commits or
  `lyx config`'s sync path. Anything that changed observable behavior beyond "which package does
  the git work" is a candidate defect, not an intentional improvement, unless you find explicit
  evidence otherwise (e.g. a doc comment explaining a deliberate behavior change).

## Mission (assess on two axes, be adversarial)
1. **Behavioral parity** — does builder/webster's weft-commit helper, and `configcli`'s sync
   wiring, behave IDENTICALLY to the pre-cutover warp/weft-backed implementation (same pathspec
   exclusions, same commit message format, same SkipGit/SkipPush/CI-bypass semantics, same
   error-propagation and exit-code contract)? Any divergence is a regression unless justified.
2. **Correctness** — bugs, races, error handling, edge cases introduced by the restructuring;
   concentrate on the historically-fragile areas below. Also assess docs accuracy (do the docs
   match the code?) and residue (stale references to the deleted `warpengine`/`weftengine`/
   `weftcli`/`warpcli` packages or the deleted `lyx warp`/`lyx weft` CLI commands anywhere
   reachable from these four files — imports, comments, help text, tests).

## High-yield focus — where this cutover's real bugs live (drive these, do not just read them)
The pure/unit-tested parts are usually solid; defects concentrate in the COMPOSED, LIVE behavior
the hermetic tests never exercise. Treat each as an INVARIANT you must actively verify by driving
the real substrate — a green `go test` proves nothing here.
- **SkipGit/CI-bypass restructuring risk.** The cutover changed both `weftCommit` helpers from a
  single `weftengine.Commit(...)` call to an explicit `if !opts.SkipGit { f, err :=
  fabricengine.New(...); ...CommitWeft... }` guard, followed by an UNCONDITIONAL
  `fabricengine.PushWeftAt(weftWorktree, opts)` call outside that guard. `PushWeftAt` internally
  re-checks `opts.SkipGit || opts.SkipPush` (`weftgit.go`), so this LOOKS safe on inspection — but
  verify it live, both ways: with `WEFT_SKIP_GIT=1` set (the CI/test bypass — confirm zero real git
  calls happen, no error even when no weft worktree exists on disk) and unset (confirm a real
  commit+push happens exactly as before). Also confirm `fabricengine.New`'s stat-based path
  validation — which the old single-call shape may not have hit at all in bypass mode — doesn't now
  spuriously reject a legitimate CI environment.
- **Pathspec exclusion parity, driven end-to-end.** Builder excludes `*.lock` and its pause flag;
  webster excludes those plus `*/webster/prompts/*`. Don't just read the exclusion list — drive a
  real builder and a real webster run that produces lock files, a pause flag, and (webster) rendered
  fork prompts, trigger a real weft commit, and confirm via `git show`/`git status` on the real weft
  worktree that none of the excluded artifacts landed in the commit or show as tracked.
- **`lyx config` sync parity.** `configcli.runConfig`'s `realSync` closure now calls
  `fabriccli.RunCLI(w, []string{"sync"})` instead of the deleted `weftcli.RunCLI(w, []string{"sync"})`.
  Confirm the exit code, stdout shape (JSON vs text, per whatever `lyx config`'s existing contract
  is), and error surfacing are byte-for-byte equivalent to what a pre-cutover run would have
  produced — drive `lyx config --set <module>.<key>=<value>` end-to-end against a real fabric-paired
  hub and confirm the weft side actually receives the sync.
- **`configreg.Modules()` completeness and correctness.** Confirm the module list no longer
  references `warp`/`weft` anywhere (template names, comments, generated YAML keys) and that
  whatever the config UI (`lyx config`) surfaces for module selection/help text is fully consistent
  post-removal — drive `lyx config` interactively (or via its non-interactive flags) and read the
  actual output, don't just read the Go literal.
- **Residue sweep, scoped to these four files' blast radius only** (NOT a whole-repo sweep — that
  is explicitly out of scope, see below): grep from `internal/buildercli`, `internal/webstercli`,
  `internal/configcli`, `internal/configreg` outward for any surviving `warpengine`/`weftengine`/
  `weftcli`/`warpcli` import, `lyx warp`/`lyx weft` string, or comment describing pre-cutover
  behavior as current. Report anything found even if you don't fix all of it (see "Deferred" note
  below for one already-known example explicitly out of scope).

## Explicitly OUT of scope for this campaign
- **Fabric's own internal git-coordination correctness** (topology invariants, reconcile/prune
  drift repair, junction/symlink wiring, weft content-sync honesty) — already independently
  hardened and converged via the separate `fabric-review-prompt.md` campaign (5 rounds,
  Opus/Fable-rotated, merged). Do not re-review `internal/fabricengine`/`internal/fabriccli`'s own
  logic; only review how the four consumer files listed above CALL it.
- **`hubgeometry`, `initengine`, `loomengine/preflight`, `reedengine/config`, `cmd/lyx`
  registration** — these were also touched by the cutover commit but are explicitly deferred to a
  separate, narrower-scoped follow-up campaign at the operator's discretion. Do not review or fix
  them here even if you notice something; note it in your report as an aside if you want, but do
  not spend review or fix budget on them.
- **`internal/treadleengine`** — unrelated to the fabric cutover (a separate round-loop-engine
  extraction, commit `90856b5d`). One known stray doc-comment residue exists there
  (`internal/treadleengine/engine.go:7` says "never imports weftengine/warpengine/hubgeometry",
  naming two now-deleted packages) — this is informational only, already known, explicitly NOT
  yours to fix; a different task owns treadle.
- **Documentation Lifecycle-wide fabric terminology sweep** — do not go hunting for every mention
  of "warp"/"weft" across the whole repo's docs; `weft` as a concept term (weft-blind, weft-commit,
  Weft Git Invariant) is intentionally retained post-cutover and is NOT itself a defect.

## Round context seeded from prior-round verification
**Round 1 — first pass, no prior rounds.** This is the first round of a fresh, narrowly-scoped
campaign; there is no CLOSED-AND-VERIFIED list yet and no known residual. Do a genuinely
independent clean-room review of the four files' cutover integration per the Mission and
High-yield focus above. The campaign is planned for up to 4 rounds total, alternating **Fable,
Opus, Fable, Opus** — this round is Fable.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the
gate; an N×-concurrent suite is optional here (this is not a concurrency-heavy surface), never a
required gate.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/buildercli/... ./internal/webstercli/... ./internal/configcli/... ./internal/configreg/... ./internal/fabricengine/... ./internal/fabriccli/...`
- `go test -count=5 ./internal/buildercli/... ./internal/webstercli/... ./internal/configcli/... ./internal/configreg/... ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/buildercli/... -run Smoke -v -count=1`
- `go test -tags smoke ./internal/webstercli/... -run Smoke -v -count=1`
- Substrate is plain `git` (must be on PATH) plus real filesystem junctions via `internal/fslink`,
  plus (for builder/webster's own live scenarios) a real logged-in `claude` — check for this FIRST,
  before anything else, so you know up front whether it applies.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the binary under test (`deploy-dev.cmd`). **FOOTGUN:** live driving
  runs the DEPLOYED snapshot, not your working tree — re-deploy after EVERY source change or you
  validate a stale binary.
- **Do NOT invoke any `sandbox-<module>-suite.cmd`.** Those launchers spawn a SEPARATE,
  context-free interactive `claude` session for a human operator's own dogfooding, not for you to
  spawn on top of yourself. Instead, run the real CLI commands yourself, directly, foreground,
  waiting for each to return: walk the "High-yield focus" list above against the dedicated fabric
  hub geometry, and record OK/WARN/FAIL for each.
- The list above is a FLOOR — devise and run MORE adversarial scenarios of your own beyond it
  (e.g. a builder/webster run interrupted mid-weft-commit; `lyx config` invoked with no weft
  worktree present at all; a `WEFT_SKIP_GIT=1` run followed immediately by a non-bypass run on the
  same worktree).
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real git/agent
  operation takes real wall-clock time, not zero. That cost is EXPECTED and BUDGETED FOR, never a
  reason to skip a scenario. You are explicitly forbidden from writing "operator-assisted",
  "cost-bearing", "long-running", "impractical", or "automated context" as a reason to skip live
  driving.
- **Before writing "could not verify", ask yourself literally: "would a human's physical eyes be
  required here, or am I just trying to avoid spending my own time/turns?"** Only the first is a
  real reason.
- The only legitimate "cannot verify" cases are: (a) a scenario that structurally requires a human
  to visually confirm something, or (b) a genuine environment gap (missing `git`, no GitHub auth
  for the dedicated test repos, no logged-in `claude` for builder/webster's own live driving — check
  for this FIRST). Flag those specific cases as not-headlessly-verifiable rather than skipping
  silently, and say exactly what blocked you.

TEARDOWN DISCIPLINE (critical): if you clone/materialize any hub, worktree, link, or builder/webster
run during testing, tear it down. At the end, confirm ZERO stray fabric-managed
worktrees/junctions and no uncommitted drift left in the dedicated test repos you touched. Leave no
stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). For parity findings: pre-cutover
behavior vs post-cutover behavior, with evidence for both sides where possible.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. The only legitimate reason to
leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this
round — an operator decision on a real design tradeoff, or a live capability you don't have. Even
then you must say so explicitly, with the specific reason, in the fixer report's deferred section —
never bucket something as "deferred, low priority" just because it felt small.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None — this is round 1 of the campaign.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build`/
  `mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect,
  extend the existing `internal/buildercli/smoke_test.go` or `internal/webstercli/smoke_test.go`
  (both `//go:build smoke`) rather than creating a new smoke file, unless the scenario doesn't fit
  either's existing shape.
- MAKE SMOKE TESTS DETERMINISTIC. Git/filesystem operations are not instantaneous; a test that
  assumes a verb is synchronous passes on a quiet machine and FLAKES on a loaded one. Wait on the
  actual state transition (poll with a deadline), never sleep a fixed amount.
- If your review surfaces a live/visual behavior `SANDBOX-BUILDER-SUITE.md` or
  `SANDBOX-WEBSTER-SUITE.md` doesn't cover, extend the relevant one (match the existing scenario
  shape; keep each file's `**Covers:**` coverage guard green in the SAME change). If the change
  doesn't warrant a new scenario, note it in your fixer report instead.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY and re-run every live
  scenario yourself, directly.
- Update `internal/builderengine/doc.go`/`internal/websterengine/doc.go` (and `docs/overview.md`/
  `CONSTRAINTS.md` if invariants move) IN THE SAME change as any fix that changes documented
  behavior. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes/worktrees/links. COMMIT each fix as
  you finish it (see "Commit per fix" above) — do NOT push unless the user explicitly asks. Report
  the changed files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Parity
   assessment pre-cutover-vs-post-cutover; Code findings severity-ranked with file:line + scenario
   + fix + CONFIRMED/PLAUSIBLE; Docs & residue findings; What-was-tested with exact commands +
   observed results, including what you could NOT verify and why). Write it to
   `.scratch/fabric-cutover-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/fabric-cutover-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the diff + code + docs, then drive the real substrate),
produce your independent findings, then implement and verify the fixes.
