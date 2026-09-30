// transport.go classifies a failed git remote operation as a transport failure:
// the remote was never reached, so the same command run again can plausibly succeed.

package gitexec

import (
	"errors"
	"strings"
)

// remoteSubcommands are the git subcommands that talk to a remote, and the only ones IsTransportFailure classifies.
var remoteSubcommands = map[string]bool{
	"push":      true,
	"fetch":     true,
	"ls-remote": true,
	"clone":     true,
}

// transportPhrases are the lower-cased stderr fragments git prints when it cannot reach the remote.
var transportPhrases = []string{
	"could not resolve host",
	"connection refused",
	"connection reset",
	"connection timed out",
	"operation timed out",
	"network is unreachable",
	"failed to connect to",
}

// IsTransportFailure reports whether err's chain holds a *GitError for a remote operation
// (push, fetch, ls-remote or clone) that failed to reach the remote.
// A rejected push, an authentication failure, a missing ref, a local command and an error
// with no *GitError in its chain are not transport failures.
// A stderr message in a format not listed in transportPhrases falls back to "not transient",
// which is today's behaviour.
func IsTransportFailure(err error) bool {
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		return false
	}
	if !remoteSubcommands[subcommand(gitErr.Args)] {
		return false
	}
	stderr := strings.ToLower(gitErr.Stderr)
	for _, phrase := range transportPhrases {
		if strings.Contains(stderr, phrase) {
			return true
		}
	}
	return false
}

// subcommand returns the first argument after git's global options.
// -c and -C each consume the argument that follows them, and any other argument starting with "-" is skipped.
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-c" || a == "-C":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a
		}
	}
	return ""
}
