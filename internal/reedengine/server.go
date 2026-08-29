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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// errWorktreeRootGone is the ONE signal the resize watch loop uses to decide the told
// Geometry.WorktreeRoot is provably gone rather than merely unreadable.
// It is matched with errors.Is, never by substring — the refusal messages that wrap it are
// operator-facing prose, free to be reworded, and a substring match would silently couple the
// watch loop's cadence decision to that prose. Only validateToldWorktreeRootLive's two terminal
// cases wrap it: os.Stat failing with fs.ErrNotExist, and the path existing but not being a
// directory. Every other refusal from that function — an empty or relative value, or any other
// stat failure such as EACCES or EIO — is a plain error that does not match it, so a momentary
// stat blip or a caller bug cannot strand a healthy session in the watch loop's dormant cadence.
var errWorktreeRootGone = errors.New("told worktree root is gone")

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
// The readable half is bounded for the same reason and by the same logic (see
// maxSocketSafeBaseRunes): a tmux -L key is a filename inside the per-user socket directory, and
// that whole path must fit sockaddr_un's 108-byte sun_path, so an unbounded basename produces a key
// no tmux invocation can spend.
// Substituting and truncating both keep the key usable and neither can collide, since the hash is
// untouched.
func ServerName(hubPath string) string {
	abs := cleanAbsHubPath(hubPath)
	base := truncateAtRuneBoundary(socketSafeBase(filepath.Base(abs)), maxSocketSafeBaseBytes)
	sum := sha256.Sum256([]byte(abs))
	shortHash := hex.EncodeToString(sum[:])[:8]
	return "lyx-" + base + "-" + shortHash
}

// maxSocketSafeBaseBytes caps the human-readable half of a ServerName key, in BYTES — the unit the
// kernel's limit is expressed in, so the resulting bound holds for a multi-byte hub name too.
//
// A tmux -L key names a socket file inside the per-user socket directory, and that whole path must
// fit sockaddr_un.sun_path (108 bytes). Measured live on tmux 3.6 with the default
// "/tmp/tmux-<uid>/" directory: a 92-byte key works and a 93-byte one fails with
// "error creating <path> (File name too long)" on EVERY invocation, which makes the hub unusable —
// `lyx reed up` cannot boot it at all (R4 review finding R4-F2).
// A ServerName key is len(base)+13 bytes, so this cap holds every key at or under 61 bytes, leaving
// headroom for a socket directory considerably longer than the default (TMUX_TMPDIR).
const maxSocketSafeBaseBytes = 48

// truncateAtRuneBoundary returns s cut to at most maxBytes bytes, never splitting a UTF-8 rune.
// A trailing rune that would straddle the limit is dropped whole rather than left half-written,
// since a socket key carrying a broken rune is no more usable than an over-long one.
func truncateAtRuneBoundary(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := 0
	for i := range s {
		if i > maxBytes {
			break
		}
		cut = i
	}
	return s[:cut]
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
// This substitution is only the FIRST of tmux's three session-name rewrite classes;
// doubledSessionNameChars and firstVisEncodedSessionNameByte below cover the other two, and
// validateToldTmuxIdentity refuses all three.
const rewrittenSessionNameChars = ".:"

// doubledSessionNameChars are the characters tmux silently DOUBLES inside a session name, as
// distinct from the substituted class above.
// There is exactly one: the backslash. tmux runs the name through a vis(3)-style encoder, and vis
// doubles '\' to "\\" unless its caller passes VIS_NOSLASH — which tmux's session_check_name does
// not (verified live, tmux 3.6: a worktree directory named "bs\slash" creates the session
// "bs\\slash" with exit 0, and the exact target "=bs\slash" then misses it forever, leaving it
// squatting on the shared per-hub server where no reed verb can address or tear it down; R4 review
// finding R4-F1).
// It is kept apart from rewrittenSessionNameChars because the two need different operator-facing
// text — one is substituted with '_', this one is doubled — and apart from
// firstVisEncodedSessionNameByte because '\' is a printable, valid-UTF-8 byte that check must keep
// passing.
// validateToldTmuxIdentity deliberately runs this check LAST, after the vis-encode one: its message
// prints the offending names unescaped, so an operator sees the directory name they must actually
// rename rather than a %q rendering that doubles the very backslash at issue — and printing them
// unescaped is only safe once the vis-encode check has already ruled out control characters, DEL,
// and invalid UTF-8.
// Together with those two, this constant completes the ban: an exhaustive round-trip sweep of every
// printable ASCII byte (0x20-0x7E) through new-session and an exact-match has-session on tmux 3.6
// found exactly three rewritten characters — '.', ':' and '\' — with the control/DEL/invalid-UTF-8
// class making up the remainder.
const doubledSessionNameChars = `\`

// socketKeySeparators are the path separators a tmux -L socket key must not contain;
// see ServerName for what tmux does with one that does.
const socketKeySeparators = `/\`

// firstVisEncodedSessionNameByte returns a printable description of the first byte or rune in name
// that tmux would silently vis-encode into a multi-character escape, and whether one exists.
// This is one of the two rewrites tmux's vis(3) pass performs, the other being the backslash
// doubling above (doubledSessionNameChars); the '.'/':' substitution runs before both.
// After substituting those two characters, tmux passes the name through a
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
	// Unescaped %s, not %q, and last in the sequence — see doubledSessionNameChars.
	if strings.ContainsAny(geom.SessionName, doubledSessionNameChars) {
		return fmt.Errorf(
			`tmux will not create session "%s" verbatim: it contains a backslash, which tmux silently doubles to "\\" — rename the worktree directory "%s" so its name carries no backslash`,
			geom.SessionName, geom.WorktreeRoot)
	}
	return nil
}

// validateToldAnchorPath reports an error when the told AnchorPath is one this engine cannot spend,
// completing validateToldTmuxIdentity's backstop over the third Geometry field whose failure is
// silent rather than loud.
//
// AnchorPath is the base stateDir joins onto and the -c every pane is spawned with, so an empty or
// relative value does not fail — it succeeds against the WRONG tree. stateDir would resolve
// reed.json/reed.lock under the lyx process's own working directory instead of the worktree, and
// tmux answers an empty -c by falling back to the invoking client's cwd, which is exactly the
// failure launchStrandLocked's explicit -c exists to prevent: every strand command running somewhere
// other than the anchor while reed reports success (R4 review finding R4-F3).
//
// Like the socket-key branch of validateToldTmuxIdentity, this is a contract backstop rather than a
// path the hub-mode teller can reach — hubgeom.ReedGeometry always passes the absolute
// Location.AnchorPath() — and it binds the standalone tellers that will populate a Geometry
// themselves.
func validateToldAnchorPath(geom Geometry) error {
	if geom.AnchorPath == "" {
		return fmt.Errorf("told geometry has an empty anchor path (worktree %q)", geom.WorktreeRoot)
	}
	if !filepath.IsAbs(geom.AnchorPath) {
		return fmt.Errorf(
			"told anchor path %q is relative: reed joins its state directory onto it and passes it as tmux's -c, so a relative value silently resolves against whatever working directory the caller happens to have (worktree %q)",
			geom.AnchorPath, geom.WorktreeRoot)
	}
	return nil
}

// validateToldWorktreeRootLive reports an error when the told WorktreeRoot is not a live
// directory, promoting WorktreeRoot from a decorative field (used only to derive the tmux
// session name and to stamp Strand.Worktree) to a load-bearing control-flow predicate: the
// op-lock chokepoint refuses to proceed, rather than conjuring state under a path that is no
// longer a worktree.
//
// A shape violation — empty or relative — is a caller bug, not proof the world changed, so
// neither wraps errWorktreeRootGone; only the two terminal liveness outcomes below do, matching
// the only-proven-gone-carries-the-sentinel contract the sentinel's own doc comment states.
func validateToldWorktreeRootLive(geom Geometry) error {
	if geom.WorktreeRoot == "" {
		return fmt.Errorf("told geometry has an empty worktree root")
	}
	if !filepath.IsAbs(geom.WorktreeRoot) {
		return fmt.Errorf(
			"told worktree root %q is relative: a relative value silently stats against whatever working directory the process happens to have",
			geom.WorktreeRoot)
	}

	info, statErr := os.Stat(geom.WorktreeRoot)
	if statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf(
				"%w: told worktree root %q does not exist — reed never creates its worktree root, so either a hub worktree was removed or renamed, or (in standalone mode) --target-dir names a directory that does not exist",
				errWorktreeRootGone, geom.WorktreeRoot)
		}
		return fmt.Errorf("could not determine whether told worktree root %q exists: %w", geom.WorktreeRoot, statErr)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: told worktree root %q is a file, not a directory", errWorktreeRootGone, geom.WorktreeRoot)
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
