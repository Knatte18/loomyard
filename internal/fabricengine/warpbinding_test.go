// warpbinding_test.go holds the table tests for the warp-binding core.
// It covers the URL normalizer, the transport-identity collapse, the conflict resolver, and the on-disk round trip.
// It is package fabricengine, so it reaches the unexported helpers directly, carries no build tag, and spawns no git.

package fabricengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestNormalizeWarpURL covers normalizeWarpURL's trailing-slash strip, trailing-.git strip, and scheme/host lowercasing.
// It applies them in that order, and it checks that local paths and scp forms keep their case byte-for-byte.
//
//testtiming:keep normalizeWarpURL's trailing-slash and .git stripping and host lowercasing while local paths and scp forms keep their case; coverage of its blocks by other tests does not show an assertion of this
func TestNormalizeWarpURL(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"bare_https_unchanged", "https://github.com/u/repo", "https://github.com/u/repo"},
		{"trailing_slash_stripped", "https://github.com/u/repo/", "https://github.com/u/repo"},
		{"trailing_dot_git_stripped", "https://github.com/u/repo.git", "https://github.com/u/repo"},
		{"both_slash_and_dot_git_stripped", "https://github.com/u/repo.git/", "https://github.com/u/repo"},
		{"scheme_and_host_lowercased_path_case_kept", "https://GitHub.COM/U/Repo", "https://github.com/U/Repo"},
		// The scp form has no <scheme>:// prefix, so only the .git strip touches its case.
		// It must not equal the https spelling of the same repo.
		{"scp_form_case_untouched", "git@github.com:u/repo.git", "git@github.com:u/repo"},
		{"empty_string_stays_empty", "", ""},
		{"posix_local_path", "/home/u/fixtures/bare.git", "/home/u/fixtures/bare"},
		{"windows_local_path_drive_letter_case_kept", "C:/Code/Repo/", "C:/Code/Repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWarpURL(tt.raw)
			if got != tt.want {
				t.Errorf("normalizeWarpURL(%q) = %q; want %q", tt.raw, got, tt.want)
			}
		})
	}

	// The scp form must not collapse onto the https spelling of the same repo: normalization
	// alone is not transport-aware.
	scp := normalizeWarpURL("git@github.com:u/repo.git")
	https := normalizeWarpURL("https://github.com/u/repo")
	if scp == https {
		t.Errorf("normalizeWarpURL(scp) = %q; must not equal normalizeWarpURL(https) = %q", scp, https)
	}
}

// TestWarpURLTransportIdentity asserts that warpURLTransportIdentity collapses transport spellings of one repo.
// It maps them to the same identity string, and a different repo path stays distinct.
func TestWarpURLTransportIdentity(t *testing.T) {
	tiergate.Require(t, 0)
	https := warpURLTransportIdentity("https://github.com/u/repo.git")
	http := warpURLTransportIdentity("http://github.com/u/repo")
	scp := warpURLTransportIdentity("git@github.com:u/repo")
	other := warpURLTransportIdentity("https://github.com/u/other")

	if https != http {
		t.Errorf("warpURLTransportIdentity(https) = %q; want equal to warpURLTransportIdentity(http) = %q", https, http)
	}
	if https != scp {
		t.Errorf("warpURLTransportIdentity(https) = %q; want equal to warpURLTransportIdentity(scp) = %q", https, scp)
	}
	if https == other {
		t.Errorf("warpURLTransportIdentity(https) = %q; must not equal warpURLTransportIdentity(other repo) = %q", https, other)
	}
}

// TestResolveEffectiveWarpURL covers every row of the conflict rule resolveEffectiveWarpURL encodes.
// The rows are absent/supplied, present/absent, matching, normalized-matching, transport swap, and differing repo.
//
//testtiming:keep every row of the warp-binding conflict rule; coverage of its blocks by other tests does not show an assertion of this
func TestResolveEffectiveWarpURL(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		recorded     string
		found        bool
		supplied     string
		wantEffFn    func(recorded, supplied string) string
		wantWrite    bool
		wantErr      bool
		wantErrIs    error
		wantErrParts []string
	}{
		{
			name:         "absent_and_absent_errors",
			recorded:     "",
			found:        false,
			supplied:     "",
			wantWrite:    false,
			wantErr:      true,
			wantErrIs:    ErrNoWarpBinding,
			wantErrParts: []string{"lym fabric clone <weft-url> <warp-url>"},
		},
		{
			name:     "absent_and_supplied_writes",
			recorded: "",
			found:    false,
			supplied: "https://github.com/u/r",
			wantEffFn: func(_, supplied string) string {
				return supplied
			},
			wantWrite: true,
			wantErr:   false,
		},
		{
			name:     "present_and_absent_uses_recorded",
			recorded: "https://github.com/u/r",
			found:    true,
			supplied: "",
			wantEffFn: func(recorded, _ string) string {
				return recorded
			},
			wantWrite: false,
			wantErr:   false,
		},
		{
			name:     "present_and_supplied_byte_identical",
			recorded: "https://github.com/u/r",
			found:    true,
			supplied: "https://github.com/u/r",
			wantEffFn: func(_, supplied string) string {
				return supplied
			},
			wantWrite: false,
			wantErr:   false,
		},
		{
			name:     "present_and_supplied_normalized_match",
			recorded: "https://github.com/u/r",
			found:    true,
			supplied: "https://github.com/u/r.git/",
			wantEffFn: func(_, supplied string) string {
				return supplied
			},
			wantWrite: false,
			wantErr:   false,
		},
		{
			name:         "present_and_supplied_transport_swap_conflicts",
			recorded:     "https://github.com/u/r",
			found:        true,
			supplied:     "git@github.com:u/r.git",
			wantWrite:    false,
			wantErr:      true,
			wantErrIs:    ErrWarpBindingMismatch,
			wantErrParts: []string{"edit " + WarpBindingFileName + " in the hub's " + BoltDirName + " worktree and commit", "https://github.com/u/r", "git@github.com:u/r.git"},
		},
		{
			name:         "present_and_supplied_different_repo_conflicts",
			recorded:     "https://github.com/u/r",
			found:        true,
			supplied:     "https://github.com/u/other",
			wantWrite:    false,
			wantErr:      true,
			wantErrIs:    ErrWarpBindingMismatch,
			wantErrParts: []string{"https://github.com/u/r", "https://github.com/u/other"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effective, writeRecord, err := resolveEffectiveWarpURL(tt.recorded, tt.found, tt.supplied)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveEffectiveWarpURL(%q, %v, %q) error = nil; want error", tt.recorded, tt.found, tt.supplied)
				}
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("resolveEffectiveWarpURL(%q, %v, %q) error = %v; want errors.Is %v", tt.recorded, tt.found, tt.supplied, err, tt.wantErrIs)
				}
				for _, part := range tt.wantErrParts {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("resolveEffectiveWarpURL(%q, %v, %q) error = %q; want substring %q", tt.recorded, tt.found, tt.supplied, err.Error(), part)
					}
				}
				if effective != "" {
					t.Errorf("resolveEffectiveWarpURL(%q, %v, %q) effective = %q; want empty on error", tt.recorded, tt.found, tt.supplied, effective)
				}
				if writeRecord {
					t.Errorf("resolveEffectiveWarpURL(%q, %v, %q) writeRecord = true; want false on error", tt.recorded, tt.found, tt.supplied)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveEffectiveWarpURL(%q, %v, %q) error = %v; want nil", tt.recorded, tt.found, tt.supplied, err)
			}
			wantEff := tt.wantEffFn(tt.recorded, tt.supplied)
			if effective != wantEff {
				t.Errorf("resolveEffectiveWarpURL(%q, %v, %q) effective = %q; want %q", tt.recorded, tt.found, tt.supplied, effective, wantEff)
			}
			if writeRecord != tt.wantWrite {
				t.Errorf("resolveEffectiveWarpURL(%q, %v, %q) writeRecord = %v; want %v", tt.recorded, tt.found, tt.supplied, writeRecord, tt.wantWrite)
			}
		})
	}
}

// TestWarpBindingReadWrite round-trips writeWarpBinding and readWarpBinding over a t.TempDir() board directory.
// It covers the absent, written, whitespace-only, and overwrite cases.
func TestWarpBindingReadWrite(t *testing.T) {
	tiergate.Require(t, 0)
	boardDir := t.TempDir()

	if url, found := readWarpBinding(boardDir); found || url != "" {
		t.Fatalf("readWarpBinding(%q) = (%q, %v); want (\"\", false) with no %s present", boardDir, url, found, WarpBindingFileName)
	}

	const warpURL = "https://github.com/u/repo"
	if err := writeWarpBinding(boardDir, warpURL); err != nil {
		t.Fatalf("writeWarpBinding(%q, %q) error = %v; want nil", boardDir, warpURL, err)
	}
	if url, found := readWarpBinding(boardDir); !found || url != warpURL {
		t.Fatalf("readWarpBinding(%q) = (%q, %v); want (%q, true)", boardDir, url, found, warpURL)
	}

	path := filepath.Join(boardDir, WarpBindingFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v; want nil", path, err)
	}
	if string(data) != warpURL+"\n" {
		t.Errorf("file content = %q; want %q (URL plus exactly one trailing newline)", string(data), warpURL+"\n")
	}

	// A whitespace-only file is treated as absent.
	if err := os.WriteFile(path, []byte("   \n\t\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v; want nil", path, err)
	}
	if url, found := readWarpBinding(boardDir); found || url != "" {
		t.Errorf("readWarpBinding(%q) after whitespace-only write = (%q, %v); want (\"\", false)", boardDir, url, found)
	}

	// A second write over an existing file replaces its content rather than appending.
	const secondURL = "https://github.com/u/other"
	if err := writeWarpBinding(boardDir, secondURL); err != nil {
		t.Fatalf("writeWarpBinding(%q, %q) error = %v; want nil", boardDir, secondURL, err)
	}
	if url, found := readWarpBinding(boardDir); !found || url != secondURL {
		t.Fatalf("readWarpBinding(%q) after second write = (%q, %v); want (%q, true)", boardDir, url, found, secondURL)
	}
}
