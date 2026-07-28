# Batch: githubclient

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: githubclient
number: 6
cards: 7
verify: go test -race -count=1 ./internal/githubclient/...
depends-on: []
```

## Batch Scope

Creates `internal/githubclient`, a new and deliberately **thin** package: token resolution, token caching, and construction of an authenticated `*github.Client`. It exposes no per-operation wrapper methods — consumers call go-github's typed API directly (`c.Issues.Create(...)`, `c.PullRequests.List(...)`). Hand-writing wrappers would reinvent a typed, maintained library and create a surface that must track consumer needs forever; the one thing that genuinely cannot be duplicated is non-blocking credential resolution, and that is exactly what this package owns.

The whole design serves one operator requirement: **no credential path may block, prompt, or hang.** `gh auth login` is never invoked, the `gh auth token` shell-out runs under a context timeout, the HTTP client carries its own deadline, and a missing token surfaces as a typed error rather than a wait. lyx runs autonomously, and a process waiting forever on a credential prompt is indistinguishable from a hang.

The batch depends on nothing in the gitrepo work and can run fully in parallel with batches 1–5. The interface batch 7 consumes is the constructor pair: a default constructor and a base-URL/`*http.Client`-taking form, so a test can build a client pointed at an `httptest` server without ever reaching token resolution.

Batch-local decision: `owner` and `repo` are caller-supplied parameters, never resolved inside the client. That is what keeps this package free of any `gitexec`/`gitrepo` import and therefore a genuine leaf.

## Cards

### Card 24: Add the go-github dependency

- **Context:**
  - `internal/selfreportengine/selfreport.go`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `github.com/google/go-github/v75 v75.0.0` as a direct dependency and tidy. Do **not** add `golang.org/x/oauth2` — the authenticated transport is hand-written, and avoiding oauth2 is what keeps this package's import allowlist short enough to enforce as a leaf. `github.com/go-git/go-git/v5` stays pinned at exactly `v5.19.1`: every verdict in the probe reports and the spike was produced against that version, and moving it would invalidate evidence this task depends on. `golang.org/x/sys` is already a direct dependency and needs no change.
- **Commit:** `build: add go-github v75 dependency`

### Card 25: Token resolution chain

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/selfreportengine/selfreport.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/token.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement the resolution chain in order: `GH_TOKEN`, then `GITHUB_TOKEN`, then the cache, then a `gh auth token` shell-out. Environment variables always win over the cache. The shell-out is bounded by `ghAuthTokenTimeout = 5 * time.Second` applied through `exec.CommandContext`, and must call `proc.HideWindow(cmd)` to suppress the Windows console window — the transport being replaced does exactly this today, and omitting it is a visible regression on the platform this is developed on. Put the shell-out behind an **unexported package-level function variable** so a test can inject a hanging fake and assert the timeout fires; a test that would hang forever on regression is the entire point, so it must be written against a seam rather than the real binary. The resolver must return, alongside the token, **which source produced it**, because the 401 policy in card 27 branches on that. An unresolvable token returns a typed error immediately — never a prompt, never a wait, and never an invocation of `gh auth login`.
- **Commit:** `feat(githubclient): add non-blocking token resolution chain`

### Card 26: Token cache file

- **Context:**
  - `internal/githubclient/token.go`
  - `internal/fslink/fslink_windows.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/cache.go`
  - `internal/githubclient/cache_windows.go`
  - `internal/githubclient/cache_other.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement the machine-global token cache at `%LOCALAPPDATA%\lyx\credentials.json` on Windows, and `$XDG_CONFIG_HOME/lyx/credentials.json` falling back to `$HOME/.config/lyx/` elsewhere. **Resolve the directory by reading those environment variables and nothing else** — no hardcoded absolute path, no `os.UserConfigDir` indirection — so a test redirects the whole cache to a temp dir with `t.Setenv` and can never read or write the operator's real credentials. That is the cache's test-isolation seam, and it is required to the same standard as card 25's injectable shell-out: without it card 30's "fully hermetic" requirement is unsatisfiable, because an implementer could follow this card literally and still have the suite touch the real file. A resolution that finds no usable environment variable is a cache miss, not an error. This path is **user config, not repo geometry** — it contains no `_lyx` token and constructs no worktree path — so it deliberately does not route through `internal/hubgeometry`, and the Hub Geometry Invariant is not triggered. It must not live in `_lyx/` (that tree is weft overlay and gets committed and pushed, so a token there would enter git history) nor in `.lyx/`/`.scratch/` (the token is per-user, not per-repo, so a per-repo cache would mean N copies of one secret and would pay the resolution cost again in every repo). Schema: exactly two fields, `token` (string) and `resolved_at` (RFC 3339 UTC). Freshness is `now - resolved_at < 12h` computed at read time; store **no** expiry field, so changing the TTL never requires migrating existing files. Any other field shape, a missing field, or an unparseable timestamp is a **cache miss**, not an error. Writes are an atomic replace: write a temp file in the same directory, apply the 0600 mode **and the Windows ACL stripping to the temp file before the rename**, then `os.Rename` over the target — applying permissions after the rename leaves a window where the real path exists world-readable, which would quietly undo the protection the ACL step exists to provide. Strip the inherited ACLs on Windows through `golang.org/x/sys/windows`, setting an explicit owner-only DACL on the temp file's handle — `SetNamedSecurityInfo` or `SetKernelObjectSecurity` with `PROTECTED_DACL_SECURITY_INFORMATION` so inheritance is broken rather than merely supplemented. Go's `os.WriteFile` permission bits are effectively ignored on Windows, so mode bits alone prove nothing. **The ACL work must be split into build-tagged files, not written inline:** `golang.org/x/sys/windows` is itself `//go:build windows`-gated, so importing it from an untagged `cache.go` breaks `GOOS=linux go build ./...` — which `cmd/lyx/crosscompile_test.go`'s `TestCrossCompileLinux` runs on every Tier-2 suite and which card 46's whole-repo verification would fail on. Mirror `internal/fslink`'s existing three-file split: `cache.go` holds the cross-platform half (directory resolution, schema, freshness, atomic rename) and calls a small unexported hardening hook; `cache_windows.go` (`//go:build windows`) implements that hook with the syscalls; `cache_other.go` (`//go:build !windows`) implements it as the 0600-only no-op. Use an explicit `//go:build !windows` constraint on the non-Windows file rather than relying on a `_linux` filename suffix, so the package still builds on darwin and the BSDs. `internal/fslink/fslink_windows.go` is in Context as the model for error handling and syscall shape. No lock file: the token is idempotent, so a lost race costs one redundant resolution and never a wrong value. A cache directory that cannot be created degrades to in-process resolution rather than failing the command.
- **Commit:** `feat(githubclient): add machine-global token cache with atomic restrictive writes`

### Card 27: Authenticating RoundTripper

- **Context:**
  - `internal/githubclient/token.go`
  - `internal/githubclient/cache.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/transport.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement `authRT`, an `http.RoundTripper` that is the client's outermost application transport and delegates inward to `http.DefaultTransport` or an injected transport. Outbound it **sets** (never appends) the `Authorization` header. Inbound it inspects the status; on `401` it invalidates the cache, re-resolves once, sets the new header, and replays the request exactly once — a second consecutive 401 is returned to the caller unchanged, never a loop. Three details are load-bearing and each is the kind that misbehaves silently. First, the replay **must rewind the request body via `req.GetBody`**: issue creation is a POST with a JSON body, and a naive replay sends an empty body and surfaces as a confusing GitHub validation error rather than as a missing rewind. Second, `authRT` must **clone** the request before setting the header, on both the initial pass and the replay — Go's `RoundTripper` contract forbids modifying the request it is handed, and here the same request object is replayed. Third, when the token came from `GH_TOKEN`/`GITHUB_TOKEN`, **skip the invalidate-and-replay entirely and return the 401**: environment variables outrank the cache by rule, so re-resolution is guaranteed to produce the identical value and the replay is a guaranteed-identical second failure. The returned error must name the environment variable as the rejected source, which is actionable where a silent retry is not. Invalidate-and-replay applies only to cache-sourced or freshly-`gh`-resolved tokens.
- **Commit:** `feat(githubclient): add authenticating RoundTripper with bounded 401 re-resolution`

### Card 28: Client construction

- **Context:**
  - `internal/githubclient/transport.go`
  - `internal/githubclient/token.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/githubclient.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Expose the package's whole public surface: a default constructor returning `(*github.Client, error)` and a second form taking a base URL and an `*http.Client` so tests can point a real go-github client at an `httptest` server. Build the client as `github.NewClient(&http.Client{Timeout: 30 * time.Second, Transport: authRT})`. **Do not use `WithAuthToken`** — it captures a fixed token inside go-github's own transport wrapper, so combined with `authRT` the header would have two owners and the post-401 replay would re-send the stale token from the closure, defeating the entire re-resolution. Exactly one layer owns the `Authorization` header, and it is `authRT`. The 30 s `Client.Timeout` is not decoration: `http.DefaultTransport` sets no response timeout, so without it a stalled connection hangs an autonomous run indefinitely and the 401 replay doubles the wait. Because `Client.Timeout` is enforced through the request context, it covers the original attempt **and** the replay together — worst case bounded at 30 s total, not 30 s per attempt. No per-operation methods: consumers call go-github's typed services directly, and `owner`/`repo` are their parameters to supply.
- **Commit:** `feat(githubclient): add authenticated client constructors`

### Card 29: Leaf enforcement test

- **Context:**
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/proc/proc_windows.go`
  - `internal/githubclient/token.go`
  - `internal/githubclient/cache.go`
  - `internal/githubclient/cache_windows.go`
  - `internal/githubclient/cache_other.go`
  - `internal/githubclient/transport.go`
  - `internal/githubclient/githubclient.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a leaf-enforcement test in the same shape `internal/modelspec`, `internal/tokenvocab`, and `internal/codeintelengine` already use, asserting `githubclient`'s **production** imports are confined to the standard library, go-github, `golang.org/x/sys`, and `internal/proc`. **Build the allowlist from the import strings the production files actually carry, not from this prose:** the allowlist is an exact-string match in the `modelspec`/`tokenvocab` pattern, and the shorthand used here does not match real import paths — go-github's importable package is `github.com/google/go-github/v75/github`, not the bare module path, and the sys dependency is `golang.org/x/sys/windows`. All six production files are in Context for exactly this reason; read them and copy the literal strings, or the guard fails on the very code this plan requires. Remember that `cache_windows.go`'s import set differs from `cache_other.go`'s, so the allowlist must cover the union across build tags rather than whatever the local `GOOS` happens to compile. `internal/proc` is on the allowlist deliberately, not by oversight: the `gh auth token` fallback needs `proc.HideWindow` to avoid flashing a console window on Windows, and `internal/proc` is itself stdlib-only (its Windows build imports `os/exec` and `syscall`; the non-Windows build is a no-op), so allowlisting it does not weaken the leaf property. No `internal/output`, no cobra, no `internal/gitexec`, no `internal/gitrepo`, no `golang.org/x/oauth2`.
- **Commit:** `test(githubclient): enforce leaf import allowlist`

### Card 30: Token, cache, and transport tests

- **Context:**
  - `internal/githubclient/token.go`
  - `internal/githubclient/cache.go`
  - `internal/githubclient/cache_windows.go`
  - `internal/githubclient/cache_other.go`
  - `internal/githubclient/transport.go`
  - `internal/githubclient/githubclient.go`
- **Edits:** none
- **Creates:**
  - `internal/githubclient/githubclient_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Table-driven and fully hermetic — these must run on a machine with no `gh` installed and no GitHub credentials. Every case reaches the shell-out only through card 25's injected seam, and **every case redirects the cache directory with `t.Setenv` to a `t.TempDir()`** before touching anything, so the operator's real credential file is never read or written. Assert that redirection works rather than assuming it: one case points the environment at an empty temp dir and confirms the resolver reports a miss there instead of silently falling back to a real user path. Cover the resolution chain: `GH_TOKEN` wins; `GITHUB_TOKEN` used when the first is empty; both empty falls through to the shell-out; the cache is read when fresh and ignored when past TTL; an environment variable always beats a fresh cache. Cover the cache file: creation with restrictive permissions, with the **Windows ACL effect asserted directly** rather than inferred from mode bits; a corrupt or unparseable file discarded as a miss rather than fatal; a cache directory that cannot be created degrading to in-process resolution rather than failing. Add a concurrent-writer case — several goroutines resolving and writing at once — asserting the file is always either absent or fully parseable, never half-written. Cover the transport against an `httptest` server: a `401` invalidates and re-resolves **exactly once**, never in a loop; a second consecutive 401 propagates. One case exists specifically for the part the design flags as silently misbehaving — a server answering `401` then `201`, asserting the **replayed request's body is byte-identical to the first**, which is what proves the `req.GetBody` rewind happened. Two further cases encode the operator requirement rather than mere behaviour: the shell-out honours its 5 s timeout when the injected fake hangs, and an unresolvable token returns a typed error rather than blocking. Both are tests that would hang on regression, which is the point.
- **Commit:** `test(githubclient): cover token chain, cache, and 401 replay`

## Batch Tests

`verify:` runs `go test -race -count=1 ./internal/githubclient/...` — scoped to the one package this batch creates. These tests are untagged Tier 1: they spawn no process (the `gh` shell-out is behind an injected seam) and need no git fixture, so they must **not** carry a build tag. That also means no `TestMain` calling `lyxtest.HermeticGitEnv` is needed, since the package is not git-spawning under the Hermetic Git Test Environment Invariant's definition.

`-race` is on because the concurrent-writer case is meaningless without it — the cache is deliberately one file shared by every lyx process on the machine, and lyx's premise is many concurrent worktrees and agents, so concurrent access is the normal case rather than an edge case.

A security note the reviewer should treat as load-bearing rather than incidental: this batch writes the operator's token to disk in plaintext in one additional location. `gh` already stores it, so this is not a new class of exposure, but it is one more place to leak from, and the ACL handling in card 26 is the only thing standing between the file's mode bits and irrelevance on Windows.
