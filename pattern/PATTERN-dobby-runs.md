# PATTERN-dobby-runs

This cluster guards how a board task's run is started or held: only its parent's dobby, started with its orch bar `lym adopt --no-orch`; front tasks only via `fronttask`; an adopt pause lifts only by hand.

## Dobby starts runs

Rule: Starting or driving a board task's run: its parent's dobby, from an empty status, in batten passes; a hand `lym batten run` is one pass.

## Dobby and orch pair

Rule: Starting a worktree's dobby: only with its orch, through `orchcli.BeamUp`, or `orchcli.BeamUpWorktree` or a `lym beam start` child from a verb that needs the dobby live; never `dobbyengine.EnsureLive` or `SpawnDetached` alone.

The one exception is the adopt `beam` step under `--no-orch`, which starts the dobby alone through `orchcli.BeamUp` with `SkipOrch`.

## Front task

Rule: Only fronttask files a front task and writes `front` and `on_land`; while one is open (empty or run status) dobby starts no other task in its worktree, and runs its `on_land` action from fronttask's closed table, on the subject's owner only; its project, kind and slug are fixed.

## Adopt pause

Rule: On a takeover, the adopt `pause` step pauses each adopted run, and each unstarted released task of the subject bar a front task, with a `pauserecord.AdoptCaller` entry; only `lym tender resume` or `lym batten cancel` lifts it, never the adopt check, the dobby's owner lift or a hibernate wake.

A legacy `adopt-hold` entry reads the same.
