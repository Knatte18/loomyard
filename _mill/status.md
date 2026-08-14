# Status

```yaml
phase: approved-treadle-runtime-read
slug: stencils-directory-reorg
branch: stencils-directory-reorg
plan: _mill/plan
parent: main
module_verify_baseline: clean
task: Relocate producer prompt files into a stencils/ directory
task_description: |
  Relocate producer prompt files into a stencils/ directory
```

## Timeline

```text
discussing  '2026-08-14T07:40:14Z'
discussed  '2026-08-14T10:46:07Z'
planning  '2026-08-14T11:04:14Z'
plan-review-r1  '2026-08-14T11:18:04Z'
plan-fix-r1  '2026-08-14T11:18:04Z'
plan-review-r2  '2026-08-14T11:30:48Z'
plan-fix-r2  '2026-08-14T11:30:48Z'
plan-fix-r3  '2026-08-14T11:40:21Z'
planned  '2026-08-14T11:40:30Z'
implementing  '2026-08-14T11:40:58Z'
approved-stencilstore-foundation  '2026-08-14T11:49:49Z'
approved-stencils-package-and-loom  '2026-08-14T11:56:36Z'
approved-seeding-trigger  '2026-08-14T12:04:22Z'
approved-burler-runtime-read  '2026-08-14T12:13:13Z'
approved-diff-base-recovery  '2026-08-14T12:19:41Z'
approved-treadle-runtime-read  '2026-08-14T12:32:23Z'
```

## Batches

```yaml
batches:
  - name: stencilstore-foundation
    state: approved
    implementer_session: 2a017dfc-d07f-4960-b0dd-ad31caf3b990
    start_sha: 12794a23d95be890edc84b13e0de9a4a0e9013c6
    commit_sha: ca1e65acc860f90163350b2f279c2899345ce02f
    verify_baseline_failures: ["FAIL\t./internal/stencilstore/... [setup failed]"]
  - name: stencils-package-and-loom
    state: approved
    implementer_session: 1ff45c71-97e0-45f7-88fd-87d48304f8e9
    start_sha: 3102a8e8a90f0c56e0cae8e92501c444165e3678
    commit_sha: 41e1ff0db67fa67866289ddf0b310980b20b9820
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: seeding-trigger
    state: approved
    implementer_session: b9b92d06-a5cc-46d1-905c-61aa02eb2dac
    start_sha: 30a9047f15e7a10d50c6de081273768b71ac2abc
    commit_sha: 7be02382e7fce6184261b8d8722187b84f23d1cf
    verify_baseline_failures: []
  - name: burler-runtime-read
    state: approved
    implementer_session: aa7de156-96b5-4c29-9146-2b61d982582d
    start_sha: 38a8395c18c2f581c75b63ef968cc062d892d0b5
    commit_sha: de09c60c3c4bf1ccf27ea1cb5efdbcbef9b20a59
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: diff-base-recovery
    state: approved
    implementer_session: 77fbd99c-765b-4979-808c-4d74ea6fc264
    start_sha: b89ac310fc5e6ce23a825ab152c31e70c2d49663
    commit_sha: 4364a9cdcc4e151e9025e348ffdbc33255de68bc
    verify_baseline_failures: []
  - name: treadle-runtime-read
    state: approved
    implementer_session: 43c07e8b-0519-4596-acf9-08a1db524f1d
    start_sha: 3a22b61c39a12a71a4339f8745ba0a182ba83595
    commit_sha: aa66b321b54c9afa2d8b30063e1717582df0055d
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: webster-runtime-read
    state: running
    implementer_session: 3a56da65-b15b-4630-98ac-933d92727f0c
    start_sha: 0af726b91aa047f06d1a6458d88d42c2f1895cb9
    verify_baseline_failures: ["FAIL\t./stencils/... [setup failed]"]
  - name: stencil-cli
    state: pending
    verify_baseline_failures: ["FAIL\t./internal/stencilcli/... [setup failed]"]
  - name: reed-rename-and-docs
    state: pending
    verify_baseline_failures: []
```
