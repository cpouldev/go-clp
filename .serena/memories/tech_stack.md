# Stack
- Go module `github.com/cpouldev/go-clp`, language level Go 1.23.
- Standard library only; no third-party module dependencies.
- Uses `go:embed` for Greek and English JSON CLP statement catalogues in `../../internal/core/clpdata/`.
- Library package only: `go build ./...` compiles it but produces no runnable application.
- Vendored catalogue provenance/licensing and refresh notes are in `../../internal/core/clpdata/NOTICE.md`.