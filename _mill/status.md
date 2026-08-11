# Status

```yaml
phase: holistic-reviewing
slug: fabric-destructive-chokepoint
branch: fabric-destructive-chokepoint
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
task_description: |
  fabric: one ownership-and-dirtiness gate for all destruction (slice 12)
```

## Timeline

```text
discussing  '2026-08-10T10:16:14Z'
discussed  '2026-08-10T12:41:06Z'
planning  '2026-08-10T12:59:08Z'
plan-review-r1  '2026-08-10T13:11:42Z'
plan-fix-r1  '2026-08-10T13:11:42Z'
plan-review-r2  '2026-08-10T13:22:31Z'
plan-fix-r2  '2026-08-10T13:22:31Z'
plan-review-r3  '2026-08-10T17:06:58Z'
plan-fix-r3  '2026-08-10T17:06:58Z'
plan-review-r4  '2026-08-10T17:18:00Z'
plan-fix-r4  '2026-08-10T17:18:00Z'
plan-fix-r5  '2026-08-10T17:31:51Z'
planned  '2026-08-10T17:32:03Z'
implementing  '2026-08-10T17:43:28Z'
approved-dirtiness-probe  '2026-08-10T17:53:36Z'
approved-the-gate  '2026-08-10T18:06:18Z'
self-resolved-verify-logic  '2026-08-10T18:36:09Z'
approved-path-callsites  '2026-08-10T18:40:06Z'
approved-clone-callsites  '2026-08-10T18:44:45Z'
approved-branch-callsites  '2026-08-10T18:57:50Z'
approved-guard-and-docs  '2026-08-11T03:43:19Z'
approved-gap-integration-tests  '2026-08-11T04:04:52Z'
holistic-reviewing  '2026-08-11T04:05:15Z'
```

## Batches

```yaml
batches:
  - name: dirtiness-probe
    state: approved
    implementer_session: 2c58d66a-c805-4f7d-b687-de9606e95776
    start_sha: bb9aa21012d0c08cc19d94e5d03eadefe53335ef
    commit_sha: c248d402fa9a0ab171a97a52bfeef8ac3a325469
    verify_baseline_failures: []
  - name: the-gate
    state: approved
    implementer_session: 57f1b8c0-fb0a-4ad3-925a-fcd3190557e4
    start_sha: 926039d1f60bee80af3448f572a021803e903308
    commit_sha: ba49173d15ac92961578bc2a683c8bf3133447bf
    verify_baseline_failures: []
  - name: path-callsites
    state: approved
    implementer_session: 887460a1-bf7c-4879-84a9-c4b13bf87dec
    start_sha: 44a55010ad28ca8198d781b96900f231453d6b6d
    commit_sha: 4f5b147fa36c28e207494ca2801c0180a2209902
    verify_baseline_failures: []
  - name: clone-callsites
    state: approved
    implementer_session: a6c1879f-ec2c-4f1b-b7b0-feeaab4a533b
    start_sha: e3db011360e6a786f1c0ab6caab31bc11ac4cd3f
    commit_sha: e35b7fd44b8d1b0d78eb2b88d6946db523466452
    verify_baseline_failures: []
  - name: branch-callsites
    state: approved
    implementer_session: fe86e9cf-b666-4bb4-97ca-769f57f02b2b
    start_sha: 6a73abfed1a837379dd90d66e505eaa882e35870
    commit_sha: 59917cd56bff892ef889cc84dc0c3a51d43731c4
    verify_baseline_failures: []
  - name: guard-and-docs
    state: approved
    implementer_session: 556e5735-846c-4bfa-8ed5-2edf9e4cfb8c
    start_sha: a37945a2330883e23dba8a18380137a823d91213
    commit_sha: 4d72fa54fd5092f1661864577ac17a4512028648
    verify_baseline_failures: []
  - name: gap-integration-tests
    state: approved
    implementer_session: 5b4f4b2e-ecb1-4e25-be04-af46346bb1a6
    start_sha: cc325b97f89defd225945a0ec19930011a8e810f
    commit_sha: 47d231a7ba3ec00273abbac25de1d093035979ac
    verify_baseline_failures: []
```
