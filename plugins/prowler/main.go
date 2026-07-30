// main.go wires the production fetcher and is the binary's entrypoint: it
// parses the URL arguments, runs the fetch cascade for each, joins the
// results, writes them to a scratch output file, and prints that file's
// path — the single line of stdout the invoking skill wrapper captures.

package main

import (
	"context"
	"fmt"
	"os"
)

// resultJoiner separates each URL's fetched content in the combined output,
// mirroring weblens' multi-URL join so a caller reading the file sees a
// clear boundary between sources.
const resultJoiner = "\n\n---\n\n"

// newFetcher wires the real, side-effecting implementations into a fetcher:
// httpClient.Do for the transport, fetchWithBrowser for the headless-Chrome
// fallback, and defaultAdapters() for the site-specific strategies fetchPage
// tries first. It is the only place production code constructs a fetcher —
// every other package function receives one as a parameter so it can be
// exercised in tests with stubs instead.
func newFetcher() fetcher {
	return fetcher{
		do:       httpClient.Do,
		browser:  fetchWithBrowser,
		adapters: defaultAdapters(),
	}
}

// runAll fetches every URL in urls via f and joins their results with
// resultJoiner, preserving input order. A per-URL failure is already
// captured inline as that URL's own "# Error fetching ..." result string
// (see fetchPage), so one bad URL never aborts or drops its siblings'
// results — the batch always returns len(urls) joined segments.
func runAll(ctx context.Context, f fetcher, urls []string) string {
	results := make([]string, len(urls))

	// Fetch every URL concurrently: each is an independent network
	// operation, so there is no reason to serialize them and make a
	// multi-URL call as slow as the sum of its parts.
	done := make(chan struct{})
	for i, u := range urls {
		go func(i int, u string) {
			results[i] = fetchPage(ctx, f, u)
			done <- struct{}{}
		}(i, u)
	}
	for range urls {
		<-done
	}

	joined := ""
	for i, r := range results {
		if i > 0 {
			joined += resultJoiner
		}
		joined += r
	}
	return joined
}

// main is the binary's entrypoint. It reads the requested URLs from the
// command line, runs the fetch cascade against each, and writes the
// combined markdown to a uniquely-named scratch file. Per the stdout/stderr
// discipline the invoking skill wrapper depends on, the created file's
// absolute path is the ONLY line ever printed to stdout — every diagnostic
// goes to stderr, so `path=$(run.sh ...)` always captures a clean path.
func main() {
	urls := os.Args[1:]
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: prowler <url> [url2] [url3]...")
		os.Exit(1)
	}

	joined := runAll(context.Background(), newFetcher(), urls)

	path, err := writeOutput(urls[0], joined)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prowler: failed to write output file: "+err.Error())
		os.Exit(1)
	}

	fmt.Println(path)
}
