// fetch_test.go exercises the static-fetch extraction cascade with a stubbed
// fetcher.do (canned responses keyed by request URL) and a stubbed
// fetcher.browser. No network, no Chrome.

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubResponses builds a fetcher whose do dispatches on the request URL to a
// canned response, and whose browser is the given stub.
func stubResponses(t *testing.T, responses map[string]*http.Response, browser func(ctx context.Context, url string) (string, bool)) fetcher {
	t.Helper()
	return fetcher{
		do: func(req *http.Request) (*http.Response, error) {
			resp, ok := responses[req.URL.String()]
			if !ok {
				t.Fatalf("unexpected request URL: %s", req.URL.String())
			}
			return resp, nil
		},
		browser: browser,
	}
}

func htmlResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

const readableArticleHTML = `<html><head><title>Ignored</title></head><body>
<article><h1>A Real Article</h1>` +
	`<p>This is a long paragraph of genuinely readable article content that easily exceeds one hundred characters in length, so Readability should treat it as the main article body.</p>` +
	`</article></body></html>`

const shortArticleHTML = `<html><body><article><h1>Tiny</h1><p>Too short.</p></article></body></html>`

// noArticleButLongBodyHTML hides its text behind display:none. go-readability's
// visibility check excludes it from scoring entirely (TextContent comes back
// empty, i.e. Readability finds no article at all), while goquery's plain-text
// extraction in stripToBodyText does not check visibility and still finds it —
// exactly the "Readability fails, body text fallback succeeds" cascade branch.
const noArticleButLongBodyHTML = `<html><body><div style="display:none">` +
	`This page has no article/semantic structure at all, just a long stretch of plain body text that is not wrapped in anything Readability recognizes as an article, but is still well over one hundred characters of readable prose for the body-text fallback to pick up.` +
	`</div></body></html>`

// noArticleShortBodyHTML uses the same display:none trick but with body text
// too short to satisfy the body-text fallback's own threshold, so both
// Readability and the body-text fallback fail and the browser fallback runs.
const noArticleShortBodyHTML = `<html><body><div style="display:none">short</div></body></html>`

func TestFetchPage_ReadabilityUsable(t *testing.T) {
	const url = "https://example.com/article"
	f := stubResponses(t, map[string]*http.Response{
		url: htmlResponse(readableArticleHTML),
	}, func(context.Context, string) (string, bool) {
		t.Fatal("browser fallback should not be invoked")
		return "", false
	})

	got := fetchPage(context.Background(), f, url)
	// go-readability derives the title from page <title>/metadata, not the
	// first <h1>, so the expected title is the page's actual <title> text.
	if !strings.HasPrefix(got, "# Ignored") {
		t.Errorf("fetchPage() = %q; want it to start with the page's <title>", got)
	}
	if !strings.Contains(got, "Source: "+url) {
		t.Errorf("fetchPage() = %q; want it to contain the source line", got)
	}
	if !strings.Contains(got, "genuinely readable article content") {
		t.Errorf("fetchPage() = %q; want it to contain the article body", got)
	}
}

func TestFetchPage_ShortReadabilityTriggersBrowser(t *testing.T) {
	const url = "https://example.com/short-article"
	f := stubResponses(t, map[string]*http.Response{
		url: htmlResponse(shortArticleHTML),
	}, func(ctx context.Context, u string) (string, bool) {
		if u != url {
			t.Errorf("browser() called with %q; want %q", u, url)
		}
		return "# Browser Rendered\n\nBrowser-fetched content.", true
	})

	got := fetchPage(context.Background(), f, url)
	if got != "# Browser Rendered\n\nBrowser-fetched content." {
		t.Errorf("fetchPage() = %q; want the browser fallback result", got)
	}
}

func TestFetchPage_ShortReadabilityBrowserAlsoFails(t *testing.T) {
	const url = "https://example.com/short-article-2"
	f := stubResponses(t, map[string]*http.Response{
		url: htmlResponse(shortArticleHTML),
	}, func(context.Context, string) (string, bool) {
		return "", false
	})

	got := fetchPage(context.Background(), f, url)
	want := "# \n\nSource: " + url + "\n\nTinyToo short."
	if got != want {
		t.Errorf("fetchPage() = %q; want the short readability result anyway: %q", got, want)
	}
}

func TestFetchPage_NoArticleLongBodyFallsBackToBodyText(t *testing.T) {
	const url = "https://example.com/no-article"
	f := stubResponses(t, map[string]*http.Response{
		url: htmlResponse(noArticleButLongBodyHTML),
	}, func(context.Context, string) (string, bool) {
		t.Fatal("browser fallback should not be invoked when body text is usable")
		return "", false
	})

	got := fetchPage(context.Background(), f, url)
	if !strings.HasPrefix(got, "# "+url) {
		t.Errorf("fetchPage() = %q; want it to start with \"# %s\"", got, url)
	}
	if !strings.Contains(got, "long stretch of plain body text") {
		t.Errorf("fetchPage() = %q; want it to contain the body text", got)
	}
}

func TestFetchPage_NoArticleShortBodyTriggersBrowser(t *testing.T) {
	const url = "https://example.com/no-article-short"
	f := stubResponses(t, map[string]*http.Response{
		url: htmlResponse(noArticleShortBodyHTML),
	}, func(ctx context.Context, u string) (string, bool) {
		return "# Browser Rendered\n\nRendered content.", true
	})

	got := fetchPage(context.Background(), f, url)
	if got != "# Browser Rendered\n\nRendered content." {
		t.Errorf("fetchPage() = %q; want the browser fallback result", got)
	}
}

func TestFetchPage_NoExtractionAtAll(t *testing.T) {
	const url = "https://example.com/no-article-short-2"
	f := stubResponses(t, map[string]*http.Response{
		url: htmlResponse(noArticleShortBodyHTML),
	}, func(context.Context, string) (string, bool) {
		return "", false
	})

	got := fetchPage(context.Background(), f, url)
	want := "# " + url + "\n\nCould not extract readable content from this page."
	if got != want {
		t.Errorf("fetchPage() = %q; want %q", got, want)
	}
}

func TestFetchPage_TransportErrorDoesNotInvokeBrowser(t *testing.T) {
	const url = "https://example.com/transport-error"
	f := fetcher{
		do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		},
		browser: func(context.Context, string) (string, bool) {
			t.Fatal("browser fallback should not be invoked on transport error")
			return "", false
		},
	}

	got := fetchPage(context.Background(), f, url)
	if !strings.HasPrefix(got, "# Error fetching "+url) || !strings.Contains(got, "connection refused") {
		t.Errorf("fetchPage() = %q; want an error message with the transport error", got)
	}
}

func TestFetchPage_Non2xxDoesNotInvokeBrowser(t *testing.T) {
	const url = "https://example.com/not-found"
	f := fetcher{
		do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		},
		browser: func(context.Context, string) (string, bool) {
			t.Fatal("browser fallback should not be invoked on non-2xx")
			return "", false
		},
	}

	got := fetchPage(context.Background(), f, url)
	want := "# Error fetching " + url + "\n\nHTTP 404"
	if got != want {
		t.Errorf("fetchPage() = %q; want %q", got, want)
	}
}

func TestFetchPage_RedditUrlRoutesThroughRedditPath(t *testing.T) {
	const url = "https://reddit.com/r/golang/comments/abc/some_post"
	f := stubResponses(t, map[string]*http.Response{
		url + ".json": {
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(redditPostFixture)),
		},
	}, func(context.Context, string) (string, bool) {
		t.Fatal("browser fallback should not be invoked for a handled Reddit fetch")
		return "", false
	})

	got := fetchPage(context.Background(), f, url)
	if !strings.HasPrefix(got, "# Test Title") {
		t.Errorf("fetchPage() = %q; want it to be the Reddit-formatted post", got)
	}
}
