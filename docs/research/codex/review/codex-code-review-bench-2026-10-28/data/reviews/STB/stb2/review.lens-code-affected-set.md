```yaml
lens: code-affected-set
outcome: reported
```

Affected set checked:

- `IsSpawnBusy`: `startChild`, `runChild`, retry test and `proc/doc.go` are covered; existing EAGAIN predicate remains independent.
- `LoopTraceFile`: both production constructors (`loomcli.loopSpecFor`, `battencli.battenLoopSpec`) supply `logger.TraceFile`; the loop consumes it conditionally; other constructors are tests with an optional nil field.
- `start-failed` cause: loop tests, package docs, halt-hook propagation and landing summary agree with the implementation’s spelling. The exact alternative spelling in the unamended decision record is not the board snapshot’s requirement.
- Atomic install: production/dev shell and Windows launchers all invoke `tools/deploy`; destination resolution still supplies the same directory. Land dispatches the configured command rather than maintaining another binary-install implementation.
- PATTERN/docs: package documentation accompanies both modified modules; no new CLI subtree, module, config or state-path vocabulary was introduced. Retry logging conflicts with the episode rule below.

Finding:

- **BLOCKING · design · `internal/shedverbs/loopchild.go:63` — Repeated ETXTBSY failures emit Warn repeatedly within one failure episode.** `PATTERN-spawn-observability.md`, “Log levels,” requires a repeating failure to log Warn once per episode, and `internal/logger/logger.go:141–142` explicitly requires subsequent occurrences to use Debug until recovery. `startChild` emits the same failure at Warn on all four retry iterations without recovery. The five-attempt bound satisfies the separate count requirement but does not exempt the episode rule. Keep the first failure at Warn and subsequent same-episode failures at Debug (or formally change the invariant); the decision record’s request for Warn on every retry is not an `Added after Discussion` amendment and cannot override the supplied constraints.

Additional tool calls:

1. Read arming diffs, Land/deploy references, `devbin` and changed-output references because the inherited manifest did not establish all launcher and destination readers.
2. Read logger policy locations, tied platform implementations, trace listing and Land dispatch because logger’s full level policy and platform call paths were missing from inherited output.
3. Read the complete logger level policy, Windows launchers and final constructor/reference search because earlier output was truncated and Windows launcher coverage had not been established.
