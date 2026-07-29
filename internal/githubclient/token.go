// token.go implements the non-blocking GitHub token resolution chain:
// GH_TOKEN, then GITHUB_TOKEN, then the on-disk cache (cache.go), then a
// bounded `gh auth token` shell-out. Every path returns quickly or returns a
// typed error — never a prompt, and never `gh auth login`.

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
// authenticating transport (transport.go) branches its 401 retry policy on
// this value: an environment-sourced token must never be invalidated and
// re-resolved, because re-resolution is guaranteed to reproduce the
// identical value and a replay would be a guaranteed-identical second
// failure.
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

// isEnvSource reports whether s is one of the environment-variable sources,
// which outrank the cache by rule and must never be invalidated-and-replayed
// on a 401.
func (s tokenSource) isEnvSource() bool {
	return s == sourceGHTokenEnv || s == sourceGitHubTokenEnv
}

// envName returns the environment variable name associated with an
// environment-sourced tokenSource, for naming the rejected source in a 401
// error message. It panics on a non-environment source, which would be an
// internal logic error in the caller.
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

// ghAuthTokenTimeout bounds the `gh auth token` shell-out so a hung or
// unexpectedly prompting `gh` process can never block an autonomous lyx run
// indefinitely.
const ghAuthTokenTimeout = 5 * time.Second

// runGHAuthToken is the seam through which the `gh auth token` shell-out
// runs. Tests replace it with a fake — including one that hangs — to assert
// that ghAuthTokenTimeout actually fires, without needing a real gh binary
// or network access.
var runGHAuthToken = realRunGHAuthToken

// realRunGHAuthToken runs `gh auth token` under ctx and returns the trimmed
// stdout token, or an error. `gh auth token` itself fails fast when no
// session exists rather than opening an interactive login flow, and
// exec.CommandContext guarantees the process is killed if ctx's deadline
// elapses first — this function never invokes `gh auth login`.
func realRunGHAuthToken(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")

	// Suppress the console window on Windows; no-op on other platforms.
	// The transport this replaces does exactly this today.
	proc.HideWindow(cmd)

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ErrTokenUnresolvable is returned by resolveToken when no credential source
// produced a usable token. Callers surface this as a typed error rather than
// waiting or prompting — there is no code path from here to `gh auth login`.
var ErrTokenUnresolvable = errors.New("githubclient: no GitHub token available (set GH_TOKEN or GITHUB_TOKEN, or run `gh auth login`)")

// resolveToken resolves a GitHub token by trying, in order: the GH_TOKEN
// environment variable, GITHUB_TOKEN, the on-disk cache (cache.go), and
// finally a bounded `gh auth token` shell-out. It returns the token alongside
// the tokenSource that produced it, because the authenticating transport
// branches its 401 retry policy on that source. Resolution never blocks
// beyond ghAuthTokenTimeout and never invokes `gh auth login`; an
// unresolvable token returns ErrTokenUnresolvable immediately.
func resolveToken() (string, tokenSource, error) {
	// Environment variables always win over the cache, in this fixed order.
	if tok := strings.TrimSpace(os.Getenv("GH_TOKEN")); tok != "" {
		return tok, sourceGHTokenEnv, nil
	}
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		return tok, sourceGitHubTokenEnv, nil
	}

	// No environment override; a fresh cache entry avoids paying the
	// `gh auth token` shell-out cost on every call.
	if tok, ok := readCachedToken(); ok {
		return tok, sourceCache, nil
	}

	// Fall through to `gh`, bounded so a hung or unexpectedly prompting
	// process can never stall an autonomous run.
	ctx, cancel := context.WithTimeout(context.Background(), ghAuthTokenTimeout)
	defer cancel()

	tok, err := runGHAuthToken(ctx)
	if err != nil || tok == "" {
		return "", sourceUnknown, ErrTokenUnresolvable
	}

	// Best-effort cache write: a failure to cache must not fail resolution,
	// since the token itself is still valid and usable for this call.
	writeCachedToken(tok)

	return tok, sourceGHCLI, nil
}
