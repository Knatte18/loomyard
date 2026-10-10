// buildvcs.go reads the VCS stamp the Go toolchain embeds in a lyx binary.

package buildvcs

import (
	"runtime/debug"
	"time"
)

// shortRevisionLength is how many leading characters of a revision a commit message and a log line carry.
const shortRevisionLength = 12

// Identity is the VCS stamp of a lyx build.
// An empty Revision means the binary carries no VCS stamp, which makes the identity unknown.
// A zero Time means the build recorded no parseable commit time.
type Identity struct {
	Revision string    `json:"vcs_revision"`
	Modified bool      `json:"vcs_modified"`
	Time     time.Time `json:"vcs_time"`
}

// Clean reports a stamped, unmodified build: a non-empty Revision with Modified false.
func (id Identity) Clean() bool {
	return id.Revision != "" && !id.Modified
}

// Label renders the identity for a commit message and a log line:
// the first shortRevisionLength characters of the revision, `unknown` for an empty one, and a `-modified` suffix for a modified build.
func (id Identity) Label() string {
	label := "unknown"
	if id.Revision != "" {
		label = id.Revision
		if len(label) > shortRevisionLength {
			label = label[:shortRevisionLength]
		}
	}
	if id.Modified {
		label += "-modified"
	}
	return label
}

// Running reads the running binary's VCS stamp, and returns the zero value when build info is unavailable.
func Running() Identity {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Identity{}
	}
	return identityOf(info.Settings)
}

// identityOf maps the vcs.revision, vcs.modified and vcs.time build settings onto an Identity and ignores every other key.
// A vcs.time that is not RFC 3339 leaves Time zero.
func identityOf(settings []debug.BuildSetting) Identity {
	var id Identity
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			id.Revision = s.Value
		case "vcs.modified":
			id.Modified = s.Value == "true"
		case "vcs.time":
			if parsed, err := time.Parse(time.RFC3339, s.Value); err == nil {
				id.Time = parsed
			}
		}
	}
	return id
}
