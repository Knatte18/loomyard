# Status

```yaml
phase: approved-gogit-handle
slug: native-clients-migration
branch: native-clients-migration
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
task_description: |
  native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github
```

## Timeline

```text
discussing  '2026-07-28T06:03:05Z'
discussion-fix-r6  '2026-07-28T08:07:34Z'
discussed  '2026-07-28T08:07:34Z'
planning  '2026-07-28T09:14:24Z'
plan-review-r1  '2026-07-28T09:24:57Z'
plan-fix-r1  '2026-07-28T09:24:57Z'
plan-review-r2  '2026-07-28T09:38:25Z'
plan-fix-r2  '2026-07-28T09:38:25Z'
plan-review-r3  '2026-07-28T09:53:42Z'
plan-fix-r3  '2026-07-28T09:53:42Z'
plan-review-r4  '2026-07-28T10:06:50Z'
plan-fix-r4  '2026-07-28T10:06:50Z'
plan-fix-r5  '2026-07-28T10:22:03Z'
planned  '2026-07-28T10:22:16Z'
implementing  '2026-07-28T10:29:07Z'
approved-gogit-handle  '2026-07-28T11:24:47Z'
```

## Batches

```yaml
batches:
  - name: gogit-handle
    state: approved
    implementer_session: 74ca48e2-6f77-4579-bbae-3304655890eb
    start_sha: d3c8828308d1a7712dbfd57b03906a22a1853233
    commit_sha: a29a5ead4d4a6ed370559c25928c5fa613d3014d
  - name: githubclient
    state: pending
  - name: parity-oracle
    state: pending
  - name: selfreport-transport
    state: pending
  - name: migrate-core-reads
    state: pending
  - name: migrate-snapshot-push-reads
    state: pending
  - name: retire-poc-and-measure
    state: pending
  - name: guards
    state: pending
  - name: docs-and-invariants
    state: pending
```
