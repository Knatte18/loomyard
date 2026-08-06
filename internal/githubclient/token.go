// token.go implements the non-blocking GitHub token resolution chain: GH_TOKEN, then GITHUB_TOKEN, then the on-disk cache (cache.go), then a bounded `gh auth token` shell-out.
// Every path returns quickly or returns a typed error — never a prompt,
// and never `gh auth login`.

package githubclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

// tokenSource identifies which resolution step produced a token. The
// authenticating transport branches its 401 retry policy on this value.
type tokenSource int

const (
	// sourceUnknown is the zero value and is never returned by resolveToken
	// on a successful resolution.
	sourceUnknown tokenSource = iota
	// sourceGHTokenEnv means the token came from the GH_TOKEN environment variable.
	sourceGHTokenEnv
	// sourceGitHubTokenEnv means the token came from the GITHUB_TOKEN environment variable.
	sourceGitHubTokenEnv
	// sourceCache means the token was read from the on-disk credential cache.
	sourceCache
	// sourceGHCLI means the token was freshly resolved via the `gh auth token` shell-out.
	sourceGHCLI
)

// isEnvSource reports whether s is environment-sourced, which must never be
// invalidated-and-replayed.
func (s tokenSource) isEnvSource() bool {
	return s == sourceGHTokenEnv || s == sourceGitHubTokenEnv
}

// envName returns the environment variable name for an environment-sourced
// tokenSource. Panics on non-environment sources.
func (s tokenSource) envName() string {
	switch s {
	case sourceGHTokenEnv:
		return "GH_TOKEN"
	case sourceGitHubTokenEnv:
		return "GITHUB_TOKEN"
	default:
		panic(fmt.Sprintf("githubclient: envName called on non-env tokenSource %d", s))
	}
}

// ghAuthTokenTimeout bounds the `gh auth token` shell-out. Var not const
// to allow test override.
var ghAuthTokenTimeout = 5 * time.Second

// runGHAuthToken is the seam through which `gh auth token` runs. Tests
// replace it with a fake.
var runGHAuthToken = realRunGHAuthToken

// realRunGHAuthToken runs `gh auth token` and returns the trimmed token, or
// an error. Never invokes `gh auth login`.
func realRunGHAuthToken(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")

	proc.HideWindow(cmd)

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ErrTokenUnresolvable is returned by resolveToken when no credential source produced a usable token.
// Callers surface this as a typed error rather than waiting or prompting — there is no code path from here to `gh auth login`.
var ErrTokenUnresolvable = errors.New("githubclient: no GitHub token available (set GH_TOKEN or GITHUB_TOKEN, or run `gh auth login`)")

// resolveToken tries in order: GH_TOKEN, GITHUB_TOKEN, the on-disk cache, and
// a bounded `gh auth token` shell-out. Never blocks beyond ghAuthTokenTimeout
// and never invokes `gh auth login`.
func resolveToken() (string, tokenSource, error) {
	if tok := strings.TrimSpace(os.Getenv("GH_TOKEN")); tok != "" {
		return tok, sourceGHTokenEnv, nil
	}
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		return tok, sourceGitHubTokenEnv, nil
	}

	if tok, ok := readCachedToken(); ok {
		return tok, sourceCache, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), ghAuthTokenTimeout)
	defer cancel()

	tok, err := runGHAuthToken(ctx)
	if err != nil || tok == "" {
		return "", sourceUnknown, ErrTokenUnresolvable
	}

	writeCachedToken(tok)

	return tok, sourceGHCLI, nil
}
