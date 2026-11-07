// taskguard.go — the refusal of a board write that creates or promotes a task from a run's agent session.

package boardengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// isRunAgentSession reports whether strandName is a session of a run's agent.
// Such a name carries a task slug under any role but the orch's;
// an empty name, a slug-free name and an orch thread's name are not.
func isRunAgentSession(strandName string) bool {
	name, err := agentname.Parse(strandName)
	return err == nil && name.Slug != "" && !agentname.MatchesRole(name.Role, agentname.RoleOrch)
}

// checkTaskCreation refuses a net change that creates a task, for a run agent's strandName.
// A task is created when after holds one whose slug loaded holds no task for.
// A promote, a note upserted to kind task and a merge upserting a new task count;
// a note write and an edit of an existing task do not.
// It returns the first refusal in after's order.
func checkTaskCreation(strandName string, loaded, after []Task) error {
	if !isRunAgentSession(strandName) {
		return nil
	}
	wasTask := make(map[string]bool, len(loaded))
	for _, task := range loaded {
		if task.Kind == KindTask {
			wasTask[task.Slug] = true
		}
	}
	for _, task := range after {
		if task.Kind == KindTask && !wasTask[task.Slug] {
			return fmt.Errorf("the run session %s cannot make %q a task: only the operator decides what becomes a task; "+
				"way forward: file the follow-up as a note with `lym board upsert` and \"kind\":\"note\", which the operator may promote",
				strandName, task.Slug)
		}
	}
	return nil
}
