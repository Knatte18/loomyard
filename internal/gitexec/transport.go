// transport.go classifies a failed git remote operation as a transport failure:
// the remote was never reached, so the same command run again can plausibly succeed.

package gitexec

import (
	"errors"
	"regexp"
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

// transportPhrases are the lower-cased stderr fragments git prints when a remote operation fails in transit.
// Git prints them when it cannot reach the remote.
// It prints them when the connection drops before the remote's answer is complete.
// It prints them when the remote's front end answers with a gateway error before git reaches the repository.
var transportPhrases = []string{
	"could not resolve host",
	"connection refused",
	"connection reset",
	"connection timed out",
	"operation timed out",
	"network is unreachable",
	"failed to connect to",
	"operation too slow",
	"http/2 stream",
	"recv failure",
	"send failure",
	"empty reply from server",
	"early eof",
	"unexpected disconnect while reading sideband packet",
	"gnutls_handshake() failed",
	"tls connection was non-properly terminated",
	"the requested url returned error: 502",
	"the requested url returned error: 503",
	"the requested url returned error: 504",
}

// httpClientStatusPattern matches the HTTP 4xx status git prints in lower-cased stderr.
// Such an answer is a refusal that running the command again does not lift.
// The statuses of httpRetryableClientStatuses are the exception.
// The remote may close the connection after refusing, so a disconnect phrase can follow the refusal in the same stderr.
var httpClientStatusPattern = regexp.MustCompile(`(?:the requested url returned error: |rpc failed; http )(4\d\d)`)

// httpRetryableClientStatuses are the 4xx statuses that name a moment rather than a refusal.
// They are a request timeout and a rate limit.
var httpRetryableClientStatuses = map[string]bool{"408": true, "429": true}

// holdsHTTPClientRefusal reports whether the lower-cased stderr holds a 4xx status other than a retryable one.
func holdsHTTPClientRefusal(stderr string) bool {
	for _, match := range httpClientStatusPattern.FindAllStringSubmatch(stderr, -1) {
		if !httpRetryableClientStatuses[match[1]] {
			return true
		}
	}
	return false
}

// IsTransportFailure reports whether err holds a *GitError for a remote operation that failed to reach the remote.
// Remote operations are push, fetch, pull, ls-remote and clone.
// A *GitError with a non-zero Timeout counts too, since git was killed for outliving its deadline or stalling twice.
// A rejected push, an authentication failure, a missing ref or a local command is not a transport failure.
// A stderr holding an HTTP 4xx status is a refusal, whatever disconnect phrase follows it.
// The exceptions are 408 and 429, which name a moment rather than a refusal.
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
	if holdsHTTPClientRefusal(stderr) {
		return false
	}
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
