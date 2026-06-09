# Status

```yaml
phase: approved-internal/git — extract RunGit
slug: extract-shared-infra
branch: extract-shared-infra
plan: _mill/plan
parent: main
task: Extract shared infrastructure (config, git, lock)
task_description: |
  Extract shared infrastructure (config, git, lock)
```

## Timeline

```text
discussing  '2026-06-09T12:01:38Z'
discussion-fix-r4  '2026-06-09T12:37:34Z'
discussed  '2026-06-09T12:38:20Z'
planning  '2026-06-09T12:52:13Z'
plan-fix-r2  '2026-06-09T18:07:41Z'
plan-fix-r3  '2026-06-09T18:16:39Z'
plan-review-r4  '2026-06-09T18:21:31Z'
implementing  '2026-06-09T18:23:54Z'
approved-internal/lock — lift lock primitives  '2026-06-09T18:32:14Z'
approved-internal/git — extract RunGit  '2026-06-09T18:35:37Z'
```

## Batches

```yaml
batches:
  - name: internal/lock — lift lock primitives
    state: approved
    implementer_session: 40613790-1c94-4b85-bfce-3c3c6d8b4932
    start_sha: 2fb2c278542441edf339393782530efcb43f0936
    commit_sha: db061781d8078364ecd19cab1790aef9a693b4ad
  - name: internal/git — extract RunGit
    state: approved
    implementer_session: 0c2ad23c-2826-4fca-a8d2-7b8f01d76a88
    start_sha: f0c0b9e3228c42bd727713a9ea1fd294c5c90e87
    commit_sha: 844be1b5aaab0c2654f00b931dcc54aea6435932
  - name: internal/config — generic config loader
    state: pending
  - name: board migration — adopt all three packages
    state: pending
```
