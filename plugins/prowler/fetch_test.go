// fetch_test.go exercises the static-fetch extraction cascade with a stubbed
// fetcher.do (canned responses keyed by request URL) and a stubbed
// fetcher.browser. No network, no Chrome.

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
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

// redditLikeHTMLWithComments mimics old.reddit.com's shape well enough to
// exercise the property fetchOldRedditHTML exists for: the self-post text
// and the comment thread sit in separate divs, neither of which is
// nav/header/footer, so both must survive stripToBodyText's strip — unlike
// Readability, which (verified empirically against the real page) keeps
// only the post and silently discards the entire comment thread.
const redditLikeHTMLWithComments = `<html><head><title>Ignored</title></head><body>
<nav>site nav — must not appear in output</nav>
<div class="thing"><p>This is the original self-post text, long enough on its own to clear the usable-content threshold for this fallback path.</p></div>
<div class="commentarea">
  <div class="comment"><p>First commenter's opinion, also reasonably long so the combined body text is well past the threshold either way.</p></div>
  <div class="comment"><p>Second commenter disagrees and explains why at some length here.</p></div>
</div>
<footer>site footer — must not appear in output</footer>
</body></html>`

func TestFetchOldRedditHTML(t *testing.T) {
	const url = "https://www.reddit.com/r/golang/comments/abc/some_post"
	const oldURL = "https://old.reddit.com/r/golang/comments/abc/some_post"

	t.Run("keeps_post_and_comments_strips_chrome", func(t *testing.T) {
		f := stubResponses(t, map[string]*http.Response{
			oldURL: htmlResponse(redditLikeHTMLWithComments),
		}, nil)

		out, ok := fetchOldRedditHTML(context.Background(), f, url)
		if !ok {
			t.Fatalf("fetchOldRedditHTML() ok = false; want true")
		}
		if !strings.HasPrefix(out, "# "+url) {
			t.Errorf("fetchOldRedditHTML() out = %q; want it to start with \"# %s\"", out, url)
		}
		if !strings.Contains(out, "original self-post text") {
			t.Errorf("fetchOldRedditHTML() out = %q; want the post text", out)
		}
		if !strings.Contains(out, "First commenter's opinion") || !strings.Contains(out, "Second commenter disagrees") {
			t.Errorf("fetchOldRedditHTML() out = %q; want both comments kept, not just the post", out)
		}
		if strings.Contains(out, "site nav") || strings.Contains(out, "site footer") {
			t.Errorf("fetchOldRedditHTML() out = %q; want nav/footer stripped", out)
		}
	})

	t.Run("no_article_long_body_falls_back_to_body_text", func(t *testing.T) {
		f := stubResponses(t, map[string]*http.Response{
			oldURL: htmlResponse(noArticleButLongBodyHTML),
		}, nil)

		out, ok := fetchOldRedditHTML(context.Background(), f, url)
		if !ok {
			t.Fatalf("fetchOldRedditHTML() ok = false; want true")
		}
		if !strings.Contains(out, "long stretch of plain body text") {
			t.Errorf("fetchOldRedditHTML() out = %q; want the body text", out)
		}
	})

	t.Run("thin_content_fails", func(t *testing.T) {
		f := stubResponses(t, map[string]*http.Response{
			oldURL: htmlResponse(noArticleShortBodyHTML),
		}, nil)

		out, ok := fetchOldRedditHTML(context.Background(), f, url)
		if ok {
			t.Fatalf("fetchOldRedditHTML() ok = true, out = %q; want false", out)
		}
	})

	t.Run("non_2xx_fails", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("blocked"))}, nil
		}}

		out, ok := fetchOldRedditHTML(context.Background(), f, url)
		if ok {
			t.Fatalf("fetchOldRedditHTML() ok = true, out = %q; want false", out)
		}
	})

	t.Run("transport_error_fails", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}}

		out, ok := fetchOldRedditHTML(context.Background(), f, url)
		if ok {
			t.Fatalf("fetchOldRedditHTML() ok = true, out = %q; want false", out)
		}
	})

	t.Run("unsupported_content_encoding_fails", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				// "compress" has no decoder in this package or the standard
				// library, unlike gzip/deflate/br.
				Header: http.Header{"Content-Encoding": []string{"compress"}},
				Body:   io.NopCloser(strings.NewReader("compressed-bytes-irrelevant")),
			}, nil
		}}

		out, ok := fetchOldRedditHTML(context.Background(), f, url)
		if ok {
			t.Fatalf("fetchOldRedditHTML() ok = true, out = %q; want false", out)
		}
	})
}

// gzipCompress compresses body for use as a stubbed gzip-encoded response.
func gzipCompress(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("gzip.Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip.Close() error = %v", err)
	}
	return buf.Bytes()
}

func TestFetchPage_GzipContentEncodingIsDecoded(t *testing.T) {
	const url = "https://example.com/gzipped"
	f := stubResponses(t, map[string]*http.Response{
		url: {
			StatusCode: 200,
			Header:     http.Header{"Content-Encoding": []string{"gzip"}},
			Body:       io.NopCloser(bytes.NewReader(gzipCompress(t, readableArticleHTML))),
		},
	}, func(context.Context, string) (string, bool) {
		t.Fatal("browser fallback should not be invoked once gzip is decoded")
		return "", false
	})

	got := fetchPage(context.Background(), f, url)
	if !strings.Contains(got, "genuinely readable article content") {
		t.Errorf("fetchPage() = %q; want the decoded article body, not compressed bytes", got)
	}
}

// brotliCompress compresses body for use as a stubbed br-encoded response.
func brotliCompress(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("brotli.Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("brotli.Close() error = %v", err)
	}
	return buf.Bytes()
}

func TestFetchPage_BrotliContentEncodingIsDecoded(t *testing.T) {
	const url = "https://example.com/brotli"
	f := stubResponses(t, map[string]*http.Response{
		url: {
			StatusCode: 200,
			Header:     http.Header{"Content-Encoding": []string{"br"}},
			Body:       io.NopCloser(bytes.NewReader(brotliCompress(t, readableArticleHTML))),
		},
	}, func(context.Context, string) (string, bool) {
		t.Fatal("browser fallback should not be invoked once brotli is decoded")
		return "", false
	})

	got := fetchPage(context.Background(), f, url)
	if !strings.Contains(got, "genuinely readable article content") {
		t.Errorf("fetchPage() = %q; want the decoded article body, not compressed bytes", got)
	}
}

func TestFetchPage_UnsupportedContentEncodingFallsBackToBrowser(t *testing.T) {
	const url = "https://example.com/compress"
	f := stubResponses(t, map[string]*http.Response{
		url: {
			StatusCode: 200,
			// "compress" (legacy Unix compress/LZW) has no decoder in this
			// package or the standard library, unlike gzip/deflate/br.
			Header: http.Header{"Content-Encoding": []string{"compress"}},
			Body:   io.NopCloser(strings.NewReader("compressed-bytes-irrelevant")),
		},
	}, func(ctx context.Context, u string) (string, bool) {
		if u != url {
			t.Errorf("browser() called with %q; want %q", u, url)
		}
		return "# Browser Rendered\n\nDecoded via the real browser's network stack.", true
	})

	got := fetchPage(context.Background(), f, url)
	want := "# Browser Rendered\n\nDecoded via the real browser's network stack."
	if got != want {
		t.Errorf("fetchPage() = %q; want %q", got, want)
	}
}
