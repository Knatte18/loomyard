// secretpaths_test.go covers record-batch's use of the secret-file matcher its CLI loads.

package webstercli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/secretpath"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestRecordBatchCmd_SecretPaths proves record-batch fails a batch whose commit adds a path the loaded matcher matches.
// A loader that fails with an invalid config refuses the verb with the fabric.yaml way forward instead.
func TestRecordBatchCmd_SecretPaths(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name        string
		secretPaths func() (secretpath.Matcher, error)
		wantSubstrs []string
	}{
		{
			name:        "HubPatternFailsBatch",
			secretPaths: func() (secretpath.Matcher, error) { return secretpath.New([]string{"*.key"}) },
			wantSubstrs: []string{`"batch_failed":true`, "tls/server.key (pattern *.key)", "lym webster reset --to batch-start --batch 01"},
		},
		{
			name: "InvalidFabricYAMLRefused",
			secretPaths: func() (secretpath.Matcher, error) {
				return secretpath.Matcher{}, fmt.Errorf("fabric.yaml secret_paths: %w", configengine.ErrInvalid)
			},
			wantSubstrs: []string{"webster: fabric.yaml did not load", "way forward: fix fabric.yaml under _lym/config"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newSkipGitVerbsFixture(t)
			fx.CLI.secretPaths = tt.secretPaths
			st := fx.initState(t)
			startSHA := gitkit.RevParse(t, fx.CLI.geom.WorktreeRoot, "HEAD")
			headSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "tls/server.key", "key material\n", "01.1: add the key")
			st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
			st.CurrentBatch = 1
			if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
				t.Fatalf("SaveState() error = %v", err)
			}
			fx.Engine.Audit = shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
			}
			writeBatchReport(t, fx.CLI.geom.ReportsDir, headSHA)

			var out strings.Builder
			exitCode := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})

			if exitCode == 0 {
				t.Fatalf("record-batch 1 = 0; want non-zero, output: %s", out.String())
			}
			for _, want := range tt.wantSubstrs {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q; got %q", want, out.String())
				}
			}
		})
	}
}
