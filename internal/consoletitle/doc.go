// Package consoletitle forms the title of a console window the operator sees.
// That is a VS Code window or a desktop terminal window.
// The title lets the windows of several hubs and projects open side by side be told apart.
// A title reads `<hub shortname>: <project handle>: <own handle>: <name>`,
// and a part that does not exist is left out.
// The hub shortname is lower case, as `lym creel hubs` prints it.
// It imports the standard library only.
package consoletitle
