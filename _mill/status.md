# Status

```yaml
phase: holistic-fixing
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
approved-remote and push helpers  '2026-08-19T18:40:56Z'
approved-mergeresolve engine  '2026-08-19T18:50:27Z'
approved-landingshed producers  '2026-08-19T19:06:48Z'
approved-loomshed wiring and integration  '2026-08-19T19:27:33Z'
approved-documentation lifecycle  '2026-08-19T19:35:55Z'
holistic-reviewing  '2026-08-19T19:36:28Z'
holistic-fixing  '2026-08-20T06:29:00Z'
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
    state: approved
    implementer_session: e9046fe0-295d-4c0a-8162-2788f2a8c01e
    start_sha: 978b30b7a76e6a7a0ea34127c35009b0da676212
    commit_sha: bb63ea3555d815568763941a7a33f65fbbb24fdd
    verify_baseline_failures: []
  - name: mergeresolve engine
    state: approved
    implementer_session: 4e2e79a7-bdb0-496f-b941-7ccacbe22167
    start_sha: c7dea943a941ff8f3292b70f2144bcd45aca6ee8
    commit_sha: a52b6f862cdf78546d60b2ef944a52c1fad4704b
    verify_baseline_failures: ["FAIL\t./internal/mergeresolve/... [setup failed]"]
  - name: landingshed producers
    state: approved
    implementer_session: 65b22a31-1543-438d-9e2c-ed7a61e1b395
    start_sha: 52c2080a6a8ac1df80d1cf2047885710a36b7a7e
    commit_sha: 53c6e1358bc63e1ad9b5324c059e21d38f6c92a5
    verify_baseline_failures: ["FAIL\t./internal/landingshed/... [setup failed]"]
  - name: loomshed wiring and integration
    state: approved
    implementer_session: 0af3fbd3-bbc9-4e77-9795-824f5e2d3992
    start_sha: ee4c012540711a1cb7199a3acbc7cd5f37625f8d
    commit_sha: 6b0303fc69593ae02bebe0285031fe68c89730f2
    verify_baseline_failures: ["FAIL\t./internal/landingshed/... [setup failed]"]
  - name: documentation lifecycle
    state: approved
    implementer_session: 8312e9fd-f272-4b04-9b2e-609e0dcc1c2a
    start_sha: 764001249d85014083a8b5cc2bb386abf450042c
    commit_sha: d742a88b7ff7aa8d13ef5a59211ad8fbf33bdc9e
    verify_baseline_failures: ["FAIL\t./internal/landingshed/... [setup failed]", "FAIL\t./internal/mergeresolve/...\
    \ [setup failed]"]
```
