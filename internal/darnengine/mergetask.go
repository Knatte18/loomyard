// mergetask.go recognizes a merge task, the darn task that brings a beam's parent branch into the beam, by its slug, and that task's writer by its strand name.

package darnengine

import (
	"regexp"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// MergeTaskSuffix ends the slug of every merge task, before the number a second one takes.
const MergeTaskSuffix = "-merge-in"

// mergeTaskSlugPattern matches the slug of a merge task: the suffix, then an optional number.
var mergeTaskSlugPattern = regexp.MustCompile(regexp.QuoteMeta(MergeTaskSuffix) + `(-[0-9]+)?$`)

// IsMergeTaskSlug reports whether slug has the shape of a merge task's slug.
func IsMergeTaskSlug(slug string) bool {
	return mergeTaskSlugPattern.MatchString(slug)
}

// IsMergeTaskWriter reports whether strand names the darn writer of a merge task, the one task session whose work is a merge of the parent branch.
func IsMergeTaskWriter(strand agentname.Name) bool {
	return agentname.MatchesRole(strand.Role, darnRole) && IsMergeTaskSlug(strand.Slug)
}
