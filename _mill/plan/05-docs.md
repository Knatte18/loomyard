# Batch: docs

```yaml
task: "Reed attach dot-fill render artifact on resize and cross-client mouse move"
batch: "docs"
number: 5
cards: 2
verify: go test -count=1 ./internal/reedengine/ ./cmd/lyx/
depends-on: [2, 3, 4]
```

## Batch Scope

This batch writes the record: the mechanism, what was fixed, and what is inherent.
reed has no `manifest/designs/reed.md` — its design record is the long comment block in `internal/reedengine/doc.go`, whose `Load-bearing behavioral assumptions` list already documents the resize round-robin, the resize-pin hook, the chained attach, the two geometry pins, why `window-resized` is the only usable event source, and the `show-options -v` array behaviour.
The new decisions belong there, in that same bullet list, in the same voice and with the same evidence discipline.

It is one batch because both documents describe the same finding to two audiences — the next reader of the code, and the next operator running the live observation suite — and both must agree.
It runs last because it must describe what actually shipped: which candidate was accepted, whether a repaint entry exists, and what the residual is.

`docs/overview.md` is deliberately untouched: no module is added and the execution stack does not change.
`manifest/roadmap.md` is deliberately untouched: this is a bugfix/hardening pass, and roadmap movement is reserved for completing or adding a planned item.

## Cards

### Card 18: the decision log in doc.go

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/probe.go`
  - `internal/shell/shell.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the package doc comment's `Load-bearing behavioral assumptions` list with the following, as bullets in the existing style, placed beside the resize round-robin and hook bullets and beside the `Measurement record (repaint candidates)` block batch 2 already added.
  Every claim that rests on live evidence names the tmux version it was measured on, matching the surrounding "verified live on tmux 3.6" voice.

  1. **The dot-fill artifact's mechanism.**
     The dots are painted by tmux itself, in the region of an attached client's terminal that the current window geometry does not cover or whose paint has gone stale relative to a just-changed window size.
     They are never content reed writes and are in no pane's grid — which is also why `capture-pane` against the reed session is structurally blind to them, and why the regression test captures the *harness* pane hosting the attach client instead.
     Both reported triggers are one mechanism, a transient or standing mismatch between a client's terminal size and the window's size:
     a resize changes the client's size and `window-size latest` moves the window to follow it;
     and delivering any input to a second client — a keystroke, a mouse report, a focus report — makes that client most-recently-used, which is exactly what `window-size latest` keys on, so the window resizes to it and every other client is left mismatched with no resize from the operator's point of view.
     State that the reporter's "mouse-tracking escape sequences leak through the shared session" hypothesis is rejected: mouse bytes are consumed by tmux as client input and never reach another client's screen;
     the mouse is only how the second client announced itself as most-recently-used.
  2. **The stale-paint / uncovered split, and which trigger is which.**
     The stale-paint subset is a client fully covered by the new window geometry showing leftover paint, and a forced redraw removes it — this is the resize trigger.
     The uncovered subset is a client whose terminal is genuinely larger than the window, so tmux has real estate with nothing behind it and legitimately pads it — this is the cross-client trigger as reported, because a VS Code integrated terminal is smaller than a standalone Konsole window.
     No repaint mechanism can remove the uncovered subset, and reed does not attempt to: removing it would require changing `window-size`, which is refused for stronger reasons.
  3. **Why `window-size latest` is not changed, made configurable, or conditioned on the client count.**
     With two clients of different sizes attached, tmux must pick one window size, so some client is always mismatched under every available policy;
     `latest` gives the artifact to the client the operator is not currently using, which is the best of the three.
     It is also structurally load-bearing: `AttachArgv` reads `#{window-size}` back and suppresses the chained `select-layout` on anything other than `latest`.
     Record the rejections: `largest` breaks attach-time layout for every operator with a second client open;
     `smallest` turns an intermittent artifact into a standing one;
     `manual` abandons client-following and is already a chain-suppressing value.
  4. **The repaint entry**, written to match what actually shipped.
     If an entry shipped, state which candidate it is, that it rides the existing `window-resized` array installed by `installResizePinsLocked` as the array's only install site, and that it is ordered after every resize-pane pin and before the watchdog's signal entry — after the pins so it paints geometry the pins have already fixed up, before the signal entry so the signal entry keeps its documented last position.
     State that it collapses the artifact's duration from the watchdog round trip — the hook's `run-shell` touch plus `watchdogSignalTick` plus `watchdogDebounceQuiet` plus the re-apply round trip, roughly a second — to a flicker, because it fires server-side synchronously with the resize.
     Record the rejected alternatives: shortening `watchdogDebounceQuiet` or `watchdogSignalTick` treats duration rather than cause and cannot help the cross-client trigger at all;
     issuing `refresh-client` from Go after a successful `reapplyLayout` adds a round trip at the exact moment the artifact already heals;
     a separate `client-resized` or `client-focus-in` hook is barred because `client-resized` reports the stale pre-resize size and a second install site would clobber the array.
     If no entry shipped, say so plainly and point at the `Measurement record (repaint candidates)` block for which candidates failed which gate.
  5. **The two acceptance criteria and why "did it clear the artifact" was not sufficient.**
     `window-resized` fires on a settled size change rather than on a paint, and a server-issued `refresh-client` repaints a client at its existing size without moving the most-recently-used pointer — but that reasoning was measured rather than assumed, because refreshing every client would otherwise hand most-recently-used to whichever client was refreshed last, resize the window to it, fire the hook again, and loop.
     Record both criteria — no repeated hook fire, no resize storm — and the tmux version they were measured on.
  6. **The repaint entry's independence from `watchdog`, with the full call-site map stated plainly** rather than left for a reader to re-derive across four files.
     `watchdog` gates the watch loop and its signal entry only;
     a forced redraw mutates nothing, so an operator who turns off self-healing must keep the repaint.
     The two *unset* sites and the two *install* sites are different sites: `pinGeometryOptionsLocked` is called from `internal/reedengine/lifecycle.go`'s boot path and from `AttachArgv`'s pre-flight, while `installResizePinsLocked` is called from that same pre-flight and from `applyLayoutLockedOpts` in `internal/reedengine/apply.go`, and nowhere else.
     Only the attach path holds both, in that order, in one locked closure.
     The boot path clears and returns without rebuilding.
     `AttachArgv` reaches the install only when its chain succeeds: it returns bare before taking the lock on non-positive `cols`/`rows`, and every in-closure degrade returns before the install — while `pinGeometryOptionsLocked` has already run, so under `watchdog: off` a degrading attach issues the unconditional clear and leaves without rebuilding.
     And `applyLayoutLockedOpts` returns immediately after `select-layout` when `opts.SkipFocus` is set, which is exactly the mode the watchdog's own re-apply uses — so the watchdog re-apply never installs the array.
     Conclude: the array is (re)established only by an attach whose chain succeeds or by a focusing apply (`up`, `add`, `remove`, `resume`), so under `watchdog: off` it is empty from boot and any degrading attach returns it to empty until one of those runs.
     State that this is accepted as-is and unchanged by this task — it is already today's behaviour for the resize-pane pins, and the repaint entry shares their lifecycle rather than inventing one — and that widening the install to the `SkipFocus` path is out of scope because it would put a hook-array rebuild inside the watchdog's own re-apply loop.
  7. **Windows.**
     `installResizePinsLocked` carries no `runtime.GOOS` gate: on Windows it issues the clear and every `resize-pane` pin argv exactly as elsewhere, and `pinGeometryOptionsLocked`'s early Windows return covers only the unset half of the lifecycle.
     The single mechanism keeping the signal entry off Windows is `resizeSignalHookCommand` returning `""` combined with `resizePinHookArgvs` emitting no entry for an empty body.
     So the repaint entry inherits nothing and carries its own `""`-returning check, for the same unchanged underlying reason: `set-hook` and `run-shell` are outside `requiredSubcommands` and psmux's support for them is unverified.
  8. **The attach-time multi-client warning.**
     `AttachArgv`'s pre-flight lists the session's attached clients and emits one `logger.Warn` per client whose size differs from the size this attach was told, naming that client and both sizes;
     zero differing clients produce zero lines.
     It never blocks, never changes the argv, and never reaches the JSON envelope.
     State that this is the primary deliverable for the cross-client trigger rather than a consolation prize, because that trigger's dots are the uncovered subset and cannot be repainted away — the residual's real operator cost is bewilderment, and the warning turns a mysterious artifact into a logged, searchable fact at the moment the operator creates the condition.
     Record the target-form rule that binds both new call sites: `list-clients -t` takes a **session** target, so both use the bare `=<name>` form, while `set-hook` keeps the `=<name>:` window form because the array is window-scoped.
  9. **`list-clients` and `refresh-client` stay out of `requiredSubcommands`.**
     Both are geometry-quality only, exactly like the `status`/`window-size` pins the Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` already exempts from fatality.
     A failing or unsupported `list-clients` logs one warning and emits no multi-client warning;
     a `refresh-client` the multiplexer does not implement makes the hook entry a server-fired no-op, which is the same outcome as the mitigation not helping.
     Adding them would make a multiplexer that runs reed perfectly well today fail at boot over a cosmetic feature.
 10. **If candidate 1 shipped**, record that its `run-shell` body's shell fragment is composed only through `internal/shell`, that the loop primitive (`ForEachLine` plus `LineVarRef`) was added there rather than concatenated inside `internal/reedengine` per CONSTRAINTS.md's Shell Mechanics Seam, and that the fragment embeds the multiplexer binary path and reed's `-L` socket in **both** of its tmux invocations because the tmux server's `run-shell` inherits no reed context.

  Keep every existing bullet in the section intact;
  this card adds and does not rewrite.
- **Commit:** `docs(reed): record the dot-fill mechanism, the repaint entry, and the documented residual`

### Card 19: the W5 scenario in the live suite

- **Context:**
  - `internal/reedengine/doc.go`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update the `### W5 -- Live resize, both directions (operator-assisted visual)` scenario so the next observation run recognises the residual instead of re-filing it.
  Keep the scenario's existing goal, its both-directions requirement, and its rule that a growth-only self-heal is a `FAIL` rather than a `WARN` — all unchanged.

  Add to its **Watch** text:
  - the confirmed mechanism in one or two sentences: the dots are tmux's own padding or stale paint in the region of a client's terminal the window does not cover, never reed content, and both the resize and cross-client triggers are the same client-versus-window size mismatch;
  - what the observer should now expect during a resize, written to match what shipped — a brief flicker if a repaint entry shipped, or the roughly one-second smear the watchdog heals if none did, with a pointer to `internal/reedengine/doc.go`'s decision log for which;
  - explicit cross-client repro steps, since the original finding could not explain them: attach a second `lyx reed attach` client of a **different** size to the same session from another terminal, move the pointer into or type into that second window with no click required, and watch the *first* client fill with dots;
  - the disposition: when the observed client is larger than the client that just became most-recently-used, those dots are correct tmux behaviour for uncovered real estate and are **expected** — report the scenario `OK` for that case rather than filing it again — and reed now logs a `logger.Warn` at attach time naming any already-attached client whose size differs, which is the searchable trace for this exact condition.

  Follow the repo's markdown convention throughout: semantic line breaks, one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, with plain newlines rather than trailing double-spaces or backslashes.
  Leave every other scenario, the session-end section, and the summary template untouched.
- **Commit:** `docs(sandbox): update the W5 scenario with the confirmed dot-fill mechanism`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/reedengine/ ./cmd/lyx/`.

`internal/reedengine` covers card 18: `internal/reedengine/doc.go` is a comment-only file, so what this proves is that the package still compiles and every existing test still passes after the edit — which is the whole of that card's runnable surface.

`cmd/lyx` covers card 19: that package holds the repo-wide grep guards (test-tier purity, sandbox suite coverage, hermetic git env, and the markdown link-integrity check), so it is the package that would notice a documentation edit breaking a structural invariant.
Both packages are named explicitly rather than running `./...`;
the whole-repo regression for this task is covered by the `pipeline.done_gate` run at task end.
