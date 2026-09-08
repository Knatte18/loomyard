# Status

```yaml
phase: approved-enforcement-and-docs
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
approved-cliwire-package  '2026-09-08T15:23:17Z'
approved-rewire-clis  '2026-09-08T15:33:20Z'
approved-enforcement-and-docs  '2026-09-08T15:37:22Z'
```

## Batches

```yaml
batches:
  - name: cliwire-package
    state: approved
    implementer_session: 579c8889-4612-4a1e-b567-c25da4d77765
    start_sha: 583a0b43ae1cd8e9b22c7f73f2b1f2575e794729
    commit_sha: 3473b47cb014b74165a8c72cc56d65c0e4e91690
    verify_baseline_failures: ["FAIL\t./internal/cliwire/... [setup failed]"]
  - name: rewire-clis
    state: approved
    implementer_session: 040e54d8-bc41-41aa-85ec-b1e5d86f3044
    start_sha: eeabc7435a93fb3f303a2e10e7d440615eb882ff
    commit_sha: 47e0f1a9ff6288b39460dc56b8079345acedfa1d
    verify_baseline_failures: ["FAIL\t./internal/cliwire/... [setup failed]"]
  - name: enforcement-and-docs
    state: approved
    implementer_session: 5e64eb22-9d7a-4e4f-81f0-17d2649de026
    start_sha: d1114d4c4a7180d7f6636b3b49c0bf31c7ed0291
    commit_sha: 38988c78a4e94cb468781e00dde141c38061a448
    verify_baseline_failures: ["FAIL\t./internal/cliwire/... [setup failed]"]
```
