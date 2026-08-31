# Plan: Add cross-repo code search to prowler

```yaml
task: Add cross-repo code search to prowler
slug: cross-repo-code-search
approved: true
started: 20260831-152840
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
    name: search-script-and-harness
    file: 01-search-script-and-harness.md
    depends-on: []
    verify: bash plugins/prowler/scripts/github-code-search-selftest.sh
  - number: 2
    name: documentation
    file: 02-documentation.md
    depends-on: [1]
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: github-tree.sh is the shape to copy, verbatim where it applies

- **Decision:** every new artefact mirrors its `github-tree.sh` sibling — the script mirrors `github-tree.sh` (header comment arguing the no-retry stance, `die()` helper, prerequisite-then-argument-then-network ordering, `gh api --jq` header-line-plus-records extraction, `#badpath` sentinel, buffer-until-complete output), the harness mirrors `github-tree-selftest.sh` (stub on `PATH`, per-scenario `map.tsv`, `calls.log` appended before shape validation, `require_jq` guard, `.scratch/` scratch root, `fail`/`pass` counters, `BASH_BIN` absolute-path trick), and the stub mirrors `testdata/github-tree/bin/gh` (log-before-validate, failure emulation writing the raw body to **stdout** with `--jq` unapplied and exit 1).
- **Rationale:** two sibling scripts that diverge in stdout discipline, exit-code convention, or failure emulation are a trap for any caller wrapping both, and the tree harness already proved this shape works offline.
  Every deliberate divergence from the sibling is named explicitly in the card that introduces it, so a reviewer can tell a copy from a decision.
- **Applies to:** all batches

### Decision: the `<owner>/<repo>` predicate is `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`, used in exactly one form everywhere

- **Decision:** `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$` — copied verbatim from `github-tree.sh` line 49 — is the single ref predicate for this task, used both by `github-code-search.sh`'s own argument validation and by the SKILL.md repo-list scan rule.
- **Rationale:** a ref the skill accepts and the script rejects surfaces as a confusing exit 1 from an invocation the skill's own documentation just told the model to make.
  The predicate is deliberately shape-only — existence and reachability are the preflight's job, and the two failures carry distinct messages.
- **Applies to:** all batches

### Decision: exit 2 is only for a usage-shape error; every other rejection is exit 1 via `die`

- **Decision:** exit 2 is reserved for too few arguments to satisfy the synopsis `github-code-search.sh <query> <owner/repo> [<owner/repo>...]` — no arguments, or a query with no repo ref.
  Every other rejection exits 1 via `die` with one stderr line: an invalid `<owner>/<repo>` ref, more than 10 distinct repo refs, a caller query containing `repo:`, an empty-string query, a whitespace-only query.
- **Rationale:** this is `github-tree.sh`'s actual convention — exit 2 only for the arg-count check printing the bare `usage:` synopsis, exit 1 via `die` for a semantically invalid ref and every other rejection.
  The split is shape-versus-semantics, not "all argument problems are 2": 11 refs still satisfies the synopsis's `<owner/repo>...`, so exit 2 would be wrong there.
- **Applies to:** all batches

### Decision: stdout is records only, buffered until the whole sweep succeeds

- **Decision:** stdout carries exactly one tab-separated record per matching file — `<owner>/<repo>\t<path>\t<snippet>` — and nothing else.
  Every record is accumulated in memory across all repos and printed only once the last repo has succeeded;
  any failure at any point means byte-empty stdout and a non-zero exit.
  Diagnostics and the `total_count` cap note go to stderr.
- **Rationale:** verbatim the discipline `github-tree.sh` documents.
  With a multi-repo sweep the risk is worse than for a single tree: a prefix covering repos 1–3 of 8 looks exactly like a sweep where repos 4–8 had no hits.
- **Applies to:** all batches

### Decision: the record shape is invariant — three fields, always

- **Decision:** the snippet is the first `text_matches` fragment with every tab/CR/LF collapsed to a single space and the result truncated to 200 characters.
  When an item's `text_matches` array is absent or empty, the record is still emitted with an **empty third field** — the two tabs are still present.
  There is no flag and no second output mode.
- **Rationale:** a hit is a hit whether or not a fragment came back with it, and dropping the record would silently under-report matches;
  a fixed field count means a caller can split on tabs without a special case.
  One format means one contract to test.
- **Applies to:** all batches

### Decision: `gh` is the only runtime dependency; system `jq` belongs to the harness alone

- **Decision:** all JSON extraction at run time goes through `gh api --jq` (gh's embedded gojq).
  System `jq` is required only by the offline selftest harness, which checks for it up front with a `require_jq` guard.
- **Rationale:** exactly the split `github-tree.sh` and `github-tree-selftest.sh` already establish and document.
  `gh` is already a hard prerequisite of the skill;
  a system-`jq` runtime dependency would narrow where the skill works for no gain.
- **Applies to:** all batches

### Decision: markdown files use semantic line breaks

- **Decision:** every `.md` file touched — `README.md`, `SKILL.md`, `INDEX.md` — uses one sentence per line, with additional breaks at internal independent-clause boundaries.
  Never a fixed-column hard wrap, never trailing double-spaces or a backslash.
  Table cells stay on one line.
- **Rationale:** the repo's `CLAUDE.md` rule, and the files being edited already follow it.
- **Applies to:** documentation

### Decision: nothing outside `plugins/prowler/` is touched

- **Decision:** `CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/`, `manifest/roadmap.md`, `.claude-plugin/marketplace.json`, `plugins/prowler/.claude-plugin/plugin.json`, `github-tree.sh`, `github-tree-selftest.sh`, `testdata/github-tree/`, `run.sh`, `selftest.sh`, the Go module under `plugins/prowler/`, the `prowler` skill, and the `distill-subagent` skill all stay unchanged.
- **Rationale:** no CONSTRAINTS.md invariant binds this change (the GitHub Auth Invariant scopes to the `lyx` Go module's production packages;
  every other invariant concerns Go packages under `internal/` or the sandbox suite, and this task adds no Go code).
  `grep -rl prowler docs manifest` returns nothing, so the module-doc and overview obligations have no target.
  This is neither completing nor adding a roadmap item.
  Unpublished loomyard plugins are not version-bumped per feature, and the comparable prior feature commit `63916b1e2` did not bump `plugin.json` either.
  The plugin-level description in the marketplace manifest describes the plugin, not the skill, and is unaffected by a skill-description change.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `plugins/prowler/README.md`
- `plugins/prowler/scripts/github-code-search-selftest.sh`
- `plugins/prowler/scripts/github-code-search.sh`
- `plugins/prowler/scripts/testdata/github-code-search/bin/gh`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/badpath-fullname.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/badpath-path.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/capped.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/error-401.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/error-403.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/error-404.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/error-422.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-alpha.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-beta.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-gamma.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-multi.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-no-textmatches.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-truncate.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-zero.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/incomplete.json`
- `plugins/prowler/scripts/testdata/github-code-search/bodies/preflight-ok.json`
- `plugins/prowler/skills/INDEX.md`
- `plugins/prowler/skills/github-repo-explorer/SKILL.md`
