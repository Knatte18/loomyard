// mergetask.go recognizes a merge task by its slug and its writer by strand name.
// A merge task is the darn task that brings a beam's parent branch into the beam.

package darnengine

import (
	"regexp"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// MergeTaskSuffix ends the slug of every merge task, before the optional number a later one takes.
const MergeTaskSuffix = "-merge-in"

// mergeTaskSlugPattern matches the slug of a merge task: the suffix, then an optional number.
var mergeTaskSlugPattern = regexp.MustCompile(regexp.QuoteMeta(MergeTaskSuffix) + `(-[0-9]+)?$`)

// IsMergeTaskSlug reports whether slug ends in MergeTaskSuffix, optionally followed by a number.
func IsMergeTaskSlug(slug string) bool {
	return mergeTaskSlugPattern.MatchString(slug)
}

// IsMergeTaskWriter reports whether strand names the darn writer of a merge task.
// Such a session's work is a merge of the parent branch.
func IsMergeTaskWriter(strand agentname.Name) bool {
	return agentname.MatchesRole(strand.Role, darnRole) && IsMergeTaskSlug(strand.Slug)
}
