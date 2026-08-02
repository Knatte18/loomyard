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

**Critical finding from exploration (reshapes the task):** `RenderForkPrompt`'s output is used by **two** callers with opposite context situations. `beginbatch.go:218` feeds the **in-session fork** (inherits everything). `recoverbatch.go:216` feeds the **cold recovery strand** — `recoverbatch.go`'s own header calls it *"the only place webster spawns a genuinely separate process… rendering the SAME fork prompt a fork would have gotten"*, and `render.go:186` says *"a cold recovery strand's rendered prompt is its whole instruction."* The recovery strand inherits **nothing**. So `shared_decisions` / `rename_mechanic` / PATTERN are redundant for the in-session fork but **load-bearing** for the cold recovery strand. Naively editing `RenderForkPrompt` in place — as the task's original "Confirmed redundant" list reads — would strip the recovery strand's only source of that context. Resolution (user decision): **split into two prompts** — a thin in-session fork prompt and a full, honest cold-start recovery prompt — built for **maximum reuse** so the large shared implementer-job body has a single source of truth. This also fixes a latent bug: today the recovery strand is handed the fork template that literally claims *"You never start cold: you inherit Master's whole context"* — false for that strand.

## Scope

**In:**

- **Master reads only `00-overview.md`** (fix #1): `master-template.md` line 29 changes from "read the whole plan — every card file, not just the overview — once" to reading only `00-overview.md` (task framing, Card Index, `## Shared Decisions`, `## Rename mechanic`, `## verify:`). Update the line-85 parenthetical ("you already read the whole plan at orientation") to match (Master read the overview, not every card file).
- **Two prompts sharing one implementer-job body** (the split, built for reuse):
  - **Shared implementer-job body** — the block both an in-session fork and a cold recovery strand execute identically: implement each card in order, build+test per card, commit per card to the host repo, bounded self-fix, write the minimal batch report. Lives in exactly one place (see the `shared-implementer-body` Decision for the mechanism), carrying the per-batch/per-invocation markers (`report_path`, `self_fix_cap`, `worktree_root`, `prev_digest`) and the card-file pointer(s).
  - **In-session fork prompt** = **inherited-context prefix** + shared body. Prefix drops `shared_decisions`, `rename_mechanic`, and the PATTERN directive entirely (all inherited); honest framing "you inherit Master's context." Rendered by `RenderForkPrompt` (thinned), called by `beginbatch.go:218`.
  - **Cold recovery prompt** = **cold-start prefix** + shared body. Prefix honestly frames the strand as starting cold and instructs it to read `_lyx/plan/00-overview.md` (framing, Shared Decisions, rename mechanic), `_pattern/PATTERN.md` (via the existing optional PATTERN directive), `CONSTRAINTS.md`, and orient to the codebase, before the shared body runs. Rendered by a **new** `RenderRecoveryPrompt`, called by `recoverbatch.go:216` (switched from `RenderForkPrompt`).
- **Card content delivered by relative pointer, in both prompts:** replace the inlined `renderBatchCards`/`renderCard` output with a rendered list of the batch's card-file paths. The path is **always relative**, produced through `hubgeometry` (see the `card-pointer-relative-via-hubgeometry` Decision); the fork/strand reads the card file itself.
- **`planparser.Card` exposes its own worktree-relative source identity** (Q2): a new field/method giving the worktree-relative `_lyx/plan/NN-<slug>.md` token, built from `hubgeometry.LyxDirName` and the card's own `NN-<Slug>.md` filename — **independent of the absolute `Plan.Dir`**. Populated during parse (planparser already reads those files and already resolves `_lyx/plan/` via `hubgeometry`).
- **Integration-suite fork stops injecting `shared_decisions`** (folded in on the user's "don't inject already-inherited" principle): `integration-template.md` + `RenderIntegrationPrompt` drop the `shared_decisions` injection. The integration fork is only ever spawned in-session by Master (no recovery path exists for it), so it always inherits `00-overview.md`. `verify` and `worktree_root` stay — `verify` is an executable command that must be rendered verbatim, not inherited context.
- **Dead-code cleanup:** `renderBatchCards`/`renderCard`/`renderFileOpField`/`renderMovesField` removed (card content now by pointer). `batchHasMove` removed (it only gated `rename_mechanic` injection). `noSharedDecisions` removed **only if** the integration fold-in survives veto (it is still used by `RenderIntegrationPrompt` at `render.go:269`; if the fold-in is dropped, the const stays).
- **Tests** (`internal/websterengine/template_test.go`, `internal/planparser/*`): flip the two fork-injection assertions, update marker-required lists and fixture marker maps, add coverage for the new recovery prompt, the shared body, and the `planparser.Card` source-path field. See Testing.
- **Docs / decision rename:** rewrite every doc comment across `render.go`, `fork-template.md`, `master-template.md`, `integration-template.md` that names the `fork-prompt-plan-level-context Shared Decision` — that decision is reversed by this task. Rename/supersede it (proposed new name: **`fork-context-hygiene`** — thin in-session prompt, full cold-recovery prompt, shared implementer body, card-content-by-relative-pointer). Update the webster module doc under `manifest/designs/` in the same commit if it references the old decision.

**Out:**

- **No change to what a fork/strand actually implements** — only how its prompt is assembled. Card semantics (What/Context/Edits/Creates/Deletes/Moves/Commit/verify) are unchanged; they now reach the agent by the agent reading the card file rather than by Go inlining the fields.
- **No new CONSTRAINTS.md invariant.** The thin-vs-cold split and shared-body reuse are module-internal design, not a cross-cutting machine-enforced invariant. (The Planparser Sole-Parser and Hub Geometry invariants are respected, not extended.)
- **No change to the batcher, plan format, state.json, or the master loop's verbs** (`begin-batch`/`await-batch`/`record-batch`/`recover-batch`).
- **No whole-repo codebase tour forced on the recovery strand** — "orient to the codebase" means read what its card needs plus the constraint/decision docs, not a gratuitous full tour.
- **`webster-rename-master-merriam`** is a separate task; do not touch its concerns beyond unavoidable same-file coordination.

## Decisions

### split-fork-and-recovery-prompts

- Decision: `RenderForkPrompt` no longer serves both callers. Keep `RenderForkPrompt` for the in-session fork (thinned); add `RenderRecoveryPrompt` for the cold recovery strand. `recoverbatch.go:216` switches to `RenderRecoveryPrompt`.
- Rationale: the two callers have opposite context situations — the in-session fork inherits Master's whole context; the cold recovery strand is a separate process that inherits nothing (`recoverbatch.go` header; `render.go:186`). One prompt cannot be honest for both: the current shared template claims "you inherit Master's context," a lie for the recovery strand. User: *"Vi kan ikke gi samme prompt til en fork som arver MASSE Context, som vi gir til en cold-start recovery tråd. Det går ikke. Må ha to forskjellige."*
- Rejected: (a) edit `RenderForkPrompt` in place per the task's original list — silently strips the recovery strand's context. (b) One thin template pointing to files for everyone — the recovery strand can't be told "already in your inherited context." (c) Implement only fix #1 and abandon #2–#5 — leaves the redundancy in place.

### shared-implementer-body

- Decision: the identical implementer-job body (implement each card → build+test+commit per card → bounded self-fix → minimal report) has one source of truth, composed into both prompts by **Go byte-level composition**: `RenderForkPrompt` composes `fork-prefix + shared-body` and Fills; `RenderRecoveryPrompt` composes `recovery-prefix + shared-body` and Fills. Suggested asset layout: embed `fork-prefix.md`, `recovery-prefix.md`, and `implementer-body.md` (or hold the body as a Go constant) — mill-plan picks the exact file/constant split. The two prompts differ only in their prefix.
- Rationale: user Q2 — *"jeg tror disse templatene bør skrives slik at det blir mye gjenbruk."* The implementer body is the large shared block (the Master-vs-recovery overlap the user first noticed is actually the smaller cold-orientation preamble; the big overlap is recovery-vs-fork's job body). Factoring kills drift: a change to the implementer contract lands once, not in two files. **Mechanism constraint:** `internal/stencil` is single-pass `text/template` with **no `{{template}}` include support** (verified in `stencil.go` — one template named "stencil" is parsed, no `ParseFiles`/associated templates). So reuse must be byte-composition of the shared body as template *text* before `Fill` — NOT injecting a pre-rendered body as a `{{.marker}}` value, since text/template does not recursively expand markers inside a substituted value, so the body's own `{{.report_path}}` etc. would pass through unfilled. Composing raw bytes keeps the body's markers in the template text where `Fill` resolves them normally.
- Rejected: (a) two fully independent templates kept in sync by review discipline (drift risk the user explicitly wants to avoid). (b) `{{template}}` partials (unsupported by stencil). (c) inject the body as a pre-filled marker value (its inner markers wouldn't fill — a single-pass limitation).

### thin-in-session-fork-prompt

- Decision: the in-session fork prefix drops `shared_decisions`, `rename_mechanic`, and the PATTERN directive entirely — no injection, no pointer. The composed fork prompt keeps `self_fix_cap`, `worktree_root`, `prev_digest`, `report_path` (all in the shared body) and the card-file pointer. If no optional (`pattern_directive`) or branch-internal (`rename_mechanic`) marker remains in the composed fork bytes, rendering can use plain `stencil.Fill`.
- Rationale: the in-session fork inherits `00-overview.md` (Shared Decisions, rename mechanic) and PATTERN from Master, so re-rendering them is pure redundancy; the PATTERN directive additionally forces a redundant `Read` action. `self_fix_cap` is the fork's own operating bound (not merely Master's tuning knob) and `worktree_root` anchors its build/commit commands — both cheap, unambiguous, kept (Q3).
- Rejected: dropping `self_fix_cap`/`worktree_root` too (leans on inheritance for a hard operating bound and cwd — brittle for little gain).

### full-cold-recovery-prompt

- Decision: the cold-start prefix honestly frames the strand as starting cold and instructs it to read `_lyx/plan/00-overview.md` (framing, Shared Decisions, rename mechanic), `_pattern/PATTERN.md` (via the existing PATTERN directive, which renders empty when PATTERN is inactive), `CONSTRAINTS.md`, and orient to the codebase, before the shared body runs. It keeps `self_fix_cap`, `worktree_root`, `prev_digest`, `report_path`.
- Rationale: user Q1 (round 2) — *"Alt relevant må være med."* The strand inherits nothing, so everything a correct, constraint-respecting implementation needs must be reachable from its prompt. Pointing to canonical files (rather than injecting copies) keeps the prompt readable and drift-free, and the strand reading the actual card file resolves the two crucible-fable-r3 concerns for free: the full `What` prose and any `Commit:` pin are in the card file the strand reads, so neither is silently degraded (the reason `renderCard` inlined the What-fallback and Commit pin no longer applies — the strand reads the source).
- Rejected: "match today's injected set only" (leaves the strand under-oriented on codebase/CONSTRAINTS, same as the current lying template).

### card-pointer-relative-via-hubgeometry

- Decision: the card pointer rendered into both prompts is **always a relative path**, produced through `hubgeometry`, RelPath-aware — never absolute, never a literal `_lyx` string join in `render.go`. `planparser.Card` exposes the card's worktree-relative identity (`_lyx/plan/NN-<slug>.md` via `hubgeometry.LyxDirName`, independent of the absolute `Plan.Dir`); `render.go` (which holds the `Layout`) produces the final relative pointer via `hubgeometry`, relative to the consumer's `Cwd`, using `hubgeometry.PlanDir(l.WorktreeRoot)` + the card filename (never a literal `_lyx`).
- Rationale: user — *"Det er relative path! ALLTID. Derfor har vi hubgeometry."* This closes both reviewer gaps at once: (GAP 1) the token is relative, not absolute-`Dir`-derived, so the weft-audit-safe `_lyx/plan/NN-<slug>.md` form is what renders and what the test asserts; (GAP 2) the pointer is relative to `Cwd`, and the cold recovery strand's launch cwd **is** `layout.Cwd` — reed launches every pane with `-c e.layout.Cwd` (`shuttleengine/wait.go:311-315`), and `render.go:103-110` fills `worktree_root` from `l.Cwd` — the exact base the relative path is computed against. So it resolves for the in-session fork (cwd = Master's `Cwd`) and the cold strand (cwd = `layout.Cwd`) alike, is RelPath-robust, and goes through the `_lyx` junction, never the physical `-weft` path (the weft-reference audit fails only on the physical path). No `worktree_root` string-anchor concept is introduced — `hubgeometry` owns the geometry.
- Rejected: (a) absolute `_lyx`-junction path — resolves from any cwd but bakes machine-specific absolute paths into the prompt and the test fixture. (b) deriving the path from the absolute `Plan.Dir` — the reviewer's GAP-1 defect (absolute in prod, `t.TempDir()` in tests).

### card-source-identity-in-planparser

- Decision: `planparser.Card` gains a worktree-relative source-identity field/method (`_lyx/plan/NN-<slug>.md` via `hubgeometry.LyxDirName`), populated during parse. `render.go` reads it and composes the final `Cwd`-relative pointer via `hubgeometry`; `render.go` never constructs a plan path from a literal `_lyx`.
- Rationale: user Q2 (batch 2) — the Planparser Sole-Parser Invariant makes `_lyx/plan/NN-<card-slug>.md` planparser's domain, and the Hub Geometry Invariant reserves the `_lyx` token to `hubgeometry`. planparser already reads each card file and resolves `_lyx/plan/` via `hubgeometry`, so it is the correct owner of the per-card relative identity.
- Rejected: `render.go` reconstructs the path from a literal `_lyx` (violates both invariants' ownership).

### integration-fork-drops-shared-decisions

- Decision: `integration-template.md` + `RenderIntegrationPrompt` drop the `shared_decisions` injection; `verify` and `worktree_root` stay. `noSharedDecisions` const removal is contingent on this decision surviving veto.
- Rationale: the integration-suite fork is only ever spawned in-session by Master and has no recovery path, so it always inherits `00-overview.md`. Consistent with the task's core principle. `verify` stays because it is the exact command the fork must run verbatim, not inherited orientation context.
- Rejected: leave the integration fork untouched (inconsistent — same redundancy the task exists to remove). If the user vetoes this fold-in, drop this bullet, keep `noSharedDecisions`, and drop the corresponding Testing item; nothing else depends on it.

## Technical context

- **`internal/websterengine/render.go`** (393 lines): owns the `go:embed` templates and their render functions. `RenderForkPrompt` (line 122) fills `fork-template.md` via `stencil.FillOptional` with `pattern_directive` optional; `RenderIntegrationPrompt` (line 261); `RenderMasterPrompt` (line 318). The header doc comment (lines 14–20) and several function comments name the `fork-prompt-plan-level-context Shared Decision` this task reverses.
- **Two callers of `RenderForkPrompt`:** `beginbatch.go:218` (in-session fork; keeps calling the thinned `RenderForkPrompt`) and `recoverbatch.go:216` (cold recovery strand; switches to `RenderRecoveryPrompt`). Both currently pass `(deps.Plan, batch, prevDigest, reportPath, deps.Layout, deps.Config.SelfFixCap)`. `RenderRecoveryPrompt` should take the same parameter shape.
- **`recoverbatch.go`** spawns the strand at `RoleRecovery` via `deps.Starter.Start(spec)` with `spec.Prompt = string(prompt)` — a genuinely separate process; the rendered prompt is its entire instruction. This is why the recovery prompt must be self-contained.
- **`internal/stencil`** (`stencil.go`): `Fill`/`FillOptional` do single-pass `text/template` substitution with a required-top-level-marker guard; `FillOptional`'s `optional` list exempts a marker (used today for `pattern_directive`). **No `{{template}}` include support** — reuse of the shared body must be Go byte-composition (see `shared-implementer-body`). Dropping a marker means removing BOTH the `{{.X}}` and its map entry.
- **`internal/pattern`** (`pattern.go`): `Directive(l, RoleImplementer)` returns the `implementerDirective` checklist (*"STOP. Read _pattern/PATTERN.md in full before editing…"*) when PATTERN is active for `l`, else `""`. The cold recovery prefix reuses this (via `FillOptional`) — it IS the "read PATTERN.md" cold-start instruction. The thin fork prefix drops it. `pattern` is a leaf package (Pattern Leaf Invariant) — do not add feature-package imports.
- **`planparser`** (`plan.go`): `Plan.Dir` is `hubgeometry.PlanDir(...)` — **absolute** in prod, a `t.TempDir()` in tests (`plan.go:14-17`). `Card` (`plan.go:64`) carries `Number`, `Slug` (names the file `NN-<Slug>.md`, line 79), `Intent`, `What`, the typed file-op fields, `Commit`, `Verify`. The new source-identity field must be built relative (from `hubgeometry.LyxDirName`), not derived from the absolute `Dir`.
- **`hubgeometry`** (`hubgeometry.go`): `Layout` carries `WorktreeRoot`, `Cwd`, `RelPath` (= `filepath.Rel(WorktreeRoot, Cwd)`, lines 69/77/155). `PlanDir(baseDir) = filepath.Join(baseDir, LyxDirName, "plan")` (line 296) — always at the worktree root (lines 305-307). `LyxDirName = "_lyx"` (line 24). `render.go` computes the `Cwd`-relative pointer from these.
- **`shuttleengine/wait.go:311-315`**: *"reed launches every pane with `-c e.layout.Cwd`"* — the recovery strand's process cwd is `layout.Cwd`, the base the relative card pointer resolves against.
- **A batch may hold multiple cards** (batcher grouping; default `identity` = one card per batch). The pointer render must list every card file in the batch, in declared order.

## Constraints

From `CONSTRAINTS.md` (hub root), the ones this task touches:

- **Planparser Sole-Parser Invariant** — `internal/planparser` is the sole parser of `_lyx/plan/` and resolves the dir via `hubgeometry`. The new `Card` source-identity field lives here; render must not parse or construct plan paths from a literal `_lyx`.
- **Hub Geometry Invariant** — the `_lyx` token is owned solely by `hubgeometry`; no other package may use it in a path-construction context. The card-file path must originate from `hubgeometry` (`LyxDirName`/`PlanDir`), never a `render.go` string literal.
- **Pattern Leaf Invariant** — `internal/pattern` imports only stdlib + `hubgeometry`. Reuse `pattern.Directive` as-is; do not extend `pattern` with new imports.
- **Fabric Git Invariant** — untouched here (no git mutation added); the fork/strand still commits its own code to the host repo via normal dev git per card, never the weft. The card pointer goes through the `_lyx` junction, never the physical `-weft` path (webster's `weft-reference` audit).
- **CLI/Cobra Invariant** — untouched (no CLI surface change).
- **Markdown rule (project):** the new/edited template `.md` assets and this discussion follow one-line-per-paragraph, no hard-wrap.

Discovered during discussion: no new invariant is required; the thin-vs-cold split and shared-body reuse are module-internal design recorded in the webster module doc + code comments, not `CONSTRAINTS.md`.

## Testing

Language: Go. Gate per the `golang-build`/`golang-testing` skills: `go build ./...` plus the affected packages' unit tests. Real execution required (not plan review) — run the tests.

Per-area approach:

- **`internal/planparser`** — TDD the new `Card` source-identity field: parse a fixture plan and assert each card's field is the worktree-relative `_lyx/plan/NN-<slug>.md` token built via `hubgeometry.LyxDirName` (NOT the absolute `Plan.Dir`, and NOT a `t.TempDir()` prefix). Cover multi-card and single-card cases.
- **`internal/websterengine` shared body + `RenderForkPrompt` (thin)** — flip `TestRenderForkPrompt_InjectsSharedDecisionsAlways` (line 732) and `TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch` (line 764): assert the composed fork prompt does NOT contain Shared Decisions / rename-mechanic / PATTERN-directive content, and DOES contain a relative pointer to each card file. Assert the card pointer is relative (no absolute prefix, no `t.TempDir()` leak). Update the required-marker deletion sweep (marker list at line 495) and `forkTemplateMarkerValues()` (line 168) to the composed thin marker set (`report_path`, `self_fix_cap`, `worktree_root`, `prev_digest`, plus the card-pointer marker). If the composed thin bytes carry no optional marker, drop the fork's `FillOptional` optional-`pattern_directive` sub-tests (`TestForkTemplate_PatternDirectiveOptional` and the empty-directive/empty-rename orphan-heading case).
- **`internal/websterengine` `RenderRecoveryPrompt` (new)** — new tests mirroring the fork tests: assert the recovery prompt instructs reading `00-overview.md`, `CONSTRAINTS.md`, and (PATTERN active) `PATTERN.md`; contains the same relative card pointer(s) and the shared implementer-body text; renders cleanly with PATTERN inactive (empty directive, no orphan `## Constraints` heading); marker-required behavior matches its composed marker set. Add a test asserting the fork and recovery prompts share byte-identical implementer-body text (the reuse guarantee).
- **`internal/websterengine` `RenderIntegrationPrompt`** — `TestRenderIntegrationPrompt_InjectsVerifyText` (line 797) and `_EmptyVerifyErrors` (line 810) stay green (verify unchanged). Add/adjust to assert the integration prompt no longer injects Shared Decisions. (Drop this item if the user vetoes the integration fold-in.)
- **`internal/websterengine` `master-template.md`** — the master marker-required tests are unaffected (`self_fix_cap` stays). Optionally assert the orientation section no longer says "every card file."
- **Removed-code cleanup** — delete the `renderBatchCards`/`renderCard`/`renderFileOpField`/`renderMovesField` tests along with the functions; delete `batchHasMove` tests; delete `noSharedDecisions` tests/const only if the integration fold-in survives.
- **Plan-level `verify:` for this task** (drives webster's own integration stage): `go build ./... && go test ./internal/websterengine/... ./internal/planparser/...`.

## Q&A log

- **Q:** `RenderForkPrompt` feeds both the in-session fork (inherits context) and the cold recovery strand (inherits nothing) — how do we drop the redundant injections without stripping the strand's only context? **A:** Split into two prompts — thin in-session `RenderForkPrompt` for the fork, full new `RenderRecoveryPrompt` for the cold strand. User: *"Må ha to forskjellige."*
- **Q:** How does a fork/strand get its per-batch card content? **A:** By pointer — the rendered prompt tells it to read `_lyx/plan/NN-<slug>.md`; stop inlining the fields. Applies to both prompts.
- **Q:** Who builds the `_lyx/plan/NN-<slug>.md` path, and is it relative or absolute? **A:** Always relative, owned by `hubgeometry` (user: *"Det er relative path! ALLTID. Derfor har vi hubgeometry."*). `planparser.Card` exposes the worktree-relative identity (`LyxDirName`-based, not the absolute `Dir`); `render.go` composes the `Cwd`-relative pointer via `hubgeometry`. Closes reviewer GAP 1 (relativity) and GAP 2 (strand cwd = `layout.Cwd`, reed `-c layout.Cwd`).
- **Q:** Keep `self_fix_cap` and `worktree_root` in the fork prompt? **A:** Keep both — `self_fix_cap` is the fork's own operating bound, `worktree_root` anchors build/commit; cheap and unambiguous.
- **Q:** How full is the cold recovery prompt? **A:** All relevant context included — read `00-overview.md`, `PATTERN.md`, `CONSTRAINTS.md`, orient to the codebase, then run the shared implementer body over its card file(s). User: *"Alt relevant må være med."*
- **Q:** The thin fork and the cold recovery strand share the whole implementer-job body — factor it or duplicate it? **A:** Factor for maximum reuse (user: *"jeg tror disse templatene bør skrives slik at det blir mye gjenbruk."*). Mechanism: Go byte-composition of a shared embedded body (stencil has no `{{template}}` include support); the two prompts differ only in prefix.
- **Q:** Integration-suite fork's redundant `shared_decisions` injection — in scope? **A:** Folded in (drop it) on the "don't inject already-inherited" principle; the integration fork is always in-session. `noSharedDecisions` removal made contingent on this surviving veto. Flagged for user veto.
