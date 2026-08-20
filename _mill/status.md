# Status

```yaml
phase: implementing
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
```

## Batches

```yaml
batches:
  - name: quarry-scaffold
    state: running
    implementer_session: 9a65255d-f1ea-4cdb-ae3d-474445656a9b
    start_sha: 7cd73c16f733288f0cbddcd4f30275855e59b3ff
    verify_baseline_failures: ["FAIL\t./internal/... [setup failed]"]
  - name: quarry-cli-infra
    state: pending
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
