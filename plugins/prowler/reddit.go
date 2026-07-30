// reddit.go implements Reddit's ".json" special-case: Reddit blocks scraping
// of its normal HTML pages but leaves the JSON API open, so hitting
// "<url>.json" yields clean, structured post/comment data with none of the
// bot-detection trouble a static HTML fetch of Reddit would run into.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// redditHostPattern matches Reddit URLs across its three common host forms
// (bare, www, and old.reddit.com); Reddit serves the same JSON API under all
// three, so any of them is eligible for the .json special-case.
var redditHostPattern = regexp.MustCompile(`^https?://(www\.|old\.)?reddit\.com`)

// maxTopComments bounds how many top-level comments formatRedditPost includes,
// mirroring weblens' behavior of showing only the first ~20 rather than an
// entire, potentially huge, comment tree.
const maxTopComments = 20

// selftextSnippetLen bounds the selftext preview formatRedditSubreddit shows
// per listing entry, keeping a subreddit summary skimmable rather than
// reproducing every post's full body inline.
const selftextSnippetLen = 200

// redditThing is Reddit API's generic envelope: every object it returns —
// posts, comments, and the listings that contain them — is wrapped in a
// {kind, data} pair, so this one struct models the whole nested shape.
type redditThing struct {
	Kind string     `json:"kind"`
	Data redditData `json:"data"`
}

// redditData is the payload carried by a redditThing. Its exact meaning
// depends on Kind: a "t3" thing's data is a post, a "t1" thing's data is a
// comment, and a "Listing" thing's data holds a page of child things. Only
// one struct is used for all three because Reddit's JSON does not
// distinguish them at the schema level — unused fields are simply absent
// (zero-valued) for a given Kind.
type redditData struct {
	Title       string        `json:"title"`
	Subreddit   string        `json:"subreddit"`
	Score       int           `json:"score"`
	NumComments int           `json:"num_comments"`
	Selftext    string        `json:"selftext"`
	URL         string        `json:"url"`
	Author      string        `json:"author"`
	Body        string        `json:"body"`
	Children    []redditThing `json:"children"`
}

// isRedditUrl reports whether rawURL points at Reddit, across its bare, www,
// and old.reddit.com host forms — the set fetchPage routes through the
// Reddit .json special-case instead of a plain static fetch.
func isRedditUrl(rawURL string) bool {
	return redditHostPattern.MatchString(rawURL)
}

// toRedditJsonUrl rewrites a Reddit post/subreddit URL to its JSON API
// equivalent by stripping one trailing slash (if present) and appending
// ".json", e.g. "https://reddit.com/r/foo/" -> "https://reddit.com/r/foo.json".
func toRedditJsonUrl(rawURL string) string {
	return strings.TrimSuffix(rawURL, "/") + ".json"
}

// fetchReddit fetches url's Reddit JSON API equivalent through f.do (the
// shared, injectable transport, so this path is unit-testable without
// network) and formats it into markdown. It reports handled=false when the
// response is not something this function knows how to interpret (a
// non-JSON Content-Type, or an unparseable body) so the caller can fall
// through to the ordinary HTML extraction cascade instead — this matters
// because some Reddit responses (e.g. search pages) are HTML, not JSON, even
// though the URL matched isRedditUrl.
func fetchReddit(ctx context.Context, f fetcher, url string) (out string, handled bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, toRedditJsonUrl(url), nil)
	if err != nil {
		// A malformed URL can never be sent, so there is nothing for f.do to
		// attempt — report it exactly like a transport failure.
		return "# Error fetching " + url + "\n\n" + err.Error(), true
	}
	// Deliberately not defaultHeaders(): Reddit's JSON API wants a distinct,
	// identifying UA rather than the browser-impersonation UA used for the
	// static HTML path.
	req.Header.Set("User-Agent", "prowler-reader/1.0")

	resp, err := f.do(req)
	if err != nil {
		return "# Error fetching " + url + "\n\n" + err.Error(), true
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "# Error fetching " + url + "\n\nHTTP " + strconv.Itoa(resp.StatusCode), true
	}

	// Some Reddit URLs (e.g. search results) respond with HTML even under
	// isRedditUrl; only a JSON response belongs to this path.
	if !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		return "", false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}

	// Peek at the top-level JSON shape: a post's .json response is a
	// two-element array of Listing things, while a subreddit's is a single
	// Listing object. Decoding into `any` first lets us dispatch on that
	// shape before committing to either formatter's struct expectations.
	var probe any
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", false
	}

	var formatted string
	if _, isArray := probe.([]any); isArray {
		formatted = formatRedditPost(body)
	} else {
		formatted = formatRedditSubreddit(body)
	}

	if formatted == "" {
		return "# " + url + "\n\nCould not parse Reddit response.", true
	}
	return formatted, true
}

// formatRedditPost renders a Reddit post .json payload (raw: a two-element
// array of Listing things — arr[0] the post, arr[1] its comments) into
// markdown: title, subreddit/score/comment-count line, selftext (or a link
// for link posts), and up to maxTopComments top-level comments. It returns
// "" when the payload does not have the expected post, signaling the caller
// to treat the fetch as unparseable.
func formatRedditPost(raw json.RawMessage) string {
	var arr []redditThing
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) < 2 {
		return ""
	}

	postChildren := arr[0].Data.Children
	if len(postChildren) == 0 {
		return ""
	}
	post := postChildren[0].Data

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", post.Title)
	fmt.Fprintf(&b, "r/%s | %d points | %d comments\n\n", post.Subreddit, post.Score, post.NumComments)

	if post.Selftext != "" {
		b.WriteString(post.Selftext)
		b.WriteString("\n\n")
	} else {
		fmt.Fprintf(&b, "Link: %s\n\n", post.URL)
	}

	topComments := make([]redditThing, 0, maxTopComments)
	for _, child := range arr[1].Data.Children {
		if child.Kind != "t1" {
			continue
		}
		topComments = append(topComments, child)
		if len(topComments) == maxTopComments {
			break
		}
	}

	if len(topComments) > 0 {
		b.WriteString("## Top Comments\n\n")
		for _, c := range topComments {
			fmt.Fprintf(&b, "**%s** (%d pts):\n%s\n\n", c.Data.Author, c.Data.Score, c.Data.Body)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// formatRedditSubreddit renders a subreddit listing .json payload (raw: a
// single Listing thing whose Data.Children are "t3" post things) into a
// markdown bullet list: one "- **title** (score pts, N comments)" line per
// post, with a short selftext snippet when present. It returns "" when the
// listing has no posts, signaling the caller to treat the fetch as
// unparseable.
func formatRedditSubreddit(raw json.RawMessage) string {
	var listing redditThing
	if err := json.Unmarshal(raw, &listing); err != nil {
		return ""
	}

	posts := listing.Data.Children
	if len(posts) == 0 {
		return ""
	}

	var b strings.Builder
	// The listing envelope itself carries no subreddit name; every post in
	// it was fetched from the same /r/<subreddit>.json endpoint, so the
	// first post's field is representative of the whole page.
	fmt.Fprintf(&b, "# r/%s\n\n", posts[0].Data.Subreddit)

	for _, p := range posts {
		d := p.Data
		fmt.Fprintf(&b, "- **%s** (%d pts, %d comments)", d.Title, d.Score, d.NumComments)
		if d.Selftext != "" {
			b.WriteString(" ")
			b.WriteString(selftextSnippet(d.Selftext))
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// selftextSnippet renders a short, single-line preview of a post's selftext:
// newlines are flattened to spaces (so the listing stays one bullet per
// line) and the result is truncated to selftextSnippetLen characters with a
// trailing ellipsis when truncation occurred.
func selftextSnippet(selftext string) string {
	flat := strings.ReplaceAll(selftext, "\n", " ")
	if len(flat) <= selftextSnippetLen {
		return flat
	}
	return flat[:selftextSnippetLen] + "..."
}
