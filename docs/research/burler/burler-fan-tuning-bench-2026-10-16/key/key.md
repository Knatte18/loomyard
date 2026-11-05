# Key: reed-view Discussion-Review, rounds 1-5

Graded against the round-1 target pair, the five reviews and fixer reports, the final record, the board entry, the Discussion-Review rubric and the code at `fd5b0f58e`.
Code premises were checked with grep/sed in `internal/reedengine` (`windowsize.go`, `apply.go`, `attach.go`, `lifecycle.go`, `config.go`, `render/rules.go`, `render/height.go`) and `PATTERN.md`.
tmux behaviour claims (zoom cleared by `resize-pane -y`/`select-layout`/`split-window`/`kill-pane`/`select-pane`, a zoomed pane reporting `pane_top` 0, `run-shell` format-expanding its command, `pane-border-status top` drawing inside the top cell) were each measured on tmux 3.6 by the reviewer and confirmed again by the fixer; they were not re-run for this key.

Severity standard used: BLOCKING = a Decision's premise is false or its stated purpose fails in the common case; MEDIUM = a wrong or missing decision the plan writer would build wrong or cannot settle from the record; LOW = a real gap with a bounded or rare effect that a plan writer could resolve sensibly; NIT = wording, bookkeeping or surplus data.
`in-r1` is judged against `round1-target-decision-record.md` / `round1-target-support-log.md`; line numbers below are in those files.
Defect ids `X<n>` are real but fix-introduced; `—` marks noise.

## Per-finding table

| round | id | reviewer severity | grade | re-rated severity | in-r1 | defect id | one-line reason |
|---|---|---|---|---|---|---|---|
| 1 | F1 | BLOCKING | real | BLOCKING | yes | K1 | D7 subtracts a row that `pane-border-status top` takes inside the top cell; the layout must still sum to the window height (code: live box is `#{window_height}`, told box `rows - reserved`). |
| 1 | F2 | BLOCKING | real | BLOCKING | yes | K2 | D12 creates the booter's session after all others ("only then") and the first nested ordinary up meets the same dead-server trigger and recurses. |
| 1 | F3 | MEDIUM | real | MEDIUM | yes | K4 | A second booter that finds the server already up skips the step and creates its session at once; the "each after its predecessor" rationale is false. |
| 1 | F4 | MEDIUM | real | MEDIUM | yes | K9, K18 | Inserting before `]` needs a comma after the last entry, outside the markers; no JSONC bracket scan; begin-without-end unhandled. |
| 1 | F5 | MEDIUM | real | MEDIUM | yes | K10 | The parent-added config-colors decision contradicts D13's Go table, D14, Scope and the "matches D13's table" criterion. |
| 1 | F6 | MEDIUM | real | MEDIUM | yes | K11 | Config values reach a tmux format and a typed `/color` command with no closed-palette validation, and no module is named as the config reader. |
| 1 | F7 | MEDIUM | real | LOW | yes | K13 | D13 puts Claude's palette names in a shared leaf package while the constraint and `PATTERN-shuttle-provider-seam` keep them in `claudeengine`; resolvable by relabelling. |
| 1 | F8 | MEDIUM | real | MEDIUM | yes | K12 | The criterion demands `@strand_color` on a pre-change pane, but its segment is only persisted by the new add path. |
| 1 | F9 | LOW | real | NIT | yes | K21 | Auto-mode says no operator answered, yet the config-colors decision is operator-sourced; the support log records no round. |
| 2 | F1 | BLOCKING | real | MEDIUM | partial | K7 | `resize-pane -y` sizes the top pane's content, so the routinely pinned driver cell grows by the title row; r1 D7 never addresses pins, the r1 fix stated the wrong direction ("grows"). |
| 2 | F2 | BLOCKING | real | MEDIUM | no (round 1 fix) | X1 | The r1 fix refuses a bad color in `LoadConfig`, which only unmarshals and is loaded by teardown, watchdog and every reed verb (confirmed in code); refusal belongs at boot validation. |
| 2 | F3 | MEDIUM | real | LOW | partial | K14 | D12 claims order without a stop: a failed predecessor up is out of order (in r1); the live-later-session case arose from the r1 fix's revival trigger. |
| 2 | F4 | MEDIUM | real | MEDIUM | yes | K5, K6 | The trigger is read inside `Up`'s `withOpLock` yet the ups run outside it with no release/re-take stated; a told ordered list cannot run another worktree's up without its config, state and geometry. |
| 2 | F5 | MEDIUM | real | LOW | no (round 1 fix) | X2 | The r1 fix's temp-and-rename write replaces a read-only file in a writable directory and drops its mode, contrary to "left untouched". |
| 2 | F6 | LOW | real | LOW | yes | K18 | Markers outside the top-level array, orphan or repeated end markers and CRLF line endings / the block's terminator are unspecified, already so in r1. |
| 2 | F7 | LOW | real | LOW | partial | K19 | A persisted segment with no config key, and the surface that hands the orch watcher its resolved color, are unstated. |
| 3 | F1 | MEDIUM | real | LOW | no (round 2 fix) | X3 | The r2 fix's "pane at window row 0" has no source: `FixedHeightPins` takes no `paneOrder` (confirmed in `render/rules.go`). |
| 3 | F2 | MEDIUM | real | LOW | partial | K15 | The title-row adjustment is unconditional although D6 makes the border-status pin non-fatal (psmux degrade); r1 D7 already subtracted on intent, not readback. |
| 3 | F3 | LOW | real | LOW | partial | K16 | A one-row top cell (`clampToFit` floors at 1) leaves zero content rows under the title; r1's design already admits it, the `-y 0` form came from the r2 fix. |
| 3 | F4 | MEDIUM | real | MEDIUM | yes | K6 | The teller's entry (name, key, up function) does not let reedengine test "recorded and not live" on another worktree without deriving its path. |
| 3 | F5 | LOW | real | NIT | no (round 2 fix) | X4 | Bound, Open risks and criteria state the r2-fix ordering stop differently; one line fuses two bullets. |
| 3 | F6 | LOW | real | LOW | yes | K17 | r1 D8 single-quotes runtime-expanded `#{socket_path}`/`#{client_name}` in the `run-shell` string; a quote in either breaks the token. |
| 3 | F7 | LOW | real | LOW | no (rounds 1-2 fixes) | X5 | The fix-added atomic write, skip list, Bound and criterion disagree on failed-write reporting, check order and the temp file. |
| 4 | F1 | BLOCKING | real | BLOCKING | yes | K3 | The `window-resized` pin hook and every `select-layout` apply unzoom the window, so a resize, attach, add or resume drops D1's strand view; no Decision covers it. |
| 4 | F2 | MEDIUM | real | LOW | partial | K16 | A one-row top cell is applied with zero content rows until a resize fires the hook; same defect as R3 F3, through the apply path. |
| 4 | F3 | MEDIUM | real | LOW | partial | K17 | `run-shell` format-expands the whole command, so a `#` in a pinned path (the r1 `lyx`/tmux paths included) is rewritten; the `q:`-inside-quotes half came from the r3 fix. |
| 4 | F4 | LOW | real | LOW | partial | K15 | Keying on "not `off`" treats an inherited `bottom` as a top title row; same keying defect as R3 F2. |
| 4 | F5 | LOW | real | NIT | yes | K20 | An existing but unreadable `keybindings.json` is neither a listed skip nor a failure, already so in r1. |
| 4 | F6 | NIT | noise | NIT | no (round 1 fix) | — | A unique temp name is standard atomic-write practice the plan writer applies anyway; not a record defect. |
| 4 | F7 | LOW | real | NIT | partial | K22 | The told spawn-order key has no reader since the teller sorts; r1's constraint already tells start times next to an ordered list. The duplicate predicate wording came from the r3 fix. |
| 4 | F8 | NIT | noise | NIT | no (round 3 fix) | — | A missing round-3 history entry in the support log, which Plan-Write never reads. |
| 5 | F1 | MEDIUM | real | MEDIUM | yes | K8 | A zoomed pane reports `pane_top` 0, so `paneIDsByTop` (apply path, `AttachArgv`'s `listPanes`) maps planned heights to the wrong panes while zoomed. |
| 5 | F2 | LOW | real | LOW | partial | K3 | `split-window`, `kill-pane` and `select-pane` without `-Z` also unzoom; the r4 fix's trigger named only two commands. |
| 5 | F3 | LOW | real | LOW | no (round 4 fix) | X6 | The r4 fix's zoom bracket and post-apply pin run are described as Go steps, but the attach chain and each hook entry run in tmux after reed returns. |
| 5 | F4 | LOW | real | NIT | yes | K22 | The Constraints bullet tells "start times" (r1 line 195, unchanged) that nothing on the reedengine side reads. |
| 5 | F5 | LOW | real | LOW | no (round 2 fix) | X7 | The r2 fix's booter re-run "from the start" meets its own revival trigger again; only revive functions carry the skip. |
| 5 | F6 | LOW | noise | NIT | no (round 4 fix) | — | Exclusive creation of an already unique temp name is implementation hardening, not a record defect. |
| 5 | F7 | LOW | real | NIT | no (round 1 fix) | X8 | Scope still says "on the boot that spawns the server" after the r1 fix widened D12 to every revival. |
| 5 | F8 | NIT | noise | NIT | no (round 3 fix) | — | The Bound's shorter predicate wording is harmless, as the review itself says, since the revive function checks the full predicate. |

## Key

| K id | re-rated severity | first round | decision / line in round-1 target | description |
|---|---|---|---|---|
| K1 | BLOCKING | 1 | D7, line 86 (criterion line 243) | D7 subtracts one row from the layout budget for `pane-border-status top`, but tmux draws the title row inside the top cell and the layout must still sum to the window height; a one-row-short layout is rewritten by tmux and diverges from the live `#{window_height}` box. |
| K2 | BLOCKING | 1 | D12, lines 131-132 (criterion line 245) | D12 brings up every other worktree first and the booter's own session last, breaking prime-first spawn order whenever the booter is not last; the nested ordinary ups are not told to skip the step, so the first one re-enters it on a dead server. |
| K3 | BLOCKING | 4 | D1, line 33 (no Decision; D7 line 86 keeps the hook) | reed's own steps unzoom a zoomed window: the `window-resized` pin hook's `resize-pane -y`, every `select-layout` apply (boot, add, resume, watchdog, attach chain) and `split-window`/`kill-pane`/`select-pane` without `-Z`; so a resize, attach or spawn drops the strand view, and no Decision states what these steps do to a zoom. |
| K4 | MEDIUM | 1 | D12, lines 135-136 | D12's claim that concurrent booters create each session only after its predecessor fails for a booter that finds the server already up: it skips the dead-server-only step and creates its session at once, racing the first booter; Open risks names only a hand-started server. |
| K5 | MEDIUM | 2 | D12, line 132 | D12 runs the nested ups "outside the booting worktree's op lock" while its trigger is evaluated inside `Up`'s `withOpLock` (`ensureServerAndSessionLocked`); the record never states where the lock is released, re-taken and what is re-checked. |
| K6 | MEDIUM | 2 | D12, lines 133 and 135 | The teller hands `reedengine` only an ordered list, which is not enough to run another worktree's `up` (its own `reed.yaml`, state, geometry) or to test whether its state records a not-live session, without deriving a path (`PATTERN-told-geometry`); the record does not say what each entry carries or which side decides. |
| K7 | MEDIUM | 2 | D7, lines 86-87 (omission) | The `window-resized` fixed-height pins (`resize-pane -y`, Selvage band and every collapsed strip, so the driver whenever there are two or more strands) size pane content, which under a top border title is one row less than the top cell; unadjusted, the top cell grows by one row on every resize, and D7 says nothing about pins. |
| K8 | MEDIUM | 5 | D1, line 33; D7, line 86 | A zoomed pane reports `pane_top` 0 and full height, so the `pane_top`-sorted pane order (`paneIDsByTop`) that every layout apply and `AttachArgv`'s pre-attach planning use maps planned heights to the wrong panes when the window is zoomed; the record does not say the pane list is read unzoomed. |
| K9 | MEDIUM | 1 | D15, lines 172-173 (criterion line 248) | Inserting the block "before its closing `]`" forces a comma after the existing last entry, a byte outside the markers, so the byte-identical criterion cannot hold; the record gives no JSONC-aware rule (comments, `]` in strings) for finding the array bounds. |
| K10 | MEDIUM | 1 | "Added after Discussion" decision, lines 187-189, against D13 line 140, Scope line 18, criterion line 246 | The config-sourced colors decision is appended without an id and is not folded in: D13 still hard-codes segment-to-color constants in Go, D14 and Scope ignore the config, and the criterion requires D13's table rather than the configured value. |
| K11 | MEDIUM | 1 | line 189; D14, lines 161-163 | Config color values flow into tmux formats (`@strand_color`) and a command typed into agent panes with no stated validation against a closed palette or outcome for a bad value, and no Decision names who reads reed's config and tells the color to shuttle and the orch watcher. |
| K12 | MEDIUM | 1 | criterion line 239, against D2 lines 38-39 and D14 line 161 | The criterion (and D2's "a strand spawned by an older lyx gets them too") requires `@strand_color` on a pane spawned before this change after an attach, but the color comes from a segment persisted only by the new add path, so an old record has none. |
| K13 | LOW | 1 | D13, line 140; Constraints line 201 | D13 places Claude Code's `/color` palette names in a shared leaf package while the constraint and `PATTERN-shuttle-provider-seam` say Claude color names live only under `claudeengine`; the two cannot both hold. |
| K14 | LOW | 2 | D12, lines 135-136; criterion line 245; Open risks line 229 | D12 states the spawn-order guarantee without its stop: a predecessor whose up fails is skipped and later comes back out of order, which neither the Bound nor Open risks names. |
| K15 | LOW | 3 | D7, line 86, against D6 line 80 | The title-row correction in D7 is applied on the intended `pane-border-status top`, not on the window's effective readback, though D6 makes that pin non-fatal (psmux may refuse it, an inherited value may be `bottom`); with no top title row the correction shortens the wrong cell. |
| K16 | LOW | 3 | D7, line 86 | A planned one-row top cell (possible via `collapsed_rows: 1` or `clampToFit` on a short window) has zero content rows under the top border title, both after a layout apply and as a pin (`-y 0`); the record states no floor. |
| K17 | LOW | 3 | D8, line 96; D11, line 125 | The `M-Left`/`M-Right` `run-shell` string is not bounded against its two expansion layers: runtime formats single-quoted in the string (`'#{socket_path}'`, `'#{client_name}'`) break on a quote, and tmux format-expands the whole command at key press, so a `#` (or `#(…)`) in a pinned `lyx`/tmux path is rewritten despite `internal/shell` quoting. |
| K18 | LOW | 1 | D15, lines 172-173 | D15's marker rules leave open a begin without an end, an end without a begin or a repeated marker, a marker outside the top-level array, and the inserted lines' line ending (CRLF files) and terminating newline after `// lyx:end`. |
| K19 | LOW | 2 | line 189; D13 line 153; D14 line 163 | The record does not say what color a persisted segment gets when the current config has no key for it, nor through which surface the orch watcher obtains the orch strand's resolved color. |
| K20 | NIT | 4 | D15, line 175 | A `keybindings.json` that exists but cannot be read is neither a listed skip nor stated to fail `lyx ide spawn`. |
| K21 | NIT | 1 | Auto-mode line 216; support-log line 62 | The Auto-mode section says no operator answered and every decision was the agent's, but the config-colors decision is operator-sourced through the parent, and the support log records that change nowhere. |
| K22 | NIT | 4 | Constraints line 195; D12 line 133 | The teller tells `reedengine` start times (later a spawn-order key) alongside an already-ordered list, data no `reedengine` step reads, inviting a sort in `reedengine`. |

## Real but fix-introduced (not in the key)

- X1 (MEDIUM, round 1 fix; R2 F2): refusing an out-of-palette color in `LoadConfig` would block `down`, `status`, `attach`, pair teardown and the watchdog; reed validates `mouse`/`watchdog` in `ensureServerAndSessionLocked`.
- X2 (LOW, round 1 fix; R2 F5): temp-file-and-rename replaces a read-only file in a writable directory and drops its mode and owner, against the "left untouched" promise.
- X3 (LOW, round 2 fix; R3 F1): "the pane at window row 0" is undefined against `FixedHeightPins`, which takes no `paneOrder`, so after a reorder the adjustment lands on the wrong pane.
- X4 (NIT, round 2 fix; R3 F5): the D12 ordering stop is stated differently in the Bound, Open risks and criteria, and one Open-risks line fuses two bullets.
- X5 (LOW, rounds 1-2 fixes; R3 F7): D15's atomic-write text, skip list, Bound and criterion disagree on reporting a failed write, the order of the read-only and unchanged checks, and the temporary file.
- X6 (LOW, round 4 fix; R5 F3): the zoom bracket and the post-apply pin run are described as Go steps, but the attach chain and each `window-resized` hook entry run inside tmux after reed returns and need run-time conditional commands.
- X7 (LOW, round 2 fix; R5 F5): the booter's lock-sequence re-run "from the start" meets the revival trigger again, since only revive functions carry the skip.
- X8 (NIT, round 1 fix; R5 F7): Scope still limits recreation to the server-spawning boot after D12 was widened to every revival.

## Totals

- Findings: 39 (round 1: 9, round 2: 7, round 3: 7, round 4: 8, round 5: 8).
- Grades: real 35, wrong 0, noise 4 (R4 F6, R4 F8, R5 F6, R5 F8).
- `in-r1`: yes 17, partial 10, no 12.
- Re-rated findings: BLOCKING 3, MEDIUM 10, LOW 16, NIT 10.
- Distinct real defects: 30 (key 22, fix-introduced 8).
- Key: 22 items: BLOCKING 3 (K1-K3), MEDIUM 9 (K4-K12), LOW 7 (K13-K19), NIT 3 (K20-K22).
- Key items by first round: round 1: 10 (K1, K2, K4, K9, K10, K11, K12, K13, K18, K21; K18 partly, via R1 F4's unmatched begin), round 2: 5 (K5, K6, K7, K14, K19), round 3: 3 (K15, K16, K17), round 4: 3 (K3, K20, K22), round 5: 1 (K8).
- Late-found key items (first raised in round 3 or later): 7, of them K3 (BLOCKING) and K8 (MEDIUM).
