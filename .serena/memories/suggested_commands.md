# Commands
- `go test ./...` — run the complete suite.
- `go test -run '^TestName$' ./...` — run a focused test; broader prefixes such as `^TestClassifyMixture_` are common.
- `go build ./...` — compile all package code.
- `go vet ./...` — run standard static analysis.
- `gofmt -w *.go` — format this flat package's Go sources.
- `gofmt -l *.go` — list formatting drift without modifying files.
- `go test -cover ./...` — inspect coverage when adding or changing rules.