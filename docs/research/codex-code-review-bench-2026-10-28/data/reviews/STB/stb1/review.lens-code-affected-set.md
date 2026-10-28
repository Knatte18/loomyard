```yaml
lens: code-affected-set
outcome: reported
```

Affected-set review:

- `proc.IsSpawnBusy`: readers are `shedverbs.startChild`, `shedverbs.runChild`, the new loop test, and `proc/doc.go`; covered.
- `startChild` and retry constants: callers and readers are `runChild`, the loop test, and `shedverbs/doc.go`; covered.
- `LoopSpec.LoopTraceFile`: both production constructors, `loomcli.loopSpecFor` and `battencli.battenLoopSpec`, supply `logger.TraceFile`; test fixtures may omit the optional field; covered.
- Interrupted trace output: `runLoop` passes the selected path through `interruptedRecord`, the step envelope, `AfterInterrupt`, `loomAfterInterrupt`, and halt-note rendering; existing consumers accept the path unchanged; covered.
- Start-failure cause: tests and package documentation match the implementation; no parser or downstream consumer depends on the old cause prefix. The implementation differs from the decision writer’s requested exact format, but the board snapshot requires explanatory error detail rather than that exact spelling.
- Atomic installation: production and development launchers both reach `tools/deploy.run`, which calls `installBinary`; the Land row executes its configured deployment command. The command documentation and new installation test reflect the new behavior; covered.
- Repository constraints: package documentation accompanies the behavior change; no new module, command, or declared PATTERN amendment requires registration. The retry remains bounded and uses the existing sleep seam.

No findings from this lens.

Tool calls:
1. Searched changed identifiers, constructors, output strings, logger policy references, and deployment configuration references; the inherited manifest did not enumerate every reader.
2. Read logger level policy, downstream trace-envelope consumers, destination references, executable-path call sites, and platform-specific tied starts; their definitions were absent from the inherited excerpts.
