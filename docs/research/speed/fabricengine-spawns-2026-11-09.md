# `internal/fabricengine` tier 1 before and after `e19a2e86`: wall and git processes (2026-11-09)

A measurement report on machine neve, run under the load the machine normally carried.
One run per commit, two runs in all; each figure is one sample.
No code or test changed.

## Answer in brief

- Tier 1 wall fell from 47.7 s to 31.5 s, 16.2 s (34 percent), and git processes fell from 61 291 to 58 693, 2 598 (4.2 percent).
- The 2 598 are `rev-parse` (1 743 fewer), `ls-tree` (452) and `show` (418), the reads of `HEAD` and of committed files that `e19a2e86` serves from Go.
  No other subcommand changed by more than a handful of processes.
- The two runs did not see equal load, so the wall difference is not all the change's:
  the mean one-minute load was 19.3 before and 16.6 after.
  The 2 598 spawns, and the 31 s fewer of git wall time summed over processes (181 s to 150 s), are the part the trace measures directly.
- Most of what is left is not a call site in production code.
  The largest remaining shapes are fixture setup in the test helpers, and the processes git starts itself during push, fetch and clone.
  The read-only production candidates are listed under the ten call sites below.

## Method

Each run was `lym gate test --wait --tier 1 ./internal/fabricengine -- -count=1 -timeout=<n>m -exec "<dir>/shard-runner.sh <out dir> 1"`,
with the timeout differing per run so the gate carries no green record forward.
The script is the one-process form of `fabricengine-shards-2026-11-09/shard-runner.sh`.
It runs the compiled test binary once with `GIT_TRACE2_EVENT` set to a directory, so every git process leaves a trace2 file.
The runs happened in one worktree, checked out detached at each commit in turn, and ran within about a minute of one another.
The script's own sampling of the machine was on in both runs.
`resources.sh` of the same directory counted the git processes.

| Run | Commit | Tier 1 wall (script) | Package time (`ok` line) | Gate tier wall | Load before | Mean sampled load |
|---|---|---|---|---|---|---|
| Before | `e19a2e86^` (`f4ed575e`) | 47.7 s | 49.0 s | 56.8 s | 14.41 20.84 26.24 | 19.3 |
| After | `e19a2e86` | 31.5 s | 32.5 s | 41.7 s | 18.82 21.29 26.08 | 16.6 |
| Difference | | −16.2 s | −16.5 s | −15.1 s | | |

The gate tier wall includes the gate's build and vet; the script's wall does not.
The tier passed in both runs: 1381 tests before and 1392 after, none failed, 174 skipped in each.

## Item 1: git processes before and after

| Figure | Before | After | Difference |
|---|---|---|---|
| Git processes in all | 61 291 | 58 693 | −2 598 |
| Started by the test binary (top level) | 41 126 | 38 526 | −2 600 |
| Started by git itself (children) | 20 172 | 20 174 | +2 |
| Git wall time summed over processes | 181 s | 150 s | −31 s |
| User and system CPU of the binary and its children | 72.6 s and 90.3 s | 66.2 s and 81.9 s | −6.4 s and −8.4 s |
| Hub copies (relocation amends) | 980 | 980 | 0 |

A top-level process is one whose trace2 session id has no `/`; a child's id carries its parent's.

By subcommand, the only changes beyond a handful of processes:

| Subcommand | Before | After | Difference |
|---|---|---|---|
| `rev-parse` | 4 940 | 3 197 | −1 743 |
| `ls-tree` | 743 | 291 | −452 |
| `show` | 523 | 105 | −418 |

By argv shape, the change is `rev-parse HEAD` 1 754 to 11, `show <rev>:<path>` 509 to 91, and `ls-tree --name-only` of `HEAD` and of a revision 452 to 0.
The 91 `show` processes left are `HEAD:.lym-anchor`, `HEAD:.lym-warp` and `HEAD:.lym-shortname` reads and `show -s --format=…`.

## Item 2: the ten subcommands started most often after the change

Counts are from the after run, over all 58 693 processes.
"Top level" is started by the test binary, "child" by another git process.
A subcommand that is only a child (`pack-objects`, `unpack-objects`, `receive-pack`, `upload-pack`) is run by git inside a `push`, `fetch` or `clone`,
so it has no call site of its own and falls with the transfers that start it.

| # | Subcommand | Total | Top level | Child | Read-only or writing |
|---|---|---|---|---|---|
| 1 | `config` | 6 052 | 6 052 | 0 | writing (`--get` is read-only, 667) |
| 2 | `rev-list` | 5 665 | 2 400 | 3 265 | read-only |
| 3 | `commit` | 3 304 | 3 303 | 1 | writing |
| 4 | `rev-parse` | 3 197 | 2 244 | 953 | read-only |
| 5 | `for-each-ref` | 3 184 | 557 | 2 627 | read-only |
| 6 | `pack-objects` | 3 060 | 0 | 3 060 | writing, git-internal |
| 7 | `unpack-objects` | 2 853 | 0 | 2 853 | writing, git-internal |
| 8 | `push` | 2 776 | 2 776 | 0 | writing |
| 9 | `receive-pack` | 2 775 | 0 | 2 775 | writing, git-internal |
| 10 | `upload-pack` | 2 685 | 0 | 2 685 | read-only, git-internal |

`worktree` (2 619) and `add` (2 276) follow.

## Item 3: the ten call sites started most often after the change

The trace holds argv, not a stack, so a shape is attributed to a call site by reading the source for that argv.
Where a shape is also written by a test helper, both are named and the split is not measured.
The shapes are top-level processes of the after run, with variable arguments elided as `<x>`.

| # | Shape (after count, before count) | Call site | Read-only or writing |
|---|---|---|---|
| 1 | `config <key> <value>` (2 274, 2 274) | `user.email` and `user.name` after `git init` in the `internal/gitkit` and `internal/hubforge` fixtures | writing, test fixture |
| 2 | `add -- <x>` (2 025, 2 021) | `internal/gitkit/query.go` `CommitFile`; production only in `addanchor.go` | writing, mostly fixture |
| 3 | `commit -m <x> -- <x>` (1 613, 1 610) | `internal/gitkit/query.go` `CommitFile` | writing, fixture |
| 4 | `rev-list --count @{u}..HEAD` (1 523, 1 523) | `internal/gitrepo/push.go`, the count of commits ahead of upstream | read-only, production |
| 5 | `config --unset <key>` (1 312, 1 312) | `internal/gitrepo/push.go` `DropRemoteTracking` | writing, production |
| 6 | `ls-files -- <x>` (1 290, 1 290) | `internal/fabricengine/pathtracked.go` and `internal/gitkit/query.go` | read-only, production and fixture |
| 7 | `ls-remote --heads origin <ref>` (1 137, 1 137) | `internal/fabricengine/pushclassified.go` before a push, and `recordsprobe.go` | read-only, production |
| 8 | `config maintenance.auto false` (1 122, 1 122) | `internal/fabricengine/housekeeping.go` | writing, production |
| 9 | `worktree repair <x>` (1 021, 1 021) | `internal/fabricengine/destroy.go` | writing, production |
| 10 | `push --force --quiet origin HEAD` and `commit --amend --no-edit --quiet -- <x>` (980 each, 980 each) | `internal/hubforge/template.go`, the hub copy relocation | writing, fixture |

Production read-only shapes that follow the ten, as the input for a next task that serves read-only calls from Go:

| Shape | After count | Call site |
|---|---|---|
| `diff --cached --quiet -- <x>` | 782 | `internal/gitrepo/merge.go` |
| `diff --name-only --diff-filter=U -z` | 648 | `internal/gitrepo/merge.go` |
| `remote get-url --push origin` | 966 | `internal/gitrepo/pushdestination.go` |
| `status --porcelain -z --untracked-files=no` | 594 | `internal/fabricengine/dirtiness.go` |
| `status --porcelain` | 574 | `internal/fabricengine/dirtiness.go` |
| `rev-parse --git-path hooks` | 567 | `internal/fabricengine/hook.go` |
| `for-each-ref --format=%(objectname) <ref>` | 520 | `internal/fabricengine/reflog.go`, `destroy.go`, `snapshotuncommitted.go` |
| `remote` | 477 | `internal/fabricengine/add.go` |
| `branch --format=%(refname:short)%(worktreepath)` | 476 | `internal/fabricengine/cleanup.go`, `destroy.go` |
| `diff --name-only` of two revisions | 1 791 in all `diff` | `internal/fabricengine/originreconcilemoves.go` `diffNames`, `configchanges.go` |

## Limits

- One run per commit; the spread between two runs of one case was 5 percent in the earlier report, and the two runs here saw different load.
- Which test fixture or production function started a shape is read from source, not traced; a shape named for both is not split.
- The runs shared the machine with other sessions' builds and tests, whose load the trace does not separate from the package's own.
