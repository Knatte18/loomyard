# Status

```yaml
phase: approved-run-routing-and-budget
slug: shedengine-segments-bounce-budget
branch: shedengine-segments-bounce-budget
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'shedengine: per-producer bounce budget + explicit OnDone routing'
task_description: |
  shedengine: per-producer bounce budget + explicit OnDone routing
```

## Timeline

```text
discussing  '2026-08-20T08:17:05Z'
discussion-fix-r2  '2026-08-20T08:41:31Z'
blocked  '2026-08-20T09:04:32Z'
discussed  '2026-08-20T09:07:13Z'
planning  '2026-08-20T09:14:55Z'
plan-review-r1  '2026-08-20T09:25:55Z'
plan-fix-r1  '2026-08-20T09:25:55Z'
plan-review-r2  '2026-08-20T09:34:01Z'
plan-fix-r2  '2026-08-20T09:34:01Z'
plan-fix-r3  '2026-08-20T09:44:16Z'
plan-fix-r4  '2026-08-20T09:51:32Z'
planned  '2026-08-20T09:51:42Z'
implementing  '2026-08-20T09:52:16Z'
approved-engine-fields-and-validation  '2026-08-20T09:56:20Z'
approved-run-routing-and-budget  '2026-08-20T10:06:49Z'
```

## Batches

```yaml
batches:
  - name: engine-fields-and-validation
    state: approved
    implementer_session: 32dd9167-a0f5-49ee-8f2f-8597412693f3
    start_sha: 569888269ea72bf7f2e670ff3ea3a2faf8c1b842
    commit_sha: e271dd7c8274b314ca856c351e029b76c9347e46
    verify_baseline_failures: []
  - name: run-routing-and-budget
    state: approved
    implementer_session: 086a8a90-f86b-410b-9c52-879e09834bbc
    start_sha: 0c07028a736b53719ef48e22092bf24e01d4a644
    commit_sha: 3bd6133401aeb4287281d351dd80042c8f572277
    verify_baseline_failures: []
  - name: loomshed-migration
    state: pending
    verify_baseline_failures: []
  - name: docs-sweep
    state: pending
    verify_baseline_failures: []
```
