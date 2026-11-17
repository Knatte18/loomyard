// secretpath.go holds the shipped secret-file patterns, the Matcher over them and a hub's, and the hit rendering.

package secretpath

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// shippedPatterns are the secret-file patterns every Matcher refuses, whatever a hub adds.
var shippedPatterns = []string{".env", "*.pem", "id_rsa", "credentials*", "secrets.y*ml"}

// Matcher matches repository-relative paths against lym's shipped secret-file patterns and a hub's added ones.
// A pattern holding a slash matches the whole slash-separated path; any other pattern matches the path's base name.
// The zero Matcher matches the shipped patterns alone.
type Matcher struct {
	added []string
}

// New returns a Matcher over the shipped patterns and added.
// It refuses an added pattern that path.Match cannot parse, naming the pattern.
func New(added []string) (Matcher, error) {
	for _, p := range added {
		if _, err := path.Match(p, ""); err != nil {
			return Matcher{}, fmt.Errorf("secretpath: malformed pattern %q: %w", p, err)
		}
	}
	return Matcher{added: slices.Clone(added)}, nil
}

// Hit is one path a Matcher refuses, with the first pattern it matched.
type Hit struct {
	Path    string
	Pattern string
}

// Match returns the first pattern relPath matches, shipped patterns first, and whether any did.
func (m Matcher) Match(relPath string) (pattern string, matched bool) {
	slashed := filepath.ToSlash(relPath)
	base := path.Base(slashed)
	for _, p := range slices.Concat(shippedPatterns, m.added) {
		target := base
		if strings.Contains(p, "/") {
			target = slashed
		}
		// New refused every pattern path.Match cannot parse, so the error is always nil.
		if ok, _ := path.Match(p, target); ok {
			return p, true
		}
	}
	return "", false
}

// Hits returns, in input order, every path of relPaths that the Matcher refuses.
func (m Matcher) Hits(relPaths []string) []Hit {
	var hits []Hit
	for _, p := range relPaths {
		if pattern, ok := m.Match(p); ok {
			hits = append(hits, Hit{Path: p, Pattern: pattern})
		}
	}
	return hits
}

// Describe renders hits for a refusal as `<path> (pattern <pattern>)` items joined by ", ".
func Describe(hits []Hit) string {
	items := make([]string, len(hits))
	for i, h := range hits {
		items[i] = fmt.Sprintf("%s (pattern %s)", h.Path, h.Pattern)
	}
	return strings.Join(items, ", ")
}
