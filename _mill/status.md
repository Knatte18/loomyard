# Status

```yaml
phase: implementing
slug: hub-dotlyx-into-board
branch: hub-dotlyx-into-board
plan: _mill/plan
parent: main
task: Move <hub>/.lyx into <hub>/_board
task_description: |
  Move <hub>/.lyx into <hub>/_board
```

## Timeline

```text
discussing  '2026-08-14T15:25:11Z'
discussed  '2026-08-14T16:50:25Z'
planning  '2026-08-14T17:01:26Z'
plan-fix-r1  '2026-08-14T17:10:05Z'
plan-review-r2  '2026-08-14T17:21:14Z'
plan-fix-r2  '2026-08-14T17:21:14Z'
plan-fix-r3  '2026-08-14T17:29:07Z'
plan-review-r4  '2026-08-14T17:36:41Z'
plan-fix-r4  '2026-08-14T17:36:41Z'
plan-review-r5  '2026-08-14T17:44:49Z'
plan-fix-r5  '2026-08-14T17:44:49Z'
plan-review-r6  '2026-08-14T17:53:12Z'
planned  '2026-08-14T17:53:22Z'
implementing  '2026-08-14T17:53:58Z'
```

## Batches

```yaml
batches:
  - name: hub-scratch-move
    state: running
    implementer_session: 96a6fd81-756a-41ac-a743-c3e6043a8e87
    start_sha: c14458134fccace6c2bb08fcd38dd19bd0d368b5
    verify_baseline_failures: ['--- FAIL: TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive (0.47s)', "FAIL\t\
    github.com/Knatte18/loomyard/internal/reedcli\t21.906s", '--- FAIL: TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive
    (0.46s)', "FAIL\tgithub.com/Knatte18/loomyard/internal/reedcli\t22.925s"]
  - name: board-junction-deletion
    state: pending
    verify_baseline_failures: []
```
