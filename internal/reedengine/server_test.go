// server_test.go verifies ServerName's determinism, socket-safety, and per-hub uniqueness,
// SessionName's worktree-slug derivation, and validateToldTmuxIdentity's refusal of a told identity
// tmux could not spend verbatim.
// ServerName is the SINGLE derivation of the -L socket key: hubgeom.ReedGeometry calls it to fill
// Geometry.SocketKey, and Engine.Socket returns that field verbatim, so there is no second
// spelling here to cross-check it against.

package reedengine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// socketUnsafeChars matches the characters ServerName must never
// produce: ':', '/', '\', and space, all of which are unsafe in a tmux -L
// socket argument.
// '/' joined the set with the R2 review's R2-F3: tmux resolves -L as a filename under its per-user
// socket directory, so a separator in the key names a path whose parent does not exist — and tmux
// answers that with a stderr line and exit 0, which no reed probe can tell apart from a slow boot.
var socketUnsafeChars = regexp.MustCompile(`[:/\\ ]`)

func TestServerName_Deterministic(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-HUB")
	got1 := ServerName(hub)
	got2 := ServerName(hub)
	if got1 != got2 {
		t.Errorf("ServerName not deterministic: %q != %q", got1, got2)
	}
}

func TestServerName_SocketSafe(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-HUB")
	got := ServerName(hub)
	if socketUnsafeChars.MatchString(got) {
		t.Errorf("ServerName(%q) = %q contains a socket-unsafe character", hub, got)
	}
}

// TestServerName_SocketSafeForAHubAtTheFilesystemRoot is the regression guard for the R2 review's
// R2-F3: a git worktree one level under the filesystem root — a container's /workspace or /app —
// resolves its hub to "/", and filepath.Base("/") is "/", so ServerName used to emit a key
// containing a path separator that tmux cannot create a socket for (and does not report as a
// failure). The hash half is asserted intact alongside, since substitution must not change hub
// identity.
func TestServerName_SocketSafeForAHubAtTheFilesystemRoot(t *testing.T) {
	root := string(filepath.Separator)

	got := ServerName(root)
	if socketUnsafeChars.MatchString(got) {
		t.Errorf("ServerName(%q) = %q contains a socket-unsafe character; tmux cannot open a socket for it", root, got)
	}
	if got == ServerName(filepath.Join(root, "elsewhere-HUB")) {
		t.Errorf("ServerName(%q) collided with a distinct hub; substitution must not touch the identity half", root)
	}
}

func TestServerName_DistinctForDistinctHubsSharingBasename(t *testing.T) {
	base := "loomyard-HUB"
	hubA := filepath.Join(t.TempDir(), "a", base)
	hubB := filepath.Join(t.TempDir(), "b", base)

	got1 := ServerName(hubA)
	got2 := ServerName(hubB)
	if got1 == got2 {
		t.Errorf("ServerName collided for distinct hubs sharing a basename: %q == %q (hubA=%q, hubB=%q)", got1, got2, hubA, hubB)
	}
}

func TestServerName_HasHubBasenameAndPrefix(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-HUB")
	got := ServerName(hub)
	want := "lyx-loomyard-HUB-"
	if len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("ServerName(%q) = %q, want prefix %q", hub, got, want)
	}
	// Everything after the prefix must be exactly 8 lowercase hex chars.
	hash := got[len(want):]
	if len(hash) != 8 {
		t.Errorf("ServerName(%q) hash suffix = %q, want length 8", hub, hash)
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("ServerName(%q) hash suffix %q has non-hex char %c", hub, hash, c)
		}
	}
}

func TestSessionName_IsWorktreeBasename(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "internal-reed")
	got := SessionName(worktree)
	want := "internal-reed"
	if got != want {
		t.Errorf("SessionName(%q) = %q, want %q", worktree, got, want)
	}
}

// TestValidateToldTmuxIdentity_SessionName is the regression guard for the R2 review's BLOCKING
// finding, the R3 review's R3-F1, and the R4 review's R4-F1 — tmux's three session-name rewrite
// classes: a worktree directory whose name carries '.' or ':' produced a session name tmux silently
// rewrote to '_'; one carrying '\' produces a name tmux silently DOUBLES ("bs\slash" becomes
// "bs\\slash"); and one carrying an ASCII control character, DEL, or an invalid-UTF-8 byte produces
// a name tmux silently vis-encodes into a multi-character escape (TAB becomes the two literal
// characters `\t`; all verified live, tmux 3.6) — so the boot loop polled
// an exact "=<name>" target that could never match, timed out after 20s with a message naming no
// cause, and left the rewritten session running on the shared per-hub server where no reed verb
// could address or tear it down.
// Each rewritten character class is asserted individually rather than as one combined case, so a
// fix that catches only some of them fails here instead of passing.
func TestValidateToldTmuxIdentity_SessionName(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		wantErr     bool
	}{
		{"plain slug", "internal-reed", false},
		{"underscores and digits", "svc_v2_3", false},
		{"dash-heavy mill slug", "reed-shuttle-crucible-hardening", false},
		{"space is left alone by tmux", "two words", false},
		{"space-only name is left alone by tmux", " ", false},
		{"valid multi-byte UTF-8 is left alone by tmux", "svc-åäö-⚙", false},
		{"literal U+FFFD is valid UTF-8 and left alone", "svc-�", false},
		{"format and target metacharacters are left alone", "a#b%c=d-e", false},
		{"quote, dollar and backtick are left alone by tmux", "q\"w $e `r", false},
		{"dot is rewritten by tmux", "svc.v2", true},
		{"colon is rewritten by tmux", "svc:v2", true},
		{"dot anywhere, not just the middle", "release-2.", true},
		{"backslash is doubled by tmux", `bs\slash`, true},
		{"backslash anywhere, not just the middle", `trailing\`, true},
		{"backslash beside an already-banned dot", `svc.v2\3`, true},
		{"tab is vis-encoded by tmux", "svc\tv3", true},
		{"newline is vis-encoded by tmux", "svc\nv3", true},
		{"escape is vis-encoded by tmux", "svc\x1bv3", true},
		{"DEL is vis-encoded by tmux", "svc\x7fv3", true},
		{"bell is vis-encoded by tmux", "svc\av3", true},
		{"invalid UTF-8 byte is vis-encoded by tmux", "svc-\xffv3", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geom := Geometry{
				SocketKey:    "lyx-somehub-deadbeef",
				SessionName:  tt.sessionName,
				WorktreeRoot: filepath.Join("hub", tt.sessionName),
			}
			err := validateToldTmuxIdentity(geom)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToldTmuxIdentity(SessionName=%q) error = %v; want error: %v", tt.sessionName, err, tt.wantErr)
			}
		})
	}
}

// TestValidateToldTmuxIdentity_SocketKey pins the contract backstop the standalone tellers of wave 3
// will be bound by: a socket key carrying a path separator is refused rather than handed to tmux,
// which answers such a key with a stderr line and exit 0.
// The hub-mode teller cannot reach these cases (ServerName substitutes separators out at the
// derivation), which is exactly why they need their own coverage here.
func TestValidateToldTmuxIdentity_SocketKey(t *testing.T) {
	tests := []struct {
		name      string
		socketKey string
		wantErr   bool
	}{
		{"derived hub-mode key", "lyx-loomyard-HUB-deadbeef", false},
		{"dots are fine in a socket key", "lyx-svc.v2-HUB-deadbeef", false},
		{"posix separator", "lyx-/-deadbeef", true},
		{"windows separator", `lyx-\-deadbeef`, true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geom := Geometry{
				SocketKey:    tt.socketKey,
				SessionName:  "some-worktree",
				WorktreeRoot: filepath.Join("hub", "some-worktree"),
				HubPath:      "hub",
			}
			err := validateToldTmuxIdentity(geom)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToldTmuxIdentity(SocketKey=%q) error = %v; want error: %v", tt.socketKey, err, tt.wantErr)
			}
		})
	}
}

// TestWithOpLock_RefusesARewrittenSessionNameBeforeTouchingTmux asserts the refusal lands at the op
// boundary, ahead of every tmux round trip and every directory creation — the property that keeps a
// bad identity from creating substrate reed cannot address.
// newTestEngine's tmux/shell paths deliberately do not exist, so any op that DID reach tmux would
// fail with an exec error naming that path; asserting the error is the identity refusal instead is
// what pins the ordering.
func TestWithOpLock_RefusesARewrittenSessionNameBeforeTouchingTmux(t *testing.T) {
	e := newTestEngine(t)
	e.geom.SessionName = "svc.v2"

	ran := false
	err := e.withOpLock(func() error {
		ran = true
		return nil
	})
	if err == nil {
		t.Fatalf("withOpLock with a rewritten session name = nil; want a refusal")
	}
	if ran {
		t.Errorf("withOpLock ran the operation body despite an unusable told session name")
	}
	if !strings.Contains(err.Error(), "svc.v2") {
		t.Errorf("withOpLock error = %q; want it to name the offending session name", err)
	}
	if got := filepath.Join(e.stateDir(), reedLockFileName); fileExists(got) {
		t.Errorf("withOpLock created the lock file %q despite refusing the told geometry", got)
	}
}

// fileExists reports whether path names an existing filesystem entry.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
