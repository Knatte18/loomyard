// errors.go defines the typed error vocabulary the codeintelengine package
// returns to its sole caller, internal/codeintelcli. Each failure mode gets
// its own sentinel or data-carrying type so the CLI layer can map it to a
// specific output.Err response instead of a generic message; see the plan's
// "typed error vocabulary" Shared Decision.

package codeintelengine

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoLanguage is returned by DetectLanguage when no registry entry's
// markers matched anything under the target directory. Callers wrap it with
// the searched markers via fmt.Errorf("...: %w", ErrNoLanguage) so
// errors.Is(err, ErrNoLanguage) still succeeds after wrapping.
var ErrNoLanguage = errors.New("codeintelengine: no language detected")

// ErrServerNotFoundSentinel is the package-level sentinel *ErrServerNotFound.Is
// compares against, so callers can test for this failure mode with
// errors.Is(err, codeintelengine.ErrServerNotFoundSentinel) without needing
// to know the concrete field values of the *ErrServerNotFound the engine
// actually returned.
var ErrServerNotFoundSentinel = errors.New("codeintelengine: language server not found")

// ErrServerNotFound reports that the language server binary for Language is
// absent from $PATH. InstallHint carries the registry entry's operator-facing
// install command so the CLI can surface it directly to the user.
type ErrServerNotFound struct {
	Language    string
	InstallHint string
}

// Error implements error, naming both the language and the install hint so a
// user sees exactly what to run to fix the failure.
func (e *ErrServerNotFound) Error() string {
	return fmt.Sprintf("codeintelengine: no language server found for %q; install with: %s", e.Language, e.InstallHint)
}

// Is reports whether target is ErrServerNotFoundSentinel, letting
// errors.Is(err, ErrServerNotFoundSentinel) match any *ErrServerNotFound
// value regardless of its field contents.
func (e *ErrServerNotFound) Is(target error) bool {
	return target == ErrServerNotFoundSentinel
}

// ErrSymbolNotFoundSentinel is the package-level sentinel *ErrSymbolNotFound.Is
// compares against, mirroring ErrServerNotFoundSentinel.
var ErrSymbolNotFoundSentinel = errors.New("codeintelengine: symbol not found")

// ErrSymbolNotFound reports that Symbol resolved to zero candidates when
// queried against the language server rooted at TargetDir.
type ErrSymbolNotFound struct {
	Symbol    string
	TargetDir string
}

// Error implements error, naming both the queried symbol and the directory
// the search was rooted at.
func (e *ErrSymbolNotFound) Error() string {
	return fmt.Sprintf("codeintelengine: symbol %q not found under %s", e.Symbol, e.TargetDir)
}

// Is reports whether target is ErrSymbolNotFoundSentinel, letting
// errors.Is(err, ErrSymbolNotFoundSentinel) match any *ErrSymbolNotFound
// value regardless of its field contents.
func (e *ErrSymbolNotFound) Is(target error) bool {
	return target == ErrSymbolNotFoundSentinel
}

// ErrAmbiguousSymbolSentinel is the package-level sentinel
// *ErrAmbiguousSymbol.Is compares against, mirroring ErrServerNotFoundSentinel.
var ErrAmbiguousSymbolSentinel = errors.New("codeintelengine: ambiguous symbol")

// ErrAmbiguousSymbol reports that Symbol resolved to more than one candidate
// position. Candidates holds each match formatted as "file:line:col" so the
// CLI layer can list them verbatim for the user to disambiguate.
type ErrAmbiguousSymbol struct {
	Symbol     string
	Candidates []string
}

// Error implements error, naming the symbol and listing every candidate
// position so the user can pick the intended one without re-running a
// broader search.
func (e *ErrAmbiguousSymbol) Error() string {
	return fmt.Sprintf("codeintelengine: symbol %q is ambiguous; candidates: %s", e.Symbol, strings.Join(e.Candidates, ", "))
}

// Is reports whether target is ErrAmbiguousSymbolSentinel, letting
// errors.Is(err, ErrAmbiguousSymbolSentinel) match any *ErrAmbiguousSymbol
// value regardless of its field contents.
func (e *ErrAmbiguousSymbol) Is(target error) bool {
	return target == ErrAmbiguousSymbolSentinel
}

// ErrResolverUnsupportedSentinel is the package-level sentinel
// *ErrResolverUnsupported.Is compares against, mirroring
// ErrServerNotFoundSentinel.
var ErrResolverUnsupportedSentinel = errors.New("codeintelengine: resolver unsupported")

// ErrResolverUnsupported reports that the language server launched for
// Language (server binary Server) does not advertise the capability the
// engine needs (e.g. workspaceSymbolProvider for name resolution).
type ErrResolverUnsupported struct {
	Language string
	Server   string
}

// Error implements error, naming both the language and the concrete server
// binary that lacks the required capability.
func (e *ErrResolverUnsupported) Error() string {
	return fmt.Sprintf("codeintelengine: language server %q for %q does not support this resolver", e.Server, e.Language)
}

// Is reports whether target is ErrResolverUnsupportedSentinel, letting
// errors.Is(err, ErrResolverUnsupportedSentinel) match any
// *ErrResolverUnsupported value regardless of its field contents.
func (e *ErrResolverUnsupported) Is(target error) bool {
	return target == ErrResolverUnsupportedSentinel
}

// ErrServerTimeoutSentinel is the package-level sentinel *ErrServerTimeout.Is
// compares against, mirroring ErrServerNotFoundSentinel.
var ErrServerTimeoutSentinel = errors.New("codeintelengine: language server timed out")

// ErrServerTimeout reports that the language server subprocess failed to
// respond to Phase within Timeout. Phase is one of "initialize",
// "references", or "workspace/symbol" — the three request phases the engine
// bounds with a context deadline (see the plan's deadline-with-hard-kill
// Shared Decision).
type ErrServerTimeout struct {
	Phase   string
	Timeout string
}

// Error implements error, naming the stalled phase and the deadline that
// expired so a user can distinguish a slow server from a hung one.
func (e *ErrServerTimeout) Error() string {
	return fmt.Sprintf("codeintelengine: language server timed out during %q after %s", e.Phase, e.Timeout)
}

// Is reports whether target is ErrServerTimeoutSentinel, letting
// errors.Is(err, ErrServerTimeoutSentinel) match any *ErrServerTimeout value
// regardless of its field contents.
func (e *ErrServerTimeout) Is(target error) bool {
	return target == ErrServerTimeoutSentinel
}
