// server.go computes the per-hub tmux server identity: the server name (also reused as the -L
// socket name) and the per-worktree session name.
// Both construction rules live here, in the tmux domain, rather than in lyxcwd, because each is a
// tmux-specific derivation (a socket key and a session name, neither of them a filesystem path)
// over a plain hub or worktree path string its caller already resolved.
// Neither function sees a *lyxcwd.Location: hubgeom.ReedGeometry calls them to fill
// Geometry.SocketKey/SessionName, and this package only ever reads those told fields back.
// It also carries validateToldTmuxIdentity, the pre-flight that refuses a told tmux identity the
// multiplexer could not spend verbatim — the one check that keeps "New validates nothing" from
// meaning "an unusable identity is discovered only as a 20-second boot timeout".
// The file is named server.go, not naming.go, so it is not confusable with the strand-name helpers
// in name.go.
package reedengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ServerName returns the deterministic tmux server name for the hub: "lyx-<basename>-<hash>", where
// hash ensures distinct hubs are distinct.
// The basename half is purely for human readability — uniqueness rests entirely on the hash of the
// cleaned absolute hub path — so any path separator it carries is substituted rather than passed
// through: filepath.Base returns a bare separator for a root path ("/" on POSIX, "C:\\" -> "\\" on
// Windows), which is reachable whenever a worktree sits one level under the filesystem root (a
// container's /workspace or /app, whose hub then resolves to "/"). tmux resolves -L <key> as a
// filename under its per-user socket directory, so a key carrying a separator names a path whose
// parent does not exist; tmux prints "error creating <path>" and STILL EXITS 0 (verified live,
// tmux 3.6), which no probe reed has can tell apart from a slow boot.
// Substituting keeps the key usable and cannot collide, since the hash is untouched.
func ServerName(hubPath string) string {
	abs := cleanAbsHubPath(hubPath)
	base := socketSafeBase(filepath.Base(abs))
	sum := sha256.Sum256([]byte(abs))
	shortHash := hex.EncodeToString(sum[:])[:8]
	return "lyx-" + base + "-" + shortHash
}

// socketSafeBase substitutes '_' for every path separator in base.
// Both separators are substituted on every platform — a separator-free contract must not depend on
// GOOS.
func socketSafeBase(base string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(socketKeySeparators, r) {
			return '_'
		}
		return r
	}, base)
}

// SessionName returns the tmux session name for a worktree: its directory slug.
func SessionName(worktreeRoot string) string {
	return filepath.Base(worktreeRoot)
}

// rewrittenSessionNameChars are the characters tmux silently rewrites to '_' inside a session name.
// tmux does not REJECT them: new-session exits 0 and creates a session under the rewritten name
// (verified live, tmux 3.6: "a.b" and "a:b" both become "a_b").
// That silence is what makes the ban necessary rather than cosmetic — every other -t target this
// package issues is an EXACT-match "=<name>" form (see exactSessionTarget), deliberately so, and an
// exact target can never match a name tmux rewrote behind reed's back.
// This substitution is only the FIRST of the two rewrite halves tmux's session-name check performs;
// firstVisEncodedSessionNameByte below covers the second (the vis-encode class), and
// validateToldTmuxIdentity refuses both.
const rewrittenSessionNameChars = ".:"

// socketKeySeparators are the path separators a tmux -L socket key must not contain;
// see ServerName for what tmux does with one that does.
const socketKeySeparators = `/\`

// firstVisEncodedSessionNameByte returns a printable description of the first byte or rune in name
// that tmux would silently vis-encode into a multi-character escape, and whether one exists.
// This is the SECOND half of tmux's session-name rewrite (the first is the '.'/':' substitution
// above): after substituting those two characters, tmux passes the name through a
// vis(3)-style encoder, which rewrites every ASCII control character (below 0x20), DEL (0x7F), and
// every byte that is not part of a valid UTF-8 sequence into an escape SEQUENCE — verified live on
// tmux 3.6: TAB becomes the two literal characters `\t`, ESC becomes `\033`, DEL becomes `\177`,
// and the invalid byte 0xFF becomes `\377`, each with exit 0 and the session created under the
// rewritten name, unreachable by any exact-match "=<name>" target carrying the raw form.
// Valid multi-byte UTF-8 passes through the encoder untouched (verified live: "svc-åäö-⚙" is
// created verbatim and exact-matches), so this check refuses ONLY the vis-encode class and never a
// unicode worktree name.
func firstVisEncodedSessionNameByte(name string) (string, bool) {
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == utf8.RuneError && size == 1 {
			return fmt.Sprintf("the invalid UTF-8 byte 0x%02X", name[i]), true
		}
		if r < 0x20 || r == 0x7F {
			return strconv.QuoteRune(r), true
		}
		i += size
	}
	return "", false
}

// validateToldTmuxIdentity reports an error when the told SocketKey or SessionName is one the
// configured multiplexer cannot spend verbatim, naming the hub or worktree directory it derives from
// so the operator can act on it.
//
// It exists because reedengine.New validates no Geometry field by contract (see geometry.go) while
// tmux answers a malformed identity with silence rather than an error — a rewritten session name and
// an uncreatable socket both look, from every probe reed has, exactly like a server that has not
// finished booting. Without this check the boot loop burns its full attempt budget and reports a
// timeout that names neither cause, and in the session-name case it leaves the rewritten session
// running on the shared per-hub server with no reed verb able to address, or tear down, the thing it
// just created.
//
// For the session name the answer is a refusal, never a sanitization: rewriting '.' to '_' here
// would map sibling worktrees "svc.v2" and "svc_v2" onto one session name, and each worktree's
// engine would then adopt the other's panes — strictly worse than refusing to start. The socket key
// is the opposite case and is substituted at its derivation instead (ServerName), because its
// human-readable half carries no identity: the hash does, so substitution cannot collide.
// The socket check below is therefore a contract backstop rather than a path the hub-mode teller can
// reach — it binds the standalone tellers that will populate a Geometry themselves.
func validateToldTmuxIdentity(geom Geometry) error {
	if geom.SocketKey == "" {
		return fmt.Errorf("told geometry has an empty tmux socket key (hub %q)", geom.HubPath)
	}
	if i := strings.IndexAny(geom.SocketKey, socketKeySeparators); i >= 0 {
		return fmt.Errorf(
			"tmux cannot open a socket for key %q: it contains the path separator %q, so tmux would resolve it as a path whose parent does not exist (the key derives from hub %q)",
			geom.SocketKey, string(geom.SocketKey[i]), geom.HubPath)
	}

	if geom.SessionName == "" {
		return fmt.Errorf("told geometry has an empty tmux session name (worktree %q)", geom.WorktreeRoot)
	}
	if i := strings.IndexAny(geom.SessionName, rewrittenSessionNameChars); i >= 0 {
		return fmt.Errorf(
			"tmux will not create session %q verbatim: it contains %q, which tmux silently rewrites to \"_\" — rename the worktree directory %q so its name carries no %q",
			geom.SessionName, string(geom.SessionName[i]), geom.WorktreeRoot, rewrittenSessionNameChars)
	}
	if desc, found := firstVisEncodedSessionNameByte(geom.SessionName); found {
		return fmt.Errorf(
			"tmux will not create session %q verbatim: it contains %s, which tmux silently rewrites into a multi-character escape — rename the worktree directory %q so its name carries no control characters or invalid UTF-8",
			geom.SessionName, desc, geom.WorktreeRoot)
	}
	return nil
}

// cleanAbsHubPath resolves hubPath to its cleaned absolute form for stable hashing.
func cleanAbsHubPath(hubPath string) string {
	abs, err := filepath.Abs(hubPath)
	if err != nil {
		// filepath.Abs only fails when the current working directory cannot
		// be resolved; fall back to a cleaned version of the input so
		// ServerName stays total rather than panicking or returning an error
		// the caller would have to plumb through every call site.
		return filepath.Clean(hubPath)
	}
	return abs
}
