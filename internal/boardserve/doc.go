// Package boardserve is the board's loopback HTTP server.
// It serves the viewer page, the board's view projection and its live changes to the operator's browser,
// and it applies the page's writes.
//
// # Routes
//
// GET / serves the embedded viewer page.
// Its Content-Security-Policy admits only the page's own inline script and style.
// GET /api/board answers the board's view projection and its revision.
// A request with no after key answers at once.
// With after=<revision> equal to the current revision it waits, up to the long-poll bound, for the board to change.
// A board with no board.json has the empty revision.
// It then answers the new projection, or "unchanged" when the bound passes, the request ends or the server drains.
// GET /file/<name> serves a design file or decision record the projection names, as plain text.
// POST /api/write applies one of six writes: create, edit, release, promote, draft and undraft.
//
// # Checks
//
// Every request must carry the Host 127.0.0.1:<port>, which stops DNS rebinding.
// Every /api request needs the bearer token, and the file route the t query parameter, compared in constant time.
// A write also needs the JSON content type and a body within the cap.
// When it sends an Origin, that must be the server's own.
// Every response carries Cache-Control: no-store and X-Content-Type-Options: nosniff, and none carries a CORS header.
//
// # Writes
//
// A write goes through the board's own exported methods.
// So the run lock, the label rules and the task-creation rule all run.
// The server adds to them only three things.
// One is the entry revision check, which the write section makes under the write lock.
// The others are the refusal to release a task a run holds, and the rule that release, draft and undraft take a task.
// A refusal that the entry changed or vanished answers 409 with the fresh entry, and any other refusal answers 422.
//
// # Lifecycle
//
// At most one server serves a hub: Run takes an exclusive lock in the hub's board-serve state directory,
// and a second Run returns ErrRunning.
// The server binds 127.0.0.1 on a free port and draws a 128-bit token.
// It writes both, with its pid, to the state file, which only its owner can read.
// A launcher page in the same directory sends a browser to the server with the token in the address fragment.
// One watch loop wakes on a change event of board.json or on a timer.
// An event wake reads the board no sooner than a second after the previous read.
// The timer doubles from one second to eight while the board is unchanged.
// Each change is told to the long polls, so a replay or sync reaches the page.
// The server ends when its binary was replaced or no API request that carried the token came within the idle bound.
// It then releases the lock so a new server can start, and finishes the requests in flight before it returns.
package boardserve
