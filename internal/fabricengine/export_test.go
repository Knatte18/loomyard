// export_test.go re-exports newPaired and the now-private warp/weft fields for the handful of
// package fabricengine_test files that build fixtures from raw scratch paths no lyxcwd.Location
// describes, and so cannot use Open — the standard Go export_test.go idiom, per the
// export-test-shim decision.

package fabricengine

import "github.com/Knatte18/loomyard/internal/gitrepo"

// NewPairedForTest re-exports newPaired for package fabricengine_test files that construct a Fabric
// from raw warp/weft paths rather than a lyxcwd.Location.
var NewPairedForTest = newPaired

// WarpForTest returns f's private warp field, for package fabricengine_test files that need
// warp-side gitrepo.Repo access no other exported accessor provides.
func WarpForTest(f *Fabric) *gitrepo.Repo {
	return f.warp
}

// WeftForTest returns f's private weft field, for package fabricengine_test files that need
// weft-side gitrepo.Repo access no other exported accessor provides.
func WeftForTest(f *Fabric) *gitrepo.Repo {
	return f.weft
}

// WarpProbeDirPrefixForTest re-exports warpProbeDirPrefix for package fabricengine_test files that
// assert on the probe's throwaway-clone directory naming without duplicating the literal.
const WarpProbeDirPrefixForTest = warpProbeDirPrefix

// ExcludePatternForTest re-exports excludePatternFor for package fabricengine_test files that
// assert on the anchored .git/info/exclude pattern without duplicating its spelling.
var ExcludePatternForTest = excludePatternFor
