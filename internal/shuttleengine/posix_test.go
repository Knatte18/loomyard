// posix_test.go tables PosixPath's conversion and rejection cases: drive
// root, spaces, forward-slash input, UNC rejection, and relative rejection.

package shuttleengine

import "testing"

func TestPosixPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"drive root", `C:\a\c`, "/c/a/c", false},
		{"spaces", `C:\a b\c`, "/c/a b/c", false},
		{"forward slashes", `C:/a/c`, "/c/a/c", false},
		{"lowercase drive letter input", `d:\tools\x`, "/d/tools/x", false},
		{"UNC path", `\\host\share\c`, "", true},
		{"relative path", `a\c`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PosixPath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("PosixPath(%q) = %q, nil; want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("PosixPath(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("PosixPath(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}
