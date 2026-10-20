// Package impactset derives the command that loom's round gate and darn's two writer gates run from the diff between a base commit and HEAD.
//
// Derive is told the worktree and the base and resolves no cwd.
// Git runs through `gitexec.Run`, and `go list` is a logged spawn.
// It returns a Derivation carrying either the derived command or a fallback reason that tells the caller to run the full plan verify instead.
// The Derivation also names the base it diffed from whenever that base was usable, and the impacted set's package arguments in Packages, empty with a fallback or when only non-Go files changed.
//
// Derive falls back when any of these holds:
// there is no `go.mod` at the worktree root;
// the base is empty, or is not an ancestor of HEAD;
// `go.mod` or `go.sum` changed since the base;
// a changed file, by its new or its old path, is neither `.md` nor under a Go package directory, so a deleted file whose directory is no longer a package counts;
// `go list` fails;
// or the derived command is longer than a command line safely holds.
//
// Otherwise the command covers the impacted set:
// the packages the diff changed, plus every package whose dependencies, test imports included, reach one of them, under the `integration`, `tmux` and `llm` tags.
// A rename or deletion maps through its old path as well as its new one.
// When a changed package is a dependency of `cmd/lyx`, every package whose tests import `internal/testkit/lyxbin`, which builds the binary, joins the set.
//
// Every top-level test marked `//lyx:guard` runs by name in its own package, even when that package is outside the set.
// A repository-scanning test can fail on a change far from its package.
// A marker is found by a static parse of every `_test.go` file under every tag.
// The marker scan is exported as FindGuardTests, and webster's card gate runs it over its own walk of the root module.
// ModuleGuardTests runs the same scan over every package directory a `go list -e ./...` of a module names, under the three tags.
// A package whose test file cannot be read stays listed, so the scan reports that file; an unreadable package directory or a failed `go list` is a plain error naming the module and the enumeration's error.
// A marker not directly above a top-level `func Test…` line, or in a file the `integration` tier never compiles, is an error naming the file and line.
// That error is a *GuardScanError whose Kind is GuardMisplaced, GuardNotIntegration or GuardParse, and whose Line is the marker's line, or the parser's for GuardParse when it reports one.
// A file that cannot be parsed or read is a GuardParse error.
// A `//testtiming:keep` line between the marker and the func line does not displace the marker, since a test carrying both stacks them as contiguous directive lines.
//
// The command is `go build ./...`, then `go vet` over the set under each of the three tags, then `lyx gate test --tags integration` over the set, which covers every untagged test too.
// The guard tests of the packages outside the set follow as one step, `lyx gate test --tags integration <packages> -- -run '^(<names>)$'`, the packages in sorted directory order and the names sorted and deduplicated, all joined by `&&`.
// The test steps go through `lyx gate test` so that inside a verify they inherit the held gate slot and share one prebuilt `lyx`; the guard step is a single call so `lyx` is built once.
// An empty set runs only the build and the guard step.
//
// Bound: the narrower command skips only packages the import graph shows unreachable.
// What the graph cannot see, such as a test reading another package's files, is caught by the full plan verify at the sites that keep it.
package impactset
