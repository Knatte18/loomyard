// notify_test.go proves notifyPrime over fake creel clients: which identity posts the notice, when it falls back to the program, and what a refusal returns.

package orchcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakePoster records each post as "<identity> <receiver> <body>" and the tags it carried.
// It answers the error it is given.
type fakePoster struct {
	identity string
	posts    *[]string
	tags     *[]creelengine.PostTags
	err      error
}

func (f fakePoster) PostTagged(receiver, body string, tags creelengine.PostTags) (int64, error) {
	*f.posts = append(*f.posts, f.identity+" "+receiver+" "+body)
	*f.tags = append(*f.tags, tags)
	return 1, f.err
}

// TestPostAsProgram_PostsOnceThroughTheProgramClient proves postAsProgram posts once through the program client.
// The receiver, line and tags arrive unchanged.
// A failed client build or post returns the line with the cause.
func TestPostAsProgram_PostsOnceThroughTheProgramClient(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const line = "[loom ask] a-slug: review the record"
	tags := creelengine.PostTags{Run: "a-slug"}
	buildErr := errors.New("creel: the program client cannot be built")
	postErr := errors.New("creel: the receiver is not a listed strand")
	tests := []struct {
		name         string
		buildErr     error
		postErr      error
		wantPosts    []string
		wantErrParts []string
	}{
		{name: "PostsOnceWithReceiverLineAndTags", wantPosts: []string{"program tst:orch " + line}},
		{name: "ClientBuildErrorReturnsTheLineWithTheCause", buildErr: buildErr, wantErrParts: []string{line, buildErr.Error()}},
		{
			name: "PostErrorReturnsTheLineWithTheCause", postErr: postErr,
			wantPosts: []string{"program tst:orch " + line}, wantErrParts: []string{line, postErr.Error()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var posts []string
			var postedTags []creelengine.PostTags
			program := func() (notifyPoster, error) {
				if tt.buildErr != nil {
					return nil, tt.buildErr
				}
				return fakePoster{identity: "program", posts: &posts, tags: &postedTags, err: tt.postErr}, nil
			}

			err := postAsProgram("tst:orch", line, tags, program)

			if !slices.Equal(posts, tt.wantPosts) {
				t.Errorf("posts = %q; want %q", posts, tt.wantPosts)
			}
			for i, got := range postedTags {
				if got != tags {
					t.Errorf("post %d tags = %+v; want %+v", i, got, tags)
				}
			}
			if len(tt.wantErrParts) == 0 {
				if err != nil {
					t.Errorf("postAsProgram() error = %v; want nil", err)
				}
				return
			}
			for _, part := range tt.wantErrParts {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("postAsProgram() error = %v; want it to hold %q", err, part)
				}
			}
		})
	}
}

// TestPostSigned_BuildsTheClientOfTheIdentityItSignsAs proves which client a signed post builds.
// A non-orch strand claim builds the strand client first and the program client for an empty claim.
// Outside a hub neither client builds, so the error carries the line and the hub refusal.
func TestPostSigned_BuildsTheClientOfTheIdentityItSignsAs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const line = "[code-review request] a-slug ask 1: review the pull request"
	tests := []struct {
		name        string
		strandClaim string
	}{
		{"NonOrchStrandClaimBuildsTheStrandClient", "tst:a-slug:code-reviewer"},
		{"EmptyClaimBuildsTheProgramClient", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := postSigned("tst:orch", line, creelengine.PostTags{Run: "a-slug"}, tt.strandClaim, notifyClientsOf(&lymcwd.Location{}, "", "LOOM"))

			if err == nil || !strings.Contains(err.Error(), line) || !strings.Contains(err.Error(), "not in one") {
				t.Errorf("postSigned() error = %v; want it to hold the line and the hub refusal", err)
			}
		})
	}
}

// TestPostNotice_PostsOnlyWhenTheOrchIsRecorded proves postNotice posts nothing for an orch with no recorded strand.
// With a recorded strand it builds its clients from the location, so outside a hub it returns the line with the cause.
func TestPostNotice_PostsOnlyWhenTheOrchIsRecorded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const line = "[batten notice] a-slug: blocked"
	tests := []struct {
		name     string
		recorded bool
		wantErr  bool
	}{
		{"NoOrchStrandRecordedPostsNothing", false, false},
		{"RecordedOrchStrandOutsideAHubReturnsTheLineWithTheCause", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			posted, err := postNotice(&lymcwd.Location{}, "", "BATTEN", line, creelengine.PostTags{}, func() (string, bool, error) { return "tst:orch", tt.recorded, nil })

			if posted {
				t.Errorf("postNotice() posted = true; want false")
			}
			if tt.wantErr != (err != nil && strings.Contains(err.Error(), line)) {
				t.Errorf("postNotice() error = %v; want an error holding the line = %v", err, tt.wantErr)
			}
		})
	}
}

func TestNotifyPrime_SignsAndPosts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const line = "[batten notice] a-slug: blocked"
	tags := creelengine.PostTags{Topic: "blocked", Run: "a-slug"}
	refused := errors.New("creel: the receiver is not a listed strand")
	tests := []struct {
		name         string
		recorded     bool
		strandClaim  string
		strandErr    error
		programErr   error
		wantPosts    []string
		wantPosted   bool
		wantErrParts []string
	}{
		{name: "NoOrchStrandRecordedPostsNothing", recorded: false, strandClaim: "tst:task:driver"},
		{
			name: "NonOrchStrandClaimCreelAcceptsPostsOnceWithTheClaim", recorded: true, strandClaim: "tst:task:driver",
			wantPosts: []string{"strand tst:orch " + line}, wantPosted: true,
		},
		{
			name: "ClaimRefusedOutsideThePaneIsPostedOnceMoreAsTheProgram", recorded: true, strandClaim: "tst:task:driver",
			strandErr: fmt.Errorf("refused: %w", creelengine.ErrStrandClaimOutsidePane),
			wantPosts: []string{"strand tst:orch " + line, "program tst:orch " + line}, wantPosted: true,
		},
		{
			name: "OrchStrandClaimPostsAsTheProgramOnly", recorded: true, strandClaim: "tst:orch",
			wantPosts: []string{"program tst:orch " + line}, wantPosted: true,
		},
		{
			name: "EmptyClaimPostsAsTheProgramOnly", recorded: true,
			wantPosts: []string{"program tst:orch " + line}, wantPosted: true,
		},
		{
			name: "OtherRefusalPostsNothingMoreAndReturnsTheLineWithTheRefusal", recorded: true, strandClaim: "tst:task:driver",
			strandErr: refused,
			wantPosts: []string{"strand tst:orch " + line}, wantErrParts: []string{line, refused.Error()},
		},
		{
			name: "ProgramRefusalReturnsTheLineWithTheRefusal", recorded: true,
			programErr: refused,
			wantPosts:  []string{"program tst:orch " + line}, wantErrParts: []string{line, refused.Error()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var posts []string
			var postedTags []creelengine.PostTags
			clients := notifyClients{
				strand: func(string) (notifyPoster, error) {
					return fakePoster{identity: "strand", posts: &posts, tags: &postedTags, err: tt.strandErr}, nil
				},
				program: func() (notifyPoster, error) {
					return fakePoster{identity: "program", posts: &posts, tags: &postedTags, err: tt.programErr}, nil
				},
			}

			posted, err := notifyPrime(func() (string, bool, error) { return "tst:orch", tt.recorded, nil }, line, tags, tt.strandClaim, clients)

			if !slices.Equal(posts, tt.wantPosts) {
				t.Errorf("posts = %q; want %q", posts, tt.wantPosts)
			}
			// Every post, the strand attempt and the program fallback alike, carries the caller's tags unchanged.
			for i, got := range postedTags {
				if got != tags {
					t.Errorf("post %d tags = %+v; want %+v", i, got, tags)
				}
			}
			if posted != tt.wantPosted {
				t.Errorf("notifyPrime() posted = %v; want %v", posted, tt.wantPosted)
			}
			if len(tt.wantErrParts) == 0 {
				if err != nil {
					t.Errorf("notifyPrime() error = %v; want nil", err)
				}
				return
			}
			for _, part := range tt.wantErrParts {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("notifyPrime() error = %v; want it to hold %q", err, part)
				}
			}
		})
	}
}
