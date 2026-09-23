Batch webster-startup-window-docs (02-webster-startup-window-docs.md) complete: 3 of 3 cards committed and verified.

Card 6 — docs(webster): state the startup window the state-mutation lease now spans (commit 36aa6aa71)
- internal/websterengine/state.go: AcquireStateMutation's contract now states the spawn's startup window (bounded by startup_timeout_s) as part of the load-mutate-save sequence at the two spawn sites, with unbounded/poll-length waits still excluded.
- internal/websterengine/recoverbatch.go: file header and RecoverSpawnOrAttach doc comment reworded, with the three accepted residuals (killed mid-startup, startup mechanism failure, failed not-ready teardown) stated.
- internal/websterengine/runlevel.go: MasterHandle doc comment gets the same three residuals; the StartMaster call site gets a new comment noting the lease now spans the startup window.
- internal/websterengine/strand.go: Starter's doc comment rewritten to state Start blocks until past startup gates.
- internal/websterengine/doc.go: cold-recovery section rewords the old "blocks at most poll_wait_s" claim.
- internal/websterengine/awaitbatch.go: parenthetical corrected to attribute "each call blocks at most one wait window" to recover-batch's re-polls specifically.
- internal/webstercli/recoverbatch.go: file header and Long help text reworded to name the spawning call's extra startup wait.

Card 7 — docs(webster): Master stencil names the spawning recover-batch call's startup wait (commit d1cad65e7)
- contracts/stencils/webster/webster-template-master.md: both places promising "each call blocks at most poll_wait_s seconds" reworded; no new template variable introduced, {{.poll_wait_s}} marker preserved for template_test.go.

Card 8 — test(websterengine): a not-ready recovery start surfaces and records no batch state (commit a96168438)
- internal/websterengine/recoverbatch_test.go: added erroringStarter double and TestRecoverSpawnOrAttach_NotReadyStartSurfacesAndRecordsNothing, proving a Starter error wrapping shuttleengine.ErrNotStarted surfaces unchanged from RecoverSpawnOrAttach with spawned=false, nil BatchState, and no entry recorded in State.Batches[1].

Verify (all three sub-invocations passed):
- go test ./internal/websterengine/ ./internal/webstercli/ — ok
- go test -tags integration ./internal/websterengine/ ./internal/webstercli/ — ok
- go test -tags smoke -run '^$' ./internal/webstercli/ — ok (compiled, 0 tests run, per Shared Decision)

Working tree is clean (git status --porcelain --untracked-files=no returned nothing). All 3 of 3 declared cards committed — no partial completion.

{"status":"success","commit_sha":"a96168438a4ea6ba06176c152fdca58704c915fa","session_id":"cd7e2a43-b5e8-4a96-89f2-853d39fee131","cards_done":[6,7,8]}
