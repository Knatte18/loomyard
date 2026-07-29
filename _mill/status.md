# Status

```yaml
phase: done
slug: codeintel-trace-rename
branch: codeintel-trace-rename
plan: null
parent: main
task: Rename codeintel module to trace
task_description: |
  Rename codeintel module to trace
```

## Timeline

```text
discussing  '2026-07-29T19:52:13Z'
implemented '2026-07-29T20:36:44Z'
reviewed    '2026-07-29T21:10:00Z'
done  '2026-07-29T21:15:38Z'
```

## Result

Repo-wide rename of the `codeintel` module to `trace`. Pure identifier rename, no design/behavior change.

- `internal/codeintelengine` -> `internal/traceengine`, `internal/codeintelcli` -> `internal/tracecli` (git mv, history preserved).
- CLI surface: `lyx codeintel ...` -> `lyx trace ...` — command, subcommands (`refs`/`definition`/`symbol`/`assert-no-callers`), flags, and help text all renamed; registration in `newRoot()` and the root `Long` module list updated.
- File renames for every path carrying `codeintel`: `internal/hubgeometry/codeinteldaemon_test.go` -> `tracedaemon_test.go`, `docs/benchmarks/codeintel-vs-grep.md` -> `trace-vs-grep.md`, `docs/research/codeintel-{agent-usage-findings,multilang,spike}.md` -> `trace-*`, `manifest/designs/codeintel-plan-symbol-fields.md` -> `trace-plan-symbol-fields.md`.
- Text substitution (`codeintel`/`Codeintel`/`CodeIntel` -> `trace`/`Trace`/`Trace`) across every git-tracked file that referenced the module: `CONSTRAINTS.md` (Leaf Invariant section + Sandbox Suite Coverage allowlist entry), `docs/overview.md`, `docs/reference/plan-format-v3.md`, `manifest/roadmap.md`, `manifest/designs/{fabric-unified-view,finalize,raddle,semantic-index,webster-parallel-execution}.md`, and doc comments in `internal/{gitrepo,boardengine,websterengine,hubgeometry}`.
- `hubgeometry.CodeintelDaemonStateFile`/`CodeintelDaemonLock` renamed to `TraceDaemonStateFile`/`TraceDaemonLock`; the on-disk runtime path moves from `_lyx/codeintel/<lang>/` to `_lyx/trace/<lang>/` (regenerated state, no migration needed).
- `gofmt -w` re-aligned a handful of files (map-literal value alignment, import-block ordering) where the shorter `trace`/`Trace` strings broke existing gofmt alignment.

Scripts used (repo discipline: scripted, not manual file-by-file edits — same as the `markdown-unwrap` task): a `git mv`-based path-rename script followed by a Python case-variant text-substitution pass, both written to the session scratchpad (not committed, per the `markdown-unwrap` precedent).

**Verification:**

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — all packages pass, including `internal/tracecli` and `internal/traceengine`.
- `go test -tags integration ./internal/traceengine/... ./internal/hubgeometry/...` — pass.
- `gofmt -l .` — clean except two pre-existing, out-of-scope drift files (`cmd/lyx/tierpurity_test.go`, `internal/builderengine/plan_test.go`) confirmed unrelated to this change via `git stash`.
- `lyx --help` / `lyx trace --help` built and run manually — command tree and help text read correctly.
- `git ls-files | xargs grep -ni codeintel` — empty except `_mill/status.md`'s own `slug`/`branch`/`task` fields, which name *this task's* worktree/branch (`codeintel-trace-rename`) and are not the module identifier. The two in-flight wiki task slugs (`codeintel-v1`, `codeintel-daemon-persistence`) live in `.wiki/`, untouched, as scoped.

**Not touched (deliberately, per rename scope):** `.wiki/` task slugs; `.scratch/prompt-codeintel-trace-rename.md` (gitignored, this task's own instructions).

**Review pass:** workflow-backed xhigh code review (`code-review` skill, task `woec3k81u`) — 6 finders + independent verifiers, 18 agents, 1.05M tokens. 8 confirmed findings, all the same root cause: the blind text substitution over-applied itself to references into *immutable history* — deleted git-history paths (`tools/codeintel-poc/`), a real git tag (`archive/codeintel-spike`), real branch/task slugs (`codeintel-v1`, `codeintel-multilang`, `codeintel-daemon-persistence`, `codeintel-spike`), gitignored scratch-directory paths from a past benchmark run (`.scratch/codeintel/...`), and a reproduction recipe pinned to pre-rename commit `448e5b25` (whose binary only ever registered a `codeintel` subcommand, never `trace`). Fixed in commit `cd293223`: those specific literal facts reverted to `codeintel`; every reference to the current, live module (package names, current file paths, present-day CLI usage) stays `trace`. Re-verified after fixes: `go build`/`go vet`/`go test ./...` clean, `gofmt -l .` clean except the same two pre-existing files.

Next: finalize (`mill-merge`).
