# Status

```yaml
phase: holistic-reviewing
slug: planparser-card-format-migration
branch: planparser-card-format-migration
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: Migrate planparser.Card to Edits/Uses fields
task_description: |
  Migrate planparser.Card to Edits/Uses fields
```

## Timeline

```text
discussing  '2026-08-24T10:32:44Z'
discussion-fix-r4  '2026-08-24T12:39:00Z'
discussion-fix-r5  '2026-08-24T12:43:42Z'
discussed  '2026-08-24T12:43:42Z'
planning  '2026-08-24T12:57:58Z'
plan-fix-r1  '2026-08-24T13:06:10Z'
plan-review-r2  '2026-08-24T13:15:52Z'
plan-fix-r2  '2026-08-24T13:15:52Z'
plan-review-r3  '2026-08-24T13:24:50Z'
planned  '2026-08-24T13:24:59Z'
implementing  '2026-08-24T13:25:30Z'
approved-planparser core — format-4 model, classifier, parser, validator  '2026-08-24T13:34:38Z'
approved-planparser tests — golden fixture and per-check suite  '2026-08-24T13:47:22Z'
approved-consumer fixtures — websterengine, loomshed, loomrecipe, loomcli, webstercli  '2026-08-24T13:52:51Z'
approved-docs — spec rewrite, stencils, sandbox suite, stale figures, roadmap  '2026-08-24T14:05:04Z'
holistic-reviewing  '2026-08-24T14:05:36Z'
```

## Batches

```yaml
batches:
  - name: planparser core — format-4 model, classifier, parser, validator
    state: approved
    implementer_session: b29f03b4-49fb-4ec6-ba99-3b8ed5b6ab3c
    start_sha: ac68afb6a56e55e6bed60719cda579977fb01ca9
    commit_sha: 9ea9289b636676e557c99664cfb853436045c0c4
    verify_baseline_failures: []
  - name: planparser tests — golden fixture and per-check suite
    state: approved
    implementer_session: 98beb5de-4a41-4ae2-bcba-e68ae16d3189
    start_sha: 83a6eb75430584cd42f4b050c9ecc2d3b8fb2128
    commit_sha: fee4e6543d7bfdd41c11deb729e795a600270a9d
    verify_baseline_failures: []
  - name: consumer fixtures — websterengine, loomshed, loomrecipe, loomcli, webstercli
    state: approved
    implementer_session: ae0a3eb6-97fe-468a-8d97-0ce6e04bee32
    start_sha: c008e1b6eb10332ed15284e7b25c1b40a4689b2e
    commit_sha: 30f780c2bff90dbe263b3c0bf993ab7940b62995
    verify_baseline_failures: []
  - name: docs — spec rewrite, stencils, sandbox suite, stale figures, roadmap
    state: approved
    implementer_session: af9a2a28-ba24-49ca-835d-74db90f1d6be
    start_sha: c6e8dd7f8d0bc87c5fc8a0588d8d817f20b794d9
    commit_sha: a583f35c0cce733853a177c0a89469147711a998
    verify_baseline_failures: []
```
