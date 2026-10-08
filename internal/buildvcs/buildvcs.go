// buildvcs.go reads the VCS stamp the Go toolchain embeds in a lyx binary.

package buildvcs

import "runtime/debug"

// Identity is the VCS stamp of a lyx build.
// An empty Revision means the binary carries no VCS stamp, which makes the identity unknown.
type Identity struct {
	Revision string `json:"vcs_revision"`
	Modified bool   `json:"vcs_modified"`
}

// Running reads the running binary's VCS stamp, and returns the zero value when build info is unavailable.
func Running() Identity {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Identity{}
	}
	return identityOf(info.Settings)
}

// identityOf maps the vcs.revision and vcs.modified build settings onto an Identity and ignores every other key.
func identityOf(settings []debug.BuildSetting) Identity {
	var id Identity
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			id.Revision = s.Value
		case "vcs.modified":
			id.Modified = s.Value == "true"
		}
	}
	return id
}
