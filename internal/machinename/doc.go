// Package machinename provides this machine's name.
// The name is read from the machine file, `machine.json` in the per-user lym config directory, on every call;
// with no such file, or no directory to hold one, it is the hostname cut at its first dot and lowercased.
// The file holds the current name under `name` and every name recorded for the machine under `names`.
// A file that does not read, does not decode or holds a name the name rule refuses is an error naming its path and way forward,
// never a fallback to the hostname, so an owner is never judged under a wrong name.
// Every error the package returns names its own way forward.
//
// It is the only production reader of the hostname and of the machine file,
// and it imports the standard library and internal/standalonestate only.
// Every caller takes the name through a seam, so a test can hand in two names for two clones.
package machinename
