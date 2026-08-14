# Status

```yaml
phase: approved-stencils-package-and-loom
slug: stencils-directory-reorg
branch: stencils-directory-reorg
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: Relocate producer prompt files into a stencils/ directory
task_description: |
  Relocate producer prompt files into a stencils/ directory
```

## Timeline

```text
discussing  '2026-08-14T07:40:14Z'
discussed  '2026-08-14T10:46:07Z'
planning  '2026-08-14T11:04:14Z'
plan-review-r1  '2026-08-14T11:18:04Z'
plan-fix-r1  '2026-08-14T11:18:04Z'
plan-review-r2  '2026-08-14T11:30:48Z'
plan-fix-r2  '2026-08-14T11:30:48Z'
plan-fix-r3  '2026-08-14T11:40:21Z'
planned  '2026-08-14T11:40:30Z'
implementing  '2026-08-14T11:40:58Z'
approved-stencilstore-foundation  '2026-08-14T11:49:49Z'
approved-stencils-package-and-loom  '2026-08-14T11:56:36Z'
```

## Batches

```yaml
batches:
  - name: stencilstore-foundation
    state: approved
    implementer_session: 2a017dfc-d07f-4960-b0dd-ad31caf3b990
    start_sha: 12794a23d95be890edc84b13e0de9a4a0e9013c6
    commit_sha: ca1e65acc860f90163350b2f279c2899345ce02f
    verify_baseline_failures: ["FAIL\t./internal/stencilstore/... [setup failed]"]
  - name: stencils-package-and-loom
    state: approved
    implementer_session: 1ff45c71-97e0-45f7-88fd-87d48304f8e9
    start_sha: 3102a8e8a90f0c56e0cae8e92501c444165e3678
    commit_sha: 41e1ff0db67fa67866289ddf0b310980b20b9820
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: seeding-trigger
    state: pending
    verify_baseline_failures: []
  - name: burler-runtime-read
    state: pending
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: diff-base-recovery
    state: pending
    verify_baseline_failures: []
  - name: treadle-runtime-read
    state: pending
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: webster-runtime-read
    state: pending
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: stencil-cli
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/stencilcli/... [setup failed]"]
  - name: reed-rename-and-docs
    state: pending
    verify_baseline_failures: []
```
