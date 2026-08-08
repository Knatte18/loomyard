# Plan: Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard

```yaml
task: 'Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard'
slug: scout-seam-conversion
approved: false
started: '20260808-061134'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: seam-tests
    file: 01-seam-tests.md
    depends-on: []
    verify: go test ./internal/scoutengine/ && go vet -tags scout ./internal/scoutengine/
  - number: 2
    name: docs-alignment
    file: 02-docs-alignment.md
    depends-on: [1]
    verify: go vet ./internal/scoutengine/ && go test ./internal/scoutengine/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: no production code changes

- **Decision:** this task edits exactly four files — two test files, one Go doc comment, one markdown line.
  No `.go` file outside `internal/scoutengine/doc.go` (a pure documentation comment) and the two test files is edited.
  In particular `internal/scoutengine/lspclient.go`'s `internal/logger` import stays exactly as it is, and no `internal/scoutcli` file is touched.
- **Rationale:** the task is a docs-and-tests hardening pass.
  The production refactor it unblocks (re-signaturing `DaemonStateFile`/`DaemonLock` onto `*lyxcwd.Location`) is the separate follow-up task `scout-lyxcwd-accessors`.
- **Applies to:** all batches

### Decision: banned list, never an allowlist

- **Decision:** `internal/scoutengine`'s package-wide import rule is purely negative: never `internal/output`, never cobra, never any `internal/*cli` package, never `internal/clihelp`.
  Nothing in this plan may introduce, restore, or approximate an allowlist — including a "report anything unrecognised" catch-all, which is an allowlist under another name.
- **Rationale:** no other engine module in the repo carries an import allowlist, and the old allowlist's isolation claim was already false: `internal/logger` was on it and imports `internal/lyxcwd` and `internal/proc` itself.
  The check polices direct imports only, never the transitive closure.
- **Applies to:** all batches

### Decision: the guard's allowed set is stdlib plus logger, and is never called "stdlib-only"

- **Decision:** the file-scoped guard on `internal/scoutengine/lspclient.go` allows the standard library plus exactly `github.com/Knatte18/loomyard/internal/logger`.
  Every header comment, doc string, and failure message describes the property as "no lyx dependency except logging".
  The words "stdlib-only" and "hermetic" must not appear as an assertion about that file anywhere this plan writes text.
- **Rationale:** `lspclient.go` already imports `internal/logger` (five `logger.Warn` call sites), so a literally stdlib-only guard would fail on its first run, and `internal/logger` pulls in `internal/lyxcwd` and `internal/proc`, so the file is not hermetic either.
  Describing it as stdlib-only would reproduce, at file scope, the exact mislabelling this task exists to correct.
- **Applies to:** all batches

### Decision: CONSTRAINTS.md is pre-staged and no card re-applies it

- **Decision:** the `## Scout Engine-Seam Invariant` section of `CONSTRAINTS.md` was written and committed during mill-start and is already on this branch.
  No card in this plan edits `CONSTRAINTS.md`;
  card 6 only verifies that the committed section's "Enforced by" line still matches the test paths and function names this plan actually lands.
  Every other section of `CONSTRAINTS.md` belongs to the parallel `leaf-invariant-audit` task and must not be touched.
- **Rationale:** the doc is deliberately ahead of the tests until this plan closes the gap;
  reverting it would recreate the discussion/doc contradiction it was pre-staged to avoid.
- **Applies to:** all batches

### Decision: both test files are untagged and satisfy Test Tier Purity

- **Decision:** neither test file carries a `//go:build` tag — both run in the default untagged build.
  Neither may contain the tokens `exec.Command`, `exec.CommandContext`, `gitexec.RunGit`, or `lyxtest.Copy` anywhere, including inside a comment or string literal, since the Test Tier Purity check is a raw substring match.
  Both parse imports with `go/parser` in `parser.ImportsOnly` mode rather than grepping, so import-path strings inside doc comments cannot produce false positives.
- **Rationale:** `internal/scoutengine`'s five `//go:build scout` test files are compiled by no pipeline gate;
  a new guard behind a tag would never run.
- **Applies to:** all batches

### Decision: guards are assertion-only and proven red by hand

- **Decision:** neither test grows a table-driven negative case.
  Both are assertion-only, matching the repo's other import guards, and card 3 proves each one red by temporarily introducing a violation, observing the failure, and reverting.
  The claim is scoped to import guards specifically: `internal/lyxcwd/raddle_guard_test.go` — cited in card 2 as the naming and directory-resolution precedent — does carry a table-driven `t.Run("predicate", ...)` negative case, because it guards a string predicate rather than an import set.
- **Rationale:** consistency with `internal/lyxtest`, `internal/shuttleengine`, `internal/pattern`, and `internal/modelspec`, none of which has a negative case;
  a table-driven variant would require refactoring the matcher into a separately-testable predicate, a shape no other guard in the repo uses.
- **Applies to:** all batches

### Decision: vocabulary bans

- **Decision:** the tokens `weft` and `warp` must not appear in any text this plan writes (Fabric Vocabulary Invariant).
  The word "leaf" must not survive in any scout-facing text this plan edits.
- **Rationale:** repo-wide invariant plus the point of the task — scout stops being a leaf the moment the allowlist goes.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `docs/overview.md`
- `internal/scoutengine/doc.go`
- `internal/scoutengine/lspclient_guard_test.go`
- `internal/scoutengine/seam_enforcement_test.go`
