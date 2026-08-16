# Status

```yaml
phase: approved-webster-producer
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
approved-package-foundation-and-singlellm  '2026-08-16T15:43:53Z'
approved-perch-producer  '2026-08-16T15:49:34Z'
approved-webster-producer  '2026-08-16T15:53:08Z'
```

## Batches

```yaml
batches:
  - name: package-foundation-and-singlellm
    state: approved
    implementer_session: 674747a3-9492-4890-82af-d8770073d066
    start_sha: 5e5c8ab0e2ee403d777ed0ec60f8b8b5aba0608f
    commit_sha: 8a816d9f3d342d038d64ed6f83fc91ce62725353
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
  - name: perch-producer
    state: approved
    implementer_session: be870bd6-8ef0-45c6-9c16-ddaf3b333a66
    start_sha: 99dae15911c5e500e56a9ef24fe1d7fdc8a4b042
    commit_sha: 357337e6279e5db7ff3fec9f5e1e1593a7abee80
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
  - name: webster-producer
    state: approved
    implementer_session: 0f89475b-2048-4d93-acd7-09d3c8badf75
    start_sha: 9b7ad49be10add09ef89708023292b8042641309
    commit_sha: 90b4406ab71e300f32526a7720b4d9f133f73a11
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
  - name: docs
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/shedadapters/... [setup failed]"]
```
