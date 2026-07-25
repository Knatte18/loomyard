# Discussion: git-native-library: feasibility spike

```yaml
task: 'git-native-library: feasibility spike'
slug: git-native-library
status: discussing
parent: main
```

## Problem

`internal/gitrepo` talks to git exclusively through `internal/gitexec.RunGit`, which
shells out to the real `git` binary and hands back raw stdout/stderr/exit-code. Every
`gitrepo` method then has to interpret git's **human-readable** output to decide what
happened: substring-matching stderr for rejection shapes (`"non-fast-forward"`,
`"rejected"`, `"fetch first"`), matching exact English phrases for special states
(`"ambiguous argument 'HEAD'"`, `"no rebase in progress"`), and flag-tuning porcelain
output (`diff --name-only -z --no-renames`) so non-ASCII paths and renames survive. The
2026-07 crucible hardening of `gitrepo` surfaced concrete bugs traceable to exactly this
"parse git's stderr as a de-facto API" pattern: SHA-shaped arguments that could be read as
git options, non-ASCII filenames coming back C-quoted, and a documented (unprovable)
assumption that git's messages are never translated.

**Why now:** the hardening run just finished and named this bug class as structural, not a
`gitrepo`-specific mistake. Before anyone commits to a large migration, we need to answer
one question with evidence: **would a native Go git library (go-git) actually hold up for
the narrow surface `gitrepo` uses?** This task is a *feasibility spike* that produces that
answer plus a reproducible prototype — not a migration.

## Scope

**In:**

- A new throwaway-but-kept prototype package **`internal/gitnativepoc/`**: a thin
  reimplementation of `gitrepo`'s git surface over **go-git**, plus an integration-tagged,
  hermetic **parity harness** that runs both the real `gitexec`/CLI path and the go-git path
  against identical fixture repos and asserts they agree.
- Empirical classification of **every** operation `gitrepo` uses as **MIGRATE** (moves
  cleanly to go-git) or **CLI-BOUND** (must stay on the git CLI because go-git genuinely
  cannot do it). This covers the **read** surface (the migration target) **and** the
  **write** surface including the **rebase-retry** path (probed because the design mandates
  verifying it — rebase is go-git's known weak spot).
- Adding **go-git** as a real `go.mod` dependency on `main` (a direct consequence of keeping
  the prototype).
- A **write-up** that replaces `manifest/designs/git-native-library.md` with the spike's
  recommendation — **ADOPT** / **ADOPT-PARTIAL** / **DECLINE** — the per-operation
  MIGRATE/CLI-BOUND table, and the evidence behind each verdict.

**Out:**

- **No change to `internal/gitexec` or its ~80 call-sites.** Untouched.
- **No change to `internal/gitrepo`'s public API or its internals.** The prototype is a
  *separate* package; it does not modify, wrap, or replace `gitrepo`.
- **No migration.** A positive result is a *proposal* to migrate, evaluated later as its own
  decision. This task does not move any real consumer onto go-git.
- **No `git2go`/libgit2 adoption.** go-git is the primary library; `git2go` (cgo) is noted
  only as a fallback-to-investigate if go-git cannot do a given operation *at all*, and even
  then adopting it is out of this task's scope.
- **No Windows test run in this task.** The harness is written to be OS-portable and to
  support Win11 as best as engineering allows, but the actual Win11 verification run is
  deferred to a later pass on a Win11 machine (see the Windows decision).

## Decisions

### method-empirical-poc

- **Decision:** Build a throwaway prototype (`internal/gitnativepoc/`) that reimplements the
  surface over go-git and proves parity empirically, rather than reasoning on paper.
- **Rationale:** The whole question is behavioural ("does go-git match git on the cases we
  actually depend on"). Reading go-git's docs/issues is weaker than running both paths
  against the same repo and diffing the results.
- **Rejected:** Paper analysis only (weak evidence on the exact tricky cases); hybrid
  paper-then-partial-PoC (no reason to skip building the read surface — it's the cheap part).

### keep-the-poc-code

- **Decision:** The prototype **is kept and merges to `main`** (it is not deleted after the
  verdict). The findings write-up replaces `manifest/designs/git-native-library.md`.
- **Rationale:** User wants the prototype retained as reproducible, runnable evidence and a
  head-start for any future migration. Keeping it makes the parity harness re-runnable
  (including the later Win11 run) via `go test`.
- **Rejected:** Delete the PoC once it proves its point (the `muxpoc` precedent in
  CONSTRAINTS.md's CLI/Cobra Invariant) — rejected because the user explicitly wants the code
  kept. Consequence accepted: go-git lands in `go.mod` on `main`, and a non-production
  experimental package lives in the tree.

### go-git-primary

- **Decision:** **go-git** (`github.com/go-git/go-git`, latest stable that `go get`
  resolves — the maintained fork; the old `src-d/go-git` is deprecated) is the primary and
  default library. `git2go`/libgit2 is investigated **only** if go-git genuinely cannot do a
  specific operation, and is not adopted by this task regardless.
- **Rationale:** go-git is pure Go — no cgo — which preserves the single-static-binary build
  and easy cross-compilation (especially to Windows) that are the main practical draw. cgo
  bindings undercut exactly that. Library choice is confirmed empirically by the harness, not
  by reputation.
- **Rejected:** go-git-only with cgo entirely off the table (too rigid — if go-git *cannot*
  do something, we should at least record whether libgit2 could); head-to-head go-git vs
  git2go (more work than a spike warrants).

### migrate-vs-cli-bound-rubric

- **Decision:** Each operation is classified **MIGRATE** or **CLI-BOUND** by this rubric.
  **go-git is used wherever feasible; an operation is CLI-BOUND only when go-git truly has no
  viable alternative.**
  - Hard gates (fail any → CLI-BOUND):
    - **(a) Typed result/error** — go-git returns a typed Go value/error, not text to parse.
      (This is the entire point; it kills the parse-stderr bug class.)
    - **(b) Behavioural parity with git** on the cases we depend on (see Technical context
      for the exact list).
    - **(c) Works on Windows (Win11)** — no Linux-only assumption. Windows is a hard gate,
      not a documented footnote (see the Windows decision for how it is verified).
  - Reported but not automatic disqualifiers (force CLI-BOUND only if *genuinely unworkable*,
    never for merely "somewhat worse"):
    - **(d) Performance** on a realistically-sized repo.
    - **(e) Hooks / credential-helper / gitattributes** divergence that would break a real
      loomyard git flow.
- **Rationale:** Matches the user's stance ("go-git for everything where feasible; CLI only
  when go-git really has no alternative"). (a)/(b)/(c) are correctness/portability
  non-negotiables; (d)/(e) are informative and only decisive at the extreme.
- **Rejected:** Dropping (d) or (e) from what the harness measures (they're cheap to report
  and inform the migration proposal even when not decisive); making (d)/(e) hard gates
  (would over-reject go-git against the user's "wherever feasible" intent).

### full-surface-including-rebase

- **Decision:** The harness exercises the **full** surface — reads (MIGRATE candidates) and
  writes including the **rebase-retry** path — over go-git, and classifies each. Writes are
  built in the prototype **only to find where the CLI boundary falls**, especially for
  rebase; this does not migrate the real write path.
- **Rationale:** The design explicitly mandates verifying `gitrepo.Push`'s rebase-retry
  dependency (`pull --rebase` / `rebase --abort`) because go-git's rebase support is
  reportedly weak, and that answer decides whether a full migration is ever possible. Building
  the write experiments in the *throwaway prototype* does not violate the "don't touch the
  real writes" non-goal — the prototype is separate code.
- **Rejected:** Reads-only with rebase answered on paper (contradicts the design's
  verify-rebase mandate and leaves the pivotal question to guesswork); treating writes as
  real migration candidates now (broader than scope).

### poc-form-integration-harness

- **Decision:** `internal/gitnativepoc/` is a thin go-git-backed library **plus an
  integration-tagged, hermetic parity test harness** — **not** a registered cobra CLI
  module.
- **Rationale:** As a non-registered package it sidesteps the CLI/Cobra and Sandbox-Coverage
  invariants (which would impose help-tree, registration, and sandbox obligations on
  throwaway code). Integration-tagging + a hermetic `TestMain` keeps it inside the Test Tier
  Purity and Hermetic Git Test Environment invariants, and makes the evidence reproducible via
  `go test -tags integration`.
- **Rejected:** A registered `lyx gitnativepoc` command like `muxpoccli` (needless CLI
  ceremony for a spike); a standalone `main` under `tools/` (not exercised by `go test`, so
  the evidence isn't re-runnable through the normal test path).

### windows-portable-now-verify-later

- **Decision:** Win11 support is a **hard gate** for the final verdict, but this task does
  **not** run on Win11. The harness is written OS-portable and engineered to support Win11 as
  best as possible; it is **verified on Linux** (this dev machine) now. The Win11 run is
  **deferred to a later pass on a Win11 machine**. Any conclusion whose deciding factor is
  Windows-specific behaviour is marked **Win11-pending** in the write-up.
- **Rationale:** The dev machine is Linux and cannot produce Windows evidence. The user's
  instruction: write everything with "must also support Win11" in mind and lay the groundwork;
  the actual Win11 test happens later on Win11 hardware.
- **Rejected:** Attempting Windows verification from here (no reachable Win11 environment);
  treating Windows as fully out of scope (contradicts the hard-gate requirement — the
  groundwork and portability must be in place now).

## Technical context

### The exact surface to reimplement and classify

Enumerated from `internal/gitrepo/*.go`. Column "must match" lists the behaviour the parity
harness has to assert, because these are the tricky cases the crucible hardening cared about.

**Read surface (primary MIGRATE candidates):**

| gitrepo method | git operation | Must match (parity assertions) |
|---|---|---|
| `CurrentSHA` | `rev-parse HEAD` | Unborn HEAD → typed `ErrNoCommits`, not a generic error. |
| `SHAExists` | `rev-parse --verify --quiet <sha>^{commit}` | Peel to commit; missing/garbage SHA → false (never error). |
| `SnapshotSHA` | `rev-parse --verify --quiet <ref>` | Missing ref = **absent** (`"",nil`), not error; corrupt store = error. |
| `ChangedFilesSince` | `diff --name-only -z --no-renames <sha>..HEAD` | NUL-separated; **non-ASCII paths verbatim** (no C-quoting); **rename split** into delete+add; committed history only. |
| `hasUnpushed` | `rev-list --count @{u}..HEAD` | `@{u}` upstream revspec; **no upstream configured → treated as unpushed (true)**. |
| `remoteName` | `symbolic-ref --short HEAD`, then `config --get branch.<b>.remote` | Fallback to `"origin"` when unset. |
| `isStrictDescendant` | `merge-base --is-ancestor <a> <b>` | Exit-code semantics: ancestor-and-not-equal; any failure → false. |
| `SnapshotSHA`/adopt | `fetch <remote> +refs/loomyard/snapshot/*:refs/loomyard/snapshot/*` and `fetch <remote> +<ref>:<ref>` | Custom `refs/loomyard/snapshot/*` namespace refspec; force-fetch into local ref. |

**Write surface (probed to locate the CLI boundary; rebase is the pivotal case):**

| gitrepo method | git operation | Note |
|---|---|---|
| `StageAndCommit` | `add -- <files>`, `diff --cached --quiet -- <files>`, `commit -m <msg> -- <files>` | Pathspec-scoped; exit-code-only diff signal; "nothing to commit" = `("",false,nil)`. |
| `StageAllAndCommit` | `add -A`, `diff --cached --quiet`, `commit -m <msg>` | Wildcard sibling. |
| `Push` / `pushWithRebaseRetry` | `push` (`-c push.autoSetupRemote=true`), on rejection `pull --rebase` then retry, `rebase --abort` on failure | **The rebase-retry path is the prime CLI-BOUND suspect and the design's mandatory check.** |
| `PushCoalesced` | (uses `hasUnpushed` + `pushWithRebaseRetry` under a `lock` file) | Push serialization is `internal/lock`'s concern, not go-git's. |
| `SetSnapshotSHA` | `update-ref <ref> <sha>`, `push <remote> <ref>`, adopt-on-conflict via `fetch`, `merge-base --is-ancestor` | Fast-forward-only ref push with adopt-on-conflict; rejection detection currently keys off stderr substrings — a good typed-error test case. |

### Parity harness design (differential testing)

- **Differential, not literal:** for each fixture repo and operation, run the real
  `gitexec`/CLI-backed path **and** the go-git-backed path, then assert the typed results
  agree (same SHA, same sorted file list, same typed error class, same boolean). Divergence is
  the finding.
- **Hermetic fixtures built in-test** (integration-tagged), each targeting a "must match"
  case: an unborn-HEAD repo; a repo with a **non-ASCII filename** change; a **rename** across
  two commits; a repo with `refs/loomyard/snapshot/<key>` refs set; a two-clone setup with a
  **bare remote** to exercise `@{u}`, push rejection, and the **rebase-retry** (local and
  remote both advance the same branch, forcing a non-fast-forward). Follow existing
  `internal/gitrepo/*_test.go` fixture patterns.
- The harness reuses `gitexec.RunGit` for the CLI side and the fixture-building side, so it is
  a **git-spawning** package (Hermetic Git Test Environment Invariant applies) and an
  **expensive-spawn** package (Test Tier Purity Invariant applies) — hence integration-tagged
  with a `TestMain` calling `lyxtest.HermeticGitEnv()`.

### Reference points in the tree

- `internal/gitrepo/gitrepo.go`, `push.go`, `snapshot.go` — the surface being mirrored, with
  extensive godoc explaining each tricky case (why `-z --no-renames`, why the unborn-HEAD
  stderr shapes, why adopt-on-conflict). Read these before reimplementing.
- `internal/gitexec/gitexec.go` — the CLI choke point the go-git path is compared against.
- `internal/gitrepo/*_test.go` — existing hermetic fixture patterns to mirror.
- `manifest/designs/git-native-library.md` — the current design doc; the write-up replaces it.
- `manifest/roadmap.md` — `git-native-library` is a **Planned** item; completing the spike
  moves it to **Done** with a link to the write-up.

## Constraints

From `CONSTRAINTS.md` (hub root) and `CLAUDE.md`:

- **CLI / Cobra Invariant** — avoided by design: `internal/gitnativepoc` is **not** a
  registered cobra module (see `poc-form-integration-harness`). If that decision is ever
  reversed, the full registration/help-tree/`Short`/Sandbox-Coverage obligations attach.
- **Sandbox Suite Coverage** — only bites registered modules; the non-registered poc package
  is outside it. Do not register the package.
- **Test Tier Purity Invariant** — the harness spawns git (via `gitexec`/fixtures), so its
  test files **must be integration-tagged** (`//go:build integration`); an untagged file that
  spawns git fails `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant** — the poc's test package **must** have a
  `TestMain` calling `lyxtest.HermeticGitEnv()` before `m.Run()`, or the harness fails
  `cmd/lyx/hermeticenv_test.go`.
- **Hub Geometry Invariant** — `internal/gitnativepoc` must not construct paths with the
  reserved geometry tokens (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`,
  `_lyx`) or call `os.Getwd`/`git rev-parse --show-toplevel`; any cwd/geometry need goes
  through `internal/hubgeometry`. (The harness uses test-local temp dirs, so this should not
  arise, but it applies.)
- **Documentation Lifecycle / Task completion (`CLAUDE.md`)** — this task ships behaviour
  (new package, new dependency) and therefore **must** update docs in the same work:
  - Replace `manifest/designs/git-native-library.md` with the findings write-up.
  - Move `git-native-library` from **Planned** to **Done** in `manifest/roadmap.md`, linking
    the write-up.
  - Add a one-line note for the experimental `internal/gitnativepoc` package to
    `docs/overview.md` if the module table lists internal packages (check at plan time;
    add only if the table's convention includes it).
  - No new cross-cutting invariant is introduced by a throwaway experimental package, so
    `CONSTRAINTS.md` needs no new entry — unless plan-time review decides the "experimental,
    never wired into production" status of `gitnativepoc` warrants a recorded note.
- **Worktree isolation (`CLAUDE.md`)** — all work stays in this worktree
  (`wts/git-native-library`, branch `git-native-library`); never touch another worktree or
  push to `main` directly from here.

## Testing

- **`internal/gitnativepoc` parity harness (integration-tagged, TDD-friendly):** the harness
  *is* the deliverable's evidence, so it is written test-first around each "must match" case.
  For every operation, one test builds a fixture repo, runs both the CLI path and the go-git
  path, and asserts typed-result parity. A divergence is recorded as a finding feeding the
  MIGRATE/CLI-BOUND classification — a divergence is **not** necessarily a test failure to fix;
  for a genuinely CLI-BOUND operation the "finding" is the expected outcome and the test
  asserts *that* (go-git cannot / diverges, therefore CLI-BOUND). Make this distinction
  explicit in the harness so a real regression is still a red test.
- **Key scenarios that must be covered:**
  - Unborn HEAD → `ErrNoCommits` parity.
  - Non-ASCII filename in `ChangedFilesSince` returns verbatim (the C-quoting bug the design
    cites) on both paths.
  - Rename split (delete old + add new), not folded.
  - Missing snapshot ref reads as absent, not error.
  - `@{u}` with and without a configured upstream (no-upstream → unpushed).
  - `merge-base --is-ancestor` truth table incl. equal-commit → false.
  - Custom `refs/loomyard/snapshot/*` refspec fetch round-trips.
  - **Push rejection + rebase-retry:** two clones over a bare remote race the same branch;
    assert whether go-git can perform the `pull --rebase`-equivalent recovery, or whether it
    is CLI-BOUND. This is the pivotal test.
  - `SetSnapshotSHA` adopt-on-conflict under a simulated concurrent writer.
- **Windows:** all tests written OS-portable; the Win11 execution is deferred. Mark tests or
  cases whose verdict hinges on Windows behaviour so the later Win11 run knows what to
  re-check.
- **Repo-wide guards:** the new integration-tagged, hermetic test package must keep
  `go test ./...` (Tier 1) green and `go test -tags integration ./...` green on Linux,
  including `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go`.
- **Do not** add tests to or modify `internal/gitrepo` or `internal/gitexec`.

## Q&A log

- **Q:** Empirical prototype or paper analysis? **A:** Throwaway prototype package built
  against the real read surface (evidence over reputation).
- **Q:** Where do findings live, and is the prototype kept? **A:** Findings replace
  `manifest/designs/git-native-library.md`; the prototype **is kept** and merges to `main`
  (accepting go-git in `go.mod` and an experimental package in the tree).
- **Q:** Which library? **A:** Deferred to me — go-git primary (pure Go, no cgo, cross-platform
  binary); `git2go`/libgit2 only investigated if go-git *cannot* do an operation, and not
  adopted by this task.
- **Q:** Is a split result (go-git reads + CLI writes) a legitimate positive outcome?
  **A:** Yes — go-git wherever feasible; an operation is CLI-BOUND only when go-git genuinely
  has no viable alternative (rebase the prime suspect).
- **Q:** "go/no-go" terminology? **A:** Dropped — it collides with the Go language. Use
  MIGRATE / CLI-BOUND per operation and ADOPT / ADOPT-PARTIAL / DECLINE for the overall
  recommendation.
- **Q:** MIGRATE-vs-CLI-BOUND rubric? **A:** (a) typed result, (b) behavioural parity,
  (c) Windows-capable are hard gates; (d) performance and (e) hooks/credential/gitattributes
  are measured/reported but only decisive if genuinely unworkable.
- **Q:** Does the harness test writes and rebase too? **A:** Yes — full surface incl.
  rebase-retry, classified MIGRATE/CLI-BOUND; writes built in the prototype only to locate the
  CLI boundary, not to migrate them.
- **Q:** What form does the kept prototype take? **A:** `internal/gitnativepoc/` — go-git
  library + integration-tagged hermetic parity harness; **not** a registered cobra module.
- **Q:** Windows? **A:** Win11 is a hard gate for the verdict, but this task only writes
  OS-portable, Win11-ready code and verifies on Linux; the actual Win11 run is deferred to a
  later Win11 machine, and Windows-dependent conclusions are marked Win11-pending.
