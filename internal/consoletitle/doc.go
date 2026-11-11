// Package consoletitle forms the title of a console window the operator sees.
// That is a VS Code window or a desktop terminal window.
// The title lets the windows of several hubs and projects open side by side be told apart.
// A title reads `<lead> [<hub shortname>:<project handle>] <name>`, as in `T2 [ly:P1] adopt-worker-record`:
// the handle the operator looks for first opens it.
// The lead is a task's handle, or `ORCH` for an orch's window.
// A part that does not exist is left out, and the brackets go when both their parts do.
// The hub shortname is lower case, as `lym creel hubs` prints it.
// It imports the standard library only.
package consoletitle
