// doc.go carries the command doc for sandbox: what it runs and the two rules its suites and binary resolution keep.

// Sandbox clones a hub and runs agent-driven suites that exercise lym's modules end to end.
//
// # Suite coverage
//
// Every registered lym module has a scenario in a *SUITE.md suite file, declared with a **Covers:** tag.
// A module without one is named on the exclusion allowlist in cmd/lym/sandbox_coverage_test.go with a reason.
// Coverage is module-level, so an exclusion covers the whole module.
//
// # Dev and prod binaries
//
// Sandbox tooling resolves `lym` only through `resolveLym` in resolve.go.
// It picks the .dev-bin binary when present and falls back to PATH.
// No other file performs a bare-PATH `lym` lookup or spawn, which pathresolve_guard_test.go enforces.
package main
