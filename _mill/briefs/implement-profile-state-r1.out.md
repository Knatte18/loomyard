{"status":"success","commit_sha":"43db6457f97d9f9786cfa6ee29aa82c4c1759bb6","session_id":"c32cc78f-69ee-4da9-8e5a-5789b276ed64"}

All three cards of the `profile-state` batch are implemented and committed:

- `C:\Code\loomyard\wts\internal-perch\internal\perchengine\profile.go` / `profile_test.go` (commit `788d8a0`): `Profile`, `Gate`, `GateMode` and `validate` with three-level default resolution (profile > Config > built-in) and every fail-loud rule.
- `C:\Code\loomyard\wts\internal-perch\internal\perchengine\result.go`, `roundfiles.go`, `roundfiles_test.go` (commit `513c97f`): `Outcome`/`StuckReason`/`RoundSummary`/`Result`, `roundToken`/`artifactPaths`/`roundArtifactPaths`, and `buildRoundProfile`.
- `C:\Code\loomyard\wts\internal-perch\internal\perchengine\state.go`, `state_test.go` (commit `43db645`): `runState`/`roundRecord`, `ProfileHash`, `DeriveRunID`, `loadOrInitState`, `saveState`, `moveStaleArtifacts`, `PauseFlagPath`/`clearPauseFlag`.

Verify (`go test ./internal/perchengine/`) passes; working tree is clean; final commit is `43db6457f97d9f9786cfa6ee29aa82c4c1759bb6`.

{"status":"success","commit_sha":"43db6457f97d9f9786cfa6ee29aa82c4c1759bb6","session_id":"c32cc78f-69ee-4da9-8e5a-5789b276ed64"}
