// transport.go classifies a failed git remote operation as a transport failure:
// the remote was never reached, so the same command run again can plausibly succeed.

package gitexec

import (
	"errors"
	"strings"
)

// remoteSubcommands names the git subcommands that talk to a remote.
// Only these get a deadline and a stall bound, and only these are classified as transport failures.
var remoteSubcommands = map[string]bool{
	"push":      true,
	"fetch":     true,
	"pull":      true,
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
	"operation too slow",
}

// IsTransportFailure reports whether err holds a *GitError for a remote operation that failed to reach the remote.
// Remote operations are push, fetch, pull, ls-remote and clone.
// A *GitError with a non-zero Timeout counts too, since git was killed for outliving its deadline or stalling twice.
// A rejected push, an authentication failure, a missing ref or a local command is not a transport failure.
// An error with no *GitError in its chain is not one either.
// An unlisted stderr message is not a transport failure.
func IsTransportFailure(err error) bool {
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		return false
	}
	if !remoteSubcommands[subcommand(gitErr.Args)] {
		return false
	}
	if gitErr.Timeout != 0 {
		return true
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
