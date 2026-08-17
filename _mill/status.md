# Status

```yaml
phase: approved-reed-geometry-hubgeom
slug: shuttle-reed-told-geometry
branch: shuttle-reed-told-geometry
plan: _mill/plan
parent: standalone-producers
module_verify_baseline: clean
task: shuttleengine + reedengine + tokenvocab told-geometry
task_description: |
  shuttleengine + reedengine + tokenvocab told-geometry
```

## Timeline

```text
discussing  '2026-08-17T12:53:07Z'
discussion-fix-r1  '2026-08-17T14:37:13Z'
discussion-fix-r2  '2026-08-17T14:42:13Z'
discussed  '2026-08-17T14:42:13Z'
planning  '2026-08-17T14:51:35Z'
plan-review-r1  '2026-08-17T14:58:57Z'
planned  '2026-08-17T14:59:14Z'
implementing  '2026-08-17T14:59:48Z'
approved-hublogsdir-move  '2026-08-17T15:05:16Z'
approved-tokenvocab-plain-fields  '2026-08-17T15:08:56Z'
approved-reed-geometry-hubgeom  '2026-08-17T15:19:10Z'
```

## Batches

```yaml
batches:
  - name: hublogsdir-move
    state: approved
    implementer_session: 350689d7-d82d-4fff-8b26-dac267a54e9a
    start_sha: 32c5afcb8c979cda23ccd6f0d80dbba2f01d9785
    commit_sha: 67975ffee9f06dcaecc617a64dda9e31e74d4f0e
    verify_baseline_failures: []
  - name: tokenvocab-plain-fields
    state: approved
    implementer_session: ae131aa5-1f3a-4e40-8db5-1f2256bafc52
    start_sha: 83313c44dafb83641941bf7ecb68b7cd4b031eb5
    commit_sha: bcaf644007238ebc2ea4bd146eeada5d35985054
    verify_baseline_failures: []
  - name: reed-geometry-hubgeom
    state: approved
    implementer_session: 4a15a59c-f2e9-4fc4-b2cc-f5163c5043f3
    start_sha: 51f3798eededf6d1043c2b3059c66996ea42201d
    commit_sha: 297d582be2b8187de8c82866d409a5c3077506f7
    verify_baseline_failures: ["FAIL\t./internal/hubgeom/... [setup failed]"]
  - name: shuttle-told-strings
    state: running
    implementer_session: deed3cbe-a3f9-48b3-8363-c124b192be50
    start_sha: fde207594c6d0103b857f66da1930215fb457abb
    verify_baseline_failures: []
```
