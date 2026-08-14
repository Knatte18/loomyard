# Pre-count — fabric crucible round 1 (orchestrator-only, round never sees this)

Counted 2026-08-14, on commit 08520a1b (branch fabric-crucible-hardening), BEFORE spawning round 1.

## Raw gitexec call sites (gitexec Checked-Call Invariant)

Pattern: `grep -rn "gitexec:raw" internal/fabricengine internal/gitrepo --include=*.go`

- `internal/fabricengine`: **2** markers, both in `weftwiring.go` (lines 73, 90) —
  `weftRepoExists` / `weftBranchExists`, both `//gitexec:raw — bool-returning predicate: the
  signature has no error channel, so every outcome must collapse to a bool.`
- `internal/gitrepo`: **3** markers — `gitrepo.go:60` (`run`'s own body), `pull.go:19` (`Pull`),
  `pull.go:34` (`Fetch`).

These match CONSTRAINTS.md's "gitexec Checked-Call Invariant" stated counts exactly
(fabricengine 2, gitrepo 3) as of this commit. A round's own count should land on the same
numbers; a round reporting fewer risks having missed a site (or the guard's substring scan
missing one — see the invariant's own "Known guard blind spot" clause), a round reporting more
found something the grep above didn't (a raw call spelled differently, or a genuinely new one
added without a marker — that's a real finding, not a discrepancy to distrust).

**Blind spot the grep above cannot see:** a raw `gitexec.RunGit(` or `r.run(` call with NO
adjacent marker at all is exactly what the Checked-Call Invariant's guard test
(`cmd/lyx/checkedcall_test.go`) exists to catch — this pre-count only counted the *marked* sites,
not an exhaustive scan for unmarked raw calls. Don't mistake "the marker count matches" for "no
unmarked raw call exists" — that's the guard test's job, not this count's.

## Smoke vs integration tag

Confirmed: **zero** `//go:build smoke` files under `internal/fabricengine` or `internal/fabriccli`
(`grep -rl "go:build smoke" internal/fabricengine internal/fabriccli` → empty). Fabric's live
substrate tests are all `//go:build integration` (real git worktrees/commits/junctions — never a
real LLM/provider subprocess). LLM-driving declaration for the round prompt: **NO**.
