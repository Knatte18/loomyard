// Package agentconfig owns the hub-wide _lym/config/agents.yaml:
// the skills each role lym starts loads, and the reasoning effort of its sessions and of their subagents.
//
// The skills key maps every Role to the list of skills its session loads in its first turn, in order.
// The hub's language skills follow that list for a role that writes or reviews code; the spawning code adds them.
// Load refuses a role without a list and a role key lym does not start, so no role starts with no list.
// A role the hub file leaves out takes the template's list, since the layered load fills a missing key.
//
// The efforts key maps default, a model alias, a role, or `<role>/<model>` to an effort in Claude Code's vocabulary.
// The most specific key wins, and default is required.
// Effort answers one session or subagent, and ModelEfforts lists the efforts a role's subagents take by model.
// SubagentEffortEnv names the Claude Code variable a session's settings carry its subagents' fallback effort in.
// The map is open: a hub file's map replaces the template's whole.
//
// skills is classed Guard, since a skill list sets the rules a writer or reviewer works to;
// efforts is classed None, so a worktree's local override may tune it.
// A content failure returns an error marked configengine.ErrInvalid that names the key and ends with the way forward.
// Load never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package agentconfig
