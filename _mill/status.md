# Status

```yaml
phase: approved-loomcli-rewiring
slug: loom-convert-to-shed-recipe
branch: loom-convert-to-shed-recipe
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'loom: convert to a Shed recipe'
task_description: |
  loom: convert to a Shed recipe
```

## Timeline

```text
discussing  '2026-08-21T13:32:35Z'
discussion-fix-r2  '2026-08-21T14:06:30Z'
discussion-fix-r5  '2026-08-21T14:28:56Z'
discussed  '2026-08-21T14:28:56Z'
planning  '2026-08-21T14:41:27Z'
plan-review-r1  '2026-08-21T14:53:42Z'
plan-fix-r1  '2026-08-21T14:53:42Z'
plan-review-r2  '2026-08-21T15:06:21Z'
plan-fix-r2  '2026-08-21T15:06:21Z'
plan-review-r3  '2026-08-21T15:16:17Z'
plan-fix-r3  '2026-08-21T15:16:17Z'
plan-review-r4  '2026-08-21T15:26:13Z'
plan-fix-r4  '2026-08-21T15:26:13Z'
plan-review-r5  '2026-08-21T15:36:09Z'
plan-fix-r5  '2026-08-21T15:36:09Z'
plan-review-r6  '2026-08-21T15:48:37Z'
plan-fix-r6  '2026-08-21T15:48:37Z'
plan-fix-r7  '2026-08-21T15:59:55Z'
planned  '2026-08-21T16:00:20Z'
implementing  '2026-08-21T16:00:45Z'
approved-recipe-file-and-loomrecipe-package  '2026-08-21T16:05:52Z'
approved-move-the-graph-tests-into-loomrecipe  '2026-08-21T16:16:35Z'
approved-loomcli-rewiring  '2026-08-21T16:21:32Z'
```

## Batches

```yaml
batches:
  - name: recipe-file-and-loomrecipe-package
    state: approved
    implementer_session: f5702947-9d01-4983-8e07-ec1361c62193
    start_sha: 993fbd7804bb9ddb394925f2ac3b449bd22dd31b
    commit_sha: 31e1e4d191bbbea3e737a64f3e46aa45bac682c6
    verify_baseline_failures: ["FAIL\t./internal/loomrecipe/... [setup failed]"]
  - name: move-the-graph-tests-into-loomrecipe
    state: approved
    implementer_session: 61bf32f9-9d9e-4eae-86be-5d035cf25371
    start_sha: 234a679ccd8339259953aa35709844664ad7156a
    commit_sha: ad00b2d505a52848f25a0e43c17b4f41166f3a13
    verify_baseline_failures: ["FAIL\t./internal/loomrecipe/... [setup failed]"]
  - name: loomcli-rewiring
    state: approved
    implementer_session: fd5527c7-ab71-4e5e-830e-7728e634d953
    start_sha: 464aaef368e497efc2bb674ee5e898609a72703e
    commit_sha: e4669de287719785ea2911216e3abcd1347c7e6f
    verify_baseline_failures: []
  - name: coverage-guard-move-and-fixture-retirement
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/loomrecipe/... [setup failed]"]
  - name: delete-loomshed-new-and-deps
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/loomrecipe/... [setup failed]"]
  - name: docs-and-comment-sweep
    state: pending
    verify_baseline_failures: []
```
