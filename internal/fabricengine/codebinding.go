// codebinding.go owns the .lym-warp warp-URL binding:
// a single-line record kept at the board root and recorded once on weft:main.
// It holds the warp URL only; each worktree's .lym-anchor, at its weft root, owns the subpath.
// The record is written to disk here but committed onto weft:main by the CLI layer through Bolt;
// this file spawns no git and never calls Bolt itself.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ErrNoWarpBinding matches, under errors.Is, the refusal of a weft with no warp binding and no supplied warp URL.
var ErrNoWarpBinding = errors.New("no recorded warp binding")

// ErrWarpBindingMismatch matches, under errors.Is, the refusal to re-point a recorded binding at a different warp URL.
var ErrWarpBindingMismatch = errors.New("warp binding mismatch")

// WarpBindingFileName is the filename of the recorded warp-URL binding at the board root (<boardDir>/.lym-warp).
// It holds only the warp URL string, plus a trailing newline.
// This is a structural per-repo record, written once at clone and read on every subsequent clone or reconcile.
// It is never a config or env override.
// The anchor file says where in warp lym is rooted; this file says which warp repo the weft pairs with.
// Exported so other packages and tests can name the file.
const WarpBindingFileName = ".lym-warp"

// readWarpBinding reads the recorded warp-URL binding from <boardDir>/.lym-warp and reports whether a
// usable URL was found.
// Any read error (an absent board directory, an absent binding file, or an unreadable file) reports ("", false).
// So does an empty-after-trim result.
// The caller therefore treats a corrupt or missing record the same as an unbound weft.
func readWarpBinding(boardDir string) (warpURL string, found bool) {
	data, err := os.ReadFile(filepath.Join(boardDir, WarpBindingFileName))
	if err != nil {
		return "", false
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// HubRepoURL returns the URL of the repository the hub at l works on:
// the warp binding recorded at the hub's board dir, else the origin remote of l's warp worktree.
// The binding wins when both exist, since it is the hub's recorded pairing.
// It returns an error naming both sources when neither yields a URL.
func HubRepoURL(l *lymcwd.Location) (string, error) {
	if recorded, found := readWarpBinding(BoltDir(l.HubPath)); found {
		return recorded, nil
	}
	origin, err := gitrepo.New(l.WorktreePath()).RemoteURL("origin")
	if err != nil {
		return "", fmt.Errorf("the hub records no %s warp binding and its warp worktree has no usable origin remote: %w", WarpBindingFileName, err)
	}
	return origin, nil
}

// HubOriginURL returns the URL of the origin remote of the prime worktree of the hub at hubPath.
// The prime is the worktree named after the hub: the hub `<name>-LYMHUB` holds it as `<name>`.
// It reads the repository's local config and spawns no git process.
// A prime with no origin remote, or an origin with no configured URL, is an error.
func HubOriginURL(hubPath string) (string, error) {
	prime := filepath.Join(hubPath, strings.TrimSuffix(filepath.Base(hubPath), HubSuffix))
	return gitrepo.New(prime).RemoteURL("origin")
}

// writeWarpBinding writes warpURL plus a trailing newline to <boardDir>/.lym-warp, replacing any
// existing content.
// The caller is responsible for committing the result onto weft:main through Bolt;
// this function only touches the working tree.
func writeWarpBinding(boardDir, warpURL string) error {
	path := filepath.Join(boardDir, WarpBindingFileName)
	if err := os.WriteFile(path, []byte(warpURL+"\n"), 0o644); err != nil {
		// Wrap with %w so a caller can unwrap to the os error while still seeing the path that failed.
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// normalizeWarpURL reduces raw to a canonical spelling for equality comparison.
// It trims surrounding whitespace and strips exactly one trailing "/", then exactly one trailing ".git".
// Only when the result begins with a "<scheme>://" prefix does it lowercase the scheme and the host segment.
// The host segment runs up to the first "/" after "://".
// A string with no "<scheme>://" prefix gets only the two trailing strips and no case change.
// That keeps a local filesystem path or an scp-form URL intact, so a Windows drive letter survives byte-for-byte.
// The slash strip runs before the .git strip, so "repo.git/" reduces to "repo".
func normalizeWarpURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimSuffix(trimmed, "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")

	schemeSep := strings.Index(trimmed, "://")
	if schemeSep < 0 {
		return trimmed
	}

	scheme := strings.ToLower(trimmed[:schemeSep])
	rest := trimmed[schemeSep+3:]
	hostEnd := strings.Index(rest, "/")
	if hostEnd < 0 {
		return scheme + "://" + strings.ToLower(rest)
	}
	host := strings.ToLower(rest[:hostEnd])
	path := rest[hostEnd:]
	return scheme + "://" + host + path
}

// warpURLTransportIdentity reduces raw to a transport-agnostic identity string, used only to word a
// reconcile divergence detail and never to decide equality.
// It normalizes the spelling, then drops a leading "<scheme>://".
// It rewrites a leading scp-form "<user>@<host>:" into "<host>/" and lowercases the whole result.
// Two spellings of the same repo over different transports therefore collapse to one string.
func warpURLTransportIdentity(raw string) string {
	normalized := normalizeWarpURL(raw)

	if schemeSep := strings.Index(normalized, "://"); schemeSep >= 0 {
		normalized = normalized[schemeSep+3:]
	} else if at := strings.Index(normalized, "@"); at >= 0 {
		if colon := strings.Index(normalized[at+1:], ":"); colon >= 0 {
			host := normalized[at+1 : at+1+colon]
			path := normalized[at+1+colon+1:]
			normalized = host + "/" + path
		}
	}

	return strings.ToLower(normalized)
}

// resolveEffectiveWarpURL applies the warp-binding conflict rule.
// It is pure and git-free.
// It takes the recorded binding, whether one was found, and the URL the caller supplied on the command line.
// It decides which URL is effective.
// It decides whether the caller must persist a new record.
// It decides whether the combination is a conflict that must abort instead.
//
// The caller adds the weft URL to the unbound-weft error text; this function's own message must not name it.
func resolveEffectiveWarpURL(recorded string, found bool, supplied string) (effective string, writeRecord bool, err error) {
	if !found && supplied == "" {
		return "", false, &sentinelError{
			sentinel: ErrNoWarpBinding,
			msg:      "weft has no recorded warp binding; supply the warp URL explicitly: lym fabric clone <weft-url> <warp-url>",
		}
	}
	if !found && supplied != "" {
		return supplied, true, nil
	}
	if found && supplied == "" {
		return recorded, false, nil
	}
	if normalizeWarpURL(recorded) == normalizeWarpURL(supplied) {
		// The supplied spelling is returned, not the recorded one,
		// so the hub name and the cloned URL come from the same string.
		// The record is left untouched either way.
		return supplied, false, nil
	}
	return "", false, &sentinelError{
		sentinel: ErrWarpBindingMismatch,
		msg: fmt.Sprintf("recorded warp binding %s does not match %s; refusing to re-point. If the warp repo moved, edit %s in the hub's %s worktree and commit.",
			recorded, supplied, WarpBindingFileName, BoltDirName),
	}
}
