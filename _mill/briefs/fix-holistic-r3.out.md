All four round-3 findings addressed, but TestSyncIntegration_EventuallyPushed now fails: detached spawn re-invokes the test binary (os.Executable()), not lyx, so the push cannot land under `go test`.

{"status":"success","commit_sha":"dfd2e60e3a7915cfa23de1cc524ffba4cef490ce","session_id":"ba70dd38-3867-4146-8d6c-a24a6a83f3fb"}
