// replies.go counts, per receiver role, the replies creel was asked for, the replies it got, the chases it sent and the messages it settled unanswered.

package reportengine

import (
	"sort"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
)

// RoleReplies counts the expected-reply traffic of one receiver role over the entries ComputeReplies read.
type RoleReplies struct {
	Role string `json:"role"`
	// Expected counts the posts marked as expecting a reply that went to the role.
	Expected int `json:"expected"`
	// Replied counts the replies the role sent to a marked message.
	Replied int `json:"replied"`
	// Chased counts the chases sent to the role.
	Chased int `json:"chased"`
	// Unanswered counts the marked messages settled unanswered with the role as holder.
	Unanswered int `json:"unanswered"`
}

// ComputeReplies counts expected replies, replies, chases and unanswered messages per receiver role over creel log entries, in role-name order.
// A strand counts under its role and a program under its upper-case name; a name that is neither counts nowhere.
// A reply counts only when its ReplyTo names a message the entries show as marked, so a reply to an unmarked message counts nowhere.
func ComputeReplies(entries []creelengine.LogEntry) []RoleReplies {
	marked := map[int64]bool{}
	for _, entry := range entries {
		if entry.Event == "posted" && entry.ExpectReply {
			marked[entry.ID] = true
		}
	}
	byRole := map[string]*RoleReplies{}
	count := func(name string) *RoleReplies {
		role, ok := replyRole(name)
		if !ok {
			return &RoleReplies{}
		}
		if byRole[role] == nil {
			byRole[role] = &RoleReplies{Role: role}
		}
		return byRole[role]
	}
	for _, entry := range entries {
		switch entry.Event {
		case "posted":
			if entry.ExpectReply {
				count(entry.To).Expected++
			}
			if marked[entry.ReplyTo] {
				count(entry.From).Replied++
			}
		case "chased":
			count(entry.To).Chased++
		case "unanswered":
			count(entry.To).Unanswered++
		}
	}
	roles := make([]RoleReplies, 0, len(byRole))
	for _, counts := range byRole {
		roles = append(roles, *counts)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Role < roles[j].Role })
	return roles
}

// replyRole returns the role a strand name carries, or the upper-case name of a program, and false for any other name.
func replyRole(name string) (string, bool) {
	if parsed, err := agentname.Parse(name); err == nil {
		return parsed.Role, true
	}
	if _, program, err := agentname.ParseProgram(name); err == nil {
		return program, true
	}
	return "", false
}
