# loom Planner producer — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the loom **Planner
producer** in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`/home/knatte/Code/loomyard/wts/loom-planner` (branch `loom-planner`). This is a **Linux** host —
the repo's `.cmd`/`tasklist` tooling elsewhere is Windows-oriented; the Linux equivalents are given
below. Adjust the path/branch if the task lives elsewhere now.

## Scope — this is a SMALL, mostly-pure module, size your effort accordingly
The Planner producer is NOT a CLI verb, NOT a phase machine, and does not itself spawn or drive
anything. It is a **prompt/profile factory**: `PlanSpec(...)` (`internal/loomengine/plan.go`) is a
pure composer that resolves a model-spec, names two output paths, fills `plan-template.md` via
`internal/stencil`, and returns a `shuttleengine.Spec` — nothing has been spawned yet when it
returns. There is no `lyx loom plan` command wired up yet (loom's phase machine is unbuilt); the
only way this Spec gets driven today is if you drive it yourself (see "Live driving" below). Do NOT
invent scope-creep findings about missing CLI wiring, missing phase-machine gating, or missing
review/approval logic — those are explicitly out of scope (see below). Your review effort should be
proportional to the module's real size: get the composer, the template's instructions-to-a-real-
agent, and the two new `hubgeometry` path accessors right, rather than manufacturing padding.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of the Planner producer's scope and correctness. Read
   the code, AND drive the real substrate — actually get a real `claude` agent to execute
   `plan-template.md`'s instructions end to end (see "Live driving" below) — this is where a
   prompt-template's real defects hide (ambiguous instructions, an agent misreading the format spec,
   a marker substitution that looks right in a unit test string-contains check but reads wrong to an
   LLM), not in the hermetic unit tests alone.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each (hermetic +,
   where relevant, a re-run of the live prompt), keep the whole test suite green, and update the docs
   in the same change as the fix they document. COMMIT after each individual fix lands green (see
   "Commit per fix" below). Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus a live
re-run if the finding needed one), and its doc update (if any) is included, COMMIT it — on the
current branch, no push — before starting the next finding. Commit message format:
`loom-planner: fix <finding-id> — <one-line what/why>` (e.g. `loom-planner: fix P3 — plan-template.md's
Step 3 ordering let the agent write 00-overview.md before every card file existed`). Do not commit
`.scratch/` (gitignored; your review and fixer reports never belong in a commit regardless). This
exists because a round agent's session can be killed mid-fix by something entirely outside the
method's control (a corrupted terminal, a lost connection). A single monolithic uncommitted diff
left behind by a crash forces the orchestrator to reverse-engineer, finding by finding, which fixes
are actually complete; a trail of small commits turns that same crash into something the
orchestrator can just read from `git log`.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/loom-planner-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. A review written or finished after code has already changed is no longer an
independent judgment — it is a post-hoc rationalization of edits you already made. If you catch
yourself wanting to patch something the moment you spot it: don't. Write it down as a finding, keep
reading, finish the review, save the file, THEN start Job 2.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have
your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior reviews
`loom-planner-review-*.md` and `*-fixer-report.md`). Reading the design SPEC and the module docs is
expected and required (those are not reviews). AFTER you have written your own independent findings,
you MAY consult the prior rounds' `.scratch/loom-planner-review-*` material — regardless of which
model produced it (rounds rotate across Fable / Opus; the most recent prior round is whichever
`loom-planner-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to (a) confirm
previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code (the module under review):
  - `internal/loomengine/plan.go` — `composePlanPrompt` + `PlanSpec`.
  - `internal/loomengine/plan-template.md` — the actual prompt handed to the real agent (this is
    the highest-value artifact to scrutinize: it is instructions to an LLM, not to Go).
  - `internal/loomengine/plantemplate.go` — the `//go:embed`.
  - `internal/loomengine/config.go`, `configtemplate.go`, `template.yaml` — the `plan` /
    `plan_timeout_min` knobs added alongside the pre-existing `discussion` ones.
  - `internal/hubgeometry/hubgeometry.go` — the new `(*Layout).PlanDir()` / `(*Layout).PlanOverview()`
    accessors (note: there is also a pre-existing free function `PlanDir(baseDir string)` used by
    builder's older plan-format v1 — read both and make sure the method vs. free-function split is
    not confused/aliased incorrectly).
  - `internal/loomengine/discussion.go` + `discussion-template.md` — the SIBLING producer this one
    was modeled on; useful for spotting where Planner's version diverged from an established,
    already-reviewed pattern without a documented reason.
  - Tests: `internal/loomengine/plan_test.go`, `internal/loomengine/config_test.go`,
    `internal/hubgeometry/planpath_test.go`.
- Docs (SPEC — authoritative, not a review):
  - `docs/reference/plan-format-v3.md` — the compact flat-card plan format `plan-template.md`
    instructs the agent to produce. This is the pinned CONTRACT: every claim in the template about
    frontmatter, field order, `Moves:`/rename-mechanic, `Depends-on:`, `Commit:`, `verify:` etc. must
    match this doc exactly, and whatever a real agent actually produces (see live driving) must match
    it too.
  - `manifest/designs/loom.md` — the Planner producer's design intent (the `producers
    (discussion / plan)` row of the module table, and the surrounding phase-machine context that
    explains why no CLI/gating exists yet).
  - `docs/overview.md` — the loom module-table entry (recently updated by this same change; verify
    it accurately describes what was actually built).
  - `internal/stencil/stencil.go` — `Fill`'s exact semantics (flat top-level `{{.X}}` substitution,
    no `{{if}}`/`{{range}}` support, `unfilledTopLevelMarkers` used to fail loud on a blank
    required marker). `plan-template.md`'s own header comment claims "no conditionals anywhere in
    this file" — verify that claim is actually true by reading the file end to end, not just trusting
    the comment.
  - `internal/shuttleengine`'s `Spec` type and its `validate` (specifically what `OutputFiles`
    pre-existence rejection means for `PlanSpec`'s contract that it "does not stat or create any
    file").
  - `docs/reference/model-spec.md`'s "Roles that use this notation" section (for the `plan` role
    model-spec grammar).
  - `CONSTRAINTS.md` (root) — in particular the **Hub Geometry Invariant**: only `internal/hubgeometry`
    may construct `_lyx`/plan paths; verify `plan.go` obtains every path via `layout.PlanDir()` /
    `layout.PlanOverview()` / `layout.DiscussionDecisionRecord()` and never hand-builds one.
  - `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) — repo rules; a change that ships behaviour without
    updating the module doc / invariants in the SAME change is incomplete.
- Design intent / history: this task's own `_mill/discussion.md` and `_mill/plan/` were cleaned up
  on merge per this repo's convention; `git log --oneline --all -- '**/plan.go' '**/plan-template.md'
  '**/plantemplate.go'` and this branch's own commit history (`git log --oneline main..loom-planner`)
  are your recovery path for the original design rationale if you need it.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — does the as-built composer + template deliver exactly what
   `docs/reference/plan-format-v3.md` and `manifest/designs/loom.md` promise, no more, no less?
   Gaps, silently-dropped requirements, or scope the Planner producer should explicitly NOT own (see
   "Explicitly OUT of scope" below) leaking in.
2. Correctness — bugs, edge cases, and — because this module's real product is an LLM prompt, not
   just Go code — INSTRUCTION QUALITY: does a real agent following `plan-template.md` verbatim
   actually produce a valid plan-format-v3 output, reliably, without misreading an ambiguous
   instruction? Also assess docs accuracy (do the docs match the code?) and the two new
   `hubgeometry` accessors' correctness/consistency with the pre-existing `DiscussionDecisionRecord`
   pattern.

## High-yield focus — where this module's real bugs live
The Go plumbing (`PlanSpec`'s field mapping, error wrapping, config parsing) is the kind of thing a
hermetic unit test already catches well — `plan_test.go` covers OutputFiles/Interactive/Role/Model/
Effort/Timeout/Prompt-non-empty and a "no leftover `{{`" check. Defects are more likely to concentrate
in these seams instead:

- **The template as instructions to an LLM, not to Go.** `stencil.Fill`'s "no leftover `{{`" test
  proves every marker got a VALUE — it says nothing about whether that value is well-formed for the
  agent to actually use (e.g., is `{{.decision_record_path}}` always an absolute path an agent's
  `Read` tool can consume directly, or could a relative path from a caller resolve against the wrong
  cwd?). Drive the real prompt (see Live driving) and read what the agent actually does with each
  path, not just that the string appears somewhere in the prompt.
- **Step 3/Step 4 ordering — the done-sentinel contract.** `00-overview.md`'s existence is documented
  as "the sole signal that the plan is complete", and Step 4 says write it LAST, "only after every
  `NN-<card-slug>.md` card file already exists on disk". `OutputFiles: []string{overviewPath}` in
  `PlanSpec` is what `shuttleengine.Run` polls for "done". Verify a real agent actually honors "write
  it last" under realistic conditions (e.g., a multi-card plan where it might be tempted to write the
  overview first as a TODO-list scaffold) — a violation here means shuttle reports "done" while card
  files are still missing.
- **`approved: false` is a MUST, not a SHOULD.** The template says "Always write `approved: false` —
  you never self-approve." Verify a real agent never flips this, and that nothing in the template's
  wording could be read as conditional ("unless the plan is trivial", etc. — it should not read that
  way, but verify).
- **The `AskUserQuestion` prohibition is load-bearing for autonomy.** `PlanSpec` never surfaces a
  slug/autonomous switch the way `DiscussionSpec` does — plan is ALWAYS autonomous (no interactive
  mode exists for it at all). If the decision record is ambiguous or missing scope, does the agent
  correctly make a best-judgment call and proceed (per Step 1's "STOP and report" instruction for a
  missing/empty file, vs. best-judgment for merely-ambiguous-but-present content), or does it stall /
  hallucinate scope? Drive both a well-formed and a deliberately sparse/ambiguous decision record.
- **Missing/empty decision record handling.** Step 1: "If the file is missing or empty, STOP and
  report that rather than inventing scope." Verify this actually happens live (missing file case AND
  empty-but-present file case are two different scenarios — test both) rather than the agent
  fabricating a plan from nothing.
- **`PlanDir` method vs. the pre-existing free function — no accidental aliasing/shadowing.** The new
  `(*Layout).PlanDir()` method delegates to the OLD free `PlanDir(baseDir)` function (originally
  authored for builder's plan-format v1, now reused/shared for v3 per the updated doc comment).
  Verify: (a) the method actually anchors on `WorktreeRoot`, not `Cwd` (per its own doc comment's
  claim, mirroring `DiscussionDir`'s rationale) — construct a `Layout` with `Cwd != WorktreeRoot` and
  confirm; (b) nothing in builder's own code path (`internal/builderengine`) that already calls the
  free `PlanDir` function got silently redirected or broken by this change; (c) the doc-comment
  claim that this is now genuinely "shared across plan-format versions" is actually still accurate —
  i.e., builder's v1 consumer and loom's v3 consumer really do agree on `_lyx/plan` as the same
  physical directory, which is a real design decision worth flagging if it looks accidental rather
  than deliberate.
- **Config knob round-trip.** `plan` / `plan_timeout_min` added to `template.yaml`,
  `ConfigTemplate()`, `Config`, and `LoadConfig`'s validation, alongside the pre-existing `discussion`
  knobs. Verify `lyx init` / `lyx config reconcile` on a real throwaway hub actually materializes
  both new keys with the documented defaults, and that a hand-edited malformed `plan:` value in a
  real `loom.yaml` fails loud at `LoadConfig` time (not just in the hermetic
  `TestPlanSpec_MalformedModelSpec` unit test, which never touches a real YAML file on disk).
- **`Config.Discussion`/`Config.Plan` are independently validated — verify no cross-contamination.**
  A malformed `plan:` value must fail even when `discussion:` is well-formed and vice versa; check
  the actual error message names the RIGHT offending key (`"plan"` vs `"discussion"`) — a
  copy-paste-from-discussion bug here would name the wrong key.

## Explicitly OUT of scope for the Planner producer v1
- **The `lyx loom` command / phase machine.** Not built yet, by design — do not flag its absence.
  Driving `PlanSpec` end to end today requires YOU to do it directly (see Live driving); that is
  expected, not a gap in the module.
- **Review/approval of the produced plan.** `00-overview.md`'s `approved: false` is flipped to `true`
  by a future review gate (perch/burler), not by this producer. Do not flag "the plan is never
  approved" as a defect — that is the intended contract.
- **Reading the support log or the board.** The package doc comment is explicit: the decision record
  is the SOLE input. Do not flag "Planner doesn't consult the board" as a gap.
- **Stat-ing or creating `_lyx/plan/` from Go.** `PlanSpec` is a pure composer; creating the
  directory is the spawned agent's own write concern (`plan-template.md` Step 3), and
  `shuttleengine.Spec.validate` rejecting a pre-existing output file is the existing, already-hardened
  contract this producer merely participates in — do not re-litigate `shuttleengine`'s own
  correctness here (that module has its own hardening lineage; scope creep into `shuttleengine`
  internals is out of bounds for this round unless you find `PlanSpec` genuinely misusing its
  contract).
- **Non-Claude engines.** Per `CLAUDE.md`, non-Claude LLM support is not a current priority.
- **Builder's plan-format v1.** `internal/builderengine`'s existing consumer of the free `PlanDir`
  function is pre-existing, separately-hardened code; only touch it if this change actually broke
  something there.

## Round context seeded from prior-round verification
You are round tag `opus-r4` — round 4 of 4, the LAST round of this campaign (the cap is fixed at 4,
no further rounds after this one regardless of what you find), alternating Fable/Opus. Rounds 1
(`fable-r1`), 2 (`opus-r2`), and 3 (`fable-r3`) all ran, each found real CONFIRMED-live findings, each
was independently re-verified by the orchestrator from a cold state (hermetic gates green,
revert-and-confirm-fail proofs reproduced for every finding pinned by a string-content test,
teardown/report claims spot-checked against actual on-disk/process state). Do NOT re-open this
CLOSED-AND-VERIFIED work — it holds:

- **T1 (MEDIUM, confirmed live, two iterations)** — `plan-template.md` never said what a card is; a
  live run produced a new-behavior card with no bundled test. Fixed `2eda08cb` with a "What a card
  is" subsection (builds-on-its-own / independently-committable / bundles-its-own-test). A live
  re-drive (Run F) showed the first wording still insufficient (card still had `Creates: none`, no
  test) — commit `b5fd12ca` made criterion 3 imperative and ruled out `verify:` as a substitute. A
  SECOND live re-drive (Run G, independently reproduced by the orchestrator by reading the throwaway
  hub's actual on-disk plan output before tearing it down) confirms the fix holds: the produced card
  now has `Creates: cmd/notes/main_test.go` and extracts a testable `run()` helper. Pinned by
  `TestPlanSpec_PromptStatesCardCriteria`; orchestrator reproduced the revert-and-confirm-fail proof
  (removing the section makes the test fail on all 5 assertions; restored, diff empty).
- **T2 (MEDIUM)** — template advertised frontmatter `root:` with no resolution rules. Fixed
  `44cb41b4`. Pinned by `TestPlanSpec_PromptStatesRootResolution`; orchestrator reproduced the
  revert-and-confirm-fail proof (removing the paragraph fails all 3 assertions; restored, diff
  empty).
- **T3 (LOW)** — `Context:` semantics + per-card field-exclusivity rule undocumented. Fixed
  `932aa9d3`, pinned by `TestPlanSpec_PromptStatesContextSemantics`.
- **T4 (LOW)** — move-redundant rule + rename-plus-extraction shape undocumented. Fixed `db6bc0a2`,
  pinned by `TestPlanSpec_PromptStatesMoveRedundantRule`.
- **T5 (NIT, confirmed live)** — `verify:` values not pinned to runnable commands; a live run wrote a
  prose `## verify:` section. Fixed `acbdb5ff`, pinned by `TestPlanSpec_PromptStatesVerifyIsRunnable`;
  Run F confirms the fix (produced plan's `verify:` is now pure runnable fenced commands).
- **H1 (LOW)** — `PlanDir`/`PlanOverview` doc comments used stale v1 batch vocabulary and
  misdescribed the overview as the "sole output artifact" (it is the sole `Spec.OutputFiles` entry —
  the agent also writes card files, which are NOT in `OutputFiles`). Fixed `3113aa47`.
- **C1 (NIT)** — `manifest/designs/loom.md`'s producer row dangled `approved: false` onto the card
  files rather than the overview frontmatter. Fixed `610b6cf9`.
- **P1 (LOW, confirmed live, intermittent)** — round 2 found that a rename-plus-extraction card could
  intermittently double-declare the moved file in both `Edits:` and as its `Moves:` destination (a
  `card-field-overlap` violation per `docs/reference/plan-format-v3.md` check 10) — the exclusivity
  rule (T3, round 1) and the Rename mechanic's "make surgical edits to the moved file" language were
  never reconciled. Reproduced live in one drive (`memstore.go` in both fields), absent in a second
  identical drive — genuinely intermittent, the signature of borderline-but-not-quite-sufficient
  wording. Fixed `80c6ef86`: the Rename mechanic now states the moved file's surgical edits are
  already covered by its `Moves:` entry and must never also appear in `Edits:`. Pinned by
  `TestPlanSpec_PromptStatesMovedFileNotInEdits`; orchestrator reproduced the revert-and-confirm-fail
  proof (removing the added sentences fails all 3 assertions; restored, diff empty) and confirmed
  round 2's own two post-fix live re-drives both came back clean.
- **R1 (NIT)** — the orphaned `PlanOverview` doc-comment line wrap (seeded for round 2) was reflowed.
  Fixed `37412dc3`.
- **N1 (LOW, confirmed live)** — round 3 found that `plan-template.md` pinned `Depends-on:`'s grammar
  but never said WHEN an edge is required: a live docs-only run produced a card whose `Context:` named
  a file an earlier card `Creates:`, yet declared `Depends-on: none` — an under-declared DAG-of-intent
  edge no mechanical plan-format-v3 check can catch. Fixed `ace2639d`: a new paragraph states
  `Depends-on:` records intent (what depends on what, not just compile order), including
  non-compile-visible reliance such as a `Context:` reference to an earlier card's output. Pinned by
  `TestPlanSpec_PromptStatesDependsOnCriterion`; orchestrator reproduced the revert-and-confirm-fail
  proof (removing the paragraph fails all 3 assertions; restored, diff empty).

Process notes (not module defects, recorded so you don't need to rediscover them):
- Round 1's session was interrupted before finishing its final live re-drive and before tearing down
  its throwaway substrate; the orchestrator cleaned up a leaked tmux server + a stray untracked file.
  Round 2 finished and tore down cleanly on its own. The FIRST attempt at round 3 was interrupted much
  earlier — before writing any review report or making any commit at all — and left another stray tmux
  server plus stray scratch dirs (including some the orchestrator had missed after round 1); the
  orchestrator treated that attempt as void (nothing durable existed to build on) and respawned round 3
  fresh under the same tag. The respawned `fable-r3` completed cleanly: one real finding (N1), fixed,
  tested, live-reconfirmed, and it also swept up the leftover scratch debris from the earlier crashed
  attempts. Takeaway for you: sessions in this campaign have twice been interrupted mid-live-drive
  without warning — if that happens to you, whatever you've already COMMITTED is safe (that's the whole
  point of committing per fix), but a review or fix that exists only in your head or an unsaved report
  is not. Save the review report to disk (per the Sequencing rule) as early as truly possible, and don't
  let a single long live run be the only thing standing between "nothing recorded" and "some progress
  recorded" for an extended stretch.
- Round 2 independently re-verified round 1's `PlanDir`/builder-consumer non-regression concern (H2
  from round 1's own review, never a finding) and confirmed builder/webster's pre-existing free-function
  call sites are untouched. Round 3 re-confirmed P1 (the intermittent rename/Edits overlap) stays fixed
  on a fresh live re-drive, plus re-confirmed T1–T5, H1, C1 all still hold with no regressions. You do
  not need to re-verify any of this a fourth time unless you have a specific new reason to suspect it.

There is nothing further seeded as a residual — round 3 closed the one thing it found (N1), and the
orchestrator's own independent pass after round 3 found no additional defect.

This is the LAST round. Do a genuinely independent clean-room pass: read the code and docs yourself
(including all CLOSED-AND-VERIFIED commits above, to confirm no regression), then drive several live
runs of your own devising — both a re-drive of at least one scenario each prior round already covered
(to build real confidence nothing has silently regressed across the whole 8-fix history) and, if you
can think of one, a genuinely new scenario none of rounds 1–3 tried. Three consecutive real, live-driven
rounds each turned up at least one genuine defect (7, then 2, then 1 — a shrinking but still nonzero
trend) — that pattern is exactly why this campaign is not stopping at round 3, and it means you should
NOT assume the well is dry. At the same time, do not manufacture a finding just because rounds 1–3 each
had one: since this is hard-capped at 4 rounds with no round 5, your job is to find what's really
there, whether that's one more real defect or genuinely nothing. Give your own honest, explicit
merge-readiness opinion either way — the orchestrator will independently verify it exactly as it did
rounds 1–3, and that verification (not your self-report) is what actually decides convergence.

State the merge bar so you calibrate: this is a small, low-concurrency, single-shot producer — there
is no meaningful "N× concurrent" stress dimension the way a stateful CLI verb has (nothing here holds
a lock or mutates shared state across invocations). The merge bar is: hermetic tests green, the
`hubgeometry` Hub Geometry Invariant honored, and — the part that actually matters for THIS module —
a real agent given the composed prompt reliably produces a correct, spec-conformant plan-format-v3
output across multiple independently-driven live runs (not just one lucky pass). **Before you end your
session, actually finish and record the result of every live run you start, and actually tear down
your throwaway substrate (`lyx reed down`, kill the tmux server, delete the throwaway hub dir and any
throwaway test files) — do not leave a run's outcome unrecorded. If your session risks being cut short,
prioritize getting your review report SAVED TO DISK and any completed fix COMMITTED over starting one
more live run — partial, recorded progress beats an unrecorded one in flight.**

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

**Environment check FIRST.** This is a Linux host. Confirm up front: `tmux` on PATH, a logged-in
`claude` on PATH, `lyx` on PATH, `go` on PATH. (At authoring time all four were present:
`/usr/bin/tmux`, `/home/knatte/.local/bin/claude`, `/home/knatte/.local/bin/lyx`, `/usr/bin/go`.) If
any is genuinely missing, that is a real environment gap — flag it specifically and say what it
blocked; it is the ONLY legitimate "cannot verify headlessly" reason besides a scenario that
structurally needs a human's physical eyes (there is no such scenario in this module — everything
here is headlessly driveable).

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/loomengine/... ./internal/hubgeometry/...`
- `go test -count=5 ./internal/loomengine/... ./internal/hubgeometry/... ./cmd/lyx/...`

Live driving — YOU drive it directly (PRIMARY — where this module's real bugs surface):
There is no `lyx loom plan` CLI verb yet, so you must compose and drive the Spec yourself. Two valid
ways to get the rendered prompt text; use whichever is faster, but understand both:
1. **Rigorous:** temporarily add a throwaway `_test.go` inside `internal/loomengine` (same package,
   so it can call the unexported `composePlanPrompt`) that renders the prompt for real
   `decision_record_path`/`plan_dir`/`overview_path` values and writes it to a file under your
   scratchpad. Delete this throwaway test file before your final commit — never leave debug
   scaffolding in the tree.
2. **Fast approximation:** `plan-template.md`'s substitution is a flat, unconditional `{{.X}}` fill
   (verify this claim yourself from `internal/stencil/stencil.go` first) — you may reconstruct the
   same text by copying `plan-template.md` and hand-substituting the three markers with your
   throwaway hub's real absolute paths.
- **Materialize a throwaway test hub yourself** (there is no sandbox-build tooling for loom yet): make
  a temp dir OUTSIDE this worktree (e.g. under `/tmp` or the session scratchpad — never inside the
  repo, never a second git worktree of loomyard), `git init` it, `lyx init` it, `lyx reed up`. Write a
  real `_lyx/discussion/decision-record.md` fixture by hand (a short but genuine decision record: a
  task-framing paragraph plus a few concrete requirements — this is what a real Discussion producer
  run would have written; do not just write one line of Lorem Ipsum, since a too-thin fixture will
  not exercise the template's judgment calls realistically).
- Run: `lyx shuttle run --prompt-file <your rendered prompt> --output-file <planDir>/00-overview.md
  --timeout 30m` (leave `--model`/`--effort` empty to use the provider default — this module's own
  correctness does not depend on model-spec resolution, that is `modelspec`'s already-separate
  concern). This spawns a REAL tmux pane and a REAL autonomous `claude` session that must independently
  read your decision record, explore the throwaway hub's codebase, and write a real plan-format-v3
  plan to disk. Wait for it to return (foreground, no launcher needed — you already have full source
  knowledge; a tmux pane is a real pty regardless of whether anyone is watching it).
- After it returns `done`, **read the actual on-disk output** against `docs/reference/plan-format-v3.md`
  line by line: correct frontmatter, correct field order and REQUIRED-always-present file-op fields
  (`Context/Edits/Creates/Deletes/Moves`, each `none` or populated, never omitted), `Depends-on`
  referencing only earlier cards, `approved: false`, card numbering `1..M` with no gaps, filenames
  matching `NN-<card-slug>.md`. Do not just check that the run outcome was "done" — a shuttle "done"
  only proves the overview file exists, not that its CONTENT is spec-conformant.
- Run this at least twice independently (different decision-record fixtures — e.g. one single-card
  trivial task, one multi-card task that should trigger `Moves:`/the Rename mechanic) to see whether
  the template reliably produces conformant output or whether conformance was a one-off fluke.
  Deliberately try: a decision record naming a rename/move (verify the agent reproduces the "Rename
  mechanic" block verbatim AND actually does `git mv` first in its own exploration/reasoning, not a
  destructive rewrite+delete); a missing decision-record file (verify STOP-and-report, no plan
  written); an empty decision-record file (same); a decision record whose scope is deliberately vague
  (verify the agent makes a reasonable judgment call rather than stalling — remember `AskUserQuestion`
  is prohibited, so watch for anything resembling a stall/hedge instead of a call).
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real autonomous
  session doing real work (reading a decision record, exploring a codebase, writing several files)
  takes real wall-clock MINUTES, not seconds. That cost is EXPECTED and BUDGETED FOR, never a reason
  to skip a live run. **You are explicitly forbidden from writing "operator-assisted",
  "cost-bearing", "long-running", "impractical", or "automated context" as a reason to skip live
  driving** — those words describe a cost to YOU, never a reason a human is required. Every scenario
  in this module is fully headless-driveable; there is no legitimate reason to substitute
  code-tracing for at least two real live runs.

TEARDOWN DISCIPLINE (critical): tear down `lyx reed down` in your throwaway hub when done. At the end
confirm ZERO stray substrate: `pgrep -a tmux` shows no leftover server for your test hub. Leave no
stray state (including: delete the throwaway hub dir itself, and delete any throwaway `_test.go` you
added for prompt-rendering per option 1 above). Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/
traced live or in a hermetic test) vs PLAUSIBLE (looks wrong, unverified). For a template/instruction
finding, "reproduced" means you actually watched a real agent misbehave because of the ambiguous
wording — a template finding that is only "this wording looks like it COULD confuse an agent" without
having actually driven it is PLAUSIBLE, not CONFIRMED. For scope: doc-promised vs shipped; flag
deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write down but
leave unfixed as "low priority" is not actually a reported finding; it is a dropped one that will
either silently vanish or re-surface and loop across future rounds instead of ever closing. The only
legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you
cannot do alone this round (an operator decision on a real design tradeoff, or a live capability you
don't have). Even then say so explicitly, with the specific reason, in the fixer report's deferred
section — never bucket something as "deferred, low priority" just because it felt small.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
Nothing is currently on this list (round 1). If your own pass surfaces something you genuinely
cannot resolve alone this round, defer it here explicitly with the reason — do not silently drop a
finding.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build`/
  `mill:golang-testing`/`mill:golang-comments` before editing — ALL of the relevant skills, not
  code-quality alone. Prefer surgical edits; match existing style and the file-level doc-comment
  convention (this codebase's doc comments are unusually dense/precise — match that register, don't
  thin it out).
- For a Go-code bug, add or extend a hermetic unit test (following `plan_test.go`'s existing style)
  that would have caught it.
- For a template/instruction-quality bug (something only a real agent run exposed), there is no
  `//go:build smoke` pattern established for this module yet since there's no CLI verb to hang one
  off — instead: (a) fix the wording in `plan-template.md`, (b) if the fix is mechanically checkable
  (e.g. "marker X must appear before marker Y in the rendered text", "no leftover `{{`"), add a
  hermetic assertion to `plan_test.go` alongside the existing ones, and (c) re-drive at least one real
  live run against the fixed template to confirm the specific failure mode is actually gone — report
  the before/after transcript evidence, not a claim.
- Keep `go build`/`vet`/`test` green after every change.
- Update `manifest/designs/loom.md`'s Planner producer description and `docs/overview.md`'s loom
  entry (and `CONSTRAINTS.md` if you touch anything Hub-Geometry-relevant) IN THE SAME change as any
  fix that changes behavior. Do NOT add bugfix/hardening notes to `manifest/roadmap.md` (roadmap is
  planned milestones only, per `CLAUDE.md`) — this module already shipped/moved to Done there.
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it (see
  "Commit per fix" above) — do NOT push unless the user explicitly asks. Report the changed files and
  how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment doc-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Template/instruction-quality findings likewise; Docs & operability findings;
   What-was-tested with exact commands + observed results, including what you could NOT verify and
   why). Write it to `.scratch/loom-planner-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/loom-planner-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code, then drive at least two real live runs of
the composed prompt against a real throwaway hub), produce your independent findings, then implement
and verify the fixes.
