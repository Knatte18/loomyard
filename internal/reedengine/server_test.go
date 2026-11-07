// server_test.go tests the names reed hands tmux.
// It covers the -L socket key from ServerName, the session name from SessionName,
// and the refusal by validateToldTmuxIdentity of a told identity that tmux could not use verbatim.
// ServerName is the only derivation of the socket key, so these tests do not re-derive it.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// socketUnsafeChars matches the characters ServerName must never produce: ':', '/', '\', and space.
// All of them are unsafe in a tmux -L socket argument.
// '/' is in the set because tmux resolves -L as a filename under its per-user socket directory,
// so a separator in the key names a path whose parent does not exist.
// tmux answers that with a stderr line and exit 0, which no reed probe can tell apart from a slow boot.
var socketUnsafeChars = regexp.MustCompile(`[:/\\ ]`)

// TestServerName runs the properties ServerName promises, each a named step below:
// determinism; socket-safe characters, a hub at the filesystem root included; a bounded length for a long hub basename;
// distinct keys for distinct hubs; and the readable prefix plus 8-hex hash shape.
func TestServerName(t *testing.T) {
	tiergate.Require(t, 0)
	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"IsDeterministic", serverNameIsDeterministic},
		{"SocketSafe", serverNameSocketSafe},
		{"SocketSafeForAHubAtTheFilesystemRoot", serverNameSocketSafeForAHubAtTheFilesystemRoot},
		{"BoundedForALongHubBasename", serverNameBoundedForALongHubBasename},
		{"DistinctForDistinctHubsSharingBasename", serverNameDistinctForDistinctHubsSharingBasename},
		{"HasHubBasenameAndPrefix", serverNameHasHubBasenameAndPrefix},
	}
	for _, step := range steps {
		t.Run(step.name, step.run)
	}
}

func serverNameIsDeterministic(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-LYMHUB")
	got1 := ServerName(hub)
	got2 := ServerName(hub)
	if got1 != got2 {
		t.Errorf("ServerName not deterministic: %q != %q", got1, got2)
	}
}

func serverNameSocketSafe(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-LYMHUB")
	got := ServerName(hub)
	if socketUnsafeChars.MatchString(got) {
		t.Errorf("ServerName(%q) = %q contains a socket-unsafe character", hub, got)
	}
}

// serverNameSocketSafeForAHubAtTheFilesystemRoot guards a hub whose basename is the filesystem root.
// A git worktree one level under the filesystem root (a container's /workspace or /app) resolves its hub to "/".
// ServerName must not emit a key with a path separator:
// tmux cannot create a socket for it, and it does not report the failure.
// The hash half is asserted intact too, since substitution must not change hub identity.
func serverNameSocketSafeForAHubAtTheFilesystemRoot(t *testing.T) {
	root := string(filepath.Separator)

	got := ServerName(root)
	if socketUnsafeChars.MatchString(got) {
		t.Errorf("ServerName(%q) = %q contains a socket-unsafe character; tmux cannot open a socket for it", root, got)
	}
	if got == ServerName(filepath.Join(root, "elsewhere-LYMHUB")) {
		t.Errorf("ServerName(%q) collided with a distinct hub; substitution must not touch the identity half", root)
	}
}

// serverNameBoundedForALongHubBasename guards the length of the readable half of the key.
// An unbounded half let a long hub name make a -L key whose socket path overflows sockaddr_un's 108-byte sun_path.
// Measured on tmux 3.6 with the default "/tmp/tmux-<uid>/" directory, a 92-byte key works,
// and a 93-byte key fails with "(File name too long)" on every invocation, so the hub cannot boot.
// Two distinct long-named hubs are asserted apart too, since truncation must not change hub identity.
func serverNameBoundedForALongHubBasename(t *testing.T) {
	longBase := strings.Repeat("h", 200) + "-LYMHUB"
	parent := t.TempDir()

	got := ServerName(filepath.Join(parent, longBase))
	// The bound asserted here is the one maxSocketSafeBaseBytes promises, with headroom for a longer TMUX_TMPDIR.
	// The cap applies whatever the suffix's length.
	// This basename is already far past the cap before the suffix is considered.
	const wantAtMost = maxSocketSafeBaseBytes + len("lym-") + len("-") + 8
	if len(got) > wantAtMost {
		t.Errorf("ServerName(<200-char hub basename>) = %q (%d bytes); want at most %d — tmux cannot open a socket for an over-long key", got, len(got), wantAtMost)
	}
	if other := ServerName(filepath.Join(parent, "other", longBase)); got == other {
		t.Errorf("ServerName collided for two distinct hubs sharing a long basename: %q; truncation must not touch the identity half", got)
	}
}

//testtiming:keep pins the readable half of the socket key being cut at a rune boundary: under or at the limit untouched, a straddling rune dropped whole, zero keeps nothing, always valid UTF-8; its covering tests run this code without asserting it
func TestTruncateAtRuneBoundary(t *testing.T) {
	tiergate.Require(t, 0)
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

func serverNameDistinctForDistinctHubsSharingBasename(t *testing.T) {
	base := "loomyard-LYMHUB"
	hubA := filepath.Join(t.TempDir(), "a", base)
	hubB := filepath.Join(t.TempDir(), "b", base)

	got1 := ServerName(hubA)
	got2 := ServerName(hubB)
	if got1 == got2 {
		t.Errorf("ServerName collided for distinct hubs sharing a basename: %q == %q (hubA=%q, hubB=%q)", got1, got2, hubA, hubB)
	}
}

func serverNameHasHubBasenameAndPrefix(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-LYMHUB")
	got := ServerName(hub)
	want := "lym-loomyard-LYMHUB-"
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

//testtiming:keep pins SessionName deriving the session name as the worktree's basename; its covering tests run this code without asserting it
func TestSessionName_IsWorktreeBasename(t *testing.T) {
	tiergate.Require(t, 0)
	worktree := filepath.Join(t.TempDir(), "internal-reed")
	got := SessionName(worktree)
	want := "internal-reed"
	if got != want {
		t.Errorf("SessionName(%q) = %q, want %q", worktree, got, want)
	}
}

// TestValidateToldTmuxIdentity_SessionName covers the three ways tmux silently rewrites a session name.
// A '.' or ':' in the worktree name is rewritten to '_'.
// A '\' is doubled, so "bs\slash" becomes "bs\\slash".
// An ASCII control character, DEL, or an invalid UTF-8 byte is vis-encoded into a multi-character escape;
// TAB becomes the two literal characters `\t`, verified on tmux 3.6.
// Each rewrite makes the boot loop poll an exact "=<name>" target that never matches,
// so the boot times out after 20s with a message that names no cause,
// and the rewritten session keeps running on the shared per-hub server where no reed verb can address it.
// Each rewritten character class is asserted on its own, so a fix that catches only some of them fails here.
func TestValidateToldTmuxIdentity_SessionName(t *testing.T) {
	tiergate.Require(t, 0)
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
				SocketKey:    "lym-somehub-deadbeef",
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

// TestSanitizeSessionName pins the exact substitution SanitizeSessionName makes, row by row.
// Standalone mode derives its session name from the checkout directory name,
// so a plain checkout named "my.repo" must be sanitized before validateToldTmuxIdentity sees it.
// Hub mode never needs this, because hub worktree names are lym-created and slug-shaped.
// Pinning each row exactly also catches a sanitizer that mangles a name it should pass through.
func TestSanitizeSessionName(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain name passes through unchanged", "internal-reed", "internal-reed"},
		{"underscores and digits pass through", "svc_v2_3", "svc_v2_3"},
		{"space passes through, tmux leaves it alone", "two words", "two words"},
		{"valid multi-byte UTF-8 passes through", "svc-åäö-⚙", "svc-åäö-⚙"},
		{"dot is substituted", "my.repo", "my_repo"},
		{"every dot is substituted, not just the first", "app.test.git", "app_test_git"},
		{"colon is substituted", "svc:v2", "svc_v2"},
		{"backslash is substituted", `bs\slash`, "bs_slash"},
		{"control character is substituted", "svc\tv3", "svc_v3"},
		{"newline is substituted", "svc\nv3", "svc_v3"},
		{"escape is substituted", "svc\x1bv3", "svc_v3"},
		{"DEL is substituted", "svc\x7fv3", "svc_v3"},
		{"invalid UTF-8 byte is substituted one byte at a time", "svc-\xffv3", "svc-_v3"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSessionName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeSessionName(%q) = %q; want %q", tt.input, got, tt.want)
			}
			// Any character class validateToldTmuxIdentity refuses must be one SanitizeSessionName substitutes.
			// The "-<hash8>" suffix matches what standalonegeom.ReedGeometry appends.
			// So the assertion checks the string reed is actually told.
			geom := Geometry{
				SocketKey:    "lym-deadbeef",
				SessionName:  got + "-deadbeef",
				WorktreeRoot: filepath.Join("targets", tt.input),
				HubPath:      "state",
			}
			if err := validateToldTmuxIdentity(geom); err != nil {
				t.Errorf("validateToldTmuxIdentity(SessionName=%q) error = %v; want nil after sanitization of %q", geom.SessionName, err, tt.input)
			}
		})
	}
}

// TestValidateToldTmuxIdentity_SocketKey pins the refusal of a socket key that carries a path separator.
// Tmux answers such a key with a stderr line and exit 0, so the key is refused rather than handed to tmux.
// Hub mode cannot reach these cases, because ServerName substitutes separators out at derivation,
// so they need their own coverage here.
func TestValidateToldTmuxIdentity_SocketKey(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name      string
		socketKey string
		wantErr   bool
	}{
		{"derived hub-mode key", "lym-loomyard-LYMHUB-deadbeef", false},
		{"dots are fine in a socket key", "lym-svc.v2-LYMHUB-deadbeef", false},
		{"posix separator", "lym-/-deadbeef", true},
		{"windows separator", `lym-\-deadbeef`, true},
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

// TestValidateToldAnchorPath covers the refusal of an AnchorPath that is empty or relative.
// A bad AnchorPath fails silently rather than loudly: stateDir joins onto it, and every pane's tmux -c uses it,
// so an empty or relative value resolves both against the caller's working directory,
// and the op then succeeds against the wrong tree.
// Hub mode cannot reach these cases, because ReedGeometry always passes the absolute Location.AnchorPath().
func TestValidateToldAnchorPath(t *testing.T) {
	tiergate.Require(t, 0)
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
				SocketKey:    "lym-somehub-deadbeef",
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

// TestValidateToldWorktreeRootLive is the table test for the worktree-root liveness validator.
// Each row stats a real filesystem entry rather than only checking shape.
// The relative-value row points at server_test.go, which is always present in this package's source directory,
// so the assertion holds whatever the test process's working directory is.
// It proves the refusal fires on shape alone, not on the target's existence.
func TestValidateToldWorktreeRootLive(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState asserts the anchor refusal.
// The refusal lands at the same op boundary as the identity refusal: before the .lym directory is created.
// Creating that directory would litter the caller's working directory with reed state.
//
// The empty told anchor is what makes this assertion possible AND what makes it fragile: stateDir()
// then joins onto "", so the subject path is a bare ".lym" relative to the TEST PROCESS's working
// directory, i.e. this package's own source directory. Any earlier run of a binary predating
// validateToldAnchorPath — precisely the bug this guards — leaves .lym/reed.lock sitting there
// permanently, and the guard is then red forever in that checkout, with a message that reads as a
// live production regression rather than as stale scratch. That is not hypothetical: it was the
// state of this branch when the R5 review began (R5 review finding R5-F7).
// Clearing the directory before asserting, and again on cleanup, makes the assertion mean "this
// call created the file" — which is what it was always trying to measure — and keeps the test from
// leaving the source tree dirtier than it found it.
func TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState(t *testing.T) {
	tiergate.Require(t, 0)
	e := newTestEngine(t)
	e.geom.AnchorPath = ""

	cwdRelativeStateDir := e.stateDir()
	if cwdRelativeStateDir != lymdirs.DotLymDirName {
		t.Fatalf("stateDir() with an empty anchor = %q; want the bare %q this test's cleanup is written for",
			cwdRelativeStateDir, lymdirs.DotLymDirName)
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

// TestWithOpLock_ToldGeometry pins that withOpLock and withTryOpLock judge the told geometry at the op boundary.
// The check runs before every tmux round trip and every directory creation,
// so a bad identity creates no substrate reed cannot address.
// The test engine's tmux and shell paths do not exist.
// An op that reached tmux would fail with an exec error naming that path.
// Asserting the told-geometry refusal instead pins the ordering.
// A rewritten session name is refused, and the error names it.
// A worktree root that vanished, is a regular file, or names a non-existent standalone target is refused.
// The refusal is errWorktreeRootGone, and nothing is created.
// The test covers withOpLock and withTryOpLock as separate rows, so a fix to only one of the two helpers fails.
// The vanished-path message mentions --target-dir.
// The not-a-directory message carries no rename remedy, since nothing was renamed.
// The standalone and hub first-run shapes succeed and create the anchor's .lym directory.
// The standalone row fails if a later change gates the predicate on AnchorPath.
func TestWithOpLock_ToldGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		try          bool
		setup        func(t *testing.T, e *Engine) (mustNotExist []string)
		wantErr      bool
		wantSentinel bool
		wantText     []string
		notText      []string
	}{
		{
			name: "RefusesARewrittenSessionName",
			setup: func(t *testing.T, e *Engine) []string {
				e.geom.SessionName = "svc.v2"
				return nil
			},
			wantErr:  true,
			wantText: []string{"svc.v2"},
		},
		{
			name: "RefusesAVanishedWorktreeRoot",
			setup: func(t *testing.T, e *Engine) []string {
				vanished := filepath.Join(filepath.Dir(e.geom.WorktreeRoot), "renamed-away")
				e.geom.WorktreeRoot = vanished
				return []string{vanished, e.geom.AnchorPath, e.stateDir()}
			},
			wantErr:      true,
			wantSentinel: true,
		},
		{
			name: "TryLockRefusesAVanishedWorktreeRoot",
			try:  true,
			setup: func(t *testing.T, e *Engine) []string {
				vanished := filepath.Join(filepath.Dir(e.geom.WorktreeRoot), "renamed-away")
				e.geom.WorktreeRoot = vanished
				return []string{vanished, e.geom.AnchorPath, e.stateDir()}
			},
			wantErr:      true,
			wantSentinel: true,
		},
		{
			name: "RefusesAWorktreeRootThatIsARegularFile",
			setup: func(t *testing.T, e *Engine) []string {
				regularFile := filepath.Join(filepath.Dir(e.geom.WorktreeRoot), "worktree-is-a-file")
				if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				e.geom.WorktreeRoot = regularFile
				return nil
			},
			wantErr:      true,
			wantSentinel: true,
			notText:      []string{"renamed", "--target-dir"},
		},
		{
			name: "RefusesTheStandaloneNonExistentTargetShape",
			setup: func(t *testing.T, e *Engine) []string {
				parent := filepath.Dir(e.geom.WorktreeRoot)
				e.geom.WorktreeRoot = filepath.Join(parent, "does-not-exist-target")
				e.geom.AnchorPath = filepath.Join(parent, "does-not-exist-anchor")
				return []string{e.geom.WorktreeRoot, e.geom.AnchorPath}
			},
			wantErr:      true,
			wantSentinel: true,
			wantText:     []string{"--target-dir"},
		},
		{
			name: "SucceedsForTheStandaloneFirstRunShape",
			setup: func(t *testing.T, e *Engine) []string {
				if e.geom.AnchorPath == e.geom.WorktreeRoot {
					t.Fatalf("fixture assumption violated: AnchorPath %q must differ from WorktreeRoot %q", e.geom.AnchorPath, e.geom.WorktreeRoot)
				}
				if fileExists(e.geom.AnchorPath) {
					t.Fatalf("fixture assumption violated: AnchorPath %q must not exist yet", e.geom.AnchorPath)
				}
				return nil
			},
		},
		{
			name: "SucceedsForTheHubFirstRunShape",
			setup: func(t *testing.T, e *Engine) []string {
				e.geom.AnchorPath = e.geom.WorktreeRoot
				if fileExists(e.stateDir()) {
					t.Fatalf("fixture assumption violated: .lym %q must not exist yet", e.stateDir())
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			mustNotExist := tt.setup(t, e)

			ran := false
			body := func() error {
				ran = true
				return nil
			}
			var acquired bool
			var err error
			if tt.try {
				acquired, err = e.withTryOpLock(body)
			} else {
				err = e.withOpLock(body)
			}

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("withOpLock: %v", err)
				}
				if !ran {
					t.Errorf("withOpLock did not run the operation body")
				}
				if !fileExists(e.stateDir()) {
					t.Errorf("withOpLock did not create the anchor's .lym directory %q", e.stateDir())
				}
				return
			}
			if err == nil {
				t.Fatalf("lock helper = nil error, want a refusal")
			}
			if tt.try && acquired {
				t.Errorf("withTryOpLock() acquired = true, want false (told-geometry refusal)")
			}
			if ran {
				t.Errorf("lock helper ran the operation body despite an unusable told geometry")
			}
			if got := errors.Is(err, errWorktreeRootGone); got != tt.wantSentinel {
				t.Errorf("errors.Is(err, errWorktreeRootGone) = %v for %v; want %v", got, err, tt.wantSentinel)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q; want it to contain %q", err, want)
				}
			}
			for _, not := range tt.notText {
				if strings.Contains(err.Error(), not) {
					t.Errorf("error = %q; want it to not contain %q", err, not)
				}
			}
			for _, path := range append(mustNotExist, filepath.Join(e.stateDir(), reedLockFileName)) {
				if fileExists(path) {
					t.Errorf("lock helper created %q despite refusing the told geometry", path)
				}
			}
		})
	}
}

// fileExists reports whether path names an existing filesystem entry.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
