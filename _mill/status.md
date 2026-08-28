# Status

```yaml
phase: approved-scrollback-backstop-and-composite-smoke
slug: reed-header-pane-boot-noise
branch: reed-header-pane-boot-noise
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'reed: header pane''s boot sometimes leaves shell/log noise in its scrollback'
task_description: |
  reed: header pane's boot sometimes leaves shell/log noise in its scrollback
```

## Timeline

```text
discussing  '2026-08-28T07:45:31Z'
discussion-fix-r1  '2026-08-28T08:10:50Z'
discussion-fix-r5  '2026-08-28T08:29:07Z'
discussed  '2026-08-28T08:29:07Z'
planning  '2026-08-28T08:38:51Z'
plan-review-r1  '2026-08-28T08:46:17Z'
plan-fix-r1  '2026-08-28T08:46:50Z'
plan-review-r2  '2026-08-28T08:52:37Z'
plan-fix-r2  '2026-08-28T08:53:01Z'
plan-review-r3  '2026-08-28T09:01:13Z'
plan-fix-r3  '2026-08-28T09:02:12Z'
plan-review-r4  '2026-08-28T09:09:35Z'
plan-fix-r4  '2026-08-28T09:09:57Z'
planned  '2026-08-28T09:10:12Z'
implementing  '2026-08-28T09:10:37Z'
approved-header-pane-runs-its-own-command  '2026-08-28T09:16:12Z'
approved-header-declines-the-stencil-seed-pass  '2026-08-28T09:21:20Z'
approved-scrollback-backstop-and-composite-smoke  '2026-08-28T09:26:07Z'
```

## Batches

```yaml
batches:
  - name: header-pane-runs-its-own-command
    state: approved
    implementer_session: 533ea1fa-1a09-498d-93f4-2af6b92cc169
    start_sha: 8345d70311519b873ace350435de1276e87fa459
    commit_sha: bef2380d206a0f04ef9153f0016b131ffdcdb73a
    verify_baseline_failures: []
  - name: header-declines-the-stencil-seed-pass
    state: approved
    implementer_session: 3fd11504-86c8-4a3c-8067-820031ea0b1f
    start_sha: 075ffa60665b8192cad8dd0683af06e6a6f0ed00
    commit_sha: a546d2c39cf201fb9c92a8f9b5364609789350af
    verify_baseline_failures: []
  - name: scrollback-backstop-and-composite-smoke
    state: approved
    implementer_session: 9211818a-ceaa-44a7-915e-0a014d59f884
    start_sha: 45abe306a117194352110780ffa87fced911004d
    commit_sha: 46785dfdbed4b10c4c28974fefc04750cb8a366c
    verify_baseline_failures: []
```
