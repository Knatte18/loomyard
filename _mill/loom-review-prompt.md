# `loom` — independent review + fix (prompt template) — ROUND 10 (glyph-hardening campaign) — GLYPH-PLAN-FORMAT SURFACE ONLY, NARROWED SCOPE (SECOND PASS)

> Filled instance of `crucible/review-prompt-template.md` for round 10 of the campaign scoped to the quarry-glyph-plan-alphabet surface (GitHub PR #230). See [../../crucible/README.md](../../crucible/README.md) for the loop, [../../_mill/loom-crucible-orchestrator-kickoff.md](loom-crucible-orchestrator-kickoff.md) for the campaign charter, and [loom-review-HANDOFF.md](loom-review-HANDOFF.md) for the campaign's full running state (read the handoff only AFTER you have your own independent findings list — see "Clean-room review constraint" below).
>
> **Why this round exists:** round 9 (Opus/High), scoped exactly as narrowly as this round, found 8 findings — 1 BLOCKING (R9-1, live-confirmed) — ALL genuinely new material on the glyph-plan-format surface, none of them scope creep. Round 9's own report named the defect shape explicitly: "a specific enumerated case was missed in an otherwise-general mechanism" — the same shape round 8's PG-1/PG-2 had. Round 9 confirmed PG-1/PG-2 were the FIRST two of a batch, not the last two: narrowing scope surfaced MORE glyph-surface material (2 findings in round 8 → 8 in round 9), not less. **This round's job is to find out whether round 9 exhausted that pattern, or only found instances 3 through 8 of a longer series.** A clean pass here would mean the glyph-plan-format surface has genuinely converged; another batch of the same-shaped findings would argue for a structural fix (consolidating the shape-classification/canonicalization mechanism itself) rather than continuing point-fix rounds indefinitely — that decision belongs to the operator after this round reports back, not to you.
>
> **Effort note:** this round runs on **Fable/High** — the operator's explicit pick. This is the campaign's THIRD Fable deployment (round 3, `fable-high-r3`, found 2 real BLOCKING defects in territory two prior rounds had already "closed" — the same shape this round is looking for again; round 7, `fable-high-r7`, found 1 more, also in territory a prior round had closed). Fable has never yet come back clean on this campaign — worth noting, not a bias to correct for, just context.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of `loom`'s glyph-plan-format surface in the loomyard repo, followed by FIXING what you find. This round's scope is deliberately NARROW, exactly as round 9's was — read "Explicitly OUT of scope" below carefully before you start.

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
Do NOT read any prior review or review-dialogue files before you have your own list — specifically do not open anything under `_mill/` matching `loom-review-*` in THIS worktree, including all nine prior rounds' review/fixer reports AND this campaign's running `loom-review-HANDOFF.md`. This is a FILENAME PATTERN, not a content judgment.
AFTER you have your own independent findings, you MAY (and should) consult the prior rounds' material and the handoff to (a) confirm the prior fixes have not regressed and (b) understand this round's specific mission below.
Reading the design SPEC and the module docs is expected and required (those are not reviews).

## What to read
- **The glyph plan-format surface, in full, at its CURRENT state (round 9 just changed it in six places — read the live files, not any prior round's description):** `internal/planparser/**` (`classify.go`, `parse.go`, `normalize.go`, `validate.go`, `containment.go`, `handle.go`, `drift.go` if present, plus every `_test.go` alongside them) and `internal/planglyph/**` (`planglyph.go`, `handle.go`, `drift.go`, `resolve.go`, `create.go`, `donecheck.go`, `containment.go`, `scope.go`, `delta.go`, `repo.go`, and their test files).
- **Round 9's own six fixes and their immediate neighborhood** — round 9 changed: `classify.go`/`normalize.go`'s canonicalization gate (R9-1), `validate.go`'s Rename-pair shape predicates (R9-2), `validate.go`+`handle.go`'s handle-collision counting (R9-3), `drift.go`'s rename-pair gate-one indexing (R9-4), `validate.go`'s file-unit rule scoping (R9-5), and `create.go`+`resolve.go`'s status-switch default arms (R9-6). Read each fix not just for correctness in isolation, but for SIBLING gaps of the same shape it just closed — the same technique that found R9-1 through R9-8 as PG-1/PG-2's siblings.
- **Round 9's own recorded-but-deliberately-unfixed observations (OBS-1, OBS-2, OBS-3)** — see "Deferred items from the prior round" below; re-evaluate each on its own merits.
- Docs: `contracts/specs/loom-plan-spec.md` (confirm the spec text, `doc.go`, the rubric, and `loom-recipe.yaml` all genuinely agree after round 9's five spec-touching commits), `manifest/designs/quarry-glyph-plan-alphabet.md`, `manifest/designs/loom.md`.
- The ten prior rounds' material on the glyph surface specifically (read AFTER your own findings list): `_mill/loom-review-opus5-high-r1.md`/`-fixer-report.md`, `_mill/loom-review-sonnet5-xhigh-r2.md`/`-fixer-report.md`, `_mill/loom-review-fable5-high-r3.md`/`-fixer-report.md`, `_mill/loom-review-opus5-high-r4.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r5.md`/`-fixer-report.md`, `_mill/loom-review-opus-medium-r6.md`/`-fixer-report.md`, `_mill/loom-review-fable-high-r7.md`/`-fixer-report.md`, `_mill/loom-review-sonnet-xhigh-r8.md`/`-fixer-report.md`, `_mill/loom-review-opus-high-r9.md`/`-fixer-report.md`, `_mill/loom-review-HANDOFF.md` (the full campaign record).
- Repo rules: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full.

## Mission (be genuinely adversarial — try to find nothing, and earn that)

Two axes, applied ONLY to the glyph-plan-format surface (see "Explicitly OUT of scope" below for the boundary):

1. **Scope/integration** — do the plan-validation checks and the handle-binding/drift-detection machinery actually cover every shape the glyph plan format allows, end to end?
2. **Correctness** — bugs, races, error handling, edge cases — including in code nine prior rounds already "fixed," and ESPECIALLY in the six code paths round 9 just changed. A fix that passed sabotage-proofing is proven correct for the SPECIFIC scenario it was tested against; it is not proven correct in general.

## High-yield focus

- **1. Adversarially re-examine round 9's own six fixes (R9-1 through R9-6) for further siblings of the same gap shape.** Round 9 already did this once against round 8's PG-1/PG-2 and found 6 more (R9-1..R9-6, plus R9-7/R9-8 elsewhere). Do the same pass against round 9's OWN fixes: is R9-1's canonicalization-gate narrowing complete for every ref shape `classifyRef` recognizes, not just the one it was built to fix? Does R9-2's inverted (fail-closed) predicate for Rename-pair shape have a matching gap on some OTHER pair-shaped check elsewhere in `validate.go`? Does R9-3's "union of both claim sources" fix for `handle-collision` need the same union anywhere else two handle-declaring sources are supposed to agree? Does R9-6's new default-arm-fails-closed pattern belong on any OTHER unswitched `Status`/enum consumer in `planglyph`?
- **2. Re-evaluate round 9's three recorded-but-unfixed observations on their own merits** (OBS-1, OBS-2, OBS-3 — see "Deferred items" below). Decide independently whether each is still correctly deferred, or whether it is cheap enough / severe enough to fix this round after all.
- **3. Full adversarial pass over the whole plan-validation pipeline and the glyph resolution/containment/drift machinery**, treating EVERY check and EVERY code path as suspect, not just the ones round 9 touched — read it as if you expect a hole, the way round 9 found six in territory eight prior rounds had passed over.
- **4. General adversarial pass over the REST of the glyph-plan-format surface** (anything not directly touched by items 1–3) — genuinely try to find nothing, and earn that if you do.

## Explicitly OUT of scope for this round (BLOCKING — do not re-review these; they are not this round's job)
- **The provider-startup seam** (`internal/shuttleengine/claudeengine/startup.go` and everything around it) — hardened across four consecutive rounds (5, 6, 7, 8); round 8 achieved genuine live confirmation twice. Further hardening of this seam is a candidate for a SEPARATE, dedicated follow-up task outside this campaign, not this round's job. Do not open `startup.go` at all this round.
- **`internal/cliwire`** — reviewed twice (round 7, round 8), by two different models, both times finding and fixing genuine gaps; converged. Do not re-review it this round.
- **`internal/webstercli/validate.go`'s fingerprint-restamp behavior (WS-1) and `websterengine.Geometry` docs (WS-2)** — both fixed in round 8, verified. Do not re-review.
- **Everything else in `loom`'s own pre-glyph pipeline machinery** (`internal/loomengine`, `internal/loomcli`, `internal/loomrecipe`, `internal/loomshed`, `internal/shedengine`, `internal/shedadapters`, `internal/shedrecipe`, `internal/shedbuild`, `internal/hubgeom`) — out of scope this round.
- **The standalone-webster material** (`internal/shuttleengine/run.go`, `internal/standalonegeom`, `internal/standalonestate`, `internal/logger/sink.go`) — hardened across rounds 1, 3, 4, 5; out of scope this round.
- Windows path behavior — unreachable from this Linux host across all ten rounds; do not reason about it as if driven.
- `quarry`'s own resolve/delta engine correctness — treat its answers as ground truth.
- Any new feature or roadmap work.

**If you find something genuinely alarming outside this round's scope while reading (not hunting for it, just noticing it), record it in your report as an OUT-OF-SCOPE OBSERVATION with file:line — do not fix it, do not let it distract from this round's actual mission.**

## Round context seeded from prior-round verification

**Rounds 1 through 9 are ALL CLOSED-AND-VERIFIED** — independently confirmed by this orchestrator, not self-reported. Full detail and verification evidence: `_mill/loom-review-HANDOFF.md`.

- **Round 1:** 22 findings (13 BLOCKING), 20 fixed. Root cause: whole-plan re-resolution against the post-change tree wedged every multi-batch plan with a Create/Delete/Rename card.
- **Round 2:** 7 findings (0 BLOCKING), all fixed. Proved a real hub-mode `lyx loom run` carries a glyph-bearing, `Plan-Write`-authored plan cleanly through `Webster-Review`.
- **Round 3:** 12 findings (5 BLOCKING), all fixed. Closed the Rename-through-Webster gap live to `Finalize → done`; found a real BLOCKING bug in file-rename plan validation (F1: a false `glyph-not-found`).
- **Round 4:** 38 findings (2 BLOCKING), 36 fixed. Fixed the last 2 fingerprint-restamp wedge bugs.
- **Round 5:** 7 findings (2 BLOCKING), all fixed. Both BLOCKING in the provider-startup seam (opened by this round; now hardened, out of this round's scope).
- **Round 6:** 28 findings (2 BLOCKING), all fixed. R6-1: provider-startup seam (out of scope). R6-3: `containment-file-overlap` indexed read-only `Uses:` refs alongside `Targets`, blocking `lyx webster run` on cards that only READ overlapping things — genuine glyph-surface material, now fixed and unregressed across four subsequent rounds.
- **Round 7:** 6 findings (1 BLOCKING), all fixed. F1: provider-startup seam (out of scope). First-ever review of the `cliwire` merge (out of scope this round).
- **Round 8:** 10 findings (2 BLOCKING), all fixed and independently verified. **SF-1** (provider-startup seam, out of scope this round). **PG-1**: a `#`-shaped glyph ref that fails quarry's own grammar produced ZERO findings across all 27 (now 28) plan-validation checks outside a `Prosa` group — fixed with a new `glyph-malformed` check. **PG-2**: a Rename card's own `plan:` New-side handle was never bound to its resolved glyph by any code path — fixed. CW-1/CW-2/WS-1/WS-2/LS-1/CW-3/CW-4 all out of scope this round.
- **Round 9:** 8 findings (1 BLOCKING, live-confirmed), all fixed and independently verified. **R9-1**: `classifyRef`'s rule 4 admitted an extensionless repository-root filename (`LICENSE`, `Makefile`) as a legal path ref, but `canonicalizeCard` never turned it into a glyph — every downstream consumer handed quarry a string it rejects pre-resolution, permanently wedging `create-not-done` and silently no-oping `delete-not-done`. **R9-2**: a Rename pair's shape check enumerated forbidden shapes positively on both halves and both omitted `refKindPath`, leaking a whole shape through unvalidated. **R9-3**: `handle-collision` counted only `Create` declarations while `handle-dangling` already accepted `Rename` to-sides as declarations too, so a handle claimed by both sources was silently rewritten to the wrong glyph. **R9-4**: `renameCardPairs`' last-wins map meant two Rename pairs sharing one Old side let `DetectDrift`'s gate one misclassify one of them as drift and auto-repair it. **R9-5**: `handle-malformed`'s file-unit rule was applied to Rename-to-side-only handles, whose unit is never actually read, producing false refusals. **R9-6**: `createFindings`/`statusFindings` failed OPEN on an unrecognized/absent quarry status — the exact shape R9-1 proved reachable. Plus R9-7/R9-8 (NIT, doc/guard-order). **Recorded but deliberately NOT fixed** (see "Deferred items" below): OBS-1, OBS-2, OBS-3.

**Convergence status for the glyph-plan-format surface specifically: still genuinely open, arguably MORE open than before round 9.** Round 8 found 2 new findings here (PG-1, PG-2); round 9 — scoped exactly this narrowly — found 8 more, all the same "missed enumerated case" shape. This round's job is to find out whether round 9 exhausted that pattern or only found instances 3 through 8 of a longer series. If this round ALSO finds a batch of the same shape, that is a strong signal the underlying mechanism (shape classification + canonicalization, spread across `classify.go`/`normalize.go`/`validate.go`) needs a structural consolidation rather than more point-fixes — say so explicitly in your verdict if you believe that's what you're seeing, even if you can't fix the structural issue within this round's own budget.

State the **merge bar**: correctness in the NORMAL single-instance flow, across every scenario above, is the gate. Do not chase artificial concurrency stress.

## Live-substrate cost declaration (loom IS an LLM-driving module, but this round's scope is mostly hermetic)

**`LLM-DRIVING: optional, not required as this round's priority.`** This round's mission is a code-level adversarial pass over the plan-validation/handle-binding machinery — most of it is directly testable hermetically. If a finding's fix genuinely benefits from one live confirmation (round 9 used `lyx quarry resolve` directly — a zero-LLM-cost, no-tmux, no-`claude` probe — for exactly this reason), you MAY drive ONE such scenario, foreground, one at a time, standalone/zero-LLM-cost probes preferred over a full real-LLM `lyx webster run` unless a finding specifically needs the latter.

- **Check your PATH's `lyx` before ANY live driving** — every prior round found stale installed binaries at least once; confirm `which lyx`/`lyx --version` reflects current HEAD, redeploy (`CGO_ENABLED=1 go run ./tools/deploy`) if not.

## What to TEST — do not just read, EXERCISE it

Hermetic (must stay green throughout):
- `CGO_ENABLED=1 go build ./...`
- `CGO_ENABLED=1 go vet ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...`
- `CGO_ENABLED=1 go test -count=5 ./internal/planparser/... ./internal/planglyph/... ./internal/loomcli/... ./internal/websterengine/... ./internal/webstercli/...`
- `CGO_ENABLED=1 go test -tags integration ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/loomcli/... ./internal/webstercli/...`
- `CGO_ENABLED=1 go test ./...` (whole repo — confirm you haven't regressed anything outside your own scope)

Optional live driving (see cost declaration above — do this only if a finding's fix genuinely needs it):
- `lyx quarry resolve <ref>` / `lyx webster validate` against a hand-authored plan carrying a deliberately adversarial ref — both are standalone, zero-LLM-cost probes (no real `claude` subprocess), cheap to run as many times as needed.
- If you genuinely need a real `lyx webster run`/`lyx loom run` for a specific finding, follow the same fixture-hygiene discipline prior rounds established (a fresh, never-claude-seen fixture if the scenario touches Master/fork startup at all — though this round's scope should rarely need that).

TEARDOWN DISCIPLINE: if you start any substrate server/session, tear it down. Confirm ZERO stray substrate processes at the end (`ps aux | grep -iE 'tmux|lyx|claude'`, scoped to what YOU started).

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED vs PLAUSIBLE. Severity affects reporting, not whether you fix it — fix everything, all severities, including NIT.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- **OBS-1** — a `Rename` pair's New side is never checked for already existing, where the `Create` inversion (`create-already-exists`, `internal/planglyph/create.go`) checks exactly that for a `Create` target. Round 9 deferred this because closing it adds a new check ID and a new batched resolve — extending rather than hardening. Re-evaluate: is it still correctly out of scope, or does this round's own findings make it cheap enough / connected enough to fold in?
- **OBS-2** — a completed symbol-Rename card's on-disk pair trips `rename-to-not-handle` under any UNSCOPED whole-plan validation post-execution. Round 9 judged this contained (every real mid-execution consumer already scopes findings to pending cards). Re-evaluate if you find a caller that doesn't scope.
- **OBS-3** — a bare extensionless directory at the repository root loses its narrow `prosa-symbol-target` fallback under round 9's R9-1 fix (deliberate, documented consequence, not a regression). Re-confirm this is still true and still acceptable.

## Fixing — after the review
- Fix EVERY finding, all severities including NIT.
- Load `/code-quality` and `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it.
- Update the relevant docs in the SAME change as the fix (e.g. `contracts/specs/loom-plan-spec.md` if the check count or check set changes again). Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Keep gates green after every change.
- Tear down all substrate state if you used any; confirm zero stray processes. Commit each fix — do NOT push.
- **Fill in the fixer report's table completely before finishing** — every finding you fixed gets a row, written before you move to the next finding, not reconstructed from memory at the end.

## Deliverables
1. A structured review report: executive summary with an EXPLICIT verdict on whether the glyph-plan-format surface specifically appears converged now, AND an explicit opinion on whether the recurring "missed enumerated case" defect shape looks exhausted or whether it argues for a structural consolidation (see "Convergence status" above); which high-yield-focus items were attempted and how far each got; findings severity-ranked with file:line/scenario/fix/CONFIRMED-PLAUSIBLE; what-was-tested with exact commands. Write to `_mill/loom-review-<yourtag>.md`, commit incrementally.
2. A fixer report: implemented/deferred/tests/changed-files, table complete and accurate. Write to `_mill/loom-review-<yourtag>-fixer-report.md`.
3. Final chat message: concise summary + severity counts + report paths + explicit merge-readiness verdict for the glyph-plan-format surface + per-high-yield-focus-item yes/no on what was achieved. State any out-of-scope observations you recorded but did not act on.

Begin with the clean-room review, produce your independent findings, then implement and verify the fixes.
