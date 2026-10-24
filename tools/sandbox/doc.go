// doc.go carries the command doc for sandbox: what it runs and the two rules its suites and binary resolution keep.

// Sandbox clones a hub and runs agent-driven suites that exercise lyx's modules end to end.
//
// # Suite coverage
//
// Every registered lyx module is either exercised by a scenario in one of the *SUITE.md suite files, declared with a **Covers:** tag, or named on cmd/lyx/sandbox_coverage_test.go's exclusion allowlist with a reason.
// Coverage is module-level, so an exclusion covers the whole module.
//
// # Dev and prod binaries
//
// Sandbox tooling resolves `lyx` only through resolveLyx in resolve.go, which picks the .dev-bin binary when present and falls back to PATH;
// no other file performs a bare-PATH `lyx` lookup or spawn, which pathresolve_guard_test.go enforces.
package main
