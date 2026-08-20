# Status

```yaml
phase: approved-wire-two-rows
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
approved-loomengine-checkseed  '2026-08-20T09:45:26Z'
approved-wire-two-rows  '2026-08-20T09:54:31Z'
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
    state: approved
    implementer_session: 8c8637a4-4502-4554-95e5-628608416e49
    start_sha: 7656c430c3e1a226224b43666695707fa5740fa5
    commit_sha: a07d1f4e724fb04497ba22c947deb52483b3adcb
    verify_baseline_failures: []
  - name: wire-two-rows
    state: approved
    implementer_session: cc9ab4f4-f932-418f-beb6-09b297ef73f0
    start_sha: 71e3e30013d4efc5f28cc5b54af6f8ebfcbc6a06
    commit_sha: 39a42c6ad748df25fef68aca9b87661d080e77e9
    verify_baseline_failures: []
  - name: delete-composite
    state: running
    implementer_session: cc984fe6-48d0-43ad-8a45-9a95778d75be
    start_sha: 070e82e89c9fa7fdfed0325327cc54d80d2d5db1
    verify_baseline_failures: []
  - name: docs
    state: pending
    verify_baseline_failures: []
```
