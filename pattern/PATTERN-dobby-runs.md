# PATTERN-dobby-runs

This cluster guards how a board task's run is started or held: only its parent's dobby, started with its orch; front tasks only via `fronttask`; an adopt hold lifts only on a green adopt check.

## Dobby starts runs

Rule: Starting or driving a board task's run: its parent's dobby, from an empty status, in batten passes; a hand `lym batten run` is one pass.

## Dobby and orch pair

Rule: Starting a worktree's dobby: only with its orch, through `orchcli.BeamUp`, or `orchcli.BeamUpWorktree` or a `lym beam start` child from a verb that needs the dobby live; never `dobbyengine.EnsureLive` or `SpawnDetached` alone.

## Front task

Rule: Only fronttask files a front task and writes `front` and `on_land`; while one is open (empty or run status) dobby starts no other task in its worktree, and runs its `on_land` action from fronttask's closed table, on the subject's owner only; its project, kind and slug are fixed.

## Adopt hold

Rule: On a takeover, `lym adopt` holds it, and each unstarted released task of the subject bar a front task, with a `pauserecord` hold-caller entry; only the green adopt check of the subject lifts it, never dobby; by hand, `lym tender resume` or `lym batten cancel`.
