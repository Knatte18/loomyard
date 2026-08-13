// Package hubforge is the repo-wide real-hub fixture factory: it builds every hub fixture through
// fabriccli.CloneAndWire and never replicates that wiring by hand.
// It asserts nothing about fabric — which is why its name does not end in "test" — so its only
// exports are the Hub type, its geometry accessors, and the two builders NewHub and AddPair.
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
// No package inside internal/fabriccli's dependency set may import hubforge — such tests use an
// external *_test package or gitkit instead.
// This is self-enforcing: the import would close a dependency cycle and fail to compile.
// See CONSTRAINTS.md's hubforge Fabric-Fixture Invariant for the machine-checked half of this
// contract.
package hubforge
