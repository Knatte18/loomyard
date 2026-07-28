# Discussion: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
slug: native-clients-migration
status: discussing
parent: main
```

## Problem

Two packages in lyx treat a command-line tool's text output as if it were an API. `internal/gitrepo` shells out to `git` through `internal/gitexec` and reads back exit codes, stdout strings, and stderr substrings — `rev-parse` output trimmed as a SHA, `diff --name-only -z` split on NUL, `push` failures classified by grepping stderr for `"non-fast-forward"`. `internal/selfreportengine` does the same to `gh`: it builds an argv, runs the binary, and takes the last non-empty line of stdout as the created issue's URL, then parses the trailing path segment as the issue number. Both are brittle in the same way — the contract is whatever the tool happened to print in the version installed on the machine.

Why now: the `git-native-library` feasibility spike is done (see `internal/gitnativepoc/doc.go`) and returned **ADOPT-PARTIAL**, so the git side is de-risked and this task is `manifest/roadmap.md`'s Planned #3. Separately, a **finalize** module is coming that must create and close pull requests. That makes the GitHub side no longer a one-consumer concern: without a shared place to resolve credentials, finalize and selfreport would each grow their own token handling, and each would be a place where an autonomous run can stall waiting for a credential prompt.

The discussion widened the task's evidence base beyond the spike in two ways, and both changed the answer. A background probe measured the five `gitrepo` methods the spike never examined, plus two Windows hazards, on Windows 11 (report: `.scratch/gogit-probe-report.md`; throwaway probe module: `.scratch/gogitprobe/`). A second background agent inventoried every GitHub operation the sibling **millhouse** toolchain performs, so lyx's GitHub surface is derived from real usage rather than from the design doc's single-operation assumption (report: `.scratch/millhouse-gh-inventory.md`).

## Scope

**In:**

- `internal/gitrepo` — migrate the **local read surface** to go-git v5.19.1: `CurrentSHA`, `SHAExists`, `ChangedFilesSince`, `CurrentBranch`, `remoteName`, `hasUnpushed`, `isStrictDescendant`, and the ref-read half of `SnapshotSHA`. Public surface unchanged; no caller is touched.
- `internal/gitrepo` — add a lazily-opened, cached `*git.Repository` handle without changing `New`'s no-I/O, cannot-fail contract.
- `internal/githubclient` — **new package**, deliberately thin: token resolution, token caching, and construction of an authenticated `*github.Client`. It contains no per-operation wrapper methods.
- `internal/selfreportengine` — replace the `gh`-CLI transport under `CreateIssue` with a go-github call through `githubclient`. `CreateIssue`'s signature and behaviour are unchanged. The exported `RunGH` seam, `realRunGH`, `buildCreateArgs`, and `lastNonEmptyLine` are deleted.
- `internal/selfreportcli` — its tests move from a fake `RunGH` to an `httptest` server; the CLI code itself does not change.
- Delete `internal/gitnativepoc`, lifting its differential-parity harness into `internal/gitrepo`'s integration tests.
- `CONSTRAINTS.md` — two new invariants (see Decisions `constraints-github-auth-invariant` and `constraints-gitrepo-boundary-invariant`).
- Documentation lifecycle: delete `manifest/designs/native-clients-migration.md`, fold the durable findings into the three packages' godoc, move `manifest/roadmap.md` Planned #3 to Done.

**Out:**

- **`gitexec`'s other call sites.** `fabricengine`, `builderengine`, `hubgeometry`, `websterengine`, `fabriccli` and the test fixtures keep shelling out. This task narrows only `gitrepo`'s use of `gitexec` — the same explicit non-goal the spike had.
- **`gitrepo`'s write and remote surface.** `StageAndCommit`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `Pull`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `SetSnapshotSHA`, and `SnapshotSHA`'s best-effort fetch all stay on the git CLI. Each is a measured decision below, not an omission.
- **The finalize module itself.** This task builds the client finalize will consume; it does not build finalize, and adds no PR-creating caller.
- **Caller changes.** `boardengine`, `fabricengine`, and `websterengine` call `gitrepo` exactly as they do today. If any caller needs editing, something has gone wrong.
- **New cobra modules or subcommands.** `githubclient` is a library package, so the CLI/Cobra Invariant's registration, help-tree, and `Short` requirements are not triggered, and neither is Sandbox Suite Coverage.
- **GitHub operations with no lyx consumer** — labels CRUD, releases, reviews, review comments. Millhouse uses none of them either.
- **A `gitrepo` → `githubclient` dependency.** `gitrepo` stays host-agnostic; it never learns what GitHub is.

## Decisions

### gogit-boundary-local-vs-remote

- Decision: the go-git/CLI boundary in `gitrepo` is **local object and ref access vs. anything that authenticates to a remote or mutates the working tree**. go-git handles the former; the CLI keeps the latter.
- Rationale: it is a boundary a reader can state in one sentence, it maps exactly onto what the evidence shows go-git can do correctly on this platform, and it is mechanically checkable (see `constraints-gitrepo-boundary-invariant`). It also lands the value where it actually is: every string-parsing site in `gitrepo` is on the read side — `rev-parse` output, `diff --name-only -z` NUL-splitting, the `ambiguous argument 'HEAD'` / `unknown revision` stderr sniff that produces `ErrNoCommits`, and the exit-code-1-means-absent convention. The write side reads exit codes only, so it has almost nothing to gain from typing.
- Rejected: the brief's original boundary (reads + both commit methods + `SetSnapshotSHA`, CLI only for `Push`'s rebase-retry). It was set from a Linux-only spike that could not observe the two Windows hazards or the credential-helper gap. Also rejected: migrating everything and accepting the divergences — each one is silent, which is the worst failure mode available.

### commit-methods-stay-cli

- Decision: `StageAndCommit` and `StageAllAndCommit` remain on the git CLI, despite the spike marking both MIGRATE.
- Rationale: **go-git performs no CRLF conversion at all.** Probe evidence, identical fixtures with `core.autocrlf=true` and a CRLF-line-ending source file: the CLI stores blob `0c2aa38e0600`, go-git stores blob `4e7cdf2bf3ef` — different bytes, different object name. A `.gitattributes` `text=auto` entry changes nothing. The consequence is not cosmetic: a file committed by go-git is subsequently seen as permanently modified by CLI git, which converts the working-tree CRLF to LF and compares against a stored CRLF blob. `core.autocrlf=true` is the Git-for-Windows system default on this machine, and the weft overlay files these methods commit are written by agents in editors that default to CRLF on Windows. This **reopens gate (c)** of the spike's own rubric for both methods; their MIGRATE verdicts were reached on Linux, where `autocrlf` defaults to `false`.
- Rationale, second half: keeping commits on the CLI also disposes of the hermetic-test problem entirely. go-git ignores `GIT_CONFIG_GLOBAL` and `GIT_CONFIG_NOSYSTEM` completely (zero occurrences in the module source; confirmed empirically — under identical env the CLI committed as `HermeticEnvIdentity` while go-git committed as `HomeGitconfigIdentity`, the operator's real identity). Every go-git commit path would therefore need an explicitly-constructed `CommitOptions.Author` just to keep `lyxtest.HermeticGitEnv` honest. With commits on the CLI, no go-git code path ever needs an author signature — `SetSnapshotSHA` writes refs, not commits, and it stays on the CLI anyway.
- Rejected: forcing `core.autocrlf=false` in the repos lyx commits to — `websterengine` commits to the operator's own host repo, so this would mutate a user repo's config to suit lyx. Rejected: implementing CRLF normalization inside `gitrepo` — `Worktree.Add` reads from disk, so blobs would have to be constructed by hand, and full `.gitattributes` semantics is a large surface to reimplement. Rejected: a platform-conditional commit path — most future lyx use is expected to be on Linux where the hazard is absent, but a commit path that is correct on one OS and silently wrong on another is worse than one that is correct everywhere.

### worktree-mutating-methods-stay-cli

- Decision: `ResetHard`, `Pull`, `CheckoutDetached`, and `RestoreBranch` remain on the git CLI. The probe's verdicts for the first three are CLI-BOUND on hard evidence; `RestoreBranch` is a judgment call recorded separately below.
- Rationale — `ResetHard`: **go-git's hard reset deletes untracked and gitignored files.** Probe `probeResetUntracked`, identical fixtures: the CLI leaves `untracked.txt`, `deep/dir/untracked.txt`, `ignored/artifact.bin` and `debug.log` in place; go-git removes every one of them and returns `err=<nil>`. In a lyx checkout that silently destroys `.lyx/`, `.scratch/`, and build output. `ResetHard` is what `RevertWithWeft`'s history recovery is built on, so this is a data-loss path in a recovery flow.
- Rationale — `Pull` and `CheckoutDetached`: **both are non-atomic.** go-git calls `updateHEAD`/`setHEADToCommit` *before* the `Reset(MergeReset)` that rejects unstaged changes. Observed: `err=worktree contains unstaged changes` with HEAD already moved from `refs/heads/main` to `HEAD` at the target SHA, the working tree still holding the old content, and `git status` reporting `MM a.txt`. The CLI aborts having moved nothing. Additionally, `Pull` failed **25 of 40 trials** with a spurious `ErrUnstagedChanges` on a stock Windows checkout with `core.autocrlf=true` (0 of 40 with `autocrlf=false`) — HEAD had already moved in all 25.
- Rationale — what the probe *cleared*: `Pull`'s fast-forward-only contract, the thing the method's godoc treats as inviolable, holds. A genuinely diverged pull returned `non-fast-forward update` with `merges=0` and `headMoved=false`. go-git never silently merges. That risk was real and is now closed, but it is not enough on its own against the two failures above.
- Rejected: migrating `Pull` and accepting a 60% spurious-failure rate on Windows. Rejected: migrating `ResetHard` with a hand-rolled untracked-file rescue around it — reconstructing what the CLI leaves alone means reimplementing gitignore evaluation, to protect against a library behaviour we do not want in the first place.

### checkout-pair-kept-together

- Decision: `RestoreBranch` stays on the CLI even though the probe returned MIGRATE for it.
- Rationale: this overrides a per-method verdict deliberately, and the reason should not be lost. `CheckoutDetached` and `RestoreBranch` are a matched pair in one flow — the integration bisect captures a branch with `CurrentBranch`, detaches per candidate, and restores at the end. Both go through go-git's `Worktree.Checkout`, and the non-atomicity finding above is a property of `Checkout` itself, not of the detached variant. The probe judged each method in isolation; the pair is the real unit. Splitting the pair across two backends buys nothing and adds a failure mode where the *recovery* step can leave the operator detached with HEAD already moved.
- Rejected: following the probe verdict literally and migrating `RestoreBranch` alone.

### remote-operations-stay-cli

- Decision: `Push`, `PushCoalesced`, `SetSnapshotSHA`, and the best-effort fetch inside `SnapshotSHA` remain on the git CLI. `SnapshotSHA`'s ref *read* migrates to go-git; only its fetch half does not.
- Rationale: **go-git never invokes a git credential helper.** Its only `credential` occurrences are doc comments on the `Auth` option field. It accepts an explicit `transport.AuthMethod` (`http.BasicAuth`, `http.TokenAuth`, `ssh.PublicKeys`, `ssh.NewSSHAgentAuth`) but has no mechanism to discover one. This repo's remote is HTTPS (`https://github.com/Knatte18/loomyard.git`) and the machine resolves credentials through `credential.helper=manager`. Both the spike and the probe exercised go-git's push and fetch against a **local bare repository on disk**, where no authentication occurs at all — so neither run touched this. Against the real remote, `SetSnapshotSHA`'s push would fail outright, and `SnapshotSHA`'s fetch would fail on every call. The fetch failure is the more dangerous of the two precisely because the method swallows fetch errors by design: snapshot refs would silently stop being shared across clones, which is the entire purpose of pushing them to a remote.
- Rationale, second half: `Push`'s rebase-retry recovery is independently CLI-bound and permanently so — go-git v5.19.1 ships no rebase implementation whatsoever. That was the spike's pivotal finding and is not an OS-specific behaviour any later run can change.
- Rejected: feeding go-git an `http.TokenAuth` built from `githubclient`'s token. It would work here, but it makes `gitrepo` fail against any non-GitHub remote and couples two packages that are otherwise independent — `gitrepo` is a handle on an arbitrary checkout and must not learn about one forge. Rejected: bridging through a `git credential fill` shell-out. It keeps the git CLI dependency on the very path we would be migrating, and Git Credential Manager is a GUI helper that can raise a window when nothing is cached — `GIT_TERMINAL_PROMPT=0` suppresses git's own terminal prompt but not GCM's — which violates the never-block-on-credentials requirement below.

### lazy-cached-repo-handle

- Decision: `Repo` gains an unexported `*git.Repository` field opened lazily on first use via `sync.Once`, with the open error stored alongside and returned by whichever method needed it. `New(path) *Repo` keeps its documented no-I/O, cannot-fail contract.
- Rationale: `New` performing no validation and no I/O is load-bearing — it is why construction cannot fail and why `gitrepo` does not care whether the checkout exists yet. Opening eagerly would force `New` to return an error and change every call site, contradicting the public-surface-unchanged requirement. `sync.Once` also gives the right concurrency posture for free: the type already documents that concurrent writes to one `Repo` are the caller's problem, but concurrent *reads* must not race on handle initialization.
- Rejected: opening per call — correct but pays repository and config discovery on every read, and `SnapshotSHA`/`SetSnapshotSHA` call several read helpers in sequence. Rejected: eager open in `New` — breaks the caller contract.

### gitexec-seam-unchanged

- Decision: `Repo.run` stays exactly as it is, delegating to `gitexec.RunGit`, and remains the single choke point for every CLI-bound method. A separate unexported accessor returns the go-git handle.
- Rationale: keeping `run` untouched means the CLI-bound methods are not edited at all by this task, which keeps the diff honest about what actually changed. The go-git/CLI split is documented on the package doc and enforced by the new invariant rather than encoded in identifier names.
- Rejected: renaming `run` to `runCLI` and pairing it with a `goGit()` accessor. It makes the split visible at call sites, but it edits every CLI-bound method for cosmetic reasons and inflates the diff of a task that is already touching the git foundation every other module sits on.

### delete-gitnativepoc-lift-harness

- Decision: delete `internal/gitnativepoc` entirely, but lift its differential-parity harness into `internal/gitrepo`'s integration tests first.
- Rationale: the prototype's purpose ends the moment production carries the real implementation; keeping it means two divergent copies of the same go-git logic, and the poc's copy has no consumer to keep it honest. The harness, however, is the genuinely valuable artifact — it is what caught the `--no-renames` rename-folding trap and the `@{u}` `ResolveRevision` gap during the spike, and it is the only mechanism that will catch a regression when go-git is next upgraded.
- Rejected: keeping the package as a permanent oracle — that is what the lifted harness does, in the package that actually ships. Rejected: deleting the code and keeping `doc.go` as a findings-only package — the findings belong in `gitrepo`'s godoc, next to the code they describe.

### githubclient-thin-auth-only

- Decision: `internal/githubclient` owns token resolution, token caching, and construction of an authenticated `*github.Client`. It exposes **no per-operation methods**. Consumers call go-github's typed API directly — `c.Issues.Create(ctx, owner, repo, req)`, `c.PullRequests.List(ctx, owner, repo, opts)`.
- Rationale: hand-writing `CreateIssue`/`ListIssues`/`CreatePR` wrappers over go-github reinvents a typed, documented, maintained library and creates a surface that must be kept in sync with consumer needs forever. The one thing that genuinely cannot be duplicated is non-blocking credential resolution: duplicate it and you get two token chains, two `gh` shell-outs, two timeouts to forget. Adding a GitHub operation costs zero package work under this shape.
- Decision, refinement: `owner` and `repo` are **caller-supplied parameters**, not resolved inside the client. `selfreportengine` keeps its existing `targetRepo` constant and passes it; finalize will pass its own. This keeps `githubclient` free of any `gitexec`/`gitrepo` import and therefore a genuine leaf.
- Rejected: extending `selfreportengine` in place — the operator ruled it out explicitly, and rightly: the package is named for one job, its `targetRepo` is hardcoded to `Knatte18/loomyard`, and it is on the Sandbox Suite Coverage allowlist precisely because `create` files a real issue. Rejected: a full `githubengine` + `githubcli` cobra module — it would give `lyx github …` verbs nobody has asked for, and pull in registration, help-tree, and sandbox-coverage obligations for no consumer. Rejected: no package at all, each consumer building its own client — see the duplication argument above.

### github-operation-surface

- Decision: the GitHub surface lyx must support is the set millhouse already uses: `issue list`, `issue view`, `issue create`, `issue close` (comment-then-close), `pr list`, `pr view`, `pr create`, `repo view`, and an auth probe. Under `githubclient-thin-auth-only` these are go-github calls at the consumer, not methods on the client, so "supporting" them means the client's auth and construction must be sufficient for all of them — which it is, since they are all standard authenticated REST calls.
- Rationale: derived from a real inventory (`.scratch/millhouse-gh-inventory.md`) rather than from the design doc's issue-creation-only assumption. Two details from that inventory matter to any consumer and are recorded here so they are not rediscovered the hard way: `gh issue view` exits 0 on a **closed** issue, so millhouse reads the `state` field to distinguish — a consumer that only checks for success will treat closed issues as open; and `gh pr list` results are consumed with an explicit state precedence of MERGED > OPEN > CLOSED.
- Rejected: building only issue creation as the design doc specifies — finalize is a known second consumer that needs PR create and close, so the one-operation scope was already stale when written. Rejected: pre-building wrapper coverage for all nine operations — under the thin-client decision there is nothing to pre-build.

### token-resolution-and-cache

- Decision: resolve the GitHub token in order — `GH_TOKEN`, then `GITHUB_TOKEN`, then a `gh auth token` shell-out. The shell-out's result is cached to a user-level file (`%LOCALAPPDATA%\lyx\credentials.json` on Windows, `$XDG_CONFIG_HOME/lyx/credentials.json` falling back to `$HOME/.config/lyx/` elsewhere) with a **12-hour TTL**. Environment variables always win over the cache. A `401` from GitHub invalidates the cache and triggers exactly one re-resolution.
- Rationale: measured, not assumed — `gh auth token` costs ~310 ms per call on this machine (306/313/323 ms over three runs). `lyx` is a CLI, so every invocation is a fresh process and in-memory caching alone would pay that cost on every command in an autonomous loop. The environment-variable path costs nothing, which is why it is first. The cache exists to make the fallback path cheap, not to be the primary mechanism.
- Decision, location: the cache is **machine-global and outside every repository**. It must not live in `_lyx/` — that tree is weft overlay and gets committed and pushed, so a token there would enter git history. It must not live in `.lyx/` or `.scratch/` either: a GitHub account token is per-user, not per-repo (`gh auth token` returns the same value regardless of cwd), so a per-repo cache would mean N copies of one secret, N places to leak from, and the 310 ms cost paid again in every repo.
- Decision, permissions: the cache file is created 0600, and on Windows its inherited ACLs are stripped explicitly so the mode bits are not merely decorative — Go's `os.WriteFile` permission bits are effectively ignored there. `golang.org/x/sys` is already a direct dependency.
- Security note, stated plainly rather than buried: this writes the operator's token to disk in plaintext in one additional location. `gh` already stores it, so this is not a new class of exposure, but it is one more place to leak from and the reviewer should treat the ACL handling as load-bearing rather than incidental.
- Rejected: no cache, shelling out every time — 310 ms per invocation in fallback mode, which is the cost that makes the CLI look competitive. Rejected: environment variable only, with no `gh` fallback — zero cost and zero disk, but `lyx selfreport create` would stop working until the operator exports a token, which is a regression against today's zero-setup behaviour.

### never-block-on-credentials

- Decision: no credential path may block, prompt, or hang. `gh auth login` is never invoked. The `gh auth token` shell-out runs under a context timeout. A missing or unusable token surfaces as a typed error through the `output.Err` envelope with a non-zero exit code.
- Rationale: an operator requirement, and the single most important property of this design — lyx runs autonomously, and a process waiting forever on a credential prompt is indistinguishable from a hang. This is also the reason the `git credential fill` bridge was rejected under `remote-operations-stay-cli`: GCM can raise a GUI prompt that no environment variable reliably suppresses.
- Rejected: treating a missing token as a soft failure that degrades silently — a self-report that quietly does not file is worse than one that fails loudly.

### httptest-seam-replaces-rungh

- Decision: `selfreportengine`'s testing seam becomes an injectable base URL plus `*http.Client` on the go-github client, exercised against an `httptest` server. The exported `RunGH` variable is deleted.
- Rationale: the real go-github client runs in the tests, so the request path, method, JSON body, and title/body/label encoding are actually covered. Under the old `RunGH` fake, tests asserted on an argv slice — a shape that ceases to exist. The risk in a REST client is exactly the request construction, so that is what the tests must exercise.
- Rejected: a like-for-like `var CreateIssueFn = realCreateIssue` func-var seam — cheapest diff, but the fake would sit above the entire go-github call, so tests would stop covering the request shape completely. Rejected: an interface over go-github's `IssuesService` — typed, but it leaks go-github types into `selfreportcli`'s tests for no gain over base-URL injection.

### go-github-version-and-auth-construction

- Decision: pin `github.com/google/go-github` to the latest stable major, **v75.0.0**, resolved and confirmed available. Build the authenticated client via `github.NewClient(nil).WithAuthToken(token)`.
- Rationale: `WithAuthToken` avoids a `golang.org/x/oauth2` dependency entirely, which keeps `githubclient`'s import allowlist short enough to enforce as a leaf. `go-git/go-git/v5 v5.19.1` is already a direct dependency from the spike and stays pinned at that exact version — every verdict in `internal/gitnativepoc/doc.go` and in the probe report was produced against it.
- Rejected: an oauth2 token source — more dependency for identical behaviour.

### constraints-github-auth-invariant

- Decision: add a **GitHub Auth Invariant** to `CONSTRAINTS.md` — all GitHub authentication goes through `internal/githubclient`, and no other production package shells out to `gh`. Machine-checked by a grep guard on the `gh` literal in non-test files outside the package, in the same form as the existing `Dev/Prod Binary Separation` guard. `githubclient`'s own production imports are allowlisted to stdlib, `go-github`, and `golang.org/x/sys`, enforced by a `leaf_enforcement_test.go` in the same shape as `modelspec`, `tokenvocab`, and `codeintelengine` already use.
- Rationale: the invariant is what protects the never-block-on-credentials property structurally. Without it there is nothing stopping finalize from shelling out to `gh` again in six weeks, at which point there are two credential paths and only one of them has a timeout.
- Rejected: a review-obligation-only entry — this repo's own history with grep guards argues for the machine check where one is cheap, and this one is cheap.

### constraints-gitrepo-boundary-invariant

- Decision: add a **gitrepo Client Boundary Invariant** to `CONSTRAINTS.md` stating the local-vs-remote split, naming the CLI-bound set explicitly, and requiring that any new `gitexec` call inside `gitrepo` come with an updated entry justifying it.
- Rationale: the ADOPT-PARTIAL boundary is invisible in the code — both backends are just method bodies. Without a written rule, CLI calls seep back in one bugfix at a time and the next reader cannot tell which methods are deliberately CLI-bound from which are simply unmigrated.
- Rejected: relying on `gitrepo`'s package doc alone. The doc explains; the invariant is what a reviewer is obliged to check.

### win11-gate-c-closure

- Decision: this task closes gate (c) — "works on Windows 11" — for every migrated method, and the outcome is recorded in `gitrepo`'s godoc, replacing the spike's `Win11-pending` markers.
- Rationale: every MIGRATE verdict in `internal/gitnativepoc/doc.go` is explicitly provisional on a Windows run that had never happened. The probe closed it for the five previously-unexamined methods and, more importantly, **failed** it for the two commit methods via the autocrlf finding. The migrated read surface must be verified the same way rather than inheriting a Linux-only verdict: the Tier-2 run on this Windows machine is what does it.
- Rejected: carrying the `Win11-pending` markers forward — they were a promise to check, and the check is now cheap.

### docs-lifecycle

- Decision: in the landing commit, delete `manifest/designs/native-clients-migration.md`; fold the durable findings into `internal/gitrepo`'s, `internal/githubclient`'s, and `internal/selfreportengine`'s package docs; move `manifest/roadmap.md` Planned #3 to Done; add the two `CONSTRAINTS.md` invariants. `docs/overview.md` is updated only if its module table or execution stack actually changes — `githubclient` is a library package, not a registered module, so most likely it does not.
- Rationale: required by CLAUDE.md's task-completion rule and by the Documentation Lifecycle entry in `CONSTRAINTS.md`. The design doc says so itself in its own header.
- Decision, content: `gitrepo`'s package doc must state the final boundary **and the evidence for it** — autocrlf blob divergence, go-git's untracked-file deletion on hard reset, `Checkout`/`Pull` non-atomicity, and the absent credential-helper support. A future reader who only sees "commits are CLI-bound" will try to migrate them again.
- Rejected: leaving the design doc in place as a record — the lifecycle rule is explicit, and a stale design doc describing a scope that was measured wrong is worse than none.

## Technical context

**`internal/gitrepo` layout.** `gitrepo.go` holds `Repo`, `New`, the `run` helper, `CurrentSHA`, `StageAndCommit`, `StageAllAndCommit`, `SHAExists`, `CurrentBranch`, `CheckoutDetached`, `RestoreBranch`, `ChangedFilesSince`, plus the `validSHA`/`shaPattern` guards and the `ErrNoCommits`/`ErrInvalidSHA` sentinels. `snapshot.go` holds `SnapshotSHA`, `SetSnapshotSHA`, `remoteName`, `advanceAndPushSnapshotRef`, `adoptSnapshotRef`, `isStrictDescendant`, `validSnapshotKey`, and `ErrInvalidSnapshotKey`. `push.go` holds `Push`, `pushWithRebaseRetry`, `PushCoalesced`, `hasUnpushed`, `containsAny`, `rebaseRetryTriggers`, and the exported `PushLockFileName`. `pull.go` and `reset.go` hold one method each.

**The go-git implementations already exist.** `internal/gitnativepoc/read.go` and `write.go` carry working, spike-verified go-git versions of every method in scope, with the tricky details already solved. Lift from there rather than reimplementing. The details that matter: `ChangedFilesSince` must call `object.DiffTree` directly — `Tree.Diff`/`DiffTreeWithOptions` perform rename detection by default since go-git v5.1.0, which folds a rename into one entry and loses the old path, exactly what the CLI's `--no-renames` exists to prevent. `hasUnpushed` cannot use `@{u}`: go-git's revision parser recognizes the syntax but `ResolveRevision` never implements the `AtUpstream` case, so the upstream ref is resolved from branch config and compared by reachability — and the upstream's **full ancestor set** must be walked first and passed as `NewCommitPreorderIter`'s `seenExternal` map, because seeding only the upstream tip would wrongly report HEAD as ahead whenever HEAD is strictly behind. `CurrentSHA` maps `plumbing.ErrReferenceNotFound` to `ErrNoCommits`.

**Error-contract mapping is the delicate part of this task.** Each migrated method must keep its documented error shape exactly. `CurrentSHA` returns `ErrNoCommits` for an unborn HEAD and a plain wrapped error otherwise. `SHAExists` swallows everything into `false`, including a non-hex SHA, without touching git. `ChangedFilesSince` returns `ErrInvalidSHA` before resolving anything. `SnapshotSHA` distinguishes three cases that are easy to collapse: a set ref returns its SHA, a verified-absent ref returns `("", nil)`, and an unreadable store returns an error — folding the third into the second would tell a miswired consumer "no snapshot" forever. `Pull` and `ResetHard` deliberately do **not** include git's stderr in their errors; they stay on the CLI so this is preserved by not touching them.

**The validation guards stay CLI-independent.** `validSHA` and `validSnapshotKey` run before any backend is reached and must keep doing so — they exist to stop an option-shaped string from being parsed as a flag (`update-ref <ref> -d` deletes the ref). Under go-git the flag-injection risk disappears, but the guards are also the source of the typed `ErrInvalidSHA`/`ErrInvalidSnapshotKey` sentinels that callers check with `errors.Is`, so they are contract, not just defence. `keyvalidation_test.go` is the untagged Tier-1 test covering them and should keep passing untouched.

**`internal/selfreportengine` is one file.** `selfreport.go` holds `targetRepo` (const `"Knatte18/loomyard"`), the `RunGH` seam, `realRunGH`, `buildCreateArgs`, `CreateIssue`, and `lastNonEmptyLine`. `CreateIssue`'s three documented error cases must survive the transport swap in spirit: binary-not-found becomes token-not-resolvable, generic exec failure becomes transport/network failure, and non-zero `gh` exit becomes a non-2xx GitHub response. Its zero-issue-number convention is load-bearing — `cli.go` omits the `number` field from the JSON envelope when it is 0. With go-github the number arrives typed in the response, so the parse-the-URL-tail heuristic and `lastNonEmptyLine` both disappear; the zero convention should be kept only if a real code path can still produce it.

**Consumers, for confidence that nothing leaks.** `gitrepo` is imported by `boardengine/sync.go`, `fabricengine/{fabric,weftgit}.go`, and `websterengine/{gitwrap,integration,runlevel}.go`. `selfreportengine` is imported only by `selfreportcli/cli.go` and its test. `RunGH` has no production consumer at all — only `selfreportcli/cli_test.go` — so deleting it touches no shipping code path.

**Probe artifacts.** `.scratch/gogit-probe-report.md` has the full verdict table, per-operation evidence, and a "what I could not determine" section that names three open unknowns: the mechanism behind the flaky dirty-detection (correlation proven, mechanism not instrumented), whether the failed-checkout partial state is fully compensable (the `MM` status suggests the index moved too), and behaviour against real lyx topology — junctions and `.git`-as-a-file worktrees, which were not exercised. None of the three blocks this task, because every operation they concern stays on the CLI, but the third is worth knowing before anyone proposes widening the boundary later. The probe module lives at `.scratch/gogitprobe/` as a **nested Go module** with no `_test.go` files, deliberately: the repo's guard tests walk every `*_test.go` under the module root by filesystem walk, so a probe test file would trip `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go`. It is disposable; nothing in the task depends on keeping it.

## Constraints

From `CONSTRAINTS.md`, the invariants this task touches:

- **Hub Geometry Invariant** — `_lyx`, its `config/` subdir, and geometry tokens resolve only through `internal/hubgeometry`, in test code too. The token cache path (`%LOCALAPPDATA%\lyx\`, `$XDG_CONFIG_HOME/lyx/`) is **user config, not repo geometry** — it contains no `_lyx` token and constructs no worktree path — so it does not route through `hubgeometry`. Call this out in review rather than leaving a reviewer to wonder.
- **CLI / Cobra Invariant** — not triggered: no module is added or changed. `selfreportcli`'s command surface, `Short`, and `Long` are unchanged because `CreateIssue`'s observable behaviour is unchanged. If any error *message* the CLI prints changes as a result of the transport swap, the help-accuracy review obligation applies.
- **Test Tier Purity Invariant** — untagged test files must not contain `gitexec.RunGit`, `exec.Command`, or `lyxtest.Copy`, checked as a raw substring. A go-git-backed test spawns no process, so it *could* be untagged — but it still needs a real git fixture to be meaningful, and building that fixture spawns git. Keep the migrated tests integration-tagged.
- **Hermetic Git Test Environment Invariant** — every git-spawning test package needs a `TestMain` calling `lyxtest.HermeticGitEnv()`. `gitrepo/testmain_test.go` already has one and must stay. Critical caveat established by the probe: **`HermeticGitEnv` does not constrain go-git at all**, since go-git ignores `GIT_CONFIG_GLOBAL` and `GIT_CONFIG_NOSYSTEM`. Any go-git-backed test that depends on config state must set it in the fixture's **local** repo config, which go-git does read.
- **Sandbox Suite Coverage** — `selfreport` is already on the `excludedModules` allowlist with the reason "`create` files a real GitHub issue". That reason survives the transport swap unchanged; no allowlist edit is needed.
- **Documentation Lifecycle** — governs the design-doc deletion and godoc folding described under `docs-lifecycle`.

New invariants this task adds: **GitHub Auth Invariant** and **gitrepo Client Boundary Invariant**, both specified under Decisions above, both landing in `CONSTRAINTS.md` in the same commit as the code.

## Testing

**`internal/gitrepo` — differential parity (the TDD candidate).** Lift `internal/gitnativepoc`'s harness and make it permanent. Every migrated method is asserted against the real git CLI as the oracle on the same fixture, not against a hand-written expectation — a go-git bug that is *consistently* wrong passes a hand-written test and fails a parity test. Stays Tier 2 (`//go:build integration`) under the existing `TestMain`. The cases to carry over, because each one caught something real: non-ASCII paths in `ChangedFilesSince` (verbatim bytes, no C-quoting layer to strip); a rename reported as delete-plus-add rather than folded; a non-hex SHA returning `ErrInvalidSHA` with no backend call; `CurrentSHA` on both a committed repo and an unborn HEAD; `SHAExists` across a real SHA, a well-formed-but-absent SHA, and a non-hex string; `SnapshotSHA` across a set ref, a never-set ref, and an invalid key; `remoteName`'s `origin` fallback and its configured-`branch.<n>.remote` path; `hasUnpushed` including the strictly-behind case that the naive single-hash exclusion gets wrong; `isStrictDescendant` across ancestor, self, and an unrelated orphan-branch commit.

**`internal/gitrepo` — the boundary itself.** The CLI-bound methods keep their existing integration tests unchanged; if any of them needs editing, the boundary was drawn wrong. Worth one explicit test that `SnapshotSHA` still reads a ref written by the CLI-side `SetSnapshotSHA`, since that method is now split across both backends and is the one place the two must interoperate on the same ref.

**`internal/githubclient` — token resolution.** Table-driven and hermetic: `GH_TOKEN` set wins; `GITHUB_TOKEN` used when the first is empty; both empty falls through to the shell-out; the cache is read when fresh and ignored when past TTL; an environment variable always beats a fresh cache; a `401` invalidates and re-resolves exactly once, never in a loop. The `gh` shell-out must be behind a seam so these run without `gh` installed. Two cases deserve explicit tests because they encode the operator requirement rather than mere behaviour: **the shell-out honours its timeout**, and **an unresolvable token returns a typed error rather than blocking** — a test that would hang on regression is the point.

**`internal/githubclient` — cache file handling.** Cover creation with restrictive permissions, a corrupt/unparseable cache file being discarded rather than fatal, and a cache directory that cannot be created degrading to in-process resolution rather than failing the command. On Windows, assert the ACL handling actually took effect — mode bits alone prove nothing there.

**`internal/selfreportcli` — request-shape coverage via `httptest`.** Rewrite the `RunGH`-fake tests against a test server: assert the request method and path (`POST /repos/Knatte18/loomyard/issues`), that the JSON body carries the title, that the body field is present only when supplied and absent otherwise, and that multiple labels survive in order. Then map responses to behaviour — a 201 returns URL and number from the typed response; a 4xx/5xx surfaces as an error through the envelope; a network failure surfaces distinctly from an API rejection. Keep the existing cobra-level tests that assert arg-count rejection reaches no transport at all.

**Whole-repo verification.** `go build ./...`, `go vet ./...`, the untagged suite, then `go test -tags integration ./... -count=1` — the full Tier-2 suite in isolation, per the brief's SOLO instruction, since this swaps the git foundation `boardengine`, `fabricengine`, and `websterengine` all sit on. The Tier-2 run on this Windows machine is what closes gate (c) per `win11-gate-c-closure`; record the result in `gitrepo`'s godoc rather than leaving `Win11-pending` markers behind. Run the constraint guards explicitly as part of this — `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, and the new leaf-enforcement and grep guards.

**Falsification.** For each parity test covering a migrated method, confirm the test fails when the go-git implementation is reverted to a plausible-but-wrong form — the rename-folding variant and the single-hash `hasUnpushed` exclusion are the two known-wrong shapes worth proving against, since both were real spike findings rather than invented ones.

## Q&A log

- **Q:** How much of `gitrepo` moves to go-git — the brief's list, or everything? **A:** As much as possible, but bounded by evidence: anything not adequately tested by the spike gets probed empirically first rather than assumed.
- **Q:** Didn't the spike already answer this? **A:** For the eleven operations it examined, yes, and those verdicts stand. Five methods (`CurrentBranch`, `CheckoutDetached`, `RestoreBranch`, `Pull`, `ResetHard`) were never examined, and the spike ran on Linux only, so its Windows gate was open for everything.
- **Q:** Where does the go-git handle get opened, given `New` cannot fail? **A:** Lazily, cached on `Repo` via `sync.Once`; `New`'s contract is untouched.
- **Q:** What happens to `internal/gitnativepoc`? **A:** Delete it — but the parity harness is worth reusing, so lift that first.
- **Q:** Should `gitexec` stay as `gitrepo`'s CLI choke point? **A:** Yes, `run` unchanged.
- **Q:** Extend `selfreportengine` with the wider GitHub surface? **A:** No, explicitly not. A new package.
- **Q:** Can't all the git-related things live in one package? **A:** No — `gitrepo` is local filesystem access with no network and no credentials; a GitHub client is authenticated REST with tokens, rate limits, and HTTP status codes. Merging them would make `boardengine`, `fabricengine`, and `websterengine` all inherit go-github and a credential path they never use.
- **Q:** Do we even need a `githubclient`, or can consumers just use go-github directly? **A:** They do use it directly for every operation. The package exists only for authentication, which is the one part that must not be duplicated.
- **Q:** How much of the GitHub surface gets built now? **A:** The client is thin enough that the question dissolves — it supports the whole millhouse-derived set by supporting authenticated calls at all.
- **Q:** Why must remote operations stay on the CLI — is it purely the credentials issue? **A:** Purely that. go-git's protocol implementation is complete and the spike ran push and fetch green against a local bare repo; it simply never consults a credential helper, and this machine's remote is HTTPS behind Git Credential Manager.
- **Q:** Doesn't go-git support credentials at all? **A:** It supports *using* them (`http.BasicAuth`, `http.TokenAuth`, `ssh.PublicKeys`, SSH agent) but not *discovering* them. Supplying them means either GitHub-specific coupling or a `git credential fill` shell-out with GUI-prompt risk.
- **Q:** Credential verification must happen once, not on every use — even if that means caching to a git-untracked file. **A:** Accepted; measured at ~310 ms per `gh auth token` call, cached with a 12-hour TTL. Environment variables bypass it entirely at zero cost.
- **Q:** Won't a machine-global cache be shared across every repo that uses lyx? **A:** Yes, and that is correct — the token is per-user, not per-repo. Per-repo caching would mean N copies of one secret.
- **Q:** Most future lyx use will be on Linux, where this is faster. **A:** Noted. It does not change the commit-method decision: CLI commits are correct on both platforms, and a platform-conditional commit path would be correct on one OS and silently wrong on the other.
- **Q:** But if the GitHub token is cached anyway, couldn't `SetSnapshotSHA` and `SnapshotSHA`'s fetch use go-git with it? **A:** Mechanically yes — the cached token works as `http.TokenAuth` against github.com over HTTPS. Rejected anyway: it would make `gitrepo` fail against SSH remotes and non-GitHub forges, and would require new public surface on `gitrepo` to accept an auth method, to migrate two operations that parse almost no text. `Push`/`PushCoalesced` would stay CLI regardless, since go-git has no rebase at all.
- **Q:** What about the commit methods, given the autocrlf finding? **A:** Left to my judgment; decided to keep them on the CLI — they have almost no text parsing to gain from typing, and the divergence they would introduce is silent and permanent.
