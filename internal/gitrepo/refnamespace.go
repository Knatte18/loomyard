// refnamespace.go defines RefNamespace.
// It maps a repository's local branch and tag names to the ref names they take on the remote a push or delete addresses.

package gitrepo

import "strings"

// Local ref prefixes, which a RefNamespace maps to and from.
const (
	localHeadsPrefix = "refs/heads/"
	localTagsPrefix  = "refs/tags/"
)

// RefNamespace maps local branch and tag names to the ref names they take on origin.
// Heads and Tags are the origin-side prefixes of branches and tags, each ending in a slash.
// The zero value is the identity namespace: refs/heads/ and refs/tags/.
// RefNamespace learns no layout; its caller builds the value that matches the remote's refspecs.
type RefNamespace struct {
	Heads string
	Tags  string
}

// Branch returns the origin ref name of the branch name.
func (n RefNamespace) Branch(name string) string {
	return n.headsPrefix() + name
}

// Tag returns the origin ref name of the tag name.
func (n RefNamespace) Tag(name string) string {
	return n.tagsPrefix() + name
}

// LocalRef maps an origin ref name read back from ls-remote or a push result to its local refs/heads/ or refs/tags/ name.
// A ref outside both origin prefixes answers false, so a listing never reads a foreign ref as a namespaced one.
func (n RefNamespace) LocalRef(ref string) (string, bool) {
	if rest, found := strings.CutPrefix(ref, n.headsPrefix()); found {
		return localHeadsPrefix + rest, true
	}
	if rest, found := strings.CutPrefix(ref, n.tagsPrefix()); found {
		return localTagsPrefix + rest, true
	}
	return "", false
}

func (n RefNamespace) headsPrefix() string {
	if n.Heads == "" {
		return localHeadsPrefix
	}
	return n.Heads
}

func (n RefNamespace) tagsPrefix() string {
	if n.Tags == "" {
		return localTagsPrefix
	}
	return n.Tags
}
