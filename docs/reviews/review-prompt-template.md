# `<MODULE>` — independent review + fix (prompt template)

> **This is a TEMPLATE.** Copy it to `docs/reviews/<module>-review-prompt.md` and replace every
> `<PLACEHOLDER>`. It is the round agent's *entire* instruction set — the orchestrator spawns a
> fresh clean-room agent told only "read this file and do exactly what it says". See
> [README.md](README.md) for the loop this prompt runs inside, and
> [`mux-review-prompt.md`](mux-review-prompt.md) for a fully-worked instance to crib from.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `<MODULE>`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`<WORKTREE_PATH>` (branch `<BRANCH>`). Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of `<MODULE>`'s scope and correctness. Hunt for bugs
   by reading the code AND by driving the real substrate (`<SUBSTRATE — e.g. real psmux>`) — this is
   where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do
   NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live
smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it —
on the current branch, no push — before starting the next finding. Commit message format:
`<module>: fix <finding-id> — <one-line what/why>` (e.g. `shuttle: fix M1 — assert redirected file
content, not Wait outcome, after interrupt+send`). Do not commit `.scratch/` (gitignored; your
review and fixer reports never belong in a commit regardless). This exists because a round agent's
session can be killed mid-fix by something entirely outside the method's control (a corrupted
terminal, a lost connection) — round 2 of shuttle's own loop hit exactly this. A single monolithic
uncommitted diff left behind by a crash forces the orchestrator to reverse-engineer, finding by
finding, which fixes are actually complete versus half-done, from the diff alone. A trail of small
commits turns that same crash into something the orchestrator can just read: `git log` shows
exactly which findings landed clean, and anything with no commit is unambiguously not done yet —
no guesswork, no risk of mistaking a half-applied fix for a finished one.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/<module>-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. A review written or finished after code has already changed is no longer an
independent judgment — it is a post-hoc rationalization of edits you already made, and it silently
destroys the one property this whole method depends on. If you catch yourself wanting to patch
something the moment you spot it: don't. Write it down as a finding, keep reading, finish the
review, save the file, THEN start Job 2. (This rule exists because a round agent interleaved review
and fix on shuttle's very first round — it had modified four production/test files before writing
a single line of its review report.)

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have
your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior reviews
`<module>-review-*.md` and `*-fixer-report.md`). Reading the design SPEC and the module docs is
expected and required (those are not reviews). AFTER you have written your own independent findings,
you MAY consult the prior rounds' `.scratch/<module>-review-*` material — regardless of which model
produced it (rounds rotate across Opus / Fable / Sonnet; the most recent prior round is whichever
`<module>-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to (a) confirm
previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code: `<CODE PATHS — e.g. internal/<module>engine/**, internal/<module>cli/**, cmd/lyx integration>`.
- Docs: `<MODULE DOC — docs/modules/<module>.md>`, `docs/overview.md`, `docs/roadmap.md`,
  `CONSTRAINTS.md`, `README.md`, and any `docs/research/<module>-*.md`.
- If one already exists, `<tools/sandbox/SANDBOX-<MODULE>-SUITE.md>` — for SCENARIO IDEAS only.
  You run every scenario yourself, directly, with your own tool calls; you do NOT invoke its
  `sandbox-<module>-suite.cmd` launcher (that spawns a SEPARATE, context-free interactive `claude`
  session for a human operator's own dogfooding — meaningless for you to spawn on top of yourself;
  see "Live driving" in "What to TEST" below). No such file needs to exist for you to do this
  module's live driving — the "High-yield focus" list above is your primary script.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry, CLI/Cobra, lyxtest Leaf, Sandbox Suite Coverage, Documentation Lifecycle). A change
  that ships behaviour without updating the module doc / invariants in the SAME change is incomplete.
- Design intent (SPEC, not a review): `<where the intended scope lives — e.g. _mill/discussion.md +
  _mill/plan/* recovered from git history at sha <SHA>>`. Use it as the authoritative source of
  intended v1 scope/behavior.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below. Also assess docs accuracy (do the docs match the code?) and operability.

## High-yield focus — where `<MODULE>`'s real bugs live (drive these, do not just read them)
The pure/unit-tested parts are usually solid; defects concentrate in the COMPOSED, LIVE behavior the
hermetic tests never exercise. Treat each as an INVARIANT you must actively verify by driving the
real substrate — a green `go test` proves nothing here. Fill in this list for THIS module; e.g.:
- `<INVARIANT 1 — a stateful edge case, its failure mode, and how to reproduce it live>`
- `<INVARIANT 2 — a crash/restart/rebirth path>`
- `<INVARIANT 3 — a concurrency / cross-instance / shared-resource scope boundary>`
- `<INVARIANT 4 — a mid-operation-failure orphan / reporting-honesty / env-hygiene invariant>`
(For a fully-worked example of this list, see the "High-yield focus" section of
[`mux-review-prompt.md`](mux-review-prompt.md).)

## Explicitly OUT of scope for `<MODULE>` v1
`<List anything whose ABSENCE is correct so the reviewer doesn't flag it — e.g. concerns that belong
to a neighboring module. State it plainly.>`

## Round context seeded from prior-round verification
`<The orchestrator rewrites THIS section each round.>` Either:
- **Residual to close:** `<the specific defect the last independent verification found, with the
  file/scenario, and an instruction to fix the right layer + add a regression test>`; or
- **Safety pass:** there is NO known residual — prior rounds CONVERGED and the last was independently
  verified clean. Do a genuinely independent clean-room pass to find anything every prior round
  missed, OR honestly confirm merge-readiness ("no new defects, ship it" is the expected, valuable
  outcome of a safety pass — do not invent work). Do NOT re-open the CLOSED-AND-VERIFIED work:
  `<bulleted list of closed items so they are not re-litigated>`.

State the **merge bar** so the reviewer calibrates: correctness in the NORMAL single-instance flow is
the gate; the N×-concurrent suite is a diagnostic amplifier, not a merge blocker.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet <MODULE PACKAGE PATHS>`
- `go test <MODULE PACKAGE PATHS> ./cmd/lyx/...` — stress timing/concurrency tests with `-count=5`.

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke <MODULE CLI PACKAGE> -run Smoke -v -count=1`
- `<substrate binary/tool locations + any absolute-path footgun>`.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the binary under test: `deploy.cmd`. **FOOTGUN:** live driving runs
  the DEPLOYED snapshot, not your working tree — re-run `deploy.cmd` after EVERY source change or
  you validate a stale binary. Deploy first, always.
- **Do NOT invoke `sandbox-<module>-suite.cmd`** (if one exists for this module). That launcher
  spawns a SEPARATE, context-free interactive `claude` session — a naive black-box tester with no
  source knowledge, meant for a human operator's own dogfooding, not for you to spawn on top of
  yourself. Instead, run the real CLI commands yourself, directly, foreground, waiting for each to
  return: walk the "High-yield focus" list above (and `<SANDBOX-<MODULE>-SUITE.md>`'s scenarios, if
  one exists, for extra ideas) and record OK/WARN/FAIL for each. This spawns real substrate
  underneath when the module rides mux/shuttle (real psmux panes, real `claude` sessions) — that is
  expected and required. None of it needs an attached TTY of its own.
- The suite/list is a FLOOR — devise and run MANY more adversarial scenarios of your own beyond it
  (combine verbs in orders nothing has tried; chase anything the code makes you suspicious of).
  Report exact commands + observations.
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real substrate
  session (a real implementer/agent doing real work) takes real wall-clock MINUTES, not seconds.
  That cost is EXPECTED and BUDGETED FOR, never a reason to skip a scenario. **You are explicitly
  forbidden from writing "operator-assisted", "cost-bearing", "long-running", "impractical", or
  "automated context" as a reason to skip live driving** — those words describe a cost to YOU,
  never a reason a human is required. Builder's first hardening round did exactly this (skipped
  its ENTIRE live suite citing those words) and it was a rationalization, not a real blocker: not
  one of that module's scenarios structurally needed a human.
- **Before writing "could not verify", ask yourself literally: "would a human's physical eyes be
  required here, or am I just trying to avoid spending my own time/turns?"** Only the first is a
  real reason. If a scenario just takes several minutes of you waiting on a real command to
  return, that is not a reason — wait for it, and report the actual output (with the commands you
  ran) as evidence, not a summary claim that you "verified" it.
- The only legitimate "cannot verify" cases are: (a) a scenario that structurally requires a human
  to visually confirm something, or (b) a genuine environment gap (a missing binary, no login —
  check for this FIRST, before anything else, so you know up front whether it applies). Flag those
  specific cases as not-headlessly-verifiable rather than skipping silently, and say exactly what
  blocked you.

TEARDOWN DISCIPLINE (critical): if you start any substrate server/session, tear it down. At the end,
confirm ZERO stray substrate processes (`<the exact check — e.g. tasklist | grep -i psmux>`). Leave
no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior),
severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs
PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped; flag
deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write down but
leave unfixed as "low priority" is not actually a reported finding; it is a dropped one that will
either silently vanish or re-surface and loop across future rounds instead of closing (this is a
known failure mode from an earlier review setup this method is descended from: NITs left unfixed
"because they're just NITs" kept escalating and going in circles instead of ever getting closed).
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires
something you cannot do alone this round — an operator decision on a real design tradeoff, or a
live capability you don't have (e.g. a second real TTY). Even then you must say so explicitly, with
the specific reason, in the fixer report's deferred section — never bucket something as "deferred,
low priority" just because it felt small. Small and low-severity findings are usually the CHEAPEST
to fix, not a reason to skip them.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
`<Bulleted list the orchestrator carries forward — consciously-deferred items to decide on each
round. Empty on the first round.>`

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT (see "How to judge each
  finding" above for the full rationale) — not just BLOCKING/MEDIUM ones.
- Load the code-quality guidance (`/code-quality` skill) AND the language-specific skill(s) for this
  codebase (e.g. `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` for a Go module —
  substitute the matching set for whatever language this module is written in) before editing — ALL
  of the relevant skills, not code-quality alone. (This rule exists because a round agent on
  shuttle's second round loaded code-quality only and skipped the language-specific skills when it
  reached this step; the operator caught it live and had to stop the round to redirect it.) Prefer
  surgical edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add
  a `//go:build smoke` test that walks the failing scenario against the real substrate (the existing
  smoke test file shows the pattern, incl. a skip when the substrate is absent). A hermetic unit test
  for the pure helper is good; a smoke test for the composed behavior is what protects the recovery
  paths.
- MAKE SMOKE TESTS DETERMINISTIC. Substrate operations are asynchronous; a test that assumes a verb
  is synchronous passes on a quiet machine and FLAKES on a loaded one. Wait on the actual state
  transition (poll with a deadline), never sleep a fixed amount. Prove determinism by running the new
  test many times in parallel under load, not once.
- If a maintained `<SANDBOX-<MODULE>-SUITE.md>` exists for this module, EXTEND IT when a review
  surfaces a live/visual behavior it doesn't cover (match the existing scenario shape; keep the
  coverage guard green in the SAME change). If none exists, note the new scenario in your fixer
  report instead — creating a brand-new suite file/launcher is not required by this method.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`deploy.cmd`) and re-run
  every live scenario yourself, directly — re-deploying FIRST is mandatory (live driving tests the
  deployed binary).
- Update `<docs/modules/<module>.md>` (and `docs/overview.md` / `CONSTRAINTS.md` if invariants or the
  module table move) IN THE SAME change. Do NOT add bugfix/hardening notes to `docs/roadmap.md`
  (roadmap is planned milestones only, per CLAUDE.md).
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it
  (see "Commit per fix" above) — do NOT push unless the user explicitly asks. Report the changed
  files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed
   results, including what you could NOT verify and why). Write it to `.scratch/<module>-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/<module>-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate),
produce your independent findings, then implement and verify the fixes.
