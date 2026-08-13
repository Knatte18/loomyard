# Status

```yaml
phase: holistic-reviewing
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
approved-stuck packages  '2026-08-13T08:17:02Z'
approved-fabriccli  '2026-08-13T08:27:57Z'
approved-fabricengine external  '2026-08-13T09:08:41Z'
approved-fabricengine in-package weft  '2026-08-13T09:42:26Z'
approved-fabricengine in-package hub  '2026-08-13T09:50:34Z'
approved-helper deletion  '2026-08-13T10:17:11Z'
approved-docs  '2026-08-13T10:23:42Z'
holistic-reviewing  '2026-08-13T10:24:11Z'
holistic-fixing  '2026-08-13T10:29:45Z'
holistic-reviewing  '2026-08-13T10:36:20Z'
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
    state: approved
    implementer_session: a63cb2f0-095b-473c-af35-2b13172e53fd
    start_sha: 78a76ec113498fc91dfcd3fe3424dec3f4247cc9
    commit_sha: 119bf2e67e017c85ec1b1aaa14848254373d5571
    verify_baseline_failures: []
  - name: fabriccli
    state: approved
    implementer_session: 0d815f62-7fd1-419b-9b01-fc27654edf2f
    start_sha: d793fc54a786625392f31ff8dd889993b0d52312
    commit_sha: aa906bd38e6aa1127274ab8850f2b286b893a987
    verify_baseline_failures: []
  - name: fabricengine external
    state: approved
    implementer_session: c21005d1-ed16-4ae6-b04b-e9690c79009a
    start_sha: fd8651f1abc8aca4dedb09d263ec063e71620f26
    commit_sha: 5664d9a6dc053fb14f14993497cae60d55be3ad6
    verify_baseline_failures: []
  - name: fabricengine in-package weft
    state: approved
    implementer_session: 2be0f014-4923-47e3-8df4-d00d70a67618
    start_sha: 6353b5a24ffa4d2a3f6a686e3898f977e0277703
    commit_sha: 36fa7fc1bd339a1bfdd3351b9296e4944cb33af7
    verify_baseline_failures: []
  - name: fabricengine in-package hub
    state: approved
    implementer_session: dedf8497-38b3-4e8e-8f6e-99e090dc99e0
    start_sha: c2b7e98fd7ff66fb734918cb85186705d71c358e
    commit_sha: ebd7b3934dbe28e7a88c38d6de795c0c869b2047
    verify_baseline_failures: []
  - name: helper deletion
    state: approved
    implementer_session: 3702e816-7176-4112-9f06-ecdc59995f72
    start_sha: f0d6caa6eb5e987a8983658abc7602cf60df10bc
    commit_sha: 8a3694f042b739bc8a6a0ca4c40cb2c9e235af4b
    verify_baseline_failures: ["FAIL\t./internal/gitkit/... [setup failed]"]
  - name: docs
    state: approved
    implementer_session: 76816d8d-6454-4452-829a-82bd17b2d49f
    start_sha: 7a07496700f4eba4df569200d717a9a6bd368821
    commit_sha: 2ca3b70b3b0e2e35bd2acad5d105553b770d9586
    verify_baseline_failures: []
```
