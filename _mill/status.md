# Status

```yaml
phase: approved-quarry-cli-infra
slug: scout-extract-standalone-repo
branch: scout-extract-standalone-repo
plan: _mill/plan
parent: main
task: Extract scout into its own standalone repo
task_description: |
  Extract scout into its own standalone repo
```

## Timeline

```text
discussing  '2026-08-20T08:21:22Z'
discussion-fix-r4  '2026-08-20T09:36:46Z'
discussion-fix-r5  '2026-08-20T09:47:18Z'
discussed  '2026-08-20T09:47:18Z'
planning  '2026-08-20T10:02:59Z'
plan-review-r1  '2026-08-20T10:41:43Z'
plan-fix-r1  '2026-08-20T10:41:43Z'
plan-fix-r2  '2026-08-20T11:27:00Z'
plan-fix-r3  '2026-08-20T11:48:03Z'
planned  '2026-08-20T11:48:13Z'
implementing  '2026-08-20T14:50:54Z'
approved-quarry-scaffold  '2026-08-20T14:58:14Z'
approved-quarry-cli-infra  '2026-08-20T15:05:26Z'
```

## Batches

```yaml
batches:
  - name: quarry-scaffold
    state: approved
    implementer_session: 9a65255d-f1ea-4cdb-ae3d-474445656a9b
    start_sha: 7cd73c16f733288f0cbddcd4f30275855e59b3ff
    commit_sha: b0c261b3870e9a1f76553ef0837d5da4169ac381
    verify_baseline_failures: ["FAIL\t./internal/... [setup failed]"]
  - name: quarry-cli-infra
    state: approved
    implementer_session: 9a2aa8ae-ec24-4c3b-abe9-be0c713bb706
    start_sha: fea8b9c277382cef0093641ddc1c9538df159bc0
    commit_sha: af9272b6bdaa8eac47b42b6a9633cf18122692e8
    verify_baseline_failures: ["FAIL\t./internal/... [setup failed]"]
  - name: port-engine
    state: pending
    verify_baseline_failures: ["FAIL\t./... [setup failed]"]
  - name: port-cli
    state: pending
    verify_baseline_failures: ["FAIL\t./... [setup failed]"]
  - name: quarry-live-and-equivalence
    state: pending
    verify_baseline_failures: ["FAIL\t./... [setup failed]"]
  - name: lyx-removal
    state: pending
    verify_baseline_failures: []
```
