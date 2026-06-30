All 27 remaining matches are inside `_mill/` (planning artifacts), which the comprehensive-reference-sweep explicitly excludes. No docs, `CONSTRAINTS.md`, or `CLAUDE.md` references dangle. Working tree is clean; `verify: null` so nothing to run.

{"status":"success","commit_sha":"d61e4e27ca792d3b8e746669d2a88629bc1ffbc4","session_id":"a678294f-fa67-4f58-b808-7bcb83317d3b"}

Batch complete. Summary of work:

- **Card 9** (`46ece85`) — `CONSTRAINTS.md`: renamed the `## Path Invariant` heading to `## Hub Geometry Invariant`, swept `internal/paths` -> `internal/hubgeometry` and `paths.` qualifiers -> `hubgeometry.`, including the lyxtest Leaf Invariant reference.
- **Plan extension** (`c35f9ac`) — `_mill/plan/02-docs.md` + `_mill/plan/00-overview.md`: added `docs/shared-libs/configengine.md` to Card 10's `Edits:` and the union list. It was missing from the plan but carried `paths.ConfigFile` / `paths.DotEnv` package-qualifiers referencing the renamed package, which the `package-name-hubgeometry` Shared Decision requires sweeping. Followed discipline rule 2 (extend plan before touching an out-of-scope file).
- **Card 10** (`d22421e`) — `git mv docs/shared-libs/paths.md docs/shared-libs/hubgeometry.md` plus a reference sweep across `docs/overview.md`, `docs/shared-libs/{README,envsource,configengine}.md`, `docs/modules/{loom,mux}.md`, `docs/benchmarks/test-suite-timing.md`, `docs/roadmap.md`. Renamed the `## Path Invariants` heading in `overview.md` and updated the dependent cross-doc anchor link in `loom.md` (`#path-invariants` -> `#hub-geometry-invariants`).
- **Card 11** (`d61e4e2`) — `CLAUDE.md`: renamed the Path Invariant mention to Hub Geometry Invariant / `internal/hubgeometry`.

Note: the historical task-name token `paths-subpath-mirroring` in the frozen trend log of `test-suite-timing.md` was deliberately left unchanged — it is a past revision name, not an `internal/paths` reference, and does not match any comprehensive-sweep grep.

{"status":"success","commit_sha":"d61e4e27ca792d3b8e746669d2a88629bc1ffbc4","session_id":"a678294f-fa67-4f58-b808-7bcb83317d3b"}
