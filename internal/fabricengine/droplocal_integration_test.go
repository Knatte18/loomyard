//go:build tier1

// droplocal_integration_test.go proves DropLocal on a hubforge hub: a pair's local-only tips are tagged and its branches moved to origin's tips, or a dirty path refuses with nothing moved.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestDropLocal covers a records branch and a code branch ahead of origin tagged and moved to origin's tips with the mutations recorded, and a dirty tracked path refusing with nothing tagged or moved.
func TestDropLocal(t *testing.T) {
	t.Parallel()

	t.Run("Branches ahead of origin are tagged and moved to origin's tips", func(t *testing.T) {
		t.Parallel()

		pair := newMovesPair(t)
		codeOrigin := gitkit.RevParse(t, pair.code, "origin/"+pair.codeBranch)
		recordsOrigin := gitkit.RevParse(t, pair.records, "origin/"+pair.recordsName)
		codeTip := gitkit.CommitFile(t, pair.code, "local-code.txt", "code", "local code")
		recordsTip := gitkit.CommitFile(t, pair.records, "notes/local.md", "records", "local records")

		rec := fabricengine.NewMutations(pair.Location.HubPath)
		res, err := fabricengine.DropLocal(rec, pair.Location, "task")
		if err != nil {
			t.Fatalf("DropLocal() error = %v; want nil", err)
		}

		if got := gitkit.RevParse(t, pair.code, "HEAD"); got != codeOrigin {
			t.Errorf("code HEAD = %q; want origin's tip %q", got, codeOrigin)
		}
		if got := gitkit.RevParse(t, pair.records, "HEAD"); got != recordsOrigin {
			t.Errorf("records HEAD = %q; want origin's tip %q", got, recordsOrigin)
		}
		wantTags := []string{"archive/task/" + codeTip[:12], "archive/task/" + recordsTip[:12]}
		if !slices.Equal(res.Tags, wantTags) {
			t.Errorf("DropLocal() Tags = %v; want %v", res.Tags, wantTags)
		}
		if got := gitkit.RevParse(t, pair.code, wantTags[0]); got != codeTip {
			t.Errorf("tag %s = %q; want the local code tip %q", wantTags[0], got, codeTip)
		}
		if got := gitkit.RevParse(t, pair.records, wantTags[1]); got != recordsTip {
			t.Errorf("tag %s = %q; want the local records tip %q", wantTags[1], got, recordsTip)
		}
		for _, mutations := range []fabricengine.Mutations{res.Mutated(), rec.Snapshot()} {
			var kinds []fabricengine.Kind
			for _, entry := range mutations.Entries() {
				kinds = append(kinds, entry.Kind)
			}
			wantKinds := []fabricengine.Kind{fabricengine.KindTagCreated, fabricengine.KindCommitsDropped, fabricengine.KindTagCreated, fabricengine.KindCommitsDropped}
			if !slices.Equal(kinds, wantKinds) {
				t.Errorf("recorded kinds = %v; want %v", kinds, wantKinds)
			}
		}
	})

	t.Run("A tracked change on a path the move rewrites refuses with nothing tagged or moved", func(t *testing.T) {
		t.Parallel()

		pair := newMovesPair(t)
		codeTip := gitkit.CommitFile(t, pair.code, "local-code.txt", "code", "local code")
		recordsTip := gitkit.CommitFile(t, pair.records, "notes/local.md", "records", "local records")
		writeFile(t, filepath.Join(pair.records, "notes", "local.md"), "dirty\n")

		res, err := fabricengine.DropLocal(fabricengine.NewMutations(pair.Location.HubPath), pair.Location, "task")

		refusal, ok := fabricengine.RefusalOf(err)
		if !ok || refusal.Check != fabricengine.CheckDirtiness || !strings.Contains(refusal.Reason, "notes/local.md") {
			t.Fatalf("DropLocal() error = %v; want a dirtiness refusal naming notes/local.md", err)
		}
		if got := gitkit.RevParse(t, pair.code, "HEAD"); got != codeTip {
			t.Errorf("code HEAD = %q; want %q, nothing moved", got, codeTip)
		}
		if got := gitkit.RevParse(t, pair.records, "HEAD"); got != recordsTip {
			t.Errorf("records HEAD = %q; want %q, nothing moved", got, recordsTip)
		}
		for _, dir := range []string{pair.code, pair.records} {
			if tags := gitkit.Git(t, dir, "tag", "--list", "archive/*"); strings.TrimSpace(tags) != "" {
				t.Errorf("tags in %s = %q; want none", dir, tags)
			}
		}
		if len(res.Tags) != 0 || res.Mutated().Len() != 0 {
			t.Errorf("DropLocal() result = %+v; want no tags and no mutations", res)
		}
	})

	t.Run("A records branch and origin's tip at different anchors refuse before any tag or move", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			// arrange leaves the records branch ahead of origin and the two anchors apart, and returns what the refusal must contain.
			arrange func(t *testing.T, pair *movesPair) []string
		}{
			{
				"OriginsTipCarriesAnotherMarker",
				func(t *testing.T, pair *movesPair) []string {
					originCommitFile(t, pair.RecordsBare, pair.recordsName, lyxcwd.AnchorFileName, "sub\n")
					gitkit.Git(t, pair.records, "fetch", "origin")
					gitkit.CommitFile(t, pair.records, "notes/local.md", "records", "local records")
					return []string{"lyx fabric anchor task --set sub", "re-run the same `lyx adopt` with `--drop-local`"}
				},
			},
			{
				"LocalWeftAheadWithAReAnchorNotYetPushed",
				func(t *testing.T, pair *movesPair) []string {
					gitkit.CommitFile(t, pair.records, lyxcwd.AnchorFileName, "sub\n", "re-anchor")
					return []string{"lyx fabric anchor task --set .", "re-run the same `lyx adopt` with `--drop-local`"}
				},
			},
			{
				"OwnMarkerEmpty",
				func(t *testing.T, pair *movesPair) []string {
					gitkit.CommitFile(t, pair.records, "notes/local.md", "records", "local records")
					writeFile(t, filepath.Join(pair.records, lyxcwd.AnchorFileName), "\n")
					return []string{lyxcwd.AnchorFileName}
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				pair := newMovesPair(t)
				codeTip := gitkit.CommitFile(t, pair.code, "local-code.txt", "code", "local code")
				want := tt.arrange(t, pair)
				recordsTip := gitkit.RevParse(t, pair.records, "HEAD")

				res, err := fabricengine.DropLocal(fabricengine.NewMutations(pair.Location.HubPath), pair.Location, "task")

				if !errors.Is(err, fabricengine.ErrAnchorChange) {
					t.Fatalf("DropLocal() error = %v; want it to wrap ErrAnchorChange", err)
				}
				for _, text := range want {
					if !strings.Contains(err.Error(), text) {
						t.Errorf("DropLocal() error = %q; want it to contain %q", err, text)
					}
				}
				if got := gitkit.RevParse(t, pair.code, "HEAD"); got != codeTip {
					t.Errorf("code HEAD = %q; want %q, nothing moved", got, codeTip)
				}
				if got := gitkit.RevParse(t, pair.records, "HEAD"); got != recordsTip {
					t.Errorf("records HEAD = %q; want %q, nothing moved", got, recordsTip)
				}
				for _, dir := range []string{pair.code, pair.records} {
					if tags := gitkit.Git(t, dir, "tag", "--list", "archive/*"); strings.TrimSpace(tags) != "" {
						t.Errorf("tags in %s = %q; want none", dir, tags)
					}
				}
				if len(res.Tags) != 0 || res.Mutated().Len() != 0 {
					t.Errorf("DropLocal() result = %+v; want no tags and no mutations", res)
				}
			})
		}
	})
}
