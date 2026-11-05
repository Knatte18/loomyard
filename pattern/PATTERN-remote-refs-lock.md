# PATTERN-remote-refs-lock

A fetch, pull or push that writes remote-tracking refs writes them under the repository's remote-refs lock, taken through `gitrepo`, and waits while another process holds it.

- The process passes the pause gate before it takes the lock, and its network calls all pass the gate at once until it releases the lock.
  A paused process therefore never holds the lock, and no other process, an exempt cancel included, waits on a pause through it.
- A push sends to the remote's push URL, which writes no remote-tracking ref, so its transfer, retries included, runs outside the lock.
  Once the transfer succeeded, the push takes the lock to write the branch's tracking ref, and the upstream setting of a first push.
  A tracking ref a later fetch already moved past the pushed commit stays where it is.
- A tag push, a `--refmap=` fetch and a probe-clone fetch write no remote-tracking ref and take no lock.
- A judge of origin against tracking refs holds the lock from reading origin to its verdict, so no tracking-ref write lands between the read and the verdict.
  A push transfer that lands meanwhile is judged against the origin read before it.

A test enforces it.
