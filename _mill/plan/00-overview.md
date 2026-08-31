# Plan: Prefer raw fetch, scope large tree listings

```yaml
task: "Prefer raw fetch, scope large tree listings"
slug: "raw-fetch-tree-scoping"
approved: true
started: "20260831-153224"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: github-tree flag parsing, children mode, entry-count guard
    file: 01-tree-children-guard.md
    depends-on: []
    verify: bash plugins/prowler/scripts/github-tree-selftest.sh
  - number: 2
    name: github-read raw-first script and its offline harness
    file: 02-github-read.md
    depends-on: []
    verify: bash plugins/prowler/scripts/github-read-selftest.sh
  - number: 3
    name: skill and README documentation
    file: 03-docs.md
    depends-on: [1, 2]
    verify: null
```

## Shared Decisions

### Decision: this is a Bash + Markdown task; no Go, no `PYTHONPATH=` prefix

- **Decision:** every file this plan touches is a `.sh` script, a JSON fixture, or a `.md` document.
  No Go source, no `go.mod` change, no `manifest/designs/` module doc, no `manifest/roadmap.md` movement, no `CONSTRAINTS.md` amendment.
  Both batch `verify:` commands are plain `bash <harness>` invocations with no `PYTHONPATH=` prefix — the repo is a Go project, not a Python one, so the prefix rule does not apply.
- **Rationale:** the discussion's Scope section lists exactly these files as In, and its Constraint check establishes that the **GitHub Auth Invariant** binds Go production packages only, leaving `plugins/prowler/scripts/*.sh` outside it.
  The project CLAUDE.md excludes hardening of an already-merged change from roadmap movement.
- **Applies to:** all batches

### Decision: strict stdout discipline, buffered output, one stderr line on failure

- **Decision:** both production scripts write the payload and nothing else to stdout, send every diagnostic to stderr as exactly one physical line, and never leave a partial prefix on stdout when they fail.
  `github-tree.sh` keeps its existing end-of-walk buffered emission;
  `github-read.sh` buffers to a `mktemp` temp file and `cat`s it only after the transfer has already succeeded.
- **Rationale:** this is the contract `run.sh` and `github-tree.sh` already establish, and the one the `github-repo-explorer` skill's "Check the exit code, always" paragraph depends on.
  A guard abort or a fallback failure that fits the existing contract needs no new caller-side handling.
- **Applies to:** all batches

### Decision: exit 2 is usage, exit 1 is every operational failure

- **Decision:** both scripts reserve exit status 2 for malformed invocations (bad argument count, unrecognised `--`-prefixed token, non-integer `--max-entries`, empty path in `github-read.sh`) and use the existing `die` helper — one stderr line, exit 1 — for every operational failure, including the new entry-count guard abort and the new directory/symlink/submodule rejection.
- **Rationale:** the existing harness already pins this distinction (`github-tree-selftest.sh` test 21 asserts `status -eq 2` specifically), and a listing that is too large is not a malformed invocation.
  No current caller branches on anything beyond zero/non-zero, so a third exit code would be an untested distinction serving nobody.
- **Applies to:** batch 1, batch 2

### Decision: no retries, no backoff; timeouts are not retries

- **Decision:** neither script retries anything.
  `github-read.sh`'s `--connect-timeout 5 --max-time 30` bounds turn a hung raw request into one clean non-zero exit that hands off to the single `gh api` attempt exactly once.
- **Rationale:** `github-tree.sh`'s header states the no-retry policy as a deliberate design decision, not an omission;
  `github-read.sh` adopts it unchanged.
  Bounding a hang is a distinct concern from retrying — without the bound the fallback would never fire at all.
- **Applies to:** batch 1, batch 2

### Decision: path validation is duplicated, never extracted to a sourced library

- **Decision:** `github-read.sh` carries its own copy of `github-tree.sh`'s path-normalisation and character-validation logic, keeping the glob-substitution form `offending="${path//[A-Za-z0-9._\/-]/}"` and its accompanying comment's reasoning, and copies the `<owner>/<repo>` slug regex verbatim including its bracket-range form.
  No shared sourced file is introduced.
- **Rationale:** `github-tree.sh`'s header documents as a deliberate property that it reads no file inside the plugin and self-locates no `SCRIPT_DIR`/`PLUGIN_ROOT`;
  a sourced library would destroy that for both scripts and add a missing-library failure mode to a script that has none today.
  The regex form of the path check wrongly accepts accented characters under a UTF-8 locale — a reproduced failure, not a hypothetical — so the glob form must survive the copy intact.
  The slug check's identical looseness is an accepted, recorded property: divergence between the two scripts' slug checks would be the worse outcome.
- **Applies to:** batch 2

### Decision: shared argument-parsing shape across both scripts

- **Decision:** both scripts use the same parse loop shape.
  A `--` token is a terminator honoured at any position;
  before the terminator, a token beginning with two dashes is either a recognised flag (only while no positional has yet been collected) or a usage error;
  after the terminator every token is a positional.
  A single-dash token is never a flag at any position.
- **Rationale:** the discussion requires that `github-read.sh acme/x --foo` and `github-tree.sh acme/x --foo` not disagree about what that token is, and that `github-read.sh acme/x -- --weird-path` be a legitimate two-positional invocation.
  One shape satisfies both, and keeps a typo'd `--childern` from becoming a confusing remote 404.
- **Applies to:** batch 1, batch 2

### Decision: markdown edits follow semantic line breaks

- **Decision:** every `.md` line this plan writes or edits is one sentence per line, with additional breaks at internal independent-clause boundaries, using plain newlines.
  No fixed-column hard-wrap, no trailing double-space or backslash breaks.
  This binds lines edited in place, not only newly-added ones.
- **Rationale:** the project CLAUDE.md's Markdown rule applies to every `.md` file in the repo, not only newly-written ones.
- **Applies to:** batch 3

### Decision: harness scratch stays under `<repo>/.scratch/`, production scripts use `mktemp`

- **Decision:** `github-read-selftest.sh` scratches under `<repo>/.scratch/github-read-selftest`, mirroring `github-tree-selftest.sh`.
  `github-read.sh` itself uses `mktemp`, honouring `TMPDIR`.
- **Rationale:** the never-write-to-a-system-temp-directory rule is scoped to ephemeral files — drafts, scratch fixtures, debug dumps — which is what a harness produces.
  It does not bind a production script that must work from an arbitrary cwd against a possibly read-only checkout and that deliberately self-locates no plugin root.
  Pointing `TMPDIR` at the harness scratch directory is also what makes the temp-file-cleanup assertion possible.
- **Applies to:** batch 2

### Decision: private repository identifiers are redacted everywhere this task writes

- **Decision:** no file this task creates or edits — plan files, captured testdata, harness comments, skill, or README — names a private repository by its literal owner-and-name string.
  Private targets are referred to by a placeholder such as `<private-repo>`, and any private path appearing inside a captured command or response body is genericized the same way.
  Public repositories may be named literally.
- **Rationale:** this repository is public and the prowler plugin is distributed as an installable package, so a private repository name committed into tracked content permanently discloses that repository's existence to everyone who installs it.
  The live captures' value is the observed response shapes, which a placeholder preserves entirely — nothing about the fixtures or the parser depends on which private repository produced them.
- **Applies to:** all batches

### Decision: neither harness is wired into CI

- **Decision:** both harnesses stay manually invoked.
  No Go test wraps them, no CI job registers them, and this plan adds no runner.
- **Rationale:** the existing `github-tree-selftest.sh` is already in that position and the discussion states explicitly that there is no runner to register a new harness with.
  The `verify:` commands in this plan invoke them directly, which is the same manual invocation mill-go performs on the plan's behalf.
- **Applies to:** batch 1, batch 2

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically._

- `plugins/prowler/README.md`
- `plugins/prowler/scripts/github-read-selftest.sh`
- `plugins/prowler/scripts/github-read.sh`
- `plugins/prowler/scripts/github-tree-selftest.sh`
- `plugins/prowler/scripts/github-tree.sh`
- `plugins/prowler/scripts/testdata/github-read/CAPTURE.md`
- `plugins/prowler/scripts/testdata/github-read/bin/curl`
- `plugins/prowler/scripts/testdata/github-read/bin/gh`
- `plugins/prowler/scripts/testdata/github-read/bodies/error-401.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/error-403.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/error-404.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/error-404.stderr`
- `plugins/prowler/scripts/testdata/github-read/bodies/error-multiline.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/error-nostatus.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/plain.txt`
- `plugins/prowler/scripts/testdata/github-read/bodies/probe-dir.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/probe-file.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/probe-submodule.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/probe-symlink.json`
- `plugins/prowler/scripts/testdata/github-read/bodies/withnul.bin`
- `plugins/prowler/scripts/testdata/github-read/bodies/zero.txt`
- `plugins/prowler/scripts/testdata/github-tree/bin/gh`
- `plugins/prowler/scripts/testdata/github-tree/bodies/children-empty-nonrec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/children-src-nonrec.json`
- `plugins/prowler/skills/github-repo-explorer/SKILL.md`
