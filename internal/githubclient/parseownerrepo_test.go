// parseownerrepo_test.go is the table-driven test suite for ParseOwnerRepo, covering every accepted
// shape and every rejected one.
// This file carries no build tag -- it spawns nothing, so the Test Tier Purity Invariant leaves it in
// Tier 1.

package githubclient

import "testing"

// TestParseOwnerRepo covers every accepted remote-URL shape and every rejected one, asserting that
// the three distinct-message error cases (non-GitHub host, no owner/repo pair, unparseable garbage)
// each surface a message distinct from the other two.
func TestParseOwnerRepo(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "SSH form with .git suffix",
			remoteURL: "git@github.com:Knatte18/loomyard.git",
			wantOwner: "Knatte18",
			wantRepo:  "loomyard",
		},
		{
			name:      "SSH form without .git suffix",
			remoteURL: "git@github.com:Knatte18/loomyard",
			wantOwner: "Knatte18",
			wantRepo:  "loomyard",
		},
		{
			name:      "HTTPS form with .git suffix",
			remoteURL: "https://github.com/Knatte18/loomyard.git",
			wantOwner: "Knatte18",
			wantRepo:  "loomyard",
		},
		{
			name:      "HTTPS form without .git suffix",
			remoteURL: "https://github.com/Knatte18/loomyard",
			wantOwner: "Knatte18",
			wantRepo:  "loomyard",
		},
		{
			name:      "HTTPS form with a trailing slash",
			remoteURL: "https://github.com/Knatte18/loomyard/",
			wantOwner: "Knatte18",
			wantRepo:  "loomyard",
		},
		{
			name:      "non-GitHub host",
			remoteURL: "https://gitlab.com/Knatte18/loomyard.git",
			wantErr:   true,
		},
		{
			name:      "no owner/repo pair",
			remoteURL: "https://github.com/loomyard",
			wantErr:   true,
		},
		{
			name:      "empty string",
			remoteURL: "",
			wantErr:   true,
		},
		{
			name:      "outright garbage",
			remoteURL: "not a url at all !!!",
			wantErr:   true,
		},
	}

	// errorsByCase collects the error messages for the three cases whose messages must be mutually
	// distinct, keyed by test name.
	errorsByCase := map[string]string{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, err := ParseOwnerRepo(tt.remoteURL)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseOwnerRepo(%q) error = nil; want an error", tt.remoteURL)
				}
				if gotOwner != "" || gotRepo != "" {
					t.Errorf("ParseOwnerRepo(%q) = (%q, %q); want both empty alongside the error", tt.remoteURL, gotOwner, gotRepo)
				}
				errorsByCase[tt.name] = err.Error()
				return
			}

			if err != nil {
				t.Fatalf("ParseOwnerRepo(%q) error = %v; want nil", tt.remoteURL, err)
			}
			if gotOwner != tt.wantOwner || gotRepo != tt.wantRepo {
				t.Errorf("ParseOwnerRepo(%q) = (%q, %q); want (%q, %q)", tt.remoteURL, gotOwner, gotRepo, tt.wantOwner, tt.wantRepo)
			}
		})
	}

	distinctCases := []string{"non-GitHub host", "no owner/repo pair", "outright garbage"}
	for i, a := range distinctCases {
		for _, b := range distinctCases[i+1:] {
			if errorsByCase[a] == errorsByCase[b] {
				t.Errorf("ParseOwnerRepo error messages for %q and %q are identical (%q); want distinct messages", a, b, errorsByCase[a])
			}
		}
	}
}
