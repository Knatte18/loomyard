# Plan: Local lyx sandbox for manual experimentation

```yaml
task: "Local lyx sandbox for manual experimentation"
slug: "lyx-sandbox"
approved: false
started: "20260626-193707"
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
    name: sandbox-tool-and-docs
    file: 01-sandbox-tool-and-docs.md
    depends-on: []
    verify: go test ./tools/sandbox/... ./internal/paths/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: drive-the-deployed-lyx

- **Decision:** The tool builds the Hub by invoking the on-PATH `lyx` binary as a
  subprocess (`lyx warp clone <host> <weft>`), never by importing `internal/warp` or
  reimplementing clone logic.
- **Rationale:** The whole point is dogfooding the real shipped CLI — exercising the actual
  command surface, JSON output, and topology wiring a user runs. Calling the library would
  test the library, not the binary.
- **Applies to:** all batches

### Decision: error-surface-verbatim

- **Decision:** The tool streams the subprocess's stdout/stderr through to the operator and
  propagates its exit code; it never swallows or reinterprets a `lyx warp clone` failure.
- **Rationale:** The two by-design failure modes — `lyx` not on PATH (exec fails) and an
  unreachable host/weft/board (`warp clone` tears down the partial Hub) — must surface with a
  legible cause, not a silent no-op.
- **Applies to:** all batches

### Decision: path-invariant-compliance

- **Decision:** `tools/sandbox` must not contain the literal tokens `os.Getwd` or
  `git rev-parse --show-toplevel`. The parent directory comes from the `-parent` flag; the
  subprocess working directory is set via `exec.Command(...).Dir`.
- **Rationale:** `internal/paths/enforcement_test.go` walks the **entire** source tree
  (allowlist only `internal/paths` and `cmd/lyx/main.go`) and fails the build on either
  token. `tools/sandbox` is in scope of that scan and needs neither primitive.
- **Applies to:** all batches

### Decision: machine-path-out-of-go

- **Decision:** No machine-specific path is baked into the committed Go source. `-parent` is
  required with no default; the thin `sandbox.cmd` launcher supplies the `C:\Code` value.
- **Rationale:** Mirrors the repo's own lesson (`deploy.cmd` holds `C:\Code\tools\bin`,
  `tools/deploy` stays general). Keeps the Hub location configurable per machine.
- **Applies to:** all batches

### Decision: dedicated-dogfood-repos

- **Decision:** Host = `https://github.com/Knatte18/lyx-test`, weft =
  `https://github.com/Knatte18/lyx-test-weft` are fixed constants in the tool; the board URL
  is left to `warp clone`'s default derivation (`<weft>.wiki.git`). No override flags.
- **Rationale:** These two repos are dedicated to this dogfood use only, so there is no need
  to parameterize them. The default board derivation is lyx's built-in behavior
  (`internal/warp/clone.go` `deriveBoardURL`) and is already correct.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` across every batch, sorted
alphabetically. mill-go reads this to warn if two parallel batches
touch the same file — a sign of a misplaced dependency._

- `docs/dogfood-hub.md`
- `docs/overview.md`
- `sandbox.cmd`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
