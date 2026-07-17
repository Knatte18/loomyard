{"status":"success","commit_sha":"c44fd6c212837964c4040320cd8fa204b81deebd","session_id":"770c3b25-6aee-4d5a-9bbf-95e9c7a2428b"}

Both NIT findings from the holistic review applied:
1. Moved the exec.LookPath("gopls") skip in refs_integration_test.go into the first t.Run subtest only, so the ErrServerNotFound subtest runs unconditionally. Commit 28101228.
2. Added untagged/spawn-free refs_test.go with TestReferences_NonExistentServerBinaryYieldsErrServerNotFound (added file to plan Card 11 Creates + overview first, commit 4e82ed8f). Commit c44fd6c2.

All four batch verify commands passed. No uncommitted tracked changes. Pushed.
