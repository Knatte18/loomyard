// server_test.go verifies ServerName's determinism, socket-safety, and per-hub uniqueness,
// SessionName's worktree-slug derivation, and validateToldTmuxIdentity's refusal of a told identity
// tmux could not spend verbatim.
// ServerName is the SINGLE derivation of the -L socket key: hubgeom.ReedGeometry calls it to fill
// Geometry.SocketKey, and Engine.Socket returns that field verbatim, so there is no second
// spelling here to cross-check it against.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
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

// TestServerName_BoundedForALongHubBasename is the regression guard for the R4 review's R4-F2: the
// readable half of the key was unbounded, so a long hub directory name produced a -L key whose
// socket path could not fit sockaddr_un's 108-byte sun_path. Measured live on tmux 3.6 with the
// default "/tmp/tmux-<uid>/": a 92-byte key works, a 93-byte one fails "(File name too long)" on
// every invocation and the hub cannot be booted at all.
// Two distinct long-named hubs are asserted apart alongside the bound, since truncation must not
// change hub identity any more than the separator substitution above does.
func TestServerName_BoundedForALongHubBasename(t *testing.T) {
	longBase := strings.Repeat("h", 200) + "-HUB"
	parent := t.TempDir()

	got := ServerName(filepath.Join(parent, longBase))
	// The measured ceiling is 92 for the default socket directory; the bound
	// asserted here is the one the cap actually promises, with headroom for a
	// longer TMUX_TMPDIR.
	const wantAtMost = maxSocketSafeBaseBytes + len("lyx-") + len("-") + 8
	if len(got) > wantAtMost {
		t.Errorf("ServerName(<200-char hub basename>) = %q (%d bytes); want at most %d — tmux cannot open a socket for an over-long key", got, len(got), wantAtMost)
	}
	if other := ServerName(filepath.Join(parent, "other", longBase)); got == other {
		t.Errorf("ServerName collided for two distinct hubs sharing a long basename: %q; truncation must not touch the identity half", got)
	}
}

func TestTruncateAtRuneBoundary(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		maxBytes int
		want     string
	}{
		{"under the limit is untouched", "short", 48, "short"},
		{"exactly at the limit is untouched", "abcd", 4, "abcd"},
		{"ascii is cut to the limit", "abcdefgh", 3, "abc"},
		{"a straddling rune is dropped whole", "ääää", 3, "ä"},
		{"a rune ending exactly at the limit is kept", "ääää", 4, "ää"},
		{"zero keeps nothing", "abc", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateAtRuneBoundary(tt.in, tt.maxBytes)
			if got != tt.want {
				t.Errorf("truncateAtRuneBoundary(%q, %d) = %q; want %q", tt.in, tt.maxBytes, got, tt.want)
			}
			if len(got) > tt.maxBytes {
				t.Errorf("truncateAtRuneBoundary(%q, %d) = %q (%d bytes); want at most %d", tt.in, tt.maxBytes, got, len(got), tt.maxBytes)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncateAtRuneBoundary(%q, %d) = %q; want valid UTF-8 (a rune must never be split)", tt.in, tt.maxBytes, got)
			}
		})
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

// TestValidateToldAnchorPath is the regression guard for the R4 review's R4-F3: the told-geometry
// pre-flight backstopped SocketKey and SessionName but not AnchorPath, the third field whose bad
// value fails SILENTLY rather than loudly — stateDir joins onto it and every pane's tmux -c is it,
// so an empty or relative value resolves both against the caller's own working directory and the op
// then SUCCEEDS against the wrong tree.
// Like the SocketKey rows above, the hub-mode teller cannot reach these cases (ReedGeometry always
// passes the absolute Location.AnchorPath()), which is exactly why they need their own coverage.
func TestValidateToldAnchorPath(t *testing.T) {
	tests := []struct {
		name       string
		anchorPath string
		wantErr    bool
	}{
		{"absolute hub-mode anchor", filepath.Join(string(filepath.Separator), "hub", "wt", "sub"), false},
		{"absolute worktree root", filepath.Join(string(filepath.Separator), "hub", "wt"), false},
		{"empty", "", true},
		{"bare relative", filepath.Join("wt", "sub"), true},
		{"dot-relative", "." + string(filepath.Separator) + "wt", true},
		{"parent-relative", ".." + string(filepath.Separator) + "wt", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geom := Geometry{
				SocketKey:    "lyx-somehub-deadbeef",
				SessionName:  "some-worktree",
				AnchorPath:   tt.anchorPath,
				WorktreeRoot: filepath.Join("hub", "some-worktree"),
			}
			err := validateToldAnchorPath(geom)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToldAnchorPath(AnchorPath=%q) error = %v; want error: %v", tt.anchorPath, err, tt.wantErr)
			}
		})
	}
}

// TestValidateToldWorktreeRootLive is the table test for the new liveness validator, modelled on
// TestValidateToldAnchorPath's shape but I/O-aware: each row stats a real filesystem entry rather
// than only checking shape.
// The relative-value row points at a name that exists relative to this package's own source
// directory (server_test.go, which is always present) rather than an absolute path made relative,
// so the assertion holds regardless of the test process's actual working directory — the row
// exists to prove the refusal fires on shape alone, not on the target's existence.
func TestValidateToldWorktreeRootLive(t *testing.T) {
	dir := t.TempDir()
	existingDir := filepath.Join(dir, "worktree")
	if err := os.Mkdir(existingDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	regularFile := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	vanished := filepath.Join(dir, "does-not-exist")

	tests := []struct {
		name         string
		worktreeRoot string
		wantErr      bool
		wantSentinel bool
	}{
		{"existing directory", existingDir, false, false},
		{"empty value", "", true, false},
		{"relative value", "server_test.go", true, false},
		{"path that does not exist", vanished, true, true},
		{"existing regular file", regularFile, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToldWorktreeRootLive(Geometry{WorktreeRoot: tt.worktreeRoot})
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToldWorktreeRootLive(WorktreeRoot=%q) error = %v; want error: %v", tt.worktreeRoot, err, tt.wantErr)
			}
			if got := errors.Is(err, errWorktreeRootGone); got != tt.wantSentinel {
				t.Errorf("validateToldWorktreeRootLive(WorktreeRoot=%q) errors.Is(err, errWorktreeRootGone) = %v; want %v", tt.worktreeRoot, got, tt.wantSentinel)
			}
		})
	}

	t.Run("vanished path message names both causes and asserts neither", func(t *testing.T) {
		err := validateToldWorktreeRootLive(Geometry{WorktreeRoot: vanished})
		if err == nil {
			t.Fatalf("validateToldWorktreeRootLive(WorktreeRoot=%q) = nil; want an error", vanished)
		}
		if !strings.Contains(err.Error(), vanished) {
			t.Errorf("error = %q; want it to quote the path %q", err, vanished)
		}
		if !strings.Contains(err.Error(), "--target-dir") {
			t.Errorf("error = %q; want it to mention --target-dir", err)
		}
		if strings.Contains(err.Error(), "the worktree was renamed") {
			t.Errorf("error = %q; want it to assert neither cause outright rather than claim a rename happened", err)
		}
	})

	t.Run("not-a-directory message carries no rename remedy", func(t *testing.T) {
		err := validateToldWorktreeRootLive(Geometry{WorktreeRoot: regularFile})
		if err == nil {
			t.Fatalf("validateToldWorktreeRootLive(WorktreeRoot=%q) = nil; want an error", regularFile)
		}
		if strings.Contains(err.Error(), "renamed") || strings.Contains(err.Error(), "--target-dir") {
			t.Errorf("error = %q; want no rename remedy for a not-a-directory refusal", err)
		}
	})
}

// TestValidateToldWorktreeRootLive_UnreadableParentIsNotTheSentinel provokes a real non-fs.ErrNotExist
// stat failure — EACCES on the parent directory — and asserts it refuses without matching the
// sentinel, per the only-proven-gone-carries-the-sentinel contract.
// Skipped on Windows, where a directory mode bit does not gate traversal the same way, and when
// running as root, since root ignores the permission bit entirely.
func TestValidateToldWorktreeRootLive_UnreadableParentIsNotTheSentinel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits do not gate traversal on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bit")
	}

	parent := t.TempDir()
	worktreeRoot := filepath.Join(parent, "worktree")
	if err := os.Mkdir(worktreeRoot, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() {
		// Restore the mode so t.TempDir's own cleanup can traverse and remove parent.
		if err := os.Chmod(parent, 0o755); err != nil {
			t.Fatalf("restore parent mode: %v", err)
		}
	})

	err := validateToldWorktreeRootLive(Geometry{WorktreeRoot: worktreeRoot})
	if err == nil {
		t.Fatal("validateToldWorktreeRootLive with an unreadable parent = nil; want an error")
	}
	if errors.Is(err, errWorktreeRootGone) {
		t.Errorf("validateToldWorktreeRootLive with an unreadable parent matched errWorktreeRootGone; want a plain non-sentinel refusal")
	}
}

// TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState asserts the anchor refusal lands at
// the same op boundary the identity refusal does — before the .lyx directory is created, which is
// the very act that would otherwise litter the caller's own working directory with reed state.
//
// The empty told anchor is what makes this assertion possible AND what makes it fragile: stateDir()
// then joins onto "", so the subject path is a bare ".lyx" relative to the TEST PROCESS's working
// directory, i.e. this package's own source directory. Any earlier run of a binary predating
// validateToldAnchorPath — precisely the bug this guards — leaves .lyx/reed.lock sitting there
// permanently, and the guard is then red forever in that checkout, with a message that reads as a
// live production regression rather than as stale scratch. That is not hypothetical: it was the
// state of this branch when the R5 review began (R5 review finding R5-F7).
// Clearing the directory before asserting, and again on cleanup, makes the assertion mean "this
// call created the file" — which is what it was always trying to measure — and keeps the test from
// leaving the source tree dirtier than it found it.
func TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState(t *testing.T) {
	e := newTestEngine(t)
	e.geom.AnchorPath = ""

	cwdRelativeStateDir := e.stateDir()
	if cwdRelativeStateDir != lyxdirs.DotLyxDirName {
		t.Fatalf("stateDir() with an empty anchor = %q; want the bare %q this test's cleanup is written for",
			cwdRelativeStateDir, lyxdirs.DotLyxDirName)
	}
	removeCwdRelativeStateDir := func() {
		if err := os.RemoveAll(cwdRelativeStateDir); err != nil {
			t.Fatalf("clear %q in the test process's working directory: %v", cwdRelativeStateDir, err)
		}
	}
	removeCwdRelativeStateDir()
	t.Cleanup(removeCwdRelativeStateDir)

	ran := false
	err := e.withOpLock(func() error {
		ran = true
		return nil
	})
	if err == nil {
		t.Fatalf("withOpLock with an empty told anchor path = nil; want a refusal")
	}
	if ran {
		t.Errorf("withOpLock ran the operation body despite an unusable told anchor path")
	}
	if got := filepath.Join(e.stateDir(), reedLockFileName); fileExists(got) {
		t.Errorf("withOpLock created the lock file %q despite refusing the told geometry", got)
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
