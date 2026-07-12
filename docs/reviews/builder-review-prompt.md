# builder — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `builder`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`C:\Code\loomyard\wts\internal-builder` (branch `internal-builder`). Adjust that path/branch if
the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of builder's scope and correctness. Hunt for bugs
   by reading the code AND by driving the real substrate — `lyx builder run`/`spawn-batch`/
   `poll`/`pause` wired to a REAL shuttle spawn (real psmux, a real logged-in `claude`) — this is
   where builder's defects hide, not in the hermetic unit tests.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against
   the real substrate, keep the whole test suite green, and update the docs in the same change as
   the fix they document. COMMIT after each individual fix lands green (see "Commit per fix"
   below). Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live
smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it —
on the current branch, no push — before starting the next finding. Commit message format:
`builder: fix <finding-id> — <one-line what/why>` (e.g. `builder: fix B3 — poll misclassifies a
paused batch as dead on the timeout branch`). Do not commit `.scratch/` (gitignored; your review
and fixer reports never belong in a commit regardless).

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/builder-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. Write it down as a finding, keep reading, finish the review, save the file, THEN
start Job 2.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have
your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior
reviews `builder-review-*.md` and `*-fixer-report.md`). Reading the design SPEC and the module docs
is expected and required (those are not reviews). AFTER you have written your own independent
findings, you MAY consult the prior rounds' `.scratch/builder-review-*` material — regardless of
which model produced it (rounds rotate across Opus / Fable; the most recent prior round is
whichever `builder-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to
(a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at
the bottom.

## What to read
- Code: `internal/builderengine/**`, `internal/buildercli/**`, and the `cmd/lyx` integration
  (`main.go`, sandbox/help/registration guard tests).
- Docs: `docs/modules/builder.md` (the as-built contract this doc pins — digest fields, poll's
  four-branch terminal classification, chain rollback, pause discipline, outcome contract +
  archiving, the three weft-commit points, the co-versioning rule between the orchestrator/
  implementer templates and their Go parsers), `docs/modules/plan-format.md` (builder's pinned
  input contract), `docs/overview.md`, `docs/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- The dedicated live-driving suite you will RUN: `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
  (scenarios B1–B5) plus [`docs/sandbox-howto.md`](../sandbox-howto.md) for how the harness works.
  This suite is brand-new (this is the first hardening round for builder) — treat it as a FLOOR,
  not a spec: extend it the moment you find a live behavior it doesn't cover.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry Invariant, CLI/Cobra Invariant, lyxtest Leaf Invariant, Sandbox Suite Coverage,
  Documentation Lifecycle). A change that ships behaviour without updating the module doc /
  invariants in the SAME change is incomplete.
- Design intent (SPEC, not a review): the 8-batch build plan that produced this module has already
  landed and its `_mill/` task state was cleaned up on merge. Treat `docs/modules/builder.md` and
  `docs/modules/plan-format.md` as the authoritative as-built contract; if you need the original
  design rationale, `git log --oneline --all -- '**/builder*'` and the PR history for
  `internal-builder` are your recovery path.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what
  `docs/modules/builder.md` promises? Gaps, over-reach, silently-dropped requirements,
  deferred-that-should-ship-in-v1. In particular: is "holistic review is perch's job, not
  builder's" actually honored (builder must never itself perform or fake a terminal review)?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below (there is no "historical" history yet — this is round 1 — so read this as
   "structurally fragile by design", the seams most likely to hide a live-only bug). Also assess
   docs accuracy (do the docs match the code?) and operability.

## High-yield focus — where builder's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (fingerprint hashing, config parsing, outcome YAML decode, role
grammar) are usually solid; defects concentrate in the COMPOSED, LIVE, timing-sensitive behavior
the hermetic tests never exercise — everything downstream of "no one holds the shuttle `Run`
handle across a batch's lifetime, so `poll` re-derives state from files and a live mux query on
every tick." Treat each of these as an INVARIANT you must actively verify by driving the real
substrate — a green `go test` proves nothing here:

- **`poll`'s four-branch terminal race.** The decision order is report-present → Stop-event
  (`dead_reason: asking`) → elapsed-timeout (`dead_reason: timeout`) → mux-strand-gone
  (`dead_reason: died`) → running. Verify each branch actually fires on the real condition it
  claims to detect, and that a report written a moment before a `poll` tick always wins over a
  simultaneously-true Stop/timeout/died condition (report-present must be checked first, for
  real, not just in the doc's prose). A misclassified `dead` when the implementer is in fact still
  quietly working (or vice versa) is exactly the false-positive/false-negative pair this method
  exists to catch.
- **`run.lock` contention.** Start a real `lyx builder run` in one shell; while it holds the lock,
  start a second `lyx builder run` (or `spawn-batch`) against the same worktree from another shell.
  Verify the loser fails fast with `ErrRunBusy` and — critically — that it skips its own
  exit-time weft-commit backstop (the doc's claim: "the losing call touched nothing on disk, so
  `run` skips its own exit-time weft-commit entirely rather than committing the winner's in-flight
  partial state under a misleading label"). Confirm the winner's `state.json` is never corrupted
  or double-written by the loser.
- **Pause is a boundary check, not an interrupt.** `pause` while a batch is in flight must let that
  batch finish normally — verify a real in-flight implementer is NOT killed, and that the NEXT
  `spawn-batch` (batch boundary) is the one that actually refuses with `{"paused": true}`. Verify
  `ClearPause` fires at `run`'s own entry (a resumed run never instantly re-pauses on its own old
  flag) and at every non-`paused` terminal outcome (a pause request that lost the race against the
  last batch settling never lingers).
- **Fingerprint mismatch + `--fresh` is archive-never-refuse.** Mutate a plan `*.md` file after a
  run has partially progressed, then run `lyx builder run` (no `--fresh`): verify the hard refusal
  names both fingerprints and points at `--fresh`. Then run with `--fresh`: verify `state.json`
  and the whole reports dir are archived (never deleted) with a timestamp suffix, a same-second
  collision gets a numeric `-1`/`-2` suffix rather than clobbering, and the run re-inits cleanly
  with a fresh `RunGUID`.
- **Outcome staleness is archive-never-refuse too.** Same discipline as above, but for
  `outcome.yaml` at every `run` entry (`ArchiveStaleOutcome`) — verify a pre-existing outcome file
  from a prior terminal run is renamed with a UTC-compact timestamp (not overwritten, not deleted)
  before the new run's own outcome is ever written.
- **Chain rollback (`--restart-chain`) is destructive by design — verify it targets the RIGHT
  sha.** A deferred-verify chain's intermediates commit non-green code; `spawn-batch <NN>
  --restart-chain` hard-resets the host repo to the chain's recorded start SHA. Verify: the
  recorded start SHA is the host `HEAD` immediately before the chain's FIRST member's first card
  commit (never overwritten by a later member's own spawn); every chain member's stale
  batch-report is deleted and its in-memory `BatchState` cleared; `state.CurrentBatch` resets to
  the chain's lowest member; and — the dangerous case — an UNRECORDED chain (no entry in
  `state.json`'s `ChainStartSHAs`) must refuse rather than reset to a hallucinated/zero SHA.
- **The three weft-commit points survive a mid-run crash.** Kill (or let time out) an in-flight
  batch between the `spawn-batch` commit and the `poll` terminal commit; verify resuming `lyx
  builder run` picks the batch up from exactly the recorded start-SHA and `BatchState`, with no
  double-spawn and no lost progress. Verify `run`'s own exit-time backstop commit fires on BOTH a
  successful and an erroring exit (but not on `ErrRunBusy`, per the `run.lock` invariant above).
- **Role resolution fails loud, never hours in.** Configure a well-formed-but-unknown role alias
  (e.g. an `implementer_oversized` model-spec pointing at a registry entry that doesn't exist);
  verify `ResolveRoles` fails BOTH `run` and `spawn-batch` at entry, before any agent spawns —
  never mid-batch when that role first needs to fire.
- **Co-versioning: template ↔ parser drift is silent, not a compile error.** The orchestrator and
  implementer prompt templates (`orchestrator-template.md`, `implementer-template.md`) are
  `//go:embed`'d and filled via `internal/stencil`; each is half of a Go-parsed contract (digest
  fields `poll` emits, the batch-report schema `Distill` parses, the outcome schema `ParseOutcome`
  decodes). Deliberately hand-edit one side (e.g. rename a batch-report field the template tells
  the implementer to emit) and confirm the property tests in `template_test.go` actually catch it
  — a drift here breaks silently in production (a prompt keying off a field Go no longer produces)
  and must not be able to slip past CI unnoticed.

## Explicitly OUT of scope for builder v1
- **Holistic/terminal review of the plan's output.** That is perch's job (`internal/perchengine`),
  driven separately by `loom` or an operator running `lyx perch run` directly, AFTER `builder run`
  returns `done`. Its absence from builder's own loop is correct — do not flag it as a gap. Do
  flag it if you find builder's code path secretly performing or faking any part of that review
  itself.
- **`loom`'s phase-machine wiring.** `loom` (not yet built) will drive `builder run` as one phase,
  gated by `perch` on either side. builder must not itself contain any loom-specific orchestration.
- **Mill's DAG-based intra-plan parallelism.** Already deliberately dropped at the plan-format
  level (see `docs/modules/plan-format.md`) — builder's strictly-sequential batch loop is correct,
  not a missing feature.
- **Non-Claude engines.** Per `CLAUDE.md`, non-Claude LLM support is not a current priority; don't
  flag the absence of a Gemini/other-provider path.

## Round context seeded from prior-round verification
There is NO known open residual — this is round 1, the first hardening pass builder has ever had.
Do a genuinely independent clean-room pass: read the code, drive the real substrate against every
"High-yield focus" invariant above, and produce your own findings before consulting anything else.
An honest "no defects, the design holds up" is a legitimate (if surprising, for a round 1) outcome
— but given this is the FIRST pass, expect to find real issues; do not under-report out of a sense
that the module "should" already be clean.

State the merge bar so you calibrate: correctness in the NORMAL single-instance flow (one `lyx
builder run` at a time, no artificial concurrency stress) is the gate. If you run N× concurrent
`lyx builder run` invocations against the SAME worktree as a diagnostic amplifier (beyond the
single deliberate `run.lock`-contention scenario above, which is itself a normal-flow correctness
requirement, not a stress test), a timeout or lock contention under that artificial peg is not
itself a defect — but any state corruption, double-spawn, or silent data loss IS, regardless of
how much concurrency it took to surface it.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/builderengine/... ./internal/buildercli/...`
- `go test -count=5 ./internal/builderengine/... ./internal/buildercli/... ./cmd/lyx/...`

Live driving via the SANDBOX SUITE (PRIMARY — where the bugs surface):
- Deploy the current source as the binary under test: `deploy.cmd`. **FOOTGUN:** the suite runs
  the DEPLOYED snapshot, not your working tree — re-run `deploy.cmd` after EVERY source change or
  you validate a stale binary. Deploy first, always.
- **Do NOT invoke `sandbox-builder-suite.cmd`.** That launcher's job is to spawn a SEPARATE,
  context-free interactive `claude` session (a naive black-box tester with no knowledge of the
  source) attached to a real console — meaningless for you to invoke, since you (the round agent)
  have no real attached console of your own to hand it, and you already have full source
  knowledge, so a second blind reviewer duplicating your own work end-to-end adds nothing. Instead,
  treat `SANDBOX-BUILDER-SUITE.md`'s scenarios (B1–B5) as a checklist YOU execute directly, with
  your own tool calls: materialize the Hub yourself (`sandbox-build.cmd`, then `lyx init` in the
  host repo), then run the real `lyx builder run` / `spawn-batch` / `poll` / `pause` commands the
  scenarios describe, foreground, waiting for each to return. This DOES spawn real psmux panes and
  real interactive `claude` sessions underneath (as builder's own substrate, via shuttle) — that is
  expected and required, not something to avoid. It needs no attached TTY of ITS OWN: a psmux pane
  is a real pty regardless of whether anyone is watching it, so `lyx builder run` blocking in your
  own foreground Bash call is a normal, fully headless-capable action for you to take, not an
  operator-assisted one.
- Budget real wall-clock time for this: a real implementer session doing real work (even a
  trivial one-card batch) takes minutes, not seconds. That cost is not a reason to skip B1–B5 and
  fall back to pure code-tracing — round 1 did exactly that ("operator-assisted / long-running /
  cost-bearing... impractical in this automated context") and as a result NONE of B1–B5 were
  actually exercised live. Reserve "operator-assisted, not headlessly verifiable" strictly for
  something that genuinely needs a human eyeball (e.g. a visual `lyx mux attach` confirmation) —
  none of B1–B5 need that; they are all observable via `lyx builder status`/`poll`'s JSON output,
  `lyx mux status`, and `psmux list-panes`. If you still cannot complete a scenario, say exactly
  what blocked you (a real environment gap, not merely "this costs agent turns/time").
- Walk every scenario (B1–B5) this way and record OK/WARN/FAIL. The suite is a FLOOR — devise and
  run MORE adversarial scenarios of your own beyond it, especially combinations the suite doesn't
  try (e.g. pause racing a batch that is *just about* to write its report; a chain restart while a
  sibling batch's implementer is still technically live from a stale strand).
- The only legitimate "cannot verify" cases are: (a) a scenario that structurally requires a human
  to visually confirm something (there are none in B1-B5 today — flag it if you add one that does),
  or (b) a genuine environment gap (`claude` not logged in, `psmux.exe` missing). Flag those as
  not-headlessly-verifiable with the specific missing precondition — never as a blanket
  cost/time/turn-budget excuse.

TEARDOWN DISCIPLINE (critical): if you start any substrate server/session (builder's own
orchestrator spawn, or any batch implementer spawn, both ride real psmux via shuttle), tear it
down. At the end, confirm ZERO stray substrate processes: `tasklist | grep -i psmux` must report
zero. Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/
traced) vs PLAUSIBLE (looks wrong, unverified). For scope: doc-promised vs shipped; flag
deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write down but
leave unfixed as "low priority" is not actually a reported finding; it is a dropped one. The only
legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you
cannot do alone this round (an operator decision on a real design tradeoff, or a live capability
you don't have). Even then say so explicitly in the fixer report's deferred section.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None yet — this is round 1.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build`/
  `mill:golang-testing`/`mill:golang-comments` before editing — ALL of the relevant skills, not
  code-quality alone. Prefer surgical edits; match existing style and the file-level doc-comment
  convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect,
  add a `//go:build smoke` test (following the pattern in other modules' `*_smoke_test.go` files,
  incl. a skip when the substrate is absent) that walks the failing scenario against the real
  substrate. A hermetic unit test for the pure helper is good; a smoke test for the composed
  behavior is what protects the recovery paths.
- MAKE SMOKE TESTS DETERMINISTIC. Substrate operations are asynchronous; wait on the actual state
  transition (poll with a deadline), never sleep a fixed amount. Prove determinism by running the
  new test many times in parallel under load, not once.
- EXTEND `SANDBOX-BUILDER-SUITE.md` when your review surfaces a live/visual behavior it doesn't
  cover (match the existing scenario shape; keep `sandbox_coverage_test.go` green in the SAME
  change — it only requires a `**Covers:** builder` tag somewhere, but keep the scenario roster
  honest).
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy.cmd`) and re-run
  the suite scenarios — re-deploying FIRST is mandatory.
- Update `docs/modules/builder.md` (and `docs/overview.md` / `CONSTRAINTS.md` if invariants or the
  module table move) IN THE SAME change. Do NOT add bugfix/hardening notes to `docs/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it —
  do NOT push unless the user explicitly asks. Report the changed files and how you verified each
  fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment doc-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed
   results, including what you could NOT verify and why). Write it to
   `.scratch/builder-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/builder-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the doc + code, then drive the real substrate), produce
your independent findings, then implement and verify the fixes.
