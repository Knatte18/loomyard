# Plan: Sandbox suite: emit findings JSON on the shared analysis contract

```yaml
task: 'Sandbox suite: emit findings JSON on the shared analysis contract'
slug: sandbox-report-json
approved: true
started: 20260630-184256
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
    name: emitter-and-fetch
    file: 01-emitter-and-fetch.md
    depends-on: []
    verify: go build ./... && go test ./tools/sandbox/... ./internal/paths/...
  - number: 2
    name: docs
    file: 02-docs.md
    depends-on: [1]
    verify: null
```

## Shared Decisions

### Decision: suite.go stamps meta.fingerprint

- **Decision:** The agent writes only `{source, items}` to `sandbox-report.json`. After a
  clean session, `suite.go`'s fetch helper sets `meta.fingerprint` from the authoritative
  `binaryInfo` it already computes (`Path`, `Size`, `ModTime`, `SHA256[:12]`); any `meta` the
  agent wrote is overwritten.
- **Rationale:** the fingerprint is machine-known; making an LLM transcribe four fields is
  needless error surface.
- **Applies to:** all batches

### Decision: -loomyard is a top-level flag, %~dp0 quoted with a trailing dot

- **Decision:** `tools/sandbox` learns the loomyard root from a new **top-level** flag
  `-loomyard` parsed by the top-level flagset `fs` in `main.go` (alongside `-parent`/`-reset`),
  threaded into `runSuite` — *not* a suite-local flag, because `sandbox.cmd`'s `%*` places the
  `suite` token after the launcher's own flags. The launcher passes `-loomyard "%~dp0."` (the
  trailing `.` avoids `%~dp0`'s trailing backslash escaping the closing quote); the tool
  `filepath.Clean`s + `filepath.Abs`es it.
- **Rationale:** the Path Invariant forbids `os.Getwd`/`git rev-parse` in `tools/sandbox`, so
  the root must arrive as data; the launcher already owns the machine-specific paths.
- **Applies to:** emitter-and-fetch

### Decision: typed-decode validation with *[]Item

- **Decision:** the fetch helper typed-decodes the report and requires `source ==
  "sandbox-report"` and a **present** `items` array. `Items` is decoded as `*[]item` (pointer):
  a nil pointer means the `items` key was absent → reject as malformed; a non-nil pointer to a
  possibly-empty slice (`[]` is valid) means present → accept.
- **Rationale:** LLM-produced JSON can be malformed or valid-but-wrong-shape; catch it at suite
  time. A plain `[]item` cannot distinguish absent from empty.
- **Applies to:** emitter-and-fetch

### Decision: clean-slate removal of any prior report before launch

- **Decision:** `runSuite` removes any pre-existing `<host>/sandbox-report.json` before
  launching the agent (`os.Remove`, ignore `os.IsNotExist`).
- **Rationale:** the report is git-excluded in the host repo, so a prior run's copy persists.
  Without a clean, a clean-exit-no-rewrite would fetch stale findings under a fresh fingerprint.
  Removal makes that scenario correctly surface as the missing-report case.
- **Applies to:** emitter-and-fetch

### Decision: normalized re-serialize, fetch only on clean exit

- **Decision:** the report landed in `.scratch/` is `suite.go`'s re-serialized form (decode →
  stamp `meta` → marshal indented), not a byte copy. The fetch runs only when the agent session
  exits 0; a non-zero exit returns the existing `claude exited with code N` error without
  fetching.
- **Rationale:** re-serialization is what carries the stamped fingerprint; a crashed session is
  already a visible failure not worth partial-report special-casing.
- **Applies to:** emitter-and-fetch

### Decision: scheme file stays SANDBOX-SUITE.md; fix stale test-scheme.md refs

- **Decision:** the embedded scheme file is and stays `tools/sandbox/SANDBOX-SUITE.md` (its
  on-disk name and the name it is copied under into the host repo). The proposal's and both
  docs' references to `tools/sandbox/test-scheme.md` describe a file that does not exist — a
  bug. Edit the real `SANDBOX-SUITE.md`, and repoint every `test-scheme.md` reference in the
  two docs to `SANDBOX-SUITE.md`. No rename.
- **Rationale:** there is no separate scheme file (`//go:embed SANDBOX-SUITE.md`,
  `suiteFileName = "SANDBOX-SUITE.md"`); `test-scheme.md` is stale doc rot.
- **Applies to:** all batches

## All Files Touched

- `docs/sandbox-howto.md`
- `docs/sandbox-hub.md`
- `sandbox.cmd`
- `tools/sandbox/SANDBOX-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
- `tools/sandbox/report.go`
- `tools/sandbox/report_test.go`
- `tools/sandbox/suite.go`
- `tools/sandbox/suite_test.go`
