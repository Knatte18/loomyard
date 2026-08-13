# Status

```yaml
phase: approved-reedcli
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
approved-fabrictest dissolution  '2026-08-13T07:32:23Z'
approved-hubforge factory  '2026-08-13T07:46:27Z'
approved-small consumers  '2026-08-13T08:03:57Z'
approved-reedcli  '2026-08-13T08:10:57Z'
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
    state: approved
    implementer_session: 3e2fd3df-1c0d-413a-abfa-e3c01e2ee5f4
    start_sha: 27dd30effdd8f1304b2b8ed9d915a7e123e64c52
    commit_sha: e0f8744da6820fb917bd01e07ba5ae3724fb6005
    verify_baseline_failures: ["FAIL\t./internal/hubforge/... [setup failed]"]
  - name: hubforge factory
    state: approved
    implementer_session: 3edd2be4-d653-49c7-a495-3659c91b91eb
    start_sha: d9af84b9c5e209adb7fbc24204cfc81c8987ac46
    commit_sha: 9b4fee50b6cdc8c9abd03b4976217bd39dbec333
    verify_baseline_failures: ["FAIL\t./internal/hubforge/... [setup failed]"]
  - name: small consumers
    state: approved
    implementer_session: cf904daf-e434-4041-ba60-f98402ad3997
    start_sha: e1823e94bad46cbe4081a1f5bd865dffa49c977f
    commit_sha: 1a267992ea30a163b3e8b31f32def52f19ec6830
    verify_baseline_failures: []
  - name: reedcli
    state: approved
    implementer_session: 638feca1-b9ba-45e9-9dfa-2e1e94034174
    start_sha: e114ddc4538eba0d6d29dcc0b06b4eed6bc65b59
    commit_sha: f80cb3f6f4701ce78f21701887cdbee254cbd8a1
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
