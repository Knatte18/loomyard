MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus (opus-4 family), best-effort self-assessment
reviewed_file: /home/knatte/Code/loomyard/wts/prowler/_mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Nested module is NOT invisible to grep-guards
**Section:** Decisions → separate-nested-go-module; Constraints
**Issue:** The rationale claims a nested `go.mod` is "automatically excluded from the CONSTRAINTS enforcement tests" and "invisible to the sweeps"; but `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go` do `filepath.WalkDir(moduleRoot, …)` off the parent module root and skip only `.git/_lyx/_mill/.scratch/.wiki/_raddle` — they descend into `plugins/prowler/1.0.0/` and read its `*_test.go` files. The `go env GOMOD` exclusion applies to `go build/test ./...`, not to these disk-reading grep guards.
**Fix:** State that prowler test files must contain none of the banned substrings (`exec.Command`, `gitexec.RunGit`, `lyxtest.*`) — critically for the Hermetic guard, which scans ALL files regardless of build tag and which prowler cannot satisfy (it can't import `lyxtest.HermeticGitEnv`) — OR decide to add `plugins/` to both guards' walk-skip set, and acknowledge that this edits `cmd/lyx` enforcement tests, contradicting the "no changes to internal/cmd tree" scope. A Go test that shells out to `run.sh` (the "concurrent build" check) would break the parent `go test`.

### [NOTE] .scratch pollution outside LoomYard
**Section:** Decisions → output-to-unique-scratch-file; Constraints
**Issue:** The design leans on LoomYard's `.gitignore` ignoring `**/.scratch/`, but prowler's whole point is any-repo reach; in an arbitrary target repo `.scratch/` is not gitignored, so fetched `.md` files appear as untracked clutter (weblens has the same trait with `_millhouse/scratch/`).
**Fix:** Note that scratch output lands in the invoking repo's cwd where it may be untracked, and state whether that is accepted parity or should be swept/relocated.

## Verdict

GAPS_FOUND
One GAP: the nested-module "invisible to enforcement sweeps" claim is false for the two disk-walking grep guards.
MILL_REVIEW_END
