// Package testkit is the root of the shared test kits.
//
// A shared test kit lives in its own package under internal/testkit/<kit>/, never under the package it fakes.
// A kit imports only the lowest packages defining the types it fakes.
// A fixture used by one package stays in that package's _test.go files.
// A package an import cycle bars from a kit keeps exactly one local copy.
//
// The Testkit Invariant in CONSTRAINTS.md states the rules; enforcement_test.go scans the import rules.
package testkit
