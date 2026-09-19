# Status

```yaml
phase: implementing
slug: seeded-shed-core
branch: seeded-shed-core
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'Seeded Shed core: run addressing, seed contract, batten'
task_description: |
  Seeded Shed core: run addressing, seed contract, batten
```

## Timeline

```text
discussing  '2026-09-19T16:20:51Z'
discussion-fix-r7  '2026-09-19T17:04:38Z'
discussed  '2026-09-19T17:04:38Z'
planning  '2026-09-19T17:24:06Z'
plan-review-r1  '2026-09-19T17:35:14Z'
plan-fix-r1  '2026-09-19T17:37:26Z'
planned  '2026-09-19T17:37:37Z'
implementing  '2026-09-19T17:37:56Z'
```

## Batches

```yaml
batches:
  - name: shedrun-leaf
    state: running
    implementer_session: 5f6b400d-be0c-40c6-8a82-c320b99f09c9
    start_sha: 40c2d81948c3d9542a1adf853c0abd95ea1a6338
    verify_baseline_failures: ["FAIL\t./internal/shedrun/... [setup failed]"]
  - name: board-type-field
    state: pending
    verify_baseline_failures: []
  - name: batten-rename
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/battenshed/... [setup failed]", "FAIL\t./internal/battenrecipe/...\
    \ [setup failed]", "FAIL\t./internal/battencli/... [setup failed]"]
  - name: loom-run-directory
    state: pending
    verify_baseline_failures: []
  - name: batten-producers
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/battenshed/... [setup failed]", "FAIL\t./internal/battenrecipe/...\
    \ [setup failed]"]
  - name: batten-wiring
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/battencli/... [setup failed]"]
  - name: shed-addressing
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/battencli/... [setup failed]"]
  - name: docs-and-integration
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/battencli/... [setup failed]"]
```
