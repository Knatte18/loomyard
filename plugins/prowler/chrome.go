// chrome.go locates a Chrome/Chromium executable on the host so the browser
// fallback (browser.go) knows what binary to launch. Discovery never shells
// out — it only checks the filesystem, so it stays fast and side-effect free
// even when no browser fallback ends up being used.

package main

import "os"

// chromeCandidates lists the well-known install locations checked, in order,
// after CHROME_PATH. The order mirrors weblens' own candidate list so
// behavior stays consistent across the Windows/macOS/Linux install paths the
// operator actually uses.
var chromeCandidates = []string{
	"C:/Program Files/Google/Chrome/Application/chrome.exe",
	"C:/Program Files (x86)/Google/Chrome/Application/chrome.exe",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/usr/bin/google-chrome",
	"/usr/bin/chromium-browser",
}

// findChromeExecutable locates a Chrome/Chromium binary to drive for the
// headless-browser fallback. It checks CHROME_PATH first (an explicit
// operator override), then a fixed candidate list of common install
// locations, in order, returning the first path that exists on disk. It
// returns "" when none of the candidates exist, signaling the browser
// fallback should be skipped rather than fail the whole fetch.
func findChromeExecutable() string {
	if envPath := os.Getenv("CHROME_PATH"); envPath != "" {
		if pathExists(envPath) {
			return envPath
		}
	}

	for _, candidate := range chromeCandidates {
		if pathExists(candidate) {
			return candidate
		}
	}

	return ""
}

// pathExists reports whether path names a file or directory that exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
