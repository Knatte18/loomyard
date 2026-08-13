// export_test.go re-exports checkResolved for package loomengine_test files that need to drive the
// preflight seam directly against an arbitrary *lyxcwd.Location, rather than through the exported
// Preflight(), whose own lyxcwd.Getwd() dependency makes it unusable against an arbitrary Location —
// the standard Go export_test.go idiom, per the export-test-shim decision.

package loomengine

// CheckResolvedForTest re-exports checkResolved for package loomengine_test files that need to run
// checks 1b-4 against a Location they built themselves, without also exercising Preflight's own
// lyxcwd.Getwd()-based resolution of the process cwd.
var CheckResolvedForTest = checkResolved
