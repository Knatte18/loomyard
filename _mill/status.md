# Status

```yaml
phase: approved-cli-gate-and-ldflags
slug: orchestrator-preflight
branch: orchestrator-preflight
plan: _mill/plan
parent: standalone-producers
module_verify_baseline: clean
task: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations
task_description: |
  lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations
```

## Timeline

```text
discussing  '2026-08-18T05:10:52Z'
discussion-fix-r3  '2026-08-18T05:49:17Z'
discussion-fix-r4  '2026-08-18T05:55:18Z'
discussion-fix-r5  '2026-08-18T06:00:16Z'
discussed  '2026-08-18T06:00:16Z'
planning  '2026-08-18T06:10:34Z'
plan-review-r1  '2026-08-18T06:18:26Z'
plan-fix-r1  '2026-08-18T06:18:26Z'
plan-review-r2  '2026-08-18T06:28:40Z'
plan-fix-r2  '2026-08-18T06:28:40Z'
plan-fix-r3  '2026-08-18T06:42:02Z'
planned  '2026-08-18T06:42:27Z'
implementing  '2026-08-18T06:42:59Z'
approved-buildinfo-and-mode-mapping  '2026-08-18T06:47:18Z'
approved-standalonestate-leaf  '2026-08-18T06:51:54Z'
approved-preflight-lift  '2026-08-18T07:00:55Z'
approved-cli-gate-and-ldflags  '2026-08-18T07:07:34Z'
```

## Batches

```yaml
batches:
  - name: buildinfo-and-mode-mapping
    state: approved
    implementer_session: c2cc5283-9aca-4b51-942a-f5080a6e2e7d
    start_sha: 9b0654f242cad5077656ea17876a5cf3ef6bc737
    commit_sha: 50eda5d3f32aa11d73d532dfae554fba855880ef
    verify_baseline_failures: ["FAIL\t./internal/buildinfo/... [setup failed]"]
  - name: standalonestate-leaf
    state: approved
    implementer_session: a851199e-5f44-487f-b5e7-642dac3312fe
    start_sha: ca62cf3ae287bcfad712de22065c859633daaedc
    commit_sha: 7ef3b358fc4a95abcf0100ec06422f9ac2845600
    verify_baseline_failures: ["FAIL\t./internal/standalonestate/... [setup failed]"]
  - name: preflight-lift
    state: approved
    implementer_session: 409ad931-50d5-4919-86b7-cef99c32dce6
    start_sha: 90f2576ba6c99f81ca34af02d795be83cd5fd2e4
    commit_sha: e9b2c73046e6316a671d74e469a19b3c83ff6eec
    verify_baseline_failures: ["FAIL\t./internal/preflight/... [setup failed]"]
  - name: cli-gate-and-ldflags
    state: approved
    implementer_session: d8ccbde8-516a-4a55-8ee7-3674b8b88b3b
    start_sha: 85cdf298d0ed95a53a824fa2a300bb8aeddfa763
    commit_sha: 423f595de7894bf9042103fce23f4c96511545c1
    verify_baseline_failures: []
  - name: docs-and-invariants
    state: running
    implementer_session: 2865451f-72d1-4d94-828a-b7ac81b053bd
    start_sha: 426c05bbf60587409b03affc6b33aa07946a5fc1
    verify_baseline_failures: []
```
