// Package clp classifies chemical mixtures under EU Regulation (EC) No
// 1272/2008 (CLP) and builds the resulting GHS label.
//
// The engine is a pure function of its input. It performs no I/O, calls no
// network service, reads no database, and uses no language model: given a
// composition and the supplier hazard data for each component, RunEngine
// returns the same classification every time. Every threshold it applies is a
// named constant carrying its Annex I table reference, so a result can be
// audited back to the legal text rather than taken on trust.
//
// # What it implements
//
// Mixture classification per Annex I, with the method chosen per hazard class
// by an explicit registry (see internal/core/method_table.go) rather than by ad-hoc branching
// — summing a per-component class, or vice versa, silently mis-classifies:
//
//   - Acute toxicity by the additivity formula 100 / Σ(Ci/ATEi), with
//     route-specific cut-offs and converted point estimates (Table 3.1.2).
//   - Skin corrosion/irritation and eye damage/irritation by summation, with
//     the ×10 corrosive-to-irritant boost (Tables 3.2.3, 3.3.3).
//   - Aquatic hazards by the summation cascade with M-factors and the 10×/100×
//     tier coefficients (Table 4.1.0).
//   - CMR, skin and respiratory sensitisation, and lactation per component
//     against generic or substance-specific concentration limits — never summed.
//   - STOT single- and repeated-exposure, and aspiration hazard.
//
// Label assembly applies Article 26 pictogram precedence (including the 26(d)
// carve-out that spares GHS07 from respiratory-sensitisation suppression),
// orders hazard statements physical → health → environmental, applies the
// EUH208 disclosure bands with the Annex II §2.8 suppression rule, and prunes
// precautionary statements to the six permitted by Annex IV §6.3 while
// retaining the full set for safety data sheet section 2.2.
//
// Substance-specific concentration limits and M-factors from Annex VI override
// the generic limits when a harmonised registry is supplied; see EngineInput.
//
// # What it does not implement
//
// The bridging principles of Annex I §1.1.3 (dilution, batching, concentration
// of highly hazardous mixtures, interpolation, substantially similar mixtures,
// aerosols) are absent: this engine classifies from full component composition
// only. Physical hazards are limited to flash-point-gated flammable liquids.
// Transport classification covers a fragrance-scoped subset of ADR/IATA.
// Rendered text is available in Greek and English only.
//
// # Legal status
//
// This package is not certified compliance software and does not constitute
// legal or regulatory advice. Only European Union legislation published in the
// Official Journal of the European Union is authentic. Verify every
// classification against the applicable legal text before relying on it.
package clp
