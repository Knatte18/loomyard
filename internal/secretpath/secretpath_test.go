// secretpath_test.go covers which paths a Matcher refuses, the malformed-pattern refusal and the hit rendering.

package secretpath_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/secretpath"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestMatcher_Match(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		added       []string
		path        string
		wantPattern string
	}{
		{"EnvAtRoot", nil, ".env", ".env"},
		{"EnvInSubdirectory", nil, "config/.env", ".env"},
		{"PemKey", nil, "certs/server.pem", "*.pem"},
		{"PrivateKey", nil, "home/id_rsa", "id_rsa"},
		{"CredentialsFile", nil, "credentials.json", "credentials*"},
		{"SecretsYml", nil, "deploy/secrets.yml", "secrets.y*ml"},
		{"SecretsYaml", nil, "secrets.yaml", "secrets.y*ml"},
		{"EnvExampleAllowed", nil, ".env.example", ""},
		{"PublicKeyAllowed", nil, "id_rsa.pub", ""},
		{"OrdinaryFileAllowed", nil, "internal/foo/impl.go", ""},
		{"AddedBaseNamePattern", []string{"*.key"}, "tls/server.key", "*.key"},
		{"AddedSlashPatternMatchesWholePath", []string{"config/prod.json"}, "config/prod.json", "config/prod.json"},
		{"AddedSlashPatternSkipsDeeperPath", []string{"config/prod.json"}, "other/config/prod.json", ""},
		{"ShippedPatternWinsOverAdded", []string{".e*"}, ".env", ".env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := secretpath.New(tt.added)
			if err != nil {
				t.Fatalf("New(%q) error = %v", tt.added, err)
			}
			pattern, matched := m.Match(tt.path)
			if pattern != tt.wantPattern || matched != (tt.wantPattern != "") {
				t.Errorf("Match(%q) = (%q, %v); want (%q, %v)", tt.path, pattern, matched, tt.wantPattern, tt.wantPattern != "")
			}
		})
	}
}

func TestNew_RefusesMalformedPattern(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	_, err := secretpath.New([]string{"*.key", "[unclosed"})
	if err == nil {
		t.Fatal("New() error = nil; want the malformed pattern refused")
	}
	if !strings.Contains(err.Error(), `"[unclosed"`) {
		t.Errorf("New() error = %q; want it to name the malformed pattern", err.Error())
	}
}

func TestHits_DescribeNamesEachPathAndPattern(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	var m secretpath.Matcher
	hits := m.Hits([]string{"README.md", "keys/id_rsa", ".env"})
	want := []secretpath.Hit{{Path: "keys/id_rsa", Pattern: "id_rsa"}, {Path: ".env", Pattern: ".env"}}
	if !reflect.DeepEqual(hits, want) {
		t.Fatalf("Hits() = %v; want %v", hits, want)
	}
	if got, want := secretpath.Describe(hits), "keys/id_rsa (pattern id_rsa), .env (pattern .env)"; got != want {
		t.Errorf("Describe() = %q; want %q", got, want)
	}
}
