// consoletitle.go joins the parts of a console window's title.
// It also names the lead that opens an orch's window title.

package consoletitle

import "strings"

// partSeparator joins a title's lead, its bracketed place and its name.
const partSeparator = " "

// placeSeparator joins the hub shortname and the project handle inside a title's brackets.
const placeSeparator = ":"

// workspaceSuffix follows the name a workspace file's title is named after.
const workspaceSuffix = " workspace"

// OrchLead opens the title of an orch's window, where a task's handle opens a task's.
const OrchLead = "ORCH"

// Title returns a console window's title, `<lead> [<shortname>:<projectHandle>] <name>`.
// An example is `T2 [ly:P1] adopt-worker-record`.
// The shortname is lower-cased.
// An empty part is left out with its separator,
// and the brackets are left out when both shortname and projectHandle are empty.
func Title(lead, shortname, projectHandle, name string) string {
	var place []string
	for _, part := range []string{strings.ToLower(shortname), projectHandle} {
		if part != "" {
			place = append(place, part)
		}
	}
	var parts []string
	if lead != "" {
		parts = append(parts, lead)
	}
	if len(place) > 0 {
		parts = append(parts, "["+strings.Join(place, placeSeparator)+"]")
	}
	if name != "" {
		parts = append(parts, name)
	}
	return strings.Join(parts, partSeparator)
}

// Workspace returns the title of a VS Code workspace file named after name, a repository or a beam:
// `[<shortname>] <name> workspace`, with the brackets left out when shortname is empty.
func Workspace(shortname, name string) string {
	return Title("", shortname, "", name+workspaceSuffix)
}
