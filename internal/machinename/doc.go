// Package machinename is this machine's name, the hostname cut at its first dot and lowercased.
// It is the only reader of the hostname in production, and it imports the standard library only.
// Every caller takes the name through a seam, so a test hands in two names for two clones.
package machinename
