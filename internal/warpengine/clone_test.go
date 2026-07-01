// clone_test.go — unit tests for URL-derivation helpers and cloneRepo's error path.

package warpengine

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveHostName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "https with .git",
			url:  "https://github.com/u/repo.git",
			want: "repo",
		},
		{
			name: "https without .git",
			url:  "https://github.com/u/repo",
			want: "repo",
		},
		{
			name: "SCP form with .git",
			url:  "git@github.com:u/repo.git",
			want: "repo",
		},
		{
			name: "trailing slash",
			url:  "https://github.com/u/repo/",
			want: "repo",
		},
		{
			name: "Windows file path",
			url:  "C:\\path\\to\\repo.git",
			want: "repo",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveHostName(tt.url)
			if got != tt.want {
				t.Errorf("DeriveHostName(%q) = %q; want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestCloneRepo_InvalidURLFails asserts that cloneRepo's error on a bogus/nonexistent
// source URL is composed from local context (the attempted URL and destination, plus
// the git exit code) rather than git's own stderr text. No real git fixture is needed:
// a nonexistent source path is enough to make `git clone` fail immediately.
func TestCloneRepo_InvalidURLFails(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "cloned-repo")
	const url = "/does/not/exist/nonexistent-repo.git"

	err := cloneRepo(url, dest)
	if err == nil {
		t.Fatalf("cloneRepo(%q, %q) error = nil; want failure for a nonexistent source", url, dest)
	}
	if !strings.Contains(err.Error(), url) {
		t.Errorf("cloneRepo(%q, %q) error = %q; want substring %q (attempted URL)", url, dest, err.Error(), url)
	}
	// Compare against filepath.Base(dest) rather than the raw dest string: %q escapes
	// backslashes on Windows, so the literal OS-native dest path would never appear
	// unescaped in err.Error() even though the destination is faithfully reported.
	if destName := filepath.Base(dest); !strings.Contains(err.Error(), destName) {
		t.Errorf("cloneRepo(%q, %q) error = %q; want substring %q (destination)", url, dest, err.Error(), destName)
	}
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("cloneRepo(%q, %q) error = %q; want no %q substring (raw git stderr leak)", url, dest, err.Error(), "fatal:")
	}
}

func TestDeriveBoardURL(t *testing.T) {
	tests := []struct {
		name    string
		weftURL string
		want    string
	}{
		{
			name:    "weft with .git",
			weftURL: "https://github.com/u/weft.git",
			want:    "https://github.com/u/weft.wiki.git",
		},
		{
			name:    "weft without .git",
			weftURL: "https://github.com/u/weft",
			want:    "https://github.com/u/weft.wiki.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveBoardURL(tt.weftURL)
			if got != tt.want {
				t.Errorf("deriveBoardURL(%q) = %q; want %q", tt.weftURL, got, tt.want)
			}
		})
	}
}
