# Status

```yaml
phase: pr-pending
slug: shed-recipe-engine-registry
branch: shed-recipe-engine-registry
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'Shed recipe: engine registry'
task_description: |
  Shed recipe: engine registry
```

## Timeline

```text
discussing  '2026-08-21T07:29:20Z'
blocked  '2026-08-21T08:30:28Z'
discussed  '2026-08-21T08:33:11Z'
planning  '2026-08-21T08:45:06Z'
plan-review-r1  '2026-08-21T08:57:05Z'
plan-fix-r1  '2026-08-21T08:57:05Z'
plan-fix-r2  '2026-08-21T09:06:58Z'
planned  '2026-08-21T09:07:18Z'
implementing  '2026-08-21T09:07:55Z'
approved-loomshed constructor exports  '2026-08-21T09:11:37Z'
approved-shedrecipe foundations  '2026-08-21T09:17:19Z'
approved-registry and value-only entries  '2026-08-21T09:24:32Z'
approved-SingleLLM entry  '2026-08-21T09:30:06Z'
approved-Bouncer and BurlerRound entries  '2026-08-21T09:36:35Z'
approved-guards and docs  '2026-08-21T09:43:52Z'
holistic-reviewing  '2026-08-21T09:44:29Z'
holistic-approved  '2026-08-21T09:47:50Z'
done  '2026-08-21T09:49:21Z'
pr-pending  '2026-08-21T09:50:53Z'
```

## Batches

```yaml
batches:
  - name: loomshed constructor exports
    state: approved
    implementer_session: d233ec3d-06fb-4c62-b7da-d74f07f41920
    start_sha: f14c70d26b5c2fe118ef190e6fe455be11af2d83
    commit_sha: 217a1c1bb263b31e40738bfe8c9327d87ed23b73
    verify_baseline_failures: []
  - name: shedrecipe foundations
    state: approved
    implementer_session: 33dc8d52-8441-48c5-8afb-6b30fd23815f
    start_sha: 9ac235c650e0934f6b1cd35ffd3b4801213d2550
    commit_sha: 00321f120393cf412564c2184cbe2e3396909771
    verify_baseline_failures: ["FAIL\t./internal/shedrecipe/... [setup failed]"]
  - name: registry and value-only entries
    state: approved
    implementer_session: 6995d15f-60ba-4b09-aeee-45f20ef4ff45
    start_sha: 23f21813398aa2f57ee4d7a98f6878a3d52e6d9d
    commit_sha: 907dc8fbd1cd48f55a6bb71f073d1141c1c9bef4
    verify_baseline_failures: ["FAIL\t./internal/shedrecipe/... [setup failed]"]
  - name: SingleLLM entry
    state: approved
    implementer_session: 0820377d-5022-4b96-8f2e-53b40a15f96c
    start_sha: f0abc2cffdd460a37f860e82294df43a5d805488
    commit_sha: 7f3081fc008b41b7a3e19d205d32fc707c738966
    verify_baseline_failures: ["FAIL\t./internal/shedrecipe/... [setup failed]"]
  - name: Bouncer and BurlerRound entries
    state: approved
    implementer_session: c348fd49-8f3c-491a-96bf-7b730defc6fc
    start_sha: fe52d03e08e04ddbaf9f5d682e40de4a5dd677a4
    commit_sha: 9a92c424ca38be6eae0faae09a8bf4cb8979f152
    verify_baseline_failures: ["FAIL\t./internal/shedrecipe/... [setup failed]"]
  - name: guards and docs
    state: approved
    implementer_session: b06ffffb-7b27-496c-8d0e-b799fd70e729
    start_sha: b7a9bb8b0d3c6cde7911f845122da59685564ece
    commit_sha: 1867aa7f7d869d135f21e564fa56637c2a12348d
    verify_baseline_failures: ["FAIL\t./internal/shedrecipe/... [setup failed]"]
```
