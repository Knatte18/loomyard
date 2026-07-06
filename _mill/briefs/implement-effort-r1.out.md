All 5 cards are committed and pushed, verify passes.

Summary of work:
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttleengine\spec.go` — added `Spec.Effort string` field (after `Model`), untouched by `validate`.
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttleengine\spec_test.go` — added `TestSpec_Validate_EffortUntouched`.
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttleengine\claudeengine\command.go` — added `validEfforts` vocabulary set, `validateEffort`, and threaded `effort` into `buildLaunchCmd` (appends `--effort '<value>'` next to `--model`; `buildResumeCmd` untouched).
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttleengine\claudeengine\command_test.go` — extended `TestBuildLaunchCmd` table and added `TestValidateEffort`.
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttleengine\claudeengine\claudeengine.go` — `Prepare` now hard-errors via `validateEffort(spec.Effort)` before writing any artifact, and threads `spec.Effort` into `buildLaunchCmd`.
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttleengine\claudeengine\prepare_test.go` (new) — bad-effort-rejected-before-artifacts, valid-effort-lands-in-Cmd, empty-effort-emits-no-flag.
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttlecli\run.go` — added `--effort` flag wired into `Spec.Effort`, mirroring `--model`.
- `C:\Code\loomyard\wts\shuttle-spec-effort\internal\shuttlecli\cli_test.go` — added `TestRunCmd_EffortFlag` using a real `*shuttleengine.Runner` built over a local spec-capturing `Engine` fake and a no-op `MuxOps` fake (no such CLI-level fake-runner pattern pre-existed in this package, so I introduced one mirroring `shuttleengine`'s own `fakeEngine`/`fakeMux` convention).
- `C:\Code\loomyard\wts\shuttle-spec-effort\docs\overview.md` — one-line note on the shuttle module's per-run `Model`/`Effort` knobs.

Note: `gofmt -l` flags every file in this repo (pre-existing CRLF vs. LF mismatch from `core.autocrlf=true`, confirmed repo-wide, not introduced by this batch) — I did not rewrite line endings, only fixed two genuine alignment issues gofmt found in my new `cli_test.go` code.

{"status":"success","commit_sha":"fbc7e7c","session_id":"670cbf67-38d4-47bf-9b3d-0eec9126ba34"}
