// discussion.go names the Discussion-Write spawn's role and skills, which DiscussionTable (discussiontable.go) builds its seats from.
// DiscussionWriterRole is the role of the session the parent-review gate prompts.

package loomengine

import (
	"github.com/Knatte18/loomyard/internal/seatengine"
)

// discussionRole is the agent-name role this module's Discussion-Write spawn carries.
const discussionRole = "discussion"

// DiscussionWriterRole returns the role of the session the parent-review gate prompts: the discussion table's lead, whose role is the discussion prefix alone.
// Advisors are out, since they take no turn after the lead's record exists.
func DiscussionWriterRole() string {
	return seatengine.SeatRole(discussionRole, "")
}

// discussionSkills are the skills the Discussion-Write spawn loads, in order.
var discussionSkills = []string{"scribe:prose", "scribe:conversation"}
