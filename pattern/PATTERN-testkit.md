# PATTERN-testkit

Shared test support — fakes, builders, fixtures and the scan harness — used by two or more packages is shared through one kit package under `internal/testkit/<kit>/`, never duplicated per package and never placed under the package it fakes.

- No non-test file outside `internal/testkit/` imports a path under it, so every kit is reachable only from tests.
  Files under `internal/testkit/` are exempt, so a kit may build on another kit.
- No non-test file under `internal/testkit/` imports an `internal/*cli` package.
- No non-test file under `internal/testkit/` imports `os/exec`, `internal/gitexec`, `internal/gitkit`, `internal/hubforge`, `internal/testkit/lyxbin`, `internal/testkit/tmuxkit` or `internal/testkit/llmkit`.
  - `internal/testkit/lyxbin` is exempt from the `os/exec` ban alone, bounded to `go build` of `./cmd/lyx`.
    Banning its import keeps the kit-on-kit exemption from handing another kit a transitive `go build`.
  - `internal/testkit/tmuxkit` is the second exemption from the `os/exec` ban alone, bounded to running the `tmux` binary against sockets under its own directory or its own fixture keys.
    No other kit imports it, for the same reason.
  - `internal/testkit/llmkit` is the third exemption from the `os/exec` ban alone, bounded to `exec.LookPath` for the `claude` binary; it starts nothing.
    Only `llm`-tagged test files import it, and no other kit does, for the same reason.
- A kit imports only the lowest packages defining the types it fakes.
  A package an import cycle bars from a kit keeps exactly one local copy, and a fixture used by one package stays in that package's `_test.go` files.
- `internal/testkit/scankit` imports the standard library only, so every package's tests can import it.

## Enforcement

`internal/testkit/enforcement_test.go` enforces the import rules.
Review discipline covers the rest: a kit starts no process, tmux server or agent beyond what its imports allow.
A kit asserts nothing beyond `t.Fatalf` on its own setup, the `shedfake` `Call` and `RequireOutcome` outcome check, and the `envelope` `RequireOK` and `RequireErr` shape check.
`scankit` additionally asserts scan results, while `plankit` and `stencilkit` assert nothing beyond `t.Fatalf` on their own setup.
