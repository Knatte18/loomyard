# quarry port log

This is a transient port record for the "Extract scout into its own standalone repo" task.
It is deleted by this task's final batch (`06-lyx-removal`), once the extraction lands.
Each quarry-side batch appends its own `## Batch N` section here, so every batch that writes in the quarry worktree also produces a commit in this worktree — see the `two-repo-worktree-authorization` decision in `_mill/discussion.md`.

## Batch 1 — quarry-scaffold

Landed in `/home/knatte/Code/quarry/wts/quarry` (repo `github.com/Knatte18/quarry`, branch `main`):

- Repo scaffolding: `go.mod` (module `github.com/Knatte18/quarry`, go 1.26, direct requires `cobra` v1.10.2, `yaml.v3` v3.0.1, `gofrs/flock` v0.8.1), `.gitignore`, `LICENSE` (Apache-2.0, copied byte-for-byte from Loomyard).
- `README.md` carrying the mandated platform-set, windows-native-strategy-only, and toolchain-cache-re-key statements, plus the config/state precedence chains and the two test tiers.
- The three leaf shared packages copied verbatim: `internal/lock`, `internal/proc`, `internal/output`. Their external test files' self-import (`lock_test.go`, `output_test.go`) was updated from `github.com/Knatte18/loomyard/...` to `github.com/Knatte18/quarry/...`, the one import-path edit required by the module rename itself — no other lines touched.
- The four research/benchmark docs (`scout-spike.md`, `scout-multilang.md`, `scout-agent-usage-findings.md`, `scout-vs-grep.md`) moved into `docs/`, with relative links repointed per the plan's rule.
- `docs/servers.yaml.example`, ported from `internal/scoutengine/template.yaml` with its operator-visible prose reworded for quarry.

Quarry commit SHAs (from `git -C /home/knatte/Code/quarry/wts/quarry log --oneline`):

```
80718f8 docs(quarry): move scout research docs and add servers.yaml example
4206148 feat(quarry): copy lock, proc, and output leaf packages verbatim
db298a9 docs(quarry): README with platform set, windows caveat, and cache re-key note
197d892 chore(quarry): initial import scaffolding from loomyard 1fda8a01c13ec3ec7bb4ef056e5ec9d8aaaac5be
```

`go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...` passes: `internal/lock`, `internal/output`, `internal/proc` all green.
