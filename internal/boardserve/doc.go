// Package boardserve is the board's loopback server: the HTTP handler behind the viewer page.
//
// # Routes
//
// `GET /` serves the viewer page, with a Content-Security-Policy that admits only its own inline script and style.
// `GET /api/board` answers the board's view projection with its revision.
// With `after=<revision>` it waits, up to a bound, until the board has another revision.
// A wait that ends without one answers "unchanged".
// `GET /file/<name>` serves a design or decision file the projection names, as plain text.
// `POST /api/write` runs one write: create, edit, release, promote, draft or undraft.
// A write answers the entry as it stands, a stale one answers 409 with the fresh entry,
// and any other refusal answers 422 with the board's message.
//
// # Checks
//
// Every request must name the server's own loopback address as its Host.
// An API request carries the token as a bearer credential, and a file request as the `t` query parameter.
// A write must be JSON, come from the server's own origin when it names one, and stay within the body cap.
// No response is cacheable, and none carries a CORS header.
//
// # Writing the board
//
// The package writes the board only through boardengine's exported board methods.
// Each write expects the entry revision its request read.
package boardserve
