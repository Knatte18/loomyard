// Package testkit is the root of the shared test kits.
//
// Shared test support lives in its own package under internal/testkit/<kit>/, never under the package it fakes.
// It covers builders, fixtures, the scan harness and stand-ins used by two or more packages.
// PATTERN-test-stand-ins admits a stand-in only for something outside the repository;
// a kit that fakes a repository module predates that rule.
// A kit imports only the lowest packages defining the types it fakes.
// scankit imports the standard library only, so every package's tests can import it.
// lymbin, tmuxkit and llmkit are the kits exempt from the `os/exec` ban, each bounded in its own package doc.
// A fixture used by one package stays in that package's _test.go files.
// A package an import cycle bars from a kit keeps exactly one local copy.
//
// PATTERN-test-isolation states the rules; enforcement_test.go scans the import rules.
package testkit
