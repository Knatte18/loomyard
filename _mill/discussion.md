# Discussion: webster: stop re-rendering already-inherited context into fork prompts

```yaml
task: 'webster: stop re-rendering already-inherited context into fork prompts'
slug: webster-fork-context-hygiene
status: discussing
parent: main
```

## Problem

webster's Master session reads the codebase, `CONSTRAINTS.md`, `PATTERN.md`, and the plan once, up front, and every implementer it spawns is an in-session `subagent_type: "fork"` that inherits that whole context for free. That is the entire point of forking. Today the fork-prompt rendering pipeline (`internal/websterengine/render.go`'s `RenderForkPrompt`, `fork-template.md`, `master-template.md`) defeats it: it re-renders content into each fork's own prompt that Master already read and every fork already inherits — plan-level Shared Decisions, the rename mechanic, and the PATTERN directive. The PATTERN directive is the worst case: it doesn't just add redundant text, it instructs the fork to independently re-`Read` `PATTERN.md`, a wasted tool action for content already sitting in inherited context. On top of that, Master itself is told to pre-read every card file when it only ever needs `00-overview.md`.

**Why now:** confirmed 2026-08-02. This task sequences back-to-back with `webster-rename-master-merriam` (touches the same files) — never in parallel.

**Critical finding from exploration (reshapes the task):** `RenderForkPrompt`'s output is used by **two** callers with opposite context situations. `beginbatch.go:218` feeds the **in-session fork** (inherits everything). `recoverbatch.go:216` feeds the **cold recovery strand** — `recoverbatch.go`'s own header calls it *"the only place webster spawns a genuinely separate process… rendering the SAME fork prompt a fork would have gotten"*, and `render.go:186` says *"a cold recovery strand's rendered prompt is its whole instruction."* The recovery strand inherits **nothing**. So `shared_decisions` / `rename_mechanic` / PATTERN are redundant for the in-session fork but **load-bearing** for the cold recovery strand. Naively editing `RenderForkPrompt` in place — as the task's original "Confirmed redundant" list reads — would strip the recovery strand's only source of that context. The resolution (user decision) is to **split** the one prompt into two: a thin in-session fork prompt and a full, honest cold-start recovery prompt. This split also fixes a latent bug: today the recovery strand is handed the fork template that literally claims *"You never start cold: you inherit Master's whole context"* — false for that strand.

## Scope

**In:**

- **Master reads only `00-overview.md`** (fix #1): `master-template.md` line 29 changes from "read the whole plan — every card file, not just the overview — once" to reading only `00-overview.md` (task framing, Card Index, `## Shared Decisions`, `## Rename mechanic`, `## verify:`). Update the line-85 parenthetical ("you already read the whole plan at orientation") to match (Master read the overview, not every card file).
- **Split the fork prompt into two render paths:**
  - `fork-template.md` + `RenderForkPrompt` → **thin, in-session only** (called by `beginbatch.go:218`). Drops `shared_decisions`, `rename_mechanic`, and the PATTERN directive (all inherited). Points to the card file(s) instead of inlining card content. Keeps `self_fix_cap`, `worktree_root`, `prev_digest`, `report_path`, and the FRESH-READ / implementer-not-driver / bounded-self-fix / final-report instructions. Honest new framing: "you inherit Master's context."
  - **New** `recovery-template.md` + **new** `RenderRecoveryPrompt` → **full, cold-start** (called by `recoverbatch.go:216`). A self-contained cold implementer prompt: instructs the strand to read `_lyx/plan/00-overview.md` (framing, Shared Decisions, rename mechanic), `_pattern/PATTERN.md` (via the existing optional PATTERN directive), `CONSTRAINTS.md`, orient to the codebase, then read + implement its card file(s). Honest framing: "you start cold." Keeps `self_fix_cap`, `worktree_root`, `prev_digest`, `report_path`.
- **Card content delivered by pointer, in both templates:** replace the inlined `renderBatchCards`/`renderCard` output with a rendered list of the batch's card-file paths (`_lyx/plan/NN-<slug>.md`). The fork/strand reads the card file itself.
- **`planparser.Card` exposes its own source-file path** (Q2): a new field/method giving the worktree-relative `_lyx/plan/NN-<slug>.md` path, populated during parse (planparser already reads those files and already resolves `_lyx/plan/` via `hubgeometry`). Render reads this field; it never constructs the path itself.
- **Integration-suite fork stops injecting `shared_decisions`** (folded in on the user's "don't inject already-inherited" principle): `integration-template.md` + `RenderIntegrationPrompt` drop the `shared_decisions` injection. The integration fork is only ever spawned in-session by Master (no recovery path exists for it), so it always inherits `00-overview.md`. `verify` and `worktree_root` stay — `verify` is an executable command that must be rendered verbatim, not inherited context.
- **Tests** (`internal/websterengine/template_test.go`, `internal/planparser/*`): flip the two fork-injection assertions, update marker-required lists and fixture marker maps, add coverage for the new recovery template + `planparser.Card` source-path field. See Testing.
- **Docs / decision rename:** rewrite every doc comment across `render.go`, `fork-template.md`, `master-template.md`, `integration-template.md` that names the `fork-prompt-plan-level-context Shared Decision` — that decision is reversed by this task. Rename/supersede it (proposed new name: **`fork-context-hygiene`** — thin in-session prompt, full cold-recovery prompt, card-content-by-pointer). Update the webster module doc under `manifest/designs/` in the same commit if it references the old decision.

**Out:**

- **No change to what a fork/strand actually implements** — only how its prompt is assembled. Card semantics (What/Context/Edits/Creates/Deletes/Moves/Commit/verify) are unchanged; they now reach the agent by the agent reading the card file rather than by Go inlining the fields.
- **No new CONSTRAINTS.md invariant.** The thin-vs-cold prompt split is module-internal design, not a cross-cutting machine-enforced invariant. (The Planparser Sole-Parser and Hub Geometry invariants are respected, not extended.)
- **No change to the batcher, plan format, state.json, or the master loop's verbs** (`begin-batch`/`await-batch`/`record-batch`/`recover-batch`).
- **No whole-repo codebase tour forced on the recovery strand** — "orient to the codebase" means read what its card needs plus the constraint/decision docs, not a gratuitous full tour.
- **`webster-rename-master-merriam`** is a separate task; do not touch its concerns beyond unavoidable same-file coordination.

## Decisions

### split-fork-and-recovery-prompts

- Decision: `RenderForkPrompt` no longer serves both callers. Keep `RenderForkPrompt` + `fork-template.md` for the in-session fork (thinned); add `RenderRecoveryPrompt` + `recovery-template.md` for the cold recovery strand (full context). `recoverbatch.go:216` switches to `RenderRecoveryPrompt`.
- Rationale: the two callers have opposite context situations — the in-session fork inherits Master's whole context; the cold recovery strand is a separate process that inherits nothing (`recoverbatch.go` header; `render.go:186`). One prompt cannot be honest for both: the current shared template claims "you inherit Master's context," which is a lie for the recovery strand. A single prompt that "points to files" for both was considered and rejected by the user: *"Vi kan ikke gi samme prompt til en fork som arver MASSE Context, som vi gir til en cold-start recovery tråd. Det går ikke. Må ha to forskjellige."*
- Rejected: (a) edit `RenderForkPrompt` in place per the task's original list — silently strips the recovery strand's context. (b) One thin template that points to `00-overview.md`/`PATTERN.md` for everyone — the recovery strand can't be told "already in your inherited context" because it has none, and the in-session fork would still do skippable-but-real redundant reads. (c) Implement only fix #1 and abandon #2–#5 — leaves the redundancy in place.

### thin-in-session-fork-prompt

- Decision: the in-session `fork-template.md` drops `shared_decisions`, `rename_mechanic`, and the PATTERN directive entirely — no injection, no pointer. It keeps `self_fix_cap`, `worktree_root`, `prev_digest`, `report_path`, and points to the card file(s). Rendering can switch from `stencil.FillOptional` to plain `stencil.Fill` since no optional (`pattern_directive`) or branch-internal (`rename_mechanic`) marker remains.
- Rationale: the in-session fork inherits `00-overview.md` (Shared Decisions, rename mechanic) and PATTERN from Master, so re-rendering them is pure redundancy; the PATTERN directive additionally forces a redundant `Read` action. `self_fix_cap` is the fork's own operating bound (not merely Master's tuning knob) and `worktree_root` anchors its build/commit commands — both cheap, unambiguous, kept (Q3).
- Rejected: dropping `self_fix_cap`/`worktree_root` too (leans on inheritance for a hard operating bound and cwd — brittle for little gain).

### full-cold-recovery-prompt

- Decision: `recovery-template.md` is a self-contained cold implementer prompt. It honestly frames the strand as starting cold and instructs it to read: `_lyx/plan/00-overview.md` (framing, Shared Decisions, rename mechanic), `_pattern/PATTERN.md` (via the existing PATTERN directive, which renders empty when PATTERN is inactive), `CONSTRAINTS.md`, orient to the codebase, then read and implement its card file(s) with the FRESH-READ rule. Keeps `self_fix_cap`, `worktree_root`, `prev_digest`, `report_path`, the bounded-self-fix and final-report sections.
- Rationale: user Q1 — *"Alt relevant må være med."* The strand inherits nothing, so everything a correct, constraint-respecting implementation needs must be reachable from its prompt. Pointing to canonical files (rather than injecting copies) keeps the prompt readable and drift-free, and the strand reading the actual card file resolves the two crucible-fable-r3 concerns for free: the full `What` prose and any `Commit:` pin are in the card file the strand reads, so neither is silently degraded (the reason `renderCard` inlined the What-fallback and Commit pin no longer applies — the strand reads the source).
- Rejected: "match today's injected set only" (leaves the strand under-oriented on codebase/CONSTRAINTS, same as the current lying template).

### card-content-by-file-pointer

- Decision: both templates point the agent at its batch's card file(s) — `_lyx/plan/NN-<slug>.md`, one per card in the batch — instead of inlining the fields via Go templating. `renderBatchCards`/`renderCard`/`renderFileOpField`/`renderMovesField` are removed. `batchHasMove` becomes dead (it only gated `rename_mechanic` injection) and is removed. `noSharedDecisions` becomes dead (no template injects `shared_decisions`) and is removed.
- Rationale: user Q2 — *"Rendret innhold skal til fil. I den fila står det at forken skal lese card (path til card)."* Card content is per-batch and was never inherited (Master, post-fix-#1, reads only the overview; even before, the fork got it via the rendered `cards` marker, not inheritance), so pointing to the single source of truth avoids the hand-rolled field-formatting drift risk and reads the real `What`/`Commit`/`verify` verbatim.
- Rejected: keep inlining via `renderBatchCards` (drift risk; duplicates on-disk content the agent can read directly).

### card-source-path-in-planparser

- Decision: `planparser.Card` gains a source-file-path field/method (worktree-relative `_lyx/plan/NN-<slug>.md`), populated during parse. Render reads it; render never constructs a plan path.
- Rationale: user Q2 — the Planparser Sole-Parser Invariant makes `_lyx/plan/NN-<card-slug>.md` planparser's domain, and the Hub Geometry Invariant reserves the `_lyx` token to `hubgeometry`. planparser already reads each card file and already resolves `_lyx/plan/` via `hubgeometry`, so it is the correct owner of the per-card path. Constructing `NN-<slug>.md` in `render.go` would put plan-path knowledge (and the `_lyx` token) outside planparser.
- Rejected: `render.go` reconstructs the path via `hubgeometry` (fewer files, but violates the ownership spirit of both invariants).

### integration-fork-drops-shared-decisions

- Decision: `integration-template.md` + `RenderIntegrationPrompt` drop the `shared_decisions` injection; `verify` and `worktree_root` stay.
- Rationale: the integration-suite fork is only ever spawned in-session by Master and has no recovery path, so it always inherits `00-overview.md`. Consistent with the task's core principle. `verify` stays because it is the exact command the fork must run verbatim, not inherited orientation context.
- Rejected: leave the integration fork untouched (inconsistent — same redundancy the task exists to remove). If the user vetoes this fold-in, drop this bullet and the corresponding Testing item; nothing else depends on it.

## Technical context

- **`internal/websterengine/render.go`** (393 lines): owns the three `go:embed` templates and their render functions. `RenderForkPrompt` (line 122) fills `fork-template.md` via `stencil.FillOptional` with `pattern_directive` optional; `RenderIntegrationPrompt` (line 261); `RenderMasterPrompt` (line 318). The header doc comment (lines 14–20) and several function comments name the `fork-prompt-plan-level-context Shared Decision` that this task reverses.
- **Two callers of `RenderForkPrompt`:** `beginbatch.go:218` (in-session fork; keeps calling the thinned `RenderForkPrompt`) and `recoverbatch.go:216` (cold recovery strand; switches to `RenderRecoveryPrompt`). Both currently pass `(deps.Plan, batch, prevDigest, reportPath, deps.Layout, deps.Config.SelfFixCap)`. `RenderRecoveryPrompt` should take the same parameter shape.
- **`recoverbatch.go`** spawns the strand at `RoleRecovery` via `deps.Starter.Start(spec)` with `spec.Prompt = string(prompt)` — a genuinely separate process; the rendered prompt is its entire instruction. This is why the recovery prompt must be self-contained.
- **`stencil.Fill` / `FillOptional`** (`internal/stencil/stencil.go`): every top-level `{{.X}}` marker in a template must be filled non-empty, except names listed as optional in `FillOptional`. To drop a marker you must remove BOTH the `{{.X}}` from the template AND its map entry. `rename_mechanic` is today a branch-internal marker (inside `{{if .rename_mechanic}}`); once dropped from the thin fork it disappears from `fork-template.md` entirely.
- **`internal/pattern`** (`pattern.go`): `Directive(l, RoleImplementer)` returns the `implementerDirective` checklist (*"STOP. Read _pattern/PATTERN.md in full before editing…"*) when PATTERN is active for `l`, else `""`. The cold recovery template reuses this (via `FillOptional`) — it IS the "read PATTERN.md" cold-start instruction. The thin fork template drops it. `pattern` is a leaf package (Pattern Leaf Invariant) — do not add feature-package imports to it.
- **`planparser.Card`** (`plan.go:64`): carries `Number`, `Slug` (the `<card-slug>` that names the file `NN-<Slug>.md`, line 79), `Intent`, `What`, the typed file-op fields, `Commit`, `Verify`. Add the source-path field here. planparser is the SOLE parser of `_lyx/plan/` (Planparser Sole-Parser Invariant) and resolves the dir via `hubgeometry`.
- **Card-file path spelling:** the pointer inside the prompt is worktree-relative `_lyx/plan/NN-<slug>.md`, matching `master-template.md`'s own `ls _lyx/plan/` idiom. The agent runs in the worktree and reaches the weft through the `_lyx` junction; never reference the physical `-weft` path (webster's `weft-reference` audit hard-fails on it).
- **A batch may hold multiple cards** (batcher grouping; default `identity` = one card per batch). The pointer render must list every card file in the batch, in declared order.

## Constraints

From `CONSTRAINTS.md` (hub root), the ones this task touches:

- **Planparser Sole-Parser Invariant** — `internal/planparser` is the sole parser of `_lyx/plan/` and resolves the dir via `hubgeometry`. The new `Card` source-path field lives here; render must not parse or construct plan paths.
- **Hub Geometry Invariant** — the `_lyx` token is owned solely by `hubgeometry`; no other package may use it in a path-construction context. The card-file path must originate from planparser/hubgeometry, never a `render.go` string literal.
- **Pattern Leaf Invariant** — `internal/pattern` imports only stdlib + `hubgeometry`. Reuse `pattern.Directive` as-is; do not extend `pattern` with new imports.
- **Fabric Git Invariant** — untouched here (no git mutation added); the fork/strand still commits its own code to the host repo via normal dev git per card, never the weft.
- **CLI/Cobra Invariant** — untouched (no CLI surface change).
- **Markdown rule (project):** the two new/edited template `.md` files and this discussion follow one-line-per-paragraph, no hard-wrap.

Discovered during discussion: no new invariant is required; the thin-vs-cold split is module-internal design recorded in the webster module doc + code comments, not `CONSTRAINTS.md`.

## Testing

Language: Go. Gate per the `golang-build`/`golang-testing` skills: `go build ./...` plus the affected packages' unit tests. Real execution required (not plan review) — run the tests.

Per-area approach:

- **`internal/planparser`** — TDD the new `Card` source-path field: parse a fixture plan and assert each card's path is the worktree-relative `_lyx/plan/NN-<slug>.md` resolved through `hubgeometry` (not a raw literal). Cover the multi-card and single-card cases.
- **`internal/websterengine` `RenderForkPrompt` (thin)** — flip `TestRenderForkPrompt_InjectsSharedDecisionsAlways` (line 732) and `TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch` (line 764): assert the thin fork prompt does NOT contain Shared Decisions / rename-mechanic / PATTERN-directive content, and DOES contain a pointer to each card file. Update the required-marker deletion sweep (marker list at line 495) and `forkTemplateMarkerValues()` (line 168) to the thin marker set (`report_path`, `self_fix_cap`, `worktree_root`, `prev_digest`, plus the card-file-pointer marker). If the thin render switches to plain `stencil.Fill`, drop the `FillOptional` optional-`pattern_directive` sub-tests for the fork (`TestForkTemplate_PatternDirectiveOptional` and the empty-directive/empty-rename orphan-heading case).
- **`internal/websterengine` `RenderRecoveryPrompt` (new)** — new tests mirroring the fork tests: assert the recovery prompt instructs reading `00-overview.md`, `CONSTRAINTS.md`, and (PATTERN active) `PATTERN.md`; contains the card-file pointer(s); renders cleanly with PATTERN inactive (empty directive, no orphan `## Constraints` heading); errors/marker-required behavior matches its own marker set.
- **`internal/websterengine` `RenderIntegrationPrompt`** — `TestRenderIntegrationPrompt_InjectsVerifyText` (line 797) and `_EmptyVerifyErrors` (line 810) stay green (verify unchanged). Add/adjust to assert the integration prompt no longer injects Shared Decisions. (Drop this item if the user vetoes the integration fold-in.)
- **`internal/websterengine` `master-template.md`** — the master marker-required tests are unaffected (`self_fix_cap` stays). Optionally assert the orientation section no longer says "every card file."
- **Removed-code cleanup** — delete the `renderBatchCards`/`renderCard`/`renderFileOpField`/`renderMovesField` tests along with the functions; delete `batchHasMove` and `noSharedDecisions` tests/consts.
- **Plan-level `verify:` for this task** (drives webster's own integration stage): `go build ./... && go test ./internal/websterengine/... ./internal/planparser/...`.

## Q&A log

- **Q:** `RenderForkPrompt` feeds both the in-session fork (inherits context) and the cold recovery strand (inherits nothing) — how do we drop the redundant injections without stripping the strand's only context? **A:** Split into two prompts — thin `fork-template.md`/`RenderForkPrompt` for the in-session fork, full new `recovery-template.md`/`RenderRecoveryPrompt` for the cold strand. User: *"Må ha to forskjellige."*
- **Q:** How does a fork/strand get its per-batch card content? **A:** By pointer — the rendered prompt tells it to read `_lyx/plan/NN-<slug>.md`; stop inlining the fields. Applies to both templates.
- **Q:** Who builds the `_lyx/plan/NN-<slug>.md` path? **A:** `planparser.Card` exposes its own source path (respects the Sole-Parser + Hub Geometry invariants); render reads it.
- **Q:** Keep `self_fix_cap` and `worktree_root` in the thin fork prompt? **A:** Keep both — `self_fix_cap` is the fork's own operating bound, `worktree_root` anchors build/commit; cheap and unambiguous.
- **Q:** How full is the cold recovery prompt? **A:** All relevant context included — read `00-overview.md`, `PATTERN.md`, `CONSTRAINTS.md`, orient to the codebase, then read + implement its card file(s). User: *"Alt relevant må være med."*
- **Q:** Integration-suite fork's redundant `shared_decisions` injection — in scope? **A:** Folded in (drop it) on the "don't inject already-inherited" principle; the integration fork is always in-session. Flagged for user veto.
