// Package annexvi provides a lazily loaded, date-aware CLP Annex VI Table 3
// registry derived from EUR-Lex.
//
// AsOf returns the CAS-keyed form accepted by clp.EngineInput.Harmonised.
// EntriesAsOf exposes the underlying legal rows and application windows. The
// embedded ATP23 dataset models the ATP22 snapshot from 2026-05-01 and applies
// Regulation (EU) 2025/1222 from 2027-02-01.
package annexvi
