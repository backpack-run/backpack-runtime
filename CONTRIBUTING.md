# Contributing

Install Go 1.24+, clone the repository, and run `go test ./...`, `go vet ./...`, and `go build ./cmd/backpack`. Keep CLI rendering outside runtime services. New adapters select exclusively from manifest runtime metadata and execute through a compute target. New targets must remain engine-agnostic. Add unit tests and update the compatibility table; do not claim support until a real inference smoke test passes. By contributing, you agree that your contribution is licensed under Apache-2.0.

