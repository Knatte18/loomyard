# Discussion: Reconsider whether lyx mux needs anchor:top at all

```yaml
task: Reconsider whether lyx mux needs anchor:top at all
slug: mux-anchor-top-redesign
status: discussing
parent: cluster-fork-spike
```

## Problem

`lyx mux`'s render layer supports an `anchor:top` placement mode that reserves a
fixed-height band at the top of the window for a strand (a status line above the
below-parent stack). A design discussion under `cluster-fork-spike` surfaced that
`anchor:top` may be redundant: `below-parent` + `ShrinkWhenWaitingOnChild` — already the
unconditional default on every `lyx mux add` (`internal/muxcli/add.go:88`) — already
delivers "mother sits above her child, compact once she's just waiting on it", but
*dynamically* (full height while alone, collapse to `collapsed_strip_rows` once a live
descendant exists) instead of as a static reservation, and without `anchor:top`'s
fixed-height corruption class (`mux-server-crash`/`TopBandRows`, both done).

This task settled the question empirically-in-principle and by decision. **Verdict:
remove `anchor:top`.** The investigation confirmed the two modes converge for Lyx's real
topology (single-rooted per window; every mother has exactly one chain of descendants),
the layout math is deterministic and already unit-tested, and the one genuine behavioral
divergence — `anchor:top` is compact *unconditionally* while `below-parent`+shrink only
collapses when a live descendant is present — was resolved as "childless full-height is
acceptable, even useful". So `anchor:top` earns no residual niche and is removed.

**Why now:** the mux layout leaf is being actively worked (`mux-server-crash`,
`TopBandRows` per-strand override, mouse-default, remove-last-pane — all recently done).
Carrying a second, static placement mode that nothing in production uses (no producer,
loom, or perch code emits `--anchor top`; only the CLI value, render tests, and the
unbuilt `loom.md` design doc reference it) is dead surface area on a shared, closed-
vocabulary leaf. Removing it now — before `loom` is built against it — is cheaper than
after.

## Scope

**In:**

- Remove the `AnchorTop` value from the render `Anchor` vocabulary and every code path
  that interprets it: `partitionByAnchor`'s top branch, `Rules`' entire top-band
  placement block (including the "last top band stretches to absorb leftover rows"
  special case and the top-only focus fallback), and `validateAnchor`'s acceptance of it.
- Remove the `TopBandRows` machinery entirely: `render.Params.TopBandRows`,
  `render.Display.TopBandRows` (the per-strand override added by the `TopBandRows` task),
  the `top_band_rows` config field (`internal/muxengine/config.go`) and its lines in
  both `template_posix.yaml` / `template_windows.yaml`, the engine wiring in
  `apply.go`, and the `--top-band-rows` CLI flag plus the `--focus`-with-`--anchor top`
  rejection guard in `add.go`.
- Update every test that exercises `anchor:top` / `TopBandRows`: migrate fixtures to
  `below-parent`, delete top-band-specific golden cases and override tests (see
  **Testing**).
- Migrate the one design-doc consumer: `docs/modules/loom.md`'s `lyx loom status` strand
  from `anchor:top, height:fixed(1)` to `below-parent` + `ShrinkWhenWaitingOnChild`.
- Sandbox mux-suite: retire the top-band scenario (M6), rewrite the mixed-adds scenario
  (M12) to a below-parent-only shape, and **add a new operator-run scenario (M18)** that
  exercises a below-parent *root* mother + child (full-alone → collapse-on-child, plus
  plain-text legibility at `collapsed_strip_rows`). Update `docs/reviews/mux-review-prompt.md`'s
  TOP-BAND LEGIBILITY items accordingly.

**Out:**

- `AnchorBelowParent`, `AnchorHidden`, and `AnchorOwnWindow` (deferred) all stay. The
  `Anchor` type itself stays — it just loses one member. `own-window` and its `Rules`
  rejection are untouched.
- The `ShrinkWhenWaitingOnChild` / `collapsed_strip_rows` / `min_full_rows` mechanics are
  not changed — this task removes a mode, it does not retune the shrink policy.
- No change to the `resequenceByPaneOrder` / `paneOrder` positional contract — below-parent
  stacks still rely on it.
- Building `loom` / `lyx loom status`. This task only migrates loom.md's *design intent*
  for the status strand; the module is unbuilt and out of scope.
- Running the live sandbox test. Delivering the M18 scenario is in scope; the operator
  runs it out-of-band in the sandbox Hub — it is a confirmation checkpoint, **not** a
  mill-go gate (mill-go cannot drive a live-psmux visual session).
- Any `own-window`/cluster-window work (roadmap milestone 24) — orthogonal and deferred.

## Decisions

### remove-not-deprecate-not-narrow

- Decision: Fully **remove** `anchor:top` and all `TopBandRows` config/override/flag
  surface, rather than deprecating (CLI rejects it, code stays) or narrowing (keep for a
  documented niche).
- Rationale: The redundancy thesis holds for Lyx's real topology (operator-confirmed
  single-rooted-per-window, so `orderStack`'s global-depth sort never interleaves
  independent trees). Nothing in production emits `--anchor top`. Keeping dead code on a
  closed-vocabulary leaf invites the exact "is this redundant?" rediscovery this task
  exists to end. Full removal leaves no dead vocabulary member and no config knob that
  serves nothing.
- Rejected: *Deprecate* — leaves the render code and a confusing half-supported CLI value
  in place indefinitely. *Narrow* — only justified if a real always-compact-childless-
  status-line need existed, which the residual-case decision below ruled out.

### childless-full-height-is-acceptable (the residual-case verdict driver)

- Decision: A `below-parent` root "mother" strand with no live in-window descendant
  rendering at **full height** (rather than collapsed) is acceptable — even desirable, since
  a status line that is momentarily alone has more room to show detail. Lyx does **not**
  need a persistently-compact status line that stays a thin strip while childless.
- Rationale: This is the single behavior `anchor:top` provided that `below-parent`+shrink
  does not (`anchor:top` is compact unconditionally; shrink is descendant-gated via
  `isAncestor`, `render/focus.go:33`). Ruling the childless-full-height case acceptable is
  what makes `anchor:top` fully redundant and removal unconditional. The realized instance
  of this case is loom's own `lyx loom status` strand between/without forked children;
  full-height there is fine.
- Rejected: "Keep anchor:top narrowly for the childless-compact case" (would preserve the
  whole top-band + TopBandRows surface to serve one unbuilt, speculative need) and "defer
  the decision until loom is built" (leaves the dead mode in place across an unknown horizon;
  the design question is answerable now and was answered).

### empirical-test-as-operator-run-sandbox-scenario

- Decision: The proposal's "empirical test" (a live below-parent mother/child run) is
  delivered as a **new sandbox mux-suite scenario (M18)**, run by the operator out-of-band
  against the sandbox Hub. The code decision proceeds on the deterministic height tests
  (already green); the sandbox run confirms, it does not gate.
- Rationale: `lyx mux` cannot run in this repo — only the sandbox Hub host repo is set up
  for a live psmux server (`tools/sandbox`, `sandbox-mux-suite.cmd`). The test is inherently
  operator-watched and visual (legibility), so it cannot be an autonomous mill-go batch. The
  layout geometry it would confirm is already proven deterministically by
  `render/height_test.go` (notably `TestStackHeightsActiveStrictlyTallestWithSingleAncestor`
  — a parentless shrink-mother + child, the exact loom shape). M18 is the durable, repeatable
  form of the "run the empirical test" ask.
- Rejected: Run it live in this session (impossible in this repo); fold silently into M12
  without a dedicated root-mother scenario (loses the specific full-alone→collapse-on-child +
  legibility checks); make it a hard precondition blocking plan-writing (blocks the autonomous
  flow on a manual step for evidence the unit tests already provide).

### staging-must-preserve-green-build

- Decision: Stage the removal as **replace-then-delete**, not "delete the type first, fix
  callers later". Recommended batch shape (mill-plan owns the final DAG):
  1. **Migrate all usages** off `anchor:top`/`TopBandRows` while the symbols still exist:
     switch every test/shuttle fixture to `below-parent`, make the CLI reject `--anchor top`
     (falling to the invalid-anchor error), stop wiring `TopBandRows` in `apply.go`, drop the
     `--top-band-rows` flag behavior. Tree stays green (symbols defined but unreferenced).
  2. **Delete the now-orphaned symbols**: `AnchorTop`, `Display.TopBandRows`,
     `Params.TopBandRows`, `config.TopBandRows` + template lines, `partitionByAnchor`'s top
     branch, `Rules`' top-band block + stretch special case + top focus fallback,
     `validateAnchor`'s `AnchorTop` case. Tree green because nothing references them.
     **Sweep every residual `top`/`top-band` reference in retained code comments AND
     user-facing CLI strings — not just the two enumerated doc spots.** Concretely that
     includes: `policy.go`'s file doc and `partitionByAnchor` doc ("the fixed top-band
     set"), `rules.go`'s `Rules`/paneOrder comment ("top bands first"), `types.go`'s "exactly
     these four values" doc comment, `template.go`'s config-key-list comment (drops
     `top_band_rows`), and the two user-facing `add.go` strings: the invalid-anchor error
     (`add.go:66`, `want top|below-parent|hidden` → `want below-parent|hidden`) and the
     `--anchor` flag usage (`add.go:113`, `placement: top|below-parent|hidden` →
     `placement: below-parent|hidden`). Leaving any of these is exactly the dead surface this
     task exists to remove.
  3. **Docs + sandbox** (independent, compile-neutral): `loom.md`, `overview.md` (line ~327
     generic anchor mention — verify still accurate), `mux-review-prompt.md`, and the
     SANDBOX-MUX-SUITE.md M6-retire / M12-rewrite / M18-add.
- Rationale: mill's per-batch green-build invariant forbids removing `render.AnchorTop` in one
  batch while `shuttleengine`/tests still reference it in a later batch — that leaves the tree
  uncompilable between batches. Replace-then-delete keeps every batch green.
- Rejected: One giant atomic batch (harder to review, and the docs/sandbox work needn't share
  a batch with the code churn); delete-first (breaks the build between batches).

### keep-partitionByAnchor-simplified

- Decision: After removal, simplify `partitionByAnchor` to return only the below-parent
  stack (drop the now-always-empty `top` return), and collapse `Rules` to
  breakCycles → partition → orderStack → `stackHeights` over the full box → resequence →
  focus. The `own-window` exclusion and the `AnchorHidden`/non-live/empty-PaneID filter stay.
- Rationale: A two-return partition where one return is always empty is misleading dead
  structure. `Rules` no longer needs the `y`-cursor top-band loop, the stretch special case,
  or the `focus == "" && len(top) > 0` fallback (there is always a stack now, or nothing).
- Rejected: Keep the `(top, stack)` signature with `top` permanently empty (retains dead
  shape and the top-band block "just in case").

## Technical context

Render is a **pure leaf** (`internal/muxengine/render`) with a deliberate two-layer split
that must not merge: **policy** (`policy.go`, `height.go`, `focus.go` — interprets the
closed `Anchor` vocabulary, does height math) and **mechanics** (`layout.go`, `checksum.go`
— turns decided placements into a tmux `window_layout` string). This removal is a
policy-layer change; **mechanics must not be touched** (`types.go` package doc; a repo
design rule).

Key files and what changes:

- `render/types.go` — remove `AnchorTop` const; remove `Display.TopBandRows` and
  `Params.TopBandRows` fields; fix the "exactly these four values" doc comment. `Display`'s
  JSON tags are an on-disk contract (`shuttle` persists `Display` verbatim into `mux.json`),
  so dropping `topBandRows` is a persisted-shape change — old records carrying it will simply
  be ignored on load (Go's json ignores unknown fields); no migration needed, but note it.
- `render/policy.go` — `partitionByAnchor` loses its `AnchorTop` case (see
  keep-partitionByAnchor-simplified).
- `render/rules.go` — remove the top-band reservation loop (`rules.go:53-69`), the
  `isLastTop && len(ordered)==0` stretch case, and the `focus == "" && len(top) > 0`
  fallback (`rules.go:85-87`). The `AnchorOwnWindow` rejection at the top stays.
- `render/height.go`, `render/focus.go` — **no change**; already stack-only. `isAncestor`
  is the mechanism the retained shrink behavior depends on.
- `muxengine/config.go:27` — remove `TopBandRows` field; `apply.go:75` — remove
  `TopBandRows: e.cfg.TopBandRows` from the `render.Params` construction; `template.go:15`
  comment — drop `top_band_rows` from the config-keys list.
- `muxengine/strand.go` — `validateAnchor` (`strand.go:50`): drop `AnchorTop`, change the
  error message `want top|below-parent|hidden` → `want below-parent|hidden`.
- `muxcli/add.go` — drop `AnchorTop` from the vocab switch (so `--anchor top` hits the
  invalid-anchor error), remove the `--focus`+`anchor top` guard (`add.go:73-76`), remove the
  `--top-band-rows` flag (`add.go:115`) + its `topBandRows` var + the `TopBandRows:` field in
  the `render.Display` it builds. Update the `Short`/`Long` help text if it names top.
- Both `template_*.yaml` — delete the `top_band_rows: 3` line (line 6).

Production reality check (informs scope, not the plan's code): **no code emits `anchor:top`.**
`grep` for `--anchor top` / `AnchorTop` across `internal/loom*`, `perchengine`, `perchcli`,
`cmd` is empty. The only references outside render+tests are `docs/modules/loom.md` (design
doc for the unbuilt loom status strand) and `docs/reviews/mux-review-prompt.md`. `loom` is not
built (only `perchcli`/`perchengine` exist).

Config defaults (both templates): `top_band_rows: 3`, `collapsed_strip_rows: 3`,
`min_full_rows: 3`. Note the mother/child layouts already converge at 3 rows — the
`height:fixed(1)` in loom.md is stale.

## Constraints

- `CONSTRAINTS.md` codifies **no** anchor/render-vocabulary invariant (grep-verified), so
  removing `AnchorTop` breaks no recorded invariant and adds none. Do **not** invent a new
  CONSTRAINTS entry for this.
- The **render two-layer split** (policy vs mechanics) is a design rule in the package doc
  (`types.go`): this change stays in the policy layer; `layout.go`/`checksum.go` stay
  untouched.
- **CLI / Cobra Invariant** (CLAUDE.md): `add.go` keeps `Short` on every command; if the
  `--anchor` help string or `Long` names `top`, update it. Help-tree tests must stay green.
- **mill per-batch green-build invariant**: every batch must leave `go build` + `go test`
  green — this forces the replace-then-delete staging (see staging-must-preserve-green-build).
- **Documentation Lifecycle** (CLAUDE.md): this changes observable CLI behavior
  (`--anchor top` and `--top-band-rows` disappear) and a shared module's design, so docs move
  **in the same batch/commit** as the code — `loom.md`, the sandbox suite, and
  `mux-review-prompt.md`. `docs/roadmap.md` is **not** touched: this is a simplification, not
  a planned milestone, and no roadmap milestone references anchor:top.
- **Sandbox black-box rule**: M18 is authored in `SANDBOX-MUX-SUITE.md` and driven via
  `lyx mux` verbs only; the `**Covers:** mux` coverage tag lives on M2 and is unaffected. M-refs
  are stable ids — retire M6 with a tombstone note rather than renumbering M7–M17.

## Testing

The existing render tests **are** the spec; migration is the bulk of the test work.

- **`render/rules_test.go`** (17 `AnchorTop` occurrences): delete every top-band golden case
  in `TestRulesGolden` (top-band placement, `>=2 top / 0 stack` stretch, top+stack mixes) and
  the `TopBandRows` per-strand override cases. Keep/confirm a golden that proves the loom shape
  at the `Rules` level: (a) a lone below-parent root strand → fills the full box; (b) root
  mother + child → mother at `collapsed_strip_rows`, child gets the remainder. Add (a)/(b) if
  not already present — (a) is the childless-full-height behavior this task explicitly endorses.
- **`render/policy_test.go`** (4): remove `partitionByAnchor` top-partition cases; if the
  signature is simplified, update remaining cases to the stack-only return.
- **`render/height_test.go`**: no removal needed — it is already `below-parent`-only and
  already covers the root-mother/child collapse
  (`TestStackHeightsActiveStrictlyTallestWithSingleAncestor`). This is the linchpin coverage
  the removal relies on; keep it.
- **Engine tests** — migrate `AnchorTop` fixtures to `AnchorBelowParent` and drop
  `TopBandRows` assertions: `apply_test.go` (5), `state_test.go` (2), `strand_test.go` (2,
  incl. the `validateAnchor` rejection test — update its expected error string and add a case
  asserting `top` is now rejected), `config_test.go` (2, drop `top_band_rows` load assertion),
  `io_test.go` (1), `contract_integration_test.go` (1), `lifecycle_test.go` (1), `lock_test.go`
  (1).
- **Shuttle tests** — `shuttleengine/spec_test.go` (3) and `run_test.go` (2): swap
  `render.AnchorTop` fixtures to `render.AnchorBelowParent`; these assert `Display` round-trips,
  so the anchor value is incidental.
- **CLI** — add/confirm a `muxcli` test that `--anchor top` is now rejected with
  `want below-parent|hidden`, and that `--top-band-rows` is no longer a registered flag.
- **TDD candidate**: the `validateAnchor` / CLI rejection of `top` (small, pure, error-string
  assertion) is the natural test-first slice. The golden-file removals are edit-existing, not TDD.
- **Sandbox M18** (operator-run, not a Go test): assert a below-parent root mother is full
  height while alone, collapses to `collapsed_strip_rows` once a child strand is added under it
  via `--parent`, the child holds the bulk of the window, and a **plain-text** status line is
  legible at the collapsed height (contrast with the box-drawing-TUI corruption that motivated
  `TopBandRows`).

## Q&A log

- **Q:** How is the proposal's "empirical test" satisfied, given `lyx mux` can't run in this
  repo? **A:** It runs only in the sandbox Hub host repo (`tools/sandbox` / `sandbox-mux-suite.cmd`).
  Deliver it as a new operator-run mux-suite scenario (M18); the code decision proceeds on the
  deterministic height tests, with the sandbox run as confirmation, not a mill-go gate.
- **Q:** Verdict lean — pre-decide, or keep open? **A:** Open, framed on the one residual case
  (persistently-compact childless status line) — which then resolved to remove (see next).
- **Q:** Does Lyx need a persistently-compact status line with NO live in-window child? **A:**
  No — childless full-height is acceptable, even useful. This makes `anchor:top` fully
  redundant → unconditional removal; loom's status strand becomes `below-parent`+shrink.
- **Q:** Deprecate, narrow, or remove? **A:** Full removal, staged multi-batch — but staged as
  replace-then-delete so every batch keeps `go build`/`go test` green.
- **Q:** Is the root-mother-collapse behavior (the thing replacing anchor:top) already tested?
  **A:** Yes — `render/height_test.go:TestStackHeightsActiveStrictlyTallestWithSingleAncestor`
  covers a parentless shrink-mother + child. The removal does not rest on untested behavior.
- **Q:** Does this touch a CONSTRAINTS invariant or a roadmap milestone? **A:** Neither — no
  codified anchor/render-vocab invariant, no roadmap milestone for anchor:top. Don't add a
  CONSTRAINTS entry, don't touch roadmap.
