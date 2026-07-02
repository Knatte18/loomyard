MILL_REVIEW_BEGIN
# Review: Build internal/mux: the window to the world (overlay + strands + render)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\internal-mux\_mill\discussion.md
date: 2026-07-02
```

## Findings

### [GAP] `hidden` strand has no defined psmux realization
**Section:** Scope / Render decision / `add` flag spec
**Issue:** `add --anchor hidden` creates a strand that runs a `--cmd` but is "excluded from the layout string"; a psmux `select-layout` custom layout must enumerate exactly the window's live panes (muxpoc's `buildColumnLayout` includes every pane), so a live-but-omitted pane makes the apply inconsistent — yet the discussion never says whether a hidden strand gets no pane, or its pane is `break-pane`/`move-pane`d to a background window (as `mux.md` hints).
**Fix:** Specify the v1 psmux mechanism for a hidden strand's pane (no pane vs. moved to a background window) so render-exclusion and `select-layout` stay consistent.

### [GAP] Dead-pane cleanup vs. `select-layout` consistency on reconcile
**Section:** Re-render on-demand / reconcile (up/resume table)
**Issue:** v1 sets `remain-on-exit on`, so a dead pane **physically persists** in the window as `pane_dead=1`; reconcile "clears the pane binding and excludes it from render" but never says the dead pane is killed — so the rendered layout string would omit a pane psmux still lists, and `select-layout` mismatches (or the dead pane lingers visibly).
**Fix:** State that reconcile `kill-pane`s the dead pane before re-applying the layout (respecting don't-kill-last-pane), or that render must include dead panes.

### [NOTE] `attach` in-place vs. JSON-envelope invariant + test path
**Section:** `attach` is session-level, in-place / Testing
**Issue:** In-place `psmux attach` takes over the operator's TTY (blocking/interactive), which cannot emit the `output.Ok/Err` JSON envelope the CLI/Cobra Invariant requires, and the integration round-trip lists only `up`/`status`/`down` — attach's stdio wiring and test approach are unstated (muxpoc popped a window precisely to return JSON).
**Fix:** Specify attach's stdio inheritance and how it is exempted from the JSON-envelope/RunCLI test path.

### [NOTE] `-v`/`-vv` flag mechanics ambiguous
**Section:** logger decision
**Issue:** "`-v`=Info, `-vv`/`--verbose`=Debug" does not say whether this is a cobra count flag or two distinct flags, nor how `--verbose` aliases `-vv`.
**Fix:** Name the wiring (e.g. `CountVarP` with 1=Info/2=Debug, `--verbose`=Debug).

### [NOTE] `status` orphan-detection scope unspecified
**Section:** CLI verb set / server-naming rationale
**Issue:** The named-server rationale says `status` "flags stray psmux processes," but v1 `status` scope neither commits to orphan detection nor gives an enumeration mechanism (listing other sockets/servers on Windows).
**Fix:** State whether v1 `status` implements orphan detection and its mechanism, or defer it explicitly.

## Verdict
GAPS_FOUND
Two layout/pane-set consistency gaps (hidden realization, dead-pane cleanup) must be resolved before planning.
MILL_REVIEW_END