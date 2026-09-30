// buildidentity.go names the build a `lyx` binary was made from, so a step's record can say which
// build ran it and status can tell when a different build is running now.

package shedverbs

import "runtime/debug"

// BuildIdentity is the VCS stamp of a lyx build. An empty Revision means the binary carries no
// VCS stamp, which makes the identity unknown.
type BuildIdentity struct {
	Revision string `json:"vcs_revision"`
	Modified bool   `json:"vcs_modified"`
}

// runningBuildIdentity reads the running binary's VCS stamp, and returns the zero value when build
// info is unavailable. It lives here rather than in internal/buildinfo, which stays import-free.
func runningBuildIdentity() BuildIdentity {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildIdentity{}
	}
	var id BuildIdentity
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			id.Revision = s.Value
		case "vcs.modified":
			id.Modified = s.Value == "true"
		}
	}
	return id
}

// binaryChanged reports whether running is a different known build than recorded: both revisions
// must be non-empty, and the pairs must differ in either field.
func binaryChanged(running, recorded BuildIdentity) bool {
	if running.Revision == "" || recorded.Revision == "" {
		return false
	}
	return running != recorded
}
