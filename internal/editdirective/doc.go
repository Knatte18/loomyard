// doc.go carries the package godoc for editdirective: what the directive says, who renders it and why the package derives no path.

// Package editdirective renders the edit directive every spawned role's opening stencil carries, so a role changes files with the Edit and Write tools rather than a script.
//
// # What it says
//
// The directive states one rule: change files only with Edit or Write, with one Edit per site or `replace_all` for a bulk change, and never with `sed`, `awk`, `perl` or an inline interpreter heredoc.
// It constrains the tool a role edits with, never what the role may write.
// It has no inactive case: the text renders in a repo without a `PATTERN.md` as in one with it.
//
// # Who renders it
//
// Each spawning module's renderer calls Directive once per prompt and passes the text as the value of the MarkerName marker its opening stencil carries.
// The marker is required, so a renderer that forgets it fails at composition rather than shipping a prompt without the rule.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path: the stencil is read at call time from the told stencils directory through `stencilstore.Read`, never from embedded bytes.
package editdirective
