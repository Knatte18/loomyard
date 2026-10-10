package agentname

import (
	"strings"
	"testing"
)

//testtiming:keep pins the exact formatted string of each name form and its Parse round trip, which Resolve and the rejection cases never assert
func TestFormatParseRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct{ shortname, slug, role, want string }{
		{"ly", "", "orch", "ly:orch"},
		{"ly", "agent-naming", "driver-2", "ly:agent-naming:driver-2"},
		{"s1a2b3", "", "strand", "s1a2b3:strand"},
	}
	for _, tt := range tests {
		got, err := Format(tt.shortname, tt.slug, tt.role)
		if err != nil || got != tt.want {
			t.Fatalf("Format(%q,%q,%q) = %q, %v; want %q", tt.shortname, tt.slug, tt.role, got, err, tt.want)
		}
		n, err := Parse(got)
		if err != nil || n != (Name{tt.shortname, tt.slug, tt.role}) || n.String() != got {
			t.Fatalf("Parse(%q) = %+v, %v", got, n, err)
		}
	}
}

func TestValidationRejections(t *testing.T) {
	t.Parallel()

	for _, shortname := range []string{"", "a", "abcdefg", "1ab", "a-b", "a:b", "a.b", "a@b", "AB"} {
		if ValidateShortname(shortname) == nil {
			t.Errorf("ValidateShortname(%q) accepted", shortname)
		}
	}
	for _, v := range []string{"", "1a", "A", "a_b", "a:b", "a.b"} {
		if ValidateSlug(v) == nil {
			t.Errorf("ValidateSlug(%q) accepted", v)
		}
		if ValidateRole(v) == nil {
			t.Errorf("ValidateRole(%q) accepted", v)
		}
	}
	for _, name := range []string{"", "ly", "ly::orch", ":ly:orch", "ly:slug:", "ly:a:b:c", "ly:Bad:orch", "x:orch", "ly:1x"} {
		if _, err := Parse(name); err == nil {
			t.Errorf("Parse(%q) accepted", name)
		}
	}
	if _, err := Format("ly", "bad slug", "orch"); err == nil {
		t.Error("Format accepted bad slug")
	}
	if _, err := Format("ly", "ok", "Bad"); err == nil {
		t.Error("Format accepted bad role")
	}
}

func TestNumberRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		held []string
		want string
	}{
		{nil, "worker"},
		{[]string{"other"}, "worker"},
		{[]string{"worker"}, "worker-2"},
		{[]string{"worker", "worker-2"}, "worker-3"},
		{[]string{"worker", "worker-3"}, "worker-2"},
		{[]string{"worker-2", "worker"}, "worker-3"},
	}
	for _, tt := range tests {
		if got := NumberRole("worker", tt.held); got != tt.want {
			t.Errorf("NumberRole(held=%v) = %q, want %q", tt.held, got, tt.want)
		}
		if got := NumberRole("worker", tt.held); !MatchesRole(got, "worker") {
			t.Errorf("MatchesRole(%q, worker) = false, want true", got)
		}
	}

	for _, role := range []string{"other", "worker-1", "worker-x", "worker-", "worker-02", "workers", "workers-2"} {
		if MatchesRole(role, "worker") {
			t.Errorf("MatchesRole(%q, worker) = true, want false", role)
		}
	}
	if MatchesRole("conflicted", "conflict") {
		t.Error(`MatchesRole("conflicted", "conflict") = true, want false`)
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		shortname, slug, query string
		wantName               string
		wantErr                bool
		wantErrContains        string
	}{
		{name: "RoleQuery", shortname: "ly", slug: "task", query: "driver", wantName: "ly:task:driver"},
		{name: "FullQuery", shortname: "ly", slug: "task", query: "ly:task:orch", wantName: "ly:task:orch"},
		{name: "PrimeFullQuery", shortname: "ly", slug: "", query: "ly:orch", wantName: "ly:orch"},
		{
			name: "ForeignPrefix", shortname: "ly", slug: "task", query: "zz:other:orch", wantErr: true,
			wantErrContains: "way forward: pass the role segment alone, or run the command from the worktree the full name belongs to",
		},
		{name: "SlugMismatch", shortname: "ly", slug: "task", query: "ly:orch", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n, err := Resolve(tt.shortname, tt.slug, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve(%q,%q,%q) = %v; want an error", tt.shortname, tt.slug, tt.query, n)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("Resolve(%q,%q,%q) err = %v; want it to contain %q", tt.shortname, tt.slug, tt.query, err, tt.wantErrContains)
				}
				return
			}
			if err != nil || n.String() != tt.wantName {
				t.Fatalf("Resolve(%q,%q,%q) = %v, %v; want %q", tt.shortname, tt.slug, tt.query, n, err, tt.wantName)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		full, query string
		want        bool
	}{
		{"ly:task:orch", "ly:task:orch", true},
		{"ly:task:orch", "orch", true},
		{"ly:task:orch", "driver", false},
		{"ly:task:orch", "ly:other:orch", false},
		{"loom-driver", "loom-driver", true},
		{"loom-driver", "driver", false},
	}
	for _, tt := range tests {
		if got := Matches(tt.full, tt.query); got != tt.want {
			t.Errorf("Matches(%q,%q) = %v", tt.full, tt.query, got)
		}
	}
}

func TestStandaloneShortname(t *testing.T) {
	t.Parallel()

	got := StandaloneShortname("a1b2c3d4")
	if got != "sa1b2c" {
		t.Fatalf("StandaloneShortname = %q", got)
	}
	if err := ValidateShortname(got); err != nil {
		t.Fatal(err)
	}
}
