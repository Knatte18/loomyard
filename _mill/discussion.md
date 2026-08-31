# Discussion: Reconsider the collapsed strand strip default size

```yaml
task: Reconsider the collapsed strand strip default size
slug: reed-collapsed-strip-readability
status: discussing
parent: main
```

## Problem

When a child strand collapses its parent (`lyx reed add --parent <guid> --cmd <long-running>`), the parent's pane shrinks to `collapsed_strip_rows` as designed.
The collapse itself is correct and is not the defect.
The defect is that the default of `3` is tight enough that the strip shows effectively one line of frozen output — a Haiku strand's final "Cogitated for 7s" status line, say — and at a glance that is indistinguishable from dead or stale leftover text, even though `lyx reed status` correctly reports the strand as `live:true`.
Confirming liveness took a cross-check against `lyx reed status`, which defeats the point of having the strip on screen at all.

Why now: this surfaced as W4 in a reed watch-suite run (`verdict: WARN`).
The strip exists precisely so an operator can glance at a collapsed parent and know it is still there.
A strip that cannot answer that question is doing no work for the rows it costs.
The fix direction chosen here is to make liveness read from *real recent output* — give the strip enough rows that a producing process visibly moves — rather than to bolt on a synthetic indicator.

## Scope

**In:**

- `internal/reedengine/template_posix.yaml` — `collapsed_strip_rows: 3` → `6`, and extend the key's inline comment with the readability rationale.
- `internal/reedengine/template_windows.yaml` — the same two edits, identical values.
- `internal/reedengine/config_test.go` — the two template-default assertions at lines 61 and 138 (`CollapsedStripRows != 3`) move to `6`.
- `internal/reedengine/doc.go` — one minimal reword of the silent-layout-rescale anecdote so its "3-row collapsed strip" is marked as the then-default rather than reading as the current one.

**Out:**

- `min_full_rows` — stays `3`. Unrelated knob.
- `internal/reedengine/render/` — no code change at all. `height.go`'s `stackHeights`/`clampToFit`, `rules.go`'s `planCells`/`FixedHeightPins`, and `layout.go` all treat `CollapsedStripRows` as an opaque absolute row budget and need no adjustment for a larger one.
- Any strip-specific floor in `clampToFit` analogous to `MinFullRows`.
- Any synthetic liveness indicator (spinner, badge, rendered "live" marker) in the strip.
- `internal/configsync` — no value-migration mechanism. Reconcile stays key-based.
- Sandbox suites (`tools/sandbox/SANDBOX-REED-SUITE.md`, `SANDBOX-REED-WATCH-SUITE.md`) — they already reference `collapsed_strip_rows` symbolically, never by value.
- `docs/overview.md` and `manifest/roadmap.md` — the module table and execution stack do not change, and this is a default-tuning pass, not a planned roadmap item.
- Every `render` test that passes `Params{CollapsedStripRows: 2, ...}` — those are unit inputs chosen for the test, not the template default, and must stay as they are.

## Decisions

### strip-default-six

- Decision: `collapsed_strip_rows` defaults to `6`.
- Rationale: the strip must show enough consecutive recent output lines that a still-producing process is visibly moving, which is what makes liveness readable without a cross-check.
  Three rows in practice yields one line of visible text once a TUI's trailing status/padding lines are accounted for.
  Six is the smallest value that reliably clears that bar while staying cheap when strips stack: on the boot box (50 rows, header 1 + its divider 1 = 48 usable), four stacked strips plus dividers cost 28 rows and still leave the active pane 20; on a 30-row attached client, three stacked strips leave the active pane 7 — in neither case does `clampToFit` fire.
- Rejected: `5` (still marginal against a TUI that pins a status line at the bottom);
  `8` (four stacked strips cost 32 of 48 usable rows on the boot box, squeezing the active pane for a readability gain over `6` that no observation supports);
  deriving the strip height proportionally from the window height (reed's entire fixed-height machinery — `FixedHeightPins`, the `window-resized` hook's `resize-pane -y` array, `clampToFit` — is built on absolute row budgets, and a proportional budget would have to be recomputed and re-pinned on every resize for no benefit the absolute budget does not already deliver);
  keeping `3` and adding a synthetic indicator (see `no-synthetic-indicator`).

### both-templates-lockstep

- Decision: `template_posix.yaml` and `template_windows.yaml` both move to `6`, with identical comments.
- Rationale: the two templates already carry a byte-identical `collapsed_strip_rows` line, and nothing about the readability problem is platform-specific — it is a function of how many text rows a pane shows, which psmux and tmux treat the same way.
  A per-platform divergence would be a new, unjustified difference to maintain.
- Rejected: POSIX-only (would leave the Windows/psmux default at the value the bug report is about).

### no-value-migration

- Decision: existing hubs whose materialized `_lyx/config/reed.yaml` already holds `collapsed_strip_rows: 3` keep `3`. No migration is added; the change is documented and operators hand-edit if they want the new default.
- Rationale: `internal/configsync`'s `ReconcileAll` is key-based by construction — it adds keys newly present in the template and removes keys absent from it (via `yamlengine.Reconcile`), and never rewrites the value of a key that already exists.
  That is deliberate: an operator who tuned `collapsed_strip_rows` for their own terminal must not have it silently reverted by an unrelated `lyx config reconcile --apply`.
  The other reed keys already document this same behaviour in their own comments ("an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value").
- Rejected: adding value migration to configsync (a cross-cutting change to the reconcile contract affecting every module's config, far outside this task, and one that would clobber deliberate operator tuning);
  a one-off reed-specific migration hook (same clobbering problem, plus a bespoke seam nothing else has).

### no-synthetic-indicator

- Decision: no spinner, badge, or rendered liveness marker is added to the strip.
- Rationale: the task's own framing is that liveness should read from real recent output *instead of* needing a synthetic indicator.
  A synthetic marker would also have to be rendered by something — reed has no per-strand text overlay for a strand pane (the header pane is the only pane reed renders text into), so it would mean a new rendering surface for a problem a row-count change solves.
- Rejected: adding one as a complement to the taller strip (YAGNI; if `6` proves insufficient in practice that is a new observation and a new task).

### clamp-path-unchanged

- Decision: `clampToFit`'s behaviour is untouched — strips remain priority-1 donors, reclaimed toward a floor of `1` row before any full pane gives up rows.
- Rationale: a window too short to seat every pane at its natural height should shed strip rows first;
  that is the correct degradation and a larger natural strip height does not change which pane should yield.
  `FixedHeightPins` already reports the height `Rules` actually *placed* a strip at (post-`clampToFit`), never the raw configured budget, so the `window-resized` `resize-pane -y` pins stay correct at any default.
- Rejected: giving strips a configured floor analogous to `MinFullRows` (would mean strips could refuse to yield rows to a pane about to be starved, inverting the priority order the clamp exists to express).

### rationale-lives-in-the-template-comment

- Decision: the "why this number" note goes in the `collapsed_strip_rows` inline comment in both templates.
- Rationale: there is no `manifest/designs/reed.md`;
  reed's durable rationale lives in `internal/reedengine/doc.go` (for measured multiplexer-contract facts) and in the template comments (for tuning knobs).
  Every other reed knob — `mouse`, `watchdog`, `debug_log`, `width`, `height` — carries its rationale and its adoption caveat in its own inline comment, so a reader looking for "why 6" finds it exactly where they find "why mouse on".
- Rejected: a new paragraph in `doc.go`'s package prose (its `# Multiplexer contract surface` list is specifically for measured multiplexer behaviours, and a default-value rationale is not one);
  creating `manifest/designs/reed.md` (a whole module design doc introduced to hold one sentence).

### doc-anecdote-marked-as-then-default

- Decision: `doc.go`'s silent-layout-rescale entry keeps its measurement but marks the `3` as the then-default.
- Rationale: that entry records a real live measurement on tmux 3.6 — a `220x50` layout string applied to a `100x30` window rescaled a 3-row collapsed strip into 1 row.
  The measurement stays true and must not be restated as though it were taken at `6`.
  But left unqualified, "a 3-row collapsed strip" reads as a statement about the current default and will mislead the next reader.
- Rejected: leaving it verbatim (misleading once the default is `6`);
  re-measuring at `6` (the entry's point is that absolute row budgets get rescaled proportionally, which is independent of the budget's magnitude — a fresh measurement would prove nothing new and needs a live tmux).

## Technical context

The change is a single template value plus its assertions and comments.
The reason it is that small is worth stating explicitly, because it is the thing mill-plan most needs to trust:

- **Config load path.** `internal/reedengine/config.go`'s `LoadConfig` calls `configengine.LoadOrTemplate(baseDir, module, ConfigTemplate())`, which degrades to the embedded template on proven absence of the config file.
  `Config.CollapsedStripRows` (`yaml:"collapsed_strip_rows"`) is a plain `int`.
  `ConfigTemplate()` selects `template_posix.yaml` or `template_windows.yaml` by build constraint (see `template.go`).
- **Where the value is consumed.** `internal/reedengine/apply.go` (~line 91) copies `e.cfg.CollapsedStripRows` into `render.Params`.
  From there `internal/reedengine/render/height.go`'s `stackHeights` reads `p.CollapsedStripRows`, floors it at `1`, multiplies by the number of strips to get `stripDemand`, and gives the remaining rows to the full panes (remainder to the active/bottom pane).
  Nothing else reads the field.
- **No coupling to the header clamp.** `clampHeaderHeight(headerRows, windowRows, minStackRows)` is called from `rules.go`'s `planCells` with `p.MinFullRows` as `minStackRows` — not `CollapsedStripRows`.
  Raising the strip budget therefore cannot perturb header sizing.
- **Absolute-budget machinery is magnitude-agnostic.** `render.FixedHeightPins` emits one `Pin{PaneID, Height}` per strip at the height `Rules` actually placed it (post-`clampToFit`), and `windowsize.go`/`apply.go` install those as `resize-pane -y` entries in the `window-resized` hook array.
  All of it is parameterised on the placed height, so `6` needs no new pinning work.
- **Divider rows.** `buildStackBody` (`render/layout.go`) budgets one divider row between adjacent panes, and `stackHeights` subtracts `n-1` dividers from `box.H` before splitting.
  A strip's configured rows are content rows only — `pane-border-status` is never set anywhere in `internal/reedengine`, so tmux's default (`off`) applies and no row of a pane is consumed by a title.
  This is why `6` means six text lines.
- **Reconcile semantics.** `internal/configsync`'s `ReconcileAll` → `yamlengine.Reconcile` tracks `Added`/`Removed` key-paths only.
  Confirm this by reading `configsync.go`'s `Result` doc comment: there is no value-update field.
- **Test assertions that pin the default.** `internal/reedengine/config_test.go` line 61 (in the explicit-template test, alongside `Width != 220`, `Height != 50`, `MinFullRows != 3`) and line 138 (in the degrade-to-embedded-template test, alongside `Width != 220` and `Header.HeightRows != 1`).
  Both assert `cfg.CollapsedStripRows != 3` and both must move to `6`, error strings included.
- **Test values that must NOT move.** Every `Params{CollapsedStripRows: 2, MinFullRows: 3}` in `internal/reedengine/render/rules_test.go`, `pins_test.go`, `height_test.go`, and every `e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3` in `internal/reedengine/apply_test.go` is a deliberately-chosen unit input, not the template default.
  `height_test.go` is table-driven on a `collapsedStripRows` field and asserts `placements[0].height == tt.collapsedStripRows`;
  it is value-agnostic by construction.
  Changing any of these would be a scope violation, not a fix.
- **Symbolic doc references.** `tools/sandbox/SANDBOX-REED-SUITE.md` (lines 235, 314) and `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md` (line 170) say "collapses to `collapsed_strip_rows`" without naming a number. They need no edit.
- **The one numeric doc reference.** `internal/reedengine/doc.go`, in the "Silent layout rescale" entry of the `# Multiplexer contract surface` list (~line 368-375): "a `220x50` string applied to a `100x30` window turned a 3-row collapsed strip into 1 row".
  This is the single place a bare `3` appears in prose.

## Constraints

From `CONSTRAINTS.md`:

- **Config Strictness Invariant** — `reedengine` is on the degrading side: it uses `configengine.LoadOrTemplate`, not `Load`. This change must not alter which of the two it adopts.
- **Told-Geometry Invariant** — `reedengine` is a bound package: it is handed its absolute paths and derives none. Nothing in this change may introduce a `lyxcwd` import or any self-derived path.
- **Test Tier Purity Invariant** — the `config_test.go` assertions are unit-tier and must stay so; no live tmux may be required to verify this change.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

From `CLAUDE.md`:

- **Docs land in the same commit** — the template comments and the `doc.go` reword ship with the value change, not in a follow-up.
- **Markdown: semantic line breaks** — one sentence per line, no fixed-column hard-wrap, in any `.md` this change touches.
- **`manifest/roadmap.md` does not move** — this is default tuning, not a planned-item completion.

Discovered during discussion:

- Reconcile never rewrites an existing key's value, so this change reaches new hubs only. That is intended, not a gap to close here.

## Testing

Unit tier only;
no live tmux, no sandbox run is required to land this.

**`internal/reedengine` (`config_test.go`)** — TDD candidate, and the only place behaviour is asserted.
Flip both existing default assertions to `6` first and watch them fail against the unchanged templates, then change the templates and watch them pass.
Two scenarios, both already scaffolded:

- The explicit-template path (materialized `_lyx/` config): asserts the full key set including `Tmux`/`Shell`/`Width`/`Height`/`MinFullRows`/`StrandName`/`DebugLog`/`Mouse`. Only the `CollapsedStripRows` assertion changes; every sibling assertion must survive untouched, which is what proves the edit was surgical.
- The degrade-to-embedded-template path (no `_lyx/` directory): asserts only GOOS-invariant keys. `CollapsedStripRows` is one of them, so this is what proves both `template_posix.yaml` and `template_windows.yaml` moved — on Windows CI it reads the Windows template, on Linux the POSIX one, and the same assertion covers both.

**`internal/reedengine/render`** — no new tests, no changed tests.
Run the package to confirm nothing regressed;
every test there supplies its own `Params` and is independent of the template default by design.
A newly-added render test would be asserting the arithmetic `stackHeights` already has coverage for.

**`internal/configsync`** — no test change. The no-migration decision is a decision to leave reconcile alone;
its existing key-based tests already pin that contract.

**Whole-repo** — `go build ./...` and `go test ./...` must pass. No golden/snapshot file embeds the template text, so there is no fixture to regenerate.

**Not tested here:** whether `6` is subjectively "enough" rows to read liveness at a glance.
That is an operator judgement confirmed by attaching to a real session, not an assertion.
If it proves insufficient in practice, that is a new observation and a new task, not a gap in this one's coverage.

## Q&A log

- **Q:** What should `collapsed_strip_rows` default to? **A:** [auto-pick] `6`. **Why:** smallest value that reliably shows several consecutive recent output lines while staying cheap when strips stack — four strips still leave the active pane 20 rows on the 50-row boot box.
- **Q:** Do both platform templates move together? **A:** [auto-pick] Yes, POSIX and Windows both to `6`. **Why:** the key is byte-identical today and the readability problem is not platform-specific; divergence would be a new difference to maintain for no reason.
- **Q:** Does `min_full_rows` change too? **A:** [auto-pick] No, stays `3`. **Why:** unrelated knob, and `clampHeaderHeight` reads `MinFullRows` (not `CollapsedStripRows`) as the stack floor, so there is no coupling to repair.
- **Q:** What happens to existing hubs whose `reed.yaml` already holds `3`? **A:** [auto-pick] Document only; no value migration. **Why:** `configsync.ReconcileAll` is key-based and never rewrites an existing value, deliberately, so operator tuning is never silently reverted.
- **Q:** Should a synthetic liveness indicator be added to the strip? **A:** [auto-pick] No. **Why:** the task's framing is that liveness should read from real recent output instead of a synthetic indicator, and reed has no per-strand text overlay to render one into.
- **Q:** Does `clampToFit` need a strip floor now that the natural strip height is larger? **A:** [auto-pick] No change. **Why:** strips yielding rows first is the correct degradation, and `FixedHeightPins` already pins placed heights rather than configured budgets.
- **Q:** What happens to `doc.go`'s "turned a 3-row collapsed strip into 1 row" measurement? **A:** [auto-pick] Minimal reword marking `3` as the then-default. **Why:** the tmux 3.6 measurement must stay accurate as a record, but unqualified it will read as a statement about the current default.
- **Q:** Where does the "why this number" rationale live durably? **A:** [auto-pick] The `collapsed_strip_rows` inline comment in both templates. **Why:** there is no `manifest/designs/reed.md`, and every other reed knob carries its rationale in its own inline comment.
- **Q:** Which tests change? **A:** [auto-pick] Only the two template-default assertions in `config_test.go` (lines 61, 138); no new tests. **Why:** every `render`/`apply` test passes `CollapsedStripRows` explicitly as a chosen unit input and is value-agnostic; sandbox suites reference the key symbolically.
