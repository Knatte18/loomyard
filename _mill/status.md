# Status

```yaml
phase: holistic-fixing
slug: scout-extract-standalone-repo
branch: scout-extract-standalone-repo
plan: _mill/plan
parent: main
task: Extract scout into its own standalone repo
task_description: |
  Extract scout into its own standalone repo
```

## Timeline

```text
discussing  '2026-08-20T08:21:22Z'
discussion-fix-r4  '2026-08-20T09:36:46Z'
discussion-fix-r5  '2026-08-20T09:47:18Z'
discussed  '2026-08-20T09:47:18Z'
planning  '2026-08-20T10:02:59Z'
plan-review-r1  '2026-08-20T10:41:43Z'
plan-fix-r1  '2026-08-20T10:41:43Z'
plan-fix-r2  '2026-08-20T11:27:00Z'
plan-fix-r3  '2026-08-20T11:48:03Z'
planned  '2026-08-20T11:48:13Z'
implementing  '2026-08-20T14:50:54Z'
approved-quarry-scaffold  '2026-08-20T14:58:14Z'
approved-quarry-cli-infra  '2026-08-20T15:05:26Z'
approved-port-engine  '2026-08-20T15:25:42Z'
approved-port-cli  '2026-08-20T15:41:15Z'
self-resolved-verify-logic  '2026-08-20T16:03:38Z'
approved-quarry-live-and-equivalence  '2026-08-20T16:07:38Z'
approved-lyx-removal  '2026-08-20T16:30:20Z'
holistic-reviewing  '2026-08-20T16:30:49Z'
holistic-fixing  '2026-08-20T16:40:06Z'
holistic-reviewing  '2026-08-20T16:50:47Z'
holistic-fixing  '2026-08-20T16:59:40Z'
```

## Batches

```yaml
batches:
  - name: quarry-scaffold
    state: approved
    implementer_session: 9a65255d-f1ea-4cdb-ae3d-474445656a9b
    start_sha: 7cd73c16f733288f0cbddcd4f30275855e59b3ff
    commit_sha: b0c261b3870e9a1f76553ef0837d5da4169ac381
    verify_baseline_failures: ["FAIL\t./internal/... [setup failed]"]
  - name: quarry-cli-infra
    state: approved
    implementer_session: 9a2aa8ae-ec24-4c3b-abe9-be0c713bb706
    start_sha: fea8b9c277382cef0093641ddc1c9538df159bc0
    commit_sha: af9272b6bdaa8eac47b42b6a9633cf18122692e8
    verify_baseline_failures: ["FAIL\t./internal/... [setup failed]"]
  - name: port-engine
    state: approved
    implementer_session: 026b10f3-ca8d-4b6a-9d11-a21e112d6776
    start_sha: 34f8b39c1ce40f5e8bf258245160bc03471ed4ae
    commit_sha: c63cd94db7580bea01c5240bafb0549a4fb2f178
    verify_baseline_failures: ["FAIL\t./... [setup failed]"]
  - name: port-cli
    state: approved
    implementer_session: 8cb3cacf-0fad-42b9-b600-0331b982e502
    start_sha: 334c6b15b76e51c7248362ccf370c5fe6ae640fe
    commit_sha: 93265ae6c84236e683b25415c68f26e63ffa65ff
    verify_baseline_failures: ["FAIL\t./... [setup failed]"]
  - name: quarry-live-and-equivalence
    state: approved
    implementer_session: 3af5617f-662c-43d3-a42e-d43af5e81b86
    start_sha: 0875403df4c6020751f0037aa106f8a1c71cfa5e
    commit_sha: 7b5d4ce24a208858328d823b0c9ddd226f549637
    verify_baseline_failures: ["FAIL\t./... [setup failed]"]
  - name: lyx-removal
    state: approved
    implementer_session: 1338f62a-0db5-4442-89b8-0e223ad92d73
    start_sha: 74abbc3a7ff0f19f570c032e6e186b72b5d73fc8
    commit_sha: d6671a4f35a03449f3ce169bcf18b2032d148635
    verify_baseline_failures: []
```
