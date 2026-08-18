# Status

```yaml
phase: approved-batcher moves to the degrading side
slug: webster-told-geometry
branch: webster-told-geometry
plan: _mill/plan
parent: standalone-producers
module_verify_baseline: clean
task: websterengine + webstercli told-geometry, and Webster standalone entry
task_description: |
  websterengine + webstercli told-geometry, and Webster standalone entry
```

## Timeline

```text
discussing  '2026-08-18T08:17:03Z'
discussed  '2026-08-18T09:06:02Z'
planning  '2026-08-18T09:22:35Z'
plan-review-r1  '2026-08-18T09:33:38Z'
plan-fix-r1  '2026-08-18T09:33:38Z'
plan-review-r2  '2026-08-18T09:45:09Z'
plan-fix-r2  '2026-08-18T09:45:09Z'
plan-fix-r3  '2026-08-18T09:54:12Z'
planned  '2026-08-18T09:54:22Z'
implementing  '2026-08-18T09:54:54Z'
approved-reed pane cwd  '2026-08-18T10:01:16Z'
approved-batcher moves to the degrading side  '2026-08-18T10:05:02Z'
```

## Batches

```yaml
batches:
  - name: reed pane cwd
    state: approved
    implementer_session: c875bda6-4fd6-480a-962e-146d15370f64
    start_sha: 406fde433a3f2317971e90d5dfa9ee5c7af30e00
    commit_sha: 0c028c4901226137fd606b2b2c3dc3fb2efe125e
    verify_baseline_failures: []
  - name: batcher moves to the degrading side
    state: approved
    implementer_session: c24e1594-960c-4ea0-82e4-0d7dfef85729
    start_sha: 72dd0c1bc45375dd0cdfe4912e0950f971f6c749
    commit_sha: 1204b1567f0ed3567ec1a506a2d7177074eb3d1d
    verify_baseline_failures: []
  - name: preflight doc correction
    state: running
    implementer_session: 2d850da3-ee88-42d6-a8f9-e053260f3dbb
    start_sha: 1e93ad689896550d069b64e9fb8c40d60f56cf9b
    verify_baseline_failures: []
  - name: websterengine Geometry and RefMatcher
    state: pending
    verify_baseline_failures: []
  - name: webster path accessors take a told anchor root
    state: pending
    verify_baseline_failures: []
  - name: hubgeom.WebsterGeometry and the standalonegeom sibling
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/standalonegeom/... [setup failed]"]
  - name: websterengine told Deps and engine-owned fabric seams
    state: pending
    verify_baseline_failures: []
  - name: webstercli standalone entry
    state: pending
    verify_baseline_failures: []
```
