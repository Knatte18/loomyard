# Discussion: Built-in operator console pane in mux

```yaml
task: Built-in operator console pane in mux
slug: mux-operator-console
status: discussing
parent: main
```

## Problem

During the session-fork-diversity-spike (2026-07-16), removing mux strands down to the
last one tore down the whole tmux session — tmux kills a session when its last pane dies
— which killed a running test mid-flight. The ad-hoc fix was a manually added keepalive
strand running a bare `bash` in the hub root.

**Why now:** the teardown is a live footgun (it already broke a test). The spike left an
operator-endorsed design note (`docs/research/session-fork-spike.md:233-239`) to make the
keepalive a built-in mux feature.

## Scope

**In:**

- A new built-in **header** construct in mux: a small, always-present tmux/psmux pane
  pinned at the **top**, at a **config-driven fixed height** (default 1 row).
- The header pane runs a new command `lyx mux header`, which prints text defined by mux
  config and then **hangs** so the pane never exits. It is the pane that keeps the session
  alive even when every strand is dead.
- The header is **NOT a strand**: separate construct, not in the `Strands` slice, excluded
  from all strand accounting (`status`, `resume`, `remove`, count).
- v1: a **single** header spanning full width at the top; the strand stack is pushed down
  below it (`box.Y += header height`). "One header per column" is the invariant for a future
  multi-column layout.
- A new pinned fixed-height **top band** in the render layer (no such anchor exists today).
- Header text is produced by a three-part pipeline: a new **`internal/tokenvocab`** module
  (builds the token→value map), the existing **`internal/stencil`** module (fills the
  template), and a **super-simple `lyx mux header`** command (glue).
- `internal/tokenvocab` is **general and shared** — loom will consume the same vocabulary.

**Out:**

- Interactive scratch-terminal use — **dropped**. The header is pure display.
- A brand-new substitution engine — **rejected**; reuse `internal/stencil` for the fill step.
- Multi-column render layout — **deferred** (render is single-column today; YAGNI).
- The `slug` token — **deferred** (added when a task-scoped need arises).
- General pane naming/tagging as a mux feature (separate spike gap, not this task).

## Decisions

### header-is-not-a-strand (BINDING — user-stated)

- Decision: The header is a first-class but separate construct, **never** a strand. Persist
  its pane id in `MuxState` (new field, e.g. `HeaderPaneID`) **outside** the `Strands` slice.
  Special-case it through boot / reconcile / apply / status.
- Rationale: User requires it not be a strand; keeps strand accounting clean.
- Rejected: hidden/special strand (user rejected); own-window keepalive (not a visible band).

### pure-header (Q1)

- Decision: Display-only. `lyx mux header` prints the config-defined, token-substituted text
  and then blocks so the pane stays alive. No interactive use.
- Rationale: a 1-row header cannot host manual jobs; the keepalive value is the long-lived
  process, not operator interaction.
- Rejected: scratch terminal.

### three-part text pipeline: tokenvocab + stencil + thin glue (Q2, Q6, Q7, Q13)

The header text is produced by three pieces, each with one job:

1. **`lyx mux header` (new muxcli verb) — SUPER-SIMPLE glue.** It does no work of its own; it
   delegates to a single engine method and then prints + blocks. Per the CLI-Cobra invariant
   (thin verb → one engine method), the assembly lives in **`Engine.HeaderText()`** (muxengine):
   read the `header:` config template (or embedded default) → `tokenvocab.Render(template, ctx)`
   → return text. The engine already holds `layout *hubgeometry.Layout`, so it can build `ctx`.
   The verb prints the returned text once and blocks forever.

2. **`internal/tokenvocab` (NEW, general/shared) — the vocabulary + the reusable compose.**
   Owns the *definition* of the vocabulary as a **registry of `Token{Name string; Resolve
   func(ctx) string}`** — adding a token = append one entry (testable per token). `ctx` wraps
   `*hubgeometry.Layout` (v1). Exposes `Build(ctx) map[string]string` and a convenience
   `Render(template []byte, ctx) ([]byte, error)` = `stencil.Fill(template, Build(ctx))`, so
   every consumer (mux header, **loom**) composes with one call instead of re-implementing it.
   v1 exposes only **always-resolvable** tokens (`repo`, `hub`); `slug` (task slug) is deferred
   with its own empty-value policy then. `repo` is derived from the layout (basename of
   `WorktreeRoot`, or via the `HubSuffix` convention — impl detail for mill-plan).

3. **`internal/stencil` (EXISTING) — the fill step.** `Fill(template, values)` (`stencil.go:36`),
   strict unfilled-marker detection. Reused as-is; template uses `{{.token}}` markers.

- **Dependency graph (acyclic, no cycle):** `stencil` → stdlib only (stays a pure leaf — the
  vocabulary is deliberately NOT inside stencil). `tokenvocab` → `{hubgeometry, stencil}`.
  `muxengine`/`muxcli` and `loom` → `{tokenvocab, ...}`. One-directional.
- Rejected: a single new `<TOKEN_NAME>` module doing map-build + fill together; putting the
  map-build inside the header command or the engine; naming the module mux-specific.

### single header for v1, one-per-column invariant (Q5)

- Decision: Render is single-column today (`buildStackBody`, `layout.go:30`). v1 ships **one**
  header spanning full width at the top; the strand stack is pushed down below it. "One header
  per column" holds naturally if columns are ever added; multi-column layout is not built now.
- Rationale: YAGNI — no column concept exists.
- Rejected: building multi-column layout now.

### top-band-in-render (Q3)

- Decision: Add a new pinned fixed-height **top band** placement to the render layer for the
  header pane, positioned at the top, outside the strand stack, visible alongside strands.
- Rationale: matches "header at the top, over the strands." No `anchor:top` exists today
  (`render/types.go:23-39`), so this is new render work (new `policy.go` case +
  `layout.go`/`height.go` mechanics; shrink the strand `Box` by the header height).
- Rejected: header in its own tmux window (`AnchorOwnWindow`) — keeps the session alive but is
  not a visible top band.

### header pane is the persistent keepalive (Q8)

- Decision: Created at `up`/boot **before** any strand, re-created if missing on `up`/`resume`,
  torn down with the session on `down`; pane id persisted in `mux.json` outside `Strands`.
  **Always on**, cannot be disabled — it is structural. It remains alive when **all strands are
  dead**, making the last-strand teardown unreachable.
- Rationale: the keepalive guarantee only holds if the header always exists and is never
  counted/removed as a strand.
- Rejected: a `header.enabled` toggle (would surrender the guarantee).

### header-config-block + asset naming (Q4, Q12, Q14)

- Decision: Add a `header:` block to the mux config (`muxengine/config.go` +
  `template.go ConfigTemplate`): text template + `height_rows` (default 1). The default header
  template ships as an embedded asset at **`internal/muxengine/header-template.md`**
  (`//go:embed`), a distinctive name following the `builderengine` `*-template.md` precedent —
  **not** `template.yaml` (the per-engine config-template convention). Config `header.template`
  overrides the default inline.
- Rationale: one config place; distinctive asset name avoids confusion with the config template;
  assembly next to the engine that owns config + Layout.
- Rejected: separate config file; naming the asset `template.yaml`; embedding it in muxcli.

## Technical context

Full codebase map: `.scratch/mux-console-exploration.md`. Highlights mill-plan needs:

- **`internal/stencil`** — `Fill(template, values)` (`stencil.go:36`), strict unfilled-marker
  error, strips a leading `<!-- -->` banner. Reused for the fill step.
- **No `anchor:top` band exists** — `render/types.go:23-39` vocab is
  `below-parent | own-window (deferred) | hidden`. Top band = new render work; render stacks
  vertically full-width within a `Box{X,Y,W,H}` (`layout.go:30 buildStackBody`).
- **Most dangerous seam:** `reconcile.go:93-113` — `planReconcile` kills any live pane not in
  `boundPaneIDs` (built from `Strand.PaneID`). The header pane id MUST be threaded into an
  exemption set here or it is reaped on the next add/resume/remove.
- **Accounting exclusion points:** `lifecycle.go:415` (`UpResult.Strands` count),
  `lifecycle.go:922-925` (`Status` loop), `noSessionMessage`/`strandCount` (`lifecycle.go:842-847`).
- **Last-pane kill site:** `strand.go:485-487` (`RemoveStrand`'s explicit kill-pane loop — what
  destroys the session today); `reconcile.go:59-79` `keptDeadPane` (partial guard, moot once a
  permanent header exists).
- **Pane creation:** `launchStrandLocked` splits with **no `-c`** cwd (`spawn.go:110-146`); only
  boot `new-session -c <layout.Cwd>` passes a cwd (`lifecycle.go:294-302`). Header wants the hub
  root → new `split-window -c <e.layout.Hub>`. Command injected via `send-keys` (`spawn.go:139-142`).
- **Hub root:** `hubgeometry.Layout.Hub = filepath.Dir(WorktreeRoot)` (`hubgeometry.go:117`);
  `Layout{Cwd, WorktreeRoot, Hub, RelPath, Prime}`. `repo` derives from `WorktreeRoot`/`HubSuffix`.
- **tmux/psmux layer:** `overlay.go` `TmuxCmd` is the only subprocess seam. The feature must work
  on **both** tmux (real session dies on last pane) and psmux (Windows; pane corpses) — smoke
  tests are psmux-gated.
- **Boot ordering:** header created during `ensureServerAndSessionLocked` / `Up`, before strands.

## Constraints

- CONSTRAINTS.md: **Hub Geometry Invariant** — all cwd/geometry via `internal/hubgeometry`
  (`layout.Hub`, never recompute). **CLI / Cobra Invariant** — `lyx mux header` needs a `Short`,
  the `Command()`/`RunCLI` seam, help-tree tests; thin verb → one `Engine` method. **lyxtest Leaf
  Invariant** for fixtures. **Documentation Lifecycle** — update `docs/modules/` (mux module doc +
  a new `tokenvocab` module doc) + `docs/overview.md` (module table gains `tokenvocab`) in the same
  commit; if the "header is not a strand / keepalive" rule is cross-cutting, record it in
  CONSTRAINTS.md.
- Dependency direction `muxcli → muxengine → render` stays one-directional; `stencil` stays a pure
  leaf; `tokenvocab → {hubgeometry, stencil}` only.

## Testing

Per-module approach (TDD candidates called out):

- **`internal/tokenvocab` (TDD):** hermetic unit tests — each token's `Resolve` given a fixture
  `Layout` (`repo`, `hub`); `Build` returns the full map; `Render` composes with stencil (fills a
  template, and surfaces stencil's unfilled-marker error for an unknown token). Adding a token = a
  new registry entry + a new case; the test file demonstrates the "trivial to add" property.
- **`Engine.HeaderText()` (TDD, hermetic):** returns the stencil-filled template from a fixture
  config + Layout; falls back to the embedded default when `header.template` is unset; propagates a
  bad-template error.
- **`lyx mux header` verb:** help-tree/CLI test (Short present, wired into the mux command tree);
  prints `HeaderText()` output and blocks.
- **Keepalive regression (real tmux):** flip `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds`
  (`contract_integration_test.go:332`) — with a header present, removing the last strand leaves the
  session + header alive and strand count reaches zero (session NOT torn down).
- **Reconcile exemption (smoke):** mirror `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`
  (`smoke_lifecycle_test.go:237`) — the header pane survives reconcile even with strands bound
  (its id is in the exemption set), and survives `up`/`add`/`remove` cycles.
- **Top-band render (hermetic):** header placed at the top at `height_rows`, strand stack tiled in
  the shrunk `Box` below, heights still tile exactly.
- **Boot/persistence:** header created before the first strand; `HeaderPaneID` persisted to
  `mux.json`; re-created if missing on `up`/`resume`. Cover both tmux and psmux backends.

## Q&A log

- **Q:** Bash scratch terminal or pure header? **A:** Pure header — fixed height, `lyx mux header`
  prints config-defined text and hangs. Not an interactive shell.
- **Q:** Is the header a strand? **A:** No — separate construct, excluded from strand accounting.
- **Q:** How is header content configured? **A:** A config-defined template filled by
  `internal/stencil` (`{{.token}}` markers). Reuse, not a new substitution engine.
- **Q:** How is the fixed top header realised? **A:** New pinned fixed-height top-band placement in
  render; header is its own thing; one header per column (single column in v1).
- **Q:** Where does header config live? **A:** A `header:` block in mux config (template + height
  rows, default 1).
- **Q:** How is the token vocabulary built, and by what? **A:** A **separate, general** module
  `internal/tokenvocab` (a `Token{Name, Resolve}` registry) that loom will also use. `lyx mux header`
  is super-simple glue over `Engine.HeaderText()`, which calls `tokenvocab.Render(template, ctx)`
  (= build map + `stencil.Fill`).
- **Q:** Circular import risk? **A:** None — stencil stays a pure leaf; `tokenvocab` imports only
  hubgeometry + stencil; the graph is acyclic.
- **Q:** v1 tokens? **A:** Only always-resolvable ones (`repo`, `hub`); `slug` deferred.
- **Q:** Default template asset name? **A:** `internal/muxengine/header-template.md` (per the
  `builderengine` `*-template.md` precedent) — never `template.yaml`.
- **Q:** Why does the header pane matter? **A:** It is THE persistent pane — a pane stays alive even
  when every strand is dead, so the session can never be torn down by last-strand removal.
