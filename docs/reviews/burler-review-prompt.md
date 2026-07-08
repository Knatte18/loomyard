# burler — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `burler`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`C:\Code\loomyard\wts\internal-burler` (branch `internal-burler`). Adjust that path/branch if
the task lives elsewhere now. Note: PR #57 (https://github.com/Knatte18/loomyard/pull/57) is open
against `main` for this branch but not yet merged — you are hardening it further before merge,
exactly like the mux and shuttle campaigns hardened their branches before their own merges.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of burler's scope and correctness. Hunt for bugs by
   reading the code AND by driving real rounds — a real psmux session with a real, logged-in
   `claude` doing an actual review+fix pass over real files (this is where burler's defects hide —
   it is a thin, prompt-composing caller on shuttle whose real behavior only shows when a live
   agent walks the A→B round it prescribes).
2. FIX: after you have a findings list, implement the fixes ONE AT A TIME, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do
   NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green, and its doc update (if any) is included, COMMIT
it — on the current branch, no push — before starting the next finding. Commit message format:
`burler: fix <finding-id> — <one-line what/why>` (e.g. `burler: fix B1 — reject a review file whose
frontmatter closes without a verdict key`), so the finding ID matches exactly what your review
report calls it. Do not commit `.scratch/` (gitignored). This exists because round 2 of the shuttle
loop was killed mid-fix by a terminal corruption issue outside the method's control, leaving
several real fixes sitting as one uncommitted diff with no fixer report — the orchestrator had to
reverse-engineer, finding by finding, which fixes were actually complete before it could safely
continue. Small per-finding commits make that recovery trivial instead.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/burler-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. A review written or finished after code has already changed is no longer an
independent judgment — it is a post-hoc rationalization of edits you already made, and it silently
destroys the one property this whole method depends on. If you catch yourself wanting to patch
something the moment you spot it: don't. Write it down as a finding, keep reading, finish the
review, save the file, THEN start Job 2. (Burler's own embedded round template states this exact
rule to the agents IT spawns — the Review Round Invariant in `CONSTRAINTS.md`. You reviewing burler
are held to the discipline burler itself enforces.)

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you
have your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior
reviews `burler-review-*.md` and `*-fixer-report.md`) or any `_mill/` content (there is none left
on this branch — it was removed by the pre-merge cleanup commit; see "Design intent" below for how
to recover it). Reading the design SPEC and the module docs is expected and required (those are
not reviews). AFTER you have written your own independent findings, you MAY consult prior rounds'
`.scratch/burler-review-*` material — regardless of which model produced it (rounds rotate across
Opus / Fable / Sonnet; the most recent prior round is whichever `burler-review-*` file is newest),
EXCEPT your own `-<yourtag>` deliverables — to (a) confirm previously-fixed behaviors have not
regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code: `internal/burlerengine/**` (incl. the embedded round template
  `review-prompt-template.md` + `template.go`, and the smoke test `smoke_round_test.go`),
  `internal/burlercli/**`, and the `cmd/lyx` integration (`main.go`, sandbox/help/registration
  guard tests). Also skim `internal/shuttleengine/**` far enough to understand the
  `Spec`/`Result`/outcome contract burler drives (burler is a thin caller on top of shuttle,
  exactly as shuttle is on mux — not a reimplementation), and `internal/stencil` (the marker-fill
  mechanism `composePrompt` uses).
- Docs: the `internal/burlerengine` package documentation (`doc.go` — the module doc was folded
  into the package doc per the Documentation Lifecycle, exactly like mux and shuttle),
  `docs/overview.md`, `docs/roadmap.md`, `CONSTRAINTS.md` (esp. the Review Round Invariant — it is
  burler's OWN invariant — plus Weft Git, Shuttle Provider-Seam, CLI/Cobra, Hub Geometry, Sandbox
  Suite Coverage), `README.md`, and `docs/reviews/README.md` (the method burler automates — burler
  is the mechanized form of the very loop reviewing it here).
- The dedicated live-driving suite you will RUN: `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  (scenarios S1–S3) plus [`docs/sandbox-howto.md`](../sandbox-howto.md) for how the sandbox harness
  works — including its attached-terminal pre-condition (see "What to TEST" below).
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`. A
  change that ships behaviour without updating the package doc / invariants in the SAME change is
  incomplete.
- Design intent (SPEC, not a review). `_mill/discussion.md` and `_mill/plan/*` were removed from
  this branch by the pre-merge cleanup commit; recover them from git history — the last commit that
  still had them is `4db6373` ("mill-go: done internal-burler"):
    - `git show 4db6373:_mill/discussion.md`
    - `git show 4db6373:_mill/plan/00-overview.md` and the per-batch cards
      `git show 4db6373:_mill/plan/NN-*.md` (01..05)
  Use these as the authoritative source of intended v1 scope/behavior. Also useful for prose
  context: `git log --oneline main..internal-burler` shows the batch-by-batch build order and the
  holistic-review round that already ran during implementation (`6ef3987`). That was a
  *design-time* review inside mill-go's own pipeline; this loop is a *deeper, adversarial,
  live-substrate* pass on top of it, the same relationship the mux and shuttle campaigns had to
  their own mill-go reviews.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below (burler's hermetic layer is a fake-shuttle unit suite — thorough, but by
   construction it never sees what a REAL agent does with the composed prompt, which is where the
   whole module's value lives). Also assess docs accuracy (do the docs match the code?) and
   operability.

## High-yield focus — where burler's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (Profile validation, ParseReview's pinned rejection table, stencil
marker filling, the CLI flag→Profile mapping) are solid and rarely wrong. The defects concentrate
in the COMPOSED, LIVE behavior the fake-shuttle tests never exercise — a real agent interpreting
the composed prompt in a real pane. Treat every one of these as an INVARIANT you must actively
verify by driving real rounds — a green `go test` proves nothing here:

- **A-BEFORE-B IS PROMPT-LEVEL ONLY.** Nothing in Go verifies the review file was complete on disk
  before the agent's first target edit — the gate lives entirely in the embedded template's
  sequencing rule. Drive real rounds and check the evidence trail (review-file write time vs the
  target file's change; the agent's own transcript): does a live agent actually hold the gate, or
  does it interleave? If it interleaves, the fix layer is the template/prompt (and possibly a
  detectable-violation check), not wishful prose.
- **DONE-BUT-MALFORMED REVIEW.** Shuttle classifies `done` on bare existence of BOTH output files;
  burler then strictly parses the review. Chase the boundary: an agent that writes a structurally
  broken frontmatter (missing verdict key, duplicated finding ids, a verdict in lowercase, prose
  before the opening `---`), or that creates the review file early and fills it late (does a
  partially-flushed review ever coexist with an existing fixer-report long enough for `done` +
  `ParseReview` to read a truncated file?). The engine's deliberate fail-loud choice must hold
  under real agent behavior, not just crafted fixtures.
- **FIX-SCOPE DISCIPLINE IS PROMPT-LEVEL ONLY.** `FixScopeOverlay` promises: write-surface is
  EXACTLY Target.Paths + the two output files, and the round runs NO git commands (Weft Git
  Invariant). `FixScopeSource` promises commit-per-fix on the host, never push. Drive both scopes
  live and audit afterwards (`git status`/`git log` in the fixture repo, file mtimes outside the
  allowed surface): does a real agent respect the boundary? An overlay round that quietly runs
  `git add`/`commit`, or writes outside its surface, is a BLOCKING finding even though no Go code
  is "wrong".
- **PROMPT-COMPOSITION INJECTION.** `composePrompt` interpolates caller content (Rubric,
  Instructions, paths) into a stencil template. Probe hostile-but-plausible content: a rubric
  containing stencil marker syntax or template-like braces, backticks, YAML-significant characters
  in instructions, worktree paths with spaces/parentheses. Does the composed prompt corrupt, and
  does an error surface loudly at compose time rather than as a confused agent?
- **NON-DONE OUTCOMES END-TO-END.** asking/died/timeout are contractually "normal loop events" with
  an empty Verdict. Drive at least one for real (e.g. a tiny `--timeout`, or kill the claude
  process in the pane mid-round) and verify: the CLI envelope reports it sanely (correct outcome
  string, no bogus verdict, non-misleading exit semantics), the pane/run-dir survive for diagnosis
  per shuttle's KeepPane-on-non-done rule, and nothing in burler mistakes a stale review file from
  a PRIOR round for this round's result.
- **TOOL-USE TOGGLE IS PROMPT-LEVEL ONLY.** `tool-use: false` promises a read-only-analysis round
  (no effect on the shuttle Spec in v1 — a documented decision). Verify a live `tool-use: false`
  round doesn't run builds/tests anyway, and that the prompt wording actually produces the
  read-only behavior it claims.
- **CLI PROFILE SURFACE.** The profile YAML is the module's only user-authored input: strict
  decode (unknown keys must be rejected, not ignored), the `--profile` requiredness path (recently
  reworked in `d737572` — probe it), relative-path resolution against the WORKTREE ROOT (not the
  shell cwd; Hub Geometry), directory targets, and the distinctness/sanity of every validate error
  (empty fasit, bad fix-scope, cluster-n > 0, pre-existing review-path via shuttle's rejection —
  S3's three cases plus any you devise beyond them).
- **VERDICT/FINDINGS CONTRACT FOR PERCH.** Findings ids are perch's future cycle-detection keys:
  uniqueness and non-emptiness are enforced, but chase what else a real agent emits that the parser
  tolerates and a future perch would choke on (unknown severity spellings? whitespace ids? a
  BLOCKING verdict with zero findings, or APPROVED with a BLOCKING-severity finding — is
  cross-field consistency checked anywhere, and should it be?).

## Explicitly OUT of scope for burler v1
`perch` (round loops, caps, convergence, cross-round no-self-grading) and `loom` are separate,
not-yet-built modules — do not review them or expect burler to already behave like their future
caller; a single `Engine.Run` only guarantees A-before-B within its own round, and that is correct.
Cluster fan-out (`ClusterN > 0`) is deliberately gated on mux own-window anchoring (roadmap
milestone 24) — its rejection with `ErrClusterUnsupported` is the v1 behavior, not a gap. Review
QUALITY (whether a round's findings are insightful) is explicitly not under test — the toy
scenarios are trivial on purpose; assess the MECHANICS. Non-Claude engines are shuttle's seam
concern, not burler's. Weft commits are the loop owner's job — burlerengine's total absence of
weft imports and `_lyx` path construction is the invariant (verify the absence holds; do not flag
it as missing functionality). `ToolUse` having no shuttle-Spec effect is a documented v1 decision.

## Round context seeded from prior-round verification
**Round 2 — independent safety pass.** Round 1 (`fable-r1`, Fable) ran a full independent
first pass and found 2 findings (0 BLOCKING, 1 MEDIUM, 0 LOW, 1 NIT), fixed both, and the
orchestrator INDEPENDENTLY VERIFIED the round clean. There is NO known open residual right
now. What is CLOSED-AND-VERIFIED, so you neither re-litigate it nor mistake it for something
still open:

- mill-go's own design-time holistic review ran during implementation and approved (`6ef3987`);
  it was a code review, not an adversarial live-substrate campaign.
- Two sandbox-suite launcher fixes at `c86dcd3`: `lyx mux down` after mux-backed suite sessions;
  non-TTY launch warning. `SANDBOX-BURLER-SUITE.md` gained the attached-terminal pre-condition,
  never-background rule, Teardown section.
- **Round `fable-r1`** (commits `c72cbd9`, `5810d0d`), independently verified by the orchestrator
  on the committed tree:
  - **N1 (NIT, `c72cbd9`):** `ErrClusterUnsupported`'s error text carried a doubled `"burler: "`
    prefix (every call site already wraps it in its own burler-prefixed message). Fixed by
    dropping the sentinel's own prefix; `TestProfile_Validate` now asserts exactly one
    `"burler: "` prefix on every validate error. Orchestrator reintroduced the doubled prefix,
    confirmed the new assertion fails at the right line, reverted to an empty diff.
  - **B1 (MEDIUM, `5810d0d`):** `Profile.validate` didn't reject `ReviewPath == FixerReportPath`
    (checked on the RESOLVED absolute paths, after `resolvePath`), letting an operator
    copy-paste mistake collapse the two-artifact file contract into one file that still
    silently classified `done`/`APPROVED` — reproduced live. Fixed with a new validate check +
    two `TestProfile_Validate` subtests (literal and post-resolution identical-path cases).
    `tools/sandbox/SANDBOX-BURLER-SUITE.md` S3 gained a 4th case for this exact mistake.
    Orchestrator reintroduced the bug, confirmed both new subtests fail at the right assertion,
    reverted to an empty diff.
  - Orchestrator's own independent gates, run cold on the committed tree AFTER `fable-r1`
    finished: `go build ./...` clean; `go vet` clean; `go test -count=5` on
    burlerengine/burlercli/cmd/lyx all green; one live serial smoke round green (39s); 3×
    CONCURRENT full smoke suites all green (51-58s each), zero corruption markers
    (`being used by another process` / `TempDir RemoveAll` / `did not start` / `FAIL`); zero
    stray psmux processes after both the serial and the concurrent runs.
  - The round's own live-substrate work: 10+ real psmux+claude rounds including S1/S2/S3 from
    the sandbox suite plus hand-driven adversarial probes (prompt-injection into composed
    prompts, `--timeout`, killed-mid-round `died`, FixScopeSource commit-per-fix audit,
    directory targets, foreign severity vocabulary). All held up — A-before-B sequencing
    verified via file-mtime ordering (not just template prose), FixScope discipline held for
    both `overlay` and `source`, non-done outcomes classified correctly, prompt composition
    proved injection-safe. No scope gaps found against the recovered design discussion.

**This is a safety pass, not a residual-chase.** There is no known open defect to hand you.
Your job: an independent, adversarial, clean-room pass over the WHOLE module (not just the two
areas `fable-r1` touched) to find what a first round, working alone, might have missed — or
honestly confirm merge-readiness if you find nothing. Do not assume `fable-r1`'s two fixes are
the only defects burler has; treat the module as if you are seeing it for the first time,
same as round 1 was instructed to. Pay particular attention to areas `fable-r1`'s report says
it verified only via the toy fixture / S1-S3, since the "High-yield focus" list below is a
floor, not a ceiling, and a differently-modeled agent (you) may probe differently.

State the **merge bar** so you calibrate: correctness in the NORMAL single-round flow (one
profile, one round, one agent, cluster-n 0) is the gate; concurrent/stress rounds (if you choose
to run them) are a diagnostic amplifier, not a merge blocker.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/burlerengine/... ./internal/burlercli/...`
- `go test -count=5 ./internal/burlerengine/... ./internal/burlercli/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/burlerengine/... -run Smoke -v -count=1` — one full real round
  (A-review then B-fix) over a toy fixture; skips when no claude resolves.
- psmux is installed at `C:\Code\tools\bin\psmux.exe` (also on PATH as `psmux`); pwsh 7 at
  `C:\Code\tools\powershell7\pwsh.exe`. A logged-in `claude` must be on PATH for the real-agent
  scenarios — launch tools with EXPLICIT absolute paths where the codebase does (a bare `pwsh`
  resolves to a 0-byte WindowsApps ConPTY stub that renders nothing).

Live driving via the SANDBOX SUITE (PRIMARY — where the bugs surface):
- Deploy the current source as the binary under test: `deploy.cmd`. **FOOTGUN:** the suite runs the
  DEPLOYED snapshot, not your working tree — re-run `deploy.cmd` after EVERY source change or you
  validate a stale binary. Deploy first, always.
- Materialize the sandbox hub: `sandbox-build.cmd` (or `-reset` for a clean start), run `lyx init`
  in the hub host repo, then launch the interactive suite session: `sandbox-burler-suite.cmd`.
  **The launcher MUST run in a real, attached terminal — never backgrounded, detached, or with
  stdio redirected** (the suite doc's pre-condition 5; the launcher warns if you get this wrong,
  and a detached driver session can end early and silently abandon scenarios). Follow the suite
  file's own Pre-conditions + "How to run a scenario" sections. Walk S1 (BLOCKING path), S2
  (APPROVED path), and S3 (three error paths) and record OK/WARN/FAIL per its verdict key. After
  the session, pull findings back with `sandbox-fetch.cmd`.
- The suite is a FLOOR, not a ceiling. Devise and run MANY more adversarial rounds of your own
  beyond S1–S3 — the "High-yield focus" list above is your checklist for what to hand-drive that
  the three scripted scenarios don't isolate (fix-scope boundary audits, malformed-review probes,
  non-done outcomes, injection-shaped rubrics, tool-use:false honesty, etc). Real rounds cost real
  agent minutes — sequence them deliberately, but do not substitute reading for driving.

TEARDOWN DISCIPLINE (critical): if you start any psmux server/session, tear it down
(`lyx mux down`, or `psmux -L <socket> kill-server`). The burler-suite launcher now runs
`lyx mux down` itself after the session — verify it actually did. At the end, confirm with
`tasklist | grep -i psmux` that ZERO psmux processes remain, and that no shuttle run directories
are left under the hub host repo's `_lyx/shuttle/` from a run you started. Leave no stray state.
Be honest about what you could NOT verify and why (e.g. anything needing a claude account state
you don't have).

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped;
flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write down but
leave unfixed as "low priority" is not actually a reported finding; it is a dropped one that will
either silently vanish or re-surface and loop across future rounds instead of closing (this is a
known failure mode from an earlier review setup this method is descended from — and it is the very
rule burler's own embedded template enforces on ITS agents; hold yourself to it). The only
legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you
cannot do alone this round — an operator decision on a real design tradeoff, or a live capability
you don't have. Even then you must say so explicitly, with the specific reason, in the fixer
report's deferred section — never bucket something as "deferred, low priority" just because it
felt small. Small and low-severity findings are usually the CHEAPEST to fix, not a reason to skip
them.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
(Empty — `fable-r1` deferred nothing; both its findings were fixed in full.)

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT (see "How to judge each
  finding" above for the full rationale) — not just BLOCKING/MEDIUM ones.
- Load the code-quality guidance (`/code-quality` skill or `mill:code-quality`) AND the
  language-specific skills for this codebase (`mill:golang-build`, `mill:golang-testing`,
  `mill:golang-comments`) before editing — all of them, not just code-quality. (This is called out
  explicitly because a round agent on shuttle's second round loaded code-quality only and skipped
  the golang skills; the operator caught it live and had to stop the round to redirect it.) Prefer
  surgical edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect,
  add a `//go:build smoke` test that walks the failing scenario against the real substrate
  (`internal/burlerengine/smoke_round_test.go` shows the pattern, incl. the skip when the
  substrate is absent and the self-contained-helpers convention). A hermetic unit test for the
  pure helper is good; a smoke test for the composed behavior is what protects the round
  discipline. For a PROMPT-LEVEL defect (template wording a live agent misreads), the fix is the
  embedded template + `template_test.go`'s pinned statements — and a re-driven live round as the
  proof.
- MAKE SMOKE TESTS DETERMINISTIC. A live round's timing is the agent's own — a test that assumes
  turn timing passes on a quiet machine and FLAKES on a loaded one. Wait on the actual state
  transition (poll with a deadline), never sleep a fixed amount. Prove determinism by running the
  new test repeatedly, not once.
- EXTEND THE SANDBOX SUITE when a review surfaces a live/visual behavior it doesn't cover (match
  the existing S1–S3 Goal/Watch/Verdict shape; keep the coverage guard green in the SAME change —
  `go test ./cmd/lyx/...` includes `sandbox_coverage_test.go`).
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy.cmd`) and re-run
  the smoke + suite scenarios — re-deploying FIRST is mandatory (the suite tests the deployed
  binary, not your working tree).
- Update the `internal/burlerengine` package documentation (and `docs/overview.md` /
  `CONSTRAINTS.md` if invariants or the module table move — the Review Round Invariant's
  enforcement note must stay accurate) IN THE SAME change. Do NOT add bugfix/hardening notes to
  `docs/roadmap.md` (roadmap is planned milestones only, per `CLAUDE.md`).
- Tear down all substrate state; confirm zero stray psmux processes and no leftover shuttle run
  directories. COMMIT each fix as you finish it (see "Commit per fix" above) — do NOT push unless
  the user explicitly asks. Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands and
   observed results, including what you could NOT verify and why). Write it to
   `.scratch/burler-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/burler-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive real rounds), produce
your independent findings, then implement and verify the fixes.
