# native-clients-migration — replace shell-out CLI wrappers with typed Go clients (git + gh)

> **Status: Design — not built.** Combines two independently-scoped migrations into one task, since both are the same underlying cleanup: replace a shell-out-and-parse-CLI-output client with a typed Go library. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into `internal/gitrepo`'s and `internal/selfreportengine`'s package docs when this lands, and this file is deleted.

## Two migrations, one task

### 1. `gitrepo` → `go-git` (ADOPT-PARTIAL, per the `git-native-library` spike)

The `git-native-library` feasibility spike (done — see `manifest/roadmap.md`'s Done entry and `internal/gitnativepoc/doc.go`) already answered the feasibility question: **ADOPT-PARTIAL**.

- **Migrates cleanly to go-git:** the read surface (`rev-parse`, `diff --name-only`, ref reads), both commit methods, `SetSnapshotSHA`.
- **Stays CLI-bound, permanently — not a TODO:** `Push`'s rebase-retry recovery path (`pull --rebase` / `rebase --abort`) — go-git ships no rebase implementation. This is a deliberate, narrow exception to `gitrepo.Repo`'s otherwise-uniform go-git surface, not a gap to close later.

This task is the actual migration into `internal/gitrepo`. The spike's prototype at `internal/gitnativepoc` was throwaway/reference code, not production — no consumer was migrated by the spike itself, so that work is still fully ahead of us.

### 2. `selfreportengine`'s internal `gh`-CLI transport → `go-github`

Not a replacement of the module — `internal/selfreportengine`'s public entry point, `CreateIssue`, keeps its exact signature and behavior, and every caller (`selfreportcli`, `mill-self-report`, etc.) is unaffected. What changes is only what's *underneath* it: today `CreateIssue` goes through `RunGH`/`realRunGH` (`selfreport.go`), which shells out to the real `gh` binary (`exec.Command("gh", args...)`) and parses its stdout/exit code — the same "parse CLI output as a de-facto API" pattern `gitexec` had for git, on a narrower and more stable surface (issue creation only, today). Replace that transport with `google/go-github`, the standard, well-maintained official REST client. No feasibility spike needed first — this surface is small (one CLI invocation shape) and GitHub's REST API is far more stable/documented than git's porcelain text output, unlike the genuine uncertainty a git-library swap carried.

**Auth wrinkle:** `gh` gets authentication for free via the user's `gh auth login` session/keychain. A `go-github`-based client needs its own token resolution — either an env var (`GH_TOKEN`/`GITHUB_TOKEN`) or a one-time `gh auth token` shell-out at startup to bootstrap the token, kept as the one remaining `gh`-CLI dependency rather than reimplementing GitHub's auth flow.

## Why one task, not two

Both are the same underlying cleanup (typed client over shell-out-and-parse), both are already de-risked (spike done for git; `gh`'s surface here is small and stable), and bundling avoids two separate review/finalize passes for what is conceptually one "stop parsing CLI text as an API" initiative.

## Related

- `internal/gitnativepoc` — the spike's kept prototype and findings write-up this task executes on for the git side.
- `internal/gitexec` — the shell-out layer for git this narrows (only `gitrepo`'s surface, not `gitexec`'s other call-sites — same explicit non-goal the spike had).
- `internal/selfreportengine` — the module the `gh` migration lands in.
