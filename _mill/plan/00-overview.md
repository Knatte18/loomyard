# Plan: Refine SANDBOX-SUITE.md from the 2026-06-30 sandbox run

```yaml
task: "Refine SANDBOX-SUITE.md from the 2026-06-30 sandbox run"
slug: "sandbox-suite-refinements"
approved: true
started: "20260630-055411"
parent: "main"
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
    name: scheme-refinements
    file: 01-scheme-refinements.md
    depends-on: []
    verify: go test ./tools/sandbox/...
```

## Shared Decisions

### Decision: single-file doc edits

- **Decision:** Every card edits exactly one file, `tools/sandbox/test-scheme.md`. This
  is the authored source the launcher renders into `SANDBOX-SUITE.md` (fingerprint header
  + this body) via `//go:embed` in `tools/sandbox/suite.go`. No literal `SANDBOX-SUITE.md`
  exists in-repo.
- **Rationale:** The issues (#39/#40/#41) name "SANDBOX-SUITE.md" but it is generated;
  `test-scheme.md` is the single edit target.
- **Applies to:** all batches

### Decision: preserve the pinned H1 heading

- **Decision:** Keep the `# Sandbox test-scheme -- lyx black-box agent suite` H1 line
  intact. `tools/sandbox/suite_test.go` (`TestRenderScheme_ContainsHeaderAndBody`) pins
  the substring `"Sandbox test-scheme"`; removing or renaming the heading breaks the test.
- **Rationale:** The only automated coupling between the markdown and Go is this heading
  assertion plus the `//go:embed` directive.
- **Applies to:** all batches

### Decision: no scenario renumbering

- **Decision:** Removing S5 from the agent scenario list leaves S6 numbered S6. The
  numbering gap (S0–S4, S6) is intentional and signals S5 moved to the operator section.
- **Rationale:** Prior GitHub issues and session logs cite scenarios by number;
  renumbering S6→S5 would silently break those references.
- **Applies to:** all batches

## All Files Touched

- `tools/sandbox/test-scheme.md`
