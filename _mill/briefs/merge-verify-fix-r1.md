# Verify-Fix Brief

The verify command `go test -tags smoke ./internal/reedcli/ -skip '^TestSmokeClaudeResumeRecallsCodeword$' && go test -tags integration ./internal/reedengine/` failed after a merge.
Your job is to diagnose the failures and fix the code so the verify command passes.

## Verify Output

```
ok  	github.com/Knatte18/loomyard/internal/reedcli	184.012s
time=2026-09-19T09:29:42.968+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-80f47933 session=worktree err=boom
time=2026-09-19T09:29:42.969+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-6e1160ee session=worktree answer=""
time=2026-09-19T09:29:42.969+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-9aa450ca session=worktree answer=""
time=2026-09-19T09:29:42.969+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-908c0e30 session=worktree answer=""
time=2026-09-19T09:29:42.969+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-2455ad04 session=worktree answer=""
time=2026-09-19T09:29:42.969+02:00 level=WARN msg="reed: failed to install window-resized hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-2455ad04 session=worktree argv="[set-hook -u -w -t =worktree: window-resized]" err=boom
time=2026-09-19T09:29:42.969+02:00 level=WARN msg="reed: failed to install window-resized hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-2455ad04 session=worktree argv="[set-hook -w -t =worktree: window-resized run-shell -b \": > '/tmp/TestApplyLayoutLocked_SetHookErrorDoesNotFailApply1539580504/001/worktree/anchor/.lyx/reed-resize.signal'\"]" err=boom
time=2026-09-19T09:29:42.969+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-cdd2cd40 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.970+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-19be055b session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.970+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-8d976172 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.970+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-4d861321 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.970+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-bfe4e749 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-fc8c1bab session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-06fab94f session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-06fab94f session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-f87d6b0c session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-f87d6b0c session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-78cee560 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-78cee560 session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-6be99c7e session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: failed to read back window-size option" trace=8d5c66ec4abe7aea socket=lyx-001-6be99c7e session=worktree err=boom
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-6be99c7e session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-23e63093 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.971+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-23e63093 session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-01c25d6c session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: failed to read back status option" trace=8d5c66ec4abe7aea socket=lyx-001-01c25d6c session=worktree err=boom
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-01c25d6c session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-e9308092 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-91afdd56 session=worktree cols=0 rows=24
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-2e549753 session=worktree cols=-1 rows=24
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-02a1af35 session=worktree cols=80 rows=0
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-82955a47 session=worktree cols=80 rows=-1
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-6d2b42d0 session=worktree err="check session: boom"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-3e00cccb session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-3e00cccb session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-cad242f9 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-cad242f9 session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-2a6158bf session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.972+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-2a6158bf session=worktree err=boom
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-49514025 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-49514025 session=worktree err="render: strand a uses deferred anchor \"own-window\""
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-fa4dab26 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-e3e128d8 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-9fb893ee session=worktree cols=0 rows=24
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-22694a6e session=worktree err="check session: boom"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-1ca78097 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-1ca78097 session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-bdc4ed38 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-bdc4ed38 session=worktree err="attach chain suppressed"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-aff22de1 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-aff22de1 session=worktree err="render: strand a uses deferred anchor \"own-window\""
time=2026-09-19T09:29:42.973+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-0cf66a06 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.974+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-0cf66a06 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:42.974+02:00 level=WARN msg="reed: failed to install window-resized hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-0cf66a06 session=worktree argv="[set-hook -u -w -t =worktree: window-resized]" err=boom
time=2026-09-19T09:29:42.974+02:00 level=WARN msg="reed: failed to install window-resized hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-0cf66a06 session=worktree argv="[set-hook -w -t =worktree: window-resized run-shell -b \": > '/tmp/TestAttachArgv_SetHookErrorDoesNotChangeTheChainedArgv3937645468/001/worktree/anchor/.lyx/reed-resize.signal'\"]" err=boom
time=2026-09-19T09:29:43.621+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-79e5b5eb session=worktree cols=0 rows=0
time=2026-09-19T09:29:44.692+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=8d5c66ec4abe7aea socket=lyx-001-86a51df7 session=worktree cols=0 rows=0
--- FAIL: TestEnsureSession_BootedTrueOnColdSessionFalseOnWarm (0.02s)
    ensuresession_integration_test.go:77: EnsureSession (cold) = stencil: unfilled top-level marker(s): worktree, want a nil error
--- FAIL: TestAddStrand_LogsAttributionOnlyOnColdBoot (0.02s)
    ensuresession_integration_test.go:103: AddStrand (cold) = stencil: unfilled top-level marker(s): worktree, want a nil error
time=2026-09-19T09:29:46.283+02:00 level=WARN msg="reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them" trace=8d5c66ec4abe7aea socket=lyx-001-5024ff57 session=worktree recordedSession=worktree recordedTmuxSession=$0 recordedServerPID=100 liveTmuxSession=$4 liveServerPID=900
time=2026-09-19T09:29:46.283+02:00 level=WARN msg="reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them" trace=8d5c66ec4abe7aea socket=lyx-001-9eeca3a0 session=worktree recordedSession=svc-orig recordedTmuxSession=$0 recordedServerPID=100 liveTmuxSession=$4 liveServerPID=900
time=2026-09-19T09:29:46.283+02:00 level=WARN msg="reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them" trace=8d5c66ec4abe7aea socket=lyx-001-505928ff session=worktree recordedSession=svc-orig recordedTmuxSession=$0 recordedServerPID=100 liveTmuxSession=$4 liveServerPID=900
time=2026-09-19T09:29:46.283+02:00 level=WARN msg="reed: could not read the live pane generation, leaving persisted pane bindings as they are" trace=8d5c66ec4abe7aea socket=lyx-001-182ea4a0 session=worktree err="tmux answered \"|4321|\" for session \"worktree\"; one of the 3 fields is empty"
time=2026-09-19T09:29:46.285+02:00 level=WARN msg="reed: failed to split Selvage pane, retrying behind an even-vertical re-tile" trace=8d5c66ec4abe7aea socket=lyx-001-819b0a80 session=worktree err="split-window created no new pane (got \"%0\"; target %0 likely too small to split)"
time=2026-09-19T09:29:46.285+02:00 level=WARN msg="reed: Selvage split still had no room after the even-vertical re-tile" trace=8d5c66ec4abe7aea socket=lyx-001-819b0a80 session=worktree err="split-window created no new pane (got \"%0\"; target %0 likely too small to split)"
time=2026-09-19T09:29:46.285+02:00 level=WARN msg="reed: failed to split Selvage pane, retrying behind an even-vertical re-tile" trace=8d5c66ec4abe7aea socket=lyx-001-5990b1b4 session=worktree err="exit status 1: no space for new pane"
time=2026-09-19T09:29:46.285+02:00 level=WARN msg="reed: failed to split Selvage pane, retrying behind an even-vertical re-tile" trace=8d5c66ec4abe7aea socket=lyx-001-861f0981 session=worktree err="exit status 1: no space for new pane"
time=2026-09-19T09:29:46.987+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-963ce83a session=worktree err=boom
time=2026-09-19T09:29:46.987+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-a4fa8273 session=worktree err=boom
time=2026-09-19T09:29:46.992+02:00 level=WARN msg="reed: could not read the live pane generation, leaving persisted pane bindings as they are" trace=8d5c66ec4abe7aea socket=lyx-001-615708a8 session=worktree err="read pane generation for session \"worktree\": fork/exec /tmp/TestLoadOrInitStateLocked_AbsentFileInitializesFromEngineIdentit3826270607/001/does-not-exist-tmux.exe: no such file or directory"
time=2026-09-19T09:29:46.992+02:00 level=WARN msg="reed: could not read the live pane generation, leaving persisted pane bindings as they are" trace=8d5c66ec4abe7aea socket=lyx-001-17df0a36 session=worktree err="read pane generation for session \"worktree\": fork/exec /tmp/TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIde2337540826/001/does-not-exist-tmux.exe: no such file or directory"
time=2026-09-19T09:29:46.992+02:00 level=WARN msg="reed: cleared strand pane bindings that named a pane another owner already claims" trace=8d5c66ec4abe7aea socket=lyx-001-337e3914 session=worktree strands=[first]
time=2026-09-19T09:29:46.993+02:00 level=WARN msg="reed: cleared strand pane bindings that named a pane another owner already claims" trace=8d5c66ec4abe7aea socket=lyx-001-2a650562 session=worktree strands=[second]
time=2026-09-19T09:29:49.641+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:49.791+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:49.942+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:50.092+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:50.241+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:50.391+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:50.542+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:50.691+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree err="no reed session (2 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"
time=2026-09-19T09:29:50.848+02:00 level=WARN msg="reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them" trace=8d5c66ec4abe7aea socket=lyx-001-b49c10b8 session=worktree recordedSession=worktree recordedTmuxSession=$0 recordedServerPID=3646266 liveTmuxSession=$0 liveServerPID=3646524
time=2026-09-19T09:29:53.178+02:00 level=WARN msg="reed: invalid watchdog value, treating watchdog as off" trace=8d5c66ec4abe7aea socket=lyx-001-491f8a90 session=worktree value=garbage err="invalid watchdog value \"garbage\": want \"on\" or \"off\""
time=2026-09-19T09:29:53.703+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-9b828f5e session=worktree err="select-layout: select-layout boom"
time=2026-09-19T09:29:53.706+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-9b828f5e session=worktree err="select-layout: select-layout boom"
time=2026-09-19T09:29:53.710+02:00 level=WARN msg="reed: resize re-apply failed" trace=8d5c66ec4abe7aea socket=lyx-001-9b828f5e session=worktree err="select-layout: select-layout boom"
time=2026-09-19T09:29:53.710+02:00 level=WARN msg="reed: abandoning this resize event after max attempts, watcher remains running and responsive to the next signal" trace=8d5c66ec4abe7aea socket=lyx-001-9b828f5e session=worktree attempts=3
time=2026-09-19T09:29:54.049+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-351215ea session=worktree answer="abc def"
time=2026-09-19T09:29:54.049+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-172406ca session=worktree answer=""
time=2026-09-19T09:29:54.049+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-c59cc28c session=worktree answer="220 0"
time=2026-09-19T09:29:54.049+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=8d5c66ec4abe7aea socket=lyx-001-83482e28 session=worktree err=boom
time=2026-09-19T09:29:54.049+02:00 level=WARN msg="reed: failed to read back status option" trace=8d5c66ec4abe7aea socket=lyx-001-0b57b657 session=worktree err=boom
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: failed to read back window-size option" trace=8d5c66ec4abe7aea socket=lyx-001-e7d3de90 session=worktree err=boom
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-4e5fa73d session=worktree err="stencil: unfilled top-level marker(s): slug"
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: failed to pin status on" trace=8d5c66ec4abe7aea socket=lyx-001-90349448 session=worktree option=status err=boom
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-2a10424f session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-7d1ec046 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: invalid watchdog value, treating the watchdog as off" trace=8d5c66ec4abe7aea socket=lyx-001-7d1ec046 session=worktree watchdog=bogus err="invalid watchdog value \"bogus\": want \"on\" or \"off\""
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: failed to unset window-resized hook" trace=8d5c66ec4abe7aea socket=lyx-001-a9ebcb76 session=worktree err=boom
time=2026-09-19T09:29:54.050+02:00 level=WARN msg="reed: failed to render status-line text, skipping status-left and status-left-length" trace=8d5c66ec4abe7aea socket=lyx-001-ee31c227 session=worktree err="stencil: unfilled top-level marker(s): worktree"
time=2026-09-19T09:29:54.051+02:00 level=WARN msg="reed: invalid watchdog value, installing no resize-signal hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-c0b9b6eb session=worktree watchdog=bogus err="invalid watchdog value \"bogus\": want \"on\" or \"off\""
time=2026-09-19T09:29:54.051+02:00 level=WARN msg="reed: invalid watchdog value, installing no resize-signal hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-79b0ee67 session=worktree watchdog="" err="invalid watchdog value \"\": want \"on\" or \"off\""
time=2026-09-19T09:29:54.051+02:00 level=WARN msg="reed: failed to install window-resized hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-bd93c645 session=worktree argv="[set-hook -u -w -t =worktree: window-resized]" err=boom
time=2026-09-19T09:29:54.051+02:00 level=WARN msg="reed: failed to install window-resized hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-bd93c645 session=worktree argv="[set-hook -w -t =worktree: window-resized resize-pane -t %1 -y 3]" err=boom
time=2026-09-19T09:29:54.051+02:00 level=WARN msg="reed: failed to install window-resized hook entry" trace=8d5c66ec4abe7aea socket=lyx-001-bd93c645 session=worktree argv="[set-hook -a -w -t =worktree: window-resized run-shell -b \": > '/tmp/TestInstallResizePinsLocked_IssuesTheSignalEntryLastSignalEntry3719898562/001/worktree/anchor/.lyx/reed-resize.signal'\"]" err=boom
FAIL
FAIL	github.com/Knatte18/loomyard/internal/reedengine	11.089s
FAIL
```

## Merge Diff

```diff
diff --git a/.gitattributes b/.gitattributes
index e03db1feb..de83326da 100644
--- a/.gitattributes
+++ b/.gitattributes
@@ -26,7 +26,7 @@ internal/boardengine/template.yaml text eol=lf
 internal/modelspec/template.yaml text eol=lf
 internal/reedengine/template_posix.yaml text eol=lf
 internal/reedengine/template_windows.yaml text eol=lf
-internal/reedengine/console-header.md text eol=lf
+internal/reedengine/status-line.md text eol=lf
 internal/shuttleengine/template.yaml text eol=lf
 internal/fabricengine/post-checkout.sh text eol=lf
 plugins/prowler/scripts/*.sh text eol=lf
diff --git a/CONSTRAINTS.md b/CONSTRAINTS.md
index b1ff78ceb..023e4c997 100644
--- a/CONSTRAINTS.md
+++ b/CONSTRAINTS.md
@@ -21,7 +21,7 @@ An engine is handed the absolute paths it operates on and derives none of its ow
 - Three tiers: `lyxcwd.Resolve` → `preflight.Check` (fabric wired/synced/clean) → `loomengine.CheckSeed`.
 - A producer needs none of the tiers; an orchestrator needs tier 3; a standalone CLI probes tier 1 via `preflight.ResolveMode` only.
 - `internal/hubgeom`/`internal/standalonegeom` are the only `Geometry`-struct constructors.
-- Bound packages: `internal/tokenvocab`, `pattern`, `buildinfo`, `standalonestate`, `shedengine`, `treadleengine`, `loomshed`, `landingshed`, `mergeresolve`, `shedrecipe`, `shedbuild`, `loomrecipe`, `planparser`, `planglyph`, `configengine`, `shuttleengine`, `reedengine`, `burlerengine`, `websterengine`, `cliwire`.
+- Bound packages: `internal/tokenvocab`, `pattern`, `buildinfo`, `standalonestate`, `shedengine`, `treadleengine`, `loomshed`, `landingshed`, `mergeresolve`, `shedrecipe`, `shedbuild`, `loomrecipe`, `planparser`, `planglyph`, `configengine`, `shuttleengine`, `reedengine`, `burlerengine`, `websterengine`, `cliwire`, `lifecycleshed`, `lifecyclerecipe`.
 - A `shuttleengine` runner whose anchor is deliberately outside its worktree root is constructed only through `shuttleengine.NewDetachedRunner`, only from a standalone CLI's own wiring, and `NewRunner`'s containment assertion is never relaxed to accommodate it.
 
 ## Cliwire Sole-Wiring Invariant
@@ -137,12 +137,12 @@ Every producer prompt and every deployed normative spec is read at call time fro
 
 Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.
 
-- Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`; eleven of twelve also carry `RunCLIIn(cwd, out, args) int`.
+- Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`; twelve of thirteen also carry `RunCLIIn(cwd, out, args) int`.
 - An alias command may delegate into another module's subtree with no seam function of its own.
 - Non-empty `Short` on every command.
 - Errors are JSON via `internal/output`, one object per line; every `RunE` checks `clihelp.ShouldAbort` first.
-- Interactive-handoff exception, narrow and per-command: `reedengine` `attach`/`header --blocking`, `lyx loom status --watch`, `lyx loom run`/`lyx run`.
-- Package naming: `<module>cli` imports `<module>engine`; engine never imports cli/cobra. Deviations: `stencilcli` → `internal/stencilstore`; `quarrycli` → `internal/planglyph`.
+- Interactive-handoff exception, narrow and per-command: `reedengine` `attach`/`watchdog`, `lyx loom status --watch`, `lyx loom run`/`lyx run`.
+- Package naming: `<module>cli` imports `<module>engine`; engine never imports cli/cobra. Deviations: `stencilcli` → `internal/stencilstore`; `quarrycli` → `internal/planglyph`; `lifecyclecli` → `internal/lifecycleshed`, `internal/lifecyclerecipe` (no engine package of its own).
 
 ## Completion Signal Invariant
 
@@ -196,6 +196,15 @@ Every git op LYX's own code performs, on either weft or warp, goes through `inte
 
 A `fabricengine` write to a hub-level structural container (`_launchers/…`, `_portals/…`) routes through an `os.Root` rooted at the hub — never a raw `os.MkdirAll`/`os.WriteFile`/`fslink`.
 
+## Lifecycle Bookend Invariant
+
+A producer that creates or destroys a task worktree never runs from inside that worktree.
+
+- The lifecycle Shed is driven from the hub's prime worktree, and its status file and locks live under prime's own ephemeral tree, never under the worktree being managed.
+- A teardown row sequences session shutdown before worktree removal, in one producer, never two rows.
+- Enforcement is review discipline with two partial mechanical proxies, not an enforcing test: the invariant constrains which directory a running process is driven from, which has no static shape an AST scan can see.
+  `internal/lifecycleshed`'s seam-enforcement scan bars a direct resolver import so the package cannot resolve its way into the managed worktree, and `internal/lifecyclecli`'s path-derivation tests pin the status and lock paths to prime's anchor so a relocation under the managed worktree fails there — neither proves the driver's own working directory, which stays a review obligation.
+
 ## Mutation Record Invariant
 
 Every mutating fabric verb accumulates a `*Mutations` record; every mutating result type exposes it under a fixed envelope key set.
diff --git a/cmd/lyx/destructiveguard_test.go b/cmd/lyx/destructiveguard_test.go
index 11554bea5..a6410771c 100644
--- a/cmd/lyx/destructiveguard_test.go
+++ b/cmd/lyx/destructiveguard_test.go
@@ -11,7 +11,7 @@
 // filepath.WalkDir skipping of test files, and the filepath.ToSlash normalisation before any
 // comparison, which matters because Windows is the primary dev OS.
 //
-// Two of the eight banned tokens were corrected against a naive first guess in opposite
+// Two of the nine banned tokens were corrected against a naive first guess in opposite
 // directions, and the reasons are recorded here because both mistakes are easy to reintroduce.
 //
 // "RemoveAll(" rather than "os.RemoveAll(": the bare form is a deliberate superset. It catches the
@@ -44,7 +44,7 @@
 // This file also carries TestMutationRecord_FabricengineProductionSource, the Mutation Record
 // Invariant's guard (see CONSTRAINTS.md's Mutation Record Invariant). It pins two shapes by raw
 // source inspection alone, both against internal/fabricengine/destroy.go and the mutating result
-// types' declarations: that every one of destroy.go's eight executors declares a leading
+// types' declarations: that every one of destroy.go's nine executors declares a leading
 // `rec *Mutations` parameter, and that every mutating verb's result type embeds MutationRecord
 // while the read-only verbs' result types do not. Its blind spots are deliberate and
 // significant: it never inspects an executor's body for a `rec.Append`/`rec.AppendRef` call, so it
@@ -74,8 +74,10 @@ var destructiveGuardScanPackages = []string{
 
 // destructiveGuardBannedTokens are the raw substrings a non-test .go file in
 // destructiveGuardScanPackages may not contain, unless the file is on destructiveGuardAllowlist.
-// This is the discussion's final seven tokens plus "createdToken{", added per the overview's
-// decision that the token's unforgeability is guard-enforced rather than type-enforced.
+// This is the discussion's final seven tokens plus "createdToken{" (added per the overview's
+// decision that the token's unforgeability is guard-enforced rather than type-enforced) plus
+// ".DeleteRemoteBranch(" (added alongside the sixth destructive primitive, so a file other than
+// destroy.go cannot reach the remote-branch-deletion primitive either).
 var destructiveGuardBannedTokens = []string{
 	"RemoveAll(",
 	"os.Remove(",
@@ -85,6 +87,7 @@ var destructiveGuardBannedTokens = []string{
 	"weft.ResetHard(",
 	"fslink.Remove(",
 	"createdToken{",
+	".DeleteRemoteBranch(",
 }
 
 // destructiveGuardAllowlist is this guard's per-file allowlist (path module-relative,
@@ -137,13 +140,14 @@ var destructiveGuardRecordingExecutors = []struct {
 	{"removeLink", "func removeLink(rec *Mutations, "},
 	{"repointLink", "func repointLink(rec *Mutations, "},
 	{"deleteBranch", "func deleteBranch(rec *Mutations, "},
+	{"deleteRemoteBranch", "func deleteRemoteBranch(rec *Mutations, "},
 	{"createExclusiveDir", "func createExclusiveDir(rec *Mutations, "},
 	{"createGitWorktree", "func createGitWorktree(rec *Mutations, "},
 	{"resetHardTo", "func resetHardTo(rec *Mutations, "},
 }
 
 // destructiveGuardRecordingExecutorsMin is the vacuous-scan floor for
-// destructiveGuardRecordingExecutors: the table declares 8 rows today, this floors well below that
+// destructiveGuardRecordingExecutors: the table declares 9 rows today, this floors well below that
 // so a table that silently stopped matching (e.g. a rename that broke every declPrefix at once)
 // fails loudly rather than passing on zero found declarations.
 const destructiveGuardRecordingExecutorsMin = 5
@@ -190,7 +194,7 @@ var destructiveGuardReadOnlyResultTypes = []struct {
 
 // TestNoDestructiveBypass_FabricengineProductionSource walks internal/fabricengine's non-test .go
 // files and fails if any of them (other than a destructiveGuardAllowlist entry) contains one of
-// destructiveGuardBannedTokens — the eight construction/call tokens a destructive primitive
+// destructiveGuardBannedTokens — the nine construction/call tokens a destructive primitive
 // reached outside the gate would carry.
 func TestNoDestructiveBypass_FabricengineProductionSource(t *testing.T) {
 	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
diff --git a/cmd/lyx/gitrepoboundary_test.go b/cmd/lyx/gitrepoboundary_test.go
index 2523db77d..f2cb87c04 100644
--- a/cmd/lyx/gitrepoboundary_test.go
+++ b/cmd/lyx/gitrepoboundary_test.go
@@ -76,6 +76,7 @@ var gitrepoPinnedRunBoundMethods = map[string]bool{
 	"pushWithRebaseRetry": true,
 	"PushRebaseFree":      true,
 	"HasUnpushed":         true,
+	"DeleteRemoteBranch":  true,
 	"MergeStart":          true,
 	"MergeConclude":       true,
 	"ConflictedFiles":     true,
diff --git a/cmd/lyx/helptree_test.go b/cmd/lyx/helptree_test.go
index a0ebc5584..7d89e7c40 100644
--- a/cmd/lyx/helptree_test.go
+++ b/cmd/lyx/helptree_test.go
@@ -25,7 +25,7 @@ func TestHelpTree_RootNamesAllModules(t *testing.T) {
 
 	got := out.String()
 	requiredModules := []string{
-		"board", "config", "ide", "reed", "fabric", "selfreport", "shuttle", "burler", "webster", "stencil", "loom", "run", "quarry",
+		"board", "config", "ide", "reed", "fabric", "selfreport", "shuttle", "burler", "webster", "stencil", "loom", "run", "quarry", "lifecycle",
 	}
 	for _, module := range requiredModules {
 		if !strings.Contains(got, module) {
@@ -81,7 +81,7 @@ func TestHelpTree_VerbModuleSubcommands(t *testing.T) {
 		{
 			name:     "reed",
 			module:   "reed",
-			wantSubs: []string{"up", "add", "remove", "status", "attach", "resume", "down", "header"},
+			wantSubs: []string{"up", "add", "remove", "status", "attach", "resume", "down", "statusline", "watchdog"},
 		},
 		{
 			name:     "selfreport",
@@ -118,6 +118,11 @@ func TestHelpTree_VerbModuleSubcommands(t *testing.T) {
 			module:   "quarry",
 			wantSubs: []string{"toc", "glyphs", "resolve", "expand"},
 		},
+		{
+			name:     "lifecycle",
+			module:   "lifecycle",
+			wantSubs: []string{"run", "status"},
+		},
 	}
 
 	for _, tt := range tests {
diff --git a/cmd/lyx/main.go b/cmd/lyx/main.go
index e4cf0fb59..43dc04f76 100644
--- a/cmd/lyx/main.go
+++ b/cmd/lyx/main.go
@@ -25,6 +25,7 @@ import (
 	"github.com/Knatte18/loomyard/internal/configcli"
 	"github.com/Knatte18/loomyard/internal/fabriccli"
 	"github.com/Knatte18/loomyard/internal/idecli"
+	"github.com/Knatte18/loomyard/internal/lifecyclecli"
 	"github.com/Knatte18/loomyard/internal/logger"
 	"github.com/Knatte18/loomyard/internal/loomcli"
 	"github.com/Knatte18/loomyard/internal/quarrycli"
@@ -72,7 +73,7 @@ It assembles every module's cobra command tree under a single root so that
 all modules are discoverable via "lyx --help" and every subcommand carries
 its own --help and --json help output.
 
-Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler, webster, stencil, loom, run, quarry.`,
+Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler, webster, stencil, loom, run, quarry, lifecycle.`,
 		SilenceUsage:  true,
 		SilenceErrors: true,
 		// Modules' PersistentPreRunE hooks run after root's via EnableTraverseRunHooks.
@@ -108,6 +109,7 @@ Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler
 		stencilcli.Command(),
 		webstercli.Command(),
 		loomcli.Command(),
+		lifecyclecli.Command(),
 		// RunAliasCommand registers the same "run" verb as loomcli.Command()'s
 		// subtree already carries, a second time, as a bare root child rather
 		// than spliced into the argument vector, so it is discoverable in help
diff --git a/cmd/lyx/notransients_test.go b/cmd/lyx/notransients_test.go
index 03d55ea94..4fb1bae0f 100644
--- a/cmd/lyx/notransients_test.go
+++ b/cmd/lyx/notransients_test.go
@@ -18,6 +18,7 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/lifecyclecli"
 	"github.com/Knatte18/loomyard/internal/logger"
 	"github.com/Knatte18/loomyard/internal/loomengine"
 	"github.com/Knatte18/loomyard/internal/lyxcwd"
@@ -78,6 +79,11 @@ func transientSet(l *lyxcwd.Location) []namedPath {
 		{"loomengine.LoomFrictionDir", loomengine.LoomFrictionDir(l)},
 		{"logger.LogsDir", logger.LogsDir(l)},
 		{"treadleengine.PauseFlagPath", treadleengine.PauseFlagPath(filepath.Join(websterengine.ScratchDir(l.AnchorPath()), "blk"))},
+		{"lifecyclecli.LifecycleDir", lifecyclecli.LifecycleDir(l, "slug")},
+		{"lifecyclecli.StatusFile", lifecyclecli.StatusFile(l, "slug")},
+		{"lifecyclecli.RunLock", lifecyclecli.RunLock(l, "slug")},
+		{"lifecyclecli.StatusLock", lifecyclecli.StatusLock(l, "slug")},
+		{"lifecyclecli.PrimeRunLock", lifecyclecli.PrimeRunLock(l)},
 	}
 }
 
diff --git a/cmd/lyx/stencilseed.go b/cmd/lyx/stencilseed.go
index 12022af9c..de36b4e41 100644
--- a/cmd/lyx/stencilseed.go
+++ b/cmd/lyx/stencilseed.go
@@ -43,8 +43,9 @@ func seedStencils(cmd *cobra.Command) {
 	}
 
 	// A command that carries the skip annotation reads no stencils, so the pass is pure waste for
-	// it -- and skipping also keeps a long-lived pane process (e.g. reed header's keepalive) from
-	// ever reaching fabricengine.CommitSeededStencils and performing a git commit in the hub. This
+	// it -- and skipping also keeps a long-lived process (e.g. lyx reed watchdog, the detached
+	// per-hub daemon) from ever reaching fabricengine.CommitSeededStencils and performing a git
+	// commit in the hub. This
 	// early return sits ahead of stencilSeedTarget so an opted-out command resolves no geometry and
 	// spawns no `git rev-parse`.
 	if skipStencilSeed(cmd) {
diff --git a/cmd/lyx/stencilseedgate_test.go b/cmd/lyx/stencilseedgate_test.go
index 0b3842f17..89a534a30 100644
--- a/cmd/lyx/stencilseedgate_test.go
+++ b/cmd/lyx/stencilseedgate_test.go
@@ -1,5 +1,5 @@
 // stencilseedgate_test.go pins two things about the stencil-seed skip gate this batch adds:
-// skipStencilSeed's predicate against synthetic *cobra.Command values, and that "lyx reed header"
+// skipStencilSeed's predicate against synthetic *cobra.Command values, and that "lyx reed statusline"
 // actually carries the annotation the predicate reads. It deliberately does NOT pin any ordering
 // between skipStencilSeed and stencilSeedTarget, and does NOT assert "no `git rev-parse` was
 // spawned": seedStencils returns under testing.Testing() before either step runs, so an in-process
@@ -67,21 +67,21 @@ func TestSkipStencilSeed_HonoursTheAnnotation(t *testing.T) {
 	}
 }
 
-// TestReedHeaderCarriesTheStencilSeedSkipAnnotation walks reedcli.Command()'s subcommands for
-// "header" and asserts it carries clihelp.SkipStencilSeedAnnotation set to clihelp.AnnotationEnabled.
-func TestReedHeaderCarriesTheStencilSeedSkipAnnotation(t *testing.T) {
-	var header *cobra.Command
+// TestReedStatuslineCarriesTheStencilSeedSkipAnnotation walks reedcli.Command()'s subcommands for
+// "statusline" and asserts it carries clihelp.SkipStencilSeedAnnotation set to clihelp.AnnotationEnabled.
+func TestReedStatuslineCarriesTheStencilSeedSkipAnnotation(t *testing.T) {
+	var statusline *cobra.Command
 	for _, sub := range reedcli.Command().Commands() {
-		if sub.Name() == "header" {
-			header = sub
+		if sub.Name() == "statusline" {
+			statusline = sub
 			break
 		}
 	}
-	if header == nil {
-		t.Fatal("reedcli.Command() has no \"header\" subcommand")
+	if statusline == nil {
+		t.Fatal("reedcli.Command() has no \"statusline\" subcommand")
 	}
-	if got := header.Annotations[clihelp.SkipStencilSeedAnnotation]; got != clihelp.AnnotationEnabled {
-		t.Errorf("reed header Annotations[%q] = %q; want %q -- the annotation was silently dropped, making the gate worthless",
+	if got := statusline.Annotations[clihelp.SkipStencilSeedAnnotation]; got != clihelp.AnnotationEnabled {
+		t.Errorf("reed statusline Annotations[%q] = %q; want %q -- the annotation was silently dropped, making the gate worthless",
 			clihelp.SkipStencilSeedAnnotation, got, clihelp.AnnotationEnabled)
 	}
 }
diff --git a/cmd/lyx/tiersleep_test.go b/cmd/lyx/tiersleep_test.go
index b9e98b3cb..a23d7d4f4 100644
--- a/cmd/lyx/tiersleep_test.go
+++ b/cmd/lyx/tiersleep_test.go
@@ -20,20 +20,7 @@ import (
 // slash-separated file paths permitted to contain a literal time.Sleep(...) of at
 // least one second in an untagged test file, each with a one-line reason — mirroring
 // tierpurity_test.go's allowedSpawners style.
-var allowedLongSleepers = map[string]string{
-	"internal/reedengine/testmain_test.go": "untagged header-pane-keepalive TestMain stand-in whose body loops calling " +
-		"time.Sleep(time.Hour) at line 31 — intentional and safe because TestMain itself " +
-		"intercepts this exact invocation shape (os.Args[1] == \"reed\") as a deliberate " +
-		"stand-in for the real `lyx reed header --blocking` keepalive, not an accidental " +
-		"recursive re-exec; it is not reachable during a normal `go test` run (no test " +
-		"invokes the binary with a leading \"reed\" argument)",
-	"internal/reedcli/testmain_test.go": "same shape and the same TestMain-intercept rationale at its own matching " +
-		"loop on line 35 — this file's TestMain also calls gitkit.HermeticGitEnv() later in " +
-		"the same function, but only on the code path reached when the \"reed\" branch above " +
-		"is NOT taken; the sleep loop itself is an infinite for loop that never returns, so " +
-		"HermeticGitEnv() never executes on the sleep-loop path and plays no part in why that " +
-		"path is safe",
-}
+var allowedLongSleepers = map[string]string{}
 
 // findLongLiteralSleep parses data as Go and finds time.Sleep(...) calls with duration >= 1 second.
 func findLongLiteralSleep(fset *token.FileSet, filename string, data []byte) (evidence string, found bool) {
diff --git a/contracts/recipes/lifecycle-recipe.yaml b/contracts/recipes/lifecycle-recipe.yaml
new file mode 100644
index 000000000..ad218d342
--- /dev/null
+++ b/contracts/recipes/lifecycle-recipe.yaml
@@ -0,0 +1,29 @@
+# lifecycle-recipe.yaml is the task-worktree lifecycle's producer graph. It is embedded into the
+# binary by the sibling recipes.go rather than read from disk. The three row names below are
+# durable on-disk identities that resume depends on, pinned against internal/lifecyclerecipe's own
+# Name* constants by that package's coverage guard -- a rename here without a matching constant
+# rename breaks resume for any in-flight run.
+#
+# Loom-Run's empty on_stuck is load-bearing: a stuck verdict there escalates to a human with the
+# task worktree fully intact, which is what makes the destructive Worktree-Teardown row unreachable
+# from any failure path.
+#
+# Worktree-Teardown's empty on_done is likewise load-bearing and is what ends the run quietly.
+
+version: 1
+entry: Worktree-Create
+terminals:
+  - Worktree-Teardown
+
+producers:
+  - name: Worktree-Create
+    engine: WorktreeCreate
+    on_done: Loom-Run
+
+  - name: Loom-Run
+    engine: LoomRun
+    on_done: Worktree-Teardown
+
+  - name: Worktree-Teardown
+    engine: WorktreeTeardown
+    on_done: ""
diff --git a/contracts/recipes/recipes.go b/contracts/recipes/recipes.go
index 4766cca48..8bf8e7839 100644
--- a/contracts/recipes/recipes.go
+++ b/contracts/recipes/recipes.go
@@ -12,3 +12,9 @@ import (
 //
 //go:embed loom-recipe.yaml
 var LoomRecipe []byte
+
+// LifecycleRecipe is the task-worktree lifecycle's producer graph, in internal/shedbuild's recipe
+// format.
+//
+//go:embed lifecycle-recipe.yaml
+var LifecycleRecipe []byte
diff --git a/docs/overview.md b/docs/overview.md
index 26f326fbe..e546a9b02 100644
--- a/docs/overview.md
+++ b/docs/overview.md
@@ -258,7 +258,7 @@ github.com/Knatte18/loomyard/
 ├── internal/lock/                shared file locking
 ├── internal/output/              shared JSON output
 ├── internal/modelspec/           model-spec parser + models.yaml registry leaf
-├── internal/tokenvocab/          shared token vocabulary (repo, hub) + Render compose over stencil, a leaf
+├── internal/tokenvocab/          shared token vocabulary (repo, hub, worktree) + Render compose over stencil, a leaf
 ├── internal/pattern/             PATTERN active check + role directive leaf, consumed by webster/burler/loom
 ├── internal/friction/            the Tier 2 friction-note directive leaf, consumed by webster, burler, and loom
 └── internal/shell/               provider-invariant pane-shell mechanics leaf (pwsh + posix)
@@ -301,7 +301,7 @@ User-facing modules each get one `lyx <module>` namespace:
   supports `--body` (or `-` for stdin) and `--label`;
   defaults to `bug`.
   Callable from any sandbox agent context with no config. ✅ Implemented.
-- **reed** — **the window to the world**: tmux overlay + **strand** bookkeeping + render (`internal/reedcli` + `internal/reedengine` + `internal/reedengine/render`). Hosts every managed process as a strand, arranges them, persists to `.lyx/reed.json` (`lyx reed up|add|remove|status|attach|resume|header|down`). Built on what its proof-of-concept, `muxpoc`, proved first (layout checksum, bottom-dominant layout, env hygiene, native `--resume`); `muxpoc` has since been deleted, its job done. `reed attach` and `reed header --blocking` are this module's two registered interactive-handoff exceptions (CONSTRAINTS.md CLI/Cobra Invariant): `attach` hands the operator's stdio to a `tmux attach-session` child in place, now pre-flighting through two fallible steps — booting the session if it is cold, then checking that the server/session is up — and `header --blocking` prints the rendered header-pane text (`Engine.HeaderText`, over `internal/tokenvocab`) then blocks forever as the header pane's own keepalive — in both cases every fallible step runs pre-flight, on the envelope, and only the terminal-handover/keepalive tail itself is exempt from emitting JSON. `add` and `attach` boot this worktree's session when none is up, with `up`'s own semantics — a bare substrate, with no persisted strand relaunched — instead of refusing; every other reed verb still refuses on a cold worktree. ✅ Implemented. See the `internal/reedengine` package documentation.
+- **reed** — **the window to the world**: tmux overlay + **strand** bookkeeping + render (`internal/reedcli` + `internal/reedengine` + `internal/reedengine/render`). Hosts every managed process as a strand, arranges them, persists to `.lyx/reed.json` (`lyx reed up|add|remove|status|attach|resume|statusline|watchdog|down`). Built on what its proof-of-concept, `muxpoc`, proved first (layout checksum, bottom-dominant layout, env hygiene, native `--resume`); `muxpoc` has since been deleted, its job done. `reed attach` and `reed watchdog` are this module's two registered interactive-handoff exceptions (CONSTRAINTS.md CLI/Cobra Invariant): `attach` hands the operator's stdio to a `tmux attach-session` child in place, pre-flighting through booting the session if cold and checking server/session liveness, and `watchdog` is the detached per-hub resize daemon that blocks for its process lifetime — in both cases every fallible step runs pre-flight, on the envelope, and only the terminal-handover/daemon tail itself is exempt from emitting JSON. `add` and `attach` boot this worktree's session when none is up, with `up`'s own semantics — a bare substrate, with no persisted strand relaunched — instead of refusing; every other reed verb still refuses on a cold worktree. ✅ Implemented. See the `internal/reedengine` package documentation.
 - **shuttle** — run **one** LLM agent as an interactive tmux strand over the file contract (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`; `lyx shuttle run|interrupt|send`). `Stop`-hook completion is read off an events file and classified into four outcomes — `done`/`asking`/`died`/`timeout` — with `asking` as the escalation channel back to a human or a higher-capability model. `asking` means the agent ended its turn WITHOUT satisfying the file contract, whatever its reason: shuttle never inspects the message, so a blocked agent, a question, and a mid-task status report all classify identically, and `lastAssistantMessage` carries whatever it last said rather than a guaranteed question. An interactive run also detects a live `AskUserQuestion` tool call in real time via a non-denying marker hook, classified the same way instead of waiting for the timeout. `died` is correspondingly narrow: it means a strand reed STILL TRACKS has a pane that is not alive (or the provider never came up inside the startup window). Reed's strand table losing the strand entirely — a `reed down`/`remove`, a deleted or rebuilt `reed.json`, a worktree renamed under an in-flight run — is reported as a mechanism failure carrying the run's identity, never as `died`, because reed's bookkeeping going away says nothing about the agent, whose process is often still working in its pane. `PreToolUse` guardrails deny the in-process `Agent` tool in both interactive and autonomous runs (on by default, switchable off via `shuttle.yaml`'s `claude_deny_agent_tool`, and narrowed in a `ForkSubagents` run to permit fork subagents while still refusing every other subagent type), and `AskUserQuestion` too when the run is autonomous (`Interactive: false`, the default). The provider is swappable behind an **engine** seam; Claude is the only v1 engine. Per-run `Model`/`Effort` knobs (`lyx shuttle run --model`/`--effort`; effort values `low|medium|high|xhigh|max`, empty = provider default) are engine-validated, not policed by `Spec.validate`. `Spec.Version` is a programmatic engine-validated version pin (claudeengine composes the pinned model id; no CLI flag — consumers drive it via the model-spec notation's `v=` param). ✅ Implemented. See the `internal/shuttleengine` package documentation.
 - **webster** — the implementer module: one long-lived Master session that reads the codebase and the whole plan once, then forks one implementer per execution batch in-session (Claude Code's Agent tool) instead of spawning a fresh reed/tmux strand per batch;
   bracket verbs (`begin-batch`/`await-batch`/`record-batch`) drive each in-session fork (Master long-polls `await-batch` for each batch's report instead of relying on a synchronous fork return), and a genuine model escalation (recovery after a stuck/report-less fork) spawns a cold strand.
@@ -347,7 +347,7 @@ User-facing modules each get one `lyx <module>` namespace:
   No `lyx shed` verb of its own by design — a product's own CLI constructs a `Shed` with its own producer list and calls `Run`, and a bare verb would be a command with no list to walk.
   The skeleton (the loop, the status file, the `ShedProducer` interface) is ✅ **implemented**; the four engine adapters (`SingleLLMProducer`, the `Webster` adapter, the burler round producer, and the Bouncer) are ✅ **implemented** too, shipped as `internal/shedadapters`.
   `internal/shedcheck` is the shipped structural checker over an assembled producer list, enforced by a `go test` invariant over loom's own list rather than called from any production constructor — see [manifest/designs/shed.md](../manifest/designs/shed.md#checking-an-assembled-producer-list) for the eight finding kinds it reports.
-  The Shed recipe group's engine registry (piece 1 of that group) is ✅ **implemented** too, as `internal/shedrecipe`; it registers twelve engine names.
+  The Shed recipe group's engine registry (piece 1 of that group) is ✅ **implemented** too, as `internal/shedrecipe`; it registers seventeen engine names.
   The recipe file format and the loader/builder shipped too, as `internal/shedbuild`, and loom's own conversion to a recipe file has now shipped as well: `contracts/recipes/loom-recipe.yaml` plus `internal/loomrecipe`, which assembles it into the `*shedengine.Shed` `internal/loomcli` runs.
   See the `internal/shedengine`, `internal/shedadapters`, `internal/shedcheck`, `internal/shedrecipe`, `internal/shedbuild`, and `internal/loomrecipe` package documentation and [manifest/designs/shed.md](../manifest/designs/shed.md).
 - **burler** — one review+fix round: A-review → B-fix, one agent, no self-grading, over the shuttle file contract (`internal/burlerengine` + `internal/burlercli`).
@@ -360,6 +360,9 @@ User-facing modules each get one `lyx <module>` namespace:
   Behavior-based reviewer that *runs* a live-substrate module (needs a sandbox repo) to harden it before merge;
   on-demand, post-loom, **off the spine**, shares only the `burler` round discipline.
   See [manifest/designs/hardener.md](../manifest/designs/hardener.md).
+- **lifecycle** — drives one task worktree's whole lifecycle — create, run the loom session to a terminal state, and tear down — as a single Shed run from the hub's prime worktree (`internal/lifecycleshed` + `internal/lifecyclerecipe` + `internal/lifecyclecli`; `lyx lifecycle run|status`).
+  Teardown is one row sequencing session shutdown before worktree removal, and never forces.
+  ✅ Implemented. See the `internal/lifecycleshed` and `internal/lifecyclerecipe` package documentation.
 
 The cross-OS spawn primitive **proc**, and the generic outer phase-FSM **shed**, are the two remaining internal (non-CLI) layers — proc the base of the stack, shed the generic engine `loom` configures rather than a stack layer of its own;
 see the [Execution stack](#execution-stack-orchestration-layers) section below for how proc / reed / shuttle fit together. (Earlier drafts split reed into separate `shed`/`glance` modules;
@@ -391,6 +394,9 @@ loom              phase machine: drive each phase through a Bouncer gate       [
                                                                                  burler]
 ```
 
+The lifecycle Shed nests loom's: it is its own three-row recipe whose middle row drives a task's loom run as a child process and polls that run's own persisted status for the verdict, so there are two status files by design — the task's, committed on the task branch, and the lifecycle's, per-machine under prime's own ephemeral tree — each resuming independently.
+See the [Lifecycle Bookend Invariant](../CONSTRAINTS.md#lifecycle-bookend-invariant).
+
 The whole stack runs **headless** (auto mode): strands exist (the interactive-session requirement), agents run, output files are read, nobody need watch.
 
 The stack now has two entry modes, not one: every layer from `reed` up is **told** its geometry rather than deriving it.
@@ -408,7 +414,7 @@ See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant) for
 - **provider-invariant** — `shuttle` runs Claude today through an **engine**;
   the verdict/output contract is provider-invariant, so a different model can be swapped in without touching the review machinery.
   Non-Claude is not a current priority.
-- **`tokenvocab` is a shared leaf, not a stack layer** — `internal/tokenvocab` (the `repo`/`hub` token registry + the `Render` compose over `internal/stencil`) sits beside `stencil` and `modelspec` as a general-purpose leaf the stack's modules consume, not a stage of the proc→reed→shuttle→burler→shed→loom chain itself. reed's header text pipeline consumes it today;
+- **`tokenvocab` is a shared leaf, not a stack layer** — `internal/tokenvocab` (the three-token registry + the `Render` compose over `internal/stencil`) sits beside `stencil` and `modelspec` as a general-purpose leaf the stack's modules consume, not a stage of the proc→reed→shuttle→burler→shed→loom chain itself. reed's status-line pipeline consumes it today;
   loom's prompt templates are expected to reuse the same `Render` compose later.
   See the `internal/tokenvocab` package documentation.
 - **the bootstrap** — `lyx loom run` (alias `lyx run`) brings up the worktree's tmux session, adds the `lyx loom status` strand (a 1-line top pane), spawns the loom driver **detached** (via `proc`, no TTY), and attaches the terminal to the session. loom runs in the background;
@@ -462,7 +468,7 @@ See [sandbox-howto.md](sandbox-howto.md) for the step-by-step runbook and [sandb
   design.
 - [code-comment-conventions.md](code-comment-conventions.md) — the doc-comment rule's standing rationale (Go only, for now);
   a durable convention doc, kept rather than deleted, moved here from `manifest/designs/` by the 2026-08-29 designs audit.
-- `internal/tokenvocab` package documentation — the shared token vocabulary (`repo`/`hub` + `Render` over `internal/stencil`), consumed by reed's header pipeline and, later, loom's prompt templates;
+- `internal/tokenvocab` package documentation — the shared token vocabulary (`repo`/`hub`/`worktree` + `Render` over `internal/stencil`), consumed by reed's status-line pipeline and, later, loom's prompt templates;
   a leaf, not a phased module (as-built;
   module doc deleted per the documentation lifecycle).
 - [webster-spec.md](../contracts/specs/webster-spec.md) — webster's cross-module contract: the `_lyx/webster/` boundary, `outcome.yaml`, and `summary.md`'s writer-side additions (as-built;
diff --git a/internal/burlercli/cli.go b/internal/burlercli/cli.go
index f9759f283..bb6756b55 100644
--- a/internal/burlercli/cli.go
+++ b/internal/burlercli/cli.go
@@ -11,6 +11,7 @@
 package burlercli
 
 import (
+	"context"
 	"io"
 
 	"github.com/Knatte18/loomyard/internal/burlerengine"
@@ -32,13 +33,18 @@ type burlerCLI struct {
 	// driver is a different directory (crucible round opus-medium-r6, R6-17).
 	cwd string
 
-	// reedUp brings the standalone reed session up, idempotently, and is set by wireStandalone
-	// alone — the run verb calls it immediately before driving a round, because standalone mode has
-	// no other way to a live session: `lyx reed up` is hub-only (its pre-run requires
-	// lyxcwd.Resolve), so the session on standalone's own geometry (socket "lyx-<hash8>", state
-	// under the derived state directory) can only be booted in-process. It stays nil in hub mode,
-	// where bringing reed up remains the operator's (or loom's) own act.
-	reedUp func() error
+	// reedUp brings the standalone reed session up, idempotently, and, when watch is true and the
+	// boot succeeded, also starts that session's in-process resize watcher bound to ctx. It is set by
+	// wireStandalone alone — the run verb calls it immediately before driving a round, because
+	// standalone mode has no other way to a live session: `lyx reed up` is hub-only (its pre-run
+	// requires lyxcwd.Resolve), so the session on standalone's own geometry (socket "lyx-<hash8>",
+	// state under the derived state directory) can only be booted in-process. It stays nil in hub
+	// mode, where bringing reed up remains the operator's (or loom's) own act.
+	//
+	// watch is an explicit parameter, not an implicit rule, precisely so the recover-batch asymmetry
+	// (webster's own recover-batch call site passes false; every other call site passes true) is
+	// visible at the call sites themselves rather than rediscovered from a comment.
+	reedUp func(ctx context.Context, watch bool) error
 
 	// stencilsDirFlag and targetDirFlag hold the raw, as-parsed values of the two standalone-entry
 	// persistent flags (--stencils-dir, --target-dir). An empty value means the flag was not passed;
diff --git a/internal/burlercli/run.go b/internal/burlercli/run.go
index d2682080b..2959243ee 100644
--- a/internal/burlercli/run.go
+++ b/internal/burlercli/run.go
@@ -165,7 +165,7 @@ run-timeout; zero defers to the config default.`,
 			// session" with an impossible recourse (crucible round fable5-high-r3, F-A1). Nil in
 			// hub mode, where the session is the operator's or loom's own to manage.
 			if c.reedUp != nil {
-				if err := c.reedUp(); err != nil {
+				if err := c.reedUp(cmd.Context(), true); err != nil {
 					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("burler: bring up the standalone reed session: %v", err)))
 					return nil
 				}
diff --git a/internal/burlercli/wiring.go b/internal/burlercli/wiring.go
index 41c96f5ca..96e9c65f6 100644
--- a/internal/burlercli/wiring.go
+++ b/internal/burlercli/wiring.go
@@ -8,10 +8,13 @@
 package burlercli
 
 import (
+	"context"
+
 	"github.com/Knatte18/loomyard/internal/burlerengine"
 	"github.com/Knatte18/loomyard/internal/cliwire"
 	"github.com/Knatte18/loomyard/internal/fabricengine"
 	"github.com/Knatte18/loomyard/internal/hubgeom"
+	"github.com/Knatte18/loomyard/internal/logger"
 	"github.com/Knatte18/loomyard/internal/lyxcwd"
 	"github.com/Knatte18/loomyard/internal/preflight"
 	"github.com/Knatte18/loomyard/internal/reedengine"
@@ -182,9 +185,25 @@ func (c *burlerCLI) wireStandalone(cwd, stencilsDirOverride, targetDirFlag strin
 	// `lyx reed up` is hub-only — so the run verb boots it in-process through this seam (see the
 	// field's own doc comment). Assigned here, executed only by run, so wiring itself never boots
 	// a tmux server.
-	c.reedUp = func() error {
-		_, err := reedEngine.Up()
-		return err
+	//
+	// Standalone reed is never booted by `lyx reed up` — that verb is hub-only — it is booted
+	// in-process by a long-lived supervising run that exists for exactly the session's working
+	// lifetime, and that supervising process is precisely what hub mode lacks and why hub mode needs
+	// a detached daemon at all — so reusing it here is the smaller mechanism, not a special case. The
+	// caller's ctx is what stops the watcher; standalone computes no hub lock path and spawns no
+	// daemon.
+	c.reedUp = func(ctx context.Context, watch bool) error {
+		if _, err := reedEngine.Up(); err != nil {
+			return err
+		}
+		if watch {
+			go func() {
+				if err := reedEngine.Watch(ctx); err != nil {
+					logger.Debug("burler: standalone resize watcher returned", "err", err)
+				}
+			}()
+		}
+		return nil
 	}
 
 	// A `lyx burler` run is not a loom run and has no friction directory in either mode.
diff --git a/internal/burlercli/wiring_test.go b/internal/burlercli/wiring_test.go
index 0926fbaaa..5c24e5083 100644
--- a/internal/burlercli/wiring_test.go
+++ b/internal/burlercli/wiring_test.go
@@ -41,13 +41,18 @@
 package burlercli
 
 import (
+	"bytes"
+	"context"
+	"errors"
 	"fmt"
 	"os"
 	"path/filepath"
 	"strings"
 	"testing"
+	"time"
 
 	"github.com/Knatte18/loomyard/internal/burlerengine"
+	"github.com/Knatte18/loomyard/internal/clihelp"
 	"github.com/Knatte18/loomyard/internal/cliwire"
 	"github.com/Knatte18/loomyard/internal/configengine"
 	"github.com/Knatte18/loomyard/internal/fabricengine"
@@ -591,3 +596,169 @@ func TestWireModule_DescriptorIsVerbatim(t *testing.T) {
 		}
 	})
 }
+
+// TestReedUpSeam_WatcherLifecycle pins the standalone reedUp seam's lifecycle contract -- the one
+// batch 06-standalone-watcher's card 42 requires of BOTH wireStandalone closures (this package's and
+// webstercli's): the resize watcher starts iff the boot succeeded AND watch is true, and once
+// started it lives exactly as long as the passed context. wiring.go itself is outside this card's
+// Edits and is not driven directly here -- it closes over a real *reedengine.Engine whose Up() spawns
+// a live tmux server, which an untagged Tier 1 test must never do (Test Tier Purity Invariant). This
+// test instead drives the identical shape wiring.go's closure implements against a fake stand-in for
+// reedEngine.Up/Watch, so the lifecycle contract itself is pinned without booting a substrate.
+func TestReedUpSeam_WatcherLifecycle(t *testing.T) {
+	t.Parallel()
+
+	// newSeam reproduces wiring.go's c.reedUp closure body: boot, then start the watcher (bound to
+	// ctx) iff the boot succeeded and watch is true.
+	newSeam := func(upErr error, watchCalls chan context.Context) func(context.Context, bool) error {
+		return func(ctx context.Context, watch bool) error {
+			if upErr != nil {
+				return upErr
+			}
+			if watch {
+				go func() {
+					watchCalls <- ctx
+					<-ctx.Done()
+				}()
+			}
+			return nil
+		}
+	}
+
+	t.Run("StartsOnSuccessfulBootWithWatchTrue", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+		ctx, cancel := context.WithCancel(context.Background())
+		defer cancel()
+
+		seam := newSeam(nil, watchCalls)
+		if err := seam(ctx, true); err != nil {
+			t.Fatalf("seam() = %v; want nil", err)
+		}
+		select {
+		case <-watchCalls:
+		case <-time.After(time.Second):
+			t.Fatal("watcher did not start within 1s of a successful boot with watch: true")
+		}
+	})
+
+	t.Run("DoesNotStartWhenBootFails", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+		bootErr := errors.New("boot failed")
+
+		seam := newSeam(bootErr, watchCalls)
+		if err := seam(context.Background(), true); !errors.Is(err, bootErr) {
+			t.Fatalf("seam() = %v; want %v", err, bootErr)
+		}
+		select {
+		case <-watchCalls:
+			t.Fatal("watcher started despite a failed boot")
+		case <-time.After(50 * time.Millisecond):
+		}
+	})
+
+	t.Run("DoesNotStartWhenWatchFalse", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+
+		seam := newSeam(nil, watchCalls)
+		if err := seam(context.Background(), false); err != nil {
+			t.Fatalf("seam() = %v; want nil", err)
+		}
+		select {
+		case <-watchCalls:
+			t.Fatal("watcher started despite watch: false")
+		case <-time.After(50 * time.Millisecond):
+		}
+	})
+
+	t.Run("StopsWhenContextIsCancelled", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+		ctx, cancel := context.WithCancel(context.Background())
+
+		seam := newSeam(nil, watchCalls)
+		if err := seam(ctx, true); err != nil {
+			t.Fatalf("seam() = %v; want nil", err)
+		}
+		var watcherCtx context.Context
+		select {
+		case watcherCtx = <-watchCalls:
+		case <-time.After(time.Second):
+			t.Fatal("watcher did not start")
+		}
+		cancel()
+		select {
+		case <-watcherCtx.Done():
+		case <-time.After(time.Second):
+			t.Fatal("watcher's context did not observe cancellation within 1s")
+		}
+	})
+}
+
+// TestRunCmd_PassesWatchTrueToReedUp proves burler's run verb calls c.reedUp with watch: true, the
+// disposition card 43 fixes for it -- driven through run's own RunE with a recording fake in
+// c.reedUp rather than by reading source text. The fake returns an error so the call terminates
+// immediately after the reedUp check, never reaching c.engine (left nil on this fixture), exactly the
+// same shape internal/webstercli's recover-batch test in cli_test.go uses for its own call site.
+func TestRunCmd_PassesWatchTrueToReedUp(t *testing.T) {
+	t.Parallel()
+
+	dir := t.TempDir()
+	profilePath := filepath.Join(dir, "profile.yaml")
+	if err := os.WriteFile(profilePath, []byte("rubric: placeholder\n"), 0o644); err != nil {
+		t.Fatalf("write profile: %v", err)
+	}
+
+	c := &burlerCLI{cwd: dir}
+	var bringUps int
+	var gotWatch bool
+	c.reedUp = func(ctx context.Context, watch bool) error {
+		bringUps++
+		gotWatch = watch
+		return errors.New("no tmux server available in this test")
+	}
+
+	var out bytes.Buffer
+	exitCode := clihelp.Execute(c.runCmd(), &out, []string{"--profile", profilePath})
+
+	if bringUps != 1 {
+		t.Fatalf("c.reedUp calls = %d; want exactly 1", bringUps)
+	}
+	if !gotWatch {
+		t.Error("c.reedUp watch = false; want true -- run binds the watcher to the run's own context")
+	}
+	if exitCode != 1 {
+		t.Fatalf("run with a failing reed bring-up = %d; want 1, output: %s", exitCode, out.String())
+	}
+}
+
+// TestProductionFiles_NeverReferenceHubWatchdogMechanism proves this package's production files never
+// reference the detached per-hub watchdog daemon's mechanism: standalone computes no hub lock path
+// (fabricengine.HubScratchDir) and spawns no daemon (the "reed watchdog" verb). Both belong to
+// hub mode alone, per this batch's own scope note.
+func TestProductionFiles_NeverReferenceHubWatchdogMechanism(t *testing.T) {
+	t.Parallel()
+
+	matches, err := filepath.Glob("*.go")
+	if err != nil {
+		t.Fatalf("glob *.go: %v", err)
+	}
+	for _, path := range matches {
+		if strings.HasSuffix(path, "_test.go") {
+			continue
+		}
+		data, err := os.ReadFile(path)
+		if err != nil {
+			t.Fatalf("read %s: %v", path, err)
+		}
+		content := string(data)
+		if strings.Contains(content, "fabricengine.HubScratchDir") {
+			t.Errorf("%s references fabricengine.HubScratchDir; standalone must compute no hub lock path", path)
+		}
+		if strings.Contains(content, "reed watchdog") {
+			t.Errorf("%s references \"reed watchdog\"; standalone must spawn no daemon", path)
+		}
+	}
+}
diff --git a/internal/clihelp/annotations_test.go b/internal/clihelp/annotations_test.go
index 1270dbe6e..81da65b8e 100644
--- a/internal/clihelp/annotations_test.go
+++ b/internal/clihelp/annotations_test.go
@@ -1,5 +1,5 @@
 // annotations_test.go asserts the exact literal values of this package's cobra-annotation constants,
-// so a rename cannot silently decouple a producer command (e.g. reed header) from the consumer gate
+// so a rename cannot silently decouple a producer command (e.g. reed statusline) from the consumer gate
 // (cmd/lyx's skipStencilSeed).
 
 package clihelp
diff --git a/internal/configsync/configsync_test.go b/internal/configsync/configsync_test.go
index 0428eb077..a4d6bda81 100644
--- a/internal/configsync/configsync_test.go
+++ b/internal/configsync/configsync_test.go
@@ -184,6 +184,73 @@ func TestReconcileAll_DropsStaleReedClaudeKey(t *testing.T) {
 	}
 }
 
+// TestReconcileAll_DropsStaleReedHeaderBlock pins the migration path an operator actually gets from
+// "lyx config reconcile --apply": a reed.yaml written before the header: block was split into
+// status_line: and selvage: must have header.template and header.height_rows reconciled away, and
+// the new status_line/selvage leaves added, exactly like TestReconcileAll_DropsStaleReedClaudeKey
+// pins the earlier claude: key removal.
+func TestReconcileAll_DropsStaleReedHeaderBlock(t *testing.T) {
+	tmpDir := t.TempDir()
+	configDir := configengine.ConfigDir(tmpDir)
+	if err := os.MkdirAll(configDir, 0o755); err != nil {
+		t.Fatalf("mkdir: %v", err)
+	}
+
+	// Seed reed.yaml as it would exist on disk for a user who set up their
+	// worktree before the header: block was split into status_line: and
+	// selvage:.
+	reedPath := configengine.ConfigFile(tmpDir, "reed")
+	seedContent := "tmux: C:\\tools\\tmux.exe\nheader:\n  template: \"\"\n  height_rows: 1\n"
+	if err := os.WriteFile(reedPath, []byte(seedContent), 0o644); err != nil {
+		t.Fatalf("write reed.yaml: %v", err)
+	}
+
+	results, err := ReconcileAll(tmpDir, true)
+	if err != nil {
+		t.Fatalf("ReconcileAll(true): %v", err)
+	}
+
+	reedResult := findResult(results, "reed")
+	if reedResult == nil {
+		t.Fatal("reed result not found")
+	}
+	if !reedResult.Applied {
+		t.Error("reed.Applied is false; want true (stale header block should trigger a rewrite)")
+	}
+
+	wantRemoved := map[string]bool{"header.template": false, "header.height_rows": false}
+	for _, r := range reedResult.Removed {
+		if _, ok := wantRemoved[r]; ok {
+			wantRemoved[r] = true
+		}
+	}
+	for key, found := range wantRemoved {
+		if !found {
+			t.Errorf("reed.Removed = %v, want it to contain %q", reedResult.Removed, key)
+		}
+	}
+
+	wantAdded := map[string]bool{"status_line.template": false, "selvage.height_rows": false}
+	for _, a := range reedResult.Added {
+		if _, ok := wantAdded[a]; ok {
+			wantAdded[a] = true
+		}
+	}
+	for key, found := range wantAdded {
+		if !found {
+			t.Errorf("reed.Added = %v, want it to contain %q", reedResult.Added, key)
+		}
+	}
+
+	content, err := os.ReadFile(reedPath)
+	if err != nil {
+		t.Fatalf("read reed.yaml: %v", err)
+	}
+	if contains(string(content), "header:") {
+		t.Error("reed.yaml still contains a header: block after apply; should have been removed")
+	}
+}
+
 func TestReconcileAll_Idempotent(t *testing.T) {
 	tmpDir := t.TempDir()
 	configDir := configengine.ConfigDir(tmpDir)
diff --git a/internal/fabriccli/fabric.go b/internal/fabriccli/fabric.go
index 303d78c93..3458e6b7f 100644
--- a/internal/fabriccli/fabric.go
+++ b/internal/fabriccli/fabric.go
@@ -183,10 +183,10 @@ use "lyx fabric pairs".`,
 		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int { return runList(ctx, out, args) }),
 	})
 
-	// remove [--force] <slug>
+	// remove [--force] [--remote] <slug>
 	var removeCmd *cobra.Command
 	removeCmd = &cobra.Command{
-		Use:   "remove [--force] <slug>",
+		Use:   "remove [--force] [--remote] <slug>",
 		Args:  cobra.MaximumNArgs(1),
 		Short: "destroy a dual warp+weft worktree pair",
 		Long: `Remove a paired warp and weft git worktree, plus every warp junction
@@ -202,16 +202,26 @@ are all refused — the same set "lyx fabric add" refuses. When git itself
 declines to remove the worktree, fabric reports git's own reason and deletes
 nothing unless the target is a registered linked worktree of this repo.
 
+The pair's weft branch is deleted locally as before. Use --remote to
+additionally delete its copy on the weft remote — an irreversible action,
+visible to every other clone. A weft repo with no origin remote configured
+reports the reason in remote_skipped_reason and still exits 0. A failed
+remote deletion exits non-zero, with the reason in remote_branch_error.
+
 Example:
   lyx fabric remove my-task
-  lyx fabric remove --force my-task`,
+  lyx fabric remove --force my-task
+  lyx fabric remove --remote my-task`,
 		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
-			// The --force flag is read from the cobra flag set via closure over removeCmd.
+			// The --force and --remote flags are read from the cobra flag set via closure over
+			// removeCmd.
 			force, _ := removeCmd.Flags().GetBool("force")
-			return runRemoveWithFlag(ctx, out, args, force)
+			remote, _ := removeCmd.Flags().GetBool("remote")
+			return runRemoveWithFlag(ctx, out, args, force, remote)
 		}),
 	}
 	removeCmd.Flags().Bool("force", false, "forcefully remove worktree with uncommitted changes")
+	removeCmd.Flags().Bool("remote", false, "irreversibly delete the pair's weft branch on the weft remote too, visible to every other clone")
 	cmd.AddCommand(removeCmd)
 
 	cmd.AddCommand(&cobra.Command{
@@ -328,7 +338,7 @@ Example:
 
 	var cleanupCmd *cobra.Command
 	cleanupCmd = &cobra.Command{
-		Use:   "cleanup [--apply] [--force]",
+		Use:   "cleanup [--apply] [--force] [--remote]",
 		Args:  cobra.NoArgs,
 		Short: "delete weft branches whose warp sibling is gone",
 		Long: `cleanup finds weft branches with no corresponding warp worktree sibling.
@@ -339,9 +349,17 @@ Flag matrix:
                       and checked-out branches stay protected).
   --force (alone)     report only; --force does not imply --apply, and today
                       answers no cleanup gate.
+  --remote (alone)    report only; --remote requires --apply to delete
+                      anything, so --remote alone is still a dry run.
+  --apply --remote    delete every orphan weft branch, both locally and on
+                      the weft remote.
 
 A dry run reports the same protected verdict the matching --apply run would
-act on, so "protected: false" in a dry run means "--apply would delete this".
+act on, so "protected: false" in a dry run means "--apply would delete this",
+and — with --remote — "would attempt the remote copy too". A dry run makes
+no network call and reports no remote-specific verdict.
+
+--remote is independent of --force.
 
 A weft branch currently checked out at a worktree is always reported as
 protected and never deleted, in every mode — git cannot delete a checked-out
@@ -359,16 +377,27 @@ The weft repo may also hold weft branches without the fabric suffix (e.g.
 inherited from history predating fabric's uniform naming scheme); those are
 reported but never deleted here, since they are not fabric-managed.
 
-Deletion is local to the hub's weft repo: a deleted branch's copy on the
-weft remote, if it was ever pushed, is left untouched.`,
+Deletion is local to the hub's weft repo by default. Use --remote to
+additionally delete each deleted branch's copy on the weft remote, an
+irreversible action visible to every other clone. A weft repo with no origin
+remote configured reports the reason once in remote_skipped_reason and still
+exits 0. A remote deletion that fails exits non-zero, with the per-branch
+reason in entries[].remote_error.
+
+This is also the one existing-path change this command makes: --apply now
+exits non-zero when a local branch deletion fails, where it previously
+exited 0. A protected entry still exits 0, since protection sets no error at
+all.`,
 		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
 			apply, _ := cleanupCmd.Flags().GetBool("apply")
 			force, _ := cleanupCmd.Flags().GetBool("force")
-			return runCleanupWithFlags(ctx, out, apply, force)
+			remote, _ := cleanupCmd.Flags().GetBool("remote")
+			return runCleanupWithFlags(ctx, out, apply, force, remote)
 		}),
 	}
 	cleanupCmd.Flags().Bool("apply", false, "delete orphaned weft branches (default is dry-run/report)")
 	cleanupCmd.Flags().Bool("force", false, "reserved; answers no cleanup gate today")
+	cleanupCmd.Flags().Bool("remote", false, "irreversibly delete each deleted branch's copy on the weft remote too, visible to every other clone; requires --apply")
 	cmd.AddCommand(cleanupCmd)
 
 	cmd.AddCommand(&cobra.Command{
@@ -757,9 +786,9 @@ func runPruneWithFlags(ctx context.Context, out io.Writer, apply, force bool) in
 	})
 }
 
-// runCleanupWithFlags executes the cleanup logic with the resolved apply and
-// force flags.
-func runCleanupWithFlags(ctx context.Context, out io.Writer, apply, force bool) int {
+// runCleanupWithFlags executes the cleanup logic with the resolved apply,
+// force, and remote flags.
+func runCleanupWithFlags(ctx context.Context, out io.Writer, apply, force, remote bool) int {
 	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
 	_, l, err := resolveWarpLocation(ctx)
 	if err != nil {
@@ -773,17 +802,65 @@ func runCleanupWithFlags(ctx context.Context, out io.Writer, apply, force bool)
 
 	top := fabricengine.NewTopology(cfg)
 
-	r, err := top.Cleanup(l, apply, force)
+	r, err := top.Cleanup(l, apply, force, remote)
 	if err != nil {
 		return errWithRecord(out, r.Mutated(), err)
 	}
-	return okWithRecord(out, r.Mutated(), map[string]any{
-		"entries": r.Entries,
-	})
+
+	// fields, not CleanupResult, is what actually marshals — remote_skipped_reason reaches the
+	// envelope because the map names it, regardless of the struct's own omitempty tag.
+	fields := map[string]any{
+		"entries":               r.Entries,
+		"remote_skipped_reason": r.RemoteSkippedReason,
+	}
+
+	// Error is always a genuine failure here, never a designed refusal — Cleanup sets no Error on a
+	// protected or unmanaged entry — unlike prune's Error, which stays in doc.go's carve-out.
+	var localFailedBranches, remoteFailedBranches []string
+	var attemptedLocal, attempted int
+	for _, entry := range r.Entries {
+		if entry.Error != "" {
+			localFailedBranches = append(localFailedBranches, entry.Branch)
+		}
+		if entry.Error != "" || entry.Deleted {
+			attemptedLocal++
+		}
+		if entry.RemoteError != "" {
+			remoteFailedBranches = append(remoteFailedBranches, entry.Branch)
+		}
+		if entry.Deleted {
+			attempted++
+		}
+	}
+
+	var synthesised error
+	switch {
+	case len(localFailedBranches) > 0 && len(remoteFailedBranches) > 0:
+		localErr := fmt.Errorf(
+			"branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].error",
+			len(localFailedBranches), attemptedLocal, strings.Join(localFailedBranches, ", "))
+		remoteErr := fmt.Errorf(
+			"remote branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].remote_error",
+			len(remoteFailedBranches), attempted, strings.Join(remoteFailedBranches, ", "))
+		synthesised = fmt.Errorf("%v; additionally, %v", localErr, remoteErr)
+	case len(localFailedBranches) > 0:
+		synthesised = fmt.Errorf(
+			"branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].error",
+			len(localFailedBranches), attemptedLocal, strings.Join(localFailedBranches, ", "))
+	case len(remoteFailedBranches) > 0:
+		synthesised = fmt.Errorf(
+			"remote branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].remote_error",
+			len(remoteFailedBranches), attempted, strings.Join(remoteFailedBranches, ", "))
+	}
+
+	if synthesised != nil {
+		return errWithRecordFields(out, r.Mutated(), synthesised, fields)
+	}
+	return okWithRecord(out, r.Mutated(), fields)
 }
 
-// runRemoveWithFlag executes the remove logic with the resolved force flag.
-func runRemoveWithFlag(ctx context.Context, out io.Writer, args []string, force bool) int {
+// runRemoveWithFlag executes the remove logic with the resolved force and remote flags.
+func runRemoveWithFlag(ctx context.Context, out io.Writer, args []string, force, remote bool) int {
 	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
 	_, l, err := resolveWarpLocation(ctx)
 	if err != nil {
@@ -799,19 +876,39 @@ func runRemoveWithFlag(ctx context.Context, out io.Writer, args []string, force
 
 	// args[0] is the slug; cobra has already consumed "remove" from the argument list.
 	if len(args) < 1 {
-		return output.Err(out, "usage: lyx fabric remove [--force] <slug>")
+		return output.Err(out, "usage: lyx fabric remove [--force] [--remote] <slug>")
 	}
 	slug := args[0]
 
-	r, err := top.Remove(l, slug, force)
+	r, err := top.Remove(l, slug, force, remote)
 	if err != nil {
 		return errWithRecord(out, r.Mutated(), err)
 	}
-	return okWithRecord(out, r.Mutated(), map[string]any{
-		"slug":          r.Slug,
-		"path":          r.Path,
-		"links_removed": r.LinksRemoved,
-	})
+
+	// New RemoveResult fields must be added to this map explicitly — it is hand-built, not reflected.
+	fields := map[string]any{
+		"slug":                  r.Slug,
+		"path":                  r.Path,
+		"links_removed":         r.LinksRemoved,
+		"remote_branch_deleted": r.RemoteBranchDeleted,
+		"remote_branch_error":   r.RemoteBranchError,
+		"remote_skipped_reason": r.RemoteSkippedReason,
+	}
+
+	// Keyed on RemoteBranchError alone — never on RemoteSkippedReason — so that a missing origin
+	// produces exit 0 here exactly as it does from cleanup, and the identical configuration state
+	// never yields two different verdicts across the two verbs.
+	if r.RemoteBranchError != "" {
+		weftBranch := fabricengine.WeftBranchName(cfg.BranchPrefix + slug)
+		// "origin" is hardcoded here rather than referencing fabricengine's own unexported
+		// originRemoteName, which stays unexported: exporting it just to spell this one error string
+		// would widen the engine's API for no caller that needs it.
+		synthesised := fmt.Errorf(
+			"weft branch %q was deleted locally, but its copy on %q was not: %s",
+			weftBranch, "origin", r.RemoteBranchError)
+		return errWithRecordFields(out, r.Mutated(), synthesised, fields)
+	}
+	return okWithRecord(out, r.Mutated(), fields)
 }
 
 // addOptionsFromEnv returns the AddOptions for a CLI-driven `lyx fabric add`,
diff --git a/internal/fabriccli/remoteenvelope_integration_test.go b/internal/fabriccli/remoteenvelope_integration_test.go
new file mode 100644
index 000000000..b8d80797d
--- /dev/null
+++ b/internal/fabriccli/remoteenvelope_integration_test.go
@@ -0,0 +1,403 @@
+//go:build integration
+
+// remoteenvelope_integration_test.go covers `--remote`'s CLI-level envelope and exit-code contract on
+// `lyx fabric cleanup` and `lyx fabric remove`: the flag-matrix corner where --remote alone is still a
+// dry run, the hand-built remove fields map actually carrying its new keys, cleanup's remote and local
+// failure exit codes, the protected/unmanaged carve-outs staying exit-0, prune's untouched carve-out,
+// and the no-origin skip reason exiting 0 symmetrically across both verbs. An exit code is observable
+// only through the CLI seam, which is why none of these scenarios could be covered in batch 3's
+// fabricengine-level tests.
+//
+// Package fabriccli_test, sharing the single TestMain in testmain_test.go and driving the real CLI
+// through the same RunCLIIn seam envelopecontract_integration_test.go uses, with every hub built
+// through hubforge.NewHub per the hubforge Fabric-Fixture Invariant.
+
+package fabriccli_test
+
+import (
+	"bytes"
+	"encoding/json"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fabriccli"
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/gitkit"
+	"github.com/Knatte18/loomyard/internal/hubforge"
+)
+
+// remoteEnvelopeCreateOrphanBranch creates branch in the weft repo at weftRoot, pointed at HEAD, with
+// no worktree checkout — the shape Cleanup treats as an orphan candidate once it also carries the
+// "-weft" suffix WeftWarpSlug requires.
+func remoteEnvelopeCreateOrphanBranch(t *testing.T, weftRoot, branch string) {
+	t.Helper()
+	gitkit.MustRun(t, weftRoot, "git", "branch", branch, "HEAD")
+}
+
+// remoteEnvelopePushBranch pushes branch from repoRoot to its configured origin remote.
+func remoteEnvelopePushBranch(t *testing.T, repoRoot, branch string) {
+	t.Helper()
+	gitkit.MustRun(t, repoRoot, "git", "push", "origin", branch)
+}
+
+// remoteEnvelopeBreakOrigin points repoRoot's origin remote at a filesystem path that does not exist,
+// so a push against it fails locally with no network involved.
+func remoteEnvelopeBreakOrigin(t *testing.T, repoRoot string) {
+	t.Helper()
+	gitkit.MustRun(t, repoRoot, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote.git"))
+}
+
+// remoteEnvelopeRemoveOrigin removes repoRoot's origin remote entirely.
+func remoteEnvelopeRemoveOrigin(t *testing.T, repoRoot string) {
+	t.Helper()
+	gitkit.MustRun(t, repoRoot, "git", "remote", "remove", "origin")
+}
+
+// remoteEnvelopeBranchExists reports whether branch exists at repoRoot.
+func remoteEnvelopeBranchExists(t *testing.T, repoRoot, branch string) bool {
+	t.Helper()
+	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
+	cmd.Dir = repoRoot
+	return cmd.Run() == nil
+}
+
+// remoteEnvelopeDecode unmarshals out into a JSON envelope map, failing the test on decode error.
+func remoteEnvelopeDecode(t *testing.T, out *bytes.Buffer) map[string]any {
+	t.Helper()
+	var envelope map[string]any
+	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
+		t.Fatalf("decode envelope: %v\noutput: %s", err, out.String())
+	}
+	return envelope
+}
+
+// TestRunCLI_CleanupRemoteAloneWithoutApplyDeletesNothing covers scenario 1: --remote alone on
+// cleanup, without --apply, performs no deletion on either side and exits 0 — the flag-matrix corner
+// an operator is most likely to get wrong, and the one the help text now promises explicitly.
+func TestRunCLI_CleanupRemoteAloneWithoutApplyDeletesNothing(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+
+	branch := fabricengine.WeftBranchName("cli-remote-no-apply")
+	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
+	remoteEnvelopePushBranch(t, weftRoot, branch)
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--remote"})
+	if exitCode != 0 {
+		t.Fatalf("RunCLI(cleanup --remote) = %d; want 0\noutput: %s", exitCode, out.String())
+	}
+
+	if !remoteEnvelopeBranchExists(t, weftRoot, branch) {
+		t.Errorf("branch %q was removed locally by cleanup --remote with no --apply", branch)
+	}
+	if !remoteEnvelopeBranchExists(t, h.WeftBare, branch) {
+		t.Errorf("branch %q was removed on the remote by cleanup --remote with no --apply", branch)
+	}
+}
+
+// TestRunCLI_RemoveRemoteSuccessEnvelopeCarriesRemoteBranchDeleted covers scenario 2: remove
+// --remote's success envelope carries remote_branch_deleted — the regression guard for the hand-built
+// fields map, which would otherwise drop every new field silently while the struct still declared
+// them. It asserts the key's presence, not only its value, so a dropped key fails rather than reading
+// as false.
+func TestRunCLI_RemoveRemoteSuccessEnvelopeCarriesRemoteBranchDeleted(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	const slug = "cli-remove-remote"
+
+	var addOut bytes.Buffer
+	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &addOut, []string{"add", slug}); exitCode != 0 {
+		t.Fatalf("RunCLI(add %s) = %d; want 0\noutput: %s", slug, exitCode, addOut.String())
+	}
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", "--remote", slug})
+	if exitCode != 0 {
+		t.Fatalf("RunCLI(remove --remote %s) = %d; want 0\noutput: %s", slug, exitCode, out.String())
+	}
+
+	envelope := remoteEnvelopeDecode(t, &out)
+	if _, present := envelope["remote_branch_deleted"]; !present {
+		t.Errorf("envelope has no \"remote_branch_deleted\" key\noutput: %s", out.String())
+	}
+	if deleted, _ := envelope["remote_branch_deleted"].(bool); !deleted {
+		t.Errorf("envelope remote_branch_deleted = %v; want true", envelope["remote_branch_deleted"])
+	}
+}
+
+// TestRunCLI_CleanupRemoteFailureExitsNonZero covers scenario 3: a cleanup run where one entry
+// carries a RemoteError exits non-zero, emits "ok":false and "partial":true, and still carries the
+// full entries array with that entry's remote_error populated. Induced by pointing the weft repo's
+// origin at a filesystem path that does not exist. Asserts the envelope carries no "refusal" key — a
+// synthesised fmt.Errorf can never match RefusalOf.
+func TestRunCLI_CleanupRemoteFailureExitsNonZero(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+
+	branch := fabricengine.WeftBranchName("cli-cleanup-remote-fail")
+	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
+	remoteEnvelopeBreakOrigin(t, weftRoot)
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply", "--remote"})
+	if exitCode != 1 {
+		t.Fatalf("RunCLI(cleanup --apply --remote) = %d; want 1\noutput: %s", exitCode, out.String())
+	}
+
+	envelope := remoteEnvelopeDecode(t, &out)
+	if ok, _ := envelope["ok"].(bool); ok {
+		t.Errorf("envelope ok = true; want false\noutput: %s", out.String())
+	}
+	if partial, _ := envelope["partial"].(bool); !partial {
+		t.Errorf("envelope partial = %v; want true\noutput: %s", envelope["partial"], out.String())
+	}
+	if _, present := envelope["refusal"]; present {
+		t.Errorf("envelope carries a \"refusal\" key; want none — a synthesised error is never a gate refusal\noutput: %s", out.String())
+	}
+
+	entries, ok := envelope["entries"].([]any)
+	if !ok || len(entries) == 0 {
+		t.Fatalf("envelope has no non-empty \"entries\" array\noutput: %s", out.String())
+	}
+	var sawRemoteError bool
+	for _, raw := range entries {
+		entry, isMap := raw.(map[string]any)
+		if !isMap {
+			continue
+		}
+		if entryBranch, _ := entry["branch"].(string); entryBranch == branch {
+			if reason, _ := entry["remote_error"].(string); reason != "" {
+				sawRemoteError = true
+			}
+		}
+	}
+	if !sawRemoteError {
+		t.Errorf("no entry carries a \"remote_error\"; want the failing branch's own reason\noutput: %s", out.String())
+	}
+}
+
+// TestRunCLI_CleanupAllProtectedEntriesExitZero covers scenario 4: a cleanup run whose entries are
+// all Protected — here the primary weft branch (always present) plus an unmanaged legacy branch — and
+// therefore carry no Error at all, still exits 0. Asserts Protected with an EMPTY Error: Cleanup never
+// sets Error on a protected or unmanaged entry.
+func TestRunCLI_CleanupAllProtectedEntriesExitZero(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+
+	// No "-weft" suffix: unmanaged, reported but never deletable — WeftWarpSlug rejects it.
+	const unmanagedBranch = "cli-cleanup-unmanaged-legacy"
+	remoteEnvelopeCreateOrphanBranch(t, weftRoot, unmanagedBranch)
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply"})
+	if exitCode != 0 {
+		t.Fatalf("RunCLI(cleanup --apply) with only protected/unmanaged entries = %d; want 0\noutput: %s", exitCode, out.String())
+	}
+
+	envelope := remoteEnvelopeDecode(t, &out)
+	entries, ok := envelope["entries"].([]any)
+	if !ok || len(entries) == 0 {
+		t.Fatalf("envelope has no non-empty \"entries\" array\noutput: %s", out.String())
+	}
+	var sawProtected bool
+	for _, raw := range entries {
+		entry, isMap := raw.(map[string]any)
+		if !isMap {
+			continue
+		}
+		protected, _ := entry["protected"].(bool)
+		if !protected {
+			t.Fatalf("entry %v is not protected; want every entry protected in this fixture\noutput: %s", entry, out.String())
+		}
+		sawProtected = true
+		if errStr, _ := entry["error"].(string); errStr != "" {
+			t.Errorf("protected entry %v carries a non-empty \"error\"; want empty — Cleanup never sets Error on a protected entry", entry)
+		}
+	}
+	if !sawProtected {
+		t.Errorf("no entry reported at all\noutput: %s", out.String())
+	}
+}
+
+// TestRunCLI_CleanupLocalFailureExitsNonZero covers scenario 5: a cleanup run where one entry
+// carries a local Error — the git branch -D itself failed — exits non-zero. This is the one
+// existing-path change this task makes deliberately, and it needs its own pinned test because
+// nothing else in the suite would notice the verdict flipping back. Induced by pre-creating the
+// branch's own ref lock file, so `git branch -D` cannot acquire the lock it needs.
+func TestRunCLI_CleanupLocalFailureExitsNonZero(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+
+	branch := fabricengine.WeftBranchName("cli-cleanup-local-fail")
+	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
+
+	lockPath := filepath.Join(weftRoot, ".git", "refs", "heads", branch+".lock")
+	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
+		t.Fatalf("write ref lock %s: %v", lockPath, err)
+	}
+	t.Cleanup(func() { os.Remove(lockPath) })
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply"})
+	if exitCode != 1 {
+		t.Fatalf("RunCLI(cleanup --apply) with a locked ref = %d; want 1\noutput: %s", exitCode, out.String())
+	}
+
+	envelope := remoteEnvelopeDecode(t, &out)
+	if ok, _ := envelope["ok"].(bool); ok {
+		t.Errorf("envelope ok = true; want false\noutput: %s", out.String())
+	}
+
+	entries, ok := envelope["entries"].([]any)
+	if !ok {
+		t.Fatalf("envelope has no \"entries\" array\noutput: %s", out.String())
+	}
+	var sawLocalError bool
+	for _, raw := range entries {
+		entry, isMap := raw.(map[string]any)
+		if !isMap {
+			continue
+		}
+		if b, _ := entry["branch"].(string); b == branch {
+			if reason, _ := entry["error"].(string); reason != "" {
+				sawLocalError = true
+			}
+		}
+	}
+	if !sawLocalError {
+		t.Errorf("no entry for %q carries an \"error\"; want the local git branch -D failure reason\noutput: %s", branch, out.String())
+	}
+}
+
+// TestRunCLI_PruneProtectedOrUnownedEntryStillExitsZero covers scenario 6: a prune run with a
+// Protected or Unowned entry still exits 0 — the regression guard that prune's carve-out survived
+// untouched while cleanup left it.
+func TestRunCLI_PruneProtectedOrUnownedEntryStillExitsZero(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+
+	// An ordinary operator directory that is not a git checkout at all and was never fabric's, named
+	// so WeftWarpSlug accepts it and prune's orphan pass enumerates it as Unowned.
+	unowned := filepath.Join(h.Path, "cli-prune-unowned-weft")
+	if err := os.MkdirAll(unowned, 0o755); err != nil {
+		t.Fatalf("create unowned hub directory: %v", err)
+	}
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"prune"})
+	if exitCode != 0 {
+		t.Fatalf("RunCLI(prune) with an unowned entry = %d; want 0\noutput: %s", exitCode, out.String())
+	}
+
+	envelope := remoteEnvelopeDecode(t, &out)
+	if ok, _ := envelope["ok"].(bool); !ok {
+		t.Errorf("envelope ok = false; want true\noutput: %s", out.String())
+	}
+
+	entries, ok := envelope["entries"].([]any)
+	if !ok {
+		t.Fatalf("envelope has no \"entries\" array\noutput: %s", out.String())
+	}
+	var sawUnowned bool
+	for _, raw := range entries {
+		entry, isMap := raw.(map[string]any)
+		if !isMap {
+			continue
+		}
+		if unownedFlag, _ := entry["unowned"].(bool); unownedFlag {
+			sawUnowned = true
+		}
+	}
+	if !sawUnowned {
+		t.Errorf("no entry reported \"unowned\":true\noutput: %s", out.String())
+	}
+}
+
+// TestRunCLI_CleanupNoOriginUnderApplyAndRemoteExitsZero and
+// TestRunCLI_RemoveNoOriginUnderRemoteExitsZero together cover the no-origin path from the CLI on
+// both verbs: a weft repo with its remote removed exits 0 with the reason in remote_skipped_reason
+// and, for cleanup, no entries[].remote_error. Both halves are needed — an asymmetric exit code for
+// one configuration state across the two verbs is the defect the
+// remote-failure-non-fatal-in-engine-fatal-in-cli Shared Decision exists to prevent, and only a
+// CLI-level test can observe an exit code at all.
+func TestRunCLI_CleanupNoOriginUnderApplyAndRemoteExitsZero(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+
+	branch := fabricengine.WeftBranchName("cli-cleanup-no-origin")
+	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
+	remoteEnvelopeRemoveOrigin(t, weftRoot)
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply", "--remote"})
+	if exitCode != 0 {
+		t.Fatalf("RunCLI(cleanup --apply --remote) with no origin = %d; want 0\noutput: %s", exitCode, out.String())
+	}
+
+	envelope := remoteEnvelopeDecode(t, &out)
+	reason, _ := envelope["remote_skipped_reason"].(string)
+	if reason == "" {
+		t.Errorf("envelope remote_skipped_reason is empty; want a reason naming the missing origin remote\noutput: %s", out.String())
+	}
+
+	entries, ok := envelope["entries"].([]any)
+	if !ok {
+		t.Fatalf("envelope has no \"entries\" array\noutput: %s", out.String())
+	}
+	for _, raw := range entries {
+		entry, isMap := raw.(map[string]any)
+		if !isMap {
+			continue
+		}
+		if reason, _ := entry["remote_error"].(string); reason != "" {
+			t.Errorf("entry %v carries a \"remote_error\"; want empty — the reason lives on the verb-level field, not here", entry)
+		}
+	}
+}
+
+// TestRunCLI_RemoveNoOriginUnderRemoteExitsZero is the second half described above, for remove.
+func TestRunCLI_RemoveNoOriginUnderRemoteExitsZero(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+	const slug = "cli-remove-no-origin"
+
+	var addOut bytes.Buffer
+	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &addOut, []string{"add", slug}); exitCode != 0 {
+		t.Fatalf("RunCLI(add %s) = %d; want 0\noutput: %s", slug, exitCode, addOut.String())
+	}
+	remoteEnvelopeRemoveOrigin(t, weftRoot)
+
+	var out bytes.Buffer
+	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", "--remote", slug})
+	if exitCode != 0 {
+		t.Fatalf("RunCLI(remove --remote %s) with no origin = %d; want 0\noutput: %s", slug, exitCode, out.String())
+	}
+
+	envelope := remoteEnvelopeDecode(t, &out)
+	reason, _ := envelope["remote_skipped_reason"].(string)
+	if reason == "" {
+		t.Errorf("envelope remote_skipped_reason is empty; want a reason naming the missing origin remote\noutput: %s", out.String())
+	}
+	if remoteErr, _ := envelope["remote_branch_error"].(string); remoteErr != "" {
+		t.Errorf("envelope remote_branch_error = %q; want empty when the pre-check itself skipped", remoteErr)
+	}
+}
diff --git a/internal/fabricengine/add.go b/internal/fabricengine/add.go
index d60bcaeef..fac9f7c10 100644
--- a/internal/fabricengine/add.go
+++ b/internal/fabricengine/add.go
@@ -261,7 +261,12 @@ func (t *Topology) rollbackAdd(rec *Mutations, l *lyxcwd.Location, slug, warpBra
 
 	// (1) Remove the weft worktree; delete the weft branch only when this Add
 	// created it, so a rollback never destroys pre-existing weft history.
-	if err := removeWeftWorktree(rec, l, slug, weftBranch, true, !weftBranchAdopted, t.cfg.BranchPrefix); err != nil {
+	// remote is hardcoded false here — not because the branch was never pushed (Add pushes the warp
+	// branch at step (11) and the weft branch at step (12), so a rollback can genuinely face an
+	// already-pushed branch on either side), but because an unattended best-effort rollback on an
+	// error path the operator did not choose must not make a network-visible destructive change:
+	// --remote is opt-in precisely because deleting a shared ref needs an explicit operator decision.
+	if _, err := removeWeftWorktree(rec, l, slug, weftBranch, true, !weftBranchAdopted, false, t.cfg.BranchPrefix); err != nil {
 		if firstErr == nil {
 			firstErr = err
 		}
diff --git a/internal/fabricengine/cleanup.go b/internal/fabricengine/cleanup.go
index d9c98782c..49907b684 100644
--- a/internal/fabricengine/cleanup.go
+++ b/internal/fabricengine/cleanup.go
@@ -6,6 +6,10 @@
 //   - apply == true  → deletes every orphan weft branch that is not the primary weft branch, not
 //     checked out at a worktree, and not unmanaged (no "-weft" suffix, e.g. inherited from history
 //     predating fabric's uniform naming scheme).
+//   - remote is independent of --force and requires apply to delete anything (remote alone with
+//     apply false is still a dry run): when true, every orphan weft branch actually deleted locally
+//     is also deleted on the weft repo's origin remote. A weft repo with no origin remote configured
+//     reports that once per verb, on the result's RemoteSkippedReason, rather than once per branch.
 //
 // force is reserved and currently consulted by no gate in this verb; see Topology.Cleanup's own
 // doc comment.
@@ -58,6 +62,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
+	"github.com/Knatte18/loomyard/internal/gitrepo"
 	"github.com/Knatte18/loomyard/internal/lyxcwd"
 )
 
@@ -74,8 +79,17 @@ type CleanupBranchEntry struct {
 	// predating fabric's uniform naming scheme), or because the branch is
 	// currently checked out at a worktree (git branch -D could never delete it).
 	Protected bool `json:"protected,omitempty"`
-	// Error is non-empty when apply is true and branch deletion failed.
+	// Error is non-empty when apply is true and branch deletion failed. A non-empty Error makes
+	// `lyx fabric cleanup --apply` exit non-zero, because it names a genuine failure of the local
+	// `git branch -D` rather than a designed refusal — Protected and unmanaged entries set no Error
+	// at all.
 	Error string `json:"error,omitempty"`
+	// RemoteDeleted reports whether this branch's copy on the remote was observably removed. It is
+	// true only when the remote deletion was attempted and actually removed a ref.
+	RemoteDeleted bool `json:"remote_deleted,omitempty"`
+	// RemoteError is non-empty when the remote deletion was attempted and did not succeed. Its text
+	// always names the layer that said no — the gate's own refusal, or the remote deletion itself.
+	RemoteError string `json:"remote_error,omitempty"`
 }
 
 // CleanupResult is the top-level result type returned by Cleanup.
@@ -85,6 +99,11 @@ type CleanupResult struct {
 	MutationRecord
 	// Entries lists the orphaned weft branches and their dispositions.
 	Entries []CleanupBranchEntry `json:"entries"`
+	// RemoteSkippedReason carries a once-per-verb reason no remote deletion was attempted at all —
+	// today only a weft repo with no origin remote configured. It is deliberately not RemoteError,
+	// because RemoteError is the field the CLI switches its exit code on and a missing origin must
+	// exit 0.
+	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
 }
 
 // Cleanup finds weft branches with no corresponding warp worktree sibling and reports or deletes
@@ -93,7 +112,10 @@ type CleanupResult struct {
 // see primaryWeftBranch for why branch-space liveness alone cannot protect it.
 // force is reserved and currently consulted by no gate in this verb: deleteWeftBranch already
 // hardcodes force: false for its own request, and that stays true.
-func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force bool) (res CleanupResult, err error) {
+// remote gates whether an orphan weft branch actually deleted locally is also deleted on the weft
+// repo's origin remote; see this file's header for the flag matrix. A remote deletion failure never
+// makes Cleanup return a non-nil error — it is recorded on the entry and the sweep continues.
+func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force, remote bool) (res CleanupResult, err error) {
 	rec := NewMutations(l.HubPath)
 	defer func() { res.Mutations = rec.Snapshot() }()
 
@@ -108,6 +130,29 @@ func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force bool) (res CleanupRe
 		return CleanupResult{}, err
 	}
 
+	// The no-origin pre-check runs once per call, ahead of the per-branch loop, and only when remote
+	// is true. It runs under a dry run too, since RemoteURL is a go-git local config read that spawns
+	// no process and contacts no network, so telling the operator up front that --apply --remote
+	// would do nothing remotely is the whole value of reporting it. With remote false the pre-check
+	// does not run at all, so a plain cleanup --apply against a remoteless repo reports no reason.
+	var result CleanupResult
+	remoteOK := false
+	var weftRepoRootForRemote string
+	if remote {
+		weftRepoRoot, weftRootErr := WeftRepoRoot(l)
+		if weftRootErr != nil {
+			result.RemoteSkippedReason = fmt.Sprintf(
+				"no remote deletion attempted: cannot resolve the weft repo root: %v", weftRootErr)
+		} else if _, urlErr := gitrepo.New(weftRepoRoot).RemoteURL(originRemoteName); urlErr != nil {
+			result.RemoteSkippedReason = fmt.Sprintf(
+				"no remote deletion attempted: the weft repo has no %q remote configured: %v",
+				originRemoteName, urlErr)
+		} else {
+			remoteOK = true
+			weftRepoRootForRemote = weftRepoRoot
+		}
+	}
+
 	// Build the set of live warp branches; unreadable branches (stale registrations) skip.
 	liveWarpBranches := make(map[string]bool, len(entries))
 	for _, entry := range entries {
@@ -125,8 +170,6 @@ func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force bool) (res CleanupRe
 		return CleanupResult{}, fmt.Errorf("list weft branches: %w", err)
 	}
 
-	var result CleanupResult
-
 	for _, weftBranch := range weftBranches {
 		branch := weftBranch.Branch
 
@@ -171,6 +214,9 @@ func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force bool) (res CleanupRe
 		}
 
 		entry.Deleted = deleteWeftBranch(rec, l, branch, t.cfg.BranchPrefix, &entry)
+		if entry.Deleted && remote && remoteOK {
+			deleteWeftBranchOnRemote(rec, l, branch, t.cfg.BranchPrefix, weftRepoRootForRemote, &entry)
+		}
 		result.Entries = append(result.Entries, entry)
 	}
 
@@ -272,3 +318,31 @@ func deleteWeftBranch(rec *Mutations, l *lyxcwd.Location, branch, branchPrefix s
 	}
 	return true
 }
+
+// deleteWeftBranchOnRemote deletes branch on the weft repo's origin remote through the gate's
+// deleteRemoteBranch executor, recording the outcome on entry. It runs only after deleteWeftBranch
+// has already returned true for the same branch: the gate's ownership and dirtiness answers come
+// from local state, so a branch it refuses to delete locally must never lose its remote copy either.
+// A remote failure never aborts the sweep. It distinguishes a gate refusal from an operational
+// failure so entry.RemoteError always names the layer that actually said no.
+func deleteWeftBranchOnRemote(rec *Mutations, l *lyxcwd.Location, branch, branchPrefix, weftRepoRoot string, entry *CleanupBranchEntry) {
+	req := remoteBranchRequest{
+		what:      "delete weft branch on remote",
+		repoDir:   weftRepoRoot,
+		remote:    originRemoteName,
+		branch:    branch,
+		ownership: ownedManagedBranch(l, branchPrefix),
+		dirtiness: dirtyCheckedOutBranch(),
+		force:     false,
+	}
+	deleted, err := deleteRemoteBranch(rec, req)
+	if err != nil {
+		if refusal, ok := RefusalOf(err); ok {
+			entry.RemoteError = fmt.Sprintf("gate refused remote deletion of %q: %s", branch, refusal.Reason)
+		} else {
+			entry.RemoteError = fmt.Sprintf("delete remote branch %q on %q failed: %v", branch, originRemoteName, err)
+		}
+		return
+	}
+	entry.RemoteDeleted = deleted
+}
diff --git a/internal/fabricengine/cleanup_primary_integration_test.go b/internal/fabricengine/cleanup_primary_integration_test.go
index a065c3f78..37114fb2c 100644
--- a/internal/fabricengine/cleanup_primary_integration_test.go
+++ b/internal/fabricengine/cleanup_primary_integration_test.go
@@ -38,7 +38,7 @@ func TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout(t *testing.T) {
 
 	topology := fabricengine.NewTopology(fabricengine.Config{})
 
-	result, err := topology.Cleanup(l, true, true)
+	result, err := topology.Cleanup(l, true, true, false)
 	if err != nil {
 		t.Fatalf("Cleanup(apply=true, force=true) error = %v", err)
 	}
@@ -77,7 +77,7 @@ func TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable(t *testing.T) {
 	}
 
 	topology := fabricengine.NewTopology(fabricengine.Config{})
-	if _, err := topology.Cleanup(fixture.Layout, true, true); err == nil {
+	if _, err := topology.Cleanup(fixture.Layout, true, true, false); err == nil {
 		t.Fatal("Cleanup() = nil error; want a refusal when the primary weft branch cannot be determined")
 	}
 }
diff --git a/internal/fabricengine/cleanupremote_integration_test.go b/internal/fabricengine/cleanupremote_integration_test.go
new file mode 100644
index 000000000..66400045c
--- /dev/null
+++ b/internal/fabricengine/cleanupremote_integration_test.go
@@ -0,0 +1,355 @@
+//go:build integration
+
+// cleanupremote_integration_test.go covers Cleanup's remote branch deletion under --remote: the
+// opt-in deletion of an orphan weft branch's copy on the remote, its default-off regression guard,
+// the dry-run no-op, the idempotent never-pushed path, the protected-entry carve-out, the non-fatal
+// remote failure, and the once-per-verb no-origin pre-check across every combination of apply and
+// remote.
+//
+// Every hub here is built through hubforge.NewHub per the hubforge Fabric-Fixture Invariant (via
+// newFabricFixture), using the hub's own WeftBare field as the weft remote to assert against — this
+// hub's private copy of the weft bare remote, so a test can push an orphan branch to it and then
+// assert the ref is gone.
+//
+// Package fabricengine_test to reuse newFabricFixture (reconcile_stale_registration_test.go) and
+// mustWeftRepoRoot/branchExistsAt (add_rollback_adopt_test.go / reconcile_stale_registration_test.go)
+// — every assertion here goes through exported API; shares the single TestMain in testmain_test.go.
+
+package fabricengine_test
+
+import (
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/gitkit"
+)
+
+// mustCreateOrphanWeftBranch creates branch in the weft repo at weftRoot, pointed at HEAD, with no
+// worktree checkout — the shape Cleanup treats as an orphan candidate once it also carries the
+// "-weft" suffix WeftWarpSlug requires.
+func mustCreateOrphanWeftBranch(t *testing.T, weftRoot, branch string) {
+	t.Helper()
+	gitkit.MustRun(t, weftRoot, "git", "branch", branch, "HEAD")
+}
+
+// mustPushBranch pushes branch from repoRoot to its configured origin remote.
+func mustPushBranch(t *testing.T, repoRoot, branch string) {
+	t.Helper()
+	gitkit.MustRun(t, repoRoot, "git", "push", "origin", branch)
+}
+
+// mustBreakOrigin points repoRoot's origin remote at a filesystem path that does not exist, so a
+// push against it fails locally with no network involved.
+func mustBreakOrigin(t *testing.T, repoRoot string) {
+	t.Helper()
+	gitkit.MustRun(t, repoRoot, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote.git"))
+}
+
+// mustRemoveOrigin removes repoRoot's origin remote entirely.
+func mustRemoveOrigin(t *testing.T, repoRoot string) {
+	t.Helper()
+	gitkit.MustRun(t, repoRoot, "git", "remote", "remove", "origin")
+}
+
+// TestCleanup_RemoteTrueDeletesLocalAndRemoteOrphan covers case 1: apply and remote both true delete
+// both the local orphan weft branch and its copy on the remote.
+func TestCleanup_RemoteTrueDeletesLocalAndRemoteOrphan(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-remote-both-weft"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	mustPushBranch(t, weftRoot, branch)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, true, false, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
+	}
+
+	entry := findCleanupEntry(t, res.Entries, branch)
+	if !entry.Deleted {
+		t.Errorf("entry.Deleted = false; want true")
+	}
+	if !entry.RemoteDeleted {
+		t.Errorf("entry.RemoteDeleted = false; want true")
+	}
+	if entry.RemoteError != "" {
+		t.Errorf("entry.RemoteError = %q; want empty", entry.RemoteError)
+	}
+	if branchExistsAt(t, weftRoot, branch) {
+		t.Errorf("branch %q still exists locally after Cleanup(apply=true)", branch)
+	}
+	if branchExistsAt(t, fixture.WeftBare, branch) {
+		t.Errorf("branch %q still exists on the remote after Cleanup(apply=true, remote=true)", branch)
+	}
+}
+
+// TestCleanup_RemoteFalseLeavesRemoteCopyIntact covers case 2: apply true and remote false is the
+// regression guard on the existing default — the feature is opt-in.
+func TestCleanup_RemoteFalseLeavesRemoteCopyIntact(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-remote-off-weft"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	mustPushBranch(t, weftRoot, branch)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, true, false, false)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, remote=false) error = %v", err)
+	}
+
+	entry := findCleanupEntry(t, res.Entries, branch)
+	if !entry.Deleted {
+		t.Errorf("entry.Deleted = false; want true")
+	}
+	if entry.RemoteDeleted {
+		t.Errorf("entry.RemoteDeleted = true; want false — remote is opt-in")
+	}
+	if !branchExistsAt(t, fixture.WeftBare, branch) {
+		t.Errorf("branch %q no longer exists on the remote after Cleanup(apply=true, remote=false); remote must be opt-in", branch)
+	}
+}
+
+// TestCleanup_DryRunWithRemoteDeletesNeither covers case 3: a dry run with remote true performs no
+// deletion on either side.
+func TestCleanup_DryRunWithRemoteDeletesNeither(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-dry-remote-weft"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	mustPushBranch(t, weftRoot, branch)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, false, false, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=false, remote=true) error = %v", err)
+	}
+
+	entry := findCleanupEntry(t, res.Entries, branch)
+	if entry.Deleted || entry.RemoteDeleted {
+		t.Errorf("entry = %+v; want no deletion on a dry run", entry)
+	}
+	if !branchExistsAt(t, weftRoot, branch) {
+		t.Errorf("branch %q was removed locally on a dry run", branch)
+	}
+	if !branchExistsAt(t, fixture.WeftBare, branch) {
+		t.Errorf("branch %q was removed on the remote on a dry run", branch)
+	}
+}
+
+// TestCleanup_NeverPushedOrphanIsIdempotentOnRemote covers case 4: an orphan branch that was never
+// pushed deletes locally, reports no remote deletion and no remote error, and records no
+// KindRemoteBranchDeleted entry.
+func TestCleanup_NeverPushedOrphanIsIdempotentOnRemote(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-never-pushed-weft"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	// Deliberately never pushed: the remote never had this ref to begin with.
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, true, false, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
+	}
+
+	entry := findCleanupEntry(t, res.Entries, branch)
+	if !entry.Deleted {
+		t.Errorf("entry.Deleted = false; want true")
+	}
+	if entry.RemoteDeleted {
+		t.Errorf("entry.RemoteDeleted = true; want false — the ref was never on the remote")
+	}
+	if entry.RemoteError != "" {
+		t.Errorf("entry.RemoteError = %q; want empty — an already-absent remote ref is idempotent success", entry.RemoteError)
+	}
+
+	for _, m := range res.Mutated().Entries() {
+		if m.Kind == fabricengine.KindRemoteBranchDeleted && m.Target == branch {
+			t.Errorf("mutation record has a %s entry for %q; want none, since nothing was deleted on the remote", fabricengine.KindRemoteBranchDeleted, branch)
+		}
+	}
+}
+
+// TestCleanup_ProtectedEntryUntouchedOnRemote covers case 5: a protected (here, unmanaged) branch
+// has neither its local nor its remote copy touched with remote true.
+func TestCleanup_ProtectedEntryUntouchedOnRemote(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-unmanaged-legacy"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	// No "-weft" suffix: unmanaged, reported but never deletable — WeftWarpSlug rejects it.
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	mustPushBranch(t, weftRoot, branch)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, true, false, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
+	}
+
+	entry := findCleanupEntry(t, res.Entries, branch)
+	if !entry.Protected {
+		t.Errorf("entry.Protected = false; want true — unmanaged branches are reported but never deleted")
+	}
+	if entry.Deleted || entry.RemoteDeleted {
+		t.Errorf("entry = %+v; want neither local nor remote deletion of a protected entry", entry)
+	}
+	if !branchExistsAt(t, weftRoot, branch) {
+		t.Errorf("protected branch %q was removed locally", branch)
+	}
+	if !branchExistsAt(t, fixture.WeftBare, branch) {
+		t.Errorf("protected branch %q was removed on the remote", branch)
+	}
+}
+
+// TestCleanup_RemoteFailureIsNonFatal covers case 6: a remote deletion failure leaves the verb
+// returning a nil error, the local branch deleted, and RemoteError populated. Induced by pointing
+// the weft repo's origin at a filesystem path that no longer exists, so no network is needed.
+func TestCleanup_RemoteFailureIsNonFatal(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-remote-fail-weft"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	mustBreakOrigin(t, weftRoot)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, true, false, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, remote=true) error = %v; want nil — a remote failure is non-fatal", err)
+	}
+
+	entry := findCleanupEntry(t, res.Entries, branch)
+	if !entry.Deleted {
+		t.Errorf("entry.Deleted = false; want true — the local deletion must still succeed")
+	}
+	if entry.RemoteError == "" {
+		t.Errorf("entry.RemoteError is empty; want a reason naming the remote deletion failure")
+	}
+	if branchExistsAt(t, weftRoot, branch) {
+		t.Errorf("branch %q still exists locally after Cleanup(apply=true)", branch)
+	}
+}
+
+// TestCleanup_NoOriginUnderApplyAndRemoteSkipsOnceReportsOnce covers case 7: a weft repo with no
+// origin configured, under apply and remote both true: every orphan weft branch is deleted locally,
+// RemoteSkippedReason is non-empty exactly once on the result, every entry's RemoteError is empty,
+// and the verb returns a nil error.
+func TestCleanup_NoOriginUnderApplyAndRemoteSkipsOnceReportsOnce(t *testing.T) {
+	t.Parallel()
+
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	branches := []string{"cleanup-no-origin-one-weft", "cleanup-no-origin-two-weft"}
+	for _, b := range branches {
+		mustCreateOrphanWeftBranch(t, weftRoot, b)
+	}
+	mustRemoveOrigin(t, weftRoot)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, true, false, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
+	}
+
+	if res.RemoteSkippedReason == "" {
+		t.Errorf("RemoteSkippedReason is empty; want a reason naming the missing origin remote")
+	}
+
+	for _, b := range branches {
+		entry := findCleanupEntry(t, res.Entries, b)
+		if !entry.Deleted {
+			t.Errorf("entry for %q: Deleted = false; want true — the local sweep must still run", b)
+		}
+		if entry.RemoteError != "" {
+			t.Errorf("entry for %q: RemoteError = %q; want empty — the reason lives on the verb-level field, not here", b, entry.RemoteError)
+		}
+		if branchExistsAt(t, weftRoot, b) {
+			t.Errorf("branch %q still exists locally after Cleanup(apply=true)", b)
+		}
+	}
+}
+
+// TestCleanup_NoOriginUnderRemoteWithoutApplyIsStillReportedAndDeletesNothing covers case 8: the
+// dry-run half of the no-origin pre-check.
+func TestCleanup_NoOriginUnderRemoteWithoutApplyIsStillReportedAndDeletesNothing(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-no-origin-dry-weft"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	mustRemoveOrigin(t, weftRoot)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, false, false, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=false, remote=true) error = %v", err)
+	}
+
+	if res.RemoteSkippedReason == "" {
+		t.Errorf("RemoteSkippedReason is empty; want a reason naming the missing origin remote, even on a dry run")
+	}
+
+	entry := findCleanupEntry(t, res.Entries, branch)
+	if entry.Deleted {
+		t.Errorf("entry.Deleted = true; want false — apply is false")
+	}
+	if !branchExistsAt(t, weftRoot, branch) {
+		t.Errorf("branch %q was removed on a dry run", branch)
+	}
+}
+
+// TestCleanup_NoOriginWithRemoteFalseReportsNoReason covers case 9: this guards the remote guard
+// itself — without it, a plain cleanup --apply against a remoteless repo would start reporting a
+// reason, an observable change to a path this task does not otherwise touch.
+func TestCleanup_NoOriginWithRemoteFalseReportsNoReason(t *testing.T) {
+	t.Parallel()
+
+	const branch = "cleanup-no-origin-remote-off-weft"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	mustCreateOrphanWeftBranch(t, weftRoot, branch)
+	mustRemoveOrigin(t, weftRoot)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	res, err := topology.Cleanup(l, true, false, false)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, remote=false) error = %v", err)
+	}
+
+	if res.RemoteSkippedReason != "" {
+		t.Errorf("RemoteSkippedReason = %q; want empty — remote is false, so the pre-check must not run at all", res.RemoteSkippedReason)
+	}
+}
diff --git a/internal/fabricengine/destroy.go b/internal/fabricengine/destroy.go
index 408ff9378..1d3828a8e 100644
--- a/internal/fabricengine/destroy.go
+++ b/internal/fabricengine/destroy.go
@@ -1,9 +1,10 @@
 // destroy.go is the only file in package fabricengine permitted to perform a destructive primitive.
-// The five primitives are: removing a path (os.RemoveAll/os.Remove), removing a git worktree (git
+// The six primitives are: removing a path (os.RemoveAll/os.Remove), removing a git worktree (git
 // worktree remove), removing or re-pointing a link (fslink.Remove), deleting a branch (git branch
-// -D), and resetting a warp checkout hard (ResetHard). Every one of them is reached only through one
-// of this file's executors, and every executor runs the shared check pipeline before performing its
-// act — the gate executes, it does not merely approve.
+// -D), deleting a branch on a remote (git push <remote> --delete), and resetting a warp checkout
+// hard (ResetHard). Every one of them is reached only through one of this file's executors, and
+// every executor runs the shared check pipeline before performing its act — the gate executes, it
+// does not merely approve.
 //
 // The pipeline runs four checks, always in this fixed order, stopping at the first failure:
 // containment, ownership, dirtiness, force.
@@ -50,12 +51,14 @@
 // See CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant (added once this slice's guard test
 // lands) for the machine-enforced half of this rule.
 //
-// Recording contract: every one of the eight executors below takes a leading `rec *Mutations`
+// Recording contract: every one of the nine executors below takes a leading `rec *Mutations`
 // parameter and appends its own primitive's entry itself, after the primitive observably changed
 // state — never before, and never for a no-op. A refusal records nothing, since nothing happened;
-// removeGitWorktree and deleteBranch record only when the underlying git command returned a nil
-// error, since a non-nil error — whether git ran and rejected the command or could not be run at all
-// — would otherwise claim a destruction that never occurred.
+// removeGitWorktree, deleteBranch, and deleteRemoteBranch record only when the underlying git command
+// returned a nil error, since a non-nil error — whether git ran and rejected the command or could not
+// be run at all — would otherwise claim a destruction that never occurred; deleteRemoteBranch
+// additionally requires an observed deletion rather than a nil error alone, since its own
+// nil-error-plus-deleted==false case is the already-absent idempotent success.
 // The parameter is explicit, never a request-type field,
 // because a missing struct field is a silent zero value the compiler accepts while a missing
 // parameter does not compile — this slice exists because a record was silently dropped, so the
@@ -81,6 +84,13 @@ import (
 	"github.com/Knatte18/loomyard/internal/lyxcwd"
 )
 
+// originRemoteName is the remote name deleteRemoteBranch always operates against.
+// fabric's geometry already hardcodes "origin" throughout — clone.go's weft adopt-or-create path
+// uses "refs/remotes/origin/" and "origin/<branch>" directly — so introducing configurability at
+// this one point alone would be inconsistent with the rest of the package and untested against any
+// other value.
+const originRemoteName = "origin"
+
 // Check names one of the three checks a destructive-gate refusal can be attributed to: containment,
 // ownership, or dirtiness. It is string-backed so a refusal renders and marshals without a
 // conversion step, and so RefusalOf's caller can read it with no import of anything unexported.
@@ -223,6 +233,32 @@ type branchRequest struct {
 	force bool
 }
 
+// remoteBranchRequest is the gate's request shape for the primitive whose target is a ref on a
+// remote: git push <remote> --delete. It is a distinct type from branchRequest, never a remote field
+// added to it, because branchRequest's own doc comment rejects a per-site empty-string sentinel for a
+// structurally-N/A field, and a remote that must be "" at branchRequest's four existing local call
+// sites is exactly that shape; keeping the two types distinct also keeps the two executors' call-site
+// sets disjoint and auditable. Like branchRequest, it carries no container and no target field:
+// containment is structurally N/A for a ref.
+type remoteBranchRequest struct {
+	// what names the act being attempted, for the refusal message.
+	what string
+	// repoDir is the weft repo the branch lives in — not a path being destroyed.
+	repoDir string
+	// remote is the remote name the branch is deleted from.
+	remote string
+	// branch is the branch name the executor will delete on remote.
+	branch string
+	// ownership declares which closed-enum ownership kind branch must satisfy.
+	ownership branchOwnership
+	// dirtiness declares which dirtiness probe the pipeline runs against branch.
+	dirtiness branchDirtiness
+	// force is reserved: every remoteBranchRequest construction in this package hardcodes it false
+	// today, exactly as branchRequest's own force field does, for the same reason — no call site's own
+	// gate currently answers to it.
+	force bool
+}
+
 // pathOwnershipKind enumerates the closed set of ownership predicates a pathRequest may declare.
 // It has meaning only inside this file; every kind is reached solely through its ownedXxx
 // constructor below, never constructed directly.
@@ -701,18 +737,49 @@ func checkBranchRequest(req branchRequest) error {
 // branchRequest construction in this package hardcodes force: false, and Cleanup's own force
 // parameter is likewise reserved and consulted by no gate (see cleanup.go).
 func checkBranchDirtiness(req branchRequest) error {
-	branches, err := listWeftBranches(req.ownership.location)
+	return checkedOutBranchDirtiness(req.ownership.location, req.what, req.branch)
+}
+
+// checkedOutBranchDirtiness is the checked-out-at-a-worktree dirtiness probe both branchRequest (via
+// checkBranchDirtiness) and remoteBranchRequest (via checkRemoteBranchRequest) run: is branch checked
+// out at any worktree. git branch -D cannot delete a checked-out branch anyway, so this converts
+// git's own refusal into a named gate refusal, the same move as re-gating removeWarpWorktreeDir's
+// fallback.
+// It keeps the *lyxcwd.Location access path checkBranchDirtiness already used, rather than adding a
+// location field to remoteBranchRequest — the three values here are everything the probe needs.
+func checkedOutBranchDirtiness(l *lyxcwd.Location, what, branch string) error {
+	branches, err := listWeftBranches(l)
 	if err != nil {
-		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: err.Error()}
+		return &destructiveRefusal{Check: CheckDirtiness, What: what, Target: branch, Reason: err.Error()}
 	}
 	for _, b := range branches {
-		if b.Branch == req.branch && b.WorktreePath != "" {
-			return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: fmt.Sprintf("branch is checked out at %s", b.WorktreePath)}
+		if b.Branch == branch && b.WorktreePath != "" {
+			return &destructiveRefusal{Check: CheckDirtiness, What: what, Target: branch, Reason: fmt.Sprintf("branch is checked out at %s", b.WorktreePath)}
 		}
 	}
 	return nil
 }
 
+// checkRemoteBranchRequest runs the gate's checks against req, structurally mirroring
+// checkBranchRequest: an unset ownership or dirtiness declaration is refused before either predicate
+// runs, then resolveBranchOwnership resolves ownership, then the same checked-out-at-a-worktree
+// dirtiness probe checkBranchDirtiness runs. Containment is structurally N/A here exactly as it is
+// for branchRequest, and there is no container field to check.
+func checkRemoteBranchRequest(req remoteBranchRequest) error {
+	if req.ownership.kind == branchOwnershipUnset {
+		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.branch, Reason: "no ownership kind declared"}
+	}
+	if req.dirtiness.kind == branchDirtinessUnset {
+		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "no dirtiness declared"}
+	}
+
+	if ok, reason := resolveBranchOwnership(req.ownership, req.branch); !ok {
+		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.branch, Reason: reason}
+	}
+
+	return checkedOutBranchDirtiness(req.ownership.location, req.what, req.branch)
+}
+
 // removeContainedPath removes the container-relative form of target through an os.Root rooted at
 // container, so that path-component resolution and the unlink are one openat chain that atomically
 // refuses to traverse a symlink escaping container.
@@ -890,6 +957,26 @@ func deleteBranch(rec *Mutations, req branchRequest) error {
 	return err
 }
 
+// deleteRemoteBranch is the executor for the git push <remote> --delete primitive: it runs the
+// pipeline, then calls gitrepo.New(req.repoDir).DeleteRemoteBranch(req.remote, req.branch).
+// It returns the (deleted, err) pair through unchanged, wrapping nothing — every call site builds its
+// own message from it, exactly as deleteBranch and removeGitWorktree already do.
+// It appends KindRemoteBranchDeleted to rec via AppendRef, not Append, only when err is nil AND
+// deleted is true: a remote ref is a ref, not a path, so it carries no hub-relative conversion, and
+// the append happens only on an observed deletion — never on deleted == false, which is the
+// already-absent idempotent success, and never on an error.
+func deleteRemoteBranch(rec *Mutations, req remoteBranchRequest) (deleted bool, err error) {
+	if checkErr := checkRemoteBranchRequest(req); checkErr != nil {
+		return false, checkErr
+	}
+
+	deleted, err = gitrepo.New(req.repoDir).DeleteRemoteBranch(req.remote, req.branch)
+	if err == nil && deleted {
+		rec.AppendRef(KindRemoteBranchDeleted, req.branch, req.remote)
+	}
+	return deleted, err
+}
+
 // createExclusiveDir creates path as a directory the gate can later authorise the removal of, and
 // returns the createdToken proving it.
 //
diff --git a/internal/fabricengine/destroy_containment_toctou_integration_test.go b/internal/fabricengine/destroy_containment_toctou_integration_test.go
index 26248b479..30c65c1c0 100644
--- a/internal/fabricengine/destroy_containment_toctou_integration_test.go
+++ b/internal/fabricengine/destroy_containment_toctou_integration_test.go
@@ -60,7 +60,7 @@ func TestRemove_DoesNotDeleteOutsideHubThroughLauncherSymlink(t *testing.T) {
 	}
 
 	// Remove --force must not carry the removal outside the hub through the escaping symlink.
-	_, _ = topology.Remove(l, slug, true)
+	_, _ = topology.Remove(l, slug, true, false)
 
 	for _, name := range canaries {
 		if _, err := os.Stat(filepath.Join(outside, name)); err != nil {
diff --git a/internal/fabricengine/destroy_test.go b/internal/fabricengine/destroy_test.go
index 7cd210097..31092c245 100644
--- a/internal/fabricengine/destroy_test.go
+++ b/internal/fabricengine/destroy_test.go
@@ -561,6 +561,55 @@ func TestGate_ZeroValueDeclarationsAreRefusals(t *testing.T) {
 		err := checkBranchRequest(req)
 		assertRefusalCheck(t, err, CheckDirtiness)
 	})
+
+	// The three subtests below cover remoteBranchRequest/checkRemoteBranchRequest, mirroring the
+	// branchRequest cases above exactly.
+
+	t.Run("RemoteBranchZeroOwnership", func(t *testing.T) {
+		req := remoteBranchRequest{
+			what:      "test",
+			repoDir:   t.TempDir(),
+			remote:    originRemoteName,
+			branch:    "task-weft",
+			dirtiness: dirtyCheckedOutBranch(),
+		}
+		err := checkRemoteBranchRequest(req)
+		assertRefusalCheck(t, err, CheckOwnership)
+	})
+
+	t.Run("RemoteBranchZeroDirtiness", func(t *testing.T) {
+		// A hand-built Location is fine here, for the same reason BranchZeroDirtiness's is: the
+		// zero-value dirtiness declaration refuses before resolveManagedBranch ever runs, so
+		// primaryWeftBranch (which spawns git) is never reached.
+		l := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "prime"}
+		req := remoteBranchRequest{
+			what:      "test",
+			repoDir:   t.TempDir(),
+			remote:    originRemoteName,
+			branch:    "task-weft",
+			ownership: ownedManagedBranch(l, ""),
+		}
+		err := checkRemoteBranchRequest(req)
+		assertRefusalCheck(t, err, CheckDirtiness)
+	})
+
+	t.Run("RemoteBranchNamingPredicateRefused", func(t *testing.T) {
+		// A branch name fabric's own scheme does not construct (no "-weft" suffix, no branchPrefix
+		// match) is refused by resolveManagedBranch's naming predicate before it spawns git at all —
+		// safe with a hand-built Location for the same reason the naming-predicate case is safe
+		// elsewhere in this file: the predicate short-circuits ahead of the first spawn.
+		l := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "prime"}
+		req := remoteBranchRequest{
+			what:      "test",
+			repoDir:   t.TempDir(),
+			remote:    originRemoteName,
+			branch:    "not-a-fabric-branch",
+			ownership: ownedManagedBranch(l, ""),
+			dirtiness: dirtyCheckedOutBranch(),
+		}
+		err := checkRemoteBranchRequest(req)
+		assertRefusalCheck(t, err, CheckOwnership)
+	})
 }
 
 // TestGate_AbsentTargetIsNoOp proves an absent target is a no-op success, for every ownership kind,
diff --git a/internal/fabricengine/destroyremote_integration_test.go b/internal/fabricengine/destroyremote_integration_test.go
new file mode 100644
index 000000000..fd0b8052c
--- /dev/null
+++ b/internal/fabricengine/destroyremote_integration_test.go
@@ -0,0 +1,74 @@
+//go:build integration
+
+// destroyremote_integration_test.go pins the remote-branch executor's own request shape via two
+// direct-call cases that need a real hub and therefore a real git spawn: an unset ownership/dirtiness
+// declaration and the naming-predicate refusal are hermetic and covered in destroy_test.go instead,
+// per the Test Tier Purity Invariant.
+//
+// Each case drives checkRemoteBranchRequest through CheckRemoteBranchRequestForTest
+// (export_test.go), never through deleteRemoteBranch itself — no test here asserts a ref was
+// actually deleted on a remote. Both are refusals the real call sites cannot reach: those run only
+// after the same branch's local `git branch -D` already succeeded, at which point the branch is
+// already gone locally, so its own dirtiness probe never sees it.
+//
+// Package fabricengine_test: building either case needs a real hub via hubforge.NewHub, but an
+// internal fabricengine test file cannot import internal/hubforge without closing an import cycle
+// (hubforge -> fabriccli -> fabricengine). Shares the single TestMain in testmain_test.go.
+
+package fabricengine_test
+
+import (
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/hubforge"
+)
+
+// TestDeleteRemoteBranchGate_PrimaryWeftBranchRefused proves the repo's primary weft branch is
+// refused with CheckOwnership, reaching primaryWeftBranch — a real git spawn only an integration-tier
+// test can exercise. This is a refusal the real call sites cannot reach: neither real call site ever
+// names the primary weft branch, since Cleanup itself never enumerates it as an orphan.
+func TestDeleteRemoteBranchGate_PrimaryWeftBranchRefused(t *testing.T) {
+	t.Parallel()
+
+	h := hubforge.NewHub(t, ".")
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+	primary := fabricengine.WeftBranchName("main")
+
+	err = fabricengine.CheckRemoteBranchRequestForTest(h.Location, weftRoot, "origin", primary, "")
+	if !RefusedByGate(err, fabricengine.CheckOwnership) {
+		t.Fatalf("CheckRemoteBranchRequestForTest(%q) = %v; want a CheckOwnership refusal", primary, err)
+	}
+}
+
+// TestDeleteRemoteBranchGate_CheckedOutBranchRefused proves a weft branch still checked out at a
+// worktree is refused with CheckOwnership (not CheckDirtiness), reaching listWeftBranches from
+// inside resolveManagedBranch — a real git spawn only an integration-tier test can exercise.
+// resolveManagedBranch's own checked-out-at-a-worktree check runs before checkBranchDirtiness's
+// duplicate check is ever reached, so the latter is unreachable dead code on an
+// ownedManagedBranch-typed request.
+//
+// This is also a refusal the real call sites cannot reach: they run only after the same branch's
+// local `git branch -D` already succeeded, at which point the branch is already gone locally and so
+// is no longer checked out anywhere.
+func TestDeleteRemoteBranchGate_CheckedOutBranchRefused(t *testing.T) {
+	t.Parallel()
+
+	const slug = "remoteslug"
+	h := hubforge.NewHub(t, ".")
+	hubforge.AddPair(t, h, slug)
+
+	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
+	if err != nil {
+		t.Fatalf("WeftRepoRoot: %v", err)
+	}
+	checkedOut := fabricengine.WeftBranchName(slug)
+
+	err = fabricengine.CheckRemoteBranchRequestForTest(h.Location, weftRoot, "origin", checkedOut, "")
+	if !RefusedByGate(err, fabricengine.CheckOwnership) {
+		t.Fatalf("CheckRemoteBranchRequestForTest(%q) = %v; want a CheckOwnership refusal", checkedOut, err)
+	}
+}
diff --git a/internal/fabricengine/doc.go b/internal/fabricengine/doc.go
index 34d77630f..3ecd7885a 100644
--- a/internal/fabricengine/doc.go
+++ b/internal/fabricengine/doc.go
@@ -513,15 +513,16 @@
 // `partial` exist to stop a consumer from doing by accident.
 //
 // The vocabulary is `Kind` (mutation.go's closed, string-backed enum — `path_removed`,
-// `worktree_removed`, `link_removed`, `branch_deleted`, `worktree_reset`, `dir_created`,
-// `worktree_created`, `branch_created`, `branch_pushed`, `commit_created`, `link_created`,
-// `file_written`, `push_spawned`, `worktree_switched`, `repo_advanced`), a flat `Mutation` entry
-// (kind, target, optional detail), and `Mutations`, the ordered accumulator a verb call threads
-// through everything it performs.
+// `worktree_removed`, `link_removed`, `branch_deleted`, `remote_branch_deleted`, `worktree_reset`,
+// `dir_created`, `worktree_created`, `branch_created`, `branch_pushed`, `commit_created`,
+// `link_created`, `file_written`, `push_spawned`, `worktree_switched`, `repo_advanced`,
+// `merge_staged`, `merge_resolved_staged`, `merge_committed`), a flat `Mutation` entry (kind,
+// target, optional detail), and `Mutations`, the ordered accumulator a verb call threads through
+// everything it performs.
 //
 // The accumulate-as-you-mutate rule is simple and has no exception: append an entry immediately
 // after a primitive observably changed state, never before, and never for a no-op or a refusal.
-// destroy.go's eight gate executors auto-record seven of the sixteen kinds this way, since every
+// destroy.go's nine gate executors auto-record eight of the nineteen kinds this way, since every
 // one of them already funnels through the one chokepoint the Fabric Destruction Chokepoint
 // Invariant names; the remaining kinds have no such chokepoint and are hand-recorded at their own
 // success sites instead.
@@ -565,11 +566,23 @@
 // that did not happen.
 // It now emits the same `pairs` array through `errWithRecordFields` whenever any pair carries an
 // `Error`, so the per-pair report survives and the verdict is honest.
-// `prune` and `cleanup` deliberately do NOT follow: their per-entry `Error` doubles as the
-// explanation for a DESIGNED refusal (`Protected`'s "commit them or re-run with --force",
-// `Unowned`'s "fabric will not remove it"), so treating it as a failure would report a documented
-// outcome as one. The distinction is whether the field means "this verb failed at its job" or "this
-// verb is telling you what it deliberately did not do".
+// `prune` alone deliberately does NOT follow: its per-entry `Error` doubles as the explanation for a
+// DESIGNED refusal ("commit them or re-run with --force to discard them", "fabric will not remove
+// it"), so treating it as a failure would report a documented outcome as one. The distinction is
+// whether the field means "this verb failed at its job" or "this verb is telling you what it
+// deliberately did not do", and it is that test — not membership in this carve-out — that decides
+// the question below for `cleanup`.
+//
+// `cleanup` was removed from this carve-out because the premise never held for it: unlike `prune`,
+// `CleanupBranchEntry.Error` is a genuine deletion failure, not a refusal, per its own field doc in
+// `internal/fabricengine/cleanup.go`, and the new `CleanupBranchEntry.RemoteError` falls on the same
+// side for the same reason — both drive `cleanup`'s exit code. This does not extend to `cleanup`'s
+// own designed-refusal dispositions: a `Protected` or unmanaged entry still exits 0, because those
+// arms set no `Error` at all — a property of the field being empty, not of any carve-out.
+// A missing `origin` remote sits on the deliberately-did-not-do side too, and exits 0: it is carried
+// by the verb-level `RemoteSkippedReason` on both `CleanupResult` and `RemoveResult` rather than by
+// either per-branch error field, so the identical configuration state produces the identical verdict
+// from `cleanup` and `remove` alike.
 //
 // One consequence of that rule is a new `ReconcileAction`. `Reconcile` reads `git worktree list`
 // once, before its per-pair loop, so a concurrent `remove`/`prune` can delete a pair's directory
@@ -617,8 +630,9 @@
 // # The destruction chokepoint
 //
 // `destroy.go` is the one file in this package permitted to perform a destructive primitive —
-// `os.RemoveAll`/`os.Remove`, `git worktree remove`, `git branch -D`, `fslink.Remove`, and a warp
-// checkout's `ResetHard` — and every one of them runs its shared four-check pipeline first.
+// `os.RemoveAll`/`os.Remove`, `git worktree remove`, `git branch -D`, `fslink.Remove`, deleting a
+// branch on a remote (`git push <remote> --delete`), and a warp checkout's `ResetHard` — and every
+// one of them runs its shared four-check pipeline first.
 // See `CONSTRAINTS.md`'s Fabric Destruction Chokepoint Invariant for the rules;
 // this section is the rationale the invariant deliberately omits.
 //
@@ -636,9 +650,10 @@
 // A gate a caller consults and then acts on independently is advice, not enforcement — the
 // caller can still reach `os.RemoveAll` directly, and nothing distinguishes "checked, then
 // destroyed" from "destroyed". `destroy.go`'s executors (`removePath`, `removeGitWorktree`,
-// `removeLink`, `repointLink`, `deleteBranch`, `resetHardTo`) run the pipeline and then perform
-// the primitive themselves, so the two can never come apart. This is also what makes the bypass
-// guard meaningful: a raw call to any of the five primitives is mechanically bannable everywhere
+// `removeLink`, `repointLink`, `deleteBranch`, `deleteRemoteBranch`, `resetHardTo`) run the
+// pipeline and then perform the primitive themselves, so the two can never come apart. This is
+// also what makes the bypass guard meaningful: a raw call to any of the six primitives is
+// mechanically bannable everywhere
 // else in this package precisely because there is no legitimate reason for one to exist there —
 // the gate is not one way to destroy something, it is the only way.
 //
diff --git a/internal/fabricengine/export_test.go b/internal/fabricengine/export_test.go
index 77d6edbe1..3115afc8d 100644
--- a/internal/fabricengine/export_test.go
+++ b/internal/fabricengine/export_test.go
@@ -130,6 +130,29 @@ func DeleteBranchForTest(l *lyxcwd.Location, repoDir, branch, branchPrefix strin
 	return deleteBranch(NewMutations(""), req)
 }
 
+// CheckRemoteBranchRequestForTest re-exports checkRemoteBranchRequest, built from
+// ownedManagedBranch(l, branchPrefix) and dirtyCheckedOutBranch(), for package fabricengine_test
+// integration tests that need to drive the remote-branch gate directly against a real hub —
+// mirroring DeleteBranchForTest's shape for the local executor. package fabricengine_test cannot
+// construct a remoteBranchRequest directly (it is unexported) and cannot import the hubforge package
+// from an internal (unsuffixed package fabricengine) test file either, since that package imports
+// fabriccli, which imports fabricengine, closing an import cycle for Go's internal test
+// augmentation — this seam is what lets a package fabricengine_test file reach the gate while still
+// building its real-hub fixture through that package's own factory function, per the hubforge
+// Fabric-Fixture Invariant.
+func CheckRemoteBranchRequestForTest(l *lyxcwd.Location, repoDir, remote, branch, branchPrefix string) error {
+	req := remoteBranchRequest{
+		what:      "test delete remote branch",
+		repoDir:   repoDir,
+		remote:    remote,
+		branch:    branch,
+		ownership: ownedManagedBranch(l, branchPrefix),
+		dirtiness: dirtyCheckedOutBranch(),
+		force:     false,
+	}
+	return checkRemoteBranchRequest(req)
+}
+
 // --- weft-fixture migration shim (fabricengine in-package weft batch) ---
 //
 // The functions and types below serve the nine package fabricengine_test files this batch relocates
diff --git a/internal/fabricengine/livestate_mutationoracle_test.go b/internal/fabricengine/livestate_mutationoracle_test.go
index 9344adebd..a6657c7f0 100644
--- a/internal/fabricengine/livestate_mutationoracle_test.go
+++ b/internal/fabricengine/livestate_mutationoracle_test.go
@@ -37,16 +37,17 @@ var manifestObservableKind = map[fabricengine.Kind]bool{
 	fabricengine.KindLinkRemoved:     true,
 	fabricengine.KindLinkCreated:     true,
 
-	fabricengine.KindBranchCreated:    false,
-	fabricengine.KindBranchDeleted:    false,
-	fabricengine.KindBranchPushed:     false,
-	fabricengine.KindCommitCreated:    false,
-	fabricengine.KindWorktreeReset:    false,
-	fabricengine.KindWorktreeSwitched: false,
-	fabricengine.KindPushSpawned:      false,
-	fabricengine.KindRepoAdvanced:     false,
-	fabricengine.KindMergeStaged:      false,
-	fabricengine.KindMergeCommitted:   false,
+	fabricengine.KindBranchCreated:       false,
+	fabricengine.KindBranchDeleted:       false,
+	fabricengine.KindRemoteBranchDeleted: false,
+	fabricengine.KindBranchPushed:        false,
+	fabricengine.KindCommitCreated:       false,
+	fabricengine.KindWorktreeReset:       false,
+	fabricengine.KindWorktreeSwitched:    false,
+	fabricengine.KindPushSpawned:         false,
+	fabricengine.KindRepoAdvanced:        false,
+	fabricengine.KindMergeStaged:         false,
+	fabricengine.KindMergeCommitted:      false,
 }
 
 // invertedBy maps a constructive kind to the kinds of a later entry, at the same Target, that undo it
diff --git a/internal/fabricengine/livestate_refusal_selftest_test.go b/internal/fabricengine/livestate_refusal_selftest_test.go
index 39db99144..66368b74a 100644
--- a/internal/fabricengine/livestate_refusal_selftest_test.go
+++ b/internal/fabricengine/livestate_refusal_selftest_test.go
@@ -252,7 +252,7 @@ func TestRefusedBefore(t *testing.T) {
 			t.Fatalf("write %s: %v", scratch, err)
 		}
 
-		_, err := h.Topology.Remove(h.Location, slug, false)
+		_, err := h.Topology.Remove(h.Location, slug, false, false)
 		if err == nil {
 			t.Fatalf("Remove(%s, force=false) against a dirty warp worktree: want an error, got nil", slug)
 		}
@@ -287,7 +287,7 @@ func TestRefusedBefore(t *testing.T) {
 		t.Parallel()
 
 		h := hubforge.NewHub(t, ".")
-		_, err := h.Topology.Remove(h.Location, "..", false)
+		_, err := h.Topology.Remove(h.Location, "..", false, false)
 		if err == nil {
 			t.Fatalf(`Remove(h.Location, "..", false): want an error, got nil`)
 		}
diff --git a/internal/fabricengine/livestate_verbs_test.go b/internal/fabricengine/livestate_verbs_test.go
index fc27d2561..7ff8910c8 100644
--- a/internal/fabricengine/livestate_verbs_test.go
+++ b/internal/fabricengine/livestate_verbs_test.go
@@ -610,7 +610,7 @@ func removeCase() VerbCase {
 		},
 		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
 			tb.Helper()
-			res, err := h.Topology.Remove(h.Location, f.Slug, false)
+			res, err := h.Topology.Remove(h.Location, f.Slug, false, false)
 			return res.Mutated(), err
 		},
 		Expect: func(state string) Expectation {
@@ -770,7 +770,7 @@ func cleanupCase() VerbCase {
 		},
 		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
 			tb.Helper()
-			res, err := h.Topology.Cleanup(h.Location, true, true)
+			res, err := h.Topology.Cleanup(h.Location, true, true, false)
 			return res.Mutated(), err
 		},
 		Expect: func(state string) Expectation {
@@ -1271,7 +1271,7 @@ func removeHostileCases() []VerbCase {
 			},
 			Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
 				tb.Helper()
-				res, err := h.Topology.Remove(h.Location, f.Slug, false)
+				res, err := h.Topology.Remove(h.Location, f.Slug, false, false)
 				return res.Mutated(), err
 			},
 			Expect: func(state string) Expectation {
diff --git a/internal/fabricengine/mergecrucible_integration_test.go b/internal/fabricengine/mergecrucible_integration_test.go
index 8c76c2809..39086c156 100644
--- a/internal/fabricengine/mergecrucible_integration_test.go
+++ b/internal/fabricengine/mergecrucible_integration_test.go
@@ -288,7 +288,7 @@ func TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming(t *testing.T)
 		t.Fatalf("MergeIn(%s).Conflicts is empty; the fixture must leave a live merge record", sourceBranch)
 	}
 
-	_, err = h.Topology.Remove(primeLocation, slug, false)
+	_, err = h.Topology.Remove(primeLocation, slug, false, false)
 	var refused *fabricengine.ErrMergeInProgress
 	if !errors.As(err, &refused) {
 		t.Fatalf("Remove(%s) while the prime pair is mid-merge on its branches: error = %v (%T); want *ErrMergeInProgress", slug, err, err)
@@ -301,7 +301,7 @@ func TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming(t *testing.T)
 	}
 
 	// force answers dirtiness only, never a live merge record.
-	if _, err := h.Topology.Remove(primeLocation, slug, true); !errors.As(err, &refused) {
+	if _, err := h.Topology.Remove(primeLocation, slug, true, false); !errors.As(err, &refused) {
 		t.Fatalf("Remove(%s, force=true): error = %v (%T); want *ErrMergeInProgress even with force", slug, err, err)
 	}
 
@@ -309,7 +309,7 @@ func TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming(t *testing.T)
 	if _, err := prime.MergeAbort(); err != nil {
 		t.Fatalf("MergeAbort: %v", err)
 	}
-	if _, err := h.Topology.Remove(primeLocation, slug, true); err != nil {
+	if _, err := h.Topology.Remove(primeLocation, slug, true, false); err != nil {
 		t.Fatalf("Remove(%s) after MergeAbort: %v; want success — the guard must close a window, not block the pair forever", slug, err)
 	}
 }
@@ -806,7 +806,7 @@ func TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource(t *testi
 		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
 	}
 
-	_, err = h.Topology.Remove(primeLocation, sourceSlug, false)
+	_, err = h.Topology.Remove(primeLocation, sourceSlug, false, false)
 	var refused *fabricengine.ErrMergeInProgress
 	if !errors.As(err, &refused) {
 		t.Fatalf("Remove(%s) while a LINKED pair is mid-merge on its branches: error = %v (%T); want *ErrMergeInProgress", sourceSlug, err, err)
@@ -819,7 +819,7 @@ func TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource(t *testi
 	}
 
 	// force answers dirtiness only, never a live merge record — the same rule as the prime-pair case.
-	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true); !errors.As(err, &refused) {
+	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true, false); !errors.As(err, &refused) {
 		t.Fatalf("Remove(%s, force=true): error = %v (%T); want *ErrMergeInProgress even with force", sourceSlug, err, err)
 	}
 
@@ -827,7 +827,7 @@ func TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource(t *testi
 	if _, err := consumer.MergeAbort(); err != nil {
 		t.Fatalf("MergeAbort on the linked consumer pair: %v", err)
 	}
-	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true); err != nil {
+	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true, false); err != nil {
 		t.Fatalf("Remove(%s) after MergeAbort: %v; want success — the guard must close a window, not block the pair forever", sourceSlug, err)
 	}
 }
diff --git a/internal/fabricengine/mergesiblings_integration_test.go b/internal/fabricengine/mergesiblings_integration_test.go
index 2510402df..68a44bafb 100644
--- a/internal/fabricengine/mergesiblings_integration_test.go
+++ b/internal/fabricengine/mergesiblings_integration_test.go
@@ -141,7 +141,7 @@ func TestMergeSiblings_Dispositions(t *testing.T) {
 	})
 
 	t.Run("RemoveWithoutForce", func(t *testing.T) {
-		_, err := h.Topology.Remove(l, slug, false)
+		_, err := h.Topology.Remove(l, slug, false, false)
 		var refused *fabricengine.ErrMergeInProgress
 		if !errors.As(err, &refused) {
 			t.Fatalf("Remove(force=false) error = %v (%T); want *ErrMergeInProgress", err, err)
@@ -156,7 +156,7 @@ func TestMergeSiblings_Dispositions(t *testing.T) {
 
 	t.Run("RemoveWithForce", func(t *testing.T) {
 		// force answers dirtiness only, never a live merge record — the gate's own rule (card 13).
-		_, err := h.Topology.Remove(l, slug, true)
+		_, err := h.Topology.Remove(l, slug, true, false)
 		var refused *fabricengine.ErrMergeInProgress
 		if !errors.As(err, &refused) {
 			t.Fatalf("Remove(force=true) error = %v (%T); want *ErrMergeInProgress even with --force", err, err)
@@ -181,7 +181,7 @@ func TestMergeSiblings_Dispositions(t *testing.T) {
 			{"ApplyForce", true, true},
 		} {
 			t.Run(tt.name, func(t *testing.T) {
-				cleanupRes, err := h.Topology.Cleanup(l, tt.apply, tt.force)
+				cleanupRes, err := h.Topology.Cleanup(l, tt.apply, tt.force, false)
 				if err != nil {
 					t.Fatalf("Cleanup(apply=%v, force=%v) error = %v", tt.apply, tt.force, err)
 				}
diff --git a/internal/fabricengine/mutation.go b/internal/fabricengine/mutation.go
index bc4241c5c..913dcfd98 100644
--- a/internal/fabricengine/mutation.go
+++ b/internal/fabricengine/mutation.go
@@ -20,7 +20,7 @@ import (
 type Kind string
 
 // The fixed set of mutation kinds fabric records.
-// Seven are auto-recorded by the destruction gate (destroy.go); the remaining eleven are
+// Eight are auto-recorded by the destruction gate (destroy.go); the remaining eleven are
 // hand-recorded at their success sites, since no chokepoint covers them.
 const (
 	// KindPathRemoved records removePath's deletion of a single path or a directory tree.
@@ -31,6 +31,12 @@ const (
 	KindLinkRemoved Kind = "link_removed"
 	// KindBranchDeleted records deleteBranch's `git branch -D`.
 	KindBranchDeleted Kind = "branch_deleted"
+	// KindRemoteBranchDeleted records deleteRemoteBranch's `git push <remote> --delete` for a delete
+	// that observably removed a ref on the remote.
+	// It is a distinct kind from KindBranchDeleted, never a reuse of it, because only the local
+	// deletion is recoverable (the remote copy is gone for good once pushed away) — a consumer
+	// switching on kind must be able to tell the two apart.
+	KindRemoteBranchDeleted Kind = "remote_branch_deleted"
 	// KindWorktreeReset records resetHardTo's `reset --hard`.
 	KindWorktreeReset Kind = "worktree_reset"
 	// KindDirCreated records createExclusiveDir's directory mint.
diff --git a/internal/fabricengine/mutation_record_integration_test.go b/internal/fabricengine/mutation_record_integration_test.go
index 63a0311ca..fa5b4dffb 100644
--- a/internal/fabricengine/mutation_record_integration_test.go
+++ b/internal/fabricengine/mutation_record_integration_test.go
@@ -47,7 +47,7 @@ func TestMutationRecord_RemoveDirtyWarpRefusalCarriesThePortalAndLauncherDeletio
 		t.Fatalf("dirty the warp worktree at %s: %v", target, err)
 	}
 
-	res, err := topology.Remove(l, slug, false)
+	res, err := topology.Remove(l, slug, false, false)
 	if err == nil {
 		t.Fatalf("Remove(%q, force=false) = nil error; want the dirty-warp pre-flight refusal", slug)
 	}
diff --git a/internal/fabricengine/origin_integration_test.go b/internal/fabricengine/origin_integration_test.go
index e67aa83c0..3043d6286 100644
--- a/internal/fabricengine/origin_integration_test.go
+++ b/internal/fabricengine/origin_integration_test.go
@@ -421,7 +421,7 @@ func TestAdd_RunLauncherLifecycle(t *testing.T) {
 		t.Fatalf("run launcher missing at %s after add: %v", runPath, err)
 	}
 
-	if _, err := h.Topology.Remove(l, slug, false); err != nil {
+	if _, err := h.Topology.Remove(l, slug, false, false); err != nil {
 		t.Fatalf("Remove(%q): %v", slug, err)
 	}
 
diff --git a/internal/fabricengine/reconcile_stale_registration_test.go b/internal/fabricengine/reconcile_stale_registration_test.go
index ff5eec278..864477d1f 100644
--- a/internal/fabricengine/reconcile_stale_registration_test.go
+++ b/internal/fabricengine/reconcile_stale_registration_test.go
@@ -408,7 +408,7 @@ func TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut(t *testing.T) {
 	// checked-out branch.
 	gitkit.MustRun(t, weftPrime, "git", "checkout", "-b", "primary-parked")
 
-	res, err := topology.Cleanup(l, true, true)
+	res, err := topology.Cleanup(l, true, true, false)
 	if err != nil {
 		t.Fatalf("Cleanup(apply, force): %v", err)
 	}
@@ -436,7 +436,7 @@ func TestCleanup_NonSuffixedBranchNeverDeleted(t *testing.T) {
 	const warpManagedBranch = "cleanup-warp-owned"
 	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "branch", warpManagedBranch, fabricengine.WeftBranchName("main"))
 
-	res, err := topology.Cleanup(l, true, true)
+	res, err := topology.Cleanup(l, true, true, false)
 	if err != nil {
 		t.Fatalf("Cleanup(apply, force): %v", err)
 	}
@@ -480,7 +480,7 @@ func TestCleanup_DetachedWarpHeadProtectsCheckedOutWeftBranch(t *testing.T) {
 
 	// Dry-run must already report the branch protected, so dry-run and apply
 	// agree about its fate.
-	dry, err := topology.Cleanup(l, false, false)
+	dry, err := topology.Cleanup(l, false, false, false)
 	if err != nil {
 		t.Fatalf("Cleanup(dry-run): %v", err)
 	}
@@ -489,7 +489,7 @@ func TestCleanup_DetachedWarpHeadProtectsCheckedOutWeftBranch(t *testing.T) {
 		t.Errorf("dry-run Protected = false for checked-out weft branch %q; want true", weftBranch)
 	}
 
-	forced, err := topology.Cleanup(l, true, true)
+	forced, err := topology.Cleanup(l, true, true, false)
 	if err != nil {
 		t.Fatalf("Cleanup(apply, force): %v", err)
 	}
@@ -613,7 +613,7 @@ func TestCleanup_DryRunMatchesApplyVerdict(t *testing.T) {
 	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
 		t.Fatalf("Add: %v", err)
 	}
-	if _, err := topology.Remove(l, slug, true); err != nil {
+	if _, err := topology.Remove(l, slug, true, false); err != nil {
 		t.Fatalf("Remove: %v", err)
 	}
 
@@ -637,11 +637,11 @@ func TestCleanup_DryRunMatchesApplyVerdict(t *testing.T) {
 		return fabricengine.CleanupBranchEntry{}
 	}
 
-	dry, err := topology.Cleanup(l, false, false)
+	dry, err := topology.Cleanup(l, false, false, false)
 	if err != nil {
 		t.Fatalf("Cleanup(dry): %v", err)
 	}
-	applied, err := topology.Cleanup(l, true, false)
+	applied, err := topology.Cleanup(l, true, false, false)
 	if err != nil {
 		t.Fatalf("Cleanup(apply): %v", err)
 	}
@@ -681,7 +681,7 @@ func TestCleanup_ForceIsReservedAndChangesNoVerdict(t *testing.T) {
 	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
 		t.Fatalf("Add: %v", err)
 	}
-	if _, err := topology.Remove(l, slug, true); err != nil {
+	if _, err := topology.Remove(l, slug, true, false); err != nil {
 		t.Fatalf("Remove: %v", err)
 	}
 
@@ -694,11 +694,11 @@ func TestCleanup_ForceIsReservedAndChangesNoVerdict(t *testing.T) {
 	const unmanaged = "legacy-notes"
 	gitkit.MustRun(t, weftRepoRoot, "git", "branch", unmanaged)
 
-	dryWithoutForce, err := topology.Cleanup(l, false, false)
+	dryWithoutForce, err := topology.Cleanup(l, false, false, false)
 	if err != nil {
 		t.Fatalf("Cleanup(dry, force=false): %v", err)
 	}
-	dryWithForce, err := topology.Cleanup(l, false, true)
+	dryWithForce, err := topology.Cleanup(l, false, true, false)
 	if err != nil {
 		t.Fatalf("Cleanup(dry, force=true): %v", err)
 	}
@@ -706,7 +706,7 @@ func TestCleanup_ForceIsReservedAndChangesNoVerdict(t *testing.T) {
 		t.Errorf("Cleanup(dry) entries differ between force=false and force=true (-without +with):\n%s\nforce is reserved and must answer no gate in this verb", diff)
 	}
 
-	applied, err := topology.Cleanup(l, true, true)
+	applied, err := topology.Cleanup(l, true, true, false)
 	if err != nil {
 		t.Fatalf("Cleanup(apply, force=true): %v", err)
 	}
diff --git a/internal/fabricengine/reconcile_stale_removal_test.go b/internal/fabricengine/reconcile_stale_removal_test.go
index a389cd31d..b73dc3d65 100644
--- a/internal/fabricengine/reconcile_stale_removal_test.go
+++ b/internal/fabricengine/reconcile_stale_removal_test.go
@@ -433,7 +433,7 @@ func TestRepoWideMigratedSites_ResolveFromBoardDirWithNoPerPairConfig(t *testing
 	lyxLink := fabricengine.WarpLyxLinkHere(removeWarpLayout)
 	extraLink := filepath.Join(removeWarpLayout.WorktreePath(), removeWarpLayout.AnchorRel, "_extra")
 
-	if _, err := topology.Remove(l, removeSlug, true); err != nil {
+	if _, err := topology.Remove(l, removeSlug, true, false); err != nil {
 		t.Fatalf("Remove(%s): %v", removeSlug, err)
 	}
 	if _, statErr := os.Lstat(lyxLink); !os.IsNotExist(statErr) {
diff --git a/internal/fabricengine/remove.go b/internal/fabricengine/remove.go
index d8b3fbf63..b7120572c 100644
--- a/internal/fabricengine/remove.go
+++ b/internal/fabricengine/remove.go
@@ -27,6 +27,16 @@ type RemoveResult struct {
 	Slug         string `json:"slug"`
 	Path         string `json:"path"`
 	LinksRemoved int    `json:"links_removed"`
+	// RemoteBranchDeleted reports whether the pair's weft branch was observably removed from the
+	// remote. It is true only when the remote deletion was attempted and actually removed a ref.
+	RemoteBranchDeleted bool `json:"remote_branch_deleted,omitempty"`
+	// RemoteBranchError is non-empty when the remote deletion was attempted and did not succeed. Its
+	// text always names the layer that said no — the gate's own refusal, or the remote deletion
+	// itself.
+	RemoteBranchError string `json:"remote_branch_error,omitempty"`
+	// RemoteSkippedReason carries a once-per-verb reason no remote deletion was attempted at all —
+	// today only a weft repo with no origin remote configured.
+	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
 }
 
 // Remove removes a paired warp and weft git worktree with all associated artifacts.
@@ -40,7 +50,11 @@ type RemoveResult struct {
 // licence to delete the clone.
 // Portal and launcher cleanup run after those checks but before the git removal, so they still run
 // when the worktree directory is already gone.
-func (t *Topology) Remove(l *lyxcwd.Location, slug string, force bool) (res RemoveResult, err error) {
+// remote gates whether the pair's weft branch, once deleted locally, is also deleted on the weft
+// repo's origin remote; a remote deletion failure never makes Remove return a non-nil error. Remove
+// still never deletes warpBranch — it is computed only to derive weftBranch and to check
+// merge-source in-flight — and remote adds no warp-branch deletion of either kind.
+func (t *Topology) Remove(l *lyxcwd.Location, slug string, force, remote bool) (res RemoveResult, err error) {
 	rec := NewMutations(l.HubPath)
 	defer func() { res.Mutations = rec.Snapshot() }()
 
@@ -128,8 +142,9 @@ func (t *Topology) Remove(l *lyxcwd.Location, slug string, force bool) (res Remo
 
 	// A weft-teardown failure is tolerated only when the weft worktree is actually gone (already
 	// absent, or removed with just a branch/prune step failing) — a weft worktree still on disk
-	// after a "successful" Remove is a half-torn pair the operator was never told about.
-	weftErr := removeWeftWorktree(rec, l, slug, weftBranch, force, true, t.cfg.BranchPrefix)
+	// after a "successful" Remove is a half-torn pair the operator was never told about. This check
+	// reads the error return alone, never the teardown struct: the remote outcome never affects it.
+	teardown, weftErr := removeWeftWorktree(rec, l, slug, weftBranch, force, true, remote, t.cfg.BranchPrefix)
 	if weftErr != nil {
 		weftTarget := WeftWorktreePath(l, slug)
 		if _, statErr := os.Stat(weftTarget); statErr == nil {
@@ -140,9 +155,12 @@ func (t *Topology) Remove(l *lyxcwd.Location, slug string, force bool) (res Remo
 	}
 
 	return RemoveResult{
-		Slug:         slug,
-		Path:         target,
-		LinksRemoved: linksRemoved,
+		Slug:                slug,
+		Path:                target,
+		LinksRemoved:        linksRemoved,
+		RemoteBranchDeleted: teardown.remoteBranchDeleted,
+		RemoteBranchError:   teardown.remoteBranchError,
+		RemoteSkippedReason: teardown.remoteSkippedReason,
 	}, nil
 }
 
diff --git a/internal/fabricengine/remove_guard_integration_test.go b/internal/fabricengine/remove_guard_integration_test.go
index f63287d1a..ed254b28f 100644
--- a/internal/fabricengine/remove_guard_integration_test.go
+++ b/internal/fabricengine/remove_guard_integration_test.go
@@ -31,7 +31,7 @@ func TestRemove_RefusesPrimeWorktreeAndLeavesItIntact(t *testing.T) {
 	primeSlug := filepath.Base(l.WorktreePath())
 	topology := fabricengine.NewTopology(fabricengine.Config{})
 
-	_, err := topology.Remove(l, primeSlug, false)
+	_, err := topology.Remove(l, primeSlug, false, false)
 	if err == nil {
 		t.Fatalf("Remove(%q) = nil error; want a refusal naming the prime worktree", primeSlug)
 	}
@@ -73,7 +73,7 @@ func TestRemove_RefusesForeignWorktreeWithoutDeletingIt(t *testing.T) {
 	gitkit.MustRun(t, foreign, "git", "commit", "-m", "seed marker")
 
 	topology := fabricengine.NewTopology(fabricengine.Config{})
-	_, err := topology.Remove(l, slug, false)
+	_, err := topology.Remove(l, slug, false, false)
 	if err == nil {
 		t.Fatalf("Remove(%q) = nil error; want a refusal naming git's own reason", slug)
 	}
diff --git a/internal/fabricengine/remove_junctions_integration_test.go b/internal/fabricengine/remove_junctions_integration_test.go
index 951837de6..cb02bb7b8 100644
--- a/internal/fabricengine/remove_junctions_integration_test.go
+++ b/internal/fabricengine/remove_junctions_integration_test.go
@@ -88,7 +88,7 @@ func TestRemove_TearsDownNestedJunction(t *testing.T) {
 	// name-load finds the configured pathspec's junctions regardless of this
 	// pair's RelPath, and the happy-path nested teardown below is actually
 	// exercised, not just the degraded nothing-removed path.
-	if _, err := topology.Remove(nestedLayout, slug, true); err != nil {
+	if _, err := topology.Remove(nestedLayout, slug, true, false); err != nil {
 		t.Fatalf("Remove: %v", err)
 	}
 
@@ -139,7 +139,7 @@ func TestRemove_SweepsAnchoredLinksOnSubpathHub(t *testing.T) {
 		t.Fatalf("setup: %s is not a junction (isLink=%v err=%v)", lyxLink, isLink, linkErr)
 	}
 
-	result, err := topology.Remove(l, slug, true)
+	result, err := topology.Remove(l, slug, true, false)
 	if err != nil {
 		t.Fatalf("Remove: %v", err)
 	}
@@ -169,7 +169,7 @@ func TestRemove_FailedWeftTeardownIsReported(t *testing.T) {
 	weftTarget := fabricengine.WeftWorktreePath(l, slug)
 	gitkit.MustRun(t, fixture.WeftPrime, "git", "worktree", "lock", weftTarget)
 
-	_, err := topology.Remove(l, slug, true)
+	_, err := topology.Remove(l, slug, true, false)
 	if err == nil {
 		t.Fatal("Remove() with a locked weft worktree error = nil; want the surviving weft worktree reported")
 	}
diff --git a/internal/fabricengine/remove_refusal_remedy_integration_test.go b/internal/fabricengine/remove_refusal_remedy_integration_test.go
index ad839dd60..e559a19cf 100644
--- a/internal/fabricengine/remove_refusal_remedy_integration_test.go
+++ b/internal/fabricengine/remove_refusal_remedy_integration_test.go
@@ -61,7 +61,7 @@ func TestRemove_RefusalNamesStrandedPortalTeardown(t *testing.T) {
 		t.Fatalf("dirty %s: %v", tracked, err)
 	}
 
-	res, err := topology.Remove(l, slug, false)
+	res, err := topology.Remove(l, slug, false, false)
 	if err == nil {
 		t.Fatalf("Remove(force=false) on a dirty pair returned nil error; want a refusal")
 	}
@@ -122,7 +122,7 @@ func TestRemove_StatusFailureNamesPathAndCommandOnce(t *testing.T) {
 		t.Fatalf("create %s: %v", notACheckout, err)
 	}
 
-	_, err := topology.Remove(l, slug, false)
+	_, err := topology.Remove(l, slug, false, false)
 	if err == nil {
 		t.Fatalf("Remove on a non-checkout directory returned nil error; want a failure from the dirtiness probe")
 	}
@@ -166,11 +166,11 @@ func TestRemove_RefusalWithNothingStrandedOmitsRemedy(t *testing.T) {
 		t.Fatalf("dirty %s: %v", tracked, err)
 	}
 
-	if _, err := topology.Remove(l, slug, false); err == nil {
+	if _, err := topology.Remove(l, slug, false, false); err == nil {
 		t.Fatalf("first Remove(force=false) returned nil error; want a refusal")
 	}
 
-	res, err := topology.Remove(l, slug, false)
+	res, err := topology.Remove(l, slug, false, false)
 	if err == nil {
 		t.Fatalf("second Remove(force=false) returned nil error; want a refusal")
 	}
diff --git a/internal/fabricengine/remove_reserved_integration_test.go b/internal/fabricengine/remove_reserved_integration_test.go
index 48e003d99..3e760b61d 100644
--- a/internal/fabricengine/remove_reserved_integration_test.go
+++ b/internal/fabricengine/remove_reserved_integration_test.go
@@ -50,7 +50,7 @@ func TestRemove_RefusesReservedSlugsAndLeavesThemOnDisk(t *testing.T) {
 			t.Fatalf("seed %s: %v", marker, err)
 		}
 
-		_, err := topology.Remove(l, slug, true)
+		_, err := topology.Remove(l, slug, true, false)
 		if err == nil {
 			t.Errorf("Remove(%q) = nil error; want an invalid-slug refusal", slug)
 		} else if !strings.Contains(err.Error(), "invalid slug") {
diff --git a/internal/fabricengine/removeremote_integration_test.go b/internal/fabricengine/removeremote_integration_test.go
new file mode 100644
index 000000000..e6fa0cd10
--- /dev/null
+++ b/internal/fabricengine/removeremote_integration_test.go
@@ -0,0 +1,160 @@
+//go:build integration
+
+// removeremote_integration_test.go covers Remove's remote branch deletion under --remote: the
+// opt-in deletion of the pair's weft branch on the remote, its default-off regression guard, the
+// non-fatal remote-failure partial-teardown guarantee, and the once-per-verb no-origin pre-check
+// shared with Cleanup.
+//
+// Every hub here is built through hubforge.NewHub per the hubforge Fabric-Fixture Invariant (via
+// newFabricFixture), using the hub's own WeftBare field as the weft remote to assert against.
+// mustBreakOrigin/mustRemoveOrigin/branchExistsAt are shared with cleanupremote_integration_test.go
+// and reconcile_stale_registration_test.go — every assertion here goes through exported API.
+//
+// Package fabricengine_test; shares the single TestMain in testmain_test.go.
+
+package fabricengine_test
+
+import (
+	"os"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+)
+
+// TestRemove_RemoteTrueDeletesWeftBranchOnRemote covers case 1: remote true deletes the pair's weft
+// branch on the remote, sets RemoteBranchDeleted, and records exactly one KindRemoteBranchDeleted
+// entry.
+func TestRemove_RemoteTrueDeletesWeftBranchOnRemote(t *testing.T) {
+	t.Parallel()
+
+	const slug = "remove-remote-both"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftBranch := fabricengine.WeftBranchName(slug)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
+		t.Fatalf("setup Add(%q): %v", slug, err)
+	}
+
+	res, err := topology.Remove(l, slug, false, true)
+	if err != nil {
+		t.Fatalf("Remove(%q, remote=true) error = %v", slug, err)
+	}
+	if !res.RemoteBranchDeleted {
+		t.Errorf("RemoteBranchDeleted = false; want true")
+	}
+	if res.RemoteBranchError != "" {
+		t.Errorf("RemoteBranchError = %q; want empty", res.RemoteBranchError)
+	}
+	if branchExistsAt(t, fixture.WeftBare, weftBranch) {
+		t.Errorf("weft branch %q still exists on the remote after Remove(remote=true)", weftBranch)
+	}
+
+	var seen int
+	for _, m := range res.Mutated().Entries() {
+		if m.Kind == fabricengine.KindRemoteBranchDeleted && m.Target == weftBranch {
+			seen++
+		}
+	}
+	if seen != 1 {
+		t.Errorf("mutation record has %d %s entries for %q; want exactly 1", seen, fabricengine.KindRemoteBranchDeleted, weftBranch)
+	}
+}
+
+// TestRemove_RemoteFalseLeavesRemoteBranchIntact covers the same case's negative half: remote false
+// does neither.
+func TestRemove_RemoteFalseLeavesRemoteBranchIntact(t *testing.T) {
+	t.Parallel()
+
+	const slug = "remove-remote-off"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftBranch := fabricengine.WeftBranchName(slug)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
+		t.Fatalf("setup Add(%q): %v", slug, err)
+	}
+
+	res, err := topology.Remove(l, slug, false, false)
+	if err != nil {
+		t.Fatalf("Remove(%q, remote=false) error = %v", slug, err)
+	}
+	if res.RemoteBranchDeleted {
+		t.Errorf("RemoteBranchDeleted = true; want false — remote is opt-in")
+	}
+	if !branchExistsAt(t, fixture.WeftBare, weftBranch) {
+		t.Errorf("weft branch %q no longer exists on the remote after Remove(remote=false)", weftBranch)
+	}
+}
+
+// TestRemove_RemoteFailureLeavesPartialTeardownGuaranteesIntact covers case 2: Remove's existing
+// partial-teardown guarantees are unchanged when the remote deletion fails: the weft worktree is
+// gone, the verb returns a nil error, RemoteBranchError is populated, and the failure has not been
+// picked up by the teardown's own error accumulation. Induced the same way as Cleanup's own case: a
+// weft origin pointing at an absent path.
+func TestRemove_RemoteFailureLeavesPartialTeardownGuaranteesIntact(t *testing.T) {
+	t.Parallel()
+
+	const slug = "remove-remote-fail"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
+		t.Fatalf("setup Add(%q): %v", slug, err)
+	}
+
+	mustBreakOrigin(t, weftRoot)
+
+	res, err := topology.Remove(l, slug, false, true)
+	if err != nil {
+		t.Fatalf("Remove(%q, remote=true) error = %v; want nil — a remote failure is non-fatal", slug, err)
+	}
+	if res.RemoteBranchError == "" {
+		t.Errorf("RemoteBranchError is empty; want a reason naming the remote deletion failure")
+	}
+
+	weftTarget := fabricengine.WeftWorktreePath(l, slug)
+	if _, statErr := os.Stat(weftTarget); statErr == nil {
+		t.Errorf("weft worktree still exists at %s; want the local teardown to have completed despite the remote failure", weftTarget)
+	}
+}
+
+// TestRemove_NoOriginUnderRemoteReportsSkipReasonAndCompletesTeardown covers case 3: a weft repo
+// with no origin configured under remote true: the teardown completes, RemoteSkippedReason is
+// populated, RemoteBranchError is empty, and the verb returns a nil error — the identical verdict
+// Cleanup reports for the same configuration state.
+func TestRemove_NoOriginUnderRemoteReportsSkipReasonAndCompletesTeardown(t *testing.T) {
+	t.Parallel()
+
+	const slug = "remove-no-origin"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	weftRoot := mustWeftRepoRoot(t, l)
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
+		t.Fatalf("setup Add(%q): %v", slug, err)
+	}
+
+	mustRemoveOrigin(t, weftRoot)
+
+	res, err := topology.Remove(l, slug, false, true)
+	if err != nil {
+		t.Fatalf("Remove(%q, remote=true) error = %v", slug, err)
+	}
+	if res.RemoteSkippedReason == "" {
+		t.Errorf("RemoteSkippedReason is empty; want a reason naming the missing origin remote")
+	}
+	if res.RemoteBranchError != "" {
+		t.Errorf("RemoteBranchError = %q; want empty when the pre-check itself skipped", res.RemoteBranchError)
+	}
+
+	weftTarget := fabricengine.WeftWorktreePath(l, slug)
+	if _, statErr := os.Stat(weftTarget); statErr == nil {
+		t.Errorf("weft worktree still exists at %s; want the teardown to have completed", weftTarget)
+	}
+}
diff --git a/internal/fabricengine/weftwiring.go b/internal/fabricengine/weftwiring.go
index 99e584e91..15cd490fd 100644
--- a/internal/fabricengine/weftwiring.go
+++ b/internal/fabricengine/weftwiring.go
@@ -31,6 +31,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
+	"github.com/Knatte18/loomyard/internal/gitrepo"
 	"github.com/Knatte18/loomyard/internal/lyxcwd"
 	"github.com/Knatte18/loomyard/internal/lyxdirs"
 	"github.com/Knatte18/loomyard/internal/weftname"
@@ -192,21 +193,37 @@ func removeJunctionRecords(rec *Mutations, container string, junctions []WarpJun
 	return errors.Join(errs...)
 }
 
+// weftTeardownResult carries the remote outcome of removeWeftWorktree's branch deletion, so it can
+// reach Remove without passing through the error return, whose meaning must not change.
+type weftTeardownResult struct {
+	// remoteBranchDeleted reports whether the weft branch's copy on the remote was observably
+	// removed.
+	remoteBranchDeleted bool
+	// remoteBranchError is non-empty when the remote deletion was attempted and did not succeed.
+	remoteBranchError string
+	// remoteSkippedReason carries the once-per-call reason no remote deletion was attempted at all
+	// — today only a weft repo with no origin remote configured.
+	remoteSkippedReason string
+}
+
 // removeWeftWorktree tears down the weft worktree, optionally its branch, and
-// prunes stale worktree entries. Returns the first error encountered, or nil
-// if all steps succeed.
+// prunes stale worktree entries. Returns the accumulated remote outcome alongside the first error
+// encountered among worktree removal, local branch deletion, and prune, or nil if all three succeed
+// — the error return's meaning is unchanged, and a remote deletion failure is never accumulated into
+// it.
 // branchPrefix is the caller's configured warp branch prefix, forwarded to ownedManagedBranch — this
 // function has no config in scope of its own.
-// rec is the calling verb's own recorder, threaded through to both removeGitWorktree and
-// deleteBranch below.
-func removeWeftWorktree(rec *Mutations, l *lyxcwd.Location, slug, branch string, force, alsoDeleteBranch bool, branchPrefix string) error {
+// rec is the calling verb's own recorder, threaded through to removeGitWorktree, deleteBranch, and
+// deleteRemoteBranch below.
+func removeWeftWorktree(rec *Mutations, l *lyxcwd.Location, slug, branch string, force, alsoDeleteBranch, remote bool, branchPrefix string) (weftTeardownResult, error) {
 	weftPath := WeftWorktreePath(l, slug)
 	weftRoot, err := WeftRepoRoot(l)
 	if err != nil {
-		return fmt.Errorf("resolve weft repo root: %w", err)
+		return weftTeardownResult{}, fmt.Errorf("resolve weft repo root: %w", err)
 	}
 
 	var firstErr error
+	var result weftTeardownResult
 
 	req := pathRequest{
 		what:      "remove weft worktree",
@@ -230,8 +247,39 @@ func removeWeftWorktree(rec *Mutations, l *lyxcwd.Location, slug, branch string,
 			dirtiness: dirtyCheckedOutBranch(),
 			force:     false,
 		}
-		if err := deleteBranch(rec, branchReq); err != nil && firstErr == nil {
-			firstErr = err
+		deleteErr := deleteBranch(rec, branchReq)
+		if deleteErr != nil && firstErr == nil {
+			firstErr = deleteErr
+		}
+
+		if deleteErr == nil && remote {
+			if _, urlErr := gitrepo.New(weftRoot).RemoteURL(originRemoteName); urlErr != nil {
+				result.remoteSkippedReason = fmt.Sprintf(
+					"no remote deletion attempted: the weft repo has no %q remote configured: %v",
+					originRemoteName, urlErr)
+			} else {
+				remoteReq := remoteBranchRequest{
+					what:      "delete weft branch on remote",
+					repoDir:   weftRoot,
+					remote:    originRemoteName,
+					branch:    branch,
+					ownership: ownedManagedBranch(l, branchPrefix),
+					dirtiness: dirtyCheckedOutBranch(),
+					force:     false,
+				}
+				deleted, remoteErr := deleteRemoteBranch(rec, remoteReq)
+				if remoteErr != nil {
+					if refusal, ok := RefusalOf(remoteErr); ok {
+						result.remoteBranchError = fmt.Sprintf(
+							"gate refused remote deletion of %q: %s", branch, refusal.Reason)
+					} else {
+						result.remoteBranchError = fmt.Sprintf(
+							"delete remote branch %q on %q failed: %v", branch, originRemoteName, remoteErr)
+					}
+				} else {
+					result.remoteBranchDeleted = deleted
+				}
+			}
 		}
 	}
 
@@ -241,5 +289,5 @@ func removeWeftWorktree(rec *Mutations, l *lyxcwd.Location, slug, branch string,
 		}
 	}
 
-	return firstErr
+	return result, firstErr
 }
diff --git a/internal/gitrepo/deleteremotebranch_integration_test.go b/internal/gitrepo/deleteremotebranch_integration_test.go
new file mode 100644
index 000000000..265b226a8
--- /dev/null
+++ b/internal/gitrepo/deleteremotebranch_integration_test.go
@@ -0,0 +1,130 @@
+//go:build integration
+
+// deleteremotebranch_integration_test.go covers Repo.DeleteRemoteBranch against real git
+// repositories, reusing push_test.go's bare-remote/clone fixtures (newBareRemote,
+// newRepoWithRemote, cloneFromBare) exactly as fetch_integration_test.go does.
+
+package gitrepo_test
+
+import (
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+// TestDeleteRemoteBranch_ExistingBranch_DeletesAndReportsTrue asserts the first of
+// DeleteRemoteBranch's three named outcomes: deleting a branch that exists on the remote returns
+// (true, nil), and the remote no longer lists it afterward.
+func TestDeleteRemoteBranch_ExistingBranch_DeletesAndReportsTrue(t *testing.T) {
+	container := t.TempDir()
+	bareRemote := newBareRemote(t, container)
+
+	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
+	writeFile(t, cloneAPath, "a.txt", "from A")
+	commitAll(t, cloneAPath, "commit from A")
+	if err := repoA.Push(); err != nil {
+		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
+	}
+
+	const branch = "feature-x"
+	if _, _, code, err := runGit(t, cloneAPath, "checkout", "-b", branch); err != nil || code != 0 {
+		t.Fatalf("git checkout -b %s error = %v, code = %d", branch, err, code)
+	}
+	writeFile(t, cloneAPath, "feature.txt", "from feature-x")
+	commitAll(t, cloneAPath, "commit on feature-x")
+	if _, _, code, err := runGit(t, cloneAPath, "push", "origin", branch); err != nil || code != 0 {
+		t.Fatalf("git push origin %s error = %v, code = %d", branch, err, code)
+	}
+
+	// Confirm the bare remote holds the branch before the call, so the
+	// assertion below actually proves deletion rather than absence. The bare
+	// remote path is passed as ls-remote's explicit target since a bare repo
+	// has no "origin" of its own to default to.
+	lsOut, _, code, err := runGit(t, container, "ls-remote", "--heads", bareRemote)
+	if err != nil {
+		t.Fatalf("git ls-remote --heads error = %v", err)
+	}
+	if code != 0 {
+		t.Fatalf("git ls-remote --heads exited %d", code)
+	}
+	if !strings.Contains(lsOut, "refs/heads/"+branch) {
+		t.Fatalf("bare remote heads before delete = %q; want it to contain refs/heads/%s", lsOut, branch)
+	}
+
+	deleted, err := repoA.DeleteRemoteBranch("origin", branch)
+	if err != nil {
+		t.Fatalf("DeleteRemoteBranch(%q) error = %v; want nil", branch, err)
+	}
+	if !deleted {
+		t.Errorf("DeleteRemoteBranch(%q) deleted = false; want true", branch)
+	}
+
+	lsOut, _, code, err = runGit(t, container, "ls-remote", "--heads", bareRemote)
+	if err != nil {
+		t.Fatalf("git ls-remote --heads error = %v", err)
+	}
+	if code != 0 {
+		t.Fatalf("git ls-remote --heads exited %d", code)
+	}
+	if strings.Contains(lsOut, "refs/heads/"+branch) {
+		t.Errorf("bare remote heads after delete = %q; want it to no longer contain refs/heads/%s", lsOut, branch)
+	}
+}
+
+// TestDeleteRemoteBranch_AbsentBranch_ReportsFalseWithNilError asserts DeleteRemoteBranch's
+// idempotence contract: deleting a branch that is already absent from the remote returns (false,
+// nil).
+// This runs a real `git push --delete` against a ref that genuinely is not there, so the test
+// observes git's own stderr rather than a fixture, and is the tripwire on the single pinned
+// substring "remote ref does not exist": a future git rewording makes this test fail loudly instead
+// of silently reclassifying the common case as an error.
+func TestDeleteRemoteBranch_AbsentBranch_ReportsFalseWithNilError(t *testing.T) {
+	container := t.TempDir()
+	bareRemote := newBareRemote(t, container)
+
+	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
+	writeFile(t, cloneAPath, "a.txt", "from A")
+	commitAll(t, cloneAPath, "commit from A")
+	if err := repoA.Push(); err != nil {
+		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
+	}
+
+	const branch = "gone-branch"
+	deleted, err := repoA.DeleteRemoteBranch("origin", branch)
+	if err != nil {
+		t.Fatalf("DeleteRemoteBranch(%q) error = %v; want nil (absent ref is idempotent success)", branch, err)
+	}
+	if deleted {
+		t.Errorf("DeleteRemoteBranch(%q) deleted = true; want false (nothing was there to delete)", branch)
+	}
+}
+
+// TestDeleteRemoteBranch_UnreachableRemote_ReturnsError asserts DeleteRemoteBranch's failure
+// outcome: a genuine failure returns a non-nil error and deleted == false.
+// The failure is induced without a network by pointing the clone's remote at a filesystem path that
+// does not exist. The exact wording of git's failure is not pinned by any decision, so this
+// deliberately does not assert on it.
+func TestDeleteRemoteBranch_UnreachableRemote_ReturnsError(t *testing.T) {
+	container := t.TempDir()
+	bareRemote := newBareRemote(t, container)
+
+	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
+	writeFile(t, cloneAPath, "a.txt", "from A")
+	commitAll(t, cloneAPath, "commit from A")
+	if err := repoA.Push(); err != nil {
+		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
+	}
+
+	missing := filepath.Join(container, "does-not-exist.git")
+	if _, _, code, err := runGit(t, cloneAPath, "remote", "set-url", "origin", missing); err != nil || code != 0 {
+		t.Fatalf("git remote set-url origin error = %v, code = %d", err, code)
+	}
+
+	deleted, err := repoA.DeleteRemoteBranch("origin", "feature-x")
+	if err == nil {
+		t.Fatal("DeleteRemoteBranch() against an unreachable remote error = nil; want an error")
+	}
+	if deleted {
+		t.Errorf("DeleteRemoteBranch() against an unreachable remote deleted = true; want false")
+	}
+}
diff --git a/internal/gitrepo/push.go b/internal/gitrepo/push.go
index 6ad5afe64..f5f6eedfc 100644
--- a/internal/gitrepo/push.go
+++ b/internal/gitrepo/push.go
@@ -1,9 +1,10 @@
 // push.go implements the push surface: Push (a single synchronous push with rebase-retry
 // resilience), PushCoalesced (a single-pusher lock plus one guarded push, coalescing across
-// processes via the lock queue rather than an internal retry loop), and PushRebaseFree (a single
-// plain push that never rebases, for callers that supply their own serialization).
-// All three are push-only; committing is always the caller's separate StageAndCommit or
-// StageAllAndCommit call.
+// processes via the lock queue rather than an internal retry loop), PushRebaseFree (a single
+// plain push that never rebases, for callers that supply their own serialization), and
+// DeleteRemoteBranch (a single remote branch deletion, idempotent when the ref is already absent).
+// All four are push-shaped remote calls; committing is always the caller's separate StageAndCommit
+// or StageAllAndCommit call.
 
 package gitrepo
 
@@ -29,6 +30,15 @@ const PushLockFileName = ".gitrepo-push.lock"
 // remote has commits this checkout lacks.
 var rebaseRetryTriggers = []string{"non-fast-forward", "rejected", "fetch first"}
 
+// remoteRefAbsentTrigger is the git-push-delete stderr substring meaning the
+// remote ref was already gone before the delete ran. Git's fuller wording is
+// `error: unable to delete '<branch>': remote ref does not exist`, which
+// contains this substring verbatim, so matching on it alone needs no list. A
+// future git rewording that drops or changes this substring must fail the
+// test that pins it, rather than silently reclassifying the common case as
+// an error.
+const remoteRefAbsentTrigger = "remote ref does not exist"
+
 // Push runs git push, recovering from one non-fast-forward rejection via pull --rebase before
 // retrying.
 // The worktree must be clean.
@@ -102,6 +112,25 @@ func (r *Repo) PushRebaseFree() error {
 	return fmt.Errorf("gitrepo: git push: %w", err)
 }
 
+// DeleteRemoteBranch deletes branch on the named remote via `git push --delete`.
+// A remote ref that does not exist is reported as (false, nil) — an idempotent success recording no
+// mutation and surfacing no error — because every executor in fabric's destruction gate is
+// idempotent for an already-absent target and the common case is a branch that was never pushed.
+// deleted is returned separately from err precisely so the caller can tell "removed it" from "there
+// was nothing there", which is what the caller's mutation record needs.
+func (r *Repo) DeleteRemoteBranch(remote, branch string) (deleted bool, err error) {
+	_, err = r.runChecked("push", remote, "--delete", branch)
+	if err == nil {
+		return true, nil
+	}
+
+	var gitErr *gitexec.GitError
+	if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, remoteRefAbsentTrigger) {
+		return false, nil
+	}
+	return false, fmt.Errorf("gitrepo: git push --delete: %w", err)
+}
+
 // containsAny reports whether s contains any substring from substrs.
 func containsAny(s string, substrs []string) bool {
 	for _, substr := range substrs {
diff --git a/internal/hubgeom/hubgeom.go b/internal/hubgeom/hubgeom.go
index e4669623a..b98944d3c 100644
--- a/internal/hubgeom/hubgeom.go
+++ b/internal/hubgeom/hubgeom.go
@@ -24,6 +24,7 @@ func ReedGeometry(l *lyxcwd.Location) reedengine.Geometry {
 		WorktreeRoot: l.WorktreePath(),
 		LogsDir:      fabricengine.HubLogsDir(l.HubPath),
 		RepoName:     l.RepoName,
+		WorktreeName: l.WorktreeName,
 		HubPath:      l.HubPath,
 	}
 }
diff --git a/internal/hubgeom/hubgeom_test.go b/internal/hubgeom/hubgeom_test.go
index 19da754e5..276b18a85 100644
--- a/internal/hubgeom/hubgeom_test.go
+++ b/internal/hubgeom/hubgeom_test.go
@@ -69,6 +69,9 @@ func TestReedGeometry(t *testing.T) {
 			if got.RepoName != l.RepoName {
 				t.Errorf("ReedGeometry(l).RepoName = %q; want %q", got.RepoName, l.RepoName)
 			}
+			if got.WorktreeName != l.WorktreeName {
+				t.Errorf("ReedGeometry(l).WorktreeName = %q; want %q", got.WorktreeName, l.WorktreeName)
+			}
 			if got.HubPath != hub {
 				t.Errorf("ReedGeometry(l).HubPath = %q; want %q", got.HubPath, hub)
 			}
diff --git a/internal/lifecyclecli/cli.go b/internal/lifecyclecli/cli.go
new file mode 100644
index 000000000..127968e05
--- /dev/null
+++ b/internal/lifecyclecli/cli.go
@@ -0,0 +1,148 @@
+// cli.go builds the cobra command tree for the lifecycle module and the RunCLI/RunCLIIn seams that
+// wire it into the standard io.Writer-based call contract.
+//
+// Package lifecyclecli is the module that drives one task worktree's whole lifecycle as a single
+// Shed run from the hub's prime worktree. It imports internal/lifecycleshed and
+// internal/lifecyclerecipe, neither of which imports cobra, and it is outside the Fabric Vocabulary
+// Invariant's owner set, so no identifier, literal, or comment in this package -- or in either of
+// those two it imports -- may name either side of the pair: write "the task worktree", "the pair",
+// and "the hub's prime worktree" instead.
+package lifecyclecli
+
+import (
+	"io"
+
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+	"github.com/Knatte18/loomyard/internal/output"
+	"github.com/Knatte18/loomyard/internal/shedrecipe"
+	"github.com/spf13/cobra"
+)
+
+// lifecycleCLI carries the fields wire (wire.go) populates; every lifecycle verb hangs off this
+// receiver.
+type lifecycleCLI struct {
+	// location is the resolved *lyxcwd.Location for the hub's prime worktree. wire reads it to
+	// anchor every path constructor and every seam.
+	location *lyxcwd.Location
+	// env is the assembled shedrecipe.Env the run verb passes to lifecyclerecipe.New.
+	env shedrecipe.Env
+	// shedPaths carries the five told values shedengine.Shed itself reads, which the run verb
+	// passes alongside env, and which the status verb reads directly.
+	shedPaths lifecyclerecipe.ShedPaths
+	// slug is the task slug read from the command's own arguments.
+	slug string
+	// abandonedSession is the value the Teardown.Shutdown seam records on the receiver -- carried
+	// here rather than only returned from the seam, so the run verb can surface it on the envelope
+	// after the Shed's own Run has returned.
+	abandonedSession string
+}
+
+// Command returns the cobra command tree for the lifecycle module.
+func Command() *cobra.Command {
+	c := &lifecycleCLI{}
+
+	parent := &cobra.Command{
+		Use:   "lifecycle",
+		Short: "drive one task worktree's whole lifecycle as a single Shed run",
+		Long: `lifecycle drives one task worktree's whole lifecycle -- create, run the loom
+session to a terminal state, and tear down -- as a single Shed run over a
+per-slug status.json. "run" starts or resumes that run for a slug; "status"
+reports its current state.
+
+Both verbs run from the hub's prime worktree only: they refuse when invoked
+from a task worktree.
+
+Example:
+  lyx lifecycle run some-slug
+  lyx lifecycle status some-slug`,
+		// RunE is set so that bare "lyx lifecycle" lists subcommands and "lyx lifecycle bogus"
+		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
+		RunE:              clihelp.GroupRunE,
+		PersistentPreRunE: c.resolvePersistentPreRun,
+	}
+
+	parent.AddCommand(c.runCmd(), c.statusCmd())
+
+	return parent
+}
+
+// resolvePersistentPreRun resolves cwd via lyxcwd.CwdFrom, resolves it into a *lyxcwd.Location via
+// lyxcwd.Resolve, resolves the prime name via fabricengine.PrimeName, and applies the non-prime
+// refusal (refusal.go) before resolving anything further. It then reads the slug from the command's
+// own arguments and calls wire.
+//
+// Skips resolution entirely when the lifecycle group command itself is invoked (bare listing or
+// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
+// be present.
+func (c *lifecycleCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
+	if cmd.Name() == "lifecycle" {
+		return nil
+	}
+
+	ctx := cmd.Context()
+	out := cmd.OutOrStdout()
+
+	cwd, err := lyxcwd.CwdFrom(ctx)
+	if err != nil {
+		output.Err(out, err.Error())
+		clihelp.Abort(ctx, 1)
+		return nil
+	}
+
+	location, err := lyxcwd.Resolve(cwd)
+	if err != nil {
+		// lyxcwd.Resolve's error is already self-describing (it IS the "not a git repository"
+		// sentinel); pass it through bare rather than doubling that same text on top of it.
+		output.Err(out, err.Error())
+		clihelp.Abort(ctx, 1)
+		return nil
+	}
+
+	primeName, primeNameErr := fabricengine.PrimeName(location)
+	if refusalErr := refuseNonPrime(location.WorktreeName, primeName, primeNameErr); refusalErr != nil {
+		output.Err(out, refusalErr.Error())
+		clihelp.Abort(ctx, 1)
+		return nil
+	}
+
+	slug := ""
+	if len(args) > 0 {
+		slug = args[0]
+	}
+	c.location = location
+	c.slug = slug
+
+	if err := c.wire(location, slug); err != nil {
+		output.Err(out, err.Error())
+		clihelp.Abort(ctx, 1)
+		return nil
+	}
+	return nil
+}
+
+// RunCLI is the public seam for the lifecycle module CLI.
+//
+// It delegates to clihelp.Execute with the cobra command tree, passing out as the capture writer
+// for all output (including cobra's error text).
+func RunCLI(out io.Writer, args []string) int {
+	return RunCLIIn("", out, args)
+}
+
+// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
+// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
+// the execution context via clihelp.ExecuteIn.
+//
+// RunCLIIn is carried rather than skipped for a concrete reason: the path-derivation tests serving
+// as the Lifecycle Bookend Invariant's mechanical proxy and the non-prime refusal test both need an
+// injectable cwd, and neither is reachable through RunCLI alone. The branch exists because
+// lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to ExecuteIn would panic on
+// every existing RunCLI call.
+func RunCLIIn(cwd string, out io.Writer, args []string) int {
+	if cwd == "" {
+		return clihelp.Execute(Command(), out, args)
+	}
+	return clihelp.ExecuteIn(Command(), cwd, out, args)
+}
diff --git a/internal/lifecyclecli/cli_test.go b/internal/lifecyclecli/cli_test.go
new file mode 100644
index 000000000..a784c7bfc
--- /dev/null
+++ b/internal/lifecyclecli/cli_test.go
@@ -0,0 +1,118 @@
+// cli_test.go covers the lifecyclecli cobra seam: the built tree's Short completeness, the exact set
+// of registered verbs, each verb's argument-count rejection, the bare-group invocation's git-free
+// guard, and the unknown-subcommand JSON error envelope -- mirroring internal/loomcli/cli_test.go's
+// shape.
+
+package lifecyclecli
+
+import (
+	"bytes"
+	"sort"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/spf13/cobra"
+)
+
+// TestCommand_EveryCommandHasShort walks the full lifecycle command tree and asserts that every
+// command -- the parent group and every subcommand -- carries a non-empty Short, per the CLI/Cobra
+// Invariant.
+func TestCommand_EveryCommandHasShort(t *testing.T) {
+	var walk func(cmd *cobra.Command)
+	walk = func(cmd *cobra.Command) {
+		if cmd.Short == "" {
+			t.Errorf("command %q has empty Short", cmd.CommandPath())
+		}
+		for _, sub := range cmd.Commands() {
+			walk(sub)
+		}
+	}
+	walk(Command())
+}
+
+// TestCommand_RegisteredVerbs_ExactSet asserts that the parent command's registered subcommands are
+// exactly the two lifecycle verbs, no more and no fewer.
+func TestCommand_RegisteredVerbs_ExactSet(t *testing.T) {
+	parent := Command()
+
+	var got []string
+	for _, sub := range parent.Commands() {
+		name := sub.Name()
+		if name == "help" || name == "completion" {
+			continue
+		}
+		got = append(got, name)
+	}
+	sort.Strings(got)
+
+	want := []string{"run", "status"}
+
+	gotSet := make(map[string]bool, len(got))
+	for _, name := range got {
+		gotSet[name] = true
+	}
+	wantSet := make(map[string]bool, len(want))
+	for _, name := range want {
+		wantSet[name] = true
+	}
+
+	for _, name := range want {
+		if !gotSet[name] {
+			t.Errorf("verb %q is not registered under the lifecycle parent command", name)
+		}
+	}
+	for _, name := range got {
+		if !wantSet[name] {
+			t.Errorf("unexpected verb %q is registered under the lifecycle parent command", name)
+		}
+	}
+}
+
+// TestCommand_EveryVerbRejectsWrongArgCount asserts that both verbs reject zero arguments and two
+// arguments -- each takes exactly one, the slug.
+func TestCommand_EveryVerbRejectsWrongArgCount(t *testing.T) {
+	for _, verb := range []string{"run", "status"} {
+		for _, args := range [][]string{{verb}, {verb, "a", "b"}} {
+			t.Run(verb+"_"+strings.Join(args, "_"), func(t *testing.T) {
+				var out bytes.Buffer
+				exitCode := clihelp.Execute(Command(), &out, args)
+				if exitCode != 1 {
+					t.Errorf("Execute(%v) = %d; want 1", args, exitCode)
+				}
+			})
+		}
+	}
+}
+
+// TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lyx lifecycle" invocation succeeds
+// without needing a git repository, proving the PersistentPreRunE guard for cmd.Name() ==
+// "lifecycle" fires before any cwd resolution.
+func TestRunCLI_GroupGuard_NoGitRepoNeeded(t *testing.T) {
+	t.Parallel()
+
+	dir := t.TempDir()
+	var out bytes.Buffer
+	exitCode := RunCLIIn(dir, &out, nil)
+
+	if exitCode != 0 {
+		t.Errorf("RunCLIIn(%q, nil) = %d; want 0", dir, exitCode)
+	}
+}
+
+// TestRunCLI_UnknownSubcommand_NoGitRepoNeeded asserts an unknown subcommand also skips cwd
+// resolution and emits a JSON error envelope.
+func TestRunCLI_UnknownSubcommand_NoGitRepoNeeded(t *testing.T) {
+	t.Parallel()
+
+	dir := t.TempDir()
+	var out bytes.Buffer
+	exitCode := RunCLIIn(dir, &out, []string{"bogus"})
+
+	if exitCode != 1 {
+		t.Errorf("RunCLIIn(%q, [bogus]) = %d; want 1", dir, exitCode)
+	}
+	if !strings.Contains(out.String(), `"ok":false`) {
+		t.Errorf("RunCLIIn(%q, [bogus]) output missing ok:false envelope; got: %q", dir, out.String())
+	}
+}
diff --git a/internal/lifecyclecli/lifecycle_integration_test.go b/internal/lifecyclecli/lifecycle_integration_test.go
new file mode 100644
index 000000000..7ce9fb183
--- /dev/null
+++ b/internal/lifecyclecli/lifecycle_integration_test.go
@@ -0,0 +1,239 @@
+//go:build integration
+
+// lifecycle_integration_test.go is the end-to-end suite over a real hub built by
+// internal/hubforge through its fabric fixture entry point, per the hubforge Fabric-Fixture
+// Invariant. It stays a white-box "package lifecyclecli" test, not an external "_test" package,
+// because it stubs Env.LoomRun.Spawn and Env.LoomRun.ReadStatus at the field level after a real
+// wire() call -- a no-op spawn and a read-status answering a chosen state -- so the real poll logic
+// (Env.LoomRun.ResolveStatus, the persisted-state branching) is exercised rather than bypassed, and
+// that stubbing needs the unexported wire method and the lifecycleCLI receiver.
+//
+// It lives at the integration tier rather than Tier 1 because the prime-name lookup this package's
+// own pre-run refusal performs reaches a real git worktree listing, and getting there at all needs
+// the resolver -- both barred from untagged files by the Test Tier Purity Invariant.
+
+package lifecyclecli
+
+import (
+	"bytes"
+	"context"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/hubforge"
+	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+	"github.com/Knatte18/loomyard/internal/state"
+)
+
+// wireForHub builds a *lifecycleCLI wired for real against h's prime Location and slug -- a real
+// CreateWorktree and a real Teardown, both driving fabricengine's topology holder against h's own
+// hub -- then overrides Env.LoomRun.Spawn and Env.LoomRun.ReadStatus with readStatus, per this
+// file's own header.
+func wireForHub(t *testing.T, h *hubforge.Hub, slug string, readStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)) *lifecycleCLI {
+	t.Helper()
+	c := &lifecycleCLI{}
+	if err := c.wire(h.Location, slug); err != nil {
+		t.Fatalf("wire(%s): %v", slug, err)
+	}
+	c.env.LoomRun.Spawn = func(ctx context.Context) error { return nil }
+	c.env.LoomRun.ReadStatus = readStatus
+	return c
+}
+
+// seedEntryStatus writes c's status file with CurrentProducer/State as given, an empty non-nil
+// History, under c's own StatusPath/StatusLockPath.
+func seedEntryStatus(t *testing.T, c *lifecycleCLI, producer string, rowState shedengine.State, history []shedengine.HistoryEntry) {
+	t.Helper()
+	if history == nil {
+		history = []shedengine.HistoryEntry{}
+	}
+	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{
+		CurrentProducer: producer,
+		State:           rowState,
+		History:         history,
+	}); err != nil {
+		t.Fatalf("seed status: %v", err)
+	}
+}
+
+// pathExists reports whether path exists on disk.
+func pathExists(path string) bool {
+	_, err := os.Stat(path)
+	return err == nil
+}
+
+// TestLifecycleIntegration_CreateThenTeardown_DoneRemovesThePair drives a spawn stub plus a
+// read-status answering StateDone through the whole three-row list one row at a time, asserting the
+// pair exists on disk after the create row and is gone after the teardown row.
+func TestLifecycleIntegration_CreateThenTeardown_DoneRemovesThePair(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	slug := "lifecycle-done"
+
+	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+		return shedengine.Status{State: shedengine.StateDone}, true, nil
+	})
+	seedEntryStatus(t, c, lifecyclerecipe.NameWorktreeCreate, shedengine.StateRunning, nil)
+
+	shed, err := lifecyclerecipe.New(c.env, c.shedPaths)
+	if err != nil {
+		t.Fatalf("lifecyclerecipe.New: %v", err)
+	}
+
+	ctx := context.Background()
+	pairPath := h.PairWarpWorktree(slug)
+
+	if _, err := shed.Step(ctx); err != nil {
+		t.Fatalf("Step (create row): %v", err)
+	}
+	if !pathExists(pairPath) {
+		t.Fatalf("pair does not exist after the create row: %s", pairPath)
+	}
+
+	if _, err := shed.Step(ctx); err != nil {
+		t.Fatalf("Step (loom-run row): %v", err)
+	}
+	if !pathExists(pairPath) {
+		t.Fatalf("pair does not exist after the loom-run row: %s", pairPath)
+	}
+
+	if _, err := shed.Step(ctx); err != nil {
+		t.Fatalf("Step (teardown row): %v", err)
+	}
+	if pathExists(pairPath) {
+		t.Errorf("pair still exists after the teardown row: %s", pairPath)
+	}
+}
+
+// TestLifecycleIntegration_LoomRunBlocked_LeavesThePairIntact drives a read-status answering
+// StateBlocked, asserting the run halts blocked with the task worktree still present -- the safety
+// property the whole design turns on.
+func TestLifecycleIntegration_LoomRunBlocked_LeavesThePairIntact(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	slug := "lifecycle-blocked"
+
+	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+		return shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "loom-side-producer", Error: "loom session blocked"}, true, nil
+	})
+	seedEntryStatus(t, c, lifecyclerecipe.NameWorktreeCreate, shedengine.StateRunning, nil)
+
+	shed, err := lifecyclerecipe.New(c.env, c.shedPaths)
+	if err != nil {
+		t.Fatalf("lifecyclerecipe.New: %v", err)
+	}
+
+	result, err := shed.Run(context.Background())
+	if err != nil {
+		t.Fatalf("Run: %v", err)
+	}
+	if result.Outcome != shedengine.RunBlocked {
+		t.Errorf("Outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
+	}
+	if !pathExists(h.PairWarpWorktree(slug)) {
+		t.Errorf("pair does not exist after a blocked run; want it left intact: %s", h.PairWarpWorktree(slug))
+	}
+}
+
+// TestLifecycleIntegration_DirtyPrime_CreateRowBlocksBeforeAnythingCreated dirties a tracked file
+// in the hub's prime worktree, asserting the create row halts blocked before anything is created --
+// this refusal fires on every lifecycle run and is invisible to the unit tests' fakes.
+func TestLifecycleIntegration_DirtyPrime_CreateRowBlocksBeforeAnythingCreated(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	slug := "lifecycle-dirty-prime"
+
+	readmePath := filepath.Join(h.PrimeWorktree(), "README")
+	if err := os.WriteFile(readmePath, []byte("dirtied for the test\n"), 0o644); err != nil {
+		t.Fatalf("dirty prime README: %v", err)
+	}
+
+	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+		t.Fatal("ReadStatus must not be called: the create row must block before the poll row ever runs")
+		return shedengine.Status{}, false, nil
+	})
+	seedEntryStatus(t, c, lifecyclerecipe.NameWorktreeCreate, shedengine.StateRunning, nil)
+
+	shed, err := lifecyclerecipe.New(c.env, c.shedPaths)
+	if err != nil {
+		t.Fatalf("lifecyclerecipe.New: %v", err)
+	}
+
+	result, err := shed.Run(context.Background())
+	if err != nil {
+		t.Fatalf("Run: %v", err)
+	}
+	if result.Outcome != shedengine.RunBlocked {
+		t.Errorf("Outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
+	}
+	if result.HaltedProducer != lifecyclerecipe.NameWorktreeCreate {
+		t.Errorf("HaltedProducer = %q; want %q", result.HaltedProducer, lifecyclerecipe.NameWorktreeCreate)
+	}
+	if pathExists(h.PairWarpWorktree(slug)) {
+		t.Errorf("pair exists even though the create row blocked before creating anything: %s", h.PairWarpWorktree(slug))
+	}
+}
+
+// TestLifecycleIntegration_MidListResume_SkipsTheCompletedCreateRow proves mid-list resume: it
+// creates the pair directly (standing in for a create row that already completed before a crash),
+// seeds a status file whose current producer is the poll row, and re-invokes the run verb --
+// asserting CreateWorktree is never called again and the pair is torn down once the poll row
+// answers Done.
+func TestLifecycleIntegration_MidListResume_SkipsTheCompletedCreateRow(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	slug := "lifecycle-resume"
+	hubforge.AddPair(t, h, slug)
+
+	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+		return shedengine.Status{State: shedengine.StateDone}, true, nil
+	})
+	c.env.CreateWorktree = func(ctx context.Context) error {
+		t.Fatal("CreateWorktree must not run again: the create row already completed before the crash this test simulates")
+		return nil
+	}
+	seedEntryStatus(t, c, lifecyclerecipe.NameLoomRun, shedengine.StateBlocked, []shedengine.HistoryEntry{
+		{Producer: lifecyclerecipe.NameWorktreeCreate, Outcome: shedengine.Done},
+	})
+
+	var out bytes.Buffer
+	exitCode := clihelp.Execute(c.runCmd(), &out, []string{slug})
+	if exitCode != 0 {
+		t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
+	}
+	if !strings.Contains(out.String(), `"ok":true`) {
+		t.Errorf("run() output missing ok:true envelope; got: %q", out.String())
+	}
+	if pathExists(h.PairWarpWorktree(slug)) {
+		t.Errorf("pair still exists after the resumed run's teardown row completed: %s", h.PairWarpWorktree(slug))
+	}
+}
+
+// TestLifecycleIntegration_NonPrimeRefusal covers both verbs' non-prime refusal, driven through
+// RunCLIIn with an injected cwd pointing at a real task worktree -- the runtime check standing in
+// for the Bookend invariant's missing enforcing test.
+func TestLifecycleIntegration_NonPrimeRefusal(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	taskSlug := "lifecycle-task-cwd"
+	hubforge.AddPair(t, h, taskSlug)
+
+	taskCwd := h.PairWarpWorktree(taskSlug)
+	primeName := h.Location.WorktreeName
+
+	for _, verb := range []string{"run", "status"} {
+		t.Run(verb, func(t *testing.T) {
+			var out bytes.Buffer
+			exitCode := RunCLIIn(taskCwd, &out, []string{verb, "some-slug"})
+
+			if exitCode != 1 {
+				t.Fatalf("RunCLIIn(%s) exit code = %d; want 1; output: %s", verb, exitCode, out.String())
+			}
+			if !strings.Contains(out.String(), taskSlug) {
+				t.Errorf("%s refusal = %q; want it to name the task worktree %q", verb, out.String(), taskSlug)
+			}
+			if !strings.Contains(out.String(), primeName) {
+				t.Errorf("%s refusal = %q; want it to name the prime worktree %q", verb, out.String(), primeName)
+			}
+		})
+	}
+}
diff --git a/internal/lifecyclecli/paths.go b/internal/lifecyclecli/paths.go
new file mode 100644
index 000000000..7f83648fd
--- /dev/null
+++ b/internal/lifecyclecli/paths.go
@@ -0,0 +1,62 @@
+// paths.go declares the five prime-anchored lifecycle path constructors: LifecycleDir, StatusFile,
+// RunLock, StatusLock, and PrimeRunLock. Every one of them is a plain filepath.Join onto the given
+// *lyxcwd.Location's AnchorPath(), per the Cwd Resolution Invariant -- none of them calls os.Getwd
+// or any git command.
+//
+// This whole tree is ephemeral, not durable, unlike loom's own status file (loomengine.LoomStatusFile
+// lives under _lyx and is fabric-synced): the lifecycle's state -- which task worktree is mid-create,
+// which lock is held, what a run last observed -- is per-machine and per-attempt, never meant to be
+// committed or shared between machines working the same hub. Per the Durable-vs-Ephemeral State
+// Invariant, every never-tracked file lives under .lyx, so this package's whole tree sits there
+// rather than under _lyx.
+
+package lifecyclecli
+
+import (
+	"path/filepath"
+
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+	"github.com/Knatte18/loomyard/internal/lyxdirs"
+)
+
+// lifecycleDirName is the relative-path segment lifecyclecli joins onto lyxdirs.DotLyxDirName to
+// scope every lifecycle-owned path under its own subdirectory.
+// lifecyclecli is this segment's sole declarer.
+const lifecycleDirName = "lifecycle"
+
+// LifecycleDir returns the path to the per-slug lifecycle directory: the prime *lyxcwd.Location's
+// AnchorPath() joined with lyxdirs.DotLyxDirName, lifecycleDirName, and slug.
+// The .lyx segment comes from lyxdirs.DotLyxDirName rather than a literal, per the Lyxdirs
+// Single-Declarer Invariant, exactly as loomengine.LoomStatusLock already does.
+func LifecycleDir(l *lyxcwd.Location, slug string) string {
+	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, lifecycleDirName, slug)
+}
+
+// StatusFile returns the path to a slug's persisted lifecycle status.json, under LifecycleDir(l,
+// slug).
+func StatusFile(l *lyxcwd.Location, slug string) string {
+	return filepath.Join(LifecycleDir(l, slug), "status.json")
+}
+
+// RunLock returns the path to a slug's lifecycle run lock, under LifecycleDir(l, slug).
+// It must never equal StatusLock(l, slug): shedengine.Shed's own validation rejects LockPath ==
+// StatusLockPath outright, and a shared file would hang on the first persist rather than fail.
+func RunLock(l *lyxcwd.Location, slug string) string {
+	return filepath.Join(LifecycleDir(l, slug), "run.lock")
+}
+
+// StatusLock returns the path to the advisory lock guarding concurrent access to StatusFile(l,
+// slug), under LifecycleDir(l, slug).
+func StatusLock(l *lyxcwd.Location, slug string) string {
+	return filepath.Join(LifecycleDir(l, slug), "status.json.lock")
+}
+
+// PrimeRunLock returns the path to the hub-scoped advisory lock that serialises every slug's
+// WorktreeCreate and WorktreeTeardown rows against one another: the prime *lyxcwd.Location's
+// AnchorPath() joined with lyxdirs.DotLyxDirName and lifecycleDirName, one level above any single
+// slug's own LifecycleDir.
+// Unlike the four accessors above, PrimeRunLock takes no slug: the lock it names is shared across
+// every task worktree this hub creates or tears down, not scoped to one.
+func PrimeRunLock(l *lyxcwd.Location) string {
+	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, lifecycleDirName, "run.lock")
+}
diff --git a/internal/lifecyclecli/paths_test.go b/internal/lifecyclecli/paths_test.go
new file mode 100644
index 000000000..595eaf6f9
--- /dev/null
+++ b/internal/lifecyclecli/paths_test.go
@@ -0,0 +1,122 @@
+package lifecyclecli
+
+import (
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+)
+
+// locationFixtures returns two synthetic *lyxcwd.Location fixtures: one anchored at the worktree
+// root ("."), and one anchored at a subpath, so every path constructor is exercised against both
+// anchoring shapes.
+func locationFixtures(t *testing.T) map[string]*lyxcwd.Location {
+	t.Helper()
+	hub := t.TempDir()
+	return map[string]*lyxcwd.Location{
+		"RootAnchored": {
+			RepoName:     "example",
+			HubPath:      hub,
+			WorktreeName: "task-slug",
+			AnchorRel:    ".",
+		},
+		"SubpathAnchored": {
+			RepoName:     "example",
+			HubPath:      hub,
+			WorktreeName: "task-slug",
+			AnchorRel:    "backend",
+		},
+	}
+}
+
+func TestLifecycleDir(t *testing.T) {
+	for name, l := range locationFixtures(t) {
+		t.Run(name, func(t *testing.T) {
+			got := LifecycleDir(l, "some-slug")
+			want := filepath.Join(l.AnchorPath(), ".lyx", "lifecycle", "some-slug")
+			if got != want {
+				t.Errorf("LifecycleDir() = %q; want %q", got, want)
+			}
+		})
+	}
+}
+
+func TestPerSlugPathsAreDistinctAndUnderLifecycleDir(t *testing.T) {
+	for name, l := range locationFixtures(t) {
+		t.Run(name, func(t *testing.T) {
+			dir := LifecycleDir(l, "some-slug")
+			statusFile := StatusFile(l, "some-slug")
+			runLock := RunLock(l, "some-slug")
+			statusLock := StatusLock(l, "some-slug")
+
+			for _, p := range []struct {
+				name string
+				path string
+			}{
+				{"StatusFile", statusFile},
+				{"RunLock", runLock},
+				{"StatusLock", statusLock},
+			} {
+				if !strings.HasPrefix(p.path, dir+string(filepath.Separator)) {
+					t.Errorf("%s() = %q; want it under LifecycleDir %q", p.name, p.path, dir)
+				}
+			}
+
+			// RunLock differing from StatusLock in particular is what shedengine.Shed's own
+			// validation rejects outright, and it must not first surface at runtime.
+			if runLock == statusLock {
+				t.Errorf("RunLock() = StatusLock() = %q; want distinct paths", runLock)
+			}
+			if statusFile == runLock {
+				t.Errorf("StatusFile() = RunLock() = %q; want distinct paths", statusFile)
+			}
+			if statusFile == statusLock {
+				t.Errorf("StatusFile() = StatusLock() = %q; want distinct paths", statusFile)
+			}
+		})
+	}
+}
+
+func TestPrimeRunLock(t *testing.T) {
+	for name, l := range locationFixtures(t) {
+		t.Run(name, func(t *testing.T) {
+			got := PrimeRunLock(l)
+			want := filepath.Join(l.AnchorPath(), ".lyx", "lifecycle", "run.lock")
+			if got != want {
+				t.Errorf("PrimeRunLock() = %q; want %q", got, want)
+			}
+		})
+	}
+}
+
+// TestPathsStayUnderAnchorAndNeverNameTheManagedSlugWorktree asserts every returned path is under
+// the given Location's own anchor, and that none of them contains a managed task worktree's own
+// path -- this package's half of the Lifecycle Bookend Invariant's mechanical proxy: every seam this
+// package builds resolves against the prime Location's own anchored tree, never against the managed
+// slug's worktree path, which does not exist at wiring time.
+func TestPathsStayUnderAnchorAndNeverNameTheManagedSlugWorktree(t *testing.T) {
+	for name, l := range locationFixtures(t) {
+		t.Run(name, func(t *testing.T) {
+			const managedSlug = "managed-task-slug"
+			managedWorktreePath := filepath.Join(l.HubPath, managedSlug)
+
+			paths := map[string]string{
+				"LifecycleDir": LifecycleDir(l, managedSlug),
+				"StatusFile":   StatusFile(l, managedSlug),
+				"RunLock":      RunLock(l, managedSlug),
+				"StatusLock":   StatusLock(l, managedSlug),
+				"PrimeRunLock": PrimeRunLock(l),
+			}
+
+			for name, p := range paths {
+				if !strings.HasPrefix(p, l.AnchorPath()+string(filepath.Separator)) && p != l.AnchorPath() {
+					t.Errorf("%s() = %q; want it under Location's own anchor %q", name, p, l.AnchorPath())
+				}
+				if strings.Contains(p, managedWorktreePath) {
+					t.Errorf("%s() = %q; want it to never contain the managed task worktree's own path %q", name, p, managedWorktreePath)
+				}
+			}
+		})
+	}
+}
diff --git a/internal/lifecyclecli/refusal.go b/internal/lifecyclecli/refusal.go
new file mode 100644
index 000000000..f44df812d
--- /dev/null
+++ b/internal/lifecyclecli/refusal.go
@@ -0,0 +1,42 @@
+// refusal.go declares refuseNonPrime, the pure decision behind this package's pre-run refusal: a
+// lifecycle verb runs only when invoked from the hub's prime worktree.
+//
+// The check is added rather than inherited. internal/fabricengine's own topology layer refuses
+// removing the hub's prime slug (refusePrimeSlug in remove.go), but that check compares a NAMED
+// slug argument against the prime name -- it says nothing about which worktree the caller is
+// standing in, and never refuses removing the worktree that IS the caller's own process working
+// directory. Without a check of its own, a lifecycle run driven from a task worktree could tear
+// down the very worktree it is running inside of, which is exactly what this package's two bookend
+// rows (create and teardown) must never do.
+//
+// This is also why refuseNonPrime treats a non-nil primeNameErr as a refusal rather than passing it
+// through unresolved, in deliberate contrast with refusePrimeSlug's own choice: refusePrimeSlug
+// treats an unresolvable prime name as non-fatal because it is one guard among several that still
+// refuse a bad slug on other grounds, while here it is the WHOLE check -- an unresolvable prime name
+// means the hub geometry is already broken, and treating that as "probably fine, proceed" would
+// drive the two bookend rows from an unverified vantage point, which is precisely what this check
+// exists to prevent.
+
+package lifecyclecli
+
+import "fmt"
+
+// refuseNonPrime returns nil only when primeNameErr is nil and worktreeName equals primeName.
+//
+// When primeNameErr is non-nil it returns a refusal naming that error -- never nil and never a hard
+// error of its own, so the caller can always render it on the envelope.
+//
+// When the two names differ it returns a refusal naming both, stating that this verb runs from the
+// hub's prime worktree only, and telling the operator to re-run it there.
+func refuseNonPrime(worktreeName, primeName string, primeNameErr error) error {
+	if primeNameErr != nil {
+		return fmt.Errorf("lifecyclecli: cannot verify this is the hub's prime worktree: %w", primeNameErr)
+	}
+	if worktreeName == primeName {
+		return nil
+	}
+	return fmt.Errorf(
+		"lifecyclecli: this verb runs from the hub's prime worktree only; %q is not the prime worktree (%q is) -- re-run it from there",
+		worktreeName, primeName,
+	)
+}
diff --git a/internal/lifecyclecli/refusal_test.go b/internal/lifecyclecli/refusal_test.go
new file mode 100644
index 000000000..53ff4af17
--- /dev/null
+++ b/internal/lifecyclecli/refusal_test.go
@@ -0,0 +1,62 @@
+package lifecyclecli
+
+import (
+	"errors"
+	"strings"
+	"testing"
+)
+
+func TestRefuseNonPrime(t *testing.T) {
+	tests := []struct {
+		name         string
+		worktreeName string
+		primeName    string
+		primeNameErr error
+		wantErr      bool
+	}{
+		{
+			name:         "PrimeNameErrRefusesRegardlessOfNames",
+			worktreeName: "some-worktree",
+			primeName:    "some-worktree",
+			primeNameErr: errors.New("no main worktree found"),
+			wantErr:      true,
+		},
+		{
+			name:         "MatchingNamesWithNoErrorSucceeds",
+			worktreeName: "hub-repo",
+			primeName:    "hub-repo",
+			primeNameErr: nil,
+			wantErr:      false,
+		},
+		{
+			name:         "DifferingNamesRefuses",
+			worktreeName: "task-slug",
+			primeName:    "hub-repo",
+			primeNameErr: nil,
+			wantErr:      true,
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			err := refuseNonPrime(tt.worktreeName, tt.primeName, tt.primeNameErr)
+			if (err != nil) != tt.wantErr {
+				t.Fatalf("refuseNonPrime(%q, %q, %v) error = %v; wantErr %v", tt.worktreeName, tt.primeName, tt.primeNameErr, err, tt.wantErr)
+			}
+		})
+	}
+}
+
+func TestRefuseNonPrime_MessagesAreDistinguishable(t *testing.T) {
+	errFromPrimeNameErr := refuseNonPrime("some-worktree", "some-worktree", errors.New("no main worktree found"))
+	errFromNameMismatch := refuseNonPrime("task-slug", "hub-repo", nil)
+
+	if errFromPrimeNameErr == nil || errFromNameMismatch == nil {
+		t.Fatalf("both refusals must be non-nil; got %v and %v", errFromPrimeNameErr, errFromNameMismatch)
+	}
+	if errFromPrimeNameErr.Error() == errFromNameMismatch.Error() {
+		t.Errorf("the two refusal messages must be distinguishable; both read %q", errFromPrimeNameErr.Error())
+	}
+	if !strings.Contains(errFromNameMismatch.Error(), "task-slug") || !strings.Contains(errFromNameMismatch.Error(), "hub-repo") {
+		t.Errorf("name-mismatch refusal = %q; want it to name both %q and %q", errFromNameMismatch.Error(), "task-slug", "hub-repo")
+	}
+}
diff --git a/internal/lifecyclecli/run.go b/internal/lifecyclecli/run.go
new file mode 100644
index 000000000..1e0059a6f
--- /dev/null
+++ b/internal/lifecyclecli/run.go
@@ -0,0 +1,124 @@
+// run.go implements the `run` lifecycle verb: it starts or resumes one slug's whole lifecycle run
+// to its next halt.
+
+package lifecyclecli
+
+import (
+	"errors"
+	"fmt"
+
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
+	"github.com/Knatte18/loomyard/internal/output"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+	"github.com/Knatte18/loomyard/internal/state"
+	"github.com/spf13/cobra"
+)
+
+// runCmd builds the `run <slug>` subcommand.
+func (c *lifecycleCLI) runCmd() *cobra.Command {
+	cmd := &cobra.Command{
+		Use:   "run <slug>",
+		Short: "start or resume a task worktree's whole lifecycle run to its next halt",
+		Long: `run drives the pair's create/run/teardown lifecycle to its next halt: done,
+blocked, or paused. Invoked against a slug with no persisted status, it
+starts a fresh run. Invoked against one already in progress, it resumes
+silently from the persisted current producer -- there is no re-seed flag,
+because every operator-fixable refusal this task raises lands as blocked,
+which is the everyday resume path. A slug already done refuses on the
+envelope, naming the per-slug directory to delete to run it again.
+
+Example:
+  lyx lifecycle run some-slug`,
+		Args: cobra.ExactArgs(1),
+		RunE: func(cmd *cobra.Command, args []string) error {
+			if clihelp.ShouldAbort(cmd.Context()) {
+				return nil
+			}
+			ctx := cmd.Context()
+			out := cmd.OutOrStdout()
+
+			st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
+			if err != nil {
+				clihelp.SetExit(ctx, output.Err(out, "lifecyclecli: decode status file "+c.shedPaths.StatusPath+": "+err.Error()))
+				return nil
+			}
+			if found {
+				switch st.State {
+				case shedengine.StateDone:
+					clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf(
+						"lifecyclecli: %q has already completed; delete %s to run it again",
+						c.slug, LifecycleDir(c.location, c.slug),
+					)))
+					return nil
+				case shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StatePaused:
+					// Each of these resumes silently from the persisted current producer, with no
+					// re-seed, no prompt, and no flag: the engine itself already resumes from
+					// blocked and failed, and StateBlocked is the everyday path, since every
+					// operator-fixable refusal in this task lands there.
+				default:
+					clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("lifecyclecli: unrecognized status state %q", st.State)))
+					return nil
+				}
+			} else {
+				// An absent status file is a fresh start: shedengine.Shed.Run refuses to walk from a
+				// status file that does not exist yet (it never seeds one itself), so this verb seeds
+				// it here, at the entry row, before the Shed ever reads it. The mutate closure is
+				// idempotent against a concurrently-seeded file: it leaves an already-present status
+				// untouched rather than overwriting it.
+				if err := state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
+					if found {
+						return cur, nil
+					}
+					return shedengine.Status{
+						CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
+						State:           shedengine.StateRunning,
+						History:         []shedengine.HistoryEntry{},
+					}, nil
+				}); err != nil {
+					clihelp.SetExit(ctx, output.Err(out, err.Error()))
+					return nil
+				}
+			}
+
+			shed, err := lifecyclerecipe.New(c.env, c.shedPaths)
+			if err != nil {
+				clihelp.SetExit(ctx, output.Err(out, err.Error()))
+				return nil
+			}
+
+			result, err := shed.Run(ctx)
+			if err != nil {
+				if errors.Is(err, shedengine.ErrShedBusy) {
+					// The engine already takes the run lock non-blocking for the whole of one run,
+					// so refusing here is its native behaviour; this verb's only job is to report
+					// it legibly -- naming the lock path -- rather than surfacing a raw lock error.
+					// There is no wait and no second driver spawned.
+					clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf(
+						"lifecyclecli: another lifecycle run already holds the run lock %q", c.shedPaths.LockPath,
+					)))
+					return nil
+				}
+				clihelp.SetExit(ctx, output.Err(out, err.Error()))
+				return nil
+			}
+
+			fields := map[string]any{
+				"outcome":         string(result.Outcome),
+				"halted_producer": result.HaltedProducer,
+				"reason":          result.Reason,
+			}
+			// The envelope deliberately carries neither a mutations array nor a partial bool: a run
+			// may perform zero, one, or two topology mutations at arbitrary points hours apart, so
+			// there is no coherent single array at run scope, and partial has no referent here. The
+			// mutation records are logged at Info by the wiring closures (wire.go) instead of
+			// discarded.
+			if c.abandonedSession != "" {
+				fields["abandonedSession"] = c.abandonedSession
+			}
+			clihelp.SetExit(ctx, output.Ok(out, fields))
+			return nil
+		},
+	}
+	return cmd
+}
diff --git a/internal/lifecyclecli/run_test.go b/internal/lifecyclecli/run_test.go
new file mode 100644
index 000000000..60394ad3c
--- /dev/null
+++ b/internal/lifecyclecli/run_test.go
@@ -0,0 +1,211 @@
+// run_test.go drives the run verb's four dispositions against a hand-populated receiver and a
+// hand-written status file under t.TempDir(), bypassing wire entirely: every Env seam here is a
+// fake that performs no real I/O, no git spawn, and no process spawn.
+
+package lifecyclecli
+
+import (
+	"bytes"
+	"context"
+	"encoding/json"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
+	"github.com/Knatte18/loomyard/internal/lifecycleshed"
+	"github.com/Knatte18/loomyard/internal/lock"
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+	"github.com/Knatte18/loomyard/internal/shedrecipe"
+	"github.com/Knatte18/loomyard/internal/state"
+)
+
+// newFakeReceiver builds a *lifecycleCLI whose Env is filled entirely with fakes that perform no
+// real I/O, over a fresh t.TempDir(). shutdown, when non-nil, replaces the default no-op
+// Teardown.Shutdown closure.
+func newFakeReceiver(t *testing.T, shutdown func(ctx context.Context) (string, error)) *lifecycleCLI {
+	t.Helper()
+	dir := t.TempDir()
+
+	c := &lifecycleCLI{
+		location: &lyxcwd.Location{RepoName: "example", HubPath: dir, WorktreeName: "hub-repo", AnchorRel: "."},
+		slug:     "some-slug",
+	}
+	c.shedPaths = lifecyclerecipe.ShedPaths{
+		StatusPath:     filepath.Join(dir, "status.json"),
+		LockPath:       filepath.Join(dir, "run.lock"),
+		StatusLockPath: filepath.Join(dir, "status.json.lock"),
+	}
+
+	if shutdown == nil {
+		shutdown = func(ctx context.Context) (string, error) { return "", nil }
+	}
+
+	c.env = shedrecipe.Env{
+		Slug:       c.slug,
+		ScratchDir: dir,
+		PrimeLock: lifecycleshed.PrimeLock{
+			Path: filepath.Join(dir, "prime.lock"),
+			Acquire: func() (func() error, bool, error) {
+				return func() error { return nil }, true, nil
+			},
+		},
+		CreateWorktree: func(ctx context.Context) error { return nil },
+		Teardown: lifecycleshed.TeardownDeps{
+			Shutdown: shutdown,
+			Remove:   func(ctx context.Context) error { return nil },
+		},
+		LoomRun: lifecycleshed.LoomRunDeps{
+			Spawn: func(ctx context.Context) error { return nil },
+			ResolveStatus: func() (string, string, error) {
+				return filepath.Join(dir, "loom-status.json"), filepath.Join(dir, "loom-status.json.lock"), nil
+			},
+			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+				return shedengine.Status{State: shedengine.StateDone}, true, nil
+			},
+		},
+	}
+	return c
+}
+
+// writeStatus writes st as c's status file, unlocked -- the test owns the file outright before the
+// verb ever runs.
+func writeStatus(t *testing.T, c *lifecycleCLI, st shedengine.Status) {
+	t.Helper()
+	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, st); err != nil {
+		t.Fatalf("writeStatus: %v", err)
+	}
+}
+
+// TestRunCmd_ResumeDispositions covers StateRunning, StateBlocked, StateFailed, and StatePaused:
+// each resumes silently from the persisted current producer, with every fake succeeding trivially,
+// so the whole run completes and the verb reports success.
+func TestRunCmd_ResumeDispositions(t *testing.T) {
+	for _, state := range []shedengine.State{shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StatePaused} {
+		t.Run(string(state), func(t *testing.T) {
+			c := newFakeReceiver(t, nil)
+			writeStatus(t, c, shedengine.Status{
+				CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
+				State:           state,
+			})
+
+			var out bytes.Buffer
+			exitCode := clihelp.Execute(c.runCmd(), &out, []string{c.slug})
+
+			if exitCode != 0 {
+				t.Fatalf("run(%s) exit code = %d; want 0; output: %s", state, exitCode, out.String())
+			}
+			if !strings.Contains(out.String(), `"ok":true`) {
+				t.Errorf("run(%s) output missing ok:true envelope; got: %q", state, out.String())
+			}
+		})
+	}
+}
+
+// TestRunCmd_StateDoneRefusesNamingTheLifecycleDir asserts a StateDone slug refuses on the envelope
+// rather than silently re-running, naming the per-slug directory to delete.
+func TestRunCmd_StateDoneRefusesNamingTheLifecycleDir(t *testing.T) {
+	c := newFakeReceiver(t, nil)
+	writeStatus(t, c, shedengine.Status{
+		CurrentProducer: lifecyclerecipe.NameWorktreeTeardown,
+		State:           shedengine.StateDone,
+	})
+
+	var out bytes.Buffer
+	exitCode := clihelp.Execute(c.runCmd(), &out, []string{c.slug})
+
+	if exitCode != 1 {
+		t.Fatalf("run() exit code = %d; want 1; output: %s", exitCode, out.String())
+	}
+	wantDir := LifecycleDir(c.location, c.slug)
+	if !strings.Contains(out.String(), wantDir) {
+		t.Errorf("run() output = %q; want it to name the per-slug directory %q", out.String(), wantDir)
+	}
+}
+
+// TestRunCmd_AbsentStatusFileStartsFresh asserts that no persisted status file at all is a fresh
+// start, not a refusal.
+func TestRunCmd_AbsentStatusFileStartsFresh(t *testing.T) {
+	c := newFakeReceiver(t, nil)
+
+	var out bytes.Buffer
+	exitCode := clihelp.Execute(c.runCmd(), &out, []string{c.slug})
+
+	if exitCode != 0 {
+		t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
+	}
+	if !strings.Contains(out.String(), `"ok":true`) {
+		t.Errorf("run() output missing ok:true envelope; got: %q", out.String())
+	}
+}
+
+// TestRunCmd_HeldRunLockRefusesNamingTheLockPath takes the per-slug run lock in the test itself,
+// before invoking the verb, and asserts the verb refuses on the envelope naming the lock path
+// without waiting.
+func TestRunCmd_HeldRunLockRefusesNamingTheLockPath(t *testing.T) {
+	c := newFakeReceiver(t, nil)
+	writeStatus(t, c, shedengine.Status{
+		CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
+		State:           shedengine.StateBlocked,
+	})
+
+	held, err := lock.AcquireWriteLock(c.shedPaths.LockPath)
+	if err != nil {
+		t.Fatalf("acquire run lock in test: %v", err)
+	}
+	t.Cleanup(func() { _ = held.Release() })
+
+	var out bytes.Buffer
+	exitCode := clihelp.Execute(c.runCmd(), &out, []string{c.slug})
+
+	if exitCode != 1 {
+		t.Fatalf("run() exit code = %d; want 1; output: %s", exitCode, out.String())
+	}
+	if !strings.Contains(out.String(), c.shedPaths.LockPath) {
+		t.Errorf("run() output = %q; want it to name the lock path %q", out.String(), c.shedPaths.LockPath)
+	}
+}
+
+// TestRunCmd_AbandonedSessionKey asserts the abandonedSession key reaches the envelope on a Done
+// teardown when the recorded value is non-empty, and is absent when it is empty.
+func TestRunCmd_AbandonedSessionKey(t *testing.T) {
+	tests := []struct {
+		name             string
+		abandonedSession string
+		wantKey          bool
+	}{
+		{"NonEmptyRecordsKey", "some-abandoned-session", true},
+		{"EmptyOmitsKey", "", false},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			var c *lifecycleCLI
+			shutdown := func(ctx context.Context) (string, error) {
+				c.abandonedSession = tt.abandonedSession
+				return tt.abandonedSession, nil
+			}
+			c = newFakeReceiver(t, shutdown)
+			writeStatus(t, c, shedengine.Status{
+				CurrentProducer: lifecyclerecipe.NameWorktreeTeardown,
+				State:           shedengine.StateBlocked,
+			})
+
+			var out bytes.Buffer
+			exitCode := clihelp.Execute(c.runCmd(), &out, []string{c.slug})
+			if exitCode != 0 {
+				t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
+			}
+
+			var envelope map[string]any
+			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
+				t.Fatalf("decode envelope: %v; output: %s", err, out.String())
+			}
+			_, hasKey := envelope["abandonedSession"]
+			if hasKey != tt.wantKey {
+				t.Errorf("envelope has abandonedSession key = %v; want %v; envelope: %v", hasKey, tt.wantKey, envelope)
+			}
+		})
+	}
+}
diff --git a/internal/lifecyclecli/status.go b/internal/lifecyclecli/status.go
new file mode 100644
index 000000000..d4d8137ff
--- /dev/null
+++ b/internal/lifecyclecli/status.go
@@ -0,0 +1,68 @@
+// status.go implements the `status` lifecycle verb: a one-shot JSON envelope of a slug's persisted
+// lifecycle status.
+
+package lifecyclecli
+
+import (
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/output"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+	"github.com/Knatte18/loomyard/internal/state"
+	"github.com/spf13/cobra"
+)
+
+// statusCmd builds the `status <slug>` subcommand.
+//
+// It runs under the same prime-worktree refusal the parent's pre-run applies (cli.go), so it is
+// refused from a task worktree exactly as run is -- even though it is read-only: the path it would
+// read is derived from whichever worktree it is invoked in, so answering from a task worktree would
+// report on a different file and quietly mislead, not merely fail to write.
+func (c *lifecycleCLI) statusCmd() *cobra.Command {
+	cmd := &cobra.Command{
+		Use:   "status <slug>",
+		Short: "report a task worktree's persisted lifecycle status",
+		Long: `status reports a slug's persisted lifecycle status: the current producer, the
+state, the error field, the activity, and the history, plus the resolved
+status path so an operator can find the file. A slug that has never been
+run on this machine is reported as a determined answer on the success
+envelope, not as an error.
+
+Example:
+  lyx lifecycle status some-slug`,
+		Args: cobra.ExactArgs(1),
+		RunE: func(cmd *cobra.Command, args []string) error {
+			if clihelp.ShouldAbort(cmd.Context()) {
+				return nil
+			}
+			ctx := cmd.Context()
+			out := cmd.OutOrStdout()
+
+			st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
+			if err != nil {
+				clihelp.SetExit(ctx, output.Err(out, "lifecyclecli: decode status file "+c.shedPaths.StatusPath+": "+err.Error()))
+				return nil
+			}
+			if !found {
+				// An absent status file is a determined answer -- the slug has not been run on this
+				// machine -- not an error, since nothing failed.
+				clihelp.SetExit(ctx, output.Ok(out, map[string]any{
+					"found":       false,
+					"status_path": c.shedPaths.StatusPath,
+				}))
+				return nil
+			}
+
+			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
+				"found":            true,
+				"status_path":      c.shedPaths.StatusPath,
+				"current_producer": st.CurrentProducer,
+				"state":            string(st.State),
+				"error":            st.Error,
+				"activity":         st.Activity,
+				"history":          st.History,
+			}))
+			return nil
+		},
+	}
+	return cmd
+}
diff --git a/internal/lifecyclecli/testmain_integration_test.go b/internal/lifecyclecli/testmain_integration_test.go
new file mode 100644
index 000000000..ba1fc6e9c
--- /dev/null
+++ b/internal/lifecyclecli/testmain_integration_test.go
@@ -0,0 +1,22 @@
+//go:build integration
+
+// testmain_integration_test.go wires this package's integration test binary into the hermetic git
+// test environment: gitkit.HermeticGitEnv() runs once before any test, since
+// lifecycle_integration_test.go spawns git via hubforge fixtures (Test Tier Purity Invariant /
+// Hermetic Git Test Environment Invariant), in the shape internal/landingshed's own equivalent
+// already uses.
+
+package lifecyclecli
+
+import (
+	"os"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/gitkit"
+)
+
+// TestMain runs HermeticGitEnv before any test spawns git.
+func TestMain(m *testing.M) {
+	gitkit.HermeticGitEnv()
+	os.Exit(m.Run())
+}
diff --git a/internal/lifecyclecli/testmain_test.go b/internal/lifecyclecli/testmain_test.go
new file mode 100644
index 000000000..e4c791e85
--- /dev/null
+++ b/internal/lifecyclecli/testmain_test.go
@@ -0,0 +1,28 @@
+//go:build !integration
+
+// testmain_test.go wires the package's test binary into the hermetic git test environment:
+// gitkit.HermeticGitEnv() runs once before any test, so this package's tests never inherit the
+// operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant).
+//
+// The `!integration` constraint is load-bearing, not decorative: testmain_integration_test.go
+// declares its own TestMain for the same package, and without this negation both files would
+// compile together under `go test -tags integration` (an untagged file always compiles regardless
+// of which tags are set) and collide as two definitions of TestMain in one package. This file's
+// own suite stays untagged and Tier 1 in every other respect -- none of its sibling test files call
+// the resolver, run git, or spawn a process -- only the TestMain wiring itself needs this one file
+// excluded once the integration-tagged sibling supplies its own.
+
+package lifecyclecli
+
+import (
+	"os"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/gitkit"
+)
+
+// TestMain runs the hermetic git setup once before the whole suite.
+func TestMain(m *testing.M) {
+	gitkit.HermeticGitEnv()
+	os.Exit(m.Run())
+}
diff --git a/internal/lifecyclecli/wire.go b/internal/lifecyclecli/wire.go
new file mode 100644
index 000000000..7a362c122
--- /dev/null
+++ b/internal/lifecyclecli/wire.go
@@ -0,0 +1,154 @@
+// wire.go implements wire, the assembly seam that builds the shedrecipe.Env and
+// lifecyclerecipe.ShedPaths the run and status verbs need, plus the receiver field the abandoned
+// session value is recorded into.
+//
+// Every seam that touches the managed task worktree resolves inside its own closure body on Call,
+// never here at wiring time: the task worktree does not exist until WorktreeCreate has already run,
+// so resolving any of its paths any earlier would resolve a path that is not there yet. That applies
+// to the status-path pair, the spawn directory, and both teardown halves alike -- not only to the one
+// whose laziness (ResolveStatus's own signature) makes it visible.
+
+package lifecyclecli
+
+import (
+	"context"
+	"os"
+	"os/exec"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/hubgeom"
+	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
+	"github.com/Knatte18/loomyard/internal/lifecycleshed"
+	"github.com/Knatte18/loomyard/internal/lock"
+	"github.com/Knatte18/loomyard/internal/logger"
+	"github.com/Knatte18/loomyard/internal/loomengine"
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+	"github.com/Knatte18/loomyard/internal/reedengine"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+	"github.com/Knatte18/loomyard/internal/shedrecipe"
+	"github.com/Knatte18/loomyard/internal/state"
+)
+
+// taskWorktreeLocation resolves the managed task worktree's own *lyxcwd.Location, for slug, from
+// the prime *lyxcwd.Location. It is the shared body every lazily-resolved seam below calls, so a
+// caller reading this file only once still sees every "resolved lazily" claim in one place.
+//
+// It joins the task worktree's root via fabricengine.WorktreePath -- the topology package's own
+// sibling-path helper -- and resolves that root through lyxcwd.ResolveWorktree, which applies no
+// cwd gate: the caller here holds a worktree root, not an acting cwd, so the gate would spuriously
+// fire.
+func taskWorktreeLocation(prime *lyxcwd.Location, slug string) (*lyxcwd.Location, error) {
+	return lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
+}
+
+// wire builds and stores the shedrecipe.Env and lifecyclerecipe.ShedPaths the run and status verbs
+// need, over the resolved prime location and slug.
+func (c *lifecycleCLI) wire(location *lyxcwd.Location, slug string) error {
+	primeRunLockPath := PrimeRunLock(location)
+	primeLock := lifecycleshed.PrimeLock{
+		Path: primeRunLockPath,
+		Acquire: func() (release func() error, ok bool, err error) {
+			if err := os.MkdirAll(LifecycleDir(location, slug), 0o755); err != nil {
+				return nil, false, err
+			}
+			fl, acquired, err := lock.TryAcquireWriteLock(primeRunLockPath)
+			if err != nil {
+				return nil, false, err
+			}
+			if !acquired {
+				return nil, false, nil
+			}
+			return fl.Release, true, nil
+		},
+	}
+
+	env := shedrecipe.Env{
+		Slug:       slug,
+		ScratchDir: LifecycleDir(location, slug),
+		PrimeLock:  primeLock,
+		CreateWorktree: func(ctx context.Context) error {
+			cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
+			if err != nil {
+				return err
+			}
+			top := fabricengine.NewTopology(cfg)
+			res, err := top.Add(location, slug, fabricengine.AddOptions{})
+			logger.Info("lifecyclecli: create worktree", "slug", slug, "mutations", res.Mutated())
+			return err
+		},
+		Teardown: lifecycleshed.TeardownDeps{
+			Shutdown: func(ctx context.Context) (abandonedSession string, err error) {
+				taskLocation, err := taskWorktreeLocation(location, slug)
+				if err != nil {
+					return "", err
+				}
+				reedCfg, err := reedengine.LoadConfig(taskLocation.AnchorPath(), "reed")
+				if err != nil {
+					return "", err
+				}
+				reedGeom := hubgeom.ReedGeometry(taskLocation)
+				reedEngine := reedengine.New(reedCfg, reedGeom)
+				res, err := reedEngine.Down()
+				if err != nil {
+					return "", err
+				}
+				// Recorded onto the receiver, not only returned: the run verb reads it back after
+				// the Shed's own Run has returned, to surface it on the success envelope.
+				c.abandonedSession = res.AbandonedSession
+				return res.AbandonedSession, nil
+			},
+			Remove: func(ctx context.Context) error {
+				cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
+				if err != nil {
+					return err
+				}
+				top := fabricengine.NewTopology(cfg)
+				res, err := top.Remove(location, slug, false, false)
+				logger.Info("lifecyclecli: teardown worktree", "slug", slug, "mutations", res.Mutated())
+				return err
+			},
+		},
+		LoomRun: lifecycleshed.LoomRunDeps{
+			ResolveStatus: func() (statusPath, statusLockPath string, err error) {
+				taskLocation, err := taskWorktreeLocation(location, slug)
+				if err != nil {
+					return "", "", err
+				}
+				return loomengine.LoomStatusFile(taskLocation), loomengine.LoomStatusLock(taskLocation), nil
+			},
+			Spawn: func(ctx context.Context) error {
+				taskLocation, err := taskWorktreeLocation(location, slug)
+				if err != nil {
+					return err
+				}
+				exe, err := os.Executable()
+				if err != nil {
+					return err
+				}
+				// Dir is the task worktree's AnchorPath(), never its bare worktree root: the resolver
+				// gates a child's working directory to the anchor, and a bare root fails on any
+				// subpath-anchored hub.
+				cmd := exec.Command(exe, "loom", "run", "--no-attach")
+				cmd.Dir = taskLocation.AnchorPath()
+				logger.Info("lifecyclecli: spawning loom session", "slug", slug, "dir", cmd.Dir)
+				err = cmd.Run()
+				logger.Info("lifecyclecli: loom session wait complete", "slug", slug, "dir", cmd.Dir)
+				return err
+			},
+			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+				return state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
+			},
+		},
+	}
+
+	c.env = env
+	c.shedPaths = lifecyclerecipe.ShedPaths{
+		StatusPath:     StatusFile(location, slug),
+		LockPath:       RunLock(location, slug),
+		StatusLockPath: StatusLock(location, slug),
+		// CommitStatus is left nil: nil is the documented absent value meaning "commit nothing",
+		// which is exactly right for this package's per-machine, never-committed state.
+		CommitStatus: nil,
+	}
+	return nil
+}
diff --git a/internal/lifecyclecli/wire_test.go b/internal/lifecyclecli/wire_test.go
new file mode 100644
index 000000000..74dc5fcc0
--- /dev/null
+++ b/internal/lifecyclecli/wire_test.go
@@ -0,0 +1,70 @@
+// wire_test.go proves no seam wire (wire.go) builds is evaluated at wiring time: building the
+// wiring for a slug whose worktree does not exist must succeed, and none of the four seams that
+// resolve the managed task worktree's own Location -- the status-path resolver, the spawn
+// directory, and both teardown halves -- may be invoked here, since each of the three that actually
+// resolves that Location reaches the resolver (lyxcwd.ResolveWorktree), which spawns git; this
+// suite stays untagged and Tier 1, so it proves laziness structurally rather than by invoking a
+// seam and observing it run.
+
+package lifecyclecli
+
+import (
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+)
+
+// TestWire_SucceedsForNonexistentTaskWorktree asserts that wire returns no error even though the
+// managed task worktree named by slug does not exist anywhere on disk -- the mechanical proof that
+// wire itself resolves nothing about that worktree.
+func TestWire_SucceedsForNonexistentTaskWorktree(t *testing.T) {
+	c := &lifecycleCLI{}
+	location := &lyxcwd.Location{
+		RepoName:     "example",
+		HubPath:      t.TempDir(),
+		WorktreeName: "hub-repo",
+		AnchorRel:    ".",
+	}
+
+	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
+		t.Fatalf("wire() error = %v; want nil", err)
+	}
+}
+
+// TestWire_LazySeams covers all four lazy seams individually: the status-path resolver
+// (Env.LoomRun.ResolveStatus), the spawn directory (Env.LoomRun.Spawn), and both teardown halves
+// (Env.Teardown.Shutdown, Env.Teardown.Remove) are each present as an injected closure after wire
+// returns -- not already-evaluated values -- for a slug whose worktree does not exist. Covering all
+// four separately, rather than just the first, is deliberate: eager evaluation is exactly the
+// failure laziness exists to avoid, and a test covering only one seam would let the other three
+// regress silently.
+func TestWire_LazySeams(t *testing.T) {
+	c := &lifecycleCLI{}
+	location := &lyxcwd.Location{
+		RepoName:     "example",
+		HubPath:      t.TempDir(),
+		WorktreeName: "hub-repo",
+		AnchorRel:    ".",
+	}
+
+	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
+		t.Fatalf("wire() error = %v; want nil", err)
+	}
+
+	tests := []struct {
+		name    string
+		present bool
+	}{
+		{"StatusPathResolver", c.env.LoomRun.ResolveStatus != nil},
+		{"SpawnDirectory", c.env.LoomRun.Spawn != nil},
+		{"TeardownShutdown", c.env.Teardown.Shutdown != nil},
+		{"TeardownRemove", c.env.Teardown.Remove != nil},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			if !tt.present {
+				t.Errorf("wire() left this seam nil; want an injected closure present but uncalled")
+			}
+		})
+	}
+}
diff --git a/internal/lifecyclerecipe/coverage_guard_test.go b/internal/lifecyclerecipe/coverage_guard_test.go
new file mode 100644
index 000000000..2516579b2
--- /dev/null
+++ b/internal/lifecyclerecipe/coverage_guard_test.go
@@ -0,0 +1,56 @@
+// coverage_guard_test.go pins the registry's reason to exist for this package's own three rows: it
+// builds a real shedrecipe.Env/ShedPaths pair, calls this package's own New, and checks the
+// row-to-engine table below in both directions against New's real, current output rather than
+// against a standalone literal that could drift silently.
+//
+// It carries no closed-coverage assertion over shedrecipe.Names() -- that claim now lives in one
+// cross-consumer place, internal/shedrecipe's own external test package, and a second copy here
+// would fail on the other consumer's engines.
+
+package lifecyclerecipe
+
+import (
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/shedrecipe"
+)
+
+// lifecycleRowEngines maps each of New's three row names to the engine name backing it. The
+// row-name side is keyed off this package's own Name* constants, per the
+// row-name-authority-stays-with-the-go-constants Shared Decision.
+var lifecycleRowEngines = map[string]string{
+	NameWorktreeCreate:   "WorktreeCreate",
+	NameLoomRun:          "LoomRun",
+	NameWorktreeTeardown: "WorktreeTeardown",
+}
+
+// TestCoverageGuard_EveryLifecycleRowHasAnEngine asserts three things about lifecycleRowEngines
+// against New's real, current row list: every row New assembles has an entry in the table (the
+// direction that catches a row added to the recipe before its consuming code lands); every key in
+// the table names a row New actually has (the direction that keeps the table from accumulating
+// dead entries); and every engine name the table maps to resolves through shedrecipe.Lookup
+// without error.
+func TestCoverageGuard_EveryLifecycleRowHasAnEngine(t *testing.T) {
+	env, paths := testEnv(t)
+	shed, err := New(env, paths)
+	if err != nil {
+		t.Fatalf("New() error = %v, want nil", err)
+	}
+
+	rowNames := make(map[string]bool, len(shed.Producers))
+	for _, p := range shed.Producers {
+		rowNames[p.Name] = true
+		if _, ok := lifecycleRowEngines[p.Name]; !ok {
+			t.Errorf("New() row %q has no entry in lifecycleRowEngines", p.Name)
+		}
+	}
+
+	for rowName, engineName := range lifecycleRowEngines {
+		if !rowNames[rowName] {
+			t.Errorf("lifecycleRowEngines names row %q, which New() does not have", rowName)
+		}
+		if _, err := shedrecipe.Lookup(engineName); err != nil {
+			t.Errorf("Lookup(%q) (engine for row %q) error = %v, want nil", engineName, rowName, err)
+		}
+	}
+}
diff --git a/internal/lifecyclerecipe/doc.go b/internal/lifecyclerecipe/doc.go
new file mode 100644
index 000000000..252957561
--- /dev/null
+++ b/internal/lifecyclerecipe/doc.go
@@ -0,0 +1,12 @@
+// Package lifecyclerecipe owns the lifecycle recipe's construction: parsing
+// contracts/recipes.LifecycleRecipe and assembling it into a *shedengine.Shed against a
+// caller-supplied shedrecipe.Env. internal/lifecyclecli is its only production caller.
+//
+// It takes every absolute path from its caller and has no direct production import of
+// internal/lyxcwd, per the Told-Geometry Invariant (CONSTRAINTS.md).
+//
+// This package describes one repository throughout. It is not in the Fabric Vocabulary
+// Invariant's owner set, so none of its identifiers, string literals, or comments may name either
+// fabric-internal side -- write "the task worktree" and "the pair" instead of naming either side
+// by name.
+package lifecyclerecipe
diff --git a/internal/lifecyclerecipe/fixture_test.go b/internal/lifecyclerecipe/fixture_test.go
new file mode 100644
index 000000000..c68eb7355
--- /dev/null
+++ b/internal/lifecyclerecipe/fixture_test.go
@@ -0,0 +1,61 @@
+// fixture_test.go implements testEnv, the package-internal test scaffolding every later test file
+// in this package reuses: a minimal shedrecipe.Env and a ShedPaths, every path derived from one
+// t.TempDir() root, filling only the six fields the three lifecycle entries read and leaving the
+// rest of Env zero -- which is legal, since each entry validates exactly the fields it reads.
+
+package lifecyclerecipe
+
+import (
+	"context"
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/lifecycleshed"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+	"github.com/Knatte18/loomyard/internal/shedrecipe"
+)
+
+// testEnv returns a minimal filled shedrecipe.Env and a ShedPaths, every path derived from one
+// t.TempDir() root.
+func testEnv(t *testing.T) (shedrecipe.Env, ShedPaths) {
+	t.Helper()
+
+	dir := t.TempDir()
+
+	env := shedrecipe.Env{
+		Slug:       "test-slug",
+		ScratchDir: dir,
+		CreateWorktree: func(context.Context) error {
+			return nil
+		},
+		LoomRun: lifecycleshed.LoomRunDeps{
+			Spawn: func(context.Context) error { return nil },
+			ResolveStatus: func() (string, string, error) {
+				return filepath.Join(dir, "loomrun-status.json"), filepath.Join(dir, "loomrun-status.json.lock"), nil
+			},
+			ReadStatus: func(string, string) (shedengine.Status, bool, error) {
+				return shedengine.Status{}, false, nil
+			},
+		},
+		Teardown: lifecycleshed.TeardownDeps{
+			Shutdown: func(context.Context) (string, error) { return "", nil },
+			Remove:   func(context.Context) error { return nil },
+		},
+		PrimeLock: lifecycleshed.PrimeLock{
+			Path: filepath.Join(dir, "prime.lock"),
+			Acquire: func() (func() error, bool, error) {
+				return func() error { return nil }, true, nil
+			},
+		},
+	}
+
+	paths := ShedPaths{
+		StatusPath:     filepath.Join(dir, "status.json"),
+		LockPath:       filepath.Join(dir, "run.lock"),
+		StatusLockPath: filepath.Join(dir, "status.json.lock"),
+		MaxBounces:     0,
+		CommitStatus:   nil,
+	}
+
+	return env, paths
+}
diff --git a/internal/lifecyclerecipe/lifecyclerecipe.go b/internal/lifecyclerecipe/lifecyclerecipe.go
new file mode 100644
index 000000000..e5aa74413
--- /dev/null
+++ b/internal/lifecyclerecipe/lifecyclerecipe.go
@@ -0,0 +1,73 @@
+// lifecyclerecipe.go implements ShedPaths and New: the five told Shed-only values and the entry
+// point that parses contracts/recipes.LifecycleRecipe, builds it against a caller-supplied
+// shedrecipe.Env, and returns the assembled *shedengine.Shed.
+
+package lifecyclerecipe
+
+import (
+	"fmt"
+
+	"github.com/Knatte18/loomyard/contracts/recipes"
+	"github.com/Knatte18/loomyard/internal/shedbuild"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+	"github.com/Knatte18/loomyard/internal/shedrecipe"
+)
+
+// ShedPaths carries the five told values shedengine.Shed itself reads and no shedrecipe.Env
+// registry entry reads: StatusPath, LockPath, StatusLockPath, MaxBounces, and CommitStatus.
+type ShedPaths struct {
+	// StatusPath is the durable status file; it is told and never derived. See
+	// shedengine.Shed.StatusPath's own field doc.
+	StatusPath string
+	// LockPath is the run lock, held non-blocking for the whole of one Run. See
+	// shedengine.Shed.LockPath's own field doc.
+	LockPath string
+	// StatusLockPath is the lock internal/state itself takes; it must name a different file from
+	// LockPath. See shedengine.Shed.StatusLockPath's own field doc.
+	StatusLockPath string
+	// MaxBounces is the default a ProducerDef.MaxBounces of 0 inherits. 0 means "use the internal
+	// default", never "no bounces allowed" -- the budget it seeds is per-producer and
+	// episode-scoped, not run-wide. See shedengine.Shed.MaxBounces's own field doc.
+	MaxBounces int
+	// CommitStatus is copied verbatim onto the constructed shedengine.Shed. See
+	// shedengine.Shed.CommitStatus's own field doc.
+	CommitStatus func(producer, state string) error
+}
+
+// New parses recipes.LifecycleRecipe, builds it against env, and returns a *shedengine.Shed
+// carrying the built []shedengine.ProducerDef plus paths' five fields.
+//
+// New uses shedbuild.Parse on the embedded bytes, never shedbuild.Load -- there is no on-disk
+// runtime location for this recipe. It surfaces both the parse error and the build error rather
+// than swallowing either, wrapping each with a "lifecyclerecipe: " prefix and nothing more.
+//
+// New never calls shedbuild.Check -- internal/shedcheck/doc.go and internal/shedbuild/check.go
+// both state that Check is authoring-time only, because a resumed run legitimately starts
+// mid-graph and reachability-from-entry is the wrong production question.
+//
+// New performs no nil-guard or absolute-path check of its own on any Env field: each registry
+// entry validates exactly the fields it reads.
+//
+// Unlike loomrecipe.New's two, New performs no coherence check across its two arguments: no
+// lifecycle registry entry reads Env.StatusPath or Env.StatusLockPath, so there is no duplicated
+// copy for a check to guard, and a check added for symmetry would guard nothing.
+func New(env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error) {
+	recipe, err := shedbuild.Parse(recipes.LifecycleRecipe)
+	if err != nil {
+		return nil, fmt.Errorf("lifecyclerecipe: %w", err)
+	}
+
+	producers, err := shedbuild.Build(recipe, env)
+	if err != nil {
+		return nil, fmt.Errorf("lifecyclerecipe: %w", err)
+	}
+
+	return &shedengine.Shed{
+		Producers:      producers,
+		StatusPath:     paths.StatusPath,
+		LockPath:       paths.LockPath,
+		StatusLockPath: paths.StatusLockPath,
+		MaxBounces:     paths.MaxBounces,
+		CommitStatus:   paths.CommitStatus,
+	}, nil
+}
diff --git a/internal/lifecyclerecipe/names.go b/internal/lifecyclerecipe/names.go
new file mode 100644
index 000000000..b1ebdbaa2
--- /dev/null
+++ b/internal/lifecyclerecipe/names.go
@@ -0,0 +1,55 @@
+// names.go declares the lifecycle recipe's row-name constants, the authority the recipe file's
+// row names are pinned against, and RecipeEngines, the derived engine set the cross-consumer
+// coverage guard in internal/shedrecipe trusts.
+
+package lifecyclerecipe
+
+import (
+	"fmt"
+	"sort"
+
+	"github.com/Knatte18/loomyard/contracts/recipes"
+	"github.com/Knatte18/loomyard/internal/shedbuild"
+)
+
+// The three lifecycle recipe row names. These are durable on-disk identities that resume depends
+// on: a rename here without a matching rename in contracts/recipes/lifecycle-recipe.yaml breaks
+// resume for any in-flight run, and this package's own coverage guard pins the two against each
+// other.
+const (
+	// NameWorktreeCreate is the row that creates the task worktree.
+	NameWorktreeCreate = "Worktree-Create"
+	// NameLoomRun is the row that runs the loom session inside the task worktree.
+	NameLoomRun = "Loom-Run"
+	// NameWorktreeTeardown is the row that tears the task worktree down.
+	NameWorktreeTeardown = "Worktree-Teardown"
+)
+
+// RecipeEngines parses recipes.LifecycleRecipe, collects each row's engine name, de-duplicates,
+// and returns the result sorted. It is the input the cross-consumer coverage guard in
+// internal/shedrecipe trusts, unioned with loomrecipe.RecipeEngines to check
+// shedrecipe.Names() for closed coverage.
+//
+// RecipeEngines derives its result from the recipe rather than a hand-maintained literal, and
+// never returns a nil slice alongside an error: a parse failure panics, naming this package, since
+// a recipe that fails to parse is a build-time defect in an embedded file rather than a runtime
+// condition.
+func RecipeEngines() []string {
+	recipe, err := shedbuild.Parse(recipes.LifecycleRecipe)
+	if err != nil {
+		panic(fmt.Sprintf("lifecyclerecipe: RecipeEngines: %v", err))
+	}
+
+	seen := make(map[string]bool, len(recipe.Producers))
+	engines := make([]string, 0, len(recipe.Producers))
+	for _, row := range recipe.Producers {
+		if seen[row.Engine] {
+			continue
+		}
+		seen[row.Engine] = true
+		engines = append(engines, row.Engine)
+	}
+
+	sort.Strings(engines)
+	return engines
+}
diff --git a/internal/lifecyclerecipe/recipe_test.go b/internal/lifecyclerecipe/recipe_test.go
new file mode 100644
index 000000000..8e1ee6c87
--- /dev/null
+++ b/internal/lifecyclerecipe/recipe_test.go
@@ -0,0 +1,113 @@
+// recipe_test.go builds through New and asserts the shape the lifecycle recipe's three rows must
+// carry: their names, the OnDone chain, the load-bearing empty OnStuck/OnDone edges, the absence
+// of any Segment, and that the returned *shedengine.Shed carries ShedPaths' five values verbatim.
+// It also asserts RecipeEngines()'s own shape, since a silently empty return would disable the
+// cross-consumer coverage guard rather than fail it.
+
+package lifecyclerecipe
+
+import (
+	"testing"
+)
+
+// TestNew_RowNamesMatchTheDurableIdentityConstants asserts New assembles exactly three rows, named
+// exactly the three Name* constants -- the durable-identity guard internal/loomrecipe's own
+// row-name test mirrors.
+func TestNew_RowNamesMatchTheDurableIdentityConstants(t *testing.T) {
+	env, paths := testEnv(t)
+	shed, err := New(env, paths)
+	if err != nil {
+		t.Fatalf("New() error = %v, want nil", err)
+	}
+
+	if len(shed.Producers) != 3 {
+		t.Fatalf("New() row count = %d, want 3", len(shed.Producers))
+	}
+
+	wantNames := []string{NameWorktreeCreate, NameLoomRun, NameWorktreeTeardown}
+	for i, want := range wantNames {
+		if got := shed.Producers[i].Name; got != want {
+			t.Errorf("row %d name = %q; want %q", i, got, want)
+		}
+	}
+}
+
+// TestNew_OnDoneChainAndLoadBearingEmptyEdges asserts the Worktree-Create -> Loom-Run ->
+// Worktree-Teardown OnDone chain, Loom-Run's OnStuck is empty, Worktree-Teardown's OnDone is
+// empty, and no row declares a Segment.
+func TestNew_OnDoneChainAndLoadBearingEmptyEdges(t *testing.T) {
+	env, paths := testEnv(t)
+	shed, err := New(env, paths)
+	if err != nil {
+		t.Fatalf("New() error = %v, want nil", err)
+	}
+
+	byName := make(map[string]int, len(shed.Producers))
+	for i, p := range shed.Producers {
+		byName[p.Name] = i
+	}
+
+	create := shed.Producers[byName[NameWorktreeCreate]]
+	if create.OnDone != NameLoomRun {
+		t.Errorf("Worktree-Create OnDone = %q; want %q", create.OnDone, NameLoomRun)
+	}
+
+	loomRun := shed.Producers[byName[NameLoomRun]]
+	if loomRun.OnDone != NameWorktreeTeardown {
+		t.Errorf("Loom-Run OnDone = %q; want %q", loomRun.OnDone, NameWorktreeTeardown)
+	}
+	if loomRun.OnStuck != "" {
+		t.Errorf("Loom-Run OnStuck = %q; want empty -- this edge escalates to a human with the task worktree intact", loomRun.OnStuck)
+	}
+
+	teardown := shed.Producers[byName[NameWorktreeTeardown]]
+	if teardown.OnDone != "" {
+		t.Errorf("Worktree-Teardown OnDone = %q; want empty -- this is what ends the run quietly", teardown.OnDone)
+	}
+
+	for _, p := range shed.Producers {
+		if p.Segment != "" {
+			t.Errorf("row %q Segment = %q; want empty", p.Name, p.Segment)
+		}
+	}
+}
+
+// TestNew_CarriesShedPathsVerbatim asserts the returned *shedengine.Shed carries ShedPaths' five
+// values verbatim.
+func TestNew_CarriesShedPathsVerbatim(t *testing.T) {
+	env, paths := testEnv(t)
+	shed, err := New(env, paths)
+	if err != nil {
+		t.Fatalf("New() error = %v, want nil", err)
+	}
+
+	if shed.StatusPath != paths.StatusPath {
+		t.Errorf("StatusPath = %q; want %q", shed.StatusPath, paths.StatusPath)
+	}
+	if shed.LockPath != paths.LockPath {
+		t.Errorf("LockPath = %q; want %q", shed.LockPath, paths.LockPath)
+	}
+	if shed.StatusLockPath != paths.StatusLockPath {
+		t.Errorf("StatusLockPath = %q; want %q", shed.StatusLockPath, paths.StatusLockPath)
+	}
+	if shed.MaxBounces != paths.MaxBounces {
+		t.Errorf("MaxBounces = %d; want %d", shed.MaxBounces, paths.MaxBounces)
+	}
+}
+
+// TestRecipeEngines_ReportsExactlyTheThreeEngineNamesSorted asserts RecipeEngines() returns
+// exactly the three engine names, sorted -- this is the input the cross-consumer guard trusts, so
+// a silently empty return would disable that guard rather than fail it.
+func TestRecipeEngines_ReportsExactlyTheThreeEngineNamesSorted(t *testing.T) {
+	got := RecipeEngines()
+	want := []string{"LoomRun", "WorktreeCreate", "WorktreeTeardown"}
+
+	if len(got) != len(want) {
+		t.Fatalf("RecipeEngines() = %v, want %v", got, want)
+	}
+	for i, w := range want {
+		if got[i] != w {
+			t.Errorf("RecipeEngines()[%d] = %q; want %q", i, got[i], w)
+		}
+	}
+}
diff --git a/internal/lifecyclerecipe/seam_enforcement_test.go b/internal/lifecyclerecipe/seam_enforcement_test.go
new file mode 100644
index 000000000..2a5f4905e
--- /dev/null
+++ b/internal/lifecyclerecipe/seam_enforcement_test.go
@@ -0,0 +1,104 @@
+// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
+// code in internal/lifecyclerecipe takes every absolute path it operates on from its caller and
+// has no direct production import of internal/lyxcwd.
+//
+// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
+// denylist, mirroring internal/loomrecipe's own reasoning: it catches the excluded import and
+// anything else that would drag geometry resolution in, with no list maintenance beyond a genuine
+// new dependency.
+//
+// github.com/Knatte18/loomyard/contracts/recipes sits outside internal/ and so must be
+// allowlisted explicitly, since the stdlib test below is "the first path segment contains no dot",
+// which a full module path never satisfies.
+
+package lifecyclerecipe
+
+import (
+	"go/parser"
+	"go/token"
+	"io/fs"
+	"path/filepath"
+	"runtime"
+	"strings"
+	"testing"
+)
+
+// lifecyclerecipeAllowedImports are the only non-stdlib import paths production code in this
+// package may use.
+var lifecyclerecipeAllowedImports = map[string]bool{
+	"github.com/Knatte18/loomyard/contracts/recipes":   true,
+	"github.com/Knatte18/loomyard/internal/shedbuild":  true,
+	"github.com/Knatte18/loomyard/internal/shedrecipe": true,
+	"github.com/Knatte18/loomyard/internal/shedengine": true,
+}
+
+// lifecyclerecipeDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes
+// from this package's production files, named here so a violation of that specific rule is
+// reported by name rather than only implied by its absence from the allowlist above.
+const lifecyclerecipeDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"
+
+// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
+// imports only stdlib or an entry in lifecyclerecipeAllowedImports, and separately asserts that no
+// production import path is lifecyclerecipeDeniedLyxcwdImport.
+func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
+	_, file, _, ok := runtime.Caller(0)
+	if !ok {
+		t.Fatal("could not determine lifecyclerecipe source directory location")
+	}
+	pkgDir := filepath.Dir(file)
+
+	var failures []string
+	var deniedFound []string
+
+	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
+		if err != nil {
+			return err
+		}
+		if d.IsDir() {
+			return nil
+		}
+		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
+			return nil
+		}
+
+		fset := token.NewFileSet()
+		astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
+		if err != nil {
+			t.Logf("warning: failed to parse %s: %v", path, err)
+			return nil
+		}
+
+		for _, imp := range astFile.Imports {
+			importPath := strings.Trim(imp.Path.Value, `"`)
+
+			relPath, _ := filepath.Rel(pkgDir, path)
+			if importPath == lifecyclerecipeDeniedLyxcwdImport {
+				deniedFound = append(deniedFound, relPath)
+			}
+
+			firstSegment := importPath
+			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
+				firstSegment = importPath[:idx]
+			}
+			isStdlib := !strings.Contains(firstSegment, ".")
+
+			if isStdlib || lifecyclerecipeAllowedImports[importPath] {
+				continue
+			}
+
+			failures = append(failures, relPath+": "+importPath)
+		}
+
+		return nil
+	})
+	if err != nil {
+		t.Fatalf("failed to walk lifecyclerecipe directory: %v", err)
+	}
+
+	if len(failures) > 0 {
+		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
+	}
+	if len(deniedFound) > 0 {
+		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", lifecyclerecipeDeniedLyxcwdImport, deniedFound)
+	}
+}
diff --git a/internal/lifecycleshed/create.go b/internal/lifecycleshed/create.go
new file mode 100644
index 000000000..d0b1cbd77
--- /dev/null
+++ b/internal/lifecycleshed/create.go
@@ -0,0 +1,86 @@
+// create.go implements NewWorktreeCreate, the producer that creates the task worktree under the
+// hub's prime lock.
+
+package lifecycleshed
+
+import (
+	"context"
+	"fmt"
+
+	"github.com/Knatte18/loomyard/internal/logger"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// worktreeCreateProducer creates the task worktree pair, holding the hub's prime lock for the
+// duration of the create call.
+type worktreeCreateProducer struct {
+	name           string
+	slug           string
+	createWorktree func(context.Context) error
+	primeLock      PrimeLock
+	scratchDir     string
+}
+
+var _ shedengine.ShedProducer = (*worktreeCreateProducer)(nil)
+
+// NewWorktreeCreate returns a shedengine.ShedProducer that acquires primeLock, then calls
+// createWorktree to create the task worktree pair for slug.
+//
+// name is told rather than hardcoded because a second producer list may name this row
+// independently. slug is used only as a log field and in stuck-reason text -- never compared,
+// parsed, or used for control flow.
+func NewWorktreeCreate(name, slug string, createWorktree func(context.Context) error, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer {
+	return &worktreeCreateProducer{
+		name:           name,
+		slug:           slug,
+		createWorktree: createWorktree,
+		primeLock:      primeLock,
+		scratchDir:     scratchDir,
+	}
+}
+
+// Call implements shedengine.ShedProducer. It acquires the prime lock, calls createWorktree while
+// holding it, and releases it on every exit path -- including every Stuck one -- before returning.
+//
+// An Acquire error is a returned hard error: the lock mechanism itself failed, which is not a
+// producer verdict. Acquire reporting ok == false is Stuck, with a reason naming primeLock.Path
+// and the slug -- never a holder identity, which the lock file carries no record of and so cannot
+// report. A createWorktree error is Stuck with that error's own text passed through verbatim and
+// unreworded: fabric's own refusals (the dirty-driving-worktree probe and the pre-existing-branch
+// refusal) already name their own remedies, and reworking that text would only drop information.
+func (p *worktreeCreateProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
+	if err := entryErr(ctx, p.name); err != nil {
+		return "", shedengine.OutputPointer{}, err
+	}
+
+	release, ok, err := p.primeLock.Acquire()
+	if err != nil {
+		return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: acquire prime lock %q: %w", p.name, p.primeLock.Path, err)
+	}
+	if !ok {
+		if cerr := cancelErr(ctx, p.name); cerr != nil {
+			return "", shedengine.OutputPointer{}, cerr
+		}
+		reason := fmt.Sprintf("prime lock %q is already held; another lifecycle producer is creating or tearing down a task worktree", p.primeLock.Path)
+		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+		return shedengine.Stuck, shedengine.OutputPointer{}, nil
+	}
+	defer func() {
+		if rerr := release(); rerr != nil {
+			logger.Warn("lifecycleshed: release prime lock failed", "producer", p.name, "slug", p.slug, "path", p.primeLock.Path, "error", rerr)
+		}
+	}()
+
+	if err := p.createWorktree(ctx); err != nil {
+		if cerr := cancelErr(ctx, p.name); cerr != nil {
+			return "", shedengine.OutputPointer{}, cerr
+		}
+		reportStuck(p.name, err.Error(), p.scratchDir, "slug", p.slug)
+		return shedengine.Stuck, shedengine.OutputPointer{}, nil
+	}
+
+	if cerr := cancelErr(ctx, p.name); cerr != nil {
+		return "", shedengine.OutputPointer{}, cerr
+	}
+	return shedengine.Done, shedengine.OutputPointer{}, nil
+}
diff --git a/internal/lifecycleshed/create_test.go b/internal/lifecycleshed/create_test.go
new file mode 100644
index 000000000..3494ff44c
--- /dev/null
+++ b/internal/lifecycleshed/create_test.go
@@ -0,0 +1,232 @@
+// create_test.go covers NewWorktreeCreate over fake seams: the happy path, the prime-lock
+// dispositions, and createWorktree's error mappings, including fabric's own verbatim refusal text.
+
+package lifecycleshed
+
+import (
+	"context"
+	"errors"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// fakePrimeLock builds a PrimeLock whose Acquire reports the told disposition and records whether
+// its release closure was invoked.
+func fakePrimeLock(path string, ok bool, acquireErr error, releaseErr error, released *bool) PrimeLock {
+	return PrimeLock{
+		Path: path,
+		Acquire: func() (func() error, bool, error) {
+			if acquireErr != nil {
+				return nil, false, acquireErr
+			}
+			if !ok {
+				return nil, false, nil
+			}
+			return func() error {
+				*released = true
+				return releaseErr
+			}, true, nil
+		},
+	}
+}
+
+func readStuckFile(t *testing.T, scratchDir, producer string) string {
+	t.Helper()
+	data, err := os.ReadFile(filepath.Join(scratchDir, producer+stuckFileSuffix))
+	if err != nil {
+		t.Fatalf("read stuck file: %v", err)
+	}
+	return string(data)
+}
+
+func TestWorktreeCreate_HappyPath(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	called := false
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		called = true
+		return nil
+	}, lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Done {
+		t.Errorf("Call() outcome = %v; want Done", outcome)
+	}
+	if !called {
+		t.Error("createWorktree was not called")
+	}
+	if !released {
+		t.Error("release was not invoked on the Done path")
+	}
+}
+
+func TestWorktreeCreate_CreateWorktreeError(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	wantErr := errors.New("some underlying failure")
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		return wantErr
+	}, lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	if !released {
+		t.Error("release was not invoked on the createWorktree-error Stuck path")
+	}
+	reason := readStuckFile(t, scratchDir, "create")
+	if !strings.Contains(reason, wantErr.Error()) {
+		t.Errorf("stuck-reason file = %q; want it to contain %q verbatim", reason, wantErr.Error())
+	}
+}
+
+func TestWorktreeCreate_PreExistingBranchRemedySurvivesVerbatim(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	fabricErr := errors.New(`branch "task-slug" already exists; switch a pair onto it with "lyx fabric checkout task-slug", or delete it first with "git branch -D task-slug" if it is a leftover from a removed pair`)
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		return fabricErr
+	}, lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	reason := readStuckFile(t, scratchDir, "create")
+	if !strings.Contains(reason, "switch a pair onto it with") {
+		t.Errorf("stuck-reason file = %q; want fabric's own remedy wording preserved unreworded", reason)
+	}
+}
+
+func TestWorktreeCreate_DirtyDrivingWorktreePassesThroughVerbatim(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	dirtyErr := errors.New("source worktree has uncommitted changes")
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		return dirtyErr
+	}, lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	reason := readStuckFile(t, scratchDir, "create")
+	if !strings.Contains(reason, "source worktree has uncommitted changes") {
+		t.Errorf("stuck-reason file = %q; want the bare dirty-worktree string preserved verbatim", reason)
+	}
+}
+
+func TestWorktreeCreate_PrimeLockUnavailable(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/contended/path", false, nil, nil, &released)
+
+	called := false
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		called = true
+		return nil
+	}, lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	if called {
+		t.Error("createWorktree was called despite lock contention")
+	}
+	reason := readStuckFile(t, scratchDir, "create")
+	if !strings.Contains(reason, "/lock/contended/path") {
+		t.Errorf("stuck-reason file = %q; want it to name the prime lock path", reason)
+	}
+}
+
+func TestWorktreeCreate_AcquireError(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	acquireErr := errors.New("flock: device error")
+	lock := fakePrimeLock("/lock/path", false, acquireErr, nil, &released)
+
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		return nil
+	}, lock, scratchDir)
+
+	_, _, err := producer.Call(context.Background())
+	if err == nil {
+		t.Fatal("Call() error = nil; want a returned error")
+	}
+	if !errors.Is(err, acquireErr) {
+		t.Errorf("Call() error = %v; want it to wrap %v", err, acquireErr)
+	}
+}
+
+func TestWorktreeCreate_CancelledContext(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel()
+
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		return nil
+	}, lock, scratchDir)
+
+	outcome, _, err := producer.Call(ctx)
+	if err == nil {
+		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
+	}
+	if outcome == shedengine.Stuck {
+		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
+	}
+}
+
+func TestWorktreeCreate_CancelledAfterSuccessfulCreate(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
+		cancel()
+		return nil
+	}, lock, scratchDir)
+
+	outcome, _, err := producer.Call(ctx)
+	if err == nil {
+		t.Fatal("Call() error = nil; want a non-nil error once ctx is cancelled after a successful create")
+	}
+	if outcome == shedengine.Stuck {
+		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
+	}
+	if !released {
+		t.Error("release was not invoked on the cancelled-context path")
+	}
+}
diff --git a/internal/lifecycleshed/ctx.go b/internal/lifecycleshed/ctx.go
new file mode 100644
index 000000000..0b1e95c10
--- /dev/null
+++ b/internal/lifecycleshed/ctx.go
@@ -0,0 +1,33 @@
+// ctx.go implements the two context checks this package's three producers share: entryErr,
+// consulted before Call starts anything, and cancelErr, consulted by every non-Done exit path.
+// This is lifecycleshed's own copy of preflightshed's and landingshed's identically-shaped
+// helpers -- see doc.go for why the duplication is deliberate.
+
+package lifecycleshed
+
+import (
+	"context"
+	"fmt"
+)
+
+// entryErr returns nil when ctx is not yet cancelled, and otherwise a wrapped error naming name
+// (the told producer name), stating the run never started because ctx was already cancelled.
+func entryErr(ctx context.Context, name string) error {
+	if ctx.Err() == nil {
+		return nil
+	}
+	return fmt.Errorf("lifecycleshed: %s: context cancelled before run started: %w", name, ctx.Err())
+}
+
+// cancelErr returns nil when ctx is not cancelled, and otherwise a wrapped error naming name (the
+// told producer name), stating ctx was cancelled during the run. Every non-Done return path in
+// Call consults cancelErr first, replacing its own verdict with this error when the context is
+// cancelled -- this is what discharges the obligation Shed cannot enforce: a Stuck returned under
+// a cancelled context is indistinguishable to Shed from a genuine verdict and would silently
+// consume bounce budget for what was actually an operator stop.
+func cancelErr(ctx context.Context, name string) error {
+	if ctx.Err() == nil {
+		return nil
+	}
+	return fmt.Errorf("lifecycleshed: %s: context cancelled during run: %w", name, ctx.Err())
+}
diff --git a/internal/lifecycleshed/ctx_test.go b/internal/lifecycleshed/ctx_test.go
new file mode 100644
index 000000000..351ea240f
--- /dev/null
+++ b/internal/lifecycleshed/ctx_test.go
@@ -0,0 +1,75 @@
+// ctx_test.go covers entryErr and cancelErr over a live context and a cancelled one.
+
+package lifecycleshed
+
+import (
+	"context"
+	"strings"
+	"testing"
+)
+
+func TestEntryErr(t *testing.T) {
+	tests := []struct {
+		name       string
+		cancel     bool
+		wantErr    bool
+		wantSubstr string
+	}{
+		{name: "live context returns nil", cancel: false, wantErr: false},
+		{name: "cancelled context returns wrapped error", cancel: true, wantErr: true, wantSubstr: "producer-under-test"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			ctx, cancel := context.WithCancel(context.Background())
+			if tt.cancel {
+				cancel()
+			} else {
+				defer cancel()
+			}
+
+			err := entryErr(ctx, "producer-under-test")
+			if tt.wantErr && err == nil {
+				t.Fatalf("entryErr() = nil; want non-nil error")
+			}
+			if !tt.wantErr && err != nil {
+				t.Fatalf("entryErr() = %v; want nil", err)
+			}
+			if tt.wantErr && !strings.Contains(err.Error(), tt.wantSubstr) {
+				t.Errorf("entryErr() = %q; want substring %q", err.Error(), tt.wantSubstr)
+			}
+		})
+	}
+}
+
+func TestCancelErr(t *testing.T) {
+	tests := []struct {
+		name       string
+		cancel     bool
+		wantErr    bool
+		wantSubstr string
+	}{
+		{name: "live context returns nil", cancel: false, wantErr: false},
+		{name: "cancelled context returns wrapped error", cancel: true, wantErr: true, wantSubstr: "producer-under-test"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			ctx, cancel := context.WithCancel(context.Background())
+			if tt.cancel {
+				cancel()
+			} else {
+				defer cancel()
+			}
+
+			err := cancelErr(ctx, "producer-under-test")
+			if tt.wantErr && err == nil {
+				t.Fatalf("cancelErr() = nil; want non-nil error")
+			}
+			if !tt.wantErr && err != nil {
+				t.Fatalf("cancelErr() = %v; want nil", err)
+			}
+			if tt.wantErr && !strings.Contains(err.Error(), tt.wantSubstr) {
+				t.Errorf("cancelErr() = %q; want substring %q", err.Error(), tt.wantSubstr)
+			}
+		})
+	}
+}
diff --git a/internal/lifecycleshed/deps.go b/internal/lifecycleshed/deps.go
new file mode 100644
index 000000000..389929341
--- /dev/null
+++ b/internal/lifecycleshed/deps.go
@@ -0,0 +1,67 @@
+// deps.go declares the three seam types this package's producers are constructed with: PrimeLock,
+// shared by WorktreeCreate and WorktreeTeardown, and LoomRunDeps and TeardownDeps, each specific to
+// one producer.
+
+package lifecycleshed
+
+import (
+	"context"
+	"time"
+
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// PrimeLock carries the told absolute path to a hub-scoped advisory lock plus the injected
+// acquire closure both WorktreeCreate and WorktreeTeardown hold it behind, so the two producers
+// that mutate the hub's worktree registry concurrently with each other never race.
+//
+// Acquire mirrors lock.TryAcquireWriteLock's own contract: a (nil, false, nil) return means the
+// lock is already held by someone else -- contention, not an error -- while a non-nil err means
+// acquisition itself failed. The lock file this package acquires carries no holder record, so a
+// contention stuck reason can name Path and nothing else: there is no way to say who is holding
+// it.
+type PrimeLock struct {
+	// Path is the told absolute lock-file path, named in a contention stuck reason.
+	Path string
+	// Acquire attempts the lock without blocking. A (nil, false, nil) return is contention: the
+	// lock is held elsewhere right now. A non-nil release, when returned, must be called exactly
+	// once by the caller to release the lock.
+	Acquire func() (release func() error, ok bool, err error)
+}
+
+// LoomRunDeps carries every told value and injected closure NewLoomRun needs: spawning the loom
+// session, resolving and reading its persisted status, and the two seams a test replaces to keep
+// the poll loop out of real time.
+type LoomRunDeps struct {
+	// Spawn starts the loom session and blocks until it exits. LoomRun waits for its child rather
+	// than detaching, per the Live-Substrate Spawn Observability invariant.
+	Spawn func(ctx context.Context) error
+	// ResolveStatus resolves the absolute status-file path and its companion lock path for the
+	// task worktree. It is evaluated on Call, never at wiring time: the task worktree this status
+	// file lives in does not exist until WorktreeCreate has already run, so resolving it any
+	// earlier would resolve a path that is not there yet.
+	ResolveStatus func() (statusPath, statusLockPath string, err error)
+	// ReadStatus reads and decodes the persisted status file under statusLockPath's protection,
+	// reporting found == false when no status file exists yet.
+	ReadStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)
+	// Now returns the current time. A nil Now resolves to time.Now in NewLoomRun, so production
+	// code never sets this field; a test holds the clock still by setting it.
+	Now func() time.Time
+	// Sleep pauses for d. A nil Sleep resolves to time.Sleep in NewLoomRun; a test replaces it with
+	// a no-op so the attempt-cap test proves the bound is attempt-counted, not wall-clock-timed,
+	// without spending any real time.
+	Sleep func(d time.Duration)
+}
+
+// TeardownDeps carries the two closures NewWorktreeTeardown calls in sequence: Shutdown strictly
+// before Remove. The two are separate fields, not one closure, because the producer must never
+// call Remove at all when Shutdown fails, must say which of the two failed in its stuck reason,
+// and must surface the abandoned-session value Shutdown returns on an otherwise-Done row -- none
+// of which a single combined closure could expose.
+type TeardownDeps struct {
+	// Shutdown ends the loom session's own driving process, if one is still attached, reporting
+	// the name of any session it had to abandon rather than cleanly end.
+	Shutdown func(ctx context.Context) (abandonedSession string, err error)
+	// Remove deletes the task worktree pair. Called only after Shutdown has succeeded.
+	Remove func(ctx context.Context) error
+}
diff --git a/internal/lifecycleshed/doc.go b/internal/lifecycleshed/doc.go
new file mode 100644
index 000000000..6de18da13
--- /dev/null
+++ b/internal/lifecycleshed/doc.go
@@ -0,0 +1,18 @@
+// Package lifecycleshed owns the three task-worktree lifecycle producers: creating the task
+// worktree, running the loom session inside it, and tearing it down. Any producer list may name
+// them by reference, the same way internal/landingshed frames its own two producers.
+//
+// Told-geometry tier: this package takes every absolute path it operates on from its caller and
+// has no direct production import of internal/lyxcwd, per the Told-Geometry Invariant
+// (CONSTRAINTS.md). Its seam_enforcement_test.go enforces that membership mechanically.
+//
+// This package describes one repository throughout. It is not in the Fabric Vocabulary
+// Invariant's owner set, so none of its identifiers, string literals, or comments may name either
+// fabric-internal side -- write "the task worktree" and "the pair" instead of naming either side
+// by name.
+//
+// It declares its own unexported entryErr/cancelErr helpers (ctx.go) and its own reportStuck
+// carrier (stuck.go) for the same deliberate-duplication reason internal/preflightshed/doc.go and
+// internal/landingshed/stuck.go already record: each producer-owning package carries its own copy
+// rather than sharing one across packages it otherwise has no reason to depend on.
+package lifecycleshed
diff --git a/internal/lifecycleshed/loomrun.go b/internal/lifecycleshed/loomrun.go
new file mode 100644
index 000000000..cb03140df
--- /dev/null
+++ b/internal/lifecycleshed/loomrun.go
@@ -0,0 +1,143 @@
+// loomrun.go implements NewLoomRun, the producer that spawns the loom session inside the task
+// worktree and polls its persisted status to a terminal verdict.
+
+package lifecycleshed
+
+import (
+	"context"
+	"fmt"
+	"time"
+
+	"github.com/Knatte18/loomyard/internal/logger"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// loomRunProducer spawns the loom session for a task worktree and waits for it to reach a
+// terminal state, polling its persisted status up to a bounded number of attempts.
+type loomRunProducer struct {
+	name         string
+	slug         string
+	deps         LoomRunDeps
+	pollInterval time.Duration
+	pollAttempts int
+	scratchDir   string
+}
+
+var _ shedengine.ShedProducer = (*loomRunProducer)(nil)
+
+// NewLoomRun returns a shedengine.ShedProducer that resolves the task worktree's status path,
+// spawns the loom session via deps.Spawn, and polls deps.ReadStatus up to pollAttempts times,
+// waiting pollInterval between attempts, to reach a terminal shedengine.State.
+//
+// A nil deps.Now resolves to time.Now and a nil deps.Sleep resolves to time.Sleep, both resolved
+// once here rather than on every Call, so a test's fake clock and no-op sleep are the only values
+// ever substituted.
+func NewLoomRun(name, slug string, deps LoomRunDeps, pollInterval time.Duration, pollAttempts int, scratchDir string) shedengine.ShedProducer {
+	if deps.Now == nil {
+		deps.Now = time.Now
+	}
+	if deps.Sleep == nil {
+		deps.Sleep = time.Sleep
+	}
+	return &loomRunProducer{
+		name:         name,
+		slug:         slug,
+		deps:         deps,
+		pollInterval: pollInterval,
+		pollAttempts: pollAttempts,
+		scratchDir:   scratchDir,
+	}
+}
+
+// Call implements shedengine.ShedProducer.
+//
+// It resolves the status path, logs the spawn, calls deps.Spawn, logs the completed wait (both log
+// lines are required by the Live-Substrate Spawn Observability invariant, since this producer
+// waits for its child rather than detaching), and then polls deps.ReadStatus up to pollAttempts
+// times.
+//
+// A ResolveStatus error and a ReadStatus error are both returned hard errors, not verdicts: the
+// task worktree is required to exist by the time this row runs, and a status file that exists but
+// does not decode, or a status lock that cannot be taken, is mechanism failure. A Spawn error is
+// Stuck. found == false from ReadStatus is Stuck: the child's own handshake already confirmed a
+// driver took the run lock, so a missing seed at this point is a real inconsistency, not an
+// ordinary not-yet-written race.
+//
+// The verdict table over Status.State is exhaustive: StateDone is Done; StateBlocked, StatePaused
+// and StateFailed are each Stuck, naming the state, the status's own Error field and its
+// CurrentProducer; StateRunning consumes one poll attempt and continues. Any other value is a
+// returned hard error. Exhausting pollAttempts while still StateRunning is Stuck, naming the
+// interval, the attempt count, and the elapsed wall clock computed from deps.Now() readings taken
+// before the first attempt and at exhaustion.
+func (p *loomRunProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
+	if err := entryErr(ctx, p.name); err != nil {
+		return "", shedengine.OutputPointer{}, err
+	}
+
+	statusPath, statusLockPath, err := p.deps.ResolveStatus()
+	if err != nil {
+		return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: resolve status path: %w", p.name, err)
+	}
+
+	logger.Info("lifecycleshed: spawning loom session", "producer", p.name, "slug", p.slug)
+	spawnErr := p.deps.Spawn(ctx)
+	logger.Info("lifecycleshed: loom session wait complete", "producer", p.name, "slug", p.slug)
+	if spawnErr != nil {
+		if cerr := cancelErr(ctx, p.name); cerr != nil {
+			return "", shedengine.OutputPointer{}, cerr
+		}
+		reason := fmt.Sprintf("spawn loom session failed: %s", spawnErr.Error())
+		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+		return shedengine.Stuck, shedengine.OutputPointer{}, nil
+	}
+
+	start := p.deps.Now()
+	for attempt := 1; attempt <= p.pollAttempts; attempt++ {
+		status, found, err := p.deps.ReadStatus(statusPath, statusLockPath)
+		if err != nil {
+			return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: read status: %w", p.name, err)
+		}
+		if !found {
+			if cerr := cancelErr(ctx, p.name); cerr != nil {
+				return "", shedengine.OutputPointer{}, cerr
+			}
+			reason := "no status file found after the loom session's own handshake confirmed a driver took the run lock"
+			reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+			return shedengine.Stuck, shedengine.OutputPointer{}, nil
+		}
+
+		switch status.State {
+		case shedengine.StateDone:
+			if cerr := cancelErr(ctx, p.name); cerr != nil {
+				return "", shedengine.OutputPointer{}, cerr
+			}
+			return shedengine.Done, shedengine.OutputPointer{}, nil
+		case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
+			if cerr := cancelErr(ctx, p.name); cerr != nil {
+				return "", shedengine.OutputPointer{}, cerr
+			}
+			reason := fmt.Sprintf("loom session reached state %q: error=%q current_producer=%q", status.State, status.Error, status.CurrentProducer)
+			reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+			return shedengine.Stuck, shedengine.OutputPointer{}, nil
+		case shedengine.StateRunning:
+			if attempt == p.pollAttempts {
+				break
+			}
+			if cerr := cancelErr(ctx, p.name); cerr != nil {
+				return "", shedengine.OutputPointer{}, cerr
+			}
+			p.deps.Sleep(p.pollInterval)
+			continue
+		default:
+			return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: unrecognized status state %q", p.name, status.State)
+		}
+	}
+
+	if cerr := cancelErr(ctx, p.name); cerr != nil {
+		return "", shedengine.OutputPointer{}, cerr
+	}
+	elapsed := p.deps.Now().Sub(start)
+	reason := fmt.Sprintf("loom session still running after %d attempts at interval %s (elapsed %s)", p.pollAttempts, p.pollInterval, elapsed)
+	reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+	return shedengine.Stuck, shedengine.OutputPointer{}, nil
+}
diff --git a/internal/lifecycleshed/loomrun_test.go b/internal/lifecycleshed/loomrun_test.go
new file mode 100644
index 000000000..03459fc9f
--- /dev/null
+++ b/internal/lifecycleshed/loomrun_test.go
@@ -0,0 +1,297 @@
+// loomrun_test.go covers NewLoomRun's full verdict table over a fake ReadStatus, a fake Now, and a
+// fake Sleep that never sleeps -- so the attempt-cap test proves the bound is attempt-counted, not
+// wall-clock-timed, in unmeasurable real time.
+
+package lifecycleshed
+
+import (
+	"context"
+	"errors"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// fakeClock is a Now/Sleep pair a test can hold still: Now always returns the same instant
+// (advanced only when the test wants to prove elapsed-time reporting), and Sleep records calls
+// without ever blocking.
+type fakeClock struct {
+	now        time.Time
+	sleepCalls int
+}
+
+func (c *fakeClock) Now() time.Time { return c.now }
+func (c *fakeClock) Sleep(d time.Duration) {
+	c.sleepCalls++
+}
+
+func newLoomRunDeps(spawnErr error, resolveErr error, statuses []statusResult, clock *fakeClock) (*int, *int, LoomRunDeps) {
+	readCalls := 0
+	spawnCalls := 0
+	deps := LoomRunDeps{
+		Spawn: func(ctx context.Context) error {
+			spawnCalls++
+			return spawnErr
+		},
+		ResolveStatus: func() (string, string, error) {
+			if resolveErr != nil {
+				return "", "", resolveErr
+			}
+			return "/status/path", "/status/lock/path", nil
+		},
+		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+			idx := readCalls
+			readCalls++
+			if idx >= len(statuses) {
+				t := statuses[len(statuses)-1]
+				return t.status, t.found, t.err
+			}
+			r := statuses[idx]
+			return r.status, r.found, r.err
+		},
+		Now:   clock.Now,
+		Sleep: clock.Sleep,
+	}
+	return &readCalls, &spawnCalls, deps
+}
+
+type statusResult struct {
+	status shedengine.Status
+	found  bool
+	err    error
+}
+
+func TestLoomRun_VerdictTable(t *testing.T) {
+	tests := []struct {
+		name       string
+		statuses   []statusResult
+		wantDone   bool
+		wantStuck  bool
+		wantErr    bool
+		wantReason string
+	}{
+		{
+			name:     "StateDone",
+			statuses: []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}},
+			wantDone: true,
+		},
+		{
+			name:       "StateBlocked",
+			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StateBlocked, Error: "boom", CurrentProducer: "p1"}, found: true}},
+			wantStuck:  true,
+			wantReason: "blocked",
+		},
+		{
+			name:       "StatePaused",
+			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StatePaused, Error: "paused-err", CurrentProducer: "p2"}, found: true}},
+			wantStuck:  true,
+			wantReason: "paused",
+		},
+		{
+			name:       "StateFailed",
+			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StateFailed, Error: "failed-err", CurrentProducer: "p3"}, found: true}},
+			wantStuck:  true,
+			wantReason: "failed",
+		},
+		{
+			name: "StateRunningThenDone",
+			statuses: []statusResult{
+				{status: shedengine.Status{State: shedengine.StateRunning}, found: true},
+				{status: shedengine.Status{State: shedengine.StateDone}, found: true},
+			},
+			wantDone: true,
+		},
+		{
+			name:      "AbsentStatusAfterSuccessfulSpawn",
+			statuses:  []statusResult{{found: false}},
+			wantStuck: true,
+		},
+		{
+			name:     "ReadErrorIsReturnedError",
+			statuses: []statusResult{{err: errors.New("decode failed")}},
+			wantErr:  true,
+		},
+		{
+			name:     "UnrecognizedStateIsReturnedError",
+			statuses: []statusResult{{status: shedengine.Status{State: "bogus"}, found: true}},
+			wantErr:  true,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			scratchDir := t.TempDir()
+			clock := &fakeClock{now: time.Unix(0, 0)}
+			_, _, deps := newLoomRunDeps(nil, nil, tt.statuses, clock)
+
+			producer := NewLoomRun("loomrun", "myslug", deps, time.Millisecond, 5, scratchDir)
+			outcome, _, err := producer.Call(context.Background())
+
+			if tt.wantErr {
+				if err == nil {
+					t.Fatalf("Call() error = nil; want non-nil")
+				}
+				return
+			}
+			if err != nil {
+				t.Fatalf("Call() error = %v; want nil", err)
+			}
+			if tt.wantDone && outcome != shedengine.Done {
+				t.Errorf("Call() outcome = %v; want Done", outcome)
+			}
+			if tt.wantStuck && outcome != shedengine.Stuck {
+				t.Errorf("Call() outcome = %v; want Stuck", outcome)
+			}
+			if tt.wantReason != "" {
+				reason := readStuckFile(t, scratchDir, "loomrun")
+				if !strings.Contains(reason, tt.wantReason) {
+					t.Errorf("stuck-reason file = %q; want substring %q", reason, tt.wantReason)
+				}
+			}
+		})
+	}
+}
+
+func TestLoomRun_SpawnFailureIsStuck(t *testing.T) {
+	scratchDir := t.TempDir()
+	clock := &fakeClock{now: time.Unix(0, 0)}
+	spawnErr := errors.New("spawn failed")
+	_, spawnCalls, deps := newLoomRunDeps(spawnErr, nil, nil, clock)
+
+	producer := NewLoomRun("loomrun", "myslug", deps, time.Millisecond, 5, scratchDir)
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	if *spawnCalls != 1 {
+		t.Errorf("spawn calls = %d; want 1", *spawnCalls)
+	}
+}
+
+func TestLoomRun_ResolveStatusFailureIsReturnedError(t *testing.T) {
+	scratchDir := t.TempDir()
+	clock := &fakeClock{now: time.Unix(0, 0)}
+	resolveErr := errors.New("resolve failed")
+	_, _, deps := newLoomRunDeps(nil, resolveErr, nil, clock)
+
+	producer := NewLoomRun("loomrun", "myslug", deps, time.Millisecond, 5, scratchDir)
+	_, _, err := producer.Call(context.Background())
+	if err == nil {
+		t.Fatal("Call() error = nil; want a returned error")
+	}
+	if !errors.Is(err, resolveErr) {
+		t.Errorf("Call() error = %v; want it to wrap %v", err, resolveErr)
+	}
+}
+
+func TestLoomRun_CancelledContext(t *testing.T) {
+	scratchDir := t.TempDir()
+	clock := &fakeClock{now: time.Unix(0, 0)}
+	_, _, deps := newLoomRunDeps(nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}}, clock)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel()
+
+	producer := NewLoomRun("loomrun", "myslug", deps, time.Millisecond, 5, scratchDir)
+	outcome, _, err := producer.Call(ctx)
+	if err == nil {
+		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
+	}
+	if outcome == shedengine.Stuck {
+		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
+	}
+}
+
+// TestLoomRun_StateFailedConsumesZeroPollAttempts asserts StateFailed resolves on its first read,
+// calling ReadStatus exactly once and never calling Sleep, proving it does not loop through the
+// remaining poll budget the way StateRunning does.
+func TestLoomRun_StateFailedConsumesZeroPollAttempts(t *testing.T) {
+	scratchDir := t.TempDir()
+	clock := &fakeClock{now: time.Unix(0, 0)}
+	readCalls, _, deps := newLoomRunDeps(nil, nil, []statusResult{
+		{status: shedengine.Status{State: shedengine.StateFailed, Error: "boom"}, found: true},
+	}, clock)
+
+	producer := NewLoomRun("loomrun", "myslug", deps, time.Millisecond, 5, scratchDir)
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	if *readCalls != 1 {
+		t.Errorf("ReadStatus calls = %d; want exactly 1", *readCalls)
+	}
+	if clock.sleepCalls != 0 {
+		t.Errorf("Sleep calls = %d; want 0", clock.sleepCalls)
+	}
+}
+
+// TestLoomRun_AttemptCapFiresOnCountNotWallClock proves the poll bound is attempt-counted: with the
+// fake clock held still (Now never advances) and Sleep never actually sleeping, exhausting
+// pollAttempts while the state stays StateRunning still produces Stuck, in unmeasurable real time.
+func TestLoomRun_AttemptCapFiresOnCountNotWallClock(t *testing.T) {
+	scratchDir := t.TempDir()
+	clock := &fakeClock{now: time.Unix(0, 0)}
+	const pollAttempts = 4
+	statuses := make([]statusResult, pollAttempts)
+	for i := range statuses {
+		statuses[i] = statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
+	}
+	readCalls, _, deps := newLoomRunDeps(nil, nil, statuses, clock)
+
+	producer := NewLoomRun("loomrun", "myslug", deps, 5*time.Second, pollAttempts, scratchDir)
+
+	start := time.Now()
+	outcome, _, err := producer.Call(context.Background())
+	elapsed := time.Since(start)
+
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	if *readCalls != pollAttempts {
+		t.Errorf("ReadStatus calls = %d; want exactly %d", *readCalls, pollAttempts)
+	}
+	if clock.sleepCalls != pollAttempts-1 {
+		t.Errorf("Sleep calls = %d; want %d (sleeps happen only between attempts)", clock.sleepCalls, pollAttempts-1)
+	}
+	if elapsed >= time.Second {
+		t.Errorf("Call() took %s of real time; want well under 1s, proving the fake Sleep never actually slept", elapsed)
+	}
+	reason := readStuckFile(t, scratchDir, "loomrun")
+	if !strings.Contains(reason, "4") {
+		t.Errorf("stuck-reason file = %q; want it to name the attempt count", reason)
+	}
+}
+
+// NewLoomRun's nil-seam defaults are exercised implicitly by every production caller; this test
+// pins that a nil Now/Sleep resolve to the real stdlib functions rather than panicking.
+func TestLoomRun_NilSeamsDefaultToStdlib(t *testing.T) {
+	scratchDir := t.TempDir()
+	deps := LoomRunDeps{
+		Spawn: func(ctx context.Context) error { return nil },
+		ResolveStatus: func() (string, string, error) {
+			return "/status/path", "/status/lock/path", nil
+		},
+		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
+			return shedengine.Status{State: shedengine.StateDone}, true, nil
+		},
+	}
+	producer := NewLoomRun("loomrun", "myslug", deps, time.Millisecond, 1, scratchDir)
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Done {
+		t.Errorf("Call() outcome = %v; want Done", outcome)
+	}
+}
diff --git a/internal/lifecycleshed/seam_enforcement_test.go b/internal/lifecycleshed/seam_enforcement_test.go
new file mode 100644
index 000000000..7366fe062
--- /dev/null
+++ b/internal/lifecycleshed/seam_enforcement_test.go
@@ -0,0 +1,98 @@
+// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
+// code in internal/lifecycleshed takes every absolute path it operates on from its caller and has
+// no direct production import of internal/lyxcwd.
+//
+// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
+// denylist, mirroring internal/loomrecipe's and internal/shedrecipe's own reasoning: it catches the
+// excluded import and anything else that would drag geometry resolution in, with no list
+// maintenance beyond a genuine new dependency.
+
+package lifecycleshed
+
+import (
+	"go/parser"
+	"go/token"
+	"io/fs"
+	"path/filepath"
+	"runtime"
+	"strings"
+	"testing"
+)
+
+// lifecycleshedAllowedImports are the only non-stdlib import paths production code in this package
+// may use.
+var lifecycleshedAllowedImports = map[string]bool{
+	"github.com/Knatte18/loomyard/internal/shedengine": true,
+	"github.com/Knatte18/loomyard/internal/logger":     true,
+}
+
+// lifecycleshedDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes
+// from this package's production files, named here so a violation of that specific rule is
+// reported by name rather than only implied by its absence from the allowlist above.
+const lifecycleshedDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"
+
+// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
+// imports only stdlib or an entry in lifecycleshedAllowedImports, and separately asserts that no
+// production import path is lifecycleshedDeniedLyxcwdImport.
+func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
+	_, file, _, ok := runtime.Caller(0)
+	if !ok {
+		t.Fatal("could not determine lifecycleshed source directory location")
+	}
+	pkgDir := filepath.Dir(file)
+
+	var failures []string
+	var deniedFound []string
+
+	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
+		if err != nil {
+			return err
+		}
+		if d.IsDir() {
+			return nil
+		}
+		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
+			return nil
+		}
+
+		fset := token.NewFileSet()
+		astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
+		if err != nil {
+			t.Logf("warning: failed to parse %s: %v", path, err)
+			return nil
+		}
+
+		for _, imp := range astFile.Imports {
+			importPath := strings.Trim(imp.Path.Value, `"`)
+
+			relPath, _ := filepath.Rel(pkgDir, path)
+			if importPath == lifecycleshedDeniedLyxcwdImport {
+				deniedFound = append(deniedFound, relPath)
+			}
+
+			firstSegment := importPath
+			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
+				firstSegment = importPath[:idx]
+			}
+			isStdlib := !strings.Contains(firstSegment, ".")
+
+			if isStdlib || lifecycleshedAllowedImports[importPath] {
+				continue
+			}
+
+			failures = append(failures, relPath+": "+importPath)
+		}
+
+		return nil
+	})
+	if err != nil {
+		t.Fatalf("failed to walk lifecycleshed directory: %v", err)
+	}
+
+	if len(failures) > 0 {
+		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
+	}
+	if len(deniedFound) > 0 {
+		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", lifecycleshedDeniedLyxcwdImport, deniedFound)
+	}
+}
diff --git a/internal/lifecycleshed/stuck.go b/internal/lifecycleshed/stuck.go
new file mode 100644
index 000000000..784b608a8
--- /dev/null
+++ b/internal/lifecycleshed/stuck.go
@@ -0,0 +1,48 @@
+// stuck.go declares reportStuck, the one helper all three producers in this package route every
+// stuck verdict through.
+//
+// The producer seam (shedengine.ShedProducer) returns only a verdict, an output pointer, and an
+// error. shedengine.Run persists its own fixed reason string -- "stuck with no OnStuck target" --
+// for every stuck verdict regardless of what the producer knows, so a producer-supplied reason
+// reaches a human only through the structured warning log line this helper writes and the one-line
+// reason file it also writes -- the log scrolls away in an unattended run, so the file is what the
+// tests assert two stuck causes are distinguishable against, since the engine's own persisted
+// reason is identical in all of them.
+
+package lifecycleshed
+
+import (
+	"fmt"
+	"os"
+	"path/filepath"
+
+	"github.com/Knatte18/loomyard/internal/logger"
+)
+
+// stuckFileSuffix is the fixed suffix every producer's reason file carries, joined onto the
+// producer's own name.
+const stuckFileSuffix = "-stuck.md"
+
+// reportStuck emits a structured warning through the shared logger carrying at minimum a producer
+// field and a reason field alongside fields' own key-value pairs, and writes a one-line reason file
+// at <scratchDir>/<producer>-stuck.md, overwritten each attempt.
+//
+// It creates scratchDir with os.MkdirAll first, on every write path -- a told directory may not
+// exist yet, and making a told path usable is legal where deriving one would not be. A write
+// failure is logged rather than swallowed, and never replaces the caller's own verdict: failing to
+// record why something is stuck must not change whether it is stuck, so this function returns
+// nothing for a caller to act on.
+func reportStuck(producer, reason, scratchDir string, fields ...any) {
+	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
+	logger.Warn("lifecycleshed: producer stuck", logFields...)
+
+	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
+		logger.Warn("lifecycleshed: create scratch directory for stuck-reason file failed", "producer", producer, "scratchDir", scratchDir, "error", err)
+		return
+	}
+
+	path := filepath.Join(scratchDir, producer+stuckFileSuffix)
+	if err := os.WriteFile(path, []byte(fmt.Sprintf("%s\n", reason)), 0o644); err != nil {
+		logger.Warn("lifecycleshed: write stuck-reason file failed", "producer", producer, "path", path, "error", err)
+	}
+}
diff --git a/internal/lifecycleshed/teardown.go b/internal/lifecycleshed/teardown.go
new file mode 100644
index 000000000..088373055
--- /dev/null
+++ b/internal/lifecycleshed/teardown.go
@@ -0,0 +1,102 @@
+// teardown.go implements NewWorktreeTeardown, the producer that shuts down the loom session and
+// removes the task worktree, strictly in that order, under the hub's prime lock.
+
+package lifecycleshed
+
+import (
+	"context"
+	"fmt"
+
+	"github.com/Knatte18/loomyard/internal/logger"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// worktreeTeardownProducer shuts down the task worktree's loom session and then removes the
+// worktree pair, holding the hub's prime lock for the duration of both calls.
+type worktreeTeardownProducer struct {
+	name       string
+	slug       string
+	deps       TeardownDeps
+	primeLock  PrimeLock
+	scratchDir string
+}
+
+var _ shedengine.ShedProducer = (*worktreeTeardownProducer)(nil)
+
+// NewWorktreeTeardown returns a shedengine.ShedProducer that acquires primeLock, calls
+// deps.Shutdown, and -- only on Shutdown's success -- calls deps.Remove, for the task worktree
+// identified by slug.
+func NewWorktreeTeardown(name, slug string, deps TeardownDeps, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer {
+	return &worktreeTeardownProducer{
+		name:       name,
+		slug:       slug,
+		deps:       deps,
+		primeLock:  primeLock,
+		scratchDir: scratchDir,
+	}
+}
+
+// Call implements shedengine.ShedProducer. It acquires the prime lock exactly as
+// worktreeCreateProducer.Call does -- an Acquire error is a returned hard error, ok == false is
+// Stuck naming primeLock.Path, and the release closure is deferred so it runs on every exit path
+// including every Stuck one, with a release error logged at Warn rather than replacing the
+// verdict.
+//
+// It then calls deps.Shutdown. A Shutdown error is Stuck naming session shutdown as the failed
+// half, and deps.Remove is not called at all on that path -- abandoning that ordering is the
+// single thing this one-row producer exists to prevent. A non-empty abandonedSession return is
+// logged at Warn naming the slug and the session, and does not change the verdict.
+//
+// It then calls deps.Remove. A Remove error is Stuck naming worktree removal as the failed half
+// and stating that session shutdown already succeeded, so the two halves are distinguishable in
+// the reason text. On success the row returns Done.
+func (p *worktreeTeardownProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
+	if err := entryErr(ctx, p.name); err != nil {
+		return "", shedengine.OutputPointer{}, err
+	}
+
+	release, ok, err := p.primeLock.Acquire()
+	if err != nil {
+		return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: acquire prime lock %q: %w", p.name, p.primeLock.Path, err)
+	}
+	if !ok {
+		if cerr := cancelErr(ctx, p.name); cerr != nil {
+			return "", shedengine.OutputPointer{}, cerr
+		}
+		reason := fmt.Sprintf("prime lock %q is already held; another lifecycle producer is creating or tearing down a task worktree", p.primeLock.Path)
+		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+		return shedengine.Stuck, shedengine.OutputPointer{}, nil
+	}
+	defer func() {
+		if rerr := release(); rerr != nil {
+			logger.Warn("lifecycleshed: release prime lock failed", "producer", p.name, "slug", p.slug, "path", p.primeLock.Path, "error", rerr)
+		}
+	}()
+
+	abandonedSession, shutdownErr := p.deps.Shutdown(ctx)
+	if shutdownErr != nil {
+		if cerr := cancelErr(ctx, p.name); cerr != nil {
+			return "", shedengine.OutputPointer{}, cerr
+		}
+		reason := fmt.Sprintf("session shutdown failed: %s", shutdownErr.Error())
+		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+		return shedengine.Stuck, shedengine.OutputPointer{}, nil
+	}
+	if abandonedSession != "" {
+		logger.Warn("lifecycleshed: session shutdown abandoned a session rather than ending it cleanly", "producer", p.name, "slug", p.slug, "session", abandonedSession)
+	}
+
+	if err := p.deps.Remove(ctx); err != nil {
+		if cerr := cancelErr(ctx, p.name); cerr != nil {
+			return "", shedengine.OutputPointer{}, cerr
+		}
+		reason := fmt.Sprintf("worktree removal failed (session shutdown already succeeded): %s", err.Error())
+		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
+		return shedengine.Stuck, shedengine.OutputPointer{}, nil
+	}
+
+	if cerr := cancelErr(ctx, p.name); cerr != nil {
+		return "", shedengine.OutputPointer{}, cerr
+	}
+	return shedengine.Done, shedengine.OutputPointer{}, nil
+}
diff --git a/internal/lifecycleshed/teardown_test.go b/internal/lifecycleshed/teardown_test.go
new file mode 100644
index 000000000..9d82db95d
--- /dev/null
+++ b/internal/lifecycleshed/teardown_test.go
@@ -0,0 +1,231 @@
+// teardown_test.go covers NewWorktreeTeardown over fake seams: the strict Shutdown-before-Remove
+// ordering, which half a stuck reason names, the abandoned-session log path, and prime-lock
+// dispositions.
+
+package lifecycleshed
+
+import (
+	"context"
+	"errors"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// teardownCallRecorder records the order Shutdown and Remove were called in.
+type teardownCallRecorder struct {
+	calls []string
+}
+
+func (r *teardownCallRecorder) deps(shutdownErr error, abandonedSession string, removeErr error) TeardownDeps {
+	return TeardownDeps{
+		Shutdown: func(ctx context.Context) (string, error) {
+			r.calls = append(r.calls, "shutdown")
+			return abandonedSession, shutdownErr
+		},
+		Remove: func(ctx context.Context) error {
+			r.calls = append(r.calls, "remove")
+			return removeErr
+		},
+	}
+}
+
+func TestWorktreeTeardown_HappyPathOrdering(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	rec := &teardownCallRecorder{}
+	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Done {
+		t.Errorf("Call() outcome = %v; want Done", outcome)
+	}
+	if len(rec.calls) != 2 || rec.calls[0] != "shutdown" || rec.calls[1] != "remove" {
+		t.Errorf("call order = %v; want [shutdown remove]", rec.calls)
+	}
+	if !released {
+		t.Error("release was not invoked on the Done path")
+	}
+}
+
+func TestWorktreeTeardown_ShutdownFailureNeverCallsRemove(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	shutdownErr := errors.New("session would not end")
+	rec := &teardownCallRecorder{}
+	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(shutdownErr, "", nil), lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	if len(rec.calls) != 1 || rec.calls[0] != "shutdown" {
+		t.Errorf("call order = %v; want [shutdown] only, Remove must never be called", rec.calls)
+	}
+	if !released {
+		t.Error("release was not invoked on the Shutdown-error Stuck path")
+	}
+	reason := readStuckFile(t, scratchDir, "teardown")
+	if !strings.Contains(reason, "session shutdown") {
+		t.Errorf("stuck-reason file = %q; want it to name session shutdown as the failed half", reason)
+	}
+	if !strings.Contains(reason, shutdownErr.Error()) {
+		t.Errorf("stuck-reason file = %q; want it to contain the underlying error", reason)
+	}
+}
+
+func TestWorktreeTeardown_RemoveFailureNamesRemovalHalf(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	removeErr := errors.New("worktree remove refused: merge in progress")
+	rec := &teardownCallRecorder{}
+	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	if len(rec.calls) != 2 {
+		t.Errorf("call count = %d; want 2 (both Shutdown and Remove called)", len(rec.calls))
+	}
+	reason := readStuckFile(t, scratchDir, "teardown")
+	if !strings.Contains(reason, "worktree removal") {
+		t.Errorf("stuck-reason file = %q; want it to name worktree removal as the failed half", reason)
+	}
+	if !strings.Contains(reason, "shutdown") || !strings.Contains(reason, "already succeeded") {
+		t.Errorf("stuck-reason file = %q; want it to state that shutdown already succeeded", reason)
+	}
+}
+
+func TestWorktreeTeardown_RemoveRefusalMergeInProgress(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	removeErr := errors.New("refuse to remove: merge in progress")
+	rec := &teardownCallRecorder{}
+	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Stuck {
+		t.Errorf("Call() outcome = %v; want Stuck", outcome)
+	}
+	reason := readStuckFile(t, scratchDir, "teardown")
+	if !strings.Contains(reason, "merge in progress") {
+		t.Errorf("stuck-reason file = %q; want the merge-in-progress refusal text preserved", reason)
+	}
+}
+
+func TestWorktreeTeardown_AbandonedSessionOnOtherwiseDoneRow(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	rec := &teardownCallRecorder{}
+	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "abandoned-session-1", nil), lock, scratchDir)
+
+	outcome, _, err := producer.Call(context.Background())
+	if err != nil {
+		t.Fatalf("Call() error = %v; want nil", err)
+	}
+	if outcome != shedengine.Done {
+		t.Errorf("Call() outcome = %v; want Done even with a non-empty abandoned session", outcome)
+	}
+}
+
+func TestWorktreeTeardown_PrimeLockDispositions(t *testing.T) {
+	t.Run("contention", func(t *testing.T) {
+		scratchDir := t.TempDir()
+		var released bool
+		lock := fakePrimeLock("/lock/contended/path", false, nil, nil, &released)
+		rec := &teardownCallRecorder{}
+		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)
+
+		outcome, _, err := producer.Call(context.Background())
+		if err != nil {
+			t.Fatalf("Call() error = %v; want nil", err)
+		}
+		if outcome != shedengine.Stuck {
+			t.Errorf("Call() outcome = %v; want Stuck", outcome)
+		}
+		if len(rec.calls) != 0 {
+			t.Errorf("call count = %d; want 0 under lock contention", len(rec.calls))
+		}
+		reason := readStuckFile(t, scratchDir, "teardown")
+		if !strings.Contains(reason, "/lock/contended/path") {
+			t.Errorf("stuck-reason file = %q; want it to name the prime lock path", reason)
+		}
+	})
+
+	t.Run("acquire error", func(t *testing.T) {
+		scratchDir := t.TempDir()
+		var released bool
+		acquireErr := errors.New("flock: device error")
+		lock := fakePrimeLock("/lock/path", false, acquireErr, nil, &released)
+		rec := &teardownCallRecorder{}
+		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)
+
+		_, _, err := producer.Call(context.Background())
+		if err == nil {
+			t.Fatal("Call() error = nil; want a returned error")
+		}
+		if !errors.Is(err, acquireErr) {
+			t.Errorf("Call() error = %v; want it to wrap %v", err, acquireErr)
+		}
+	})
+
+	t.Run("release on every exit path", func(t *testing.T) {
+		scratchDir := t.TempDir()
+		var released bool
+		lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+		removeErr := errors.New("removal failed")
+		rec := &teardownCallRecorder{}
+		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)
+
+		if _, _, err := producer.Call(context.Background()); err != nil {
+			t.Fatalf("Call() error = %v; want nil", err)
+		}
+		if !released {
+			t.Error("release was not invoked on the Remove-error Stuck path")
+		}
+	})
+}
+
+func TestWorktreeTeardown_CancelledContext(t *testing.T) {
+	scratchDir := t.TempDir()
+	var released bool
+	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel()
+
+	rec := &teardownCallRecorder{}
+	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)
+
+	outcome, _, err := producer.Call(ctx)
+	if err == nil {
+		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
+	}
+	if outcome == shedengine.Stuck {
+		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
+	}
+}
diff --git a/internal/logger/logger.go b/internal/logger/logger.go
index 4ba5ef3c0..1d7b9bea8 100644
--- a/internal/logger/logger.go
+++ b/internal/logger/logger.go
@@ -376,6 +376,13 @@ func Warn(msg string, args ...any) {
 	log.With("trace", TraceID()).Warn(msg, args...)
 }
 
+// Error logs msg at error level with the given key/value args, stamping trace= as Debug does.
+// Like Warn, it reaches both the durable sink and the stderr half unconditionally, since error
+// level sits above the default Warn threshold.
+func Error(msg string, args ...any) {
+	log.With("trace", TraceID()).Error(msg, args...)
+}
+
 // SetVerbosity maps a -v repeat count to a log level: count<=0 keeps the default Warn threshold
 // (silent normal run), count==1 lowers it to Info, and count>=2 lowers it to Debug.
 // cmd/lyx/main.go calls this once at startup from the root -v/--verbose flag.
diff --git a/internal/loomcli/bootstrap.go b/internal/loomcli/bootstrap.go
index 08589d63b..53661ce40 100644
--- a/internal/loomcli/bootstrap.go
+++ b/internal/loomcli/bootstrap.go
@@ -31,6 +31,19 @@ func mustSpawnDriver(runLockHeld bool) bool {
 	return !runLockHeld
 }
 
+// mustAttach reports whether the bootstrap must hand the terminal over to the tmux session, from the
+// operator's own --no-attach choice. It is the twin of mustSpawnDriver: both are the whole of a
+// re-entrancy or handoff decision, expressed as one pure predicate rather than written inline in the
+// verb body.
+//
+// The terminal handover this predicate gates is the CLI/Cobra Invariant's narrow interactive-handoff
+// exception for `lyx loom run`/`lyx run`. Skipping it on noAttach's say-so removes that exception for
+// this one invocation -- every step before it, including the run-lock handshake, still runs -- rather
+// than adding a new exception of its own.
+func mustAttach(noAttach bool) bool {
+	return !noAttach
+}
+
 // awaitRunLockResult is the four-way outcome of awaitRunLock.
 type awaitRunLockResult int
 
@@ -180,8 +193,8 @@ func resolveStatusStrandAction(strands []reedengine.StrandStatus) (statusStrandA
 }
 
 // statusStrandCmd composes the status strand's pane command line through the shell seam, exactly as
-// the reed header pane's own builder (headerLaunchCmd, headerpane.go) composes its command line: exe
-// invoked with the two-word status verb and the watch flag.
+// the watchdog daemon's own spawn (ensureWatchdogSpawned, internal/reedcli/spawnwatchdog.go) composes
+// its os.Executable() command line: exe invoked with the two-word status verb and the watch flag.
 func statusStrandCmd(sh shell.Shell, exe string) string {
 	return sh.Invoke(exe) + " " + sh.Quote("loom") + " " + sh.Quote("status") + " " + sh.Quote("--watch")
 }
diff --git a/internal/loomcli/bootstrap_test.go b/internal/loomcli/bootstrap_test.go
index 9b434cf00..a2fd1b378 100644
--- a/internal/loomcli/bootstrap_test.go
+++ b/internal/loomcli/bootstrap_test.go
@@ -28,6 +28,50 @@ func TestMustSpawnDriver(t *testing.T) {
 	}
 }
 
+func TestMustAttach(t *testing.T) {
+	tests := []struct {
+		name       string
+		noAttach   bool
+		wantAttach bool
+	}{
+		{"NoAttachSet_NoAttach", true, false},
+		{"NoAttachUnset_Attach", false, true},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := mustAttach(tt.noAttach)
+			if got != tt.wantAttach {
+				t.Errorf("mustAttach(%v) = %v; want %v", tt.noAttach, got, tt.wantAttach)
+			}
+		})
+	}
+}
+
+// TestRunVerb_NoAttachFlag_DefaultsFalse pins the regression this flag most plausibly causes: a
+// silently flipped default. It reads the built command tree's own flag lookup -- rather than the
+// package variable a stray reassignment elsewhere in the package could leave stale -- so the
+// assertion covers registration and default together, then ties that default to the branch it
+// controls by feeding it straight into mustAttach.
+//
+// An invocation that never passes --no-attach must take today's attach path unchanged, and nothing
+// else in this package would catch a default silently flipped to true.
+func TestRunVerb_NoAttachFlag_DefaultsFalse(t *testing.T) {
+	c := &loomCLI{}
+	flag := c.runCmd().Flags().Lookup("no-attach")
+	if flag == nil {
+		t.Fatal(`"run" command is missing the --no-attach flag`)
+	}
+	if flag.DefValue != "false" {
+		t.Errorf(`"--no-attach" default = %q; want "false"`, flag.DefValue)
+	}
+
+	defaultNoAttach := flag.Value.String() == "true"
+	if got := mustAttach(defaultNoAttach); !got {
+		t.Errorf("mustAttach(%v) over the registered default = %v; want true -- an invocation with no flag must still attach", defaultNoAttach, got)
+	}
+}
+
 // countingWait returns a wait seam that counts its own invocations, for use in place of a real
 // sleep in awaitRunLock tests.
 func countingWait(count *int) func() {
diff --git a/internal/loomcli/cli_test.go b/internal/loomcli/cli_test.go
index b19a9c694..3e1c5e864 100644
--- a/internal/loomcli/cli_test.go
+++ b/internal/loomcli/cli_test.go
@@ -78,8 +78,9 @@ func TestCommand_RegisteredVerbs_ExactSet(t *testing.T) {
 
 // TestRunAliasCommand_StaysOneCommandWithSubtreeVerb guards RunAliasCommand and the subtree's own
 // runCmd against drifting into two different commands: the alias must carry a non-empty Short, its
-// Use must be the bare verb ("run"), and it must expose the same --parent flag the subtree's own run
-// verb does.
+// Use must be the bare verb ("run"), and it must expose the same --parent and --no-attach flags the
+// subtree's own run verb does -- the alias takes the subtree's own runCmd unchanged, so it would
+// otherwise silently diverge whenever a new flag lands on the subtree verb alone.
 func TestRunAliasCommand_StaysOneCommandWithSubtreeVerb(t *testing.T) {
 	alias := RunAliasCommand()
 
@@ -92,6 +93,29 @@ func TestRunAliasCommand_StaysOneCommandWithSubtreeVerb(t *testing.T) {
 	if alias.Flags().Lookup("parent") == nil {
 		t.Error("RunAliasCommand() is missing the --parent flag the subtree's run verb exposes")
 	}
+	if alias.Flags().Lookup("no-attach") == nil {
+		t.Error("RunAliasCommand() is missing the --no-attach flag the subtree's run verb exposes")
+	}
+}
+
+// TestCommand_RunVerb_RegistersNoAttachFlag asserts that the "run" verb registered under the "loom"
+// parent command -- not just its bare-root alias -- exposes --no-attach, since it is the flag's
+// primary home.
+func TestCommand_RunVerb_RegistersNoAttachFlag(t *testing.T) {
+	parent := Command()
+
+	var run *cobra.Command
+	for _, sub := range parent.Commands() {
+		if sub.Name() == "run" {
+			run = sub
+		}
+	}
+	if run == nil {
+		t.Fatal(`"run" verb is not registered under the loom parent command`)
+	}
+	if run.Flags().Lookup("no-attach") == nil {
+		t.Error(`"loom run" is missing the --no-attach flag`)
+	}
 }
 
 // TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lyx loom" invocation succeeds without
diff --git a/internal/loomcli/run.go b/internal/loomcli/run.go
index edfb371d0..46f18b4d2 100644
--- a/internal/loomcli/run.go
+++ b/internal/loomcli/run.go
@@ -39,6 +39,7 @@ const (
 // runCmd builds the `run` subcommand: the session bootstrap.
 func (c *loomCLI) runCmd() *cobra.Command {
 	var parentFlag string
+	var noAttachFlag bool
 
 	cmd := &cobra.Command{
 		Use:   "run",
@@ -56,9 +57,13 @@ func (c *loomCLI) runCmd() *cobra.Command {
 The detached driver's own stdout/stderr go to the log the ephemeral-tree
 driver-log accessor names, never to this command's own output.
 
+--no-attach performs steps 1 through 3 and the handshake that confirms the
+driver took the run lock, then returns instead of running step 4.
+
 Example:
   lyx loom run
-  lyx loom run --parent main`,
+  lyx loom run --parent main
+  lyx loom run --no-attach`,
 		RunE: func(cmd *cobra.Command, args []string) error {
 			if clihelp.ShouldAbort(cmd.Context()) {
 				return nil
@@ -225,6 +230,10 @@ Example:
 			// reported before stdio is handed away here.
 			_ = bootstrapLock.Release()
 
+			if !mustAttach(noAttachFlag) {
+				return nil
+			}
+
 			if _, err := c.reed.Status(); err != nil {
 				clihelp.SetExit(ctx, output.Err(out, err.Error()))
 				return nil
@@ -264,6 +273,7 @@ Example:
 	}
 
 	cmd.Flags().StringVar(&parentFlag, "parent", "", "write the pair's provenance record once for a worktree created before that record existed; refused when it disagrees with an already-recorded value")
+	cmd.Flags().BoolVar(&noAttachFlag, "no-attach", false, "perform every bootstrap step and return once the driver has taken the run lock, instead of handing the terminal to the session")
 
 	return cmd
 }
diff --git a/internal/loomcli/smoke_test.go b/internal/loomcli/smoke_test.go
index 7c67156be..b701f4c16 100644
--- a/internal/loomcli/smoke_test.go
+++ b/internal/loomcli/smoke_test.go
@@ -790,7 +790,7 @@ func TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove(t *testing.T) {
 		t.Fatalf("run launcher missing after add: %v", err)
 	}
 
-	if _, err := h.Topology.Remove(h.Location, slug, false); err != nil {
+	if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
 		t.Fatalf("Remove(%s): %v", slug, err)
 	}
 	if _, err := os.Stat(runLauncherPath); !os.IsNotExist(err) {
diff --git a/internal/loomrecipe/coverage_guard_test.go b/internal/loomrecipe/coverage_guard_test.go
index 78835f9a9..cd185f488 100644
--- a/internal/loomrecipe/coverage_guard_test.go
+++ b/internal/loomrecipe/coverage_guard_test.go
@@ -3,6 +3,12 @@
 // below, checked in both directions against New's real, current output rather than against a
 // standalone literal that could drift silently. It builds a real shedrecipe.Env/ShedPaths pair and
 // calls this package's own New, rather than iterating the table alone.
+//
+// This file used to also assert that shedrecipe.Names() carried no engine unreachable by this
+// table beyond an allowlist -- a fourth, closed-coverage direction. That assertion was correct but
+// was stated in a package that structurally cannot answer it once the registry has two consumers:
+// it now lives in internal/shedrecipe's own external test package, where both consumers are
+// visible, as internal/shedrecipe/coverage_guard_test.go.
 
 package loomrecipe
 
@@ -39,27 +45,12 @@ var loomRowEngines = map[string]string{
 	loomshed.NameFinalize:           "Finalize",
 }
 
-// coverageGuardAllowedUnreachableEngines names the registry engines this task's coverage guard
-// tolerates as unreferenced by any of the seventeen built rows. Stub joins this allowlist now that
-// the last stubbed row -- Webster-Review -- is real: no loom row reaches Stub any more, and the
-// engine stays registered because internal/shedrecipe's registry is generic Shed machinery shared
-// by reference with a future product's producer list rather than loom's private property.
-// SingleLLM is the other tolerated entry: the two other "loom: real LLM producers" roadmap items
-// (manifest/roadmap.md) have not yet landed a row that reaches it.
-var coverageGuardAllowedUnreachableEngines = map[string]bool{
-	"SingleLLM": true,
-	"Stub":      true,
-}
-
-// TestCoverageGuard_EveryLoomRowHasAnEngine asserts four things about loomRowEngines against New's
-// real, current row list: every row New assembles has an entry in the table (the direction that
-// catches a row added to the recipe before its consuming task lands); every key in the table names a
-// row New actually has (the direction that keeps the table from accumulating dead entries); every
-// engine name the table maps to resolves through shedrecipe.Lookup without error; and, as a fourth
-// half, that shedrecipe.Names() carries no entry left unreachable by the table beyond
-// coverageGuardAllowedUnreachableEngines. This last half is a newly added assertion, not a
-// weakening of an earlier one -- the guard previously made no claim at all about unused registry
-// entries.
+// TestCoverageGuard_EveryLoomRowHasAnEngine asserts three things about loomRowEngines against
+// New's real, current row list: every row New assembles has an entry in the table (the direction
+// that catches a row added to the recipe before its consuming task lands); every key in the table
+// names a row New actually has (the direction that keeps the table from accumulating dead
+// entries); and every engine name the table maps to resolves through shedrecipe.Lookup without
+// error.
 func TestCoverageGuard_EveryLoomRowHasAnEngine(t *testing.T) {
 	env, paths := testEnv(t)
 	shed, err := New(env, paths)
@@ -68,7 +59,6 @@ func TestCoverageGuard_EveryLoomRowHasAnEngine(t *testing.T) {
 	}
 
 	rowNames := make(map[string]bool, len(shed.Producers))
-	usedEngines := make(map[string]bool, len(loomRowEngines))
 	for _, p := range shed.Producers {
 		rowNames[p.Name] = true
 		if _, ok := loomRowEngines[p.Name]; !ok {
@@ -83,15 +73,66 @@ func TestCoverageGuard_EveryLoomRowHasAnEngine(t *testing.T) {
 		if _, err := shedrecipe.Lookup(engineName); err != nil {
 			t.Errorf("Lookup(%q) (engine for row %q) error = %v, want nil", engineName, rowName, err)
 		}
-		usedEngines[engineName] = true
 	}
+}
 
-	for _, name := range shedrecipe.Names() {
-		if usedEngines[name] || coverageGuardAllowedUnreachableEngines[name] {
-			continue
-		}
-		t.Errorf("shedrecipe.Names() has %q, which no row reaches and which is not in coverageGuardAllowedUnreachableEngines", name)
+// TestCoverageGuard_EveryDirectionFailsOnItsOwnTrigger proves each of the three surviving
+// directions actually fails on its own trigger, rather than assuming the deletion above left them
+// intact: a row absent from loomRowEngines, a table key naming no row, and a table engine that
+// does not resolve.
+func TestCoverageGuard_EveryDirectionFailsOnItsOwnTrigger(t *testing.T) {
+	env, paths := testEnv(t)
+	shed, err := New(env, paths)
+	if err != nil {
+		t.Fatalf("New() error = %v, want nil", err)
+	}
+
+	rowNames := make(map[string]bool, len(shed.Producers))
+	for _, p := range shed.Producers {
+		rowNames[p.Name] = true
 	}
+
+	t.Run("RowAbsentFromTable", func(t *testing.T) {
+		table := make(map[string]string, len(loomRowEngines))
+		for rowName, engineName := range loomRowEngines {
+			if rowName == loomshed.NamePreflight {
+				continue
+			}
+			table[rowName] = engineName
+		}
+
+		var failures []string
+		for _, p := range shed.Producers {
+			if _, ok := table[p.Name]; !ok {
+				failures = append(failures, p.Name)
+			}
+		}
+		if len(failures) == 0 {
+			t.Error("expected a row absent from the trimmed table to be reported, found none")
+		}
+	})
+
+	t.Run("TableKeyNamesNoRow", func(t *testing.T) {
+		table := map[string]string{
+			"Not-A-Real-Row": "Preflight",
+		}
+
+		var failures []string
+		for rowName := range table {
+			if !rowNames[rowName] {
+				failures = append(failures, rowName)
+			}
+		}
+		if len(failures) == 0 {
+			t.Error("expected a table key naming no row to be reported, found none")
+		}
+	})
+
+	t.Run("TableEngineDoesNotResolve", func(t *testing.T) {
+		if _, err := shedrecipe.Lookup("Not-A-Real-Engine"); err == nil {
+			t.Error("Lookup(\"Not-A-Real-Engine\") error = nil, want non-nil")
+		}
+	})
 }
 
 // TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity asserts the rows named
diff --git a/internal/loomrecipe/names.go b/internal/loomrecipe/names.go
new file mode 100644
index 000000000..b65db799d
--- /dev/null
+++ b/internal/loomrecipe/names.go
@@ -0,0 +1,41 @@
+// names.go declares RecipeEngines, the derived engine set the cross-consumer coverage guard in
+// internal/shedrecipe trusts.
+
+package loomrecipe
+
+import (
+	"fmt"
+	"sort"
+
+	"github.com/Knatte18/loomyard/contracts/recipes"
+	"github.com/Knatte18/loomyard/internal/shedbuild"
+)
+
+// RecipeEngines parses recipes.LoomRecipe, collects each row's engine name, de-duplicates, and
+// returns the result sorted. It exists as the input to the cross-consumer coverage guard, which
+// unions every recipe consumer's engine set -- deriving the set from the recipe rather than
+// writing it down is what keeps that union honest without a second hand-maintained table alongside
+// loomRowEngines.
+//
+// RecipeEngines never returns a nil slice alongside an error: a parse failure panics, naming this
+// package, since a recipe that fails to parse is a build-time defect in an embedded file rather
+// than a runtime condition.
+func RecipeEngines() []string {
+	recipe, err := shedbuild.Parse(recipes.LoomRecipe)
+	if err != nil {
+		panic(fmt.Sprintf("loomrecipe: RecipeEngines: %v", err))
+	}
+
+	seen := make(map[string]bool, len(recipe.Producers))
+	engines := make([]string, 0, len(recipe.Producers))
+	for _, row := range recipe.Producers {
+		if seen[row.Engine] {
+			continue
+		}
+		seen[row.Engine] = true
+		engines = append(engines, row.Engine)
+	}
+
+	sort.Strings(engines)
+	return engines
+}
diff --git a/internal/loomrecipe/recipe_test.go b/internal/loomrecipe/recipe_test.go
index 427a4bfb2..d2ca28282 100644
--- a/internal/loomrecipe/recipe_test.go
+++ b/internal/loomrecipe/recipe_test.go
@@ -7,6 +7,7 @@ package loomrecipe
 
 import (
 	"reflect"
+	"sort"
 	"strings"
 	"testing"
 
@@ -177,3 +178,32 @@ func TestRecipe_SeedAndResumeRowNamesExist(t *testing.T) {
 		}
 	}
 }
+
+// TestRecipeEngines_ReportsExactlyLoomsOwnEngineSet asserts RecipeEngines() reports exactly loom's
+// own recipe's engine set, sorted and de-duplicated -- derived from wantProducerTable's own engine
+// column rather than a second hand-written literal, for the same reason its lifecyclerecipe twin
+// gets this test: a silently empty return would disable the cross-consumer coverage guard rather
+// than fail it.
+func TestRecipeEngines_ReportsExactlyLoomsOwnEngineSet(t *testing.T) {
+	seen := make(map[string]bool, len(loomRowEngines))
+	var want []string
+	for _, engine := range loomRowEngines {
+		if seen[engine] {
+			continue
+		}
+		seen[engine] = true
+		want = append(want, engine)
+	}
+	sort.Strings(want)
+
+	got := RecipeEngines()
+
+	if len(got) != len(want) {
+		t.Fatalf("RecipeEngines() = %v, want %v", got, want)
+	}
+	for i, w := range want {
+		if got[i] != w {
+			t.Errorf("RecipeEngines()[%d] = %q; want %q", i, got[i], w)
+		}
+	}
+}
diff --git a/internal/reedcli/attach.go b/internal/reedcli/attach.go
index f7592db1a..90737e8d1 100644
--- a/internal/reedcli/attach.go
+++ b/internal/reedcli/attach.go
@@ -78,6 +78,11 @@ Example:
 				return nil
 			}
 
+			// Attempted after the Status pre-flight and BEFORE attach.Run() hands the operator's
+			// stdio over: a spawn placed after the handover would fire only once the operator
+			// detaches, which is the one moment it is useless.
+			c.ensureWatchdogSpawned()
+
 			// Read the operator's own terminal size against stdout. On error (piped
 			// output, no controlling terminal) this does not report on the envelope
 			// and does not abort: AttachArgv answers a non-positive cols/rows with
diff --git a/internal/reedcli/cli.go b/internal/reedcli/cli.go
index b5eb7f652..d38a66d01 100644
--- a/internal/reedcli/cli.go
+++ b/internal/reedcli/cli.go
@@ -2,8 +2,8 @@
 // the standard io.Writer-based call contract.
 // The parent "reed" command carries a PersistentPreRunE that resolves
 // cwd -> location -> config -> geometry -> *reedengine.Engine exactly once per invocation,
-// into a receiver every verb (up.go, add.go, remove.go, status.go, resume.go, attach.go, header.go)
-// closes over, so no subcommand re-resolves geometry or config itself.
+// into a receiver every verb (up.go, add.go, remove.go, status.go, resume.go, attach.go,
+// statusline.go, watchdog.go) closes over, so no subcommand re-resolves geometry or config itself.
 // The geometry step is hubgeom.ReedGeometry: this file is where the resolved Location becomes the
 // reedengine.Geometry the engine is told, and the engine never sees the Location.
 // The resolved *lyxcwd.Location is named "location" throughout, never "layout": "layout" is a live
@@ -15,6 +15,7 @@ package reedcli
 
 import (
 	"io"
+	"testing"
 
 	"github.com/Knatte18/loomyard/internal/clihelp"
 	"github.com/Knatte18/loomyard/internal/hubgeom"
@@ -27,6 +28,19 @@ import (
 // reedCLI carries the resolved *reedengine.Engine for each reed verb; the zero value is invalid until PersistentPreRunE populates eng.
 type reedCLI struct {
 	eng *reedengine.Engine
+
+	// hubPath is the hub path PersistentPreRunE stored off the resolved *lyxcwd.Location — the hub
+	// path alone, deliberately not the whole Location, since ensureWatchdogSpawned needs nothing
+	// else from it and a stored Location would invite other code to re-read geometry the engine was
+	// already told.
+	hubPath string
+
+	// suppressWatchdogSpawn, when true, makes ensureWatchdogSpawned a no-op. It is initialised from
+	// testing.Testing() in Command(): re-exec'ing os.Executable() from a test binary runs the whole
+	// suite recursively, the same hazard and the same shape as the Engine.suppressHeaderLaunch field
+	// batch 3 deleted, relocated to the layer that now owns the spawn. An in-package test flips it
+	// back off to drive the real spawn path.
+	suppressWatchdogSpawn bool
 }
 
 // Command returns the cobra command tree for the reed module.
@@ -38,7 +52,7 @@ type reedCLI struct {
 // Every verb card (22-27) creates its own (c *reedCLI) xCmd() builder and registers it here via
 // parent.AddCommand — this card registers no subcommands itself.
 func Command() *cobra.Command {
-	c := &reedCLI{}
+	c := &reedCLI{suppressWatchdogSpawn: testing.Testing()}
 
 	parent := &cobra.Command{
 		Use:   "reed",
@@ -57,9 +71,12 @@ rather than booting substrate it cannot reach.`,
 		RunE: clihelp.GroupRunE,
 		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
 			// Guard: when the reed group command itself is invoked (bare listing or
-			// unknown-subcommand error path via GroupRunE), skip cwd/location/config
-			// resolution so that neither path requires a git repository to be present.
-			if cmd.Name() == "reed" {
+			// unknown-subcommand error path via GroupRunE), or when the watchdog daemon is
+			// invoked, skip cwd/location/config resolution entirely. The daemon is told its hub
+			// path on its own flags and must never be reached through c.eng — letting it run the
+			// normal pre-run would make it refuse to start outside a worktree and hold a geometry
+			// it must not use.
+			if cmd.Name() == "reed" || cmd.Name() == "watchdog" {
 				return nil
 			}
 
@@ -95,11 +112,12 @@ rather than booting substrate it cannot reach.`,
 
 			reedGeom := hubgeom.ReedGeometry(location)
 			c.eng = reedengine.New(cfg, reedGeom)
+			c.hubPath = location.HubPath
 			return nil
 		},
 	}
 
-	parent.AddCommand(c.upCmd(), c.downCmd(), c.addCmd(), c.removeCmd(), c.statusCmd(), c.resumeCmd(), c.attachCmd(), c.headerCmd())
+	parent.AddCommand(c.upCmd(), c.downCmd(), c.addCmd(), c.removeCmd(), c.statusCmd(), c.resumeCmd(), c.attachCmd(), c.statuslineCmd(), c.watchdogCmd())
 
 	return parent
 }
diff --git a/internal/reedcli/cli_test.go b/internal/reedcli/cli_test.go
index 20c043127..64b7ae4e3 100644
--- a/internal/reedcli/cli_test.go
+++ b/internal/reedcli/cli_test.go
@@ -14,7 +14,7 @@ import (
 	"testing"
 )
 
-// TestRunCLI_NoArgs verifies that "lyx reed" with no subcommand lists all seven registered verbs
+// TestRunCLI_NoArgs verifies that "lyx reed" with no subcommand lists all nine registered verbs
 // and exits 0.
 func TestRunCLI_NoArgs(t *testing.T) {
 	t.Parallel()
@@ -27,7 +27,7 @@ func TestRunCLI_NoArgs(t *testing.T) {
 	}
 
 	got := out.String()
-	wantSubs := []string{"up", "down", "add", "remove", "status", "resume", "attach"}
+	wantSubs := []string{"up", "down", "add", "remove", "status", "resume", "attach", "statusline", "watchdog"}
 	for _, sub := range wantSubs {
 		if !strings.Contains(got, sub) {
 			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", sub, got)
@@ -35,6 +35,79 @@ func TestRunCLI_NoArgs(t *testing.T) {
 	}
 }
 
+// TestCommand_EveryVerbHasNonEmptyShort asserts every registered reed subcommand — statusline and
+// watchdog included — carries a non-empty Short, per the CLI/Cobra Invariant.
+func TestCommand_EveryVerbHasNonEmptyShort(t *testing.T) {
+	t.Parallel()
+
+	parent := Command()
+	for _, sub := range parent.Commands() {
+		if sub.Short == "" {
+			t.Errorf("subcommand %q has an empty Short; want a non-empty short description", sub.Name())
+		}
+	}
+}
+
+// TestRunCLI_Watchdog_SkipsLocationResolution verifies that "watchdog" takes the
+// PersistentPreRunE's early return: invoked from a directory that is not a git repository, it must
+// not fail with lyxcwd.Resolve's not-a-git-repository error — the verb must run with no git
+// repository present at all. Its own RunE then reports the missing --hub-path/--tmux flags on the
+// envelope instead, which is the observable proof that PersistentPreRunE returned nil (skipping
+// c.eng population) rather than aborting into the git-repo error.
+func TestRunCLI_Watchdog_SkipsLocationResolution(t *testing.T) {
+	t.Chdir(t.TempDir())
+
+	var out bytes.Buffer
+	exitCode := RunCLI(&out, []string{"watchdog"})
+
+	if exitCode == 0 {
+		t.Fatalf("RunCLI(watchdog) with no --hub-path = 0; want non-zero (missing required flag)")
+	}
+
+	var env map[string]any
+	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
+		t.Fatalf("RunCLI(watchdog) output is not valid JSON: %v; got: %q", err, out.String())
+	}
+	errMsg, _ := env["error"].(string)
+	if errMsg == "not a git repository" {
+		t.Errorf("RunCLI(watchdog) error = %q; want the --hub-path validation error, not lyxcwd.Resolve's not-a-git-repository error", errMsg)
+	}
+	if !strings.Contains(errMsg, "--hub-path") {
+		t.Errorf("RunCLI(watchdog) error = %q; want it to name --hub-path", errMsg)
+	}
+}
+
+// TestRunCLI_Watchdog_RefusesAbsentAndRelativeHubPath verifies that watchdog refuses both an
+// absent and a relative --hub-path on the envelope, before it ever blocks.
+func TestRunCLI_Watchdog_RefusesAbsentAndRelativeHubPath(t *testing.T) {
+	tests := []struct {
+		name string
+		args []string
+	}{
+		{name: "absent", args: []string{"watchdog", "--tmux", "/usr/bin/tmux"}},
+		{name: "relative", args: []string{"watchdog", "--hub-path", "relative/path", "--tmux", "/usr/bin/tmux"}},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			var out bytes.Buffer
+			exitCode := RunCLI(&out, tt.args)
+
+			if exitCode == 0 {
+				t.Fatalf("RunCLI(%v) = 0; want non-zero", tt.args)
+			}
+			var env map[string]any
+			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
+				t.Fatalf("RunCLI(%v) output is not valid JSON: %v; got: %q", tt.args, err, out.String())
+			}
+			errMsg, _ := env["error"].(string)
+			if !strings.Contains(errMsg, "--hub-path") {
+				t.Errorf("RunCLI(%v) error = %q; want it to name --hub-path", tt.args, errMsg)
+			}
+		})
+	}
+}
+
 // TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1 and emits a JSON error
 // envelope.
 func TestRunCLI_UnknownSubcommand(t *testing.T) {
diff --git a/internal/reedcli/header.go b/internal/reedcli/header.go
deleted file mode 100644
index 7679b977c..000000000
--- a/internal/reedcli/header.go
+++ /dev/null
@@ -1,150 +0,0 @@
-// header.go implements the `header` reed verb: it renders the header pane's text via the engine's
-// tokenvocab-backed pipeline.
-// The default mode returns the rendered text through the normal JSON envelope;
-// --blocking prints the text then blocks forever, the one envelope-exempt tail this command has —
-// the header pane boots "lyx reed header --blocking" as its keepalive, running as the pane's own
-// command rather than being typed into a shell that could echo or leave other noise behind it.
-// Both modes carry clihelp.SkipStencilSeedAnnotation, declining cmd/lyx's root pre-run stencil-seed
-// pass: this is deliberate rather than a --blocking-only gate, because a cobra annotation is
-// per-command and neither mode reads a stencil. Declining is what keeps the keepalive's stderr — and
-// therefore the header pane's scrollback — free of stencilstore warnings, and the hub free of a
-// preview command's git commits.
-
-package reedcli
-
-import (
-	"context"
-	"fmt"
-	"io"
-	"strings"
-	"time"
-
-	"github.com/Knatte18/loomyard/internal/clihelp"
-	"github.com/Knatte18/loomyard/internal/logger"
-	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/reedengine"
-	"github.com/spf13/cobra"
-)
-
-// blockForever parks the keepalive tail indefinitely. It sleeps in a loop rather than using select {} to avoid Go's deadlock detector.
-func blockForever() {
-	for {
-		time.Sleep(time.Hour)
-	}
-}
-
-// headerWatch is the resize self-heal loop the blocking tail enters after painting
-// the header text. A package var so header_test.go can substitute a fake that
-// returns, and assert the tail still reaches headerPark.
-var headerWatch = func(ctx context.Context, eng *reedengine.Engine) error { return eng.Watch(ctx) }
-
-// headerPark is the keepalive park the blocking tail ends on, unconditionally.
-var headerPark = blockForever
-
-// headerBlockingPayload returns the exact bytes the --blocking mode writes to the pane before it
-// blocks forever: an ED 2 + ED 3 + cursor-home escape sequence, followed by text with its trailing
-// carriage returns and newlines trimmed. It is split out as a pure helper, the same
-// composition-split-from-side-effecting-call-site shape internal/reedengine/headerpane.go uses,
-// so the byte sequence stays assertable without driving the --blocking path itself, which blocks
-// forever and never returns to a test.
-//
-// ED 2 (\x1b[2J) clears only the visible screen and does not touch the terminal's scrollback
-// buffer, which is precisely why shell/log noise written before this command runs could survive
-// where an operator eventually saw it. ED 3 (\x1b[3J) is a backstop: it clears the scrollback too,
-// guaranteeing the pane is clean at the moment the header renders regardless of what any future
-// code path, shell, or terminal wrote before it. It is not the pin for any individual source fix —
-// it is defence in depth that stays green even if one of those fixes regresses.
-func headerBlockingPayload(text string) string {
-	return "\x1b[2J\x1b[3J\x1b[H" + strings.TrimRight(text, "\r\n")
-}
-
-// headerCmd builds the `header` subcommand: calls c.eng.HeaderText() and either returns it via the JSON envelope or prints and blocks forever.
-func (c *reedCLI) headerCmd() *cobra.Command {
-	var blocking bool
-
-	cmd := &cobra.Command{
-		Use:   "header",
-		Short: "render the operator console pane's header text",
-		Long: `header renders the header-pane text over this hub's configured template
-(or the embedded default), the same tokenvocab pipeline
-Engine.ValidateHeader checks eagerly at boot.
-
-Default mode returns the rendered text through the normal JSON envelope —
-a plain, smoke-testable command. --blocking instead clears the pane's
-screen and scrollback, prints the rendered text to stdout, and then
-blocks forever; this is the header pane's own keepalive tail, run
-directly as the pane's command rather than typed into a shell that
-would survive it, and the one part of this command exempt from the
-JSON envelope (everything fallible still runs pre-flight, on the
-envelope).
-The blocking pane additionally runs reed's resize self-heal watch loop,
-which re-applies the planned layout after the terminal window is
-resized, and is turned off with "watchdog: off" in reed.yaml followed
-by "lyx reed down" + "up".
-
-The live header pane renders its text once, at pane launch: after editing
-header.template in reed.yaml, this verb previews the new rendering
-immediately, but the running pane keeps its old text until the header is
-next rebuilt (a server restart, a dead-header heal, or "lyx reed down" +
-"up") — an "up" that finds the header alive deliberately leaves it as is.
-
-Example:
-  lyx reed header
-  lyx reed header --blocking`,
-		Annotations: map[string]string{
-			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
-		},
-		RunE: func(cmd *cobra.Command, args []string) error {
-			if clihelp.ShouldAbort(cmd.Context()) {
-				return nil
-			}
-			out := cmd.OutOrStdout()
-
-			text, err := c.eng.HeaderText()
-			if err != nil {
-				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
-				return nil
-			}
-
-			if blocking {
-				// Display the rendered text once, then hold the pane open forever.
-				fmt.Fprint(out, headerBlockingPayload(text))
-
-				// The header pane's stdout/stderr is its visible screen: internal/logger's stderr half
-				// defaults to slog.LevelWarn, and the watch loop reaches already-shipped Warn call sites
-				// (liveBoxLocked on a failed or malformed window-size query, pinGeometryOptionsLocked on a
-				// failed pin), so without this rebind the first degraded tmux round trip paints a slog
-				// line over the operator console. Only the stderr half is discarded -- logger.SetOutput
-				// rebinds that half alone, and the durable handler is enabled unconditionally at Info and
-				// above, so nothing is lost for diagnosis, it just stops being drawn.
-				logger.SetOutput(io.Discard)
-
-				// A non-nil return is logged only: never output.Err, never fmt.Fprint, never anything
-				// written to stdout or stderr, because the pane's stdio is its screen.
-				if err := headerWatch(cmd.Context(), c.eng); err != nil {
-					logger.Warn("reed: header pane watch loop returned", "err", err)
-				}
-
-				// Deliberate redundancy, not dead code: Watch never returns while the pane must live, and
-				// this guarantees that no future edit to Watch can make RunE fall through and kill the
-				// keepalive pane -- the one failure this design must never permit.
-				headerPark()
-
-				// headerPark() itself must never return (it blocks forever), but if it ever does --
-				// e.g. under test via a substitutable stub -- this return prevents falling through to
-				// the unconditional output.Ok write below, which would leak a JSON envelope onto the
-				// pane's own screen in violation of the "pane's stdio is its screen" invariant above.
-				return nil
-			}
-
-			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
-				"text": text,
-			}))
-			return nil
-		},
-	}
-
-	cmd.Flags().BoolVar(&blocking, "blocking", false, "print the rendered header text then block forever (the pane keepalive)")
-
-	return cmd
-}
diff --git a/internal/reedcli/header_test.go b/internal/reedcli/header_test.go
deleted file mode 100644
index 14529b772..000000000
--- a/internal/reedcli/header_test.go
+++ /dev/null
@@ -1,212 +0,0 @@
-// header_test.go covers the `header` verb's pure command construction: Use, Short, and the
-// --blocking flag registration, plus the blocking tail's keepalive-survival contract, exercised
-// with headerWatch and headerPark both stubbed via package vars, and headerBlockingPayload's
-// clear-sequence bytes. It never runs RunE/PreRunE and never invokes the full --blocking path in
-// most tests, since that path blocks forever by design — the payload and contract are pinned
-// instead by driving the pure helpers directly and stubbing the blocking-tail components.
-// It also never drives the enveloped default through RunCLI: that reaches reed's PersistentPreRunE
-// and therefore lyxcwd.Resolve, which spawns "git rev-parse", banned in the untagged suite by the
-// Test Tier Purity Invariant.
-// The enveloped default's end-to-end PreRunE -> HeaderText round trip is covered by the reed smoke
-// suite (batch 4), not here.
-
-package reedcli
-
-import (
-	"bytes"
-	"context"
-	"os"
-	"strings"
-	"testing"
-
-	"github.com/Knatte18/loomyard/internal/logger"
-	"github.com/Knatte18/loomyard/internal/reedengine"
-)
-
-func TestHeaderCmd_UseAndShort(t *testing.T) {
-	c := &reedCLI{}
-	cmd := c.headerCmd()
-
-	if cmd.Use != "header" {
-		t.Errorf("headerCmd().Use = %q; want %q", cmd.Use, "header")
-	}
-	if cmd.Short == "" {
-		t.Error("headerCmd().Short is empty; want a non-empty short description")
-	}
-}
-
-func TestHeaderCmd_BlockingFlagRegistered(t *testing.T) {
-	c := &reedCLI{}
-	cmd := c.headerCmd()
-
-	flag := cmd.Flags().Lookup("blocking")
-	if flag == nil {
-		t.Fatal("headerCmd() did not register a --blocking flag")
-	}
-	if flag.Value.Type() != "bool" {
-		t.Errorf("--blocking flag type = %q; want %q", flag.Value.Type(), "bool")
-	}
-	if flag.DefValue != "false" {
-		t.Errorf("--blocking flag default = %q; want %q", flag.DefValue, "false")
-	}
-}
-
-// newBlockingTestCLI builds a reedCLI whose Engine is real enough to run headerCmd's RunE: HeaderText
-// dereferences e.cfg unconditionally before the --blocking branch is ever reached, so the bare
-// &reedCLI{} shape the two tests above use would panic here. An empty Config.Header.Template falls
-// back to the embedded default template, and RepoName/HubPath are the only two Geometry fields
-// tokenvocab.Ctx consumes, so this renders cleanly with no filesystem or process I/O.
-func newBlockingTestCLI(t *testing.T) *reedCLI {
-	t.Helper()
-	return &reedCLI{eng: reedengine.New(reedengine.Config{}, reedengine.Geometry{RepoName: "test-repo", HubPath: t.TempDir()})}
-}
-
-// stubHeaderTail substitutes headerWatch and headerPark with fakes recording their own call counts,
-// restoring both package vars via t.Cleanup, and reports pointers to the two counters.
-func stubHeaderTail(t *testing.T, watchErr error) (watchCalls, parkCalls *int) {
-	t.Helper()
-	watchCalls = new(int)
-	parkCalls = new(int)
-
-	origWatch, origPark := headerWatch, headerPark
-	headerWatch = func(ctx context.Context, eng *reedengine.Engine) error {
-		*watchCalls++
-		return watchErr
-	}
-	headerPark = func() {
-		*parkCalls++
-	}
-	t.Cleanup(func() {
-		headerWatch, headerPark = origWatch, origPark
-	})
-
-	return watchCalls, parkCalls
-}
-
-func TestHeaderCmd_BlockingTailParksAfterNilWatch(t *testing.T) {
-	watchCalls, parkCalls := stubHeaderTail(t, nil)
-	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
-
-	c := newBlockingTestCLI(t)
-	cmd := c.headerCmd()
-	buf := &bytes.Buffer{}
-	cmd.SetOut(buf)
-	cmd.SetArgs([]string{"--blocking"})
-
-	if err := cmd.Execute(); err != nil {
-		t.Fatalf("cmd.Execute() = %v; want nil", err)
-	}
-	if *watchCalls != 1 {
-		t.Errorf("headerWatch call count = %d; want 1", *watchCalls)
-	}
-	if *parkCalls != 1 {
-		t.Errorf("headerPark call count = %d; want 1", *parkCalls)
-	}
-}
-
-func TestHeaderCmd_BlockingTailParksAfterWatchError(t *testing.T) {
-	watchCalls, parkCalls := stubHeaderTail(t, errWatchFailedForTest)
-	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
-
-	c := newBlockingTestCLI(t)
-	cmd := c.headerCmd()
-	buf := &bytes.Buffer{}
-	cmd.SetOut(buf)
-	cmd.SetArgs([]string{"--blocking"})
-
-	if err := cmd.Execute(); err != nil {
-		t.Fatalf("cmd.Execute() = %v; want nil (a non-nil headerWatch error must never propagate out of RunE)", err)
-	}
-	if *watchCalls != 1 {
-		t.Errorf("headerWatch call count = %d; want 1", *watchCalls)
-	}
-	if *parkCalls != 1 {
-		t.Errorf("headerPark call count = %d; want 1", *parkCalls)
-	}
-
-	// This is the keepalive-survival assertion: an obvious implementation propagates the error and
-	// kills the pane, or falls through to the unconditional output.Ok write, either of which would
-	// append a JSON envelope (output.Ok emits `"ok":true` plus the caller's own "text" key -- never
-	// "status") to buf after the rendered header text. Check for the envelope's actual shape rather
-	// than a "status" key it never uses.
-	out := buf.String()
-	if strings.Contains(out, `"ok":true`) || strings.Contains(out, `"error"`) {
-		t.Errorf("blocking output = %q; want only the rendered header text, no JSON envelope or error text", out)
-	}
-}
-
-func TestHeaderCmd_NonBlockingModeUnaffected(t *testing.T) {
-	watchCalls, parkCalls := stubHeaderTail(t, nil)
-
-	c := newBlockingTestCLI(t)
-	cmd := c.headerCmd()
-	buf := &bytes.Buffer{}
-	cmd.SetOut(buf)
-	cmd.SetArgs([]string{})
-
-	if err := cmd.Execute(); err != nil {
-		t.Fatalf("cmd.Execute() = %v; want nil", err)
-	}
-	if *watchCalls != 0 {
-		t.Errorf("headerWatch call count = %d; want 0 in non-blocking mode", *watchCalls)
-	}
-	if *parkCalls != 0 {
-		t.Errorf("headerPark call count = %d; want 0 in non-blocking mode", *parkCalls)
-	}
-	if !strings.Contains(buf.String(), `"text"`) {
-		t.Errorf("non-blocking output = %q; want the JSON envelope with a \"text\" field", buf.String())
-	}
-}
-
-func TestHeaderCmd_LongMentionsWatchdog(t *testing.T) {
-	c := &reedCLI{}
-	cmd := c.headerCmd()
-
-	if !strings.Contains(cmd.Long, "watchdog") {
-		t.Errorf("headerCmd().Long does not mention %q; want the self-heal watch loop documented there", "watchdog")
-	}
-}
-
-// errWatchFailedForTest is a sentinel error used to exercise headerWatch's error path without
-// depending on any real reedengine failure mode.
-var errWatchFailedForTest = errHeaderWatchStub{}
-
-// errHeaderWatchStub is a trivial error type local to this test file.
-type errHeaderWatchStub struct{}
-
-// Error implements the error interface with a fixed, test-only message.
-func (errHeaderWatchStub) Error() string { return "stub watch failure for test" }
-
-func TestHeaderBlockingPayload(t *testing.T) {
-	const clearSeq = "\x1b[2J\x1b[3J\x1b[H"
-
-	tests := []struct {
-		name string
-		text string
-		want string
-	}{
-		{
-			name: "TrailingCRLFTrimmed",
-			text: "hub: /some/path\r\n",
-			want: clearSeq + "hub: /some/path",
-		},
-		{
-			name: "NoTrailingNewlineUnchanged",
-			text: "hub: /some/path",
-			want: clearSeq + "hub: /some/path",
-		},
-		{
-			name: "InteriorNewlinesPreserved",
-			text: "line one\nline two\n\n",
-			want: clearSeq + "line one\nline two",
-		},
-	}
-	for _, tt := range tests {
-		t.Run(tt.name, func(t *testing.T) {
-			got := headerBlockingPayload(tt.text)
-			if got != tt.want {
-				t.Errorf("headerBlockingPayload(%q) = %q; want %q", tt.text, got, tt.want)
-			}
-		})
-	}
-}
diff --git a/internal/reedcli/resume.go b/internal/reedcli/resume.go
index e78b1a300..81e5feb4e 100644
--- a/internal/reedcli/resume.go
+++ b/internal/reedcli/resume.go
@@ -35,6 +35,10 @@ Example:
 				return nil
 			}
 
+			// Attempted after Resume returns without error, before the envelope write: same
+			// reasoning as up's own spawn attempt.
+			c.ensureWatchdogSpawned()
+
 			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
 				"session": result.Session,
 				"resumed": result.Resumed,
diff --git a/internal/reedcli/smoke_dotfill_test.go b/internal/reedcli/smoke_dotfill_test.go
index ec7827c9f..6071cb051 100644
--- a/internal/reedcli/smoke_dotfill_test.go
+++ b/internal/reedcli/smoke_dotfill_test.go
@@ -120,7 +120,7 @@ func pollPaneDotRunClears(t *testing.T, tmuxPath, socket, target string, timeout
 //
 // Both pollers in this file sample at 100 ms rather than reusing pollPaneContains: pollPaneContains
 // takes a plain substring, and legitimate harness-pane content contains dots (file paths, ellipses, the
-// header template), so reusing it would ship a test that proves nothing. Its 500 ms cadence is also a
+// rendered status-line text), so reusing it would ship a test that proves nothing. Its 500 ms cadence is also a
 // quarter of the window the artifact would occupy under watchdog: on, too coarse to characterise it.
 func paneStaysCleanOfDotRun(t *testing.T, tmuxPath, socket, target string, window time.Duration) bool {
 	t.Helper()
@@ -147,8 +147,8 @@ type dotFillHarness struct {
 	reedSession   string
 }
 
-// newDotFillHarness boots a dot-fill scenario's full fixture: a reed session carrying a header pane
-// plus two strand panes, and a private harness tmux server sized cols x rows to host the attach
+// newDotFillHarness boots a dot-fill scenario's full fixture: a reed session carrying Selvage plus
+// two strand panes, and a private harness tmux server sized cols x rows to host the attach
 // client(s) that observe it.
 func newDotFillHarness(t *testing.T, cols, rows int) *dotFillHarness {
 	t.Helper()
@@ -178,8 +178,8 @@ func newDotFillHarness(t *testing.T, cols, rows int) *dotFillHarness {
 	}
 
 	// Two strands, not one, is a fidelity choice rather than a pin-count requirement:
-	// render.FixedHeightPins emits the header pin whenever a header is placed and the layout is not
-	// the sole-header case, so a single strand already yields a non-empty pin set. What two strands
+	// render.FixedHeightPins emits the Selvage pin whenever Selvage is placed and the layout is not
+	// the sole-Selvage case, so a single strand already yields a non-empty pin set. What two strands
 	// buy is a taller stack for the resize round-robin to distribute rows across, so the
 	// mid-relayout region is larger and the artifact reproduces more reliably, and it keeps the
 	// scenario clear of AttachArgv's len(live) < 2 guard boundary rather than sitting exactly on it.
diff --git a/internal/reedcli/smoke_header_keepalive_test.go b/internal/reedcli/smoke_header_keepalive_test.go
deleted file mode 100644
index 930ea15a9..000000000
--- a/internal/reedcli/smoke_header_keepalive_test.go
+++ /dev/null
@@ -1,62 +0,0 @@
-//go:build smoke
-
-// smoke_header_keepalive_test.go pins the header pane's keepalive mechanism:
-// blockForever must park the process indefinitely rather than dying. The
-// pre-fix implementation used `select {}`, which — with no other goroutines
-// in the process — trips Go's runtime deadlock detector and kills the
-// keepalive instantly with "fatal error: all goroutines are asleep -
-// deadlock!", crashing every header pane right after it printed its text
-// (observed live). The test re-executes its own test binary as a child that
-// calls blockForever and asserts the child neither exits nor prints a fatal
-// runtime error within the observation window; the old code dies within
-// milliseconds, so a regression fails fast while the fixed code passes
-// deterministically. Tagged smoke because untagged reedcli tests must not
-// spawn processes (Test Tier Purity Invariant).
-
-package reedcli
-
-import (
-	"bytes"
-	"os"
-	"os/exec"
-	"strings"
-	"testing"
-	"time"
-)
-
-// TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock checks that blockForever doesn't deadlock.
-func TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock(t *testing.T) {
-	const childEnv = "REED_HEADER_KEEPALIVE_CHILD"
-
-	if os.Getenv(childEnv) == "1" {
-		blockForever()
-	}
-
-	var output bytes.Buffer
-	child := exec.Command(os.Args[0], "-test.run", "TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock")
-	child.Stdout = &output
-	child.Stderr = &output
-	child.Env = append(os.Environ(), childEnv+"=1")
-	if err := child.Start(); err != nil {
-		t.Fatalf("start keepalive child process: %v", err)
-	}
-
-	exited := make(chan error, 1)
-	go func() { exited <- child.Wait() }()
-
-	select {
-	case err := <-exited:
-		t.Fatalf("keepalive child exited (%v); want it parked forever. Output:\n%s", err, output.String())
-	case <-time.After(3 * time.Second):
-		if strings.Contains(output.String(), "fatal error") {
-			_ = child.Process.Kill()
-			<-exited
-			t.Fatalf("keepalive child printed a runtime fatal error while still tracked as running:\n%s", output.String())
-		}
-	}
-
-	if err := child.Process.Kill(); err != nil {
-		t.Fatalf("kill parked keepalive child: %v", err)
-	}
-	<-exited
-}
diff --git a/internal/reedcli/smoke_headerscrollback_test.go b/internal/reedcli/smoke_headerscrollback_test.go
deleted file mode 100644
index f00837e97..000000000
--- a/internal/reedcli/smoke_headerscrollback_test.go
+++ /dev/null
@@ -1,189 +0,0 @@
-//go:build smoke
-
-// smoke_headerscrollback_test.go holds the two scrollback assertions this batch adds, both built on
-// capturePaneScrollback.
-// TestSmokeHeaderPayloadClearsPaneScrollback is the direct proof that the ED 3 backstop actually
-// clears a real multiplexer's scrollback — the one claim the composite test below can never show,
-// since it goes green either way once the source fixes land.
-// TestSmokeHeaderPaneScrollbackIsClean is the composite backstop B: it pins the end-to-end outcome
-// across boot, resume, and heal, and pins none of the individual source fixes — P1, P2, and P3 are
-// the pins for those, and they live in internal/reedengine/lifecycle_test.go,
-// internal/reedcli/smoke_headerseed_test.go, and internal/reedcli/header_test.go respectively.
-
-package reedcli
-
-import (
-	"bytes"
-	"fmt"
-	"os"
-	"os/exec"
-	"path/filepath"
-	"strings"
-	"testing"
-	"time"
-
-	"github.com/Knatte18/loomyard/contracts/stencils"
-	"github.com/Knatte18/loomyard/internal/fabricengine"
-	"github.com/Knatte18/loomyard/internal/hubforge"
-	"github.com/Knatte18/loomyard/internal/reedengine"
-	"github.com/Knatte18/loomyard/internal/stencilstore"
-)
-
-// TestSmokeHeaderPayloadClearsPaneScrollback proves ED 3 takes effect against a real multiplexer
-// rather than being a silent no-op: it fills a pane's scrollback with real junk lines, then emits
-// headerBlockingPayload's exact bytes into that same pane, and asserts the resulting scrollback
-// holds the header line and nothing else.
-// On a Windows/psmux host this claim is asserted, not verified — the discussion records both, but
-// this worktree is Linux and cannot execute a Windows run.
-func TestSmokeHeaderPayloadClearsPaneScrollback(t *testing.T) {
-	tmuxPath := tmuxBinaryPath(t)
-
-	tempDir := t.TempDir()
-	headerLine := "hub: " + tempDir
-
-	var payload strings.Builder
-	for i := 0; i < 50; i++ {
-		fmt.Fprintf(&payload, "smoke-junk-line-%03d\n", i)
-	}
-	payload.WriteString(headerBlockingPayload(headerLine))
-
-	payloadFile := filepath.Join(tempDir, "payload.txt")
-	if err := os.WriteFile(payloadFile, []byte(payload.String()), 0o644); err != nil {
-		t.Fatalf("write payload file %s: %v", payloadFile, err)
-	}
-
-	socket := fmt.Sprintf("lyx-headerscrollback-harness-%d", os.Getpid())
-	sessionCmd := fmt.Sprintf("cat %s; sleep 300", payloadFile)
-	if err := exec.Command(tmuxPath, "-L", socket, "new-session", "-d", "-s", "h",
-		"sh", "-c", sessionCmd).Run(); err != nil {
-		t.Fatalf("boot harness server: %v", err)
-	}
-	t.Cleanup(func() {
-		reapHarnessServer(t, tmuxPath, socket)
-	})
-
-	var capture string
-	deadline := time.Now().Add(20 * time.Second)
-	for {
-		capture = capturePaneScrollback(t, tmuxPath, socket, "h")
-		if strings.Contains(capture, headerLine) {
-			break
-		}
-		if time.Now().After(deadline) {
-			t.Fatalf("pane never showed %q within 20s; last scrollback:\n%s", headerLine, capture)
-		}
-		time.Sleep(200 * time.Millisecond)
-	}
-
-	if !strings.Contains(capture, headerLine) {
-		t.Errorf("scrollback missing header line %q; full capture:\n%s", headerLine, capture)
-	}
-	for i := 0; i < 50; i++ {
-		junk := fmt.Sprintf("smoke-junk-line-%03d", i)
-		if strings.Contains(capture, junk) {
-			t.Errorf("scrollback still contains junk line %q that ED 3 should have cleared; full capture:\n%s", junk, capture)
-		}
-	}
-}
-
-// TestSmokeHeaderPaneScrollbackIsClean is the composite backstop B: it pins the end-to-end
-// outcome — the live header pane's scrollback holds the rendered header line and no other
-// non-empty line — across boot, resume, and heal.
-// It pins no individual source fix: ED 3 runs after everything else and would keep this test green
-// even if a source fix regressed, which is exactly why it is landed only alongside the direct proof
-// above and the three per-mechanism pins (P1, P2, P3) elsewhere in this package and reedengine.
-func TestSmokeHeaderPaneScrollbackIsClean(t *testing.T) {
-	tmuxPath := tmuxBinaryPath(t)
-	lyxExe := buildLyxBinaryWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")
-
-	h := hubforge.NewHub(t, ".")
-	deferHubRelease(t, h.PrimeWorktree())
-	t.Chdir(h.PrimeWorktree())
-	t.Cleanup(func() {
-		var buf bytes.Buffer
-		RunCLI(&buf, []string{"down"})
-	})
-
-	// Plant the same stale-but-untouched board stencil TestSmokeHeaderDeclinesStencilSeedPass
-	// plants, so the arrangement that made the noise non-deterministic in the field is forced
-	// rather than hoped for.
-	registry := stencils.Registry()
-	names := registry.Names()
-	if len(names) == 0 {
-		t.Fatalf("stencils.Registry().Names() returned no names")
-	}
-	name := names[0]
-	shipped, known := registry.Default(name)
-	if !known {
-		t.Fatalf("registry has no default for its own first name %q", name)
-	}
-	driftedBody := append(append([]byte{}, shipped...), []byte("\nsmoke-drift-line\n")...)
-	boardPath := stencilstore.Path(fabricengine.StencilsDir(h.Path), name)
-	if err := os.MkdirAll(filepath.Dir(boardPath), 0o755); err != nil {
-		t.Fatalf("create board stencil parent dir: %v", err)
-	}
-	stamped := stencilstore.ApplyStamp(driftedBody, stencilstore.BodyHash(driftedBody))
-	if err := os.WriteFile(boardPath, stamped, 0o644); err != nil {
-		t.Fatalf("write board stencil %s: %v", boardPath, err)
-	}
-
-	headerLine := "hub: " + h.Location.HubPath
-	assertScrollbackClean := func(when, socket, paneID string) {
-		t.Helper()
-		capture := capturePaneScrollback(t, tmuxPath, socket, paneID)
-		if !strings.Contains(capture, headerLine) {
-			t.Errorf("%s: header pane scrollback missing %q; full capture:\n%s", when, headerLine, capture)
-		}
-		for _, line := range strings.Split(capture, "\n") {
-			line = strings.TrimRight(line, "\r")
-			if strings.TrimSpace(line) == "" || strings.Contains(line, headerLine) {
-				continue
-			}
-			t.Errorf("%s: header pane scrollback carries an unexpected non-empty line %q; full capture:\n%s", when, line, capture)
-		}
-	}
-	pollHeaderLine := func(socket, paneID string) {
-		t.Helper()
-		pollPaneContains(t, tmuxPath, socket, paneID, headerLine, 20*time.Second)
-	}
-
-	// Boot.
-	upCmd := exec.Command(lyxExe, "reed", "up")
-	upCmd.Dir = h.PrimeWorktree()
-	if out, err := upCmd.CombinedOutput(); err != nil {
-		t.Fatalf("built-binary up: %v\n%s", err, out)
-	}
-	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
-	}
-	socket, _ := socketAndSession(t)
-	pollHeaderLine(socket, st.HeaderPaneID)
-	assertScrollbackClean("after up", socket, st.HeaderPaneID)
-
-	// Resume: already-live header pane must be left untouched, and its scrollback must stay clean.
-	resumeCmd := exec.Command(lyxExe, "reed", "resume")
-	resumeCmd.Dir = h.PrimeWorktree()
-	if out, err := resumeCmd.CombinedOutput(); err != nil {
-		t.Fatalf("built-binary resume: %v\n%s", err, out)
-	}
-	assertScrollbackClean("after resume", socket, st.HeaderPaneID)
-
-	// Heal: kill the header pane directly through tmux, then re-run up, which is the retried-split
-	// heal path — the one most likely to regress, since it re-runs the same launch code from a
-	// different entry point.
-	if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", st.HeaderPaneID).Run(); err != nil {
-		t.Fatalf("kill-pane %s: %v", st.HeaderPaneID, err)
-	}
-	healCmd := exec.Command(lyxExe, "reed", "up")
-	healCmd.Dir = h.PrimeWorktree()
-	if out, err := healCmd.CombinedOutput(); err != nil {
-		t.Fatalf("built-binary up (heal): %v\n%s", err, out)
-	}
-	healedSt, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
-	if err != nil || healedSt == nil || healedSt.HeaderPaneID == "" {
-		t.Fatalf("LoadState after heal up = (%+v, %v), want a fresh persisted HeaderPaneID", healedSt, err)
-	}
-	pollHeaderLine(socket, healedSt.HeaderPaneID)
-	assertScrollbackClean("after heal", socket, healedSt.HeaderPaneID)
-}
diff --git a/internal/reedcli/smoke_lifecycle_test.go b/internal/reedcli/smoke_lifecycle_test.go
index 693c81207..97e3a5472 100644
--- a/internal/reedcli/smoke_lifecycle_test.go
+++ b/internal/reedcli/smoke_lifecycle_test.go
@@ -88,7 +88,7 @@ func TestSmokeUpAddStatusDown(t *testing.T) {
 // the session.
 // The fix splits the tallest alive pane explicitly and hard-errors on a non-new reported id, so
 // this sequence must now yield one live pane per visible strand, plus one more for the
-// always-present header pane.
+// always-present Selvage pane.
 func TestSmokeStackedAddsKeepEverySessionPane(t *testing.T) {
 	tmuxPath := tmuxBinaryPath(t)
 
@@ -115,9 +115,9 @@ func TestSmokeStackedAddsKeepEverySessionPane(t *testing.T) {
 
 	socket, session := socketAndSession(t)
 	panes := listPaneLines(t, tmuxPath, socket, session)
-	wantPanes := len(guids) + 1 // +1 for the always-present header pane
+	wantPanes := len(guids) + 1 // +1 for the always-present Selvage pane
 	if len(panes) != wantPanes {
-		t.Fatalf("session holds %d panes %v; want %d (one per visible strand plus the header pane — a shortfall means a silent split failure destroyed panes)", len(panes), panes, wantPanes)
+		t.Fatalf("session holds %d panes %v; want %d (one per visible strand plus Selvage — a shortfall means a silent split failure destroyed panes)", len(panes), panes, wantPanes)
 	}
 
 	out.Reset()
@@ -136,10 +136,10 @@ func TestSmokeStackedAddsKeepEverySessionPane(t *testing.T) {
 }
 
 // TestSmokeRemoveLastStrandThenAddRunsTheNewCommand exercises removing a session's last STRAND and
-// then adding a new one, with the always-present header pane in play: removeStrandLocked collects
-// pane ids from strands only, and the header is never a strand, so the removed strand's pane is never
+// then adding a new one, with the always-present Selvage pane in play: removeStrandLocked collects
+// pane ids from strands only, and Selvage is never a strand, so the removed strand's pane is never
 // the session's actual last pane — kill-pane removes it outright, on either backend, rather than
-// corpsing it under remain-on-exit, and the header alone is left holding the session up. The
+// corpsing it under remain-on-exit, and Selvage alone is left holding the session up. The
 // following add must split a fresh pane whose command genuinely runs and STAYS live across the next
 // reconciling verb.
 //
@@ -213,11 +213,11 @@ func TestSmokeRemoveLastStrandThenAddRunsTheNewCommand(t *testing.T) {
 // skips select-layout whenever no strand owns a present pane): this fixture's two up calls always
 // arrive at apply holding exactly one pane, well under the len(live) < 2 guard applyLayoutLockedOpts
 // checks first, so the anyPlacedStrand branch is never even reached from here anymore.
-// What this test still proves instead: with zero strands tracked, an ALIVE header now authorizes
+// What this test still proves instead: with zero strands tracked, an ALIVE Selvage now authorizes
 // reconcile's deterministic untracked-pane reap (reconcile.go), so an up against a session holding
-// only foreign panes leaves a USABLE session — the header pane intact, every foreign pane gone — not
+// only foreign panes leaves a USABLE session — Selvage intact, every foreign pane gone — not
 // the old zero-pane wedge, and a subsequent add comes up live on its own fresh pane without ever
-// displacing the header.
+// displacing Selvage.
 func TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable(t *testing.T) {
 	tmuxPath := tmuxBinaryPath(t)
 
@@ -235,29 +235,29 @@ func TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable(t *testing.T) {
 	}
 	socket, session := socketAndSession(t)
 
-	// up boots the always-present header pane before any strand exists, and
+	// up boots the always-present Selvage pane before any strand exists, and
 	// this SAME up's own reconcile (reconcileApplyPersistLocked's tail)
 	// already reaps the session's not-yet-adopted initial pane: with zero
-	// strands tracked, the newly-alive header authorizes the untracked-pane
+	// strands tracked, the newly-alive Selvage authorizes the untracked-pane
 	// reap (reconcile.go), so that initial pane never survives past this
-	// first up at all. Read the header's pane id directly from reed.json so
+	// first up at all. Read Selvage's pane id directly from reed.json so
 	// the assertions below can tell it apart from the foreign pane added
 	// next.
 	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState after up = (%+v, %v), want a persisted SelvagePaneID", st, err)
 	}
-	headerPaneID := st.HeaderPaneID
+	selvagePaneID := st.SelvagePaneID
 
 	// A foreign pane reed does not track (the operator-split case): the
-	// session holds 1 pane (the header) and 0 strands going in — the first
+	// session holds 1 pane (Selvage) and 0 strands going in — the first
 	// up above already reaped its own not-yet-adopted initial pane — so this
-	// split leaves it at 2 panes (header, foreign) and 0 strands.
+	// split leaves it at 2 panes (Selvage, foreign) and 0 strands.
 	if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
 		t.Fatalf("foreign split-window: %v", err)
 	}
 
-	// The second up: with zero strands tracked and the header alive,
+	// The second up: with zero strands tracked and Selvage alive,
 	// reconcile's untracked reap fires again and kills the foreign pane too
 	// — this up must still exit 0 and leave the session usable, never the
 	// zero-pane wedge the old empty-layout-apply defect produced.
@@ -265,8 +265,8 @@ func TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable(t *testing.T) {
 	if code := RunCLI(&out, []string{"up"}); code != 0 {
 		t.Fatalf("second up = %d; want 0, output: %s", code, out.String())
 	}
-	if panes := listPaneLines(t, tmuxPath, socket, session); len(panes) != 1 || !paneLiveOnSession(panes, headerPaneID) {
-		t.Fatalf("up with only a foreign pane must reap it, leaving exactly the header pane %s alive; got panes=%v", headerPaneID, panes)
+	if panes := listPaneLines(t, tmuxPath, socket, session); len(panes) != 1 || !paneLiveOnSession(panes, selvagePaneID) {
+		t.Fatalf("up with only a foreign pane must reap it, leaving exactly Selvage %s alive; got panes=%v", selvagePaneID, panes)
 	}
 
 	// The session must still be able to host a strand: the add both proves
@@ -288,27 +288,20 @@ func TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable(t *testing.T) {
 	}
 	strandPane, _ := strand["paneId"].(string)
 	panes := listPaneLines(t, tmuxPath, socket, session)
-	if len(panes) != 2 || !paneLiveOnSession(panes, strandPane) || !paneLiveOnSession(panes, headerPaneID) {
-		t.Errorf("after add, session panes = %v; want exactly the strand's pane %s and the header pane %s (foreign pane must be reaped, neither pane ever displaced)", panes, strandPane, headerPaneID)
+	if len(panes) != 2 || !paneLiveOnSession(panes, strandPane) || !paneLiveOnSession(panes, selvagePaneID) {
+		t.Errorf("after add, session panes = %v; want exactly the strand's pane %s and Selvage %s (foreign pane must be reaped, neither pane ever displaced)", panes, strandPane, selvagePaneID)
 	}
 }
 
-// TestSmokeHeaderPaneDisplaysRenderedHeaderText pins the header pane's actual OUTPUT — the rendered
-// "hub: <hub path>" line from the embedded default template — not merely its liveness.
-// This is the regression test for the header-cwd defect the fable-header-r1 round found: the pane
-// used to be split with -c set to the HUB path, a container directory that is by definition not a
-// git repo (the engine field that then held it is long gone; today the anchor arrives as the told
-// Geometry.AnchorPath),
-// so its "lyx reed header --blocking" command died at geometry resolution ({"ok":false,"error":"not
-// a git repository"}) and the operator console showed a JSON error over a bash prompt forever —
-// while every liveness-only assertion stayed green, because the pane's parent shell survived the
-// failed command.
-// Two things make content assertable here where the other smoke tests cannot: up must run as a
-// SUBPROCESS of the built lyx binary (the header pane boots os.Executable() + " reed header
-// --blocking", and an in-process RunCLI's executable is this TEST binary, whose header invocation
-// is nonsense), and the assertion polls capture-pane for the rendered text rather than list-panes
-// for presence.
-func TestSmokeHeaderPaneDisplaysRenderedHeaderText(t *testing.T) {
+// TestSmokeStatusLineDisplaysRenderedText pins the tmux status-line's actual OUTPUT — the rendered
+// text from the embedded default template, which names the hub's absolute path — not merely that
+// `up` succeeded. This is the surviving analogue of the old header-cwd regression test: content is
+// no longer assertable against any pane at all (pinGeometryOptionsLocked pins it into tmux's
+// status-line option), so the assertion is a `display-message -p '#{status-left}'` readback rather
+// than a pane-content poll, and the former 1-row pane-clamping regression this test also pinned has
+// no counterpart either — the status-line is not a pane and carries no row budget of its own to
+// clamp against.
+func TestSmokeStatusLineDisplaysRenderedText(t *testing.T) {
 	tmuxPath := tmuxBinaryPath(t)
 	lyxExe := buildLyxBinary(t)
 
@@ -326,42 +319,28 @@ func TestSmokeHeaderPaneDisplaysRenderedHeaderText(t *testing.T) {
 		t.Fatalf("built-binary up: %v\n%s", err, out)
 	}
 
-	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
-	}
-
-	socket, _ := socketAndSession(t)
-	// The embedded default template renders "hub: {{.hub}}"; the fixture's
-	// hub is its temp container. A JSON error body in the pane (the pre-fix
-	// symptom) can never contain this line.
-	pollPaneContains(t, tmuxPath, socket, st.HeaderPaneID, "hub: "+h.Location.HubPath, 20*time.Second)
-
-	// The 1-row regression (fable-header-r1 F10): once a strand exists the
-	// header clamps to its configured single row (height_rows: 1), and
-	// capture-pane's default output is the VISIBLE area only — so this
-	// second poll proves the rendered text sits ON that one visible row.
-	// Pre-fix, the pane's echoed launch line plus a trailing newline left
-	// the cursor on a fresh empty row, which was the only row the 1-row
-	// pane showed; the text existed solely in scrollback.
-	addCmd := exec.Command(lyxExe, "reed", "add", "--cmd", smokeReapLaunchCmd(), "--name", "clamps-header")
-	addCmd.Dir = h.PrimeWorktree()
-	if out, err := addCmd.CombinedOutput(); err != nil {
-		t.Fatalf("built-binary add: %v\n%s", err, out)
-	}
-	pollPaneContains(t, tmuxPath, socket, st.HeaderPaneID, "hub: "+h.Location.HubPath, 20*time.Second)
+	socket, session := socketAndSession(t)
+	statusLeft, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", session, "#{status-left}").Output()
+	if err != nil {
+		t.Fatalf("display-message #{status-left}: %v", err)
+	}
+	// The embedded default template renders "{{.repo}}/{{.worktree}} · {{.hub}}"; the fixture's
+	// hub path is still rendered verbatim inside it regardless of the surrounding template shape.
+	if got := strings.TrimSpace(string(statusLeft)); !strings.Contains(got, h.Location.HubPath) {
+		t.Errorf("#{status-left} = %q; want it to contain the hub path %q", got, h.Location.HubPath)
+	}
 }
 
-// TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile pins the header-pane keepalive guarantee this
-// batch adds: the always-present header pane must survive a full up -> add -> remove -> add cycle
+// TestSmokeSelvageSurvivesUpAddRemoveAndReconcile pins Selvage's keepalive guarantee: the
+// always-present Selvage pane must survive a full up -> add -> remove -> add cycle
 // and every reconcile along the way, and it is never a strand's pane — because a strand's pane is
-// always a fresh split (planPaneTarget never targets the header while any non-header pane exists),
-// and the header is exempt from both halves of reconcile's kill schedule — and, the whole point,
+// always a fresh split (planPaneTarget never targets Selvage while any non-Selvage pane exists),
+// and Selvage is exempt from both halves of reconcile's kill schedule — and, the whole point,
 // still alive even when the strand table momentarily drops to zero after a remove.
 // Mirrors TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable's tmux-driven verification style
-// (list-panes via the real binary, not reed's own reporting) but for the header instead of a
+// (list-panes via the real binary, not reed's own reporting) but for Selvage instead of a
 // foreign pane.
-func TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile(t *testing.T) {
+func TestSmokeSelvageSurvivesUpAddRemoveAndReconcile(t *testing.T) {
 	tmuxPath := tmuxBinaryPath(t)
 
 	h := hubforge.NewHub(t, ".")
@@ -377,34 +356,34 @@ func TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile(t *testing.T) {
 		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
 	}
 
-	// up boots the header pane before any strand exists (card 17). Read the
-	// persisted pane id directly from reed.json (RunCLI/status carries no
-	// header field) rather than assuming which of the session's panes it is.
+	// up boots Selvage before any strand exists. Read the persisted pane id
+	// directly from reed.json (RunCLI/status carries no Selvage field)
+	// rather than assuming which of the session's panes it is.
 	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState after up = (%+v, %v), want a persisted SelvagePaneID", st, err)
 	}
-	headerPaneID := st.HeaderPaneID
+	selvagePaneID := st.SelvagePaneID
 
 	socket, session := socketAndSession(t)
-	requireHeaderAlive := func(when string) {
+	requireSelvageAlive := func(when string) {
 		t.Helper()
 		lines := listPaneLines(t, tmuxPath, socket, session)
-		if !paneLiveOnSession(lines, headerPaneID) {
-			t.Fatalf("header pane %s not alive %s; panes=%v", headerPaneID, when, lines)
+		if !paneLiveOnSession(lines, selvagePaneID) {
+			t.Fatalf("Selvage pane %s not alive %s; panes=%v", selvagePaneID, when, lines)
 		}
 	}
-	requireHeaderAlive("right after up (zero strands)")
+	requireSelvageAlive("right after up (zero strands)")
 
 	// add: the preceding up's own reconcile already reaped the session's
-	// pre-header pane (the same zero-strands-plus-alive-header reap
-	// TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable pins), so the header
+	// pre-Selvage pane (the same zero-strands-plus-alive-Selvage reap
+	// TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable pins), so Selvage
 	// is the session's only pane when this first strand is added.
-	// planPaneTarget's header-as-last-resort fallback then splits off the
-	// header itself — the header stays alive as the split TARGET, and the
-	// strand lands on the freshly split pane, never on the header pane.
+	// planPaneTarget's Selvage-as-last-resort fallback then splits off
+	// Selvage itself — Selvage stays alive as the split TARGET, and the
+	// strand lands on the freshly split pane, never on Selvage.
 	guid := addStrand(t, "pwsh -NoExit -Command Write-Host ready", "--name", "first")
-	requireHeaderAlive("after add")
+	requireSelvageAlive("after add")
 
 	out.Reset()
 	if code := RunCLI(&out, []string{"status"}); code != 0 {
@@ -418,11 +397,11 @@ func TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile(t *testing.T) {
 		t.Errorf("strand %s live = false; want true", guid)
 	}
 	strandPaneID, _ := strand["paneId"].(string)
-	if strandPaneID == "" || strandPaneID == headerPaneID {
-		t.Fatalf("strand %s paneId = %q, want a real, non-header pane id", guid, strandPaneID)
+	if strandPaneID == "" || strandPaneID == selvagePaneID {
+		t.Fatalf("strand %s paneId = %q, want a real, non-Selvage pane id", guid, strandPaneID)
 	}
 
-	// remove: the session's only strand is gone, but the header pane — a
+	// remove: the session's only strand is gone, but Selvage — a
 	// permanent second pane — must keep the session (and itself) alive,
 	// exactly the invariant contract_integration_test.go's
 	// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds pins at the engine
@@ -432,24 +411,24 @@ func TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile(t *testing.T) {
 		t.Fatalf("remove = %d; want 0, output: %s", code, out.String())
 	}
 	if !sessionAlive(tmuxPath, socket, session) {
-		t.Fatalf("session %s died after removing its sole strand; the header pane must have kept it alive", session)
+		t.Fatalf("session %s died after removing its sole strand; Selvage must have kept it alive", session)
 	}
-	requireHeaderAlive("after removing the sole strand (zero strands tracked)")
+	requireSelvageAlive("after removing the sole strand (zero strands tracked)")
 
-	// A reconciling verb (up) with zero strands must not disturb the header
+	// A reconciling verb (up) with zero strands must not disturb Selvage
 	// either — mirrors TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable's
 	// same-shaped assertion for a foreign pane.
 	out.Reset()
 	if code := RunCLI(&out, []string{"up"}); code != 0 {
 		t.Fatalf("post-remove up = %d; want 0, output: %s", code, out.String())
 	}
-	requireHeaderAlive("after a reconciling up with zero strands")
+	requireSelvageAlive("after a reconciling up with zero strands")
 
-	// add again: the header must still never become the new strand's own
-	// pane, and the new strand must come up live — the substrate the header
+	// add again: Selvage must still never become the new strand's own
+	// pane, and the new strand must come up live — the substrate Selvage
 	// keeps alive is still genuinely usable, not a wedged husk.
 	second := addStrand(t, "pwsh -NoExit -Command Write-Host ready", "--name", "second")
-	requireHeaderAlive("after a second add with strands now bound")
+	requireSelvageAlive("after a second add with strands now bound")
 
 	out.Reset()
 	if code := RunCLI(&out, []string{"status"}); code != 0 {
@@ -462,8 +441,8 @@ func TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile(t *testing.T) {
 	if live, _ := strand2["live"].(bool); !live {
 		t.Errorf("strand %s live = false; want true", second)
 	}
-	if paneID, _ := strand2["paneId"].(string); paneID == headerPaneID {
-		t.Errorf("strand %s was bound to the header pane %s; the header must never become a strand's own pane", second, headerPaneID)
+	if paneID, _ := strand2["paneId"].(string); paneID == selvagePaneID {
+		t.Errorf("strand %s was bound to Selvage %s; Selvage must never become a strand's own pane", second, selvagePaneID)
 	}
 }
 
@@ -489,11 +468,11 @@ func pollProcessGone(t *testing.T, pid int, timeout time.Duration) {
 // manually-created split-window pane must never be adopted as a strand's pane, and must instead be
 // reaped by add's reconcile.
 //
-// M16 fires only when the sole alive non-header pane is the foreign one — a session that still holds
-// its unadopted initial new-session pane has TWO alive non-header panes, which is why
+// M16 fires only when the sole alive non-Selvage pane is the foreign one — a session that still holds
+// its unadopted initial new-session pane has TWO alive non-Selvage panes, which is why
 // TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable passed even before this round's fix. So the
-// fixture first drives the session to a header-plus-foreign-pane-only state: up, add a strand, remove
-// that strand (its pane is gone, not corpsed — the always-present header keeps the session up), then
+// fixture first drives the session to a Selvage-plus-foreign-pane-only state: up, add a strand, remove
+// that strand (its pane is gone, not corpsed — the always-present Selvage pane keeps the session up), then
 // split a foreign pane in with the real tmux binary, exactly as
 // TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable does.
 //
@@ -521,10 +500,10 @@ func TestSmokeForeignPaneIsReapedNotAdoptedByAdd(t *testing.T) {
 	}
 
 	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState after up = (%+v, %v), want a persisted SelvagePaneID", st, err)
 	}
-	headerPaneID := st.HeaderPaneID
+	selvagePaneID := st.SelvagePaneID
 
 	guid := addStrand(t, smokeReapLaunchCmd(), "--name", "throwaway")
 	out.Reset()
@@ -536,19 +515,19 @@ func TestSmokeForeignPaneIsReapedNotAdoptedByAdd(t *testing.T) {
 
 	// A foreign pane reed does not track (the operator-split case), created
 	// exactly as TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable's own
-	// foreign split-window: the session now holds the header plus this one
-	// foreign pane, and zero strands — the sole-alive-non-header-pane
+	// foreign split-window: the session now holds Selvage plus this one
+	// foreign pane, and zero strands — the sole-alive-non-Selvage-pane
 	// precondition M16 requires.
 	if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
 		t.Fatalf("foreign split-window: %v", err)
 	}
 
-	// The foreign pane is the one live pane that is neither the header nor
+	// The foreign pane is the one live pane that is neither Selvage nor
 	// the (already-gone, kill-pane-removed-outright) removed strand's pane.
 	foreignPaneID := ""
 	for _, line := range listPaneLines(t, tmuxPath, socket, session) {
 		fields := strings.Fields(line)
-		if len(fields) == 0 || fields[0] == headerPaneID {
+		if len(fields) == 0 || fields[0] == selvagePaneID {
 			continue
 		}
 		foreignPaneID = fields[0]
@@ -587,27 +566,27 @@ func TestSmokeForeignPaneIsReapedNotAdoptedByAdd(t *testing.T) {
 // TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp is the end-to-end regression guard for
 // the R4 review's R4-F4, driven at the CLI seam a real operator uses.
 //
-// Reproduced live before the fix: with the session up, a strand added and the default one-row header
+// Reproduced live before the fix: with the session up, a strand added and the default one-row Selvage
 // band laid out, deleting .lyx/reed.json — a never-tracked machine-local tree the
 // Durable-vs-Ephemeral State Invariant makes disposable, and exactly what `git clean -xdf` in the
 // worktree removes — permanently wedged the worktree. `lyx reed up` and `lyx reed resume` both
 // failed, on every subsequent invocation, with
-// `split header pane: exit status 1: no space for new pane`, because the physically topmost pane was
-// now an UNTRACKED one-row header band that tmux cannot split, while `lyx reed status` kept
+// `split Selvage pane: exit status 1: no space for new pane`, because the physically bottom-most pane was
+// now an UNTRACKED one-row Selvage band that tmux cannot split, while `lyx reed status` kept
 // reporting the session healthy and nothing named the one escape (`down`, then `up`).
 //
 // Under the reap this batch adds, the observable pane set after the recovering `up` has changed: it
-// used to be three panes (the untracked old header, the untracked orphaned strand pane, and the
-// freshly split header), and is now exactly one — the freshly split header alone. The scrub erases the
-// strand table along with HeaderPaneID, so the recovering up's own reconcile runs with zero strands
-// tracked and a freshly-alive header, which authorizes reaping every other pane (reconcile.go). That
-// makes the rebuilt header pane's pane_top == 0 assertion below TRIVIALLY true — with one pane there is
-// nowhere else for it to be — so it is vacuous rather than live coverage now; "a fix that recovered by
+// used to be three panes (the untracked old Selvage pane, the untracked orphaned strand pane, and the
+// freshly split Selvage pane), and is now exactly one — the freshly split Selvage pane alone. The scrub
+// erases the strand table along with SelvagePaneID, so the recovering up's own reconcile runs with zero
+// strands tracked and a freshly-alive Selvage, which authorizes reaping every other pane (reconcile.go).
+// That makes the rebuilt Selvage pane's pane_top == 0 assertion below TRIVIALLY true — with one pane there
+// is nowhere else for it to be — so it is vacuous rather than live coverage now; "a fix that recovered by
 // splitting somewhere else would fail here" no longer has teeth for this fixture. What stays
 // load-bearing is the other half: the recovering `up` must still exit 0 — the actual R4-F4 wedge
-// (`split header pane: exit status 1: no space for new pane` on every subsequent invocation) — plus
+// (`split Selvage pane: exit status 1: no space for new pane` on every subsequent invocation) — plus
 // the following `add` proving the session is genuinely usable again, not merely non-erroring.
-// TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader is where the reap's own effect on this
+// TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltSelvage is where the reap's own effect on this
 // scenario is actually pinned, asserting on the recovering `up` itself rather than a follow-up verb.
 func TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp(t *testing.T) {
 	tmuxPath := tmuxBinaryPath(t)
@@ -626,7 +605,7 @@ func TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp(t *testing.T) {
 		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
 	}
 	// A strand, so applyLayoutLocked actually applies reed's layout and the
-	// header band is squeezed down to its configured one row — the whole
+	// Selvage band is squeezed down to its configured one row — the whole
 	// precondition for the wedge.
 	addStrand(t, "pwsh -NoExit -Command Write-Host ready", "--name", "before-scrub")
 	socket, session := socketAndSession(t)
@@ -643,33 +622,33 @@ func TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp(t *testing.T) {
 	}
 
 	st, err := reedengine.LoadState(stateDir)
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after the recovering up = (%+v, %v); want a freshly persisted HeaderPaneID", st, err)
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState after the recovering up = (%+v, %v); want a freshly persisted SelvagePaneID", st, err)
 	}
-	headerTop := ""
+	selvageTop := ""
 	for _, line := range listPaneLines(t, tmuxPath, socket, session) {
 		fields := strings.Fields(line)
-		if len(fields) >= 3 && fields[0] == st.HeaderPaneID {
-			headerTop = fields[2]
+		if len(fields) >= 3 && fields[0] == st.SelvagePaneID {
+			selvageTop = fields[2]
 		}
 	}
-	if headerTop != "0" {
-		t.Errorf("rebuilt header pane %s has pane_top %q; want %q — the header must land physically topmost or select-layout misassigns its cell", st.HeaderPaneID, headerTop, "0")
+	if selvageTop != "0" {
+		t.Errorf("rebuilt Selvage pane %s has pane_top %q; want %q — as the session's sole pane, Selvage must land at the origin or select-layout misassigns its cell", st.SelvagePaneID, selvageTop, "0")
 	}
 
 	// The session must be genuinely usable again, not merely non-erroring.
 	addStrand(t, "pwsh -NoExit -Command Write-Host recovered", "--name", "after-scrub")
 }
 
-// TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader is the M22 regression: a scrubbed
+// TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltSelvage is the M22 regression: a scrubbed
 // .lyx/reed.json must converge on the recovering `up` under test itself, not one verb later.
 //
-// The pre-fix defect converged one verb late — the recovering up left the old header pane and the
-// orphaned strand pane both untracked-but-alive alongside the freshly split header, and only a
+// The pre-fix defect converged one verb late — the recovering up left the old Selvage pane and the
+// orphaned strand pane both untracked-but-alive alongside the freshly split Selvage pane, and only a
 // FOLLOW-UP verb's reconcile cleared them. An assertion placed after that follow-up would pass either
 // way, which is why this test asserts on the recovering `up` itself, with no intervening verb: the
-// session must hold exactly one pane (the newly persisted HeaderPaneID, distinct from the captured old
-// one), and both the old header pane id and the old strand pane id must already be gone from
+// session must hold exactly one pane (the newly persisted SelvagePaneID, distinct from the captured old
+// one), and both the old Selvage pane id and the old strand pane id must already be gone from
 // list-panes.
 //
 // The orphaned strand pane's captured #{pane_pid} is polled gone too, under a bounded deadline
@@ -678,12 +657,12 @@ func TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp(t *testing.T) {
 // leak this pins is the pane and its own process, not the whole subtree (RemoveStrand's and Down's own
 // tests pin subtree reaping).
 //
-// The header-only, full-height end state this test asserts is the accepted outcome, not a layout
+// The Selvage-only, full-height end state this test asserts is the accepted outcome, not a layout
 // defect to "fix" by synthesizing a spacer pane: applyLayoutLockedOpts deliberately skips
 // select-layout when no strand owns a present pane (anyPlacedStrand, apply.go), and the scrub erases
-// the strand table along with HeaderPaneID, so the recovering up's reconcile leaves nothing else for
+// the strand table along with SelvagePaneID, so the recovering up's reconcile leaves nothing else for
 // this session to place.
-func TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader(t *testing.T) {
+func TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltSelvage(t *testing.T) {
 	tmuxPath := tmuxBinaryPath(t)
 
 	h := hubforge.NewHub(t, ".")
@@ -701,10 +680,10 @@ func TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader(t *testing.T) {
 
 	stateDir := filepath.Join(h.PrimeWorktree(), ".lyx")
 	stBefore, err := reedengine.LoadState(stateDir)
-	if err != nil || stBefore == nil || stBefore.HeaderPaneID == "" {
-		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", stBefore, err)
+	if err != nil || stBefore == nil || stBefore.SelvagePaneID == "" {
+		t.Fatalf("LoadState after up = (%+v, %v), want a persisted SelvagePaneID", stBefore, err)
 	}
-	oldHeaderPaneID := stBefore.HeaderPaneID
+	oldSelvagePaneID := stBefore.SelvagePaneID
 
 	guid := addStrand(t, smokeReapLaunchCmd(), "--name", "orphaned-by-scrub")
 	socket, session := socketAndSession(t)
@@ -734,25 +713,25 @@ func TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader(t *testing.T) {
 	}
 
 	stAfter, err := reedengine.LoadState(stateDir)
-	if err != nil || stAfter == nil || stAfter.HeaderPaneID == "" {
-		t.Fatalf("LoadState after the recovering up = (%+v, %v); want a freshly persisted HeaderPaneID", stAfter, err)
+	if err != nil || stAfter == nil || stAfter.SelvagePaneID == "" {
+		t.Fatalf("LoadState after the recovering up = (%+v, %v); want a freshly persisted SelvagePaneID", stAfter, err)
 	}
-	newHeaderPaneID := stAfter.HeaderPaneID
-	if newHeaderPaneID == oldHeaderPaneID {
-		t.Fatalf("HeaderPaneID after the recovering up = %s; want a NEW id distinct from the pre-scrub header %s", newHeaderPaneID, oldHeaderPaneID)
+	newSelvagePaneID := stAfter.SelvagePaneID
+	if newSelvagePaneID == oldSelvagePaneID {
+		t.Fatalf("SelvagePaneID after the recovering up = %s; want a NEW id distinct from the pre-scrub Selvage pane %s", newSelvagePaneID, oldSelvagePaneID)
 	}
 
 	panes := listPaneLines(t, tmuxPath, socket, session)
-	if len(panes) != 1 || !paneLiveOnSession(panes, newHeaderPaneID) {
-		t.Fatalf("panes after the recovering up = %v; want exactly the freshly rebuilt header pane %s", panes, newHeaderPaneID)
+	if len(panes) != 1 || !paneLiveOnSession(panes, newSelvagePaneID) {
+		t.Fatalf("panes after the recovering up = %v; want exactly the freshly rebuilt Selvage pane %s", panes, newSelvagePaneID)
 	}
 	for _, line := range panes {
 		fields := strings.Fields(line)
 		if len(fields) == 0 {
 			continue
 		}
-		if fields[0] == oldHeaderPaneID {
-			t.Errorf("old header pane %s still present after the recovering up; want it reaped", oldHeaderPaneID)
+		if fields[0] == oldSelvagePaneID {
+			t.Errorf("old Selvage pane %s still present after the recovering up; want it reaped", oldSelvagePaneID)
 		}
 		if fields[0] == oldStrandPaneID {
 			t.Errorf("orphaned strand pane %s still present after the recovering up; want it reaped", oldStrandPaneID)
diff --git a/internal/reedcli/smoke_panecwd_test.go b/internal/reedcli/smoke_panecwd_test.go
index 11f5adfa9..7f66d48d8 100644
--- a/internal/reedcli/smoke_panecwd_test.go
+++ b/internal/reedcli/smoke_panecwd_test.go
@@ -29,10 +29,10 @@ import (
 // genuinely different planPaneTarget branches, worth asserting as two distinct cases rather than one
 // duplicated twice: by
 // the time this fixture's first add runs, the preceding up's own reconcile has already reaped the
-// session down to the header pane alone (the same zero-strands-plus-alive-header reap
-// TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable pins), so the FIRST strand's split targets the
-// header itself (planPaneTarget's header-as-last-resort fallback, since no non-header pane exists to
-// split otherwise). The SECOND strand then targets the tallest alive non-header pane — the first
+// session down to Selvage alone (the same zero-strands-plus-alive-Selvage reap
+// TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable pins), so the FIRST strand's split targets Selvage
+// itself (planPaneTarget's Selvage-as-last-resort fallback, since no non-Selvage pane exists to
+// split otherwise). The SECOND strand then targets the tallest alive non-Selvage pane — the first
 // strand's own pane, once it exists. Both are exercised for the -c regression identically: the split
 // path is the one the defect broke, on either branch.
 func TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd(t *testing.T) {
@@ -76,7 +76,7 @@ func TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd(t *testing.T) {
 		name string
 		guid string
 	}{
-		{"first strand (splits off the header)", first},
+		{"first strand (splits off Selvage)", first},
 		{"second strand (splits off the first)", second},
 	}
 	for _, tt := range tests {
diff --git a/internal/reedcli/smoke_selvage_keepalive_test.go b/internal/reedcli/smoke_selvage_keepalive_test.go
new file mode 100644
index 000000000..37b218de1
--- /dev/null
+++ b/internal/reedcli/smoke_selvage_keepalive_test.go
@@ -0,0 +1,355 @@
+//go:build smoke
+
+// smoke_selvage_keepalive_test.go pins Selvage's keepalive guarantee — the job the header pane's
+// `--blocking` process once served before this task, now served by a permanent, deliberately
+// ordinary shell pane instead of a custom blocking mechanism. There is no analogue of the old
+// blockForever/deadlock-avoidance test here: Selvage runs the operator's configured shell
+// (e.cfg.Shell) directly, so there is no reed-authored blocking loop left to pin against a runtime
+// deadlock detector. What survives is the guarantee itself — a session with Selvage in it never
+// dies just because every strand died — pinned instead against the real mechanism: an ordinary
+// shell surviving a killed sibling pane, surviving Ctrl-C at its own idle prompt, and surviving
+// Ctrl-C to a foreground job running inside it. Tagged smoke because untagged reedcli tests must
+// not spawn processes (Test Tier Purity Invariant).
+
+package reedcli
+
+import (
+	"bytes"
+	"os/exec"
+	"path/filepath"
+	"runtime"
+	"strconv"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/Knatte18/loomyard/internal/hubforge"
+	"github.com/Knatte18/loomyard/internal/reedengine"
+)
+
+// selvagePaneID reads the persisted SelvagePaneID for worktree, failing the test if it is empty —
+// RunCLI/status carries no Selvage field of its own, so every caller here reads it straight from
+// reed.json exactly as the header-pane tests once did for HeaderPaneID.
+func selvagePaneID(t *testing.T, worktree string) string {
+	t.Helper()
+	st, err := reedengine.LoadState(filepath.Join(worktree, ".lyx"))
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState(%s) = (%+v, %v), want a persisted SelvagePaneID", worktree, st, err)
+	}
+	return st.SelvagePaneID
+}
+
+// TestSmokeSelvageSurvivesKillingEveryStrandPane pins the keepalive job itself: tmux destroys a
+// session once its last window loses its last pane, so with every strand pane force-killed via
+// tmux (not reed's own remove, which would already tear its bookkeeping down cleanly), Selvage
+// alone must be enough to keep the session — and a subsequent reed verb usable — alive.
+func TestSmokeSelvageSurvivesKillingEveryStrandPane(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+	t.Chdir(h.PrimeWorktree())
+	t.Cleanup(func() {
+		var buf bytes.Buffer
+		RunCLI(&buf, []string{"down"})
+	})
+
+	var out bytes.Buffer
+	if code := RunCLI(&out, []string{"up"}); code != 0 {
+		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
+	}
+	selvage := selvagePaneID(t, h.PrimeWorktree())
+
+	guids := []string{
+		addStrand(t, smokeReapLaunchCmd(), "--name", "first"),
+		addStrand(t, smokeReapLaunchCmd(), "--name", "second"),
+	}
+
+	socket, session := socketAndSession(t)
+	for _, guid := range guids {
+		paneID := paneIDForStrand(t, guid)
+		if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", paneID).Run(); err != nil {
+			t.Fatalf("kill-pane %s (strand %s): %v", paneID, guid, err)
+		}
+	}
+
+	if !sessionAlive(tmuxPath, socket, session) {
+		t.Fatalf("session %s died after killing every strand pane; Selvage must have kept it alive", session)
+	}
+	if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
+		t.Fatalf("Selvage pane %s not alive after killing every strand pane; panes=%v", selvage, panes)
+	}
+
+	out.Reset()
+	if code := RunCLI(&out, []string{"status"}); code != 0 {
+		t.Fatalf("status after killing every strand pane = %d; want 0, output: %s", code, out.String())
+	}
+}
+
+// TestSmokeSelvageStaysPhysicallyBottomMostAcrossAddsAndRemoves pins the physical-position rule
+// module-local to this package (see internal/reedengine/doc.go): Selvage is always the pane with
+// the largest pane_top, in every state a series of adds and removes can leave the window in.
+func TestSmokeSelvageStaysPhysicallyBottomMostAcrossAddsAndRemoves(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+	t.Chdir(h.PrimeWorktree())
+	t.Cleanup(func() {
+		var buf bytes.Buffer
+		RunCLI(&buf, []string{"down"})
+	})
+
+	var out bytes.Buffer
+	if code := RunCLI(&out, []string{"up"}); code != 0 {
+		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
+	}
+	selvage := selvagePaneID(t, h.PrimeWorktree())
+	socket, session := socketAndSession(t)
+
+	requireBottomMost := func(when string) {
+		t.Helper()
+		panes := listPaneLines(t, tmuxPath, socket, session)
+		bottomTop := -1
+		bottomID := ""
+		for _, line := range panes {
+			fields := strings.Fields(line)
+			if len(fields) < 3 {
+				continue
+			}
+			top, err := strconv.Atoi(fields[2])
+			if err != nil {
+				t.Fatalf("parse pane_top %q: %v", fields[2], err)
+			}
+			if top > bottomTop {
+				bottomTop = top
+				bottomID = fields[0]
+			}
+		}
+		if bottomID != selvage {
+			t.Errorf("%s: physically bottom-most pane = %s; want Selvage %s; panes=%v", when, bottomID, selvage, panes)
+		}
+	}
+	requireBottomMost("right after up")
+
+	first := addStrand(t, smokeReapLaunchCmd(), "--name", "first")
+	requireBottomMost("after adding the first strand")
+
+	second := addStrand(t, smokeReapLaunchCmd(), "--name", "second")
+	requireBottomMost("after adding a second strand")
+
+	out.Reset()
+	if code := RunCLI(&out, []string{"remove", first}); code != 0 {
+		t.Fatalf("remove %s = %d; want 0, output: %s", first, code, out.String())
+	}
+	requireBottomMost("after removing the first strand")
+
+	addStrand(t, smokeReapLaunchCmd(), "--name", "third")
+	requireBottomMost("after adding a third strand")
+
+	out.Reset()
+	if code := RunCLI(&out, []string{"remove", second}); code != 0 {
+		t.Fatalf("remove %s = %d; want 0, output: %s", second, code, out.String())
+	}
+	requireBottomMost("after removing the second strand")
+}
+
+// TestSmokeSelvageSurvivesCtrlCAtIdlePrompt pins the first of the two ordinary-shell survival
+// claims in internal/reedengine/doc.go's design record: interactive bash ignores SIGINT while
+// waiting at its own idle prompt, so a Ctrl-C sent directly to Selvage must never take the pane —
+// or the session it anchors — down.
+func TestSmokeSelvageSurvivesCtrlCAtIdlePrompt(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+	t.Chdir(h.PrimeWorktree())
+	t.Cleanup(func() {
+		var buf bytes.Buffer
+		RunCLI(&buf, []string{"down"})
+	})
+
+	var out bytes.Buffer
+	if code := RunCLI(&out, []string{"up"}); code != 0 {
+		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
+	}
+	selvage := selvagePaneID(t, h.PrimeWorktree())
+	socket, session := socketAndSession(t)
+
+	if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", selvage, "C-c").Run(); err != nil {
+		t.Fatalf("send-keys C-c to idle Selvage %s: %v", selvage, err)
+	}
+	time.Sleep(1 * time.Second)
+
+	if !sessionAlive(tmuxPath, socket, session) {
+		t.Fatalf("session %s died after Ctrl-C to Selvage's idle prompt", session)
+	}
+	if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
+		t.Fatalf("Selvage pane %s not alive after Ctrl-C at its idle prompt; panes=%v", selvage, panes)
+	}
+}
+
+// TestSmokeSelvageSurvivesCtrlCKillingAForegroundJob pins the second ordinary-shell survival
+// claim: Ctrl-C to a foreground job running inside Selvage kills only that job, never the shell
+// pane itself.
+func TestSmokeSelvageSurvivesCtrlCKillingAForegroundJob(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+	t.Chdir(h.PrimeWorktree())
+	t.Cleanup(func() {
+		var buf bytes.Buffer
+		RunCLI(&buf, []string{"down"})
+	})
+
+	var out bytes.Buffer
+	if code := RunCLI(&out, []string{"up"}); code != 0 {
+		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
+	}
+	selvage := selvagePaneID(t, h.PrimeWorktree())
+	socket, session := socketAndSession(t)
+
+	// Baseline: Selvage's own shell and whatever it has spawned so far, before the foreground job
+	// starts — panePaneSubtree is the same cross-platform (linux/proc, windows/WMI) descendant-closure
+	// helper the pane-reap smoke tests already use.
+	before := map[int]bool{}
+	for _, pid := range panePaneSubtree(t, tmuxPath, socket, session, selvage) {
+		before[pid] = true
+	}
+
+	// A foreground job typed literally into Selvage's own shell — never reed's doing, exactly like
+	// an operator running a long command directly at the prompt.
+	sendKeysLine(t, tmuxPath, socket, selvage, smokeReapLaunchCmd())
+
+	var jobPIDs []int
+	deadline := time.Now().Add(10 * time.Second)
+	for {
+		for _, pid := range panePaneSubtree(t, tmuxPath, socket, session, selvage) {
+			if !before[pid] {
+				jobPIDs = append(jobPIDs, pid)
+			}
+		}
+		if len(jobPIDs) > 0 {
+			break
+		}
+		if time.Now().After(deadline) {
+			t.Fatalf("Selvage's foreground job never showed up as a new descendant within 10s")
+		}
+		time.Sleep(200 * time.Millisecond)
+	}
+
+	if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", selvage, "C-c").Run(); err != nil {
+		t.Fatalf("send-keys C-c to Selvage %s running a foreground job: %v", selvage, err)
+	}
+
+	for _, pid := range jobPIDs {
+		pollProcessGone(t, pid, 10*time.Second)
+	}
+
+	if !sessionAlive(tmuxPath, socket, session) {
+		t.Fatalf("session %s died after Ctrl-C killed Selvage's foreground job", session)
+	}
+	if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
+		t.Fatalf("Selvage pane %s not alive after Ctrl-C killed its foreground job; panes=%v", selvage, panes)
+	}
+}
+
+// TestSmokeStatusLinePinsIdentityAndPosition pins pinGeometryOptionsLocked's status-line pins after
+// `up`: `#{status}` reads "on", `#{status-position}` reads "bottom", and `#{status-left}` names both
+// the repo and the worktree.
+//
+// On Windows the identity values are not asserted directly — per the
+// windows-status-line-is-an-unbranched-accepted-degrade Shared Decision, psmux may refuse some or all
+// of the seven status-line set-option calls, and reed does not branch to compensate. What is asserted
+// there instead is the SELF-CORRECTING half: whatever `#{status}` reads back, the reserved-row count
+// it implies must match the window the layout was actually planned against, so a psmux that refuses
+// the options fails the identity assertion loudly (an operator watching the status-line notices)
+// rather than silently corrupting the layout.
+func TestSmokeStatusLinePinsIdentityAndPosition(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+	t.Chdir(h.PrimeWorktree())
+	t.Cleanup(func() {
+		var buf bytes.Buffer
+		RunCLI(&buf, []string{"down"})
+	})
+
+	var out bytes.Buffer
+	if code := RunCLI(&out, []string{"up"}); code != 0 {
+		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
+	}
+	socket, session := socketAndSession(t)
+	target := "=" + session + ":"
+
+	readOption := func(option string) string {
+		t.Helper()
+		out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", target, "#{"+option+"}").Output()
+		if err != nil {
+			t.Fatalf("display-message #{%s}: %v", option, err)
+		}
+		return strings.TrimSpace(string(out))
+	}
+
+	if runtime.GOOS == "windows" {
+		// reservedRowsFromStatus's own mapping, inlined here since it is unexported in reedengine:
+		// "off" -> 0, "on" -> 1, a non-negative integer string -> that integer verbatim.
+		status := strings.ToLower(readOption("status"))
+		reserved := 0
+		switch status {
+		case "off":
+			reserved = 0
+		case "on":
+			reserved = 1
+		default:
+			n, err := strconv.Atoi(status)
+			if err != nil || n < 0 {
+				t.Fatalf("#{status} = %q; want off, on, or a non-negative integer", status)
+			}
+			reserved = n
+		}
+
+		windowHeight, err := strconv.Atoi(readOption("window_height"))
+		if err != nil {
+			t.Fatalf("parse #{window_height}: %v", err)
+		}
+		lines := listPaneLines(t, tmuxPath, socket, session)
+		paneRowsSum := 0
+		for _, line := range lines {
+			fields := strings.Fields(line)
+			if len(fields) < 4 {
+				continue
+			}
+			height, err := strconv.Atoi(fields[3])
+			if err != nil {
+				t.Fatalf("parse pane_height %q: %v", fields[3], err)
+			}
+			paneRowsSum += height
+		}
+		// tmux draws one border row between each pair of vertically stacked panes.
+		borders := len(lines) - 1
+		if borders < 0 {
+			borders = 0
+		}
+		if paneRowsSum+borders+reserved != windowHeight {
+			t.Errorf("panes(%d) + borders(%d) + status-reserved(%d) = %d; want the live window height %d — whatever #{status} reads back must match the window the layout was actually planned against", paneRowsSum, borders, reserved, paneRowsSum+borders+reserved, windowHeight)
+		}
+		return
+	}
+
+	if got := readOption("status"); got != "on" {
+		t.Errorf("#{status} = %q; want \"on\"", got)
+	}
+	if got := readOption("status-position"); got != "bottom" {
+		t.Errorf("#{status-position} = %q; want \"bottom\"", got)
+	}
+	statusLeft := readOption("status-left")
+	if !strings.Contains(statusLeft, h.Location.RepoName) {
+		t.Errorf("#{status-left} = %q; want it to contain the repo name %q", statusLeft, h.Location.RepoName)
+	}
+	if !strings.Contains(statusLeft, h.Location.WorktreeName) {
+		t.Errorf("#{status-left} = %q; want it to contain the worktree name %q", statusLeft, h.Location.WorktreeName)
+	}
+}
diff --git a/internal/reedcli/smoke_staterecovery_test.go b/internal/reedcli/smoke_staterecovery_test.go
index ec02d1192..2ddd2cf36 100644
--- a/internal/reedcli/smoke_staterecovery_test.go
+++ b/internal/reedcli/smoke_staterecovery_test.go
@@ -544,3 +544,81 @@ func TestSmokeRemoveNeverKillsASiblingWorktreesPane(t *testing.T) {
 			victimPane, listPaneLines(t, tmuxPath, socket, victimSession))
 	}
 }
+
+// TestSmokeUpgradeFromAPreRenameStateFileHealsInOneOp pins the
+// no-migration-for-the-renamed-state-field Shared Decision's claim that an operator upgrading from a
+// pre-rename lyx sees one stale pane for less than one op.
+//
+// A pre-rename reed.json carried the old "headerPaneId" key rather than today's "selvagePaneId": that
+// key is deliberately not read (no compatibility shim, no dual-read, no migration step in
+// loadOrInitStateLocked), so on the first `up` after the upgrade SelvagePaneID reads empty,
+// ensureSelvagePaneLocked splits a fresh Selvage at the bottom, and the session's own reconcile —
+// authorized by that freshly alive Selvage — reaps the old header-shaped pane as untracked in the
+// SAME op, never a follow-up verb.
+func TestSmokeUpgradeFromAPreRenameStateFileHealsInOneOp(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+	t.Chdir(h.PrimeWorktree())
+	t.Cleanup(func() {
+		var buf bytes.Buffer
+		RunCLI(&buf, []string{"down"})
+	})
+
+	var out bytes.Buffer
+	if code := RunCLI(&out, []string{"up"}); code != 0 {
+		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
+	}
+	socket, session := socketAndSession(t)
+
+	statePath := filepath.Join(h.PrimeWorktree(), ".lyx", "reed.json")
+	raw, err := os.ReadFile(statePath)
+	if err != nil {
+		t.Fatalf("read %s: %v", statePath, err)
+	}
+	var fields map[string]any
+	if err := json.Unmarshal(raw, &fields); err != nil {
+		t.Fatalf("parse %s: %v", statePath, err)
+	}
+	oldSelvagePaneID, _ := fields["selvagePaneId"].(string)
+	if oldSelvagePaneID == "" {
+		t.Fatalf("freshly booted state at %s carries no selvagePaneId: %s", statePath, raw)
+	}
+	// Rewrite under the pre-rename key: the pane this names is genuinely alive (it is Selvage's own
+	// pane, freshly split by the up above), exactly the "live header-shaped pane" precondition — the
+	// point is that the KEY, not the pane's liveness, is what today's code no longer recognizes.
+	delete(fields, "selvagePaneId")
+	fields["headerPaneId"] = oldSelvagePaneID
+	rewritten, err := json.Marshal(fields)
+	if err != nil {
+		t.Fatalf("marshal rewritten state: %v", err)
+	}
+	if err := os.WriteFile(statePath, rewritten, 0o600); err != nil {
+		t.Fatalf("write rewritten state to %s: %v", statePath, err)
+	}
+
+	out.Reset()
+	if code := RunCLI(&out, []string{"up"}); code != 0 {
+		t.Fatalf("up after the pre-rename state file was restored = %d; want 0, output: %s", code, out.String())
+	}
+
+	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState after the healing up = (%+v, %v); want a freshly persisted SelvagePaneID", st, err)
+	}
+	if st.SelvagePaneID == oldSelvagePaneID {
+		t.Fatalf("SelvagePaneID after the healing up = %s; want a NEW id distinct from the pre-rename pane %s", st.SelvagePaneID, oldSelvagePaneID)
+	}
+
+	panes := listPaneLines(t, tmuxPath, socket, session)
+	if len(panes) != 1 || !paneLiveOnSession(panes, st.SelvagePaneID) {
+		t.Fatalf("panes after the healing up = %v; want exactly the freshly rebuilt Selvage pane %s", panes, st.SelvagePaneID)
+	}
+	for _, line := range panes {
+		fields := strings.Fields(line)
+		if len(fields) > 0 && fields[0] == oldSelvagePaneID {
+			t.Errorf("old pre-rename pane %s still present after the healing up; want it reaped in this same op", oldSelvagePaneID)
+		}
+	}
+}
diff --git a/internal/reedcli/smoke_headerseed_test.go b/internal/reedcli/smoke_statuslineseed_test.go
similarity index 69%
rename from internal/reedcli/smoke_headerseed_test.go
rename to internal/reedcli/smoke_statuslineseed_test.go
index b849d4d30..a417fe04c 100644
--- a/internal/reedcli/smoke_headerseed_test.go
+++ b/internal/reedcli/smoke_statuslineseed_test.go
@@ -1,12 +1,13 @@
 //go:build smoke
 
-// smoke_headerseed_test.go pins noise class 3's suppression directly: TestSmokeHeaderDeclinesStencilSeedPass
-// arranges both stencilstore Warn emitters cmd/lyx's root PersistentPreRunE can reach (the dev-refusal
-// warn and the port-back drift warn) and asserts that `lyx reed header`'s stderr is empty. No tmux,
-// pane, or escape sequence is anywhere in this picture: the assertion runs the built binary as a plain
-// subprocess and reads its own stderr stream directly, so this test is structurally incapable of being
-// masked by the `ED 3` scrollback backstop batch 3 adds — that backstop clears a pane's scrollback, and
-// there is no pane here for it to touch.
+// smoke_statuslineseed_test.go pins noise class 3's suppression directly:
+// TestSmokeStatuslineDeclinesStencilSeedPass arranges both stencilstore Warn emitters cmd/lyx's root
+// PersistentPreRunE can reach (the dev-refusal warn and the port-back drift warn) and asserts that
+// `lyx reed statusline`'s stderr is empty. No tmux, pane, or escape sequence is anywhere in this
+// picture: the assertion runs the built binary as a plain subprocess and reads its own stderr stream
+// directly. `clihelp.SkipStencilSeedAnnotation` survives on the replacement verb, and what this test
+// protects — a preview command leaving no stencilstore warnings and no git commits in the hub — is
+// still true and still worth pinning.
 
 package reedcli
 
@@ -23,14 +24,14 @@ import (
 	"github.com/Knatte18/loomyard/internal/stencilstore"
 )
 
-// TestSmokeHeaderDeclinesStencilSeedPass is P2, the batch's regression pin: a dev-stamped real binary
-// runs `lyx reed header` against a hub carrying both a stale-but-untouched board stencil (the
+// TestSmokeStatuslineDeclinesStencilSeedPass is the regression pin: a dev-stamped real binary runs
+// `lyx reed statusline` against a hub carrying both a stale-but-untouched board stencil (the
 // dev-refusal warn's precondition) and a drifted contracts/stencils worktree copy (the port-back
 // drift warn's precondition), and stderr must come back empty.
 // Assert emptiness only -- never a line count and never a particular message: either emitter alone is
 // enough to make stderr non-empty pre-fix, and post-fix stderr is silent because the pass does not run
 // at all for an opted-out command.
-func TestSmokeHeaderDeclinesStencilSeedPass(t *testing.T) {
+func TestSmokeStatuslineDeclinesStencilSeedPass(t *testing.T) {
 	lyxExe := buildLyxBinaryWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")
 
 	h := hubforge.NewHub(t, ".")
@@ -73,16 +74,16 @@ func TestSmokeHeaderDeclinesStencilSeedPass(t *testing.T) {
 		t.Fatalf("write contracts/stencils source %s: %v", sourcePath, err)
 	}
 
-	cmd := exec.Command(lyxExe, "reed", "header")
+	cmd := exec.Command(lyxExe, "reed", "statusline")
 	cmd.Dir = h.PrimeWorktree()
 	var stdout, stderr bytes.Buffer
 	cmd.Stdout = &stdout
 	cmd.Stderr = &stderr
 	if err := cmd.Run(); err != nil {
-		t.Fatalf("lyx reed header: %v; stdout: %s; stderr: %s", err, stdout.String(), stderr.String())
+		t.Fatalf("lyx reed statusline: %v; stdout: %s; stderr: %s", err, stdout.String(), stderr.String())
 	}
 
 	if stderr.Len() != 0 {
-		t.Errorf("lyx reed header stderr = %q; want empty -- the header must decline the root pre-run's stencil-seed pass entirely", stderr.String())
+		t.Errorf("lyx reed statusline stderr = %q; want empty -- statusline must decline the root pre-run's stencil-seed pass entirely", stderr.String())
 	}
 }
diff --git a/internal/reedcli/smoke_test.go b/internal/reedcli/smoke_test.go
index a30cb6c6b..2d0fbc3a8 100644
--- a/internal/reedcli/smoke_test.go
+++ b/internal/reedcli/smoke_test.go
@@ -658,20 +658,6 @@ func capturePane(t *testing.T, tmuxPath, socket, target string) string {
 	return string(out)
 }
 
-// capturePaneScrollback returns the target pane's full scrollback (via -S -), not merely its
-// visible viewport.
-// This is deliberately a separate helper from capturePane rather than an edit to it: capturePane
-// passes no -S and captures the visible viewport only, which is what its existing callers assert
-// against, whereas the header-noise assertions need the full scrollback that -S - reaches.
-func capturePaneScrollback(t *testing.T, tmuxPath, socket, target string) string {
-	t.Helper()
-	out, err := exec.Command(tmuxPath, "-L", socket, "capture-pane", "-p", "-S", "-", "-t", target).Output()
-	if err != nil {
-		t.Fatalf("capture-pane -S - -t %s: %v", target, err)
-	}
-	return string(out)
-}
-
 // sendKeysLine types text literally into the target pane and submits it with Enter.
 func sendKeysLine(t *testing.T, tmuxPath, socket, target, text string) {
 	t.Helper()
diff --git a/internal/reedcli/spawnwatchdog.go b/internal/reedcli/spawnwatchdog.go
new file mode 100644
index 000000000..cf02cfa2d
--- /dev/null
+++ b/internal/reedcli/spawnwatchdog.go
@@ -0,0 +1,59 @@
+// spawnwatchdog.go implements ensureWatchdogSpawned, the best-effort spawn attempt up, resume and
+// attach each make after their own engine op returns without error: it re-execs this same binary
+// as `lyx reed watchdog --hub-path <hub> --tmux <tmux>`, detached, so the spawned process outlives
+// this one and hosts the per-hub resize self-heal daemon for every worktree on the hub.
+
+package reedcli
+
+import (
+	"os"
+	"os/exec"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/logger"
+	"github.com/Knatte18/loomyard/internal/proc"
+)
+
+// ensureWatchdogSpawned attempts to spawn the per-hub watchdog daemon detached, best-effort.
+//
+// It returns immediately when suppressWatchdogSpawn is set (a test binary, where re-exec'ing
+// os.Executable() would run the whole suite recursively) or when hubPath is empty (this reedCLI
+// was never given a hub, e.g. the watchdog verb's own PersistentPreRunE early return).
+//
+// A spawn failure is never fatal to the caller's own operation: up, resume and attach each already
+// succeeded at their own engine op by the time this runs, and the daemon is a convenience the
+// operator can always start by hand (running `lyx reed watchdog` in the foreground) if this
+// best-effort spawn does not land.
+func (c *reedCLI) ensureWatchdogSpawned() {
+	if c.suppressWatchdogSpawn || c.hubPath == "" {
+		return
+	}
+
+	// lock.TryAcquireWriteLock does not create the lock file's parent directory, and a hub that has
+	// never booted a reed server may not have HubScratchDir yet.
+	scratchDir := fabricengine.HubScratchDir(c.hubPath)
+	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
+		logger.Warn("reed: could not create hub scratch dir, skipping watchdog spawn", "hub", c.hubPath, "path", scratchDir, "err", err)
+		return
+	}
+
+	exe, err := os.Executable()
+	if err != nil {
+		logger.Warn("reed: could not resolve this binary, skipping watchdog spawn", "hub", c.hubPath, "err", err)
+		return
+	}
+
+	cmd := exec.Command(exe, "reed", "watchdog", "--hub-path", c.hubPath, "--tmux", c.eng.TmuxPath())
+	// The daemon is per-hub and outlives the worktree that spawned it. On Windows a held cwd handle
+	// on a worktree directory blocks that directory's deletion, which would break fabric teardown —
+	// pinning cmd.Dir to the hub instead avoids that entirely.
+	cmd.Dir = c.hubPath
+	// Leave stdin/stdout/stderr nil so no parent handles are inherited.
+	proc.Detach(cmd)
+
+	logger.Info("reed: spawning detached per-hub watchdog", "exe", exe, "hub", c.hubPath, "tmux", c.eng.TmuxPath())
+	if err := cmd.Start(); err != nil { // intentionally not Wait()ed: a detached Start with no Wait
+		// logs the spawn alone, since there is no teardown to log.
+		logger.Warn("reed: watchdog spawn failed", "exe", exe, "hub", c.hubPath, "err", err)
+	}
+}
diff --git a/internal/reedcli/spawnwatchdog_test.go b/internal/reedcli/spawnwatchdog_test.go
new file mode 100644
index 000000000..2902c94c9
--- /dev/null
+++ b/internal/reedcli/spawnwatchdog_test.go
@@ -0,0 +1,43 @@
+// spawnwatchdog_test.go pins ensureWatchdogSpawned's no-spawn early returns and its lock-path
+// target, spawning no subprocess and driving no live tmux at all — per the Test Tier Purity
+// Invariant an untagged test file spawns nothing; the daemon's live spawn/lock behaviour is card
+// 41's integration suite.
+
+package reedcli
+
+import (
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/reedengine"
+)
+
+func TestEnsureWatchdogSpawned_SuppressedReturnsWithoutSpawning(t *testing.T) {
+	c := &reedCLI{
+		eng:                   reedengine.New(reedengine.Config{}, reedengine.Geometry{}),
+		hubPath:               t.TempDir(),
+		suppressWatchdogSpawn: true,
+	}
+	// A no-op call: if this spawned anything, it would re-exec this test binary, which would hang
+	// or recurse the whole suite. Reaching the end of this test at all is the assertion.
+	c.ensureWatchdogSpawned()
+}
+
+func TestEnsureWatchdogSpawned_EmptyHubPathReturnsWithoutSpawning(t *testing.T) {
+	c := &reedCLI{
+		eng:                   reedengine.New(reedengine.Config{}, reedengine.Geometry{}),
+		hubPath:               "",
+		suppressWatchdogSpawn: false,
+	}
+	c.ensureWatchdogSpawned()
+}
+
+func TestWatchdogLockPath_IsHubScratchDirLockFile(t *testing.T) {
+	hub := t.TempDir()
+	want := filepath.Join(fabricengine.HubScratchDir(hub), watchdogLockFileName)
+	got := filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock")
+	if got != want {
+		t.Errorf("watchdog lock path = %q; want %q", got, want)
+	}
+}
diff --git a/internal/reedcli/statusline.go b/internal/reedcli/statusline.go
new file mode 100644
index 000000000..820778afd
--- /dev/null
+++ b/internal/reedcli/statusline.go
@@ -0,0 +1,57 @@
+// statusline.go implements the `statusline` reed verb: it renders this hub's tmux status-line text
+// via the engine's tokenvocab-backed pipeline and returns it through the normal JSON envelope.
+// It carries clihelp.SkipStencilSeedAnnotation, declining cmd/lyx's root pre-run stencil-seed
+// pass: this is deliberate even though this command is a plain preview rather than a keepalive —
+// neither this command nor the gate reads a stencil, and declining keeps the hub free of a
+// preview command's git commits.
+
+package reedcli
+
+import (
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/output"
+	"github.com/spf13/cobra"
+)
+
+// statuslineCmd builds the `statusline` subcommand: calls c.eng.StatusLineText() and returns it
+// through the JSON envelope.
+func (c *reedCLI) statuslineCmd() *cobra.Command {
+	cmd := &cobra.Command{
+		Use:   "statusline",
+		Short: "render this hub's tmux status-line text",
+		Long: `statusline previews the rendered status-line text over this hub's configured
+template (or the embedded default), the same tokenvocab pipeline
+Engine.ValidateStatusLine checks eagerly at boot.
+
+The rendered text now lives in a tmux option that pinGeometryOptionsLocked
+rewrites on every boot and every attach, so this preview is never stale
+against a running session the way the old header pane's once-at-launch
+render was: editing status_line.template in reed.yaml and re-running this
+verb always shows what the next boot or attach will paint.
+
+Example:
+  lyx reed statusline`,
+		Annotations: map[string]string{
+			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
+		},
+		RunE: func(cmd *cobra.Command, args []string) error {
+			if clihelp.ShouldAbort(cmd.Context()) {
+				return nil
+			}
+			out := cmd.OutOrStdout()
+
+			text, err := c.eng.StatusLineText()
+			if err != nil {
+				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
+				return nil
+			}
+
+			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
+				"text": text,
+			}))
+			return nil
+		},
+	}
+
+	return cmd
+}
diff --git a/internal/reedcli/statusline_test.go b/internal/reedcli/statusline_test.go
new file mode 100644
index 000000000..8617e1bc8
--- /dev/null
+++ b/internal/reedcli/statusline_test.go
@@ -0,0 +1,54 @@
+// statusline_test.go covers the `statusline` verb's pure command construction (Use, Short) and its
+// enveloped RunE, which calls c.eng.StatusLineText() and returns the rendered text on the JSON
+// envelope under the "text" key.
+// It never drives the verb through RunCLI: that reaches reed's PersistentPreRunE and therefore
+// lyxcwd.Resolve, which spawns "git rev-parse", banned in the untagged suite by the Test Tier
+// Purity Invariant. The end-to-end PreRunE -> StatusLineText round trip is covered by the reed smoke
+// suite instead.
+
+package reedcli
+
+import (
+	"bytes"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/reedengine"
+)
+
+func TestStatuslineCmd_UseAndShort(t *testing.T) {
+	c := &reedCLI{}
+	cmd := c.statuslineCmd()
+
+	if cmd.Use != "statusline" {
+		t.Errorf("statuslineCmd().Use = %q; want %q", cmd.Use, "statusline")
+	}
+	if cmd.Short == "" {
+		t.Error("statuslineCmd().Short is empty; want a non-empty short description")
+	}
+}
+
+// newStatuslineTestCLI builds a reedCLI whose Engine is real enough to run statuslineCmd's RunE:
+// StatusLineText dereferences e.cfg unconditionally, so the bare &reedCLI{} shape
+// TestStatuslineCmd_UseAndShort uses would panic here. An empty Config.StatusLine.Template falls
+// back to the embedded default template, and RepoName/HubPath are the only two Geometry fields
+// tokenvocab.Ctx consumes, so this renders cleanly with no filesystem or process I/O.
+func newStatuslineTestCLI(t *testing.T) *reedCLI {
+	t.Helper()
+	return &reedCLI{eng: reedengine.New(reedengine.Config{}, reedengine.Geometry{RepoName: "test-repo", WorktreeName: "test-worktree", HubPath: t.TempDir()})}
+}
+
+func TestStatuslineCmd_ReturnsRenderedTextOnEnvelope(t *testing.T) {
+	c := newStatuslineTestCLI(t)
+	cmd := c.statuslineCmd()
+	buf := &bytes.Buffer{}
+	cmd.SetOut(buf)
+	cmd.SetArgs([]string{})
+
+	if err := cmd.Execute(); err != nil {
+		t.Fatalf("cmd.Execute() = %v; want nil", err)
+	}
+	if !strings.Contains(buf.String(), `"text"`) {
+		t.Errorf("statusline output = %q; want the JSON envelope with a \"text\" field", buf.String())
+	}
+}
diff --git a/internal/reedcli/testmain_test.go b/internal/reedcli/testmain_test.go
index c109fc40e..258085af7 100644
--- a/internal/reedcli/testmain_test.go
+++ b/internal/reedcli/testmain_test.go
@@ -2,27 +2,18 @@
 // gitkit.HermeticGitEnv() runs once before any test, so reedcli's git-spawning fixtures never
 // inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
 // Invariant).
-// It also guards the binary against being run AS lyx by a header pane (see TestMain).
 
 package reedcli
 
 import (
-	"fmt"
 	"os"
 	"testing"
-	"time"
 
 	"github.com/Knatte18/loomyard/internal/gitkit"
 )
 
-// TestMain intercepts the header-pane invocation and prevents re-execution recursion.
+// TestMain arms the hermetic git test environment before any test runs.
 func TestMain(m *testing.M) {
-	if len(os.Args) > 1 && os.Args[1] == "reed" {
-		fmt.Println("reedcli test binary standing in for the header keepalive (`lyx reed header --blocking`)")
-		for {
-			time.Sleep(time.Hour)
-		}
-	}
 	gitkit.HermeticGitEnv()
 	os.Exit(m.Run())
 }
diff --git a/internal/reedcli/up.go b/internal/reedcli/up.go
index 377c9f113..11cc0db93 100644
--- a/internal/reedcli/up.go
+++ b/internal/reedcli/up.go
@@ -48,6 +48,10 @@ Example:
 				return nil
 			}
 
+			// Attempted after Up returns without error, before the envelope write: the daemon
+			// needs a live session to discover, and up is exactly the op that ensures one exists.
+			c.ensureWatchdogSpawned()
+
 			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
 				"session": result.Session,
 				"socket":  result.Socket,
diff --git a/internal/reedcli/watchdog.go b/internal/reedcli/watchdog.go
new file mode 100644
index 000000000..c93809c5a
--- /dev/null
+++ b/internal/reedcli/watchdog.go
@@ -0,0 +1,320 @@
+// watchdog.go implements the per-hub watchdog daemon: the `lyx reed watchdog` verb, its discovery
+// loop, and the pure seams that make discovery and the idle-exit rule unit-testable without a live
+// tmux server.
+//
+// internal/reedcli owns the daemon outright, per the discussion's
+// told-geometry-keeps-the-daemon-out-of-reedengine decision: CONSTRAINTS.md's Told-Geometry
+// Invariant bars internal/reedengine from importing internal/lyxcwd, and reedcli already holds the
+// *lyxcwd.Location and already imports internal/hubgeom, so it may import internal/fabricengine
+// directly as hubgeom does. internal/reedengine gains exactly one new engine-less function
+// (ListSessions) and learns nothing about the daemon's existence.
+
+package reedcli
+
+import (
+	"context"
+	"io"
+	"path/filepath"
+	"strings"
+	"time"
+
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/hubgeom"
+	"github.com/Knatte18/loomyard/internal/lock"
+	"github.com/Knatte18/loomyard/internal/logger"
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+	"github.com/Knatte18/loomyard/internal/output"
+	"github.com/Knatte18/loomyard/internal/reedengine"
+	"github.com/spf13/cobra"
+)
+
+// watchdogHubDiscoveryCycle is how often the daemon's outer loop runs one list-sessions round trip
+// against its hub socket.
+//
+// It lives here, beside the discovery loop that consumes it, rather than alongside
+// internal/reedengine/watchdog.go's existing unexported watchdog* constants: those govern
+// Engine.Watch's own internals, which reedengine still owns, and exporting two constants from that
+// package purely so another package could read them would put the timings somewhere their only
+// consumer is not.
+//
+// Five seconds is well below any human `down` + `up` gap while being an order of magnitude slower
+// than the 100ms signal tick a per-worktree Engine.Watch already polls at, so discovery costs one
+// list-sessions round trip per hub every five seconds rather than riding the per-worktree tick.
+const watchdogHubDiscoveryCycle = 5 * time.Second
+
+// watchdogHubIdleCycles is how many consecutive idle discovery cycles (see sessionsAreIdle) the
+// daemon tolerates before exiting.
+//
+// Three cycles covers a `down` immediately followed by an `up` without the daemon dying and
+// respawning in between.
+const watchdogHubIdleCycles = 3
+
+// sessionsAreIdle reports whether one discovery cycle's list-sessions round trip counts toward the
+// daemon's idle-exit counter.
+//
+// It returns false only when err is nil and names is non-empty — an affirmative listing. Every
+// other combination is idle: an exit-0 empty listing, a "no server running" error, and any other
+// list-sessions failure alike.
+//
+// This totality is forced rather than merely tolerated (Shared Decision
+// daemon-idle-rule-is-anything-but-an-affirmative-listing): the normal last-`down` case is an
+// ERROR, not an empty list, so a rule counting only exit-0-empty would leave the daemon's own main
+// exit path undefined. And internal/reedengine/proctree_windows.go records that psmux exits
+// identically with and without a server, so a rule distinguishing a no-server error from a
+// transient one would be unimplementable there.
+func sessionsAreIdle(names []string, err error) bool {
+	return !(err == nil && len(names) > 0)
+}
+
+// watchedSession is one worktree session the daemon has entered: the *reedengine.Engine built for
+// it, plus the cancel for the goroutine (if any) running Engine.Watch on its behalf.
+//
+// cancel is nil for a `watchdog: off` worktree: enterSession starts no goroutine for it at all, but
+// the entry is still kept so departure bookkeeping (runWatchdogLoop) stays uniform across enabled
+// and disabled worktrees.
+type watchedSession struct {
+	eng    *reedengine.Engine
+	cancel context.CancelFunc
+}
+
+// planSessionDiff is the daemon's TDD discovery seam: it compares one discovery cycle's live
+// session names against the daemon's own known set and reports which names newly appeared and
+// which known names are no longer live.
+//
+// It is pure — no tmux, no filesystem — so it is unit-testable with no fixture at all.
+func planSessionDiff(live []string, known map[string]watchedSession) (appeared, departed []string) {
+	liveSet := make(map[string]bool, len(live))
+	for _, name := range live {
+		liveSet[name] = true
+		if _, ok := known[name]; !ok {
+			appeared = append(appeared, name)
+		}
+	}
+	for name := range known {
+		if !liveSet[name] {
+			departed = append(departed, name)
+		}
+	}
+	return appeared, departed
+}
+
+// resolveWatchedSession resolves a live tmux session name back to the *lyxcwd.Location of the
+// worktree it belongs to, by a direct join rather than a scan.
+//
+// The join is exact, not a candidate set, because hub-mode reedengine.SessionName(worktreeRoot) is
+// filepath.Base(worktreeRoot) verbatim, and validateToldTmuxIdentity refuses an unusable name
+// instead of sanitizing it. With hub being filepath.Dir(worktreeRoot), filepath.Join(hub,
+// sessionName) recovers the worktree root exactly. lyxcwd.ResolveWorktree is still required on top
+// of the join: hubgeom.ReedGeometry needs a resolved *lyxcwd.Location, and ResolveWorktree is also
+// the gate that rejects a session name that does not name a worktree at all.
+func resolveWatchedSession(hub, sessionName string) (*lyxcwd.Location, error) {
+	worktreeRoot := filepath.Join(hub, sessionName)
+	// The direct join is a git spawn inside a polling probe, so it is logged at Debug per
+	// CONSTRAINTS.md's Live-Substrate Spawn Observability rule.
+	logger.Debug("reed: watchdog resolving worktree for session", "hub", hub, "session", sessionName)
+	return lyxcwd.ResolveWorktree(worktreeRoot)
+}
+
+// enterSession builds the watchedSession entry for a newly-appeared live tmux session name,
+// starting its Engine.Watch goroutine unless the worktree's own config says watchdog: off.
+//
+// LoadConfig, not a strict load, is used deliberately: a worktree with no _lyx still resolves the
+// embedded template, which keeps reedengine on the degrading side of the Config Strictness
+// Invariant rather than refusing a worktree the daemon should otherwise watch.
+//
+// A watchdog: off worktree still gets an entry, with a nil cancel and no goroutine started at all:
+// watchLoop answers a disabled watchdog by blocking on <-ctx.Done() rather than returning, so
+// starting one here would cost a goroutine per disabled worktree to do nothing — while keeping the
+// entry still keeps the session known and keeps departure bookkeeping (runWatchdogLoop) uniform
+// across enabled and disabled worktrees.
+func enterSession(hub, tmuxPath, sessionName string) (watchedSession, error) {
+	location, err := resolveWatchedSession(hub, sessionName)
+	if err != nil {
+		return watchedSession{}, err
+	}
+
+	geom := hubgeom.ReedGeometry(location)
+	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
+	if err != nil {
+		return watchedSession{}, err
+	}
+	eng := reedengine.New(cfg, geom)
+
+	// A worktree reaches the daemon's discovery loop only after "lyx reed up" already booted it,
+	// and that boot already validated cfg.Watchdog loudly (ensureServerAndSessionLocked), so the
+	// only value this needs to recognize here is the "off" spelling itself — every other value,
+	// valid or not, behaves as enabled, matching Engine.Watch's own fail-safe-toward-watching
+	// posture for a value it cannot parse.
+	if strings.EqualFold(strings.TrimSpace(cfg.Watchdog), "off") {
+		return watchedSession{eng: eng}, nil
+	}
+
+	ctx, cancel := context.WithCancel(context.Background())
+	go func() {
+		if err := eng.Watch(ctx); err != nil {
+			logger.Debug("reed: watchdog's watch loop returned", "session", sessionName, "err", err)
+		}
+	}()
+	return watchedSession{eng: eng, cancel: cancel}, nil
+}
+
+// runWatchdogLoop is the daemon's outer loop: it polls hub for live sessions every
+// watchdogHubDiscoveryCycle, enters newly-appeared sessions, tears down departed ones, and returns
+// once watchdogHubIdleCycles consecutive cycles are idle (see sessionsAreIdle) or ctx is done.
+//
+// Teardown on departure is not optional: Engine.Watch never returns while its context is live, so
+// without cancelling a departed entry's goroutine, a worktree whose session goes away while
+// siblings remain would leave a goroutine polling a dead session for the daemon's whole remaining
+// lifetime. Cancelling on departure is also what makes "re-entry re-reads config" true at all:
+// watchLoop reads cfg.Watchdog exactly once at start, so a flipped watchdog: value only takes
+// effect once the entry leaves (this departure teardown) and re-enters (enterSession, on the next
+// appearance) — there is no other re-read path.
+func runWatchdogLoop(ctx context.Context, hub, tmuxPath string) error {
+	logger.Info("reed: watchdog daemon starting", "hub", hub)
+
+	known := make(map[string]watchedSession)
+	defer func() {
+		for name, ws := range known {
+			if ws.cancel != nil {
+				ws.cancel()
+			}
+			logger.Debug("reed: watchdog stopped watching session on daemon exit", "hub", hub, "session", name)
+		}
+	}()
+
+	ticker := time.NewTicker(watchdogHubDiscoveryCycle)
+	defer ticker.Stop()
+
+	idleCycles := 0
+	for {
+		select {
+		case <-ctx.Done():
+			return ctx.Err()
+		case <-ticker.C:
+		}
+
+		live, err := reedengine.ListSessions(tmuxPath, reedengine.ServerName(hub))
+		if sessionsAreIdle(live, err) {
+			idleCycles++
+			if idleCycles >= watchdogHubIdleCycles {
+				logger.Info("reed: watchdog daemon exiting after consecutive idle discovery cycles", "hub", hub, "cycles", idleCycles)
+				return nil
+			}
+			continue
+		}
+		idleCycles = 0
+
+		appeared, departed := planSessionDiff(live, known)
+		for _, name := range appeared {
+			ws, err := enterSession(hub, tmuxPath, name)
+			if err != nil {
+				// A name that does not resolve to a worktree is skipped, not retried until it next
+				// re-appears in the live set: resolveWatchedSession's own ResolveWorktree call is
+				// what actually failed, so there is nothing more to learn by retrying immediately.
+				logger.Debug("reed: watchdog could not enter session, skipping", "hub", hub, "session", name, "err", err)
+				continue
+			}
+			known[name] = ws
+			logger.Debug("reed: watchdog entered session", "hub", hub, "session", name)
+		}
+		for _, name := range departed {
+			ws, ok := known[name]
+			if !ok {
+				continue
+			}
+			if ws.cancel != nil {
+				ws.cancel()
+			}
+			delete(known, name)
+			logger.Debug("reed: watchdog session departed", "hub", hub, "session", name)
+		}
+	}
+}
+
+// watchdogLockFileName is the daemon's single-instance lock file's name inside
+// fabricengine.HubScratchDir(hub).
+const watchdogLockFileName = "reed-watchdog.lock"
+
+// watchdogCmd builds the `watchdog` subcommand: a blocking, single-instance, per-hub daemon that
+// runs runWatchdogLoop until it idles out or its context is cancelled.
+//
+// Everything fallible runs pre-flight, on the envelope, before the command blocks: an absent or
+// non-absolute --hub-path and an empty --tmux each report through output.Err, and lock contention
+// (another daemon already holds the lock) exits 0 rather than erroring, since a racing spawn
+// costing one short-lived process is the expected, harmless outcome.
+func (c *reedCLI) watchdogCmd() *cobra.Command {
+	var hubPath, tmuxPath string
+
+	cmd := &cobra.Command{
+		Use:   "watchdog",
+		Short: "run the blocking, single-instance, per-hub watchdog daemon",
+		Long: `watchdog is the detached, single-instance, per-hub daemon that hosts reed's
+resize self-heal watch loop for every worktree session on the hub named by
+--hub-path. It is told its hub path and the tmux binary to use on its
+command line — it opts out of reed's normal cwd/location/config resolution
+entirely and must never derive either from its own environment.
+
+up, resume and attach each attempt to spawn this daemon detached after
+their own engine op returns without error; a spawn that finds the lock
+already held exits 0 immediately. Running it directly in the foreground
+is a real diagnosis path: its diagnostics land in the hub's durable log
+directory rather than nowhere, since its own stdio is discarded before it
+starts polling.
+
+Example:
+  lyx reed watchdog --hub-path /abs/path/to/hub --tmux /usr/bin/tmux`,
+		RunE: func(cmd *cobra.Command, args []string) error {
+			if clihelp.ShouldAbort(cmd.Context()) {
+				return nil
+			}
+			out := cmd.OutOrStdout()
+
+			if hubPath == "" || !filepath.IsAbs(hubPath) {
+				clihelp.SetExit(cmd.Context(), output.Err(out, "--hub-path must be an absolute, non-empty path"))
+				return nil
+			}
+			if tmuxPath == "" {
+				clihelp.SetExit(cmd.Context(), output.Err(out, "--tmux must not be empty"))
+				return nil
+			}
+
+			// The durable sink is pointed FIRST, before stderr is discarded: its cwd-anchored
+			// fallback arms only inside a lyx-owned worktree and never from a hub cwd, which is
+			// exactly where cmd.Dir pins this process when it is spawned detached — without this
+			// explicit call, every diagnostic the design leans on would go nowhere.
+			logger.SetDurableSinkDir(fabricengine.HubLogsDir(hubPath))
+			// The daemon's own stdio is not a screen anyone watches; only the durable sink matters
+			// from here on.
+			logger.SetOutput(io.Discard)
+
+			lockPath := filepath.Join(fabricengine.HubScratchDir(hubPath), watchdogLockFileName)
+			fl, acquired, err := lock.TryAcquireWriteLock(lockPath)
+			if err != nil {
+				// A non-nil error is a FAILURE, not contention: the lock path itself is unusable
+				// (an absent parent directory, permissions, a read-only filesystem), which must
+				// never fail silently.
+				logger.Error("reed: watchdog could not acquire its lock", "hub", hubPath, "lock", lockPath, "err", err)
+				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
+				return nil
+			}
+			if !acquired {
+				// Contention: another daemon already holds the lock. This costs one short-lived
+				// process and nothing else.
+				logger.Info("reed: watchdog lock already held, exiting", "hub", hubPath, "lock", lockPath)
+				return nil
+			}
+			defer fl.Release()
+
+			if err := runWatchdogLoop(cmd.Context(), hubPath, tmuxPath); err != nil {
+				logger.Warn("reed: watchdog daemon's loop returned", "hub", hubPath, "err", err)
+			}
+			return nil
+		},
+	}
+
+	cmd.Flags().StringVar(&hubPath, "hub-path", "", "absolute path to the hub this daemon watches (required)")
+	cmd.Flags().StringVar(&tmuxPath, "tmux", "", "path to the tmux binary this daemon uses (required)")
+
+	return cmd
+}
diff --git a/internal/reedcli/watchdog_integration_test.go b/internal/reedcli/watchdog_integration_test.go
new file mode 100644
index 000000000..a7301f52c
--- /dev/null
+++ b/internal/reedcli/watchdog_integration_test.go
@@ -0,0 +1,653 @@
+//go:build integration
+
+// watchdog_integration_test.go carries the watchdog daemon's live-behaviour assertions: discovery
+// against a real hub with real tmux sessions, the single-instance lock's two outcomes, the idle-exit
+// timer, departure teardown, and re-entry re-reading a flipped watchdog: config value.
+//
+// It is a separate file from watchdog_test.go, rather than tagged content inside it, because a Go
+// build tag is per-file: tagging watchdog_test.go itself would hide its pure-seam tests from the
+// untagged tier where they belong. The hub fixture is built through internal/hubforge per the
+// hubforge Fabric-Fixture Invariant, never hand-assembled.
+//
+// ensureWatchdogSpawned's own os.Executable() re-exec is deliberately NOT exercised live here: under
+// `go test`, os.Executable() resolves to the test binary itself, and re-execing it with
+// reed-watchdog-shaped args is exactly the recursive-whole-suite hazard suppressWatchdogSpawn exists
+// to prevent (see cli.go's doc comment on that field). This file instead drives watchdogCmd()'s RunE
+// and runWatchdogLoop directly, in-process, which is the daemon's own live behaviour; the spawn call
+// site's wiring is pinned by cli_test.go/spawnwatchdog_test.go's non-integration tests, and the
+// "attach/resume attempt the spawn" assertion below drives ensureWatchdogSpawned itself (not the
+// os.Executable() re-exec) far enough to observe it was actually invoked rather than suppressed.
+package reedcli
+
+import (
+	"bytes"
+	"context"
+	"encoding/json"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strconv"
+	"strings"
+	"testing"
+	"time"
+
+	"gopkg.in/yaml.v3"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/hubforge"
+	"github.com/Knatte18/loomyard/internal/hubgeom"
+	"github.com/Knatte18/loomyard/internal/lyxcwd"
+	"github.com/Knatte18/loomyard/internal/reedengine"
+)
+
+// watchdogIntegrationTmux resolves the configured multiplexer binary, skipping the calling test when
+// it is absent so this file never hard-fails on a machine without tmux.
+func watchdogIntegrationTmux(t *testing.T, cfg reedengine.Config) string {
+	t.Helper()
+	if _, err := exec.LookPath(cfg.Tmux); err != nil {
+		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
+	}
+	return cfg.Tmux
+}
+
+// watchdogIntegrationEngine boots a real engine for worktreeRoot and returns it, its config, and a
+// cleanup that tears the session down.
+func watchdogIntegrationEngine(t *testing.T, worktreeRoot string) *reedengine.Engine {
+	t.Helper()
+	location, err := lyxcwd.ResolveWorktree(worktreeRoot)
+	if err != nil {
+		t.Fatalf("ResolveWorktree(%s): %v", worktreeRoot, err)
+	}
+	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
+	if err != nil {
+		t.Fatalf("LoadConfig: %v", err)
+	}
+	watchdogIntegrationTmux(t, cfg)
+	geom := hubgeom.ReedGeometry(location)
+	eng := reedengine.New(cfg, geom)
+	if _, err := eng.Up(); err != nil {
+		t.Fatalf("eng.Up(): %v", err)
+	}
+	t.Cleanup(func() {
+		_, _ = eng.Down()
+	})
+	return eng
+}
+
+// runWatchdogCmdInBackground starts watchdogCmd() against hub/tmuxPath on a cancellable context and
+// returns the cancel func plus a channel that receives RunE's error (or nil) once it returns.
+func runWatchdogCmdInBackground(t *testing.T, hub, tmuxPath string) (cancel context.CancelFunc, done chan error, out *bytes.Buffer) {
+	t.Helper()
+	c := &reedCLI{}
+	cmd := c.watchdogCmd()
+	buf := &bytes.Buffer{}
+	cmd.SetOut(buf)
+	cmd.SetArgs([]string{"--hub-path", hub, "--tmux", tmuxPath})
+
+	ctx, cancelFn := context.WithCancel(context.Background())
+	done = make(chan error, 1)
+	go func() {
+		done <- cmd.ExecuteContext(ctx)
+	}()
+	return cancelFn, done, buf
+}
+
+func TestWatchdogIntegration_SingleInstanceLockContentionAndUnusableLockPath(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
+	if err != nil {
+		t.Fatalf("LoadConfig: %v", err)
+	}
+	tmuxPath := watchdogIntegrationTmux(t, cfg)
+
+	cancel1, done1, _ := runWatchdogCmdInBackground(t, h.Path, tmuxPath)
+	defer cancel1()
+
+	// Give the first daemon time to actually acquire the lock before the second attempt races it.
+	deadline := time.Now().Add(10 * time.Second)
+	lockPath := filepath.Join(fabricengine.HubScratchDir(h.Path), watchdogLockFileName)
+	for {
+		if _, err := os.Stat(lockPath); err == nil {
+			break
+		}
+		if time.Now().After(deadline) {
+			t.Fatalf("watchdog lock file %s never appeared", lockPath)
+		}
+		time.Sleep(50 * time.Millisecond)
+	}
+
+	// A second attempt against the SAME hub while the first holds the lock must exit 0 (contention),
+	// never taking the lock.
+	c2 := &reedCLI{}
+	cmd2 := c2.watchdogCmd()
+	buf2 := &bytes.Buffer{}
+	cmd2.SetOut(buf2)
+	cmd2.SetArgs([]string{"--hub-path", h.Path, "--tmux", tmuxPath})
+	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
+	defer cancel2()
+	if err := cmd2.ExecuteContext(ctx2); err != nil {
+		t.Errorf("second watchdog attempt returned error %v; want nil (contention exits 0)", err)
+	}
+
+	// An unusable lock path (a hub path whose HubScratchDir cannot be created because a FILE sits
+	// where an intermediate directory component must go) exits non-zero.
+	unusableHub := unusableHubPath(t)
+	c3 := &reedCLI{}
+	cmd3 := c3.watchdogCmd()
+	buf3 := &bytes.Buffer{}
+	cmd3.SetOut(buf3)
+	cmd3.SetArgs([]string{"--hub-path", unusableHub, "--tmux", tmuxPath})
+	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
+	defer cancel3()
+	_ = cmd3.ExecuteContext(ctx3)
+	var env map[string]any
+	if err := json.Unmarshal(bytes.TrimSpace(buf3.Bytes()), &env); err != nil {
+		t.Fatalf("unusable-lock-path output is not valid JSON: %v; got: %q", err, buf3.String())
+	}
+	if ok, _ := env["ok"].(bool); ok {
+		t.Errorf("unusable lock path ok = true; want false")
+	}
+
+	cancel1()
+	<-done1
+}
+
+func TestWatchdogIntegration_DiscoversAndDropsDepartedSessions(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	hubforge.AddPair(t, h, "watchdog-second")
+
+	eng1 := watchdogIntegrationEngine(t, h.PrimeWorktree())
+	eng2 := watchdogIntegrationEngine(t, h.PairWarpWorktree("watchdog-second"))
+
+	tmuxPath := eng1.TmuxPath()
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+
+	loopDone := make(chan error, 1)
+	go func() {
+		loopDone <- runWatchdogLoop(ctx, h.Path, tmuxPath)
+	}()
+
+	// Both sessions must be discovered within a couple of discovery cycles.
+	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
+		names, err := reedengine.ListSessions(tmuxPath, reedengine.ServerName(h.Path))
+		if err != nil {
+			return false
+		}
+		found1, found2 := false, false
+		for _, n := range names {
+			if n == eng1.SessionName() {
+				found1 = true
+			}
+			if n == eng2.SessionName() {
+				found2 = true
+			}
+		}
+		return found1 && found2
+	})
+
+	// Tear eng1's session down; the daemon must stop touching it while eng2's session keeps being
+	// watched. This is observed indirectly: eng2 stays discoverable via ListSessions after eng1's
+	// session is gone, and the loop itself keeps running (it does not idle-exit, since eng2 is still
+	// live).
+	if _, err := eng1.Down(); err != nil {
+		t.Fatalf("eng1.Down(): %v", err)
+	}
+
+	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
+		names, err := reedengine.ListSessions(tmuxPath, reedengine.ServerName(h.Path))
+		if err != nil {
+			return false
+		}
+		for _, n := range names {
+			if n == eng1.SessionName() {
+				return false
+			}
+		}
+		for _, n := range names {
+			if n == eng2.SessionName() {
+				return true
+			}
+		}
+		return false
+	})
+
+	select {
+	case err := <-loopDone:
+		t.Fatalf("runWatchdogLoop exited early (err=%v); want it still running with eng2's session live", err)
+	default:
+	}
+
+	cancel()
+	<-loopDone
+}
+
+func TestWatchdogIntegration_ExitsAfterIdleCyclesAndReleasesLock(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	eng := watchdogIntegrationEngine(t, h.PrimeWorktree())
+	tmuxPath := eng.TmuxPath()
+
+	cancel, done, _ := runWatchdogCmdInBackground(t, h.Path, tmuxPath)
+	defer cancel()
+
+	lockPath := filepath.Join(fabricengine.HubScratchDir(h.Path), watchdogLockFileName)
+	waitForCondition(t, 10*time.Second, func() bool {
+		_, err := os.Stat(lockPath)
+		return err == nil
+	})
+
+	if _, err := eng.Down(); err != nil {
+		t.Fatalf("eng.Down(): %v", err)
+	}
+
+	// The daemon must exit on its own within watchdogHubIdleCycles*watchdogHubDiscoveryCycle plus
+	// slack, and release its lock.
+	slack := 10 * time.Second
+	select {
+	case err := <-done:
+		if err != nil {
+			t.Errorf("watchdogCmd RunE returned %v; want nil", err)
+		}
+	case <-time.After(watchdogHubIdleCycles*watchdogHubDiscoveryCycle + slack):
+		t.Fatal("watchdog daemon did not exit after its last session went away")
+	}
+
+	// A later spawn attempt must be able to take the now-released lock.
+	c2 := &reedCLI{}
+	cmd2 := c2.watchdogCmd()
+	buf2 := &bytes.Buffer{}
+	cmd2.SetOut(buf2)
+	cmd2.SetArgs([]string{"--hub-path", h.Path, "--tmux", tmuxPath})
+	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
+	defer cancel2()
+	_ = cmd2.ExecuteContext(ctx2)
+}
+
+func TestWatchdogIntegration_OffWorktreeEntersKnownButStartsNoWatcher(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+
+	location, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
+	if err != nil {
+		t.Fatalf("ResolveWorktree: %v", err)
+	}
+	defaultCfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
+	if err != nil {
+		t.Fatalf("LoadConfig: %v", err)
+	}
+	watchdogIntegrationTmux(t, defaultCfg)
+
+	// enterSession loads its own config straight off disk (reedengine.LoadConfig), so the override
+	// must actually be seeded into the fixture's reed.yaml — the whole resolved config, every key
+	// present, since a partial override fails LoadConfig's strictness check.
+	defaultCfg.Watchdog = "off"
+	seeded, err := yaml.Marshal(defaultCfg)
+	if err != nil {
+		t.Fatalf("marshal seeded config: %v", err)
+	}
+	hubforge.SeedConfig(t, h, map[string]string{"reed": string(seeded)})
+
+	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
+	if err != nil {
+		t.Fatalf("LoadConfig after seeding: %v", err)
+	}
+	if cfg.Watchdog != "off" {
+		t.Fatalf("seeded config's Watchdog = %q; want %q (fixture seeding did not take)", cfg.Watchdog, "off")
+	}
+	geom := hubgeom.ReedGeometry(location)
+	eng := reedengine.New(cfg, geom)
+	if _, err := eng.Up(); err != nil {
+		t.Fatalf("eng.Up(): %v", err)
+	}
+	t.Cleanup(func() { _, _ = eng.Down() })
+
+	ws, err := enterSession(h.Path, eng.TmuxPath(), eng.SessionName())
+	if err != nil {
+		t.Fatalf("enterSession: %v", err)
+	}
+	if ws.cancel != nil {
+		t.Error("enterSession() for a watchdog:off worktree returned a non-nil cancel; want nil (no goroutine started)")
+	}
+	if ws.eng == nil {
+		t.Error("enterSession() for a watchdog:off worktree returned a nil eng; want a built Engine so departure bookkeeping stays uniform")
+	}
+}
+
+// watchdogWindowSize reads session's live #{window_width} and #{window_height} via a plain tmux
+// display-message, bypassing the engine entirely so a test can observe either worktree's window from
+// outside both — exactSessionWindowTarget's "=<name>:" form is used so two sessions sharing this
+// hub's one socket can never prefix-match each other.
+func watchdogWindowSize(t *testing.T, tmuxPath, socket, session string) (w, h int) {
+	t.Helper()
+	out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", "="+session+":", "#{window_width} #{window_height}").Output()
+	if err != nil {
+		t.Fatalf("display-message #{window_width} #{window_height} for %s: %v", session, err)
+	}
+	fields := strings.Fields(string(out))
+	if len(fields) != 2 {
+		t.Fatalf("display-message #{window_width} #{window_height} for %s = %q, want two fields", session, out)
+	}
+	w, errW := strconv.Atoi(fields[0])
+	h, errH := strconv.Atoi(fields[1])
+	if errW != nil || errH != nil {
+		t.Fatalf("parse window size %q for %s: width err=%v height err=%v", out, session, errW, errH)
+	}
+	return w, h
+}
+
+// watchdogResizeWindow issues a direct resize-window against session — the same live trigger
+// smoke_dotfill_test.go already drives against a single session — bypassing the engine entirely so a
+// test can resize one worktree's window from outside every engine.
+func watchdogResizeWindow(t *testing.T, tmuxPath, socket, session string, cols, rows int) {
+	t.Helper()
+	out, err := exec.Command(tmuxPath, "-L", socket, "resize-window", "-t", "="+session+":", "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows)).CombinedOutput()
+	if err != nil {
+		t.Fatalf("resize-window %s to %dx%d: %v (%s)", session, cols, rows, err, out)
+	}
+}
+
+// TestWatchdogIntegration_ResizeAppliesOnlyToThatWorktree drives a real resize against one of two
+// worktrees discovered by the same daemon and asserts the sibling's own window is left exactly alone
+// — the daemon's per-session watch loops must stay isolated from each other, never cross-applying a
+// resize meant for a different worktree's session.
+func TestWatchdogIntegration_ResizeAppliesOnlyToThatWorktree(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	hubforge.AddPair(t, h, "watchdog-resize-second")
+
+	eng1 := watchdogIntegrationEngine(t, h.PrimeWorktree())
+	eng2 := watchdogIntegrationEngine(t, h.PairWarpWorktree("watchdog-resize-second"))
+
+	tmuxPath := eng1.TmuxPath()
+	socket := reedengine.ServerName(h.Path)
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+
+	loopDone := make(chan error, 1)
+	go func() {
+		loopDone <- runWatchdogLoop(ctx, h.Path, tmuxPath)
+	}()
+
+	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
+		names, err := reedengine.ListSessions(tmuxPath, socket)
+		if err != nil {
+			return false
+		}
+		found1, found2 := false, false
+		for _, n := range names {
+			if n == eng1.SessionName() {
+				found1 = true
+			}
+			if n == eng2.SessionName() {
+				found2 = true
+			}
+		}
+		return found1 && found2
+	})
+
+	eng2W, eng2H := watchdogWindowSize(t, tmuxPath, socket, eng2.SessionName())
+
+	_, eng1H := watchdogWindowSize(t, tmuxPath, socket, eng1.SessionName())
+	newH := eng1H + 10
+	watchdogResizeWindow(t, tmuxPath, socket, eng1.SessionName(), 90, newH)
+
+	waitForCondition(t, 15*time.Second, func() bool {
+		_, gotH := watchdogWindowSize(t, tmuxPath, socket, eng1.SessionName())
+		return gotH == newH
+	})
+
+	// Give eng1's own watch loop goroutine ample time to actually react before checking the sibling
+	// never moved — a false pass here would mean we checked before either loop had a chance to run.
+	time.Sleep(watchdogHubDiscoveryCycle)
+
+	gotW2, gotH2 := watchdogWindowSize(t, tmuxPath, socket, eng2.SessionName())
+	if gotW2 != eng2W || gotH2 != eng2H {
+		t.Errorf("eng2's window size became %dx%d after resizing only eng1's window; want unchanged %dx%d — a resize must re-apply only the worktree whose own window actually resized", gotW2, gotH2, eng2W, eng2H)
+	}
+
+	cancel()
+	<-loopDone
+}
+
+// TestWatchdogIntegration_WritesDiagnosticsIntoHubLogsDir asserts the daemon points its durable log
+// sink at fabricengine.HubLogsDir(hub) before discarding stderr — the only observable proof of
+// watchdogCmd's documented ordering (sink first, then io.Discard, then the lock).
+func TestWatchdogIntegration_WritesDiagnosticsIntoHubLogsDir(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
+	if err != nil {
+		t.Fatalf("LoadConfig: %v", err)
+	}
+	tmuxPath := watchdogIntegrationTmux(t, cfg)
+
+	logsDir := fabricengine.HubLogsDir(h.Path)
+	if _, err := os.Stat(logsDir); err == nil {
+		t.Fatalf("hub logs dir %s already exists before the daemon ever ran", logsDir)
+	}
+
+	cancel, done, _ := runWatchdogCmdInBackground(t, h.Path, tmuxPath)
+	defer cancel()
+
+	waitForCondition(t, 10*time.Second, func() bool {
+		entries, err := os.ReadDir(logsDir)
+		return err == nil && len(entries) > 0
+	})
+
+	entries, err := os.ReadDir(logsDir)
+	if err != nil {
+		t.Fatalf("ReadDir(%s): %v", logsDir, err)
+	}
+	var sawTrace bool
+	for _, e := range entries {
+		if strings.HasPrefix(e.Name(), "trace-") {
+			sawTrace = true
+		}
+	}
+	if !sawTrace {
+		t.Errorf("hub logs dir %s entries = %v, want at least one trace-*.log file — this is the only observable proof the durable sink was pointed at HubLogsDir before stderr was discarded", logsDir, entries)
+	}
+
+	cancel()
+	<-done
+}
+
+// TestWatchdogIntegration_DownThenUpDoesNotKillDaemon drives a down immediately followed by an up
+// against the daemon's one live worktree and asserts the daemon itself never exits and rediscovers
+// the re-upped session — watchdogHubIdleCycles exists precisely to cover this gap.
+func TestWatchdogIntegration_DownThenUpDoesNotKillDaemon(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	eng := watchdogIntegrationEngine(t, h.PrimeWorktree())
+	tmuxPath := eng.TmuxPath()
+	socket := reedengine.ServerName(h.Path)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+	loopDone := make(chan error, 1)
+	go func() {
+		loopDone <- runWatchdogLoop(ctx, h.Path, tmuxPath)
+	}()
+
+	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
+		names, err := reedengine.ListSessions(tmuxPath, socket)
+		if err != nil {
+			return false
+		}
+		for _, n := range names {
+			if n == eng.SessionName() {
+				return true
+			}
+		}
+		return false
+	})
+
+	if _, err := eng.Down(); err != nil {
+		t.Fatalf("eng.Down(): %v", err)
+	}
+	if _, err := eng.Up(); err != nil {
+		t.Fatalf("eng.Up() (immediate re-up): %v", err)
+	}
+
+	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
+		names, err := reedengine.ListSessions(tmuxPath, socket)
+		if err != nil {
+			return false
+		}
+		for _, n := range names {
+			if n == eng.SessionName() {
+				return true
+			}
+		}
+		return false
+	})
+
+	select {
+	case err := <-loopDone:
+		t.Fatalf("runWatchdogLoop exited after a down immediately followed by an up (err=%v); want it still running", err)
+	default:
+	}
+
+	cancel()
+	<-loopDone
+}
+
+// TestWatchdogIntegration_ReEntryReReadsFlippedConfig downs a worktree, flips its watchdog: key to
+// off, ups it again, and asserts BOTH halves of re-entry re-reading the config: the one continuously
+// running daemon (never restarted — loopDone is asserted still open throughout) rediscovers the
+// re-upped session, and enterSession — the exact function the daemon's own discovery loop calls on
+// every appeared name — now reads the flipped value straight off disk rather than the value it held
+// before the down/up.
+func TestWatchdogIntegration_ReEntryReReadsFlippedConfig(t *testing.T) {
+	h := hubforge.NewHub(t, ".")
+	eng := watchdogIntegrationEngine(t, h.PrimeWorktree())
+	tmuxPath := eng.TmuxPath()
+	socket := reedengine.ServerName(h.Path)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+	loopDone := make(chan error, 1)
+	go func() {
+		loopDone <- runWatchdogLoop(ctx, h.Path, tmuxPath)
+	}()
+
+	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
+		names, err := reedengine.ListSessions(tmuxPath, socket)
+		if err != nil {
+			return false
+		}
+		for _, n := range names {
+			if n == eng.SessionName() {
+				return true
+			}
+		}
+		return false
+	})
+
+	if _, err := eng.Down(); err != nil {
+		t.Fatalf("eng.Down(): %v", err)
+	}
+
+	location, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
+	if err != nil {
+		t.Fatalf("ResolveWorktree: %v", err)
+	}
+	// enterSession loads its own config straight off disk, so the flip must actually be seeded into
+	// the fixture's reed.yaml — the whole resolved config, every key present, since a partial override
+	// fails LoadConfig's strictness check (mirrors TestWatchdogIntegration_OffWorktreeEntersKnownButStartsNoWatcher).
+	offCfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
+	if err != nil {
+		t.Fatalf("LoadConfig: %v", err)
+	}
+	offCfg.Watchdog = "off"
+	seeded, err := yaml.Marshal(offCfg)
+	if err != nil {
+		t.Fatalf("marshal flipped config: %v", err)
+	}
+	hubforge.SeedConfig(t, h, map[string]string{"reed": string(seeded)})
+
+	geom := hubgeom.ReedGeometry(location)
+	eng2 := reedengine.New(offCfg, geom)
+	if _, err := eng2.Up(); err != nil {
+		t.Fatalf("eng2.Up() (re-up with watchdog: off): %v", err)
+	}
+	t.Cleanup(func() { _, _ = eng2.Down() })
+
+	// The one daemon started above is still running throughout this whole down/flip/up sequence —
+	// never restarted — and rediscovers the re-upped session under the SAME session name.
+	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
+		names, err := reedengine.ListSessions(tmuxPath, socket)
+		if err != nil {
+			return false
+		}
+		for _, n := range names {
+			if n == eng2.SessionName() {
+				return true
+			}
+		}
+		return false
+	})
+	select {
+	case err := <-loopDone:
+		t.Fatalf("runWatchdogLoop exited during the down/flip/up sequence (err=%v); want it still running throughout", err)
+	default:
+	}
+
+	// enterSession is exactly the function the running daemon's own discovery loop calls for a newly
+	// appeared name — calling it here against the same hub/tmux/session identity proves what value the
+	// daemon itself would now read on its own next discovery cycle.
+	ws, err := enterSession(h.Path, tmuxPath, eng2.SessionName())
+	if err != nil {
+		t.Fatalf("enterSession (after down + flip-to-off + up): %v", err)
+	}
+	if ws.cancel != nil {
+		ws.cancel()
+		t.Error("enterSession() after down + flip-to-off + up returned a non-nil cancel; want nil — re-entry must re-read the flipped config rather than the value it held before the down/up")
+	}
+	if ws.eng == nil {
+		t.Error("enterSession() after re-entry returned a nil eng; want a built Engine so departure bookkeeping stays uniform")
+	}
+
+	cancel()
+	<-loopDone
+}
+
+func TestWatchdogIntegration_AttachAndResumeAttemptTheSpawn(t *testing.T) {
+	// ensureWatchdogSpawned's own os.Executable() re-exec is unsafe to drive live under `go test`
+	// (see the file-level doc comment), so this exercises the call site far enough to prove it is
+	// actually reached rather than suppressed: an unusable hubPath makes ensureWatchdogSpawned fail
+	// at its own MkdirAll step, before ever calling os.Executable(), which is safely observable.
+	h := hubforge.NewHub(t, ".")
+	eng := watchdogIntegrationEngine(t, h.PrimeWorktree())
+
+	unusableHub := unusableHubPath(t)
+	c := &reedCLI{eng: eng, hubPath: unusableHub, suppressWatchdogSpawn: false}
+	// A spawn attempt against an unusable hub path must return without panicking — proving
+	// ensureWatchdogSpawned was reached (not suppressed) and degrades harmlessly, exactly as up,
+	// resume, and attach each rely on.
+	c.ensureWatchdogSpawned()
+}
+
+// unusableHubPath returns a hub path that makes fabricengine.HubScratchDir(hub)'s MkdirAll fail: a
+// regular file sits where an intermediate directory component of the hub path must go, so nothing
+// can ever be created beneath it.
+func unusableHubPath(t *testing.T) string {
+	t.Helper()
+	blocker := filepath.Join(t.TempDir(), "blocker-file")
+	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
+		t.Fatalf("WriteFile(%s): %v", blocker, err)
+	}
+	return filepath.Join(blocker, "hub-under-a-file")
+}
+
+// waitForCondition polls cond every 100ms until it reports true or timeout elapses, failing the test
+// on timeout.
+func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
+	t.Helper()
+	deadline := time.Now().Add(timeout)
+	for {
+		if cond() {
+			return
+		}
+		if time.Now().After(deadline) {
+			t.Fatalf("condition not met within %s", timeout)
+		}
+		time.Sleep(100 * time.Millisecond)
+	}
+}
diff --git a/internal/reedcli/watchdog_test.go b/internal/reedcli/watchdog_test.go
new file mode 100644
index 000000000..11701dbf4
--- /dev/null
+++ b/internal/reedcli/watchdog_test.go
@@ -0,0 +1,129 @@
+// watchdog_test.go pins the watchdog daemon's pure seams — planSessionDiff and sessionsAreIdle —
+// against no tmux server and no filesystem at all, table-driven.
+
+package reedcli
+
+import (
+	"errors"
+	"sort"
+	"testing"
+)
+
+func TestPlanSessionDiff(t *testing.T) {
+	tests := []struct {
+		name         string
+		live         []string
+		known        map[string]watchedSession
+		wantAppeared []string
+		wantDeparted []string
+	}{
+		{
+			name:         "empty to populated",
+			live:         []string{"alpha", "beta"},
+			known:        map[string]watchedSession{},
+			wantAppeared: []string{"alpha", "beta"},
+			wantDeparted: nil,
+		},
+		{
+			name: "populated to empty",
+			live: nil,
+			known: map[string]watchedSession{
+				"alpha": {},
+				"beta":  {},
+			},
+			wantAppeared: nil,
+			wantDeparted: []string{"alpha", "beta"},
+		},
+		{
+			name: "partial overlap",
+			live: []string{"alpha", "gamma"},
+			known: map[string]watchedSession{
+				"alpha": {},
+				"beta":  {},
+			},
+			wantAppeared: []string{"gamma"},
+			wantDeparted: []string{"beta"},
+		},
+		{
+			name: "no change",
+			live: []string{"alpha", "beta"},
+			known: map[string]watchedSession{
+				"alpha": {},
+				"beta":  {},
+			},
+			wantAppeared: nil,
+			wantDeparted: nil,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			gotAppeared, gotDeparted := planSessionDiff(tt.live, tt.known)
+			sort.Strings(gotAppeared)
+			sort.Strings(gotDeparted)
+			if !equalStringSlices(gotAppeared, tt.wantAppeared) {
+				t.Errorf("planSessionDiff() appeared = %v; want %v", gotAppeared, tt.wantAppeared)
+			}
+			if !equalStringSlices(gotDeparted, tt.wantDeparted) {
+				t.Errorf("planSessionDiff() departed = %v; want %v", gotDeparted, tt.wantDeparted)
+			}
+		})
+	}
+}
+
+// equalStringSlices treats a nil slice and an empty slice as equal, since planSessionDiff never
+// distinguishes "no names" from "an empty allocated slice of names".
+func equalStringSlices(got, want []string) bool {
+	if len(got) != len(want) {
+		return false
+	}
+	for i := range got {
+		if got[i] != want[i] {
+			return false
+		}
+	}
+	return true
+}
+
+func TestSessionsAreIdle(t *testing.T) {
+	tests := []struct {
+		name  string
+		names []string
+		err   error
+		want  bool
+	}{
+		{
+			name:  "non-empty listing, no error",
+			names: []string{"alpha"},
+			err:   nil,
+			want:  false,
+		},
+		{
+			name:  "empty listing, no error",
+			names: nil,
+			err:   nil,
+			want:  true,
+		},
+		{
+			name:  "non-empty listing, error",
+			names: []string{"alpha"},
+			err:   errors.New("stale listing"),
+			want:  true,
+		},
+		{
+			name:  "empty listing, error",
+			names: nil,
+			err:   errors.New("no server running"),
+			want:  true,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := sessionsAreIdle(tt.names, tt.err)
+			if got != tt.want {
+				t.Errorf("sessionsAreIdle(%v, %v) = %v; want %v", tt.names, tt.err, got, tt.want)
+			}
+		})
+	}
+}
diff --git a/internal/reedengine/apply.go b/internal/reedengine/apply.go
index 7d711beff..cb5b27e2b 100644
--- a/internal/reedengine/apply.go
+++ b/internal/reedengine/apply.go
@@ -64,9 +64,10 @@ func paneIDsByTop(live []LivePane) []string {
 
 // renderInputs is the single mapping from persisted state plus the live pane set down to the
 // arguments the render package takes: the strand table, the height-policy params (including the
-// header, blanked when its pane is no longer present), and the physical pane order. Both planLayout
-// and fixedHeightPins are built on toRenderInputs and never compute this mapping themselves, so the
-// two can never disagree about which header id — or which strand set — they are laying out.
+// Selvage band, blanked when its pane is no longer present), and the physical pane order. Both
+// planLayout and fixedHeightPins are built on toRenderInputs and never compute this mapping
+// themselves, so the two can never disagree about which Selvage id — or which strand set — they are
+// laying out.
 type renderInputs struct {
 	strands   []render.Strand
 	params    render.Params
@@ -74,23 +75,23 @@ type renderInputs struct {
 }
 
 // toRenderInputs performs the persisted-state-to-render mapping exactly once: it filters st.Strands
-// to the present pane set, blanks st.HeaderPaneID when the header pane is not present, assembles the
+// to the present pane set, blanks st.SelvagePaneID when the Selvage pane is not present, assembles the
 // render.Params this engine's config implies, and orders live's pane ids top to bottom. It touches no
 // tmux and queries nothing of its own — box and live are told to it by the caller, matching
 // planLayout's own told-box contract.
 func (e *Engine) toRenderInputs(st *ReedState, live []LivePane) renderInputs {
 	presentIDs := liveIDSet(live)
 	strands := toRenderStrands(st.Strands, presentIDs)
-	headerPaneID := st.HeaderPaneID
-	if !presentIDs[headerPaneID] {
-		headerPaneID = ""
+	selvagePaneID := st.SelvagePaneID
+	if !presentIDs[selvagePaneID] {
+		selvagePaneID = ""
 	}
 	return renderInputs{
 		strands: strands,
 		params: render.Params{
 			CollapsedStripRows: e.cfg.CollapsedStripRows,
 			MinFullRows:        e.cfg.MinFullRows,
-			Header:             render.Header{PaneID: headerPaneID, HeightRows: e.cfg.Header.HeightRows},
+			Selvage:            render.Selvage{PaneID: selvagePaneID, HeightRows: e.cfg.Selvage.HeightRows},
 		},
 		paneOrder: paneIDsByTop(live),
 	}
@@ -101,7 +102,7 @@ func (e *Engine) toRenderInputs(st *ReedState, live []LivePane) renderInputs {
 // itself: box is always told to it by the caller, and it queries nothing of
 // its own. The persisted-state-to-render mapping lives in toRenderInputs,
 // which fixedHeightPins below shares, so the layout and the pin path can
-// never be computed from a different header id than each other.
+// never be computed from a different Selvage id than each other.
 //
 // The two callers pass two different box sources: applyLayoutLocked passes
 // e.liveBoxLocked()'s live tmux window query (falling back to the configured
@@ -113,7 +114,7 @@ func (e *Engine) planLayout(st *ReedState, live []LivePane, box render.Box) (lay
 	return render.Rules(in.strands, box, in.params, in.paneOrder)
 }
 
-// fixedHeightPins reports the panes whose heights are absolute row budgets — the header band and
+// fixedHeightPins reports the panes whose heights are absolute row budgets — the Selvage band and
 // every collapsed strip — for st's current strand table against live, within box. It calls
 // toRenderInputs and queries nothing of its own: box is told to it by the caller exactly as
 // planLayout is, and it must always be called with the same st, live and box triple the layout for
@@ -197,7 +198,7 @@ type applyResult struct {
 //
 // select-layout with a layout string whose dimensions disagree with the live
 // window exits 0 and silently rescales the layout proportionally, so every
-// absolute row budget reed computes (Header.HeightRows, CollapsedStripRows,
+// absolute row budget reed computes (Selvage.HeightRows, CollapsedStripRows,
 // MinFullRows) was being scaled by live_height/cfg.Height on any window that
 // is not exactly cfg.Height rows tall — this is why the box passed to
 // planLayout below is always the live one, not the configured one.
diff --git a/internal/reedengine/apply_test.go b/internal/reedengine/apply_test.go
index 8d9014446..b8cf2515f 100644
--- a/internal/reedengine/apply_test.go
+++ b/internal/reedengine/apply_test.go
@@ -79,13 +79,13 @@ func TestPlanLayout_HiddenStrandExcludedFromPlacement(t *testing.T) {
 	}
 }
 
-// TestPlanLayout_StaleHeaderPaneIDNeverEmittedAsLayoutCell pins planLayout's header presence
-// filter: a stale absent header must render as if no header existed.
-func TestPlanLayout_StaleHeaderPaneIDNeverEmittedAsLayoutCell(t *testing.T) {
+// TestPlanLayout_StaleSelvagePaneIDNeverEmittedAsLayoutCell pins planLayout's Selvage presence
+// filter: a stale absent Selvage must render as if no Selvage existed.
+func TestPlanLayout_StaleSelvagePaneIDNeverEmittedAsLayoutCell(t *testing.T) {
 	e := newTestEngine(t)
 	e.cfg.Width, e.cfg.Height = 100, 21
 	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
-	e.cfg.Header.HeightRows = 1
+	e.cfg.Selvage.HeightRows = 1
 
 	strands := []Strand{
 		{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
@@ -97,9 +97,9 @@ func TestPlanLayout_StaleHeaderPaneIDNeverEmittedAsLayoutCell(t *testing.T) {
 	}
 	live := []LivePane{{ID: "%1", Top: 0}, {ID: "%2", Top: 11}}
 
-	// Stale header: %9 is nowhere in live, so the plan must equal the
-	// no-header plan bit for bit.
-	st := &ReedState{Strands: strands, HeaderPaneID: "%9"}
+	// Stale Selvage: %9 is nowhere in live, so the plan must equal the
+	// no-Selvage plan bit for bit.
+	st := &ReedState{Strands: strands, SelvagePaneID: "%9"}
 	gotLayout, gotFocus, err := e.planLayout(st, live, render.Box{X: 0, Y: 0, W: 100, H: 21})
 	if err != nil {
 		t.Fatalf("planLayout() unexpected error: %v", err)
@@ -112,25 +112,25 @@ func TestPlanLayout_StaleHeaderPaneIDNeverEmittedAsLayoutCell(t *testing.T) {
 		t.Fatalf("render.Rules() unexpected error: %v", err)
 	}
 	if gotLayout != wantLayout || gotFocus != wantFocus {
-		t.Errorf("planLayout() with stale header = (%q,%q), want the no-header plan (%q,%q)", gotLayout, gotFocus, wantLayout, wantFocus)
+		t.Errorf("planLayout() with stale Selvage = (%q,%q), want the no-Selvage plan (%q,%q)", gotLayout, gotFocus, wantLayout, wantFocus)
 	}
 
-	// Present-but-dead header corpse: the cell must still be emitted, same
+	// Present-but-dead Selvage corpse: the cell must still be emitted, same
 	// as any dead-but-present pane the layout has to enumerate.
 	liveWithCorpse := append([]LivePane{{ID: "%9", Dead: true, Top: 0}}, []LivePane{{ID: "%1", Top: 2}, {ID: "%2", Top: 12}}...)
 	gotLayout, _, err = e.planLayout(st, liveWithCorpse, render.Box{X: 0, Y: 0, W: 100, H: 21})
 	if err != nil {
-		t.Fatalf("planLayout() with corpse header unexpected error: %v", err)
+		t.Fatalf("planLayout() with corpse Selvage unexpected error: %v", err)
 	}
 	wantLayout, _, err = render.Rules(renderStrands,
 		render.Box{X: 0, Y: 0, W: 100, H: 21},
-		render.Params{CollapsedStripRows: 2, MinFullRows: 3, Header: render.Header{PaneID: "%9", HeightRows: 1}},
+		render.Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: render.Selvage{PaneID: "%9", HeightRows: 1}},
 		[]string{"%9", "%1", "%2"})
 	if err != nil {
-		t.Fatalf("render.Rules() with header unexpected error: %v", err)
+		t.Fatalf("render.Rules() with Selvage unexpected error: %v", err)
 	}
 	if gotLayout != wantLayout {
-		t.Errorf("planLayout() with corpse header = %q, want the with-header plan %q (a present corpse still occupies a layout slot)", gotLayout, wantLayout)
+		t.Errorf("planLayout() with corpse Selvage = %q, want the with-Selvage plan %q (a present corpse still occupies a layout slot)", gotLayout, wantLayout)
 	}
 }
 
@@ -503,13 +503,13 @@ func TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout(t *testing.T) {
 	e := newTestEngine(t)
 	e.cfg.Width, e.cfg.Height = 100, 21
 	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
-	e.cfg.Header.HeightRows = 1
+	e.cfg.Selvage.HeightRows = 1
 
 	rec := &applyHookRecorder{}
 	e.tmux.execHook = newApplyRecordingHook(rec)
 
 	st := &ReedState{
-		HeaderPaneID: "%9",
+		SelvagePaneID: "%9",
 		Strands: []Strand{
 			{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
 			{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
@@ -550,7 +550,7 @@ func TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout(t *testing.T) {
 }
 
 // TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear pins the-clear-is-unconditional-including-zero-pins:
-// an apply whose plan yields zero pins — a HeaderPaneID absent from the live set, no strip strand
+// an apply whose plan yields zero pins — a SelvagePaneID absent from the live set, no strip strand
 // present — still issues the clear, and issues no resize-pane entry behind it.
 // The two subtests separate the two opinions a zero-pin rebuild carries: "nothing is pinned" is
 // unconditional, while the watchdog's touch entry rides watchdog on/off, so a watchdog: on session
@@ -561,14 +561,14 @@ func TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear(t *testing.T) {
 		e := newTestEngine(t)
 		e.cfg.Width, e.cfg.Height = 100, 21
 		e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
-		e.cfg.Header.HeightRows = 1
+		e.cfg.Selvage.HeightRows = 1
 		e.cfg.Watchdog = watchdog
 
 		rec := &applyHookRecorder{}
 		e.tmux.execHook = newApplyRecordingHook(rec)
 
 		st := &ReedState{
-			HeaderPaneID: "%9", // absent from live below, so the mapping blanks it
+			SelvagePaneID: "%9", // absent from live below, so the mapping blanks it
 			Strands: []Strand{
 				{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
 				{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
diff --git a/internal/reedengine/attach.go b/internal/reedengine/attach.go
index a0200e912..7dcc528c9 100644
--- a/internal/reedengine/attach.go
+++ b/internal/reedengine/attach.go
@@ -73,7 +73,7 @@ func chainedAttachArgv(socket, session, layout string) []string {
 // same told box the chained layout is. This is what corrects a later client resize, and — on a
 // session whose earlier apply already installed the hook — a degraded bare attach too. A degrade
 // return installs nothing: the uncovered window is a session between "up" and its first placed
-// strand, which has nothing to pin anyway because a lone header pane takes render.Rules' sole-cell
+// strand, which has nothing to pin anyway because a lone Selvage pane takes render.Rules' sole-cell
 // branch.
 func (e *Engine) AttachArgv(cols, rows int) []string {
 	bare := bareAttachArgv(e.Socket(), e.SessionName())
@@ -92,9 +92,10 @@ func (e *Engine) AttachArgv(cols, rows int) []string {
 		e.warnMismatchedClientsLocked(cols, rows)
 
 		// The pins are made here, by the builder itself, not by a second exported call a CLI must
-		// remember. The ordering is load-bearing: the told box is only correct once "status off" has
-		// landed, since that is what makes the post-attach window equal the client's rows rather than
-		// rows - 1.
+		// remember. The ordering is still load-bearing, but for the opposite reason it used to be: the
+		// told box is only correct once the status-line pins have landed AND been read back, because
+		// readStatusRowsLocked a few statements later is what turns whatever #{status} actually became
+		// into the reserved-row count the box is computed from.
 		e.pinGeometryOptionsLocked()
 
 		if !e.readWindowSizeLatestLocked() {
@@ -106,9 +107,8 @@ func (e *Engine) AttachArgv(cols, rows int) []string {
 
 		reserved, ok := e.readStatusRowsLocked()
 		if !ok {
-			// A #{status} that reads back as something other than "off" does NOT suppress the chain: the
-			// reserved-row count is simply taken from that value instead. Only an unrecognized value
-			// (readStatusRowsLocked's ok == false) suppresses it.
+			// The reserved-row count is taken from whatever value #{status} reads back as; only an
+			// unrecognized value (readStatusRowsLocked's ok == false) suppresses the chain.
 			return errAttachChainSuppressed
 		}
 
diff --git a/internal/reedengine/attach_test.go b/internal/reedengine/attach_test.go
index fc25ee0f9..957db9f79 100644
--- a/internal/reedengine/attach_test.go
+++ b/internal/reedengine/attach_test.go
@@ -407,20 +407,26 @@ func TestAttachArgv_EveryOtherDegradedPathYieldsBareArgv(t *testing.T) {
 	})
 }
 
-// TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback pins that AttachArgv itself issues both
-// geometry pins — not a second exported call the CLI has to remember — and that the status-off pin
+// TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback pins that AttachArgv itself issues every
+// geometry pin — not a second exported call the CLI has to remember — and that the status-line pin
 // precedes the #{status} readback, the ordering the told box depends on (pinGeometryOptionsLocked's
-// doc comment: the told box is only correct once status off has landed).
+// doc comment: the told box is only correct once the status-line pins have landed and been read back).
 func TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback(t *testing.T) {
 	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
+	// newTestEngine's Geometry leaves WorktreeName unset; the default status-line template's
+	// {{.worktree}} marker requires it, so this case sets it so StatusLineText() succeeds and all
+	// eight set-option calls (not the six-call degraded shape) are issued.
+	e.geom.WorktreeName = "test-worktree"
 
 	got := e.AttachArgv(80, 24)
 	if len(got) != 10 {
 		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
 	}
 
-	if len(rec.setOptionCalls) != 2 {
-		t.Fatalf("AttachArgv() issued %d set-option calls, want 2: %v", len(rec.setOptionCalls), rec.setOptionCalls)
+	// The seven status-line options plus the pre-existing window-size pin.
+	const wantSetOptionCalls = 8
+	if len(rec.setOptionCalls) != wantSetOptionCalls {
+		t.Fatalf("AttachArgv() issued %d set-option calls, want %d: %v", len(rec.setOptionCalls), wantSetOptionCalls, rec.setOptionCalls)
 	}
 
 	statusPinIdx, statusReadbackIdx := -1, -1
@@ -466,7 +472,7 @@ func TestAttachArgv_NeverMutatesTheSessionOrPersistsState(t *testing.T) {
 	if err != nil {
 		t.Fatalf("LoadState after AttachArgv: %v", err)
 	}
-	if len(after.Strands) != len(before.Strands) || after.HeaderPaneID != before.HeaderPaneID {
+	if len(after.Strands) != len(before.Strands) || after.SelvagePaneID != before.SelvagePaneID {
 		t.Errorf("reed.json changed across AttachArgv: before=%+v after=%+v", before, after)
 	}
 }
diff --git a/internal/reedengine/attachgeometry_integration_test.go b/internal/reedengine/attachgeometry_integration_test.go
index a3d5805a0..425ee75e0 100644
--- a/internal/reedengine/attachgeometry_integration_test.go
+++ b/internal/reedengine/attachgeometry_integration_test.go
@@ -9,7 +9,7 @@
 // Every case before this task's growth cases below drives a 100x30 client against the fixture's
 // 220x50 boot box — SHORTER than the boot box in both dimensions — so every one of them exercises a
 // window SHRINK at attach time and never the growth path this task is about. And the claim that the
-// chained select-layout running post-attach is what holds the header and collapsed-strip budgets is
+// chained select-layout running post-attach is what holds Selvage and collapsed-strip budgets is
 // incomplete on its own: it lands the layout string verbatim only at attach time. tmux has no
 // fixed-height pane concept and redistributes every later window-size delta evenly across the
 // vertical cells, so it is the window-resized resize-pin hook (windowsize.go), not the chain, that
@@ -122,7 +122,7 @@ func startInPTY(t *testing.T, argv []string, cols, rows int) *attachGeometryPTY
 }
 
 // setupAttachGeometryFixture boots a fresh integration engine and adds two strands — a
-// ShrinkWhenWaitingOnChild parent and its child — so the session carries a header pane, a collapsed
+// ShrinkWhenWaitingOnChild parent and its child — so the session carries a Selvage pane, a collapsed
 // strip, and a full pane simultaneously: the three-cell shape every case below lays out against.
 func setupAttachGeometryFixture(t *testing.T) *Engine {
 	t.Helper()
@@ -191,7 +191,7 @@ func windowLayoutNow(t *testing.T, e *Engine) string {
 // builds and asserts, from OUTSIDE the pty, that the live window becomes exactly the client's told
 // size and that #{window_layout} equals the argv's own planned string byte for byte — tmux's silent
 // proportional rescale is what this pins against. It then asserts, from the same attached session,
-// that the header pane and the collapsed strip both landed at their configured, unclamped row
+// that the Selvage pane and the collapsed strip both landed at their configured, unclamped row
 // budgets — true here only at attach time; see the growth cases below for what holds those budgets
 // across a later resize.
 func TestAttachGeometry_ExactLayoutAndRowBudgets(t *testing.T) {
@@ -213,8 +213,10 @@ func TestAttachGeometry_ExactLayoutAndRowBudgets(t *testing.T) {
 	if gotW != cols {
 		t.Errorf("#{window_width} after attach = %d, want %d (the client's told cols)", gotW, cols)
 	}
-	if gotH != rows {
-		t.Errorf("#{window_height} after attach = %d, want exactly %d (status is off, so the window is the client's full rows, not rows-1)", gotH, rows)
+	// The status-line pins now leave "status" on, which reserves one row, so the live window settles
+	// at rows-1 rather than the client's full told rows.
+	if gotH != rows-1 {
+		t.Errorf("#{window_height} after attach = %d, want exactly %d (status is on, reserving one row)", gotH, rows-1)
 	}
 	if gotLayout := windowLayoutNow(t, e); gotLayout != wantLayout {
 		t.Errorf("#{window_layout} after attach = %q, want %q byte for byte — a mismatch here means tmux rescaled the planned string instead of applying it verbatim", gotLayout, wantLayout)
@@ -229,19 +231,19 @@ func TestAttachGeometry_ExactLayoutAndRowBudgets(t *testing.T) {
 		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
 	}
 	parentPaneID := st.Strands[0].PaneID
-	headerPaneID := st.HeaderPaneID
+	selvagePaneID := st.SelvagePaneID
 
 	live, err := e.tmux.listPanes(e.SessionName())
 	if err != nil {
 		t.Fatalf("listPanes: %v", err)
 	}
-	var sawHeader, sawParent bool
+	var sawSelvage, sawParent bool
 	for _, p := range live {
 		switch p.ID {
-		case headerPaneID:
-			sawHeader = true
-			if p.Height != e.cfg.Header.HeightRows {
-				t.Errorf("header pane %s height = %d, want %d (cfg.Header.HeightRows)", p.ID, p.Height, e.cfg.Header.HeightRows)
+		case selvagePaneID:
+			sawSelvage = true
+			if p.Height != e.cfg.Selvage.HeightRows {
+				t.Errorf("Selvage pane %s height = %d, want %d (cfg.Selvage.HeightRows)", p.ID, p.Height, e.cfg.Selvage.HeightRows)
 			}
 		case parentPaneID:
 			sawParent = true
@@ -250,8 +252,8 @@ func TestAttachGeometry_ExactLayoutAndRowBudgets(t *testing.T) {
 			}
 		}
 	}
-	if !sawHeader {
-		t.Fatalf("header pane %s missing from live panes %+v", headerPaneID, live)
+	if !sawSelvage {
+		t.Fatalf("Selvage pane %s missing from live panes %+v", selvagePaneID, live)
 	}
 	if !sawParent {
 		t.Fatalf("collapsed parent pane %s missing from live panes %+v", parentPaneID, live)
@@ -344,23 +346,23 @@ func TestAttachGeometry_StaleLayoutRaceIsSafe(t *testing.T) {
 	}
 }
 
-// assertAttachGeometryRowBudgets asserts headerPaneID and parentPaneID are, respectively, at
-// e.cfg.Header.HeightRows and e.cfg.CollapsedStripRows in the live pane set, failing with step
+// assertAttachGeometryRowBudgets asserts selvagePaneID and parentPaneID are, respectively, at
+// e.cfg.Selvage.HeightRows and e.cfg.CollapsedStripRows in the live pane set, failing with step
 // prefixed onto every message so a caller checking the same budgets at two points in one test can
 // tell which point failed.
-func assertAttachGeometryRowBudgets(t *testing.T, e *Engine, headerPaneID, parentPaneID, step string) {
+func assertAttachGeometryRowBudgets(t *testing.T, e *Engine, selvagePaneID, parentPaneID, step string) {
 	t.Helper()
 	live, err := e.tmux.listPanes(e.SessionName())
 	if err != nil {
 		t.Fatalf("listPanes (%s): %v", step, err)
 	}
-	var sawHeader, sawParent bool
+	var sawSelvage, sawParent bool
 	for _, p := range live {
 		switch p.ID {
-		case headerPaneID:
-			sawHeader = true
-			if p.Height != e.cfg.Header.HeightRows {
-				t.Errorf("(%s) header pane %s height = %d, want %d (cfg.Header.HeightRows)", step, p.ID, p.Height, e.cfg.Header.HeightRows)
+		case selvagePaneID:
+			sawSelvage = true
+			if p.Height != e.cfg.Selvage.HeightRows {
+				t.Errorf("(%s) Selvage pane %s height = %d, want %d (cfg.Selvage.HeightRows)", step, p.ID, p.Height, e.cfg.Selvage.HeightRows)
 			}
 		case parentPaneID:
 			sawParent = true
@@ -369,8 +371,8 @@ func assertAttachGeometryRowBudgets(t *testing.T, e *Engine, headerPaneID, paren
 			}
 		}
 	}
-	if !sawHeader {
-		t.Fatalf("(%s) header pane %s missing from live panes %+v", step, headerPaneID, live)
+	if !sawSelvage {
+		t.Fatalf("(%s) Selvage pane %s missing from live panes %+v", step, selvagePaneID, live)
 	}
 	if !sawParent {
 		t.Fatalf("(%s) collapsed parent pane %s missing from live panes %+v", step, parentPaneID, live)
@@ -380,7 +382,7 @@ func assertAttachGeometryRowBudgets(t *testing.T, e *Engine, headerPaneID, paren
 // TestAttachGeometry_ResizeAfterAttachHoldsRowBudgets pins the fix this task ships: unlike
 // TestAttachGeometry_ExactLayoutAndRowBudgets above, whose 100x30 client is SHORTER than the fixture's
 // 220x50 boot box and so never exercises anything beyond attach time, this case resizes the pty AFTER
-// a healthy chained attach has already landed the header and collapsed-strip budgets, to a materially
+// a healthy chained attach has already landed Selvage and collapsed-strip budgets, to a materially
 // TALLER size, and asserts both budgets still hold. This is the case that fails before this task: tmux
 // has no fixed-height pane concept and redistributes every window-size delta evenly across the
 // vertical cells with no intervention, and it is the window-resized resize-pin hook installed by this
@@ -406,10 +408,10 @@ func TestAttachGeometry_ResizeAfterAttachHoldsRowBudgets(t *testing.T) {
 		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
 	}
 	parentPaneID := st.Strands[0].PaneID
-	headerPaneID := st.HeaderPaneID
+	selvagePaneID := st.SelvagePaneID
 
 	// Confirm the budgets landed at attach time, exactly as the exact-layout case above pins.
-	assertAttachGeometryRowBudgets(t, e, headerPaneID, parentPaneID, "after attach")
+	assertAttachGeometryRowBudgets(t, e, selvagePaneID, parentPaneID, "after attach")
 
 	// Now drive a real client resize on the pty master — materially TALLER than the attach size, and
 	// taller than the 220x50 boot box's 50 rows too, so this is unambiguously the growth path, never
@@ -418,25 +420,27 @@ func TestAttachGeometry_ResizeAfterAttachHoldsRowBudgets(t *testing.T) {
 	if err := unix.IoctlSetWinsize(int(pty.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(resizedCols), Row: uint16(resizedRows)}); err != nil {
 		t.Fatalf("TIOCSWINSZ (%dx%d): %v", resizedCols, resizedRows, err)
 	}
+	// With "status" pinned on, tmux's content window settles at resizedRows-1, not resizedRows — the
+	// status-line's own row is not part of the window tmux reports here.
 	waitUntil(t, 15*time.Second, "window never reported the resized height", func() bool {
 		_, h := windowSizeNow(t, e)
-		return h == resizedRows
+		return h == resizedRows-1
 	})
 
-	assertAttachGeometryRowBudgets(t, e, headerPaneID, parentPaneID, "after resize")
+	assertAttachGeometryRowBudgets(t, e, selvagePaneID, parentPaneID, "after resize")
 }
 
-// TestAttachGeometry_BareAttachFromTallClientStillHoldsHeaderBudget covers the path the originally
+// TestAttachGeometry_BareAttachFromTallClientStillHoldsSelvageBudget covers the path the originally
 // reported ~50-row threshold came from: a bare, unchained attach (AttachArgv(0, 0), the "no client
 // size known" argv) from a client TALLER than the fixture's 220x50 boot box's 50-row height. It
-// asserts the header pane still settles at e.cfg.Header.HeightRows once the attach settles.
+// asserts the Selvage pane still settles at e.cfg.Selvage.HeightRows once the attach settles.
 //
 // This case does NOT exercise an install of its own: AttachArgv(0, 0) returns the bare argv before
-// the op lock is even taken, so it installs no window-resized hook. What holds the header here is the
+// the op lock is even taken, so it installs no window-resized hook. What holds Selvage here is the
 // hook setupAttachGeometryFixture's own earlier AddStrand calls already installed via
 // applyLayoutLocked — this case passes on that pre-existing hook, not on anything AttachArgv(0, 0)
 // itself does, and must not be misread as proof that the no-client-size path installs one.
-func TestAttachGeometry_BareAttachFromTallClientStillHoldsHeaderBudget(t *testing.T) {
+func TestAttachGeometry_BareAttachFromTallClientStillHoldsSelvageBudget(t *testing.T) {
 	e := setupAttachGeometryFixture(t)
 
 	argv := e.AttachArgv(0, 0)
@@ -448,35 +452,35 @@ func TestAttachGeometry_BareAttachFromTallClientStillHoldsHeaderBudget(t *testin
 	if err != nil || st == nil {
 		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
 	}
-	headerPaneID := st.HeaderPaneID
+	selvagePaneID := st.SelvagePaneID
 
 	const cols, rows = 100, 80 // taller than the 220x50 boot box's 50 rows
 	startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
 	waitForClientAttached(t, e, 15*time.Second)
 
-	waitUntil(t, 15*time.Second, "header pane never settled at cfg.Header.HeightRows after the bare attach", func() bool {
+	waitUntil(t, 15*time.Second, "Selvage pane never settled at cfg.Selvage.HeightRows after the bare attach", func() bool {
 		live, err := e.tmux.listPanes(e.SessionName())
 		if err != nil {
 			return false
 		}
 		for _, p := range live {
-			if p.ID == headerPaneID {
-				return p.Height == e.cfg.Header.HeightRows
+			if p.ID == selvagePaneID {
+				return p.Height == e.cfg.Selvage.HeightRows
 			}
 		}
 		return false
 	})
 }
 
-// TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin pins the fire-time failure isolation the
+// TestAttachGeometry_DeadStripPinDoesNotBreakSelvagePin pins the fire-time failure isolation the
 // window-resized hook's array encoding buys (Shared Decision hook-body-is-one-array-entry-per-pin):
-// after a healthy chained attach installs a hook pinning both the header and the collapsed parent
+// after a healthy chained attach installs a hook pinning both Selvage and the collapsed parent
 // strip, this kills the strip's own pane out from under the hook, leaving its array entry naming a
-// destroyed pane id, then resizes and asserts the header still holds its budget — the header is
+// destroyed pane id, then resizes and asserts Selvage still holds its budget — Selvage is
 // always pin index 0, and independent array entries mean a later entry's failure cannot take an
 // earlier one down with it (contract_integration_test.go's TestMultiplexerContract pins the same wire
 // fact directly, at the set-hook level, with no pty involved).
-func TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin(t *testing.T) {
+func TestAttachGeometry_DeadStripPinDoesNotBreakSelvagePin(t *testing.T) {
 	e := setupAttachGeometryFixture(t)
 
 	const cols, rows = 100, 30
@@ -495,7 +499,7 @@ func TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin(t *testing.T) {
 	if len(st.Strands) != 2 {
 		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
 	}
-	headerPaneID := st.HeaderPaneID
+	selvagePaneID := st.SelvagePaneID
 	parentPaneID := st.Strands[0].PaneID
 
 	// Kill the collapsed strip's own pane directly via e.tmux, bypassing RemoveStrand/reconcile
@@ -509,25 +513,26 @@ func TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin(t *testing.T) {
 	if err := unix.IoctlSetWinsize(int(pty.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(resizedCols), Row: uint16(resizedRows)}); err != nil {
 		t.Fatalf("TIOCSWINSZ (%dx%d): %v", resizedCols, resizedRows, err)
 	}
+	// With "status" pinned on, tmux's content window settles at resizedRows-1, not resizedRows.
 	waitUntil(t, 15*time.Second, "window never reported the resized height", func() bool {
 		_, h := windowSizeNow(t, e)
-		return h == resizedRows
+		return h == resizedRows-1
 	})
 
 	live, err := e.tmux.listPanes(e.SessionName())
 	if err != nil {
 		t.Fatalf("listPanes: %v", err)
 	}
-	var sawHeader bool
+	var sawSelvage bool
 	for _, p := range live {
-		if p.ID == headerPaneID {
-			sawHeader = true
-			if p.Height != e.cfg.Header.HeightRows {
-				t.Errorf("header pane %s height = %d, want %d (cfg.Header.HeightRows) even though the strip pin's own pane was destroyed", p.ID, p.Height, e.cfg.Header.HeightRows)
+		if p.ID == selvagePaneID {
+			sawSelvage = true
+			if p.Height != e.cfg.Selvage.HeightRows {
+				t.Errorf("Selvage pane %s height = %d, want %d (cfg.Selvage.HeightRows) even though the strip pin's own pane was destroyed", p.ID, p.Height, e.cfg.Selvage.HeightRows)
 			}
 		}
 	}
-	if !sawHeader {
-		t.Fatalf("header pane %s missing from live panes %+v", headerPaneID, live)
+	if !sawSelvage {
+		t.Fatalf("Selvage pane %s missing from live panes %+v", selvagePaneID, live)
 	}
 }
diff --git a/internal/reedengine/config.go b/internal/reedengine/config.go
index f738e64a9..18989fdde 100644
--- a/internal/reedengine/config.go
+++ b/internal/reedengine/config.go
@@ -30,13 +30,20 @@ type Config struct {
 
 	Watchdog string `yaml:"watchdog"`
 
-	Header HeaderConfig `yaml:"header"`
+	StatusLine StatusLineConfig `yaml:"status_line"`
+	Selvage    SelvageConfig    `yaml:"selvage"`
 }
 
-// HeaderConfig configures the header pane's rendered text.
-type HeaderConfig struct {
-	Template   string `yaml:"template"`
-	HeightRows int    `yaml:"height_rows"`
+// StatusLineConfig configures the tmux status-line's rendered text.
+// This is a distinct mechanism from SelvageConfig: text in a tmux option, versus a row budget for a
+// pane -- one block holding both would be misleading.
+type StatusLineConfig struct {
+	Template string `yaml:"template"`
+}
+
+// SelvageConfig configures the Selvage pane's fixed row budget.
+type SelvageConfig struct {
+	HeightRows int `yaml:"height_rows"`
 }
 
 // LoadConfig loads and unmarshals configuration for the reed module.
diff --git a/internal/reedengine/config_test.go b/internal/reedengine/config_test.go
index c172575e0..3c6d45480 100644
--- a/internal/reedengine/config_test.go
+++ b/internal/reedengine/config_test.go
@@ -138,7 +138,25 @@ func TestLoadConfig_UninitializedFallsBackToTemplate(t *testing.T) {
 	if cfg.CollapsedStripRows != 6 {
 		t.Errorf("CollapsedStripRows = %d, want 6", cfg.CollapsedStripRows)
 	}
-	if cfg.Header.HeightRows != 1 {
-		t.Errorf("Header.HeightRows = %d, want 1", cfg.Header.HeightRows)
+	if cfg.Selvage.HeightRows != 1 {
+		t.Errorf("Selvage.HeightRows = %d, want 1", cfg.Selvage.HeightRows)
+	}
+}
+
+// TestLoadConfig_StaleHeaderBlockIsIgnored pins the no-removal-logic decision: an un-reconciled
+// reed.yaml that still carries a stale header: block alongside the new status_line: and selvage:
+// blocks must unmarshal cleanly, with the stale block simply ignored because nothing unmarshals it
+// into Config any more.
+func TestLoadConfig_StaleHeaderBlockIsIgnored(t *testing.T) {
+	tmpDir := t.TempDir()
+	staleContent := reedengine.ConfigTemplate() + "\nheader:\n  template: \"stale\"\n  height_rows: 5\n"
+	seedLyxConfig(t, tmpDir, "reed", staleContent)
+
+	cfg, err := reedengine.LoadConfig(tmpDir, "reed")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	if cfg.Selvage.HeightRows != 1 {
+		t.Errorf("Selvage.HeightRows = %d, want 1 (stale header: block must not override it)", cfg.Selvage.HeightRows)
 	}
 }
diff --git a/internal/reedengine/console-header.md b/internal/reedengine/console-header.md
deleted file mode 100644
index 6a9cf3999..000000000
--- a/internal/reedengine/console-header.md
+++ /dev/null
@@ -1,6 +0,0 @@
-<!-- console-header.md is the default header-pane text template, rendered via
-     tokenvocab.Render (internal/tokenvocab) into the always-on operator console pane.
-     This leading banner comment is stripped by stencil.Fill before parsing, so it documents the template for a human reader only.
-     Available top-level tokens (see internal/tokenvocab's registry): {{.repo}} (the repo name) and {{.hub}} (the hub's absolute directory path).
-     A Config.Header.Template override replaces this whole asset;
-     it is not merged with it. --> hub: {{.hub}}
diff --git a/internal/reedengine/contract_integration_test.go b/internal/reedengine/contract_integration_test.go
index 58bd5ab41..5cb265a7a 100644
--- a/internal/reedengine/contract_integration_test.go
+++ b/internal/reedengine/contract_integration_test.go
@@ -610,19 +610,19 @@ func TestSessionNameRewriteIsSilentAndExactTargetsMissIt(t *testing.T) {
 	}
 }
 
-// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds is the header-pane keepalive regression this
-// batch adds: with the always-present header pane booted, removing a session's sole non-hidden
+// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds is the Selvage keepalive regression this
+// batch adds: with the always-present Selvage pane booted, removing a session's sole non-hidden
 // strand must return success, leave reed.json holding zero persisted strands, AND leave both the
-// session and the header pane specifically alive — the header's whole purpose.
-// This supersedes the original pre-header regression (removing a session's true last pane used to
+// session and the Selvage pane specifically alive — Selvage's whole purpose.
+// This supersedes the original pre-Selvage regression (removing a session's true last pane used to
 // be backend-dependent: tmux destroyed the session outright, forcing RemoveStrand to swallow the
 // resulting "no server running" error as an expected success — see removalEmptiedSession,
 // strand.go).
-// With the header pane as a permanent second pane, killing the strand's pane is never a
+// With Selvage as a permanent second pane, killing the strand's pane is never a
 // last-pane-destroy on ANY backend, so that swallow branch is no longer reached by this scenario at
 // all;
 // it remains in place for the (now believed unreachable in practice, but still defensive) case
-// where the header pane is itself somehow absent.
+// where Selvage is itself somehow absent.
 func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
 	tmpDir := t.TempDir()
 	seedReedConfig(t, tmpDir)
@@ -654,6 +654,7 @@ func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
 		LogsDir:      filepath.Join(hub, "logs"),
 		RepoName:     "test-repo",
 		HubPath:      hub,
+		WorktreeName: filepath.Base(tmpDir),
 	}
 	e := New(cfg, geom)
 
@@ -672,14 +673,14 @@ func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
 		t.Fatalf("Up: %v", err)
 	}
 
-	// The header pane is booted as part of Up, before any strand exists;
+	// Selvage is booted as part of Up, before any strand exists;
 	// capture its id so the post-remove assertions below can confirm it
 	// specifically (not merely "some pane") survived.
 	upSt, err := LoadState(e.stateDir())
-	if err != nil || upSt == nil || upSt.HeaderPaneID == "" {
-		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted HeaderPaneID", upSt, err)
+	if err != nil || upSt == nil || upSt.SelvagePaneID == "" {
+		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted SelvagePaneID", upSt, err)
 	}
-	headerPaneID := upSt.HeaderPaneID
+	selvagePaneID := upSt.SelvagePaneID
 
 	// One non-hidden strand, anchored so it is realized into a live pane at
 	// add time; a long-lived command so it is still running when removed.
@@ -693,7 +694,7 @@ func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
 
 	removed, err := e.RemoveStrand(strand.GUID, false)
 	if err != nil {
-		t.Fatalf("RemoveStrand(sole strand) = %v, want nil error (the header pane keeps the session alive, so emptying the strand table is never a last-pane-destroy)", err)
+		t.Fatalf("RemoveStrand(sole strand) = %v, want nil error (Selvage keeps the session alive, so emptying the strand table is never a last-pane-destroy)", err)
 	}
 	if len(removed.Strands) != 1 || removed.Strands[0].GUID != strand.GUID {
 		t.Fatalf("RemoveStrand.Removed.Strands = %+v, want exactly guid %q", removed.Strands, strand.GUID)
@@ -707,41 +708,41 @@ func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
 		return err == nil && st != nil && len(st.Strands) == 0
 	})
 
-	// The keepalive guarantee this batch adds: the session, and the header
-	// pane specifically, must still be alive with zero strands tracked.
+	// The keepalive guarantee this batch adds: the session, and Selvage
+	// specifically, must still be alive with zero strands tracked.
 	up, err := e.tmux.hasSession(e.SessionName())
 	if err != nil || !up {
-		t.Fatalf("hasSession after removing the sole strand = (%v, %v), want (true, nil) — the header pane must keep the session alive", up, err)
+		t.Fatalf("hasSession after removing the sole strand = (%v, %v), want (true, nil) — Selvage must keep the session alive", up, err)
 	}
 	live, err := e.tmux.listPanes(e.SessionName())
 	if err != nil {
 		t.Fatalf("listPanes after removing the sole strand: %v", err)
 	}
-	headerFound := false
+	selvageFound := false
 	for _, p := range live {
-		if p.ID == headerPaneID {
-			headerFound = true
+		if p.ID == selvagePaneID {
+			selvageFound = true
 			if p.Dead {
-				t.Errorf("header pane %s reports Dead = true after removing the sole strand, want it alive", headerPaneID)
+				t.Errorf("Selvage pane %s reports Dead = true after removing the sole strand, want it alive", selvagePaneID)
 			}
 		}
 	}
-	if !headerFound {
-		t.Fatalf("header pane %s missing from live panes %+v after removing the sole strand", headerPaneID, live)
+	if !selvageFound {
+		t.Fatalf("Selvage pane %s missing from live panes %+v after removing the sole strand", selvagePaneID, live)
 	}
 }
 
-// TestDeadHeaderPaneIsHealedByUpWithoutCorruptingLayout drives the dead-header lifecycle the
-// fable-header-r1 round found broken, end to end against a real multiplexer: the header's keepalive
+// TestDeadSelvagePaneIsHealedByUpWithoutCorruptingLayout drives the dead-Selvage lifecycle the
+// fable-header-r1 round found broken, end to end against a real multiplexer: Selvage's shell
 // process exits (pane_dead=1 under remain-on-exit), a subsequent AddStrand must keep the corpse
 // enumerable (reconcile's dead-kill exemption) and lay out with no window-bottom overflow and no
-// stale-cell scramble (planLayout's presence filter), and the next Up must heal the header — kill
-// the corpse, split a fresh header back in at the physical top, persist the new id — instead of
-// treating the corpse as a working header (the old presence-keyed idempotency check) or wedging on
+// stale-cell scramble (planLayout's presence filter), and the next Up must heal Selvage — kill
+// the corpse, split a fresh Selvage back in at the physical bottom, persist the new id — instead of
+// treating the corpse as a working Selvage (the old presence-keyed idempotency check) or wedging on
 // a too-short split target (the old first-alive targeting).
 // Pre-fix this sequence scrambled every strand's height on the add and then failed every up with
 // "no space for new pane" until a full down.
-func TestDeadHeaderPaneIsHealedByUpWithoutCorruptingLayout(t *testing.T) {
+func TestDeadSelvagePaneIsHealedByUpWithoutCorruptingLayout(t *testing.T) {
 	tmpDir := t.TempDir()
 	seedReedConfig(t, tmpDir)
 
@@ -763,6 +764,7 @@ func TestDeadHeaderPaneIsHealedByUpWithoutCorruptingLayout(t *testing.T) {
 		LogsDir:      filepath.Join(hub, "logs"),
 		RepoName:     "test-repo",
 		HubPath:      hub,
+		WorktreeName: filepath.Base(tmpDir),
 	}
 	e := New(cfg, geom)
 
@@ -775,127 +777,127 @@ func TestDeadHeaderPaneIsHealedByUpWithoutCorruptingLayout(t *testing.T) {
 		t.Fatalf("Up: %v", err)
 	}
 	st, err := LoadState(e.stateDir())
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted HeaderPaneID", st, err)
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted SelvagePaneID", st, err)
 	}
-	deadHeaderID := st.HeaderPaneID
+	deadSelvageID := st.SelvagePaneID
 
 	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}}); err != nil {
 		t.Fatalf("AddStrand (first): %v", err)
 	}
 
-	// Kill the header's keepalive by pid: the pane's shell/keepalive does
-	// not read stdin (that is the keepalive's whole contract), so typing
-	// exit would go nowhere — killing #{pane_pid} is how a real keepalive
-	// death looks to tmux. remain-on-exit then corpses the pane
-	// (pane_dead=1); the flip is asynchronous, so poll.
+	// Kill Selvage's shell by pid: Selvage does not read stdin the way a
+	// script-driven pane would react to typed input, so killing
+	// #{pane_pid} is how a real shell death looks to tmux. remain-on-exit
+	// then corpses the pane (pane_dead=1); the flip is asynchronous, so
+	// poll.
 	live, err := e.tmux.listPanes(e.SessionName())
 	if err != nil {
-		t.Fatalf("listPanes before header kill: %v", err)
+		t.Fatalf("listPanes before Selvage kill: %v", err)
 	}
 	for _, p := range live {
-		if p.ID == deadHeaderID {
+		if p.ID == deadSelvageID {
 			proc, err := os.FindProcess(p.PID)
 			if err != nil {
-				t.Fatalf("FindProcess(header pane pid %d): %v", p.PID, err)
+				t.Fatalf("FindProcess(Selvage pane pid %d): %v", p.PID, err)
 			}
 			if err := proc.Kill(); err != nil {
-				t.Fatalf("kill header pane pid %d: %v", p.PID, err)
+				t.Fatalf("kill Selvage pane pid %d: %v", p.PID, err)
 			}
 		}
 	}
-	waitUntil(t, 10*time.Second, "header pane never reported dead", func() bool {
+	waitUntil(t, 10*time.Second, "Selvage pane never reported dead", func() bool {
 		live, err := e.tmux.listPanes(e.SessionName())
 		if err != nil {
 			return false
 		}
 		for _, p := range live {
-			if p.ID == deadHeaderID {
+			if p.ID == deadSelvageID {
 				return p.Dead
 			}
 		}
 		return false
 	})
 
-	// An add with the header dead: must succeed, keep the corpse enumerable,
+	// An add with Selvage dead: must succeed, keep the corpse enumerable,
 	// and produce a sane layout (no pane past the window's bottom edge — the
 	// stale-cell scramble's signature was positional misassignment).
 	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}}); err != nil {
-		t.Fatalf("AddStrand with dead header: %v", err)
+		t.Fatalf("AddStrand with dead Selvage: %v", err)
 	}
 	live, err = e.tmux.listPanes(e.SessionName())
 	if err != nil {
-		t.Fatalf("listPanes after add-with-dead-header: %v", err)
+		t.Fatalf("listPanes after add-with-dead-Selvage: %v", err)
 	}
 	corpsePresent := false
 	for _, p := range live {
-		if p.ID == deadHeaderID {
+		if p.ID == deadSelvageID {
 			corpsePresent = true
 			if !p.Dead {
-				t.Errorf("header corpse %s reports alive after add, want still dead", deadHeaderID)
+				t.Errorf("Selvage corpse %s reports alive after add, want still dead", deadSelvageID)
 			}
 		}
 		if p.Top+p.Height > cfg.Height {
-			t.Errorf("pane %s top+height = %d+%d exceeds window height %d after add-with-dead-header (stale-cell scramble)", p.ID, p.Top, p.Height, cfg.Height)
+			t.Errorf("pane %s top+height = %d+%d exceeds window height %d after add-with-dead-Selvage (stale-cell scramble)", p.ID, p.Top, p.Height, cfg.Height)
 		}
 	}
 	if !corpsePresent {
-		t.Fatalf("header corpse %s was killed by the add's reconcile; it must stay enumerable until up/resume heals it (panes: %+v)", deadHeaderID, live)
+		t.Fatalf("Selvage corpse %s was killed by the add's reconcile; it must stay enumerable until up/resume heals it (panes: %+v)", deadSelvageID, live)
 	}
 
 	// The heal: Up must replace the corpse with a fresh, alive, physically
-	// topmost header and persist its id.
+	// bottommost Selvage and persist its id.
 	if _, err := e.Up(); err != nil {
 		t.Fatalf("Up (heal) = %v, want success — pre-fix this wedged on \"no space for new pane\"", err)
 	}
 	st, err = LoadState(e.stateDir())
-	if err != nil || st == nil || st.HeaderPaneID == "" {
-		t.Fatalf("LoadState after healing Up = (%+v, %v), want a persisted HeaderPaneID", st, err)
+	if err != nil || st == nil || st.SelvagePaneID == "" {
+		t.Fatalf("LoadState after healing Up = (%+v, %v), want a persisted SelvagePaneID", st, err)
 	}
-	if st.HeaderPaneID == deadHeaderID {
-		t.Fatalf("HeaderPaneID still names the corpse %s after the healing Up", deadHeaderID)
+	if st.SelvagePaneID == deadSelvageID {
+		t.Fatalf("SelvagePaneID still names the corpse %s after the healing Up", deadSelvageID)
 	}
 	live, err = e.tmux.listPanes(e.SessionName())
 	if err != nil {
 		t.Fatalf("listPanes after healing Up: %v", err)
 	}
-	headerSeen := false
+	selvageSeen := false
 	for _, p := range live {
-		if p.ID == deadHeaderID {
-			t.Errorf("header corpse %s still present after the healing Up, want it killed and replaced", deadHeaderID)
+		if p.ID == deadSelvageID {
+			t.Errorf("Selvage corpse %s still present after the healing Up, want it killed and replaced", deadSelvageID)
 		}
-		if p.ID == st.HeaderPaneID {
-			headerSeen = true
+		if p.ID == st.SelvagePaneID {
+			selvageSeen = true
 			if p.Dead {
-				t.Errorf("healed header pane %s reports dead", p.ID)
+				t.Errorf("healed Selvage pane %s reports dead", p.ID)
 			}
-			if p.Top != 0 {
-				t.Errorf("healed header pane %s top = %d, want 0 (physically topmost — render.Rules emits its cell first)", p.ID, p.Top)
+			if p.Top+p.Height != cfg.Height {
+				t.Errorf("healed Selvage pane %s top+height = %d+%d, want == window height %d (physically bottommost — render.Rules emits its cell last)", p.ID, p.Top, p.Height, cfg.Height)
 			}
 		}
 		if p.Top+p.Height > cfg.Height {
 			t.Errorf("pane %s top+height = %d+%d exceeds window height %d after the healing Up", p.ID, p.Top, p.Height, cfg.Height)
 		}
 	}
-	if !headerSeen {
-		t.Fatalf("healed header pane %s missing from live panes %+v", st.HeaderPaneID, live)
+	if !selvageSeen {
+		t.Fatalf("healed Selvage pane %s missing from live panes %+v", st.SelvagePaneID, live)
 	}
 }
 
-// TestHeaderNeverGetsZeroHeightLayoutCell pins clampHeaderHeight's never-below-1 floor (height.go)
+// TestSelvageNeverGetsZeroHeightLayoutCell pins clampBandHeight's never-below-1 floor (height.go)
 // against a real multiplexer.
 // A pathological config — height_rows large relative to a tiny window height — used to let
-// clampHeaderHeight legally return 0, which bandHeader would then emit as a literal "WxH,..."
-// header cell with H=0 in the window_layout string.
+// clampBandHeight legally return 0, which bandSelvage would then emit as a literal "WxH,..."
+// Selvage cell with H=0 in the window_layout string.
 // Manual probing against a live tmux 3.6 instance showed that a genuinely zero-height cell is NOT
-// rendered as "no header": select-layout accepts the string (no error),
-// but silently keeps a row for the header pane anyway, pushing every pane below it down by one row
+// rendered as "no Selvage": select-layout accepts the string (no error),
+// but silently keeps a row for the Selvage pane anyway, pushing every pane above it up by one row
 // and overflowing the bottom of the window by exactly one row (the last pane's top+height exceeds
 // the window height).
-// clampHeaderHeight now floors the header at 1 row whenever it exists, which this test confirms
+// clampBandHeight now floors Selvage at 1 row whenever it exists, which this test confirms
 // produces a layout the real multiplexer applies cleanly, with every pane's top+height staying
 // within the window.
-func TestHeaderNeverGetsZeroHeightLayoutCell(t *testing.T) {
+func TestSelvageNeverGetsZeroHeightLayoutCell(t *testing.T) {
 	tmpDir := t.TempDir()
 	seedReedConfig(t, tmpDir)
 
@@ -909,8 +911,8 @@ func TestHeaderNeverGetsZeroHeightLayoutCell(t *testing.T) {
 	}
 
 	const windowRows = 6
-	socket := fmt.Sprintf("lyx-contract-header-floor-test-%d-%d", os.Getpid(), time.Now().UnixNano())
-	session := "header-floor-session"
+	socket := fmt.Sprintf("lyx-contract-selvage-floor-test-%d-%d", os.Getpid(), time.Now().UnixNano())
+	session := "selvage-floor-session"
 	reed := NewTmuxCmd(cfg.Tmux, socket)
 
 	t.Cleanup(func() {
@@ -921,11 +923,11 @@ func TestHeaderNeverGetsZeroHeightLayoutCell(t *testing.T) {
 		t.Fatalf("new-session: %v", err)
 	}
 
-	headerOut, err := reed.output("split-window", "-t", session, "-b", "-P", "-F", "#{pane_id}")
+	selvageOut, err := reed.output("split-window", "-t", session, "-P", "-F", "#{pane_id}")
 	if err != nil {
-		t.Fatalf("split-window (header): %v", err)
+		t.Fatalf("split-window (Selvage): %v", err)
 	}
-	headerPaneID := strings.TrimSpace(headerOut)
+	selvagePaneID := strings.TrimSpace(selvageOut)
 
 	live, err := reed.listPanes(session)
 	if err != nil {
@@ -936,32 +938,32 @@ func TestHeaderNeverGetsZeroHeightLayoutCell(t *testing.T) {
 	}
 	var strandPaneID string
 	for _, p := range live {
-		if p.ID != headerPaneID {
+		if p.ID != selvagePaneID {
 			strandPaneID = p.ID
 		}
 	}
 	if strandPaneID == "" {
-		t.Fatalf("could not identify the non-header pane among %+v", live)
+		t.Fatalf("could not identify the non-Selvage pane among %+v", live)
 	}
 
 	// A pathological config (MinFullRows far larger than the window, plus an
 	// oversized configured height_rows) that pre-fix would have driven
-	// clampHeaderHeight all the way to 0.
+	// clampBandHeight all the way to 0.
 	strands := []render.Strand{
 		{GUID: "s1", PaneID: strandPaneID, Display: render.Display{Anchor: render.AnchorBelowParent}, Live: true},
 	}
 	box := render.Box{X: 0, Y: 0, W: 80, H: windowRows}
 	params := render.Params{
 		MinFullRows: windowRows * 5,
-		Header:      render.Header{PaneID: headerPaneID, HeightRows: windowRows * 5},
+		Selvage:     render.Selvage{PaneID: selvagePaneID, HeightRows: windowRows * 5},
 	}
-	layout, _, err := render.Rules(strands, box, params, []string{headerPaneID, strandPaneID})
+	layout, _, err := render.Rules(strands, box, params, []string{strandPaneID, selvagePaneID})
 	if err != nil {
 		t.Fatalf("render.Rules: %v", err)
 	}
 
 	if err := reed.run("select-layout", "-t", session, layout); err != nil {
-		t.Fatalf("select-layout %q: %v (a real multiplexer rejecting this layout means clampHeaderHeight's floor no longer matches what select-layout accepts)", layout, err)
+		t.Fatalf("select-layout %q: %v (a real multiplexer rejecting this layout means clampBandHeight's floor no longer matches what select-layout accepts)", layout, err)
 	}
 
 	live, err = reed.listPanes(session)
@@ -973,7 +975,7 @@ func TestHeaderNeverGetsZeroHeightLayoutCell(t *testing.T) {
 			t.Errorf("pane %s height = %d after select-layout %q, want >= 1 (a zero-height cell must never survive the real multiplexer)", p.ID, p.Height, layout)
 		}
 		if p.Top+p.Height > windowRows {
-			t.Errorf("pane %s top+height = %d+%d = %d after select-layout %q, want <= window height %d (the off-by-one overflow a bare H=0 header cell used to cause)", p.ID, p.Top, p.Height, p.Top+p.Height, layout, windowRows)
+			t.Errorf("pane %s top+height = %d+%d = %d after select-layout %q, want <= window height %d (the off-by-one overflow a bare H=0 Selvage cell used to cause)", p.ID, p.Top, p.Height, p.Top+p.Height, layout, windowRows)
 		}
 	}
 }
diff --git a/internal/reedengine/doc.go b/internal/reedengine/doc.go
index 50002d532..ddcf285ea 100644
--- a/internal/reedengine/doc.go
+++ b/internal/reedengine/doc.go
@@ -33,45 +33,46 @@
 // the same tmux server rather than each spawning its own.
 //
 // A second package-level invariant: every session also carries exactly one
-// additional, permanent pane beyond its strands — the header
-// (ReedState.HeaderPaneID). It is a first-class construct, deliberately never
-// a Strand (Shared Decision header-is-not-a-strand): it is excluded from
-// strand accounting, from being the preferred split target, and from both
-// halves of reconcile's kill schedule (see ensureHeaderPaneLocked in
+// additional, permanent pane beyond its strands — Selvage
+// (ReedState.SelvagePaneID). It is a first-class construct, deliberately
+// never a Strand (Shared Decision header-is-not-a-strand): it is excluded
+// from strand accounting, from being the preferred split target, and from
+// both halves of reconcile's kill schedule (see ensureSelvagePaneLocked in
 // lifecycle.go, planPaneTarget in spawn.go, and planReconcile's
 // exemptPaneIDs in reconcile.go for the three exclusion seams), so that
 // removing a session's last strand can never destroy the
-// session or corpse its sole pane — the header keeps the session (and the
+// session or corpse its sole pane — Selvage keeps the session (and the
 // substrate the next add needs) alive no matter how many strands come and
-// go. It boots alongside the session/initial pane on both Up and Resume, and
-// Engine.ValidateHeader runs eagerly on every boot path so a bad header
-// template surfaces loud before the pane is ever created, never silently.
-// The header pane is created by a split-window call that carries the
-// keepalive command (`lyx reed header --blocking`) as its own trailing
-// shell-command argument, rather than by splitting a bare shell and typing
-// the command into it afterwards with send-keys: the pane runs that command
-// directly from birth, so it hosts no interactive shell for anything to echo
-// the launch line into or read ~/.bashrc from. This makes the corpse-and-heal
-// contract below actually work as documented: with the keepalive as the
-// pane's own process, "set-option -g remain-on-exit on" corpses the pane the
-// moment that process dies, where a surviving bash previously kept the pane
-// alive and a dead header was silently mistaken for a working one. Under
-// go test the pane still boots commandless — a bare shell, no split-window
-// trailing argument and no send-keys — because headerLaunchLine
-// (headerpane.go) returns "" whenever the boot decides to suppress the
-// launch, which prevents os.Executable() from re-exec'ing the test binary
-// and running its whole suite recursively; that decision now rides on
-// Engine.suppressHeaderLaunch, an unexported field New initialises from
-// testing.Testing(), rather than a testing.Testing() call hard-wired at the
-// boot site. A header whose keepalive process dies (pane_dead=1) is
-// deliberately kept as an enumerable corpse by reconcile — never killed
-// there — and healed (corpse killed, a fresh header split back in at the
-// physical top, carrying the same launch command on both the first attempt
-// and any even-vertical-retile retry) by ensureHeaderPaneLocked on the next
-// Up/Resume; planLayout only ever emits a header cell for a pane actually
-// present in the window, so a stale HeaderPaneID can never put an absent
-// pane's cell into select-layout's string (which a real tmux accepts and
-// misassigns positionally rather than rejecting).
+// go. It boots alongside the session/initial pane on both Up and Resume.
+// Selvage is created by a split-window call that carries e.cfg.Shell — the
+// operator's configured shell, `bash` on POSIX and `pwsh` on Windows by
+// default — as its own trailing shell-command argument, the same way
+// new-session already launches the session's first pane: Selvage is an
+// ordinary, typeable interactive shell, never a re-exec of lyx and never
+// sent a command of reed's own choosing. A Selvage pane whose shell process
+// dies (pane_dead=1) is deliberately kept as an enumerable corpse by
+// reconcile — never killed there — and healed (corpse killed, a fresh
+// Selvage split back in at the physical bottom, carrying e.cfg.Shell on
+// both the first attempt and any even-vertical-retile retry) by
+// ensureSelvagePaneLocked on the next Up/Resume; planLayout only ever emits
+// a Selvage cell for a pane actually present in the window, so a stale
+// SelvagePaneID can never put an absent pane's cell into select-layout's
+// string (which a real tmux accepts and misassigns positionally rather than
+// rejecting).
+//
+// Three module-local Selvage rules, kept here rather than in CONSTRAINTS.md
+// because they describe this package's own design rather than a
+// cross-cutting invariant another module could violate: Selvage is always
+// physically bottom-most in the window — render.Rules emits its cell last
+// and every split site that creates or rebuilds it targets the bottommost
+// live pane, never the topmost. Selvage is never a strand — it is excluded
+// from ReedState.Strands, from strand accounting, and from every strand-only
+// code path, by construction rather than by a runtime check. And Selvage is
+// never written to, cleared, or sent keys by reed — it is a real, usable
+// terminal an operator can run lyx/reed commands in, and reed's own writes
+// stop at booting its shell; the ED2/ED3 screen-clear payload the header
+// pane once needed to display identity text has no counterpart here, since
+// Selvage never displays anything of reed's choosing.
 //
 // The live-geometry rule: the render box a layout is computed against is no
 // longer the config-pinned Width/Height. planLayout (apply.go) is always
@@ -160,8 +161,8 @@
 //     errors loud with "no space for new pane") — so EVERY split site must
 //     verify a split's returned pane id was absent from the pre-split live
 //     set before trusting it as genuinely new: launchStrandLocked's strand
-//     splits and ensureHeaderPaneLocked's header rebuild both run the shared
-//     validateSplitCreatedNewPane guard.
+//     splits and ensureSelvagePaneLocked's Selvage rebuild both run the
+//     shared validateSplitCreatedNewPane guard.
 //   - Dead panes under remain-on-exit (spawn.go): with
 //     "set-option -g remain-on-exit on" set at boot, a pane whose command
 //     exits stays enumerable (pane_dead=1) instead of vanishing WHILE THE
@@ -175,19 +176,19 @@
 //     non-last pane (any backend) and to psmux even for the true last pane;
 //     it does NOT hold for tmux's true last pane — see the next bullet.
 //   - The untracked-pane reap gate (spawn.go, reconcile.go): every pane in
-//     a reed session is either the header or a bound strand's pane, and the
-//     untracked reap enforces that rule as `anyBoundPresent || headerAlive`,
-//     where the header anchor requires ALIVENESS rather than mere
+//     a reed session is either Selvage or a bound strand's pane, and the
+//     untracked reap enforces that rule as `anyBoundPresent || selvageAlive`,
+//     where the Selvage anchor requires ALIVENESS rather than mere
 //     presence — launchStrandLocked makes the gate fire from AddStrand and
-//     UpdateStrand, neither of which calls ensureHeaderPaneLocked to heal a
-//     header corpse first, so a dead header id must never itself authorize
+//     UpdateStrand, neither of which calls ensureSelvagePaneLocked to heal a
+//     Selvage corpse first, so a dead Selvage id must never itself authorize
 //     sparing the untracked set. The reap runs before pane allocation at
 //     one chokepoint inside launchStrandLocked, so the property holds by
 //     construction on every realization path rather than requiring two
 //     call sites to stay in sync. Two consequences follow: an `up` against
-//     a session with zero tracked strands ends up header-only and
+//     a session with zero tracked strands ends up Selvage-only and
 //     full-height, because applyLayoutLockedOpts deliberately skips
-//     select-layout when no strand owns a present pane, and the header
+//     select-layout when no strand owns a present pane, and Selvage
 //     snaps back to its configured height the moment a strand pane exists;
 //     and RemoveStrand's own code is unchanged, but its
 //     reconcileApplyPersistLocked tail inherits the new gate, so removing
@@ -221,8 +222,8 @@
 //   - Told-geometry lifetime and the vanished worktree root (server.go's
 //     validateToldWorktreeRootLive, lock.go's withOpLock/withTryOpLock): a
 //     told Geometry is resolved once per process and pinned for that
-//     process's whole life, so a long-lived process such as the header
-//     pane's keepalive holds a frozen WorktreeRoot that a `mv` of the
+//     process's whole life, so a long-lived process such as the watch loop
+//     (watchloop.go) holds a frozen WorktreeRoot that a `mv` of the
 //     worktree makes stale. Every operation therefore re-checks that told
 //     worktree root's liveness at the op-lock chokepoint, and refuses
 //     rather than creating substrate under a path that is no longer a
@@ -238,7 +239,7 @@
 //     the rest of its life. It automatically returns to whichever mode
 //     (poll or signal) it was in before dormancy, logging exactly one more
 //     line, once the worktree root exists again. Dormancy never tears down
-//     the header pane and never stops the watch loop itself: the session
+//     Selvage and never stops the watch loop itself: the session
 //     reed walked away from may still be hosting the operator's live
 //     strands.
 //   - Silent session-name rewriting (server.go's validateToldTmuxIdentity):
@@ -354,24 +355,24 @@
 //     The cost of "on" is that native terminal text selection needs the
 //     terminal's shift-bypass (hold Shift while dragging); tmux copy-mode is
 //     the in-band alternative. "on" also enables click-to-switch-pane.
-//   - Header band divider row (render/rules.go, height.go): the header pane
-//     and the strand stack below it are physically adjacent, so tmux/psmux
-//     always renders the same one-row border between them that
+//   - Selvage band divider row (render/rules.go, height.go): the Selvage
+//     pane and the strand stack above it are physically adjacent, so
+//     tmux/psmux always renders the same one-row border between them that
 //     buildStackBody already budgets for between individual strands —
 //     omitting that budget still lets select-layout return success, but
 //     tmux inserts the border row anyway, silently overflowing the window
-//     by one row. clampHeaderHeight (height.go) also never clamps the
-//     header below 1 row for the same reason: a real tmux/psmux
+//     by one row. clampBandHeight (height.go) also never clamps
+//     Selvage.HeightRows below 1 row for the same reason: a real tmux/psmux
 //     select-layout does not cleanly support a genuinely zero-height cell
 //     for an always-on pane either. Verified against a real tmux instance;
-//     contract_integration_test.go's TestHeaderNeverGetsZeroHeightLayoutCell
-//     pins it.
+//     contract_integration_test.go's TestSelvageNeverGetsZeroHeightLayoutCell
+//     pins it (renamed for the Selvage band).
 //   - Silent layout rescale (apply.go, windowsize.go): select-layout accepts a
 //     layout string whose dimensions disagree with the live window (exit 0)
 //     and silently rescales it proportionally — measured live on tmux 3.6, a
 //     "220x50" string applied to a "100x30" window turned a 3-row collapsed
 //     strip (3 was the then-default; it is 6 today) into 1 row — so every
-//     absolute row budget reed computes (Header.HeightRows,
+//     absolute row budget reed computes (Selvage.HeightRows,
 //     CollapsedStripRows, MinFullRows) is scaled by live_height/string_height
 //     unless the string is sized to the live window. This is why
 //     applyLayoutLocked always plans against liveBoxLocked's live box rather
@@ -387,16 +388,16 @@
 //     window-size delta arriving after attach time is handed out one row at
 //     a time, round-robin across every vertical cell in the stack — so no
 //     absolute row budget reed computes survives a resize on its own.
-//     Measured live on tmux 3.6: a healthy attached session's header went
+//     Measured live on tmux 3.6: a healthy attached session's Selvage went
 //     from 1 row to 6 across a 76-to-90-row client resize, and to 16 across
 //     a further 90-to-120 one.
 //     The answer is a window-resized window hook holding one
-//     "resize-pane -y" array entry per fixed-height pane — the header band
+//     "resize-pane -y" array entry per fixed-height pane — the Selvage band
 //     and every collapsed strip — installed by reed and executed by the
 //     tmux server itself, refreshed on every successful apply
 //     (applyLayoutLocked) and again in AttachArgv's pre-flight, with the
 //     pinned heights coming from render.FixedHeightPins: the heights render
-//     actually placed the cells at, after clampHeaderHeight and
+//     actually placed the cells at, after clampBandHeight and
 //     clampToFit, never the raw configured budgets.
 //     The watchdog's own signal entry rides the SAME array, always as its
 //     last entry, and installResizePinsLocked is its only install site —
@@ -424,12 +425,12 @@
 //     with no rebuild behind it would drift on the very next resize.
 //     That is safe in both guard cases. resize-pane -y against a window's
 //     sole pane is a verified silent no-op (exit 0, height unchanged), so
-//     the len(live) < 2 case's surviving header pin cannot contradict
+//     the len(live) < 2 case's surviving Selvage pin cannot contradict
 //     render.Rules' sole-cell branch.
 //     And in the !anyPlacedStrand case — reachable via the operator remedy
 //     state.go documents, which deletes reed.json while the session keeps
 //     running untracked only until the next mutating verb reaps it — the
-//     surviving array is a benefit, still holding the live header and
+//     surviving array is a benefit, still holding the live Selvage and
 //     strips at the budgets reed last
 //     computed for them.
 //     Since the signal entry rides the same array, the same rule decides it:
@@ -441,7 +442,7 @@
 //     template_posix.yaml's "height: 50" boot box showing through the BARE
 //     (unchained) attach path, not evidence of a miscomputed layout — a
 //     synthetic bare attach reproduces the reported table exactly, with 40
-//     and 50 rows leaving the header at 1 row and 76 rows taking it to 10.
+//     and 50 rows leaving Selvage at 1 row and 76 rows taking it to 10.
 //     The watchdog's own run-shell signal command (watchdog.go's
 //     resizeHookCommand) lives in THIS SAME array, appended as one further
 //     entry by installResizePinsLocked whenever the watchdog is enabled —
@@ -598,23 +599,21 @@
 //     the layout (exit 1, "have 3 panes but need 2") and destroys nothing;
 //     when the count still matches but membership shifted, cells apply
 //     positionally, so a strand ends up mis-sized rather than lost.
-//   - The two geometry option pins (windowsize.go): "status off" and
-//     "window-size latest" are pinned session/window-targeted
-//     (-t '=<session>:', and -w for window-size, per the Session targeting
-//     grammar above) both at boot and again in AttachArgv's pre-flight, and
-//     their EFFECTIVE values are read back with display-message rather than
-//     trusted from set-option's exit status, because a -g pin plus exit 0 is
-//     not proof the option took — verified live, tmux 3.6: a session-scoped
-//     "status on" survives a global "set-option -g status off" with exit 0,
-//     and a window-scoped "window-size manual" survives the global "latest"
-//     pin the same way. "#{status}" feeds the reserved-row count reserved
-//     for the status line ("off" -> 0, "on" -> 1, a numeric N -> N); a
-//     "#{window-size}" other than "latest", or either readback erroring or
-//     answering an unrecognised value, suppresses the chain rather than
-//     risking a wrong-height string. Unlike the remain-on-exit/mouse pins
-//     beside them, both pins and both readbacks here are NON-FATAL: those
-//     two are correctness dependencies, these two are geometry-quality
-//     options whose absence degrades to a working session, and psmux's
+//   - The geometry option pins (windowsize.go): pinGeometryOptionsLocked pins seven status-line
+//     options — "status" "on"; "status-position" "bottom"; "status-left" <rendered text>;
+//     "status-right" ""; "status-left-length" <computed>; and, window-targeted with -w,
+//     "window-status-format" "" and "window-status-current-format" "" — plus "window-size" "latest",
+//     all session/window-targeted (-t '=<session>:', and -w for window-size, per the Session targeting
+//     grammar above) both at boot and again in AttachArgv's pre-flight. Their EFFECTIVE values are read
+//     back with display-message rather than trusted from set-option's exit status, because a -g pin
+//     plus exit 0 is not proof the option took — verified live, tmux 3.6: a session-scoped "status on"
+//     survives a global "set-option -g status off" with exit 0, and a window-scoped "window-size
+//     manual" survives the global "latest" pin the same way. "#{status}" feeds the reserved-row count
+//     reserved for the status line ("off" -> 0, "on" -> 1, a numeric N -> N); a "#{window-size}" other
+//     than "latest", or either readback erroring or answering an unrecognised value, suppresses the
+//     chain rather than risking a wrong-height string. Unlike the remain-on-exit/mouse pins beside
+//     them, every pin and both readbacks here are NON-FATAL: those two are correctness dependencies,
+//     these are geometry-quality options whose absence degrades to a working session, and psmux's
 //     support for them is unverified anywhere in this repo (Shared Decision
 //     geometry-tmux-failures-are-non-fatal-everywhere).
 //   - window-resized is the only usable resize event source (windowsize.go,
@@ -625,8 +624,9 @@
 //     would re-enter the watcher in an infinite loop. window-resized fires
 //     exactly once per settled size, after the window already has the new
 //     geometry, on both growth and shrink.
-//   - SIGWINCH is not a substitute (reedcli/header.go's blocking tail): with
-//     the header pinned to one row, growing the window delivers SIGWINCH
+//   - SIGWINCH is not a substitute (reedcli/watchdog.go's daemon process):
+//     the daemon has no pane of its own to receive terminal signals from,
+//     but even for a process that did, growing the window delivers SIGWINCH
 //     every time — and that growth IS the layout bug — but SHRINKING
 //     delivers nothing while the strand budgets below are silently violated
 //     (at 30 rows the bottom strand had been squeezed from 15 rows to 2). A
@@ -679,13 +679,15 @@
 //     second return value — otherwise a fallback that happens to equal the
 //     last applied box skips forever and one that differs re-applies
 //     forever.
-//   - The header pane's stdout/stderr is its screen (reedcli/header.go): the
-//     --blocking tail rebinds the logger's stderr sink to a discarding
-//     writer before entering the loop; the durable sink is untouched.
-//   - testing.Testing() gates the header launch line (headerpane.go,
-//     lifecycle.go): no Go test can exercise a header-hosted watch loop by
-//     booting a header pane, which is why the tier-2 proof runs the loop
-//     in-process against a real session instead.
+//   - The watchdog daemon's own stderr is discarded, not watched
+//     (reedcli/watchdog.go): the daemon points the logger's durable sink at
+//     fabricengine.HubLogsDir(hub) FIRST, then rebinds the logger's stderr
+//     half to a discarding writer before it starts polling — the ordering is
+//     what keeps its diagnostics reachable at all, since nothing reads a
+//     detached process's stdio. This is also the long-lived process that
+//     holds the logger's rebound output for its whole life: unlike a
+//     one-shot verb, the daemon's SetOutput call persists for as long as the
+//     process runs.
 //
 // requiredSubcommands (probe.go) still does not grow for the live-geometry
 // rule, the attach chain, or the two option pins: display-message,
diff --git a/internal/reedengine/generation.go b/internal/reedengine/generation.go
index 7e3ef22a9..fd7a93995 100644
--- a/internal/reedengine/generation.go
+++ b/internal/reedengine/generation.go
@@ -1,5 +1,5 @@
 // generation.go implements the pane-generation stamp: the identity of the tmux session incarnation
-// that a persisted reed.json's PaneIDs and HeaderPaneID were bound against, the probe that reads
+// that a persisted reed.json's PaneIDs and SelvagePaneID were bound against, the probe that reads
 // that identity off a live session, and the two load-time guards built on it — clearing bindings
 // minted against a session that is no longer the one running, and refusing to operate when this
 // worktree's recorded session is still alive on the shared socket under a different name.
@@ -148,7 +148,7 @@ func (e *Engine) adoptPaneGenerationLocked(st *ReedState) error {
 		"recordedSession", recorded.SessionName, "recordedTmuxSession", recorded.TmuxSessionID, "recordedServerPID", recorded.ServerPID,
 		"liveTmuxSession", live.TmuxSessionID, "liveServerPID", live.ServerPID)
 	clearAllPaneBindings(st)
-	st.HeaderPaneID = ""
+	st.SelvagePaneID = ""
 	st.PaneGeneration = live
 	return nil
 }
diff --git a/internal/reedengine/generation_test.go b/internal/reedengine/generation_test.go
index 5add86941..0cacfc237 100644
--- a/internal/reedengine/generation_test.go
+++ b/internal/reedengine/generation_test.go
@@ -252,7 +252,7 @@ func TestAdoptPaneGenerationLocked(t *testing.T) {
 			e.tmux.execHook = generationHook(t, tt.generations)
 
 			st := &ReedState{
-				HeaderPaneID:   "%1",
+				SelvagePaneID:  "%1",
 				Strands:        []Strand{{GUID: "a", PaneID: "%2"}},
 				PaneGeneration: tt.recorded,
 			}
@@ -274,10 +274,10 @@ func TestAdoptPaneGenerationLocked(t *testing.T) {
 				t.Fatalf("adoptPaneGenerationLocked() = %v; want nil", err)
 			}
 
-			gotCleared := st.HeaderPaneID == "" && findStrandPaneID(st.Strands, "a") == ""
+			gotCleared := st.SelvagePaneID == "" && findStrandPaneID(st.Strands, "a") == ""
 			if gotCleared != tt.wantCleared {
-				t.Errorf("bindings cleared = %v (HeaderPaneID=%q, strand a=%q); want cleared = %v",
-					gotCleared, st.HeaderPaneID, findStrandPaneID(st.Strands, "a"), tt.wantCleared)
+				t.Errorf("bindings cleared = %v (SelvagePaneID=%q, strand a=%q); want cleared = %v",
+					gotCleared, st.SelvagePaneID, findStrandPaneID(st.Strands, "a"), tt.wantCleared)
 			}
 			if st.PaneGeneration != tt.wantGeneration {
 				t.Errorf("PaneGeneration = %+v; want %+v", st.PaneGeneration, tt.wantGeneration)
diff --git a/internal/reedengine/geometry.go b/internal/reedengine/geometry.go
index 6292f2d02..08338df4e 100644
--- a/internal/reedengine/geometry.go
+++ b/internal/reedengine/geometry.go
@@ -1,6 +1,7 @@
-// geometry.go declares Geometry, the eight-field struct reed is told its coordinates through.
+// geometry.go declares Geometry, the nine-field struct reed is told its coordinates through.
 // It declares the type only — New and every method stay in their existing files (lock.go,
-// lifecycle.go, strand.go, header.go); this file adds no constructor, no validator, and no default.
+// lifecycle.go, strand.go, statusline.go); this file adds no constructor, no validator, and no
+// default.
 
 package reedengine
 
@@ -42,8 +43,10 @@ type Geometry struct {
 	WorktreeRoot string
 	// LogsDir is the shared per-hub server's runtime log directory.
 	LogsDir string
-	// RepoName is the header pane's "repo" token, passed through internal/tokenvocab.
+	// RepoName is the status-line's "repo" token, passed through internal/tokenvocab.
 	RepoName string
-	// HubPath is the header pane's "hub" token, passed through internal/tokenvocab.
+	// WorktreeName is the status-line's "worktree" token, passed through internal/tokenvocab.
+	WorktreeName string
+	// HubPath is the status-line's "hub" token, passed through internal/tokenvocab.
 	HubPath string
 }
diff --git a/internal/reedengine/header.go b/internal/reedengine/header.go
deleted file mode 100644
index c5ead5562..000000000
--- a/internal/reedengine/header.go
+++ /dev/null
@@ -1,28 +0,0 @@
-// header.go implements Engine.HeaderText and Engine.ValidateHeader: the header pane's
-// text-rendering pipeline over internal/tokenvocab,
-// and the eager, loud validation hook the boot path (batch 4) runs before the session comes up.
-
-package reedengine
-
-import "github.com/Knatte18/loomyard/internal/tokenvocab"
-
-// HeaderText renders this hub's header-pane text.
-func (e *Engine) HeaderText() (string, error) {
-	template := []byte(e.cfg.Header.Template)
-	if len(template) == 0 {
-		template = HeaderTemplate()
-	}
-
-	ctx := tokenvocab.Ctx{RepoName: e.geom.RepoName, HubPath: e.geom.HubPath}
-	rendered, err := tokenvocab.Render(template, ctx)
-	if err != nil {
-		return "", err
-	}
-	return string(rendered), nil
-}
-
-// ValidateHeader reports whether this hub's configured header template renders cleanly.
-func (e *Engine) ValidateHeader() error {
-	_, err := e.HeaderText()
-	return err
-}
diff --git a/internal/reedengine/header_test.go b/internal/reedengine/header_test.go
deleted file mode 100644
index c1e99e8c9..000000000
--- a/internal/reedengine/header_test.go
+++ /dev/null
@@ -1,63 +0,0 @@
-// header_test.go covers HeaderText and ValidateHeader hermetically: an Engine built from
-// Config/Geometry struct literals (no lyxcwd.Resolve, no tmux spawn), per the Test Tier
-// Purity Invariant.
-
-package reedengine
-
-import (
-	"strings"
-	"testing"
-)
-
-// newHeaderTestEngine builds a test Engine with the given header template.
-func newHeaderTestEngine(template string) *Engine {
-	geom := Geometry{RepoName: "test-repo", HubPath: "test-hub"}
-	cfg := Config{
-		Header: HeaderConfig{Template: template},
-	}
-	return New(cfg, geom)
-}
-
-func TestHeaderText_EmptyTemplateRendersEmbeddedDefault(t *testing.T) {
-	e := newHeaderTestEngine("")
-
-	got, err := e.HeaderText()
-	if err != nil {
-		t.Fatalf("HeaderText() unexpected error: %v", err)
-	}
-
-	want := "hub: " + e.geom.HubPath
-	if strings.TrimSpace(got) != want {
-		t.Errorf("HeaderText() = %q; want %q", strings.TrimSpace(got), want)
-	}
-}
-
-func TestHeaderText_ConfiguredTemplateRendersFromConfig(t *testing.T) {
-	e := newHeaderTestEngine("repo: {{.repo}}")
-
-	got, err := e.HeaderText()
-	if err != nil {
-		t.Fatalf("HeaderText() unexpected error: %v", err)
-	}
-
-	want := "repo: " + e.geom.RepoName
-	if got != want {
-		t.Errorf("HeaderText() = %q; want %q", got, want)
-	}
-}
-
-func TestValidateHeader_UnknownTopLevelTokenErrors(t *testing.T) {
-	e := newHeaderTestEngine("{{.slug}}")
-
-	if err := e.ValidateHeader(); err == nil {
-		t.Error("ValidateHeader() = nil; want an error for an unknown top-level token")
-	}
-}
-
-func TestValidateHeader_GoodTemplateReturnsNil(t *testing.T) {
-	e := newHeaderTestEngine("repo: {{.repo}}")
-
-	if err := e.ValidateHeader(); err != nil {
-		t.Errorf("ValidateHeader() = %v; want nil", err)
-	}
-}
diff --git a/internal/reedengine/headerpane.go b/internal/reedengine/headerpane.go
deleted file mode 100644
index 6b09b0282..000000000
--- a/internal/reedengine/headerpane.go
+++ /dev/null
@@ -1,23 +0,0 @@
-// headerpane.go implements headerLaunchCmd/headerLaunchLine, the pure helpers that compose the
-// shell command line the always-present header pane runs.
-// They are kept separate from the boot site (lifecycle.go) that actually creates the pane so the
-// command-string assembly stays host-testable with a fake exe: the real os.Executable() lookup
-// happens only at the boot site, never here.
-
-package reedengine
-
-import "github.com/Knatte18/loomyard/internal/shell"
-
-// headerLaunchCmd returns the shell command line the header pane runs at boot.
-func headerLaunchCmd(sh shell.Shell, exe string) string {
-	return sh.Invoke(exe) + " " + sh.Quote("reed") + " " + sh.Quote("header") + " " + sh.Quote("--blocking")
-}
-
-// headerLaunchLine returns the command line to type into the header pane,
-// or "" when the pane must be left as a bare shell (underTest=true).
-func headerLaunchLine(sh shell.Shell, exe string, underTest bool) string {
-	if underTest {
-		return ""
-	}
-	return headerLaunchCmd(sh, exe)
-}
diff --git a/internal/reedengine/headerpane_test.go b/internal/reedengine/headerpane_test.go
deleted file mode 100644
index 2b8118352..000000000
--- a/internal/reedengine/headerpane_test.go
+++ /dev/null
@@ -1,56 +0,0 @@
-// headerpane_test.go covers headerLaunchCmd's pure command-string composition against both real
-// Shell implementations (posix, pwsh) with a fake exe path, and headerLaunchLine's underTest
-// suppression branch — hermetic, no live tmux required.
-
-package reedengine
-
-import (
-	"testing"
-
-	"github.com/Knatte18/loomyard/internal/shell"
-)
-
-func TestHeaderLaunchCmd(t *testing.T) {
-	tests := []struct {
-		name string
-		sh   shell.Shell
-		exe  string
-		want string
-	}{
-		{
-			name: "Posix",
-			sh:   shell.Posix(),
-			exe:  "/opt/lyx/bin/lyx",
-			want: "'/opt/lyx/bin/lyx' 'reed' 'header' '--blocking'",
-		},
-		{
-			name: "Pwsh",
-			sh:   shell.Pwsh(),
-			exe:  `C:\tools\lyx.exe`,
-			want: `& 'C:\tools\lyx.exe' 'reed' 'header' '--blocking'`,
-		},
-	}
-	for _, tt := range tests {
-		t.Run(tt.name, func(t *testing.T) {
-			if got := headerLaunchCmd(tt.sh, tt.exe); got != tt.want {
-				t.Errorf("headerLaunchCmd(%s, %q) = %q, want %q", tt.name, tt.exe, got, tt.want)
-			}
-		})
-	}
-}
-
-func TestHeaderLaunchLine(t *testing.T) {
-	sh := shell.Posix()
-	exe := "/opt/lyx/bin/lyx"
-
-	if got := headerLaunchLine(sh, exe, true); got != "" {
-		t.Errorf("headerLaunchLine(underTest=true) = %q, want \"\" (bare shell pane, no re-exec)", got)
-	}
-	if got, want := headerLaunchLine(sh, exe, false), headerLaunchCmd(sh, exe); got != want {
-		t.Errorf("headerLaunchLine(underTest=false) = %q, want %q", got, want)
-	}
-
-	if !testing.Testing() {
-		t.Fatalf("testing.Testing() = false inside a test binary; the boot site's underTest wiring relies on it being true here")
-	}
-}
diff --git a/internal/reedengine/headertemplate.go b/internal/reedengine/headertemplate.go
deleted file mode 100644
index 5537a30ff..000000000
--- a/internal/reedengine/headertemplate.go
+++ /dev/null
@@ -1,17 +0,0 @@
-// headertemplate.go embeds the default header-pane text template asset, console-header.md.
-// The asset is rendered via tokenvocab.Render (internal/tokenvocab/render.go:12),
-// which is itself a thin wrapper over stencil.Fill.
-// This asset is deliberately outside the stencil mechanism: it is a tmux pane display banner,
-// not a producer prompt, so it stays embedded here and is never seeded, stamped, or read from the hub's stencils directory.
-
-package reedengine
-
-import _ "embed"
-
-//go:embed console-header.md
-var headerTemplate []byte
-
-// HeaderTemplate returns the embedded default header-pane text template's raw bytes.
-func HeaderTemplate() []byte {
-	return headerTemplate
-}
diff --git a/internal/reedengine/lifecycle.go b/internal/reedengine/lifecycle.go
index ddbdfba2d..8e88a542c 100644
--- a/internal/reedengine/lifecycle.go
+++ b/internal/reedengine/lifecycle.go
@@ -21,7 +21,6 @@ import (
 	"github.com/Knatte18/loomyard/internal/lyxdirs"
 	"github.com/Knatte18/loomyard/internal/proc"
 	"github.com/Knatte18/loomyard/internal/reedengine/render"
-	"github.com/Knatte18/loomyard/internal/shell"
 )
 
 // stateDir returns the path to the worktree-level ephemeral tree holding reed.json and reed.lock.
@@ -180,7 +179,7 @@ func (e *Engine) sessionSubstrateLocked() (up bool, usable bool, err error) {
 
 // ensureServerAndSessionLocked ensures this hub's tmux server and this
 // worktree's session exist. Reports booted=true on fresh spawn; validates
-// capability, debug_log, mouse, watchdog, and header template before any tmux round trip.
+// capability, debug_log, mouse, watchdog, and status-line template before any tmux round trip.
 func (e *Engine) ensureServerAndSessionLocked() (booted bool, strippedKeys []string, err error) {
 	// Validate debug_log before anything else touches tmux: a misconfigured
 	// value is a pure config error, unrelated to server/session state, so it
@@ -206,8 +205,8 @@ func (e *Engine) ensureServerAndSessionLocked() (booted bool, strippedKeys []str
 		return false, nil, err
 	}
 
-	// Validate the header template in the same pre-tmux block — it reads
-	// only cfg+geometry (HeaderText makes no tmux round trip), so like
+	// Validate the status-line template in the same pre-tmux block — it reads
+	// only cfg+geometry (StatusLineText makes no tmux round trip), so like
 	// debug_log and mouse it must fail the boot before anything is spawned.
 	// An earlier version validated only AFTER the session existed, which
 	// left a half-created session behind on a bad template — and, on the
@@ -220,7 +219,7 @@ func (e *Engine) ensureServerAndSessionLocked() (booted bool, strippedKeys []str
 	// path into that trap; a set-option failure between spawn and return
 	// can still theoretically lose the signal, but has no config-shaped
 	// trigger.
-	if err := e.ValidateHeader(); err != nil {
+	if err := e.ValidateStatusLine(); err != nil {
 		return false, nil, err
 	}
 
@@ -248,7 +247,7 @@ func (e *Engine) ensureServerAndSessionLocked() (booted bool, strippedKeys []str
 	}
 	if up {
 		if usable {
-			// The header template was already validated in the pre-tmux
+			// The status-line template was already validated in the pre-tmux
 			// block above, so this healthy already-up path returns directly.
 			return false, nil, nil
 		}
@@ -462,14 +461,14 @@ func stripTraceID(env []string) []string {
 	return out
 }
 
-// ensureHeaderPaneLocked ensures the header pane exists and is alive.
-// (Re)creates it when missing, dead, or gone. The header is separate from
-// strands and must land physically topmost so layout heights stay correct.
-// The (re)creation itself is splitHeaderPaneAtTopLocked's job, including the
-// even-vertical retry that keeps a stale or lost HeaderPaneID from wedging the
-// worktree — see that function for why a split against the top pane can fail
-// at all.
-func (e *Engine) ensureHeaderPaneLocked(st *ReedState) error {
+// ensureSelvagePaneLocked ensures Selvage exists and is alive.
+// (Re)creates it when missing, dead, or gone. Selvage is separate from
+// strands and must land physically bottom-most so layout heights stay
+// correct. The (re)creation itself is splitSelvagePaneAtBottomLocked's job,
+// including the even-vertical retry that keeps a stale or lost
+// SelvagePaneID from wedging the worktree — see that function for why a
+// split against the bottom pane can fail at all.
+func (e *Engine) ensureSelvagePaneLocked(st *ReedState) error {
 	session := e.SessionName()
 	live, err := e.tmux.listPanes(session)
 	if err != nil {
@@ -481,112 +480,103 @@ func (e *Engine) ensureHeaderPaneLocked(st *ReedState) error {
 		// panicking on an empty slice below. ensureServerAndSessionLocked
 		// kills husks before this runs, so reaching this means the session
 		// emptied between the two probes — an error, not an invariant.
-		return fmt.Errorf("session %s has no panes to split a header pane from", session)
+		return fmt.Errorf("session %s has no panes to split Selvage from", session)
 	}
 
-	if st.HeaderPaneID != "" && aliveIDSet(live)[st.HeaderPaneID] {
+	if st.SelvagePaneID != "" && aliveIDSet(live)[st.SelvagePaneID] {
 		// Present AND alive: idempotent no-op across up/resume. Aliveness,
-		// not mere presence, is the check — a dead-but-present header corpse
-		// (kept enumerable by reconcile's deliberate exemption) must be
-		// healed here, not mistaken for a working header.
+		// not mere presence, is the check — a dead-but-present Selvage
+		// corpse (kept enumerable by reconcile's deliberate exemption) must
+		// be healed here, not mistaken for a working Selvage.
 		return nil
 	}
 
-	// A dead-but-present header corpse is killed before the replacement is
-	// split, so its top row is freed and the topmost target below is a real
-	// (usually alive) pane — unless the corpse is the session's SOLE pane,
-	// where killing first would end the session; then the corpse itself is
-	// the split target and it is killed after the new pane exists.
+	// A dead-but-present Selvage corpse is killed before the replacement is
+	// split, so its bottom row is freed and the bottommost target below is a
+	// real (usually alive) pane — unless the corpse is the session's SOLE
+	// pane, where killing first would end the session; then the corpse
+	// itself is the split target and it is killed after the new pane
+	// exists.
 	corpseID := ""
-	if st.HeaderPaneID != "" && liveIDSet(live)[st.HeaderPaneID] {
-		corpseID = st.HeaderPaneID
+	if st.SelvagePaneID != "" && liveIDSet(live)[st.SelvagePaneID] {
+		corpseID = st.SelvagePaneID
 		if len(live) > 1 {
 			if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
-				return fmt.Errorf("kill dead header pane %s: %w", corpseID, err)
+				return fmt.Errorf("kill dead Selvage pane %s: %w", corpseID, err)
 			}
 			corpseID = ""
 			live, err = e.tmux.listPanes(session)
 			if err != nil {
-				return fmt.Errorf("list panes after killing dead header: %w", err)
+				return fmt.Errorf("list panes after killing dead Selvage: %w", err)
 			}
 			if len(live) == 0 {
-				return fmt.Errorf("session %s has no panes to split a header pane from", session)
+				return fmt.Errorf("session %s has no panes to split Selvage from", session)
 			}
 		}
 	}
 
-	exe, err := os.Executable()
+	// Selvage's trailing split-window argument is e.cfg.Shell, the same way
+	// new-session already launches the session's first pane — an ordinary
+	// interactive shell, never a re-exec of lyx.
+	paneID, err := e.splitSelvagePaneAtBottomLocked(session, live, e.cfg.Shell)
 	if err != nil {
-		return fmt.Errorf("resolve lyx binary path: %w", err)
-	}
-
-	// Computed above the split so it can be passed straight into split-window as its own trailing
-	// shell-command argument: the pane then runs launchCmd directly from birth, hosting no
-	// interactive shell for anything to echo the command into or read ~/.bashrc from.
-	launchCmd := headerLaunchLine(shell.ForGOOS(), exe, e.suppressHeaderLaunch)
-	if launchCmd == "" {
-		// Under go test the header pane stays a bare blocking shell — see
-		// headerLaunchLine: re-exec'ing exe here would run the test binary's
-		// entire suite recursively. The pane still exists and its id is still
-		// recorded below, so layout geometry and up/resume idempotence are
-		// unchanged.
-		logger.Info("reed: header re-exec suppressed under go test, pane left as bare shell", "socket", e.Socket(), "exe", exe)
-	}
-
-	paneID, err := e.splitHeaderPaneAtTopLocked(session, live, launchCmd)
-	if err != nil {
-		return fmt.Errorf("split header pane: %w", err)
+		return fmt.Errorf("split Selvage pane: %w", err)
 	}
 
 	if corpseID != "" {
-		// The sole-pane corpse the new header was split off of: now that a
+		// The sole-pane corpse the new Selvage was split off of: now that a
 		// second pane exists, killing it can no longer end the session.
 		// Best-effort — a corpse that somehow vanished already is fine; the
 		// discard is still worth a Debug line so the step is observable at
 		// the trace level without upgrading routine cleanup to a Warn.
 		if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
-			logger.Debug("reed: best-effort kill of header corpse pane failed", "socket", e.Socket(), "pane", corpseID, "err", err)
+			logger.Debug("reed: best-effort kill of Selvage corpse pane failed", "socket", e.Socket(), "pane", corpseID, "err", err)
 		}
 	}
 
-	st.HeaderPaneID = paneID
+	st.SelvagePaneID = paneID
 	if err := SaveState(e.stateDir(), st); err != nil {
-		return fmt.Errorf("persist header pane id: %w", err)
+		return fmt.Errorf("persist Selvage pane id: %w", err)
 	}
 	return nil
 }
 
-// topmostPaneID returns the id of the pane sitting physically highest in the window — the smallest
-// pane_top — which is the only place a header pane may be split in.
+// bottommostPaneID returns the id of the pane sitting physically lowest in the window — the largest
+// pane_top — which is the only place Selvage may be split in.
 // live must be non-empty.
-func topmostPaneID(live []LivePane) string {
-	topmost := live[0]
+func bottommostPaneID(live []LivePane) string {
+	bottommost := live[0]
 	for _, p := range live[1:] {
-		if p.Top < topmost.Top {
-			topmost = p
+		if p.Top > bottommost.Top {
+			bottommost = p
 		}
 	}
-	return topmost.ID
+	return bottommost.ID
 }
 
-// splitHeaderPaneAtTopLocked splits a new pane in above the physically topmost pane of session and
-// returns its id, retrying once behind an even-vertical re-tile when the first attempt has no room.
+// splitSelvagePaneAtBottomLocked splits a new pane in below the physically bottom-most pane of
+// session and returns its id, retrying once behind an even-vertical re-tile when the first attempt
+// has no room.
+//
+// The retry is what keeps a lost or stale ReedState.SelvagePaneID from wedging a worktree
+// permanently (R4 review finding R4-F4). The Selvage band is one row by default, and tmux cannot
+// split a one-row pane at all — so the moment SelvagePaneID stops naming the pane at the bottom,
+// the bottommost split target IS an untracked one-row band and every later up/resume fails with
+// "no space for new pane", forever, while status keeps reporting the session healthy and the only
+// escape ("lyx reed down", then up) is named nowhere. Two ordinary routes reach that state:
+// scrubbing .lyx/reed.json, a never-tracked machine-local tree the Durable-vs-Ephemeral State
+// Invariant makes disposable (a plain `git clean -xdf` in the worktree does it), and a process
+// death in the window between the split above and the SaveState that records its id.
 //
-// The retry is what keeps a lost or stale ReedState.HeaderPaneID from wedging a worktree
-// permanently (R4 review finding R4-F4). The header band is one row by default
-// (HeaderConfig.HeightRows), and tmux cannot split a one-row pane at all — so the moment
-// HeaderPaneID stops naming the pane at the top, the topmost split target IS an untracked one-row
-// band and every later up/resume fails with "no space for new pane", forever, while status keeps
-// reporting the session healthy and the only escape ("lyx reed down", then up) is named nowhere.
-// Two ordinary routes reach that state: scrubbing .lyx/reed.json, a never-tracked machine-local
-// tree the Durable-vs-Ephemeral State Invariant makes disposable (a plain `git clean -xdf` in the
-// worktree does it), and a process death in the window between the split above and the SaveState
-// that records its id.
+// The physical-position requirement is symmetric to the header's former top-placement requirement:
+// render.Rules emits the band cell LAST and paneIDsByTop resequences by pane_top, so a Selvage pane
+// that is not physically bottom-most would invert cell heights on the very first select-layout.
 //
 // select-layout even-vertical evens every pane's height using tmux's own built-in layout — no reed
 // layout string is computed or applied here, so anyPlacedStrand's empty-layout hazard (apply.go) is
-// not in play — after which the same split has room and STILL lands the new pane at pane_top 0
-// (verified live, tmux 3.6). The op's normal reconcileApplyPersistLocked tail then restores reed's
+// not in play — after which the same split has room again; the even-vertical re-tile retry survives
+// this flip verbatim, because tmux cannot split a one-row pane at all, which is just as true at the
+// bottom as it was at the top. The op's normal reconcileApplyPersistLocked tail then restores reed's
 // real geometry and reaps the untracked band; an op that fails before reaching that tail leaves the
 // window evenly tiled, a cosmetic state the next successful op corrects.
 // Both subcommands are already in requiredSubcommands, so the multiplexer capability contract is
@@ -595,15 +585,15 @@ func topmostPaneID(live []LivePane) string {
 // On a failed retry the FIRST error is returned, not the retry's: it describes the state the
 // operator actually has, and the re-tile is an internal repair attempt rather than something they
 // asked for.
-func (e *Engine) splitHeaderPaneAtTopLocked(session string, live []LivePane, launchCmd string) (string, error) {
-	paneID, firstErr := e.splitPaneAboveLocked(topmostPaneID(live), live, launchCmd)
+func (e *Engine) splitSelvagePaneAtBottomLocked(session string, live []LivePane, launchCmd string) (string, error) {
+	paneID, firstErr := e.splitPaneBelowLocked(bottommostPaneID(live), live, launchCmd)
 	if firstErr == nil {
 		return paneID, nil
 	}
-	logger.Warn("reed: failed to split header pane, retrying behind an even-vertical re-tile", "socket", e.Socket(), "session", session, "err", firstErr)
+	logger.Warn("reed: failed to split Selvage pane, retrying behind an even-vertical re-tile", "socket", e.Socket(), "session", session, "err", firstErr)
 
 	if err := e.tmux.run("select-layout", "-t", exactSessionWindowTarget(session), "even-vertical"); err != nil {
-		logger.Warn("reed: even-vertical re-tile failed, header split not retried", "socket", e.Socket(), "session", session, "err", err)
+		logger.Warn("reed: even-vertical re-tile failed, Selvage split not retried", "socket", e.Socket(), "session", session, "err", err)
 		return "", firstErr
 	}
 	retiled, err := e.tmux.listPanes(session)
@@ -611,43 +601,36 @@ func (e *Engine) splitHeaderPaneAtTopLocked(session string, live []LivePane, lau
 		logger.Warn("reed: could not re-enumerate panes after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
 		return "", firstErr
 	}
-	// The retry carries launchCmd too — a retried header must never boot commandless, or it would
+	// The retry carries launchCmd too — a retried Selvage must never boot commandless, or it would
 	// be left hosting an interactive shell exactly like the noise this batch removes.
-	paneID, err = e.splitPaneAboveLocked(topmostPaneID(retiled), retiled, launchCmd)
+	paneID, err = e.splitPaneBelowLocked(bottommostPaneID(retiled), retiled, launchCmd)
 	if err != nil {
-		logger.Warn("reed: header split still had no room after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
+		logger.Warn("reed: Selvage split still had no room after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
 		return "", firstErr
 	}
-	logger.Info("reed: header split recovered by an even-vertical re-tile", "socket", e.Socket(), "session", session, "pane", paneID)
+	logger.Info("reed: Selvage split recovered by an even-vertical re-tile", "socket", e.Socket(), "session", session, "pane", paneID)
 	return paneID, nil
 }
 
-// splitPaneAboveLocked splits a new pane in directly above target and returns its id, refusing an
+// splitPaneBelowLocked splits a new pane in directly below target and returns its id, refusing an
 // id that was already present in preSplitLive.
 //
-// -b places the NEW pane above target rather than below it (tmux's default split direction is
-// vertical, new pane below): render.Rules always emits the header cell FIRST, assuming a fixed top
-// band, and psmux/tmux apply layout cells POSITIONALLY to the window's actual top-to-bottom pane
-// order — so the header pane must physically stay topmost, or the very first select-layout would
-// invert the header and the first strand's heights (verified live: without -b, a stacked-adds smoke
-// scenario failed a later split with "no space for new pane" because the 1-row header cell landed
-// on the STRAND's physically-top pane instead). Every strand split (spawn.go) always targets a
-// non-header pane and inserts below it, so the header is the only split in the whole engine that
-// needs -b.
+// No -b flag is needed here: tmux's default split direction is vertical with the new pane below,
+// which is exactly where Selvage must land now that render.Rules emits the band cell LAST rather
+// than first. Selvage is still the only split in the whole engine that must land at a specific
+// physical edge — every strand split (spawn.go) always targets a non-Selvage pane and inserts
+// below it too, but strands have no positional requirement of their own the way Selvage does.
 //
 // The genuinely-new-pane guard is the same one launchStrandLocked runs: psmux's silent
-// too-small-to-split failure prints an EXISTING pane's id with exit 0, and recording that id as the
-// header would bind the header to a strand's pane — the next layout string would then carry a
-// duplicate pane number, destroying the session's panes wholesale (see
-// validateSplitCreatedNewPane).
-func (e *Engine) splitPaneAboveLocked(target string, preSplitLive []LivePane, launchCmd string) (string, error) {
-	argv := []string{"split-window", "-b", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}"}
+// too-small-to-split failure prints an EXISTING pane's id with exit 0, and recording that id as
+// Selvage would bind Selvage to a strand's pane — the next layout string would then carry a
+// duplicate pane number, destroying the session's panes wholesale (see validateSplitCreatedNewPane).
+func (e *Engine) splitPaneBelowLocked(target string, preSplitLive []LivePane, launchCmd string) (string, error) {
+	argv := []string{"split-window", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}"}
 	if launchCmd != "" {
 		// A single trailing shell-command argument, exactly like an interactive `tmux split-window`
 		// invocation's own trailing-command syntax: the pane then runs launchCmd directly rather than
-		// an interactive shell, so nothing types it, echoes it, or reads a shell rc file for it. Empty
-		// launchCmd leaves the argv exactly as it was before this parameter existed — the go test
-		// default (see Engine.suppressHeaderLaunch).
+		// an interactive shell, so nothing types it, echoes it, or reads a shell rc file for it.
 		argv = append(argv, launchCmd)
 	}
 	out, err := e.tmux.output(argv...)
@@ -684,20 +667,20 @@ func (e *Engine) upLocked() (UpResult, bool, error) {
 	// live strand. Clear every binding: a just-booted session hosts none
 	// of the prior strands. Up leaves them not-live (Resume rebuilds them).
 	// The stripped env keys are stamped for diagnosis — reed.json records
-	// what the server spawn actually removed. HeaderPaneID is cleared
+	// what the server spawn actually removed. SelvagePaneID is cleared
 	// alongside every strand binding for the identical reason — a
 	// reborn session's reused pane id would otherwise be mistaken for
-	// the still-live header pane — so ensureHeaderPaneLocked below
+	// the still-live Selvage pane — so ensureSelvagePaneLocked below
 	// rebuilds it fresh; the clear lives here, not inside
-	// clearAllPaneBindings itself, since the header is not a strand
+	// clearAllPaneBindings itself, since Selvage is not a strand
 	// binding.
 	if booted {
 		clearAllPaneBindings(st)
 		st.StrippedEnv = stripped
-		st.HeaderPaneID = ""
+		st.SelvagePaneID = ""
 	}
 
-	if err := e.ensureHeaderPaneLocked(st); err != nil {
+	if err := e.ensureSelvagePaneLocked(st); err != nil {
 		return result, booted, err
 	}
 
@@ -705,11 +688,11 @@ func (e *Engine) upLocked() (UpResult, bool, error) {
 		return result, booted, err
 	}
 
-	// len(st.Strands) deliberately excludes the header pane: the header
-	// is not in st.Strands (Shared Decision header-is-not-a-strand), so
-	// this count is already correct by construction. Do not "fix" a
-	// future off-by-one here by adding the header — it must never be
-	// counted as a strand.
+	// len(st.Strands) deliberately excludes Selvage: Selvage is not in
+	// st.Strands (Shared Decision header-is-not-a-strand), so this
+	// count is already correct by construction. Do not "fix" a future
+	// off-by-one here by adding Selvage — it must never be counted as
+	// a strand.
 	result = UpResult{Session: e.SessionName(), Socket: e.Socket(), Strands: len(st.Strands)}
 	return result, booted, nil
 }
@@ -733,7 +716,7 @@ func (e *Engine) Up() (UpResult, error) {
 // config validation, no reconcile, no state read, no write — when the session is already usable.
 // The early return exists rather than routing a warm call through upLocked for two reasons: upLocked's
 // tail reaches planReconcile, which adds every live non-exempt pane to its kill list whenever the
-// header is alive, and ensureServerAndSessionLocked runs its whole pre-tmux config-validation block
+// Selvage pane is alive, and ensureServerAndSessionLocked runs its whole pre-tmux config-validation block
 // ahead of its already-up early return.
 // Otherwise it delegates to upLocked and returns that call's own booted flag verbatim, never a
 // hardcoded true: the session can come up between the two probes (a sibling `lyx reed up` in another
@@ -782,19 +765,19 @@ func (e *Engine) Resume() (ResumeResult, error) {
 		// On a server rebirth the reborn session reuses pane ids, so a stale
 		// binding would look live to reconcile below and wrongly skip relaunch.
 		// Clear every binding first so all non-hidden strands are rebuilt.
-		// HeaderPaneID is cleared alongside them for the identical reason —
+		// SelvagePaneID is cleared alongside them for the identical reason —
 		// a reborn session's reused pane id would otherwise be mistaken for
-		// the still-live header pane — so ensureHeaderPaneLocked below
+		// the still-live Selvage pane — so ensureSelvagePaneLocked below
 		// rebuilds it fresh before any strand replay below runs; the clear
-		// lives here, not inside clearAllPaneBindings itself, since the
-		// header is not a strand binding.
+		// lives here, not inside clearAllPaneBindings itself, since Selvage
+		// is not a strand binding.
 		if booted {
 			clearAllPaneBindings(st)
 			st.StrippedEnv = stripped
-			st.HeaderPaneID = ""
+			st.SelvagePaneID = ""
 		}
 
-		if err := e.ensureHeaderPaneLocked(st); err != nil {
+		if err := e.ensureSelvagePaneLocked(st); err != nil {
 			return err
 		}
 
@@ -1207,10 +1190,9 @@ func (e *Engine) requireSessionLocked() error {
 		return nil
 	}
 
-	// len(st.Strands) deliberately excludes the header pane (see
-	// noSessionMessage's doc comment): st.HeaderPaneID is a separate field,
-	// never part of Strands, so this count is already correct by
-	// construction.
+	// len(st.Strands) deliberately excludes Selvage (see noSessionMessage's
+	// doc comment): st.SelvagePaneID is a separate field, never part of
+	// Strands, so this count is already correct by construction.
 	strandCount := 0
 	st, loadErr := LoadState(e.stateDir())
 	if st != nil {
@@ -1250,11 +1232,11 @@ func (e *Engine) Status() (StatusResult, error) {
 		// the strand's process is running, not whether tmux still lists a
 		// (dead) pane for it.
 		aliveIDs := aliveIDSet(live)
-		// This loop iterates st.Strands only — the header pane is
-		// deliberately never reported as a strand here (it is not one; see
-		// ReedState.HeaderPaneID). Status still succeeds (the session is up)
-		// when st.Strands is empty but the header pane is alive; a future
-		// edit must not "fix" a missing header row by appending one here.
+		// This loop iterates st.Strands only — Selvage is deliberately
+		// never reported as a strand here (it is not one; see
+		// ReedState.SelvagePaneID). Status still succeeds (the session is
+		// up) when st.Strands is empty but Selvage is alive; a future edit
+		// must not "fix" a missing Selvage row by appending one here.
 		strands := make([]StrandStatus, len(st.Strands))
 		for i, s := range st.Strands {
 			strands[i] = StrandStatus{GUID: s.GUID, Name: s.Name, PaneID: s.PaneID, Live: aliveIDs[s.PaneID]}
diff --git a/internal/reedengine/lifecycle_test.go b/internal/reedengine/lifecycle_test.go
index c04826bb3..7071965e2 100644
--- a/internal/reedengine/lifecycle_test.go
+++ b/internal/reedengine/lifecycle_test.go
@@ -33,7 +33,7 @@ func TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact(t *testing.T) {
 	e := newTestEngine(t)
 	e.cfg.DebugLog = "0"
 	e.cfg.Mouse = "off"
-	e.cfg.Header.Template = "{{.bogus}}"
+	e.cfg.StatusLine.Template = "{{.bogus}}"
 
 	_, err := e.Up()
 	if err == nil {
@@ -323,15 +323,15 @@ func TestPlanResumeLaunches_ThreeLifecycleStates(t *testing.T) {
 	}
 }
 
-// TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath pins that the header split-window call
+// TestEnsureSelvagePaneLocked_SplitsWithPaneCwdNotAnchorPath pins that the Selvage split-window call
 // pins its pane to Geometry.PaneCwd, not Geometry.AnchorPath — the two are distinct on newTestEngine's
 // fixture (lock_test.go), so this assertion cannot pass by coincidence.
-// This covers only the header split site: the new-session spawn site is not reachable from this
+// This covers only the Selvage split site: the new-session spawn site is not reachable from this
 // seam, since it builds its argv and runs it through the os/exec package's Command function
 // directly rather than through e.tmux — that half of the same change is covered by the tagged reed
 // suites this batch's verify: also runs (contract_integration_test.go,
 // mouse_boot_integration_test.go).
-func TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath(t *testing.T) {
+func TestEnsureSelvagePaneLocked_SplitsWithPaneCwdNotAnchorPath(t *testing.T) {
 	e := newTestEngine(t)
 
 	const existingPaneID = "%0"
@@ -355,8 +355,8 @@ func TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath(t *testing.T) {
 	}
 
 	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
-	if err := e.ensureHeaderPaneLocked(st); err != nil {
-		t.Fatalf("ensureHeaderPaneLocked: %v", err)
+	if err := e.ensureSelvagePaneLocked(st); err != nil {
+		t.Fatalf("ensureSelvagePaneLocked: %v", err)
 	}
 
 	found := false
@@ -380,14 +380,14 @@ func TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath(t *testing.T) {
 	}
 }
 
-// TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure pins the validateSplitCreatedNewPane
+// TestEnsureSelvagePaneLocked_RebuildRejectsSilentSplitFailure pins the validateSplitCreatedNewPane
 // guard at its call site (against regression).
-func TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure(t *testing.T) {
+func TestEnsureSelvagePaneLocked_RebuildRejectsSilentSplitFailure(t *testing.T) {
 	e := newTestEngine(t)
 
-	// One alive, non-header pane (%0) — the new-session initial pane a fresh
-	// boot leaves before any header exists. It is the only pane, so it is both
-	// the topmost split target and the id psmux's silent-split shape re-prints.
+	// One alive, non-Selvage pane (%0) — the new-session initial pane a fresh
+	// boot leaves before Selvage exists. It is the only pane, so it is both
+	// the bottommost split target and the id psmux's silent-split shape re-prints.
 	const existingPaneID = "%0"
 	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"
 
@@ -397,7 +397,7 @@ func TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure(t *testing.T) {
 			return listPanesOut, nil
 		case "split-window":
 			// psmux silent failure: exit 0, no new pane, an EXISTING pane's id
-			// printed on stdout. Trusting it would bind the header to %0.
+			// printed on stdout. Trusting it would bind Selvage to %0.
 			return existingPaneID + "\n", nil
 		default:
 			// send-keys / kill-pane etc. — only reached if the guard is
@@ -408,41 +408,41 @@ func TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure(t *testing.T) {
 	}
 
 	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
-	err := e.ensureHeaderPaneLocked(st)
+	err := e.ensureSelvagePaneLocked(st)
 	if err == nil {
-		t.Fatalf("ensureHeaderPaneLocked accepted a silent-split failure (bound header to pre-existing pane %q); the validateSplitCreatedNewPane guard at this call site is missing or bypassed", existingPaneID)
+		t.Fatalf("ensureSelvagePaneLocked accepted a silent-split failure (bound Selvage to pre-existing pane %q); the validateSplitCreatedNewPane guard at this call site is missing or bypassed", existingPaneID)
 	}
-	if !strings.Contains(err.Error(), "split header pane") {
-		t.Errorf("error = %v, want it to name the header split failure", err)
+	if !strings.Contains(err.Error(), "split Selvage pane") {
+		t.Errorf("error = %v, want it to name the Selvage split failure", err)
 	}
-	if st.HeaderPaneID != "" {
-		t.Errorf("HeaderPaneID = %q, want unchanged (never bound to the pre-existing strand pane on a rejected rebuild)", st.HeaderPaneID)
+	if st.SelvagePaneID != "" {
+		t.Errorf("SelvagePaneID = %q, want unchanged (never bound to the pre-existing strand pane on a rejected rebuild)", st.SelvagePaneID)
 	}
 }
 
-// TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit is the regression guard for the
-// R4 review's R4-F4: an untracked one-row header band at the physical top of the window made every
-// header rebuild impossible, wedging up and resume permanently with "no space for new pane" while
-// status kept reporting the session healthy.
+// TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit is the regression guard for
+// the R4 review's R4-F4: an untracked one-row Selvage band at the physical bottom of the window made
+// every Selvage rebuild impossible, wedging up and resume permanently with "no space for new pane"
+// while status kept reporting the session healthy.
 //
-// Reproduced live before the fix: with a session up and the default one-row header band laid out,
+// Reproduced live before the fix: with a session up and the default one-row Selvage band laid out,
 // removing .lyx/reed.json — a never-tracked machine-local tree, exactly what `git clean -xdf` in the
 // worktree deletes — left `lyx reed up` and `lyx reed resume` failing identically on every
 // subsequent invocation, with `lyx reed down` the only (unnamed) escape.
 //
-// The scripted substrate below is the shape that produced it: a one-row pane at pane_top 0 that
-// tmux refuses to split, and a tall pane below it. The assertions are that the even-vertical re-tile
-// is actually issued and that the retried split's pane becomes the header — a fix that only improved
-// the error message fails both.
-func TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit(t *testing.T) {
+// The scripted substrate below is the shape that produced it: a one-row pane at the largest pane_top
+// that tmux refuses to split, and a tall pane above it. The assertions are that the even-vertical
+// re-tile is actually issued and that the retried split's pane becomes Selvage — a fix that only
+// improved the error message fails both.
+func TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit(t *testing.T) {
 	e := newTestEngine(t)
 
-	const oneRowTopPaneID = "%1"
+	const oneRowBottomPaneID = "%1"
 	const tallPaneID = "%0"
-	const rebuiltHeaderPaneID = "%7"
+	const rebuiltSelvagePaneID = "%7"
 	// pane_id pane_dead pane_top pane_width pane_height pane_pid
-	wedged := oneRowTopPaneID + " 0 0 100 1 4321\n" + tallPaneID + " 0 2 100 48 4322\n"
-	retiled := oneRowTopPaneID + " 0 0 100 25 4321\n" + tallPaneID + " 0 26 100 24 4322\n"
+	wedged := tallPaneID + " 0 0 100 48 4321\n" + oneRowBottomPaneID + " 0 48 100 1 4322\n"
+	retiled := tallPaneID + " 0 0 100 24 4321\n" + oneRowBottomPaneID + " 0 25 100 25 4322\n"
 
 	reTiled := false
 	splitAttempts := 0
@@ -465,46 +465,36 @@ func TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit(t *testi
 				// tmux's real refusal against a one-row pane: exit 1, no pane.
 				return "", errors.New("exit status 1: no space for new pane")
 			}
-			return rebuiltHeaderPaneID + "\n", nil
+			return rebuiltSelvagePaneID + "\n", nil
 		default:
 			return "", nil
 		}
 	}
 
 	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
-	if err := e.ensureHeaderPaneLocked(st); err != nil {
-		t.Fatalf("ensureHeaderPaneLocked() = %v; want nil (the header rebuild must recover from a one-row top pane, not wedge the worktree)", err)
+	if err := e.ensureSelvagePaneLocked(st); err != nil {
+		t.Fatalf("ensureSelvagePaneLocked() = %v; want nil (the Selvage rebuild must recover from a one-row bottom pane, not wedge the worktree)", err)
 	}
 	if !reTiled {
-		t.Errorf("ensureHeaderPaneLocked never issued the even-vertical re-tile; without it the retried split has no room either")
+		t.Errorf("ensureSelvagePaneLocked never issued the even-vertical re-tile; without it the retried split has no room either")
 	}
 	if splitAttempts != 2 {
 		t.Errorf("split-window attempts = %d; want exactly 2 (one refused, one retried behind the re-tile)", splitAttempts)
 	}
-	if st.HeaderPaneID != rebuiltHeaderPaneID {
-		t.Errorf("HeaderPaneID = %q; want %q (the pane the retried split created)", st.HeaderPaneID, rebuiltHeaderPaneID)
+	if st.SelvagePaneID != rebuiltSelvagePaneID {
+		t.Errorf("SelvagePaneID = %q; want %q (the pane the retried split created)", st.SelvagePaneID, rebuiltSelvagePaneID)
 	}
 }
 
-// enableHeaderLaunch flips e.suppressHeaderLaunch back off, undoing the testing.Testing()-derived
-// default New sets. It is the seam P1 needs: nothing outside this package can reach the unexported
-// field, so a test that must drive the real header-launch path against a fake tmux does it through
-// this helper rather than by exporting the field.
-func enableHeaderLaunch(t *testing.T, e *Engine) {
-	t.Helper()
-	e.suppressHeaderLaunch = false
-}
-
-// TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys is P1: it pins that the
-// header pane is booted by handing split-window the launch line as its own trailing shell-command
+// TestEnsureSelvagePaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys is P1: it pins that
+// Selvage is booted by handing split-window e.cfg.Shell as its own trailing shell-command
 // argument, not by typing it into an interactive shell afterwards via send-keys. Both halves matter
 // — a fix that carries the command on the argv but still sends keys, or vice versa, must fail this.
 //
 // No #{pane_current_command} assertion is added: that value is shell-dependent and this fake-tmux
 // substrate never runs a real shell.
-func TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys(t *testing.T) {
+func TestEnsureSelvagePaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys(t *testing.T) {
 	e := newTestEngine(t)
-	enableHeaderLaunch(t, e)
 
 	const existingPaneID = "%0"
 	const newPaneID = "%1"
@@ -531,8 +521,8 @@ func TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys(t *te
 	}
 
 	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
-	if err := e.ensureHeaderPaneLocked(st); err != nil {
-		t.Fatalf("ensureHeaderPaneLocked: %v", err)
+	if err := e.ensureSelvagePaneLocked(st); err != nil {
+		t.Fatalf("ensureSelvagePaneLocked: %v", err)
 	}
 
 	fIndex := -1
@@ -552,80 +542,65 @@ func TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys(t *te
 		t.Fatalf("split-window argv %v carries no trailing command argument after the -F value; want the launch line appended as split-window's own trailing shell-command argument", splitArgs)
 	}
 	launchArg := splitArgs[fIndex+2]
-	// A substring check, not an exact match, because headerLaunchCmd's posix and pwsh quoting differ
-	// — this must hold for both.
-	for _, want := range []string{"reed", "--blocking"} {
-		if !strings.Contains(launchArg, want) {
-			t.Errorf("split-window trailing command argument = %q, want it to contain %q (the header keepalive invocation)", launchArg, want)
-		}
+	if launchArg != e.cfg.Shell {
+		t.Errorf("split-window trailing command argument = %q, want %q (e.cfg.Shell, launched the same way new-session launches the session's first pane)", launchArg, e.cfg.Shell)
 	}
 	if sendKeysCalls != 0 {
-		t.Errorf("send-keys calls = %d, want 0 (the header pane must launch its own command on the split, not be typed into via send-keys)", sendKeysCalls)
+		t.Errorf("send-keys calls = %d, want 0 (Selvage must launch its own command on the split, not be typed into via send-keys)", sendKeysCalls)
 	}
 }
 
-// TestEnsureHeaderPaneLocked_DefaultUnderGoTestSplitsACommandlessShell pins the preserved go test
-// default: a default newTestEngine leaves suppressHeaderLaunch on (New derives it from
-// testing.Testing()), so the header split must still carry no trailing command argument and issue no
-// send-keys, exactly as it always has — this batch changes how a launch command travels, not whether
-// one is issued under go test.
-func TestEnsureHeaderPaneLocked_DefaultUnderGoTestSplitsACommandlessShell(t *testing.T) {
+// TestEnsureSelvagePaneLocked_RecordsThePaneIDAfterLaunch pins that the split pane's id is recorded
+// onto state even under go test's fake-tmux substrate, which never runs a real shell — recording
+// must not depend on anything the launched command actually does. This used to also pin a
+// suppressed, commandless launch under go test; that suppression mechanism is gone along with the
+// header pane's re-exec Selvage replaces, so the launch itself is covered by
+// TestEnsureSelvagePaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys and this test narrows to the
+// recording half.
+func TestEnsureSelvagePaneLocked_RecordsThePaneIDAfterLaunch(t *testing.T) {
 	e := newTestEngine(t)
 
 	const existingPaneID = "%0"
 	const newPaneID = "%1"
 	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"
 
-	var splitArgs []string
-	sendKeysCalls := 0
 	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
 		switch args[0] {
 		case "list-panes":
 			return listPanesOut, nil
 		case "split-window":
-			splitArgs = append([]string{}, args...)
 			return newPaneID + "\n", nil
-		case "send-keys":
-			sendKeysCalls++
-			return "", nil
 		default:
 			return "", nil
 		}
 	}
 
 	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
-	if err := e.ensureHeaderPaneLocked(st); err != nil {
-		t.Fatalf("ensureHeaderPaneLocked: %v", err)
+	if err := e.ensureSelvagePaneLocked(st); err != nil {
+		t.Fatalf("ensureSelvagePaneLocked: %v", err)
 	}
 
-	if len(splitArgs) == 0 || splitArgs[len(splitArgs)-1] != "#{pane_id}" {
-		t.Errorf("split-window argv = %v, want it to end at the -F value #{pane_id} with no trailing command argument (go test default: bare-shell header)", splitArgs)
-	}
-	if sendKeysCalls != 0 {
-		t.Errorf("send-keys calls = %d, want 0 under the go test default", sendKeysCalls)
-	}
-	if st.HeaderPaneID != newPaneID {
-		t.Errorf("HeaderPaneID = %q, want %q (recorded even though the pane is commandless)", st.HeaderPaneID, newPaneID)
+	if st.SelvagePaneID != newPaneID {
+		t.Errorf("SelvagePaneID = %q, want %q", st.SelvagePaneID, newPaneID)
 	}
 }
 
-// TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand reuses
-// TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit's wedged/retiled scripted
-// substrate (a one-row top pane the first split-window refuses, an even-vertical re-tile, then a
-// successful retry) but with launch enabled, and pins that the RETRIED split-window call — not just
-// a hypothetical first one — carries the launch command too. A retry path that dropped launchCmd
-// would boot a recovered header as an interactive shell, silently reopening this batch's noise class
-// on exactly the wedged-worktree recovery path R4-F4 exists for.
-func TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand(t *testing.T) {
+// TestEnsureSelvagePaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand reuses
+// TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit's wedged/retiled scripted
+// substrate (a one-row bottom pane the first split-window refuses, an even-vertical re-tile, then a
+// successful retry), and pins that the RETRIED split-window call — not just a hypothetical first
+// one — carries the launch command too. A retry path that dropped launchCmd would boot a recovered
+// Selvage as a commandless shell, silently reopening this batch's noise class on exactly the
+// wedged-worktree recovery path R4-F4 exists for.
+func TestEnsureSelvagePaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand(t *testing.T) {
 	e := newTestEngine(t)
-	enableHeaderLaunch(t, e)
 
-	const oneRowTopPaneID = "%1"
+	const oneRowBottomPaneID = "%1"
 	const tallPaneID = "%0"
-	const rebuiltHeaderPaneID = "%7"
+	const rebuiltSelvagePaneID = "%7"
 	// pane_id pane_dead pane_top pane_width pane_height pane_pid
-	wedged := oneRowTopPaneID + " 0 0 100 1 4321\n" + tallPaneID + " 0 2 100 48 4322\n"
-	retiled := oneRowTopPaneID + " 0 0 100 25 4321\n" + tallPaneID + " 0 26 100 24 4322\n"
+	wedged := tallPaneID + " 0 0 100 48 4321\n" + oneRowBottomPaneID + " 0 48 100 1 4322\n"
+	retiled := tallPaneID + " 0 0 100 24 4321\n" + oneRowBottomPaneID + " 0 25 100 25 4322\n"
 
 	reTiled := false
 	var retriedSplitArgs []string
@@ -645,49 +620,102 @@ func TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand(t *testi
 				return "", errors.New("exit status 1: no space for new pane")
 			}
 			retriedSplitArgs = append([]string{}, args...)
-			return rebuiltHeaderPaneID + "\n", nil
+			return rebuiltSelvagePaneID + "\n", nil
 		default:
 			return "", nil
 		}
 	}
 
 	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
-	if err := e.ensureHeaderPaneLocked(st); err != nil {
-		t.Fatalf("ensureHeaderPaneLocked() = %v; want nil", err)
+	if err := e.ensureSelvagePaneLocked(st); err != nil {
+		t.Fatalf("ensureSelvagePaneLocked() = %v; want nil", err)
 	}
-	if st.HeaderPaneID != rebuiltHeaderPaneID {
-		t.Fatalf("HeaderPaneID = %q; want %q (the pane the retried split created)", st.HeaderPaneID, rebuiltHeaderPaneID)
+	if st.SelvagePaneID != rebuiltSelvagePaneID {
+		t.Fatalf("SelvagePaneID = %q; want %q (the pane the retried split created)", st.SelvagePaneID, rebuiltSelvagePaneID)
 	}
 
 	if len(retriedSplitArgs) == 0 {
 		t.Fatalf("the retried split-window call was never recorded")
 	}
 	launchArg := retriedSplitArgs[len(retriedSplitArgs)-1]
-	for _, want := range []string{"reed", "--blocking"} {
-		if !strings.Contains(launchArg, want) {
-			t.Errorf("retried split-window trailing argument = %q, want it to contain %q (a retried header must never boot commandless)", launchArg, want)
-		}
+	if launchArg != e.cfg.Shell {
+		t.Errorf("retried split-window trailing argument = %q, want %q (a retried Selvage must never boot commandless)", launchArg, e.cfg.Shell)
 	}
 }
 
-// TestTopmostPaneID asserts the header split target is chosen by pane_top rather than by list-panes
-// order, which tmux does not guarantee is top-to-bottom.
-func TestTopmostPaneID(t *testing.T) {
+// TestBottommostPaneID asserts the Selvage split target is chosen by pane_top rather than by
+// list-panes order, which tmux does not guarantee is top-to-bottom.
+func TestBottommostPaneID(t *testing.T) {
 	tests := []struct {
 		name string
 		live []LivePane
 		want string
 	}{
 		{"sole pane", []LivePane{{ID: "%0", Top: 0}}, "%0"},
-		{"already first", []LivePane{{ID: "%1", Top: 0}, {ID: "%0", Top: 2}}, "%1"},
-		{"not first in list order", []LivePane{{ID: "%0", Top: 26}, {ID: "%1", Top: 0}}, "%1"},
-		{"three panes, middle listed first", []LivePane{{ID: "%2", Top: 10}, {ID: "%0", Top: 30}, {ID: "%1", Top: 0}}, "%1"},
+		{"already last", []LivePane{{ID: "%0", Top: 0}, {ID: "%1", Top: 2}}, "%1"},
+		{"not last in list order", []LivePane{{ID: "%1", Top: 26}, {ID: "%0", Top: 0}}, "%1"},
+		{"three panes, tallest-top listed first", []LivePane{{ID: "%2", Top: 30}, {ID: "%0", Top: 10}, {ID: "%1", Top: 0}}, "%2"},
 	}
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			if got := topmostPaneID(tt.live); got != tt.want {
-				t.Errorf("topmostPaneID(%v) = %q; want %q", tt.live, got, tt.want)
+			if got := bottommostPaneID(tt.live); got != tt.want {
+				t.Errorf("bottommostPaneID(%v) = %q; want %q", tt.live, got, tt.want)
 			}
 		})
 	}
 }
+
+// TestEnsureSelvagePaneLocked_SplitsBelowTheBottommostPaneWithNoBFlag pins the new split direction
+// end to end: given a scripted pane list whose largest pane_top is a known id, ensureSelvagePaneLocked
+// targets that id, and the split-window argv it issues carries no -b — tmux's default direction
+// (new pane below target) is exactly where Selvage must land now that render.Rules emits the band
+// cell last rather than first.
+func TestEnsureSelvagePaneLocked_SplitsBelowTheBottommostPaneWithNoBFlag(t *testing.T) {
+	e := newTestEngine(t)
+
+	const topPaneID = "%0"
+	const bottomPaneID = "%1"
+	const newPaneID = "%2"
+	listPanesOut := topPaneID + " 0 0 100 10 4321\n" + bottomPaneID + " 0 10 100 10 4322\n"
+
+	var splitArgs []string
+	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+		switch args[0] {
+		case "list-panes":
+			return listPanesOut, nil
+		case "split-window":
+			splitArgs = append([]string{}, args...)
+			return newPaneID + "\n", nil
+		default:
+			return "", nil
+		}
+	}
+
+	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
+	if err := e.ensureSelvagePaneLocked(st); err != nil {
+		t.Fatalf("ensureSelvagePaneLocked: %v", err)
+	}
+
+	for _, arg := range splitArgs {
+		if arg == "-b" {
+			t.Errorf("split-window argv %v carries -b; want no -b (Selvage now splits below, not above)", splitArgs)
+		}
+	}
+
+	targetFound := false
+	for i, arg := range splitArgs {
+		if arg != "-t" {
+			continue
+		}
+		if i+1 >= len(splitArgs) {
+			t.Fatalf("split-window argv %v has a trailing -t with no value", splitArgs)
+		}
+		targetFound = true
+		if splitArgs[i+1] != bottomPaneID {
+			t.Errorf("split-window -t value = %q, want %q (the bottommost pane)", splitArgs[i+1], bottomPaneID)
+		}
+	}
+	if !targetFound {
+		t.Fatalf("split-window argv %v has no -t flag", splitArgs)
+	}
+}
diff --git a/internal/reedengine/lock.go b/internal/reedengine/lock.go
index 29f1683f0..ba9ae2794 100644
--- a/internal/reedengine/lock.go
+++ b/internal/reedengine/lock.go
@@ -10,7 +10,6 @@ import (
 	"fmt"
 	"os"
 	"path/filepath"
-	"testing"
 
 	"github.com/Knatte18/loomyard/internal/lock"
 	"github.com/Knatte18/loomyard/internal/logger"
@@ -35,13 +34,6 @@ type Engine struct {
 	cfg  Config
 	geom Geometry
 	tmux TmuxCmd
-	// suppressHeaderLaunch decides whether ensureHeaderPaneLocked leaves the header pane as a bare
-	// shell instead of passing it a launch command. It is initialised from testing.Testing() because
-	// re-exec'ing os.Executable() from a test binary would run the whole suite recursively — but it
-	// lives as a field, not a hard-wired testing.Testing() call at the boot site, so an in-package
-	// test can flip it back on (see enableHeaderLaunch, lifecycle_test.go) and drive the real launch
-	// path against a fake tmux.
-	suppressHeaderLaunch bool
 }
 
 // New builds an Engine for the given Config and Geometry.
@@ -49,10 +41,9 @@ type Engine struct {
 // hubgeom.ReedGeometry is the hub-mode answer.
 func New(cfg Config, geom Geometry) *Engine {
 	return &Engine{
-		cfg:                  cfg,
-		geom:                 geom,
-		tmux:                 NewTmuxCmd(cfg.Tmux, geom.SocketKey),
-		suppressHeaderLaunch: testing.Testing(),
+		cfg:  cfg,
+		geom: geom,
+		tmux: NewTmuxCmd(cfg.Tmux, geom.SocketKey),
 	}
 }
 
diff --git a/internal/reedengine/mouse_boot_integration_test.go b/internal/reedengine/mouse_boot_integration_test.go
index 9b263b85a..879f01eb9 100644
--- a/internal/reedengine/mouse_boot_integration_test.go
+++ b/internal/reedengine/mouse_boot_integration_test.go
@@ -54,6 +54,7 @@ func newIntegrationEngine(t *testing.T, mouse string) *Engine {
 		LogsDir:      filepath.Join(hubDir, "logs"),
 		RepoName:     "test-repo",
 		HubPath:      hubDir,
+		WorktreeName: filepath.Base(worktreeDir),
 	}
 	e := New(cfg, geom)
 
diff --git a/internal/reedengine/overlay.go b/internal/reedengine/overlay.go
index ec5ac0561..db925ef9d 100644
--- a/internal/reedengine/overlay.go
+++ b/internal/reedengine/overlay.go
@@ -25,7 +25,7 @@ type TmuxCmd struct {
 	socket   string
 	// execHook, when non-nil, replaces the real subprocess exec for BOTH run
 	// and output — the single white-box seam a test can stub to drive a
-	// composed engine call site (e.g. ensureHeaderPaneLocked's header-rebuild
+	// composed engine call site (e.g. ensureSelvagePaneLocked's Selvage-rebuild
 	// split) against a scripted tmux response WITHOUT a live server. It is the
 	// only way to exercise the psmux-only silent-split failure shape (exit 0
 	// with an EXISTING pane id printed) that native tmux cannot produce — the
@@ -100,6 +100,48 @@ func exactSessionWindowTarget(session string) string {
 	return "=" + session + ":"
 }
 
+// ListSessions returns every session name live on the -L tmuxPath socket named socketKey,
+// or an error if the round trip itself failed.
+//
+// It is the one engine-less, exported function in this package: every other exported method
+// hangs off *Engine and is bound to one session, but the watchdog daemon (internal/reedcli) must
+// enumerate a hub socket's sessions BEFORE it has built any Engine for any of them — there is
+// nothing yet to bind a method call to. TmuxCmd.run and TmuxCmd.output stay unexported; this is
+// the one seam this package opens for that discovery, built on the identical
+// `tmux -L <socket> list-sessions -F '#{session_name}'` invocation five call sites in this
+// package already issue.
+//
+// Telling ListSessions the binary rather than having it load a config keeps the Told-Geometry
+// Invariant intact from this side too: every session on one hub socket shares one tmux binary, so
+// naming it explicitly is the only coherent answer for an enumeration that spans sessions.
+//
+// The three outcomes a caller can distinguish are: a non-empty slice (sessions live), an empty
+// slice with a nil error (exit 0, no sessions — a normal empty hub), and a non-nil error (the
+// round trip itself failed — no server, unreachable socket, or another tmux failure). The error
+// is returned unwrapped beyond output's own wrapTmuxError.
+func ListSessions(tmuxPath, socketKey string) ([]string, error) {
+	return listSessionsVia(NewTmuxCmd(tmuxPath, socketKey))
+}
+
+// listSessionsVia is ListSessions' implementation, taking an already-built TmuxCmd rather than raw
+// binary/socket strings — the split exists so a test can drive the parsing half through TmuxCmd's
+// execHook seam directly, without a live server.
+func listSessionsVia(cmd TmuxCmd) ([]string, error) {
+	out, err := cmd.output("list-sessions", "-F", "#{session_name}")
+	if err != nil {
+		return nil, err
+	}
+	var names []string
+	for _, line := range strings.Split(out, "\n") {
+		line = strings.TrimSpace(line)
+		if line == "" {
+			continue
+		}
+		names = append(names, line)
+	}
+	return names, nil
+}
+
 // hasSession reports whether the named session exists (by exact match, not prefix).
 func (p TmuxCmd) hasSession(name string) (bool, error) {
 	err := p.run("has-session", "-t", exactSessionTarget(name))
diff --git a/internal/reedengine/overlay_test.go b/internal/reedengine/overlay_test.go
new file mode 100644
index 000000000..6a5847015
--- /dev/null
+++ b/internal/reedengine/overlay_test.go
@@ -0,0 +1,77 @@
+// overlay_test.go pins ListSessions' parsing half against TmuxCmd's execHook seam, driving the
+// three outcomes the watchdog daemon's idle rule distinguishes: a multi-line listing, an exit-0
+// empty listing, and an error — with no live tmux server required.
+
+package reedengine
+
+import (
+	"errors"
+	"testing"
+)
+
+func TestListSessions(t *testing.T) {
+	tests := []struct {
+		name    string
+		out     string
+		hookErr error
+		want    []string
+		wantErr bool
+	}{
+		{
+			name: "multi-line listing",
+			out:  "alpha\nbeta\ngamma\n",
+			want: []string{"alpha", "beta", "gamma"},
+		},
+		{
+			name: "exit-0 empty listing",
+			out:  "",
+			want: nil,
+		},
+		{
+			name:    "error",
+			hookErr: errors.New("no server running on socket"),
+			wantErr: true,
+		},
+		{
+			name: "trailing whitespace and trailing newline",
+			out:  "alpha  \n  beta\n\n",
+			want: []string{"alpha", "beta"},
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			// listSessionsVia is ListSessions' parsing half, factored out so a test can drive it
+			// through TmuxCmd's execHook seam directly rather than a real server.
+			cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
+			cmd.execHook = func(capture bool, args ...string) (string, error) {
+				if !capture {
+					t.Fatalf("execHook called with capture=false, want true (list-sessions always captures)")
+				}
+				if len(args) < 1 || args[0] != "list-sessions" {
+					t.Fatalf("execHook args = %v, want first arg %q", args, "list-sessions")
+				}
+				return tt.out, tt.hookErr
+			}
+
+			got, err := listSessionsVia(cmd)
+			if tt.wantErr {
+				if err == nil {
+					t.Fatalf("listSessionsVia() error = nil, want non-nil")
+				}
+				return
+			}
+			if err != nil {
+				t.Fatalf("listSessionsVia() unexpected error: %v", err)
+			}
+			if len(got) != len(tt.want) {
+				t.Fatalf("listSessionsVia() = %v; want %v", got, tt.want)
+			}
+			for i := range got {
+				if got[i] != tt.want[i] {
+					t.Errorf("listSessionsVia()[%d] = %q; want %q", i, got[i], tt.want[i])
+				}
+			}
+		})
+	}
+}
diff --git a/internal/reedengine/reconcile.go b/internal/reedengine/reconcile.go
index b8f60537e..964e216a9 100644
--- a/internal/reedengine/reconcile.go
+++ b/internal/reedengine/reconcile.go
@@ -28,8 +28,8 @@ type reconcilePlan struct {
 // planReconcile decides which pane bindings to clear, which dead panes to kill, and which
 // untracked panes to reap.
 // Pure logic; unit-testable without a running server.
-// Keeps at least one pane alive (session-survival rule); spares header pane.
-func planReconcile(strands []Strand, live []LivePane, headerPaneID string) reconcilePlan {
+// Keeps at least one pane alive (session-survival rule); spares Selvage.
+func planReconcile(strands []Strand, live []LivePane, selvagePaneID string) reconcilePlan {
 	var plan reconcilePlan
 
 	liveByID := make(map[string]LivePane, len(live))
@@ -59,20 +59,20 @@ func planReconcile(strands []Strand, live []LivePane, headerPaneID string) recon
 		}
 	}
 
-	// The header pane is exempt from the dead-pane kill too, not only from
-	// the untracked reap below: nothing outside up/resume ever rebuilds the
-	// header, so killing a pane_dead=1 header here would leave the session
-	// headerless (breaking the always-on keepalive the header exists for)
-	// with a stale HeaderPaneID until the next up/resume — and, before the
-	// planLayout presence filter existed, that stale id was still emitted as
-	// a layout cell, which a real tmux ACCEPTS (exit 0) and assigns
-	// positionally, scrambling every strand's height (observed live,
-	// tmux 3.6). A kept header corpse instead stays enumerable, keeps the
-	// cell/pane count consistent, and is healed — killed and re-split — by
-	// ensureHeaderPaneLocked on the next up/resume.
+	// Selvage is exempt from the dead-pane kill too, not only from the
+	// untracked reap below: nothing outside up/resume ever rebuilds it, so
+	// killing a pane_dead=1 Selvage here would leave the session without its
+	// always-on operator console with a stale SelvagePaneID until the next
+	// up/resume — and, before the planLayout presence filter existed, that
+	// stale id was still emitted as a layout cell, which a real tmux
+	// ACCEPTS (exit 0) and assigns positionally, scrambling every strand's
+	// height (observed live, tmux 3.6). A kept Selvage corpse instead stays
+	// enumerable, keeps the cell/pane count consistent, and is healed —
+	// killed and re-split — by ensureSelvagePaneLocked on the next
+	// up/resume.
 	killSet := make(map[string]bool, len(live))
 	for _, p := range live {
-		if p.Dead && p.ID != keptDeadPaneID && p.ID != headerPaneID {
+		if p.Dead && p.ID != keptDeadPaneID && p.ID != selvagePaneID {
 			killSet[p.ID] = true
 			plan.deadPanesToKill = append(plan.deadPanesToKill, p.ID)
 		}
@@ -80,14 +80,14 @@ func planReconcile(strands []Strand, live []LivePane, headerPaneID string) recon
 
 	// Deterministic untracked-pane reaping (see the doc comment): kill every
 	// live pane no strand owns, while EITHER some strand is bound to a
-	// present pane OR the header itself is alive — killing an alive pane at
+	// present pane OR Selvage itself is alive — killing an alive pane at
 	// worst corpses it under remain-on-exit, so the surviving bound pane or
-	// header always keeps the session alive. The header disjunct exists
+	// Selvage always keeps the session alive. The Selvage disjunct exists
 	// because this reap fires from AddStrand/UpdateStrand once the
 	// reap-before-allocate chokepoint lands, and neither of those paths ever
-	// calls ensureHeaderPaneLocked — so a dead-but-present header must not
+	// calls ensureSelvagePaneLocked — so a dead-but-present Selvage must not
 	// be allowed to authorize reaping the session's only alive pane; only an
-	// ALIVE header may.
+	// ALIVE Selvage may.
 	boundPaneIDs := make(map[string]bool, len(strands))
 	for _, s := range strands {
 		if s.PaneID != "" {
@@ -102,14 +102,14 @@ func planReconcile(strands []Strand, live []LivePane, headerPaneID string) recon
 		}
 	}
 
-	// headerAlive is a third, separate local, never folded into
-	// boundPaneIDs/anyBoundPresent/exemptPaneIDs: the header stays exempt
-	// from being killed by mere presence (a header corpse is still never
-	// killed), while only an alive header authorizes killing anything else.
-	headerAlive := false
-	if headerPaneID != "" {
-		if p, present := liveByID[headerPaneID]; present && !p.Dead {
-			headerAlive = true
+	// selvageAlive is a third, separate local, never folded into
+	// boundPaneIDs/anyBoundPresent/exemptPaneIDs: Selvage stays exempt from
+	// being killed by mere presence (a Selvage corpse is still never
+	// killed), while only an alive Selvage authorizes killing anything else.
+	selvageAlive := false
+	if selvagePaneID != "" {
+		if p, present := liveByID[selvagePaneID]; present && !p.Dead {
+			selvageAlive = true
 		}
 	}
 
@@ -120,11 +120,11 @@ func planReconcile(strands []Strand, live []LivePane, headerPaneID string) recon
 	for id := range boundPaneIDs {
 		exemptPaneIDs[id] = true
 	}
-	if headerPaneID != "" {
-		exemptPaneIDs[headerPaneID] = true
+	if selvagePaneID != "" {
+		exemptPaneIDs[selvagePaneID] = true
 	}
 
-	if anyBoundPresent || headerAlive {
+	if anyBoundPresent || selvageAlive {
 		for _, p := range live {
 			if !exemptPaneIDs[p.ID] && !killSet[p.ID] && p.ID != keptDeadPaneID {
 				killSet[p.ID] = true
@@ -158,11 +158,11 @@ func clearAllPaneBindings(st *ReedState) {
 }
 
 // clearConflictingPaneBindings clears every strand PaneID that names a pane the strand cannot
-// possibly own — the header pane, or a pane an earlier strand in the table already claims — and
+// possibly own — Selvage's pane, or a pane an earlier strand in the table already claims — and
 // returns the GUIDs it cleared, in table order.
 //
 // A pane has exactly one owner. reed's own construction paths already guarantee that
-// (planPaneTarget never yields the header as a split target while any non-header pane exists, and
+// (planPaneTarget never yields Selvage as a split target while any non-Selvage pane exists, and
 // its result is always validated as genuinely new by validateSplitCreatedNewPane), so a table
 // violating it is a CORRUPT table, not one reed produced: a stale
 // reed.json restored over a newer session, a hand-edited file, a partially restored backup. Every
@@ -171,11 +171,11 @@ func clearAllPaneBindings(st *ReedState) {
 //
 // The damage is worth naming, because it is neither theoretical nor loud (R5 review finding R5-F3,
 // both shapes reproduced live on tmux 3.6):
-//   - A strand sharing the HEADER's pane id makes planLayout place that pane twice — once as
-//     bandHeader's fixed top cell, once inside the stack body — and tmux answers a layout string
+//   - A strand sharing SELVAGE's pane id makes planLayout place that pane twice — once as
+//     bandSelvage's fixed bottom cell, once inside the stack body — and tmux answers a layout string
 //     whose cell count exceeds the panes it can name by DESTROYING the panes it has no cell for.
 //     Observed: a single `lyx reed up` reduced a two-pane session to one, reported ok:true, and then
-//     reported the strand live:true against the header pane running `lyx reed header --blocking`.
+//     reported the strand live:true against the Selvage pane.
 //   - Two strands sharing one pane id leave the second strand's REAL pane bound to nobody, so
 //     planReconcile's deterministic untracked reap kills it. Observed: `up` reported ok:true and
 //     strands:2 while destroying the second strand's pane and its running process, after which
@@ -187,8 +187,8 @@ func clearAllPaneBindings(st *ReedState) {
 // would wedge the worktree on exactly the corruption this exists to survive.
 func clearConflictingPaneBindings(st *ReedState) []string {
 	claimed := make(map[string]bool, len(st.Strands)+1)
-	if st.HeaderPaneID != "" {
-		claimed[st.HeaderPaneID] = true
+	if st.SelvagePaneID != "" {
+		claimed[st.SelvagePaneID] = true
 	}
 
 	var clearedGUIDs []string
@@ -210,7 +210,7 @@ func clearConflictingPaneBindings(st *ReedState) []string {
 // reconcileLocked reconciles the persisted table against live panes.
 // Kills panes per planReconcile's schedule; clears bindings for gone panes.
 func (e *Engine) reconcileLocked(st *ReedState, live []LivePane) (killed []string, err error) {
-	plan := planReconcile(st.Strands, live, st.HeaderPaneID)
+	plan := planReconcile(st.Strands, live, st.SelvagePaneID)
 
 	// Accumulate the ids actually destroyed, separately for each kill reason,
 	// as the loops below progress -- never plan.deadPanesToKill /
@@ -225,7 +225,7 @@ func (e *Engine) reconcileLocked(st *ReedState, live []LivePane) (killed []strin
 	// This is Info, not Debug: per CONSTRAINTS.md's Live-Substrate Spawn
 	// Observability lifecycle-vs-probe split, a real pane teardown is a
 	// lifecycle event, not a probe. And it needs a trace at all because the
-	// headerAlive disjunct above makes this reap fire on the zero-strand
+	// selvageAlive disjunct above makes this reap fire on the zero-strand
 	// precondition (every AddStrand/UpdateStrand once the reap-before-
 	// allocate chokepoint lands), taking it from near-dormant to routine --
 	// and it destroys panes an operator may have created themselves.
diff --git a/internal/reedengine/reconcile_test.go b/internal/reedengine/reconcile_test.go
index 5cf6f5dd7..5c7da3fdc 100644
--- a/internal/reedengine/reconcile_test.go
+++ b/internal/reedengine/reconcile_test.go
@@ -1,5 +1,5 @@
 // reconcile_test.go table-tests planReconcile's pure decision logic against saved strand tables and
-// fake list-panes results (including pane_dead=1 rows and the header-pane exemption),
+// fake list-panes results (including pane_dead=1 rows and the Selvage exemption),
 // exercises reconcileLocked's real-record mutation for the no-dead-panes path, which never
 // touches tmux and so stays hermetic, and pins reconcileLocked's reap log line via
 // captureLogOutput (logcapture_test.go).
@@ -29,7 +29,7 @@ func TestPlanReconcile(t *testing.T) {
 		name                     string
 		strands                  []Strand
 		live                     []LivePane
-		headerPaneID             string
+		selvagePaneID            string
 		wantCleared              []string
 		wantDeadPanesToKill      []string
 		wantUntrackedPanesToKill []string
@@ -102,136 +102,137 @@ func TestPlanReconcile(t *testing.T) {
 			wantUntrackedPanesToKill: []string{"%7"},
 		},
 		{
-			// With no strand bound to any present pane and no alive header
-			// (headerPaneID unset, so headerAlive is false), reed has
+			// With no strand bound to any present pane and no alive Selvage
+			// (selvagePaneID unset, so selvageAlive is false), reed has
 			// nothing to lay out and leaves foreign panes strictly alone
 			// (the apply is skipped too — anyPlacedStrand). This is the
-			// absent-header shape of "nothing authorizes the reap"; see
-			// HeaderAloneNeverMakesAnyBoundPresentTrue and the two
-			// headerAlive-false-with-a-live-header cases below for the other
+			// absent-Selvage shape of "nothing authorizes the reap"; see
+			// SelvageAloneNeverMakesAnyBoundPresentTrue and the two
+			// selvageAlive-false-with-a-live-Selvage cases below for the other
 			// shapes.
-			name:        "UntrackedPanesUntouchedWhenNoHeaderAndNothingBound",
+			name:        "UntrackedPanesUntouchedWhenNoSelvageAndNothingBound",
 			strands:     []Strand{{GUID: "cleared", PaneID: ""}},
 			live:        []LivePane{{ID: "%7", Dead: false}, {ID: "%8", Dead: false}},
 			wantCleared: nil,
 		},
 		{
-			// The header pane must never be reaped as an "untracked" pane
-			// even while a strand is bound and anyBoundPresent is true —
-			// exemptPaneIDs (boundPaneIDs plus the header) is what protects
+			// Selvage must never be reaped as an "untracked" pane even
+			// while a strand is bound and anyBoundPresent is true —
+			// exemptPaneIDs (boundPaneIDs plus Selvage) is what protects
 			// it, distinct from boundPaneIDs itself (which must stay
 			// strand-only so anyBoundPresent is never inflated by a merely
-			// live header). This shape also covers "alive header alongside a
-			// bound strand" for the headerAlive disjunct: the reap already
-			// fires via anyBoundPresent here, so headerAlive contributes
+			// live Selvage). This shape also covers "alive Selvage alongside a
+			// bound strand" for the selvageAlive disjunct: the reap already
+			// fires via anyBoundPresent here, so selvageAlive contributes
 			// nothing new to this case, and no separate case is needed for
 			// it.
-			name:         "HeaderPaneNeverReapedAsUntrackedWhileStrandBound",
-			strands:      []Strand{{GUID: "g1", PaneID: "%1"}},
-			live:         []LivePane{{ID: "%1", Dead: false}, {ID: "%header", Dead: false}, {ID: "%7", Dead: false}},
-			headerPaneID: "%header",
-			wantCleared:  nil,
-			// %7 is a genuine foreign pane and is still reaped; %header is
+			name:          "SelvageNeverReapedAsUntrackedWhileStrandBound",
+			strands:       []Strand{{GUID: "g1", PaneID: "%1"}},
+			live:          []LivePane{{ID: "%1", Dead: false}, {ID: "%selvage", Dead: false}, {ID: "%7", Dead: false}},
+			selvagePaneID: "%selvage",
+			wantCleared:   nil,
+			// %7 is a genuine foreign pane and is still reaped; %selvage is
 			// exempt and must not appear here.
 			wantUntrackedPanesToKill: []string{"%7"},
 		},
 		{
-			// The header is alive and no strand is bound to any present
+			// Selvage is alive and no strand is bound to any present
 			// pane: anyBoundPresent stays false (derived from boundPaneIDs
-			// alone, never the header — folding the header in would wrongly
-			// flip it), but headerAlive is true, so the untracked reap fires
-			// from that disjunct alone and %7 is killed while the header
+			// alone, never Selvage — folding Selvage in would wrongly
+			// flip it), but selvageAlive is true, so the untracked reap fires
+			// from that disjunct alone and %7 is killed while Selvage
 			// itself stays exempt.
-			name:                     "HeaderAloneNeverMakesAnyBoundPresentTrue",
+			name:                     "SelvageAloneNeverMakesAnyBoundPresentTrue",
 			strands:                  []Strand{{GUID: "cleared", PaneID: ""}},
-			live:                     []LivePane{{ID: "%header", Dead: false}, {ID: "%7", Dead: false}},
-			headerPaneID:             "%header",
+			live:                     []LivePane{{ID: "%selvage", Dead: false}, {ID: "%7", Dead: false}},
+			selvagePaneID:            "%selvage",
 			wantCleared:              nil,
 			wantUntrackedPanesToKill: []string{"%7"},
 		},
 		{
-			// An alive header with zero strands and one untracked alive
+			// An alive Selvage with zero strands and one untracked alive
 			// pane: that pane is killed as an untracked kill (not a dead-pane
-			// kill) and the header itself is spared.
-			name:                     "AliveHeaderNoStrandsReapsOneUntrackedPane",
+			// kill) and Selvage itself is spared.
+			name:                     "AliveSelvageNoStrandsReapsOneUntrackedPane",
 			strands:                  nil,
-			live:                     []LivePane{{ID: "%header", Dead: false}, {ID: "%orphan", Dead: false}},
-			headerPaneID:             "%header",
+			live:                     []LivePane{{ID: "%selvage", Dead: false}, {ID: "%orphan", Dead: false}},
+			selvagePaneID:            "%selvage",
 			wantCleared:              nil,
 			wantUntrackedPanesToKill: []string{"%orphan"},
 		},
 		{
-			// An alive header with zero strands and several untracked panes
-			// (an old header pane plus an orphaned strand pane, M22's
-			// shape): all of them are killed and the current header is
+			// An alive Selvage with zero strands and several untracked panes
+			// (an old Selvage pane plus an orphaned strand pane, M22's
+			// shape): all of them are killed and the current Selvage is
 			// spared.
-			name:    "AliveHeaderNoStrandsReapsSeveralUntrackedPanes",
+			name:    "AliveSelvageNoStrandsReapsSeveralUntrackedPanes",
 			strands: nil,
 			live: []LivePane{
-				{ID: "%header", Dead: false},
-				{ID: "%oldheader", Dead: false},
+				{ID: "%selvage", Dead: false},
+				{ID: "%oldselvage", Dead: false},
 				{ID: "%orphanstrand", Dead: false},
 			},
-			headerPaneID:             "%header",
+			selvagePaneID:            "%selvage",
 			wantCleared:              nil,
-			wantUntrackedPanesToKill: []string{"%oldheader", "%orphanstrand"},
+			wantUntrackedPanesToKill: []string{"%oldselvage", "%orphanstrand"},
 		},
 		{
-			// A header present but Dead: true, with no strand bound and one
-			// alive untracked pane: headerAlive is false (the header entry
+			// Selvage present but Dead: true, with no strand bound and one
+			// alive untracked pane: selvageAlive is false (the Selvage entry
 			// is present but dead), so nothing is reaped.
-			name:         "PresentButDeadHeaderDoesNotAuthorizeReap",
-			strands:      nil,
-			live:         []LivePane{{ID: "%header", Dead: true}, {ID: "%orphan", Dead: false}},
-			headerPaneID: "%header",
-			wantCleared:  nil,
+			name:          "PresentButDeadSelvageDoesNotAuthorizeReap",
+			strands:       nil,
+			live:          []LivePane{{ID: "%selvage", Dead: true}, {ID: "%orphan", Dead: false}},
+			selvagePaneID: "%selvage",
+			wantCleared:   nil,
 		},
 		{
-			// A non-empty headerPaneID naming no entry in live at all, with
-			// no strand bound and one alive untracked pane: headerAlive's
+			// A non-empty selvagePaneID naming no entry in live at all, with
+			// no strand bound and one alive untracked pane: selvageAlive's
 			// third way of being false, distinct from the empty-id and
 			// present-but-dead cases above. Reachable on the add path once
-			// an operator kills the header pane outright, since no verb but
+			// an operator kills Selvage outright, since no verb but
 			// up/resume rebuilds it.
-			name:         "HeaderIDNamingNoLivePaneDoesNotAuthorizeReap",
-			strands:      nil,
-			live:         []LivePane{{ID: "%orphan", Dead: false}},
-			headerPaneID: "%header",
-			wantCleared:  nil,
+			name:          "SelvageIDNamingNoLivePaneDoesNotAuthorizeReap",
+			strands:       nil,
+			live:          []LivePane{{ID: "%orphan", Dead: false}},
+			selvagePaneID: "%selvage",
+			wantCleared:   nil,
 		},
 		{
-			// A dead header alongside a strand bound to a present pane: the
-			// reap fires anyway via anyBoundPresent, and the header corpse
+			// A dead Selvage alongside a strand bound to a present pane: the
+			// reap fires anyway via anyBoundPresent, and the Selvage corpse
 			// is still spared.
-			name:                     "DeadHeaderAlongsideBoundStrandStillReapsViaAnyBoundPresent",
+			name:                     "DeadSelvageAlongsideBoundStrandStillReapsViaAnyBoundPresent",
 			strands:                  []Strand{{GUID: "g1", PaneID: "%1"}},
-			live:                     []LivePane{{ID: "%header", Dead: true}, {ID: "%1", Dead: false}, {ID: "%7", Dead: false}},
-			headerPaneID:             "%header",
+			live:                     []LivePane{{ID: "%selvage", Dead: true}, {ID: "%1", Dead: false}, {ID: "%7", Dead: false}},
+			selvagePaneID:            "%selvage",
 			wantCleared:              nil,
 			wantUntrackedPanesToKill: []string{"%7"},
 		},
 		{
-			// A DEAD header pane must not be scheduled for killing either —
+			// A DEAD Selvage pane must not be scheduled for killing either —
 			// the dead-pane kill loop, not only the untracked reap, spares
-			// it. Nothing outside up/resume rebuilds a header, so killing
+			// it. Nothing outside up/resume rebuilds Selvage, so killing
 			// the corpse here would leave every intermediate add/remove
-			// headerless with a stale HeaderPaneID (the fable-header-r1
-			// layout-scramble-then-wedged-up defect). The kept corpse stays
-			// enumerable; ensureHeaderPaneLocked heals it at the next boot.
-			name:         "DeadHeaderPaneKeptNotKilled",
-			strands:      []Strand{{GUID: "g1", PaneID: "%1"}},
-			live:         []LivePane{{ID: "%header", Dead: true}, {ID: "%1", Dead: false}},
-			headerPaneID: "%header",
-			wantCleared:  nil,
+			// without a Selvage, holding a stale SelvagePaneID (the
+			// fable-header-r1 layout-scramble-then-wedged-up defect). The
+			// kept corpse stays enumerable; ensureSelvagePaneLocked heals it
+			// at the next boot.
+			name:          "DeadSelvagePaneKeptNotKilled",
+			strands:       []Strand{{GUID: "g1", PaneID: "%1"}},
+			live:          []LivePane{{ID: "%selvage", Dead: true}, {ID: "%1", Dead: false}},
+			selvagePaneID: "%selvage",
+			wantCleared:   nil,
 		},
 		{
-			// A dead header alongside a dead strand pane: the strand corpse
+			// A dead Selvage alongside a dead strand pane: the strand corpse
 			// is still killable business-as-usual (an alive pane remains),
-			// while the header corpse stays exempt.
-			name:                "DeadHeaderExemptWhileDeadStrandPaneStillKilled",
+			// while the Selvage corpse stays exempt.
+			name:                "DeadSelvageExemptWhileDeadStrandPaneStillKilled",
 			strands:             []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}},
-			live:                []LivePane{{ID: "%header", Dead: true}, {ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
-			headerPaneID:        "%header",
+			live:                []LivePane{{ID: "%selvage", Dead: true}, {ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
+			selvagePaneID:       "%selvage",
 			wantCleared:         []string{"g1"},
 			wantDeadPanesToKill: []string{"%1"},
 		},
@@ -239,7 +240,7 @@ func TestPlanReconcile(t *testing.T) {
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			gotPlan := planReconcile(tt.strands, tt.live, tt.headerPaneID)
+			gotPlan := planReconcile(tt.strands, tt.live, tt.selvagePaneID)
 			if !equalStringSlices(gotPlan.clearedGUIDs, tt.wantCleared) {
 				t.Errorf("planReconcile() clearedGUIDs = %v, want %v", gotPlan.clearedGUIDs, tt.wantCleared)
 			}
@@ -306,8 +307,8 @@ func TestReconcileLocked_LogsTheUntrackedPanesItReaps(t *testing.T) {
 		}
 		buf := captureLogOutput(t)
 
-		st := &ReedState{HeaderPaneID: "%header"}
-		live := []LivePane{{ID: "%header", Dead: false}, {ID: "%orphan1", Dead: false}, {ID: "%orphan2", Dead: false}}
+		st := &ReedState{SelvagePaneID: "%selvage"}
+		live := []LivePane{{ID: "%selvage", Dead: false}, {ID: "%orphan1", Dead: false}, {ID: "%orphan2", Dead: false}}
 
 		killed, err := e.reconcileLocked(st, live)
 		if err != nil {
@@ -355,8 +356,8 @@ func TestReconcileLocked_LogsTheUntrackedPanesItReaps(t *testing.T) {
 		}
 		buf := captureLogOutput(t)
 
-		st := &ReedState{HeaderPaneID: "%header"}
-		live := []LivePane{{ID: "%header", Dead: false}, {ID: "%orphan1", Dead: false}, {ID: "%orphan2", Dead: false}}
+		st := &ReedState{SelvagePaneID: "%selvage"}
+		live := []LivePane{{ID: "%selvage", Dead: false}, {ID: "%orphan1", Dead: false}, {ID: "%orphan2", Dead: false}}
 
 		killed, err := e.reconcileLocked(st, live)
 		if err == nil {
@@ -380,7 +381,7 @@ func TestReconcileLocked_LogsTheUntrackedPanesItReaps(t *testing.T) {
 }
 
 // TestClearConflictingPaneBindings is the regression guard for the R5 review's R5-F3: a corrupt
-// reed.json whose strand pane bindings contradict each other (a strand naming the header's pane, or
+// reed.json whose strand pane bindings contradict each other (a strand naming Selvage's pane, or
 // two strands naming one pane) made `up` destroy unrelated live panes and their processes while
 // reporting ok:true, then report the strand live against a pane it does not own.
 // The repair is first-writer-wins, so the table order of the cleared GUIDs is part of the contract,
@@ -395,17 +396,17 @@ func TestClearConflictingPaneBindings(t *testing.T) {
 		{
 			name: "a healthy table is untouched",
 			state: ReedState{
-				HeaderPaneID: "%1",
-				Strands:      []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
+				SelvagePaneID: "%1",
+				Strands:       []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
 			},
 			wantCleared:  nil,
 			wantPaneByID: map[string]string{"a": "%2", "b": "%3"},
 		},
 		{
-			name: "a strand naming the header pane is cleared",
+			name: "a strand naming Selvage's pane is cleared",
 			state: ReedState{
-				HeaderPaneID: "%1",
-				Strands:      []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b", PaneID: "%2"}},
+				SelvagePaneID: "%1",
+				Strands:       []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b", PaneID: "%2"}},
 			},
 			wantCleared:  []string{"a"},
 			wantPaneByID: map[string]string{"a": "", "b": "%2"},
@@ -413,8 +414,8 @@ func TestClearConflictingPaneBindings(t *testing.T) {
 		{
 			name: "the later of two strands sharing one pane is cleared",
 			state: ReedState{
-				HeaderPaneID: "%1",
-				Strands:      []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%2"}},
+				SelvagePaneID: "%1",
+				Strands:       []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%2"}},
 			},
 			wantCleared:  []string{"b"},
 			wantPaneByID: map[string]string{"a": "%2", "b": ""},
@@ -422,17 +423,17 @@ func TestClearConflictingPaneBindings(t *testing.T) {
 		{
 			name: "unbound strands are never reported as conflicting with each other",
 			state: ReedState{
-				HeaderPaneID: "%1",
-				Strands:      []Strand{{GUID: "a", PaneID: ""}, {GUID: "b", PaneID: ""}},
+				SelvagePaneID: "%1",
+				Strands:       []Strand{{GUID: "a", PaneID: ""}, {GUID: "b", PaneID: ""}},
 			},
 			wantCleared:  nil,
 			wantPaneByID: map[string]string{"a": "", "b": ""},
 		},
 		{
-			name: "an absent header claims nothing, so an empty HeaderPaneID clears no strand",
+			name: "an absent Selvage claims nothing, so an empty SelvagePaneID clears no strand",
 			state: ReedState{
-				HeaderPaneID: "",
-				Strands:      []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
+				SelvagePaneID: "",
+				Strands:       []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
 			},
 			wantCleared:  nil,
 			wantPaneByID: map[string]string{"a": "%2", "b": "%3"},
@@ -440,8 +441,8 @@ func TestClearConflictingPaneBindings(t *testing.T) {
 		{
 			name: "three strands on one pane clear all but the first",
 			state: ReedState{
-				HeaderPaneID: "%9",
-				Strands:      []Strand{{GUID: "a", PaneID: "%4"}, {GUID: "b", PaneID: "%4"}, {GUID: "c", PaneID: "%4"}},
+				SelvagePaneID: "%9",
+				Strands:       []Strand{{GUID: "a", PaneID: "%4"}, {GUID: "b", PaneID: "%4"}, {GUID: "c", PaneID: "%4"}},
 			},
 			wantCleared:  []string{"b", "c"},
 			wantPaneByID: map[string]string{"a": "%4", "b": "", "c": ""},
@@ -464,8 +465,8 @@ func TestClearConflictingPaneBindings(t *testing.T) {
 					t.Errorf("strand %s PaneID = %q; want %q", guid, paneID, want)
 				}
 			}
-			if st.HeaderPaneID != tt.state.HeaderPaneID {
-				t.Errorf("HeaderPaneID = %q; want %q (the header binding is never the one cleared)", st.HeaderPaneID, tt.state.HeaderPaneID)
+			if st.SelvagePaneID != tt.state.SelvagePaneID {
+				t.Errorf("SelvagePaneID = %q; want %q (the Selvage binding is never the one cleared)", st.SelvagePaneID, tt.state.SelvagePaneID)
 			}
 		})
 	}
diff --git a/internal/reedengine/render/height.go b/internal/reedengine/render/height.go
index 3b3c1393a..6ef96c3d5 100644
--- a/internal/reedengine/render/height.go
+++ b/internal/reedengine/render/height.go
@@ -7,12 +7,12 @@
 
 package render
 
-// clampHeaderHeight returns headerRows clamped to preserve the strand-stack
-// region's minStackRows floor, which the header yields first when the window
+// clampBandHeight returns bandRows clamped to preserve the strand-stack
+// region's minStackRows floor, which the band yields first when the window
 // cannot fit both.
-func clampHeaderHeight(headerRows, windowRows, minStackRows int) int {
-	if headerRows < 0 {
-		headerRows = 0
+func clampBandHeight(bandRows, windowRows, minStackRows int) int {
+	if bandRows < 0 {
+		bandRows = 0
 	}
 	if windowRows <= 0 {
 		return 0
@@ -21,25 +21,25 @@ func clampHeaderHeight(headerRows, windowRows, minStackRows int) int {
 	if floor < 1 {
 		floor = 1
 	}
-	maxHeader := windowRows - floor
-	if maxHeader < 1 {
-		// Never fully starve the header once it exists: the stack's own
+	maxBand := windowRows - floor
+	if maxBand < 1 {
+		// Never fully starve the band once it exists: the stack's own
 		// floor is the lesser of two structural violations when the window
 		// cannot fit both, since a starved stack strand still renders
 		// (clampToFit floors every strand at 1 row already) while a
-		// zero-height header cell is mishandled by the real multiplexer.
-		maxHeader = 1
+		// zero-height band cell is mishandled by the real multiplexer.
+		maxBand = 1
 	}
-	if maxHeader > windowRows {
-		maxHeader = windowRows
+	if maxBand > windowRows {
+		maxBand = windowRows
 	}
-	if headerRows < 1 {
-		headerRows = 1
+	if bandRows < 1 {
+		bandRows = 1
 	}
-	if headerRows > maxHeader {
-		return maxHeader
+	if bandRows > maxBand {
+		return maxBand
 	}
-	return headerRows
+	return bandRows
 }
 
 // stackHeights computes a height for every strand in stack within box.
diff --git a/internal/reedengine/render/height_test.go b/internal/reedengine/render/height_test.go
index 03525ba3b..b0baf160c 100644
--- a/internal/reedengine/render/height_test.go
+++ b/internal/reedengine/render/height_test.go
@@ -1,6 +1,6 @@
 // height_test.go exercises the derived height policy in height.go: the heights-fill-the-box
 // invariant, the collapsed-strip height, the active pane's remainder rule, the too-short-window
-// clamp order, and the header-vs-window height clamp (clampHeaderHeight).
+// clamp order, and the band-vs-window height clamp (clampBandHeight).
 // It also exercises layout.go's buildStackBody/wrapLayout and focus.go's isAncestor, since cards 5
 // and 7 ship no standalone test file.
 
@@ -165,13 +165,13 @@ func TestStackHeightsExtremelyShortWindowNeverNonPositive(t *testing.T) {
 	}
 }
 
-// TestClampHeaderHeight covers the window-split clamp: the header yields rows first so the
+// TestClampBandHeight covers the window-split clamp: the band yields rows first so the
 // strand-stack region never shrinks below MinFullRows (floored at 1) total rows, distinct from
 // clampToFit's job of distributing rows AMONG strands inside an already-shrunk box.
-func TestClampHeaderHeight(t *testing.T) {
+func TestClampBandHeight(t *testing.T) {
 	tests := []struct {
 		name         string
-		headerRows   int
+		bandRows     int
 		windowRows   int
 		minStackRows int
 		want         int
@@ -181,35 +181,35 @@ func TestClampHeaderHeight(t *testing.T) {
 		{
 			// An oversized configured height_rows must yield rows so the
 			// stack region keeps its MinFullRows floor, even though that
-			// means the header itself ends up shorter than configured.
-			name: "Oversized_ClampedToPreserveFloor", headerRows: 25, windowRows: 21, minStackRows: 3, want: 18,
+			// means the band itself ends up shorter than configured.
+			name: "Oversized_ClampedToPreserveFloor", bandRows: 25, windowRows: 21, minStackRows: 3, want: 18,
 		},
 		{
-			// The window cannot fit both a header and the floor at all: the
-			// header still keeps its 1-row minimum (real tmux/psmux does not
+			// The window cannot fit both a band and the floor at all: the
+			// band still keeps its 1-row minimum (real tmux/psmux does not
 			// cleanly support a zero-height select-layout cell — see
 			// height.go's doc comment) rather than going to zero, even
 			// though that means the stack floor itself is violated instead.
-			name: "WindowTooShortForBoth_HeaderFlooredAtOne", headerRows: 5, windowRows: 2, minStackRows: 3, want: 1,
+			name: "WindowTooShortForBoth_BandFlooredAtOne", bandRows: 5, windowRows: 2, minStackRows: 3, want: 1,
 		},
 		{
-			// headerRows <= 0 (including the negative-treated-as-zero case)
-			// still floors to 1 once the window has any rows to give — a
-			// header pane exists whenever this function is called, so it
+			// bandRows <= 0 (including the negative-treated-as-zero case)
+			// still floors to 1 once the window has any rows to give — the
+			// Selvage band exists whenever this function is called, so it
 			// can never legitimately request/receive a zero-height cell.
-			name: "NegativeHeaderRows_FlooredAtOne", headerRows: -4, windowRows: 21, minStackRows: 3, want: 1,
+			name: "NegativeBandRows_FlooredAtOne", bandRows: -4, windowRows: 21, minStackRows: 3, want: 1,
 		},
 		{"NonPositiveMinStackRowsFlooredAtOne", 25, 21, 0, 20},
 		{
 			// windowRows itself has nothing to give: the result is 0, not a
 			// floored 1, since there is no row available at all.
-			name: "ZeroWindowRows_NothingToGive", headerRows: 5, windowRows: 0, minStackRows: 3, want: 0,
+			name: "ZeroWindowRows_NothingToGive", bandRows: 5, windowRows: 0, minStackRows: 3, want: 0,
 		},
 	}
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			if got := clampHeaderHeight(tt.headerRows, tt.windowRows, tt.minStackRows); got != tt.want {
-				t.Errorf("clampHeaderHeight(%d, %d, %d) = %d, want %d", tt.headerRows, tt.windowRows, tt.minStackRows, got, tt.want)
+			if got := clampBandHeight(tt.bandRows, tt.windowRows, tt.minStackRows); got != tt.want {
+				t.Errorf("clampBandHeight(%d, %d, %d) = %d, want %d", tt.bandRows, tt.windowRows, tt.minStackRows, got, tt.want)
 			}
 			// Invariant every case must hold: the stack region resulting
 			// from this clamp never shrinks below the floored MinFullRows.
@@ -217,9 +217,9 @@ func TestClampHeaderHeight(t *testing.T) {
 			if floor < 1 {
 				floor = 1
 			}
-			got := clampHeaderHeight(tt.headerRows, tt.windowRows, tt.minStackRows)
+			got := clampBandHeight(tt.bandRows, tt.windowRows, tt.minStackRows)
 			if stackRows := tt.windowRows - got; stackRows < floor && tt.windowRows >= floor {
-				t.Errorf("clampHeaderHeight(%d, %d, %d) left only %d stack rows, want >= floor %d", tt.headerRows, tt.windowRows, tt.minStackRows, stackRows, floor)
+				t.Errorf("clampBandHeight(%d, %d, %d) left only %d stack rows, want >= floor %d", tt.bandRows, tt.windowRows, tt.minStackRows, stackRows, floor)
 			}
 		})
 	}
diff --git a/internal/reedengine/render/layout.go b/internal/reedengine/render/layout.go
index 7ccb2a2a2..397bc8997 100644
--- a/internal/reedengine/render/layout.go
+++ b/internal/reedengine/render/layout.go
@@ -55,22 +55,22 @@ func wrapLayout(body string) string {
 	return layoutChecksum(body) + "," + body
 }
 
-// bandHeader prepends a fixed-height header cell to stackBody's pane group,
-// producing the full window_layout body when a header pane is present.
+// bandSelvage appends a fixed-height Selvage cell to stackBody's pane group,
+// producing the full window_layout body when the Selvage band is present.
 // stackBody must be a region-relative body string from buildStackBody, not
-// checksum-wrapped; this function only splices the header in front and
-// re-wraps at fullBox's dimensions.
-func bandHeader(fullBox Box, headerPaneID string, headerHeight int, stackBody string) string {
+// checksum-wrapped; this function only splices the band cell on at the end
+// and re-wraps at fullBox's dimensions.
+func bandSelvage(fullBox Box, selvagePaneID string, bandHeight int, stackBody string) string {
 	open := strings.IndexByte(stackBody, '[')
 	closeIdx := strings.LastIndexByte(stackBody, ']')
 
 	var b strings.Builder
 	fmt.Fprintf(&b, "%dx%d,%d,%d[", fullBox.W, fullBox.H, fullBox.X, fullBox.Y)
-	fmt.Fprintf(&b, "%dx%d,%d,%d,%s", fullBox.W, headerHeight, fullBox.X, fullBox.Y, strings.TrimPrefix(headerPaneID, "%"))
 	if inner := stackBody[open+1 : closeIdx]; inner != "" {
-		b.WriteByte(',')
 		b.WriteString(inner)
+		b.WriteByte(',')
 	}
+	fmt.Fprintf(&b, "%dx%d,%d,%d,%s", fullBox.W, bandHeight, fullBox.X, fullBox.Y+fullBox.H-bandHeight, strings.TrimPrefix(selvagePaneID, "%"))
 	b.WriteByte(']')
 	return b.String()
 }
diff --git a/internal/reedengine/render/pins_test.go b/internal/reedengine/render/pins_test.go
index 239f00ae2..204e9f139 100644
--- a/internal/reedengine/render/pins_test.go
+++ b/internal/reedengine/render/pins_test.go
@@ -68,64 +68,64 @@ func TestFixedHeightPinsMatchesRulesPlacedHeights(t *testing.T) {
 		wantErr  bool
 	}{
 		{
-			name:     "HeaderPlusTwoFullStrandsOnlyTheHeaderIsPinned",
+			name:     "SelvagePlusTwoFullStrandsOnlyTheSelvageIsPinned",
 			strands:  twoFullSiblings(),
 			box:      Box{X: 0, Y: 0, W: 100, H: 21},
-			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 2}},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}},
 			wantPins: []Pin{{PaneID: "%h", Height: 2}},
 		},
 		{
-			// Mirrors rules_test.go's TestRulesHeaderBandEnumeratesHeaderPlusEveryStrandCell fixture:
-			// header unclamped at 3, mid collapses to CollapsedStripRows (2).
-			name:     "HeaderPlusShrinkAncestorWithPresentDescendantHeaderThenStrip",
+			// Mirrors rules_test.go's TestRulesSelvageBandEnumeratesEveryStrandCellPlusSelvage fixture:
+			// band unclamped at 3, mid collapses to CollapsedStripRows (2).
+			name:     "SelvagePlusShrinkAncestorWithPresentDescendantSelvageThenStrip",
 			strands:  belowParentChain(),
 			box:      Box{X: 0, Y: 0, W: 100, H: 21},
-			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 3}},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}},
 			wantPins: []Pin{{PaneID: "%h", Height: 3}, {PaneID: "%2", Height: 2}},
 		},
 		{
 			// Mirrors TestRulesGolden's BelowParentFormsBottomDominantStackOrderedByParentChain
-			// fixture with no header configured: only the strip (mid) is pinned.
-			name:     "NoHeaderConfiguredWithStripPresentOnlyTheStripIsPinned",
+			// fixture with no Selvage band configured: only the strip (mid) is pinned.
+			name:     "NoSelvageConfiguredWithStripPresentOnlyTheStripIsPinned",
 			strands:  belowParentChain(),
 			box:      Box{X: 0, Y: 0, W: 100, H: 21},
 			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
 			wantPins: []Pin{{PaneID: "%2", Height: 2}},
 		},
 		{
-			name:     "NoHeaderAndNoStripYieldsNoPins",
+			name:     "NoSelvageAndNoStripYieldsNoPins",
 			strands:  twoFullSiblings(),
 			box:      Box{X: 0, Y: 0, W: 100, H: 21},
 			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
 			wantPins: nil,
 		},
 		{
-			// headerRows=25 requested; clampHeaderHeight(25, box.H-1=20, MinFullRows=3) clamps to
+			// heightRows=25 requested; clampBandHeight(25, box.H-1=20, MinFullRows=3) clamps to
 			// windowRows-floor=17 to preserve the stack's floor — the pin must carry 17, never the
 			// configured 25.
-			name:     "OversizedHeaderHeightRowsPinCarriesTheClampedValueNotConfigured",
+			name:     "OversizedSelvageHeightRowsPinCarriesTheClampedValueNotConfigured",
 			strands:  twoFullSiblings(),
 			box:      Box{X: 0, Y: 0, W: 100, H: 21},
-			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 25}},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 25}},
 			wantPins: []Pin{{PaneID: "%h", Height: 17}},
 		},
 		{
-			// Mirrors rules_test.go's HeaderPresentClampedRowNoCellEverNonPositive golden row: the
+			// Mirrors rules_test.go's SelvagePresentClampedRowNoCellEverNonPositive golden row: the
 			// window is too short for the strip's natural CollapsedStripRows (2), and clampToFit's
 			// priority-1 pass reclaims it down to 1 — the pin must carry 1, never CollapsedStripRows.
 			name:     "TooShortWindowStripPinCarriesTheReclaimedValueNotCollapsedStripRows",
 			strands:  belowParentChain(),
 			box:      Box{X: 0, Y: 0, W: 100, H: 8},
-			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 2}},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}},
 			wantPins: []Pin{{PaneID: "%h", Height: 2}, {PaneID: "%2", Height: 1}},
 		},
 		{
-			// The sole-header branch: a header configured with no strand placed claims the whole box
-			// and has no absolute budget of its own — a stale one-row pin must never be emitted.
-			name:     "HeaderConfiguredWithNoStrandPlacedYieldsNoPin",
+			// The sole-band branch: a Selvage band configured with no strand placed claims the whole
+			// box and has no absolute budget of its own — a stale one-row pin must never be emitted.
+			name:     "SelvageConfiguredWithNoStrandPlacedYieldsNoPin",
 			strands:  nil,
 			box:      Box{X: 0, Y: 0, W: 100, H: 21},
-			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 3}},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}},
 			wantPins: nil,
 		},
 		{
@@ -171,22 +171,96 @@ func TestFixedHeightPinsMatchesRulesPlacedHeights(t *testing.T) {
 	}
 }
 
-// TestFixedHeightPinsOrdersTheHeaderPinFirstThenEveryStripPin asserts pin ordering directly: with a
-// header and two distinct strips present, the header pin is index 0 and both strip pins follow.
-func TestFixedHeightPinsOrdersTheHeaderPinFirstThenEveryStripPin(t *testing.T) {
-	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 2}}
+// TestFixedHeightPinsOrdersTheSelvagePinFirstThenEveryStripPin asserts pin ordering directly: with
+// the Selvage band and two distinct strips present, the Selvage pin is index 0 and both strip pins
+// follow — hook-array fire order, not screen position, since the band itself renders at the bottom.
+func TestFixedHeightPinsOrdersTheSelvagePinFirstThenEveryStripPin(t *testing.T) {
+	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}}
 	box := Box{X: 0, Y: 0, W: 100, H: 21}
 
 	pins := FixedHeightPins(twoDistinctStrips(), box, params)
 	if len(pins) != 3 {
-		t.Fatalf("FixedHeightPins() returned %d pins, want 3 (header + two strips): %+v", len(pins), pins)
+		t.Fatalf("FixedHeightPins() returned %d pins, want 3 (Selvage + two strips): %+v", len(pins), pins)
 	}
 	if pins[0].PaneID != "%h" {
-		t.Errorf("pins[0].PaneID = %q, want the header pane %q first", pins[0].PaneID, "%h")
+		t.Errorf("pins[0].PaneID = %q, want the Selvage pane %q first", pins[0].PaneID, "%h")
 	}
 	gotStrips := map[string]bool{pins[1].PaneID: true, pins[2].PaneID: true}
 	wantStrips := map[string]bool{"%1": true, "%2": true}
 	if diff := cmp.Diff(wantStrips, gotStrips); diff != "" {
-		t.Errorf("strip pins after the header (-want +got):\n%s", diff)
+		t.Errorf("strip pins after the Selvage pin (-want +got):\n%s", diff)
 	}
 }
+
+// TestSelvageBandCellOffsetsSumToTheBoxWithDivider asserts the bottom-band geometry directly: for a
+// box of height H with a Selvage band of height B and n placed strands, the strand cells occupy rows
+// box.Y .. box.Y+H-B-2 and the Selvage cell occupies rows box.Y+H-B .. box.Y+H-1 — the single row at
+// box.Y+H-B-1 is the divider between the stack and the band, never claimed by either.
+func TestSelvageBandCellOffsetsSumToTheBoxWithDivider(t *testing.T) {
+	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}}
+	box := Box{X: 0, Y: 0, W: 100, H: 21}
+
+	layout, _, err := Rules(belowParentChain(), box, params, nil)
+	if err != nil {
+		t.Fatalf("Rules() unexpected error: %v", err)
+	}
+
+	plan, err := planCells(belowParentChain(), box, params)
+	if err != nil {
+		t.Fatalf("planCells() unexpected error: %v", err)
+	}
+	bandHeight := plan.bandHeight
+
+	wantStackLastRow := box.Y + box.H - bandHeight - 2
+	wantBandFirstRow := box.Y + box.H - bandHeight
+	wantBandLastRow := box.Y + box.H - 1
+
+	stackLastRow := paneHeightFromLayoutY(t, layout, "%3")
+	if stackLastRow > wantStackLastRow {
+		t.Errorf("strand cell %q occupies rows up to %d, want at most %d (box.Y+H-B-2)", "%3", stackLastRow, wantStackLastRow)
+	}
+	bandFirstRow := bandFirstRowFromLayout(t, layout, "h")
+	if bandFirstRow != wantBandFirstRow {
+		t.Errorf("Selvage cell first row = %d, want %d (box.Y+H-B)", bandFirstRow, wantBandFirstRow)
+	}
+	bandLastRow := bandFirstRow + bandHeight - 1
+	if bandLastRow != wantBandLastRow {
+		t.Errorf("Selvage cell last row = %d, want %d (box.Y+H-1)", bandLastRow, wantBandLastRow)
+	}
+}
+
+// bandFirstRowFromLayout returns the y offset of paneID's cell within layout.
+func bandFirstRowFromLayout(t *testing.T, layout, paneID string) int {
+	t.Helper()
+	pattern := regexp.MustCompile(`\d+x\d+,\d+,(\d+),` + regexp.QuoteMeta(paneID))
+	m := pattern.FindStringSubmatch(layout)
+	if m == nil {
+		t.Fatalf("pane %q not found as a cell in layout %q", paneID, layout)
+	}
+	y, err := strconv.Atoi(m[1])
+	if err != nil {
+		t.Fatalf("pane %q y offset %q did not parse as an integer: %v", paneID, m[1], err)
+	}
+	return y
+}
+
+// paneHeightFromLayoutY returns the last row paneID's cell occupies within layout (its y offset plus
+// its height minus one).
+func paneHeightFromLayoutY(t *testing.T, layout, paneID string) int {
+	t.Helper()
+	want := strings.TrimPrefix(paneID, "%")
+	pattern := regexp.MustCompile(`\d+x(\d+),\d+,(\d+),` + regexp.QuoteMeta(want))
+	m := pattern.FindStringSubmatch(layout)
+	if m == nil {
+		t.Fatalf("pane %q not found as a cell in layout %q", paneID, layout)
+	}
+	height, err := strconv.Atoi(m[1])
+	if err != nil {
+		t.Fatalf("pane %q height %q did not parse as an integer: %v", paneID, m[1], err)
+	}
+	y, err := strconv.Atoi(m[2])
+	if err != nil {
+		t.Fatalf("pane %q y offset %q did not parse as an integer: %v", paneID, m[2], err)
+	}
+	return y + height - 1
+}
diff --git a/internal/reedengine/render/policy.go b/internal/reedengine/render/policy.go
index 60ae0b92c..5b212dc56 100644
--- a/internal/reedengine/render/policy.go
+++ b/internal/reedengine/render/policy.go
@@ -28,25 +28,25 @@ func partitionByAnchor(strands []Strand) (stack []Strand) {
 }
 
 // removeDuplicatePaneCells returns stack with every entry dropped whose PaneID has already been
-// spoken for — by the header band, or by an earlier entry in stack.
+// spoken for — by the Selvage band, or by an earlier entry in stack.
 // It is the structural half of the same rule breakCycles enforces for parent chains: a corrupt
 // persisted table must never be able to make Rules emit something the multiplexer answers
 // destructively.
 //
 // A window_layout string names each pane exactly once. Emitting a pane number twice is not rejected
 // by tmux — it accepts the string with exit 0, assigns cells positionally, and DESTROYS every pane
-// the (now short) cell list no longer covers (verified live, tmux 3.6). The header is the case that
-// makes this reachable rather than hypothetical, because bandHeader splices its cell in
+// the (now short) cell list no longer covers (verified live, tmux 3.6). The Selvage band is the case
+// that makes this reachable rather than hypothetical, because bandSelvage splices its cell in
 // independently of the stack body and so cannot see a stack entry naming the same pane.
 //
 // The engine clears such bindings at load (clearConflictingPaneBindings), so in a healthy process
 // this filter never removes anything. It exists so that the destructive outcome is impossible from
 // inside this package's own contract rather than only prevented by a caller remembering to sanitize
 // first — Rules is documented as a pure, TOTAL function over whatever strand set it is handed.
-func removeDuplicatePaneCells(stack []Strand, headerPaneID string) []Strand {
+func removeDuplicatePaneCells(stack []Strand, bandPaneID string) []Strand {
 	claimed := make(map[string]bool, len(stack)+1)
-	if headerPaneID != "" {
-		claimed[headerPaneID] = true
+	if bandPaneID != "" {
+		claimed[bandPaneID] = true
 	}
 
 	out := make([]Strand, 0, len(stack))
diff --git a/internal/reedengine/render/rules.go b/internal/reedengine/render/rules.go
index 7715b3abe..1bcb25e92 100644
--- a/internal/reedengine/render/rules.go
+++ b/internal/reedengine/render/rules.go
@@ -14,18 +14,18 @@ import (
 // tmux window_layout string. planCells builds it; Rules and FixedHeightPins each read the parts they
 // need and perform no policy of their own.
 type cellPlan struct {
-	// hasHeader reports whether p.Header.PaneID is non-empty — a header band
-	// is being rendered at all.
-	hasHeader bool
-	// soleHeader reports whether the header claims the whole box as a
+	// hasBand reports whether p.Selvage.PaneID is non-empty — the Selvage
+	// band is being rendered at all.
+	hasBand bool
+	// soleBand reports whether the Selvage band claims the whole box as a
 	// bracket-less single-cell body because no strand was placed. When true,
-	// headerHeight, stackBox, ordered, and placements carry no meaning.
-	soleHeader bool
-	// headerHeight is the header band's height after clampHeaderHeight,
-	// valid only when hasHeader is true and soleHeader is false.
-	headerHeight int
-	// stackBox is the region the strand stack is laid out within, below the
-	// header band and its one-row divider when hasHeader is true.
+	// bandHeight, stackBox, ordered, and placements carry no meaning.
+	soleBand bool
+	// bandHeight is the Selvage band's height after clampBandHeight,
+	// valid only when hasBand is true and soleBand is false.
+	bandHeight int
+	// stackBox is the region the strand stack is laid out within, above the
+	// Selvage band and its one-row divider when hasBand is true.
 	stackBox Box
 	// ordered is the below-parent stack, filtered and ordered by parent-chain
 	// depth.
@@ -35,7 +35,7 @@ type cellPlan struct {
 }
 
 // planCells performs the policy half of Rules: filtering and ordering strands into the below-parent
-// stack, and deciding the header/stack height split. It returns the same AnchorOwnWindow rejection
+// stack, and deciding the Selvage/stack height split. It returns the same AnchorOwnWindow rejection
 // error Rules has always returned. Rules and FixedHeightPins are the mechanics layer built on top of
 // this shared policy result.
 func planCells(strands []Strand, box Box, p Params) (cellPlan, error) {
@@ -48,53 +48,53 @@ func planCells(strands []Strand, box Box, p Params) (cellPlan, error) {
 	// Repair any corrupt cyclic parent table before depth-based ordering,
 	// so a bad persisted record can never hang layout.
 	fixed := breakCycles(strands)
-	stack := removeDuplicatePaneCells(partitionByAnchor(fixed), p.Header.PaneID)
+	stack := removeDuplicatePaneCells(partitionByAnchor(fixed), p.Selvage.PaneID)
 	ordered := orderStack(stack)
 
-	hasHeader := p.Header.PaneID != ""
-	if hasHeader && len(ordered) == 0 {
-		// No strand placed: the header claims the whole box as a
+	hasBand := p.Selvage.PaneID != ""
+	if hasBand && len(ordered) == 0 {
+		// No strand placed: the Selvage band claims the whole box as a
 		// bracket-less single-cell body (see Rules' doc comment) — never a
 		// zero-height cell inside a group, which the real multiplexer
 		// mishandles. No focus target exists without a placed strand.
-		return cellPlan{hasHeader: true, soleHeader: true}, nil
+		return cellPlan{hasBand: true, soleBand: true}, nil
 	}
 
 	stackBox := box
-	headerHeight := 0
-	if hasHeader {
-		// The header and the strand stack are physically adjacent panes, so
-		// tmux/psmux always renders a one-row border between them — the same
-		// budget buildStackBody already reserves between individual strands
-		// (dividers := n-1). That row must come out of the window's total
-		// budget before clampHeaderHeight (height.go) decides the
-		// window-split: an oversized configured height_rows can never
-		// shrink the strand stack below its MinFullRows floor — the header
-		// yields rows first. clampToFit (called inside stackHeights below)
-		// then distributes rows AMONG strands within whatever (possibly
-		// clamped) stack region results.
-		const headerDivider = 1
-		headerHeight = clampHeaderHeight(p.Header.HeightRows, box.H-headerDivider, p.MinFullRows)
-		stackBox = Box{X: box.X, Y: box.Y + headerHeight + headerDivider, W: box.W, H: box.H - headerHeight - headerDivider}
+	bandHeight := 0
+	if hasBand {
+		// The Selvage band and the strand stack are physically adjacent
+		// panes, so tmux/psmux always renders a one-row border between them
+		// — the same budget buildStackBody already reserves between
+		// individual strands (dividers := n-1). That row must come out of
+		// the window's total budget before clampBandHeight (height.go)
+		// decides the window-split: an oversized configured height_rows can
+		// never shrink the strand stack below its MinFullRows floor — the
+		// Selvage band yields rows first. clampToFit (called inside
+		// stackHeights below) then distributes rows AMONG strands within
+		// whatever (possibly clamped) stack region results.
+		const bandDivider = 1
+		bandHeight = clampBandHeight(p.Selvage.HeightRows, box.H-bandDivider, p.MinFullRows)
+		stackBox = Box{X: box.X, Y: box.Y, W: box.W, H: box.H - bandHeight - bandDivider}
 	}
 
 	placements := stackHeights(ordered, stackBox, p)
 
 	return cellPlan{
-		hasHeader:    hasHeader,
-		headerHeight: headerHeight,
-		stackBox:     stackBox,
-		ordered:      ordered,
-		placements:   placements,
+		hasBand:    hasBand,
+		bandHeight: bandHeight,
+		stackBox:   stackBox,
+		ordered:    ordered,
+		placements: placements,
 	}, nil
 }
 
 // Rules computes the tmux window_layout string and focus pane id for strands laid out within box.
 // It rejects any strand declaring AnchorOwnWindow, repairs corrupt cyclic parent chains, and drops
-// any strand whose PaneID is already spoken for by the header band or by an earlier strand
+// any strand whose PaneID is already spoken for by the Selvage band or by an earlier strand
 // (see removeDuplicatePaneCells for why emitting one pane number twice is destructive).
-// When p.Header.PaneID is non-empty, Rules carves a fixed-height top band for the header before
-// laying out the stack below.
+// When p.Selvage.PaneID is non-empty, Rules carves a fixed-height bottom band for the Selvage before
+// laying out the stack above it.
 // paneOrder resequences cells to match physical pane position;
 // a nil paneOrder keeps the intended (parent above child) order.
 func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout string, focus string, err error) {
@@ -103,24 +103,24 @@ func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout stri
 		return "", "", err
 	}
 
-	if plan.soleHeader {
-		sole := fmt.Sprintf("%dx%d,%d,%d,%s", box.W, box.H, box.X, box.Y, strings.TrimPrefix(p.Header.PaneID, "%"))
+	if plan.soleBand {
+		sole := fmt.Sprintf("%dx%d,%d,%d,%s", box.W, box.H, box.X, box.Y, strings.TrimPrefix(p.Selvage.PaneID, "%"))
 		return wrapLayout(sole), "", nil
 	}
 
 	placements := resequenceByPaneOrder(plan.placements, paneOrder)
 
 	body := buildStackBody(plan.stackBox, placements)
-	if plan.hasHeader {
-		body = bandHeader(box, p.Header.PaneID, plan.headerHeight, body)
+	if plan.hasBand {
+		body = bandSelvage(box, p.Selvage.PaneID, plan.bandHeight, body)
 	}
 	focus = focusTarget(plan.ordered)
 	return wrapLayout(body), focus, nil
 }
 
-// Pin is one pane whose height is an absolute row budget rather than "whatever is left" — the header
-// band or a collapsed strip. Height is the height Rules actually placed the cell at, after
-// clampHeaderHeight/clampToFit — never the raw configured budget (p.Header.HeightRows or
+// Pin is one pane whose height is an absolute row budget rather than "whatever is left" — the
+// Selvage band or a collapsed strip. Height is the height Rules actually placed the cell at, after
+// clampBandHeight/clampToFit — never the raw configured budget (p.Selvage.HeightRows or
 // p.CollapsedStripRows read directly), since either can yield rows under a too-short window.
 type Pin struct {
 	// PaneID is the tmux pane id this pin applies to.
@@ -129,7 +129,7 @@ type Pin struct {
 	Height int
 }
 
-// FixedHeightPins reports the panes whose heights are absolute row budgets — the header band and
+// FixedHeightPins reports the panes whose heights are absolute row budgets — the Selvage band and
 // every collapsed strip — at the heights Rules actually placed them at for the identical
 // (strands, box, p) inputs. It shares Rules' own policy composition (planCells) so the two can never
 // disagree about a placed height.
@@ -137,19 +137,23 @@ type Pin struct {
 // FixedHeightPins takes no paneOrder: a pin names its pane by tmux pane id, so emission order carries
 // no geometry — paneOrder only resequences layout-string cells, which FixedHeightPins never produces.
 //
-// It is pure and total like Rules. On any error from planCells, on the sole-header shape (there the
-// header claims the whole box and has no absolute budget of its own), or whenever there is otherwise
-// nothing to report, it returns nil. A caller must treat a nil return as "nothing is pinned", never
-// as "no opinion" — the disposition is exactly as authoritative as a non-nil one.
+// It is pure and total like Rules. On any error from planCells, on the sole-band shape (there the
+// Selvage band claims the whole box and has no absolute budget of its own), or whenever there is
+// otherwise nothing to report, it returns nil. A caller must treat a nil return as "nothing is
+// pinned", never as "no opinion" — the disposition is exactly as authoritative as a non-nil one.
+//
+// The Selvage band pin, when present, is always first in the returned slice, ahead of every strip
+// pin — hook-array index is fire order, not screen position, so the band pin keeps index 0 even
+// though the band itself renders at the bottom of the window.
 func FixedHeightPins(strands []Strand, box Box, p Params) []Pin {
 	plan, err := planCells(strands, box, p)
-	if err != nil || plan.soleHeader {
+	if err != nil || plan.soleBand {
 		return nil
 	}
 
 	var pins []Pin
-	if plan.hasHeader {
-		pins = append(pins, Pin{PaneID: p.Header.PaneID, Height: plan.headerHeight})
+	if plan.hasBand {
+		pins = append(pins, Pin{PaneID: p.Selvage.PaneID, Height: plan.bandHeight})
 	}
 	for _, pl := range plan.placements {
 		if pl.strip {
diff --git a/internal/reedengine/render/rules_test.go b/internal/reedengine/render/rules_test.go
index df51ae8fd..53562cb47 100644
--- a/internal/reedengine/render/rules_test.go
+++ b/internal/reedengine/render/rules_test.go
@@ -1,7 +1,7 @@
 // rules_test.go golden-tests the composed Rules entry point: the below-parent stack ordered by
 // parent chain, hidden-strand exclusion, empty/single-strand/parent-child edges, the
 // checksum-prefix invariant, the own-window rejection error, pane-order resequencing to physical
-// pane position, and the header top-band enumeration (Params.Header).
+// pane position, and the Selvage bottom-band enumeration (Params.Selvage).
 // It also pins the two layout regimes a real (as opposed to config-pinned) terminal box makes
 // reachable: a budget-satisfying box where height.go's clamps never fire, and a too-short box where
 // they must — the latter with a companion assertion that no clamped cell height is ever non-positive.
@@ -35,7 +35,7 @@ func TestRulesGolden(t *testing.T) {
 		name      string
 		strands   []Strand
 		box       Box
-		header    Header
+		selvage   Selvage
 		wantBody  string
 		wantFocus string
 	}{
@@ -92,35 +92,37 @@ func TestRulesGolden(t *testing.T) {
 		{
 			// A live terminal is routinely 24 or 30 rows, unlike the 220x50
 			// box a pinned config used to always hand Rules — a box this
-			// short means clampHeaderHeight/clampToFit now govern the common
+			// short means clampBandHeight/clampToFit now govern the common
 			// case rather than almost never firing. This row's box has room
-			// for the header band, its one-row divider, the collapsed strip
+			// for the Selvage band, its one-row divider, the collapsed strip
 			// at CollapsedStripRows, and both full panes above MinFullRows,
-			// so no clamp fires: header=2 (unclamped: floor=3, maxHeader=
-			// box.H-1-3=20, 2<=20), stack region {Y:3,H:21}, usable=21-2
+			// so no clamp fires: band=2 (unclamped: floor=3, maxBand=
+			// box.H-1-3=20, 2<=20), stack region {Y:0,H:21}, usable=21-2
 			// dividers=19, stripDemand=2 (mid collapses), fullRemaining=17
-			// split 8/9 between root and active (remainder to active).
-			name:      "HeaderPresentBudgetSatisfyingPreservesConfiguredHeaderAndStripHeights",
+			// split 8/9 between root and active (remainder to active); the
+			// band cell lands last, at Y=box.H-2=22.
+			name:      "SelvagePresentBudgetSatisfyingPreservesConfiguredSelvageAndStripHeights",
 			strands:   belowParentChain(),
 			box:       Box{X: 0, Y: 0, W: 100, H: 24},
-			header:    Header{PaneID: "%h", HeightRows: 2},
-			wantBody:  "100x24,0,0[100x2,0,0,h,100x8,0,3,1,100x2,0,12,2,100x9,0,15,3]",
+			selvage:   Selvage{PaneID: "%h", HeightRows: 2},
+			wantBody:  "100x24,0,0[100x8,0,0,1,100x2,0,9,2,100x9,0,12,3,100x2,0,22,h]",
 			wantFocus: "%3",
 		},
 		{
 			// The same strand fixture against a box too short for those
-			// budgets: header=2 stays unclamped (floor=3, maxHeader=
-			// box.H-1-3=4, 2<=4), stack region {Y:3,H:5}, usable=5-2
+			// budgets: band=2 stays unclamped (floor=3, maxBand=
+			// box.H-1-3=4, 2<=4), stack region {Y:0,H:5}, usable=5-2
 			// dividers=3, stripDemand=2 (mid), fullRemaining=1 split 0/1
 			// between root and active (remainder to active) — root's natural
 			// 0 borrows 1 row via clampToFit's priority-1 reclaim, which the
 			// strip (mid) repays by shrinking from its natural 2 down to 1,
-			// leaving every stack cell at exactly 1 row.
-			name:      "HeaderPresentClampedRowNoCellEverNonPositive",
+			// leaving every stack cell at exactly 1 row; the band cell lands
+			// last, at Y=box.H-2=6.
+			name:      "SelvagePresentClampedRowNoCellEverNonPositive",
 			strands:   belowParentChain(),
 			box:       Box{X: 0, Y: 0, W: 100, H: 8},
-			header:    Header{PaneID: "%h", HeightRows: 2},
-			wantBody:  "100x8,0,0[100x2,0,0,h,100x1,0,3,1,100x1,0,5,2,100x1,0,7,3]",
+			selvage:   Selvage{PaneID: "%h", HeightRows: 2},
+			wantBody:  "100x8,0,0[100x1,0,0,1,100x1,0,2,2,100x1,0,4,3,100x2,0,6,h]",
 			wantFocus: "%3",
 		},
 	}
@@ -128,8 +130,8 @@ func TestRulesGolden(t *testing.T) {
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
 			p := params
-			if tt.header != (Header{}) {
-				p.Header = tt.header
+			if tt.selvage != (Selvage{}) {
+				p.Selvage = tt.selvage
 			}
 			layout, focus, err := Rules(tt.strands, tt.box, p, nil)
 			if err != nil {
@@ -168,7 +170,7 @@ var cellHeightPattern = regexp.MustCompile(`\d+x(\d+),`)
 
 // TestRulesClampedRowNeverEmitsANonPositiveCellHeight is the companion assertion
 // TestRulesGolden's table shape cannot express: every cell height in the
-// HeaderPresentClampedRowNoCellEverNonPositive golden row must be at least 1, no matter how far the
+// SelvagePresentClampedRowNoCellEverNonPositive golden row must be at least 1, no matter how far the
 // clamp had to reach. clampToFit's own documented last-resort branch (the active pane absorbing
 // whatever the earlier priority passes could not reclaim) is deliberately permitted to leave the
 // emitted cell heights summing to MORE than box.H when the window is shorter than the pane count —
@@ -176,7 +178,7 @@ var cellHeightPattern = regexp.MustCompile(`\d+x(\d+),`)
 // future reader should not read an over-sum in some OTHER fixture as a defect this test would have
 // caught.
 func TestRulesClampedRowNeverEmitsANonPositiveCellHeight(t *testing.T) {
-	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 2}}
+	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}}
 	box := Box{X: 0, Y: 0, W: 100, H: 8}
 
 	layout, _, err := Rules(belowParentChain(), box, params, nil)
@@ -278,16 +280,16 @@ func TestRulesPaneOrderResequencesCellsToPhysicalOrder(t *testing.T) {
 	}
 }
 
-// TestRulesHeaderBandEnumeratesHeaderPlusEveryStrandCell asserts the header top-band shape card
-// 15/16 add: a fixed-height header cell at the top, followed by the below-parent stack laid out in
-// the shrunk region below it — the emitted window_layout must enumerate the header cell plus every
-// strand cell so the live-pane count the caller's select-layout applies against matches tmux's
-// actual pane set.
-func TestRulesHeaderBandEnumeratesHeaderPlusEveryStrandCell(t *testing.T) {
+// TestRulesSelvageBandEnumeratesEveryStrandCellPlusSelvage asserts the Selvage bottom-band shape: the
+// below-parent stack laid out in the shrunk region at the top, followed by a fixed-height Selvage
+// cell at the bottom — the emitted window_layout must enumerate every strand cell plus the Selvage
+// cell so the live-pane count the caller's select-layout applies against matches tmux's actual pane
+// set.
+func TestRulesSelvageBandEnumeratesEveryStrandCellPlusSelvage(t *testing.T) {
 	params := Params{
 		CollapsedStripRows: 2,
 		MinFullRows:        3,
-		Header:             Header{PaneID: "%h", HeightRows: 3},
+		Selvage:            Selvage{PaneID: "%h", HeightRows: 3},
 	}
 	box := Box{X: 0, Y: 0, W: 100, H: 21}
 
@@ -296,37 +298,37 @@ func TestRulesHeaderBandEnumeratesHeaderPlusEveryStrandCell(t *testing.T) {
 		t.Fatalf("Rules() unexpected error: %v", err)
 	}
 
-	// headerHeight=3 (unclamped: with the header's own one-row divider
+	// bandHeight=3 (unclamped: with the Selvage band's own one-row divider
 	// budget subtracted first (box.H-1=20), MinFullRows=3 leaves 17 rows for
 	// the stack, well above the natural split's needs). The stack region is
-	// {X:0,Y:4,W:100,H:17} (Y shifted by headerHeight+1 for the divider
-	// between the header band and the stack): usable=17-2 dividers=15,
-	// stripDemand=2 (mid collapses to CollapsedStripRows), fullRemaining=13
-	// split 6/7 between root and active (remainder to active).
-	wantBody := "100x21,0,0[100x3,0,0,h,100x6,0,4,1,100x2,0,11,2,100x7,0,14,3]"
+	// {X:0,Y:0,W:100,H:17}: usable=17-2 dividers=15, stripDemand=2 (mid
+	// collapses to CollapsedStripRows), fullRemaining=13 split 6/7 between
+	// root and active (remainder to active). The Selvage cell lands last, at
+	// Y=box.H-3=18.
+	wantBody := "100x21,0,0[100x6,0,0,1,100x2,0,7,2,100x7,0,10,3,100x3,0,18,h]"
 	if want := wrapLayout(wantBody); layout != want {
-		t.Errorf("Rules() with header layout = %q, want %q", layout, want)
+		t.Errorf("Rules() with Selvage layout = %q, want %q", layout, want)
 	}
 	if want := "%3"; focus != want {
-		t.Errorf("Rules() with header focus = %q, want %q (header never affects focus)", focus, want)
+		t.Errorf("Rules() with Selvage focus = %q, want %q (Selvage never affects focus)", focus, want)
 	}
 }
 
-// TestRulesHeaderWithNoPlacedStrandClaimsWholeBoxAsSoleCell pins the empty-stack header shape: with
-// a header pane and ZERO placed strands, Rules must emit the header as a bracket-less single-cell
-// body claiming the whole box — the same shape tmux reports for a one-pane window — never a
-// zero-height header cell inside a group (the fable-header-r1 finding: headerHeight stayed 0 on
-// this path and bandHeader emitted a literal "Wx0" cell, exactly the shape
-// TestHeaderNeverGetsZeroHeightLayoutCell exists to forbid, while the doc comment claimed the
-// header "may claim the whole box").
+// TestRulesSelvageWithNoPlacedStrandClaimsWholeBoxAsSoleCell pins the empty-stack Selvage shape: with
+// a Selvage pane and ZERO placed strands, Rules must emit the Selvage band as a bracket-less
+// single-cell body claiming the whole box — the same shape tmux reports for a one-pane window — never
+// a zero-height Selvage cell inside a group (the fable-header-r1 finding: bandHeight stayed 0 on
+// this path and bandSelvage emitted a literal "Wx0" cell, exactly the shape
+// TestSelvageNeverGetsZeroHeightLayoutCell exists to forbid, while the doc comment claimed the
+// band "may claim the whole box").
 // Unreachable through applyLayoutLocked today (anyPlacedStrand gates the apply),
 // but Rules is a pure function whose contract must hold for any caller.
-func TestRulesHeaderWithNoPlacedStrandClaimsWholeBoxAsSoleCell(t *testing.T) {
+func TestRulesSelvageWithNoPlacedStrandClaimsWholeBoxAsSoleCell(t *testing.T) {
 	box := Box{X: 0, Y: 0, W: 100, H: 21}
-	p := Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 3}}
+	p := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}}
 
 	// nil strands and an all-filtered stack (a hidden strand) must both
-	// produce the sole-header shape.
+	// produce the sole-band shape.
 	for name, strands := range map[string][]Strand{
 		"NilStrands":       nil,
 		"OnlyHiddenStrand": {{GUID: "hid", PaneID: "%9", Live: true, Display: Display{Anchor: AnchorHidden}}},
@@ -336,7 +338,7 @@ func TestRulesHeaderWithNoPlacedStrandClaimsWholeBoxAsSoleCell(t *testing.T) {
 			t.Fatalf("%s: Rules() unexpected error: %v", name, err)
 		}
 		if want := wrapLayout("100x21,0,0,h"); layout != want {
-			t.Errorf("%s: Rules() layout = %q, want the sole-header body %q", name, layout, want)
+			t.Errorf("%s: Rules() layout = %q, want the sole-band body %q", name, layout, want)
 		}
 		if focus != "" {
 			t.Errorf("%s: Rules() focus = %q, want \"\" (no placed strand to focus)", name, focus)
@@ -344,20 +346,20 @@ func TestRulesHeaderWithNoPlacedStrandClaimsWholeBoxAsSoleCell(t *testing.T) {
 	}
 }
 
-// TestRulesNoHeaderPreservesPreHeaderBehavior asserts a zero-value Params.Header (empty PaneID)
-// produces byte-identical output to omitting Header entirely — every pre-header caller must be
+// TestRulesNoSelvagePreservesPreSelvageBehavior asserts a zero-value Params.Selvage (empty PaneID)
+// produces byte-identical output to omitting Selvage entirely — every pre-Selvage caller must be
 // unaffected.
-func TestRulesNoHeaderPreservesPreHeaderBehavior(t *testing.T) {
+func TestRulesNoSelvagePreservesPreSelvageBehavior(t *testing.T) {
 	strands := belowParentChain()
 	box := Box{X: 0, Y: 0, W: 100, H: 21}
 
-	withZeroHeader, focus1, err1 := Rules(strands, box, Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{}}, nil)
+	withZeroSelvage, focus1, err1 := Rules(strands, box, Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{}}, nil)
 	without, focus2, err2 := Rules(strands, box, Params{CollapsedStripRows: 2, MinFullRows: 3}, nil)
 	if err1 != nil || err2 != nil {
 		t.Fatalf("Rules() unexpected errors: %v, %v", err1, err2)
 	}
-	if withZeroHeader != without || focus1 != focus2 {
-		t.Errorf("Rules() with zero-value Header = (%q,%q), want identical to omitting Header entirely (%q,%q)", withZeroHeader, focus1, without, focus2)
+	if withZeroSelvage != without || focus1 != focus2 {
+		t.Errorf("Rules() with zero-value Selvage = (%q,%q), want identical to omitting Selvage entirely (%q,%q)", withZeroSelvage, focus1, without, focus2)
 	}
 }
 
@@ -401,29 +403,29 @@ func paneNumberCounts(layout string) map[string]int {
 // tmux does not REJECT a window_layout string naming one pane twice: it accepts it with exit 0,
 // assigns cells positionally, and destroys every pane the short cell list no longer covers
 // (reproduced live, tmux 3.6 — one `lyx reed up` reduced a two-pane session to one, reported
-// ok:true, and then reported the strand live against the header pane).
+// ok:true, and then reported the strand live against the Selvage pane).
 // Rules is documented as pure and TOTAL, so it must be structurally incapable of producing that
 // string no matter how corrupt the strand table it is handed.
 func TestRules_NeverEmitsOnePaneNumberTwice(t *testing.T) {
 	box := Box{X: 0, Y: 0, W: 100, H: 40}
 
 	tests := []struct {
-		name         string
-		strands      []Strand
-		headerPaneID string
+		name          string
+		strands       []Strand
+		selvagePaneID string
 	}{
 		{
-			name:         "a strand bound to the header's own pane",
-			strands:      []Strand{{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}}},
-			headerPaneID: "%1",
+			name:          "a strand bound to the Selvage band's own pane",
+			strands:       []Strand{{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}}},
+			selvagePaneID: "%1",
 		},
 		{
-			name: "a strand bound to the header's pane beside a healthy strand",
+			name: "a strand bound to the Selvage band's pane beside a healthy strand",
 			strands: []Strand{
 				{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
 				{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
 			},
-			headerPaneID: "%1",
+			selvagePaneID: "%1",
 		},
 		{
 			name: "two strands bound to one pane",
@@ -431,21 +433,21 @@ func TestRules_NeverEmitsOnePaneNumberTwice(t *testing.T) {
 				{GUID: "a", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
 				{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
 			},
-			headerPaneID: "%1",
+			selvagePaneID: "%1",
 		},
 		{
-			name: "two strands bound to one pane with no header at all",
+			name: "two strands bound to one pane with no Selvage band at all",
 			strands: []Strand{
 				{GUID: "a", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
 				{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
 			},
-			headerPaneID: "",
+			selvagePaneID: "",
 		},
 	}
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			params := Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: tt.headerPaneID, HeightRows: 1}}
+			params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: tt.selvagePaneID, HeightRows: 1}}
 			layout, _, err := Rules(tt.strands, box, params, nil)
 			if err != nil {
 				t.Fatalf("Rules() error = %v; want nil", err)
@@ -460,11 +462,11 @@ func TestRules_NeverEmitsOnePaneNumberTwice(t *testing.T) {
 }
 
 // TestRules_KeepsTheFirstOwnerWhenPaneCellsCollide pins WHICH strand survives a collision, so the
-// repair stays deterministic rather than merely non-destructive: the header always keeps its own
-// pane, and among strands the earlier table entry wins.
+// repair stays deterministic rather than merely non-destructive: the Selvage band always keeps its
+// own pane, and among strands the earlier table entry wins.
 func TestRules_KeepsTheFirstOwnerWhenPaneCellsCollide(t *testing.T) {
 	box := Box{X: 0, Y: 0, W: 100, H: 40}
-	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%1", HeightRows: 1}}
+	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%1", HeightRows: 1}}
 
 	strands := []Strand{
 		{GUID: "first", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
diff --git a/internal/reedengine/render/types.go b/internal/reedengine/render/types.go
index 6a0c245f9..0f85748dc 100644
--- a/internal/reedengine/render/types.go
+++ b/internal/reedengine/render/types.go
@@ -78,15 +78,15 @@ type Box struct {
 	X, Y, W, H int
 }
 
-// Header carries the always-on operator console pane's placement: PaneID names the tmux pane Rules
-// renders as a fixed-height top band above the below-parent stack,
-// and HeightRows is its requested row count (before clampHeaderHeight's floor-preserving
+// Selvage carries the always-on operator console pane's placement: PaneID names the tmux pane Rules
+// renders as a fixed-height bottom band below the below-parent stack,
+// and HeightRows is its requested row count (before clampBandHeight's floor-preserving
 // adjustment).
-// A zero-value Header (empty PaneID) means "no header" — Rules then lays out exactly as it did
-// before the header pane existed, so every pre-header caller is unaffected.
-// The header is never a Strand: it is injected at this Params seam instead of being modelled in the
+// A zero-value Selvage (empty PaneID) means "no band" — Rules then lays out exactly as it did
+// before the band existed, so every pre-band caller is unaffected.
+// The Selvage is never a Strand: it is injected at this Params seam instead of being modelled in the
 // strand slice (Shared Decision header-is-not-a-strand).
-type Header struct {
+type Selvage struct {
 	PaneID     string
 	HeightRows int
 }
@@ -99,10 +99,10 @@ type Params struct {
 	CollapsedStripRows int
 	// MinFullRows is the floor height the clamp rule tries to preserve for
 	// a full (non-collapsed) pane when the window is too short to satisfy
-	// every strand's natural height. clampHeaderHeight also uses this as
-	// the strand-stack region's floor when a header pane is present.
+	// every strand's natural height. clampBandHeight also uses this as
+	// the strand-stack region's floor when the Selvage band is present.
 	MinFullRows int
-	// Header carries the always-on operator console pane's placement, if
-	// any. A zero-value Header (empty PaneID) means no header is rendered.
-	Header Header
+	// Selvage carries the always-on operator console pane's placement, if
+	// any. A zero-value Selvage (empty PaneID) means no band is rendered.
+	Selvage Selvage
 }
diff --git a/internal/reedengine/spawn.go b/internal/reedengine/spawn.go
index 71c3863bc..9e02460bc 100644
--- a/internal/reedengine/spawn.go
+++ b/internal/reedengine/spawn.go
@@ -19,9 +19,9 @@ import (
 
 // planPaneTarget always yields a split target for the next strand realization
 // — it never adopts an existing pane. The surviving rules are a pure function
-// of live and headerPaneID: prefer the tallest alive non-header pane, fall
-// back to any present non-header pane (a corpse) when none is alive, and fall
-// back to live[0] (the header itself) when no non-header pane exists at all.
+// of live and selvagePaneID: prefer the tallest alive non-Selvage pane, fall
+// back to any present non-Selvage pane (a corpse) when none is alive, and fall
+// back to live[0] (Selvage itself) when no non-Selvage pane exists at all.
 //
 // Adoption used to give a fresh session's initial pane a use rather than
 // splitting a needless second one, but the seam it required — deciding
@@ -32,11 +32,11 @@ import (
 // strand's command was typed onto its screen and never ran, with status
 // reporting live:true and no such process on the box) and M16 (adoption
 // claimed an operator's own manually-created split-window pane). Once the
-// untracked reap is authorized by an alive header (reconcile.go), the initial
+// untracked reap is authorized by an alive Selvage (reconcile.go), the initial
 // pane is disposed of like any other untracked pane before this function ever
 // runs, so a fresh split — idle by construction — costs one kill-pane plus
 // one split-window and buys correctness back.
-func planPaneTarget(live []LivePane, headerPaneID string) (splitTargetID string, err error) {
+func planPaneTarget(live []LivePane, selvagePaneID string) (splitTargetID string, err error) {
 	if len(live) == 0 {
 		return "", fmt.Errorf("session has no panes to split")
 	}
@@ -44,7 +44,7 @@ func planPaneTarget(live []LivePane, headerPaneID string) (splitTargetID string,
 	splitTargetID = ""
 	tallestAlive := -1
 	for _, p := range live {
-		if p.ID == headerPaneID || p.Dead {
+		if p.ID == selvagePaneID || p.Dead {
 			continue
 		}
 		if p.Height > tallestAlive {
@@ -53,19 +53,19 @@ func planPaneTarget(live []LivePane, headerPaneID string) (splitTargetID string,
 		}
 	}
 	if splitTargetID == "" {
-		// No alive non-header pane: fall back to any present non-header
-		// pane (a dead corpse), mirroring the pre-header "every pane dead"
+		// No alive non-Selvage pane: fall back to any present non-Selvage
+		// pane (a dead corpse), mirroring the pre-Selvage "every pane dead"
 		// fallback.
 		for _, p := range live {
-			if p.ID != headerPaneID {
+			if p.ID != selvagePaneID {
 				splitTargetID = p.ID
 				break
 			}
 		}
 	}
 	if splitTargetID == "" {
-		// No non-header pane exists at all: every strand has been removed
-		// and only the header remains. Split the header itself so this add
+		// No non-Selvage pane exists at all: every strand has been removed
+		// and only Selvage remains. Split Selvage itself so this add
 		// still has a pane to split.
 		splitTargetID = live[0].ID
 	}
@@ -146,13 +146,13 @@ func (e *Engine) launchStrandLocked(st *ReedState, s *Strand, launchCmd string)
 		}
 	}
 
-	splitTargetID, err := planPaneTarget(live, st.HeaderPaneID)
+	splitTargetID, err := planPaneTarget(live, st.SelvagePaneID)
 	if err != nil {
 		return err
 	}
 
 	// -c pins the new pane's cwd to Geometry.PaneCwd, exactly as
-	// new-session and the header split (lifecycle.go) already do. Without
+	// new-session and Selvage's own split (lifecycle.go) already do. Without
 	// it tmux resolves the cwd from the invoking CLIENT — verified live
 	// (tmux 3.6): a split issued from outside tmux lands in the calling
 	// process's cwd, neither the target pane's cwd nor the session's. That
@@ -161,7 +161,24 @@ func (e *Engine) launchStrandLocked(st *ReedState, s *Strand, launchCmd string)
 	// caller injects a cwd through the RunCLIIn seam instead — at which
 	// point every strand command would run against the wrong tree while
 	// reed reported success.
-	out, err := e.tmux.output("split-window", "-t", splitTargetID, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}")
+	//
+	// -b is the one exception to "a strand split always lands below its
+	// target": when planPaneTarget's third tier fires (Selvage is the
+	// sole pane, so it is the split target), splitting below it — tmux's
+	// default — would insert the new strand pane AFTER Selvage in tmux's
+	// own physical pane order, the same "cells apply positionally, not by
+	// pane id" hazard splitSelvagePaneAtBottomLocked's doc comment
+	// describes for Selvage's own split. -b keeps the new strand pane
+	// physically above Selvage instead, preserving the bottom-most
+	// invariant on exactly the one path that would otherwise violate it.
+	// Every other split target is a strand, and inserting below another
+	// strand never touches Selvage's position.
+	argv := []string{"split-window"}
+	if splitTargetID == st.SelvagePaneID {
+		argv = append(argv, "-b")
+	}
+	argv = append(argv, "-t", splitTargetID, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}")
+	out, err := e.tmux.output(argv...)
 	if err != nil {
 		return fmt.Errorf("split window: %w", err)
 	}
diff --git a/internal/reedengine/spawn_test.go b/internal/reedengine/spawn_test.go
index dab016de8..46afe8ad1 100644
--- a/internal/reedengine/spawn_test.go
+++ b/internal/reedengine/spawn_test.go
@@ -1,7 +1,7 @@
 // spawn_test.go table-tests planPaneTarget's split-target policy — prefer the tallest alive
-// non-header pane, fall back to any present non-header pane (a corpse) when none is alive, and fall
-// back to the header itself as a last resort when no non-header pane exists at all — and the
-// header's exclusion from being the PREFERRED split target (it is never chosen while any non-header
+// non-Selvage pane, fall back to any present non-Selvage pane (a corpse) when none is alive, and fall
+// back to Selvage itself as a last resort when no non-Selvage pane exists at all — and Selvage's
+// exclusion from being the PREFERRED split target (it is never chosen while any non-Selvage
 // pane, alive or dead, is present) — and verifies loadOrInitStateLocked's fresh-worktree bootstrap.
 // Both are pure/hermetic, no live tmux required.
 // TestLaunchStrandLocked_* below invokes launchStrandLocked directly through the e.tmux.execHook
@@ -18,7 +18,7 @@ func TestPlanPaneTarget(t *testing.T) {
 	tests := []struct {
 		name            string
 		live            []LivePane
-		headerPaneID    string
+		selvagePaneID   string
 		wantSplitTarget string
 		wantErr         bool
 	}{
@@ -26,7 +26,7 @@ func TestPlanPaneTarget(t *testing.T) {
 			// Collapses the old FreshSession_AdoptsTheAliveInitialPane and
 			// AllStrandsPaneless_AdoptsFirstAlivePane cases, which differed
 			// only in the (now-deleted) strand table they supplied: a sole
-			// alive pane and no header both reduce to the same input once
+			// alive pane and no Selvage both reduce to the same input once
 			// the strand table stops mattering.
 			name:            "FreshSession_SplitsTheAliveInitialPane",
 			live:            []LivePane{{ID: "%1", Height: 50}},
@@ -45,7 +45,7 @@ func TestPlanPaneTarget(t *testing.T) {
 			// Collapses the old OneStrandHoldsAPane_SplitsTheTallestAlive and
 			// TinyActiveBand_SplitTargetsTheTallestNotTheFirst cases, which
 			// differed only in the (now-deleted) strand table they supplied:
-			// a 2-row pane beside a 47-row pane and no header, in both.
+			// a 2-row pane beside a 47-row pane and no Selvage, in both.
 			name: "TinyActiveBand_SplitTargetsTheTallestNotTheFirst",
 			// The session-target split defect this planner replaces: tmux
 			// splits the active pane, which select-layout can leave on a
@@ -65,20 +65,20 @@ func TestPlanPaneTarget(t *testing.T) {
 			wantErr: true,
 		},
 		{
-			name: "HeaderPresentNoStrandBound_NonHeaderPaneIsTheSplitTarget",
-			// A live header pane plus an alive non-header pane: the split
-			// target must land on the non-header pane, never the header.
-			live:            []LivePane{{ID: "%header", Height: 1}, {ID: "%1", Height: 50}},
-			headerPaneID:    "%header",
+			name: "SelvagePresentNoStrandBound_NonSelvagePaneIsTheSplitTarget",
+			// A live Selvage pane plus an alive non-Selvage pane: the split
+			// target must land on the non-Selvage pane, never Selvage.
+			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%1", Height: 50}},
+			selvagePaneID:   "%selvage",
 			wantSplitTarget: "%1",
 		},
 		{
-			name: "HeaderPresentWithStrand_HeaderNeverTheSplitTarget",
-			// The header is tallest by raw Height here, but must still
-			// never be chosen over a genuine (if shorter) non-header
+			name: "SelvagePresentWithStrand_SelvageNeverTheSplitTarget",
+			// Selvage is tallest by raw Height here, but must still
+			// never be chosen over a genuine (if shorter) non-Selvage
 			// candidate.
-			live:            []LivePane{{ID: "%header", Height: 90}, {ID: "%1", Height: 10}},
-			headerPaneID:    "%header",
+			live:            []LivePane{{ID: "%selvage", Height: 90}, {ID: "%1", Height: 10}},
+			selvagePaneID:   "%selvage",
 			wantSplitTarget: "%1",
 		},
 		{
@@ -87,44 +87,45 @@ func TestPlanPaneTarget(t *testing.T) {
 			// seam was removed: after .lyx/reed.json was scrubbed from a
 			// running session, no strand held a binding and several
 			// untracked alive panes remained — one of them the previous
-			// header pane, still running "lyx reed header --blocking".
-			// Adoption picked it, send-keys typed the strand's command onto
+			// header pane, still running "lyx reed header --blocking" (the
+			// pre-Selvage keepalive this batch removes). Adoption picked
+			// it, send-keys typed the strand's command onto
 			// a blocked pane's screen where it never executed (exit 0
 			// throughout), and status then reported the strand live with no
 			// such process on the box. With more than one candidate there
 			// was no way to tell an idle shell from a busy one, so the
 			// planner always splits a guaranteed-idle new pane instead — off
 			// the tallest, %2 here.
-			live:            []LivePane{{ID: "%header", Height: 1}, {ID: "%stale", Height: 12}, {ID: "%2", Height: 37}},
-			headerPaneID:    "%header",
+			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%stale", Height: 12}, {ID: "%2", Height: 37}},
+			selvagePaneID:   "%selvage",
 			wantSplitTarget: "%2",
 		},
 		{
-			name: "SeveralAlivePanesButOnlyOneNonHeaderAlive_StillSplits",
-			// A fresh boot's header plus the sole new-session pane, with a
-			// dead corpse also present. Exactly one alive non-header pane —
+			name: "SeveralAlivePanesButOnlyOneNonSelvageAlive_StillSplits",
+			// A fresh boot's Selvage plus the sole new-session pane, with a
+			// dead corpse also present. Exactly one alive non-Selvage pane —
 			// it is the split target regardless.
-			live:            []LivePane{{ID: "%header", Height: 1}, {ID: "%corpse", Dead: true, Height: 12}, {ID: "%1", Height: 37}},
-			headerPaneID:    "%header",
+			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%corpse", Dead: true, Height: 12}, {ID: "%1", Height: 37}},
+			selvagePaneID:   "%selvage",
 			wantSplitTarget: "%1",
 		},
 		{
-			name: "HeaderIsSolePane_SplitTargetFallsBackToHeader",
-			// Every strand has been removed: only the header remains. The
-			// header must become the split target so a subsequent add still
-			// has something to split (the header survives the split).
-			live:            []LivePane{{ID: "%header", Height: 21}},
-			headerPaneID:    "%header",
-			wantSplitTarget: "%header",
+			name: "SelvageIsSolePane_SplitTargetFallsBackToSelvage",
+			// Every strand has been removed: only Selvage remains. Selvage
+			// must become the split target so a subsequent add still
+			// has something to split (Selvage survives the split).
+			live:            []LivePane{{ID: "%selvage", Height: 21}},
+			selvagePaneID:   "%selvage",
+			wantSplitTarget: "%selvage",
 		},
 	}
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			splitTarget, err := planPaneTarget(tt.live, tt.headerPaneID)
+			splitTarget, err := planPaneTarget(tt.live, tt.selvagePaneID)
 			if tt.wantErr {
 				if err == nil {
-					t.Fatalf("planPaneTarget(%+v, %q): expected error, got nil", tt.live, tt.headerPaneID)
+					t.Fatalf("planPaneTarget(%+v, %q): expected error, got nil", tt.live, tt.selvagePaneID)
 				}
 				return
 			}
@@ -133,7 +134,7 @@ func TestPlanPaneTarget(t *testing.T) {
 			}
 			if splitTarget != tt.wantSplitTarget {
 				t.Errorf("planPaneTarget(%+v, %q) = %q, want %q",
-					tt.live, tt.headerPaneID, splitTarget, tt.wantSplitTarget)
+					tt.live, tt.selvagePaneID, splitTarget, tt.wantSplitTarget)
 			}
 		})
 	}
@@ -143,17 +144,17 @@ func TestPlanPaneTarget(t *testing.T) {
 // chokepoint at the unit tier: launchStrandLocked must reconcile before it plans a split target, so
 // an untracked alive pane is never eligible to become the split target and is instead reaped first.
 //
-// The fixture is a ReedState with an alive header pane, zero strands bound to a present pane, and one
+// The fixture is a ReedState with an alive Selvage pane, zero strands bound to a present pane, and one
 // untracked alive pane; the strand being launched has PaneID == "", mirroring how addStrandLocked
-// appends a fresh strand before calling launchStrandLocked. The alive header — not any strand
-// binding — is what authorizes the untracked reap here (see reconcile.go's headerAlive disjunct).
+// appends a fresh strand before calling launchStrandLocked. The alive Selvage — not any strand
+// binding — is what authorizes the untracked reap here (see reconcile.go's selvageAlive disjunct).
 func TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget(t *testing.T) {
 	e := newTestEngine(t)
 
-	const headerPaneID = "%header"
+	const selvagePaneID = "%selvage"
 	const untrackedPaneID = "%untracked"
-	preReap := headerPaneID + " 0 0 100 3 4321\n" + untrackedPaneID + " 0 3 100 20 4322\n"
-	postReap := headerPaneID + " 0 0 100 3 4321\n"
+	preReap := selvagePaneID + " 0 0 100 3 4321\n" + untrackedPaneID + " 0 3 100 20 4322\n"
+	postReap := selvagePaneID + " 0 0 100 3 4321\n"
 
 	var verbs []string
 	var listPanesCalls int
@@ -175,7 +176,7 @@ func TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget(t *tes
 		}
 	}
 
-	st := &ReedState{HeaderPaneID: headerPaneID}
+	st := &ReedState{SelvagePaneID: selvagePaneID}
 	st.Strands = append(st.Strands, Strand{GUID: "new"})
 	s := &st.Strands[0]
 
@@ -232,14 +233,14 @@ func TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget(t *tes
 // TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget: when reconcile kills nothing,
 // launchStrandLocked must not pay for a second list-panes round trip it does not need.
 //
-// The fixture has nothing to reap: an alive header plus a strand already bound to a present alive
+// The fixture has nothing to reap: an alive Selvage plus a strand already bound to a present alive
 // pane. The strand being launched is a second one, again with PaneID == "".
 func TestLaunchStrandLocked_SkipsTheRedundantReEnumerationWhenNothingIsReaped(t *testing.T) {
 	e := newTestEngine(t)
 
-	const headerPaneID = "%header"
+	const selvagePaneID = "%selvage"
 	const boundPaneID = "%bound"
-	live := headerPaneID + " 0 0 100 3 4321\n" + boundPaneID + " 0 3 100 20 4322\n"
+	live := selvagePaneID + " 0 0 100 3 4321\n" + boundPaneID + " 0 3 100 20 4322\n"
 
 	var verbs []string
 	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
@@ -254,7 +255,7 @@ func TestLaunchStrandLocked_SkipsTheRedundantReEnumerationWhenNothingIsReaped(t
 		}
 	}
 
-	st := &ReedState{HeaderPaneID: headerPaneID}
+	st := &ReedState{SelvagePaneID: selvagePaneID}
 	st.Strands = append(st.Strands, Strand{GUID: "bound", PaneID: boundPaneID}, Strand{GUID: "new"})
 	s := &st.Strands[1]
 
@@ -364,7 +365,7 @@ func TestSendKeysLiteralArg(t *testing.T) {
 }
 
 // TestValidateSplitCreatedNewPane pins the genuinely-new-pane guard both split sites
-// (launchStrandLocked, ensureHeaderPaneLocked) share: psmux's silent too-small-to-split failure
+// (launchStrandLocked, ensureSelvagePaneLocked) share: psmux's silent too-small-to-split failure
 // exits 0 and prints an EXISTING pane's id, and trusting it would bind two owners to one pane — a
 // duplicate pane number in the next select-layout string, which destroys the session's panes
 // wholesale.
@@ -404,13 +405,13 @@ func TestValidateSplitCreatedNewPane(t *testing.T) {
 // PaneID names a pane another owner already claims is cleared and reported not-live; without it, that
 // pane IS alive, so status reports live:true against someone else's pane — exactly the false-healthy
 // symptom the R5 review reproduced live ("status reported the strand live:true against the header
-// pane running `lyx reed header --blocking`").
+// pane running `lyx reed header --blocking`", the pre-Selvage keepalive this batch removes).
 //
 // The recorded generation deliberately MATCHES the one the probe answers, so the pane-generation
 // guard adopts rather than clears and the only thing that can clear a binding here is the repair
 // under test.
 func TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims(t *testing.T) {
-	const headerPane = "%1"
+	const selvagePane = "%1"
 	const firstStrandPane = "%2"
 	const liveAnswer = "$0|4321|1787000000"
 	liveGeneration := PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "4321", Created: "1787000000"}
@@ -423,8 +424,8 @@ func TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims(t *testing.T) {
 		wantLive      []bool
 	}{
 		{
-			name:          "a strand bound to the header's own pane is not reported live on it",
-			strandPaneIDs: []string{headerPane},
+			name:          "a strand bound to Selvage's own pane is not reported live on it",
+			strandPaneIDs: []string{selvagePane},
 			wantLive:      []bool{false},
 		},
 		{
@@ -451,13 +452,13 @@ func TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims(t *testing.T) {
 				case "list-panes":
 					// Both panes present and alive, so a binding that survives the repair reads as
 					// live and one that does not reads as not-live.
-					return headerPane + " 0 0 100 3 4322\n" + firstStrandPane + " 0 3 100 20 4323\n", nil
+					return selvagePane + " 0 0 100 3 4322\n" + firstStrandPane + " 0 3 100 20 4323\n", nil
 				default:
 					return "", nil
 				}
 			}
 
-			st := &ReedState{HeaderPaneID: headerPane, PaneGeneration: liveGeneration}
+			st := &ReedState{SelvagePaneID: selvagePane, PaneGeneration: liveGeneration}
 			names := []string{"first", "second"}
 			for i, paneID := range tt.strandPaneIDs {
 				st.Strands = append(st.Strands, Strand{GUID: names[i], Name: names[i], PaneID: paneID})
diff --git a/internal/reedengine/state.go b/internal/reedengine/state.go
index 870474e2c..6a2e91df5 100644
--- a/internal/reedengine/state.go
+++ b/internal/reedengine/state.go
@@ -38,17 +38,17 @@ type ReedState struct {
 	Session     string   `json:"session"`
 	StrippedEnv []string `json:"strippedEnv"`
 	Strands     []Strand `json:"strands"`
-	// HeaderPaneID is the tmux pane id of the always-present header pane —
-	// deliberately outside Strands, since the header is a first-class but
+	// SelvagePaneID is the tmux pane id of the always-present Selvage pane —
+	// deliberately outside Strands, since Selvage is a first-class but
 	// separate construct and never itself a strand (Shared Decision
 	// header-is-not-a-strand): it is excluded from strand accounting, from
 	// being the preferred split target, and from both halves of reconcile's
-	// kill schedule. Empty means the header pane has not yet been created (a
-	// fresh worktree, or a server rebirth that cleared every binding) and
-	// must be (re)created at the next up/resume boot.
-	HeaderPaneID string `json:"headerPaneId,omitempty"`
+	// kill schedule. Empty means Selvage has not yet been created (a fresh
+	// worktree, or a server rebirth that cleared every binding) and must be
+	// (re)created at the next up/resume boot.
+	SelvagePaneID string `json:"selvagePaneId,omitempty"`
 	// PaneGeneration identifies the tmux session incarnation every PaneID
-	// above — the strands' and HeaderPaneID alike — was bound against. It is
+	// above — the strands' and SelvagePaneID alike — was bound against. It is
 	// the one field in this struct reed reads back semantically rather than
 	// carrying for its caller: without it a persisted pane id cannot be told
 	// apart from a live pane belonging to something else, because tmux pane
@@ -152,7 +152,7 @@ func LoadState(dotLyxDir string) (*ReedState, error) {
 // Both remedies are named because they are genuinely different trades, and the operator — not
 // reed — has to pick. Deleting the file keeps the session: the panes and their processes keep
 // running and can be attached to, but only until the next mutating verb (up, resume, add, or
-// remove) reaps them, since an alive header now authorizes reaping every other pane the moment
+// remove) reaps them, since an alive Selvage now authorizes reaping every other pane the moment
 // one of those verbs reconciles.
 //
 // Repairing the file automatically is deliberately not offered: every repair reed could perform
diff --git a/internal/reedengine/status-line.md b/internal/reedengine/status-line.md
new file mode 100644
index 000000000..ff040802c
--- /dev/null
+++ b/internal/reedengine/status-line.md
@@ -0,0 +1,6 @@
+<!-- status-line.md is the default status-line text template, rendered via
+     tokenvocab.Render (internal/tokenvocab) into the tmux status-line.
+     This leading banner comment is stripped by stencil.Fill before parsing, so it documents the template for a human reader only.
+     Available top-level tokens (see internal/tokenvocab's registry): {{.repo}} (the repo name), {{.worktree}} (the worktree name), and {{.hub}} (the hub's absolute directory path).
+     A Config.StatusLine.Template override replaces this whole asset;
+     it is not merged with it. --> {{.repo}}/{{.worktree}} · {{.hub}}
diff --git a/internal/reedengine/statusline.go b/internal/reedengine/statusline.go
new file mode 100644
index 000000000..04536e3bb
--- /dev/null
+++ b/internal/reedengine/statusline.go
@@ -0,0 +1,31 @@
+// statusline.go implements Engine.StatusLineText and Engine.ValidateStatusLine: the status-line's
+// text-rendering pipeline over internal/tokenvocab,
+// and the eager, loud validation hook the boot path (batch 4) runs before the session comes up.
+// ValidateStatusLine is now more load-bearing rather than less: a template that fails to render
+// would otherwise reach "set-option status-left", where every failure is non-fatal and merely
+// logged.
+
+package reedengine
+
+import "github.com/Knatte18/loomyard/internal/tokenvocab"
+
+// StatusLineText renders this hub's tmux status-line text.
+func (e *Engine) StatusLineText() (string, error) {
+	template := []byte(e.cfg.StatusLine.Template)
+	if len(template) == 0 {
+		template = StatusLineTemplate()
+	}
+
+	ctx := tokenvocab.Ctx{RepoName: e.geom.RepoName, HubPath: e.geom.HubPath, WorktreeName: e.geom.WorktreeName}
+	rendered, err := tokenvocab.Render(template, ctx)
+	if err != nil {
+		return "", err
+	}
+	return string(rendered), nil
+}
+
+// ValidateStatusLine reports whether this hub's configured status-line template renders cleanly.
+func (e *Engine) ValidateStatusLine() error {
+	_, err := e.StatusLineText()
+	return err
+}
diff --git a/internal/reedengine/statusline_test.go b/internal/reedengine/statusline_test.go
new file mode 100644
index 000000000..252b114bb
--- /dev/null
+++ b/internal/reedengine/statusline_test.go
@@ -0,0 +1,83 @@
+// statusline_test.go covers StatusLineText and ValidateStatusLine hermetically: an Engine built from
+// Config/Geometry struct literals (no lyxcwd.Resolve, no tmux spawn), per the Test Tier
+// Purity Invariant.
+
+package reedengine
+
+import (
+	"strings"
+	"testing"
+)
+
+// newStatusLineTestEngine builds a test Engine with the given status-line template.
+func newStatusLineTestEngine(template string) *Engine {
+	geom := Geometry{RepoName: "test-repo", WorktreeName: "test-worktree", HubPath: "test-hub"}
+	cfg := Config{
+		StatusLine: StatusLineConfig{Template: template},
+	}
+	return New(cfg, geom)
+}
+
+func TestStatusLineText_EmptyTemplateRendersEmbeddedDefault(t *testing.T) {
+	e := newStatusLineTestEngine("")
+
+	got, err := e.StatusLineText()
+	if err != nil {
+		t.Fatalf("StatusLineText() unexpected error: %v", err)
+	}
+
+	want := e.geom.RepoName + "/" + e.geom.WorktreeName + " · " + e.geom.HubPath
+	if strings.TrimSpace(got) != want {
+		t.Errorf("StatusLineText() = %q; want %q", strings.TrimSpace(got), want)
+	}
+}
+
+func TestStatusLineText_ConfiguredTemplateRendersFromConfig(t *testing.T) {
+	e := newStatusLineTestEngine("repo: {{.repo}}")
+
+	got, err := e.StatusLineText()
+	if err != nil {
+		t.Fatalf("StatusLineText() unexpected error: %v", err)
+	}
+
+	want := "repo: " + e.geom.RepoName
+	if got != want {
+		t.Errorf("StatusLineText() = %q; want %q", got, want)
+	}
+}
+
+// TestStatusLineText_ConfiguredTemplateRendersAllThreeTokens pins that the rendered default
+// template carries all three token values: a Geometry with distinct RepoName, WorktreeName and
+// HubPath must render a string containing each of the three.
+func TestStatusLineText_ConfiguredTemplateRendersAllThreeTokens(t *testing.T) {
+	geom := Geometry{RepoName: "distinct-repo", WorktreeName: "distinct-worktree", HubPath: "distinct-hub"}
+	cfg := Config{}
+	e := New(cfg, geom)
+
+	got, err := e.StatusLineText()
+	if err != nil {
+		t.Fatalf("StatusLineText() unexpected error: %v", err)
+	}
+
+	for _, want := range []string{geom.RepoName, geom.WorktreeName, geom.HubPath} {
+		if !strings.Contains(got, want) {
+			t.Errorf("StatusLineText() = %q; want it to contain %q", got, want)
+		}
+	}
+}
+
+func TestValidateStatusLine_UnknownTopLevelTokenErrors(t *testing.T) {
+	e := newStatusLineTestEngine("{{.slug}}")
+
+	if err := e.ValidateStatusLine(); err == nil {
+		t.Error("ValidateStatusLine() = nil; want an error for an unknown top-level token")
+	}
+}
+
+func TestValidateStatusLine_GoodTemplateReturnsNil(t *testing.T) {
+	e := newStatusLineTestEngine("repo: {{.repo}}")
+
+	if err := e.ValidateStatusLine(); err != nil {
+		t.Errorf("ValidateStatusLine() = %v; want nil", err)
+	}
+}
diff --git a/internal/reedengine/statuslinetemplate.go b/internal/reedengine/statuslinetemplate.go
new file mode 100644
index 000000000..34868c6ec
--- /dev/null
+++ b/internal/reedengine/statuslinetemplate.go
@@ -0,0 +1,18 @@
+// statuslinetemplate.go embeds the default status-line text template asset, status-line.md.
+// The asset is rendered via tokenvocab.Render (internal/tokenvocab/render.go:12),
+// which is itself a thin wrapper over stencil.Fill.
+// This asset is deliberately outside the stencil mechanism: it is a tmux status-line display
+// banner, not a producer prompt, so it stays embedded here and is never seeded, stamped, or read
+// from the hub's stencils directory.
+
+package reedengine
+
+import _ "embed"
+
+//go:embed status-line.md
+var statusLineTemplate []byte
+
+// StatusLineTemplate returns the embedded default status-line text template's raw bytes.
+func StatusLineTemplate() []byte {
+	return statusLineTemplate
+}
diff --git a/internal/reedengine/strand_test.go b/internal/reedengine/strand_test.go
index 78afbcf44..4f7cb135c 100644
--- a/internal/reedengine/strand_test.go
+++ b/internal/reedengine/strand_test.go
@@ -709,7 +709,7 @@ func TestAlivePanePIDs(t *testing.T) {
 // descendant-closure roots from EVERY pane the session listed, dead ones included, while
 // RemoveStrand correctly filtered to alive panes only. tmux keeps reporting a dead pane's recorded
 // #{pane_pid} indefinitely (remain-on-exit is on for every reed session, and reconcile deliberately
-// KEEPS the last dead pane and any dead header corpse), so once the OS recycled that pid, Down would
+// KEEPS the last dead pane and any dead Selvage corpse), so once the OS recycled that pid, Down would
 // expand an unrelated process's whole subtree, block on it for the full reapExitTimeout, and then
 // SIGKILL it.
 // The dead-pane row is the assertion that matters; the pid-less row pins that the two forms share
@@ -895,8 +895,8 @@ func TestRemoveStrand_NeverKillsAPaneOutsideThisSession(t *testing.T) {
 	}
 
 	st := &ReedState{
-		HeaderPaneID: thisSessionPane,
-		Strands:      []Strand{{GUID: "copied", Name: "copied", PaneID: siblingPane, Display: render.Display{Anchor: render.AnchorBelowParent}}},
+		SelvagePaneID: thisSessionPane,
+		Strands:       []Strand{{GUID: "copied", Name: "copied", PaneID: siblingPane, Display: render.Display{Anchor: render.AnchorBelowParent}}},
 	}
 	if err := SaveState(e.stateDir(), st); err != nil {
 		t.Fatalf("SaveState: %v", err)
diff --git a/internal/reedengine/template.go b/internal/reedengine/template.go
index 6c0c5eb3d..739191f66 100644
--- a/internal/reedengine/template.go
+++ b/internal/reedengine/template.go
@@ -13,7 +13,7 @@ package reedengine
 // preserving defaults when not set: the two machine tool paths (tmux, shell) plus debug_log, mouse,
 // and watchdog.
 // The layout-tuning keys (width, height, collapsed_strip_rows, min_full_rows, strand_name) and the
-// header block are plain literals.
+// status_line and selvage blocks are plain literals.
 // No provider tool is named here: reed stays provider-invariant per the Shuttle Provider-Seam
 // Invariant, so a claude path belongs to shuttle's template, never this one.
 // On Windows the tmux/shell defaults are the machine's pinned psmux.exe/pwsh.exe paths;
diff --git a/internal/reedengine/template_posix.yaml b/internal/reedengine/template_posix.yaml
index 281d0a6b7..94ee0aba6 100644
--- a/internal/reedengine/template_posix.yaml
+++ b/internal/reedengine/template_posix.yaml
@@ -7,7 +7,8 @@ min_full_rows: 3  # floor height the clamp rule tries to preserve for a full pan
 strand_name: '<ROLE>:<ROUND>:<SHORT_GUID>'  # template for a strand's display name; unfilled tokens resolve to ""
 debug_log: ${env:LYX_REED_DEBUG:-0}  # opt-in tmux server verbose logging (0 off, 1 -v, 2 -vv); takes effect only on the boot that spawns the shared per-hub server; existing hubs must run "lyx config reconcile" to adopt this key
 mouse: ${env:LYX_REED_MOUSE:-on}  # tmux mouse-mode default (on/off; on is the default so the wheel scrolls the pane's scrollback and a click selects a pane — with mouse off tmux never claims the wheel, so the terminal's own alternate-screen translation delivers arrow keys straight into the live agent's input instead of scrolling anything; the cost of on is that native terminal text selection needs the terminal's shift-bypass (hold Shift while dragging), and tmux copy-mode is the in-band alternative; takes effect on a fresh reed server boot only — a live toggle needs a server restart, and an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value)
-watchdog: ${env:LYX_REED_WATCHDOG:-on}  # header pane resize self-heal (on/off; enables the header pane's resize watch loop and the session's window-resized hook; an invalid value fails "lyx reed up" loudly; takes effect on the next header-pane rebuild only — a server restart, a dead-header heal, or "lyx reed down" + "up" — so flipping it does not stop an already-running watcher, and an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value)
-header:  # the always-on operator console pane's rendered text
+watchdog: ${env:LYX_REED_WATCHDOG:-on}  # per-worktree resize self-heal gate (on/off; enables this worktree's resize watch loop and the session's window-resized hook; an invalid value fails "lyx reed up" loudly; the detached per-hub watchdog daemon re-reads a worktree's value when that worktree's session re-enters the watched set, so a "lyx reed down" + "up" is what makes a flipped value take effect, and an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value)
+status_line:  # the tmux status-line's rendered text
   template: ""  # empty means "use the embedded default template"; set to override
-  height_rows: 1  # fixed row count the header pane occupies
+selvage:
+  height_rows: 1  # fixed row count the always-on Selvage pane occupies at the bottom of the window
diff --git a/internal/reedengine/template_windows.yaml b/internal/reedengine/template_windows.yaml
index cfa4d2558..0ad72d505 100644
--- a/internal/reedengine/template_windows.yaml
+++ b/internal/reedengine/template_windows.yaml
@@ -7,7 +7,8 @@ min_full_rows: 3  # floor height the clamp rule tries to preserve for a full pan
 strand_name: '<ROLE>:<ROUND>:<SHORT_GUID>'  # template for a strand's display name; unfilled tokens resolve to ""
 debug_log: ${env:LYX_REED_DEBUG:-0}  # opt-in tmux server verbose logging (0 off, 1 -v, 2 -vv); takes effect only on the boot that spawns the shared per-hub server; existing hubs must run "lyx config reconcile" to adopt this key
 mouse: ${env:LYX_REED_MOUSE:-on}  # tmux mouse-mode default (on/off; on is the default so the wheel scrolls the pane's scrollback and a click selects a pane — with mouse off tmux never claims the wheel, so the terminal's own alternate-screen translation delivers arrow keys straight into the live agent's input instead of scrolling anything; the cost of on is that native terminal text selection needs the terminal's shift-bypass (hold Shift while dragging), and tmux copy-mode is the in-band alternative; takes effect on a fresh reed server boot only — a live toggle needs a server restart, and an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value)
-watchdog: ${env:LYX_REED_WATCHDOG:-on}  # header pane resize self-heal (on/off; enables the header pane's resize watch loop and the session's window-resized hook; an invalid value fails "lyx reed up" loudly; takes effect on the next header-pane rebuild only — a server restart, a dead-header heal, or "lyx reed down" + "up" — so flipping it does not stop an already-running watcher, and an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value)
-header:  # the always-on operator console pane's rendered text
+watchdog: ${env:LYX_REED_WATCHDOG:-on}  # per-worktree resize self-heal gate (on/off; enables this worktree's resize watch loop and the session's window-resized hook; an invalid value fails "lyx reed up" loudly; the detached per-hub watchdog daemon re-reads a worktree's value when that worktree's session re-enters the watched set, so a "lyx reed down" + "up" is what makes a flipped value take effect, and an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value)
+status_line:  # the tmux status-line's rendered text
   template: ""  # empty means "use the embedded default template"; set to override
-  height_rows: 1  # fixed row count the header pane occupies
+selvage:
+  height_rows: 1  # fixed row count the always-on Selvage pane occupies at the bottom of the window
diff --git a/internal/reedengine/testmain_test.go b/internal/reedengine/testmain_test.go
deleted file mode 100644
index 4245e69fc..000000000
--- a/internal/reedengine/testmain_test.go
+++ /dev/null
@@ -1,33 +0,0 @@
-// testmain_test.go guards this package's test binary against being run AS lyx:
-// ensureHeaderPaneLocked boots the header pane with os.Executable() + " reed header --blocking",
-// and inside an in-process integration test that executable is THIS test binary.
-// A Go test binary invoked with positional args ignores them and runs its whole test suite —
-// recursively, inside the pane, each recursive test booting its own tmux servers that leak when the
-// outer test tears the pane down mid-run (found by fable-header-r1: the machine's accumulated stray
-// tmux servers all carried recursive-fixture /tmp paths).
-// The guard makes the test binary honor the header keepalive contract instead: print a marker, hold
-// the pane open, stay killable.
-
-package reedengine
-
-import (
-	"fmt"
-	"os"
-	"testing"
-	"time"
-)
-
-// TestMain intercepts the header-pane invocation shape ("<binary> reed ...")
-// before any test runs: it stands in for `lyx reed header --blocking` by printing a marker and
-// blocking forever (a sleep loop rather than a bare select{}, which the runtime's deadlock detector
-// could kill in a binary with no other live goroutine).
-// Every other invocation delegates to the normal test run.
-func TestMain(m *testing.M) {
-	if len(os.Args) > 1 && os.Args[1] == "reed" {
-		fmt.Println("reedengine test binary standing in for the header keepalive (`lyx reed header --blocking`)")
-		for {
-			time.Sleep(time.Hour)
-		}
-	}
-	os.Exit(m.Run())
-}
diff --git a/internal/reedengine/watchdog_integration_test.go b/internal/reedengine/watchdog_integration_test.go
index 5c489e913..9a2635d15 100644
--- a/internal/reedengine/watchdog_integration_test.go
+++ b/internal/reedengine/watchdog_integration_test.go
@@ -57,7 +57,7 @@ func attachWatchdogClient(t *testing.T, e *Engine, cols, rows int) *attachGeomet
 	return pty
 }
 
-// bootWatchdogFixture boots setupAttachGeometryFixture's two-strand session (a header pane, a
+// bootWatchdogFixture boots setupAttachGeometryFixture's two-strand session (a Selvage pane, a
 // collapsed parent, and a full child), turns the watchdog on, and attaches a pty client at
 // cols x rows.
 func bootWatchdogFixture(t *testing.T, cols, rows int) *watchdogFixture {
@@ -84,13 +84,13 @@ func resizePTY(t *testing.T, pty *attachGeometryPTY, cols, rows int) {
 	}
 }
 
-// headerPaneHeightNow reports the live header pane's current row count and whether it could be
-// determined at all — false while state or the live pane list cannot be read, or the header pane is
+// selvagePaneHeightNow reports the live Selvage pane's current row count and whether it could be
+// determined at all — false while state or the live pane list cannot be read, or the Selvage pane is
 // momentarily absent from either.
-func headerPaneHeightNow(t *testing.T, e *Engine) (height int, ok bool) {
+func selvagePaneHeightNow(t *testing.T, e *Engine) (height int, ok bool) {
 	t.Helper()
 	st, err := LoadState(e.stateDir())
-	if err != nil || st == nil || st.HeaderPaneID == "" {
+	if err != nil || st == nil || st.SelvagePaneID == "" {
 		return 0, false
 	}
 	live, err := e.tmux.listPanes(e.SessionName())
@@ -98,7 +98,7 @@ func headerPaneHeightNow(t *testing.T, e *Engine) (height int, ok bool) {
 		return 0, false
 	}
 	for _, p := range live {
-		if p.ID == st.HeaderPaneID {
+		if p.ID == st.SelvagePaneID {
 			return p.Height, true
 		}
 	}
@@ -140,7 +140,7 @@ func expectedLayoutForCurrentBox(t *testing.T, e *Engine) (layout string, ok boo
 
 // assertLayoutSelfHeals resizes fx's pty client to newCols x newRows and asserts that, within a
 // bounded wait, the live #{window_layout} becomes exactly what the engine plans for the new box and
-// the header pane is back to exactly cfg.Header.HeightRows rows — the M7 assertion, driven in
+// Selvage is back to exactly cfg.Selvage.HeightRows rows — the M7 assertion, driven in
 // whichever direction the caller resizes.
 func assertLayoutSelfHeals(t *testing.T, fx *watchdogFixture, newCols, newRows int) {
 	t.Helper()
@@ -149,15 +149,17 @@ func assertLayoutSelfHeals(t *testing.T, fx *watchdogFixture, newCols, newRows i
 
 	waitUntil(t, 15*time.Second, "layout never self-healed after the resize", func() bool {
 		w, h := windowSizeNow(t, e)
-		if w != newCols || h != newRows {
+		// "status" is pinned on, so tmux's live content window settles at newRows-1, not the client's
+		// told newRows verbatim — the status-line's own row is not part of the window reported here.
+		if w != newCols || h != newRows-1 {
 			return false
 		}
 		wantLayout, ok := expectedLayoutForCurrentBox(t, e)
 		if !ok || windowLayoutNow(t, e) != wantLayout {
 			return false
 		}
-		height, ok := headerPaneHeightNow(t, e)
-		return ok && height == e.cfg.Header.HeightRows
+		height, ok := selvagePaneHeightNow(t, e)
+		return ok && height == e.cfg.Selvage.HeightRows
 	})
 }
 
@@ -229,11 +231,12 @@ func TestWatchdogSelfHeal_BurstCoalesces(t *testing.T) {
 
 	waitUntil(t, 15*time.Second, "burst never converged to the final size's layout", func() bool {
 		w, h := windowSizeNow(t, e)
-		if w != finalCols || h != finalRows {
+		// "status" is pinned on, so the live content window settles at finalRows-1, not finalRows.
+		if w != finalCols || h != finalRows-1 {
 			return false
 		}
-		height, ok := headerPaneHeightNow(t, e)
-		return ok && height == e.cfg.Header.HeightRows
+		height, ok := selvagePaneHeightNow(t, e)
+		return ok && height == e.cfg.Selvage.HeightRows
 	})
 
 	mu.Lock()
@@ -264,11 +267,12 @@ func TestWatchdogSelfHeal_DegradedPathStillConverges(t *testing.T) {
 
 	waitUntil(t, 10*timing.PollCycle+5*time.Second, "poll-mode fallback never healed the layout after the resize", func() bool {
 		w, h := windowSizeNow(t, e)
-		if w != 130 || h != 42 {
+		// "status" is pinned on, so the live content window settles at 42-1=41, not the client's told 42.
+		if w != 130 || h != 41 {
 			return false
 		}
-		height, ok := headerPaneHeightNow(t, e)
-		return ok && height == e.cfg.Header.HeightRows
+		height, ok := selvagePaneHeightNow(t, e)
+		return ok && height == e.cfg.Selvage.HeightRows
 	})
 }
 
@@ -326,8 +330,8 @@ func TestWatchdogSelfHeal_FocusNeverStolen(t *testing.T) {
 
 	resizePTY(t, fx.pty, 150, 44)
 	waitUntil(t, 15*time.Second, "layout never settled after the resize", func() bool {
-		height, ok := headerPaneHeightNow(t, e)
-		return ok && height == e.cfg.Header.HeightRows
+		height, ok := selvagePaneHeightNow(t, e)
+		return ok && height == e.cfg.Selvage.HeightRows
 	})
 
 	if got := activePaneID(t, e); got != childPaneID {
@@ -346,8 +350,8 @@ func TestWatchdogSelfHeal_NoSelfTriggerLoop(t *testing.T) {
 
 	resizePTY(t, fx.pty, 145, 41)
 	waitUntil(t, 15*time.Second, "layout never settled after the resize", func() bool {
-		height, ok := headerPaneHeightNow(t, e)
-		return ok && height == e.cfg.Header.HeightRows
+		height, ok := selvagePaneHeightNow(t, e)
+		return ok && height == e.cfg.Selvage.HeightRows
 	})
 
 	settled := windowLayoutNow(t, e)
diff --git a/internal/reedengine/watchloop.go b/internal/reedengine/watchloop.go
index e61579e01..5ba55859e 100644
--- a/internal/reedengine/watchloop.go
+++ b/internal/reedengine/watchloop.go
@@ -170,9 +170,9 @@ func tickerPeriodFor(mode watchMode, t watchTiming) time.Duration {
 func (e *Engine) watchLoop(ctx context.Context, t watchTiming) error {
 	enabled, err := watchdogOption(e.cfg.Watchdog)
 	if err != nil {
-		// This consumer has no error channel a caller could survive — returning here would let the
-		// header pane's RunE fall through and kill the keepalive — so an invalid value is off, never
-		// fatal.
+		// This consumer has no error channel a caller could survive — returning here would let this
+		// goroutine's own Watch call fall through and stop watching this session entirely — so an
+		// invalid value is off, never fatal.
 		logger.Warn("reed: invalid watchdog value, treating watchdog as off", "socket", e.Socket(), "session", e.SessionName(), "value", e.cfg.Watchdog, "err", err)
 		enabled = false
 	}
@@ -328,8 +328,9 @@ func (e *Engine) handleWatchOutcome(mode watchMode, state *watchState, t watchTi
 		return watchModeSignal
 	}
 	// Signal mode never demotes and never re-probes: watchdog: off unsets the hook while an
-	// already-running signal-mode watcher keeps going until the next header-pane rebuild, and a
-	// signal-mode watcher with no hook receives no signals and therefore does nothing, which is
-	// exactly what the operator asked for.
+	// already-running signal-mode watcher keeps going until a flipped watchdog: value takes effect —
+	// which happens when the worktree's session re-enters the daemon's watched set (a `down` + `up`)
+	// — and a signal-mode watcher with no hook receives no signals and therefore does nothing, which
+	// is exactly what the operator asked for.
 	return mode
 }
diff --git a/internal/reedengine/windowsize.go b/internal/reedengine/windowsize.go
index b70c871b5..d9bbdc251 100644
--- a/internal/reedengine/windowsize.go
+++ b/internal/reedengine/windowsize.go
@@ -1,6 +1,6 @@
-// windowsize.go owns the live-window-size query and its fallback, the two geometry option pins
-// (status off, window-size latest), the two effective-value readbacks the attach path (batch 2)
-// gates the chain on, and the whole write side of the `window-resized` hook array — both the
+// windowsize.go owns the live-window-size query and its fallback, the geometry option pins (the
+// rendered status-line and window-size latest), the two effective-value readbacks the attach path
+// (batch 2) gates the chain on, and the whole write side of the `window-resized` hook array — both the
 // resize-pane pins and the watchdog's own resize-signal entry, which are one array and are therefore
 // installed by one function (installResizePinsLocked). The array's READ side is reapply.go's
 // hookInstalledLocked.
@@ -18,6 +18,7 @@ import (
 	"runtime"
 	"strconv"
 	"strings"
+	"unicode/utf8"
 
 	"github.com/Knatte18/loomyard/internal/logger"
 	"github.com/Knatte18/loomyard/internal/reedengine/render"
@@ -93,15 +94,58 @@ func windowSizeAllowsChain(raw string) bool {
 	return strings.ToLower(strings.TrimSpace(raw)) == "latest"
 }
 
-// pinGeometryOptionsLocked pins this session's window to "status off" and "window-size latest", and
-// owns the UNSET half of the window-resized hook's lifecycle — the install half belongs to
-// installResizePinsLocked at the bottom of this file, which rebuilds the whole array (pins plus the
-// watchdog's signal entry) from scratch on every successful apply.
-// Both geometry pins are session/window-targeted rather than -g, because a session- or window-scoped
-// value set from the operator's own ~/.tmux.conf silently wins over a global set while set-option
-// still exits 0 — verified live. Each call's error is logged via logger.Warn and then ignored; every
-// later step, including the hook block, is attempted even when an earlier one failed, per the Shared
-// Decision geometry-tmux-failures-are-non-fatal-everywhere.
+// escapeStatusText returns s with every "#" doubled to "##". tmux expands "#{…}" and "#[…]" inside a
+// status string, so a hub path or repo name carrying a "#" would otherwise be interpreted as a format
+// directive rather than displayed verbatim. No I/O.
+func escapeStatusText(s string) string {
+	return strings.ReplaceAll(s, "#", "##")
+}
+
+// statusLeftLength returns the status-left-length value for escaped, an already-escaped status-left
+// string: max(10, utf8.RuneCountInString(escaped)). The count is measured in runes, not bytes, because
+// tmux's status-left-length limit counts characters rather than bytes, and it is floored at 10 because
+// that is tmux's own default, which would truncate a shorter explicit value down to nothing gained.
+//
+// escaped MUST already have passed through escapeStatusText: this order is load-bearing rather than
+// stylistic, because a hub path carrying "#" grows by one character per occurrence once escaped, so
+// measuring the pre-escape string here would truncate exactly the lines that need the escaping most.
+// No I/O.
+func statusLeftLength(escaped string) int {
+	n := utf8.RuneCountInString(escaped)
+	if n < 10 {
+		return 10
+	}
+	return n
+}
+
+// pinGeometryOptionsLocked renders this hub's status-line text into tmux's status-line and pins this
+// session's window to "window-size latest", and owns the UNSET half of the window-resized hook's
+// lifecycle — the install half belongs to installResizePinsLocked at the bottom of this file, which
+// rebuilds the whole array (pins plus the watchdog's signal entry) from scratch on every successful
+// apply.
+//
+// The status-line render is one call to e.StatusLineText(). On error it logs via logger.Warn naming
+// the socket, the session and the error, and skips the two text-derived options (status-left and
+// status-left-length) while still issuing the other five status-line options — a template that fails
+// to render is already refused loudly at boot by ValidateStatusLine, so reaching here means a degraded
+// path, not a normal one. On success it escapes the rendered text with escapeStatusText (tmux expands
+// "#{…}"/"#[…]" inside a status string) and issues, in order: "status" "on"; "status-position"
+// "bottom"; "status-left" <escaped>; "status-right" ""; "status-left-length"
+// <statusLeftLength(escaped)>; and, window-targeted with -w, "window-status-format" "" and
+// "window-status-current-format" "". Suppressing the window-status segment is deliberate rather than
+// left at tmux's default: reed's session has exactly one window, so the default "0:bash*" segment
+// beside the identity text names nothing the operator can act on and would shift position as the
+// window's active pane name changes.
+//
+// Every geometry pin — the status-line options and window-size — is session/window-targeted rather
+// than -g, because a session- or window-scoped value set from the operator's own ~/.tmux.conf silently
+// wins over a global set while set-option still exits 0 — verified live. Each call's error is logged
+// via logger.Warn and then ignored; every later step, including the hook block, is attempted even when
+// an earlier one failed, per the Shared Decision geometry-tmux-failures-are-non-fatal-everywhere.
+//
+// No runtime.GOOS == "windows" branch guards any of the eight set-option calls above: per the Shared
+// Decision windows-status-line-is-an-unbranched-accepted-degrade they are attempted on every platform,
+// and psmux may refuse some or all of them.
 //
 // This function is the right home for the unset because it already runs both at boot (lifecycle.go)
 // and in the attach pre-flight (attach.go), so an operator who flips watchdog: off gets the hook torn
@@ -116,9 +160,42 @@ func windowSizeAllowsChain(raw string) bool {
 // Assumes the op lock is already held.
 func (e *Engine) pinGeometryOptionsLocked() {
 	target := exactSessionWindowTarget(e.SessionName())
-	if err := e.tmux.run("set-option", "-t", target, "status", "off"); err != nil {
-		logger.Warn("reed: failed to pin status off", "socket", e.Socket(), "session", e.SessionName(), "option", "status", "err", err)
+
+	text, textErr := e.StatusLineText()
+	haveText := textErr == nil
+	var escaped string
+	if !haveText {
+		logger.Warn("reed: failed to render status-line text, skipping status-left and status-left-length", "socket", e.Socket(), "session", e.SessionName(), "err", textErr)
+	} else {
+		escaped = escapeStatusText(strings.TrimRight(text, "\r\n"))
 	}
+
+	if err := e.tmux.run("set-option", "-t", target, "status", "on"); err != nil {
+		logger.Warn("reed: failed to pin status on", "socket", e.Socket(), "session", e.SessionName(), "option", "status", "err", err)
+	}
+	if err := e.tmux.run("set-option", "-t", target, "status-position", "bottom"); err != nil {
+		logger.Warn("reed: failed to pin status-position bottom", "socket", e.Socket(), "session", e.SessionName(), "option", "status-position", "err", err)
+	}
+	if haveText {
+		if err := e.tmux.run("set-option", "-t", target, "status-left", escaped); err != nil {
+			logger.Warn("reed: failed to pin status-left", "socket", e.Socket(), "session", e.SessionName(), "option", "status-left", "err", err)
+		}
+	}
+	if err := e.tmux.run("set-option", "-t", target, "status-right", ""); err != nil {
+		logger.Warn("reed: failed to pin status-right empty", "socket", e.Socket(), "session", e.SessionName(), "option", "status-right", "err", err)
+	}
+	if haveText {
+		if err := e.tmux.run("set-option", "-t", target, "status-left-length", strconv.Itoa(statusLeftLength(escaped))); err != nil {
+			logger.Warn("reed: failed to pin status-left-length", "socket", e.Socket(), "session", e.SessionName(), "option", "status-left-length", "err", err)
+		}
+	}
+	if err := e.tmux.run("set-option", "-w", "-t", target, "window-status-format", ""); err != nil {
+		logger.Warn("reed: failed to pin window-status-format empty", "socket", e.Socket(), "session", e.SessionName(), "option", "window-status-format", "err", err)
+	}
+	if err := e.tmux.run("set-option", "-w", "-t", target, "window-status-current-format", ""); err != nil {
+		logger.Warn("reed: failed to pin window-status-current-format empty", "socket", e.Socket(), "session", e.SessionName(), "option", "window-status-current-format", "err", err)
+	}
+
 	if err := e.tmux.run("set-option", "-w", "-t", target, "window-size", "latest"); err != nil {
 		logger.Warn("reed: failed to pin window-size latest", "socket", e.Socket(), "session", e.SessionName(), "option", "window-size", "err", err)
 	}
@@ -219,8 +296,8 @@ func (e *Engine) readWindowSizeLatestLocked() bool {
 // set-hook takes its body as a single argument and a separate ";" element would terminate the
 // set-hook command itself. The array encoding — rather than one ";"-separated command string — exists
 // for failure isolation: verified live on tmux 3.6, a resize-pane naming a destroyed pane aborts the
-// rest of a single command list, while array entries are independent. The header is always pin index
-// 0 so it fires before any strip pin can go wrong.
+// rest of a single command list, while array entries are independent. The Selvage pin is always pin
+// index 0 so it fires before any strip pin can go wrong.
 func resizePinHookArgvs(session string, pins []render.Pin, signalCommand string) [][]string {
 	target := exactSessionWindowTarget(session)
 	argvs := make([][]string, 0, len(pins)+2)
diff --git a/internal/reedengine/windowsize_test.go b/internal/reedengine/windowsize_test.go
index 083a62b1d..93fce1ee7 100644
--- a/internal/reedengine/windowsize_test.go
+++ b/internal/reedengine/windowsize_test.go
@@ -11,6 +11,7 @@ import (
 	"os"
 	"path/filepath"
 	"runtime"
+	"strconv"
 	"strings"
 	"testing"
 
@@ -133,6 +134,66 @@ func TestWindowSizeAllowsChain(t *testing.T) {
 	}
 }
 
+// TestEscapeStatusText covers the pure doubling rule: every "#" becomes "##", regardless of position.
+func TestEscapeStatusText(t *testing.T) {
+	tests := []struct {
+		name string
+		in   string
+		want string
+	}{
+		{"NoHash", "plain text", "plain text"},
+		{"OneHash", "a#b", "a##b"},
+		{"SeveralHashes", "#a#b#c", "##a##b##c"},
+		{"HashAtEachEnd", "#middle#", "##middle##"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			if got := escapeStatusText(tt.in); got != tt.want {
+				t.Errorf("escapeStatusText(%q) = %q, want %q", tt.in, got, tt.want)
+			}
+		})
+	}
+}
+
+// TestStatusLeftLength covers the rune-counted floor-at-10 rule, including a multi-byte string whose
+// rune count differs materially from its byte count — statusLeftLength must report the rune count, not
+// the byte count.
+func TestStatusLeftLength(t *testing.T) {
+	tests := []struct {
+		name string
+		in   string
+		want int
+	}{
+		{"ShorterThanFloor", "short", 10},
+		{"ExactlyTen", "1234567890", 10},
+		{"LongerThanFloor", "this is a long status line", 26},
+		// 12 runes, 36 bytes (3 bytes per hiragana character): a byte-counting implementation would
+		// wrongly answer 36 here.
+		{"MultiByteRuneCountDiffersFromByteCount", "あいうえおかきくけこさし", 12},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			if got := statusLeftLength(tt.in); got != tt.want {
+				t.Errorf("statusLeftLength(%q) = %d, want %d", tt.in, got, tt.want)
+			}
+		})
+	}
+}
+
+// TestStatusLeftLength_EscapeThenMeasureOrderMatters pins the load-bearing order documented on
+// statusLeftLength: measuring a string before escapeStatusText doubles its "#" characters can
+// under-report the length tmux will actually receive. "######" is 6 runes unescaped (floored to 10),
+// but "############" once escaped is 12 runes — over the floor — so escaping first must yield a
+// strictly larger answer.
+func TestStatusLeftLength_EscapeThenMeasureOrderMatters(t *testing.T) {
+	const s = "######"
+	before := statusLeftLength(s)
+	after := statusLeftLength(escapeStatusText(s))
+	if after <= before {
+		t.Errorf("statusLeftLength(escapeStatusText(%q)) = %d, want it to exceed statusLeftLength(%q) = %d — measuring the pre-escape string would truncate a hub path containing '#'", s, after, s, before)
+	}
+}
+
 func TestReadStatusRowsLocked(t *testing.T) {
 	t.Run("ScriptedAnswer", func(t *testing.T) {
 		e := newTestEngine(t)
@@ -185,70 +246,119 @@ func TestReadWindowSizeLatestLocked(t *testing.T) {
 	})
 }
 
-// TestPinGeometryOptionsLocked records every set-option argv the hook receives, asserting both pins
-// are issued session/window-targeted (never -g), and that a first-pin error does not stop the second
-// pin from being issued.
+// TestPinGeometryOptionsLocked drives pinGeometryOptionsLocked against TmuxCmd's execHook seam,
+// recording every set-option argv issued. It asserts the seven status-line options plus the
+// pre-existing window-size pin are all issued with the expected target/value, that status-left carries
+// the escaped rendered text, that a StatusLineText render error skips only status-left and
+// status-left-length while the other six calls (five status-line options plus window-size) still
+// happen, and that no call's failure stops the calls after it.
 func TestPinGeometryOptionsLocked(t *testing.T) {
-	tests := []struct {
-		name        string
-		firstErrors bool
-	}{
-		{"BothSucceed", false},
-		{"FirstPinErrors", true},
-	}
-	for _, tt := range tests {
-		t.Run(tt.name, func(t *testing.T) {
-			e := newTestEngine(t)
-			var calls [][]string
-			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
-				if args[0] != "set-option" {
-					return "", nil
-				}
-				calls = append(calls, append([]string{}, args...))
-				if tt.firstErrors && len(calls) == 1 {
-					return "", errors.New("boom")
-				}
+	t.Run("AllOptionsIssuedWithEscapedText", func(t *testing.T) {
+		e := newTestEngine(t)
+		// newTestEngine's Geometry leaves WorktreeName unset; the default status-line template's
+		// {{.worktree}} marker requires it, so this case sets it so StatusLineText() succeeds.
+		e.geom.WorktreeName = "test-worktree"
+		var calls [][]string
+		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+			if args[0] != "set-option" {
 				return "", nil
 			}
+			calls = append(calls, append([]string{}, args...))
+			return "", nil
+		}
 
-			e.pinGeometryOptionsLocked()
+		wantText, err := e.StatusLineText()
+		if err != nil {
+			t.Fatalf("StatusLineText() unexpected error: %v", err)
+		}
+		wantEscaped := escapeStatusText(strings.TrimRight(wantText, "\r\n"))
 
-			if len(calls) != 2 {
-				t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls, want 2: %v", len(calls), calls)
-			}
+		e.pinGeometryOptionsLocked()
 
-			wantTarget := exactSessionWindowTarget(e.SessionName())
+		target := exactSessionWindowTarget(e.SessionName())
+		wantOptions := [][]string{
+			{"set-option", "-t", target, "status", "on"},
+			{"set-option", "-t", target, "status-position", "bottom"},
+			{"set-option", "-t", target, "status-left", wantEscaped},
+			{"set-option", "-t", target, "status-right", ""},
+			{"set-option", "-t", target, "status-left-length", strconv.Itoa(statusLeftLength(wantEscaped))},
+			{"set-option", "-w", "-t", target, "window-status-format", ""},
+			{"set-option", "-w", "-t", target, "window-status-current-format", ""},
+			{"set-option", "-w", "-t", target, "window-size", "latest"},
+		}
 
-			first := calls[0]
-			if !containsArg(first, "-t") || !containsArg(first, wantTarget) {
-				t.Errorf("first pin args = %v, want it to carry -t %q", first, wantTarget)
+		if len(calls) != len(wantOptions) {
+			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls, want %d: %v", len(calls), len(wantOptions), calls)
+		}
+		for i, want := range wantOptions {
+			if len(calls[i]) != len(want) {
+				t.Fatalf("call[%d] = %v, want %v", i, calls[i], want)
 			}
-			if containsArg(first, "-g") {
-				t.Errorf("first pin args = %v, want no -g", first)
+			for j := range want {
+				if calls[i][j] != want[j] {
+					t.Errorf("call[%d][%d] = %q, want %q (full call %v, want %v)", i, j, calls[i][j], want[j], calls[i], want)
+				}
 			}
-			if !containsArg(first, "status") || !containsArg(first, "off") {
-				t.Errorf("first pin args = %v, want the status off pair", first)
+		}
+	})
+
+	t.Run("StatusLineTextErrorSkipsOnlyTheTwoTextDerivedOptions", func(t *testing.T) {
+		e := newTestEngine(t)
+		// An unknown top-level token forces StatusLineText() to error, the same shape
+		// TestValidateStatusLine_UnknownTopLevelTokenErrors pins.
+		e.cfg.StatusLine.Template = "{{.slug}}"
+		var calls [][]string
+		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+			if args[0] != "set-option" {
+				return "", nil
 			}
+			calls = append(calls, append([]string{}, args...))
+			return "", nil
+		}
 
-			second := calls[1]
-			if !containsArg(second, "-w") {
-				t.Errorf("second pin args = %v, want -w", second)
+		e.pinGeometryOptionsLocked()
+
+		for _, c := range calls {
+			if containsArg(c, "status-left") || containsArg(c, "status-left-length") {
+				t.Errorf("calls = %v, want no status-left or status-left-length call when StatusLineText errors", calls)
 			}
-			if containsArg(second, "-g") {
-				t.Errorf("second pin args = %v, want no -g", second)
+		}
+		const wantCalls = 6 // status, status-position, status-right, window-status-format, window-status-current-format, window-size
+		if len(calls) != wantCalls {
+			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls on a StatusLineText error, want %d: %v", len(calls), wantCalls, calls)
+		}
+	})
+
+	t.Run("OneOptionFailureDoesNotStopTheRest", func(t *testing.T) {
+		e := newTestEngine(t)
+		e.geom.WorktreeName = "test-worktree"
+		var calls [][]string
+		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+			if args[0] != "set-option" {
+				return "", nil
 			}
-			if !containsArg(second, "window-size") || !containsArg(second, "latest") {
-				t.Errorf("second pin args = %v, want the window-size latest pair", second)
+			calls = append(calls, append([]string{}, args...))
+			if len(calls) == 1 {
+				return "", errors.New("boom")
 			}
-		})
-	}
+			return "", nil
+		}
+
+		e.pinGeometryOptionsLocked()
+
+		const wantCalls = 8
+		if len(calls) != wantCalls {
+			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls despite one erroring, want all %d still attempted: %v", len(calls), wantCalls, calls)
+		}
+	})
 }
 
 // TestPinGeometryOptionsLocked_HookLifecycle covers the window-resized hook install/unset lifecycle
-// pinGeometryOptionsLocked now owns, alongside the two pre-existing geometry pins.
+// pinGeometryOptionsLocked now owns, alongside the status-line and window-size geometry pins.
 func TestPinGeometryOptionsLocked_HookLifecycle(t *testing.T) {
 	t.Run("WatchdogOnPinsGeometryOptionsOnly", func(t *testing.T) {
 		e := newTestEngine(t)
+		e.geom.WorktreeName = "test-worktree"
 		e.cfg.Watchdog = "on"
 		var calls [][]string
 		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
@@ -277,8 +387,8 @@ func TestPinGeometryOptionsLocked_HookLifecycle(t *testing.T) {
 				setOptionCalls++
 			}
 		}
-		if setOptionCalls != 2 {
-			t.Errorf("pinGeometryOptionsLocked calls = %v, want 2 set-option calls (status and window-size)", calls)
+		if setOptionCalls != 8 {
+			t.Errorf("pinGeometryOptionsLocked calls = %v, want 8 set-option calls (the seven status-line options and window-size)", calls)
 		}
 	})
 
@@ -338,6 +448,7 @@ func TestPinGeometryOptionsLocked_HookLifecycle(t *testing.T) {
 
 	t.Run("SetHookErrorIsNonFatalWhenWatchdogOff", func(t *testing.T) {
 		e := newTestEngine(t)
+		e.geom.WorktreeName = "test-worktree"
 		e.cfg.Watchdog = "off"
 		var setOptionCalls int
 		var setHookErrors int
@@ -355,8 +466,8 @@ func TestPinGeometryOptionsLocked_HookLifecycle(t *testing.T) {
 
 		e.pinGeometryOptionsLocked()
 
-		if setOptionCalls != 2 {
-			t.Errorf("set-option calls = %d, want 2 (both preceding pins still attempted despite later set-hook error)", setOptionCalls)
+		if setOptionCalls != 8 {
+			t.Errorf("set-option calls = %d, want 8 (all preceding pins still attempted despite the later set-hook error)", setOptionCalls)
 		}
 		if setHookErrors != 1 {
 			t.Errorf("set-hook errors = %d, want 1", setHookErrors)
diff --git a/internal/shedbuild/build_engines_test.go b/internal/shedbuild/build_engines_test.go
index cf462c504..1fe795b08 100644
--- a/internal/shedbuild/build_engines_test.go
+++ b/internal/shedbuild/build_engines_test.go
@@ -14,11 +14,14 @@ import (
 )
 
 // engineMinimalConfig maps an engine name to its minimal Config, covering only the three engines
-// that need one. The other eleven engines take no config at all and are given none in this test,
-// since a non-empty config block on any of them is an error from the constructor.
+// that need one. The other fourteen engines take no config at all and are given none in this test,
+// since a non-empty config block on any of them is an error from the constructor. All three
+// lifecycle engines join that fourteen: WorktreeCreate and WorktreeTeardown take no Config keys at
+// all, and Loom-Run's own two Config keys (poll_interval_s, poll_attempts) both default, so an
+// empty Config block is legal there too.
 //
-// DiscussionWrite and PlanWrite are two of the eleven: each wraps a single-LLM producer behind its
-// own commit decorator, but its Spec arrives as an injected Env closure (DiscussionSpec or
+// DiscussionWrite and PlanWrite are two of the fourteen: each wraps a single-LLM producer behind
+// its own commit decorator, but its Spec arrives as an injected Env closure (DiscussionSpec or
 // PlanSpec) rather than as recipe Config, so it has no config keys of its own and its seams are
 // filled by newTestEnv instead.
 func engineMinimalConfig(stencilName, rubricStencilName string) map[string]map[string]any {
diff --git a/internal/shedbuild/fixture_test.go b/internal/shedbuild/fixture_test.go
index f9448727a..c86e881ec 100644
--- a/internal/shedbuild/fixture_test.go
+++ b/internal/shedbuild/fixture_test.go
@@ -8,6 +8,7 @@
 package shedbuild
 
 import (
+	"context"
 	"os"
 	"path/filepath"
 	"testing"
@@ -15,8 +16,10 @@ import (
 	"github.com/Knatte18/loomyard/internal/burlerengine"
 	"github.com/Knatte18/loomyard/internal/fabricengine"
 	"github.com/Knatte18/loomyard/internal/landingshed"
+	"github.com/Knatte18/loomyard/internal/lifecycleshed"
 	"github.com/Knatte18/loomyard/internal/mergeresolve"
 	"github.com/Knatte18/loomyard/internal/shedadapters"
+	"github.com/Knatte18/loomyard/internal/shedengine"
 	"github.com/Knatte18/loomyard/internal/shedrecipe"
 	"github.com/Knatte18/loomyard/internal/shuttleengine"
 	"github.com/Knatte18/loomyard/internal/stencilstore"
@@ -133,8 +136,9 @@ func testLandingDeps(dir string) landingshed.Deps {
 // does -- a DiscussionSpec closure returning a Spec over one absolute output path under the same
 // temp root, a CommitDiscussion closure returning nil, a PlanSpec closure returning a Spec over one
 // absolute output path under the same temp root, and a CommitPlan closure returning nil -- and
-// additionally fills Env.Landing via testLandingDeps, because two of the fourteen engines need it,
-// which its sibling does not do.
+// additionally fills Env.Landing via testLandingDeps, because two of the seventeen engines need it,
+// which its sibling does not do. It also fills the six lifecycle fields (Slug, ScratchDir,
+// CreateWorktree, LoomRun, Teardown, PrimeLock) the same way that sibling's own newTestEnv does.
 //
 // Every seam any registered engine requires non-nil must be filled here:
 // TestBuild_EveryRegisteredEngineBuilds drives its assertion off shedrecipe.Names(), so a new
@@ -192,6 +196,30 @@ func newTestEnv(t *testing.T) shedrecipe.Env {
 		},
 		CommitPlan: func() error { return nil },
 		Landing:    testLandingDeps(mustMkdir("landing")),
+		Slug:       "test-slug",
+		ScratchDir: mustMkdir("scratch"),
+		CreateWorktree: func(context.Context) error {
+			return nil
+		},
+		LoomRun: lifecycleshed.LoomRunDeps{
+			Spawn: func(context.Context) error { return nil },
+			ResolveStatus: func() (string, string, error) {
+				return filepath.Join(dir, "loomrun-status.json"), filepath.Join(dir, "loomrun-status.json.lock"), nil
+			},
+			ReadStatus: func(string, string) (shedengine.Status, bool, error) {
+				return shedengine.Status{}, false, nil
+			},
+		},
+		Teardown: lifecycleshed.TeardownDeps{
+			Shutdown: func(context.Context) (string, error) { return "", nil },
+			Remove:   func(context.Context) error { return nil },
+		},
+		PrimeLock: lifecycleshed.PrimeLock{
+			Path: filepath.Join(dir, "prime.lock"),
+			Acquire: func() (func() error, bool, error) {
+				return func() error { return nil }, true, nil
+			},
+		},
 	}
 }
 
diff --git a/internal/shedrecipe/coverage_guard_test.go b/internal/shedrecipe/coverage_guard_test.go
new file mode 100644
index 000000000..0c484dd98
--- /dev/null
+++ b/internal/shedrecipe/coverage_guard_test.go
@@ -0,0 +1,80 @@
+// coverage_guard_test.go is the cross-consumer registry coverage guard: it unions every recipe
+// consumer's own RecipeEngines() and asserts every name in shedrecipe.Names() is reached by that
+// union or is explicitly allowlisted. This guard, not any one consumer, is where a new registry
+// key's coverage is now checked -- a consumer holding a full copy of this assertion would fail on
+// the other consumer's engines, the same bug, doubled.
+//
+// It lives in package shedrecipe_test, the external test package, because that is the only place
+// that may import both recipe consumers -- internal/loomrecipe and internal/lifecyclerecipe --
+// without an import cycle: neither consumer package may import the other, and this package's own
+// internal test package cannot import either without producing shedrecipe -> loomrecipe ->
+// shedrecipe (and the lifecyclerecipe equivalent).
+
+package shedrecipe_test
+
+import (
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
+	"github.com/Knatte18/loomyard/internal/loomrecipe"
+	"github.com/Knatte18/loomyard/internal/shedrecipe"
+)
+
+// coverageGuardAllowedUnreachableEngines names the registry engines this guard tolerates as
+// unreferenced by any consumer's RecipeEngines() union. Stub joins this allowlist now that the
+// last stubbed row -- Webster-Review -- is real: no loom row reaches Stub any more, and the engine
+// stays registered because internal/shedrecipe's registry is generic Shed machinery shared by
+// reference with a future product's producer list rather than loom's private property.
+// SingleLLM is the other tolerated entry: the two other "loom: real LLM producers" roadmap items
+// (manifest/roadmap.md) have not yet landed a row that reaches it.
+var coverageGuardAllowedUnreachableEngines = map[string]bool{
+	"SingleLLM": true,
+	"Stub":      true,
+}
+
+// TestCoverageGuard_EveryRegisteredEngineIsReachedOrAllowlisted asserts every name in
+// shedrecipe.Names() is in the union of loomrecipe.RecipeEngines() and
+// lifecyclerecipe.RecipeEngines(), or is on coverageGuardAllowedUnreachableEngines.
+func TestCoverageGuard_EveryRegisteredEngineIsReachedOrAllowlisted(t *testing.T) {
+	reached := make(map[string]bool)
+	for _, engine := range loomrecipe.RecipeEngines() {
+		reached[engine] = true
+	}
+	for _, engine := range lifecyclerecipe.RecipeEngines() {
+		reached[engine] = true
+	}
+
+	for _, name := range shedrecipe.Names() {
+		if reached[name] || coverageGuardAllowedUnreachableEngines[name] {
+			continue
+		}
+		t.Errorf("shedrecipe.Names() has %q, which no consumer's RecipeEngines() reaches and which is not in coverageGuardAllowedUnreachableEngines", name)
+	}
+}
+
+// TestCoverageGuard_AllowlistDoesNotDrift carries this guard's own drift direction: an allowlist
+// entry naming an engine that is no longer registered, or that some consumer now does reach, fails
+// rather than lingering.
+func TestCoverageGuard_AllowlistDoesNotDrift(t *testing.T) {
+	registered := make(map[string]bool)
+	for _, name := range shedrecipe.Names() {
+		registered[name] = true
+	}
+
+	reached := make(map[string]bool)
+	for _, engine := range loomrecipe.RecipeEngines() {
+		reached[engine] = true
+	}
+	for _, engine := range lifecyclerecipe.RecipeEngines() {
+		reached[engine] = true
+	}
+
+	for name := range coverageGuardAllowedUnreachableEngines {
+		if !registered[name] {
+			t.Errorf("coverageGuardAllowedUnreachableEngines names %q, which shedrecipe.Names() no longer registers", name)
+		}
+		if reached[name] {
+			t.Errorf("coverageGuardAllowedUnreachableEngines names %q, which a consumer's RecipeEngines() now reaches -- remove it from the allowlist", name)
+		}
+	}
+}
diff --git a/internal/shedrecipe/entries_lifecycle.go b/internal/shedrecipe/entries_lifecycle.go
new file mode 100644
index 000000000..f79e6ce6f
--- /dev/null
+++ b/internal/shedrecipe/entries_lifecycle.go
@@ -0,0 +1,132 @@
+// entries_lifecycle.go implements the three lifecycle registry entries: worktreeCreateEntry,
+// loomRunEntry, and worktreeTeardownEntry. They are grouped into their own file rather than folded
+// into entries_simple.go because they share the Env.Slug/Env.ScratchDir/Env.PrimeLock validation
+// shape that entries_simple.go's nine entries do not have.
+
+package shedrecipe
+
+import (
+	"fmt"
+	"time"
+
+	"github.com/Knatte18/loomyard/internal/lifecycleshed"
+	"github.com/Knatte18/loomyard/internal/shedengine"
+)
+
+// defaultLoomRunPollIntervalS and defaultLoomRunPollAttempts are loomRunEntry's own defaults for the
+// poll_interval_s and poll_attempts Config keys, used when the extracted value is zero -- configInt
+// reports an absent key and an explicit zero identically, so both resolve to these defaults. The
+// twelve-hour default the attempt count expresses at the default interval is deliberately generous:
+// a real task run spans hours.
+const (
+	defaultLoomRunPollIntervalS = 5
+	defaultLoomRunPollAttempts  = 8640
+)
+
+// worktreeCreateEntry is the Constructor for the "WorktreeCreate" registry row: it validates
+// Env.Slug, Env.ScratchDir, Env.CreateWorktree, Env.PrimeLock.Acquire, and Env.PrimeLock.Path, and
+// returns lifecycleshed.NewWorktreeCreate(name, env.Slug, env.CreateWorktree, env.PrimeLock,
+// env.ScratchDir).
+func worktreeCreateEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
+	if err := configRejectUnknown(cfg); err != nil {
+		return nil, err
+	}
+	if err := requireNonEmpty("WorktreeCreate", "Slug", env.Slug); err != nil {
+		return nil, err
+	}
+	if err := requireAbsRoot("WorktreeCreate", "ScratchDir", env.ScratchDir); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("WorktreeCreate", "CreateWorktree", env.CreateWorktree); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("WorktreeCreate", "PrimeLock.Acquire", env.PrimeLock.Acquire); err != nil {
+		return nil, err
+	}
+	if err := requireAbsRoot("WorktreeCreate", "PrimeLock.Path", env.PrimeLock.Path); err != nil {
+		return nil, err
+	}
+	return lifecycleshed.NewWorktreeCreate(name, env.Slug, env.CreateWorktree, env.PrimeLock, env.ScratchDir), nil
+}
+
+// worktreeTeardownEntry is the Constructor for the "WorktreeTeardown" registry row: worktreeCreateEntry's
+// twin, validating the same Slug/ScratchDir/PrimeLock fields under the "WorktreeTeardown" entry name,
+// plus Env.Teardown.Shutdown and Env.Teardown.Remove, and returns
+// lifecycleshed.NewWorktreeTeardown(name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir).
+func worktreeTeardownEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
+	if err := configRejectUnknown(cfg); err != nil {
+		return nil, err
+	}
+	if err := requireNonEmpty("WorktreeTeardown", "Slug", env.Slug); err != nil {
+		return nil, err
+	}
+	if err := requireAbsRoot("WorktreeTeardown", "ScratchDir", env.ScratchDir); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("WorktreeTeardown", "Teardown.Shutdown", env.Teardown.Shutdown); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("WorktreeTeardown", "Teardown.Remove", env.Teardown.Remove); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("WorktreeTeardown", "PrimeLock.Acquire", env.PrimeLock.Acquire); err != nil {
+		return nil, err
+	}
+	if err := requireAbsRoot("WorktreeTeardown", "PrimeLock.Path", env.PrimeLock.Path); err != nil {
+		return nil, err
+	}
+	return lifecycleshed.NewWorktreeTeardown(name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir), nil
+}
+
+// loomRunEntry is the Constructor for the "LoomRun" registry row: it reads the optional int Config
+// keys poll_interval_s and poll_attempts through configInt, defaulting to
+// defaultLoomRunPollIntervalS and defaultLoomRunPollAttempts respectively when the extracted value
+// is zero -- configInt reports an absent key and an explicit zero identically, so both resolve to
+// the same default -- and rejects a negative value for either key with an error naming that key. It
+// validates Env.Slug, Env.ScratchDir, and Env.LoomRun.Spawn/ResolveStatus/ReadStatus -- and neither
+// Env.LoomRun.Now nor Env.LoomRun.Sleep, whose nil values are legitimate and select the production
+// clock and sleep. It returns lifecycleshed.NewLoomRun(name, env.Slug, env.LoomRun,
+// time.Duration(pollIntervalS)*time.Second, pollAttempts, env.ScratchDir).
+func loomRunEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
+	pollIntervalS, err := configInt(cfg, "poll_interval_s", false)
+	if err != nil {
+		return nil, err
+	}
+	if pollIntervalS < 0 {
+		return nil, fmt.Errorf("shedrecipe: LoomRun: config key %q must not be negative, got %d", "poll_interval_s", pollIntervalS)
+	}
+	if pollIntervalS == 0 {
+		pollIntervalS = defaultLoomRunPollIntervalS
+	}
+
+	pollAttempts, err := configInt(cfg, "poll_attempts", false)
+	if err != nil {
+		return nil, err
+	}
+	if pollAttempts < 0 {
+		return nil, fmt.Errorf("shedrecipe: LoomRun: config key %q must not be negative, got %d", "poll_attempts", pollAttempts)
+	}
+	if pollAttempts == 0 {
+		pollAttempts = defaultLoomRunPollAttempts
+	}
+
+	if err := configRejectUnknown(cfg, "poll_interval_s", "poll_attempts"); err != nil {
+		return nil, err
+	}
+	if err := requireNonEmpty("LoomRun", "Slug", env.Slug); err != nil {
+		return nil, err
+	}
+	if err := requireAbsRoot("LoomRun", "ScratchDir", env.ScratchDir); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("LoomRun", "LoomRun.Spawn", env.LoomRun.Spawn); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("LoomRun", "LoomRun.ResolveStatus", env.LoomRun.ResolveStatus); err != nil {
+		return nil, err
+	}
+	if err := requireSeam("LoomRun", "LoomRun.ReadStatus", env.LoomRun.ReadStatus); err != nil {
+		return nil, err
+	}
+	return lifecycleshed.NewLoomRun(name, env.Slug, env.LoomRun, time.Duration(pollIntervalS)*time.Second, pollAttempts, env.ScratchDir), nil
+}
diff --git a/internal/shedrecipe/entries_lifecycle_test.go b/internal/shedrecipe/entries_lifecycle_test.go
new file mode 100644
index 000000000..d51b8434f
--- /dev/null
+++ b/internal/shedrecipe/entries_lifecycle_test.go
@@ -0,0 +1,235 @@
+// entries_lifecycle_test.go covers the three lifecycle entries: worktreeCreateEntry, loomRunEntry,
+// and worktreeTeardownEntry. It follows entries_simple_test.go's table shape for the shared
+// Slug/ScratchDir/seam validation, plus loomRunEntry's own poll_interval_s/poll_attempts config
+// coverage.
+//
+// Every seam the three lifecycle entries validate -- CreateWorktree, PrimeLock.Acquire,
+// Teardown.Shutdown, Teardown.Remove, LoomRun.Spawn, LoomRun.ResolveStatus, and LoomRun.ReadStatus
+// -- is a concrete func type, not an interface, so there is no separate typed-nil-interface case to
+// exercise beyond the plain-nil case requireSeam handles for a reflect.Func value: a nil func value
+// passed as any already reports Kind() == reflect.Func with IsNil() true, the same detection path a
+// typed-nil interface takes.
+
+package shedrecipe
+
+import (
+	"strings"
+	"testing"
+)
+
+// lifecycleEntryCase is one row of the table shared by the happy-path, Slug, ScratchDir, and
+// unrecognised-config-key tests below.
+type lifecycleEntryCase struct {
+	// registryKey is the name this entry is registered under in registry.go.
+	registryKey string
+	// entry is the Constructor under test.
+	entry Constructor
+	// zeroSeams lists, per seam this entry validates, a closure returning a copy of env with that
+	// seam nilled out and the field name it is validated under -- used to drive the per-seam
+	// nil-rejection subtests.
+	zeroSeams []lifecycleSeamCase
+}
+
+// lifecycleSeamCase names one nil-able seam a lifecycle entry validates.
+type lifecycleSeamCase struct {
+	field string
+	zero  func(env Env) Env
+}
+
+func lifecycleEntryCases() []lifecycleEntryCase {
+	return []lifecycleEntryCase{
+		{
+			registryKey: "WorktreeCreate",
+			entry:       worktreeCreateEntry,
+			zeroSeams: []lifecycleSeamCase{
+				{"CreateWorktree", func(env Env) Env { env.CreateWorktree = nil; return env }},
+				{"PrimeLock.Acquire", func(env Env) Env { env.PrimeLock.Acquire = nil; return env }},
+			},
+		},
+		{
+			registryKey: "WorktreeTeardown",
+			entry:       worktreeTeardownEntry,
+			zeroSeams: []lifecycleSeamCase{
+				{"Teardown.Shutdown", func(env Env) Env { env.Teardown.Shutdown = nil; return env }},
+				{"Teardown.Remove", func(env Env) Env { env.Teardown.Remove = nil; return env }},
+				{"PrimeLock.Acquire", func(env Env) Env { env.PrimeLock.Acquire = nil; return env }},
+			},
+		},
+		{
+			registryKey: "LoomRun",
+			entry:       loomRunEntry,
+			zeroSeams: []lifecycleSeamCase{
+				{"LoomRun.Spawn", func(env Env) Env { env.LoomRun.Spawn = nil; return env }},
+				{"LoomRun.ResolveStatus", func(env Env) Env { env.LoomRun.ResolveStatus = nil; return env }},
+				{"LoomRun.ReadStatus", func(env Env) Env { env.LoomRun.ReadStatus = nil; return env }},
+			},
+		},
+	}
+}
+
+func TestLifecycleEntries_HappyPath(t *testing.T) {
+	for _, tt := range lifecycleEntryCases() {
+		t.Run(tt.registryKey, func(t *testing.T) {
+			producer, err := tt.entry("row-name", Config{}, newTestEnv(t))
+			if err != nil {
+				t.Fatalf("%s() error = %v; want nil", tt.registryKey, err)
+			}
+			if producer == nil {
+				t.Fatalf("%s() = nil producer; want non-nil", tt.registryKey)
+			}
+		})
+	}
+}
+
+func TestLifecycleEntries_RejectsEmptySlug(t *testing.T) {
+	for _, tt := range lifecycleEntryCases() {
+		t.Run(tt.registryKey, func(t *testing.T) {
+			env := newTestEnv(t)
+			env.Slug = ""
+			_, err := tt.entry("row-name", Config{}, env)
+			if err == nil {
+				t.Fatalf("%s() error = nil; want non-nil for an empty Env.Slug", tt.registryKey)
+			}
+			if !strings.Contains(err.Error(), "Slug") {
+				t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, "Slug")
+			}
+		})
+	}
+}
+
+func TestLifecycleEntries_RejectsBadScratchDir(t *testing.T) {
+	for _, tt := range lifecycleEntryCases() {
+		t.Run(tt.registryKey+"_Empty", func(t *testing.T) {
+			env := newTestEnv(t)
+			env.ScratchDir = ""
+			_, err := tt.entry("row-name", Config{}, env)
+			if err == nil {
+				t.Fatalf("%s() error = nil; want non-nil for an empty Env.ScratchDir", tt.registryKey)
+			}
+			if !strings.Contains(err.Error(), "ScratchDir") {
+				t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, "ScratchDir")
+			}
+		})
+
+		t.Run(tt.registryKey+"_Relative", func(t *testing.T) {
+			env := newTestEnv(t)
+			env.ScratchDir = "relative/scratch"
+			_, err := tt.entry("row-name", Config{}, env)
+			if err == nil {
+				t.Fatalf("%s() error = nil; want non-nil for a relative Env.ScratchDir", tt.registryKey)
+			}
+			if !strings.Contains(err.Error(), "ScratchDir") {
+				t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, "ScratchDir")
+			}
+		})
+	}
+}
+
+func TestLifecycleEntries_RejectsNilSeams(t *testing.T) {
+	for _, tt := range lifecycleEntryCases() {
+		t.Run(tt.registryKey, func(t *testing.T) {
+			for _, seam := range tt.zeroSeams {
+				t.Run("Nil"+strings.ReplaceAll(seam.field, ".", "_"), func(t *testing.T) {
+					env := seam.zero(newTestEnv(t))
+					_, err := tt.entry("row-name", Config{}, env)
+					if err == nil {
+						t.Fatalf("%s() error = nil; want non-nil when Env.%s is nil", tt.registryKey, seam.field)
+					}
+					if !strings.Contains(err.Error(), seam.field) {
+						t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, seam.field)
+					}
+				})
+			}
+		})
+	}
+}
+
+func TestLifecycleEntries_RejectsUnrecognisedConfigKey(t *testing.T) {
+	for _, tt := range lifecycleEntryCases() {
+		t.Run(tt.registryKey, func(t *testing.T) {
+			_, err := tt.entry("row-name", Config{"bogus_key": "x"}, newTestEnv(t))
+			if err == nil {
+				t.Fatalf("%s() error = nil; want non-nil for an unrecognised config key", tt.registryKey)
+			}
+			if !strings.Contains(err.Error(), "bogus_key") {
+				t.Errorf("%s() error = %v; want it to name the offending key %q", tt.registryKey, err, "bogus_key")
+			}
+		})
+	}
+}
+
+// TestLoomRunEntry_NilNowAndSleepAreAccepted asserts loomRunEntry does not validate Env.LoomRun.Now
+// or Env.LoomRun.Sleep: their nil values are legitimate and select the production clock and sleep
+// inside lifecycleshed.NewLoomRun.
+func TestLoomRunEntry_NilNowAndSleepAreAccepted(t *testing.T) {
+	env := newTestEnv(t)
+	if env.LoomRun.Now != nil {
+		t.Fatalf("newTestEnv(t).LoomRun.Now is non-nil; want nil by default")
+	}
+	if env.LoomRun.Sleep != nil {
+		t.Fatalf("newTestEnv(t).LoomRun.Sleep is non-nil; want nil by default")
+	}
+	producer, err := loomRunEntry("LoomRun", Config{}, env)
+	if err != nil {
+		t.Fatalf("loomRunEntry() error = %v; want nil", err)
+	}
+	if producer == nil {
+		t.Fatalf("loomRunEntry() = nil producer; want non-nil")
+	}
+}
+
+// TestLoomRunEntry_PollConfigKeys covers loomRunEntry's poll_interval_s and poll_attempts config
+// keys: both absent resolve to defaultLoomRunPollIntervalS and defaultLoomRunPollAttempts, an
+// explicit value for either builds successfully, and a negative value for either is rejected naming
+// that key.
+func TestLoomRunEntry_PollConfigKeys(t *testing.T) {
+	t.Run("AbsentBuildsSuccessfully", func(t *testing.T) {
+		producer, err := loomRunEntry("LoomRun", Config{}, newTestEnv(t))
+		if err != nil {
+			t.Fatalf("loomRunEntry() error = %v; want nil", err)
+		}
+		if producer == nil {
+			t.Fatalf("loomRunEntry() = nil producer; want non-nil")
+		}
+	})
+
+	t.Run("ExplicitPollIntervalS", func(t *testing.T) {
+		producer, err := loomRunEntry("LoomRun", Config{"poll_interval_s": 30}, newTestEnv(t))
+		if err != nil {
+			t.Fatalf("loomRunEntry() error = %v; want nil", err)
+		}
+		if producer == nil {
+			t.Fatalf("loomRunEntry() = nil producer; want non-nil")
+		}
+	})
+
+	t.Run("ExplicitPollAttempts", func(t *testing.T) {
+		producer, err := loomRunEntry("LoomRun", Config{"poll_attempts": 3}, newTestEnv(t))
+		if err != nil {
+			t.Fatalf("loomRunEntry() error = %v; want nil", err)
+		}
+		if producer == nil {
+			t.Fatalf("loomRunEntry() = nil producer; want non-nil")
+		}
+	})
+
+	t.Run("NegativePollIntervalSIsRejected", func(t *testing.T) {
+		_, err := loomRunEntry("LoomRun", Config{"poll_interval_s": -1}, newTestEnv(t))
+		if err == nil {
+			t.Fatalf("loomRunEntry() error = nil; want non-nil for a negative poll_interval_s")
+		}
+		if !strings.Contains(err.Error(), "poll_interval_s") {
+			t.Errorf("loomRunEntry() error = %v; want it to name the offending key %q", err, "poll_interval_s")
+		}
+	})
+
+	t.Run("NegativePollAttemptsIsRejected", func(t *testing.T) {
+		_, err := loomRunEntry("LoomRun", Config{"poll_attempts": -1}, newTestEnv(t))
+		if err == nil {
+			t.Fatalf("loomRunEntry() error = nil; want non-nil for a negative poll_attempts")
+		}
+		if !strings.Contains(err.Error(), "poll_attempts") {
+			t.Errorf("loomRunEntry() error = %v; want it to name the offending key %q", err, "poll_attempts")
+		}
+	})
+}
diff --git a/internal/shedrecipe/env.go b/internal/shedrecipe/env.go
index e17a441a2..31437b06c 100644
--- a/internal/shedrecipe/env.go
+++ b/internal/shedrecipe/env.go
@@ -1,5 +1,6 @@
-// env.go implements the two unexported helpers every entry uses to validate the Env fields it
-// reads: requireAbsRoot for a told absolute-path root, and requireSeam for a told injected seam.
+// env.go implements the three unexported helpers every entry uses to validate the Env fields it
+// reads: requireAbsRoot for a told absolute-path root, requireNonEmpty for a told plain-name value,
+// and requireSeam for a told injected seam.
 
 package shedrecipe
 
@@ -24,6 +25,16 @@ func requireAbsRoot(entry, field, value string) error {
 	return nil
 }
 
+// requireNonEmpty errors when value is empty, naming entry and field. It is a separate helper from
+// requireAbsRoot rather than a reuse of it, because Env.Slug is a plain name and not a path, so
+// requireAbsRoot's absoluteness half would be wrong for it.
+func requireNonEmpty(entry, field, value string) error {
+	if value == "" {
+		return fmt.Errorf("shedrecipe: %s: Env.%s must not be empty", entry, field)
+	}
+	return nil
+}
+
 // requireSeam errors when seam is nil, naming entry and field.
 //
 // It detects a typed-nil interface value as nil too, since Env.Shuttle and Env.Burler are
diff --git a/internal/shedrecipe/env_test.go b/internal/shedrecipe/env_test.go
index b8eec5619..0edd802dc 100644
--- a/internal/shedrecipe/env_test.go
+++ b/internal/shedrecipe/env_test.go
@@ -37,6 +37,24 @@ func TestRequireAbsRoot(t *testing.T) {
 	})
 }
 
+func TestRequireNonEmpty(t *testing.T) {
+	t.Run("Empty", func(t *testing.T) {
+		err := requireNonEmpty("MyEntry", "MyField", "")
+		if err == nil {
+			t.Fatalf("requireNonEmpty() error = nil; want non-nil")
+		}
+		if !strings.Contains(err.Error(), "MyEntry") || !strings.Contains(err.Error(), "MyField") {
+			t.Errorf("requireNonEmpty() error = %v; want it to name entry %q and field %q", err, "MyEntry", "MyField")
+		}
+	})
+
+	t.Run("NonEmpty", func(t *testing.T) {
+		if err := requireNonEmpty("MyEntry", "MyField", "a-slug"); err != nil {
+			t.Errorf("requireNonEmpty() error = %v; want nil", err)
+		}
+	})
+}
+
 func TestRequireSeam(t *testing.T) {
 	t.Run("UntypedNil", func(t *testing.T) {
 		err := requireSeam("MyEntry", "MyField", nil)
diff --git a/internal/shedrecipe/fixture_test.go b/internal/shedrecipe/fixture_test.go
index 7ec0d5d90..20283c7dd 100644
--- a/internal/shedrecipe/fixture_test.go
+++ b/internal/shedrecipe/fixture_test.go
@@ -5,12 +5,15 @@
 package shedrecipe
 
 import (
+	"context"
 	"os"
 	"path/filepath"
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/burlerengine"
+	"github.com/Knatte18/loomyard/internal/lifecycleshed"
 	"github.com/Knatte18/loomyard/internal/shedadapters"
+	"github.com/Knatte18/loomyard/internal/shedengine"
 	"github.com/Knatte18/loomyard/internal/shuttleengine"
 	"github.com/Knatte18/loomyard/internal/websterengine"
 )
@@ -76,14 +79,18 @@ type (
 
 // newTestEnv builds an Env whose every path field is an absolute path derived from a single
 // t.TempDir(), one subdirectory per field: a directory field (Cwd, WorktreeRoot, StencilsDir,
-// SpecsDir, RunRoot, AnchorPath) is created with os.MkdirAll, while a file field (StatusPath,
-// StatusLockPath, DecisionRecordPath, SupportLogPath) is left as a joined path nobody creates. It
-// fills Shuttle, Burler, and WebsterRun with this file's fakes, fills WebsterDeps with the four
-// required seams non-nil and every other field left zero, fills DiscussionSpec with a closure
-// returning a shuttleengine.Spec over one absolute output path under the same temp root, fills
-// CommitDiscussion with a closure returning nil, fills PlanSpec with a closure returning a
-// shuttleengine.Spec over one absolute output path under the same temp root, fills CommitPlan with
-// a closure returning nil, leaves Landing zero, and leaves Now nil.
+// SpecsDir, RunRoot, AnchorPath, ScratchDir) is created with os.MkdirAll, while a file field
+// (StatusPath, StatusLockPath, DecisionRecordPath, SupportLogPath, PrimeLock.Path) is left as a
+// joined path nobody creates. It fills Shuttle, Burler, and WebsterRun with this file's fakes,
+// fills WebsterDeps with the four required seams non-nil and every other field left zero, fills
+// DiscussionSpec with a closure returning a shuttleengine.Spec over one absolute output path under
+// the same temp root, fills CommitDiscussion with a closure returning nil, fills PlanSpec with a
+// closure returning a shuttleengine.Spec over one absolute output path under the same temp root,
+// fills CommitPlan with a closure returning nil, leaves Landing zero, and leaves Now nil.
+//
+// It also fills the six lifecycle fields: a non-empty Slug, CreateWorktree returning nil, LoomRun
+// and Teardown whose own closures return nil or zero values, and a PrimeLock whose Path sits under
+// the same temp root and whose Acquire seam returns a no-op release with ok == true.
 //
 // No test in this package may reference a path outside its own t.TempDir(): a real repo path would
 // mask a told-geometry violation, which is the exact property this package's own Env validation
@@ -137,5 +144,29 @@ func newTestEnv(t *testing.T) Env {
 			}, nil
 		},
 		CommitPlan: func() error { return nil },
+		Slug:       "test-slug",
+		ScratchDir: mustMkdir("scratch"),
+		CreateWorktree: func(context.Context) error {
+			return nil
+		},
+		LoomRun: lifecycleshed.LoomRunDeps{
+			Spawn: func(context.Context) error { return nil },
+			ResolveStatus: func() (string, string, error) {
+				return filepath.Join(dir, "loomrun-status.json"), filepath.Join(dir, "loomrun-status.json.lock"), nil
+			},
+			ReadStatus: func(string, string) (shedengine.Status, bool, error) {
+				return shedengine.Status{}, false, nil
+			},
+		},
+		Teardown: lifecycleshed.TeardownDeps{
+			Shutdown: func(context.Context) (string, error) { return "", nil },
+			Remove:   func(context.Context) error { return nil },
+		},
+		PrimeLock: lifecycleshed.PrimeLock{
+			Path: filepath.Join(dir, "prime.lock"),
+			Acquire: func() (func() error, bool, error) {
+				return func() error { return nil }, true, nil
+			},
+		},
 	}
 }
diff --git a/internal/shedrecipe/recipe.go b/internal/shedrecipe/recipe.go
index 8ba81bb5a..0fe438a1e 100644
--- a/internal/shedrecipe/recipe.go
+++ b/internal/shedrecipe/recipe.go
@@ -5,9 +5,11 @@
 package shedrecipe
 
 import (
+	"context"
 	"time"
 
 	"github.com/Knatte18/loomyard/internal/landingshed"
+	"github.com/Knatte18/loomyard/internal/lifecycleshed"
 	"github.com/Knatte18/loomyard/internal/shedadapters"
 	"github.com/Knatte18/loomyard/internal/shedengine"
 	"github.com/Knatte18/loomyard/internal/websterengine"
@@ -111,4 +113,28 @@ type Env struct {
 	// entry. It is invoked on the approved branch of that producer's settle, before the commit
 	// seam.
 	ApprovePlan func() error
+
+	// Slug is the run-wide task slug, read by all three lifecycle entries (WorktreeCreate, LoomRun,
+	// WorktreeTeardown) for producer identity and stuck-reason text. It is legal on Env because Env
+	// carries roots and run-wide values, and a value that differs per row belongs in Config instead
+	// -- Slug does not differ between the three lifecycle rows a single caller wires.
+	Slug string
+	// ScratchDir is the told absolute directory the three lifecycle producers write their
+	// stuck-reason file into, read by all three.
+	ScratchDir string
+	// CreateWorktree is the single closure WorktreeCreate calls to create the task worktree pair,
+	// following the CommitDiscussion/CommitPlan/ApprovePlan convention: that producer's whole job is
+	// one told action with no behaviour of its own a caller must observe.
+	CreateWorktree func(context.Context) error
+	// LoomRun is a whole-struct passthrough to lifecycleshed.NewLoomRun, following Env.Landing's own
+	// precedent: LoomRun has behaviour of its own -- spawning, resolving, and polling status -- that
+	// per-seam fakes must be able to substitute individually.
+	LoomRun lifecycleshed.LoomRunDeps
+	// Teardown is a whole-struct passthrough to lifecycleshed.NewWorktreeTeardown, following
+	// Env.Landing's own precedent for the same reason as LoomRun: it has behaviour of its own that
+	// per-seam fakes must be able to substitute individually.
+	Teardown lifecycleshed.TeardownDeps
+	// PrimeLock is the told hub-scoped advisory lock WorktreeCreate and WorktreeTeardown both
+	// acquire, read by those two bookend entries only.
+	PrimeLock lifecycleshed.PrimeLock
 }
diff --git a/internal/shedrecipe/registry.go b/internal/shedrecipe/registry.go
index 4bb807561..a45f4d5f3 100644
--- a/internal/shedrecipe/registry.go
+++ b/internal/shedrecipe/registry.go
@@ -11,13 +11,13 @@ import (
 // registry is the single place every engine name is declared, mapping each recipe row's engine
 // name to the Constructor that builds it.
 //
-// The table is complete at fourteen keys. Any fifteenth entry must arrive with a coverage-guard
-// update in the same commit.
+// The table is complete at seventeen keys. Any new entry's coverage is checked in exactly one
+// place: the cross-consumer coverage guard in this package's own external test package.
 //
-// init() self-registration was rejected: the entries span four packages
-// (internal/shedrecipe, internal/preflightshed, internal/landingshed, internal/loomshed), and
-// registration would then depend on link-time blank imports of packages this package already
-// imports directly -- an indirection with no benefit here.
+// init() self-registration was rejected: the entries span five packages
+// (internal/shedrecipe, internal/preflightshed, internal/landingshed, internal/loomshed,
+// internal/lifecycleshed), and registration would then depend on link-time blank imports of
+// packages this package already imports directly -- an indirection with no benefit here.
 var registry = map[string]Constructor{
 	"Preflight":          preflightEntry,
 	"Publish":            publishEntry,
@@ -33,6 +33,9 @@ var registry = map[string]Constructor{
 	"SingleLLM":          singleLLMEntry,
 	"Bouncer":            bouncerEntry,
 	"BurlerRound":        burlerRoundEntry,
+	"WorktreeCreate":     worktreeCreateEntry,
+	"LoomRun":            loomRunEntry,
+	"WorktreeTeardown":   worktreeTeardownEntry,
 }
 
 // Lookup resolves name against the registry, returning its Constructor.
diff --git a/internal/shedrecipe/registry_test.go b/internal/shedrecipe/registry_test.go
index bc3e59b7c..cceb28dc0 100644
--- a/internal/shedrecipe/registry_test.go
+++ b/internal/shedrecipe/registry_test.go
@@ -78,11 +78,11 @@ func TestNames(t *testing.T) {
 	})
 }
 
-// TestRegistry_ShipsFourteenEntries asserts Names() returns exactly the sorted fourteen engine
+// TestRegistry_ShipsSeventeenEntries asserts Names() returns exactly the sorted seventeen engine
 // names this task's registry ships. What is unique to this test relative to TestNames above is the
 // exact-contents pin -- TestNames covers Names()<->registry key agreement and sortedness alone -- and
 // the pin belongs beside the registry rather than with any one consumer of it.
-func TestRegistry_ShipsFourteenEntries(t *testing.T) {
+func TestRegistry_ShipsSeventeenEntries(t *testing.T) {
 	want := []string{
 		"Batchifier",
 		"Bouncer",
@@ -91,6 +91,7 @@ func TestRegistry_ShipsFourteenEntries(t *testing.T) {
 		"DiscussionWrite",
 		"Finalize",
 		"LoomPreflight",
+		"LoomRun",
 		"PlanValidate",
 		"PlanWrite",
 		"Preflight",
@@ -98,6 +99,8 @@ func TestRegistry_ShipsFourteenEntries(t *testing.T) {
 		"SingleLLM",
 		"Stub",
 		"Webster",
+		"WorktreeCreate",
+		"WorktreeTeardown",
 	}
 
 	got := Names()
diff --git a/internal/shedrecipe/seam_enforcement_test.go b/internal/shedrecipe/seam_enforcement_test.go
index f7027f31e..2eb8dacf8 100644
--- a/internal/shedrecipe/seam_enforcement_test.go
+++ b/internal/shedrecipe/seam_enforcement_test.go
@@ -35,6 +35,7 @@ var shedrecipeAllowedImports = map[string]bool{
 	"github.com/Knatte18/loomyard/internal/shedadapters":  true,
 	"github.com/Knatte18/loomyard/internal/loomshed":      true,
 	"github.com/Knatte18/loomyard/internal/landingshed":   true,
+	"github.com/Knatte18/loomyard/internal/lifecycleshed": true,
 	"github.com/Knatte18/loomyard/internal/preflightshed": true,
 	"github.com/Knatte18/loomyard/internal/websterengine": true,
 	"github.com/Knatte18/loomyard/internal/burlerengine":  true,
diff --git a/internal/standalonegeom/reedgeom.go b/internal/standalonegeom/reedgeom.go
index 3d274646e..b30271e0a 100644
--- a/internal/standalonegeom/reedgeom.go
+++ b/internal/standalonegeom/reedgeom.go
@@ -40,8 +40,16 @@ import (
 //
 // PaneCwd and WorktreeRoot stay exactly as TOLD: both name a directory rather than an identity, both
 // spellings reach the same one, and the told spelling is the one the operator typed and will
-// recognise in a header or an error. RepoName is likewise left RAW -- it is the header pane's display
-// token, never a tmux target.
+// recognise in a status line or an error. RepoName is likewise left RAW -- it is the status-line's
+// display token, never a tmux target.
+//
+// WorktreeName is also filled with the RAW filepath.Base(target) -- the identical expression used
+// one line away for RepoName, and deliberately not standalonestate.Normalize(target)'s basename nor
+// readableName above. Standalone mode has no worktree distinct from the repository it targets, so
+// {{.worktree}} and {{.repo}} must render the same string byte for byte and the default template
+// reads "foo/foo · <stateDir>". Taking the raw spelling for both is what makes that exact rather than
+// approximate: normalizing only the new token would make a symlinked target render two different
+// names on one line, even though both tokens are display values that never reach a tmux target.
 func ReedGeometry(target, stateDir, hash8 string) reedengine.Geometry {
 	readableName := filepath.Base(standalonestate.Normalize(target))
 	return reedengine.Geometry{
@@ -53,8 +61,9 @@ func ReedGeometry(target, stateDir, hash8 string) reedengine.Geometry {
 		// LogsDir is stateDir joined with "logs", told directly and deliberately NOT
 		// fabricengine.HubLogsDir(stateDir), which would produce a board-shaped path that does
 		// not exist in standalone mode.
-		LogsDir:  filepath.Join(stateDir, "logs"),
-		RepoName: filepath.Base(target),
-		HubPath:  stateDir,
+		LogsDir:      filepath.Join(stateDir, "logs"),
+		RepoName:     filepath.Base(target),
+		WorktreeName: filepath.Base(target),
+		HubPath:      stateDir,
 	}
 }
diff --git a/internal/standalonegeom/standalonegeom_test.go b/internal/standalonegeom/standalonegeom_test.go
index fef16a62b..182d85bcd 100644
--- a/internal/standalonegeom/standalonegeom_test.go
+++ b/internal/standalonegeom/standalonegeom_test.go
@@ -132,6 +132,14 @@ func TestReedGeometry(t *testing.T) {
 	if got.HubPath != stateDir {
 		t.Errorf("ReedGeometry().HubPath = %q; want %q (stateDir)", got.HubPath, stateDir)
 	}
+	if want := "distinctive-repo-name"; got.WorktreeName != want {
+		t.Errorf("ReedGeometry().WorktreeName = %q; want %q", got.WorktreeName, want)
+	}
+	// Standalone mode has no worktree distinct from the repository it targets, so the two
+	// tokens must render the same string byte for byte.
+	if got.WorktreeName != got.RepoName {
+		t.Errorf("ReedGeometry().WorktreeName = %q; want == RepoName %q", got.WorktreeName, got.RepoName)
+	}
 }
 
 // TestReedGeometry_SessionNameSanitizesTheReadableHalf is the regression guard for the R4 review's
@@ -175,10 +183,48 @@ func TestReedGeometry_SessionNameSanitizesTheReadableHalf(t *testing.T) {
 			if want := reedengine.SanitizeSessionName(c.basename) + "-" + hash8; got.SessionName != want {
 				t.Errorf("ReedGeometry(%q).SessionName = %q; want %q (reedengine.SanitizeSessionName + hash8)", target, got.SessionName, want)
 			}
-			// RepoName is the header pane's display token, not a tmux target, so it stays raw.
+			// RepoName is the status-line's display token, not a tmux target, so it stays raw.
 			if got.RepoName != c.basename {
 				t.Errorf("ReedGeometry(%q).RepoName = %q; want %q (raw basename)", target, got.RepoName, c.basename)
 			}
+			// WorktreeName takes the identical raw expression, so a routine repository name
+			// must not be sanitized or normalized out from under either token.
+			if got.WorktreeName != c.basename {
+				t.Errorf("ReedGeometry(%q).WorktreeName = %q; want %q (raw basename)", target, got.WorktreeName, c.basename)
+			}
+			if got.WorktreeName != got.RepoName {
+				t.Errorf("ReedGeometry(%q).WorktreeName = %q; want == RepoName %q", target, got.WorktreeName, got.RepoName)
+			}
+		})
+	}
+}
+
+// TestReedGeometry_WorktreeNameMatchesRepoNameForSymlinkedTarget pins the standalone-specific claim
+// in reedgeom.go's doc comment: {{.worktree}} and {{.repo}} must render the same string byte for
+// byte for BOTH a symlinked and a real spelling of one target, since both tokens are taken from the
+// raw filepath.Base(target) rather than a normalized spelling that would resolve a symlinked
+// spelling to a different basename than the real one.
+func TestReedGeometry_WorktreeNameMatchesRepoNameForSymlinkedTarget(t *testing.T) {
+	t.Parallel()
+
+	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lyx-state", "abcd1234")
+	hash8 := "abcd1234"
+
+	targets := []string{
+		filepath.Join(string(filepath.Separator), "home", "operator", "src", "real-repo-name"),
+		filepath.Join(string(filepath.Separator), "home", "operator", "links", "symlinked-repo-name"),
+	}
+	for _, target := range targets {
+		t.Run(target, func(t *testing.T) {
+			t.Parallel()
+
+			got := ReedGeometry(target, stateDir, hash8)
+			if got.WorktreeName != got.RepoName {
+				t.Errorf("ReedGeometry(%q).WorktreeName = %q; want == RepoName %q", target, got.WorktreeName, got.RepoName)
+			}
+			if want := filepath.Base(target); got.WorktreeName != want {
+				t.Errorf("ReedGeometry(%q).WorktreeName = %q; want %q (raw basename)", target, got.WorktreeName, want)
+			}
 		})
 	}
 }
diff --git a/internal/tokenvocab/doc.go b/internal/tokenvocab/doc.go
index 4529fb5e5..afec85a6a 100644
--- a/internal/tokenvocab/doc.go
+++ b/internal/tokenvocab/doc.go
@@ -2,8 +2,8 @@
 // extension rule for adding a new token.
 
 // Package tokenvocab is the shared token vocabulary for prompt/template rendering across lyx: today
-// reed's header text pipeline, later loom's prompt templates.
-// It owns the token registry (currently "repo" and "hub", both plain fields on Ctx) and
+// reed's status-line text pipeline, later loom's prompt templates.
+// It owns the token registry (currently "repo", "hub", and "worktree", all plain fields on Ctx) and
 // Render, the reusable compose over internal/stencil that every consumer calls to fill a template
 // with the vocabulary.
 //
diff --git a/internal/tokenvocab/render.go b/internal/tokenvocab/render.go
index 33117b942..1644a779c 100644
--- a/internal/tokenvocab/render.go
+++ b/internal/tokenvocab/render.go
@@ -1,5 +1,5 @@
 // render.go isolates the stencil dependency: Render is the single reusable compose every consumer
-// (reed's header pipeline, loom's prompt templates) calls to fill a template with the token
+// (reed's status-line pipeline, loom's prompt templates) calls to fill a template with the token
 // vocabulary.
 
 package tokenvocab
diff --git a/internal/tokenvocab/tokenvocab.go b/internal/tokenvocab/tokenvocab.go
index 8850ad746..d88f74885 100644
--- a/internal/tokenvocab/tokenvocab.go
+++ b/internal/tokenvocab/tokenvocab.go
@@ -10,6 +10,8 @@ type Ctx struct {
 	RepoName string
 	// HubPath feeds the "hub" token.
 	HubPath string
+	// WorktreeName feeds the "worktree" token.
+	WorktreeName string
 }
 
 // Token is one named, resolvable entry in the vocabulary.
@@ -24,6 +26,7 @@ type Token struct {
 var registry = []Token{
 	{Name: "repo", Resolve: func(c Ctx) string { return c.RepoName }},
 	{Name: "hub", Resolve: func(c Ctx) string { return c.HubPath }},
+	{Name: "worktree", Resolve: func(c Ctx) string { return c.WorktreeName }},
 }
 
 // Build resolves every token in the registry against c.
diff --git a/internal/tokenvocab/tokenvocab_test.go b/internal/tokenvocab/tokenvocab_test.go
index ee3d72437..841323eaf 100644
--- a/internal/tokenvocab/tokenvocab_test.go
+++ b/internal/tokenvocab/tokenvocab_test.go
@@ -50,6 +50,12 @@ func TestTokenResolve(t *testing.T) {
 			ctx:       Ctx{RepoName: "unrelated-repo-value", HubPath: "/hub/loomyard-LYXHUB"},
 			want:      "/hub/loomyard-LYXHUB",
 		},
+		{
+			name:      "worktree reads Ctx.WorktreeName",
+			tokenName: "worktree",
+			ctx:       Ctx{RepoName: "unrelated-repo-value", HubPath: "unrelated-hub-value", WorktreeName: "reed-header-selvage"},
+			want:      "reed-header-selvage",
+		},
 	}
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
@@ -79,15 +85,15 @@ func TestTokenResolve_RepoReadsFieldVerbatim(t *testing.T) {
 	}
 }
 
-// TestBuild_ReturnsBothKeys verifies Build resolves the full registry into a flat map keyed by
-// token name, with both current tokens (repo, hub) present and correctly valued.
-func TestBuild_ReturnsBothKeys(t *testing.T) {
+// TestBuild_ReturnsAllThreeKeys verifies Build resolves the full registry into a flat map keyed by
+// token name, with all three current tokens (repo, hub, worktree) present and correctly valued.
+func TestBuild_ReturnsAllThreeKeys(t *testing.T) {
 	t.Parallel()
 
-	ctx := Ctx{RepoName: "loomyard", HubPath: "/hub/loomyard-LYXHUB"}
+	ctx := Ctx{RepoName: "loomyard", HubPath: "/hub/loomyard-LYXHUB", WorktreeName: "reed-header-selvage"}
 	got := Build(ctx)
 
-	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-LYXHUB"}
+	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-LYXHUB", "worktree": "reed-header-selvage"}
 	if len(got) != len(want) {
 		t.Fatalf("Build() returned %d keys; want %d: %+v", len(got), len(want), got)
 	}
@@ -150,13 +156,13 @@ func TestRegistry_AddingATokenIsOneEntry(t *testing.T) {
 		Resolve: func(c Ctx) string { return "example-slug" },
 	})
 
-	ctx := Ctx{RepoName: "loomyard", HubPath: "/hub/loomyard-LYXHUB"}
+	ctx := Ctx{RepoName: "loomyard", HubPath: "/hub/loomyard-LYXHUB", WorktreeName: "reed-header-selvage"}
 	got := make(map[string]string, len(hypothetical))
 	for _, token := range hypothetical {
 		got[token.Name] = token.Resolve(ctx)
 	}
 
-	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-LYXHUB", "slug": "example-slug"}
+	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-LYXHUB", "worktree": "reed-header-selvage", "slug": "example-slug"}
 	if len(got) != len(want) {
 		t.Fatalf("hypothetical registry resolved to %d keys; want %d: %+v", len(got), len(want), got)
 	}
diff --git a/internal/webstercli/cli.go b/internal/webstercli/cli.go
index 237ca168a..b5e279bc1 100644
--- a/internal/webstercli/cli.go
+++ b/internal/webstercli/cli.go
@@ -24,6 +24,7 @@
 package webstercli
 
 import (
+	"context"
 	"fmt"
 	"io"
 
@@ -53,20 +54,27 @@ type websterCLI struct {
 	engine shuttleengine.Engine
 	reed   shuttleengine.ReedOps
 
-	// reedUp brings the standalone reed session up, idempotently, and is set by wireStandalone
-	// alone — run calls it immediately before spawning Master and recover-batch immediately before
-	// spawning its cold recovery strand, because standalone mode has no other way to a live session:
-	// `lyx reed up` is hub-only (its pre-run requires lyxcwd.Resolve), so the session on standalone's
-	// own geometry (socket "lyx-<hash8>", state under the derived state directory) can only be booted
-	// in-process, mirroring what internal/loomcli's run/drive verbs already do for hub mode. It stays
-	// nil in hub mode, where bringing reed up remains the operator's (or loom's) own act.
+	// reedUp brings the standalone reed session up, idempotently, and, when watch is true and the
+	// boot succeeded, also starts that session's in-process resize watcher bound to ctx. It is set by
+	// wireStandalone alone — run calls it immediately before spawning Master and recover-batch
+	// immediately before spawning its cold recovery strand, because standalone mode has no other way
+	// to a live session: `lyx reed up` is hub-only (its pre-run requires lyxcwd.Resolve), so the
+	// session on standalone's own geometry (socket "lyx-<hash8>", state under the derived state
+	// directory) can only be booted in-process, mirroring what internal/loomcli's run/drive verbs
+	// already do for hub mode. It stays nil in hub mode, where bringing reed up remains the
+	// operator's (or loom's) own act.
+	//
+	// watch is an explicit parameter, not an implicit rule, precisely so the recover-batch asymmetry
+	// is visible at the call sites themselves rather than rediscovered from a comment: run passes
+	// true, recover-batch passes false, because recover-batch is a short-lived verb whose context
+	// would be gone before a bound watcher observed anything.
 	//
 	// It is called by those two spawning verbs alone, never from wiring, so validate, status, pause
 	// and await-batch still boot no tmux server. The membership rule is "does this verb start an OS
 	// process of its own", not "does it write": begin-batch and record-batch mutate state but only
 	// inject into or read around a pane Master already owns, so a session they could reach exists by
 	// construction whenever they are legitimately called.
-	reedUp func() error
+	reedUp func(ctx context.Context, watch bool) error
 
 	// planDirOverridden reports whether --plan-dir moved the plan off the mode's own default
 	// (<stateDir>/_lyx/plan in standalone, the hub anchor's _lyx/plan in hub mode). Set by BOTH
diff --git a/internal/webstercli/cli_test.go b/internal/webstercli/cli_test.go
index 345d89b01..acfe56a87 100644
--- a/internal/webstercli/cli_test.go
+++ b/internal/webstercli/cli_test.go
@@ -15,6 +15,7 @@ package webstercli
 
 import (
 	"bytes"
+	"context"
 	"errors"
 	"os"
 	"path/filepath"
@@ -617,8 +618,10 @@ func TestRecoverBatchCmd_BootsStandaloneReedSessionFirst(t *testing.T) {
 	}
 
 	bringUps := 0
-	c.reedUp = func() error {
+	var gotWatch bool
+	c.reedUp = func(ctx context.Context, watch bool) error {
 		bringUps++
+		gotWatch = watch
 		return errors.New("no tmux server available in this test")
 	}
 
@@ -628,6 +631,9 @@ func TestRecoverBatchCmd_BootsStandaloneReedSessionFirst(t *testing.T) {
 	if bringUps != 1 {
 		t.Fatalf("c.reedUp calls = %d; want exactly 1 -- recover-batch spawns an agent and must boot standalone's own reed session first", bringUps)
 	}
+	if gotWatch {
+		t.Error("c.reedUp watch = true; want false -- recover-batch is short-lived and its context would be gone before a bound watcher observed anything")
+	}
 	if exitCode != 1 {
 		t.Fatalf("recover-batch with a failing reed bring-up = %d; want 1, output: %s", exitCode, out.String())
 	}
@@ -640,6 +646,41 @@ func TestRecoverBatchCmd_BootsStandaloneReedSessionFirst(t *testing.T) {
 	}
 }
 
+// TestRunCmd_PassesWatchTrueToReedUp proves webster's run verb calls c.reedUp with watch: true, the
+// disposition card 43 fixes for it and the counterpart of
+// TestRecoverBatchCmd_BootsStandaloneReedSessionFirst's watch: false assertion above -- driven
+// through run's own RunE with a recording fake in c.reedUp rather than by reading source text. The
+// fake returns an error so the call terminates immediately after the reedUp check, never reaching
+// websterengine.Run.
+func TestRunCmd_PassesWatchTrueToReedUp(t *testing.T) {
+	c, _ := newTestCLI(t)
+	seedValidPlanDir(t, c.geom.PlanDir)
+
+	var bringUps int
+	var gotWatch bool
+	c.reedUp = func(ctx context.Context, watch bool) error {
+		bringUps++
+		gotWatch = watch
+		return errors.New("no tmux server available in this test")
+	}
+
+	var out bytes.Buffer
+	exitCode := clihelp.Execute(c.runCmd(), &out, nil)
+
+	if bringUps != 1 {
+		t.Fatalf("c.reedUp calls = %d; want exactly 1", bringUps)
+	}
+	if !gotWatch {
+		t.Error("c.reedUp watch = false; want true -- run binds the watcher to the run's own context")
+	}
+	if exitCode != 1 {
+		t.Fatalf("run with a failing reed bring-up = %d; want 1, output: %s", exitCode, out.String())
+	}
+	if got := out.String(); !strings.Contains(got, "bring up the standalone reed session") {
+		t.Errorf("output does not name the reed bring-up failure; got %q", got)
+	}
+}
+
 // singleFlagEnvelope matches the one-word machine-readable signal shape webster's verbs use to tell
 // Master WHY a call refused: a whole envelope whose only field is a boolean flag
 // (map[string]any{"plan_drifted": true}). Master keys a failure-ladder rung off each such flag, so
diff --git a/internal/webstercli/recoverbatch.go b/internal/webstercli/recoverbatch.go
index 877e49859..f0d05d5a0 100644
--- a/internal/webstercli/recoverbatch.go
+++ b/internal/webstercli/recoverbatch.go
@@ -131,8 +131,13 @@ Example:
 			// wherever AddStrand is called, not at a controlled chokepoint like this one; the
 			// state-mutation lease is already held across the spawn RecoverSpawnOrAttach itself
 			// performs, so bringing the session up under it adds no new hold.
+			//
+			// watch is false here, unlike run's true: recover-batch is a short-lived verb that spawns
+			// a cold recovery strand and returns, so a watcher bound to this call's context would be
+			// dead before it observed anything, while one detached from that context would be an
+			// unowned goroutine in an exiting process.
 			if c.reedUp != nil {
-				if err := c.reedUp(); err != nil {
+				if err := c.reedUp(cmd.Context(), false); err != nil {
 					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: bring up the standalone reed session: %v", err)))
 					return nil
 				}
diff --git a/internal/webstercli/run.go b/internal/webstercli/run.go
index e6d893ed2..885f6e7c5 100644
--- a/internal/webstercli/run.go
+++ b/internal/webstercli/run.go
@@ -107,7 +107,7 @@ Example:
 			// wherever AddStrand is first called. Nil in hub mode, where the session is the
 			// operator's or loom's own to manage.
 			if c.reedUp != nil {
-				if err := c.reedUp(); err != nil {
+				if err := c.reedUp(cmd.Context(), true); err != nil {
 					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: bring up the standalone reed session: %v", err)))
 					return nil
 				}
diff --git a/internal/webstercli/wiring.go b/internal/webstercli/wiring.go
index c26008667..201e7472a 100644
--- a/internal/webstercli/wiring.go
+++ b/internal/webstercli/wiring.go
@@ -8,6 +8,7 @@
 package webstercli
 
 import (
+	"context"
 	"fmt"
 
 	"github.com/Knatte18/loomyard/internal/batcher"
@@ -270,9 +271,25 @@ func (c *websterCLI) wireStandalone(cwd, stencilsDir, planDir, targetDirFlag str
 	// (see the field's own doc comment). Assigned here, executed by the two verbs that spawn an
 	// agent themselves, run and recover-batch: wiring runs for EVERY verb, and validate, status,
 	// pause and await-batch must boot no tmux server at all.
-	c.reedUp = func() error {
-		_, err := reedEngine.Up()
-		return err
+	//
+	// Standalone reed is never booted by `lyx reed up` — that verb is hub-only — it is booted
+	// in-process by a long-lived supervising run that exists for exactly the session's working
+	// lifetime, and that supervising process is precisely what hub mode lacks and why hub mode needs
+	// a detached daemon at all — so reusing it here is the smaller mechanism, not a special case. The
+	// caller's ctx is what stops the watcher; standalone computes no hub lock path and spawns no
+	// daemon.
+	c.reedUp = func(ctx context.Context, watch bool) error {
+		if _, err := reedEngine.Up(); err != nil {
+			return err
+		}
+		if watch {
+			go func() {
+				if err := reedEngine.Watch(ctx); err != nil {
+					logger.Debug("webster: standalone resize watcher returned", "err", err)
+				}
+			}()
+		}
+		return nil
 	}
 
 	c.setRunner(runner, claudeEngine, reedEngine)
diff --git a/internal/webstercli/wiring_test.go b/internal/webstercli/wiring_test.go
index 60582da5a..8a157b789 100644
--- a/internal/webstercli/wiring_test.go
+++ b/internal/webstercli/wiring_test.go
@@ -27,11 +27,14 @@
 package webstercli
 
 import (
+	"context"
+	"errors"
 	"fmt"
 	"os"
 	"path/filepath"
 	"strings"
 	"testing"
+	"time"
 
 	"github.com/Knatte18/loomyard/internal/cliwire"
 	"github.com/Knatte18/loomyard/internal/logger"
@@ -883,3 +886,133 @@ func TestWireStandalone_FrictionDirAlwaysEmpty(t *testing.T) {
 		t.Errorf("c.frictionDir = %q; want \"\" in standalone mode", c.frictionDir)
 	}
 }
+
+// TestReedUpSeam_WatcherLifecycle pins the standalone reedUp seam's lifecycle contract -- the one
+// batch 06-standalone-watcher's card 42 requires of BOTH wireStandalone closures (this package's and
+// internal/burlercli's, which carries a copy of this same test): the resize watcher starts iff the
+// boot succeeded AND watch is true, and once started it lives exactly as long as the passed context.
+// wiring.go itself is outside this card's Edits and is not driven directly here -- it closes over a
+// real *reedengine.Engine whose Up() spawns a live tmux server, which an untagged Tier 1 test must
+// never do (Test Tier Purity Invariant). This test instead drives the identical shape wiring.go's
+// closure implements against a fake stand-in for reedEngine.Up/Watch, so the lifecycle contract
+// itself is pinned without booting a substrate.
+func TestReedUpSeam_WatcherLifecycle(t *testing.T) {
+	t.Parallel()
+
+	// newSeam reproduces wiring.go's c.reedUp closure body: boot, then start the watcher (bound to
+	// ctx) iff the boot succeeded and watch is true.
+	newSeam := func(upErr error, watchCalls chan context.Context) func(context.Context, bool) error {
+		return func(ctx context.Context, watch bool) error {
+			if upErr != nil {
+				return upErr
+			}
+			if watch {
+				go func() {
+					watchCalls <- ctx
+					<-ctx.Done()
+				}()
+			}
+			return nil
+		}
+	}
+
+	t.Run("StartsOnSuccessfulBootWithWatchTrue", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+		ctx, cancel := context.WithCancel(context.Background())
+		defer cancel()
+
+		seam := newSeam(nil, watchCalls)
+		if err := seam(ctx, true); err != nil {
+			t.Fatalf("seam() = %v; want nil", err)
+		}
+		select {
+		case <-watchCalls:
+		case <-time.After(time.Second):
+			t.Fatal("watcher did not start within 1s of a successful boot with watch: true")
+		}
+	})
+
+	t.Run("DoesNotStartWhenBootFails", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+		bootErr := errors.New("boot failed")
+
+		seam := newSeam(bootErr, watchCalls)
+		if err := seam(context.Background(), true); !errors.Is(err, bootErr) {
+			t.Fatalf("seam() = %v; want %v", err, bootErr)
+		}
+		select {
+		case <-watchCalls:
+			t.Fatal("watcher started despite a failed boot")
+		case <-time.After(50 * time.Millisecond):
+		}
+	})
+
+	t.Run("DoesNotStartWhenWatchFalse", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+
+		seam := newSeam(nil, watchCalls)
+		if err := seam(context.Background(), false); err != nil {
+			t.Fatalf("seam() = %v; want nil", err)
+		}
+		select {
+		case <-watchCalls:
+			t.Fatal("watcher started despite watch: false")
+		case <-time.After(50 * time.Millisecond):
+		}
+	})
+
+	t.Run("StopsWhenContextIsCancelled", func(t *testing.T) {
+		t.Parallel()
+		watchCalls := make(chan context.Context, 1)
+		ctx, cancel := context.WithCancel(context.Background())
+
+		seam := newSeam(nil, watchCalls)
+		if err := seam(ctx, true); err != nil {
+			t.Fatalf("seam() = %v; want nil", err)
+		}
+		var watcherCtx context.Context
+		select {
+		case watcherCtx = <-watchCalls:
+		case <-time.After(time.Second):
+			t.Fatal("watcher did not start")
+		}
+		cancel()
+		select {
+		case <-watcherCtx.Done():
+		case <-time.After(time.Second):
+			t.Fatal("watcher's context did not observe cancellation within 1s")
+		}
+	})
+}
+
+// TestProductionFiles_NeverReferenceHubWatchdogMechanism proves this package's production files never
+// reference the detached per-hub watchdog daemon's mechanism: standalone computes no hub lock path
+// (fabricengine.HubScratchDir) and spawns no daemon (the "reed watchdog" verb). Both belong to
+// hub mode alone, per this batch's own scope note.
+func TestProductionFiles_NeverReferenceHubWatchdogMechanism(t *testing.T) {
+	t.Parallel()
+
+	matches, err := filepath.Glob("*.go")
+	if err != nil {
+		t.Fatalf("glob *.go: %v", err)
+	}
+	for _, path := range matches {
+		if strings.HasSuffix(path, "_test.go") {
+			continue
+		}
+		data, err := os.ReadFile(path)
+		if err != nil {
+			t.Fatalf("read %s: %v", path, err)
+		}
+		content := string(data)
+		if strings.Contains(content, "fabricengine.HubScratchDir") {
+			t.Errorf("%s references fabricengine.HubScratchDir; standalone must compute no hub lock path", path)
+		}
+		if strings.Contains(content, "reed watchdog") {
+			t.Errorf("%s references \"reed watchdog\"; standalone must spawn no daemon", path)
+		}
+	}
+}
diff --git a/manifest/designs/reed-fabric-standalone-api.md b/manifest/designs/reed-fabric-standalone-api.md
index 45bdad560..72950a690 100644
--- a/manifest/designs/reed-fabric-standalone-api.md
+++ b/manifest/designs/reed-fabric-standalone-api.md
@@ -109,8 +109,8 @@ a reader who wants to confirm the layout or façade shape must clone `github.com
 ## Reed — the measured contract today
 
 **Size.**
-`internal/reedengine` is 6,615 production lines (17,474 with tests, measured by `wc -l` over non-`_test.go` files).
-`internal/reedengine/render` adds 773 production lines (1,893 with tests).
+`internal/reedengine` is 6,772 production lines (17,927 with tests, measured by `wc -l` over non-`_test.go` files).
+`internal/reedengine/render` adds 777 production lines (1,973 with tests).
 
 **Direct internal imports.**
 `configengine`, `lock`, `logger`, `lyxdirs`, `proc`, `shell`, `state`, `tokenvocab`, `reedengine/render` — from `internal/reedengine/doc.go` and `go list`.
@@ -121,13 +121,14 @@ a reader who wants to confirm the layout or façade shape must clone `github.com
 `reedengine/doc.go` already documents this honestly and is quoted rather than re-derived: reed is told its geometry and derives none of it, so `internal/lyxcwd` is absent from reed's *direct* production imports even though it is present transitively.
 
 **Public surface.**
-7 free functions, 20 types, and `*Engine` with **18** exported methods and no value-receiver methods — `Socket`, `SessionName`, `TmuxPath`, `AddStrand`, `UpdateStrand`, `RemoveStrand`, `AttachArgv`, `SendText`, `SendKey`, `CapturePane`, `Up`, `EnsureSession`, `Resume`, `Down`, `Status`, `Watch`, `HeaderText`, `ValidateHeader`.
+7 free functions, 20 types, and `*Engine` with **18** exported methods and no value-receiver methods — `Socket`, `SessionName`, `TmuxPath`, `AddStrand`, `UpdateStrand`, `RemoveStrand`, `AttachArgv`, `SendText`, `SendKey`, `CapturePane`, `Up`, `EnsureSession`, `Resume`, `Down`, `Status`, `Watch`, `StatusLineText`, `ValidateStatusLine`.
 Producing command: `go doc ./internal/reedengine Engine | grep -c '^func (e \*Engine)'`.
 
 **External contract footprint.**
-**13 distinct exported package-level identifiers** are referenced *in code* by production packages outside `reedengine`: `AddSpec`, `ConfigTemplate`, `Engine`, `Geometry`, `LoadConfig`, `LoadState`, `New`, `Removed`, `ServerName`, `SessionName`, `StatusResult`, `Strand`, `StrandStatus`.
+**14 distinct exported package-level identifiers** are referenced *in code* by production packages outside `reedengine`: `AddSpec`, `ConfigTemplate`, `Engine`, `Geometry`, `ListSessions`, `LoadConfig`, `LoadState`, `New`, `Removed`, `ServerName`, `SessionName`, `StatusResult`, `Strand`, `StrandStatus`.
 The metric's definition is exactly that phrase — "referenced in code by production packages outside the module" — and it is stated here alongside the number because the same shape of count appears throughout this document.
-This footprint counts `render`'s own exported names (`Display`, `Strand`, `Box`, `Params`, and the `Anchor` constants) and the 18 methods reached through `*Engine` separately from the 13 above, not folded into it.
+This footprint counts `render`'s own exported names (`Display`, `Strand`, `Box`, `Params`, and the `Anchor` constants) and the 18 methods reached through `*Engine` separately from the 14 above, not folded into it.
+`ListSessions` is the fourteenth: `internal/reedcli`'s watchdog daemon (`internal/reedcli/watchdog.go`) is its one production caller, per the `told-geometry-keeps-the-daemon-out-of-reedengine` decision that keeps the daemon itself out of `reedengine`.
 
 A bare `grep -ro 'reedengine\.[A-Za-z0-9_]*'` over non-test files additionally returns three identifiers, none of which is a real external reference: `CleanClaudeEnv` and `AddStrand` appear only as doc-comment prose (`internal/burlerengine/doc.go:205`, `internal/reedcli/add.go:1,4`), and `requireSessionLocked` (`internal/reedcli/attach.go`) is not even exported.
 Counting these three is how the first draft of the underlying investigation reached 15 external identifiers instead of 13.
@@ -166,7 +167,7 @@ The gate is not code readiness — Reed is already close to ready, per the measu
 The gate is the existence of a second consumer, or Reed becoming a long-running service rather than a library.
 
 **Reasoning.**
-Reed's contract is small enough to freeze: 13 external identifiers across 9 direct production importers, most of them construction-only, with one handle type carrying 18 methods.
+Reed's contract is small enough to freeze: 14 external identifiers across 9 direct production importers, most of them construction-only, with one handle type carrying 18 methods.
 But loomyard is Reed's only user, and the quarry precedent measures what extracting ahead of a consumer buys — a separate release cadence and a dependency edge that, months later, still is not drawn.
 
 Three further Reed items sit in `manifest/roadmap.md`'s Someday section — `reed: cross-worktree columns`, `reed: own-window strand anchoring`, `reed: daemon Slack relay` — and all three are Someday, committed but unscheduled, **not Planned**.
diff --git a/manifest/designs/reed-header-selvage.md b/manifest/designs/reed-header-selvage.md
index 7d9030546..55de91f5a 100644
--- a/manifest/designs/reed-header-selvage.md
+++ b/manifest/designs/reed-header-selvage.md
@@ -1,6 +1,6 @@
 # reed: replace the header pane with a native status-line plus "Selvage"
 
-> **Status: Planned, not yet built.** Every claim below marked *(confirmed live)* was tested in a throwaway tmux session during design; everything else is reasoned but unverified.
+> **Status: Implemented.** The design below describes what shipped, not a proposal.
 
 ## The problem
 
@@ -20,9 +20,11 @@ Split all three jobs onto three different mechanisms, instead of hardening the o
 
 tmux's own status-line (`status on`, `status-position bottom`) is not a pane — it cannot receive focus and cannot be typed into.
 
-- The header's only two tokens today, `{{.repo}}` and `{{.hub}}` (`tokenvocab.Ctx`), are static and never need a live update *(confirmed live: `HeaderText()` makes no tmux round trip and reads only `cfg`+`geom`)*.
-- *(confirmed live)* A status-line alone does **not** keep a session alive when every real pane dies — killing all panes in a throwaway session with the status-line on still killed the whole tmux server. The status-line only ever covers content, never keepalive.
-- Showing both repo and worktree (not just hub) needs a new `worktree` token added to `tokenvocab` alongside the existing two — this token does not exist today.
+`internal/reedengine/windowsize.go`'s `pinGeometryOptionsLocked` renders the status-line text via `Engine.StatusLineText()` and issues seven `set-option` calls to pin it: `status on`, `status-position bottom`, `status-left <escaped text>`, `status-right ""`, `status-left-length <statusLeftLength(escaped)>`, and, window-targeted, `window-status-format ""` and `window-status-current-format ""`. `escapeStatusText` doubles every `#` in the rendered text before it reaches `status-left`, since tmux expands `#{…}`/`#[…]` inside a status string. `statusLeftLength` floors the length at tmux's own default of 10 and otherwise measures the escaped string in runes. `pinGeometryOptionsLocked` runs at boot (`lifecycle.go`) and again in the attach pre-flight (`attach.go`), and every call it issues is non-fatal — logged via `logger.Warn` and ignored, per the `geometry-tmux-failures-are-non-fatal-everywhere` decision — because a `set-option` failing loudly changes nothing and is answered by the `#{status}` readback (`readStatusRowsLocked`) rather than trusted from `set-option`'s own exit status.
+
+- The status-line's three tokens, `{{.repo}}`, `{{.hub}}` and `{{.worktree}}` (`tokenvocab.Ctx`), are static and never need a live update. `StatusLineText()` makes no tmux round trip and reads only `cfg`+`geom`.
+- A status-line alone does **not** keep a session alive when every real pane dies — killing all panes in a session with the status-line on still kills the whole tmux server. The status-line only ever covers content, never keepalive.
+- Showing both repo and worktree needs the `worktree` token this task added to `tokenvocab` alongside the pre-existing `repo` and `hub` tokens.
 
 ### Keepalive → a permanent pane named "Selvage"
 
@@ -30,42 +32,57 @@ The pane that must always exist is named **Selvage** — the self-finished edge
 
 Selvage is a deliberately ordinary shell — **not** a custom binary, and it needs no signal-handling code:
 
-- *(confirmed live)* A plain shell already survives an accidental Ctrl-C at its idle prompt (interactive bash ignores SIGINT while waiting at the prompt).
-- *(confirmed live)* Ctrl-C to a foreground job running inside it kills only that job, not the shell — the pane survives.
+- A plain shell already survives an accidental Ctrl-C at its idle prompt (interactive bash ignores SIGINT while waiting at the prompt).
+- Ctrl-C to a foreground job running inside it kills only that job, not the shell — the pane survives.
 - It still dies cleanly when `reed down` deliberately tears the session down (pty close / SIGHUP path, distinct from SIGINT) — a session's life is bounded to its worktree's life, not immortal. Killing it is a normal part of worktree housekeeping, not something to defend against.
 
 Being a real, typeable shell is intentional, not a residual flaw: it is the always-on control terminal for running `lyx`/`reed` commands directly against the worktree — e.g. `lyx reed add` to spawn a new strand (a new Claude instance) — without needing a spare pane first.
 
+`internal/reedengine/lifecycle.go`'s `ensureSelvagePaneLocked` ensures Selvage exists and is alive on both Up and Resume, (re)creating it when missing, dead, or gone via `splitSelvagePaneAtBottomLocked`, which splits a new pane in below the physically bottom-most live pane and retries once behind an even-vertical re-tile when the first attempt has no room — the retry is what keeps a lost or stale `ReedState.SelvagePaneID` from wedging a worktree permanently. `ReedState.SelvagePaneID` (`json:"selvagePaneId,omitempty"`) is the persisted binding; `reed.yaml`'s `selvage.height_rows` config key configures the band's height (the render side is `render.Selvage`/`Params.Selvage`).
+
 ### Placement
 
-Selvage is pinned to the bottom, one row tall, with every strand pane scaled to fill the remainder above it — mirroring today's fixed-band header layout in `apply.go`, just from the opposite edge. *(Not yet confirmed live, unlike everything above.)*
+Selvage is pinned to the bottom, one row tall by default, with every strand pane scaled to fill the remainder above it — mirroring the former fixed-band header layout in `apply.go`, just from the opposite edge.
 
 It stays in the **same single window** as every strand, never a second window:
 
-- Reed has no window support today (see `reed: own-window strand anchoring`).
+- Reed has no window support today (see the Someday `reed: own-window strand anchoring` item).
 - *(confirmed live)* tmux auto-switches the attached client to a window the instant its previously-active window loses its last pane — a "hidden" second window holding Selvage would pop into view at exactly the moment it needs to stay out of the way, defeating the point.
 
 ### The single-pane degenerate case
 
-*(confirmed live)* When Selvage is the only pane left (every strand has died), it automatically fills the entire window — tmux always tiles existing panes to 100% of the window, so there is no "shrink to minimum, leave blank space" option. This needs no special-casing: it is tmux's own default tiling behavior, not something reed's layout math has to detect or handle.
+When Selvage is the only pane left (every strand has died), it automatically fills the entire window — tmux always tiles existing panes to 100% of the window, so there is no "shrink to minimum, leave blank space" option. This needs no special-casing: it is tmux's own default tiling behavior, not something reed's layout math has to detect or handle.
 
 ### Watchdog daemon → a detached, per-hub background process
 
-`eng.Watch(ctx)` needs no tty and no pane at all — it only ever issues tmux commands against a socket from the outside. Once it is no longer riding along inside the header pane's process, its hosting can be chosen freely; it does not have to move to Selvage (or to any pane).
+`eng.Watch(ctx)` needs no tty and no pane at all — it only ever issues tmux commands against a socket from the outside. Once it is no longer riding along inside the header pane's process, its hosting is chosen freely; it does not have to live on Selvage (or on any pane).
 
 Granularity: **per hub, not per worktree-session and not per machine.**
 
 - **Per machine** was considered and rejected. The tempting argument (a reconcile daemon is cheap, so why not have just one) optimizes for CPU, which was never the actual cost. The real cost is that a single machine-wide daemon must multiplex across every hub's own tmux socket at once (today's daemon only ever touches the one socket it was started against), discover hubs appearing and disappearing over time, and — since it no longer maps to any single `reed up`/`down` call — needs its own independent lifecycle (e.g. autostart at login) rather than starting and stopping with worktree activity. It also turns every hub on the machine into one shared blast radius: a crash takes down reconcile for all of them at once, not just one.
 - **Per hub** matches a boundary that already exists — `SocketKey`/`ServerName` is already keyed on hub path (`hubgeom.ReedGeometry`), so a per-hub daemon only ever talks to the one tmux socket that hub already owns, no multiplexing needed.
-- Lifecycle is a genuine open question at this granularity (see below): unlike a per-session daemon, it can no longer simply start with one worktree's `reed up` and stop with that same worktree's `reed down`, since other worktree sessions under the same hub may still be alive.
+
+`internal/reedcli/watchdog.go` implements the daemon as `lyx reed watchdog --hub-path <abs> --tmux <path>`: told its hub path and tmux binary on the command line, opting out of reed's normal cwd/location/config resolution. `runWatchdogLoop` polls `reedengine.ListSessions` (the one new exported, engine-less function `internal/reedengine` gained for this — see the `told-geometry-keeps-the-daemon-out-of-reedengine` decision below) against the hub's socket every `watchdogHubDiscoveryCycle` (5s), enters newly-appeared sessions by building a `*reedengine.Engine` for each and starting its `Engine.Watch` goroutine (unless that worktree's own config says `watchdog: off`), and tears down departed ones. Single-instance per hub is enforced by a `reed-watchdog.lock` file under `fabricengine.HubScratchDir(hub)`: a spawn that finds the lock already held exits 0 immediately, since a racing spawn costing one short-lived process is the expected, harmless outcome. The daemon's own stdio is discarded and its diagnostics land in `fabricengine.HubLogsDir(hub)` instead.
+
+Lifecycle: the daemon exits itself rather than being killed by any reed verb. `watchdogHubDiscoveryCycle`/`watchdogHubIdleCycles` govern this — the idle counter increments on anything other than an affirmative listing (exit 0 with at least one session name), so an exit-0 empty listing, a "no server running" error, and any other `list-sessions` failure all count as idle alike; a non-empty listing resets the counter to zero, and `watchdogHubIdleCycles` (3) consecutive idle cycles exit the daemon. `reed down` never kills it — the daemon notices the hub going quiet on its own next discovery cycle instead.
+
+It is spawned as a **detached child**, never an OS-level service, launch agent, or systemd unit — that broader pattern is explicitly out of scope for this task. Three spawn sites attempt it: `up`, `resume`, and `attach`, each after its own engine op returns without error, via `os.Executable()` + `exec.Command` + `proc.Detach`.
 
 ## Open items
 
-- The `worktree` token for the status-line template does not exist yet — needs adding to `tokenvocab`.
-- Exact layout-math change in `apply.go` to flip the fixed band from top to bottom is reasoned by analogy, not yet implemented or tested.
-- Whether Selvage's shell should be the user's own `$SHELL` or a fixed `bash` is not yet decided.
-- The per-hub daemon's exact lifecycle is undecided: started by the first `reed up` in a hub and stopped by the last `reed down` (reference-counted), or something else entirely (e.g. spawned once at `fabric clone` time and left running for the hub's whole existence)?
-- Whether the daemon should keep being spawned as a detached child of whichever `reed up` starts it, or move to a proper OS-level service/supervisor pattern, is not yet decided.
+- **Windows/psmux verification of the status-line options is unverified, not verified.** The exact check: run `lyx reed up` under psmux, then read back `#{status}`, `#{status-position}`, `#{status-left}` and `#{window-status-format}`, recording which of the options survived. The named degrade accepted in the meantime: if psmux refuses them, Windows loses the identity text, while Selvage, the layout, the reap rules and the watchdog are all unaffected, and `status-position` falling back to `top` is acceptable on its own since the status-line is not a pane and its edge is independent of Selvage's. reed does not branch on Windows here — unlike `hookInstalledLocked`, the one place it does branch — because a refused `set-option` fails loudly into the log, changes nothing, and is answered by the `#{status}` readback, whereas `hookInstalledLocked`'s consequence would be silent and unrecoverable. This decision is to be revisited against the verification's result, not treated as settled.
+
+## Module-local Selvage rules
+
+Kept here rather than in `CONSTRAINTS.md`, because they describe this package's own design rather than a cross-cutting invariant another module could violate:
+
+- Selvage is always physically bottom-most in the window.
+- Selvage is never a strand.
+- Selvage is never written to, cleared, or sent keys by reed — it is a real, usable terminal an operator can run lyx/reed commands in.
+
+## Standalone disposition
+
+Standalone reed runs `Engine.Watch` as an in-process goroutine off the `reedUp` seam, never the daemon — the per-hub watchdog daemon is a hub-mode-only mechanism, since standalone mode has no hub to key a shared socket off of.
 
 ## Related
 
diff --git a/manifest/designs/reed-selvage-pane-extraction.md b/manifest/designs/reed-selvage-pane-extraction.md
new file mode 100644
index 000000000..684b99f16
--- /dev/null
+++ b/manifest/designs/reed-selvage-pane-extraction.md
@@ -0,0 +1,39 @@
+# reed: extract Selvage-pane lifecycle out of apply/reconcile/spawn/lifecycle
+
+> **Status: Someday, not yet designed in depth.** Follow-up to `reed: replace the header pane with a native tmux status-line, a permanent "Selvage" terminal pane, and a detached per-hub watchdog process` — a post-merge audit found that item only fully achieved one of its three separation goals.
+
+## The audit finding
+
+`reed-header-selvage.md`'s own pre-refactor complaint was that the old header pane's lifecycle was scattered across `apply.go` (fixed-band layout), `reconcile.go` (reap-exemption), `spawn.go` (special split-target), and `lifecycle.go` (creation, corpse-detection, up/resume healing) — "~230 lines of pane-lifecycle machinery."
+
+A post-merge audit (grepping the shipped code on `main` at commit `d39b30648`) found the Selvage pane's lifecycle still lives in exactly those same four files, just renamed:
+
+- `lifecycle.go` — 57 hits for "Selvage"
+- `reconcile.go` — 20 hits
+- `spawn.go` — 17 hits
+- `apply.go` — 8 hits
+- `state.go` — 6 hits
+- `config.go` — 4 hits
+
+No `selvagepane.go` (or equivalent) exists. The refactor moved the pane's *responsibilities* (content vs. keepalive vs. watchdog-hosting) apart cleanly, but did not extract the keepalive pane's own *lifecycle code* into one file — the shotgun-surgery shape the original design doc named is unchanged for this piece.
+
+By contrast, the other two goals landed cleanly:
+
+- **Watchdog daemon**: `watchloop.go`'s `watchLoop` calls exactly one engine method, `e.reapplyLayout(...)` — zero direct access to `ReedState`/`SelvagePaneID` anywhere in `watchloop.go`, `watchdog.go`, or `spawnwatchdog.go`.
+- **Content (status-line)**: `statusline.go` is a narrow, self-contained `StatusLineText()`/`ValidateStatusLine()` pair with no Selvage or watchdog awareness. `config.go` splits `StatusLineConfig`/`SelvageConfig` into distinct types.
+
+## The one complication
+
+`windowsize.go`'s `pinGeometryOptionsLocked` deliberately recombines all three concerns in one function: it issues the status-line `set-option` calls, pins Selvage as "always pin index 0," and installs the watchdog's resize-signal hook — all three writers target the same live tmux session-option/hook state, so the file's own comment states this is one atomic writer by design, not an accident. Any extraction work has to either preserve this invariant (keep `pinGeometryOptionsLocked` as the single writer, calling out to per-concern helper functions instead of inlining) or find a different way to avoid the race between three uncoordinated writers.
+
+## What needs to happen
+
+Not yet designed — open questions for whoever picks this up:
+
+- Extract Selvage's pane creation/reap/reconcile logic into its own file (`selvagepane.go` or similar), leaving `apply.go`/`reconcile.go`/`spawn.go`/`lifecycle.go` calling a narrow interface instead of inlining Selvage-specific logic.
+- Decide whether `pinGeometryOptionsLocked`'s three-way merge can be preserved as a thin coordinator over three separately-testable helpers, or whether the atomic-writer requirement makes further separation not worth it.
+- Re-run the same kind of grep-based audit after any extraction to confirm the scatter is actually gone, not just relocated again.
+
+## Related
+
+- [reed-header-selvage.md](reed-header-selvage.md) — the shipped item this is a follow-up audit of; read its own "Status: Implemented" section for what shipped and why.
diff --git a/manifest/designs/worktree-lifecycle-shed-producers.md b/manifest/designs/worktree-lifecycle-shed-producers.md
deleted file mode 100644
index d0bc2d7a8..000000000
--- a/manifest/designs/worktree-lifecycle-shed-producers.md
+++ /dev/null
@@ -1,35 +0,0 @@
-# worktree spawn/teardown as Shed producers
-
-> **Status: Planned, not yet designed in depth — its own bootstrap step relies on the `AddStrand`/`attach` self-heal item, which has landed.** Fold today's three manually-sequenced steps (`lyx fabric` create, `lyx loom run`, `lyx fabric` teardown) into `ShedProducer` rows bookending `loom`'s own list, so the task lifecycle is one driven `Shed` run instead of a human bridging three CLI invocations.
-
-## The full lifecycle
-
-- **Create**: a `fabric create`-equivalent producer row.
-- **Bootstrap**: no explicit "reed up" row needed. Since `AddStrand`/`attach` self-heal shipped, whatever runs next — a `loom run` producer spawning strands, or an operator's `reed attach` — brings the session up as a side effect of actually using it.
-- **Optional VS Code embedding**: a worktree can optionally spawn VS Code, with its `.vscode/tasks.json` `folderOpen` task running `lyx reed attach` directly (self-healing, landing in the always-present Selvage pane once the header-pane split ships). This is just another passive tmux client attaching to the same session — it never conflicts with the Shed driver's own producer work.
-- **Content producers** (`Discussion-Write`, `Plan-Write`, `Webster-Write`, etc.): unchanged by any of this. The Shed driver's own orchestration loop runs wherever it runs (never needs to be inside a pane), but every content-producing producer still spawns its agent strand via `AddStrand`, which always roots that strand's pane inside the actual worktree's reed session — that's inherent to what `AddStrand`/reed already do today, not something this item changes.
-- **Teardown**: a single producer, sequencing internally — never two separate Shed rows — first `reed down`, then fabric's own local cleanup (`Cleanup`/`removeWeftWorktree`). One row keeps this simple; `reed down` is idempotent and cheap, so the producer never needs to check whether a session actually exists before calling it.
-
-## Why fabric never calls reed, and reed never calls fabric
-
-Established while designing this: `fabricengine` must never import `reedengine`, in either direction — `reedengine` already imports fabric-ish path/geometry concepts, so the reverse would risk an import cycle, and conceptually fabric (git/worktree mechanics) is orthogonal to whatever orchestration substrate an agent happens to use. Neither "up" nor "down" is fabric's job.
-
-- **Up** needs no explicit owner: it is ambient, self-healing behavior inside reed's own entrypoints (`AddStrand`, `attach`), triggered by whoever is about to actually use the session. This also naturally survives a machine restart — tmux doesn't, but worktrees do, so "set up once at creation" would go stale anyway; self-heal-on-use is the only mechanism that keeps working after a reboot with zero special-casing.
-- **Down** cannot self-heal from inside reed (reed has no signal that a worktree's life is ending) and must not be fabric's job either. It has to be an explicit step owned by whatever coordinates the whole task lifecycle — this Shed-producer pipeline, once built.
-
-## Today, without this item built yet
-
-Nothing today sequences `reed down` before a worktree is removed — confirmed empirically: no call site anywhere calls `reed down`, and `git worktree remove`/`fabricengine.removeWarpWorktreeDir` doesn't know or check for a live tmux session at all. On Linux this doesn't fail (removing a directory a live process has as its cwd is allowed — POSIX unlink/rmdir don't require an exclusive lock the way Windows/NTFS does), it just silently orphans the tmux session, bound to a now-nonexistent path.
-
-Until this item ships, the manual equivalent is two sequential CLI commands, run by whoever concludes a task and decides the worktree should go:
-
-```
-lyx reed down
-lyx fabric remove <slug>
-```
-
-## Related
-
-- `AddStrand`/`attach` self-heal — the up-side mechanism this item's bootstrap step relies on, shipped.
-- [reed-header-selvage.md](reed-header-selvage.md) — the per-hub daemon whose orphan-reaping extension (see `reed: per-hub daemon reaps orphaned sessions`) is the safety net for when this item's own teardown sequencing doesn't run.
-- [shed-generic-watchdog.md](shed-generic-watchdog.md) — a related but orthogonal generalization axis: that item generalizes the watchdog/CLI-verb layer across any `Shed` recipe; this item adds bookend producer rows to a specific recipe's own `Shed` run. They compose, but are separate efforts.
diff --git a/manifest/roadmap.md b/manifest/roadmap.md
index 5bf3fd17f..02616e82e 100644
--- a/manifest/roadmap.md
+++ b/manifest/roadmap.md
@@ -9,17 +9,9 @@ See Maintenance below for how the numbering works.
 
 This section holds what's committed to next.
 
-1. **reed: replace the header pane with a native tmux status-line, a permanent "Selvage" terminal pane, and a detached per-hub watchdog process** — the header pane conflates three unrelated jobs (content, session keepalive, watchdog hosting); split each onto its own purpose-built mechanism.
-   See [designs/reed-header-selvage.md](designs/reed-header-selvage.md).
-
 1. **loom CLI: rename `run`/`drive`/`step` for verb/engine symmetry, plus rename `ly-supervise`** — today's verb names don't match what each one actually calls; not yet decided.
    See [designs/loom-cli-rename.md](designs/loom-cli-rename.md).
 
-1. **worktree spawn/teardown as Shed producers** — fold `fabric create`, reed's self-healing bootstrap, optional VS Code embedding, and `loom`'s own producer list into one driven `Shed` run, with a single teardown producer sequencing `reed down` then fabric's own cleanup at the end. Depends on the `AddStrand`/`attach` self-heal item, which has landed.
-   See [designs/worktree-lifecycle-shed-producers.md](designs/worktree-lifecycle-shed-producers.md).
-
-1. **fabric: no remote/GitHub branch deletion** — `fabricengine`'s existing branch cleanup (`Cleanup`, `removeWeftWorktree`'s `alsoDeleteBranch`) only ever runs `git branch -D` locally; there is no capability anywhere to delete the corresponding branch on the GitHub remote. Part of why task cleanup today leaves orphaned branches upstream.
-
 ## Next Up
 
 What comes right after Planned clears — committed and ordered, unlike Someday below.
@@ -31,9 +23,8 @@ Not yet started, and exact order can still shift as Planned work reveals what un
 1. **generalize `ly-supervise` and loom's `run`/`drive`/`step` CLI verbs into a Shed-generic watchdog** — `shedengine`/`shedbuild`/`shedrecipe` are already fully generic; only `loomcli` hardcodes loom's own recipe/paths. Speculative until a second `shedrecipe` consumer exists.
    See [designs/shed-generic-watchdog.md](designs/shed-generic-watchdog.md).
 
-1. **reed: per-hub daemon reaps orphaned sessions** — the per-hub watchdog daemon (see the Planned header-pane split) periodically checks whether each live session's worktree still exists on disk, and tears down any that don't. A safety net for when the Planned `worktree spawn/teardown as Shed producers` item's deliberate teardown sequencing doesn't run (crash, manual deletion, aborted task) — not a replacement for it.
+1. **reed: per-hub daemon reaps orphaned sessions** — the per-hub watchdog daemon periodically checks whether each live session's worktree still exists on disk, and tears down any that don't. A safety net for when the Done `worktree spawn/teardown as Shed producers` item's deliberate teardown sequencing doesn't run (crash, manual deletion, aborted task) — not a replacement for it.
    See [designs/reed-header-selvage.md](designs/reed-header-selvage.md).
-
 ## Someday
 
 Committed to eventually — will be done — but not scheduled next.
@@ -53,6 +44,9 @@ No build order is implied between these items.
 1. **reed: strand-based mailbox/addressing system** — deliver messages/events to any Strand by address; being a Strand is required to *receive* mail, not to *send* it.
    See [designs/reed-mailbox.md](designs/reed-mailbox.md).
 
+1. **reed: extract Selvage-pane lifecycle out of apply/reconcile/spawn/lifecycle** — a post-merge audit of the shipped header-pane split found the Selvage pane's own lifecycle code still scattered across the same four files the original design doc named as the smell (just renamed from Header to Selvage); the watchdog and status-line separations landed cleanly, this third one didn't.
+   See [designs/reed-selvage-pane-extraction.md](designs/reed-selvage-pane-extraction.md).
+
 1. **reed: cross-worktree columns** — all worktrees in one tmux window, a column per worktree; needs a name for the new per-worktree grouping layer this introduces and a column-count/fallback policy.
    See [designs/reed-multi-window.md](designs/reed-multi-window.md#cross-worktree-columns).
 
@@ -107,7 +101,7 @@ No build order is implied between these items.
 
 1. **finalize: the discrepancy-document conflict shape** — some divergences cannot be expressed as a git conflict at all, so there are no markers to hand a resolving agent; the answer is a precomputed document describing the disagreement instead. Only the ordinary-git-conflict shape shipped (`internal/mergeresolve`), while `PullResult.PatternResidue` already is this shape for the history-rewrite case — design it once, for both, whenever picked up.
 
-1. **shedrecipe: capability-declaration instead of manual seam-threading** — giving a producer a new capability means hand-threading a passthrough `Env` field through three layers, because the Shed Recipe Registry Invariant bars `shedrecipe` from importing the capability's owning package. The idea, not yet designed: let a producer declare what it needs and have the registry wire it — deep, likely touching the invariant itself and all fourteen registry entries.
+1. **shedrecipe: capability-declaration instead of manual seam-threading** — giving a producer a new capability means hand-threading a passthrough `Env` field through three layers, because the Shed Recipe Registry Invariant bars `shedrecipe` from importing the capability's owning package. The idea, not yet designed: let a producer declare what it needs and have the registry wire it — deep, likely touching the invariant itself and all seventeen registry entries.
 
 1. **reed: daemon Slack relay** — bidirectional Slack relay per worktree, riding on the now-Done `reed: watchdog daemon`. Low priority, well behind the daemon's own self-heal jobs — split out on purpose so it never blocks or gets conflated with the watchdog work.
 
@@ -117,6 +111,12 @@ Cleared 2026-08-25 to keep this file lean — shipped items' history lives in `g
 
 1. **reed: `AddStrand` and `attach` self-heal a cold worktree instead of requiring `up` first** — both verbs pre-flight through the new `ensureSessionLocked`/`EnsureSession()` seam, which probes session liveness first and only delegates to `upLocked()` when nothing usable is up, so any spawn OR view into a worktree nobody has visited (no VS Code, no manual `up`) just works. `attach` boots via `EnsureSession()` and keeps its existing `Status()` call, in that order, so both the friendly no-session diagnosis and the foreign-session refusal survive unchanged. A warm call against a live session is never routed through `Up()`/`upLocked()`: that path reaches `planReconcile`, which would kill an operator's hand-split pane, and validates config ahead of its already-up early return, which would refuse a healthy `attach` on an unrelated typo. Fully internal to `reedengine` — fabric never needs to know reed exists.
 
+1. **fabric: no remote/GitHub branch deletion** — `lyx fabric cleanup` and `lyx fabric remove` both gained an opt-in `--remote` flag that additionally deletes each deleted weft branch's copy on the weft remote.
+   See the `internal/fabricengine` package documentation's destruction chokepoint section.
+
+1. **worktree spawn/teardown as Shed producers** — one driven `Shed` run now takes a task worktree through create, run the loom session to a terminal state, and tear down, driven from the hub's prime worktree (`internal/lifecycleshed` + `internal/lifecyclerecipe` + `internal/lifecyclecli`; `lyx lifecycle run|status`); teardown is one row sequencing session shutdown before worktree removal, never forcing. Optional VS Code embedding did not ship as part of this driven run — see the Someday `VS Code as opt-in per worktree, not spun up by default` item for where that work lives.
+   See the `internal/lifecycleshed` and `internal/lifecyclerecipe` package documentation.
+
 1. **ly-supervise + orchestrator: launch via `lyx reed add`, not ad hoc** — the generated VS Code `folderOpen` task is now the reed launch chain, with both binary paths stamped absolute; `lyx reed add` gained `--if-absent` so reopening a worktree is idempotent; and the `/ly:ly-supervise` skill now treats the session running it as the orchestrator strand itself, rather than telling the operator to open a second one.
 
 1. **Adopt quarry's glyph alphabet as the plan alphabet** — `planparser`/the validator switched a card's symbol declarations from bare names to quarry glyphs, resolved via batched `Resolve`, with placeholder handles (`plan:<expected-glyph>`) for symbols a plan itself creates and mechanical drift detection against the code. Superseded the Someday `quarry-backed plan symbol verification` item.
@@ -131,6 +131,9 @@ Cleared 2026-08-25 to keep this file lean — shipped items' history lives in `g
 1. **reed: watchdog daemon** — the header-pane watch loop, with both halves landed: the resize-geometry reconcile and the pane reap.
    See `internal/reedengine`'s package documentation.
 
+1. **reed: replace the header pane with a native tmux status-line, a permanent "Selvage" terminal pane, and a detached per-hub watchdog process** — the one header pane's three conflated jobs are split apart: identity content now renders through tmux's own native status-line; a deliberately ordinary shell pane, named **Selvage**, is the always-on, pinned-to-the-bottom control terminal that keeps the session alive and doubles as where you'd run `lyx reed add` and friends directly; and the watchdog daemon moved out of any pane entirely, into its own detached background process scoped one-per-hub (matching the existing tmux-server-per-hub boundary).
+   See [designs/reed-header-selvage.md](designs/reed-header-selvage.md).
+
 1. **Real-Linux validation** — the sandbox suite and every tmux/`/proc` assumption are exercised on real Linux, now the platform everything runs on.
 
 1. **loom: Discussion-Review producer** — replaced the `Discussion-Review` stub with a `Discussion-Bouncer`/`Discussion-Burler` segment.
diff --git a/tools/sandbox/SANDBOX-FABRIC-SUITE.md b/tools/sandbox/SANDBOX-FABRIC-SUITE.md
index 1d411573b..6e08fc6d2 100644
--- a/tools/sandbox/SANDBOX-FABRIC-SUITE.md
+++ b/tools/sandbox/SANDBOX-FABRIC-SUITE.md
@@ -526,6 +526,26 @@ Finally check the opposite state is NOT refused -- a target merely **behind** it
 
 ---
 
+### F22 -- Lifecycle status and refusal surface (`lifecycle status`, `lifecycle run`)
+
+**Covers:** lifecycle
+
+**Goal:** "Exercise everything `lyx lifecycle`'s run/status surface can prove without an LLM: the status verb round-tripping a hand-written fixture, the run verb's refusals on a `done` status and a held run lock, both verbs' prime-only refusal, and the create row halting blocked against a deliberately dirty prime."
+
+**Fixture note:** This scenario deliberately never completes the middle row, because a freshly created pair has no task status file and the bootstrap spawns a real driver into the LLM rows whenever the run lock is free, and the lifecycle module exposes no step or pause verb to interpose a fixture mid-run.
+
+**Watch:** Hand-write a slug's `.lyx/lifecycle/<slug>/status.json` as a fixture, following `shedengine.Status`'s own shape (`current_producer`/`state`/`error`/`activity`/`history`), and confirm `lyx lifecycle status <slug>` round-trips every one of those fields back out through its JSON envelope unchanged.
+Set the fixture's `state` to `done` and confirm `lyx lifecycle run <slug>` refuses, naming the per-slug directory to delete to run it again.
+Hold `.lyx/lifecycle/<slug>/run.lock` yourself (any advisory-lock-compatible hold) and confirm `lyx lifecycle run <slug>` refuses, naming that lock path.
+Run both `lyx lifecycle run <slug>` and `lyx lifecycle status <slug>` from a task worktree rather than the hub's prime, and confirm each refuses naming both worktree names and telling the operator to re-run it from the prime.
+Finally, dirty the prime worktree with an uncommitted edit to a tracked file, then run `lyx lifecycle run <a-fresh-slug>` with no status file present for that slug: confirm the create row halts `blocked` rather than `stuck`, the reason surfaces fabric's own dirty-worktree refusal text, and no task worktree was created.
+
+Say in the report that the full driven path -- a completed create, loom run, and teardown -- is covered by the `integration`-tagged end-to-end test instead, so a sandbox operator does not read this scenario's narrower scope as an oversight.
+
+**Verdict:** `OK` / `WARN` / `FAIL`
+
+---
+
 ## Session log format
 
 After running all scenarios, record a short session summary:
@@ -556,6 +576,7 @@ F18: <OK|WARN|FAIL> -- <one-line note if not OK>
 F19: <OK|WARN|FAIL> -- <one-line note if not OK>
 F20: <OK|WARN|FAIL> -- <one-line note if not OK>
 F21: <OK|WARN|FAIL> -- <one-line note if not OK>
+F22: <OK|WARN|FAIL> -- <one-line note if not OK>
 
 sandbox-report.json written: <count of WARN/FAIL items>
 ```
diff --git a/tools/sandbox/SANDBOX-REED-SUITE.md b/tools/sandbox/SANDBOX-REED-SUITE.md
index 6f8a8e5b5..255163814 100644
--- a/tools/sandbox/SANDBOX-REED-SUITE.md
+++ b/tools/sandbox/SANDBOX-REED-SUITE.md
@@ -291,7 +291,7 @@ Skip with a note if no claude is configured. (Covered headlessly by `TestSmokeCl
 **Goal:** "With the overlay up and **no strands added**, create a pane in the reed session behind reed's back, then run `lyx reed up` again and prove the session is still usable."
 
 **Watch:** `tmux -L <socket> split-window -t <session>` (controlled exception) simulates an operator-split/foreign pane.
-The follow-up `lyx reed up` must **not** destroy the session's pane set (`tmux -L <socket> list-panes` still shows panes — an empty pane list means an empty layout was applied and tmux wiped the window: `FAIL`), and that SAME `up` -- not a subsequent `add` -- is what **deterministically reaps** the foreign pane (the documented "reed owns the session window" policy, not a finding), since the untracked reap now fires from the alive header this `up` boots.
+The follow-up `lyx reed up` must **not** destroy the session's pane set (`tmux -L <socket> list-panes` still shows panes — an empty pane list means an empty layout was applied and tmux wiped the window: `FAIL`), and that SAME `up` -- not a subsequent `add` -- is what **deterministically reaps** the foreign pane (the documented "reed owns the session window" policy, not a finding), since the untracked reap now fires from the alive Selvage this `up` boots.
 A subsequent `lyx reed add --cmd <long-running command>` must succeed and read `live: true` in `status` (a "session has no panes to split" error means the session became a zero-pane husk: `FAIL`) — what would be a `FAIL` is a *tracked* strand's pane disappearing instead of the foreign one.
 Covered headlessly by `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` (the `up` reap) and `TestSmokeForeignPaneIsReapedNotAdoptedByAdd` (the faithful M16 regression).
 
@@ -330,18 +330,17 @@ a plain-text line has no such corruption risk.
 
 ---
 
-### M19 -- Always-on header pane (operator console)
+### M19 -- Always-on Selvage pane
 
-**Goal:** "With the overlay up, find the always-on header pane, confirm it actually shows its rendered text, and prove it survives everything the strand lifecycle throws at it — including the removal of the session's last strand and the death of its own process."
+**Goal:** "With the overlay up, find the always-on Selvage pane, confirm it is a genuine shell prompt, confirm the rendered identity text lives in the status-line instead, and prove Selvage survives everything the strand lifecycle throws at it — including the removal of the session's last strand and the death of its own process."
 
-**Watch:** After `lyx reed up`, the session holds one extra pane beyond any strands: the header, physically topmost, whose **visible content** is the rendered header line (default template: `hub: <hub path>`) — a JSON error body, a bare shell prompt,
-or an empty row where the text should be is a `FAIL`, not cosmetics (the pane merely being *alive* is not enough). `lyx reed status` must never list the header as a strand, and `up`'s `strands` count must exclude it.
-Then: add a strand, remove it — the session **survives** on the header alone (pre-header reed destroyed the session with its last pane;
-that teardown is the footgun this feature exists to remove) and a follow-up `add` still works, with the header back at its configured `height_rows` (default 1) and still topmost.
-Finally kill the header's own process (`tmux -L <socket> list-panes -t "=<session>:" -F "#{pane_id} #{pane_pid}"` to find it, then kill that pid — controlled exception;
-on POSIX use `kill -9`, interactive shells ignore TERM): intermediate verbs (`add`/`remove`) must keep working with sane pane geometry (a strand squeezed to 1 row means a stale header cell scrambled the layout: `FAIL`),
-and the next `lyx reed up` must **heal** the header — a fresh, alive, topmost pane showing the rendered text again, with the corpse gone.
-A `split header pane: ... no space for new pane` error from `up` is the wedged-heal regression: `FAIL`.
+**Watch:** After `lyx reed up`, the session holds one extra pane beyond any strands: Selvage, physically **bottom**-most, whose **visible content** is a bare **shell prompt** — a JSON error body, an empty row, or anything other than an idle shell prompt is a `FAIL`, not cosmetics (the pane merely being *alive* is not enough). Separately, `tmux -L <socket> display-message -p '#{status-left}'` (controlled exception) must show the rendered identity text (default template names the repo and the hub) — a blank or stale readback there is a `FAIL` in its own right. `lyx reed status` must never list Selvage as a strand, and `up`'s `strands` count must exclude it.
+Then: add a strand, remove it — the session **survives** on Selvage alone (a session with no permanent pane would die with its last strand;
+that teardown is the footgun this feature exists to remove) and a follow-up `add` still works, with Selvage back at its configured `selvage.height_rows` (default 1) and still bottom-most.
+Finally kill Selvage's own shell process (`tmux -L <socket> list-panes -t "=<session>:" -F "#{pane_id} #{pane_pid}"` to find it, then kill that pid — controlled exception;
+on POSIX use `kill -9`, interactive shells ignore TERM): intermediate verbs (`add`/`remove`) must keep working with sane pane geometry (a strand squeezed to 1 row means a stale Selvage cell scrambled the layout: `FAIL`),
+and the next `lyx reed up` must **heal** Selvage — a fresh, alive, bottom-most pane with an idle shell prompt again, with the corpse gone.
+A `split Selvage pane: ... no space for new pane` error from `up` is the wedged-heal regression: `FAIL`.
 
 **Verdict:** `OK` / `WARN` / `FAIL`
 
@@ -378,11 +377,11 @@ A new server on the default socket (the capability probe's historical leak, R2-F
 
 **Goal:** "Prove a lost `.lyx/reed.json` does not permanently wedge a worktree whose tmux session is still running."
 
-**Watch:** `up`, then `add` one strand, so the one-row header band at the top of the window is actually laid out.
+**Watch:** `up`, then `add` one strand, so the one-row Selvage band at the bottom of the window is actually laid out.
 Delete `.lyx/reed.json` (or run `git clean -xdf` in the worktree -- `.lyx` is never-tracked machine-local scratch, so this is a sanctioned operator action) while leaving the session up.
-`lyx reed up` must then SUCCEED and converge IMMEDIATELY, not one verb late: the old header and the now-untracked strand pane are both reaped in this same `up`, and the freshly rebuilt header ends up alone, full-height, at the very top of the window, with nothing below it -- a full-height header with an empty strand stack is the expected `OK` here, not a defect.
+`lyx reed up` must then SUCCEED and converge IMMEDIATELY, not one verb late: the old Selvage pane and the now-untracked strand pane are both reaped in this same `up`, and the freshly rebuilt Selvage ends up alone, full-height, filling the whole window, with nothing above it -- Selvage alone filling the window is tmux's own tiling rather than something reed computes, and it is the expected `OK` here, not a defect.
 A subsequent `lyx reed add` must run its command for real (check the pane, and check the process exists), not type it onto a pane that is already busy.
-An `up` that fails with `no space for new pane`, a header that does not end up topmost, or an added strand whose command never runs is a `FAIL` -- and so now is a surviving OLD header pane or a surviving ORPHANED strand pane left behind by that `up`.
+An `up` that fails with `no space for new pane`, a Selvage that does not end up bottom-most, or an added strand whose command never runs is a `FAIL` -- and so now is a surviving OLD Selvage pane or a surviving ORPHANED strand pane left behind by that `up`.
 
 **Verdict:** `OK` / `WARN` / `FAIL`
 
@@ -416,7 +415,7 @@ controlled exception, restore the name when done).
 Running the `kill-session` the error names, then `lyx reed resume` again, must succeed normally.
 A silent success, two copies of the strand process, a second session left on the socket, or a refusal the operator cannot escape is a `FAIL`.
 After that successful `resume`, inspect the hub root: it must contain no directory named after the pre-rename worktree -- a stray directory there is a `FAIL`, since the whole point of the refusal is that nothing gets conjured under the old name.
-Before running the remedy, check the renamed session's header pane log: it must show exactly one warning about the vanished worktree root rather than a reconcile failure repeating every two seconds, which is a `FAIL`.
+Before running the remedy, check the hub's log file (`fabricengine.HubLogsDir(hub)`): it must show exactly one warning about the vanished worktree root rather than a reconcile failure repeating every two seconds, which is a `FAIL`.
 
 **Verdict:** `OK` / `WARN` / `FAIL`
 
@@ -434,7 +433,7 @@ A `down` that kills the old session is also a `FAIL`.
 Then run an ordinary `lyx reed up` / `down` cycle in a normal worktree and confirm `abandonedSession` is ABSENT there -- the key must be signal, not noise.
 Inspect the hub root as well, but not immediately: wait well past the two-second watchdog poll cycle before checking, since `down` deliberately leaves the abandoned session running, and checking too early would mask a watcher that resumed leaking.
 Once that wait has elapsed, the hub root must contain no directory named after the pre-rename worktree -- a stray directory there is a `FAIL`.
-Across that same wait, the abandoned session's header pane must have logged exactly one warning about the vanished worktree root, not one every two seconds -- a stream of repeating warnings there is a `FAIL`, since it means the watcher never dropped to its dormant cadence.
+Across that same wait, the hub's log file (`fabricengine.HubLogsDir(hub)`) must show exactly one warning about the vanished worktree root, not one every two seconds -- a stream of repeating warnings there is a `FAIL`, since it means the watcher never dropped to its dormant cadence.
 
 **Verdict:** `OK` / `WARN` / `FAIL`
 
@@ -446,19 +445,19 @@ Across that same wait, the abandoned session's header pane must have logged exac
 
 **Goal:** "Prove a live terminal resize re-applies the planned layout on its own, in both directions, with no `lyx` command run."
 
-**Watch:** The agent pauses and instructs the operator to attach with `lyx reed attach` **in a second terminal** against a session holding a header pane and at least two strands.
-Confirm the header is exactly `header.height_rows` tall and the strand budgets look right.
+**Watch:** The agent pauses and instructs the operator to attach with `lyx reed attach` **in a second terminal** against a session holding Selvage and at least two strands.
+Confirm Selvage is exactly `selvage.height_rows` tall and the strand budgets look right.
 Then have the operator **drag the terminal window larger** and confirm the layout re-applies within about a second, with no `lyx` command run.
 Then have the operator **drag it smaller** and confirm the same.
 The shrink direction is the non-negotiable half of this scenario -- it is the one SIGWINCH misses entirely, and a watcher that only self-heals on growth must be reported as a `FAIL` here, not a `WARN`.
-A header that grows past its configured row count, or a bottom strand squeezed below `min_full_rows`, is also a `FAIL`.
+A Selvage that grows past its configured row count, or a bottom strand squeezed below `min_full_rows`, is also a `FAIL`.
 The operator must also confirm the cursor did NOT jump to another pane across either resize (the focus-steal regression), and that typing into a pane before a resize leaves that same pane focused after it.
 Rationale: the agent session owns the current terminal, so it cannot demonstrate or observe a live client resize itself.
 
 The agent then checks the mechanism the self-heal is supposed to be RUNNING on, which the visual half cannot distinguish on its own -- a watcher stuck on the two-second poll fallback heals a resize about as convincingly as a signal-driven one does, only later.
 `tmux -L <socket> show-options -v -t '=<session>:' window-resized` (controlled exception) must print one `resize-pane -t "%<id>" -y <rows>` line per fixed-height pane AND, as its LAST line, a `run-shell -b` line naming THIS worktree's own `.lyx/reed-resize.signal` path.
 A missing `run-shell` line is a `FAIL`: it means every watcher on the box is silently in poll mode, with the hook probe reporting "absent" forever.
-The header pane's log must also carry exactly one `promoting resize watchdog to signal mode` line for the session -- absent means the probe never matched, and repeated means the watcher is flapping between modes.
+The hub's log file (`fabricengine.HubLogsDir(hub)`) must also carry exactly one `promoting resize watchdog to signal mode` line for the session -- absent means the probe never matched, and repeated means the watcher is flapping between modes.
 
 **Verdict:** `OK` / `WARN` / `FAIL`
 

```

## Instructions

1. Read the failing tests and the source files they exercise.
2. Fix the root cause of the failures.
   Do not modify tests unless they are genuinely wrong due to the merge (e.g. a test asserted against a value that the merge legitimately changed).
3. Re-run `go test -tags smoke ./internal/reedcli/ -skip '^TestSmokeClaudeResumeRecallsCodeword$' && go test -tags integration ./internal/reedengine/` after each fix attempt using `git -C /home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal` for git commands.
4. Commit each fix attempt with a clear commit message.
5. Self-fix up to `3` times.
   If the verify command still fails after `3` attempts, stop and report stuck.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

**`commit_sha` MUST be the full SHA from `git rev-parse HEAD` -- never the abbreviated form (`git rev-parse --short HEAD`) or a `git log --oneline` hash.**

On success:

{"status":"success","commit_sha":"<last-HEAD-sha>"}

After exhausting fix rounds:

{"status":"stuck","stuck_type":"verify","reason":"<one-line description of what still fails>","commit_sha":"<last-HEAD-sha>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal` for git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal`.
