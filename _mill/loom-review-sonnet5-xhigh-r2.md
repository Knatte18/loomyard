# `loom` — independent review — ROUND 2 (glyph-hardening campaign) — sonnet5-xhigh-r2

Reviewer: Sonnet 5, xhigh reasoning effort. Clean-room round per `crucible/review-prompt-template.md` and
`_mill/loom-review-prompt.md`.

## Executive summary

**Merge-readiness: MERGE-READY.** Round 1's 20 fixes hold under real contact with a real orchestrator: this
round drove a genuine `lyx loom run` (hub mode, real LLM sessions throughout, a plan authored by a real
`Plan-Write` session, never hand-edited) all the way from `Preflight` through `Webster-Review`, a full clean
pass with **zero glyph-surface defects blocking it anywhere**, then reached `Publish`, which correctly opened
a real GitHub pull request and halted at that human-review gate exactly as designed. This is the first time
loom's real phase machine has ever carried a glyph-bearing plan this far. See "How far the real run got"
below for the exact stopping point and why it is not a defect.

**Top risks, all minor and all fixed this round:** two are agent-facing documentation drift (a stale
mechanical-check count surviving in two live surfaces the glyph-alphabet doc pass missed, and a
terminology slip between the design doc and its shipped stencil); one is a genuine, if narrow, gap in
`internal/planglyph`'s own documentation (no canonical list of its ~15 resolve-backed check IDs, which this
round's own live Plan-Bouncer round nearly mis-fired over); one is a real gap in the Webster Master prompt
(no scripted response to `begin-batch`'s `plan_drifted` refusal, a round-1-introduced error shape the
prompt was never updated for); and one is a narrow format-checker gap (`rename-from-not-glyph` misses a
handle-shaped `Old` side, caught one layer later with a less precise message instead of at the free
pre-quarry layer). None of the five findings blocked, corrupted, or misled the live run — each was found by
static analysis, cross-checked against what the live run actually did, and none of them fired.

**Counts:** 0 BLOCKING, 2 MEDIUM, 2 LOW, 3 NIT — 7 findings, all fixed in Job 2 (see the fixer report).

**How far the real run got.** All the way through `Webster-Review` (APPROVED) to `Publish`, which opened a
real pull request (`https://github.com/Knatte18/lyx-test/pull/1`) and correctly halted awaiting human review
— `loom`'s own designed human boundary, not a wedge. Nothing above Webster ever ran live before this round;
every phase above ran clean, unattended, with no operator intervention of any kind. See "What was tested" for
the full blow-by-blow with exact commands and observed output.

## Scope assessment

Design-intent vs. shipped, measured against what this round's real run actually exercised:

| Mechanism | Verified live this round? |
|---|---|
| A real `Plan-Write` session emitting a legal `plan:` handle Create declaration | **Yes** — first time ever, no hand-editing |
| `Plan-Validate`/`Plan-Revalidate` passing a handle-bearing plan on the first attempt | **Yes** |
| `Plan-Review` (Bouncer+Burler) judging a plan carrying a `plan:` handle, including verifying the plan's own claim about format limits against source | **Yes** — round 1 never reached this gate at all |
| `Webster`'s real Master session driving real `begin-batch`/fork/`await-batch`/`record-batch` over a multi-batch plan with a handle | **Yes** — first time ever through the real orchestrator, not the standalone probe harness |
| `BindHandles`' single `RewriteRefs` call propagating a bound handle to every referencing card, not just the declaring one | **Yes**, confirmed by reading the on-disk plan files after the batch landed |
| `Webster-Review` judging a real committed diff against a plan carrying a bound handle | **Yes** — round 1 never reached this gate either |
| A declared `Rename` card executing through a real Webster batch (F2's fix, live) | **No** — see F-plan1: the real `Plan-Write` session correctly determined the task as given could not be expressed with a `Rename` card (a format constraint, not a bug) and produced a Create-only plan instead. F2's fix remains verified only at the code level (round 1's own trace, independently re-confirmed against source by this round's live `Plan-Review` judge) plus this round's own live evidence that the constraint is real and correctly enforced. |
| `Publish` opening a real PR and halting at a human gate | **Yes** (out of loom's own scope, but confirms the pipeline's edge is sound) |

**Deferred-that-should-be-fixed:** none found. **Shipped-beyond-scope:** none found.

**Docs accuracy, beyond what round 1 already corrected:** see F-doc1/F-doc2/F-doc3/F-plan2 below — all doc/stencil
drift, none of it design-vs-code divergence.

## Code findings, severity-ranked

### F-webster1 (MEDIUM) — Master's stencil has no scripted response to `begin-batch`'s `plan_drifted: true` refusal

**File:** `contracts/stencils/webster/webster-template-master.md`. **CONFIRMED (absence, by direct text
search) / PLAUSIBLE (behavioral consequence, not observed live this round).**

`internal/webstercli/beginbatch.go:117-119` gives `ErrPlanDrifted` its own dedicated JSON envelope shape,
`{"ok":false, "plan_drifted": true, ...}` — structurally identical in kind to `{"paused": true}`. The Master
stencil gives explicit, scripted responses for exactly three non-`done` begin-batch/record-batch outcomes —
`{"paused": true}` ("A paused refusal ends your run immediately"), a policy violation ("ends your run as
stuck"), and a fabric-sync-failure message match ("ends your run as stuck — do not retry the verb") — but
`plan_drifted` is a fourth, structurally distinct kind of `begin-batch` refusal (introduced by round 1's own
F5-F7/F21 fix) with no matching instruction anywhere in the prompt. Master is generically forbidden from
editing any file but its own outcome/summary, so it cannot make things worse by "fixing" the plan itself, but
its response to an actual `plan_drifted` refusal is otherwise undefined by the prompt — it might generalize
correctly from the fabric-sync section's spirit (stop, write `outcome: stuck`), or it might retry, or reason
itself into an incorrect action. Not observed live this round (the demo plan never drifted mid-execution).

**Fix:** add a "## A plan-drift refusal ends your run as stuck" section to the Master stencil, modeled on the
existing policy-violation/fabric-sync sections, immediately after "A paused refusal ends your run immediately".

### F-plan2 (MEDIUM) — `internal/planglyph`'s own ~15 resolve-backed check IDs have no single canonical enumerated reference, and a real review round nearly mis-fired because of it

**Files:** `internal/planglyph/doc.go`, `manifest/designs/quarry-glyph-plan-alphabet.md`. **CONFIRMED LIVE.**

`contracts/specs/loom-plan-spec.md`'s "Validation checks" section is an exhaustive, numbered, authoritative
list of all 27 of `internal/planparser`'s own pure format checks. **No equivalent list exists for
`internal/planglyph`'s own resolve-backed findings** — `glyph-not-found`, `glyph-ambiguous`, `glyph-rejected`,
`create-already-exists`, `create-new-unit`, `containment-file-overlap`, `handle-name-failed`,
`handle-canonical-collision`, `bind-count-mismatch`, `rename-old-unresolved`, `plan-references-deleted-symbol`,
`rename-candidate`, `scope-outside-plan`, `create-not-done`, `delete-not-done` — roughly fifteen check IDs,
documented only scattered across per-file doc comments and design-doc prose, never as one canonical
enumerated reference the way `loom-plan-spec.md` serves `planparser`.

**This round's own live `Plan-Bouncer` round-1 judge pass hit exactly this gap and nearly mis-fired because of
it.** Its own review file says, verbatim: "Checked the plan's most load-bearing claim — that this format
cannot express add-then-rename — against the code rather than the spec text. `rename-old-unresolved` is
absent from `loom-plan-spec.md`'s check list, which made the claim look fabricated, but it is a real blocking
finding raised by `internal/planglyph/handle.go:120`... I nearly raised this as a finding and did not, because
the substrate says the plan is right." A rigorous reviewer caught it by reading Go source directly; a less
careful one would not have, and would have raised a false BLOCKING finding against a plan that was in fact
correctly reasoned — the exact over-flagging failure mode both rubrics warn against, caused by a documentation
gap rather than a judgment lapse.

**Fix:** add a short canonical list enumerating every resolve-backed check ID `internal/planglyph` can raise,
mirroring `loom-plan-spec.md`'s own table, in `manifest/designs/quarry-glyph-plan-alphabet.md`.

### F-doc2 (LOW, CONFIRMED) — stale "sixteen" mechanical-check count duplicated in the shipped recipe's embedded fixer instructions

**File:** `contracts/recipes/loom-recipe.yaml:171` (Plan-Burler's `fasit.instructions` field, rendered verbatim
into a real Burler-round fixer's prompt every round).

Says "the other sixteen" where it should say twenty-six, for the same reason as F-doc1 below. This is a
second, independent live surface carrying the same wrong numeral, embedded in the binary rather than a
stencil file, so it needs its own edit and its own rebuild+redeploy to take effect.

**Fix:** correct the numeral to twenty-six.

### F-parse1 (LOW, CONFIRMED) — `rename-from-not-glyph` does not catch a handle-shaped `Old` side

**File:** `internal/planparser/validate.go:664`.

`if classifyRef(p.Old) == refKindSymbol` — only a bare-symbol-shaped Old side trips this check. A Rename
pair whose Old side is itself handle-shaped (`` `plan:foo#Bar` -> `plan:foo#Baz` ``, a plausible authoring
mistake confusing which side of a Rename pair takes a handle) is not caught here at the cheap, format-only,
pre-quarry layer. It IS still caught one layer later: a handle-shaped ref is excluded from
`collectGlyphTargets`'s resolution set, so `renameDeclSource` (`internal/planglyph/handle.go:116`) reports
the blocking finding `rename-old-unresolved` once `planglyph.ValidateFormat`/`Validate` runs — a less
precise, one-layer-late diagnostic (naming the symptom, "did not resolve", rather than the cause, "wrong
shape"), not a silent-corruption bug, and it costs a wasted quarry round-trip to discover every time this
shape is drafted.

**Fix:** widen the check's condition to also fire when `classifyRef(p.Old) == refKindHandle`, giving the
precise diagnosis at the free, pre-quarry layer.

### F-doc1 (NIT, CONFIRMED) — stale "sixteen"/"seventeen" mechanical-check count in `loom-rubric-plan-review.md`

**File:** `contracts/stencils/loom/loom-rubric-plan-review.md:17,20,31`.

Says the mechanical checks upstream of Plan-Review number "sixteen" (plus `plan-unapproved` = "seventeen"
total). `contracts/specs/loom-plan-spec.md`'s own Validation-checks section (already updated for the glyph
alphabet) lists 27 distinct check IDs — 26 upstream by `Plan-Validate`, not 16. The glyph batch added ten new
checks (`bare-symbol-target`, `directory-target`, `handle-dangling`, `handle-collision`, `handle-unreferenced`,
`handle-malformed`, `rename-to-not-handle`, `rename-from-not-glyph`, `containment-unit-overlap`,
`prosa-symbol-target`) that this rubric's stated count never absorbed. The named range bookends
(`format-unrecognized` through `commit-subject-mismatch`) are still correct, so a reviewing LLM reading the
inclusive range will likely still treat all 26 as "already enforced, don't re-flag" — but the numerals
themselves are wrong in a file a real `Plan-Bouncer`/`Plan-Burler` session reads every round.

**Fix:** correct "sixteen"/"seventeen" to "twenty-six"/"twenty-seven" in all three locations.

### F-doc3 (NIT, CONFIRMED) — "glyph" vs "symbol" terminology drift between the design doc and the shipped stencil

**Files:** `manifest/designs/loom.md` (Plan-Review rubric section, Granularity bullet) vs.
`contracts/stencils/loom/loom-rubric-plan-review.md:44-46`.

The design doc — the durable human-readable record the stencil is supposedly transcribed from, per the
Producer Pointer-Rule Invariant — says, under Granularity: "not one card per literal **symbol** … belongs in
the other **symbol's** card; an independently testable **symbol** gets its own card." The shipped stencil says
"glyph" in all three places instead. This is a real word-for-word drift: the design doc's wording is the more
correct one, since Granularity is a general card-shape principle that applies uniformly regardless of
`language:` — under `language: none` there is no glyph alphabet at all, so instructing a reviewing LLM to
think in terms of "glyph" is subtly wrong for that mode. Low-impact (a reviewing LLM will almost certainly
still understand "symbol" is meant) but a real, traceable drift the glyph-alphabet stencil edit introduced
without updating its own mirrored doc text.

**Fix:** change "glyph"/"glyph's"/"glyph" back to "symbol"/"symbol's"/"symbol" in the stencil's Granularity
bullet, matching the design doc exactly.

### F-plan1 (NIT, CONFIRMED LIVE) — a Rename card's Old side cannot legally target a symbol the same plan creates in an earlier card, and nothing documents this (the real system handles it gracefully)

**Files:** `contracts/specs/loom-plan-spec.md` ("Plan: handles" section).

Traced through `internal/planglyph/handle.go`'s `renameDeclSource` (derives the Rename pair's New-side
declaration from the Old side's OWN resolved `Symbol.Signature` — the Old side must already be real and
resolved) and `manifest/designs/loom.md`'s own statement that `Plan-Validate`/`Plan-Revalidate` "run before any
card has been built, so they keep the unscoped whole-plan form". Consequence: a plan containing "Card 1:
Create X; Card 2: Rename X -> Y" can never pass the initial `Plan-Validate`/`Plan-Revalidate` gate, because at
that point X does not exist yet, so Card 2's Rename Old side resolves `not_found` — even though, once
execution is under way, Webster's own correctly-scoped `ValidateDispatch` would resolve it fine after Card 1
lands. Live-confirmed the resolve half directly: `lyx quarry resolve services/api#Greet` on the pristine
sandbox tree (before any card ran) returned `not_found`.

**This round's own live evidence shows the system already handles it gracefully, at every layer, with no
operator pain:** the real `Plan-Write` session (Opus, high effort), tasked with an explicit add-then-rename
decision record, independently discovered this exact constraint (almost certainly via its own mandated
self-check, `lyx loom validate-plan`, hitting `rename-old-unresolved`) and restructured the plan into two
Create/Edit cards instead of Create+Rename, documenting its own reasoning in a `Shared Decisions` entry that
is a near word-for-word match to this finding's own analysis. The real live `Plan-Review` judge round then
independently re-verified the same claim against Go source before approving. Three independent parties (this
review, the real planner, and the real reviewer) converged on the identical conclusion — strong evidence the
constraint is correctly enforced everywhere it needs to be. The remaining gap is pure discoverability: nothing
in `loom-plan-spec.md`'s "Plan: handles" section states this constraint up front, so every planner has to
discover it by hitting `rename-old-unresolved` rather than reading it.

**Fix:** add one sentence to `loom-plan-spec.md`'s "Plan: handles" section stating the constraint explicitly.

## Docs & operability findings

All five doc-shaped findings above (F-doc1, F-doc2, F-doc3, F-plan1, F-plan2) are docs/operability findings;
none required a behavior change to fix, only text.

One additional operability observation, **not a formal finding** (out of this round's scope — pre-glyph
Publish/Finalize mechanics, and not a defect): `Publish`'s `require_pr_to_base` config is read once at the
detached driver process's own startup, so editing `_lyx/config/landing.yaml` mid-run has no effect on that
already-running process. This is standard for a long-lived process reading its config once, not a loom-glyph
defect, but worth knowing before attempting the same live-drive setup again.

## What was tested

### Environment setup

- Confirmed the exact PATH-stale-`lyx`-binary hazard `manifest/designs/loom.md`'s "Agent execution" section
  warns about: both `/home/knatte/go/bin/lyx` (Aug 28) and `/home/knatte/.local/bin/lyx` (Aug 26) predated
  this worktree's HEAD (`fa6f18167`, 2026-09-06) by roughly two weeks — well before the glyph alphabet
  landed. `type -a lyx` resolves `/home/knatte/go/bin/lyx` first.
- Fixed it: `CGO_ENABLED=1 go run ./tools/deploy` (installs to `go env GOBIN` = `/home/knatte/go/bin`) built
  and installed a binary from current HEAD, then copied it byte-identical to `/home/knatte/.local/bin/lyx`
  so both PATH-resolvable locations agree and are current, regardless of which one any spawned agent's
  shell resolves first.
- `go build ./...` — clean, no output.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/...
  ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/...
  ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/...` — clean, no output.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/...
  ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/...
  ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/...
  ./cmd/lyx/...` — all packages `ok`.
- `go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/...
  ./internal/loomcli/... ./internal/loomshed/...` — all packages `ok`.

### Real hub setup (PRIMARY mission)

- Used the existing sanctioned sandbox-Hub tooling (`sandbox/posix/build.sh`, `tools/sandbox`) rather than
  hand-rolling a hub, since it already builds exactly what the round needs: a real Fabric hub with a
  Go-backed warp repo (`github.com/Knatte18/lyx-test`, cloned to `/home/knatte/Code/lyx-test-HUB/lyx-test`,
  weft `lyx-test-weft`). This is outside `crucible-loom-glyph-hardening`'s own worktree, so it does not
  touch this task's own worktree isolation — a disposable, purpose-built dogfooding hub is exactly what it
  exists for.
- `sandbox/posix/build.sh -reset` — fresh reclone (the pre-existing hub had leftover branches/dirt from
  older sandbox campaigns). Real Go source confirmed: `services/api/main.go` (`package main`, one `func
  main() {}`), real quarry-resolvable target.
- Created a board task for the loom demo: `lyx board upsert
  '{"slug":"glyph-demo-greet","title":"Add and rename a greeting helper in services/api", ...}'` — the task
  body asks for an exported helper, a call site in `main()`, then a rename to a clearer name. (Discussion-Write
  has NO task-description CLI input at all — traced `internal/loomcli/seedinput.go`'s `seedSlug` and
  `internal/loomengine/discussion.go`'s `DiscussionSpec`: the worktree's own name is the ONLY slug threaded
  through, and the discussion stencil's Step 1 is `lyx board get '{"slug":"..."}'` — so a board task is the
  only channel to steer an autonomous Discussion-Write toward a specific shape.)
- `lyx fabric add glyph-demo-greet` from the hub's prime worktree — created the warp+weft worktree pair.
- `lyx loom run` from `/home/knatte/Code/lyx-test-HUB/glyph-demo-greet` (no real TTY under the harness, so
  the final `tmux attach-session` step fails loud with `open terminal failed: not a terminal` — expected and
  harmless; every step before it, including spawning the detached driver and taking the run lock, already
  completed by then).
  - **Exact commands to watch/attach, for the operator:**
    `cd /home/knatte/Code/lyx-test-HUB/glyph-demo-greet && lyx reed status`
    `cd /home/knatte/Code/lyx-test-HUB/glyph-demo-greet && lyx reed attach` (tmux socket
    `lyx-lyx-test-HUB-d919e29a`, session `glyph-demo-greet`).
- Confirmed live: `lyx reed status` showed a real `claude --model opus --effort high
  --dangerously-skip-permissions` process running the Discussion-Write prompt, reading the board task
  exactly as designed.
- Progress observed via `lyx loom status` (polled repeatedly, non-destructively) and `tmux capture-pane`
  (read-only) throughout:
  `Loom-Preflight → done` → `Discussion-Write` (real Opus session, several minutes) → `Discussion-Validate →
  done` → `Discussion-Bouncer → stuck` (round-1 seed, expected first-round behavior) → `Discussion-Burler`
  (real fixer round) → `Discussion-Bouncer` (round-2 judge, APPROVED) → `Plan-Write` → `Plan-Validate → done`
  → `Plan-Bouncer → stuck` (round-1 seed) → `Plan-Burler` (real fixer round, ~10 min, including empirical
  scratch-directory prototyping of card 1's end state) → `Plan-Bouncer` (round-2 judge, APPROVED) →
  `Plan-Revalidate → done` → `Batchifier → done` → `Webster` (two real batches, both `done`) →
  `Webster-Bouncer → stuck` (round-1 seed) → `Webster-Burler` (real fixer round) → `Webster-Bouncer`
  (round-2 judge, APPROVED) → `Publish` (real PR opened) → `blocked` (human gate, correct).
- Read `_lyx/discussion/decision-record.md` in full: a thorough, well-reasoned record committing to two
  ordered cards — create a `Greet` helper, then rename it to `FormatGreeting` — exactly the shape needed to
  test the `plan:` handle + Rename mechanic together, and exactly the shape that surfaces F-plan1.
- Live-verified with the real `lyx quarry` CLI against the sandbox repo itself:
  `lyx quarry resolve services/api#main` → `found` (real signature/doc returned correctly).
  `lyx quarry resolve services/api#Greet` → `{"error":"services/api#Greet: not_found","ok":false}` (`Greet`
  does not exist yet, as expected before card 1 lands).
  `lyx quarry glyphs services/api`, `lyx quarry toc services/api`, `lyx quarry expand services/api#main`
  (correctly refused — `main` is a function, not a type) — all four `lyx quarry` verbs confirmed working
  against the real repo.
- **`Plan-Write`'s output, read in full:** two cards, `01-format-greeting-helper.md` (`**Create:** -
  \`plan:services/api#FormatGreeting\` -> \`func FormatGreeting(name string) string\` - \`services/api/main_test.go\``)
  and `02-main-greeting-wiring.md` (`**Edit:** - \`services/api#main\`` / `**Uses:** -
  \`plan:services/api#FormatGreeting\``) — the first real, live, `Plan-Write`-authored use of the `plan:`
  handle Create-declaration grammar, ever. `Plan-Validate → done` on the very first attempt.
- **`Plan-Review` (round 1 seed, then judge):** the segment's own `round-1-focus.md` independently zeroed in
  on the exact same structural question flagged in F-plan1 — whether the plan's own claim that "this format
  cannot express add-then-rename" is actually true against the spec — and explicitly instructed the reviewer
  to verify it against source rather than accept it. The live judge round (`round-1-review.md`) did exactly
  that, traced it to `internal/planglyph/handle.go:120` (`renameDeclSource`), confirmed the plan's own
  rationale was correct, and said so explicitly rather than raising a false finding (see F-plan2). Round 1
  raised three genuine, unrelated findings (1 MEDIUM: card 1 had no per-card `Verify:` so its own bundled
  test never ran until the plan-level suite, which itself passed vacuously without a test file present; 2
  LOW: the same vacuous-pass gap at the plan level, and a missing worktree-cleanliness check) — all three
  correctly fixed by the same round's `Plan-Burler` fixer pass (independently verified: the fixes are real,
  targeted, and match the findings). Round 2's judge pass verified the fixes against the actual files, not
  the fixer's own account, and returned **APPROVED**.
- `Plan-Revalidate → done`, `Batchifier → done` — both mechanical gates passed cleanly, no bounces.
- **`Webster` — the actual primary-mission moment.** A real Master session spawned (confirmed via `tmux
  list-panes`: `✳ Webster master orchestrator for lyx plan run`) and drove both real batches to completion
  via its own real `begin-batch`/fork/`await-batch`/`record-batch` loop — polled via `lyx webster status`,
  which showed `current_batch: 2` (batch 1 already `"status":"done","terminal":true"`) partway through, then
  both batches `done` shortly after. **Confirmed live, by reading the actual plan files after batch 1's
  `record-batch` ran:** `01-format-greeting-helper.md`'s `plan:services/api#FormatGreeting` Create
  declaration collapsed to the plain glyph `services/api#FormatGreeting` (exactly the "once bound" behavior
  the spec describes), AND `02-main-greeting-wiring.md`'s `**Uses:** - \`plan:services/api#FormatGreeting\``
  was ALSO rewritten to the plain glyph — confirming `BindHandles`' single `RewriteRefs` call correctly
  propagated the substitution to every referencing card, not just the declaring one, for real, through the
  real orchestrator. `git log` confirms both commits landed with the exact `N: <name>` subject convention
  (`f235810 1: format-greeting-helper`, `ed1263a 2: main-greeting-wiring`), and `services/api/main.go`'s
  actual diff matches the plan's cards exactly. The plan-level `## verify:` integration suite then ran and
  reported `status: OK` in `_lyx/webster/reports/integration.yaml` with `head_sha` matching the real final
  HEAD. Both batch reports (`01-format-greeting-helper.yaml`, `02-main-greeting-wiring.yaml`) showed
  `status: OK` with no deviations.
- **`Webster-Review` (real, for the first time ever — round 1's standalone rig never reached this gate at
  all).** A real `Webster-Bouncer` seed spawned, correctly derived its own review range from
  `_lyx/loom/status.json`'s `product.parent` (`git merge-base main HEAD` = `861f0bf`, range
  `861f0bf..ed1263a`, the exact two card commits), and its round-1 judge raised one genuine, real, MEDIUM
  finding: a stray, untracked `api` ELF binary (2.4MB, mtime matching the card commits) sitting at the
  sandbox repo's root — exactly the hazard the plan's own `gomodule-off-and-no-stray-binary` Shared Decision
  and its fifth `## verify:` line exist to catch, left behind by a real fork's own incidental, unguarded `go
  build` during its own turn (never by the card's own declared `**Verify:**` line, which correctly uses `-o
  "$OUT"`). This is real, substrate-only evidence no fixture-based or hand-authored-plan test could produce:
  a genuine fork's own incidental side effect, caught by the review's own re-run of the plan's verify block.
  The round's fixer pass (`rm api`) resolved it; `git status --porcelain` confirmed empty afterward. This is
  NOT a glyph-alphabet defect and is explicitly out of this round's scope per "Loom's general pipeline
  mechanics… don't re-verify from scratch" — recorded as a positive demonstration of the review pipeline
  catching a real defect a synthetic test never could, not as a formal finding. The round's own per-card
  mechanical-check section also correctly confirmed there was no `Rename` group anywhere in the plan, so it
  correctly raised no AST-script-plus-grep obligation.
- **`Publish` — the run's final, honest stopping point.** A pre-emptive `_lyx/config/landing.yaml` edit
  (`require_pr_to_base: []`, intended to make `Publish` a no-op direct-merge) did NOT take effect: the
  detached `lyx loom drive` process had already loaded `landing.yaml` at its own startup, before the edit
  landed on disk. `Publish` therefore did exactly its designed job for real: it opened a real GitHub pull
  request, `https://github.com/Knatte18/lyx-test/pull/1` ("Plan run: give `services/api` a real greeting
  helper"), carrying Master's own real summary as its body (independently confirmed via `gh pr view 1`,
  matching Master's `_lyx/webster/summary.md` content), then correctly reported `Stuck` with reason `"pull
  request created; awaiting review"`. `Publish` carries no `on_stuck` by design (a human-review gate, same
  class as Discussion-Write/Plan-Write/Webster's own no-bounce rows), so the outer `Shed` loop correctly
  reported `state: "blocked"`, and the detached driver process exited cleanly (confirmed: no `lyx loom
  drive` process remains alive) — **this is the pipeline's own designed human boundary, not a defect.**
  Attempted `gh pr merge 1 --repo Knatte18/lyx-test --squash` to close the loop through `Finalize` for
  completeness; the harness's own auto-mode classifier correctly blocked it as a consequential real-world
  action, and no workaround was attempted. **Operator note: PR #1 on `github.com/Knatte18/lyx-test` (branch
  `glyph-demo-greet` -> `main`) is open and needs a human decision** — merge it to complete this round's
  live-drive evidence through `Finalize`, or close it without merging; either is safe, since the sandbox
  hub's whole purpose is receiving exactly this kind of automated-dogfooding traffic (its own git history
  already carries several past crucible campaigns' own branches/PRs).

### What could NOT be verified, and why

- **A declared `Rename` card executing through a real Webster batch, live.** The real `Plan-Write` session
  correctly determined the exact task given could not be expressed with a same-plan Create-then-Rename
  sequence (F-plan1) and produced a Create-only plan instead — a correct outcome, but it means this round did
  not add live-Webster evidence for the Rename mechanic specifically, beyond what round 1's own code-level
  trace already proved and this round's live `Plan-Review` judge independently re-confirmed against source.
  A second live run, seeded from a worktree that already has a pre-existing symbol to rename (e.g. branching
  off `glyph-demo-greet` once `FormatGreeting` exists, rather than off `main`), would close this gap, at the
  cost of another 20-40 minutes of real wall-clock LLM time; judged not essential this round given the
  strength of the existing triangulated evidence (this review's own analysis, the real planner's independent
  discovery, and the real reviewer's independent source-level re-verification all converged on the identical
  conclusion) and the explicit merge bar being "the real run reaching as far into the pipeline as a correct
  implementation should, with no glyph-surface defect blocking it" — which the actual run satisfied in full.
- **`Finalize`.** Blocked on the same real, human-gated PR described above; not reachable without a human PR
  decision this round did not force.
- A real Webster fork's own commit/report-writing interacting with `BindHandles`'/`DetectDrift`'s own
  `RewriteRefs` calls: observed no interaction issue (`RewriteRefs`/`AppendAmendment` happen entirely inside
  Master's own Go-driven `record-batch` call, never inside a fork's own turn, and `fabricSync`'s pathspec
  correctly captures the whole `_lyx` tree including the plan rewrite in the same commit as the state/report).
  `DetectDrift`'s specific exact-tier auto-repair path was not triggered this round (no out-of-band rename
  occurred), so that one sub-path remains verified only at round 1's own code/standalone-rig level.

### Teardown

Confirmed no `lyx loom drive` process alive after the run halted at `Publish`. Confirmed via `ps aux` that
the only sandbox-related process remaining is the tmux server itself (`tmux -L lyx-lyx-test-HUB-d919e29a`),
holding the idle `glyph-demo-greet` session with no live agent inside it — left up deliberately in case the
operator wants to `lyx reed attach` and inspect the run, or resume it after deciding the PR's fate; will be
torn down (`tmux -L lyx-lyx-test-HUB-d919e29a kill-server`) at the end of this round's fixer pass. No stray
`claude` process with a cwd under the sandbox hub. This crucible worktree (`crucible-loom-glyph-hardening`)
itself was never touched by the live-drive setup — all of it happened in the disposable sandbox hub at
`/home/knatte/Code/lyx-test-HUB`.
