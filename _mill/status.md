# Status

```yaml
phase: implementing
slug: unify-webster-burler-wiring
branch: unify-webster-burler-wiring
plan: _mill/plan
parent: crucible-loom-glyph-hardening
task: Unify webster/burler CLI wiring into a shared module
task_description: |
  Unify webster/burler CLI wiring into a shared module
```

## Timeline

```text
discussing  '2026-09-08T12:26:39Z'
discussion-fix-r2  '2026-09-08T13:08:57Z'
discussed  '2026-09-08T13:08:57Z'
planning  '2026-09-08T13:25:05Z'
plan-review-r1  '2026-09-08T13:32:58Z'
plan-fix-r1  '2026-09-08T13:34:16Z'
plan-review-r2  '2026-09-08T13:39:59Z'
plan-fix-r2  '2026-09-08T13:41:34Z'
plan-review-r3  '2026-09-08T13:48:15Z'
plan-fix-r3  '2026-09-08T13:50:34Z'
plan-review-r4  '2026-09-08T15:01:01Z'
plan-fix-r4  '2026-09-08T15:02:42Z'
plan-review-r5  '2026-09-08T15:09:17Z'
plan-fix-r5  '2026-09-08T15:11:29Z'
planned  '2026-09-08T15:11:38Z'
implementing  '2026-09-08T15:13:45Z'
```

## Batches

```yaml
batches:
  - name: cliwire-package
    state: running
    implementer_session: 579c8889-4612-4a1e-b567-c25da4d77765
    start_sha: 583a0b43ae1cd8e9b22c7f73f2b1f2575e794729
    verify_baseline_failures: ["FAIL\t./internal/cliwire/... [setup failed]"]
  - name: rewire-clis
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/cliwire/... [setup failed]"]
  - name: enforcement-and-docs
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/cliwire/... [setup failed]"]
```
