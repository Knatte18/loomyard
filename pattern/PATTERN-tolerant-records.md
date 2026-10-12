# PATTERN-tolerant-records

No reader of a Go-written JSON record refuses an unknown key, and a rewrite keeps the keys it does not know.
A long-lived process on an older binary then neither halts on, nor drops, a key a newer binary wrote.

- A rewrite goes through `internal/state`, which keeps the keys its type does not know; its package doc holds the merge rules and their bound.
  A type that keeps them itself is the other way, as `board.json`'s `storeFile` and `Task` do.
- A read-only decode outside `internal/state`, such as batten cleanup's judge of committed bytes, is allowed, since it rewrites nothing.
- Agent-written contract files stay strict, since no binary rewrites them: webster's `outcome.yaml` and `report.yaml`, the arbiter's `verdict.yaml`, plan and recipe files.
- The scan covers only `encoding/json`'s `DisallowUnknownFields`; `yaml.v3`'s `KnownFields` is outside it.
  A future agent-written JSON contract that needs strictness validates its keys some other way.

## Enforcement

`cmd/lyx/tolerantrecords_test.go` parses every production `.go` file and fails, naming file and line, on a call expression whose selector is `DisallowUnknownFields`.
A comment or a string naming the method is never a finding.
