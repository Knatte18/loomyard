// doc.go carries the package godoc for parentdirective:
// what the directive says, its variants, who renders it and why the package derives no path.

// Package parentdirective renders the parent directive every spawned role's opening stencil carries,
// so a role that is blocked escalates to its parent session rather than the operator.
//
// # Variants
//
// Directive picks its text by its arguments.
// A non-empty parent name renders the parent variant.
// It names the parent and says a message from that name is the operator's delegate within the role's limits.
// It keeps the role's own judgment and every limit and deny in force.
// It also admits the decision notice `lym loom decision add` posts as a message from the parent.
// The notice is a `[creel]` line from the hub's `LOOM` program opening with a fixed phrase;
// creel accepts a program claim only from an orch pane or a process in no pane,
// so no task agent can sign one, and every other session's message stays untrusted.
// A non-interactive role also gets the operator-ban line.
// An interactive role does not, since its interview questions go to the operator; only the ban line differs.
// An empty parent name renders the no-parent variant: a blocked role writes its report and stops.
//
// # Who renders it
//
// Each spawning module calls Directive once per prompt.
// It passes the text as the value of the MarkerName marker its opening stencil carries.
// The parent name is told, never derived:
// a caller takes it from the origin record reed injects as `agentname.ParentEnv`.
// That record is the `ParentName` of the reed geometry `hubgeom` builds.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path:
// the stencils are read at call time from the told stencils directory through `stencilstore.Read`.
// They are never embedded bytes.
package parentdirective
