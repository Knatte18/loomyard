# Status

```yaml
phase: approved-shedbuild-loader
slug: shed-recipe-loader-builder
branch: shed-recipe-loader-builder
plan: _mill/plan
parent: main
task: 'Shed recipe: loader/builder'
task_description: |
  Shed recipe: loader/builder
```

## Timeline

```text
discussing  '2026-08-21T11:12:31Z'
discussion-fix-r3  '2026-08-21T11:53:36Z'
discussion-fix-r4  '2026-08-21T11:59:31Z'
discussed  '2026-08-21T11:59:31Z'
planning  '2026-08-21T12:09:05Z'
plan-review-r1  '2026-08-21T12:20:05Z'
plan-fix-r1  '2026-08-21T12:20:05Z'
plan-fix-r2  '2026-08-21T12:28:47Z'
planned  '2026-08-21T12:28:57Z'
implementing  '2026-08-21T12:29:24Z'
approved-shedbuild-loader  '2026-08-21T12:34:23Z'
```

## Batches

```yaml
batches:
  - name: shedbuild-loader
    state: approved
    implementer_session: 750264cd-b503-43b8-ba0b-278777b45fa9
    start_sha: 5f76c0cb4f30e5158c4be145c3eb25d08d25ea58
    commit_sha: 8cf8974c59f45cbd10fa1b8a3aead1f152376807
    verify_baseline_failures: ["FAIL\t./internal/shedbuild/... [setup failed]"]
  - name: shedbuild-builder
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/shedbuild/... [setup failed]"]
  - name: loom-equivalence
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/shedbuild/... [setup failed]"]
  - name: docs
    state: pending
```
