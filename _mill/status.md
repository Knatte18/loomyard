# Status

```yaml
phase: approved-B -- config collapse
slug: fabric-cutover
branch: fabric-cutover
plan: _mill/plan
parent: main
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
task_description: |
  fabric: cutover -- rewire consumers onto fabric, delete warp/weft
```

## Timeline

```text
discussing  '2026-07-26T09:53:20Z'
discussed  '2026-07-26T10:38:03Z'
planning  '2026-07-26T10:54:19Z'
plan-review-r1  '2026-07-26T11:09:34Z'
plan-fix-r1  '2026-07-26T11:09:34Z'
plan-review-r2  '2026-07-26T14:42:44Z'
plan-fix-r2  '2026-07-26T14:42:44Z'
plan-fix-r3  '2026-07-26T14:47:55Z'
planned  '2026-07-26T14:48:20Z'
implementing  '2026-07-26T14:49:21Z'
approved-A -- consumers  '2026-07-26T15:02:47Z'
approved-B -- config collapse  '2026-07-26T15:14:16Z'
```

## Batches

```yaml
batches:
  - name: A -- consumers
    state: approved
    implementer_session: 5125010a-d6f6-4a34-8b0c-2ca81606d967
    start_sha: 6209cab47bf4135847ceffb05c8e661313c5cbb8
    commit_sha: cf13ab47a794f43483079752d430a2ff60ea869c
  - name: B -- config collapse
    state: approved
    implementer_session: e6973662-8f32-4475-a8cc-2855b82cce44
    start_sha: ef4186d46b86a00ff2cfe04627df38af55957e69
    commit_sha: aa67bcdf79673b804565a18ab7a22a0e03f4be6d
  - name: C -- CLI de-registration + sandbox tags
    state: running
    implementer_session: ff5af2e0-8796-4f98-a7e4-e159eab96204
    start_sha: 9416063d96b39e76104a5bb11247351a2c8db5a8
  - name: D1 -- delete modules + enforcement
    state: pending
  - name: D2 -- doc repoint
    state: pending
  - name: D3 -- de-parallel-build prose + final gate
    state: pending
```
