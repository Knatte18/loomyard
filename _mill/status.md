# Status

```yaml
phase: done
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
approved-C -- CLI de-registration + sandbox tags  '2026-07-26T15:21:02Z'
approved-D1 -- delete modules + enforcement  '2026-07-26T16:13:44Z'
approved-D2 -- doc repoint  '2026-07-26T16:22:55Z'
approved-D3 -- de-parallel-build prose + final gate  '2026-07-26T16:45:15Z'
holistic-reviewing  '2026-07-26T16:45:40Z'
holistic-fixing  '2026-07-26T16:50:02Z'
holistic-reviewing  '2026-07-26T16:58:10Z'
holistic-fixing  '2026-07-26T17:05:47Z'
holistic-reviewing  '2026-07-26T17:08:55Z'
holistic-fixing  '2026-07-26T17:14:47Z'
holistic-reviewing  '2026-07-26T17:18:12Z'
holistic-fixing  '2026-07-26T17:25:16Z'
holistic-reviewing  '2026-07-26T17:31:09Z'
holistic-fixing  '2026-07-26T17:36:06Z'
nits-fixed-holistic  '2026-07-26T17:39:12Z'
holistic-approved  '2026-07-26T17:39:23Z'
done  '2026-07-26T17:39:54Z'
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
    state: approved
    implementer_session: ff5af2e0-8796-4f98-a7e4-e159eab96204
    start_sha: 9416063d96b39e76104a5bb11247351a2c8db5a8
    commit_sha: 98ffd3c396ed2af2c84d1ed52e12f56986a5e474
  - name: D1 -- delete modules + enforcement
    state: approved
    implementer_session: 5a32ca4d-1964-43f8-9d40-25342e769209
    start_sha: 8b3b7efcb7e100a116e8f681d6dded059700e1bc
    commit_sha: 91332f90c16f6d1f5f5cee139c8ee4e84c307a4b
  - name: D2 -- doc repoint
    state: approved
    implementer_session: a3cd5c05-c087-4f72-883c-3c077a480546
    start_sha: 80bb4e51ba3fe3b9daa029775bbc0f939abce84f
    commit_sha: 65889a07de6d22de901df0062344c1fe5b52739e
  - name: D3 -- de-parallel-build prose + final gate
    state: approved
    implementer_session: d67a4c77-8c38-4e54-ae64-a87f274dcecd
    start_sha: 28aa98962acf0373ee2438e9abe00e6400b49cb7
    commit_sha: efae2aa9afa5ead39a34ac1dbd67d35218a0fe81
```
