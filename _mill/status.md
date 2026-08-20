# Status

```yaml
phase: approved-preflightshed-package
slug: preflight-loom-agnostic
branch: preflight-loom-agnostic
plan: _mill/plan
parent: main
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
task_description: |
  preflight: split into two Shed rows -- a generic one, and loom's own
```

## Timeline

```text
discussing  '2026-08-20T08:17:14Z'
discussion-fix-r2  '2026-08-20T08:44:34Z'
discussion-fix-r3  '2026-08-20T08:49:32Z'
discussion-fix-r5  '2026-08-20T08:59:33Z'
discussed  '2026-08-20T08:59:33Z'
planning  '2026-08-20T09:18:50Z'
plan-fix-r1  '2026-08-20T09:25:18Z'
planned  '2026-08-20T09:25:28Z'
implementing  '2026-08-20T09:26:03Z'
approved-preflightshed-package  '2026-08-20T09:38:47Z'
```

## Batches

```yaml
batches:
  - name: preflightshed-package
    state: approved
    implementer_session: 82d66d57-4f06-45ae-8964-3c0763176041
    start_sha: f4e2b5a32ee13d9428cd23bc74f064600a0eb8b5
    commit_sha: 00e134b56f93a437dff30314e2a61fc3f4adf7cc
    verify_baseline_failures: []
  - name: loomengine-checkseed
    state: running
    implementer_session: 8c8637a4-4502-4554-95e5-628608416e49
    start_sha: 7656c430c3e1a226224b43666695707fa5740fa5
    verify_baseline_failures: []
  - name: wire-two-rows
    state: pending
    verify_baseline_failures: []
  - name: delete-composite
    state: pending
    verify_baseline_failures: []
  - name: docs
    state: pending
    verify_baseline_failures: []
```
