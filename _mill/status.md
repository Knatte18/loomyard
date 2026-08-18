# Status

```yaml
phase: approved-buildinfo-and-mode-mapping
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
    state: running
    implementer_session: a851199e-5f44-487f-b5e7-642dac3312fe
    start_sha: ca62cf3ae287bcfad712de22065c859633daaedc
    verify_baseline_failures: ["FAIL\t./internal/standalonestate/... [setup failed]"]
  - name: preflight-lift
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/preflight/... [setup failed]"]
  - name: cli-gate-and-ldflags
    state: pending
    verify_baseline_failures: []
  - name: docs-and-invariants
    state: pending
    verify_baseline_failures: []
```
