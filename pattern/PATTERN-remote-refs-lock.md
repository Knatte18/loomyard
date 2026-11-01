# PATTERN-remote-refs-lock

A fetch, pull or push that writes remote-tracking refs runs under the repository's remote-refs lock, taken through `gitrepo`, and waits while another process holds it.

- The process passes the pause gate before it takes the lock, and its network calls all pass the gate at once until it releases the lock.
  A paused process therefore never holds the lock, and no other process, an exempt cancel included, waits on a pause through it.
- A tag push, a `--refmap=` fetch and a probe-clone fetch write no remote-tracking ref and take no lock.
- A judge of origin against tracking refs holds the lock from reading origin to its verdict, so no write lands between the read and the verdict.

A test enforces it.
