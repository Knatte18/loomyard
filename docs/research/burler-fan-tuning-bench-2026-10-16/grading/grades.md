# Grades: reed-view Discussion-Review round 1, iteration 1

Graded against `key/originals/round1-target-decision-record.md` and `round1-target-support-log.md`, the board entry, the Discussion-Review rubric, `key/key.md` and the code at `fd5b0f58e`.
Severity standard is the key's: BLOCKING = a premise is false or a Decision's purpose fails in the common case; MEDIUM = the plan writer would build wrong or cannot settle it from the record; LOW = a real gap with a bounded or rare effect a plan writer could resolve sensibly; NIT = wording or bookkeeping.
A finding that states more than one defect lists every id it states; `—` marks noise.

Code checks behind the new ids:

- No `status-style` pin exists under `internal/reedengine` and the server is started without `-f`, so tmux's default green `status-style` applies (N2).
- `lock.AcquireWriteLock` (`internal/lock/lock.go:27`) blocks with no timeout; `withTryOpLock`'s comment records an 11 s wait behind a held lock (N6).
- `probeCapability` refuses the boot on any missing `requiredSubcommands` entry (`internal/reedengine/probe.go:73-78`), and `set-hook`/`run-shell` are kept out of it on purpose for psmux (`windowsize.go:232`, `doc.go:660`) (N7).
- `orchSpec` (`internal/orchcli/start.go:63-78`) launches the orch through `shuttleengine.Spec` with `Skills: orchSkills`; the watcher (`internal/orchengine/watcher.go`) has reload sequences (`startReload`, `startAutoReload`) and no start sequence that types (N8).
- `reed` is not `HubWide` in `configreg.Modules()` (`internal/configreg/configreg.go:59`), while `PATTERN-hub-wide-config` requires a hub-level fact to live in a `HubWide` module (N4).
- `reedcli`'s `PersistentPreRunE` skips resolution only for `reed` and `watchdog` (`internal/reedcli/cli.go:110`): true, but the record already states the requirement and the watchdog is the precedent (R-r F11 graded noise).
- `strandWindowTarget` falls back to the current window only when no recorded Selvage or strand pane is in the session's listing (`windowtarget.go:20-36`), the same seam every reed op uses (way-forward 7 graded noise).

## R-p

| id | reviewer severity | grade | re-rated | K/N id | one-line reason |
|---|---|---|---|---|---|
| F1 | BLOCKING | real | MEDIUM | K10, K11, K13, N4 | Config-colors entry unreconciled with D13/D14/criterion 246 (K10); no reader named for shuttle (K11, reader half only); leaf package holds Claude names against the seam constraint (K13); per-worktree vs `HubWide` unstated (N4). |
| F2 | MEDIUM | real | MEDIUM | K4 | A booter that finds the server already up skips the ordered step and creates its session mid-sequence; the rationale's claim fails. |
| F3 | MEDIUM | real | MEDIUM | K9 | No JSONC-aware bracket scan and no comma rule; a comma outside the markers breaks byte-identity, and a replaced block drops a comma VS Code added inside it. |
| F4 | MEDIUM | real | LOW | N1 | A missing file "created holding only the block" is not a JSON array; a plan writer would likely wrap it, but the record says otherwise. |
| F5 | MEDIUM | real | LOW | N2 | D6 drops the board's `status-style bg=colour234,fg=white`; reed pins none today, so the bar sits on tmux's green default; display only. |
| F6 | MEDIUM | real | MEDIUM | K12 | Criterion 239 demands `@strand_color` on a pre-change pane whose record carries no segment. |
| F7 | MEDIUM | real | LOW | K15 | D7 subtracts the border row on intent, not on a readback, while D6 makes that pin non-fatal; the subtraction itself is wrong too (K1, not raised here). |
| F8 | LOW | real | LOW | N3 | Pre-change sessions reached by switching keep `status on`, no `@lyx_strands` and no `@strand` under the new global formats until their own attach. |
| F9 | NIT | noise | NIT | — | D8's "no other key" plainly means keyboard keys and D9 states its own mouse bound; no plan writer misreads it. |

## R-q

| id | reviewer severity | grade | re-rated | K/N id | one-line reason |
|---|---|---|---|---|---|
| F1 | BLOCKING | real | BLOCKING | K2 | "Only then does it bring up its own session" puts the booter last, breaking spawn order whenever it is not the newest; criterion 245 encodes it. |
| F2 | BLOCKING | real | MEDIUM | K4 | Same late-booter race as K4; bounded to concurrent boots after a server kill, so MEDIUM as in the key. |
| F3 | LOW | real | LOW | K2 | The nested ordinary up meets the dead-server trigger itself; the recursion half of K2, resolvable by telling inner ups no list. |
| F4 | BLOCKING | real | MEDIUM | K10, K11, N4 | D13's Go table contradicts the config entry (K10); no reader for shuttle and the orch watcher (K11, reader half); `HubWide` unstated (N4). |
| F5 | MEDIUM | real | LOW | K13 | Claude palette names in the leaf package against the line-201 constraint; resolvable by relabelling. |
| F6 | MEDIUM | real | MEDIUM | K12 | Pre-change pane cannot carry `@strand_color`; confirmed `launchStrandLocked` is the one launch chokepoint. |
| F7 | MEDIUM | real | MEDIUM | K9 | Comma and JSONC scan unspecified; byte-identity forces the separator inside the markers. |
| F8 | MEDIUM | real | LOW | N8 | Confirmed: shuttle launches the orch with its skills, and the watcher has no start sequence; only the reload leg needs the watcher, and the record's version still works. |
| F9 | LOW | noise | NIT | — | The hub's reed server holds reed sessions only; a foreign session there is not a case the plan needs. |
| F10 | LOW | noise | NIT | — | tmux's right-click menu still needs an explicit Kill/Respawn choice, and status-left already exposed a menu; the bound is accurate. |

## R-r

| id | reviewer severity | grade | re-rated | K/N id | one-line reason |
|---|---|---|---|---|---|
| F1 | BLOCKING | real | BLOCKING | K1 | The top border row is taken inside the top cell; a one-row-short layout leaves the last row unused (probe on tmux 3.6). |
| F2 | BLOCKING | real | MEDIUM | K10, K11, N4 | Config entry contradicts D13 and criterion 246 (K10); value domain, readers and invalid-value outcome unstated (K11, both halves); per-worktree vs `HubWide` (N4). |
| F3 | MEDIUM | real | LOW | K13 | Claude palette names outside `claudeengine`. |
| F4 | MEDIUM | real | BLOCKING | K3 | `select-layout`, the focus `select-pane` and the `window-resized` `resize-pane -y` pins unzoom, so D1's strand view collapses on every add, remove, resume or resize. |
| F5 | MEDIUM | real | MEDIUM | K4, K5, N5 | Late booter breaks the order (K4); no entry point and no position relative to the caller's own op lock (K5); a booter killed mid-list leaves the rest unordered for good (N5). |
| F6 | MEDIUM | real | LOW | N6 | Confirmed: `AcquireWriteLock` blocks with no timeout, and the Bound skips only failed ups; op locks are normally short, so the hang is rare. |
| F7 | MEDIUM | real | MEDIUM | K12 | Criterion 239 asks for a color a pre-change strand cannot have. |
| F8 | MEDIUM | real | MEDIUM | K9 | Comma rule and JSONC scan unspecified; a leading comma breaks on an empty array. |
| F9 | MEDIUM | real | LOW | N1, K18 | Created file is not an array (N1); begin-without-end, end-without-begin and two blocks unhandled (K18); the missing `Code/User` directory part is noise. |
| F10 | MEDIUM | real | LOW | N7 | Confirmed: adding `bind-key`/`run-shell` to `requiredSubcommands` would refuse every boot on a psmux lacking them, against D6's non-fatal pins and the Windows degrade. |
| F11 | MEDIUM | noise | NIT | — | D10 already states that the verb resolves nothing from a cwd; joining the existing `PersistentPreRunE` skip next to `watchdog` is implementation. |
| F12 | LOW | real | LOW | N3 | Once one worktree pins, sessions from an older boot render the new line 1 with no strand buttons and inert Alt keys until their own attach. |
| F13 | LOW | noise | NIT | — | A failed `@strand` set at launch is display-only like the pane title beside it; the plan writer follows the existing non-fatal practice. |
| F14 | LOW | noise | NIT | — | The open risk at line 230 already records that the binding pins whichever `lyx` last attached; a deleted dev binary is the same risk. |

## R-r-forks

| fork | finding | grade | re-rated | K/N id | kept in review |
|---|---|---|---|---|---|
| i1-way-forward | 1 (MEDIUM) interrupted pre-boot leaves order broken | real | LOW | N5 | yes (F5) |
| i1-way-forward | 2 (MEDIUM) held foreign op lock has no timeout | real | LOW | N6 | yes (F6) |
| i1-way-forward | 3 (MEDIUM) created file is not a JSON array | real | LOW | N1 | yes (F9) |
| i1-way-forward | 4 (MEDIUM) partial or duplicated markers unhandled | real | LOW | K18 | yes (F9) |
| i1-way-forward | 5 (MEDIUM) invalid configured color has no outcome | real | MEDIUM | K11 | yes (F2) |
| i1-way-forward | 6 (LOW) pinned binary path may vanish | noise | NIT | — | yes (F14, noise) |
| i1-way-forward | 7 (LOW) attach may mark a batten window | noise | NIT | — | no (Rejected) |
| i1-way-forward | 8 (LOW) failed `@strand` set at launch unstated | noise | NIT | — | yes (F13, noise) |
| i1-way-forward | 9 (LOW) missing `Code/User` directory unstated | noise | NIT | — | yes (in F9, noise part) |
| i1-existing-paths | 1 (BLOCKING) D7 border-row subtraction is false | real | BLOCKING | K1 | yes (F1) |
| i1-existing-paths | 2 (MEDIUM) reed's apply and resize paths unzoom | real | BLOCKING | K3 | yes (F4) |
| i1-bound-breaker | 1 (MEDIUM) late booter breaks spawn order | real | MEDIUM | K4 | yes (F5) |
| i1-bound-breaker | 2 (MEDIUM) held foreign op lock hangs the boot | real | LOW | N6 | yes (F6) |
| i1-bound-breaker | 3 (LOW) no entry point for the step relative to the op lock | real | MEDIUM | K5 | yes (F5) |
| i1-bound-breaker | 4 (MEDIUM) pre-change pane cannot get `@strand_color` | real | MEDIUM | K12 | yes (F7) |
| i1-bound-breaker | 5 (LOW) mixed-state server shows unmarked sessions | real | LOW | N3 | yes (F12) |
| i1-bound-breaker | 6 (BLOCKING) D7 border-row subtraction is false | real | BLOCKING | K1 | yes (F1) |
| i1-bound-breaker | 7 (LOW) unmarked strands read as Selvage in mixed state | real | LOW | N3 | yes (F12) |
| i1-bound-breaker | 8 (MEDIUM) JSONC comma and scan break the bound | real | MEDIUM | K9 | yes (F8) |
| i1-bound-breaker | 9 (LOW) created file not an array, directory created | real | LOW | N1 | yes (F9) |
| i1-interactions | F1 (BLOCKING) config colors contradict D13 | real | MEDIUM | K10, K11, N4 | yes (F2) |
| i1-interactions | F2 (MEDIUM) Claude palette outside the seam | real | LOW | K13 | yes (F3) |
| i1-interactions | F3 (MEDIUM) criterion 239 outruns D2/D14 | real | MEDIUM | K12 | yes (F7) |
| i1-interactions | F4 (BLOCKING) D7 border-row subtraction is false | real | BLOCKING | K1 | yes (F1) |
| i1-interactions | F5 (MEDIUM) probe constraint vs non-fatal pins | real | LOW | N7 | yes (F10) |
| i1-interactions | F6 (LOW) global formats reach unmarked sessions | real | LOW | N3 | yes (F12) |
| i1-interactions | F7 (MEDIUM) JSONC insertion and created file | real | MEDIUM | K9, N1 | yes (F8, F9) |

## Summary per review

"Distinct real ids at MEDIUM or above" counts an id by its own key or pool severity.

| review | findings | real | real B/M/L/N | wrong | noise | distinct real ids | distinct ≥ MEDIUM | key ids hit |
|---|---|---|---|---|---|---|---|---|
| R-p | 9 | 8 | 0/4/4/0 | 0 | 1 | 11 (K4, K9, K10, K11, K12, K13, K15, N1, N2, N3, N4) | 5 (K4, K9, K10, K11, K12) | K4, K9, K10, K11, K12, K13, K15 |
| R-q | 10 | 8 | 1/4/3/0 | 0 | 2 | 9 (K2, K4, K9, K10, K11, K12, K13, N4, N8) | 6 (K2, K4, K9, K10, K11, K12) | K2, K4, K9, K10, K11, K12, K13 |
| R-r | 14 | 11 | 2/4/5/0 | 0 | 3 | 16 (K1, K3, K4, K5, K9, K10, K11, K12, K13, K18, N1, N3, N4, N5, N6, N7) | 8 (K1, K3, K4, K5, K9, K10, K11, K12) | K1, K3, K4, K5, K9, K10, K11, K12, K13, K18 |

## Summary per fork (R-r-forks)

| fork | raw findings | real | distinct ids | ids no other fork raised | ids also raised by another fork |
|---|---|---|---|---|---|
| i1-way-forward | 9 | 5 | 5 (N1, N5, N6, K11, K18) | N5, K18 | N1, N6, K11 |
| i1-existing-paths | 2 | 2 | 2 (K1, K3) | K3 | K1 |
| i1-bound-breaker | 9 | 9 | 8 (K1, K4, K5, K9, K12, N1, N3, N6) | K4, K5 | K1, K9, K12, N1, N3, N6 |
| i1-interactions | 7 | 7 | 10 (K1, K9, K10, K11, K12, K13, N1, N3, N4, N7) | K10, K13, N4, N7 | K1, K9, K11, K12, N1, N3 |

## Pool

| N id | re-rated severity | description |
|---|---|---|
| N1 | LOW | D15 creates a missing `keybindings.json` "holding only the block", which has no enclosing `[` `]`, so VS Code cannot parse it and every later run only replaces the block in place, leaving the file invalid for good. |
| N2 | LOW | D6's pin list omits the board's `status-style bg=colour234,fg=white`; reed pins no `status-style` today, so the new bar sits on tmux's default green status style, where a lit green (Webster) button merges into the bar. |
| N3 | LOW | The global `status-format` lines and root bindings reach every session once one worktree pins them, but `status 2`, `@lyx_strands` and `@strand` reach a pre-change session only at its own boot or attach; until then it shows a broken bar (no run row or no strand buttons) and treats its strands as Selvage for Alt+z and Alt+Up/Down, and pair sessions are reached by switching, not attach. |
| N4 | LOW | The config-colors decision puts the segment colors in reed's config, which is per-worktree (not `HubWide` in `configreg`), although the colors are a server-wide display fact under `PATTERN-hub-wide-config`; the record does not say which, so two worktrees on one server can tint the same segment differently. |
| N5 | LOW | D12's ordered step runs only on a dead server, so a booter killed or failing after the first session has started the server leaves every remaining session to come back in arbitrary order, and no later boot resumes the list; neither the Bound nor Open risks names it. |
| N6 | LOW | D12 takes each foreign worktree's op lock through `lock.AcquireWriteLock`, which blocks with no timeout, so a long or stuck op in one worktree stalls the booter and its own session; the Bound skips only an up that fails, not one that waits. |
| N7 | LOW | The constraint that the capability probe "gains any verb the new pins need" would put `bind-key`/`run-shell` into `requiredSubcommands`, which refuses the whole boot when one is missing, contradicting D6's non-fatal pins and the Windows unbranched-degrade constraint. |
| N8 | LOW | D14 has the orch watcher type the orch's color as the first step of its start sequence because shuttle may not type into the orch, but `orchcli` already launches the orch through shuttle with its skills and the watcher has no start sequence; only the reload leg needs the watcher. |
