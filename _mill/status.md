# Status

```yaml
phase: approved-fabric-destroy-caller-files
slug: gitexec-checked-entry-point
branch: gitexec-checked-entry-point
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'gitexec: add the checked entry point and migrate the call sites'
task_description: |
  gitexec: add the checked entry point and migrate the call sites
```

## Timeline

```text
discussing  '2026-08-13T13:19:23Z'
discussion-fix-r4  '2026-08-13T14:11:20Z'
discussion-fix-r5  '2026-08-13T14:18:03Z'
discussed  '2026-08-13T14:18:03Z'
planning  '2026-08-13T14:31:58Z'
plan-fix-r1  '2026-08-13T14:42:01Z'
plan-review-r2  '2026-08-13T14:48:54Z'
plan-fix-r2  '2026-08-13T14:48:54Z'
plan-review-r3  '2026-08-13T15:02:12Z'
plan-fix-r3  '2026-08-13T15:02:12Z'
plan-fix-r4  '2026-08-13T15:13:33Z'
planned  '2026-08-13T15:13:52Z'
implementing  '2026-08-13T15:14:25Z'
approved-gitexec-checked-entry-point  '2026-08-13T15:21:27Z'
approved-gitrepo-checked-pair  '2026-08-13T15:29:59Z'
approved-fabric-destroy-executors  '2026-08-13T15:36:55Z'
approved-outer-call-sites  '2026-08-13T15:40:23Z'
approved-fabric-destroy-caller-files  '2026-08-13T15:46:50Z'
```

## Batches

```yaml
batches:
  - name: gitexec-checked-entry-point
    state: approved
    implementer_session: 4fbd83b2-7521-487e-be84-24d13ac74470
    start_sha: a0ab68e150243a4dca289d05fc22ad072cd52482
    commit_sha: 3050d8b5c203b38ac7f62cd281d85b3af5123d66
    verify_baseline_failures: []
  - name: gitrepo-checked-pair
    state: approved
    implementer_session: 83620bdd-0ab8-44e6-8d70-538e0428edee
    start_sha: 476c782abcbdb1849dacab14c0c54c606d35f33b
    commit_sha: 870dec82d43a8476ac3f1035bb4f8e011111d10b
    verify_baseline_failures: []
  - name: fabric-destroy-executors
    state: approved
    implementer_session: 1c626ce1-d075-4322-8a56-01d0c04616a1
    start_sha: 46cec7b4e5c4c43e1953e2c906c60b1f49eb03e1
    commit_sha: 40b2e512dc035a1929087f2d1675af40ba06d217
    verify_baseline_failures: []
  - name: outer-call-sites
    state: approved
    implementer_session: 67298108-874c-4c01-9322-7b6495420f80
    start_sha: 7fc1fe4a22ef9f9b0d337088cc1aeeb0ab8fb89a
    commit_sha: edff8d727bd0b74848158d9dd4824cabdb2f4dbc
    verify_baseline_failures: []
  - name: fabric-destroy-caller-files
    state: approved
    implementer_session: 40208072-5d92-42e2-b18c-a9a1cfbd8bfa
    start_sha: 73513e314968c515cd831bb7ebcd108ca8b87a95
    commit_sha: ca38dfd5a550716aacbb8cb20cc7fea32db53def
    verify_baseline_failures: []
  - name: fabric-probe-clone-reconcile
    state: pending
    verify_baseline_failures: []
  - name: fabric-remaining-sites
    state: pending
    verify_baseline_failures: []
  - name: checked-call-invariant-and-docs
    state: pending
    verify_baseline_failures: []
```
