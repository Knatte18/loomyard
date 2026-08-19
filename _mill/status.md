# Status

```yaml
phase: approved-merge-stage-resolved verb
slug: landing-publish-finalize-producers
branch: landing-publish-finalize-producers
plan: _mill/plan
parent: standalone-producers
module_verify_baseline: clean
task: 'landing: Publish + Finalize producers'
task_description: |
  landing: Publish + Finalize producers
```

## Timeline

```text
discussing  '2026-08-19T11:36:08Z'
discussion-fix-r6  '2026-08-19T12:48:02Z'
discussed  '2026-08-19T12:48:02Z'
planning  '2026-08-19T13:02:07Z'
plan-review-r1  '2026-08-19T13:14:41Z'
plan-fix-r1  '2026-08-19T13:14:41Z'
plan-review-r2  '2026-08-19T17:37:45Z'
plan-fix-r2  '2026-08-19T17:37:45Z'
plan-fix-r3  '2026-08-19T17:46:47Z'
planned  '2026-08-19T17:46:59Z'
implementing  '2026-08-19T18:25:51Z'
approved-merge-stage-resolved verb  '2026-08-19T18:33:51Z'
```

## Batches

```yaml
batches:
  - name: merge-stage-resolved verb
    state: approved
    implementer_session: 5b55eff4-e53b-40d8-959f-41bccd8f2cdf
    start_sha: 55db31690ef6461c9fd6ee58f620afc369bb0829
    commit_sha: cb3289108dbbc78280db30deb7d52b4d8e60ea6e
    verify_baseline_failures: []
  - name: remote and push helpers
    state: pending
    verify_baseline_failures: []
  - name: mergeresolve engine
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/mergeresolve/... [setup failed]"]
  - name: landingshed producers
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/landingshed/... [setup failed]"]
  - name: loomshed wiring and integration
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/landingshed/... [setup failed]"]
  - name: documentation lifecycle
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/landingshed/... [setup failed]", "FAIL\t./internal/mergeresolve/...\
    \ [setup failed]"]
```
