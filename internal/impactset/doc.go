// Package impactset derives the command a Webster-Burler round gate runs from the diff between a base commit and HEAD.
//
// Derive is told the worktree and the base and resolves no cwd.
// Git runs through `gitexec.Run`, and `go list` is a logged spawn.
// It returns a Derivation carrying either the derived command or a fallback reason that tells the caller to run the full plan verify instead.
// The Derivation also names the base it diffed from whenever that base was usable.
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
// A marker not directly above a top-level `func Test…` line, or in a file the `integration` tier never compiles, is an error naming the file and line.
// A `//testtiming:keep` line between the marker and the func line does not displace the marker, since a test carrying both stacks them as contiguous directive lines.
//
// The command is `go build ./...`, then `go vet` over the set under each of the three tags, then `go test` over the set, then `go test -tags integration` over the set.
// The guard tests of each package outside the set follow, all joined by `&&`.
// An empty set runs only the build and the guard tests.
//
// Bound: the narrower command skips only packages the import graph shows unreachable.
// What the graph cannot see, such as a test reading another package's files, is caught by the full plan verify at the sites that keep it.
package impactset
