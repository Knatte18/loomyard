All work complete.

Finding 1 [BLOCKING] Resume-after-crash: added reconcileLocked in Resume() after ensureServerAndSessionLocked, before planResumeLaunches, clearing stale PaneID bindings so planLaunch adopts the new-session pane.

Finding 2 [NIT] status side effect: replaced reconcileApplyPersistLocked in Status() with listPanes + reconcileLocked (no apply/persist tail).

Self-fix: reworded server.go doc comment that tripped hubgeometry enforcement scan (no behavior change).

Commits pushed to internal-mux: 784f00a, 8617bde. All batch verify commands + go build ./... pass.

{"status":"success","commit_sha":"8617bde","session_id":"f4bf81b2-f634-47ae-aa92-63e91cae666a"}
