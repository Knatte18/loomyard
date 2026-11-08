// Package impactset derives the packages a `lym gate test` run has to cover.
// It reads the diff between a base commit and the working tree, for a computed, `--base` or round scope.
//
// DeriveTargets is the `go` scoper of `lym gate test`.
// It takes a context, the worktree and the base, and resolves no cwd.
// Git runs through `gitexec.Run`, and `go list` is a logged spawn.
// It diffs the base to the working tree, not to HEAD:
// tracked changes, staged or not, a rename counted at both paths, and the untracked files git does not ignore.
// It returns Targets{Whole, Fallback, Base, Modules, Guards, BinaryTests}.
// Each ModuleTargets names a module directory, `.` for the root,
// and its module-relative package arguments in the order the graph lists them, test-less packages included.
// Targets also names the base it diffed from whenever that base was usable.
//
// ChangedPaths and AncestorBase are the diff reads DeriveTargets starts with, exported for other scoping code.
// ChangedPaths returns the paths that differ between a base and the working tree.
// AncestorBase resolves a base to a commit and reports whether it is an ancestor of HEAD.
//
// DeriveTargets falls back to Whole, with the reason, when any of these holds:
// there is no `go.mod` at the worktree root;
// the base is empty, or is not an ancestor of HEAD;
// `go.mod` or `go.sum` changed since the base;
// a changed file, by its new or its old path, is neither `.md` nor under a Go package directory;
// or `go list` fails.
// A deleted file whose directory is not a package any more counts as a file outside a Go package directory.
// So does a file of a nested module, because the root `go list` does not list it.
//
// Otherwise the set is the packages the diff changed.
// It adds every package whose dependencies reach one of them, test imports included.
// The graph is built under the tier tags `tier0` through `tier3`.
// A rename or deletion maps through its old path as well as its new one.
// A change to the `cmd/lym` main package is invisible to the graph, because no test imports it.
// So the set also gets every package whose tests import `internal/testkit/lymbin`, which builds the binary;
// any other change reaches those packages through the import graph only.
// Targets.BinaryTests lists those packages, inside the set or not.
// A diff touching only Markdown files yields no module and no package.
// The context is checked before each git read, and cancelling it kills a stalled `go list`;
// a local git read already in flight runs to its end.
//
// Every top-level test marked `//lym:guard` runs by name in its own package, even outside the set.
// A repository-scanning test can fail on a change far from its package.
// Guards holds, per directory, the `//lym:guard` tests of the packages outside the set, sorted by name.
// The gate's guard step runs them under the command of each test's tier.
// Each GuardTest carries the tier of its file.
// FileTier reads it from the `tiergate.Require` call opening the file's test functions (RequiredTier reads one).
// For a file making no such call, the tier is the N of the `tierN` tag in its `//go:build` line.
// The tier is 0 when the line names none.
//
// A static parse of every `_test.go` file under every tag finds the markers.
// FindGuardTests exports that scan for callers that enumerate the package directories themselves.
// ModuleGuardTests runs the same scan under every tier tag.
// It covers every package directory a `go list -e ./...` of a module names.
// A package whose test file cannot be read stays listed, so the scan reports that file.
// An unreadable package directory or a failed `go list` is an error naming the module and the enumeration's error.
// A marker in a `tier2` or `tier3` file is an error naming the file and line.
// So is a marker not directly above a top-level `func Test…` line.
// That error is a *GuardScanError.
// Its Kind is GuardMisplaced, GuardWrongTier, GuardParse, SeamMalformed or WatchMalformed.
// Its Line is the marker's line, or the parser's for GuardParse when it reports one.
// A file that cannot be parsed or read is a GuardParse error.
// Another `//lym:` line or a `//testtiming:keep` line between the marker and the func line does not displace the marker.
// A test carrying several markers stacks them as contiguous directive lines.
// DeriveTargets returns a guard or watch scan failure as the *GuardScanError.
//
// A top-level test marked `//lym:watch <pattern>...` guards only the files its patterns name, such as a cross-compile.
// A narrowed set runs it only when a changed path matches one of its `path.Match` patterns or its own file changed.
// A pattern holding a `/` matches a worktree-relative path, and any other matches a path's base name.
// Targets.Unwatched names, per package inside the set, the watched tests the diff leaves alone.
// Guards leaves those tests out.
// A whole run still runs them.
// FindWatchedTests is the static parse.
// UnwatchedTests matches its result against a list of changed paths, for a scope that reads them itself.
// A watch line outside the comment block of a top-level `func Test…` is a GuardMisplaced error.
// A watch line naming no pattern, or a pattern `path.Match` rejects, is a WatchMalformed error.
//
// FindSeamTests is the same static parse for `//lym:seam <unit>#<Type>`.
// That marker belongs to a test that reflects over a struct's fields.
// It returns, per directory, the SeamTest values of the tests carrying at least one such line:
// the test's name, its glyph unit, its file and the marked glyphs in comment order.
// A seam line outside the comment block of a top-level `func Test…` is a GuardMisplaced error.
// A seam line whose argument is not a single `<unit>#<Type>` token is a SeamMalformed error.
// Whether a glyph names a struct is for the caller to judge.
//
// ModuleMainPackages lists the `package main` packages of a module through `go list -e ./...`.
// It returns them sorted, as module-relative package arguments.
// A program's own tests are where a change to the packages it wires shows, and no other package imports a main package.
//
// HoldsTierTest reports whether a package directory holds a test function a run of given tiers executes.
// A test function opening with `tiergate.Require(t, N)` counts for tier N alone, and any other for every tier.
// The gate's `{test_targets}` keeps a package on it, so a tier run starts only the test binaries with a test to run.
//
// A narrower run skips only packages the import graph shows unreachable.
// Whole runs catch what the graph cannot see, such as a test reading another package's files.
// A binary-driving test can reach a change only through a subcommand its own package does not import.
// A narrowed run omits that test, and a whole run still runs it.
package impactset
