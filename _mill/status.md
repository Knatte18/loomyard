# Status

```yaml
phase: approved-batten-wiring
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
approved-shedrun-leaf  '2026-09-19T17:43:20Z'
approved-board-type-field  '2026-09-19T17:46:12Z'
approved-batten-rename  '2026-09-19T18:01:51Z'
approved-loom-run-directory  '2026-09-19T18:21:06Z'
approved-batten-producers  '2026-09-19T18:30:06Z'
approved-batten-wiring  '2026-09-19T18:46:34Z'
```

## Batches

```yaml
batches:
  - name: shedrun-leaf
    state: approved
    implementer_session: 5f6b400d-be0c-40c6-8a82-c320b99f09c9
    start_sha: 40c2d81948c3d9542a1adf853c0abd95ea1a6338
    commit_sha: 7cc03e538e1d81d787274bd467e383684eb84c07
    verify_baseline_failures: ["FAIL\t./internal/shedrun/... [setup failed]"]
  - name: board-type-field
    state: approved
    implementer_session: 368ab9ad-b686-42f9-b03d-a03e39f1e455
    start_sha: 52b9db7971c4d1177a8417a97e2069fb2a5540b9
    commit_sha: 2070ab3f3bd57a0f68d2c5c180672c69c8dee682
    verify_baseline_failures: []
  - name: batten-rename
    state: approved
    implementer_session: 2b87ce82-4eb2-4fb5-9d44-6fbc5804dbd3
    start_sha: e012f1bef06e890fc8b675a106ff3cb861781ffa
    commit_sha: c0ac05af41eeaf1606918e7da4b662fe8ea1e2bd
    verify_baseline_failures: ["FAIL\t./internal/battenshed/... [setup failed]", "FAIL\t./internal/battenrecipe/...\
    \ [setup failed]", "FAIL\t./internal/battencli/... [setup failed]"]
  - name: loom-run-directory
    state: approved
    implementer_session: 060abfe6-f21a-4ddd-a258-b2458c105f00
    start_sha: ddb48667942309459b1923387a4a662464d1687e
    commit_sha: 32cb9ee6641cf71f28481c7c8982a0c4519d3d6d
    verify_baseline_failures: []
  - name: batten-producers
    state: approved
    implementer_session: 9308179b-197c-49c0-8221-49bed03a45a0
    start_sha: 020f47292620705c82427733ef7ea0377b3cbefb
    commit_sha: f1a2c8f5c953936ffc717379eced2b7df544e2dc
    verify_baseline_failures: ["FAIL\t./internal/battenshed/... [setup failed]", "FAIL\t./internal/battenrecipe/...\
    \ [setup failed]"]
  - name: batten-wiring
    state: approved
    implementer_session: c07581ee-2c45-4d91-82e1-2adb29df2066
    start_sha: cbec5b147e90235add56d4e9891a6ec07816b365
    commit_sha: 78c64550acac55aa497481868c6f24002729b8fb
    verify_baseline_failures: ["FAIL\t./internal/battencli/... [setup failed]"]
  - name: shed-addressing
    state: running
    implementer_session: a66a298d-f8e6-441e-8b93-ac5032f9891a
    start_sha: 893e44315ef5cfbecc796b0dc37bdaeff1270350
    verify_baseline_failures: ["FAIL\t./internal/battencli/... [setup failed]"]
  - name: docs-and-integration
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/battencli/... [setup failed]"]
```
