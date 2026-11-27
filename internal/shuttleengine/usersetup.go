// usersetup.go declares the seam through which an engine hands another engine the user's Claude setup.
// The setup is what a Claude session in a worktree loads from the user's Claude config directory.
// It also holds the CLAUDE.md files above the worktree.

package shuttleengine

import "time"

// UserSetupSource resolves what a Claude session in a worktree loads from the user's Claude setup.
// It lets a provider other than Claude give its sessions the same instruction files, skills and plugin contexts.
// That provider does not import the Claude engine.
type UserSetupSource interface {
	// UserSetup resolves the setup of the session req describes.
	UserSetup(req UserSetupRequest) (UserSetup, error)
}

// UserSetupRequest describes the session a user setup is resolved for.
type UserSetupRequest struct {
	// WorkDir is the session's working directory.
	WorkDir string
	// RepoRoot is the root of the worktree the session runs in.
	RepoRoot string
	// Skills are the skills the session preloads.
	Skills []string
	// Role is the session's strand role.
	Role string
	// SessionID is the lym run's id.
	SessionID string
	// HookTimeout bounds each plugin hook run.
	HookTimeout time.Duration
	// RunHook runs one plugin hook; nil runs none.
	RunHook HookRunner
}

// UserSetup is the user's Claude setup resolved for one session.
type UserSetup struct {
	// InstructionFiles are the user's global CLAUDE.md and the CLAUDE.md files above the worktree root, outermost first.
	InstructionFiles []InstructionItem
	// SkillListing lists the skills the session's scope turns on, with the rule for using one; empty when none is listed.
	SkillListing string
	// ListedSkills holds the names SkillListing lists, in listing order.
	ListedSkills []string
	// PluginContexts are the contexts the scope's plugins' session-start hooks produce.
	PluginContexts []InstructionItem
	// Omitted lists each plugin hook whose context is left out, with the reason.
	Omitted []OmittedHook
	// Warnings name what the resolution went on despite.
	Warnings []string
}

// InstructionItem is one named piece of instruction text.
type InstructionItem struct {
	// Name labels the item, in a refusal that names the largest item.
	Name string
	// Text is the item's text.
	Text string
	// WayForward is the step that shrinks the item.
	WayForward string
	// Plugin and Hook are the plugin id and hook command of a plugin session-start context; empty on any other item.
	Plugin string
	Hook   string
}

// OmittedHook is a plugin hook whose context a session goes without.
type OmittedHook struct {
	// Name labels the hook.
	Name string
	// Reason says why its context is left out.
	Reason string
	// Plugin and Hook are the plugin id and hook command; empty for an omission that names no one hook.
	Plugin string
	Hook   string
}

// HookRun describes one plugin hook run.
type HookRun struct {
	// Command is the hook's shell command.
	Command string
	// Dir is the directory the command runs in.
	Dir string
	// Env holds the `KEY=value` variables the command's environment gains.
	Env []string
	// Stdin is the payload the command reads.
	Stdin []byte
	// Timeout bounds the run.
	Timeout time.Duration
}

// HookRunner runs one plugin hook and returns its standard output.
type HookRunner func(run HookRun) ([]byte, error)
