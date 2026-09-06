# `loom` — independent review — ROUND 2 (glyph-hardening campaign) — sonnet5-xhigh-r2

Reviewer: Sonnet 5, xhigh reasoning effort. Clean-room round per `crucible/review-prompt-template.md` and
`_mill/loom-review-prompt.md`. This file is written and committed incrementally as review work proceeds,
per that prompt's "Log as you go" requirement.

## What was tested — running log

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
- Created a board task or the loom demo: `lyx board upsert
  '{"slug":"glyph-demo-greet","title":"Add and rename a greeting helper in services/api", ...}'` — the task
  body asks for an exported helper, a call site in `main()`, then a rename to a clearer name. (Discussion-Write
  has NO task-description CLI input at all — I traced `internal/loomcli/seedinput.go`'s `seedSlug` and
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
- Progress observed via `lyx loom status` (polled repeatedly, non-destructively):
  `Loom-Preflight → done` → `Discussion-Write` (real Opus session, several minutes) → `Discussion-Validate →
  done` → `Discussion-Bouncer → stuck` (round 1 seed, expected first-round behavior per the segment's own
  design, not a defect) → `Discussion-Burler` (real fixer round, several more minutes) → `Discussion-Bouncer`
  (round 2 judge pass) — in progress as of this log entry.
- Read the produced `_lyx/discussion/decision-record.md` in full: a thorough, well-reasoned record. It
  commits to **two ordered cards**: card 1 creates `func FormatGreeting`... wait — actually names the
  intermediate name `Greet`, wired into `main()`; card 2 renames `Greet` -> `FormatGreeting`. This is
  exactly the shape needed to test the `plan:` handle + Rename mechanic together, and exactly the shape that
  exposes a structural tension described below under Finding G1.
- Live-verified with the real `lyx quarry` CLI against the sandbox repo itself:
  `lyx quarry resolve services/api#main` → `found` (real signature/doc returned correctly).
  `lyx quarry resolve services/api#Greet` → `{"error":"services/api#Greet: not_found","ok":false}` (`Greet`
  does not exist yet, as expected before card 1 lands).
- **Plan-Write's output, read in full:** two cards, `01-format-greeting-helper.md` (`**Create:** -
  \`plan:services/api#FormatGreeting\` -> \`func FormatGreeting(name string) string\` - \`services/api/main_test.go\``)
  and `02-main-greeting-wiring.md` (`**Edit:** - \`services/api#main\`` / `**Uses:** -
  \`plan:services/api#FormatGreeting\``) — the FIRST real, live, `Plan-Write`-authored use of the `plan:` handle
  Create-declaration grammar, ever. `Plan-Validate → done` on the very first attempt: the plan cleared the
  initial gate cleanly.
- **`Plan-Review` (round 1 seed, then judge):** the segment's own `round-1-focus.md` independently zeroed in on
  the exact same structural question I had already flagged in F-plan1 (see below) — whether the plan's own
  claim that "this format cannot express add-then-rename" is actually true against the spec — and explicitly
  instructed the reviewer to verify it against source rather than accept it. The live judge round
  (`round-1-review.md`) did exactly that, traced it to `internal/planglyph/handle.go:120`
  (`renameDeclSource`), confirmed the plan's own rationale was correct, and said so explicitly rather than
  raising a false finding — see F-plan2 below for the near-miss this surfaced. Round 1 raised three genuine,
  unrelated findings (1 MEDIUM: card 1 had no per-card `Verify:` so its own bundled test never ran until the
  the plan-level suite, which itself passed vacuously without a test file present; 2 LOW: same vacuous-pass
  gap at the plan level, and a missing worktree-cleanliness check) — all three correctly fixed by the same
  round's `Plan-Burler` fixer pass (verified myself: the fixes are real, targeted, and match the findings).
  Round 2's judge pass verified the fixes against the actual files (not the fixer's own account) and returned
  **APPROVED**.
- **`Plan-Revalidate → done`, `Batchifier → done`** — both mechanical gates passed cleanly, no bounces.
- **`Webster` — the actual primary-mission moment.** A real Master session spawned (confirmed via `tmux
  list-panes`: `✳ Webster master orchestrator for lyx plan run`) and drove BOTH real batches to completion via
  its own real `begin-batch`/fork/`await-batch`/`record-batch` loop — polled via `lyx webster status`, which
  showed `current_batch: 2` (batch 1 already `"status":"done","terminal":true"`) partway through, then both
  batches `done` shortly after. **Confirmed live, by reading the actual plan files after batch 1's
  `record-batch` ran:** `01-format-greeting-helper.md`'s `plan:services/api#FormatGreeting` Create declaration
  collapsed to the plain glyph `services/api#FormatGreeting` (exactly the "once bound" behavior the spec
  describes), AND `02-main-greeting-wiring.md`'s `**Uses:** - \`plan:services/api#FormatGreeting\`` was ALSO
  rewritten to the plain glyph — confirming `BindHandles`' single `RewriteRefs` call correctly propagated the
  substitution to every referencing card, not just the declaring one, for real, for the first time ever
  through the real orchestrator (not the standalone probe harness round 1 used). `git log` confirms both
  commits landed with the exact `N: <name>` subject convention (`f235810 1: format-greeting-helper`,
  `ed1263a 2: main-greeting-wiring`), and `services/api/main.go`'s actual diff matches the plan's cards
  exactly. The plan-level `## verify:` integration suite then ran and reported `status: OK` in
  `_lyx/webster/reports/integration.yaml` with `head_sha` matching the real final HEAD.
- **Continuing to monitor through `Webster-Review`/`Publish`/`Finalize`; this section will be appended as the
  run progresses.** (Pre-emptively edited `_lyx/config/landing.yaml` in the sandbox hub — an operator-owned
  test-fixture config file, not loom's own source — setting `require_pr_to_base: []` so `Publish` direct-merges
  into this disposable sandbox hub's own `main` rather than opening a real pull request against the public
  `github.com/Knatte18/lyx-test` repository if the run reaches that far.)

## Findings — provisional, appended as spotted (severity/CONFIRMED-PLAUSIBLE finalized in the final report)

### F-doc1 (NIT, CONFIRMED) — stale "sixteen"/"seventeen" check-count in `loom-rubric-plan-review.md`
`contracts/stencils/loom/loom-rubric-plan-review.md:17,20,31` says the mechanical checks upstream of
Plan-Review number "sixteen" (plus `plan-unapproved` = "seventeen" total). `contracts/specs/loom-plan-spec.md`'s
own Validation-checks section (already updated for the glyph alphabet) lists **27** distinct check IDs (26 in
`ValidateFormat` + `plan-unapproved` in `Validate`), i.e. 26 upstream by Plan-Validate, not 16. The glyph
batch added ten new checks (`bare-symbol-target`, `directory-target`, `handle-dangling`, `handle-collision`,
`handle-unreferenced`, `handle-malformed`, `rename-to-not-handle`, `rename-from-not-glyph`,
`containment-unit-overlap`, `prosa-symbol-target`) that this rubric's stated count never absorbed. The named
range bookends (`format-unrecognized` through `commit-subject-mismatch`) are still correct, so a reviewing
LLM reading the inclusive range will likely still treat all 26 as "already enforced, don't re-flag" — but the
numerals are wrong in an agent-facing file a real Plan-Bouncer/Plan-Burler session reads every round.

### F-doc2 (LOW, CONFIRMED) — same stale count duplicated in the shipped recipe's embedded fixer instructions
`contracts/recipes/loom-recipe.yaml:171` (Plan-Burler's `fasit.instructions` field, rendered verbatim into a
real Burler-round fixer's prompt) also says "the other sixteen" instead of twenty-six. This is a second,
independent live surface carrying the same wrong numeral — worth fixing in the same change as F-doc1 since
both trace to the same root cause (round 1's glyph-alphabet doc corrections, F13/F15/F19, did not reach
either of these two Plan-Review-adjacent surfaces).

### F-doc3 (NIT, CONFIRMED) — "glyph" vs "symbol" terminology drift between the design doc and the shipped stencil
`manifest/designs/loom.md`'s "Plan-Review rubric" section (the durable human-readable record the stencil is
supposedly transcribed from, per the Producer Pointer-Rule Invariant) says, under Granularity: "not one card
per literal **symbol** ... belongs in the other **symbol's** card; an independently testable **symbol** gets
its own card." The shipped `contracts/stencils/loom/loom-rubric-plan-review.md:44-46` says "glyph" in all
three places instead. This is a real word-for-word drift (not just paraphrase): the design doc is the more
correct wording, since Granularity is a general card-shape principle that applies uniformly regardless of
`language:` — under `language: none` there is no glyph alphabet at all, so instructing a reviewing LLM to
think in terms of "glyph" is subtly wrong for that mode. Low-impact (an LLM reviewer will almost certainly
still understand "symbol" is meant) but a real, traceable drift introduced by the glyph-alphabet stencil
edit without updating its own mirrored doc text (or vice versa).

### F-webster1 (MEDIUM, CONFIRMED absence / PLAUSIBLE behavioral consequence) — Master's stencil has no scripted response to `begin-batch`'s `plan_drifted: true` refusal
`internal/webstercli/beginbatch.go:117-119` gives `ErrPlanDrifted` its own dedicated JSON envelope shape,
`{"ok":false, "plan_drifted": true, ...}` — structurally identical in kind to `{"paused": true}`. Traced the
full text of `contracts/stencils/webster/webster-template-master.md` (the ONLY prompt Master ever reads) and
confirmed by direct text search: it contains zero mentions of `plan_drifted`, `ErrPlanDrifted`, or "drift"
anywhere. Master's stencil gives explicit, scripted responses for exactly three non-`done` begin-batch/record-batch
outcomes — `{"paused": true}` ("A paused refusal ends your run immediately"), a policy violation ("ends your
run as stuck"), and a fabric-sync-failure message match ("ends your run as stuck — do not retry the verb") —
but `plan_drifted` is a fourth, structurally distinct kind of `begin-batch` refusal (introduced by round 1's
own F5-F7/F21 fix, i.e. it postdates whatever revision last touched this section of the Master stencil) with
no matching instruction. Master is generically forbidden from editing any file but its own outcome/summary,
so it cannot make things worse by "fixing" the plan itself, but its response to an actual `plan_drifted`
refusal is otherwise undefined by the prompt — it might generalize correctly from the fabric-sync section's
spirit (stop, write `outcome: stuck`), or it might retry, or reason itself into an incorrect action. Not yet
observed live (my demo run has not yet reached Webster); recorded here as a static-analysis finding pending
whatever the live run's Webster phase shows.

### F-plan1 (NIT, CONFIRMED LIVE) — a Rename card's Old side cannot legally target a symbol the SAME plan creates in an earlier card, and nothing documents this (but the real system handles it gracefully)

**UPDATE after observing the real Plan-Write session's output — this is now fully confirmed live, and the
severity is downgraded from the original LOW-MEDIUM guess to NIT, because the real system's own self-check
loop handled it exactly as designed, with no operator intervention and no wedge anywhere in the pipeline.**
The real Plan-Write session (Opus, high effort) read the same decision record quoted above (which commits to
an add-then-rename two-card sequence), explored the format, and produced a plan whose own `00-overview.md`
`## Shared Decisions` section contains this, verbatim:

> **Decision:** the helper is created as `FormatGreeting` in card 1. There is no intermediate `Greet` and no
> rename card, and the two cards split on helper-versus-caller instead.
> **Rationale:** the decision record sequences add-then-rename, but this plan format cannot express it: a
> `Rename` pair's `Old` side must resolve against the current worktree (`rename-old-unresolved`), so a symbol
> the same plan creates can never be a later card's rename source. ...

This is an exact, independent match to my own static-analysis chain (`renameDeclSource` requiring the Old
side to already resolve `found`; rows 8/10 running the unscoped whole-plan form) — the real agent apparently
discovered it via its own stencil-mandated self-check (`lyx loom validate-plan`, Step 5 of
`loom-template-plan.md`) hitting `rename-old-unresolved` during drafting, understood exactly why, and
restructured cleanly rather than producing an invalid plan or getting stuck in the Plan-Review loop. `Plan-Validate`
then passed clean on the very first attempt (`Plan-Validate → done`, no bounce). This is genuinely one of the
most valuable data points this round produced: proof that the self-check loop correctly steers a real planner
around a real format boundary condition, without ever surfacing as operator pain.

Residual finding, now purely a documentation gap rather than a functional defect: nothing in
`contracts/specs/loom-plan-spec.md`'s "Plan: handles" section or `loom-template-plan.md`'s stencil states this
constraint explicitly ("a Rename's Old side must already exist before this plan starts executing, even if a
same-plan Create would otherwise make it exist by the time execution reaches that card") — a real planner has
to discover it by hitting `rename-old-unresolved` rather than reading it up front. Worth one added sentence to
the spec's "Plan: handles" section for a planner that gets there before self-check catches it, or one wasted
round-trip against quarry every time this shape is drafted.

The resulting plan DOES exercise the `plan:` handle Create-declaration grammar for real, live, for the first
time ever (see "What was tested" below): card 1 declares `` `plan:services/api#FormatGreeting` ->
`func FormatGreeting(name string) string` ``, and card 2 references that same handle in its own `**Uses:**`
field. It does NOT exercise the Rename mechanic (F2's fix) at all, since the real planner correctly avoided
producing a Rename card given the task as stated — see "PRIMARY mission coverage" below for what this means
for the round's stated goal of observing a Rename through Webster live.

<details><summary>Original pre-live-confirmation write-up (kept for the record)</summary>

Original finding text, before the live Plan-Write session's output was read:
Traced through `contracts/specs/loom-plan-spec.md`'s "Plan: handles" section, `internal/planglyph/handle.go`'s
`renameDeclSource` (derives the Rename pair's New-side declaration from the Old side's OWN resolved
`Symbol.Signature` — i.e. the Old side must already be real and resolved), and `manifest/designs/loom.md`'s
own explicit statement that rows 8/10 (`Plan-Validate`/`Plan-Revalidate`) "run before any card has been
built, so they keep the unscoped whole-plan form" (as opposed to `Webster`'s own mid-execution
`ValidateDispatch`, correctly scoped to pending cards). Consequence: a plan containing "Card 1: Create X;
Card 2: Rename X -> Y" can never pass the INITIAL Plan-Validate/Plan-Revalidate gate, because at that point
(before Webster ever starts) X does not exist yet in the tree, so Card 2's Rename Old side resolves
`not_found` — even though, if execution were somehow allowed to proceed anyway, Webster's own correctly-scoped
`ValidateDispatch` would resolve it fine once Card 1 actually lands. Live-confirmed the resolve half directly:
`lyx quarry resolve services/api#Greet` on the pristine (pre-card-1) sandbox tree returns `not_found`. This is
NOT necessarily a "bug" in the sense of breaking a well-formed plan — the natural authoring fix is to create
the symbol with its final name to begin with, since renaming something in the very plan that created it
achieves nothing a correctly-named Create doesn't already achieve — but the CONSTRAINT itself is real,
load-bearing, and undocumented: nothing in `loom-plan-spec.md`, `loom-template-plan.md`, or either rubric
states "a Rename's Old side must already exist before this plan starts running", so a real Plan-Write session
has no way to know this ahead of time except by hitting it. The live decision record this round's real,
autonomous Discussion-Write session produced commits to EXACTLY this shape (create `Greet` in card 1, rename
it to `FormatGreeting` in card 2) — whether the real Plan-Write session independently discovers and works
around this (its own stencil's Step 5 self-check, `lyx loom validate-plan`, should in principle catch it
before Plan-Write ends its turn) is exactly what this round's live run is now testing. Recorded here
provisionally; will be upgraded to CONFIRMED or downgraded/withdrawn once the live Plan-Write/Plan-Validate
phase is observed.

</details>

### F-plan2 (MEDIUM, CONFIRMED LIVE) — planglyph's own ~15 resolve-backed check IDs have no single canonical enumerated reference, and a real review round nearly mis-fired because of it

`contracts/specs/loom-plan-spec.md`'s "Validation checks" section is an exhaustive, numbered, authoritative
list of all 27 of `internal/planparser`'s own pure format checks — exactly the kind of reference a reviewing
LLM (or a human) can check a claim against without reading Go source. **No equivalent list exists for
`internal/planglyph`'s own resolve-backed findings** — `glyph-not-found`, `glyph-ambiguous`, `glyph-rejected`,
`create-already-exists`, `create-new-unit`, `containment-file-overlap`, `handle-name-failed`,
`handle-canonical-collision`, `bind-count-mismatch`, `rename-old-unresolved`, `plan-references-deleted-symbol`,
`rename-candidate`, `scope-outside-plan`, `create-not-done`, `delete-not-done` — roughly fifteen check IDs,
each documented only in its own package doc comment (`handle.go`, `create.go`, `resolve.go`, `containment.go`,
`drift.go`, `scope.go`, `donecheck.go`) or in `manifest/designs/quarry-glyph-plan-alphabet.md`'s prose, never
as one canonical enumerated reference the way `loom-plan-spec.md` serves `planparser`.

**This round's own live Plan-Bouncer round-1 judge pass hit exactly this gap and nearly mis-fired because of
it.** Its own review file (`round-1-review.md`, quoted in "What was tested" below) says, verbatim: "Checked
the plan's most load-bearing claim — that this format cannot express add-then-rename — against the code
rather than the spec text. `rename-old-unresolved` is absent from `loom-plan-spec.md`'s check list, which made
the claim look fabricated, but it is a real blocking finding raised by `internal/planglyph/handle.go:120`...
I nearly raised this as a finding and did not, because the substrate says the plan is right." A rigorous
reviewer caught it by going all the way to the Go source; a less careful one would not have, and would have
raised a false BLOCKING finding against a plan that was in fact correctly reasoned — precisely the
over-flagging failure mode both rubrics warn against, caused here by a documentation gap rather than a
judgment lapse. Worth a short canonical list (a new subsection of `manifest/designs/quarry-glyph-plan-alphabet.md`,
or a `internal/planglyph/doc.go` addition) enumerating every resolve-backed check ID this package can raise,
mirroring `loom-plan-spec.md`'s own table for `planparser`.

## Fork-assisted research (parent's own synthesis, independently spot-checked)

Two research forks were used to widen static-analysis coverage in parallel with live-run monitoring — I
independently spot-checked their most load-bearing claims against the source myself before recording them
above (F-doc1/F-doc2 confirmed directly by grep+read; the `rename-from-not-glyph` gap below confirmed
directly by reading `internal/planparser/validate.go:634-678`).

### F-parse1 (LOW, CONFIRMED) — `rename-from-not-glyph` does not catch a handle-shaped `Old` side
`internal/planparser/validate.go:664`: `if classifyRef(p.Old) == refKindSymbol` — only a bare-symbol-shaped
Old side trips this check. A Rename pair whose Old side is itself handle-shaped (`` `plan:foo#Bar` ->
`plan:foo#Baz` ``, a plausible authoring mistake — confusing which side of a Rename pair takes a handle) is
NOT caught here at the cheap, format-only, pre-quarry layer. It IS still caught, one layer later: since a
handle-shaped ref is excluded from `collectGlyphTargets`'s resolution set, `renameDeclSource`
(`internal/planglyph/handle.go:116`) reports the blocking finding `rename-old-unresolved` ("did not resolve
found; nothing to derive the new declaration from") once `planglyph.ValidateFormat`/`Validate` runs — so this
is a less-precise, one-layer-late diagnostic rather than a silent-corruption bug. Matches the check's own
narrow contract as literally stated in `loom-plan-spec.md` check 17 ("a symbol Rename pair's Old side
classifies as a bare symbol rather than a glyph") — so arguably not a spec violation, but a real gap versus
the check's own name/intent ("rename-from-**not-glyph**", not "rename-from-not-bare-symbol"). Worth widening
to catch this shape at the free, pre-quarry layer instead of paying a resolve round-trip to discover it.

## Deferred to the fixer pass
Everything above will be fixed in Job 2 once the review is complete, all severities including NIT, per the
prompt's "Fixing" section.
