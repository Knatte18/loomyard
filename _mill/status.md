# Status

```yaml
phase: implementing
slug: shed-adapters
branch: shed-adapters
plan: _mill/plan
parent: main
task: 'Shed engine adapters: SingleLLMProducer, perch, Webster'
task_description: |
  Shed engine adapters: SingleLLMProducer, perch, Webster
```

## Timeline

```text
discussing  '2026-08-16T14:00:14Z'
discussed  '2026-08-16T15:09:07Z'
planning  '2026-08-16T15:17:43Z'
plan-fix-r1  '2026-08-16T15:29:08Z'
plan-review-r2  '2026-08-16T15:37:50Z'
planned  '2026-08-16T15:37:58Z'
implementing  '2026-08-16T15:38:22Z'
```

## Batches

```yaml
batches:
  - name: package-foundation-and-singlellm
    state: running
    implementer_session: 674747a3-9492-4890-82af-d8770073d066
    start_sha: 5e5c8ab0e2ee403d777ed0ec60f8b8b5aba0608f
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
  - name: perch-producer
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
  - name: webster-producer
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
  - name: docs
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
```
