// Package hubforge is the repo-wide real-hub fixture factory: it builds every hub fixture through
// fabriccli.CloneAndWire and never replicates that wiring by hand.
// It asserts nothing about fabric — which is why its name does not end in "test" —
// so its only exports are the Hub type with its geometry accessors and TestShortname, the Shape type with Shapes and TemplatePairSlug, the builders NewHub, CopyHub, SharedHub, AddPair, AddPairWith and OpenFabric, and the seeders SeedConfig and SeedFabricConfig.
//
// hubforge drives fabriccli.CloneAndWire rather than fabricengine.CloneHub because CloneHub alone
// yields a partial hub — warp clone, weft clone, board, anchor marker, warp binding, but no junctions
// and no repo-wide fabric.yaml — leaving three of the destruction gate's eight path-ownership kinds
// (ownedWiredJunction, ownedDriftedWiredJunction, ownedUnderGeometryRoot) structurally unreachable.
// CloneAndWire runs the whole wiring sequence, so a hub built here is real and fully wired, never a
// hand-assembled stand-in.
//
// The factory follows a copy-the-bares, clone-the-hub model: buildBareTemplate builds a warp/weft
// bare-repo pair once per test binary via sync.Once, copyBares copies that cached pair fresh into each
// scenario's own tb.TempDir(), and NewHub drives CloneAndWire against the copies.
// A hub itself cannot be copied the same way its bares are, because its junctions carry absolute
// targets that a plain directory copy would leave pointing at the wrong tree.
//
// The same template-and-copy model extends to the hub.
// A Shape is an anchor plus an ordered list of pair slugs, and Shapes is the closed set: the "." and "backend" anchors, each with no pair and with the one pair TemplatePairSlug.
// The first CopyHub or SharedHub of a shape builds its template once per test binary, through NewHub's CloneAndWire plus AddPair, in a directory whose name begins with templatePrefix so no copy path has the template root as a string prefix.
// The template directory holds the bares and the hub, and lives in the binary's temp directory, which tmuxkit.Main removes.
// CopyHub relocates the template into the test's own tb.TempDir: files are copied without following links, the template root path is rewritten to the copy's in every text file in both separator spellings, each link is recreated with its rewritten target, and the copy is verified to name the template root nowhere.
// A link keeps its one-hop target, so a chain of links stays a chain.
// A copy's Mutations are the template's, unrewritten.
// A shape outside the set is a tb.Fatalf pointing at NewHub, which stays for such shapes.
//
// SharedHub returns the template's own Hub for a test that only reads it, where a read is a file read, a link read or an in-process go-git read.
// A write is anything that may change a file under the hub: a fabric mutating verb, SeedConfig, AddPair, any run of the lyx binary, any in-process CLI verb and any spawned git subcommand.
// Both entry points compare the template's fingerprint, the digest of every regular file's relative path, size and modification time, with the one taken at build time.
// A mismatch, or a template already marked poisoned, fails the test naming the fixture.
// A SharedHub test's cleanup takes the fingerprint again, and on a change marks the template poisoned and fails naming the fixture; the blame may land on a parallel sharer.
//
// Seeding contract: SeedConfig overrides one or more modules' config, writing into the anchor-joined
// h.WeftBase and committing at the weft worktree root h.PrimeRecords().
// SeedFabricConfig overrides the repo-wide fabric.yaml, writing into h.BoardDir() and committing
// through fabricengine.NewBolt, matching what CloneAndWire itself does after ReconcileHubWideAt.
// Most former seeding sites need neither: fabriccli.CloneAndWire already runs
// configsync.ReconcileAll and ReconcileHubWideAt, so a real hub arrives with every registered module's
// default config already materialized and committed on the weft primary branch — which is why
// SeedConfig commits with an empty stage allowed: a seed byte-identical to the already-committed
// file stages nothing, and a bare commit over nothing would otherwise fail.
//
// Fixture marker: git a hub build spawns carries gitkit.FixtureGitEnv, set two ways.
// The local mustGit helper sets it on its own child's environment.
// NewHub, AddPairWith and buildBareTemplate also hold a mark on the process environment for their body, because production code spawns git there and no helper reaches it;
// the first concurrent build sets the variable and the last one to finish unsets it.
//
// Teardown contract: junctions are discovered by walking the hub root with fslink.IsLink, never by
// slug, and removed with fslink.Remove — a tb.Cleanup registered before tb.TempDir()'s own cleanup
// runs, so every junction is unwired before Go's os.RemoveAll ever walks into it.
//
// No package inside internal/fabriccli's dependency set may import hubforge — such tests use an
// external *_test package or gitkit instead.
// This is self-enforcing: the import would close a dependency cycle and fail to compile.
// See PATTERN-hubforge-fixtures for the machine-checked half of this
// contract.
package hubforge
