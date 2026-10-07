package burlermarker

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	t.Parallel()

	anchor := filepath.Join(string(filepath.Separator), "hub", "repo")
	target := filepath.Join(string(filepath.Separator), "work", "target")
	state := filepath.Join(string(filepath.Separator), "work", "state")
	outside := filepath.Join(string(filepath.Separator), "elsewhere", "review.md")

	tests := []struct {
		name       string
		root, base string
		review     string
		want       string
		wantErr    []string
	}{
		{
			name: "hub path under _lyx mirrors under .lyx",
			root: anchor, base: anchor,
			review: filepath.Join(anchor, "_lyx", "reviews", "webster", "round-2-review.md"),
			want:   filepath.Join(anchor, ".lyx", "reviews", "webster", "round-2-review.md.ready"),
		},
		{
			name: "hub-root path not under _lyx keeps its subpath",
			root: anchor, base: anchor,
			review: filepath.Join(anchor, "docs", "review.md"),
			want:   filepath.Join(anchor, ".lyx", "docs", "review.md.ready"),
		},
		{
			name: "relative review path resolves against root",
			root: anchor, base: anchor,
			review: filepath.Join("_lyx", "reviews", "x-review.md"),
			want:   filepath.Join(anchor, ".lyx", "reviews", "x-review.md.ready"),
		},
		{
			name: "standalone marker sits in the state directory at the path relative to the target",
			root: target, base: state,
			review: filepath.Join(target, "notes", "review.md"),
			want:   filepath.Join(state, ".lyx", "notes", "review.md.ready"),
		},
		{
			name: "path outside root is refused naming both paths and the way forward",
			root: target, base: state,
			review:  outside,
			wantErr: []string{outside, target, "place the review path under " + target},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Path(tt.root, tt.base, tt.review)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Path() = %q, nil; want an error", got)
				}
				for _, part := range tt.wantErr {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("error %q does not contain %q", err, part)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Path() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("Path() = %q; want %q", got, tt.want)
			}
		})
	}
}
