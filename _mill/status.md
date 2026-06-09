# Status

```yaml
phase: implementing
slug: extract-shared-infra
branch: extract-shared-infra
plan: _mill/plan
parent: main
task: Extract shared infrastructure (config, git, lock)
task_description: |
  Extract shared infrastructure (config, git, lock)
```

## Timeline

```text
discussing  '2026-06-09T12:01:38Z'
discussion-fix-r4  '2026-06-09T12:37:34Z'
discussed  '2026-06-09T12:38:20Z'
planning  '2026-06-09T12:52:13Z'
plan-fix-r2  '2026-06-09T18:07:41Z'
plan-fix-r3  '2026-06-09T18:16:39Z'
plan-review-r4  '2026-06-09T18:21:31Z'
implementing  '2026-06-09T18:23:54Z'
```

## Batches

```yaml
batches:
  - name: internal/lock 🡒 lift lock primitives
    state: pending
  - name: internal/git 🡒 extract RunGit
    state: pending
  - name: internal/config 🡒 generic config loader
    state: pending
  - name: board migration 🡒 adopt all three packages
    state: pending
```
