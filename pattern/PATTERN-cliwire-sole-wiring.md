# PATTERN-cliwire-sole-wiring

`internal/cliwire` is the sole owner of standalone and hub CLI wiring resolution for the standalone-capable CLIs.

- A `<module>cli` never re-implements `--target-dir` resolution, the repository-root lift, mode-derived state, plan and stencils resolution, the nested-geometry guard, or the durable-sink redirect.
  It declares its own `cliwire.Module` descriptor and calls in.
- `internal/cliwire` is the only production caller of `standalonestate.Derive`; test files may call it to build fixtures and to assert the real derivation.
- Both halves are enforced by tests in `internal/cliwire`.
