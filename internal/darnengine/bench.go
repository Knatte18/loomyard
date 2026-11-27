// bench.go recognizes a bench pair by its slug and a darn writer by its strand name.
// A bench pair is a pair a darn writer makes for its own bench, forked from its task worktree.

package darnengine

import (
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// BenchPairSlugInfix joins a task's slug and a bench pair's number in the bench pair's slug, as in `<slug>-b1`.
const BenchPairSlugInfix = "-b"

// benchPairNumberPattern matches the number that ends a bench pair's slug: a positive number with no leading zero.
var benchPairNumberPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// IsBenchPairSlug reports whether slug names a bench pair of the task taskSlug:
// the task's slug, BenchPairSlugInfix, then a positive number.
func IsBenchPairSlug(taskSlug, slug string) bool {
	number, found := strings.CutPrefix(slug, taskSlug+BenchPairSlugInfix)
	return found && benchPairNumberPattern.MatchString(number)
}

// IsWriter reports whether strand names a darn writer, the session that makes a darn task's change.
func IsWriter(strand agentname.Name) bool {
	return agentname.MatchesRole(strand.Role, darnRole)
}
