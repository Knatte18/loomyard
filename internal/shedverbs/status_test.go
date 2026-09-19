// status_test.go covers the generic status body's both told absent-file dispositions,
// StatusExtras merging and its un-re-prefixed error, the told decode prefix, the told lock-dir
// boolean's effect over an as-yet-uncreated parent directory, and the --watch tail's rendering,
// dedupe, and told-label properties.

package shedverbs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func statusTexts() VerbTexts {
	return VerbTexts{Status: VerbText{Use: "status", Short: "status of the fake shed"}}
}

// TestStatusCmd_AbsentFile_Refuse covers the refusing disposition: the told message lands on the
// error envelope. The status-lock path's parent already exists, matching the state both shipped
// consumers are in whenever this disposition actually fires.
func TestStatusCmd_AbsentFile_Refuse(t *testing.T) {
	paths := newTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusLockPath), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}

	spec := &Spec{
		StatusPath:      paths.StatusPath,
		StatusLockPath:  paths.StatusLockPath,
		DecodeErrPrefix: "loom:",
		AbsentStatus:    AbsentDisposition{Refuse: true, RefuseMessage: "loom: no status file; run \"lyx loom start\" first"},
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if env["error"] != spec.AbsentStatus.RefuseMessage {
		t.Errorf("error = %v; want %q", env["error"], spec.AbsentStatus.RefuseMessage)
	}
}

// TestStatusCmd_AbsentFile_FoundFalse covers the non-refusing disposition: the success envelope
// carries exactly the two keys found and status_path, no core key and no extras key.
func TestStatusCmd_AbsentFile_FoundFalse(t *testing.T) {
	paths := newTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusLockPath), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}

	spec := &Spec{
		StatusPath:      paths.StatusPath,
		StatusLockPath:  paths.StatusLockPath,
		DecodeErrPrefix: "lifecyclecli:",
		AbsentStatus:    AbsentDisposition{Refuse: false},
		Hooks: Hooks{
			StatusExtras: func(st shedengine.Status) (map[string]any, error) {
				t.Fatal("StatusExtras must not run against an absent status file")
				return nil, nil
			},
		},
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	wantKeys := map[string]bool{"found": true, "status_path": true, "ok": true}
	if len(env) != len(wantKeys) {
		t.Fatalf("envelope keys = %v; want exactly %v", env, wantKeys)
	}
	for k := range env {
		if !wantKeys[k] {
			t.Errorf("unexpected key %q in absent-file success envelope: %v", k, env)
		}
	}
	if env["found"] != false {
		t.Errorf("found = %v; want false", env["found"])
	}
	if env["status_path"] != paths.StatusPath {
		t.Errorf("status_path = %v; want %q", env["status_path"], paths.StatusPath)
	}
}

// TestStatusCmd_StatusExtrasMerging covers a StatusExtras hook merging onto the four-key core.
func TestStatusCmd_StatusExtrasMerging(t *testing.T) {
	paths := newTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusLockPath), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}
	seedStatus(t, paths, "Only")

	spec := &Spec{
		StatusPath:      paths.StatusPath,
		StatusLockPath:  paths.StatusLockPath,
		DecodeErrPrefix: "loom:",
		Hooks: Hooks{
			StatusExtras: func(st shedengine.Status) (map[string]any, error) {
				return map[string]any{"slug": "some-slug"}, nil
			},
		},
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	for _, key := range []string{"current_producer", "state", "error", "activity", "slug", "ok"} {
		if _, ok := env[key]; !ok {
			t.Errorf("envelope missing key %q: %v", key, env)
		}
	}
	if env["slug"] != "some-slug" {
		t.Errorf("slug = %v; want %q", env["slug"], "some-slug")
	}
}

// TestStatusCmd_StatusExtrasErrorVerbatim covers a StatusExtras error reaching the error envelope
// verbatim, with no re-prefixing.
func TestStatusCmd_StatusExtrasErrorVerbatim(t *testing.T) {
	paths := newTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusLockPath), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}
	seedStatus(t, paths, "Only")

	wantErr := errors.New("extras exploded")
	spec := &Spec{
		StatusPath:      paths.StatusPath,
		StatusLockPath:  paths.StatusLockPath,
		DecodeErrPrefix: "loom:",
		Hooks: Hooks{
			StatusExtras: func(st shedengine.Status) (map[string]any, error) {
				return nil, wantErr
			},
		},
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if env["error"] != wantErr.Error() {
		t.Errorf("error = %v; want verbatim %q (no re-prefixing)", env["error"], wantErr.Error())
	}
}

// TestStatusCmd_DecodeErrorPrefixIsTold asserts the decode-error prefix is the told one.
func TestStatusCmd_DecodeErrorPrefixIsTold(t *testing.T) {
	paths := newTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusLockPath), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}
	// Malformed JSON: state.ReadJSONStrict fails to decode.
	if err := os.WriteFile(paths.StatusPath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write malformed status file: %v", err)
	}

	spec := &Spec{
		StatusPath:      paths.StatusPath,
		StatusLockPath:  paths.StatusLockPath,
		DecodeErrPrefix: "lifecyclecli:",
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	gotErr, _ := env["error"].(string)
	if !strings.HasPrefix(gotErr, "lifecyclecli: decode status file "+paths.StatusPath+": ") {
		t.Errorf("error = %q; want the told prefix", gotErr)
	}
}

// TestStatusCmd_EnsureStatusLockDir_True covers the told boolean's true arm: over a status-lock
// path whose parent directory does not yet exist, the parent is created and the read goes on to
// produce the told absent-file disposition.
func TestStatusCmd_EnsureStatusLockDir_True(t *testing.T) {
	paths := newTestPaths(t)
	// The status lock's parent directory does not exist yet: nest it one level below the
	// already-created temp dir so this test actually exercises a missing parent, rather than the
	// temp dir itself (which t.TempDir() already created).
	paths.StatusLockPath = filepath.Join(filepath.Dir(paths.StatusLockPath), "ephemeral", "status.lock")

	spec := &Spec{
		StatusPath:          paths.StatusPath,
		StatusLockPath:      paths.StatusLockPath,
		DecodeErrPrefix:     "lifecyclecli:",
		EnsureStatusLockDir: true,
		AbsentStatus:        AbsentDisposition{Refuse: false},
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0 (the read must reach the absent-file disposition): %v", code, env)
	}
	if env["found"] != false {
		t.Errorf("found = %v; want false", env["found"])
	}
	if _, err := os.Stat(filepath.Dir(paths.StatusLockPath)); err != nil {
		t.Errorf("status lock parent directory was not created: %v", err)
	}
}

// TestStatusCmd_EnsureStatusLockDir_False covers the told boolean's false arm: over the same kind
// of never-created parent, the parent stays absent and the read fails in lock acquisition instead
// of ever reaching found.
func TestStatusCmd_EnsureStatusLockDir_False(t *testing.T) {
	paths := newTestPaths(t)
	// The status lock's parent directory does not exist yet: nest it one level below the
	// already-created temp dir so this test actually exercises a missing parent, rather than the
	// temp dir itself (which t.TempDir() already created).
	paths.StatusLockPath = filepath.Join(filepath.Dir(paths.StatusLockPath), "ephemeral", "status.lock")

	spec := &Spec{
		StatusPath:          paths.StatusPath,
		StatusLockPath:      paths.StatusLockPath,
		DecodeErrPrefix:     "lifecyclecli:",
		EnsureStatusLockDir: false,
		AbsentStatus:        AbsentDisposition{Refuse: false},
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1 (lock acquisition must fail before found is ever produced): %v", code, env)
	}
	if _, err := os.Stat(filepath.Dir(paths.StatusLockPath)); err == nil {
		t.Error("status lock parent directory was created despite EnsureStatusLockDir being false")
	}
}

// TestRenderStatusLine_LabelAndOptionalTails asserts the told label appears in the rendered line
// and the optional last/wait tails are included only when non-empty.
func TestRenderStatusLine_LabelAndOptionalTails(t *testing.T) {
	st := shedengine.Status{
		State:    shedengine.StateRunning,
		Activity: shedengine.Activity{Now: "Plan-Write", Last: "Discussion-Write → done", Wait: "5m"},
	}
	line := RenderStatusLine("loom", st)
	want := "loom running | now Plan-Write | last Discussion-Write → done | wait 5m"
	if line != want {
		t.Errorf("RenderStatusLine = %q; want %q", line, want)
	}

	stNoTails := shedengine.Status{State: shedengine.StateRunning, Activity: shedengine.Activity{Now: "Plan-Write"}}
	line = RenderStatusLine("lifecycle", stNoTails)
	want = "lifecycle running | now Plan-Write"
	if line != want {
		t.Errorf("RenderStatusLine (no tails) = %q; want %q", line, want)
	}
}

// TestUnavailableLine_DedupesAcrossPolls asserts the composed unavailable line is byte-identical
// across polls for a given label -- the property PrintStatusLinesOnChange's dedupe relies on.
func TestUnavailableLine_DedupesAcrossPolls(t *testing.T) {
	a := UnavailableLine("loom")
	b := UnavailableLine("loom")
	if a != b {
		t.Errorf("UnavailableLine is not stable across calls: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "loom ") {
		t.Errorf("UnavailableLine = %q; want it to start with the told label", a)
	}
}

// TestPrintStatusLinesOnChange_ChangeOnlyPrinting drives PrintStatusLinesOnChange through a finite
// polls count with an injected sleep and no wall-clock wait, asserting change-only printing.
func TestPrintStatusLinesOnChange_ChangeOnlyPrinting(t *testing.T) {
	lines := []string{"a", "a", "b", "b", "b", "c"}
	i := 0
	poll := func() string {
		line := lines[i]
		i++
		return line
	}
	sleeps := 0
	sleep := func() { sleeps++ }

	var out strings.Builder
	PrintStatusLinesOnChange(&out, poll, sleep, len(lines))

	printed := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{"a", "b", "c"}
	if len(printed) != len(want) {
		t.Fatalf("printed = %v; want %v", printed, want)
	}
	for idx, w := range want {
		if printed[idx] != w {
			t.Errorf("printed[%d] = %q; want %q", idx, printed[idx], w)
		}
	}
	if sleeps != len(lines) {
		t.Errorf("sleeps = %d; want %d", sleeps, len(lines))
	}
}

// TestStatusCmd_WatchAgainstAbsentFile_ReturnsDispositionImmediately asserts a --watch against an
// absent status file returns the told disposition immediately instead of entering the tail (which
// would otherwise hang the test on an infinite poll loop).
func TestStatusCmd_WatchAgainstAbsentFile_ReturnsDispositionImmediately(t *testing.T) {
	paths := newTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusLockPath), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}

	spec := &Spec{
		StatusPath:      paths.StatusPath,
		StatusLockPath:  paths.StatusLockPath,
		DecodeErrPrefix: "loom:",
		AbsentStatus:    AbsentDisposition{Refuse: true, RefuseMessage: "loom: no status file"},
	}

	env, code := execEnvelope(t, statusCmd(statusTexts(), spec), []string{"--watch"})
	if code != 1 {
		t.Fatalf("exit code = %d; want 1 (the test would otherwise hang inside the tail)", code)
	}
	if env["error"] != spec.AbsentStatus.RefuseMessage {
		t.Errorf("error = %v; want %q", env["error"], spec.AbsentStatus.RefuseMessage)
	}
}

// TestEnsureStatusLockDir_ErrorReusesToldPrefix asserts ensureStatusLockDir's own error text
// reuses the told DecodeErrPrefix, reproducing loomcli's existing wording for loom's spec.
func TestEnsureStatusLockDir_ErrorReusesToldPrefix(t *testing.T) {
	dir := t.TempDir()
	// A regular file where a directory must go forces MkdirAll to fail.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	statusLockPath := filepath.Join(blocker, "status.lock")

	err := ensureStatusLockDir("loom:", statusLockPath)
	if err == nil {
		t.Fatal("expected an error when the lock's parent cannot be created")
	}
	if !strings.HasPrefix(err.Error(), "loom: create the status lock's directory ") {
		t.Errorf("error = %q; want the told prefix", err.Error())
	}
}
