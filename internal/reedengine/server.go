// server.go derives the tmux identity: the server name, also the -L socket name, and the worktree's session name.
// hubgeom and standalonegeom fill Geometry.SocketKey and Geometry.SessionName from these,
// and this package only reads those told fields back.
// It also carries validateToldTmuxIdentity, the pre-flight that refuses a told identity tmux cannot use verbatim.
// Without that check, an unusable identity would surface only as a boot timeout.
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

// errWorktreeRootGone is the one signal the resize watch loop uses to decide the told Geometry.WorktreeRoot is gone.
// It means provably gone, not merely unreadable.
// It is matched with errors.Is, never by substring.
// The refusal messages that wrap it are operator-facing prose, free to be reworded,
// and a substring match would couple the watch loop's cadence to that prose.
// Only validateToldWorktreeRootLive's two terminal cases wrap it:
// os.Stat failing with fs.ErrNotExist, and the path existing but not being a directory.
// Every other refusal from that function is a plain error that does not match it:
// an empty or relative value, or a stat failure other than not-exist, such as EACCES or EIO.
// So a momentary stat blip or a caller bug cannot strand a healthy session in the watch loop's dormant cadence.
var errWorktreeRootGone = errors.New("told worktree root is gone")

// ServerName returns the deterministic tmux server name for the hub: "lym-<basename>-<hash>".
// The hash ensures distinct hubs get distinct names.
// The basename half is only for human readability;
// uniqueness rests entirely on the hash of the cleaned absolute hub path.
// Any path separator in the basename is substituted rather than passed through.
// filepath.Base returns a bare separator for a filesystem root ("/" on POSIX, "\\" on Windows).
// That happens whenever a worktree sits one level under the filesystem root.
// tmux resolves a -L key as a filename under its per-user socket directory,
// so a key with a separator names a path whose parent does not exist.
// tmux then prints "error creating <path>" and still exits 0 (verified live on tmux 3.6).
// No probe reed has can tell that apart from a slow boot.
// The readable half is capped too, at maxSocketSafeBaseBytes.
// The whole socket path must fit sockaddr_un's 108-byte sun_path.
// Substituting and truncating both keep the key usable, and neither can collide, because the hash is untouched.
func ServerName(hubPath string) string {
	abs := cleanAbsHubPath(hubPath)
	base := truncateAtRuneBoundary(socketSafeBase(filepath.Base(abs)), maxSocketSafeBaseBytes)
	sum := sha256.Sum256([]byte(abs))
	shortHash := hex.EncodeToString(sum[:])[:8]
	return "lym-" + base + "-" + shortHash
}

// maxSocketSafeBaseBytes caps the human-readable half of a ServerName key, in bytes.
// Bytes is the unit the kernel's limit is expressed in, so the bound holds for a multi-byte hub name too.
//
// A tmux -L key names a socket file inside the per-user socket directory.
// That whole path must fit sockaddr_un.sun_path, which is 108 bytes.
// On tmux 3.6, with the default "/tmp/tmux-<uid>/" directory, a 93-byte key fails on every invocation.
// The error is "File name too long", and `lym reed up` cannot boot the hub.
// A ServerName key is the base plus 13 bytes of fixed framing.
// The cap leaves headroom for a socket directory considerably longer than the default (TMUX_TMPDIR).
const maxSocketSafeBaseBytes = 48

// truncateAtRuneBoundary returns s cut to at most maxBytes bytes, never splitting a UTF-8 rune.
// A trailing rune that would straddle the limit is dropped whole rather than left half-written.
// A socket key carrying a broken rune is no more usable than an over-long one.
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
// Both separators are substituted on every platform, so a separator-free contract does not depend on GOOS.
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

// sanitizedSessionNameChars is the union of the two literal-character classes tmux rewrites in a session name.
// It concatenates the two constants that declare those classes rather than repeating their literals,
// so a character added to either class is sanitized automatically,
// and the sanitizer cannot fall behind the identity check's refusals.
const sanitizedSessionNameChars = rewrittenSessionNameChars + doubledSessionNameChars

// SanitizeSessionName returns name with each character that tmux silently rewrites in a session name replaced by '_'.
// A caller that builds a session name from an uncontrolled directory basename can use this to pass the identity check.
//
// The alphabet lives beside the identity check, because a second copy would drift from it,
// and the drift would surface only at `up` time.
//
// Substituting is correct only where the readable half carries no identity.
// Standalone mode appends a "-<hash8>" of the normalized target path to the session name.
// Sanitizing therefore costs nothing: "my.repo" and "my_repo" still cannot collapse onto one session.
// Hub mode does not sanitize;
// it refuses the identity instead, because a hub session name is the bare worktree slug with no hash beside it.
// Rewriting '.' there would map sibling worktrees "svc.v2" and "svc_v2" onto one session,
// so each worktree's engine would adopt the other's panes.
func SanitizeSessionName(name string) string {
	var sanitized strings.Builder
	sanitized.Grow(len(name))
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		// An invalid UTF-8 byte is substituted here, one byte at a time, rather than left to strings.Map.
		// strings.Map re-encodes it as U+FFFD: three printable, valid bytes that would pass the vis-encode check.
		// The name would then no longer hold the byte the caller had.
		if r == utf8.RuneError && size == 1 {
			sanitized.WriteByte('_')
			i++
			continue
		}
		if r < 0x20 || r == 0x7F || strings.ContainsRune(sanitizedSessionNameChars, r) {
			sanitized.WriteByte('_')
			i += size
			continue
		}
		// Valid multi-byte UTF-8 is written through untouched.
		// tmux's vis pass leaves it alone (verified live on tmux 3.6).
		// A unicode directory name must therefore survive sanitization as itself.
		sanitized.WriteString(name[i : i+size])
		i += size
	}
	return sanitized.String()
}

// rewrittenSessionNameChars are the characters tmux silently rewrites to '_' inside a session name.
// tmux does not reject them: new-session exits 0 and creates the session under the rewritten name
// (verified live on tmux 3.6: "a.b" and "a:b" both become "a_b").
// The silence makes the ban necessary rather than cosmetic:
// every other -t target this package issues is an exact-match "=<name>" form,
// and an exact target never matches a name tmux rewrote.
// This is the first of tmux's three session-name rewrite classes; the other two follow below.
const rewrittenSessionNameChars = ".:"

// doubledSessionNameChars are the characters tmux silently DOUBLES inside a session name.
// There is exactly one: the backslash.
// tmux runs the name through a vis(3)-style encoder, which doubles '\' to "\\" unless its caller passes VIS_NOSLASH;
// tmux's session_check_name does not pass it.
// Verified live on tmux 3.6, a worktree directory named "bs\slash" creates the session "bs\\slash" with exit 0.
// The exact target "=bs\slash" then misses it forever,
// so the session is stranded on the shared per-hub server, where no reed verb can address or tear it down.
// This class is kept apart from the substituted class, because the two need different operator-facing text:
// one is replaced with '_' and this one is doubled.
// It is also kept apart from the control-byte class.
// The vis-encode check must keep passing '\', a printable, valid UTF-8 byte.
// The identity check runs this class last, after the vis-encode check.
// Its message prints the offending names unescaped.
// Printing them unescaped is safe only once control characters, DEL and invalid UTF-8 are ruled out.
// Together with the other two classes, this one completes the ban.
// A round-trip sweep of printable ASCII on tmux 3.6 found '.', ':' and '\' to be the only rewritten characters.
const doubledSessionNameChars = `\`

// socketKeySeparators are the path separators a tmux -L socket key must not contain:
// tmux would resolve such a key as a filesystem path.
const socketKeySeparators = `/\`

// firstVisEncodedSessionNameByte describes the first byte or rune in name that tmux rewrites.
// It reports whether one exists.
// tmux passes a session name through a vis(3)-style encoder.
// The encoder rewrites every ASCII control character, DEL, and every byte outside a valid UTF-8 sequence.
// Each becomes an escape such as `\t` or `\177`.
// The session is still created under the rewritten name.
// An exact-match target with the raw name cannot reach it.
// Valid multi-byte UTF-8 passes through the encoder unchanged, so a non-ASCII name is never refused.
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
// The engine constructor validates no Geometry field by contract.
// tmux answers a malformed identity with silence rather than an error.
// A rewritten session name and an uncreatable socket both look like a server that has not finished booting.
// Without this check the boot loop burns its full attempt budget and reports a timeout that names neither cause.
// For a rewritten session name, the session is left running on the shared per-hub server.
// No reed verb can address or tear it down.
//
// For the session name the answer is a refusal, never a sanitization.
// Rewriting '.' to '_' would map sibling worktrees "svc.v2" and "svc_v2" onto one session name.
// Each worktree's engine would then adopt the other's panes, which is strictly worse than refusing to start.
// A socket key is sanitized where it is derived instead.
// Its hash carries the identity, so substitution cannot collide.
// The socket check is therefore a contract backstop for callers that populate a Geometry themselves.
// It is not a path every caller reaches.
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

// validateToldAnchorPath reports an error when the told AnchorPath is one this engine cannot spend.
// It guards the AnchorPath field, whose failure is silent rather than loud.
//
// AnchorPath is the base the state directory joins onto and the -c every pane is spawned with.
// An empty or relative value does not fail; it succeeds against the wrong tree.
// The state files would resolve under the lym process's working directory instead of the worktree.
// tmux answers an empty -c by falling back to the invoking client's working directory.
// Every strand command would then run outside the anchor while reed reports success.
//
// Hub-derived geometry always carries an absolute AnchorPath.
// This check is therefore a backstop for callers that populate a Geometry themselves.
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

// validateToldWorktreeRootLive reports an error when the told WorktreeRoot is not a live directory.
// The op-lock chokepoint must not create state under a path that is no longer a worktree,
// so a root that has gone stops the operation here.
//
// An empty or relative root is a caller bug, not proof the world changed, so it wraps no sentinel.
// Only the two terminal liveness outcomes below wrap errWorktreeRootGone.
// That sentinel is reserved for a root proven gone.
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
		// filepath.Abs only fails when the current working directory cannot be resolved.
		// Falling back to a cleaned version of the input keeps the function total:
		// it neither panics nor returns an error that every call site would have to plumb through.
		return filepath.Clean(hubPath)
	}
	return abs
}
