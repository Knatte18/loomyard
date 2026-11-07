// Package machinename provides this machine's name: the hostname cut at its first dot and lowercased.
// It is the only production reader of the hostname, and it imports the standard library only.
// Every caller takes the name through a seam, so a test can hand in two names for two clones.
package machinename
