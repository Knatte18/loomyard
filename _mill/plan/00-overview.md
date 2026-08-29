# Plan: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call

```yaml
task: "prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call"
slug: "prowler-github-tree-script"
approved: true
started: "20260829-110046"
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
    name: test-harness-and-fixtures
    file: 01-test-harness-and-fixtures.md
    depends-on: []
    verify: bash -n plugins/prowler/scripts/testdata/github-tree/bin/gh && bash -n plugins/prowler/scripts/github-tree-selftest.sh
  - number: 2
    name: tree-script-and-docs
    file: 02-tree-script-and-docs.md
    depends-on: [1]
    verify: bash plugins/prowler/scripts/github-tree-selftest.sh
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: TDD ordering is structural, not advisory

- **Decision:** batch 1 delivers the stub `gh`, every JSON fixture body, and the full harness — with no implementation behind it.
  Batch 2 delivers `github-tree.sh` and the documentation.
  Batch 1's `verify:` is a syntax gate (`bash -n`) on the two shell files it creates, because the harness it writes cannot pass until batch 2 exists;
  batch 2's `verify:` is the harness itself.
- **Rationale:** the discussion names the walk logic as the TDD candidate and says the stub and its canned responses "fully define the script's contract".
  Splitting on that seam makes the contract reviewable on its own, before any implementation exists to rationalize it, and makes it impossible for the implementation to be written first.
- **Applies to:** all batches

### Decision: the script's error taxonomy and message wording are part of the contract

- **Decision:** every distinguished failure listed in the discussion's "Strict stdout discipline and fail-fast errors" section has one exact stderr wording, fixed in batch 1's harness assertions and implemented verbatim in batch 2.
  The full table lives in batch 2, card 5;
  batch 1's harness asserts a distinguishing substring of each.
- **Rationale:** the harness is written first, so it must already know what it is asserting.
  Pinning wording in one place stops the two batches from drifting into a mismatch that reads as an implementation bug.
- **Applies to:** all batches

### Decision: exactly one `gh` invocation shape, and the stub only accepts that shape

- **Decision:** `github-tree.sh` calls `gh` in exactly one form — `gh api "<endpoint>" --jq "<expr>"` — four arguments, always in that order, never any other subcommand or flag.
  The stub `gh` rejects anything else with a non-zero exit and a `gh-stub: unsupported invocation` message rather than guessing.
- **Rationale:** the "one call" property this task exists to deliver is only assertable if the call shape is fixed;
  a stub that tolerates arbitrary invocations would silently absorb a re-added `gh auth status` preflight or a `gh repos view` branch-resolve call, which are exactly the two calls this task removes.
- **Applies to:** all batches

### Decision: stub wiring (maps, call log) is generated at run time under `.scratch/`; only bodies are committed

- **Decision:** the committed fixture data is the stub executable plus 25 JSON response bodies.
  The endpoint-to-body map for each scenario, and the stub's call log, are written by the harness into `.scratch/github-tree-selftest/` at run time.
- **Rationale:** the map is test wiring, not fixture data — keeping it inside the harness puts each scenario's endpoint routing next to the assertions that depend on it, instead of in a 14th and 15th file a reader has to cross-reference.
  `.scratch/` is the repo's sanctioned scratch location and is gitignored (`**/.scratch/`);
  the discussion explicitly permits the harness to keep temp state there.
- **Applies to:** all batches

### Decision: fixture bodies are reused across scenarios wherever the shape is identical

- **Decision:** the `midwalk` and `nonrectrunc` scenarios reuse the `trunc1-*` bodies for every endpoint whose response is unchanged, differing only in their generated map (a 403 status on one subtree, a truncated non-recursive root).
  The one-level-fallback scenario doubles as the sibling-order scenario.
- **Rationale:** four scenarios that differ only in *which* endpoint fails do not need four copies of the same tree JSON, and a reader comparing them should see the difference in the map rather than by diffing near-identical files.
- **Applies to:** batch 1

### Decision: no self-location idiom in `github-tree.sh` — a deliberate deviation from the discussion's phrasing

- **Decision:** `github-tree.sh` does **not** compute a `SCRIPT_DIR`/`PLUGIN_ROOT` pair.
  What it does carry over from `run.sh` is the strict stdout discipline, the `set -u` posture, and the `command -v` prerequisite check.
- **Rationale:** the discussion's Technical-context bullet says the script "needs the same self-location idiom", but `run.sh` self-locates in order to reach `bin/`, `go.mod`, and its lock directory.
  `github-tree.sh` references no file inside the plugin at all — it takes two arguments and calls `gh`.
  Adding a `PLUGIN_ROOT` that nothing reads is dead code, and the repo's own code-quality rules would flag it.
  Recorded here explicitly so a reviewer sees a considered deviation rather than an omission.
- **Applies to:** batch 2

### Decision: `CONSTRAINTS.md` is not amended

- **Decision:** no invariant is added, and the GitHub Auth Invariant is not amended.
- **Rationale:** the discussion disposes of the GitHub Auth Invariant explicitly — it binds Go packages in the root module, `cmd/lyx/ghguard_test.go` scans only non-test `.go` files, and a plugin shell script has no route to import `internal/githubclient`.
  This task narrows the plugin's existing `gh` surface from LLM-composed calls to one fixed script;
  it introduces no new cross-cutting rule.
  Every other invariant governs `internal/` Go code, the `lyx` CLI, or hub/worktree geometry.
- **Applies to:** all batches

### Decision: `pipeline.done_gate` is left as configured

- **Decision:** the hub's existing `done_gate` (`go test ./... && go test -tags integration ./...`) is not changed by this task.
- **Rationale:** the task adds no Go code, so the existing repo-wide Go gate remains the right task-wide backstop and already exits 0 today.
  `golangci-lint` is not on this box's `PATH`, so it is not a candidate to add.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `plugins/prowler/README.md`
- `plugins/prowler/scripts/github-tree-selftest.sh`
- `plugins/prowler/scripts/github-tree.sh`
- `plugins/prowler/scripts/testdata/github-tree/bin/gh`
- `plugins/prowler/scripts/testdata/github-tree/bodies/badpath-root-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/error-401.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/error-403.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/error-404.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/error-422.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/noblobs-root-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/nonrectrunc-root-nonrec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/scoped-src-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-lib-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-src-nonrec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-src-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/small-root-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-aaa-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-bbb-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-mmm-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-nonrec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-a-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-b-nonrec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-b-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-bx-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-by-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-root-nonrec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-root-rec.json`
- `plugins/prowler/scripts/testdata/github-tree/bodies/types-root-rec.json`
- `plugins/prowler/skills/github-repo-explorer/SKILL.md`
