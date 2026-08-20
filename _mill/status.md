# Status

```yaml
phase: approved-loomshed-migration
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
approved-loomshed-migration  '2026-08-20T10:11:17Z'
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
    state: approved
    implementer_session: 6ef01e26-bfbf-4de1-a84b-88ac2ee809cf
    start_sha: d4006c8934eca3abb83f5d85ce51d40aec4afe9b
    commit_sha: ec01e786112ccb773fc80bac1c0f3c177a144cd3
    verify_baseline_failures: []
  - name: docs-sweep
    state: running
    implementer_session: e9832e99-5ae8-43ad-8864-81dbeb6158f3
    start_sha: e6e4dbd22957aaff4de07a588e40d08e956e2822
    verify_baseline_failures: []
```
