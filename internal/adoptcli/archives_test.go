// archives_test.go drives `lym adopt --archives` over fake seams: which slugs a subject lists, and the refusals.

package adoptcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunArchives(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	owners := boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
		"alpha": {Owner: there, IsProject: true, Beam: true},
		"t1":    {IsTask: true},
		"t2":    {IsTask: true},
	}}
	parents := map[string]string{"t2": "alpha"}
	archive := func(slug, side string) fabricengine.OwnerArchive {
		tag := "archive-owner/" + slug + "/" + side
		return fabricengine.OwnerArchive{Slug: slug, Side: side, Tag: tag, Commit: "c-" + slug, Branch: "b-" + slug, Dir: "/repo/" + side}
	}
	// stored is every owner archive origin holds that its branch lacks, sorted by slug as the read returns them.
	// gone is a slug the board no longer holds, under the prime.
	stored := []fabricengine.OwnerArchive{
		archive("alpha", "code"), archive("gone", "code"),
		archive("root", "code"), archive("root", "records"), archive("t1", "code"), archive("t2", "records"),
	}
	listing := func(slug, subject string, sides ...string) archiveSlug {
		listed := archiveSlug{Slug: slug, Subject: subject}
		for _, side := range sides {
			a := archive(slug, side)
			listed.Tags = append(listed.Tags, archiveTag{Tag: a.Tag, Side: side, Commit: a.Commit, Branch: a.Branch, Look: "git -C /repo/" + side + " log " + a.Tag})
		}
		return listed
	}

	tests := []struct {
		name    string
		req     request
		flags   callFlags
		stored  []fabricengine.OwnerArchive
		readErr error
		want    []archiveSlug
		// wantErr lists what a refusal's message holds; a refusal reads no archive.
		wantErr []string
	}{
		{
			name: "RootListsItsOwnTagsAndThoseOfTheSlugsItMovesPerSlug", req: request{Slugs: []string{"root"}}, stored: stored,
			want: []archiveSlug{listing("gone", "root", "code"), listing("root", "root", "code", "records"), listing("t1", "root", "code")},
		},
		{
			name: "ABeamListsItsOwnTagsAndThoseOfItsMembers", req: request{Slugs: []string{"alpha"}}, stored: stored,
			want: []archiveSlug{listing("alpha", "alpha", "code"), listing("t2", "alpha", "records")},
		},
		{name: "NoTagListedIsAnEmptyList", req: request{Slugs: []string{"root"}}, want: []archiveSlug{}},
		{name: "NoSubjectIsRefused", wantErr: []string{"lym adopt --archives root"}},
		{name: "ASlugOffTheBoardIsRefused", req: request{Slugs: []string{"nosuch"}}, wantErr: []string{"lym board list"}},
		{name: "ATaskIsRefusedNamingItsSubject", req: request{Slugs: []string{"t2"}}, wantErr: []string{"lym adopt --archives alpha"}},
		{name: "AnotherFlagIsRefused", req: request{Slugs: []string{"root"}, KeepLocal: true}, wantErr: []string{"no other flag"}},
		{name: "HereBesideItIsRefused", req: request{Slugs: []string{"root"}}, flags: callFlags{Here: true}, wantErr: []string{"no other flag"}},
		{
			name: "AFailedReadNamesTheRerun", req: request{Slugs: []string{"root"}}, readErr: errors.New("origin did not answer"),
			wantErr: []string{"origin did not answer", "lym adopt --archives root"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reads := 0
			deps := archiveDeps{
				ReadOwners: func() (boardengine.Owners, error) { return owners, nil },
				ParentOf:   func(slug string) (string, error) { return parents[slug], nil },
				ReadArchives: func(keep func(slug string) bool) ([]fabricengine.OwnerArchive, error) {
					reads++
					var kept []fabricengine.OwnerArchive
					for _, a := range tt.stored {
						if keep(a.Slug) {
							kept = append(kept, a)
						}
					}
					return kept, tt.readErr
				},
			}
			flags := tt.flags
			flags.Archives = true
			var out bytes.Buffer

			exit := runArchives(deps, tt.req, flags, &out)

			if len(tt.wantErr) > 0 {
				if exit == 0 {
					t.Fatalf("runArchives exit 0; want a refusal, output %s", out.String())
				}
				for _, text := range tt.wantErr {
					if !strings.Contains(out.String(), text) {
						t.Errorf("output = %s, want it to contain %q", out.String(), text)
					}
				}
				if tt.readErr == nil && reads != 0 {
					t.Errorf("runArchives read the archives %d times; want none for a refusal", reads)
				}
				return
			}
			var envelope struct {
				Archives []archiveSlug `json:"archives"`
			}
			if exit != 0 || json.Unmarshal(out.Bytes(), &envelope) != nil {
				t.Fatalf("runArchives exit %d, output %s; want exit 0 and an envelope", exit, out.String())
			}
			if !reflect.DeepEqual(envelope.Archives, tt.want) {
				t.Errorf("runArchives archives = %+v; want %+v", envelope.Archives, tt.want)
			}
		})
	}
}
