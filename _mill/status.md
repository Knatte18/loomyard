# Status

```yaml
phase: approved-gitkit leaf
slug: lyxtest-real-hubs
branch: lyxtest-real-hubs
plan: _mill/plan
parent: main
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
task_description: |
  lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency
```

## Timeline

```text
discussing  '2026-08-12T12:12:27Z'
discussion-fix-r3  '2026-08-12T17:42:51Z'
discussed  '2026-08-12T17:51:13Z'
planning  '2026-08-12T18:19:43Z'
plan-fix-r1  '2026-08-13T05:48:32Z'
plan-review-r2  '2026-08-13T05:56:40Z'
plan-fix-r2  '2026-08-13T05:58:31Z'
plan-review-r3  '2026-08-13T06:07:47Z'
plan-fix-r3  '2026-08-13T06:08:54Z'
plan-review-r4  '2026-08-13T06:18:03Z'
plan-fix-r4  '2026-08-13T06:20:28Z'
plan-review-r5  '2026-08-13T06:28:34Z'
plan-fix-r5  '2026-08-13T06:29:20Z'
plan-review-r6  '2026-08-13T06:39:22Z'
plan-fix-r6  '2026-08-13T06:40:57Z'
plan-review-r7  '2026-08-13T06:49:26Z'
plan-fix-r7  '2026-08-13T06:50:16Z'
blocked  '2026-08-13T06:50:48Z'
planned  '2026-08-13T06:56:36Z'
implementing  '2026-08-13T07:02:52Z'
approved-gitkit leaf  '2026-08-13T07:18:31Z'
```

## Batches

```yaml
batches:
  - name: gitkit leaf
    state: approved
    implementer_session: 4337e8c3-0904-4691-887b-42eae518bef3
    start_sha: e74a0dca4202934d7d95f834e4d2e4eb5b08ad37
    commit_sha: ffcf226a3dc9cf306439658ff9d46f6cfc10fd0a
    verify_baseline_failures: ["FAIL\t./internal/gitkit/... [setup failed]"]
  - name: fabrictest dissolution
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/hubforge/... [setup failed]"]
  - name: hubforge factory
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/hubforge/... [setup failed]"]
  - name: small consumers
    state: pending
    verify_baseline_failures: []
  - name: reedcli
    state: pending
    verify_baseline_failures: []
  - name: stuck packages
    state: pending
    verify_baseline_failures: []
  - name: fabriccli
    state: pending
    verify_baseline_failures: []
  - name: fabricengine external
    state: pending
    verify_baseline_failures: []
  - name: fabricengine in-package weft
    state: pending
    verify_baseline_failures: []
  - name: fabricengine in-package hub
    state: pending
    verify_baseline_failures: []
  - name: helper deletion
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/gitkit/... [setup failed]"]
  - name: docs
    state: pending
    verify_baseline_failures: []
```
