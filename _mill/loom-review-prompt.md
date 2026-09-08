# `loom` — independent review + fix (prompt template) — ROUND 9 (glyph-hardening campaign) — GLYPH-PLAN-FORMAT SURFACE ONLY, NARROWED SCOPE

> Filled instance of `crucible/review-prompt-template.md` for round 9 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230). See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's full running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **Why this round is scoped narrower than rounds 5–8:** round 8 (Sonnet/xhigh) found 10 findings — 2 BLOCKING. Only 2 of the 10 (PG-1, PG-2) were genuine glyph-plan-format defects, the surface this campaign exists to harden. The other 8 (SF-1: the provider-startup seam, a pre-existing area unrelated to the glyph feature, now on its FOURTH consecutive round of BLOCKING material; CW-1..4: `internal/cliwire`, landed via a separate mill task; WS-1/WS-2: webster validate/geometry docs) were scope creep from each round's broad "general adversarial sweep" mandate compounding round over round — every round's mandate kept expanding to "the whole module plus everything that ever landed in this branch's tree," and a sufficiently broad, sufficiently adversarial pass over a large-enough codebase will always find *something*, which is not the same as the glyph feature itself still having open defects. **The operator made an explicit call after round 8: all 10 of its findings are real and are already fixed, but round 9 narrows back to the glyph-plan-format surface specifically — no general sweep of the whole module, no re-review of the provider-startup seam or `cliwire` (both hardened across rounds 5–8 and 7–8 respectively, and further hardening of the startup seam is now a candidate for its own separate, small, dedicated follow-up task outside this campaign).**
>
> **Effort note:** this round runs on **Opus/High** — the operator's own pre-committed conditional pick, decided before round 8 even finished ("if round 8 comes back with BLOCKING, run round 9 on Opus/High"). Round 8 did find BLOCKING material, confirming the pick.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of `loom`'s glyph-plan-format surface in the loomyard repo, followed by FIXING what you find. This round's scope is deliberately NARROWER than every prior round — read "Explicitly OUT of scope" below carefully before you start.

## Where to work — determine this yourself, do not assume a path
Work in the current git worktree. Confirm it yourself at the start (`git rev-parse --show-toplevel`, `git branch --show-current`) rather than trusting any hardcoded path in this file or in a prior round's report — this campaign has already run across multiple host/user environments. The branch is `crucible-loom-glyph-hardening`.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of the glyph-plan-format surface's correctness — not the whole `loom` module, just this surface (see "What to read" / "Explicitly OUT of scope" below for the exact boundary).
   Hunt for bugs by reading the code AND, if a scenario genuinely needs it, driving the real substrate.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `loom: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/loom-review-<yourtag>.md` and `_mill/loom-review-<yourtag>-fixer-report.md` as you write or update them, and **fill in every row of the fixer report's own table before you consider Job 2 done** — round 4's table shipped with 7 real, committed fixes missing from it, caught only by the orchestrator's own independent audit. Don't repeat that.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/loom-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below, APPEND your observations to `_mill/loom-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns, and jot findings provisionally as you spot them.
**COMMIT each append.**

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including all eight prior rounds' review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment.
AFTER you have your own independent findings, you MAY (and should) consult the prior rounds' material and the handoff to (a) confirm the prior fixes have not regressed and (b) understand this round's specific mission below.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- **The glyph plan-format surface, in full, at its CURRENT state (round 8 just changed it — read the live files, not any prior round's description):** `internal/planparser/**` (`classify.go`, `parse.go`, `validate.go`, `containment.go`, `glyphref_test.go`, `validate_test.go`) and `internal/planglyph/**` (`planglyph.go`, `handle.go`, `drift.go`, `resolve.go`, and their test files).
- **PG-1's and PG-2's own fresh fixes and their immediate neighborhood** — round 8 added a 28th check (`glyph-malformed`) and a new `BindHandles` code path for Rename cards' New-side handles. Read these not just for correctness but for SIBLING gaps of the same shape: is `glyph-malformed` scoped correctly to every group/label combination the way `bare-symbol-target` is? Are there other type-label branches (`Delete`, `Move`, whatever `parseTypeLabelCase` handles) with the same "only `Create` populates `Declarations`" gap PG-2 just closed for `Rename`? Does `renameCardPairs`/`DetectDrift`'s own gate-one logic have any adjacent case PG-2's fix didn't touch?
- Docs: `contracts/specs/loom-plan-spec.md` (now 28 checks — confirm the spec text, `doc.go`, the rubric, and `loom-recipe.yaml` all genuinely agree), `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`.
- The nine prior rounds' material on the glyph surface specifically (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-fable5-high-r3.md`/`-fixer-report.md`, `_mill/loom-review-opus5-high-r4.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r5.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r6.md`/`-fixer-report.md`, `_mill/loom-review-fable-high-r7.md`/`-fixer-report.md`, `_mill/loom-review-sonnet-xhigh-r8.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the full campaign record).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full.

## Mission (be genuinely adversarial — try to find nothing, and earn that)

Two axes, applied ONLY to the glyph-plan-format surface (see "Explicitly OUT of scope" below for the boundary):

1. **Scope/integration** — do the 28 plan-validation checks and the handle-binding/drift-detection machinery actually cover every shape the glyph plan format allows, end to end?
2. **Correctness** — bugs, races, error handling, edge cases — including in code eight prior rounds already "fixed," and ESPECIALLY in the two code paths round 8 just changed (the new `glyph-malformed` check, the new Rename-handle-binding path). A fix that passed sabotage-proofing is proven correct for the SPECIFIC scenario it was tested against; it is not proven correct in general.

## High-yield focus

- **1. Adversarially re-examine round 8's own two fixes (PG-1, PG-2) for siblings of the same gap shape.** PG-1 found that ONE ref-shape class (`#`-shaped glyph refs) could fail validation invisibly; PG-2 found that ONE card-type-label (`Rename`) had a handle-binding gap. Both are instances of "a specific enumerated case was missed in an otherwise-general mechanism." Hunt for other instances: are there other ref shapes `classify.go` recognizes that get similarly under-validated? Are there other card shapes (beyond Create/Rename) whose `Declarations`/`Targets` population might have the same asymmetry?
- **2. Full adversarial pass over the 28-check plan-validation pipeline** (`internal/planparser/validate.go`) and the glyph resolution/containment machinery (`internal/planglyph/resolve.go`, `containment.go`, `drift.go`) — read it as if you expect a hole, the way round 8 found one in territory seven prior rounds had passed over.
- **3. `handle.go`'s `BindHandles` and its interaction with `DetectDrift`'s rename-pair gate** — PG-2's fix touched this; verify the fix itself is complete (does a Rename's New-side handle get bound correctly across every WAY a rename can be declared — e.g. a rename chain, two Rename cards on the same symbol, a Rename card whose New side collides with an existing symbol?).
- **4. General adversarial pass over the REST of the glyph-plan-format surface** (anything in `internal/planparser`/`internal/planglyph` not directly touched by items 1–3) — genuinely try to find nothing, and earn that if you do.

## Explicitly OUT of scope for this round (BLOCKING — do not re-review these; they are not this round's job)
- **The provider-startup seam** (`internal/shuttleengine/claudeengine/startup.go` and everything around it) — hardened across four consecutive rounds (5, 6, 7, 8); round 8 achieved genuine live confirmation twice. Further hardening of this seam (e.g. the "derive a single source of truth for gate identity" idea raised after round 8) is a candidate for a SEPARATE, dedicated follow-up task outside this campaign, not this round's job. Do not open `startup.go` at all this round.
- **`internal/cliwire`** — reviewed twice (round 7, round 8), by two different models, both times finding and fixing genuine gaps; converged. Do not re-review it this round.
- **`internal/webstercli/validate.go`'s fingerprint-restamp behavior (WS-1) and `websterengine.Geometry` docs (WS-2)** — both fixed in round 8, verified. Do not re-review.
- **Everything else in `loom`'s own pre-glyph pipeline machinery** (`internal/loomengine`, `internal/loomcli`, `internal/loomrecipe`, `internal/loomshed`, `internal/shedengine`, `internal/shedadapters`, `internal/shedrecipe`, `internal/shedbuild`, `internal/hubgeom`) — out of scope this round. This campaign's prior rounds (especially round 6's general sweep) already covered this material once; a further pass belongs to a future round or a separate task, not this narrowly-scoped one.
- **The standalone-webster material** (`internal/shuttleengine/run.go`, `internal/standalonegeom`, `internal/standalonestate`, `internal/logger/sink.go`) — hardened across rounds 1, 3, 4, 5; out of scope this round.
- Windows path behavior — unreachable from this Linux host across all nine rounds; do not reason about it as if driven.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Any new feature or roadmap work.

**If you find something genuinely alarming outside this round's scope while reading (not hunting for it, just noticing it), record it in your report as an OUT-OF-SCOPE OBSERVATION with file:line — do not fix it, do not let it distract from this round's actual mission.**

## Round context seeded from prior-round verification

**Rounds 1 through 8 are ALL CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

- **Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: whole-plan re-resolution against the post-change tree wedged every multi-batch plan with a Create/Delete/Rename card.
- **Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`.
- **Round 3:** 12 findings (5 BLOCKING), all fixed. Closed the Rename-through-Webster gap live to `Finalize → done`; found a real BLOCKING bug in file-rename plan validation (F1: a false `glyph-not-found`).
- **Round 4:** 38 findings (2 BLOCKING), 36 fixed. Fixed the last 2 fingerprint-restamp wedge bugs.
- **Round 5:** 7 findings (2 BLOCKING), all fixed. Both BLOCKING in the provider-startup seam (opened by this round; now hardened, out of this round's scope).
- **Round 6:** 28 findings (2 BLOCKING), all fixed. R6-1: provider-startup seam (out of scope). R6-3: `containment-file-overlap` indexed read-only `Uses:` refs alongside `Targets`, blocking `lyx webster run` on cards that only READ overlapping things — genuine glyph-surface material, now fixed and unregressed across three subsequent rounds.
- **Round 7:** 6 findings (1 BLOCKING), all fixed. F1: provider-startup seam (out of scope). First-ever review of the `cliwire` merge (out of scope this round).
- **Round 8:** 10 findings (2 BLOCKING), all fixed and independently verified. **SF-1** (provider-startup seam, out of scope this round). **PG-1**: a `#`-shaped glyph ref that fails quarry's own grammar produced ZERO findings across all 27 (now 28) plan-validation checks outside a `Prosa` group — fixed with a new `glyph-malformed` check. **PG-2**: a Rename card's own `plan:` New-side handle was never bound to its resolved glyph by any code path — permanently losing containment/resolution checking for later references to the renamed symbol — fixed. CW-1/CW-2/WS-1/WS-2/LS-1/CW-3/CW-4 all out of scope this round (see "Explicitly OUT of scope" above).

**Convergence status for the glyph-plan-format surface specifically: genuinely open.** PG-1/PG-2 are the first new glyph-surface findings since round 3 — five rounds (4, 5, 6, 7, and most of 8) found nothing new here. Round 9's job is to find out whether PG-1/PG-2 were the last two gaps of this shape, or the first two of a new batch — the same "sibling of a just-found gap" pattern that made PG-1/PG-2 findable in the first place (high-yield-focus item 1 above).

State the **merge bar**: correctness in the NORMAL single-instance flow, across every scenario above, is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module, but this round's scope is mostly hermetic)

**`LLM-DRIVING: optional, not required as this round's priority.`** This round's mission is a code-level adversarial pass over the plan-validation/handle-binding machinery — most of it is directly testable hermetically (unit tests over `classify.go`/`validate.go`/`handle.go`/`drift.go`/`resolve.go`, no real substrate needed). If you find a finding whose fix genuinely benefits from one live confirmation (e.g. confirming the new `glyph-malformed` check actually surfaces through a real `lyx webster validate` or a real `Plan-Review` pass on a hand-authored plan with a deliberately malformed ref), you MAY drive ONE such scenario — foreground, one at a time, standalone/zero-LLM-cost probe (`lyx webster validate`) preferred over a full real-LLM `lyx webster run` unless the finding specifically needs the latter. Do not treat live driving as this round's primary activity the way rounds 5–8 did for the startup seam.

- **Check your PATH's `lyx` before ANY live driving** — every prior round found stale installed binaries at least once; confirm `which lyx`/`lyx --version` reflects current HEAD, redeploy (`CGO_ENABLED=1 go run ./tools/deploy`) if not.
- If you do drive a live scenario touching Master/fork startup, it is not this round's concern — you should not need one, since this round's scope is plan-format validation, testable via `lyx webster validate`'s standalone, zero-LLM-cost path.

## What to TEST — do not just read, EXERCISE it

Hermetic (must stay green throughout):
- `CGO_ENABLED=1 go build ./...`
- `CGO_ENABLED=1 go vet ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...`
- `CGO_ENABLED=1 go test -count=5 ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...`
- `CGO_ENABLED=1 go test -tags integration ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/loomcli/... ./internal/webstercli/...`
- `CGO_ENABLED=1 go test ./...` (whole repo — confirm you haven't regressed anything outside your own scope)

Optional live driving (see cost declaration above — do this only if a finding's fix genuinely needs it):
- `lyx webster validate` against a hand-authored plan carrying a deliberately malformed/adversarial glyph ref or a Rename-then-reference pattern — this is a standalone, zero-LLM-cost probe (no real `claude` subprocess), cheap to run as many times as needed.
- If you genuinely need a real `lyx webster run`/`lyx loom run` for a specific finding, follow the same fixture-hygiene discipline prior rounds established (a fresh, never-claude-seen fixture if the scenario touches Master/fork startup at all — though this round's scope should rarely need that).

TEARDOWN DISCIPLINE: if you start any substrate server/session, tear it down. Confirm ZERO stray substrate processes at the end (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started).

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED vs PLAUSIBLE. Severity affects reporting, not whether you fix it — fix everything, all severities, including NIT.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- None carried forward as open coverage gaps from round 8 within THIS round's scope.
- Anything round 8 left open outside the glyph-plan-format surface (the startup-seam consolidation idea, further `cliwire`/pre-glyph-pipeline review) is explicitly out of scope this round — see "Explicitly OUT of scope" above. Do not re-evaluate those; they are not this round's job.

## Fixing — after the review
- Fix EVERY finding, all severities including NIT.
- Load `/code-quality` and `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it.
- Update the relevant docs in the SAME change as the fix (e.g. `contracts/specs/loom-plan-spec.md` if the check count or check set changes again). Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Keep gates green after every change.
- Tear down all substrate state if you used any; confirm zero stray processes. Commit each fix — do NOT push.
- **Fill in the fixer report's table completely before finishing** — every finding you fixed gets a row, written before you move to the next finding, not reconstructed from memory at the end.

## Deliverables
1. A structured review report: executive summary with an EXPLICIT verdict on whether the glyph-plan-format surface specifically appears converged now; which high-yield-focus items were attempted and how far each got; findings severity-ranked with file:line/scenario/fix/CONFIRMED-PLAUSIBLE; what-was-tested with exact commands. Write to `_mill/loom-review-<yourtag>.md`, commit incrementally.
2. A fixer report: implemented/deferred/tests/changed-files, table complete and accurate. Write to `_mill/loom-review-<yourtag>-fixer-report.md`.
3. Final chat message: concise summary + severity counts + report paths + explicit merge-readiness verdict for the glyph-plan-format surface + per-high-yield-focus-item yes/no on what was achieved. State any out-of-scope observations you recorded but did not act on.

Begin with the clean-room review, produce your independent findings, then implement and verify the fixes.
