{"status":"success","commit_sha":"77b7ea2fe28aeccba7d9571f7c140bba2fd5b4a6","session_id":"358ab17f-342d-4f59-8d50-2c43a51e722b"}

Summary of work:
- Created internal/logger/logger.go — a thin log/slog wrapper with a package-level slog.LevelVar (default Warn), an injectable io.Writer sink, Debug/Info/Warn helpers, SetVerbosity(count int) mapping 0->Warn/1->Info/>=2->Debug, and SetOutput(io.Writer) as a test seam.
- Created internal/logger/logger_test.go — covers default-Warn silence, -v/-vv thresholds, and SetOutput capture.
- Ran go vet and go test ./internal/logger/... (pass), gofmt -l (clean).
- Committed and pushed to internal-mux.

{"status":"success","commit_sha":"77b7ea2fe28aeccba7d9571f7c140bba2fd5b4a6","session_id":"358ab17f-342d-4f59-8d50-2c43a51e722b"}
