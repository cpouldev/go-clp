# Conventions
- Preserve a pure, deterministic engine: classification code performs no I/O, network access, database reads, or model calls.
- Use standard Go formatting and naming: tabs via `gofmt`, MixedCaps identifiers, exported-symbol comments, and lower-case unexported helpers.
- Keep regulatory thresholds as named constants and retain Annex/table references near rule implementations.
- Tests stay in package `clp`, beside sources as `*_test.go`; use `TestFeature_Scenario`, table-driven cases, and `t.Run`.
- Test exact threshold boundaries and precedence interactions, not only typical inputs.
- `clp_phrases.go` curated entries override the embedded catalogue; preserve `// ref` provenance markers and `../../internal/core/clpdata/NOTICE.md` attribution when changing regulatory text/data.