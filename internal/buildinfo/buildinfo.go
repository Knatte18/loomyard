// buildinfo.go declares the ldflags-stamped build-channel variable and the exact-match accessors that read it.

package buildinfo

const (
	// ChannelProduction is the Channel value tools/deploy stamps on the production route.
	ChannelProduction = "production"
	// ChannelDev is the Channel value tools/deploy -dev stamps.
	ChannelDev = "dev"
)

// Channel is set by tools/deploy through the linker's -X flag, to ChannelDev under -dev and to ChannelProduction on the production route.
// An unstamped binary -- a plain `go build`, a `go install`, or a `go test` binary -- leaves it empty, and empty is neither dev nor production.
var Channel string

// IsDev reports whether Channel is exactly ChannelDev -- an exact comparison, never a prefix match and never case-insensitive.
func IsDev() bool {
	return Channel == ChannelDev
}

// IsProduction reports whether Channel is exactly ChannelProduction -- an exact comparison, never a prefix match and never case-insensitive.
func IsProduction() bool {
	return Channel == ChannelProduction
}
