// clone_test.go — unit tests for URL-derivation helpers.

package warpengine

import (
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
