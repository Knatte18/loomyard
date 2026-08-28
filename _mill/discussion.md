# Discussion: reed: attach's layout computation scales header pane height with terminal height

```yaml
task: "reed: attach's layout computation scales header pane height with terminal height"
slug: reed-attach-header-height-bug
status: discussing
parent: main
```

## Problem

The reed header pane is supposed to be exactly `header.height_rows` tall (default 1) — a one-row strip showing ` hub: <path>`.
The operator instead saw it 10-plus rows tall, filled with the header process's own startup WARN scrollback rather than the rendered header line.
The original finding attributed this to `lyx reed attach`'s chained "layout computed for this terminal's own size" step, and observed a threshold: header height stays 1 up to roughly 50 terminal rows and grows beyond it.

Live investigation in this worktree, against real tmux 3.6 and a real `lyx reed` session, refines that attribution — and the refinement changes where the fix belongs:

1. **The attach chain is not broken.**
   Booting a real session (`lyx reed up` + two `lyx reed add`), then attaching from a 127x76 harness terminal, leaves the header at exactly 1 row and the window at 127x76.
   The chained `select-layout` lands its planned string verbatim.
   No suppression warning is logged.
2. **The defect is that tmux redistributes *every* window-size delta evenly across the vertical cells, and reed never re-pins the header afterwards.**
   Continuing from that same healthy attached session and resizing the client terminal:
   - 76 → 90 rows: header 1 → **6**
   - 90 → 120 rows: header 6 → **16**
   tmux has no notion of a fixed-height pane; `layout_resize` hands out the extra rows one at a time, round-robin, and the header takes an equal share.
3. **The reported ~50-row threshold is the boot height, and it identifies the sub-case where the chain does *not* run.**
   `reed.yaml` ships `height: 50` (`new-session -y 50`), so a bare, unchained attach (`AttachArgv(0, 0)`, or any degraded pre-flight) leaves the window at 50 rows until the client's own resize lands.
   Reproduced synthetically: a bare attach from a 127x76 client takes the header from 1 to **10**, from 127x40 leaves it at 1, and from 127x50 leaves it at 1 — exactly the reported table.
   So the operator's headless repro was measuring the *bare* attach path (or a harness pane that resized after the attach), not a miscomputed layout.

Either way the same single mechanism is responsible, and it is broader than attach: **any window resize inflates the header, and a routine terminal resize after a healthy attach reproduces the bug on demand.**
A fix that only hardens the attach chain leaves the reported symptom fully reachable.

**Why now:** this is an M14/M19 scenario FAIL — M19 requires the header stay at its configured `height_rows` and show the rendered header line.
The inflated pane is also what makes `reed-header-pane-boot-noise` visible: the header process's startup WARN lines are always in the pane's scrollback, and only a taller-than-1-row pane reveals them.

## Scope

**In:**

- A tmux `window-resized` window hook, installed by `internal/reedengine`, that re-pins every fixed-height pane via `resize-pane -y` after tmux has finished redistributing a resize.
- Pins for the header pane and for every collapsed strip pane, each at the height `render` actually placed it at — that is, after `clampHeaderHeight` for the header and after `clampToFit` for both.
  Those are the two cells whose heights are absolute row budgets rather than "whatever is left".
- A pure entry point in `internal/reedengine/render` that reports those fixed-height pins for the same strand set and box `Rules` was given, so the hook is derived from reed's own policy rather than from raw config.
- Hook installation/refresh at two named statements: in `applyLayoutLocked`, immediately after the `select-layout` call returns without error and before the `select-pane` call; and in `AttachArgv`, inside the `withOpLock` closure immediately after `planLayout` returns without error and before `chained` is assigned.
  Every guard and degrade return at both sites precedes its install statement, and none of them is changed — see `hook-install-points-are-named-statements`.
- Documentation work at four named sites in `internal/reedengine` — two `doc.go` geometry bullets, `attachgeometry_integration_test.go`'s file comment, and `doc.go`'s "Subcommand set" paragraph plus its closing psmux-risk sentence — plus one narrowing edit to `manifest/roadmap.md`'s watchdog-daemon item.
  See `doc-work-is-additions-at-four-named-sites`, `set-hook-and-resize-pane-are-optional-wire-surface`, and `roadmap-watchdog-item-is-narrowed`.
- A new `TestMultiplexerContract` case in `contract_integration_test.go` for the two tmux verbs this task adds to reed's wire surface.
- Tests: unit tests for the pure pin computation and the pure hook-argv construction, plus a real-tmux/real-pty integration case that resizes the client after attach.

**Out:**

- The header process's own startup log noise.
  That is `reed-header-pane-boot-noise`; this task only stops the pane from being tall enough to reveal it.
- The missing tmux status bar noted as a secondary observation in the task body.
  It is deliberate: `pinGeometryOptionsLocked` (`internal/reedengine/windowsize.go`) pins `status off` on purpose, and the whole told-box arithmetic (`rows - reserved`) depends on it.
  Not a defect; no change.
- Removing or rewriting the attach chain.
  It demonstrably works and it is the only mechanism that restores reed's *full* layout policy on attach; the hook is additive.
- Preserving reed's exact strand split (equal, remainder to the active pane) across a resize.
  After a resize tmux's round-robin leaves full panes slightly uneven (measured 52/45 where reed's policy says 48/49).
  Both panes are full panes with no absolute budget, so this is cosmetic and not worth a process spawn per resize.
- `render.Rules`' signature.
  It stays `(layout, focus, err)`.
- Any change to `internal/reedcli`.
  The CLI's `attach` verb, its terminal-size read, and its envelope discipline are unaffected.

## Decisions

### hook-mechanism-is-a-pure-tmux-resize-pane

- Decision: fix the drift with a `window-resized` window hook holding one `resize-pane -y` command per fixed-height pane, installed by reed and executed by the tmux server itself — pure tmux, no process spawned.
- Rationale: verified live against the real session — with the hook installed, the header held at 1 row across 120→76, 76→100, 100→45 and 45→8 client resizes, and at 8 rows tmux degraded gracefully (header 1, strands 4 and 1) instead of erroring.
  It costs no process spawn, takes no op lock, needs nothing on `PATH`, and survives a drag-resize storm.
  It also fires on the attach-time resize, so a *degraded* attach — one that falls back to the bare argv — is still corrected, provided an earlier `applyLayoutLocked` installed the hook, which the first `add` or strand-placing `resume` does.
  A session that has never placed a strand is the one case it does not cover; see `hook-install-points-are-named-statements`.
- Rejected: `client-resized` — verified to fire *before* the layout is resized (`#{window_height}` still reads the old value inside the hook), so a `resize-pane` there is undone microseconds later; measured header 6 instead of 1.
  `window-layout-changed` works identically to `window-resized` in testing but also fires on reed's own `select-layout`, inviting re-entrancy for no benefit.
  A hook running `run-shell -b '<lyx> reed relayout'` would restore full policy fidelity, but costs a new verb, a process per resize, op-lock contention during a drag, and an absolute-exe dependency — all to fix a cosmetic strand split.
  Chaining a `resize-pane` onto the attach argv fixes nothing: like the chained `select-layout`, it runs before any later resize.

### pins-come-from-render-policy-not-raw-config

- Decision (engine seam): the pins are produced from the **same mapping `planLayout` already performs** — one call site, not a second one.
  `planLayout` (`apply.go`) owns `toRenderStrands`, the present-pane filtering, and the `HeaderPaneID` blanking the zero-pin case depends on; a second mapping site would be free to diverge from it silently, and the zero-pin disposition would then be computed from a different header id than the layout was.
  Whether that surfaces as an extra return from `planLayout` or as a small shared helper both it and the pin path call is left to the plan; what is fixed here is that the mapping happens once.
- Decision: the pinned heights are the ones `render.Rules` actually placed the cells at — the header's height *after* `clampHeaderHeight`, and each strip's height *after* `clampToFit`, never `cfg.Header.HeightRows` or `cfg.CollapsedStripRows` read raw.
  A new pure function in `internal/reedengine/render` reports them, sharing `Rules`' policy composition rather than duplicating it, and returns the strip pins from the same `placements` slice `stackHeights` produced.
- Rationale: `render` is the single owner of reed's height policy, and both budgets yield under pressure.
  Pinning raw `cfg.Header.HeightRows` would let the hook contradict `clampHeaderHeight`'s "the header yields rows first" rule on a short window, and pinning raw `cfg.CollapsedStripRows` would contradict `clampToFit` exactly the same way — `height.go`'s priority-1 pass reclaims strip rows first, down to 1, so a strip's placed height is below `CollapsedStripRows` on any window short enough to trigger it.
- Rejected: parsing the layout string reed just built to recover cell heights — it cannot distinguish a fixed budget from a computed one, so it cannot say which cells to pin.
  Changing `Rules` to return a fourth value or a plan struct — a wider blast radius across both call sites and their tests than this bugfix needs.

### pins-are-a-snapshot-refreshed-at-every-apply

- Decision: the hook is a snapshot of the pins computed for the box at install time, fully rebuilt on every successful `applyLayoutLocked` and in `AttachArgv`'s pre-flight.
  A pure resize does not recompute it.
- Rationale: every event that changes which panes are strips — add, remove, resume, reconcile — already routes through `applyLayoutLocked`, so the pin *set* is never stale; only a pinned *height* can be, and only through the clamp.
- Rejected: recomputing pins inside the hook (needs the `run-shell` design already rejected).
  Installing once at boot in `pinGeometryOptionsLocked` — that function has no access to the strand table or the header pane id, and the header pane can be recreated with a new `%N` id.
- Known limitation to document: any clamp-derived pin — the header's `clampHeaderHeight` value and every strip's `clampToFit` value alike — is computed for the box at install time, so an operator who shrinks the terminal past a clamp threshold with no intervening reed op keeps a pre-shrink pin.
  It is bounded in both directions: `resize-pane` cannot starve the stack below tmux's own one-row floor, and at the shipped `height_rows: 1` / `collapsed_strip_rows: 3` the clamps are no-ops on any window with room for the pane count.
  It self-corrects on the next reed op.

### hook-body-is-one-array-entry-per-pin

- Decision: encode the hook as one tmux hook-array entry per pin, not as one multi-command string.
  A refresh is `set-hook -u -w -t "=<session>:" window-resized` to clear the array, then `set-hook -w -t ... window-resized "resize-pane -t <id> -y <n>"` for the first pin and `set-hook -a -w -t ...` for each subsequent one, header always first.
  Each `resize-pane` is therefore a whole hook value in a single argv element — never a `";"` argv element of its own.
  **The clear is unconditional at the install statement, including when the pin list is empty.**
  Reaching the install statement with zero pins means `render` placed no fixed-height cell this time, and the only correct answer is an empty array — issuing nothing would leave a previously installed pin clamping a pane `render` has since placed as a full pane, once per resize, forever.
  That case is reachable: `planLayout` blanks `st.HeaderPaneID` when the header pane is not in the present set, so an apply with no live header and no strip yields zero pins.
  This is the opposite disposition from the two guard-skip states in `hook-failure-is-non-fatal-everywhere`, and deliberately so — those never reach the install statement at all, so reed has computed no opinion to write; here it has computed one, and the opinion is "nothing is pinned".
- Rationale: verified live on tmux 3.6, and the reason is failure isolation.
  A single `";"`-separated command string works when every pane is alive (measured: header pinned at 1, strip at 3, across attach and a 76→100 resize), but a `resize-pane` naming a destroyed pane **aborts the rest of that command list** — with a dead id placed first, the header pin behind it never ran and the header ballooned to 25 rows.
  Separate array entries are independent: the same dead-id-first arrangement left entry `[0]` failing and entry `[1]` still pinning the header at 1 row.
  `set-hook -u` was verified to clear the whole array, so the rebuild cannot accumulate stale entries.
  A bare `";"` argv element is also simply wrong here, whatever the isolation argument: `set-hook` takes its body as one argument, so a separate `";"` element would terminate the `set-hook` command itself.
  `chainedAttachArgv`'s literal `";"` element (`attach.go`) is not a precedent — that one is parsed by tmux as a *top-level* command sequence, which a hook value is not.
- Rejected: one concatenated `";"`-separated string — one round trip instead of `1 + N`, but it fails closed in exactly the case the snapshot design makes reachable (a pinned `%N` destroyed between applies).
  The extra round trips are per reed op, not per resize, and there are at most a handful of pins.
- Accepted consequence: the clear-then-rebuild refresh is not atomic.
  A resize landing between the `set-hook -u` and the first `set-hook` sees no hook and drifts one round; the next resize corrects it, and `applyLayoutLocked` re-applies the full layout immediately afterwards anyway.

### hook-install-points-are-named-statements

- Decision: the hook is installed at exactly two statements, and no guard or degrade return at either site is moved or changed.
  In `applyLayoutLocked` (`apply.go`): immediately after `e.tmux.run("select-layout", ...)` returns without error, before the `select-pane` call — so both of that function's guards (`len(live) < 2` and `!anyPlacedStrand`) and a failed `planLayout` or `select-layout` all precede it.
  In `AttachArgv` (`attach.go`): inside the `withOpLock` closure, immediately after `planLayout` returns without error and before `chained` is assigned — so all eight of that closure's earlier degrade returns (`cols <= 0 || rows <= 0` before the lock is even taken, `requireSessionLocked`, `readWindowSizeLatestLocked`, `readStatusRowsLocked`, `loadOrInitStateLocked`, `listPanes`, `len(live) < 2`, `!anyPlacedStrand`) precede it too.
- Rationale: both statements sit where the pins are already computed against the same box the layout was, and where the strand table and pane list are already in hand — no extra I/O, no new lock, no reordering of a degrade ladder whose ordering is load-bearing.
- Consequence, stated rather than coded around: a degrade path installs no hook on that call.
  In practice this only matters for a session that has *never* reached either install statement.
  A fresh `up` is not an installer: `lifecycle.go` clears every pane binding and blanks `HeaderPaneID` on boot, so the `reconcileApplyPersistLocked` that follows hits `!anyPlacedStrand` and installs nothing.
  The first installer is the first `add`, or a `resume` that places a strand — after which any attach with a readable size refreshes it.
  The uncovered window is therefore a session between `up` and its first placed strand, attached with no readable terminal size.
  That window is also the one in which there is nothing to pin: with only the header pane live, `render.Rules` takes its sole-cell branch.
  Neither branch should install a hook anyway: with a single live pane `render.Rules` takes its sole-cell branch and gives the header the entire box (`rules.go`), so there is no fixed-height budget to pin; and `AttachArgv`'s no-size return is lock-free by contract, so installing there would mean taking the op lock, loading state, and listing panes on exactly the path built to skip all three.
  The gap is bounded and self-healing — the first `add`, or the first `resume` that places a strand, installs the hook.
- Rejected: installing before the guards, so every path gets a hook — it would pin heights `render` never placed, on sessions whose layout reed deliberately did not apply.

### hook-failure-is-non-fatal-everywhere

- Decision: a failed `set-hook` is logged via `logger.Warn` and ignored; neither `applyLayoutLocked` nor `AttachArgv` may fail because of it.
- Rationale: this is the existing Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` (`internal/reedengine/lifecycle.go`, `windowsize.go`), and it is what makes the change safe on psmux, whose `set-hook`/`window-resized` support is unverified anywhere in this repo — exactly the situation the `status`/`window-size` pins are already treated this way for.
  `attach` additionally may never be blocked by an engine-side failure at all, by its own contract.
- Rejected: probing psmux capability first — reed's capability probe (`probe.go`) is a boot-time version gate, and adding a runtime probe for a strictly-optional quality option would cost a round trip per apply for no behavioural gain.
- Fire-time failure, the separate case: an installed hook runs inside the tmux server, where reed has no return value to inspect and nothing to log.
  The reachable failure is a pinned `%N` naming a pane destroyed since the last install — a snapshot hook makes that reachable by design, and reconcile does destroy panes.
  Disposition: the array encoding (`hook-body-is-one-array-entry-per-pin`) contains the blast radius to the one entry that failed, so a dead strip pin can no longer take the header pin down with it, and the header is entry `[0]` so it fires before any strip pin can go wrong.
  Beyond that it is accepted, and self-healing on the paths that reach the install statement: a pane disappears through reconcile, `remove`, or a dead process, and each of those routes back through `applyLayoutLocked` — which rebuilds the array *provided* neither of its guards fires first.
  A pane id is never reused within a tmux server incarnation, so a stale pin can only fail, never hit the wrong pane; across a server restart reed already discards every binding minted against a different pane generation.
- The two guard-skip states leave a stale array installed, deliberately, with no removal path.
  `len(live) < 2` leaves pins naming panes that are mostly gone, and a header pin that now contradicts `render.Rules`' sole-cell branch — but `resize-pane -y` against the only pane in a window is a verified silent no-op (exit 0, height unchanged), so the contradiction cannot express itself.
  `!anyPlacedStrand` is the reachable, long-lived one: `state.go` documents an operator remedy that deletes `reed.json` while the session and its processes keep running untracked, and `anyPlacedStrand` is then false forever.
  There the stale array is a benefit, not a hazard — it keeps pinning the still-alive header and strips at the budgets reed last computed for them, which is what the operator would want from a session reed has stepped back from managing.
- Rejected: moving the `set-hook -u` clear ahead of the guards so every call site clears.
  It would strip the pins from exactly that untracked-but-running session, and a clear with no rebuild behind it is strictly worse than a slightly stale array — a cleared hook drifts on the very next resize, while a stale one keeps working for every pin whose pane is still alive.

### attach-chain-is-kept-and-its-docs-corrected

- Decision: keep `AttachArgv`'s chained `select-layout` and its whole told-box pre-flight unchanged; correct only the comments that credit it with holding the header at its budget.
- Rationale: the chain works — it was verified landing its planned string byte-for-byte on a real attach — and it is the only path that restores reed's full policy (equal split, remainder to active) on attach, repairing whatever drift a previous session's resizes left behind.
  Deleting a working, integration-tested mechanism is not this bugfix's job.
- Rejected: deleting the chain and its pre-flight in favour of hook-only — smaller code, but it drops the one place reed's complete layout policy is reasserted for a new client, and it churns a large tested surface for a defect the chain does not cause.

### roadmap-watchdog-item-is-narrowed

- Decision: edit `manifest/roadmap.md`'s **reed: watchdog daemon** item in this task's commit, narrowing its resize clause to what this fix does not deliver, and recording that a `window-resized` + `resize-pane` reaction is now shipped.
  The item stays on the roadmap; only its resize half shrinks.
- Rationale: that item currently owns "reconciles session geometry after a live terminal resize" and explicitly records that an earlier task's discussion *rejected* a one-off tmux `set-hook`+`run-shell` reaction, on the grounds that a shared daemon needing event-driven hooks for the pane-reap job would pay for hook infrastructure and psmux verification once rather than per job.
  This task ships a `set-hook` resize reaction, so leaving the item as written would put a shipped mechanism and a documented rejection of it side by side in the plan of record.
  The daemon rationale survives in narrowed form: this fix pays only for `window-resized` + `resize-pane`, a hook with no `run-shell` and no out-of-tmux actor, while the pane-reap job still needs its own hook class, its intentional-vs-bug-induced policy, and the cheaper reap probe named as its prerequisite — and the resize job's remaining half, re-rendering reed's *full* layout policy (the strand split, which tmux leaves uneven), still needs an actor outside tmux.
- Rationale for moving the roadmap at all: CLAUDE.md holds `manifest/roadmap.md` still for bugfixes, hardening, and polish passes.
  This commit is a bugfix, but it also changes the scope of a planned item, which is the case that rule leaves open.
  Silently shipping a mechanism a roadmap item rejects is the failure the rule exists to prevent, not an instance of it.
- Rejected: deleting the item — the pane-reap job and the full-policy resize re-render are untouched by this fix.
  Leaving it unedited and noting the overlap only in `doc.go` — the plan of record is the roadmap, and a reader planning the daemon would start from the stale rejection.

### set-hook-and-resize-pane-are-optional-wire-surface

- Decision: `set-hook` and `resize-pane` are the first tmux verbs reed uses **without** probing for them.
  `requiredSubcommands` (`probe.go`) does not grow, and `doc.go`'s "Subcommand set" paragraph gains a sentence splitting the surface in two: the required verbs, whose absence makes a multiplexer binary unusable, and these two optional ones, whose absence costs only the header pin.
- Rationale: both verbs are new to `internal/` — neither appears anywhere in the package today — so this task genuinely widens the wire contract, and `doc.go`'s current closing sentence ("`requiredSubcommands` … add no capability-probe change and no new psmux risk") stops being true the moment the hook ships.
  But adding them to `requiredSubcommands` would make a psmux lacking `set-hook` fail the *whole* capability probe at server-ensure, taking down every reed verb over a quality-only option that is already designed to degrade silently — the exact trade `geometry-tmux-failures-are-non-fatal-everywhere` settles the other way for `status`/`window-size`.
  So the honest disposition is to widen the doc, not the gate.
- `contract_integration_test.go`'s `TestMultiplexerContract` gains a case, since it is the named canary for precisely this wire surface and the fix's correctness rests on two behaviours no unit test can assert: that a `set-hook -u` / `set-hook` / `set-hook -a` sequence produces independent array entries readable back through `show-hooks`, and that a `window-resized` hook fires *after* tmux has resized the layout (which `client-resized` does not — measured).
  It self-skips when the configured binary is absent, so adding the case costs psmux nothing.
- Rejected: adding the two verbs to `requiredSubcommands` — see above.
  Leaving `TestMultiplexerContract` alone with an "not covered, because non-fatal" note — non-fatal describes what happens when the verb is missing, not whether its semantics are what reed assumed, and the latter is the whole reason this file exists.

### doc-work-is-additions-at-four-named-sites

- Decision: the documentation work is additions, not corrections — with one exception, `doc.go`'s closing "no new psmux risk" sentence, which this task falsifies and which is rewritten rather than extended (see `set-hook-and-resize-pane-are-optional-wire-surface`).
  Four named sites: (1) `doc.go`'s geometry bullet list gains a bullet for the window-resize round-robin and the hook that answers it, stating that tmux distributes a resize delta one row at a time across the vertical cells and that no absolute row budget survives a resize without the hook;
  (2) `doc.go`'s chained-attach bullet gains one sentence noting that "lands verbatim with no rescale" holds only until the next window resize, and pointing at the hook;
  (3) `attachgeometry_integration_test.go`'s file-level comment gains a note that its `100x30` client is shorter than the 220x50 boot box, so the existing cases exercise a shrink and never the growth path this task is about;
  (4) `doc.go`'s "Subcommand set" paragraph gains the required-versus-optional split, and its closing "`requiredSubcommands` … no new psmux risk" sentence is rewritten to say the probe still does not grow *and why that is now a deliberate choice* rather than a free consequence.
- Rationale: `doc.go`'s existing rescale bullet is about a mismatched layout *string*, a different mechanism from a window *resize*, and its chained-attach bullet's verbatim-landing claim was confirmed true by this task's own live measurements — so a reader who takes either as a promise that the header stays at its budget is filling in a gap, not reading a wrong sentence.
  Naming the three sites keeps the plan writer from hunting for a falsehood that is not there.
- Also record in `doc.go`, in the same bullet: the ~50-row threshold in the original report is `reed.yaml`'s `height: 50` boot size showing through the *bare* attach path, not evidence of a miscomputed layout.
  The original root-cause note points the next reader at `render/{rules.go,height.go}` and at the chain; both are correct as written, and leaving the misattribution in the record costs the next investigator the same day.
- Rejected: leaving it to the commit message — `doc.go` is where this package's geometry reasoning lives, and CLAUDE.md requires the module doc to move in the same commit.
  Rewriting the existing bullets rather than extending them — they are accurate, and churning them would lose the mismatched-string mechanism they do document.

## Technical context

**Where the mechanism lives**

- `internal/reedengine/render/` is a pure leaf: `Rules(strands, box, params, paneOrder) (layout, focus, err)` in `rules.go` composes `height.go`'s policy (`clampHeaderHeight`, `stackHeights`, `clampToFit`) with `layout.go`/`checksum.go`'s tmux string mechanics.
  It never touches tmux and always asks for `HeightRows` straight from config — it cannot itself produce a scaling height, which is what the original investigation already established.
- `rules.go` computes `headerHeight` via `clampHeaderHeight(p.Header.HeightRows, box.H-1, p.MinFullRows)`; `height.go`'s `stackHeights` marks a cell a strip when `isAncestor(s, stack) && s.Display.ShrinkWhenWaitingOnChild`, giving it `p.CollapsedStripRows`.
  Those two are the fixed-height cells the new pure function must report.
- `internal/reedengine/apply.go` holds `planLayout` (pure w.r.t. tmux; box is always told to it) and `applyLayoutLocked` (guards: `len(live) < 2`, `anyPlacedStrand`; then `liveBoxLocked` → `planLayout` → `select-layout` → `select-pane`).
  Its doc comment already documents tmux's silent rescale of a mismatched layout string; it does not yet document the resize round-robin.
- `internal/reedengine/attach.go` holds `AttachArgv`, which composes the whole pre-flight under one `withOpLock` and degrades to `bareAttachArgv` on every failure.
  It already loads state and lists panes, so the pins are available there with no extra I/O.
- `internal/reedengine/windowsize.go` holds `pinGeometryOptionsLocked` (`status off`, `window-size latest`), the two readbacks, and `liveBoxLocked`.
  A new `set-hook` helper belongs here, alongside them, and should follow their `logger.Warn`-and-continue shape exactly.
- `e.tmux.run(...)` / `e.tmux.output(...)` is the tmux seam; every call in this package goes through it, and the exact-target helpers are `exactSessionTarget` / `exactSessionWindowTarget`.

**Verified tmux behaviour (tmux 3.6, this machine)**

| observation | measurement |
| --- | --- |
| real reed session, chained attach at 127x76 | header 1 row, layout lands verbatim, no warning |
| same session, client resize 76 → 90 | header 1 → 6 |
| same session, client resize 90 → 120 | header 6 → 16 |
| synthetic bare attach, boot 220x50 → client 127x76 | header 1 → 10 |
| synthetic bare attach, boot 220x50 → client 127x40 or 127x50 | header stays 1 |
| `client-resized` hook + `resize-pane -y 1` | header 6 — the hook fires before the layout resize |
| `window-resized` hook + `resize-pane -y 1` | header 1 across attach and every subsequent resize |
| `window-resized` hook, window shrunk to 8 rows | header 1, strands 4 and 1 — no error |
| `#{status}` / `#{window-size}` readbacks after the pins | `off` / `latest` |
| hook body as one `";"`-separated string, all panes alive | header 1, strip 3 — holds across attach and a 76 → 100 resize |
| same single-string body, a dead `%99` pin placed first | header 25 — the dead command aborts the rest of the list |
| same dead-first arrangement as two `set-hook -a` array entries | header 1 — entry `[0]` fails, entry `[1]` still runs |
| `set-hook -u -w -t <win> window-resized` after three `-a` entries | `show-hooks` empty; a following `set-hook` rebuilds from `[0]` |

**Gotchas**

- The hook command must carry the pane's `%N` id, not an index: reed's header is pane index 0 today, but pane indices renumber and the header pane can be recreated with a new id.
  Refreshing on every apply is what keeps the id current.
- `set-hook -w` (window scope) is what was verified; a session-scoped hook was not tested and reed only ever uses one window.
- `set-hook` takes its body as a **single argument**, so each `resize-pane` is one whole argv element and a separate `";"` argv element would terminate the `set-hook` command itself.
  Do not reach for `chainedAttachArgv`'s literal `";"` element as a precedent: that one is parsed by tmux as a top-level command sequence, which a hook value is not.
  Multiple pins are multiple hook-array entries (`set-hook -u`, then `set-hook`, then `set-hook -a` per extra pin) — see `hook-body-is-one-array-entry-per-pin` for the failure-isolation reason and the measurements.
- `show-hooks -w -t "=<session>:"` is the readback used to check the array in a test or by hand; it prints `window-resized[0] …`, `window-resized[1] …`, one line per entry.
- reed's tmux floor is `3.3.0` (native) / `3.3.3` (psmux) per `version.go`; `window-resized` predates both, so no version gate is needed.
- The boot size that produces the reported threshold is `width: 220` / `height: 50` in `template_posix.yaml` and `template_windows.yaml`.
  Do not change it; it is the documented never-attached fallback box.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant** — `internal/reedengine` is a bound package: it is handed its absolute paths and derives none, and must not import `internal/lyxcwd`.
  The new code stays inside the engine and its `render` leaf; it introduces no new geometry source.
- **CLI / Cobra Invariant** — untouched: no new verb, no change to `internal/reedcli`.
  `reed attach` remains one of the two registered interactive-handoff exceptions.
- **Test Tier Purity Invariant** — no `exec.Command` and no `time.Sleep` of a second or more in an untagged test file.
  Everything that drives a real tmux server or a real pty belongs in an `integration`-tagged file.
- **Documentation Lifecycle** (CLAUDE.md) — `internal/reedengine/doc.go` is this module's doc and must move in the same commit.
  `docs/overview.md` needs no change: no module-table or execution-stack change.
  `manifest/roadmap.md` gets exactly one edit — narrowing the **reed: watchdog daemon** item's resize clause — which is not the bugfix-moves-the-roadmap case that rule forbids but the planned-item-changes-scope case it leaves open; see `roadmap-watchdog-item-is-narrowed`.
  No other roadmap line moves.
- **Markdown** (CLAUDE.md) — semantic line breaks, one sentence per line, in every `.md` touched.

Discovered during discussion:

- The Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` governs the new `set-hook` call, and `AttachArgv`'s "never refuses" contract governs its use on the attach path.
- The Shared Decision `told-box-wins-live-query-is-the-fallback` still holds: `AttachArgv` must not call `liveBoxLocked`, so its pins are computed against the same told box its layout is.

## Testing

**`internal/reedengine/render` — untagged, TDD candidate.**
The new pure pin function is the clearest TDD target in this task: it is total, has no I/O, and its expected values are already pinned by `height_test.go` and `rules_test.go` fixtures.
Scenarios that must be covered: header plus two full strands (one pin, the header's); a shrink-when-waiting ancestor present (two pins, header plus strip); no header configured (`Header.PaneID == ""`, no header pin); a box short enough that `clampHeaderHeight` clamps a large `HeightRows` (the header pin must be the clamped value, not the configured one); a box short enough that `clampToFit`'s priority-1 pass reclaims strip rows (the strip pin must be the reclaimed value, not `CollapsedStripRows`); no strand placed (the header-claims-the-whole-box branch, which must not emit a stale 1-row pin); and pin ordering, with the header always first.
The pins must agree with the heights the same inputs produce in the `Rules` layout string — a table test asserting both together is the strongest shape here.

**`internal/reedengine` — untagged.**
The hook-argv builder is pure and should be tested directly: zero pins (the `set-hook -u` clear alone, and nothing after it — never an empty hook body, and never nothing at all); one pin (a `set-hook -u` clear followed by one `set-hook`, no `-a`); several pins (the clear, then `set-hook`, then one `set-hook -a` per extra pin, header first); the correct `-w` window target; and each body being one argv element of the form `resize-pane -t %N -y <n>` with no separate `";"` element anywhere.
Existing `apply_test.go`/`attach_test.go` fakes already record `e.tmux` calls; extend them to assert the `set-hook` sequence is issued after a successful `select-layout` and before `select-pane`, that neither of `applyLayoutLocked`'s guards reaches it, that an apply yielding zero pins still issues the `set-hook -u` clear (the case a live header pane absent from the present set produces, since `planLayout` blanks `HeaderPaneID`), that a `set-hook` error does not fail `applyLayoutLocked`, and that a `set-hook` error neither suppresses the attach chain nor changes a single element of `AttachArgv`'s argv.

**`internal/reedengine` — `//go:build integration && linux`, in `attachgeometry_integration_test.go`.**
This is the test that would have caught the bug, and the file's `startInPTY` harness already owns the `TIOCSWINSZ` ioctl needed to drive it.
The scenario: bring up the existing fixture (header, a shrink-when-waiting parent, its child), attach through the chained argv at one size, confirm today's assertions still hold, then resize the pty to a materially taller size, wait for the window to report the new height from outside the pty, and assert the header pane is still `cfg.Header.HeightRows` and the collapsed parent still `cfg.CollapsedStripRows`.
A second case must cover the path the reported threshold came from: `AttachArgv(0, 0)`'s bare argv from a client taller than the 50-row boot box, asserting the header is at its budget once the attach settles — this is the one that fails hardest before the fix.
That case does not exercise an install: `AttachArgv(0, 0)` returns before the lock, so it passes on the hook the fixture's own earlier `applyLayoutLocked` already installed.
Say so in the test's comment, or the next reader will misread it as proof the no-size path installs one.
A third case should pin the fire-time isolation the array encoding buys: kill a strand pane so its strip pin names a destroyed id, resize the pty, and assert the header is still at its budget.
Correct `TestAttachGeometry_ExactLayoutAndRowBudgets`' file-level comment while there: its claim that the chained `select-layout` runs post-attach is not what makes the current assertions pass, and its `100x30` client is shorter than the boot box, so it never exercised growth.

**`internal/reedengine` — `//go:build integration`, in `contract_integration_test.go`.**
One new `TestMultiplexerContract` case for the two verbs this task adds to reed's wire surface, on the file's own scratch socket like every other case there.
It must pin the two behaviours the fix's correctness rests on and that no unit test can reach: that `set-hook -u` followed by `set-hook` and one `set-hook -a` yields independent array entries, readable back through `show-hooks -w` as `window-resized[0]` and `[1]`; and that a `window-resized` hook fires after tmux has resized the layout, so a `resize-pane -y` inside it survives the resize that triggered it.
The file self-skips when the configured binary is absent, which is what keeps this from becoming a psmux gate.

**Manual verification (record the numbers in the commit or the task result).**
`lyx reed up`, two `lyx reed add --cmd 'sleep 999'`, attach from a terminal taller than 50 rows, then resize that terminal at least twice.
The header must read 1 row at every step, and its visible content must be the ` hub: <path>` line — not scrollback.

## Q&A log

- **Q:** Is the defect in reed's layout computation, in the attach chain, or elsewhere? **A:** [auto-pick] Elsewhere — verified live that a real chained attach at 127x76 holds the header at 1 row, while any later client resize inflates it (76→90 gives 6, 90→120 gives 16). **Why:** tmux distributes resize deltas round-robin across vertical cells and has no fixed-height pane concept; reed never re-pins after a resize.
- **Q:** What explains the reported ~50-row threshold? **A:** [auto-pick] `reed.yaml`'s `height: 50` boot box seen through the *bare* (unchained) attach path. **Why:** a synthetic bare attach reproduces the reported table exactly — 40 and 50 rows leave the header at 1, 76 rows takes it to 10 — because the window only resizes to the client after the attach.
- **Q:** Which mechanism fixes it? **A:** [auto-pick] A tmux `window-resized` window hook re-pinning fixed-height panes with `resize-pane -y`. **Why:** verified to hold the header at 1 row across attach and four subsequent resizes down to an 8-row window, with no process spawn, no op lock, and no `PATH` dependency; `client-resized` was measured firing too early to work.
- **Q:** Which panes does the hook pin? **A:** [auto-pick] The header and every collapsed strip, at the heights `render` actually computed. **Why:** those are reed's only absolute row budgets; full panes have no fixed budget, so tmux's even redistribution among them is acceptable.
- **Q:** Does `render.Rules` change shape to expose those heights? **A:** [auto-pick] No — add a second pure entry point in `render` sharing `Rules`' policy composition. **Why:** keeps both existing call sites and their tests untouched while keeping policy single-owned.
- **Q:** How is a multi-pin hook body encoded? **A:** [auto-pick] One hook-array entry per pin — `set-hook -u`, then `set-hook`, then `set-hook -a` per extra pin — never one `";"`-separated string. **Why:** verified live that a `resize-pane` naming a destroyed pane aborts the rest of a single command list (header ballooned to 25 rows with a dead id placed first), while array entries are independent (same arrangement, header still pinned at 1); a separate `";"` argv element would also terminate `set-hook` itself, since it takes its body as one argument.
- **Q:** Do `set-hook` and `resize-pane` join `requiredSubcommands`? **A:** [auto-pick] No — they become reed's first deliberately *optional* wire surface, documented as such. **Why:** both verbs are new to `internal/`, so the contract genuinely widens and `doc.go`'s "no new psmux risk" sentence must be rewritten; but gating the capability probe on them would take every reed verb down on a psmux lacking `set-hook`, over an option already designed to degrade silently.
- **Q:** How does the engine reach the pins? **A:** [auto-pick] From the same mapping `planLayout` already performs, once. **Why:** `planLayout` owns `toRenderStrands`, the present-pane filter, and the `HeaderPaneID` blanking the zero-pin disposition depends on; a second mapping site could silently diverge and compute the pins from a different header id than the layout used.
- **Q:** What does an apply that computes zero pins issue? **A:** [auto-pick] The `set-hook -u` clear on its own — never nothing. **Why:** reaching the install statement means `render` has an opinion, and here the opinion is "nothing is pinned"; skipping the clear would leave an earlier strip pin clamping a pane now placed as a full pane on every resize, and that state is reachable whenever `planLayout` blanks a header pane id that is no longer live.
- **Q:** Which reed verb first installs the hook? **A:** [auto-pick] The first `add`, or a `resume` that places a strand — not `up`. **Why:** `lifecycle.go` clears every pane binding and blanks `HeaderPaneID` on boot, so the apply that follows a fresh `up` hits `!anyPlacedStrand` and installs nothing; there is also nothing to pin there, since a lone header pane takes `render.Rules`' sole-cell branch.
- **Q:** What happens when an installed hook fires against a pane that no longer exists? **A:** [auto-pick] Accepted and self-healing, with the header pinned as entry `[0]` so it fires first. **Why:** reed has no return value to inspect from inside the tmux server; the array encoding contains the failure to the one dead entry, pane ids are never reused within a server incarnation, and every way a pane disappears routes back through `applyLayoutLocked`, which rebuilds the array.
- **Q:** Where exactly is the hook installed and refreshed? **A:** [auto-pick] Two named statements — in `applyLayoutLocked` between `select-layout` and `select-pane`, and in `AttachArgv` between a successful `planLayout` and the `chained` assignment. **Why:** both already hold the op lock, the strand table, and the box the pins must match, so no extra I/O and no degrade ladder is reordered; every earlier guard therefore installs nothing, which is stated as a consequence rather than coded around.
- **Q:** What happens when `set-hook` fails, e.g. on psmux? **A:** [auto-pick] Log via `logger.Warn` and continue. **Why:** the existing Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere`, and `AttachArgv`'s contract that no engine-side failure may block the handover.
- **Q:** Keep or delete the attach chain? **A:** [auto-pick] Keep it, and correct only the comments that miscredit it. **Why:** it was verified working and it is the only path that reasserts reed's full layout policy for a new client; deleting a working, integration-tested mechanism is out of a bugfix's scope.
- **Q:** Does the fix restore reed's exact strand split across a resize? **A:** [auto-pick] No, and that is accepted. **Why:** after a resize tmux leaves full panes slightly uneven (52/45 where policy says 48/49); no absolute budget is violated, and the only fix with full fidelity costs a process spawn per resize.
- **Q:** What about the missing tmux status bar and the header's startup log spam? **A:** [auto-pick] Both out of scope, recorded with their reasons. **Why:** `status off` is pinned deliberately and the told-box arithmetic depends on it; the log spam is `reed-header-pane-boot-noise`, and this task only stops the pane from being tall enough to reveal it.
