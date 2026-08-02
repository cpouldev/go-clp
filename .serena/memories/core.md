# Project map
- Single Go package `clp` at repository root; no executable or nested code module.
- `engine.go` orchestrates the pure deterministic classification pipeline. Domain areas are split across composition, mixture classification, routing, transport, presentation, harmonised lookup, UFI, and validation files.
- Public data contracts live primarily in `../../internal/core/types.go`; package scope, omissions, and legal caveats are documented in `doc.go`.
- Embedded bilingual CLP statement catalogues live under `../../internal/core/clpdata/` and are loaded by `../../internal/core/clp_embed.go`.
- Read `mem:tech_stack` for toolchain/data dependencies, `mem:conventions` for regulatory and test conventions, `mem:suggested_commands` for daily commands, and `mem:task_completion` before handing off code changes.