// export_test.go re-exports checkResolved for package loomengine_test files that need to drive the
// preflight seam directly against an arbitrary *lyxcwd.Location — one built synthetically, with no
// backing directory on disk for lyxcwd.Resolve(cwd) to resolve — rather than through the exported
// Preflight(cwd string), which requires a real, resolvable cwd — the standard Go export_test.go
// idiom, per the export-test-shim decision.

package loomengine

// CheckResolvedForTest re-exports checkResolved for package loomengine_test files that need to run
// checks 1b-4 against a Location they built themselves, without needing a real directory on disk for
// Preflight's lyxcwd.Resolve(cwd) call to succeed against.
var CheckResolvedForTest = checkResolved
