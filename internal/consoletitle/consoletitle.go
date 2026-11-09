// consoletitle.go joins the parts of a console window's title.
// It also names the labels that stand in a title in place of a slug.

package consoletitle

import "strings"

// separator joins the parts of a title.
const separator = ": "

// workspaceSuffix follows the name a workspace file's title is named after.
const workspaceSuffix = " workspace"

// Labels that stand as a title's name in place of a slug.
const (
	// PrimeLabel names the prime's own VS Code window.
	PrimeLabel = "prime"
	// BeamLabel names a beam's own VS Code window, after the beam's handle.
	BeamLabel = "beam"
	// OrchLabel names a terminal window on the reed session an orch runs in, the prime's or a beam's.
	OrchLabel = "orch"
)

// Title returns a console window's title: shortname lower-cased, projectHandle, ownHandle and name, joined by ": ".
// A part that is empty is left out, so a task with no project reads `ly: T12: adopt-fix-root-t1`.
func Title(shortname, projectHandle, ownHandle, name string) string {
	var parts []string
	for _, part := range []string{strings.ToLower(shortname), projectHandle, ownHandle, name} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, separator)
}

// Workspace returns the title of a VS Code workspace file named after name, a repository or a beam:
// `<shortname>: <name> workspace`, with an empty shortname left out.
func Workspace(shortname, name string) string {
	return Title(shortname, "", "", name+workspaceSuffix)
}
