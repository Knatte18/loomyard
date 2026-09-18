# Status

```yaml
phase: approved-watchdog-daemon
slug: reed-header-selvage
branch: reed-header-selvage
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: Replace reed's header pane with a status-line and Selvage
task_description: |
  Replace reed's header pane with a status-line and Selvage
```

## Timeline

```text
discussing  '2026-09-18T16:29:48Z'
discussion-fix-r1  '2026-09-18T16:41:14Z'
discussion-gap-fix-r2  '2026-09-18T16:46:30Z'
discussion-gap-fix-r3  '2026-09-18T16:53:05Z'
discussion-gap-fix-r4  '2026-09-18T16:58:30Z'
discussion-gap-fix-r5  '2026-09-18T17:04:34Z'
discussion-gap-fix-r6  '2026-09-18T17:10:31Z'
discussion-gap-fix-r7  '2026-09-18T17:14:39Z'
discussed  '2026-09-18T17:14:39Z'
planning  '2026-09-18T17:32:29Z'
plan-review-r1  '2026-09-18T17:41:35Z'
plan-fix-r1  '2026-09-18T17:42:23Z'
plan-review-r2  '2026-09-18T17:49:55Z'
plan-fix-r2  '2026-09-18T17:51:49Z'
plan-review-r3  '2026-09-18T17:59:18Z'
plan-fix-r3  '2026-09-18T18:00:04Z'
planned  '2026-09-18T18:00:17Z'
implementing  '2026-09-18T18:00:49Z'
approved-render-bottom-band  '2026-09-18T18:10:34Z'
blocked  '2026-09-18T18:20:09Z'
self-resolved-verify-logic  '2026-09-18T18:40:12Z'
approved-vocabulary-and-config  '2026-09-18T18:40:12Z'
approved-selvage-pane  '2026-09-18T19:06:29Z'
approved-status-line-pins  '2026-09-18T19:18:53Z'
approved-watchdog-daemon  '2026-09-18T19:34:52Z'
```

## Batches

```yaml
batches:
  - name: render-bottom-band
    state: approved
    implementer_session: a3b414f3-4e7e-4135-ac43-0f5db3fc6bbb
    start_sha: 9e6e98a7dbe8fda7ac65890dd80f86b9434aaec8
    commit_sha: f17bc76749c1175e9d16aab635259164a485ea94
    verify_baseline_failures: []
  - name: vocabulary-and-config
    state: approved
    implementer_session: b10b8c5f-73a0-4eb4-8fa4-ca98ae6f09e6
    start_sha: 3032ada47f66bd210b1f1095c813a110e5b4c657
    commit_sha: 803ca020f
    blocked_reason: 'verify/logic self-resolve blocked: card 15 already used by batch ''03-selvage-pane''; ''02-vocabulary-and-config'' and ''03-selvage-pane'' occupy overlapping numeric ranges (PlanDAGError while computing next card number for vocabulary-and-config); underlying failure: [module-wide verify] internal/reedcli/header.go:103:23: c.eng.HeaderText undefined (type *reedengine.Engine has no field or method HeaderText)'
    verify_baseline_failures: []
  - name: selvage-pane
    state: approved
    implementer_session: 6b6152cc-6acd-4760-b9c2-cc6c15db87c9
    start_sha: 6bf4f4da04ddd4e725052171933a7b9d763a3c56
    commit_sha: 64fa306ca56c50805e920b12036ea92c1f7beefd
    verify_baseline_failures: []
  - name: status-line-pins
    state: approved
    implementer_session: e660438e-bc71-4acd-95e4-6eb8123352b5
    start_sha: 3b4097a8757b4c27cd532455312ac21a20b42ceb
    commit_sha: c2608520b44c0552475c5b9236e69fe731436a59
    verify_baseline_failures: []
  - name: watchdog-daemon
    state: approved
    implementer_session: 398d825e-1d2c-489f-a26a-e2c536b749ed
    start_sha: 6aed78f01190d7d81e0e3008a81278c517c0c42c
    commit_sha: b8126d29749e74b7a5a271c8dcefa4af18188937
    verify_baseline_failures: []
  - name: standalone-watcher
    state: running
    implementer_session: 14443cb4-b31b-440a-ac48-dcf26b1c40fe
    start_sha: c91df24c296f951de9b7924d6b81811e5d7cf98f
    verify_baseline_failures: []
  - name: docs-smokes-and-residue
    state: pending
    verify_baseline_failures: []
```
