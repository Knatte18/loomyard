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
- A measured repaint mitigation for the subset of the artifact that is a stale paint: one additional entry in the session's `window-resized` window-hook array that forces every attached client to redraw — **conditional on the measurement gate under Testing**, which may conclude that no candidate mechanism helps and that no entry ships.
  The measurement and its recorded outcome are the unconditional deliverable; the entry itself is not.
- An attach-time operator warning when another client is already attached to the same session at a different size, so the residual artifact is explained rather than mysterious.
- Documentation: the mechanism, what was fixed, and what is inherent, in `internal/reedengine/doc.go` and in `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`'s W5 scenario.

**Out:**

- Changing the `window-size latest` pin, or introducing any other `window-size` policy.
  That pin is load-bearing for the whole told-geometry design (`AttachArgv` gates its chained `select-layout` on reading `latest` back) and every alternative policy makes the artifact worse, not better — see the `window-size-latest-stays` decision.
- Changing the `mouse` pin, or anything about mouse-tracking escape sequences.
  The reporter's mouse hypothesis is addressed by the root-cause decision below and then dropped; mouse tracking is the *messenger*, not the cause.
- The Windows/psmux path for the new hook entry.
  Note the premise precisely, because it is easy to state wrongly: the `window-resized` array *is* installed on Windows — `installResizePinsLocked` carries no `runtime.GOOS` gate and issues the clear plus every `resize-pane` pin argv there like anywhere else.
  What is excluded on Windows today is the **signal entry alone**, and it is excluded by exactly one mechanism: `resizeSignalHookCommand` returns `""` on `runtime.GOOS == "windows"`, and `resizePinHookArgvs` emits no entry for an empty body.
  (`pinGeometryOptionsLocked`'s early Windows return covers only the *unset* half of the lifecycle, not the install half.)
  The repaint entry therefore inherits nothing automatically and must carry its own `""`-returning builder modelled on `resizeSignalHookCommand`; no psmux work is attempted.
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
     Its composition is specified under `repaint-body-composition` below, because it is not a one-liner and cannot be built from what `internal/shell` exposes today.
  2. A bare `refresh-client` entry with no target, relying on the hook's own client.
     Cheaper, needs no shell fragment and no new `internal/shell` primitive, but structurally cannot cover the cross-client trigger; acceptable only if measurement shows candidate 1 does not work and candidate 2 fixes the resize trigger.
- Rationale: the artifact's whole duration today is the latency of reed's watchdog round trip, because the re-apply's `select-layout` is what incidentally forces the redraw.
  Moving the redraw into the hook array makes it fire server-side, synchronously with the resize, before the watchdog has even been told a resize happened — collapsing a roughly one-second smear into a flicker.
  Putting it in the existing array rather than in a second hook install is mandatory, not stylistic: `installResizePinsLocked` is documented as the array's *only* install site precisely because the array is a whole-snapshot rebuild, and any second writer would clobber the pins or accumulate duplicate entries per attach.
- Rejected: shortening `watchdogDebounceQuiet` / `watchdogSignalTick` — treats the symptom's duration rather than its cause, costs re-apply churn on every resize burst, and cannot help the cross-client trigger at all (the artifact there is in a client whose own window never resized from its perspective).
  Rejected: issuing `refresh-client` from Go after a successful `reapplyLayout` — that is the exact moment the artifact already heals today, so it would add a tmux round trip and change nothing.
  Rejected: a separate `client-resized` or `client-focus-in` hook — `client-resized` is already documented as reporting the stale pre-resize size, and a second hook install site violates the single-install-site rule above.

### repaint-body-composition

- Decision: the repaint entry's body is built by a new `""`-returning builder split across the two files the existing pair already occupies, following their split exactly rather than inventing a third home:
  the *pure* body builder (the string, no I/O, no engine state) joins `resizeHookCommand` and `tmuxQuoteValue` in `internal/reedengine/watchdog.go`, tested in `watchdog_test.go`;
  the *engine-method* wrapper that decides `""` versus a real body — the direct analogue of `resizeSignalHookCommand`, which reads `runtime.GOOS` and engine state — joins it in `internal/reedengine/windowsize.go` beside `resizeSignalHookCommand` itself, tested in `windowsize_test.go`.
  The shell fragment inside it is built **only** through `internal/shell` — never by string-concatenating shell syntax inside `reedengine`.
  Concretely, for candidate 1 the body needs four things, and each has exactly one source:
  - The multiplexer binary path — `e.TmuxPath()`, the engine accessor `internal/reedcli/attach.go` already uses to spawn the attach child.
    The tmux server's `run-shell` inherits no reed context, so the path must be embedded in the fragment; it is quoted with `shell.Shell.Quote`.
  - Reed's socket — `e.Socket()`, embedded as the `-L` argument for the same reason, quoted the same way.
    Without it the fragment would talk to the default socket, which is not reed's.
  - The session target — **`exactSessionTarget(e.SessionName())`**, the bare `=<name>` form, not `exactSessionWindowTarget`.
    The two are different targets, and the choice is not free: `exactSessionTarget` yields `=<name>` and `exactSessionWindowTarget` yields `=<name>:`, where the trailing colon exists solely because tmux's window/pane target parsers reject the bare form.
    `list-clients -t` takes a **session** target, so the bare form is the correct one; the window form would be parsed by a different grammar and scope the query differently or reject it.
    The same rule binds the attach-time warning's own `list-clients` call in `AttachArgv` — one form, both sites.
    (`set-hook`, by contrast, keeps `exactSessionWindowTarget` throughout, because the `window-resized` array is window-scoped and has no session scope to fall back on — that is already established in `doc.go` and does not change.)
    The `=` prefix is what stops tmux prefix-matching a sibling worktree's session on the shared per-hub server, and is non-negotiable on every form.
  - A loop over `list-clients -F '#{client_name}'` issuing one `refresh-client -t <name>` per line.
    **This primitive does not exist today.** `shell.Shell` exposes `Quote`, `Invoke`, `ReadFile`, `WithEnv`, and `Touch` and nothing else, and the **Shell Mechanics Seam** constraint requires shell command strings to be built via `internal/shell` alone.
    So the loop is added to `internal/shell` as a new `Shell` method (a line-iterating construct along the lines of `ForEachLine(command, body string) string`), stdlib-only, with both a POSIX and a pwsh implementation and table tests in `internal/shell`, exactly as the interface's existing members are shaped.
    Adding it to the interface means both implementations must satisfy it even though only the POSIX one is ever executed in production here.
  The assembled fragment is then wrapped by the existing `tmuxQuoteValue` and prefixed with `run-shell -b`, unchanged.
  `tmuxQuoteValue`'s `$` escaping is load-bearing for this body specifically: the loop's own shell variable must reach the shell as a literal `$`, which is precisely what escaping it away from tmux's double-quote expansion achieves.
  The whole string must still round-trip byte-identically through `show-options -v`, which the plan verifies live, since `hookInstalledLocked`'s per-entry matching depends on it.
- Rationale: this is the difference between a decision and a wish.
  `resizeHookCommand` wraps `sh.Touch(path)` and nothing more, so it establishes none of the binary path, socket, target, or loop machinery candidate 1 needs, and a plan told only "reuse `tmuxQuoteValue` and `shell.ForGOOS()`" would have to invent the rest — most likely by concatenating shell syntax inside `reedengine`, which the Shell Mechanics Seam forbids.
- Rejected: hardcoding `tmux` as the binary name — `LYX_REED_TMUX` exists precisely so the binary is configurable, and the fragment would silently target the wrong multiplexer.
  Rejected: omitting `-L` and relying on the default socket — reed never uses it.
  Rejected: building the loop inline in `watchdog.go` to avoid touching `internal/shell` — a direct Shell Mechanics Seam violation, and it would leave the pwsh dialect unrepresented.
  Rejected: replacing the loop with tmux's own syntax — tmux has no iteration construct, and `refresh-client` takes one client.

### repaint-must-not-self-retrigger

- Decision: the repaint entry is only acceptable if a server-issued redraw provably cannot feed back into the event that fired it.
  The reasoning the design rests on, which the measurement gate must confirm rather than assume:
  - `window-resized` fires on a settled **size change**, not on a paint.
    A `refresh-client` repaints a client at its existing size and changes no geometry, so on its own it has no path back to the hook.
  - `window-size latest` keys on the *most-recently-used* client, and "used" means client **input** — keystrokes, mouse reports, focus reports.
    A `refresh-client` issued by the tmux server on the server's own behalf is not client input and must not move the MRU pointer.
    This is the load-bearing assumption of candidate 1, because candidate 1 refreshes *every* client, including ones that are not current: if a server-issued refresh did mark a client as used, refreshing all of them would hand MRU to whichever client was refreshed last, resize the window to it, fire `window-resized` again, and loop.
  - `run-shell -b` runs detached, so even a pathological body cannot block the server while this is being established.
- Measurement-gate acceptance criteria — a candidate that clears the artifact but trips either of these is **rejected**, not shipped:
  1. **No repeated hook fire.** During each smoke scenario, count `window-resized` fires across the trigger and the settling window (a counting entry appended to the array for the duration of the measurement is the cheapest instrument).
     One settled resize must yield the documented single fire, not a growing series.
  2. **No resize storm.** The window's size must be observably stable after the trigger settles — sample `#{window_width} #{window_height}` across the settling window and require it to stop changing, rather than oscillating between two clients' sizes.
  Both criteria are recorded in `doc.go` with the tmux version they were measured on, in the same voice as the surrounding "verified live on tmux 3.6" evidence.
- Rationale: this is the one failure mode where the mitigation would be worse than the bug.
  A resize storm inside the tmux server would be far more damaging than a one-second cosmetic smear, and it would be reached by exactly the mechanism the rest of this design leans on — `window-size latest` following whichever client most recently did something.
  "Did it clear the artifact" is therefore not a sufficient measurement criterion on its own.
- Rejected: relying on the reasoning above without measuring it — the whole task exists because a plausible-sounding hypothesis about this subsystem (the mouse-tracking one) turned out to be wrong.
  Rejected: guarding at runtime with a re-entrancy flag in the hook body — the array is executed by the tmux server with no place to hold state, and a flag written to a file would need its own cleanup and would reintroduce the signal-file machinery for a second purpose.
  Rejected: dropping candidate 1 pre-emptively on this risk — it is the only candidate that can reach the cross-client trigger at all, so it is measured first and rejected on evidence if it fails.

### repaint-is-independent-of-watchdog

- Decision: the repaint entry is **not** gated on `watchdogOption`.
  It is installed whenever `installResizePinsLocked` runs on a non-Windows host, whatever `watchdog` is set to.
  The `watchdog` key gates the watch loop and its signal entry, and nothing else.
- Rationale: `watchdog: off` is a kill-switch for the self-healing re-apply loop — a behaviour that mutates layout.
  A forced redraw mutates nothing and costs one tmux round trip per resize; conflating the two would mean an operator who turns off self-healing silently also turns off the repaint that this task exists to add, which is the opposite of what "off" means to them.
- Reconciliation with the unconditional unset — this is the subtlety the gating question turns on, and the call sites do **not** pair up the way a quick reading suggests.
  The two *unset* sites and the two *install* sites are different sites:
  - `pinGeometryOptionsLocked` (which performs the `watchdog: off` clear) is called from `internal/reedengine/lifecycle.go`'s boot path and from `AttachArgv`'s pre-flight in `attach.go`.
  - `installResizePinsLocked` (the rebuild) is called from `attach.go`'s pre-flight and from `applyLayoutLockedOpts` in `apply.go` — and nowhere else.
  Only the attach path holds both, in that order, in one locked closure.
  The boot path clears and then **returns without rebuilding**: `lifecycle.go` calls `pinGeometryOptionsLocked()` as its last act and never reaches an install.
  So the true residual for `watchdog: off` is: from boot until the first attach or the first non-`SkipFocus` apply, the session's `window-resized` array is **empty** — no pins, no repaint entry.
  There is a second, sharper edge on the same fact, which matters more than the boot one and which `doc.go` does not currently say out loud: `applyLayoutLockedOpts` returns immediately after `select-layout` when `opts.SkipFocus` is set, *before* the `installResizePinsLocked` call — and `SkipFocus: true` is exactly the mode the watchdog's own re-apply uses (`reapply.go`).
  The watchdog re-apply therefore never installs the array.
  The array is (re)established only by an attach or by a focusing apply — `up`, `add`, `remove`, `resume` — which is why a session can run for a long time on whatever array its last such operation left behind.
  Accepted as-is, and unchanged by this task: it is already today's behaviour for the resize-pane pins, and the repaint entry simply shares their lifecycle rather than inventing a new one.
  Widening the install to the `SkipFocus` path is explicitly out of scope — it would put a hook-array rebuild inside the watchdog's own re-apply loop, which is the one place re-entrancy has to be avoided.
- This whole call-site map goes into `doc.go`, stated plainly, rather than left for a reader to re-derive across `lifecycle.go`, `attach.go`, `apply.go`, and `reapply.go`.
- Rejected: gating the repaint entry on `watchdogOption` for symmetry with the signal entry — symmetry is not a reason; the two entries answer different questions ("does anyone want to hear about a resize" versus "should the screen be correct"), which is the same distinction `doc.go` already draws when it explains why the signal entry ships even with zero pins.
  Rejected: making the repaint entry survive the unset by moving it out of the array — that would create a second install site, which the single-install-site rule forbids.

### probe-verbs-not-extended

- Decision: `list-clients` and `refresh-client` are **not** added to `requiredSubcommands` in `internal/reedengine/probe.go`.
  That list holds fourteen verbs today (`new-session`, `has-session`, `split-window`, `select-layout`, `select-pane`, `send-keys`, `capture-pane`, `list-panes`, `list-sessions`, `display-message`, `set-option`, `kill-pane`, `kill-session`, `kill-server`) and neither of the two is among them — nor are `set-hook` or `run-shell`, which the existing hook machinery already depends on.
  Both new call sites degrade instead:
  - A failing or unsupported `list-clients` in `AttachArgv` logs one `logger.Warn` and emits no multi-client warning. The argv is unaffected.
  - A `refresh-client` the multiplexer does not implement makes the hook entry a no-op fired by the tmux server, which is the same outcome as the mitigation not helping — the artifact stays, nothing breaks.
- Rationale: `requiredSubcommands` is documented as the set the engine "cannot work without", enforced by failing loud once at server-ensure.
  Both of these are geometry-quality only, exactly like the `status`/`window-size` pins that the Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` already exempts from fatality.
  Adding them would make a multiplexer that runs reed perfectly well today fail at boot over a cosmetic feature.
- Rejected: adding them and letting the probe fail loud — turns a WARN-severity cosmetic fix into a hard compatibility break for psmux and any other tmux-alike.
  Rejected: probing for them separately at install time — a per-attach capability round trip for a feature whose absence is already silent and harmless.

### uncovered-subset-is-documented-not-fixed

- Decision: when an attached client's terminal is genuinely larger than the window, the dots are correct tmux behaviour and reed does not attempt to remove them.
  This residual is documented in `doc.go` and in the W5 scenario text of `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`, so the next observation run recognises it instead of re-filing it.
- Rationale: removing it requires changing `window-size`, which the `window-size-latest-stays` decision rejects for stronger reasons.
  A documented, explained, understood artifact with a known trigger is an acceptable outcome for a WARN-severity cosmetic finding; a broken attach-time layout is not.
- Rejected: refusing the second attach, or detaching the other client on attach — reed has no business evicting an operator's other terminal, and `attach` is explicitly the escape hatch that must never refuse.

### attach-time-multi-client-warning

- Decision: `AttachArgv`'s pre-flight additionally lists the session's currently attached clients and their sizes, and emits a `logger.Warn` when any existing client's size differs from the size this attach was told, naming the other client and both sizes.
  It never blocks, never changes the argv, and never reaches the JSON envelope.
- Cardinality: **one warning line per differing client**, not one aggregate line, and no line at all for a client whose size matches.
  Zero differing clients therefore produce zero lines — the common single-client case stays silent.
  Per-client is the right granularity because the line's whole job is to name the specific other terminal the operator should go look at, and an aggregate ("3 clients differ") would drop exactly that.
  The line count is bounded by the number of attached clients, which is bounded by how many terminals a human has open.
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
  The scaffolding already exists and is split across two files, which the plan must cite correctly: `internal/reedcli/smoke_test.go` holds the shared primitives (`tmuxBinaryPath`, `harnessShellBinaryPath`, `buildLyxBinary`, `sendKeysLine`, `pollPaneContains`, `reapHarnessServer`), while `internal/reedcli/smoke_attach_test.go` is where the harness-in-harness *pattern* — booting a second server, sending the attach line into its pane, polling that pane — is demonstrated end to end.
  `hubforge.NewHub` comes from `internal/hubforge`.
  So the test is new scenarios over existing scaffolding rather than new infrastructure, with one new helper (the dot-run predicate, see Testing).
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
Listing clients will need a new `e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", ...)` call — the bare `=<name>` session form, per `repaint-body-composition`, never the `=<name>:` window form.
The `#{client_name}`, `#{client_width}`, `#{client_height}` formats are the relevant ones.
Every tmux failure on this path is non-fatal per the Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere`, which already governs both files.

**Windows — read this carefully, the obvious summary of it is wrong.**
`installResizePinsLocked` has **no** `runtime.GOOS` gate: on Windows it issues the `set-hook -u` clear and every `resize-pane` pin argv exactly as it does elsewhere.
`pinGeometryOptionsLocked`'s early Windows return covers only the *unset* half of the hook lifecycle (plus the signal-file removal), not the install half.
The single mechanism that keeps the signal entry off Windows is `resizeSignalHookCommand` returning `""` there, combined with `resizePinHookArgvs` emitting no entry for an empty body.
Consequently the repaint entry has nothing to inherit and will be installed on Windows unless it is given its own `""`-returning builder on the `runtime.GOOS == "windows"` path — which it must be, mirroring `resizeSignalHookCommand` exactly.
`set-hook`/`run-shell` are absent from `requiredSubcommands` and psmux's support for them is unverified, which is the original reason for the Windows exclusion and applies unchanged.

The attach-time warning has no such restriction — `list-clients` is a plain query and is issued on every platform.
It is not in `requiredSubcommands` and is not being added (decision `probe-verbs-not-extended`), so its absence or failure degrades to no warning, never to an error.

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
- **Shell Mechanics Seam** — shell command strings are built ONLY via `internal/shell` (`Quote`/`Invoke`/`ReadFile`, stdlib-only).
  This binds candidate 1's `run-shell` body directly: the client-refresh loop it needs has no existing primitive, so the primitive is added to `internal/shell` (both POSIX and pwsh implementations) rather than concatenated inside `internal/reedengine`.
  See the `repaint-body-composition` decision.
- **CLI/Cobra Invariant** — no new subcommand is added, so the `Command()`/`RunCLI` seam, the `Short`-on-every-command rule, and the help-tree tests are untouched.
  `attach`'s registered exception (no JSON envelope after the terminal handover) stays exactly as it is.
- **Documentation Lifecycle** — the doc updates named under Technical context land in the same commit as the code, per CLAUDE.md.

Discovered during discussion:

- `installResizePinsLocked` must remain the sole install site for the `window-resized` array. Any second writer clobbers or duplicates.
- `AttachArgv` must never return an error and must never block the handover, no matter what the new client listing does.
- The `window-resized` array **is** installed on Windows (`installResizePinsLocked` has no `runtime.GOOS` gate); only the signal entry is excluded there, and only because its builder returns `""`.
  The repaint entry needs its own `""`-returning builder to be excluded — there is no inheritance to rely on.
- The repaint entry is independent of the `watchdog` key.
  The array's only install sites are `AttachArgv`'s pre-flight and non-`SkipFocus` applies — the boot path clears without rebuilding, and the watchdog's own re-apply (`SkipFocus: true`) returns before the install.
  So with `watchdog: off` the array is empty from boot until the first attach or first focusing apply.
  See `repaint-is-independent-of-watchdog`.
- The repaint mechanism must be proven not to feed back into `window-resized` via `window-size latest`. See `repaint-must-not-self-retrigger`.
- `list-clients` and `refresh-client` stay out of `requiredSubcommands`; both call sites degrade silently.
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
**`internal/shell` — pure unit tests (TDD candidate).**
If candidate 1 of `repaint-mechanism` is selected, the new line-iterating `Shell` primitive gets table tests in `internal/shell` for both the POSIX and pwsh implementations, in the shape the package's existing member tests already use: the emitted syntax for a simple command and body, correct quoting of a command containing spaces and quotes, and the interaction with `Quote`.
Both implementations must exist even though only POSIX executes here, because the member is on the interface.

**`internal/reedengine` — pure unit tests, continued.**

- The new client-listing helper: parsing a `list-clients -F` answer into name/size triples.
  Cover the empty answer (no clients attached), one client, several clients, a malformed line, and trailing whitespace — mirroring `parseWindowSize`'s existing table-test shape and its strict "any other shape reports not-ok" discipline.
- `AttachArgv` (extend `attach_test.go`): with a fake tmux answering `list-clients`, a same-size existing client produces no warning, a different-size one produces exactly one warning, a `list-clients` error produces a warning and no behaviour change, and in every one of those cases the returned argv is byte-identical to what it is today.
  Add the cardinality case: three attached clients of which two differ in size produce exactly two warning lines, one naming each differing client, and none for the matching one.
  The argv is the contract; the warning is a side effect that must never perturb it.

**`internal/reedcli` — smoke tests (build tag `smoke`, real tmux required).**
A new file beside `smoke_attach_test.go`, built on the harness-server pattern that file establishes and the shared primitives in `smoke_test.go`.

*The assertion predicate.*
`pollPaneContains` must not be used for this, and reusing it would be the single easiest way to ship a test that proves nothing: it takes a plain substring, and legitimate harness-pane content contains dots (file paths, ellipses, the header template).
The predicate is a new helper in the same file: capture the harness pane, and report a hit when **any single captured line contains a run of at least 20 consecutive `.` characters**.
Twenty is **fixed**, not a starting guess: it is far above anything reed's own rendered content produces on one line and far below the width of any pane region tmux would pad.
The plan validates it once against a clean capture, and that validation is a gate, not a licence to retune — if a clean capture trips a 20-dot run, the finding is that something in reed's rendered output produces long dot runs, which is itself news and must be reported rather than papered over by raising the floor until the test goes quiet.
The helper polls to a bounded deadline in the style of `pollPaneContains` rather than sampling once, because the artifact is timing-dependent in both directions: it can take a moment to appear, and it heals on its own.

*The negative control — this is what keeps the suite honest.*
"Reproduce it once by hand, then assert absence forever" is not a test;
once the repaint entry lands, an absence-only assertion passes vacuously on any machine or tmux build where the artifact never appears, and it would keep passing if the repaint entry were deleted outright.
So each trigger ships as a **pair** of scenarios sharing one setup helper:

- *Control (artifact expected).* The test **overwrites the `window-resized` array itself** with direct `tmux set-hook` calls against reed's socket — reproducing reed's own array minus the repaint entry — then fires the trigger and asserts the predicate **hits**.
  Rewriting the array from the test rather than adding a production seam is deliberate: it needs no build-tagged env knob, no exported test hook, and no branch in shipping code, and it exercises the exact array shape reed produced before this task.
  A control that does not hit fails the run: that means the harness can no longer reproduce the bug, and every companion assertion has become vacuous.

  **Sequencing is load-bearing and must be pinned, or the control silently tests the wrong array.**
  Every `AttachArgv` pre-flight rebuilds the array from scratch, so any attach performed *after* the rewrite re-installs the repaint entry and the control ends up asserting against reed's post-fix array — which will not hit, and will look like a broken harness rather than a broken test.
  The rule is therefore: **the rewrite is the last setup step, after every attach the scenario performs, and immediately before the trigger.**
  In the cross-client scenario that means both attaches complete first, then the rewrite, then the input that flips the most-recently-used client.
  The trigger itself is safe to run after the rewrite because neither trigger goes through an attach: a harness-pane resize fires `window-resized` inside the tmux server, and delivering input to an existing client touches no reed code path at all.
  The control must also **prove** it fired against the array it wrote, rather than assuming it: immediately before the trigger, read the array back with `show-options -v` on `window-resized` and assert per entry that the repaint entry is absent and the expected pins are present.
  That readback is the same per-entry matching `hookInstalledLocked` performs, and it converts "we think no attach intervened" into an assertion.
- *Treatment (artifact absent).* Same setup, reed's own array left untouched, same trigger, assert the predicate **does not hit** for the whole deadline.

Both triggers get this pair:

- *Resize trigger*: boot a reed session with at least two live panes, attach inside a harness pane of a known size, resize that harness pane in both directions, capture it.
- *Cross-client trigger*: attach two clients of deliberately different sizes to the same reed session from two harness panes, deliver input to one so it becomes the most-recently-used client, capture the other.
  Size the two clients so the observed client ends up **fully covered** by the new window geometry — otherwise the scenario lands in the uncovered subset of `root-cause-model`, where dots are correct tmux behaviour and no mitigation can clear them.
  Getting that sizing right is part of the scenario, not an accident of the machine.
  If measurement shows this trigger cannot be cleared even when fully covered, the treatment scenario is inverted to assert the *documented* residual instead, and its comment cites `uncovered-subset-is-documented-not-fixed` by name — but the control scenario stays either way.

The smoke tests must not assume the artifact appears on every run at every size;
that is precisely why the control exists as an executable assertion rather than as a note in a commit message.

**Measurement gate.**
Before the repaint entry is implemented, the plan runs the resize smoke scenario against candidate 1 and, if needed, candidate 2 from `repaint-mechanism`, and records which one cleared the artifact and on what tmux version — in `doc.go`, in the same "verified live on tmux 3.6" voice the surrounding decisions use.
A candidate is accepted only if it clears the artifact **and** satisfies both acceptance criteria in `repaint-must-not-self-retrigger` (no repeated hook fire, no resize storm);
clearing the artifact alone is not sufficient.

*The no-candidate branch.*
If no candidate is accepted, no repaint entry ships, and the treatment scenarios have nothing to assert absence against — so their disposition must be stated here rather than improvised:

- Both triggers' **control** scenarios land unchanged and stay in the suite.
  They assert the artifact appears against reed's shipped array, which is now the pre-task array, and they are the durable record that the artifact is real and reproducible.
  Their array-rewrite setup step becomes a no-op in effect but is kept, so the scenarios do not have to be rewritten if a mechanism is found later.
- Both triggers' **treatment** scenarios are **inverted**, not skipped and not deleted: each asserts the artifact **appears** and cites `uncovered-subset-is-documented-not-fixed` and the recorded measurement by name in its comment.
  An inverted treatment is a live tripwire — if a future tmux release or a future reed change makes the artifact stop appearing, the scenario fails and someone finds out, which a `t.Skip` would never do.
- The negative result itself is recorded in `doc.go`: which candidates were tried, on which tmux version, and which criterion each failed.

That branch is a complete, acceptable outcome, not a failure — the artifact is WARN-severity and cosmetic, and an explained, tested, reproducible residual is worth more than a speculative hook entry.
This disposition supersedes nothing in the cross-client scenario's own sizing-related inversion rule above: that rule covers the case where a mechanism ships but the scenario lands in the uncovered subset, and this one covers the case where no mechanism ships at all.

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
- **Q:** Does the new hook entry ship on Windows? **A:** [auto-pick] No — and it must be given its own `""`-returning builder to achieve that, because it inherits nothing. **Why:** `installResizePinsLocked` has no `runtime.GOOS` gate and installs the clear plus every pin on Windows; only the signal entry is excluded there, solely because `resizeSignalHookCommand` returns `""`. The underlying reason for excluding it is unchanged: `set-hook`/`run-shell` are outside `requiredSubcommands` and psmux support is unverified.
- **Q:** Where is the `window-resized` array actually (re)installed? **A:** [auto-pick] `AttachArgv`'s pre-flight and non-`SkipFocus` applies only — never at boot, and never by the watchdog's own re-apply, which sets `SkipFocus: true` and returns before the install. **Why:** it changes the true `watchdog: off` residual and it is the reason the smoke control must sequence its array rewrite after every attach.
- **Q:** Which target form does `list-clients` take? **A:** [auto-pick] `exactSessionTarget` — the bare `=<name>` session form, at both call sites. **Why:** `-t` on `list-clients` is a session target; `exactSessionWindowTarget`'s trailing colon exists only for the window/pane parsers.
- **Q:** What if measurement shows no candidate mechanism clears either trigger? **A:** [auto-pick] Land the regression tests, the attach-time warning, and the documentation, and record the negative result in `doc.go`. **Why:** a WARN-severity cosmetic artifact that is understood, tested, and explained is an acceptable outcome; a speculative hook entry that fixes nothing is not.
- **Q:** Is the repaint entry gated on `watchdog: off` like the signal entry? **A:** [auto-pick] No — independent; `watchdog` gates the self-healing loop and its signal entry only. **Why:** the kill-switch means "stop mutating my layout", and a forced redraw mutates nothing. The clear that `watchdog: off` performs is *not* always followed by a rebuild — with `watchdog: off` the array stays empty from boot until the first attach or first focusing apply — but that residual is today's behaviour for the pins too, and the repaint entry shares their lifecycle rather than inventing one.
- **Q:** Where do the tmux binary path, socket, and client loop in candidate 1's hook body come from? **A:** [auto-pick] `e.TmuxPath()`, `e.Socket()`, `exactSessionTarget` (bare `=<name>`, since `list-clients -t` takes a session target), and a **new** `internal/shell` line-iterating primitive with POSIX and pwsh implementations. **Why:** `resizeHookCommand` establishes none of them, and the Shell Mechanics Seam forbids building the fragment inside `reedengine`.
- **Q:** Can the repaint entry retrigger `window-resized` through `window-size latest`? **A:** [auto-pick] It must be proven it cannot — a server-issued `refresh-client` changes no geometry and is not client input, so it should not move MRU; "no repeated fire" and "no resize storm" become hard acceptance criteria of the measurement gate. **Why:** a resize storm inside the tmux server would be far worse than the one-second cosmetic smear the entry exists to remove.
- **Q:** What happens to the treatment scenarios if no candidate is accepted? **A:** [auto-pick] Inverted to assert the artifact appears, citing the recorded measurement — never skipped or deleted. **Why:** an inverted treatment is a tripwire that fires if the artifact ever stops reproducing; a `t.Skip` tells nobody anything.
- **Q:** Do `list-clients` and `refresh-client` join `requiredSubcommands`? **A:** [auto-pick] No — both call sites degrade silently. **Why:** that list is the set the engine cannot work without; adding a cosmetic-feature verb would fail boot for multiplexers that run reed fine today.
- **Q:** How does the smoke suite avoid asserting absence vacuously? **A:** [auto-pick] Each trigger ships as a control/treatment pair; the control rewrites the `window-resized` array from the test without the repaint entry and asserts the artifact **appears**. **Why:** an absence-only assertion would keep passing if the fix were deleted, and would pass vacuously wherever the timing-dependent artifact never shows.
- **Q:** What predicate detects the dots? **A:** [auto-pick] A run of at least 20 consecutive `.` on any single captured line, polled to a bounded deadline — not `pollPaneContains`. **Why:** `pollPaneContains` is a plain substring match and legitimate pane content contains dots.
- **Q:** Does `manifest/roadmap.md` move? **A:** [auto-pick] No. **Why:** CLAUDE.md reserves roadmap movement for completing or adding a planned item; this is a bugfix/hardening pass covered by git history and `doc.go`.
