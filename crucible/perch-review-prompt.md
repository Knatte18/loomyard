# `perch` — independent review + fix (instantiated from the template)

> Instantiated from [`review-prompt-template.md`](review-prompt-template.md) — see
> [README.md](README.md) for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `perch`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`C:\Code\loomyard\wts\crucible-treadle-run` (branch `crucible-treadle-run`).

## Why this campaign exists (read this first)

`perch` used to own its round loop directly. Commit `90856b5d` ("Treadle: shared round-loop
engine + perch rewrite") extracted that loop — the round-caps ladder, the verdict-judge model
(circling + milestone), the judge-maintained handoff, the pluggable gate, the died/timeout
retry and asking-triage machinery, pause, and run-dir locking — out of `internal/perchengine`
into a new, generalized `internal/treadleengine`, so a second future consumer (Tenter, see
`manifest/designs/hardener.md`) can reuse it. `internal/perchengine` is now a thin
configuration layer: it resolves `perch.yaml`/profile data, adapts `internal/burlerengine`
onto `treadleengine.RoundRunner` (`internal/perchengine/adapter.go`), and delegates to
`treadleengine.Engine.Run`. Both package docs assert perch's own exported Go API and observable
behavior are **unchanged from the outside** after this extraction.

**Your job is to find out whether that claim is actually true.** `treadleengine` has no CLI and
no stand-alone consumer of its own today — `perch` is its only reference consumer, and
`internal/treadleengine` is explicitly forbidden from importing `internal/burlerengine` or any
`internal/*cli` package (the Treadle Runner-Seam Invariant, `CONSTRAINTS.md`, enforced by
`internal/treadleengine/seam_enforcement_test.go`). That means the only way to exercise
treadle's real, composed, live behavior is through `perch`'s own CLI (`lyx perch run|pause`)
driving real `burler` rounds over real substrate. This is NOT a from-scratch review of perch —
it is a regression-hardening campaign for a large internal refactor, so weight your review
toward "did the extraction change any observable behavior, timing, or edge case" over "is
perch's design good."

## Your two jobs, in order

1. REVIEW: form your own independent judgment of whether perch's round-loop behavior — as
   observed through `lyx perch run`/`lyx perch pause` — is unchanged from what the pre-extraction
   design promised (see "What to read" and "High-yield focus" below). Hunt for bugs by reading the
   code AND by driving the real substrate (real tmux via `reed`, real `claude` via `shuttle`,
   real `burler` rounds) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do
   NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)

As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live
smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it —
on the current branch, no push — before starting the next finding. Commit message format:
`perch: fix <finding-id> — <one-line what/why>` (use `treadle: fix <finding-id> — ...` instead if
the actual defect lives in `internal/treadleengine` rather than `internal/perchengine` — name the
module the fix actually lands in). Do not commit `.scratch/` (gitignored). This exists because a
round agent's session can be killed mid-fix by something entirely outside the method's control — a
trail of small commits turns a crash into something the orchestrator can just read from `git log`.

## Sequencing rule (BLOCKING — do not skip, do not interleave)

Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/perch-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a single
production or test file. Do not fix findings as you go, even ones that look small and obviously
right. Write it down as a finding, keep reading, finish the review, save the file, THEN start
Job 2.

## Clean-room review constraint (do this part unprimed)

Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have
your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior reviews
`perch-review-*.md` and `*-fixer-report.md`). Reading the design SPEC and the module docs is
expected and required (those are not reviews). AFTER you have written your own independent
findings, you MAY consult the prior rounds' `.scratch/perch-review-*` material — regardless of
which model produced it (rounds rotate across Opus / Fable; the most recent prior round is
whichever `perch-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to (a)
confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the
bottom.

## What to read

- Code: `internal/perchengine/**`, `internal/perchcli/**`, `internal/treadleengine/**` (the engine
  perch now delegates to — a defect here is just as in-scope as one in perch's own package, since
  this campaign exists to verify the extraction), `cmd/lyx` integration for the `perch` wiring.
- Docs: `internal/perchengine/doc.go` and `internal/treadleengine/doc.go` (the module docs — this
  repo deletes standalone `manifest/designs/<module>.md` files once a module ships, per the
  Documentation Lifecycle, so the package doc IS the module doc), `docs/overview.md` (perch/burler/
  treadle entries), `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- `tools/sandbox/SANDBOX-PERCH-SUITE.md` — for SCENARIO IDEAS only (S1: gate-loop convergence +
  pause/resume; S2: command gate). You run every scenario yourself, directly, with your own tool
  calls; you do NOT invoke `sandbox-perch-suite.cmd` (that spawns a SEPARATE, context-free
  interactive `claude` session for a human operator's own dogfooding — meaningless for you to spawn
  on top of yourself).
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry, CLI/Cobra, lyxtest Leaf, Sandbox Suite Coverage, Documentation Lifecycle, and
  especially the **Treadle Runner-Seam Invariant** and **Weft Git Invariant** — both directly
  relevant to this refactor).
- Design intent (SPEC, not a review): there is no separate discussion/plan doc for this refactor.
  The authoritative "intended behavior" is the PRE-extraction perch (recover it via
  `git show 90856b5d^:internal/perchengine/...` / `git log -p` on the parent commit) compared
  against today's `internal/perchengine` + `internal/treadleengine` — the extraction's own stated
  goal is zero externally-observable behavior change, so any diff you find IS the finding.

## Mission (assess on two axes, be adversarial)

1. Scope / omfang — did the extraction actually preserve every invariant perch's old round loop
   had? Anything silently dropped, subtly reordered, or newly gated on something the old code
   didn't gate on (e.g. the new bounded judge handoff read-set is a genuine behavior change from
   the old "read every prior review" — is it actually equivalent in every case that matters, or
   does it introduce a new "judge-gap" edge case the old code never had)?
2. Correctness — bugs, races, error handling, edge cases in the composed perch+treadle+burler
   flow; concentrate on the areas below. Also assess docs accuracy (do `doc.go`'s claims match
   actual behavior?) and operability.

## High-yield focus — where a treadle-extraction regression would hide (drive these, do not just read them)

The pure/unit-tested parts (treadle's own fake-RoundRunner tests, perch's fake-burler tests) are
usually solid; a real seam defect concentrates in the COMPOSED, LIVE, cross-package behavior none
of those hermetic suites exercise together. Treat each as an INVARIANT you must actively verify by
driving `lyx perch run`/`lyx perch pause` against real burler rounds — a green `go test` proves
nothing here:

- **RoundRunner seam hydration fidelity** — run a multi-round BLOCKING block (`round-caps: [2, 3]`
  or wider) and confirm round N+1's burler round actually receives round N's review AND
  fixer-report as hydration (via the adapter, `perchengine/adapter.go` →
  `treadleengine.AttemptInput.PriorReviews`/`PriorFixerReports`) — not silently dropped or
  truncated by the seam. Inspect the round's own prompt/context, not just the final verdict.
- **Milestone ladder + verdict-judge triggers** — with round-caps like `[2, 4, 6]`: confirm the
  circling judge fires ONLY after a BLOCKING round with a prior round to compare (never round 1,
  never right after an APPROVED round), the milestone judge fires INSTEAD OF the circling check
  exactly on a milestone-rung round still BLOCKING, and the LAST rung (hard cap) still BLOCKING is
  an unconditional STUCK (`StuckHardCap`) with NO judge call at all — confirm this by inspecting
  `state.json`'s per-round records for the absence of a judge call on that final round.
- **Judge-maintained handoff — bounded read-set + fallback** — force at least one real judge call,
  inspect the written `round-<token>-handoff.md`: confirm every ledger entry from the previous
  handoff reappears (open or resolved, never dropped) and `covers_rounds` grows correctly across
  rounds. Then deliberately corrupt or delete a handoff file mid-block (between rounds) and confirm
  the next judge call degrades gracefully — falls back to the next older valid handoff, or to the
  full pre-handoff all-reviews read (`collectJudgeReviews`) if none are valid — rather than
  crashing the block or forcing a false STUCK.
- **Pause mid-flight, and resume** — S1's pause/resume scenario, but push on the race
  `treadleengine/doc.go` itself flags: request a pause while the LAST round is still actively
  running (not idle between rounds) and confirm the block still settles cleanly at the next round
  boundary rather than losing state or double-processing. Confirm resume continues `roundsRun` from
  where it paused (not from 0) and that the pause flag does not linger into a later terminal
  (APPROVED/STUCK) block.
- **Run-dir mutual exclusion under real concurrency** — start one `lyx perch run` against a
  run-dir, and WHILE it is still running, start a second `lyx perch run` against the exact same
  run-dir (same `--run-id`) from a second terminal/process. Confirm the second fails FAST with the
  `ErrBlockBusy`-wrapped "already running" error rather than blocking, corrupting `state.json`, or
  interleaving rounds from both processes.
- **died/timeout retry, and asking-triage** — these are now generic `treadleengine` machinery
  rather than perch-owned code. Force (or find a way to simulate) a died/timeout burler attempt and
  confirm the deterministic single retry + second-consecutive-hard-error behavior is unchanged;
  same for an `asking` outcome and the RETRY/GIVE_UP triage call. A hard error must still name the
  round, the shuttle SessionID, and the kept run dir.
- **Gate modes, esp. `command` and `both`** — S2's command-gate coverage, but also drive a gate
  command that exceeds `gate.timeout`: confirm it is recorded as a FAILING gate with a timeout note
  (not a crash), and that mode `both` genuinely requires BOTH a burler APPROVED verdict AND a zero
  exit code to converge (test the case where one is true and the other false, both directions).
- **Weft commit at block exit** — confirm the weft commit still happens exactly once, only at a
  terminal outcome (APPROVED/STUCK/PAUSED), with the correct commit message shape, and that a
  PAUSED-then-resumed-then-terminal block does not double-commit or skip the commit.

## Explicitly OUT of scope for this campaign

- A single burler round's own internal mechanics (verdict parsing, the A-before-B gate, cluster
  fan-out, `FixScope` behavior) — that is `SANDBOX-BURLER-SUITE.md`'s and burler's own crucible
  instance's territory. perch composes burler; this campaign does not re-review burler itself,
  except where the SEAM between perch/treadle and burler is the actual suspect.
- Building a new `hardener`/Tenter consumer of treadle, or judging whether the RoundRunner seam
  is a good abstraction for a hypothetical second consumer — that is a design question for
  `manifest/designs/hardener.md`, not a defect in what already shipped.
- `internal/burlerengine`'s cluster/fork-subagent machinery — unrelated to the treadle extraction.

## Round context seeded from prior-round verification

**Safety pass — no known residual.** Round `fable-r1` found and fixed 5 real defects (F1 BLOCKING,
F2 MEDIUM, F3/F4 LOW, F5 NIT) plus one out-of-scope substrate bug (O2, `reed header --blocking`
instant deadlock-panic) and a sandbox-suite gap (D3, now closed with a new S3 scenario). The
orchestrator independently re-verified from a cold state on the committed tree (`go build`, `go
vet`, `go test -count=5` across `internal/perchengine`, `internal/perchcli`, `internal/treadleengine`,
`cmd/lyx` — all green; `git ls-files --eol` confirms the four embedded templates are back to
`w/lf`; the new `TestEngine_JudgeSkippedRoundReadsNoHandoffFiles` and
`TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock` regression tests both pass `-count=3..5`;
`TestSandboxCoverage_AllModulesCoveredOrExcluded` still green after the S3 addition; working tree
clean). The orchestrator did not re-run fable-r1's full live-substrate campaign (~20 real burler
rounds + 6 real judge calls) — that evidence is taken from the round's own report, which is
detailed and command-by-command, but treat it as read, not re-proven; you are not primed by it and
your own live driving is the actual gate.

**CLOSED-AND-VERIFIED — do NOT re-open or re-litigate these:**
- F1: `.gitattributes` `eol=lf` pins moved to `internal/treadleengine/{judge-circling,judge-milestone,triage,targeting}-template.md` (`97aa88e0`).
- F2: `internal/treadleengine/smoke_judge_test.go` now sets `judgeInputs.HandoffPath` and asserts the produced handoff parses (`40825a62`).
- F3: the runner-agnostic error-body generalization ("round N attempt run", "kept run dir") is now documented in both `doc.go`s as a deliberate, prefix-preserving change (`b7f695b2`).
- F4: `judgeReadSet` is now assembled lazily inside the judge branches so judge-skipped rounds do no handoff I/O (`56fdf6ad`).
- F5: `ConfigTemplate`'s stale `judge_effort` doc comment corrected (`c64ec86a`).
- D3: `tools/sandbox/SANDBOX-PERCH-SUITE.md` S3 (milestone ladder, circling vs. milestone judge, handoff ledger, hard-cap-no-judge) added (`7fa7a141`).
- O2 (out-of-scope but fixed): `lyx reed header --blocking` deadlock-panic fixed via a sleep-loop park (`c7f0ace3`), with a smoke regression test.
- Every "High-yield focus" invariant in this file was live-driven at least once by fable-r1 with real evidence (session output, state.json contents, timestamps) recorded in `.scratch/perch-review-fable-r1.md` — read it AFTER you have your own independent findings (see "Clean-room review constraint" above), and use it to decide what's worth re-driving vs. what's worth attacking from a fresh angle.

**Your job this round:** do a genuinely independent clean-room pass. Do NOT assume fable-r1 caught
everything — a different model, rotated in deliberately, is expected to notice different things.
Either find a real residual (a behavior the treadle extraction changed that fable-r1 missed, or a
new defect its own fixes introduced) or honestly confirm merge-readiness. "No new defects, ship it"
is a valid, expected, valuable outcome of a safety pass — do not invent work to justify the round.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the
gate; concurrent/stress scenarios (e.g. many parallel perch invocations) are a diagnostic
amplifier, not a merge blocker — but the run-dir mutual-exclusion scenario above is itself part of
the NORMAL correctness bar (two operators/terminals racing the same run-id is an ordinary use
case), not a stress test.

## What to TEST — do not just read, EXERCISE it

Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/perchengine/... ./internal/perchcli/... ./internal/treadleengine/...`
- `go test -count=5 ./internal/perchengine/... ./internal/perchcli/... ./internal/treadleengine/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/perchcli/... -run Smoke -v -count=1`
- `go test -tags smoke ./internal/treadleengine/... -run Smoke -v -count=1` (treadle's own
  `smoke_judge_test.go` drives a real judge call directly — this is in scope too, since a seam
  defect can live on either side).
- tmux (or the Windows tmux port), PowerShell 7, and a logged-in `claude` must be on PATH.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary under test: `deploy-dev.cmd`. **FOOTGUN:** live
  driving runs the DEPLOYED snapshot, not your working tree — re-run `deploy-dev.cmd` after EVERY
  source change or you validate a stale binary. Deploy first, always.
- **Do NOT invoke `sandbox-perch-suite.cmd`.** Instead, run `lyx perch run`/`lyx perch pause`
  yourself, directly, foreground, waiting for each to return: walk the "High-yield focus" list
  above (and `SANDBOX-PERCH-SUITE.md`'s S1/S2 for extra scenario ideas) and record OK/WARN/FAIL for
  each. This spawns real substrate underneath (real tmux panes, real `claude` sessions, real burler
  rounds) — that is expected and required. None of it needs an attached TTY of its own.
- The list above is a FLOOR — devise and run MANY more adversarial scenarios of your own beyond it.
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real perch block can
  take real wall-clock MINUTES per round (a real burler round is a real shuttle/claude session).
  That cost is EXPECTED and BUDGETED FOR. You are explicitly forbidden from writing
  "operator-assisted", "cost-bearing", "long-running", "impractical", or "automated context" as a
  reason to skip live driving.
- **Before writing "could not verify", ask yourself literally: "would a human's physical eyes be
  required here, or am I just trying to avoid spending my own time/turns?"** Only the first is a
  real reason.
- The only legitimate "cannot verify" cases: (a) a scenario that structurally requires a human to
  visually confirm something, or (b) a genuine environment gap (missing binary, no `claude` login —
  check this FIRST). Flag those specific cases rather than skipping silently.

TEARDOWN DISCIPLINE (critical): tear down every substrate server/session you start. At the end,
confirm ZERO stray substrate processes: `tasklist | findstr /i tmux` (must show nothing), plus
`lyx reed down` if you ran `lyx reed up` directly. Leave no stray state. Be honest about what you
could NOT verify and why.

## How to judge each finding

For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/
traced) vs PLAUSIBLE (looks wrong, unverified). For scope: pre-extraction vs post-extraction
behavior; flag any silent behavior change even if it looks like an improvement — it still needs to
be a deliberate, documented decision, not an accidental side effect of the refactor.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT. The only legitimate reason to leave a finding unfixed is that
fixing it genuinely requires something you cannot do alone this round (an operator design decision,
or a capability you don't have) — say so explicitly in the fixer report's deferred section.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)

- **O1** (out of scope, recorded not fixed): a burler reviewer twice produced YAML-invalid review
  frontmatter (a quoted-fragment summary followed by trailing prose), a fail-loud hard error that
  kills the whole perch block. 2 of ~20 live rounds hit this. It is burler's own review-template
  territory (SANDBOX-BURLER-SUITE/burler's crucible instance), explicitly out of scope for this
  campaign — re-confirm that judgment still holds; do not fix it here.
- **O3** (out of scope, recorded not fixed): the sandbox hub fixture repo ships a POSIX `reed.yaml`
  (`shell: bash`), unusable as-is on Windows (worked around via `LYX_REED_SHELL`). Fixture-repo
  content, not source in this tree — re-confirm still out of scope.
- **Residual smoke-harness "died" gap**: `go test -tags smoke ./internal/treadleengine/... -run
  Smoke` still dies in the fixture hub on fable-r1's machine (`outcome=died`), and fable-r1 showed
  `internal/burlerengine`'s own untouched smoke test fails IDENTICALLY on the same machine — a
  pre-existing, machine-wide substrate gap, not a treadle regression. If your environment can run
  the live smoke tests, try it yourself and either corroborate this (same identical burler-and-treadle
  failure) or, if it actually reveals something ONLY treadle hits, that would be a real finding —
  don't just take the prior round's word for the "identical failure" claim if you can cheaply check it.
- **O4** (not a finding): `perchcli` has no smoke test of its own. Per the brief's rule, add one only
  if you find a live-only defect that's genuinely perchcli's own; do not add one just to have coverage.

## Fixing — after the review

- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build`/
  `mill:golang-testing`/`mill:golang-comments` before editing. Prefer surgical edits; match existing
  style and the file-level doc-comment convention. If the actual defect is in `internal/treadleengine`
  rather than `internal/perchengine`, fix it in the module it actually lives in — don't work around
  a treadle bug from inside perch's adapter.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add
  a `//go:build smoke` test in whichever package (`perchcli` or `treadleengine`) actually owns the
  behavior.
- MAKE SMOKE TESTS DETERMINISTIC. Wait on the actual state transition (poll with a deadline), never
  sleep a fixed amount. Prove determinism by running the new test many times in parallel under load.
- If `SANDBOX-PERCH-SUITE.md` needs a new scenario to cover something this review surfaced, extend
  it (keep `sandbox_coverage_test.go` green). If none is warranted, note it in your fixer report
  instead.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy-dev.cmd`) and
  re-run every live scenario yourself, directly.
- Update `internal/perchengine/doc.go` and/or `internal/treadleengine/doc.go` (and `docs/overview.md`
  / `CONSTRAINTS.md` if invariants move) IN THE SAME change as any fix that changes documented
  behavior. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it —
  do NOT push unless the user explicitly asks. Report the changed files and how you verified each
  fix.

## Deliverables

1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment pre-extraction-vs-post-extraction; Code findings severity-ranked with file:line +
   scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact
   commands + observed results, including what you could NOT verify and why). Write it to
   `.scratch/perch-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/perch-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate),
produce your independent findings, then implement and verify the fixes.
