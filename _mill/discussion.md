# Discussion: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
task: Reed attach dot-fill render artifact on resize and cross-client mouse move
slug: reed-attach-dotfill-artifact
status: discussing
parent: main
```

## Problem

An operator attached to a reed session with `lyx reed attach` from a real terminal emulator sees regions of the screen fill with placeholder dots (`......`) in two situations.
The first is a live resize of the attach window: mid-relayout, one or more panes show dot-filled regions, and the real content redraws about a second later.
The layout does re-apply correctly in both the grow and shrink directions and the focused pane stays focused, so nothing functional is broken — the dots are a visible rendering artifact only.

The second situation involves no resize at all.
With two `lyx reed attach` clients open on the same session — one inside a VS Code integrated terminal, one in a separate Konsole window — moving the mouse pointer anywhere into the VS Code window, with no click, reproducibly produces the same dot-fill in the *Konsole* client.

**Why now:** this is finding W5 (verdict WARN) from the `reed-watch-suite` live-observation run.
It was reported explicitly as not root-caused, with a guess on record that terminal mouse-tracking escape sequences leak through the shared tmux session rather than reed rendering anything wrong.
The guess is close but not the mechanism, and nothing in reed today knows that more than one client can be attached at once — so the behaviour is undiagnosed, untested, and undocumented.

## Scope

**In:**

- A root-cause determination, made by live measurement against real tmux, recorded in `internal/reedengine/doc.go`'s decision log — not left as a hypothesis.
- A deterministic regression test that reproduces the artifact headlessly and asserts on the *rendered client output*, not on pane grids.
- A measured repaint mitigation for the subset of the artifact that is a stale paint: one additional entry in the session's `window-resized` window-hook array that forces every attached client to redraw.
- An attach-time operator warning when another client is already attached to the same session at a different size, so the residual artifact is explained rather than mysterious.
- Documentation: the mechanism, what was fixed, and what is inherent, in `internal/reedengine/doc.go` and in `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`'s W5 scenario.

**Out:**

- Changing the `window-size latest` pin, or introducing any other `window-size` policy.
  That pin is load-bearing for the whole told-geometry design (`AttachArgv` gates its chained `select-layout` on reading `latest` back) and every alternative policy makes the artifact worse, not better — see the `window-size-latest-stays` decision.
- Changing the `mouse` pin, or anything about mouse-tracking escape sequences.
  The reporter's mouse hypothesis is addressed by the root-cause decision below and then dropped; mouse tracking is the *messenger*, not the cause.
- The Windows/psmux path for the new hook entry.
  The existing `window-resized` array is already never installed on Windows (`resizeSignalHookCommand` returns `""` there); the new entry inherits that rule unchanged and no psmux work is attempted.
- Any change to the watchdog's timings (`watchdogDebounceQuiet`, `watchdogSignalTick`, `watchdogPollCycle`).
  Shortening the debounce was considered and rejected — see `repaint-mechanism`.
- Any change to `render/` layout computation. The layout is correct; only its painting is at issue.
- `manifest/roadmap.md`. This is a bugfix/hardening pass, not a planned-item completion (per CLAUDE.md's task-completion rule).

## Decisions

### root-cause-model

- Decision: design against this model — the dots are drawn by **tmux itself**, in the region of an attached client's terminal that is not covered by the current window's geometry, or in a region whose paint has gone stale relative to a just-changed window size.
  They are never content reed writes, and never anything a pane's grid contains.
  Both reported triggers are one mechanism: a transient or standing mismatch between a *client's* terminal size and the *window's* size.
  - Resize trigger: the client's size changes, `window-size latest` moves the window to follow it, and the region between the old and new geometry is left showing a stale or padded paint until something forces a redraw.
    Today the thing that forces the redraw is reed's own watchdog re-apply — hence the roughly one-second heal, which is the sum of the hook's `run-shell` touch, `watchdogSignalTick` (100 ms), `watchdogDebounceQuiet` (200 ms), and the re-apply round trip.
  - Cross-client trigger: moving the pointer into the VS Code window makes VS Code's terminal deliver input to its tmux client (a focus-in report and/or mouse-tracking bytes — either suffices).
    Any input makes that client the *most recently used* one, which is exactly what `window-size latest` keys on, so tmux resizes the window to the VS Code client's size.
    The Konsole client, which did not change and was never touched, is now mismatched against the new window size and shows the same artifact.
    No resize happened from the operator's point of view, which is why the trigger looked inexplicable.
- Rationale: the two triggers are otherwise unrelated, and one mechanism explains both, including the specific detail that the pointer only had to enter the VS Code *window* rather than its terminal pane.
  It also explains the negative evidence: `grep` finds no dot-fill string, no placeholder character, and no `.`-writing code path anywhere under `internal/reedengine` or `internal/reedengine/render` — reed cannot be the author of these characters.
  The model further predicts a split the mitigation depends on, and the plan must confirm the split experimentally:
  - **Stale-paint subset** — the client is fully covered by the new window geometry, and the dots are leftover paint.
    A forced redraw of that client removes them.
  - **Uncovered subset** — the client's terminal is genuinely *larger* than the window, so tmux has real estate with nothing behind it and legitimately pads it.
    A forced redraw repaints the same padding; nothing short of a different `window-size` policy removes it, and every such policy is worse.
- Rejected: the reporter's "mouse-tracking escape sequences leak through the shared tmux session and corrupt reed's rendering" — mouse bytes are consumed by tmux as client input and never reach another client's screen, and reed does no rendering of its own that could be corrupted.
  The mouse is only how the VS Code client announced itself as most-recently-used.
  Also rejected: "reed's chained `select-layout` hands tmux a wrong-sized layout string" — a mismatched layout is refused by tmux outright (exit 1) or rescaled proportionally, and neither outcome leaves uncovered window area (already established in `doc.go`'s layout-string bullets).

### window-size-latest-stays

- Decision: the `window-size latest` pin (`pinGeometryOptionsLocked`, `internal/reedengine/windowsize.go`) is not changed, not made configurable, and not conditioned on the number of attached clients.
- Rationale: with two clients of different sizes attached, tmux must pick one window size, so *some* client is always mismatched — the artifact is inherent to multi-client attach under every available policy, and the only question is which client suffers.
  `latest` gives the artifact to the client the operator is not currently using, which is the best of the three.
  It is also structurally load-bearing: `AttachArgv` reads `#{window-size}` back and suppresses the chained `select-layout` entirely on anything other than `latest`, because the told box's whole premise is that the post-attach window becomes the attaching client's size.
- Rejected: `largest` — the window becomes the biggest client's size, so reed's told box for any smaller attaching client is wrong and the chain must be suppressed, breaking attach-time layout for every operator with a second client open.
  `smallest` — every larger client pads permanently instead of transiently, turning an intermittent artifact into a standing one.
  `manual` — abandons client-following entirely and is already treated as a chain-suppressing value.

### repaint-mechanism

- Decision: add exactly one new entry to the session's `window-resized` window-hook array, installed by `installResizePinsLocked` alongside the existing resize-pane pins and the watchdog's signal entry, whose body forces **every attached client** of this session to redraw.
  It is ordered **after** the resize-pane pins and **before** the watchdog's signal entry, so it paints the geometry the pins have already fixed up, and so the signal entry keeps its documented position as the array's last entry.
  The plan must select the body by live measurement from this candidate list, in order, taking the first that demonstrably clears the artifact in the regression test:
  1. A `run-shell -b` invocation that enumerates the session's clients and refreshes each one — the only candidate that can reach a client *other* than the one whose resize fired the hook, and therefore the only candidate that can cover the cross-client trigger.
     It reuses the existing `tmuxQuoteValue` escaping and `shell.ForGOOS()` dialect selection that `resizeHookCommand` already establishes in `internal/reedengine/watchdog.go`.
  2. A bare `refresh-client` entry with no target, relying on the hook's own client.
     Cheaper and quoting-free, but structurally cannot cover the cross-client trigger; acceptable only if measurement shows candidate 1 does not work and candidate 2 fixes the resize trigger.
- Rationale: the artifact's whole duration today is the latency of reed's watchdog round trip, because the re-apply's `select-layout` is what incidentally forces the redraw.
  Moving the redraw into the hook array makes it fire server-side, synchronously with the resize, before the watchdog has even been told a resize happened — collapsing a roughly one-second smear into a flicker.
  Putting it in the existing array rather than in a second hook install is mandatory, not stylistic: `installResizePinsLocked` is documented as the array's *only* install site precisely because the array is a whole-snapshot rebuild, and any second writer would clobber the pins or accumulate duplicate entries per attach.
- Rejected: shortening `watchdogDebounceQuiet` / `watchdogSignalTick` — treats the symptom's duration rather than its cause, costs re-apply churn on every resize burst, and cannot help the cross-client trigger at all (the artifact there is in a client whose own window never resized from its perspective).
  Rejected: issuing `refresh-client` from Go after a successful `reapplyLayout` — that is the exact moment the artifact already heals today, so it would add a tmux round trip and change nothing.
  Rejected: a separate `client-resized` or `client-focus-in` hook — `client-resized` is already documented as reporting the stale pre-resize size, and a second hook install site violates the single-install-site rule above.

### uncovered-subset-is-documented-not-fixed

- Decision: when an attached client's terminal is genuinely larger than the window, the dots are correct tmux behaviour and reed does not attempt to remove them.
  This residual is documented in `doc.go` and in the W5 scenario text of `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`, so the next observation run recognises it instead of re-filing it.
- Rationale: removing it requires changing `window-size`, which the `window-size-latest-stays` decision rejects for stronger reasons.
  A documented, explained, understood artifact with a known trigger is an acceptable outcome for a WARN-severity cosmetic finding; a broken attach-time layout is not.
- Rejected: refusing the second attach, or detaching the other client on attach — reed has no business evicting an operator's other terminal, and `attach` is explicitly the escape hatch that must never refuse.

### attach-time-multi-client-warning

- Decision: `AttachArgv`'s pre-flight additionally lists the session's currently attached clients and their sizes, and emits a `logger.Warn` when any existing client's size differs from the size this attach was told, naming the other client and both sizes.
  It never blocks, never changes the argv, and never reaches the JSON envelope.
- Rationale: this is the cheapest possible cure for the *actual* operator cost of the residual, which is bewilderment rather than pixels — the reporter spent a scenario on it and still could not explain it.
  The warning turns a mysterious artifact into a logged, searchable fact at the exact moment the operator creates the condition.
  It also fits `AttachArgv`'s existing contract exactly: the builder already performs several tmux round trips under the op lock and already answers every failure with `logger.Warn` plus a degrade, per the Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere`.
- Rejected: reporting on the JSON envelope — `attach`'s envelope is reserved for pre-flight aborts, and this is not an abort.
  Rejected: printing to the operator's terminal — stdio is about to be handed to tmux, and anything printed is immediately overwritten by the attach.
  Rejected: doing nothing — leaves the residual as undiagnosable in the field as it was in the observation run.

### test-vehicle-is-harness-in-harness

- Decision: the regression test is a build-tagged `smoke` test in `internal/reedcli`, following the existing pattern of `smoke_attach_test.go`: boot a **second, separate tmux server** (the harness) on its own socket, run `lyx reed attach` *inside* a harness pane, and assert on `capture-pane` of that harness pane.
- Rationale: this is the only way to observe the artifact at all.
  The dots are a client-side render artifact — they exist in what tmux paints to a client's terminal and are in no pane's grid, so `capture-pane` against the reed session itself is structurally blind to them.
  Capturing the *harness* pane that hosts the attach client captures exactly the bytes that client rendered, dots included.
  `smoke_attach_test.go` already establishes every primitive this needs (`tmuxBinaryPath`, `harnessShellBinaryPath`, `buildLyxBinary`, `hubforge.NewHub`, `sendKeysLine`, `pollPaneContains`, `reapHarnessServer`), so the test is new scenarios over existing scaffolding rather than new infrastructure.
- Rejected: adding a pty dependency such as `creack/pty` to drive the attach directly — a new third-party module for something the harness-server pattern already does, on a repo whose `go.mod` is deliberately small.
  Rejected: asserting only in the live `reed-watch-suite` — that suite is explicitly operator-assisted and non-automated, so it can never gate a regression.
  Rejected: a unit test over a fake tmux — the artifact is produced by real tmux's renderer and cannot be faked into existence.

## Technical context

Everything below was established by reading the code during exploration;
mill-plan should not need to re-derive it.

**Where the geometry pins and the hook array live.**
`internal/reedengine/windowsize.go` owns both geometry pins (`status off`, `window-size latest`) in `pinGeometryOptionsLocked`, the two effective-value readbacks (`readStatusRowsLocked`, `readWindowSizeLatestLocked`), and the entire write side of the `window-resized` array:
`resizePinHookArgvs` (pure, builds the argv sequence) and `installResizePinsLocked` (issues them).
The read side is `hookInstalledLocked` in `internal/reedengine/reapply.go`, which matches per array entry against the multi-line `show-options -v` answer — never against the answer as a whole.
A new array entry therefore has three touch points: `resizePinHookArgvs` builds it, `installResizePinsLocked` installs it as part of the same snapshot rebuild, and `hookInstalledLocked`'s per-entry matching must keep working unchanged (it matches the *signal* command specifically, so an additional unrelated entry must not break it — this is worth an explicit test).

**The array's construction rules, which the new entry must respect.**
`resizePinHookArgvs` emits an unconditional `set-hook -u` clear first, then establishes index 0 with a plain (replacing) `set-hook`, then uses `-a` for every entry after it.
That plain-first/`-a`-after pattern is what keeps the rebuild idempotent across the repeated installs that every attach pre-flight performs.
Entry order today is: resize-pane pins (header first), then the watchdog signal entry last.
The new repaint entry goes between them.
Array entries fire independently, so an entry naming a destroyed pane cannot swallow the ones behind it.

**Hook body quoting.**
`tmuxQuoteValue` in `internal/reedengine/watchdog.go` wraps a body in tmux double quotes and backslash-escapes `\`, `"`, and `$`.
The `$` escaping matters for candidate 1 of `repaint-mechanism`, whose shell fragment will contain shell variables.
`run-shell` must carry `-b`, or the tmux server blocks while the command runs — live-verified and already documented.
The resulting string must round-trip byte-identically through `show-options -v`, since that is what makes the availability probe viable.

**Where the attach pre-flight lives.**
`internal/reedengine/attach.go`'s `AttachArgv(cols, rows)` performs the whole pre-flight under one `withOpLock`, and returns no error by contract — every failure logs and degrades to `bareAttachArgv`.
The multi-client warning from `attach-time-multi-client-warning` belongs inside that same locked closure, after `requireSessionLocked` and before or beside the existing readbacks, and must not introduce a new degrade path: a failed client listing warns and continues, exactly like the geometry pins do.
`cols`/`rows` reach it from `internal/reedcli/attach.go`, which reads them with `golang.org/x/term`'s `GetSize` against stdout and passes `0, 0` when there is no TTY.

**The tmux seam.**
The engine talks to tmux through `e.tmux.run(...)` (no output) and `e.tmux.output(...)` (captured), always with exact targets built by `exactSessionTarget` / `exactSessionWindowTarget`.
Listing clients will need a new `e.tmux.output("list-clients", "-t", ..., "-F", ...)` call; the `#{client_name}`, `#{client_width}`, `#{client_height}` formats are the relevant ones.
Every tmux failure on this path is non-fatal per the Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere`, which already governs both files.

**Windows.**
`resizeSignalHookCommand` returns `""` on `runtime.GOOS == "windows"`, and `pinGeometryOptionsLocked`'s hook block returns early there, because `set-hook`/`run-shell` are absent from `requiredSubcommands` and psmux's support is unverified.
The new repaint entry follows the same rule with no new reasoning: it is never installed on Windows.
The attach-time warning has no such restriction — `list-clients` is a plain query — but if it turns out not to be in `requiredSubcommands`, its absence must degrade to no warning rather than to an error.

**Nothing in reed currently knows about multiple clients.**
`grep` for `list-clients`, `client_width`, or any client concept across `internal/reedengine` and `internal/reedcli` returns nothing.
This task introduces reed's first client-awareness, which is why the model in `root-cause-model` was not obvious from the code.

**Where the decision log goes.**
reed has no `manifest/designs/reed.md`;
its design record is the long comment block in `internal/reedengine/doc.go`, whose geometry section (roughly lines 370–545) already documents the resize round-robin, the resize-pin hook, the chained attach, the two geometry pins, why `window-resized` is the only usable event source, and the `show-options -v` array behaviour.
The new decisions belong there, in that section, in the same voice and with the same "verified live on tmux 3.6" evidence discipline.
`docs/overview.md` is not touched: no module is added and the execution stack does not change.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant** — `internal/reedengine` is a bound package and is handed the absolute paths it operates on;
  it must not import `internal/lyxcwd` or derive its own geometry.
  The signal file's path stays behind `e.stateDir()`.
- **Durable-vs-Ephemeral State Invariant** — the resize signal file lives under `.lyx` via `stateDir()`, one per worktree so sibling worktrees on the shared per-hub tmux server cannot collide.
  No new never-tracked file is introduced by this task;
  if one were, it would have to follow the same rule.
- **CLI/Cobra Invariant** — no new subcommand is added, so the `Command()`/`RunCLI` seam, the `Short`-on-every-command rule, and the help-tree tests are untouched.
  `attach`'s registered exception (no JSON envelope after the terminal handover) stays exactly as it is.
- **Documentation Lifecycle** — the doc updates named under Technical context land in the same commit as the code, per CLAUDE.md.

Discovered during discussion:

- `installResizePinsLocked` must remain the sole install site for the `window-resized` array. Any second writer clobbers or duplicates.
- `AttachArgv` must never return an error and must never block the handover, no matter what the new client listing does.
- The `window-resized` array is never installed on Windows; the new entry inherits that unchanged.
- Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` governs every tmux call added by this task.
- Shared Decision `the-clear-is-unconditional-including-zero-pins` still holds: a zero-pin rebuild with the watchdog on still installs the signal entry, and now also the repaint entry.

## Testing

**`internal/reedengine` — pure unit tests (TDD candidates).**
These are the primary TDD candidates because the functions are pure and the existing test files establish the shape.

- `resizePinHookArgvs` (extend `windowsize_test.go`): the new repaint entry appears exactly once, in the documented position — after every resize-pane pin, before the signal entry.
  Cover zero pins, one pin, several pins, watchdog on and off, and the case where the repaint entry is the array's only entry.
  Assert the clear stays first and unconditional, that index 0 uses the plain replacing form and everything after it uses `-a`, and that the entry is absent on the Windows path.
- The repaint body builder, wherever it lands beside `resizeHookCommand` in `watchdog.go` (extend `watchdog_test.go`): correct `-b`, correct `tmuxQuoteValue` escaping of `\`, `"`, and `$`, and byte-identical round-trip shape.
- `hookInstalledLocked` (extend `reapply_test.go`): an array that now contains the repaint entry alongside the pins and the signal entry is still probed correctly.
  This is the regression that matters most — the probe matches per entry, and a new entry must not make a healthy session read as "no hook".
- The new client-listing helper: parsing a `list-clients -F` answer into name/size triples.
  Cover the empty answer (no clients attached), one client, several clients, a malformed line, and trailing whitespace — mirroring `parseWindowSize`'s existing table-test shape and its strict "any other shape reports not-ok" discipline.
- `AttachArgv` (extend `attach_test.go`): with a fake tmux answering `list-clients`, a same-size existing client produces no warning, a different-size one produces exactly one warning, a `list-clients` error produces a warning and no behaviour change, and in every one of those cases the returned argv is byte-identical to what it is today.
  The argv is the contract; the warning is a side effect that must never perturb it.

**`internal/reedcli` — smoke tests (build tag `smoke`, real tmux required).**
Two new scenarios in a new file beside `smoke_attach_test.go`, both built on the harness-server pattern that file already establishes.

- *Resize trigger*: boot a reed session with at least two live panes, attach inside a harness pane of a known size, resize that harness pane, and capture it.
  The scenario must first be shown to reproduce the artifact — a run of dots in the captured harness pane within the heal window — and then, with the repaint entry installed, to be free of it.
  Reproducing before fixing is the point: a test that only ever passes proves nothing about a rendering artifact.
- *Cross-client trigger*: attach two clients of deliberately different sizes to the same reed session from two harness panes, deliver input to one of them so it becomes the most-recently-used client, and capture the other.
  Assert the same before/after property.
  If measurement shows this trigger falls in the uncovered subset of `root-cause-model` and cannot be cleared, the test asserts the *documented* behaviour instead and its comment cites the `uncovered-subset-is-documented-not-fixed` decision by name.

Scenarios the smoke tests must not assume: that the artifact appears on every run at every size.
It is timing-dependent, so each assertion polls within a bounded deadline in the style of the existing `pollPaneContains` helper rather than sampling once.

**Measurement gate.**
Before the repaint entry is implemented, the plan runs the resize smoke scenario against candidate 1 and, if needed, candidate 2 from `repaint-mechanism`, and records which one cleared the artifact and on what tmux version — in `doc.go`, in the same "verified live on tmux 3.6" voice the surrounding decisions use.
If neither candidate clears either trigger, that negative result is itself recorded in `doc.go`, the repaint entry is not added at all, and the task still lands the regression tests, the attach-time warning, and the documentation.
That branch is a complete, acceptable outcome, not a failure — the artifact is WARN-severity and cosmetic, and an explained residual is worth more than a speculative hook entry.

**Live suite.**
`tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`'s W5 scenario text is updated with the confirmed mechanism, the cross-client repro steps, and what the observer should now expect to see — so the next observation run recognises the residual rather than re-filing it.

## Q&A log

- **Q:** Should this task root-cause and fix the artifact, or only diagnose it and defer the fix? **A:** [auto-pick] Root-cause, regression-test, mitigate, and document in one task. **Why:** the finding is already a deferred observation; splitting it again would leave a second undiagnosed WARN behind, and the diagnosis work is what makes the mitigation cheap.
- **Q:** Which root-cause model should the design commit to? **A:** [auto-pick] tmux client-vs-window size mismatch — the dots are tmux's own padding or stale paint, never reed content. **Why:** it is the only model that explains both triggers at once, and it is consistent with the negative grep evidence that no reed code path writes a dot.
- **Q:** Is the reporter's mouse-tracking-leak hypothesis adopted? **A:** [auto-pick] No — mouse input is the trigger's messenger, not its cause. **Why:** mouse bytes are consumed as client input by tmux and cannot reach another client's screen; what they do is make the VS Code client most-recently-used, which is what `window-size latest` keys on.
- **Q:** Should the `window-size latest` pin change? **A:** [auto-pick] No, and it is explicitly out of scope. **Why:** it is load-bearing for the told-geometry attach chain, and every alternative policy makes the artifact standing rather than transient.
- **Q:** Fix the cross-client trigger, or document it? **A:** [auto-pick] Fix the stale-paint subset, document the genuinely-uncovered subset as inherent tmux behaviour. **Why:** the uncovered subset is tmux drawing correct padding for real estate with nothing behind it; only a worse `window-size` policy would remove it.
- **Q:** What is the repaint mechanism? **A:** [auto-pick] One new entry in the existing `window-resized` hook array that refreshes every attached client, ordered after the pins and before the signal entry. **Why:** it fires server-side at resize time, ahead of the watchdog's own ~300 ms debounce plus re-apply, which is the entire duration of the artifact today.
- **Q:** Shorten the watchdog debounce instead? **A:** [auto-pick] No. **Why:** it treats duration rather than cause, adds re-apply churn per resize burst, and cannot help the cross-client trigger at all.
- **Q:** Where does the repaint entry get installed? **A:** [auto-pick] Inside `installResizePinsLocked`'s existing whole-snapshot rebuild, never a second install site. **Why:** the array's plain-first/`-a`-after rebuild means any second writer clobbers the pins or accumulates duplicates on every attach.
- **Q:** How is a client-side render artifact tested deterministically? **A:** [auto-pick] Harness-in-harness — a second tmux server whose pane hosts the attach client, asserted via `capture-pane` on that outer pane. **Why:** the dots live in what tmux paints to a client, not in any pane grid, so capturing the reed session directly is structurally blind to them; `smoke_attach_test.go` already establishes every primitive.
- **Q:** Add a pty dependency to drive the attach instead? **A:** [auto-pick] No. **Why:** a new third-party module for something the existing harness-server pattern already does, on a deliberately small `go.mod`.
- **Q:** Should the smoke test be required to reproduce the artifact before the fix? **A:** [auto-pick] Yes. **Why:** for a timing-dependent rendering artifact, a test that has never failed proves nothing.
- **Q:** Warn the operator when a second differently-sized client is already attached? **A:** [auto-pick] Yes — one `logger.Warn` from `AttachArgv`'s pre-flight, naming the other client and both sizes. **Why:** the real operator cost of the residual is bewilderment, and the observation run proved it; the warning must never touch the envelope or the argv.
- **Q:** Does the new hook entry ship on Windows? **A:** [auto-pick] No. **Why:** the `window-resized` array is already never installed there because `set-hook`/`run-shell` are outside `requiredSubcommands` and psmux support is unverified; the entry inherits that rule with no new reasoning.
- **Q:** What if measurement shows no candidate mechanism clears either trigger? **A:** [auto-pick] Land the regression tests, the attach-time warning, and the documentation, and record the negative result in `doc.go`. **Why:** a WARN-severity cosmetic artifact that is understood, tested, and explained is an acceptable outcome; a speculative hook entry that fixes nothing is not.
- **Q:** Does `manifest/roadmap.md` move? **A:** [auto-pick] No. **Why:** CLAUDE.md reserves roadmap movement for completing or adding a planned item; this is a bugfix/hardening pass covered by git history and `doc.go`.
