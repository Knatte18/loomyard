# Plan: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
slug: native-clients-migration
approved: false
started: '20260728-090000'
parent: main
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gogit-handle
    file: 01-gogit-handle.md
    depends-on: []
    verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
  - number: 2
    name: parity-oracle
    file: 02-parity-oracle.md
    depends-on: [1]
    verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
  - number: 3
    name: migrate-core-reads
    file: 03-migrate-core-reads.md
    depends-on: [2]
    verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
  - number: 4
    name: migrate-snapshot-push-reads
    file: 04-migrate-snapshot-push-reads.md
    depends-on: [3]
    verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
  - number: 5
    name: retire-poc-and-measure
    file: 05-retire-poc-and-measure.md
    depends-on: [4]
    verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
  - number: 6
    name: githubclient
    file: 06-githubclient.md
    depends-on: []
    verify: go test -race -count=1 ./internal/githubclient/...
  - number: 7
    name: selfreport-transport
    file: 07-selfreport-transport.md
    depends-on: [6]
    verify: go test -race -count=1 ./internal/selfreportengine/... ./internal/selfreportcli/...
  - number: 8
    name: guards
    file: 08-guards.md
    depends-on: [5, 7]
    verify: go test -count=1 ./cmd/lyx/... ./tools/sandbox/... ./internal/gitrepo/...
  - number: 9
    name: docs-and-invariants
    file: 09-docs-and-invariants.md
    depends-on: [8]
    verify: go test -count=1 ./cmd/lyx/... ./internal/gitrepo/... ./internal/githubclient/...
```

The DAG has two independent roots. Batches 1–5 are the gitrepo migration and run in strict sequence, because each one flips a backend the previous one's harness measures. Batches 6–7 are the GitHub side and share no file with the gitrepo chain, so they can run fully in parallel with it. Batch 8 joins both (its guards can only pass once both migrations have landed), and batch 9 lands the documentation last, because several doc sites cannot be written truthfully until the measurements in batch 5 exist.

## Shared Decisions

### Decision: the go-git/CLI boundary is local reads vs. remote-or-mutating

- **Decision:** go-git handles local object and ref access; the git CLI keeps anything that authenticates to a remote or mutates the working tree. Migrating: `CurrentSHA`, `SHAExists`, `ChangedFilesSince`, `CurrentBranch`, `remoteName`, `hasUnpushed`, `isStrictDescendant`, `SnapshotSHA`'s ref read, and `SetSnapshotSHA`'s two inline local reads. Staying on the CLI: `StageAndCommit`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `Pull`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `SetSnapshotSHA`'s push, `SnapshotSHA`'s fetch.
- **Rationale:** it is statable in one sentence, it maps exactly onto what the evidence shows go-git does correctly on this platform, and it lands the value where it actually is — every string-parsing site in `gitrepo` is on the read side. Each exclusion is a measured decision, not an omission: go-git performs **no CRLF conversion at all** (a file it commits under `core.autocrlf=true` is thereafter permanently "modified" to CLI git); its hard reset **deletes untracked and gitignored files**; `Pull` and `Checkout` are **non-atomic**, moving HEAD before the dirty check, with `Pull` failing 25 of 40 trials on a stock Windows checkout; and go-git **never invokes a git credential helper**, so every remote operation would fail against this repo's HTTPS remote behind Git Credential Manager. `Push`'s rebase-retry is permanently CLI-bound — go-git ships no rebase implementation at all.
- **Applies to:** batches 1–5, 8, 9.

### Decision: the boundary is call-granular, not method-granular

- **Decision:** classification is per call, not per method. Four CLI-bound methods call helpers that migrate, so each becomes a go-git read immediately after a CLI write on a handle opened before that write. The sites, exhaustively: `StageAndCommit` and `StageAllAndCommit` each end with `r.CurrentSHA()`; `SetSnapshotSHA` calls `remoteName` and `isStrictDescendant`, and its CLI-side push writes the very ref a later `SnapshotSHA` reads via go-git; `SnapshotSHA` calls `remoteName` **before** its CLI fetch, then reads the ref via go-git; `PushCoalesced` gates on `hasUnpushed`.
- **Rationale:** these are the interop surface and must be treated as such rather than discovered mid-implementation. The failure mode is silent and severe — `StageAndCommit` returning a stale SHA after a successful commit would feed a wrong value straight into `SetSnapshotSHA`, recording a snapshot that points off-history. Being call-granular is also what makes the boundary invariant's "justify every `gitexec` call" rule answerable: the unit it asks about is a call.
- **Applies to:** batches 3, 4, 8.

### Decision: every object lookup goes through the fingerprint-gated helper

- **Decision:** no migrated read calls the storer directly. All object lookups route through the shared helper from batch 1, which on an `object not found` compares a pack fingerprint — the sorted `(name, size)` list of `*.idx` files in the common dir's `objects/pack` — against the one recorded at the last index build, reindexes and retries once only if it differs, and otherwise returns the not-found as truth.
- **Rationale:** go-git's object index is built once and never refreshed, so an object living only in a packfile written after the handle indexed reads as absent while `Head()` returns that very SHA. The gate must be on-disk state rather than a per-`Repo` call counter, because one physical checkout is addressed concurrently by several live `Repo` values and by a separate OS process. Routing *every* read through the helper is what makes the remedy reachable at all: `SHAExists`, `isStrictDescendant` and `hasUnpushed` swallow failure into `false`/`false`/`true`, so an error never escapes them and a trigger placed anywhere else would be structurally unreachable from exactly the methods most likely to hit it.
- **Applies to:** batches 1, 3, 4.

### Decision: RWMutex spanning every go-git call

- **Decision:** RLock for the duration of each go-git call; Lock for handle initialization and for the fingerprint-check/reindex/retry sequence as one unit. Every gitrepo batch runs its tests with `-race`.
- **Rationale:** guarding only the reindex does not deliver the guarantee it claims — go-git's `filesystem.Storage` builds its object index lazily on first read, so even two reindex-free concurrent first reads mutate shared state. This is not theoretical: `internal/gitrepo/push_test.go` already drives two goroutines through one shared `*Repo`. `Repo`'s godoc says concurrent *writes* are the caller's problem; this makes concurrent *reads* safe internally without widening that contract.
- **Applies to:** batches 1, 3, 4, 5.

### Decision: the parity harness needs its own CLI oracle

- **Decision:** every migrated method is asserted against the **real git CLI** on the same fixture, never against a hand-written expectation — and the oracle is an independent test-only layer built on raw `gitexec.RunGit`, not a call back into `gitrepo`.
- **Rationale:** a go-git bug that is *consistently* wrong passes a hand-written test and fails a parity test. But the harness being lifted uses `gitrepo`'s own CLI methods as its reference side, so a straight copy would compare go-git against go-git — asserting nothing while still passing green, which is worse than no test because it looks like coverage. The output parsing production is deleting (the `-z` NUL split, the `--verify --quiet` exit conventions, the unborn-HEAD stderr match) therefore lives on in test code, precisely so the thing it validates can stop depending on it.
- **Applies to:** batches 2, 3, 4, 5.

### Decision: the linked worktree is the only topology that counts

- **Decision:** the handle opens with `git.PlainOpenWithOptions(path, &git.PlainOpenOptions{EnableDotGitCommonDir: true})`. `PlainOpen` and `DetectDotGit: true` are banned outright, `KeepDescriptors` stays `false`, and the parity suite runs against a linked-worktree fixture with the two worktrees on different branches at different HEADs, reached both directly and through a junction.
- **Rationale:** against a linked worktree, `PlainOpen` **returns no error** and hands back a handle that cannot read HEAD, cannot read any object, and reports existing snapshot refs as **absent** — `SnapshotSHA` would report every key as missing, forever, with no error. Eight methods appeared to pass on that broken handle. `DetectDotGit` is worse: it walks *up* and silently opens the parent repository. `refs/loomyard/snapshot/*` lives in the shared common dir while `HEAD` is per-worktree, so a commondir mishandling surfaces as a wrong value rather than a clean error, and a standalone `git init` fixture cannot see any of it. This checkout's own `.git` is a file; `fabricengine` creates both host and weft as linked worktrees; `internal/fslink` reaches them through junctions. This is not an edge case — it is production.
- **Applies to:** batches 1, 2, 3, 4.

### Decision: no credential path may block, prompt, or hang

- **Decision:** `gh auth login` is never invoked. The `gh auth token` shell-out is bounded at 5 s via `exec.CommandContext` behind an injectable seam; the HTTP client carries a 30 s timeout covering the original attempt and the 401 replay together; a missing or unusable token surfaces as a typed error through the `output.Err` envelope with a non-zero exit code.
- **Rationale:** an operator requirement and the single most important property of the GitHub side. lyx runs autonomously, and a process waiting forever on a credential prompt is indistinguishable from a hang. It is also why the `git credential fill` bridge was rejected for gitrepo's remote operations: Git Credential Manager can raise a GUI prompt that no environment variable reliably suppresses. Treating a missing token as a soft failure that degrades silently was rejected — a self-report that quietly does not file is worse than one that fails loudly.
- **Applies to:** batches 6, 7, 8, 9.

### Decision: githubclient is auth-only, with no per-operation methods

- **Decision:** `internal/githubclient` owns token resolution, token caching, and construction of an authenticated `*github.Client`, and nothing else. Consumers call go-github's typed API directly; `owner` and `repo` are caller-supplied parameters.
- **Rationale:** hand-writing wrappers reinvents a typed, maintained library and creates a surface that must track consumer needs forever. The one thing that genuinely cannot be duplicated is non-blocking credential resolution — duplicate it and you get two token chains, two shell-outs, two timeouts to forget. Under this shape, adding a GitHub operation costs zero package work, which is what makes the coming finalize module's PR create/close needs a non-event. Caller-supplied `owner`/`repo` is also what keeps the package free of any `gitexec`/`gitrepo` import and therefore a genuine leaf. `selfreportengine` is explicitly **not** extended.
- **Applies to:** batches 6, 7, 9.

### Decision: exactly one layer owns the Authorization header

- **Decision:** the header is set only by `githubclient`'s own `http.RoundTripper`, which clones the request, sets (never appends) the header, and on a 401 invalidates the cache, re-resolves once, rewinds the body via `req.GetBody`, and replays exactly once. `github.WithAuthToken` is banned.
- **Rationale:** `WithAuthToken` captures a fixed token inside go-github's own transport wrapper, so combined with the 401-invalidating transport the header would have two owners and the replay would re-send the *stale* token, defeating the entire re-resolution. One owner removes the ordering question rather than answering it. The `GetBody` rewind is not incidental: issue creation is a POST with a JSON body, and a naive replay sends an empty body and surfaces as a confusing GitHub validation error rather than as a missing rewind.
- **Applies to:** batches 6, 7.

### Decision: the public surface does not change and no caller is touched

- **Decision:** `gitrepo`'s exported API, `CreateIssue`'s signature and behaviour, and `selfreport`'s CLI flags, arg validation, and JSON output envelope are all unchanged. `New` keeps its no-I/O, cannot-fail contract. `Repo.run` stays byte-unchanged. `boardengine`, `fabricengine`, and `websterengine` call `gitrepo` exactly as they do today.
- **Rationale:** this is the constraint that keeps a foundation swap reviewable. If any caller needs editing, something has gone wrong and it must be raised rather than patched at the call site — card 23 checks exactly that. `New` performing no validation is load-bearing: it is why construction cannot fail and why `gitrepo` does not care whether the checkout exists yet, which in turn is why only a *successful* handle open may be cached.
- **Applies to:** all batches.

### Decision: both new invariants ship with machine enforcement

- **Decision:** the GitHub Auth Invariant and the gitrepo Client Boundary Invariant land in `CONSTRAINTS.md` with guard tests in the same commit, not as review obligations.
- **Rationale:** every other entry in that file names its enforcement, and a rule with no mechanism decays into prose. The failures both prevent are the slow kind review discipline misses: CLI calls seeping back into `gitrepo` one bugfix at a time, and a future module growing its own `gh` shell-out until there are two credential paths and only one has a timeout. Both guards must be line-based rather than literal-matching, because `exec.CommandContext("gh"` never appears in compilable Go — a naive literal would miss the exact call shape this task introduces, which is the same latent hole the guard being copied from already has and which card 36 fixes.
- **Applies to:** batches 8, 9.

## All Files Touched

_This list is the union of every card's `Creates:` and `Edits:` paths, by the format's own definition. **Deleted paths are deliberately absent**, the same way Move source paths are: `internal/gitnativepoc/`'s eight files (card 22) and `manifest/designs/native-clients-migration.md` (card 45) are dropped by this plan and appear only in their cards' `Deletes:` fields._

- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/ghguard_test.go`
- `cmd/lyx/gitrepoboundary_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/overview.md`
- `go.mod`
- `go.sum`
- `internal/githubclient/cache.go`
- `internal/githubclient/cache_other.go`
- `internal/githubclient/cache_windows.go`
- `internal/githubclient/doc.go`
- `internal/githubclient/githubclient.go`
- `internal/githubclient/githubclient_test.go`
- `internal/githubclient/leaf_enforcement_test.go`
- `internal/githubclient/token.go`
- `internal/githubclient/transport.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/fixtures_test.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitrepo/gogit.go`
- `internal/gitrepo/gogit_test.go`
- `internal/gitrepo/oracle_test.go`
- `internal/gitrepo/parity_test.go`
- `internal/gitrepo/push.go`
- `internal/gitrepo/snapshot.go`
- `internal/selfreportcli/cli.go`
- `internal/selfreportcli/cli_test.go`
- `internal/selfreportengine/selfreport.go`
- `internal/selfreportengine/selfreport_test.go`
- `manifest/roadmap.md`
- `tools/sandbox/pathresolve_guard_test.go`
