# Discussion: reed: attach doesn't reconcile session geometry with the terminal

```yaml
task: 'reed: attach doesn''t reconcile session geometry with the terminal'
slug: reed-attach-geometry-reconcile
status: discussing
parent: main
```

## Problem

reed boots every tmux session at a fixed, config-pinned size (`reed.yaml`'s `width: 220` / `height: 50`, spent as `new-session -x/-y` in `internal/reedengine/lifecycle.go:310-313`) and computes its whole pane layout against that same pinned box (`render.Box{W: e.cfg.Width, H: e.cfg.Height}` in `internal/reedengine/apply.go:75`).
Nothing in reed ever asks the multiplexer how big the window actually is, and neither attach path — `internal/reedcli/attach.go`'s `attachArgv` nor `internal/loomcli/bootstrap.go`'s duplicate of it — does anything but a bare `attach-session`.
The task was raised as wiki #115 / loom crucible finding **F12**, operator-confirmed with a screenshot: `lyx reed attach` left the operator's pre-attach terminal content visible around the session instead of the full-screen handover `manifest/designs/loom.md`'s "Entry point" promises ("**loom goes to the background and the tmux session takes the window**").

**Why now:** the finding was recorded NOT-FIXED-THIS-ROUND during the loom crucible campaign (commit `1f549606`) because the pinned geometry and the attach argv live outside that campaign's module, and was carried out as this task.

**What live probing changed about the diagnosis.**
F12's stated mechanism was checked against real tmux 3.6 on this Linux box, driving `attach-session` through a real pty at a chosen size (probe scaffolding under the session scratchpad, not committed).
Two of F12's premises do not hold on native tmux, and one holds harder than stated:

- **tmux DOES reconcile the session size on attach.** The global `window-size` option defaults to `latest`, so a 220x50 detached session attached from a 100x30 client resized to `100x29` (29 = 30 rows minus the tmux status line). A 250x60 client resized it to `250x59`. reed never sets `window-size`, and does not need to.
- **tmux DOES clear the screen on attach.** The first bytes tmux wrote to the pty were `ESC[?1049h … ESC[H ESC[2J` (alternate screen + clear). With `TERM=vt100`, which has no alternate screen, it still cleared with `ESC[H ESC[J`. So "no screen clear before handover" is not the mechanism either.
- **The pinned RENDER BOX is a real, reproducible defect.** `select-layout` with a layout string whose dimensions disagree with the live window exits 0 and silently **rescales the layout proportionally**. Measured: the layout `220x50,0,0[220x1,…,220x3,…,220x21,…,220x21,…]` applied to a 100x30 window became `100x30,0,0[100x1,…,100x1,…,100x12,…,100x13,…]` — a `collapsed_strip_rows: 3` strip **silently became 1 row**. Every absolute row budget reed computes (`header.height_rows`, `collapsed_strip_rows`, `min_full_rows`) is scaled by `live_height / 50` and is therefore wrong on any terminal that is not exactly 50 rows tall.

So the defect this task fixes is precisely the one its title names — reed never reconciles its geometry with the terminal — but the observable damage that is *provable on native tmux* is a distorted layout, not an unpainted region.
The operator's screenshot is not dismissed: it is carried as the acceptance check (see **Testing**), and if it survives the fix it is a separate, platform-specific defect (psmux on Windows is the leading suspect, since psmux is a tmux port whose `window-size` and screen-clear behaviour are not verified anywhere in this repo).

## Scope

**In:**

- `internal/reedengine`: render the layout against the **live** window size instead of the config-pinned box, with the pinned box as the fallback when no live size can be read.
- `internal/reedengine`: own the attach argv — one exported builder that returns the tmux argv for an in-place attach, optionally chaining a `select-layout` computed for the attaching client's size.
- `internal/reedengine`: pin the two tmux options the geometry contract rests on — `status off` and `window-size latest` — at boot, and idempotently before an attach, so the window's size equals the attaching client's size exactly.
- `internal/reedcli/attach.go`: read the operator's own terminal size, call the engine's builder, drop the local `attachArgv`.
- `internal/loomcli`: same, deleting `bootstrap.go`'s duplicated `attachArgv` in favour of the engine's builder. Both builders die, so **both** their tests go with them: `internal/loomcli/bootstrap_test.go:224-232` and `internal/reedcli/cli_test.go:84-95`.
- `reed.yaml` template comments for `width`/`height` in **both** embedded templates — `internal/reedengine/template_posix.yaml` and `internal/reedengine/template_windows.yaml` — restating what those keys now mean (the detached-boot size, not the render box).
- `internal/reedengine/doc.go`: document the live-geometry rule and the attach chain.
- Tests: tier-1 units for the new pure decisions, plus a build-tagged live-tmux pty test that proves the attach handover lands the exact layout.

**Out:**

- **Re-rendering on a live terminal resize while attached.** reed is a CLI, not a daemon, and has no process alive during the handover to react to `SIGWINCH`. tmux keeps rescaling proportionally in that window; the next reed op re-renders correctly at the new size. No tmux `set-hook` / `run-shell` machinery is introduced (neither is in `requiredSubcommands`, and psmux support for both is unverified).
- **Reacting to `window-size` values other than by pinning it.** The option is pinned to `latest` (see the Decision below); no fallback path is designed for a multiplexer that rejects or ignores the pin.
- **Chasing the unreproduced visual symptom on Windows/psmux.** If it survives this fix, it is a new task with its own evidence.
- **The header pane's own text rendering**, mouse mode, `remain-on-exit`, and every other boot option.
- **The status line as a configurable** — it goes off unconditionally; no new config key.
- **`internal/reedengine/render`'s layout algebra.** The box it is handed changes; the rules inside it do not.

## Decisions

### live-window-size-is-the-render-box

- Decision: `Engine.planLayout` (`apply.go:68-80`) gains an **explicit `render.Box` parameter** and queries nothing itself — the box is always told to it. A separate engine helper resolves "the live box": it runs `display-message -p -t '=<session>:' '#{window_width} #{window_height}'` and falls back to `cfg.Width`/`cfg.Height` when the query fails, returns an unparseable pair, or returns a non-positive dimension. `applyLayoutLocked` calls that resolver and passes the result. **The attach path does not**: it passes the client's own told size, and never the live query, because at argv-build time the live window is still the pre-attach size and would be exactly the wrong answer. Told size wins wherever a told size exists; the live query is the fallback for every caller that has none.
- Rationale: separating "what box" from "how to plan" is what keeps the two callers from disagreeing — without the parameter, a plan writer could reasonably wire the attach path through the live query and reintroduce the rescale this task exists to remove. It also makes the pure half testable at tier 1 with no tmux at all. Beyond the seam, the live query is the actual defect fix — a layout string sized to a box the window does not have is rescaled by tmux, silently violating every absolute row budget reed computes. `display-message` is already in `requiredSubcommands` (`probe.go`), so no capability-probe change and no new psmux risk. The `=<name>:` window-target form is required (a bare `=<name>` is parsed as a pane target and fails with `can't find pane`) — verified live.
- Rejected: leaving `planLayout` box-less and having it query internally (the attach path would then plan against the pre-attach size — the seam this finding was raised on); keeping the pinned box and letting tmux rescale (the defect); deriving the window size from `list-panes` output by summing pane heights and dividers (fragile, and wrong whenever foreign panes are present); passing the size down from the CLI on every op (only the attach path knows a terminal size at all).

### attach-chains-select-layout

- Decision: the attach argv becomes the seven elements `["-L", <socket>, "attach-session", "-t", "=<session>", ";", "select-layout"]` followed by `"-t"`, `"=<session>:"` and `<layout>`, where `<layout>` is planned for the attaching client's own size. The separator is a **literal single-character `;` argv element** — not `\;`, which is only how a shell is told to pass `;` through; `exec.Command` takes the argv directly and never sees a shell. The chained `select-layout` carries an explicit `-t =<session>:` (the `exactSessionWindowTarget` form) rather than relying on the client's current window, matching the exact-target discipline `doc.go` documents and every other reed call site follows. When no client size is available, or when the engine would skip the layout apply anyway, the argv is today's bare `attach-session` form.
- Rationale: verified live — a chained command runs *after* the client has attached and the window has already been resized to the client, so the layout lands verbatim with no rescale (`100x29` window, header 1 row, collapsed strip 3 rows, exactly as planned); re-verified with the explicit `-t =<session>:` present, which changes nothing about the outcome and removes the dependency on which window the new client happens to land in. It needs only `attach-session` and `select-layout`, both already required. `attach-session` is first in the chain, so if `select-layout` is unsupported or fails, the operator still gets an attached session — strictly no worse than today.
- Rejected: `resize-window` before attaching (not in `requiredSubcommands`; also flips the window's `window-size` option to `manual`, which then *stops* tmux resizing on a later terminal resize — verified live — turning a layout bug into the very unpainted-region bug F12 described); a tmux `client-attached` hook via `set-hook` (needs `set-hook` + `run-shell`, neither required nor verified on psmux, plus a re-entrant `lyx` invocation racing the lock); chaining `run-shell "lyx reed …"` (same objection); doing nothing and relying on the next reed op to re-render (leaves the operator's first frame — the one they look at — wrong).

### geometry-options-pinned-at-boot-and-attach

- Decision: reed sets **two** options on the fresh-boot path beside `remain-on-exit` and `mouse` — `set-option -g status off` and `set-option -g window-size latest` — and sets both again, idempotently and non-fatally, in the attach pre-flight.
- Rationale for `window-size latest`: `latest` is tmux's default, but a default is exactly what reed must not rely on here. reed's server loads the operator's `~/.tmux.conf` (no `-f` is passed), and a conf setting `window-size largest` or `window-size manual` makes "the window becomes the attaching client's size" false — which silently invalidates the told box the attach path computes from its own TTY and reintroduces the rescale this whole task removes. That is the same argument that already makes `mouse` an explicit both-ways set at boot, and it applies with more force here because the geometry contract is now load-bearing rather than cosmetic. Pinning costs one `set-option` call and removes the only remaining way an operator's config can break the fix.
- Rationale for `status off`: the attach path must predict the post-attach window height from its own TTY row count *before* the client exists. With the status line on, the window is `rows - 1` (verified: 100x30 client → `100x29` window); with it off the window is exactly `rows` (verified: `100x30`). Guessing `-1` would be wrong whenever the status line is off, two rows tall, or moved — all reachable, because reed's tmux server loads the operator's `~/.tmux.conf` (reed passes no `-f`), which is exactly why reed already pins `mouse` explicitly in both directions at boot. reed also already draws its own header pane, so the tmux status line is redundant chrome, not lost information. Re-setting it in the attach pre-flight covers a server booted by an older lyx, where the boot-path option was never set (boot options never re-apply to an already-up session — see the early return in `ensureServerLocked`).
- Rejected: leaving `window-size` to its default (trusts a tmux default in the one place a hostile `~/.tmux.conf` would silently break the fix, while pinning `status` and `mouse` against exactly that risk — an inconsistency with no argument behind it); predicting the reserved rows by reading the `status` option (`show-options` is not in `requiredSubcommands` — a capability-probe change and a psmux unknown, to recover information we are better off pinning); hardcoding `-1` (wrong under a hostile or merely different `~/.tmux.conf`); making it a config key (no requirement behind it — YAGNI).

### engine-owns-the-attach-argv

- Decision: the attach argv builder moves into `internal/reedengine` as an exported `Engine` method taking the client's terminal size — e.g. `AttachArgv(cols, rows int) ([]string, error)` — and both `internal/reedcli/attach.go` and `internal/loomcli` call it. `loomcli`'s `attachArgv` and its test (`bootstrap_test.go`'s `attachArgv` case) are deleted.
- Rationale: the brief names the duplication explicitly. Planning the chained layout needs the strand table, the live pane list, and the op lock — all engine-side; a CLI-side builder would need every one of them exported. Keeping tmux vocabulary in the engine is also what the CLI/Cobra Invariant asks for (cli imports engine, never the reverse; engine returns `(T, error)`). One builder means one place where the "skip the chain" guards live.
- Rejected: keeping two builders and duplicating the size logic (the exact defect shape the brief calls out); exporting `planLayout` and letting each CLI assemble the argv (leaks layout internals and the lock discipline into two callers).

### terminal-size-from-x-term

- Decision: the CLIs read the operator's terminal size with `golang.org/x/term`'s `GetSize` against `os.Stdout`'s fd. This is a **new direct module requirement**: `go.mod` has no `golang.org/x/term` line today, in either require block, and no Go source in the repo imports it — only `go.sum` carries it (`v0.42.0`), as a transitive artefact. Adding it means a `go get golang.org/x/term` and a new direct `require` line, not a promotion. A non-TTY stdout (piped output, no controlling terminal) yields no size, and the engine builds the bare attach argv.
- Rationale: the module is a two-function wrapper (`GetSize`, `IsTerminal`) maintained by the Go team over `golang.org/x/sys`, which is already a direct dependency at `v0.45.0` — so the new edge adds no new transitive subtree, only a name in `go.mod`. It covers the POSIX `TIOCGWINSZ` ioctl and the Windows console API behind one call; the alternative is hand-writing both against `x/sys` with per-GOOS build tags, which is the same dependency reached the long way round. Falling back to the bare argv on a non-TTY is exactly today's behaviour, so nothing regresses.
- Rejected: `golang.org/x/sys` directly with per-GOOS build tags (avoids the `go.mod` line at the cost of hand-written platform code — a defensible alternative if the plan writer prefers zero new requires, and the *only* thing that changes is where the ioctl lives); shelling out to `stty size` (a subprocess, POSIX-only); trusting `$COLUMNS`/`$LINES` (unexported by most shells, stale after a resize).

### chain-skipped-when-the-layout-apply-would-be

- Decision: the attach argv omits the chained `select-layout` under the same conditions `applyLayoutLocked` already skips its own apply: fewer than two live panes, or no strand owning a present pane (`anyPlacedStrand`). It is also omitted when no client size is known, and when planning returns an error.
- Rationale: `applyLayoutLocked`'s guards are not stylistic. A layout string enumerating zero panes is accepted by tmux (exit 0) and answered by **destroying every pane in the session**, which the existing comment records as verified live. An attach that wipes the session it is attaching to would be a far worse bug than the one being fixed, so the guards must be reproduced on this path — which is a second reason the builder belongs in the engine, where they already live.
- Rejected: chaining unconditionally (reachable session-wipe); refusing to attach when the layout cannot be planned (attach must stay the operator's escape hatch into a session, including a broken one).

### width-height-keys-keep-their-name-and-change-their-meaning

- Decision: `reed.yaml`'s `width`/`height` stay, and stay the `new-session -x/-y` boot size — the size a session has while no client has ever attached. Their template comments are updated to say so. No migration, no `configsync` change, no new key.
- Rationale: the keys still have exactly one real job after this change, and it is the one they already do. Renaming them would churn every materialized `reed.yaml` in every hub for no behavioural gain, and it would break every existing worktree — by the *missing*-key mechanism, not an unknown-key one. `internal/configengine`'s `load()` runs `yamlengine.MissingKeys(template, fileBytes)` and hard-fails with `config file <path>: missing keys: …; run "lyx config reconcile"` (`config.go:101-113`) whenever the **template** names a key the on-disk file lacks, and `LoadOrTemplate` degrades only on an absent `_lyx/` or absent file — an existing `reed.yaml` still goes through that check. So a renamed key is absent from every already-materialized `reed.yaml` and hard-fails on load until the operator runs `lyx config reconcile`. The reverse direction is silent: `internal/reedengine/config.go:50` unmarshals with plain `yaml.Unmarshal` and no `KnownFields`, so the *old* key left on disk is dropped without error, and `internal/configsync`'s `Reconcile` merely reports it as `removed`. Nothing in the repo hard-fails on an unknown key, so a migration/compat path would have to handle the missing-key failure, not an unknown-key rejection.
- Rejected: removing the keys (the detached boot still needs a size); renaming to `boot_width`/`boot_height` (breaks every materialized config for a comment's worth of clarity); adding a separate `render_width`/`render_height` (the render box is now read from the live window — a config key for it would be a second source of truth for a value tmux already owns).

### live-resize-while-attached-is-out-of-scope

- Decision: reed does not react to a terminal resize that happens while the operator is attached. tmux's proportional rescale stands until the next reed op re-renders.
- Rationale: reacting requires a process alive during the handover, which the interactive-handoff exception (CLI/Cobra Invariant) explicitly does not have — the lyx process has given its stdio away and is blocked in `attach.Run()`. The alternatives are a tmux hook (rejected above) or a background watcher process, which is a daemon reed has deliberately never been. In the loom case the driver drives reed ops continuously, so the window between a resize and a correct re-render is short in the flow this task was raised from.
- Rejected: a `SIGWINCH` forwarder in the attach parent (it does not own the terminal any more); a tmux hook (see **attach-chains-select-layout**).

## Technical context

**The two call sites the fix converges.**

- `internal/reedcli/attach.go:21` — `attachArgv(socket, session)`, called at line 53. The `RunE` is already structured as pre-flight (`c.eng.Status()`) followed by a terminal-handover tail, and is a registered holder of the CLI/Cobra Invariant's interactive-handoff exception. The new size read and `AttachArgv` call belong in the pre-flight half, above the handover, so a failure still reports on the JSON envelope.
- `internal/loomcli/bootstrap.go:166` — a verbatim copy of the same builder, called from `internal/loomcli/run.go:295` in the `lyx loom run` bootstrap's step 7, which is the same pre-flight-then-handover shape and the same registered exception. `internal/loomcli` already imports `internal/reedengine`, so calling the engine's builder adds no import edge.

**Where the render box comes from today.**

- `internal/reedengine/apply.go:75` — `planLayout` is the sole construction of `render.Box`, and `e.cfg.Width`/`e.cfg.Height` have exactly two production readers repo-wide: this line and `lifecycle.go:312-313`'s `new-session -x/-y`. That is the whole blast radius.
- `applyLayoutLocked` (same file) runs `planLayout`, then skips both tmux calls when `len(live) < 2` or `!anyPlacedStrand(...)`. Those guards, and the session-wipe they prevent, are documented in place.
- `render.Rules` (`render/rules.go:20`) subtracts the header band and its one-row divider from the box before `stackHeights` distributes rows among strands, so a box that is too tall inflates every strand and a box that is too short starves them — before tmux's own rescale is applied on top.

**The tmux overlay.**

- `TmuxCmd.output` / `TmuxCmd.run` (`overlay.go`) are the only exec paths, both auto-prefixing `-L <socket>`; a new query goes through `output`, not a fresh `exec.Command`.
- `exactSessionWindowTarget(session)` already builds the `=<name>:` form the `display-message` query needs. `exactSessionTarget` builds the bare `=<name>` session form the attach argv already uses.
- `requiredSubcommands` (`probe.go`) already lists `display-message`, `select-layout`, `set-option`, `list-panes`. **No probe change is needed**, which is the point — every subcommand this task spends is already proven present before the engine runs.
- `TmuxCmd.execHook` is the white-box seam a test stubs to drive a composed engine call site against scripted tmux output with no live server. It is the seam for tier-1 coverage of the live-size query's fallback paths.

**Live tmux facts established for this task (tmux 3.6, Linux, real pty).**

| Probe | Result |
| --- | --- |
| `show-options -g window-size` | `latest` (tmux reconciles session size to the attaching client itself) |
| Attach 220x50 session from a 100x30 pty | window becomes `100x29`; output begins `ESC[?1049h … ESC[H ESC[2J` |
| Same, `TERM=vt100` | window becomes `100x29`; clears with `ESC[H ESC[J` (no alternate screen) |
| Same, with `set-option -g status off` | window becomes `100x30` — exactly the client's rows |
| `select-layout` of a `220x50` string into a `100x30` window | exit 0, silently rescaled; a 3-row cell became 1 row |
| `resize-window -x -y` | works, and sets that window's `window-size` to `manual` |
| `attach-session … ; select-layout <string sized to the client>` | layout applied verbatim post-resize; window `100x29`, cells exactly as planned |
| Same chain with an explicit `select-layout -t '=<sess>:'`, status off | window `100x30`, `window_layout` byte-identical to the planned string |
| `go.mod` | carries `golang.org/x/sys v0.45.0` and **no** `golang.org/x/term` line; only `go.sum` has term (`v0.42.0`), and nothing imports it |
| `display-message -p -t '=<sess>:' '#{window_width} #{window_height}'` | prints `220 50`; the bare `=<sess>` form fails with `can't find pane` |

**Gotchas.**

- Boot options never re-apply to an already-up session: `ensureServerLocked` returns early on the healthy already-up path, above the `set-option` block (`lifecycle.go:399-410`, where `remain-on-exit` and `mouse` are set). Any option this task needs at attach time must be set at attach time too — which is why both new pins run in the attach pre-flight as well as at boot.
- `planLayout` today takes no box (`apply.go:68-80`); adding the box parameter changes its signature and its single call site in `applyLayoutLocked`. It is unexported, so the change stays inside the package.
- reed's tmux server loads the operator's `~/.tmux.conf` (no `-f` is passed), which is why `mouse` is pinned explicitly in both directions at boot, and why the status line must be pinned rather than assumed.
- The session's tmux server is shared per hub across sibling worktrees, so `-g` options are hub-wide. That is already true of `mouse` and `remain-on-exit`, and is intended for the status line too.
- psmux (Windows) is a tmux port, not tmux. Everything above is verified on native tmux only. Every new tmux interaction on the attach path must fail non-fatally.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant** — `Short` on every command; errors as JSON envelopes via `internal/output`; every `RunE` checks `clihelp.ShouldAbort` first; one envelope per invocation. `lyx reed attach` and `lyx loom run` are named holders of the **interactive-handoff exception**, so only the terminal-handover tail is exempt: the terminal-size read, the `status off` call, and the `AttachArgv` call are all fallible and must therefore sit in the pre-flight half, reporting on the envelope. Help accuracy is a review obligation — `attach`'s `Long` describes the handover and must stay true.
- **Told-Geometry Invariant** — `internal/reedengine` is on the review-obligation list: it takes its absolute paths from its caller and must not import `internal/lyxcwd`. Nothing here needs a path, but the new engine method must not start resolving one.
- **Shuttle Provider-Seam Invariant** — `internal/reedengine` stays provider-invariant; nothing Claude-specific enters it.
- **Test Tier Purity Invariant** — an untagged test file may not call `exec.Command`/`exec.CommandContext` or sleep ≥1s on a constant duration. Every live-tmux and pty test in this task must carry a `//go:build` constraint naming `integration` or `smoke`.
- **Hermetic Git Test Environment Invariant** — applies to git-spawning packages; a tmux-only test package does not acquire a `TestMain` obligation from it.
- **Config Strictness Invariant** — `internal/reedengine` is pinned on the **degrading** side (`LoadOrTemplate`), so an absent `_lyx/` or absent `reed.yaml` resolves the embedded template rather than erroring; this task must not move it to the strict side. Separately, and not part of that invariant: an *existing* `reed.yaml` still goes through `configengine`'s `MissingKeys` check, which hard-fails on any key the template names and the file lacks — the reason `width`/`height` keep their names.
- **Documentation Lifecycle** — docs land in the same commit. reed has no `manifest/designs/reed.md`; its module doc is `internal/reedengine/doc.go`, which already documents the subcommand set and the layout rules and must record the live-geometry rule, the attach chain, and the status-line pin. `docs/overview.md`'s module table does not change (no module added or removed).

Discovered during discussion:

- `requiredSubcommands` must not grow — every subcommand used is already in it, and adding one is a psmux-compatibility risk with no benefit here.
- The attach path must never be able to destroy panes: the `anyPlacedStrand` / `len(live) < 2` guards are load-bearing, not cosmetic.

## Testing

**Tier 1 (untagged, no spawn).**

- The live-size query's parsing and fallback: given scripted `display-message` output via `TmuxCmd.execHook` — a well-formed `"220 50"`, a trailing-newline form, garbage, an empty string, a non-positive dimension, and an error return — assert the resulting `render.Box` is the live pair in the good cases and the configured `cfg.Width`/`cfg.Height` in every degraded one. **TDD candidate** — pure decision, many edges, no substrate.
- The attach argv builder's shape: with a known client size and a plannable layout, the argv is the bare attach form followed by the literal `;` element, `select-layout`, `-t`, `=<session>:`, and the planned layout string — assert the separator is the one-character `;`, never `\;`; with a zero/absent size, with fewer than two live panes, with no strand owning a present pane, and with a planning error, the argv is exactly today's bare form. **TDD candidate** — this is the guard that keeps an attach from wiping a session.
- The box source on each path: assert the attach builder plans against the **told** client box and issues no `display-message` query at all (a stubbed `execHook` that fails the query must not change the argv), while `applyLayoutLocked` plans against the resolver's box. This is the seam the two decisions rest on, so it is worth an explicit test rather than an inference.
- Layout correctness at a given box: `render.Rules` against a small box asserts that `header.height_rows`, `collapsed_strip_rows` and `min_full_rows` are honoured as absolute row counts — the budgets tmux's rescale was destroying. Extend the existing `render/rules_test.go` table rather than adding a parallel one.
- Both old builders' tests go with their functions: `internal/loomcli/bootstrap_test.go:224-232` and `internal/reedcli/cli_test.go:84-95`, each of which pins the bare `-L <socket> attach-session -t =<session>` argv the engine builder replaces. Their coverage is not lost — it moves into the engine builder's own no-size/no-layout case, which asserts exactly that argv.
- Help-tree and seam tests (`cmd/lyx/helptree_test.go`, `seamsignature_test.go`) must stay green unchanged — no command is added or renamed.

**Tier 2 (build-tagged `smoke`/`integration`, live tmux).**

The brief is explicit that the visual half needs a real attached TTY, so the acceptance evidence is a live pty test, not source review. It must:

1. Boot a real reed session at the pinned 220x50 with a header pane and at least two strands.
2. Fork a pty of a deliberately different size (e.g. 100x30), set its winsize with `TIOCSWINSZ`, and exec the engine's attach argv in the child.
3. After the attach settles, assert from outside the pty that `#{window_width}x#{window_height}` equals the pty's size exactly (status line off) and that `#{window_layout}` equals the layout the engine planned for that size, byte for byte — the assertion that fails today, because tmux's rescale rewrites it.
4. Assert the collapsed-strip and header row counts survived, which is the operator-visible half stated as a number.
5. Cover the degraded path: attach with no chained layout (non-TTY size) still attaches successfully.

Platform gating: skip on Windows/psmux with a stated reason, in the shape `internal/reedcli/smoke_lifecycle_test.go` already uses for its psmux-specific and tmux-specific premises. The pty harness is POSIX-only and must live behind that skip.

**Operator acceptance (outside the suite).**

The final check is the one that raised the task: run `lyx loom run` (or `lyx reed attach`) in a real terminal and confirm the session takes the whole window with no pre-attach content around it. If leftover content survives a green tier-2 test, the residue is platform-specific and gets its own task with its own screenshot — it is not this task's failure to fix the geometry, which the tier-2 assertion proves independently.

## Q&A log

- **Q:** F12 claims attach leaves pre-attach terminal content on screen because reed never reconciles session size and never clears. Live probing on tmux 3.6 shows tmux resizes the window to the client (`window-size latest`) and clears with `ESC[2J` (and with `ESC[J` under `TERM=vt100`). Do we fix the stated mechanism or the defect that reproduces? **A:** [auto-pick] Fix the verified defect — the pinned render box — and carry the operator's visual symptom as the acceptance check. **Why:** the rescale is measurable and wrong on every terminal that is not exactly 50 rows (a 3-row collapsed strip became 1 row); the unpainted-region symptom cannot be reproduced on native tmux, so "fixing" it would mean changing code against no evidence, and if it survives it is platform-specific and needs its own evidence.
- **Q:** How should the attach path get a correct layout onto the window, given the client does not exist until `attach-session` runs? **A:** [auto-pick] Chain `attach-session … ; select-layout <layout planned for the client's size>`. **Why:** verified live that the chained command runs after the resize and lands verbatim; it needs only already-required subcommands, and putting `attach-session` first means a failure of the chained half still leaves the operator attached.
- **Q:** The post-attach window height is the client's rows minus the status line. How is that predicted before attaching? **A:** [auto-pick] Pin `status off` at boot and idempotently at attach. **Why:** verified that with the status line off the window equals the client's rows exactly; reed already draws its own header pane, and reed's server loads the operator's `~/.tmux.conf`, so the status line's state must be pinned rather than assumed — the same argument that already makes `mouse` an explicit both-ways set at boot.
- **Q:** `resize-window` before attaching is the more obvious fix. Why not? **A:** [auto-pick] Rejected. **Why:** it is not in `requiredSubcommands`, and it flips the window's `window-size` to `manual` (verified live), which stops tmux resizing on a later terminal resize — converting a layout bug into the unpainted-region bug F12 described.
- **Q:** Where does the render box come from for ordinary (non-attach) ops? **A:** [auto-pick] Query the live window with `display-message -p -t '=<sess>:' '#{window_width} #{window_height}'`, falling back to `cfg.Width`/`cfg.Height`. **Why:** `display-message` is already required, so no capability-probe change and no new psmux risk; the fallback keeps a session with no readable window rendering exactly as it does today.
- **Q:** Does the capability probe's `requiredSubcommands` list need to grow? **A:** [auto-pick] No. **Why:** every subcommand this design spends — `attach-session`'s argv is built but never probed, `select-layout`, `display-message`, `set-option` — is already listed or already unprobed; growing it would add a psmux unknown for no gain.
- **Q:** Where does the attach argv builder live, given `loomcli` duplicates it? **A:** [auto-pick] In `internal/reedengine`, as an exported `Engine` method both CLIs call; delete `loomcli`'s copy and its test. **Why:** planning the chain needs the strand table, the live pane list, and the op lock, all engine-side; the CLI/Cobra Invariant already puts tmux vocabulary in the engine; and one builder means one place for the session-wipe guards.
- **Q:** How does the CLI learn the terminal size? **A:** [auto-pick] `golang.org/x/term`'s `GetSize`, promoted from indirect to direct. **Why:** already in `go.sum`, and it covers POSIX and Windows without hand-written build-tagged ioctl code; a non-TTY yields no size and falls back to today's bare argv.
- **Q:** Must the attach path reproduce `applyLayoutLocked`'s skip guards? **A:** [auto-pick] Yes — omit the chained `select-layout` under the same conditions. **Why:** a layout enumerating zero panes is accepted by tmux and destroys every pane in the session (documented as verified live in `apply.go`); an attach that wipes its own session is worse than the bug being fixed.
- **Q:** Do `reed.yaml`'s `width`/`height` keys change? **A:** [auto-pick] They keep their names and stay the detached-boot size; only their comments change. **Why:** a rename breaks every already-materialized `reed.yaml` through `configengine`'s `MissingKeys` hard failure (the new name would be absent from every on-disk file), while the old key would be dropped silently — no unknown-key rejection exists anywhere in the repo; the keys still have exactly the job they already do.
- **Q:** What about a terminal resize while the operator is attached? **A:** [auto-pick] Out of scope, documented. **Why:** reacting needs a process alive during the handover, which the interactive-handoff exception explicitly does not have; the alternatives are a tmux hook or a daemon, both larger than the defect.
- **Q:** `planLayout` would query the live window, but at attach-argv-build time the live window is still the *pre-attach* size — so which source feeds the chained layout? **A:** [auto-pick] Give `planLayout` an explicit `render.Box` parameter and query nothing inside it; a separate resolver does the live query for `applyLayoutLocked`, and the attach path passes its told client box. **Why:** without the parameter both sources are reachable from the same function and a plan writer could wire the attach path through the live query, which would plan against the pre-attach size and reintroduce the exact rescale this task removes; told size wins wherever a told size exists.
- **Q:** The design trusts tmux's `window-size latest` default while pinning `status` and `mouse` against defaults on the grounds that `~/.tmux.conf` is loaded. Is that consistent? **A:** [auto-pick] No — pin `window-size latest` too, at boot and in the attach pre-flight. **Why:** a conf setting `largest` or `manual` makes "the window becomes the attaching client's size" false, which invalidates the told box the attach path computes from its own TTY; one `set-option` call closes the last way an operator's config can silently break the fix.
- **Q:** Is `golang.org/x/term` a promotion from indirect, as first written? **A:** [auto-pick] No — it is a new direct requirement; restate it as one. **Why:** `go.mod` has no `golang.org/x/term` line in either require block and nothing in the repo imports it; only `go.sum` carries it transitively. The decision still stands (it wraps `x/sys`, already direct, and adds no new subtree), but a plan writer must expect a `go get` and a new `require` line, and `x/sys` with build tags remains a defensible alternative.
- **Q:** How is the chain separator spelled, and does the chained `select-layout` carry a target? **A:** [auto-pick] A literal one-character `;` argv element, and yes — `-t =<session>:`. **Why:** `exec.Command` passes argv directly with no shell, so `\;` would be a literal backslash-semicolon; and every other reed call site uses an exact `=<name>`/`=<name>:` target by deliberate discipline, so the chained call should not be the one place that relies on wherever the new client happens to land. Re-verified live with the explicit target: the layout still lands byte-identical.
- **Q:** How is the fix proven, given the brief says visual confirmation needs a real TTY? **A:** [auto-pick] A build-tagged live-tmux test driving `attach-session` through a real pty of a deliberately different size, asserting window dimensions and the exact `window_layout` string, plus an operator run. **Why:** it converts the operator's screenshot into a numeric assertion that fails today and passes after; the pty harness is POSIX-only and skips on Windows/psmux in the shape the existing smoke suite already uses.
