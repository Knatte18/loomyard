// discussion.go holds the agent role and skill list of the Discussion-Write spawn.

package loomengine

// discussionRole is the agent-name role this module's Discussion-Write spawn carries.
const discussionRole = "discussion"

// discussionSkills are the skills the Discussion-Write spawn loads, in order.
var discussionSkills = []string{"scribe:prose", "scribe:conversation"}
