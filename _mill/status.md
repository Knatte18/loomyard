# Status

```yaml
phase: approved-docs
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
approved-shedbuild-builder  '2026-08-21T12:39:10Z'
approved-loom-equivalence  '2026-08-21T12:42:01Z'
approved-docs  '2026-08-21T12:46:52Z'
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
    state: approved
    implementer_session: 5959c856-95af-41b2-8ea9-d8d68de73284
    start_sha: d4b85bd9215b64853c461aff1bab21298b8f3750
    commit_sha: 24d7b8f3c11b66288f3091102c9b3af41ab39ff2
    verify_baseline_failures: ["FAIL\t./internal/shedbuild/... [setup failed]"]
  - name: loom-equivalence
    state: approved
    implementer_session: be0e3780-0659-4685-b95f-318afbbc9c4c
    start_sha: 05f7d6a36e3e7ca35a3ba8c614e4e8cd854254f8
    commit_sha: d8f9a6d9a0ff58ca697fb42526b9f1564abf1a10
    verify_baseline_failures: ["FAIL\t./internal/shedbuild/... [setup failed]"]
  - name: docs
    state: approved
    implementer_session: 5d3dd1a1-aa2c-43a0-92c2-f1cd8b3e958e
    start_sha: 598926cba5336dade52aff7abf71aaff79785a84
    commit_sha: 48ba5cdf4c758eb889d792c79591e25965f59856
```
