| Refusal or recovery path | Named next step | Result |
|---|---|---|
| Card 1: invalid scope, forbidden `card` scope, missing/empty gate or undeclared tier | Edit `gate.yaml` through `lyx config gate` | Progresses. |
| Card 1: retired list-shaped gate | `lyx fabric reconcile`, alternatively `lyx config reconcile`, from prime | Fabric reconcile progresses; the bare config-reconcile alternative does not migrate the hub file. |
| Card 2: incompatible flags | Drop the conflicting flag | Progresses. |
| Card 2: `card` scope without card IDs | Supply `--card <id>` | Progresses. |
| Card 2: unknown key | `lyx config gate` | Progresses. |
| Cards 4–5: non-string `gate_key` | Set a string key or remove the override | Progresses. |
| Card 6: retirement of tree-scope command guards | Migration retains or clears trees according to the old value | Recovery remains available through config editing. |
| Card 7: unreadable cache, lock/read/write failure | Warn and run normally | Progresses without relying on the cache. |
| Card 10: malformed date | Supply `YYYY-MM-DD` | Progresses. |
| Card 12: prohibited raw tier invocation | Existing gate steer | Progresses; no new recovery spelling. |
| Card 14: guard in a forbidden tier | Move the guard to tier0/tier1 | Progresses for an eligible guard. |
| Card 14: undeclared guard tier or missing command placeholders | `lyx config gate` | Progresses. |
| Card 15: command omits its tier tag | Correct the tier through `lyx config gate` | Progresses. |
| Card 17: gate evaluation error | Correct settings and resume; row remains unentered | Progresses. |
| Card 17: excessive skip chain | Correct settings or use shed goto, then resume | The settings repair is available; goto is the deliberate bypass. |
| Card 19: malformed check or unknown operand | Correct the check, module/key, or segment | Progresses. |
| Card 19: nil lookup seams | Correct caller wiring | Explicit wiring guard, exempt from reaching-test requirements. |
| Card 19: non-bool setting | Inspect `--resolved`, then edit from prime for a hub/default layer | Does not repair a per-worktree module’s board-default value without `--layer default`. |
| Card 20: retired `switch` key | Move it to `entry_gate` | Progresses. |
| Card 20: missing/non-string `command` after switch retirement | Existing recovery still says to set both `switch` and `command` | Following it introduces the newly forbidden key. |
| Card 24: missing tier tags or retained `covers` | Orch updates hub config from prime and reruns the card | Progresses. |

Findings:

1. **BLOCKING — consistency — `_lyx/plan/19-recipe-entry-gate.md:43`**  
   **The non-bool-setting recovery edits the wrong layer for a board-default value.**  
   The plan explicitly sends a default-layer error to `lyx config <module>` from prime. For a per-worktree module such as `batten`, that command selects the prime’s own worktree file, leaving the board default consumed by the task unchanged. Resuming therefore returns to the same evaluation error. The recovery needs to distinguish a hub-wide module from a per-worktree module’s default layer, using `--layer default` for the latter.  
   Evidence: `internal/configcli/configcli.go:42` documents the default selection; `internal/configcli/configcli.go:126–155` dispatches default-layer editing separately from worktree editing; `internal/configreg/configreg.go:120` registers `batten` without `HubWide`; `internal/configreg/configreg.go:121–124` explicitly marks hub-wide modules. This conflicts with `PATTERN-refusal-way-forward`.

2. **BLOCKING — consistency — `_lyx/plan/20-batten-switch-entry-gate.md:30`**  
   **Switch retirement leaves an existing recovery instruction telling users to add the forbidden `switch` key.**  
   Card 20 updates `commandKeyWayForward`, but leaves `commandRefWayForward` unchanged. A Command row with missing `command` or `command: 3` passes the new unknown-key check, reaches `commandSettingRef`, and is told to “set switch and command.” Following that instruction reaches card 20’s new unknown-`switch` refusal. The existing missing/non-string-command cases also pin the obsolete recovery text.  
   Evidence: `internal/shedrecipe/entries_command.go:19`, `:58–67`; `internal/shedrecipe/entries_command_test.go:60`, `:69–70`; card 20’s retirement at `_lyx/plan/20-batten-switch-entry-gate.md:30` and its specified test change at `:39`. This conflicts with `PATTERN-refusal-way-forward`.

3. **MEDIUM — consistency — `_lyx/plan/01-gate-key-shape.md:53`**  
   **The alternative `lyx config reconcile` command cannot perform the promised hub-file migration.**  
   Bare `lyx config reconcile` defaults to a dry run over the worktree’s own/local files. It neither applies changes nor selects the hub’s board-default files. The primary `lyx fabric reconcile` option is valid, but the alternative needs `--layer default --apply` to repair the stated hub-file refusal.  
   Evidence: `internal/configcli/configcli.go:525–563` separates worktree reconciliation from `--layer default`; `:729–730` defaults `--apply` to false and the layer to empty; `internal/configcli/hubwide.go:171–194` leaves a default-layer dry run unwritten.

Tool-call manifest:

1. Read the refusal contract, Command recovery constant/call sites, and recovery-test references; the inherited context lacked the exact recovery contract and test assertions.
2. Located reconcile flag defaults and Command rejection cases; the inherited context did not establish whether the proposed reconcile command writes the relevant configuration.
3. Read reconcile dispatch/default-layer handling, fabric reconcile behavior, and setting-reference recovery; these recovery entry points were absent from the pre-read.
4. Read denied-recovery background and Command refusal tests, and located plan references; this established permitted recovery commands and exact stale assertions.
5. Read config editor dispatch and resolved-output handling; the inherited context did not establish which layer the proposed editor command selects.
6. Read module registration and setting-lookup tests; this confirmed that `batten` is a per-worktree module and that board-default values are distinct from prime-worktree values.
