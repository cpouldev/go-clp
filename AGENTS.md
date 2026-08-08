# Repository Guidelines

## Working principles

- Clarify requirements that would materially change architecture or product behavior. For small gaps, choose the simplest safe interpretation and record it.
- Preserve unrelated work in this fresh worktree. Do not reset, overwrite, or broadly reformat files outside the task.
- Prefer small, typed, testable changes. Keep external side effects behind durable local state and idempotent/retry-safe boundaries.
- Treat eligibility, consent, billing, authentication, and account deletion as correctness-critical. Fail closed when source evidence or state is ambiguous.
- Never commit plaintext credentials, local certificates, database dumps, or production environment files.

## Project Structure & Module Organization

The root `clp` package is the public facade and the only supported import path: `api.go` exposes the supported API and `doc.go` documents its scope. Deterministic classification rules and white-box tests live in `internal/core/`. Public companion packages are `annexvi/` for the embedded harmonised registry, `allergens/` for the advisory cosmetics registry, and `ufi/` for identifier generation. Build-time parsers and comparison logic belong under `internal/`; executable generators belong under `cmd/`. Embedded datasets are stored below each package's `data/` directory. Preserve attribution in `NOTICE` and `internal/core/clpdata/NOTICE.md`.

## Build, Test, and Development Commands

- `go test ./...` runs all unit tests and executable examples.
- `go test -race ./...` checks concurrent code and lazy dataset loading.
- `go vet ./...` runs standard static analysis.
- `gofmt -l .` lists files needing formatting; use `gofmt -w .` to fix them.
- `go build ./...` compiles every package and command.
- `go generate ./annexvi ./allergens` regenerates pinned EUR-Lex blobs and requires network access.

The module targets Go 1.23 and has no third-party runtime dependencies.

## Coding Style & Naming Conventions

Accept `gofmt` output and idiomatic tab indentation. Use MixedCaps for exported identifiers and descriptive lower-case names internally. Add doc comments to exported APIs. Keep `internal/core` deterministic and free of I/O or mutable external state. Represent regulatory thresholds with named constants and cite the relevant Annex, table, or pinned CELEX near each rule.

## Testing Guidelines

Use Go's `testing` package and keep tests beside their package as `*_test.go`. Prefer table-driven cases with `t.Run`; name tests `TestFeature_Scenario`. Cover exact threshold boundaries, precedence, invalid input, date cutovers, and Greek/English output. Data-parser changes need fixture tests plus count guards for generated registries. Run the full suite before opening a pull request.

## Commit & Pull Request Guidelines

Release Please derives versions from commits on `main`. Use Conventional Commits for every commit and for PR titles that may become squash commits: `fix(classifier): reject an invalid threshold` releases a patch, `feat(annexvi): add a registry query` a minor, and `feat(api)!: remove a public field` a major. A `BREAKING CHANGE: <description>` footer also marks a major release. Prefer squash merges, keep descriptions imperative and lower-case, and do not create release tags or edit versions manually.

Merge the generated Release Please PR when the queued changes are ready to publish; that merge creates the GitHub release and its `vMAJOR.MINOR.PATCH` tag. Release Please owns `CHANGELOG.md` and the version in `.release-please-manifest.json`. Never edit either by hand. The `0.0.0` in the manifest means "nothing released yet", not a version to bump from; until a release exists, Release Please ignores the bump settings and takes the version from `initial-version`, which the config pins to `0.1.0`. From there `bump-minor-pre-major` keeps breaking changes inside `0.x`, so reach 1.0 deliberately. Go module paths carry the major version from v2 onward. Cutting `v2.0.0` therefore means adding the `/v2` suffix to the `module` line in `go.mod` and to every internal import; Release Please never touches them.

Keep changes focused and avoid mixing generated-data updates with unrelated refactors. Pull requests should explain behavioral and compliance impact, link the issue and authoritative source, list verification commands, and identify changed embedded data or attribution. Include screenshots only when downstream rendered output changes.
