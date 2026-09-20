# Status

```yaml
phase: approved-loomshed-gates
slug: producer-gates
branch: producer-gates
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'Producer gates: mechanical gates before session release'
task_description: |
  Producer gates: mechanical gates before session release
```

## Timeline

```text
discussing  '2026-09-20T13:38:49Z'
discussion-fix-r1  '2026-09-20T13:52:24Z'
discussion-gap-fix-r2  '2026-09-20T13:59:02Z'
discussion-gap-fix-r3  '2026-09-20T14:04:05Z'
discussion-gap-fix-r4  '2026-09-20T14:09:14Z'
discussion-gap-fix-r5  '2026-09-20T14:13:28Z'
discussion-gap-fix-r6  '2026-09-20T14:18:11Z'
discussed  '2026-09-20T14:18:19Z'
planning  '2026-09-20T14:40:11Z'
plan-review-r1  '2026-09-20T14:47:27Z'
plan-fix-r1  '2026-09-20T14:49:29Z'
plan-review-r2  '2026-09-20T14:56:33Z'
plan-fix-r2  '2026-09-20T14:57:27Z'
planned  '2026-09-20T14:57:37Z'
implementing  '2026-09-20T14:57:55Z'
approved-shuttle-gate-loop  '2026-09-20T15:13:03Z'
approved-seam-and-producers  '2026-09-20T15:22:27Z'
approved-loomshed-gates  '2026-09-20T15:29:29Z'
```

## Batches

```yaml
batches:
  - name: shuttle-gate-loop
    state: approved
    implementer_session: 05443526-89b7-4a35-8a93-96bb76687f95
    start_sha: d469a0502560548b157273f2fc75c467cdd273ba
    commit_sha: 94735db5daaa00daa419ce2ec8af0be1a9edd8e5
    verify_baseline_failures: []
  - name: seam-and-producers
    state: approved
    implementer_session: 3e1f9530-795a-4873-9067-d51b716c6e00
    start_sha: 804c9439d9cb1e211ff86d72adab9763702576ae
    commit_sha: 91a0b7731cc168caa6a99fef20c33283c6a29f26
    verify_baseline_failures: []
  - name: loomshed-gates
    state: approved
    implementer_session: 3fa704e4-eaa1-4530-9014-709218638c98
    start_sha: f52dee20fc793c60cba966b2ceec6bf1244ba9a0
    commit_sha: 88aafa794d788cec09108f26635515cfba5a0946
    verify_baseline_failures: []
  - name: shedrecipe-wiring
    state: running
    implementer_session: 5e81325f-34eb-4a05-a64b-73f2cdc23a71
    start_sha: 17365449613f0e9b46a1195d9e7ac8deacb86b3f
    verify_baseline_failures: []
  - name: row-removal
    state: pending
    verify_baseline_failures: []
  - name: parity-docs-sweep
    state: pending
    verify_baseline_failures: []
```
