// entries_describe_test.go covers describeEntry: its construction-time validation over Config, the
// three injected Env seams and the told DescriptionPath, plus the happy path.

package shedrecipe

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// newDescribeTestEnv returns newTestEnv's Env with the Describe fields filled.
func newDescribeTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.DescriptionPath = filepath.Join(dir, "summary.md")
	env.DescribeSpec = func() (shuttleengine.Spec, error) {
		return shuttleengine.Spec{Prompt: "test describe prompt", OutputFiles: []string{env.DescriptionPath}}, nil
	}
	env.CommitDescription = func() error { return nil }
	return env
}

func TestDescribeEntry_ConstructionFailures(t *testing.T) {
	gated := gatesCfg("description", 3)
	cases := []struct {
		name   string
		mutate func(*Env)
		cfg    Config
		want   string
	}{
		{"NilDescribeSpec", func(e *Env) { e.DescribeSpec = nil }, gated, "DescribeSpec"},
		{"NilCommitDescription", func(e *Env) { e.CommitDescription = nil }, gated, "CommitDescription"},
		{"NilShuttle", func(e *Env) { e.Shuttle = nil }, gated, "Shuttle"},
		{"MissingDescriptionPath", func(e *Env) { e.DescriptionPath = "" }, gated, "DescriptionPath"},
		{"UnknownConfigKey", func(e *Env) {}, Config{"bogus_key": "x"}, "bogus_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newDescribeTestEnv(t)
			tc.mutate(&env)
			_, err := describeEntry("Row", tc.cfg, env)
			if err == nil {
				t.Fatalf("describeEntry() error = nil; want non-nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("describeEntry() error = %v; want it to name %q", err, tc.want)
			}
		})
	}
}

func TestDescribeEntry_HappyPath(t *testing.T) {
	env := newDescribeTestEnv(t)
	p, err := describeEntry("Row", gatesCfg("description", 2), env)
	if err != nil {
		t.Fatalf("describeEntry() error = %v; want nil", err)
	}
	if p == nil {
		t.Fatalf("describeEntry() = nil producer; want non-nil")
	}
}
