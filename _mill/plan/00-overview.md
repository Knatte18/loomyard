# Plan: Rename internal/ghissues → selfreport

```yaml
task: Rename internal/ghissues → selfreport
slug: rename-ghissues-to-selfreport
approved: false
started: 20260630-141815
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: code-rename
    file: 01-code-rename.md
    depends-on: []
    verify: go build ./... && go test ./...
  - number: 2
    name: docs
    file: 02-docs.md
    depends-on: [1]
    verify: null
```

## Shared Decisions

### Decision: module-name-selfreport

- **Decision:** Rename the module `ghissues` → `selfreport`: package `ghissuescli` →
  `selfreportcli`, package `ghissuesengine` → `selfreportengine`, and the CLI verb
  `lyx ghissues create` → `lyx selfreport create`. The hardcoded `targetRepo =
  "Knatte18/loomyard"` is unchanged.
- **Rationale:** Names the responsibility (lyx self-reporting bugs/enhancements to its own
  repo), matches the `millhouse-issue` precedent, and follows the repo's
  `<module>cli`/`<module>engine` convention. Behaviour-preserving: no flags, output shape,
  target repo, label defaults, stdin handling, JSON envelope, or exit codes change.
- **Applies to:** all batches

### Decision: rename-via-git-mv

- **Decision:** Perform directory/file renames with `git mv` FIRST, then make only surgical
  edits to package clauses, import paths, identifiers, and comments. No full-file rewrites.
  The engine implementation file is renamed `ghissues.go` → `selfreport.go` to match the
  package name.
- **Rationale:** Preserves git rename history/blame and keeps the diff reviewable as a
  rename + small edits. Standing repo convention for renames.
- **Applies to:** code-rename

### Decision: rename-all-identifiers

- **Decision:** Rename **every** `Ghissues`/`ghissues` occurrence in test function names,
  doc comments, and string literals to `Selfreport`/`selfreport`. No stale identifiers
  remain anywhere outside `_mill/`.
- **Rationale:** Operator decision in discussion — a fully consistent rename, not a
  minimal pin-only change.
- **Applies to:** all batches

### Decision: reframe-help-prose

- **Decision:** Reframe the parent `Short` and the `create` subcommand `Short`/`Long` to
  lead with the self-report responsibility (file a LoomYard bug/enhancement to lyx's own
  repo), while still naming `gh`/GitHub as the mechanism. Update every `Long` usage example
  to `lyx selfreport create`.
- **Rationale:** The module name now advertises the responsibility, so help should too;
  but the CLI/Cobra Invariant requires help prose to stay behaviour-accurate, and the
  command genuinely files a GitHub issue via `gh` — so the mechanism stays visible.
- **Applies to:** code-rename

### Decision: drop-mill-pipeline-refs

- **Decision:** **Remove** every `mill-ghissues-to-tasks` reference from Loomyard docs and
  reword the surrounding prose so the sentences stand without naming that mill pipeline. Do
  NOT rename it to `mill-selfreport-to-tasks`.
- **Rationale:** `mill-ghissues-to-tasks` is a millhouse skill in a separate repo; lyx has
  no authority to rename it and no reason to mention it from Loomyard docs. The docs should
  describe what lyx does (file a self-report GitHub issue) and stop there.
- **Applies to:** docs

## All Files Touched

- `cmd/lyx/helptree_test.go`
- `cmd/lyx/jsonhelp_test.go`
- `cmd/lyx/main.go`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/sandbox-howto.md`
- `docs/sandbox-hub.md`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/selfreportcli/cli.go`
- `internal/selfreportcli/cli_test.go`
- `internal/selfreportengine/selfreport.go`
- `tools/sandbox/test-scheme.md`
