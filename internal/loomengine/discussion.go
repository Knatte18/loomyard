// discussion.go names the Discussion-Write spawn's role and skills, which DiscussionTable (discussiontable.go) builds its seats from.

package loomengine

// discussionRole is the agent-name role this module's Discussion-Write spawn carries.
const discussionRole = "discussion"

// discussionSkills are the skills the Discussion-Write spawn loads, in order.
var discussionSkills = []string{"scribe:prose", "scribe:conversation"}
