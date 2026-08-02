# go-clp

[![CI](https://github.com/cpouldev/go-clp/actions/workflows/ci.yml/badge.svg)](https://github.com/cpouldev/go-clp/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cpouldev/go-clp.svg)](https://pkg.go.dev/github.com/cpouldev/go-clp)
[![Go 1.23+](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go)](./go.mod)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](./LICENSE)

`go-clp` is a zero-dependency Go library for deterministic EU CLP/GHS mixture classification. It calculates mixture hazards, constructs labels, generates UFIs, applies date-aware Annex VI data, and produces Greek and English presentation text.

> [!WARNING]
> This library is not certified compliance software or legal advice. Only legislation published in the Official Journal of the European Union is authentic. Verify every result against the law applicable on the classification date.

## Features

| Package | Purpose |
| --- | --- |
| `go-clp` | Mixture classification, labels, bilingual presentation, and the end-to-end engine |
| `annexvi` | Embedded, date-aware harmonised SCL and M-factor registry |
| `allergens` | Cosmetics Annex III fragrance-allergen CAS and INCI registry |
| `ufi` | Country-aware Unique Formula Identifier generation and validation |

The classification core performs no I/O and has no mutable external state. Runtime use requires only the Go standard library.

## Installation

```sh
go get github.com/cpouldev/go-clp
```

Go 1.23 or newer is required.

## Quick Start

Classify concentration-resolved components with `ClassifyMixture`:

```go
package main

import (
    "fmt"

    clp "github.com/cpouldev/go-clp"
)

func main() {
    result := clp.ClassifyMixture([]clp.ClassComponent{
        {
            Name:                "sensitising fragrance",
            ConcentrationPct:    1.2,
            ConcentrationLowPct: 1.2,
            SkinSensCategory:    "1B",
            Hazards: []clp.ComponentHazard{
                {
                    Class:    "Skin sensitisation",
                    Category: "1B",
                    HCodes:   []string{"H317"},
                },
            },
        },
    }, nil, false)

    fmt.Println(result.Label.SignalWord) // Warning
    fmt.Println(result.Label.HCodes)     // [H317]
    fmt.Println(result.Label.Pictograms) // [GHS07]
}
```

Use `ParseHazardClass` at input boundaries to reject unknown class names before classification.

## End-to-End Engine

`RunEngine` accepts recipe quantities and structured supplier SDS extractions. It computes concentrations, applies regulatory registries, classifies the mixture, evaluates supported transport rules, generates a UFI when required, and builds both language views.

```go
date := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
harmonised, err := annexvi.AsOf(date)
if err != nil {
    log.Fatal(err)
}

fragranceAllergens, err := allergens.Load()
if err != nil {
    log.Fatal(err)
}

result := clp.RunEngine(clp.EngineInput{
    Recipe:         &clp.Recipe{Name: "Example fragrance", FormulationNumber: 123456},
    CompanyCountry: "FR",
    CompanyVAT:     "AB123456789",
    ProductClass:   clp.ClassLiquid,
    Lines: []clp.RecipeLine{
        {MaterialID: "fragrance", MaterialName: "Fragrance concentrate", Quantity: 1.2, Unit: "g"},
        {MaterialID: "carrier", MaterialName: "Carrier", Quantity: 98.8, Unit: "g"},
    },
    Extractions: map[string]clp.ParsedExtraction{
        "fragrance": {
            Substances: []clp.ParsedSubstance{
                {
                    Name:               "Limonene",
                    CasNumber:          "5989-27-5",
                    ConcentrationRange: "100%",
                    HCodes:             []string{"H317"},
                    Hazards: []clp.ParsedHazard{
                        {Class: "Skin sensitisation", Category: "1B"},
                    },
                },
            },
        },
    },
    Harmonised: harmonised,
    AllergenCAS: fragranceAllergens.CAS,
    InciNames:   fragranceAllergens.INCIByCAS,
})

for _, flag := range result.Flags {
    log.Printf("%s: %s", flag.Severity, flag.Message)
}
```

Treat every `SeverityBlock` flag as unresolved input. `SeverityWarn` and `SeverityInfo` identify decisions that need review or confirmation.

## UFI Generation

```go
code, err := ufi.Generate("FR", "AB123456789", 123456)
if err != nil {
    log.Fatal(err)
}
fmt.Println(code) // 27W0-KC81-E00T-V314
fmt.Println(ufi.Validate(code)) // true
```

`ufi.Generate` supports the country-specific VAT schemes published by ECHA and EU company keys. Formulation numbers must be in `0..268435455`. Validation checks syntax and checksum; it does not submit a Poison Centre Notification.

## Annex VI by Date

```go
registry, err := annexvi.AsOf(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC))
input.Harmonised = registry
```

The embedded dataset contains the 4,419-entry ATP22 state and 32 ATP23 insertions or replacements. ATP23 applies from 1 February 2027. Dates before 1 May 2026 return an empty registry; the package does not infer voluntary early application.

## Fragrance-Allergen Disclosure

The `allergens` package embeds 81 Annex III references, 167 distinct CAS numbers, and 140 distinct Common Ingredients Glossary names. It feeds the engine's advisory allergen coverage check and never changes CLP classification.

```go
registry, err := allergens.Load()
if err != nil {
    log.Fatal(err)
}
required, err := allergens.RequiresDisclosure(allergens.LeaveOn, 0.0011)
if err != nil {
    log.Fatal(err)
}
fmt.Println(required) // true

input.AllergenCAS = registry.CAS
input.InciNames = registry.INCIByCAS
```

The individual-labelling thresholds are concentrations above `0.001%` for leave-on products and `0.01%` for rinse-off products. Callers remain responsible for transition rules and the complete Cosmetics Regulation assessment.

## Greek and English Labels

```go
view := clp.BuildPresentation(clp.SdsClassification{
    Label: clp.SdsLabel{SignalWord: "Warning", HCodes: []string{"H317"}},
})

fmt.Println(view.EN.HStatements[0].Text)
fmt.Println(view.EL.HStatements[0].Text)
```

Runnable examples also live in `example_test.go` and each public subpackage.

## Project Layout

```text
.
├── api.go                 Public root-package facade
├── allergens/             Cosmetics Annex III registry
├── annexvi/               Harmonised classification registry
├── ufi/                   UFI generation and validation
├── internal/core/         Classification engine and white-box tests
├── internal/*source/      EUR-Lex build-time parsers
├── cmd/                   Data generators and private comparison tool
└── .github/workflows/     Public CI and private ECHA comparison
```

## Development

```sh
go test ./...          # all tests and examples
go test -race ./...    # race detector
go vet ./...           # static analysis
gofmt -l .             # formatting check; should print nothing
go build ./...         # compile every package and command
```

Regenerate the pinned EUR-Lex datasets with `go generate ./annexvi` and `go generate ./allergens`. To compare a manually exported ECHA CSV privately, run:

```sh
go run ./cmd/annexvi-crosscheck -echa /secure/path/annex-vi.csv
```

The command performs no network request and writes no ECHA-derived artifact.

## Scope and Limitations

- The engine does not implement Annex I §1.1.3 bridging principles.
- Physical hazards cover flash-point-gated flammable liquids only.
- Transport covers a fragrance-focused subset: UN 1170, 1197, 1266, 3077, and 3082, including LQ/EQ and SP375 decisions.
- Rendered regulatory text is available in Greek and English only.
- Consolidated EUR-Lex texts aid documentation but have no legal force.

## Data and License

Code is licensed under Apache-2.0. Embedded EU reference data is reused under CC BY 4.0. See [NOTICE](./NOTICE) for sources, transformations, and attribution.
