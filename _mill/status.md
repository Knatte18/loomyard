# Status

```yaml
phase: implementing
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
```

## Batches

```yaml
batches:
  - name: A -- consumers
    state: pending
  - name: B -- config collapse
    state: pending
  - name: C -- CLI de-registration + sandbox tags
    state: pending
  - name: D1 -- delete modules + enforcement
    state: pending
  - name: D2 -- doc repoint
    state: pending
  - name: D3 -- de-parallel-build prose + final gate
    state: pending
```
