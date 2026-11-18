# PATTERN-dobby-runs

This cluster guards how a board task's run is started or held: only its parent's dobby, started with its orch bar `lym adopt --no-orch`; front tasks only via `fronttask`; an adopt holds the hub through Bolt's holds until an adopt's check is green.

## Dobby starts runs

Rule: Starting or driving a board task's run: its parent's dobby, from an empty status, in batten passes; a hand `lym batten run` is one pass.

## Dobby and orch pair

Rule: Starting a worktree's dobby: only with its orch, through `orchcli.BeamUp`, or `orchcli.BeamUpWorktree` or a `lym beam start` child from a verb that needs the dobby live; never `dobbyengine.EnsureLive` or `SpawnDetached` alone.

The one exception is the adopt's last step under `--no-orch`, the `beam` step, which starts the dobby alone through `orchcli.BeamUp` with `SkipOrch`.

## Front task

Rule: Only fronttask files a front task and writes `front` and `on_land`; while one is open (empty or run status) dobby starts no other task in its worktree, and runs its `on_land` action from fronttask's closed table, on the subject's owner only; its project, kind and slug are fixed.

## Adopt hold

Rule: An adopt holds the hub through Bolt's holds, written only by `boardengine`'s setters, from its owner write until an adopt's check is green.

A green check clears the hold and drops the bolt-hold entries with no hand lift.
A failing check keeps the hold under an adopt-fix hold, and the fix task's landing checks it again.
A plain adopt never moves another machine's hold, and `lym adopt --take-hold`, which does, is denied to agents.
