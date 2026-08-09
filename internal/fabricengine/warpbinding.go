// warpbinding.go owns the .lyx-warp warp-URL binding: a plain single-line record kept at the board
// root, recorded once on weft:main beside .lyx-anchor.
// It holds the warp URL only, never the subpath — the anchor already owns that.
// The record is written to disk here but committed onto weft:main by the CLI layer through Bolt;
// this file spawns no git and never calls Bolt itself.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WarpBindingFileName is the filename of the recorded warp-URL binding at the board root
// (<boardDir>/.lyx-warp).
// It holds only the warp URL string, plus a trailing newline.
// This is a structural per-repo record, written once at clone and read on every subsequent clone or
// reconcile — never a config or env override.
// It is distinct from lyxcwd.AnchorFileName: the anchor says where in warp lyx is rooted, this says
// which warp repo the weft pairs with.
// Exported because both fabricengine_test integration tests and this package's own reconcile code
// need to name the file.
const WarpBindingFileName = ".lyx-warp"

// readWarpBinding reads the recorded warp-URL binding from <boardDir>/.lyx-warp and reports whether a
// usable URL was found.
// It mirrors lyxcwd's readRecordedAnchor: any read error (an absent board directory, an absent
// binding file, or an unreadable file) and an empty-after-trim result both report ("", false), so the
// caller treats a corrupt or missing record the same as a genuinely unbound weft.
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

// writeWarpBinding writes warpURL plus a trailing newline to <boardDir>/.lyx-warp, replacing any
// existing content.
// The caller is responsible for committing the result onto weft:main through Bolt;
// this function only touches the working tree.
func writeWarpBinding(boardDir, warpURL string) error {
	path := filepath.Join(boardDir, WarpBindingFileName)
	if err := os.WriteFile(path, []byte(warpURL+"\n"), 0o644); err != nil {
		// Wrap with %w so a caller can unwrap to the underlying os error while still seeing the
		// full path that failed.
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// normalizeWarpURL reduces raw to a canonical spelling for equality comparison: it trims surrounding
// whitespace, strips exactly one trailing "/", then exactly one trailing ".git", then — only when the
// result begins with a "<scheme>://" prefix — lowercases the scheme and the host segment up to the
// first "/" after "://".
// A string with no "<scheme>://" prefix, such as a local filesystem path or an scp-form URL, is
// returned with only the two trailing strips applied and no case change at all, so a Windows drive
// letter survives byte-for-byte.
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
// It starts from normalizeWarpURL, drops a leading "<scheme>://", rewrites a leading scp-form
// "<user>@<host>:" into "<host>/", and lowercases the whole result, so two spellings of the same repo
// over different transports collapse to the same string.
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

// resolveEffectiveWarpURL encodes the whole warp-binding conflict rule, pure and git-free: given the
// recorded binding (and whether one was found) plus the URL the caller supplied on the command line,
// it decides which URL is effective, whether the caller must persist a new record, and whether the
// combination is a conflict that must abort instead.
//
// The caller prefixes the weft URL into the unbound-weft error text (see the CLI layer); this
// function's own message must not attempt to name it.
func resolveEffectiveWarpURL(recorded string, found bool, supplied string) (effective string, writeRecord bool, err error) {
	if !found && supplied == "" {
		return "", false, fmt.Errorf("weft has no recorded warp binding; supply the warp URL explicitly: lyx fabric clone <weft-url> <warp-url>")
	}
	if !found && supplied != "" {
		return supplied, true, nil
	}
	if found && supplied == "" {
		return recorded, false, nil
	}
	if normalizeWarpURL(recorded) == normalizeWarpURL(supplied) {
		// The supplied spelling is returned rather than the recorded one so the hub name and the
		// URL actually cloned are derived from the same string; the record itself is left
		// untouched either way.
		return supplied, false, nil
	}
	return "", false, fmt.Errorf("recorded warp binding %s does not match %s; refusing to re-point. If the warp repo moved, edit %s in the hub's _board worktree and commit.", recorded, supplied, WarpBindingFileName)
}
