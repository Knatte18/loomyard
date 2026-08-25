// redditformat_test.go exercises the shared representation and renderer: redditPostFromListings'
// mapping from decoded OAuth listings, and formatRedditThread's markdown rendering, including the
// byte-for-byte OAuth golden regression that proves the tier-neutral refactor changed nothing
// observable on the credentialed path. No network call.

package main

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

// updateRedditGolden regenerates testdata/reddit-thread-golden.md from the
// current formatter's output instead of comparing against it. It is never
// set in CI; a human sets it deliberately, via
// `go -C plugins/prowler test -run TestFormatRedditThread_GoldenOAuthOutput . -args -update-golden`,
// after confirming a formatter change is intended.
var updateRedditGolden = flag.Bool("update-golden", false, "regenerate testdata/reddit-thread-golden.md from the current formatter's output")

// testdata/reddit-thread.json is hand-authored rather than captured from a
// live response, because oauth.reddit.com cannot be read without
// credentials that do not exist yet at this point in the plan; batch 4's
// live integration test is what validates the real response shape. It is
// structurally faithful to Reddit's documented comments-endpoint Listing
// shape: a two-element array of [post listing, comments listing].
func TestFormatRedditThread(t *testing.T) {
	data := readTestdataFile(t, "reddit-thread.json")

	var listings []redditListing
	if err := json.Unmarshal([]byte(data), &listings); err != nil {
		t.Fatalf("json.Unmarshal(reddit-thread.json) error = %v", err)
	}

	post, err := redditPostFromListings(listings)
	if err != nil {
		t.Fatalf("redditPostFromListings() error = %v; want nil", err)
	}
	out := formatRedditThread(post, "https://oauth.reddit.com/r/golang/comments/abc123")

	if !strings.Contains(out, "What is the idiomatic way to handle errors in Go?") {
		t.Errorf("formatRedditThread() out missing title:\n%s", out)
	}
	if !strings.Contains(out, "I've been writing Go for a few months") {
		t.Errorf("formatRedditThread() out missing selftext:\n%s", out)
	}

	if topLevelCount := strings.Count(out, "** ("); topLevelCount != maxTopComments {
		t.Errorf("formatRedditThread() top-level comment count = %d; want %d", topLevelCount, maxTopComments)
	}
	// c20 and c21 fall beyond the cap; the "more" child sits before them in
	// the fixture, so if it were mistakenly counted toward the cap it would
	// have displaced a real comment earlier than c20 -- these two absences
	// together prove the more child contributed nothing and the cap landed
	// in the right place.
	if strings.Contains(out, "top_author_20") || strings.Contains(out, "top_author_21") {
		t.Errorf("formatRedditThread() out contains a comment beyond maxTopComments:\n%s", out)
	}

	if replyCount := strings.Count(out, "> **u/capreply"); replyCount != maxTopComments {
		t.Errorf("formatRedditThread() reply count for the over-capped comment = %d; want %d", replyCount, maxTopComments)
	}

	if strings.Contains(out, "A reply to a reply") {
		t.Errorf("formatRedditThread() out contains a reply's own nested reply, which must not render:\n%s", out)
	}

	t.Run("zero_comment_thread", func(t *testing.T) {
		const fixture = `[{"kind":"Listing","data":{"children":[{"kind":"t3","data":{"title":"A quiet post","selftext":"Nobody has replied yet.","author":"solo","subreddit":"golang","score":1,"url":"","replies":""}}]}}]`
		var listings []redditListing
		if err := json.Unmarshal([]byte(fixture), &listings); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		post, err := redditPostFromListings(listings)
		if err != nil {
			t.Fatalf("redditPostFromListings() error = %v; want nil", err)
		}
		out := formatRedditThread(post, "https://oauth.reddit.com/r/golang/comments/quiet")
		if out == "" {
			t.Error("formatRedditThread() out is empty; want a non-empty result")
		}
		if strings.Contains(out, "## Top Comments") {
			t.Errorf("formatRedditThread() out contains a Top Comments heading with zero comments:\n%s", out)
		}
	})
}

// TestFormatRedditThread_GoldenOAuthOutput compares formatRedditThread's
// current output for testdata/reddit-thread.json against the byte-for-byte
// golden file committed at testdata/reddit-thread-golden.md. This is the
// regression that proves the tier-neutral-representation refactor changes
// nothing observable on the credentialed OAuth path: the golden file was
// captured before that refactor and must still match after it.
func TestFormatRedditThread_GoldenOAuthOutput(t *testing.T) {
	data := readTestdataFile(t, "reddit-thread.json")

	var listings []redditListing
	if err := json.Unmarshal([]byte(data), &listings); err != nil {
		t.Fatalf("json.Unmarshal(reddit-thread.json) error = %v", err)
	}

	const sourceURL = "https://www.reddit.com/r/golang/comments/abc123/idiomatic_errors/"
	const goldenPath = "testdata/reddit-thread-golden.md"

	post, err := redditPostFromListings(listings)
	if err != nil {
		t.Fatalf("redditPostFromListings() error = %v; want nil", err)
	}
	out := formatRedditThread(post, sourceURL)

	if *updateRedditGolden {
		if err := os.WriteFile(goldenPath, []byte(out), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", goldenPath, err)
	}

	if out != string(want) {
		t.Errorf("formatRedditThread() output does not match golden file %s; want the two byte-for-byte identical.\n"+
			"If this difference is intended, re-run with -args -update-golden after confirming it.\ngot:\n%s\nwant:\n%s",
			goldenPath, out, want)
	}
}
