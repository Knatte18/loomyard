// Package testkit is the root of the shared test kits.
//
// Shared test support — fakes, builders, fixtures and the scan harness — used by two or more packages lives in its own package under internal/testkit/<kit>/, never under the package it fakes.
// A kit imports only the lowest packages defining the types it fakes.
// scankit imports the standard library only, so every package's tests can import it.
// lyxbin and tmuxkit are the two kits exempt from the `os/exec` ban, each bounded in its own package doc.
// A fixture used by one package stays in that package's _test.go files.
// A package an import cycle bars from a kit keeps exactly one local copy.
//
// The Testkit Invariant in CONSTRAINTS.md states the rules; enforcement_test.go scans the import rules.
package testkit
