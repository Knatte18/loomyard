// notify_test.go proves notifyPrime over fake creel clients: which identity posts the notice, when it falls back to the program, and what a refusal returns.

package orchcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
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
