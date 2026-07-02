All five cards for batch cmd-lyx-integration implemented, committed, verified. main.go registers internal/muxcli.Command(), removes muxpoccli wiring, adds -v/--verbose count flag -> logger.SetVerbosity via root PersistentPreRunE (EnableTraverseRunHooks). Updated helptree/jsonhelp/unknown_subcommand/registration/sandbox_coverage tests muxpoc->mux. configreg registers muxengine.ConfigTemplate. sandbox SANDBOX-SUITE.md scenario S9. go build/vet/test all pass.

{"status":"success","commit_sha":"2429c56beb4f97416343f0f8c12c2d9001b1388c","session_id":"0c79d409-4703-4771-8adc-29a186882428"}
