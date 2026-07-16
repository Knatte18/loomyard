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
- A new command `lyx mux header` with two modes: **default** returns the header text via the
  output envelope (smoke-testable, `--json` available); **`--blocking`** prints the text and hangs
  — the mux pane boots `lyx mux header --blocking`. It is the pane that keeps the session alive
  even when every strand is dead.
- A new **`Repo` field on `hubgeometry.Layout`** (git-derived repo name), so the `repo` token is
  always resolvable. (Task now also touches `internal/hubgeometry`.)
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

1. **`lyx mux header` (new muxcli verb) — two modes (Q13, GAP-1, GAP-3).**
   - **Default (non-blocking, enveloped):** renders and **returns** the header text via the
     `internal/output` envelope (`output.Ok`; a `--json` form is available). This is a normal,
     fully-testable CLI command — used as a **smoke test** and for inspection — so it needs no
     carve-out and satisfies the CLI/Cobra invariant outright. A bad template / unresolvable
     token surfaces as `output.Err` (non-zero exit) — loud, not silent.
   - **`--blocking`:** the variant the mux pane actually runs — prints the rendered text, then
     **blocks forever** (keepalive). Only this flag-gated mode is exempt from the JSON envelope
     (it prints raw text then hangs). This narrow, self-displaying keepalive exemption **extends
     the CONSTRAINTS.md interactive-handoff exception** (which today only covers handing stdio to
     *another* interactive program); record the extension in CONSTRAINTS.md in the same commit.
   - Assembly lives in **`Engine.HeaderText()`** (muxengine, per CLI/Cobra: thin verb → one engine
     method): read the `header:` config template (or embedded default) → `tokenvocab.Render(template,
     ctx)` → return text. The engine holds `layout *hubgeometry.Layout`, so it builds `ctx`. Both
     modes call `HeaderText()`; they differ only in blocking + output format. The mux pane boots
     with `lyx mux header --blocking`.

2. **`internal/tokenvocab` (NEW, general/shared) — the vocabulary + the reusable compose.**
   Owns the *definition* of the vocabulary as a **registry of `Token{Name string; Resolve
   func(ctx) string}`** — adding a token = append one entry (testable per token). `ctx` wraps
   `*hubgeometry.Layout` (v1). Exposes `Build(ctx) map[string]string` and a convenience
   `Render(template []byte, ctx) ([]byte, error)` = `stencil.Fill(template, Build(ctx))`, so
   every consumer (mux header, **loom**) composes with one call instead of re-implementing it.
   v1 exposes only **always-resolvable** tokens (`repo`, `hub`); `slug` (task slug) is deferred
   with its own empty-value policy then. The `repo` resolver simply reads a **new
   `hubgeometry.Layout.Repo` field** (GAP-2): hubgeometry owns geometry (Hub Geometry Invariant),
   so it — not tokenvocab — sets the repo name. Derivation is **`filepath.Base(Prime)`** (r2 NOTE):
   `Prime` (the main worktree path) is already computed by `Resolve`, so this is **spawn-free** — no
   `git remote get-url` subprocess is added to the `Resolve` hot path (the repo recently invested in
   reducing git spawns). Always non-empty. `hub` reads `Layout.Hub`.

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
- **Layout enumeration (GAP-2 r2 — BINDING correctness):** `render.Rules(strands, box, params,
  paneOrder)` (`rules.go:29`) enumerates only strands, but the emitted `window_layout` must list
  **every** live pane or tmux rejects the mismatched-count layout and psmux reaps the extra pane
  (`apply.go:74` renders over `Box{0,0,Width,Height}`). So `HeaderPaneID` + header height must be
  **threaded into `planLayout`→`Rules` as a new parameter** (NOT a synthetic strand): `Rules`
  emits the header band cell at `{Y:0, H:headerHeight}` and lays the strand stack in the shrunk
  `Box{Y:headerHeight, H:Height-headerHeight}` below it.
- **Height clamp (NOTE-2, refined r2):** the window→(header, strand-box) split is **new clamp
  logic**, distinct from the existing `clampToFit` (`height.go:88`, which distributes rows *among
  strands inside* an already-shrunk Box). The new step clamps `header height` so the strand-stack
  **region** keeps a floor (a stack-total minimum, e.g. `MinFullRows` for the region — not
  per-strand); `clampToFit` then distributes within that region as today.
- Rejected: header in its own tmux window (`AnchorOwnWindow`); an unclamped header height that can
  starve the strand stack; modelling the header as a hidden/special strand just to reach `Rules`.

### header pane is the persistent keepalive (Q8)

- Decision: Created at `up`/boot **before** any strand, re-created if missing on `up`/`resume`,
  torn down with the session on `down`; pane id persisted in `mux.json` outside `Strands`.
  **Always on**, cannot be disabled — it is structural. It remains alive when **all strands are
  dead**, making the last-strand teardown unreachable.
- **Adoption/split protection (GAP-2 r2 → GAP-1 r2 — BINDING correctness):** `planPaneTarget`
  (`spawn.go:40`) returns the **first alive pane** as the adoption target when no strand is bound
  yet (exactly the post-boot state) and picks the **tallest alive pane** as the split target. The
  header pane is alive but not a strand, so the first strand added after boot would **adopt (take
  over) the header pane**, destroying the keepalive — or split it. `HeaderPaneID` must be
  **excluded from both** `planPaneTarget`'s adoption-candidate loop and its split-target selection,
  in addition to the `reconcile.go:93-113` exemption.
- **Error path (GAP-3 — errors are fixed loudly, never left):** `up`/config-load **eagerly
  validates** the header template (renders it once, `output.Err` + non-zero exit on a bad
  template / unresolvable token), so a misconfiguration fails **loud and early**, before the
  session boots — not silently swallowed. Only if a render error somehow reaches the running
  `--blocking` pane does it print the error text into the pane and keep blocking (keepalive
  preserved); that path is a rare fallback because eager validation catches broken configs first.
- Rationale: the keepalive guarantee only holds if the header always exists and is never
  counted/removed as a strand; and a bad config must surface loudly at `up`, not hide in a pane.
- Rejected: a `header.enabled` toggle (would surrender the guarantee); silently ignoring a render
  error; exiting on render failure (would drop the keepalive at boot when misconfigured).

### header-config-block + asset naming (Q4, Q12, Q14)

- Decision: Add a `header:` block (text template + `height_rows`, default 1) to the mux config
  struct in `muxengine/config.go`. The embedded config-template YAML is **GOOS-selected**
  (NOTE-1): the `header:` block must be added to **both** `internal/muxengine/template_posix.yaml`
  and `internal/muxengine/template_windows.yaml` (embedded by `template_posix.go` /
  `template_windows.go`; `template.go` is only the accessor). The default *header* text template
  ships as a separate embedded asset at **`internal/muxengine/header-template.md`** (`//go:embed`),
  a distinctive name following the `builderengine` `*-template.md` precedent — **not**
  `template.yaml` (the per-engine config-template convention). Config `header.template` overrides
  the default inline.
- Rationale: one config place; distinctive asset name avoids confusion with the config template;
  assembly next to the engine that owns config + Layout; both GOOS variants must carry the block or
  one platform ships without a header default.
- Rejected: separate config file; naming the asset `template.yaml`; embedding it in muxcli; editing
  only `template.go` (an accessor — embeds nothing).

## Technical context

Full codebase map: `.scratch/mux-console-exploration.md`. Highlights mill-plan needs:

- **`internal/stencil`** — `Fill(template, values)` (`stencil.go:36`), strict unfilled-marker
  error, strips a leading `<!-- -->` banner. Reused for the fill step.
- **No `anchor:top` band exists** — `render/types.go:23-39` vocab is
  `below-parent | own-window (deferred) | hidden`. Top band = new render work; render stacks
  vertically full-width within a `Box{X,Y,W,H}` (`layout.go:30 buildStackBody`).
- **Pane seams the header id MUST be excluded from (all three, or the keepalive breaks):**
  (1) `reconcile.go:93-113` — `planReconcile` kills any live pane not in `boundPaneIDs` (from
  `Strand.PaneID`); thread the header id into an exemption set. (2) `planPaneTarget` (`spawn.go:40`)
  — excludes the header pane from adoption (first-alive-pane, post-boot) and split-target
  (tallest-alive) selection. (3) `render.Rules`/`planLayout` (`rules.go:29`, `apply.go:74`) — the
  header id + height are passed in as a new param so the emitted `window_layout` enumerates the
  header band; otherwise tmux rejects the layout / psmux reaps the pane.
- **Accounting exclusion points:** `lifecycle.go:415` (`UpResult.Strands` count),
  `lifecycle.go:922-925` (`Status` loop), `noSessionMessage`/`strandCount` (`lifecycle.go:842-847`).
- **Last-pane kill site:** `strand.go:485-487` (`RemoveStrand`'s explicit kill-pane loop — what
  destroys the session today); `reconcile.go:59-79` `keptDeadPane` (partial guard, moot once a
  permanent header exists).
- **Pane creation:** `launchStrandLocked` splits with **no `-c`** cwd (`spawn.go:110-146`); only
  boot `new-session -c <layout.Cwd>` passes a cwd (`lifecycle.go:294-302`). Header wants the hub
  root → new `split-window -c <e.layout.Hub>`. Command injected via `send-keys` (`spawn.go:139-142`).
- **Hub root:** `hubgeometry.Layout.Hub = filepath.Dir(WorktreeRoot)` (`hubgeometry.go:117`);
  `Layout{Cwd, WorktreeRoot, Hub, RelPath, Prime}`. A **new `Repo` field** is added to `Layout`
  and populated by `hubgeometry.Resolve` from git (always non-empty), so `tokenvocab`'s `repo`
  resolver just reads `layout.Repo` (Hub Geometry Invariant keeps this derivation in hubgeometry).
- **tmux/psmux layer:** `overlay.go` `TmuxCmd` is the only subprocess seam. The feature must work
  on **both** tmux (real session dies on last pane) and psmux (Windows; pane corpses) — smoke
  tests are psmux-gated.
- **Boot ordering:** header created during `ensureServerAndSessionLocked` / `Up`, before strands.

## Constraints

- CONSTRAINTS.md: **Hub Geometry Invariant** — all cwd/geometry via `internal/hubgeometry`; the new
  `Layout.Repo` field belongs here (never recompute repo/geometry outside hubgeometry). **CLI /
  Cobra Invariant** — `lyx mux header` needs a `Short`, the `Command()`/`RunCLI` seam, help-tree
  tests, thin verb → one `Engine` method. Default mode emits the JSON envelope (`output.Ok`/`Err`);
  the **`--blocking` mode extends the narrow interactive-handoff exception** (CONSTRAINTS.md lines
  ~71-76 today cover only handing stdio to *another* interactive program — the self-displaying
  keepalive is a new sub-case). **This CONSTRAINTS.md extension must land in the same commit as the
  implementation** (Documentation Lifecycle). **lyxtest Leaf Invariant** for fixtures.
  **Documentation Lifecycle** — update `docs/modules/` (mux module doc + a new `tokenvocab` module
  doc) + `docs/overview.md` (module table gains `tokenvocab`) in the same commit; record the
  "header is not a strand / structural keepalive" rule in CONSTRAINTS.md if cross-cutting.
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
- **`lyx mux header` verb:** help-tree/CLI test (Short present, wired into the mux command tree).
  The **default non-blocking mode is the test seam** (NOTE-3): tests execute the command and assert
  the rendered/enveloped text (and `--json` shape) without hanging; a bad template asserts the
  `output.Err` non-zero exit. `--blocking` is exercised only indirectly via the keepalive smoke
  test — never run inline in a unit test (it hangs by design).
- **Keepalive regression (real tmux):** flip `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds`
  (`contract_integration_test.go:332`) — with a header present, removing the last strand leaves the
  session + header alive and strand count reaches zero (session NOT torn down).
- **Reconcile exemption (smoke):** mirror `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`
  (`smoke_lifecycle_test.go:237`) — the header pane survives reconcile even with strands bound
  (its id is in the exemption set), and survives `up`/`add`/`remove` cycles.
- **Adoption protection (hermetic, `planPaneTarget`):** given a live header pane and no bound strand,
  `planPaneTarget` neither adopts nor split-targets the header id (the first real strand splits a
  fresh pane, header untouched).
- **Layout enumeration (hermetic, `planLayout`/`Rules`):** the emitted `window_layout` contains the
  header band cell + one cell per strand (count matches live panes), header at top, strand box shrunk.
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
- **Q (GAP-1):** How is `lyx mux header` reconciled with the JSON-envelope invariant? **A:** Two modes:
  default returns the text via the envelope (smoke-testable, `--json`), and `--blocking` prints text +
  hangs for the pane. Only `--blocking` is exempt — a narrow extension of the interactive-handoff
  exception, recorded in CONSTRAINTS.md.
- **Q (GAP-2):** How is `repo` derived so it is always non-empty? **A:** A new git-derived
  `hubgeometry.Layout.Repo` field; `tokenvocab`'s `repo` resolver just reads it. Derivation stays in
  hubgeometry (Hub Geometry Invariant).
- **Q (GAP-3):** What happens on a header render error? **A:** Errors are fixed loudly — `up`/config-load
  validates the template eagerly (`output.Err`, early), so a broken config never boots silently. The
  `--blocking` pane only prints-and-keeps-blocking as a rare last-resort fallback.
- **Q (r2 GAP-1):** Can the first strand take over the header pane? **A:** Not once fixed — `HeaderPaneID`
  is excluded from `planPaneTarget`'s adoption and split-target selection (`spawn.go:40`), alongside the
  reconcile exemption.
- **Q (r2 GAP-2):** How does the header pane get into the tmux layout string? **A:** `HeaderPaneID` + height
  are threaded into `planLayout`→`render.Rules` as a new param (not a synthetic strand); Rules emits the
  header band cell + the shrunk strand stack, so the layout enumerates every live pane.
