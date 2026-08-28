# Plan: reed: attach's layout computation scales header pane height with terminal height

```yaml
task: "reed: attach's layout computation scales header pane height with terminal height"
slug: reed-attach-header-height-bug
approved: false
started: "20260828-083039"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: render-fixed-height-pins
    file: 01-render-fixed-height-pins.md
    depends-on: []
    verify: go test ./internal/reedengine/render/...
  - number: 2
    name: engine-hook-install
    file: 02-engine-hook-install.md
    depends-on: [1]
    verify: go test ./internal/reedengine/...
  - number: 3
    name: integration-contract-docs
    file: 03-integration-contract-docs.md
    depends-on: [2]
    verify: go test ./internal/reedengine/... && go test -tags integration ./internal/reedengine/...
```

## Shared Decisions

### Decision: pins-are-render-placed-heights-never-raw-config

- **Decision:** every pinned height is the height `render.Rules` actually placed the cell at — the header's height after `clampHeaderHeight`, each collapsed strip's height after `clampToFit` — never `cfg.Header.HeightRows` or `cfg.CollapsedStripRows` read raw.
  The new `render.FixedHeightPins` entry point shares `Rules`' own policy composition rather than re-deriving it.
- **Rationale:** `internal/reedengine/render` is the single owner of reed's height policy and both budgets yield under pressure.
  Pinning a raw config value would let the hook contradict `clampHeaderHeight`'s "the header yields rows first" rule, and contradict `clampToFit`'s priority-1 strip reclaim, on any window short enough to trigger either.
- **Applies to:** all batches

### Decision: one-mapping-site-from-state-to-render-inputs

- **Decision:** the persisted-state-to-`render` mapping (`toRenderStrands`, the present-pane filter, the `HeaderPaneID` blanking, the `render.Params` assembly, `paneIDsByTop`) happens at exactly one code site, `Engine.toRenderInputs` in `apply.go`.
  Both `planLayout` and the new `Engine.fixedHeightPins` call it; neither performs a mapping of its own.
- **Rationale:** a second mapping site would be free to diverge silently, and the zero-pin disposition would then be computed from a different header id than the layout was.
- **Applies to:** batch 2, batch 3

### Decision: hook-body-is-one-array-entry-per-pin

- **Decision:** the `window-resized` hook is encoded as one tmux hook-array entry per pin: an unconditional `set-hook -u -w -t "=<session>:" window-resized` clear, then `set-hook -w -t "=<session>:" window-resized "resize-pane -t %N -y <n>"` for the first pin and the same with `-a` for each subsequent pin, header always first.
  Each `resize-pane` body is one whole argv element; there is never a separate `";"` argv element.
- **Rationale:** verified live on tmux 3.6 — a `resize-pane` naming a destroyed pane aborts the rest of a single `";"`-separated command list (header ballooned to 25 rows with a dead id first), while array entries are independent (same arrangement, header still pinned at 1).
  `set-hook` also takes its body as one argument, so a separate `";"` element would terminate the `set-hook` command itself.
- **Applies to:** batch 2, batch 3

### Decision: the-clear-is-unconditional-including-zero-pins

- **Decision:** reaching an install statement always issues the `set-hook -u` clear, including when the pin list is empty.
- **Rationale:** reaching the install statement means `render` has computed an opinion, and with zero pins the opinion is "nothing is pinned".
  Issuing nothing would leave a previously installed pin clamping a pane `render` has since placed as a full pane, once per resize, forever.
  That state is reachable: `planLayout` blanks `st.HeaderPaneID` when the header pane is absent from the present set, so an apply with no live header and no strip yields zero pins.
- **Applies to:** batch 2, batch 3

### Decision: hook-failure-is-non-fatal-everywhere

- **Decision:** every `set-hook` failure is logged via `logger.Warn` and ignored.
  Neither `applyLayoutLocked` nor `AttachArgv` may fail, degrade, or change a single returned argv element because of one.
  A failed clear does not abort the rebuild that follows it.
- **Rationale:** this is the existing Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` that already governs `pinGeometryOptionsLocked`, and it is what makes the change safe on psmux, whose `set-hook`/`window-resized` support is unverified anywhere in this repo.
  `AttachArgv` additionally may never be blocked by an engine-side failure at all, by its own contract.
  Continuing past a failed clear is right because the first (non-`-a`) `set-hook` overwrites the array from entry `[0]` regardless.
- **Applies to:** batch 2, batch 3

### Decision: guard-skip-leaves-a-stale-array-deliberately

- **Decision:** the two guard-skip states — `applyLayoutLocked` returning at `len(live) < 2` or at `!anyPlacedStrand` — leave a previously installed hook array in place, with no clear and no removal path.
  This is the opposite disposition from `the-clear-is-unconditional-including-zero-pins` above, and deliberately so: a guard-skip never reaches an install statement at all, so reed has computed no opinion to write, whereas an install statement reached with zero pins has computed one and the opinion is "nothing is pinned".
- **Rationale:** `len(live) < 2` is harmless — `resize-pane -y` against a window's sole pane is a silent no-op, re-verified live on tmux 3.6 during this plan's round-1 review (a `resize-pane -t %0 -y 1` on a one-pane 24-row window exits 0 and leaves the pane at 24 rows), so the stale header pin cannot express itself against `render.Rules`' sole-cell branch.
  `!anyPlacedStrand` is the reachable, long-lived one: `internal/reedengine/state.go` documents an operator remedy that deletes `reed.json` while the session and its processes keep running untracked, after which `anyPlacedStrand` is false forever.
  There the stale array is a benefit — it keeps pinning the still-alive header and strips at the budgets reed last computed for them, which is what an operator would want from a session reed has stepped back from managing.
- **Rejected:** moving the `set-hook -u` clear ahead of the guards so every call site clears.
  It would strip the pins from exactly that untracked-but-running session, and a clear with no rebuild behind it is strictly worse than a slightly stale array — a cleared hook drifts on the very next resize, while a stale one keeps working for every pin whose pane is still alive.
- **Applies to:** batch 2, batch 3

### Decision: set-hook-and-resize-pane-stay-out-of-requiredSubcommands

- **Decision:** `set-hook` and `resize-pane` are reed's first deliberately optional wire surface.
  `requiredSubcommands` in `internal/reedengine/probe.go` does not grow; `internal/reedengine/doc.go` documents the required-versus-optional split instead.
- **Rationale:** adding them to `requiredSubcommands` would make a psmux lacking `set-hook` fail the whole capability probe at server-ensure, taking down every reed verb over a quality-only option already designed to degrade silently.
- **Applies to:** batch 3

### Decision: install-points-are-two-named-statements-no-guard-moves

- **Decision:** the hook is installed at exactly two statements and no guard or degrade return at either site is moved, reordered, or changed.
  In `applyLayoutLocked`: immediately after the `select-layout` call returns without error, before the `select-pane` call.
  In `AttachArgv`: inside the `withOpLock` closure, immediately after `planLayout` returns without error and before `chained` is assigned.
- **Rationale:** both statements sit where the pins are already computed against the same box the layout was, with the strand table and pane list already in hand — no extra I/O, no new lock, no reordering of a degrade ladder whose ordering is load-bearing.
  A degrade path installing no hook is an accepted, documented consequence, not something to code around.
- **Applies to:** batch 2, batch 3

### Decision: go-test-verify-shape

- **Decision:** this is a Go repo, so every `verify:` command uses the native Go test runner with no `PYTHONPATH=` prefix.
- **Rationale:** the `PYTHONPATH= ` prefix rule is Python/mill-project-specific.
- **Applies to:** all batches

## All Files Touched

- `internal/reedengine/apply.go`
- `internal/reedengine/apply_test.go`
- `internal/reedengine/attach.go`
- `internal/reedengine/attach_test.go`
- `internal/reedengine/attachgeometry_integration_test.go`
- `internal/reedengine/contract_integration_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/render/height.go`
- `internal/reedengine/render/layout.go`
- `internal/reedengine/render/pins_test.go`
- `internal/reedengine/render/rules.go`
- `internal/reedengine/windowsize.go`
- `internal/reedengine/windowsize_test.go`
- `manifest/roadmap.md`
