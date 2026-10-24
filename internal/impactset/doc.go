// Package impactset derives the packages a `lyx gate test` run over a computed, `--base` or round scope has to cover, from the diff between a base commit and the working tree.
//
// DeriveTargets is the `go` scoper of `lyx gate test`.
// It is told a context, the worktree and the base and resolves no cwd.
// Git runs through `gitexec.Run`, and `go list` is a logged spawn.
// It diffs the base to the working tree, not to HEAD:
// tracked changes, staged or not, a rename counted at both paths, and the untracked files git does not ignore.
// It returns Targets{Whole, Fallback, Base, Modules, Guards, BinaryTests}: each ModuleTargets names a module directory, `.` for the root, and its module-relative package arguments in the order the graph lists them, every impacted package kept, test-less ones included.
// Targets also names the base it diffed from whenever that base was usable.
//
// DeriveTargets falls back to Whole, with the reason, when any of these holds:
// there is no `go.mod` at the worktree root;
// the base is empty, or is not an ancestor of HEAD;
// `go.mod` or `go.sum` changed since the base;
// a changed file, by its new or its old path, is neither `.md` nor under a Go package directory, so a deleted file whose directory is no longer a package counts, and so does a file of a nested module, because the root `go list` does not list it;
// or `go list` fails.
//
// Otherwise the set is the packages the diff changed, plus every package whose dependencies, test imports included, reach one of them, under the tier tags `tier0` through `tier3`.
// A rename or deletion maps through its old path as well as its new one.
// When the diff changes the `cmd/lyx` main package, which no test imports and so the graph cannot see, every package whose tests import `internal/testkit/lyxbin`, which builds the binary, joins the set;
// any other change reaches those packages through the import graph only.
// Targets.BinaryTests lists those packages, inside the set or not.
// A diff touching only Markdown files yields no module and no package.
// The context bounds the derivation: it is checked before each git read, and a cancelled context kills a stalled `go list`, while a local git read already in flight runs to its end.
//
// Every top-level test marked `//lyx:guard` runs by name in its own package, even when that package is outside the set.
// A repository-scanning test can fail on a change far from its package.
// Guards holds, per directory, the `//lyx:guard` tests of the packages outside the set, sorted by name, so the gate's guard step runs them by name.
// Each GuardTest carries the tier of its file, which FileTier reads from the `//go:build` line: the N of the `tierN` tag it names, and 0 for a file naming none.
// The guard step runs each tier's guard tests under that tier's command.
// A marker is found by a static parse of every `_test.go` file under every tag.
// The marker scan is exported as FindGuardTests, and webster's card gate runs it over its own walk of the root module.
// ModuleGuardTests runs the same scan over every package directory a `go list -e ./...` of a module names, under every tier tag.
// A package whose test file cannot be read stays listed, so the scan reports that file; an unreadable package directory or a failed `go list` is a plain error naming the module and the enumeration's error.
// A marker not directly above a top-level `func Test…` line, or in a `tier2` or `tier3` file, is an error naming the file and line.
// That error is a *GuardScanError whose Kind is GuardMisplaced, GuardWrongTier or GuardParse, and whose Line is the marker's line, or the parser's for GuardParse when it reports one.
// A file that cannot be parsed or read is a GuardParse error.
// A `//testtiming:keep` line between the marker and the func line does not displace the marker, since a test carrying both stacks them as contiguous directive lines.
// A guard scan failure from DeriveTargets is returned as the *GuardScanError.
//
// Bound: a narrower run skips only packages the import graph shows unreachable.
// What the graph cannot see, such as a test reading another package's files, is caught by the whole runs at the sites that keep them.
// A binary-driving test that reaches a change only through a subcommand its own package does not import leaves the narrowed run, and the whole runs at Merge-In, Publish and Finalize still run it.
package impactset
