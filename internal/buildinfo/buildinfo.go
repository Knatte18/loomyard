// buildinfo.go declares the ldflags-stamped build-channel variable and the exact-match accessor
// that reads it.

package buildinfo

// Channel is set by tools/deploy -dev via
// -ldflags "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev".
// An unstamped binary -- a plain `go build`, a `go install`, or a `go test` binary -- leaves it
// empty, and empty means production.
// Production is the conservative default because it keeps shipped defaults converging;
// dev is the exception and must opt in explicitly.
var Channel string

// IsDev reports whether Channel is exactly "dev" -- an exact comparison, never a prefix match and
// never case-insensitive.
func IsDev() bool {
	return Channel == "dev"
}
