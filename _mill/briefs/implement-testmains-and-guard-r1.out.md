Final verify run confirms zero failures across all 52 packages. All 6 cards of the batch are committed, pushed, and the batch verify command passes cleanly.

{"status":"success","commit_sha":"627573d","session_id":"36dfd378-ad6f-4683-892d-05fe5e1772a1"}

Card-count self-check: 6 of 6 cards committed (verified via `git log b06d127..HEAD --oneline`, matching all six `## Cards` `Commit:` messages in `_mill/plan/03-testmains-and-guard.md`). Batch fully complete.

Summary of work:

- `C:\Code\loomyard\wts\faster-git-fixture-tests\cmd\lyx\hermeticenv_test.go` (new) — `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`, the module-walking guard. Confirmed it correctly enumerated all 22 packages missing a TestMain on first run (TDD-red), then went green as testmains landed.
- `C:\Code\loomyard\wts\faster-git-fixture-tests\cmd\lyx\tierpurity_test.go` — added the reciprocal `allowedSpawners` entry for the new guard file.
- 22 new `testmain_test.go` files across `internal/warpengine`, `internal/warpcli`, `internal/weftengine`, `internal/weftcli`, `internal/boardcli`, `internal/boardengine/boardtest`, `internal/buildercli`, `internal/builderengine`, `internal/perchcli`, `internal/perchengine`, `internal/muxcli`, `internal/muxpoccli`, `internal/shuttlecli`, `internal/burlerengine`, `internal/initengine`, `internal/initcli`, `internal/configcli`, `internal/idecli`, `internal/ideengine`, `internal/hubgeometry`, `internal/gitexec`, `cmd/lyx`.
- `C:\Code\loomyard\wts\faster-git-fixture-tests\CONSTRAINTS.md` — new `## Hermetic Git Test Environment Invariant` section.
- `C:\Code\loomyard\wts\faster-git-fixture-tests\internal\lyxtest\doc.go` — added a paragraph on the two-layer hermetic mechanism.

One bug caught and fixed during implementation: `cmd/lyx/testmain_test.go`'s original doc comment mentioned "exec.Command" literally, which tripped `TestTierPurity_UntaggedTestsSpawnNothing` (raw-substring match in an untagged file); reworded to "launched child processes" to preserve the explanation without the banned token. Also normalized `internal/lyxtest/doc.go` to consistent LF line endings after an Edit-tool artifact left it with mixed CRLF/LF that `gofmt -l` flagged.
