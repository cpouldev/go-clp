# Completion checks
1. Run `gofmt -w *.go` on Go changes.
2. Run focused tests while iterating, then `go test ./...`.
3. Run `go vet ./...`.
4. Run `go build ./...` when public types, embeds, or package wiring changed.
5. For phrase/catalogue changes, ensure the phrase and label-construction tests pass and preserve source/licence attribution.
6. Explain any omitted check in the handoff.