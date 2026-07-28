# Batch: docs-and-invariants

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: docs-and-invariants
number: 9
cards: 7
verify: go test -count=1 ./cmd/lyx/... ./internal/gitrepo/... ./internal/githubclient/...
depends-on: [8]
```

## Batch Scope

Lands the documentation half of the task: the two new `CONSTRAINTS.md` invariants whose enforcement shipped in batch 8, the package docs that become the durable home for the migration's evidence, and every doc site this task **falsifies**.

The godoc is not decoration here. Both probe reports live under `.scratch/`, which is gitignored and disappears with this worktree, so the package doc is the only durable record the evidence gets. That is why the list in card 41 is exhaustive rather than illustrative: a future reader who sees only "commits are CLI-bound" will try to migrate them again, and one who sees only "use go-git" will write `PlainOpen` and ship a silently broken handle.

Four doc sites are actively **false** after this task rather than merely incomplete, and all four repeat the same claim: `docs/overview.md`'s module table, `internal/gitrepo/doc.go`'s opening, and two separate `manifest/roadmap.md` Done entries all describe `gitrepo` as built on `internal/gitexec`. A Done entry is no more exempt than a Planned one.

## Cards

### Card 40: Add the two CONSTRAINTS.md invariants

- **Context:**
  - `cmd/lyx/ghguard_test.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `internal/githubclient/leaf_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two entries in the file's existing shape — statement, bullets, and a closing **Enforced by** line naming the test. The **GitHub Auth Invariant**: all GitHub authentication goes through `internal/githubclient`, and no other production package shells out to `gh`; `githubclient`'s production imports are allowlisted to stdlib, `go-github`, `golang.org/x/sys`, and `internal/proc`, with the reason `internal/proc` is on that list stated explicitly (the `gh auth token` fallback needs `proc.HideWindow`, and `internal/proc` is itself stdlib-only, so allowlisting it does not weaken the leaf property). Enforced by `cmd/lyx/ghguard_test.go` and `internal/githubclient/leaf_enforcement_test.go`. The **gitrepo Client Boundary Invariant**: state the local-vs-remote split — go-git for local object and ref access, `gitexec` for anything that authenticates to a remote or mutates the working tree — name the CLI-bound set explicitly, and require that any new `gitexec` call inside `gitrepo` come with an updated entry justifying it. Record the guard's known blind spot in the entry itself rather than only in the test: set-equality on method names cannot see a new `r.run` call added inside a method already on the pinned list, so the per-call review obligation stands for those methods. Enforced by `cmd/lyx/gitrepoboundary_test.go`. Note in the entry that widening the CLI-bound set deliberately requires editing the pinned list in the same commit as the invariant — that is the review moment the rule exists to force.
- **Commit:** `docs(constraints): add GitHub Auth and gitrepo Client Boundary invariants`

### Card 41: Rewrite internal/gitrepo's package doc

- **Context:**
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/snapshot.go`
  - `.scratch/gogit-probe-report.md`
  - `.scratch/gogit-worktree-probe-report.md`
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three sections change. **(1) The gitexec-relationship section** currently describes the package as "built on top of internal/gitexec's raw command runner" and states that gitrepo "never calls exec.Command itself; every method goes through a single unexported run helper". After this task the read surface bypasses `run` entirely, so both claims are false as written. Rewrite it to describe the two-backend shape — go-git for local reads, `gitexec` for worktree-mutating and remote operations — and keep the closing argument for why `gitexec` stays a separate zero-dependency leaf with roughly eighty call sites elsewhere, which remains true. **(2) The locale paragraph** states that outcome classification depends on git's untranslated English message text, naming three sites: `CurrentSHA`'s unborn-HEAD detection, the push-rejection triggers behind rebase-retry and snapshot adopt-on-conflict, and the benign "no rebase in progress" abort check. Rewrite rather than delete — the first is now typed (`plumbing.ErrReferenceNotFound` → `ErrNoCommits`) while the other two stay CLI-bound and therefore stay locale-dependent, so two-thirds of it remains true. **(3) Add the evidence section**, exhaustively, because both probe reports are gitignored and vanish with this worktree. From probe 1: the autocrlf blob divergence that keeps both commit methods on the CLI, go-git's deletion of untracked and gitignored files on hard reset, `Checkout`/`Pull` non-atomicity with HEAD moved before the dirty check, and the absent credential-helper support. From probe 2: `PlainOpenWithOptions` with `EnableDotGitCommonDir: true` is mandatory and `PlainOpen` fails **silently** on a linked worktree; `DetectDotGit` is banned because it retargets a parent repository; `KeepDescriptors` must stay `false` or packfiles lock against `worktree remove`; the fingerprint-gated `Reindex()` remedy for packfile-only objects and why the policy is reactive; and the `extensions.worktreeConfig` incompatibility that makes go-git refuse to open a repo outright. Also record that gate (c) — "works on Windows 11" — is now **closed** for every migrated method by this task's Tier-2 run, replacing the spike's `Win11-pending` markers rather than carrying them forward.
- **Commit:** `docs(gitrepo): rewrite package doc for the two-backend boundary`

### Card 42: Package docs for githubclient and selfreportengine

- **Context:**
  - `internal/githubclient/githubclient.go`
  - `internal/githubclient/token.go`
  - `internal/githubclient/cache.go`
  - `internal/githubclient/transport.go`
- **Edits:**
  - `internal/selfreportengine/selfreport.go`
- **Creates:**
  - `internal/githubclient/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `githubclient`'s package doc covering what the package deliberately is **not**: it owns authentication only and exposes no per-operation wrappers, because consumers call go-github's typed API directly and `owner`/`repo` are their parameters to supply. Record the resolution order and the 12-hour cache TTL; the cache location and why it is machine-global and outside every repository; why `WithAuthToken` is banned (two owners of the `Authorization` header would make the post-401 replay re-send the stale token); why the 401 replay is skipped for environment-variable tokens; the `req.GetBody` rewind requirement; and the two deadlines, 5 s on the `gh` shell-out and 30 s on the HTTP client, with the note that `Client.Timeout` covers the original attempt and the replay **together**. State the never-block-on-credentials property as the package's reason for existing, and state plainly that the cache writes the operator's token to disk in plaintext in one additional location. Also note the GitHub surface consumers need — the millhouse-derived set of issue list/view/create/close, PR list/view/create, repo view, and an auth probe — with the two consumption details worth not rediscovering the hard way: `gh issue view` exits 0 on a **closed** issue so the `state` field must be read, and PR list results carry a state precedence of MERGED > OPEN > CLOSED. Separately, update `selfreportengine`'s package godoc to describe the go-github transport instead of the `gh` CLI.
- **Commit:** `docs(githubclient): document the auth-only client and its credential policy`

### Card 43: Update docs/overview.md

- **Context:**
  - `internal/gitrepo/doc.go`
  - `internal/githubclient/doc.go`
  - `internal/selfreportengine/selfreport.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three edits. The `internal/gitrepo/` tree entry reads "typed Repo over one local git checkout, built on gitexec" — the same claim already false in the package doc, corrected the same way to the two-backend phrasing. The `internal/gitnativepoc/` tree entry is now **dangling** and must be removed, since batch 5 deleted the package; `overview.md`'s tree lists library packages, not only registered modules, so a deleted library leaves a hole here. Add an `internal/githubclient/` entry, which is missing entirely. In the module section, the **selfreport** bullet still describes filing issues "via the `gh` CLI" and must say go-github through `githubclient`, with `gh` demoted to a fallback token source.
- **Commit:** `docs(overview): correct gitrepo, drop gitnativepoc, add githubclient`

### Card 44: Update README.md

- **Context:**
  - `internal/githubclient/doc.go`
  - `internal/selfreportcli/cli.go`
- **Edits:**
  - `README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two edits. The **selfreport** module line says issues are filed "via `gh`" and must match the new transport. The prerequisites list names "`gh` CLI authenticated (`gh auth login`)" as a hard requirement — this task **demotes it to a fallback**, since `GH_TOKEN` or `GITHUB_TOKEN` removes the need for the binary entirely. Rewrite it to say exactly that rather than deleting the line, since `gh` remains a supported and zero-setup path.
- **Commit:** `docs(readme): demote the gh CLI from prerequisite to fallback`

### Card 45: Rewrite the roadmap entries and delete the design doc

- **Context:**
  - `internal/gitrepo/doc.go`
  - `docs/overview.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/native-clients-migration.md`
- **Moves:** none
- **Requirements:** Four roadmap edits and one deletion, per the Documentation Lifecycle. **(1)** The Planned entry for this task states the superseded scope verbatim — "read surface, both commit methods, and `SetSnapshotSHA` migrate cleanly" — and links the design doc being deleted. It is **rewritten** on the way into Done, never moved as-is: moving it verbatim would ship a Done entry describing work that deliberately did not happen. State what actually landed, including that the commit methods and `SetSnapshotSHA`'s writes stayed on the CLI on measured Windows evidence. **(2)** The `git-native-library` spike's Done entry points readers at `internal/gitnativepoc/doc.go`, now deleted, and repeats the same overturned commit-method recommendation. Drop the dangling pointer and add that the Windows evidence narrowed the verdict, so the historical record stays accurate rather than becoming a trap. **(3)** The `gitrepo` module's own Done entry describes its primitives as built on `internal/gitexec` — verbatim the claim this task falsifies elsewhere — and takes the same two-backend phrasing. **(4)** Delete `manifest/designs/native-clients-migration.md`; the lifecycle rule is explicit, and a stale design doc describing a scope that was measured wrong is worse than none. Its durable findings are already folded into the package docs by cards 41 and 42. Note that `manifest/roadmap.md` moves only for completing or adding a planned item, which this is — the entry genuinely completes.
- **Commit:** `docs(roadmap): record what landed and retire the design doc`

### Card 46: Whole-repo verification

- **Context:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification-only card, no diff. Run, in order: `go build ./...`, `go vet ./...`, the untagged suite, then `go test -tags integration ./... -count=1` — the full Tier-2 suite in isolation, per the task's SOLO instruction, because this swaps the git foundation that `boardengine`, `fabricengine`, and `websterengine` all sit on. Run the constraint guards explicitly as part of this rather than trusting them to be swept up: `cmd/lyx/tierpurity_test.go`, `cmd/lyx/hermeticenv_test.go`, `cmd/lyx/sandbox_coverage_test.go`, both new guards from batch 8, and `internal/githubclient`'s leaf-enforcement test. The Tier-2 run on this Windows machine is what closes gate (c) per card 41 — if it has not been run on Windows, that section of the package doc is a claim rather than a record. Report the actual result; a suite that was not run is not a suite that passed.
- **Commit:** none

## Batch Tests

`verify:` runs `go test -count=1 ./cmd/lyx/... ./internal/gitrepo/... ./internal/githubclient/...`. The scope is deliberately wider than "the files this batch edits" because this is a docs batch whose edits are load-bearing for two guards: card 41 rewrites `internal/gitrepo/doc.go`, which the boundary guard's comment-stripping assertion scans, and card 40 edits `CONSTRAINTS.md`, whose entries the guards are named in. A doc edit that breaks a guard is exactly the failure mode a narrower scope would miss.

Card 46 is the batch's real gate and runs the whole-repo verification the task requires, including the full integration suite that no per-batch `verify:` covers. It produces no diff.

Consider setting `pipeline.done_gate` in `mill-config.yaml` to `go test ./... -count=1` for this repo: every batch here is package-scoped, so nothing in the per-batch gates catches a regression in a package outside the batch's own scope until card 46 runs at the very end.
