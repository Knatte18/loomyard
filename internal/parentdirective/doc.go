// doc.go carries the package godoc for parentdirective: what the directive says, its variants, who renders it and why the package derives no path.

// Package parentdirective renders the parent directive every spawned role's opening stencil carries, so a role that is blocked escalates to its parent session rather than the operator.
//
// # Variants
//
// Directive picks one of three texts by its arguments.
// A non-empty parent name renders the parent variant: it names the parent, says a message from that name is the operator's delegate within the role's limits, and keeps the role's own judgment and every limit and deny in force.
// A non-interactive role also gets the operator-ban line; an interactive role, whose interview questions go to the operator, does not, so only the ban line differs.
// An empty parent name renders the no-parent variant: a blocked role writes its report and stops.
//
// # Who renders it
//
// Each spawning module calls Directive once per prompt and passes the text as the value of the MarkerName marker its opening stencil carries.
// The parent name is told, never derived: a caller takes it from the same origin record reed injects as `agentname.ParentEnv`, the `ParentName` of the reed geometry `hubgeom` builds.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path: the stencils are read at call time from the told stencils directory through `stencilstore.Read`, never from embedded bytes.
package parentdirective
