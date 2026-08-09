# Status

```yaml
phase: done
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
approved-file-renames  '2026-08-09T06:49:51Z'
self-resolved-verify-logic  '2026-08-09T06:52:56Z'
approved-cli-surface-review  '2026-08-09T06:56:38Z'
approved-docs-sweep  '2026-08-09T07:10:42Z'
approved-constraints-and-guard  '2026-08-09T07:18:54Z'
holistic-reviewing  '2026-08-09T07:19:09Z'
holistic-fixing  '2026-08-09T07:23:39Z'
nits-fixed-holistic  '2026-08-09T07:26:45Z'
holistic-approved  '2026-08-09T07:26:57Z'
done  '2026-08-09T07:27:36Z'
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
    state: approved
    implementer_session: b290a829-2c2a-4835-add8-becd5d4d87c0
    start_sha: 92dfaaaca26223c343e4d975bf78aae780f9c43b
    commit_sha: f3b465c7ee20b89095f1b9043a46a2832c4fd6e6
    verify_baseline_failures: []
  - name: cli-surface-review
    state: approved
    implementer_session: c11b60d3-bb2d-42af-af1a-f3f8bb3274b0
    start_sha: 45676f507638134b7b13552cb7fc976bfe3e6e76
    commit_sha: 43dd14dc5a75994b8cb6253d7dc3d55bec92f7e8
    verify_baseline_failures: []
  - name: docs-sweep
    state: approved
    implementer_session: ed5429b7-4755-49b8-8fd3-0d42f9074c47
    start_sha: cf38eef4d4310a84cde8de90551692e88215ef83
    commit_sha: a2bfff66d34463d4e136fbe899cdf78506954704
    verify_baseline_failures: []
  - name: constraints-and-guard
    state: approved
    implementer_session: 75b8ba39-b19c-433e-be04-c5e01f7cb97a
    start_sha: 303d548ff8382cd3782b8cef0319dd4a8faa089f
    commit_sha: 114d70d7f4e571a68321f87d08b40e075a07151c
    verify_baseline_failures: []
```
