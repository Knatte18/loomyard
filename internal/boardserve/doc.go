// Package boardserve serves a hub's board to a browser page over loopback HTTP.
//
// # Routes
//
// `GET /` serves the viewer page, which holds no board data.
// Its content security policy admits only the page's own inline script and style, by hash.
// `GET /api/board` answers the board's view projection and its revision.
// With `after=<revision>` naming the current revision it waits a bounded time for another one.
// When the wait ends with no other revision, it answers "unchanged".
// `GET /file/<name>` serves a design or decision file the current projection names, and no other file.
// `POST /api/write` runs one write of the page: create, edit, release, promote, draft or undraft.
//
// # Checks
//
// Every request must name the host 127.0.0.1 at the bound port, which stops DNS rebinding.
// An `/api` request carries the token as a bearer token, and a file request as the query parameter `t`.
// A write is JSON, comes from the page's own origin when it names one, and is bounded in size.
// No response is cached or carries a CORS header.
//
// # Writes
//
// The package writes the board only through boardengine's exported board methods.
// The command line calls the same methods, so the board's own rules all apply.
// Every write but a create carries the revision of the entry the page read,
// and a stale one is refused with the entry as the board holds it now.
// On top of the board's rules the server refuses release, draft and undraft of anything but a task,
// and the release of a task a run holds.
package boardserve
