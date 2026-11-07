// Package hubforge is the repo-wide real-hub fixture factory:
// it builds every hub fixture through fabriccli.CloneAndWire and never replicates that wiring by hand.
// It asserts nothing about fabric, which is why its name does not end in "test".
// Its exports are Hub with its geometry accessors, the hub builders, the config seeders, Shape and TestShortname.
//
// hubforge drives fabriccli.CloneAndWire, not fabricengine.CloneHub, because CloneHub alone yields a partial hub.
// That hub has the warp clone, weft clone, board, anchor marker and warp binding.
// It has no junctions and no repo-wide fabric.yaml.
// That leaves these destruction-gate path-ownership kinds structurally unreachable:
// ownedWiredJunction, ownedDriftedWiredJunction and ownedUnderGeometryRoot.
// CloneAndWire runs the whole wiring sequence.
// A hub built here is therefore real and fully wired, never a hand-assembled stand-in.
//
// The factory follows a copy-the-bares, clone-the-hub model.
// buildBareTemplate builds a warp/weft bare-repo pair once per test binary via sync.Once.
// copyBares copies that cached pair fresh into each scenario's own tb.TempDir().
// NewHub drives CloneAndWire against the copies.
// A hub itself cannot be copied the same way its bares are:
// its junctions carry absolute targets that a plain directory copy would leave pointing at the wrong tree.
//
// The same template-and-copy model extends to the hub.
// A Shape is an anchor plus an ordered list of pair slugs, and Shapes is the closed set:
// the "." and "backend" anchors, each with no pair and with the one pair TemplatePairSlug.
// The first CopyHub or SharedHub of a shape builds its template once per test binary.
// The build runs NewHub's CloneAndWire plus AddPair.
// The template directory's name begins with templatePrefix.
// That keeps the template root from being a string prefix of any copy path.
// The template directory holds the bares and the hub.
// It lives in the process run directory of internal/lymtemp, which tmuxkit.Main removes.
// CopyHub relocates the template into the test's own tb.TempDir.
// Files are copied without following links.
// The template root path is rewritten to the copy's in every text file, in both separator spellings.
// Each link is recreated with its rewritten target.
// The copy is verified to name the template root nowhere.
// The board's tracked `.lym-warp` binding is among the rewritten files.
// CopyHub therefore amends it into the board's one commit and force-pushes that to the copy's own records bare.
// That leaves every checkout of the copy clean, as a fresh hub's is.
// A link keeps its one-hop target, so a chain of links stays a chain.
// A copy's Mutations are the template's, unrewritten.
// A shape outside the set fails through tb.Fatalf, whose message points at NewHub for building such shapes.
//
// SharedHub returns the template's own Hub for a test that only reads it.
// A read is a file read, a link read or an in-process go-git read.
// A write is anything that may change a file under the hub.
// Writes include a fabric mutating verb, SeedConfig and AddPair.
// They also include any run of the lym binary, any in-process CLI verb and any spawned git subcommand.
// Both entry points compare the template's fingerprint with the one taken at build time.
// The fingerprint is the digest of every regular file's relative path, size and modification time.
// A mismatch, or a template already marked poisoned, fails the test naming the fixture.
// A SharedHub test's cleanup takes the fingerprint again.
// On a change it marks the template poisoned and fails naming the fixture; the blame may land on a parallel sharer.
//
// Seeding contract: SeedConfig overrides one or more modules' config.
// It writes into the anchor-joined h.WeftBase and commits at the weft worktree root h.PrimeRecords().
// SeedFabricConfig overrides the repo-wide fabric.yaml.
// It writes into the Bolt checkout and commits through fabricengine.NewBolt.
// CloneAndWire does the same after ReconcileHubWideAt.
// Most tests need neither: fabriccli.CloneAndWire already ends in the hub walk.
// A real hub therefore arrives with each per-worktree module's repo default committed on the board.
// Its prime carries only the per-worktree config files the records repository already held.
// The clone's walk also migrates the prime, so its prime arrives with the sparse marker committed.
// SeedConfig writes a sparse worktree layer over those defaults.
// SeedConfig commits with an empty stage allowed:
// a seed byte-identical to a file already committed stages nothing.
// A bare commit over nothing would otherwise fail.
// cloneHub removes the build stamp the clone's walk wrote.
// A start verb driven in a test therefore walks the hub as on a binary change, while the seeded defaults stay committed.
//
// Fixture marker: git a hub build spawns carries gitkit.FixtureGitEnv, set two ways.
// The local mustGit helper sets it on its own child's environment.
// NewHub, AddPairWith and buildBareTemplate also hold a mark on the process environment for their body.
// Production code spawns git there and no helper reaches it.
// The first concurrent build sets the variable and the last one to finish unsets it.
//
// Teardown contract: junctions are discovered by walking the hub root with fslink.IsLink, never by slug.
// They are removed with fslink.Remove in a tb.Cleanup registered before tb.TempDir()'s own cleanup runs.
// Every junction is therefore unwired before Go's os.RemoveAll walks into it.
//
// No package inside internal/fabriccli's dependency set may import hubforge:
// such tests use an external *_test package or gitkit instead.
// This is self-enforcing: the import would close a dependency cycle and fail to compile.
// See PATTERN-test-isolation for the machine-checked half of this contract.
package hubforge
