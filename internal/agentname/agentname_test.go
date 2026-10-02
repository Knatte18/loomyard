package agentname

import (
	"strings"
	"testing"
)

func TestFormatParseRoundTrip(t *testing.T) {
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
	}
}

func TestResolve(t *testing.T) {
	n, err := Resolve("ly", "task", "driver")
	if err != nil || n.String() != "ly:task:driver" {
		t.Fatalf("role query = %v, %v", n, err)
	}
	n, err = Resolve("ly", "task", "ly:task:orch")
	if err != nil || n.Role != "orch" {
		t.Fatalf("full query = %v, %v", n, err)
	}
	n, err = Resolve("ly", "", "ly:orch")
	if err != nil || n.String() != "ly:orch" {
		t.Fatalf("prime full query = %v, %v", n, err)
	}
	_, err = Resolve("ly", "task", "zz:other:orch")
	if err == nil || !strings.Contains(err.Error(), "way forward: pass the role segment alone, or run the command from the worktree the full name belongs to") {
		t.Fatalf("foreign prefix err = %v", err)
	}
	if _, err = Resolve("ly", "task", "ly:orch"); err == nil {
		t.Fatal("slug mismatch accepted")
	}
}

func TestMatches(t *testing.T) {
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
	got := StandaloneShortname("a1b2c3d4")
	if got != "sa1b2c" {
		t.Fatalf("StandaloneShortname = %q", got)
	}
	if err := ValidateShortname(got); err != nil {
		t.Fatal(err)
	}
}
