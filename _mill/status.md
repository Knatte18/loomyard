# Status

```yaml
phase: approved-go-sweep
slug: fabric-host-to-warp-rename
branch: fabric-host-to-warp-rename
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: Rename the fabric host vocabulary to warp, and name the composite repo Fabric
task_description: |
  Rename the fabric host vocabulary to warp, and name the composite repo Fabric
```

## Timeline

```text
discussing  '2026-08-08T19:39:36Z'
discussed  '2026-08-09T05:53:11Z'
planning  '2026-08-09T06:12:15Z'
plan-review-r1  '2026-08-09T06:20:03Z'
plan-fix-r1  '2026-08-09T06:21:12Z'
plan-review-r2  '2026-08-09T06:29:02Z'
planned  '2026-08-09T06:29:34Z'
implementing  '2026-08-09T06:30:13Z'
approved-wordswap-tool  '2026-08-09T06:38:43Z'
approved-pre-sweep-rewords  '2026-08-09T06:41:14Z'
approved-go-sweep  '2026-08-09T06:45:47Z'
```

## Batches

```yaml
batches:
  - name: wordswap-tool
    state: approved
    implementer_session: f52a7dc3-205e-421d-9030-85971e638e0b
    start_sha: 9045c7272c1e677a6335b45274b4e1f0d22a5c70
    commit_sha: 3be81471cb02275fdea17b29e997ccd92b022e3c
    verify_baseline_failures: ["FAIL\t./tools/wordswap/... [setup failed]"]
  - name: pre-sweep-rewords
    state: approved
    implementer_session: 0e1cd2aa-1d75-4ca2-9157-6dc6c3184122
    start_sha: 97c5869745a7f2f4281bceb4c2a2a3f2347cdca3
    commit_sha: 5ee3045bad8521e8dfaf5fb889a3d8ac8ba29526
    verify_baseline_failures: []
  - name: go-sweep
    state: approved
    implementer_session: 415260b7-642b-4ffd-b0b6-4838e87edbaf
    start_sha: f05af15cdb48055cb0ee77c665d8795e11d6eedb
    commit_sha: d6d3e68c4d82b055a322b577f55fbf3d21a9740f
    verify_baseline_failures: []
  - name: file-renames
    state: pending
    verify_baseline_failures: []
  - name: cli-surface-review
    state: pending
    verify_baseline_failures: []
  - name: docs-sweep
    state: pending
    verify_baseline_failures: []
  - name: constraints-and-guard
    state: pending
    verify_baseline_failures: []
```
