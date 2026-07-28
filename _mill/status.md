# Status

```yaml
phase: approved-selfreport-transport
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
approved-githubclient  '2026-07-28T11:55:44Z'
approved-parity-oracle  '2026-07-28T12:36:58Z'
approved-selfreport-transport  '2026-07-28T17:15:43Z'
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
    state: approved
    implementer_session: a7f0765e-de94-4bb2-8acf-10053a2c131e
    start_sha: 390de36275247311965761a37c6e8731f35bb73e
    commit_sha: 1b29d86f5cf93ecaa7610c7edafe963fad91ca74
  - name: parity-oracle
    state: approved
    implementer_session: 0c8bd158-ad12-41b6-912b-e7d937547b91
    start_sha: 6d03d2c41cf0170636bebc8f3b5f7600db65e865
    commit_sha: 7b9df6499f3120cf6da1733089b696e77e9d39ff
  - name: selfreport-transport
    state: approved
    implementer_session: 2b7eda9e-202c-4f70-ab23-704e3220099a
    start_sha: 1a375a97cf9d6bc8f1660bbb744edf8c56ea3a41
    commit_sha: 12aee0e3bdcd907d176c7c8840c3127f66d5c2b4
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
