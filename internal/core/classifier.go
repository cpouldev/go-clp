package core

import (
	"strings"
)

// classifier.go is the deterministic CLP regulatory core: pure, zero-I/O
// functions that classify the three decisive classes — skin sensitisation
// (per-component, no additivity), aquatic toxicity (summation with M-factors),
// and flammable data-completeness. ClassifySkinSens and ClassifyAquatic feed the
// engine's finished-mixture classification; CheckFlammable turns a missing flash
// point into a FLAG rather than a silently-shipped guess.
//
// The numeric boundaries here are legally load-bearing: the CLP thresholds
// (Annex I §3.4 skin sensitisation, §4.1.3 aquatic summation) are decision
// boundaries, so every threshold comparison uses ">=" at the boundary value.
// A ">" vs ">=" slip would be a regulatory mis-classification.
//
// All functions are pure — they read only their arguments and return new
// values, with no I/O, no globals, and no time dependence.

// ── CLP class constants ───────────────────────────────────────────────────────

// Skin-sensitisation generic concentration limits (GCL), % w/w. CLP 1272/2008
// Annex I Table 3.4.6: Sens 1 / 1B classify the mixture (H317) at ≥ 1.0% and
// trigger the supplementary EUH208 statement at ≥ 0.1%; the more potent Sens 1A
// classifies at ≥ 0.1% and triggers EUH208 at ≥ 0.01%. EUH208 is consistently
// one tenth of the active H317 limit (the CLP "1/10 of the relevant GCL" rule),
// which is why an SCL override scales both limits together.
const (
	gclSens1bH317   = 1.0  // Sens 1 / 1B → H317
	gclSens1aH317   = 0.1  // Sens 1A     → H317
	euh208Divisor   = 10.0 // EUH208 limit = active H317 limit ÷ 10
	skinSensClass1  = "Skin Sens. 1"
	aquaticThreshld = 25.0 // summation ≥ 25% — acute and every chronic rung (incl. the Chronic 4 safety net)
)

// Aquatic classification codes, ordered by the top-down cascade (§4.1.3): the
// first rung whose summation reaches the threshold wins.
const (
	aquaticAcute1 = "Aquatic Acute 1"   // H400
	aquaticChron1 = "Aquatic Chronic 1" // H410
	aquaticChron2 = "Aquatic Chronic 2" // H411
	aquaticChron3 = "Aquatic Chronic 3" // H412
	aquaticChron4 = "Aquatic Chronic 4" // H413
)

// ── Skin sensitisation (Annex I §3.4 — per-component, NO additivity) ──────────

// ClassifySkinSens applies the skin-sensitisation criteria to each component
// independently — there is no additivity for this class. The highest-ranking
// outcome across all components becomes the mixture's classification: a single
// component at or above its H317 limit classifies the whole mixture Skin Sens. 1
// (returned class), and EUH208 (returned as a warn FLAG) applies whenever any
// component sits in the EUH208 band but below the H317 limit.
//
// Each component's active H317 limit is its substance-specific concentration
// limit (SCL) when one is listed in the harmonised classification; otherwise the
// generic limit (GCL) for its category (Sens 1A = 0.1%, Sens 1 / 1B = 1.0%).
// The EUH208 limit is one tenth of that active limit. The returned class is
// "Skin Sens. 1" when classified and "" when not; the FLAG carries EUH208 when
// it (but not H317) is reached for at least one component.
func ClassifySkinSens(components []ClassComponent) (string, []Flag) {
	classified := false
	euh208 := false

	for _, c := range components {
		c = withDedicatedFields(c)
		h317Limit, ok := skinSensH317Limit(c)
		if !ok {
			continue // not a skin sensitiser
		}

		// CLP boundaries are inclusive: a component AT its limit triggers.
		switch {
		case reachesLimit(c.ConcentrationPct, h317Limit):
			classified = true
		case reachesLimit(c.ConcentrationPct, h317Limit/euh208Divisor):
			euh208 = true
		}
	}

	if classified {
		// H317 supersedes the supplementary EUH208 statement entirely.
		return skinSensClass1, nil
	}
	if euh208 {
		return "", []Flag{
			{
				Section:  "2.2",
				Code:     FlagNotClassified,
				Severity: SeverityInfo,
				Message:  "Not classified Skin Sens. 1; supplementary statement EUH208 applies (a sensitising component is present at or above its EUH208 limit).",
			},
		}
	}
	return "", nil
}

// skinSensH317Limit returns the active H317 concentration limit (% w/w) for a
// component and whether the component is a skin sensitiser at all. An SCL, when
// present, overrides the generic limit for the component's category.
func skinSensH317Limit(c ClassComponent) (float64, bool) {
	gcl, ok := gclFor(kindSkinSens, c.SkinSensCategory)
	if !ok {
		return 0, false
	}
	if c.SkinSensSCLPct > 0 {
		return c.SkinSensSCLPct, true // SCL overrides GCL
	}
	return gcl, true
}

// ── Aquatic (Annex I §4.1.3 — summation with M-factors, top-down cascade) ─────

// ClassifyAquatic applies the aquatic-environment summation method and returns
// the single highest-priority class via the top-down cascade
// (Acute 1 → Chronic 1 → Chronic 2 → Chronic 3 → Chronic 4); the first rung
// whose summation reaches 25% wins. M-factors attach ONLY to Aquatic Acute 1
// and Aquatic Chronic 1 substances; every other contribution uses M = 1.
//
// The cross-tier weighting follows CLP 1272/2008 Annex I Table 4.1.0 exactly:
// each more-toxic tier carries an EXTRA 10× per rung it is folded down through.
// The Chronic-1 sum is therefore weighted 10× at the Chronic 2 rung and 100× at
// the Chronic 3 rung (a Chronic 1 component is two rungs more toxic than a
// Chronic 3 component → 10× × 10× = 100×); the Chronic-2 sum is weighted 10× at
// the Chronic 3 rung. Anything weaker than the rung itself uses 1×.
//
// The returned class is one of the aquatic* constants, or "" when no rung is
// reached. No FLAGs are produced here; the slice is returned for signature
// symmetry with the other classifiers and is always nil.
func ClassifyAquatic(components []ClassComponent) (string, []Flag) {
	var acuteSum, chron1Sum, chron2Sum, chron3Sum, chron4Sum, chron1Raw float64

	for _, c := range components {
		c = withDedicatedFields(c)
		switch normHazardCategory(c.AquaticAcuteCategory) {
		case "1":
			acuteSum += c.ConcentrationPct * mFactor(c.MAcute)
		}
		switch normHazardCategory(c.AquaticChronicCategory) {
		case "1":
			chron1Sum += c.ConcentrationPct * mFactor(c.MChronic)
			chron1Raw += c.ConcentrationPct // unweighted, for the Chronic 4 safety net
		case "2":
			chron2Sum += c.ConcentrationPct // M-factor never applies below Chronic 1
		case "3":
			chron3Sum += c.ConcentrationPct
		case "4":
			chron4Sum += c.ConcentrationPct
		}
	}

	// Top-down cascade — inclusive ">=" at the 25% boundary; first hit wins
	// (CLP 1272/2008 Annex I Table 4.1.0, summation method). The cross-tier
	// coefficients are NOT a uniform 10×: the Chronic-1 sum is folded into the
	// Chronic 3 rung at 100×, because a Chronic 1 component is two tiers more
	// toxic than a Chronic 3 component (10× per tier, twice). Getting the
	// Chronic-3 Chronic-1 coefficient wrong (10× instead of 100×) silently
	// UNDER-classifies in the dangerous direction — e.g. a lone Chronic 1 at
	// 0.3% is Chronic 3 (100×0.3 = 30 ≥ 25) but would read as not-classified
	// under a 10× slip (3 < 25).
	//
	// The Chronic 4 rung is the "safety net" (Table 4.1.0, last row): the
	// UNWEIGHTED sum of the concentrations of ALL chronically-classified
	// components (Chronic 1 + 2 + 3 + 4, no M-factor) ≥ 25%. It is a 25%
	// summation like every other rung — NOT a per-component "any Chronic 4 ≥ 1%"
	// trigger (which both over-classified a lone sub-25% Chronic 4 and
	// under-classified many sub-1% Chronic 4 components summing past 25%).
	switch {
	case reachesLimit(acuteSum, aquaticThreshld):
		return aquaticAcute1, nil
	case reachesLimit(chron1Sum, aquaticThreshld):
		return aquaticChron1, nil
	case reachesLimit(10*chron1Sum+chron2Sum, aquaticThreshld):
		return aquaticChron2, nil
	case reachesLimit(100*chron1Sum+10*chron2Sum+chron3Sum, aquaticThreshld):
		return aquaticChron3, nil
	case reachesLimit(chron1Raw+chron2Sum+chron3Sum+chron4Sum, aquaticThreshld):
		return aquaticChron4, nil
	default:
		return "", nil
	}
}

// mFactor returns the effective multiplier for a component: the explicit
// M-factor when set (> 0), or the default of 1 otherwise.
func mFactor(m int) float64 {
	if m > 0 {
		return float64(m)
	}
	return 1
}

// normCategory trims and upper-cases a category string for comparison.
func normCategory(category string) string {
	return strings.ToUpper(strings.TrimSpace(category))
}

// normHazardCategory canonicalises a CLP hazard category as suppliers actually
// write it: "Cat. 1B", "Category 1B", "cat 1b" and " 1B " all resolve to "1B".
//
// The method table keys on the bare category, and an unmatched key drops the
// hazard from classification entirely, so this normalisation is load-bearing
// rather than cosmetic. Anything it still cannot resolve is reported by
// invalidHazardCategoryFlags instead of being silently ignored.
func normHazardCategory(category string) string {
	c := normCategory(category)
	for _, prefix := range []string{"CATEGORY", "CAT.", "CAT"} {
		if rest, found := strings.CutPrefix(c, prefix); found {
			c = strings.TrimSpace(rest)
			break
		}
	}
	return strings.TrimSpace(strings.TrimPrefix(c, "."))
}

// ── Flammable liquid (Annex I §2.6 — physical, NOT computed from components) ──

// CheckFlammable performs the flammability data-completeness check. Flammable
// classification is a physical property determined by flash point and initial
// boiling point, not a composition sum, so the engine never derives a category
// here; it only verifies the decisive datum is present:
//
//   - flash point UNKNOWN → data_missing (block): the decisive datum is absent,
//     so no flammable classification can be confirmed.
//
// When the flash point IS known, no FLAG is produced (the deterministic PG
// cut-offs are resolved by the caller alongside the boiling-point datum).
func CheckFlammable(flashPtKnown bool) []Flag {
	if flashPtKnown {
		return nil
	}
	return []Flag{
		{
			Section:  "9.1",
			Code:     FlagDataMissing,
			Severity: SeverityBlock,
			Message:  "Flash point unknown: flammability cannot be confirmed. Provide the flash point to complete the physical-hazard classification.",
		},
	}
}
