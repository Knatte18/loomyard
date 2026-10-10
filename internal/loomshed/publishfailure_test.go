// publishfailure_test.go covers the shape checks on a Publish failure record and the extra round-gate steps built from it, with no repository.

package loomshed

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/verifytree"
)

// writeRecord writes f as the failure record at paths, bypassing WritePublishFailure so a field can hold any value.
func writeRecord(t *testing.T, paths verifytree.Paths, f verifytree.PublishFailure) {
	t.Helper()
	data, err := yaml.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.PublishFailure), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublishFailure, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPublishFailure(t *testing.T) {
	t.Parallel()

	t.Run("checked record drops each field that fails its shape check", func(t *testing.T) {
		t.Parallel()

		verifyDir := t.TempDir()
		paths := verifytree.NewPaths(t.TempDir(), verifyDir)
		goodTest := verifytree.FailedTest{Package: "example.com/m/a", Test: "TestA/sub_case=1"}
		tests := []struct {
			name      string
			record    verifytree.PublishFailure
			wantKept  bool
			wantAfter verifytree.PublishFailure
		}{
			{
				name:      "well-formed record without a merge commit is kept whole",
				record:    verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify, Tests: []verifytree.FailedTest{goodTest}, LogPath: paths.PublishFailureLog, Head: "abc"},
				wantKept:  true,
				wantAfter: verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify, Tests: []verifytree.FailedTest{goodTest}, LogPath: paths.PublishFailureLog, Head: "abc"},
			},
			{
				name:      "merge field that is an option is dropped",
				record:    verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify, MergeCommit: "HEAD --output=x"},
				wantKept:  true,
				wantAfter: verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify},
			},
			{
				name:      "short hash merge field is dropped",
				record:    verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify, MergeCommit: "abc1234"},
				wantKept:  true,
				wantAfter: verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify},
			},
			{
				name: "tests with a metacharacter, a bad identifier or a bad package are dropped",
				record: verifytree.PublishFailure{Kind: verifytree.FailureKindPublishVerify, Tests: []verifytree.FailedTest{
					{Package: "example.com/m/a", Test: "TestA; rm x"},
					{Package: "example.com/m/a", Test: "TestA/it's"},
					{Package: "example.com/m/a", Test: "helper"},
					{Package: "$(id)", Test: "TestA"},
					goodTest,
				}},
				wantKept:  true,
				wantAfter: verifytree.PublishFailure{Kind: verifytree.FailureKindPublishVerify, Tests: []verifytree.FailedTest{goodTest}},
			},
			{
				name:      "log path outside the verify directory is dropped",
				record:    verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify, LogPath: filepath.Join(t.TempDir(), "other.log")},
				wantKept:  true,
				wantAfter: verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify},
			},
			{
				name:     "unknown kind drops the record",
				record:   verifytree.PublishFailure{Kind: "go test ./...", Tests: []verifytree.FailedTest{goodTest}},
				wantKept: false,
			},
		}
		for _, tt := range tests {
			writeRecord(t, paths, tt.record)
			got, ok := checkedPublishFailure(paths, t.TempDir())
			if ok != tt.wantKept {
				t.Errorf("%s: kept = %v; want %v", tt.name, ok, tt.wantKept)
				continue
			}
			if ok && !reflect.DeepEqual(got, tt.wantAfter) {
				t.Errorf("%s: record = %+v; want %+v", tt.name, got, tt.wantAfter)
			}
		}

		if err := verifytree.RemovePublishFailure(paths); err != nil {
			t.Fatal(err)
		}
		if _, ok := checkedPublishFailure(paths, t.TempDir()); ok {
			t.Error("absent record kept; want none")
		}
	})

	t.Run("command runs each failing test under the tier of its verify", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			failure  verifytree.PublishFailure
			packages []string
			want     string
		}{
			{
				name: "plan verify runs its tests under integration and no package pass",
				failure: verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify, Tests: []verifytree.FailedTest{
					{Package: "example.com/m/a", Test: "TestA/sub.case"},
					{Package: "example.com/m/b", Test: "TestB"},
				}},
				packages: []string{"./a"},
				want:     `go test -tags integration -run '^TestA$/^sub\.case$' example.com/m/a && go test -tags integration -run '^TestB$' example.com/m/b`,
			},
			{
				name: "publish_verify runs its tests then the impacted set under tmux",
				failure: verifytree.PublishFailure{Kind: verifytree.FailureKindPublishVerify, Tests: []verifytree.FailedTest{
					{Package: "example.com/m/a", Test: "TestA"},
				}},
				packages: []string{"./a", "./b"},
				want:     `go test -tags tmux -run '^TestA$' example.com/m/a && go test -tags tmux ./a ./b`,
			},
			{
				name:     "publish_verify with no surviving test runs the impacted set alone",
				failure:  verifytree.PublishFailure{Kind: verifytree.FailureKindPublishVerify},
				packages: []string{"./..."},
				want:     "go test -tags tmux ./...",
			},
			{
				name:    "publish_verify with an empty package list skips the impacted-set pass",
				failure: verifytree.PublishFailure{Kind: verifytree.FailureKindPublishVerify, Tests: []verifytree.FailedTest{{Package: "example.com/m/a", Test: "TestA"}}},
				want:    `go test -tags tmux -run '^TestA$' example.com/m/a`,
			},
			{
				name:    "plan verify with no surviving test adds nothing",
				failure: verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify},
				want:    "",
			},
		}
		for _, tt := range tests {
			if got := publishFailureCommand(tt.failure, tt.packages); got != tt.want {
				t.Errorf("%s: command = %q; want %q", tt.name, got, tt.want)
			}
		}
	})
}
