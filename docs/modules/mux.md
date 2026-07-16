# mux: the tmux window to the world

> **Status: ✅ Implemented.** `mux` is a shipped module — see the `internal/muxengine` and
> `internal/muxengine/render` package documentation for the full design (strand bookkeeping,
> layout policy, the multiplexer contract surface) and [overview.md](../overview.md#modules)
> for its place in the execution stack. This doc covers one construct in depth: the
> **always-on header pane** — a persistent operator console band that is deliberately not a
> strand.

## What the header pane is

Every mux session carries exactly one extra, permanent pane beyond its strands: the
**header**. It is booted before any strand exists, rendered as a fixed-height band across
the top of the window, and — the whole point of it — can never be torn down by ordinary
strand churn. Removing a session's last strand used to be able to destroy the session
outright (tmux) or corpse its sole pane (psmux); with the header pane always present as a
second, permanent pane, that scenario can no longer happen on either backend — the header
keeps the session (and therefore the substrate an operator's next `lyx mux add` needs)
alive no matter how many strands come and go.

## Not a strand (keepalive)

The header is a **first-class but separate construct, never a `Strand`** (discussion
decision `header-is-not-a-strand`). Concretely:

- It is persisted as `MuxState.HeaderPaneID`, a field of its own outside the `Strands`
  slice — never a strand record, never carrying a `Display`, never reachable through any
  strand-keyed lookup.
- It is **excluded from every strand accounting, adoption, reconcile, and layout path** a
  real strand would otherwise be subject to (the exclusion seams below).
- It is **always-on and structural** — there is no config toggle that disables it. A hub
  whose `mux.yaml` predates the `header:` block adopts it via `lyx config reconcile`,
  matching the `debug_log`/`mouse` precedent.

Because it is never in `Strands`, every strand-counting site in the engine — `UpResult`'s
strand count, `Status`'s per-strand loop, the no-session error's strand-count pointer — is
already correct by construction; each carries a short godoc note recording that fact so a
future edit does not "fix" the count by folding the header in.

## Boot and lifecycle

`ensureHeaderPaneLocked` (lifecycle.go) runs on **both** `Up` and `Resume` — the shared boot
path both compose — right after the session/initial pane is confirmed to exist:

1. If `MuxState.HeaderPaneID` is already set and still names a live pane, it is a no-op —
   idempotent across repeated `up`/`resume` calls.
2. Otherwise it splits the session's current pane with `-b` (so the new pane lands
   **above**, not below, the target — see [Render: the top band](#render-the-top-band) for
   why physical position matters) and `-c <Hub>` (the Hub Geometry Invariant: the header
   pane's cwd is never recomputed here), runs the header's own launch command
   (`headerLaunchCmd`, composed entirely through `internal/shell` — the Shell Mechanics
   Seam), captures the new pane id, and persists it.
3. On a server rebirth (the `if booted` block both `Up` and `Resume` already run to clear
   every stale strand binding), `HeaderPaneID` is cleared alongside them — a reborn
   session reuses pane ids, so a stale header binding would otherwise be mistaken for the
   still-live header pane. The clear lives in `lifecycle.go`, not inside
   `clearAllPaneBindings` itself, since the header is not a strand binding.

**Eager validation.** Before any of that, `ensureServerAndSessionLocked` calls
`Engine.ValidateHeader()` — right after the session/initial pane is confirmed to exist, on
every return path, so it runs on `Resume` (a crash recovery) exactly as it does on a first
`Up`. A bad header template or unresolvable token surfaces as a loud, early error before
the header pane is ever created (discussion decision `eager-header-validation`); it is
never silently swallowed. `ValidateHeader` composes `internal/tokenvocab.Render` over
`Engine.HeaderText` — see the `internal/muxengine` package documentation for the text
pipeline itself.

## Render: the top band

`render.Params` carries an optional `Header{PaneID, HeightRows}`. A zero `PaneID` means "no
header" — every pre-header caller of `render.Rules` is unaffected. When a header pane id is
present, `Rules`:

1. Reserves one divider row between the header band and the stack — the same one-row-per-gap
   border tmux/psmux always renders between vertically adjacent panes that
   `buildStackBody` already budgets for *between* strands (`dividers := n-1`). Verified
   against a real tmux instance: omitting this budget still lets `select-layout` return
   success, but tmux inserts the border row anyway, silently overflowing the bottom of the
   window by exactly one row (`contract_integration_test.go`'s
   `TestHeaderNeverGetsZeroHeightLayoutCell` pins this). The divider is only reserved when
   at least one strand is actually placed — an empty stack (header as the window's sole
   pane) never reaches `select-layout` at all (`applyLayoutLocked` skips both tmux calls
   below two live panes).
2. Computes the header's actual row count via `clampHeaderHeight(headerRows,
   windowRows-1, minStackRows)` (the window's row budget minus that reserved divider) — the
   **window-split clamp**, distinct from `clampToFit` (which distributes rows *among*
   strands inside an already-shrunk box). `clampHeaderHeight` instead decides how much of
   the *whole window* the header band may claim: the strand-stack region never shrinks
   below `MinFullRows` (floored at 1) total rows, so an oversized configured `height_rows`
   can never starve the strand stack — the header yields rows first. The result itself is
   never clamped below 1 row (as long as the window has any rows to give): a real
   tmux/psmux `select-layout` does not cleanly support a genuinely zero-height cell for an
   always-on pane either — it silently keeps a row for it anyway, causing the same
   off-by-one overflow the missing divider did.
3. Lays the below-parent stack out in the shrunk region below the header band and its
   divider (`{X:0, Y:headerHeight+1, W:box.W, H:box.H-headerHeight-1}`), reusing the
   existing `buildStackBody` unchanged against that shrunk box.
4. Splices a fixed-height header cell in front of the stack's cells (`bandHeader`,
   layout.go) so the emitted `window_layout` enumerates the header cell **plus every
   strand cell** — the live-pane count the caller's `select-layout` must match exactly, or
   tmux/psmux reaps the mismatch (`applyLayoutLocked`'s doc comment; this is why lifecycle
   and render land in one batch — a header pane the render layer does not enumerate is
   reaped the moment a layout is next applied).

The header is injected at this `Params` seam, **never** appended to the strand slice —
`partitionByAnchor`/`orderStack` never see it and are unchanged.

**Why the header's own split uses `-b`.** psmux/tmux apply a layout string's cells
*positionally* to the window's actual top-to-bottom pane order (see the
`internal/muxengine` package documentation's "Multiplexer contract surface"), and
`render.Rules` always emits the header cell *first*, assuming it is the physically topmost
pane. `ensureHeaderPaneLocked`'s split therefore passes `-b` (new pane created **above**
the target, since tmux's default split direction is vertical/below) so the header pane
stays physically topmost forever after — every subsequent *strand* split (`spawn.go`)
targets a non-header pane and inserts **below** it, so nothing ever pushes the header out
of the top position once it is created.

## Exclusion seams: adoption, split-target, reconcile

The header pane must never be treated as fungible strand real estate. Three seams enforce
this, all downstream of `MuxState.HeaderPaneID`:

- **Adoption** (`planPaneTarget`, spawn.go). When no strand currently holds a pane binding
  (the fresh-substrate case), the header pane is skipped when searching for an alive pane
  to adopt — the header always runs its own print-then-block pipeline, never a strand's
  command.
- **Split-target selection** (`planPaneTarget`). The preferred split target is always the
  tallest alive **non-header** pane. Only when *no* non-header pane exists at all — every
  strand has been removed and the header is the session's sole remaining pane — does the
  header become the split target, as a last-resort fallback: this is what lets an
  add-after-all-strands-removed still have something to split. The header survives that
  split (tmux's split-window keeps the target pane alive alongside the new one), and
  `clampHeaderHeight` restores its configured fixed height on the next render.
- **Reconcile** (`planReconcile`, reconcile.go). The header pane id is **never** folded into
  `boundPaneIDs` — that map also gates `anyBoundPresent`, and folding the header in would
  make `anyBoundPresent` true whenever the header is merely live, reaping
  operator/foreign panes even with zero strands bound and breaking the documented "no
  bound content, foreign panes untouched" invariant. Instead a **separate** `exemptPaneIDs`
  set (`boundPaneIDs` plus the header pane id) gates only the untracked-pane reap loop, so
  the header is never killed as an "untracked" pane while `anyBoundPresent` stays derived
  from real strand bindings alone.

## Config block

```yaml
header:  # the always-on operator console pane's rendered text
  template: ""  # empty means "use the embedded default template"; set to override
  height_rows: 1  # fixed row count the header pane occupies
```

`template` empty means "use the embedded default" (`HeaderTemplate()`, filled via
`internal/tokenvocab.Render`); `height_rows` is the value `clampHeaderHeight` treats as the
header's *requested* height, clamped down (never up) so the strand stack keeps its floor.
See the `internal/muxengine` package documentation for the full text-rendering pipeline and
`internal/tokenvocab`'s module doc ([tokenvocab.md](tokenvocab.md)) for the token
vocabulary itself.

## Testing

- **Hermetic (untagged):** `headerpane_test.go` (`headerLaunchCmd`'s pure command-string
  composition), `spawn_test.go`/`reconcile_test.go` (the three exclusion seams above),
  `render/rules_test.go`/`render/height_test.go` (the top-band enumeration and
  `clampHeaderHeight`).
- **Real-tmux (`//go:build integration`):**
  `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds` (contract_integration_test.go) proves
  end to end that removing a session's sole strand leaves the session **and the header
  pane specifically** alive, with zero strands persisted.
  `TestHeaderNeverGetsZeroHeightLayoutCell` (contract_integration_test.go) applies a
  `render.Rules`-generated header layout against a live tmux session with a pathological
  window/header-height ratio and asserts every resulting pane keeps a positive height and
  stays within the window's bounds — the real-multiplexer contract check the header/stack
  divider budget and `clampHeaderHeight`'s never-below-1 floor exist to satisfy.
- **Smoke (`//go:build smoke`):** `TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile`
  (smoke_lifecycle_test.go) drives a full `up` → `add` → `remove` → `add` cycle through the
  real CLI, asserting the header pane survives every step and is never adopted as a
  strand's pane.
