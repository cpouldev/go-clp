package core

import (
	"fmt"
	"strings"
)

// transport.go implements the fragrance-scoped dangerous-goods transport
// classification used by SDS Section 14. It is a PURE, stdlib-only functional
// core: no I/O, deterministic for identical input. The caller supplies the
// deterministically computed flash point, aquatic classification, and product
// type.
//
// The tree is deliberately narrow — it covers only the dangerous-goods
// outcomes a liquid home-fragrance product realistically reaches under ADR/RID
// (road) and IATA DGR (air):
//
//	UN 1197  Class 3  PG III — Extracts, flavouring, liquid (botanical extract / flavouring)
//	UN 1266  Class 3  PG II/III — Perfumery products with flammable solvents (finished fragrance)
//	UN 3082  Class 9  PG III — Environmentally hazardous substance, liquid, n.o.s. (aquatic, non-flammable)
//	(none)   not regulated — low-hazard water-based product
//
// Anything the tree cannot resolve (most importantly an unknown flash point,
// where flammable-vs-not cannot be decided) yields NO guessed classification:
// it returns a not-regulated zero value plus a blocking data_missing FLAG so an
// operator enters Section 14 by hand.
//
// References: SKILL "Transport Classification" table (UN 1197/1266/3082, LQ/EQ,
// env-hazard mark); ADR 2025 UN 1197 / UN 1266 / UN 3082 entries.

// Product-type tokens recognised by Classify. Matching is case-insensitive and
// substring-based, so values such as "extract", "natural extract", "perfumery
// product" or "finished fragrance" all route correctly. The empty string (or
// any unrecognised value) is treated as a finished consumer fragrance and, when
// flammable, resolves to UN 1266 per the "prefer 1266 when in doubt" rule.
const (
	productTypeExtract    = "extract"
	productTypeFlavouring = "flavouring"
	productTypePerfumery  = "perfumery"
	productTypeFinished   = "finished"
)

// alcoholTokens route an alcohol-based liquid (not a finished perfumery product
// or a botanical extract) to UN 1170 — an ethanol / ethanol-solution carrier
// shipped as such, rather than the finished-fragrance UN 1266.
var alcoholTokens = []string{"ethanol", "ethyl alcohol", "alcohol", "isopropanol", "isopropyl", "ipa"}

// Proper shipping names per ADR 2025.
const (
	psnUN1170 = "Ethanol solution"
	psnUN1197 = "Extracts, flavouring, liquid"
	psnUN1266 = "Perfumery products with flammable solvents"
	psnUN3082 = "Environmentally hazardous substance, liquid, n.o.s."
	psnUN3077 = "Environmentally hazardous substance, solid, n.o.s."
)

// Dangerous-goods regimes a regulated product can ship under, per transport mode.
const (
	regimeFullDG   = "full_dg"
	regimeLimited  = "limited_quantity"
	regimeExcepted = "excepted_quantity"
)

// flammableUpperC is the closed-cup flash-point ceiling (°C) for an ADR Class 3
// flammable liquid. At or below this, the product is a flammable liquid; above
// it the product is not regulated for flammability.
const flammableUpperC = 60.0

// pgIICeilingC is the flash-point boundary between packing group II and III for
// a flammable liquid. Below 23 °C → PG II (PG I additionally needs an initial
// boiling point ≤ 35 °C, which is not available to the transport core, so the
// conservative PG II is assigned); from 23 °C up to flammableUpperC → PG III.
const pgIICeilingC = 23.0

// Classify derives the transport classification for a finished fragrance
// mixture from its flash point, aquatic classification, and product type.
//
//   - flashPt      — closed-cup flash point in °C; nil means unknown.
//   - aquaticClass — the aquatic classification string from the classifier
//     (e.g. "Aquatic Acute 1", "Aquatic Chronic 2"); empty means not
//     aquatic-toxic. Only Acute 1 / Chronic 1 / Chronic 2 trigger the
//     transport environmentally-hazardous designation.
//   - productType  — material nature hint ("extract"/"flavouring" → UN 1197;
//     "perfumery"/"finished"/empty → UN 1266).
//
// It returns the populated TransportResult and any FLAGs. The only FLAG this
// core raises is a blocking data_missing when the flash point is unknown and a
// flammability decision therefore cannot be made — a genuine ambiguity that
// must not be guessed.
func Classify(flashPt *float64, aquaticClass string, productType string, innerPackageLitres *float64) (
	TransportResult,
	[]Flag,
) {
	envHazard := isTransportEnvHazard(aquaticClass)

	// Ambiguity gate: without a flash point the flammable-vs-not branch cannot
	// be decided. Refuse to guess — emit a blocking FLAG for manual entry.
	if flashPt == nil {
		return TransportResult{Regulated: false}, []Flag{
			{
				Section:  "14",
				Code:     FlagDataMissing,
				Severity: SeverityBlock,
				Message:  "Flash point unknown: cannot determine whether the product is a flammable liquid (UN 1197/1266 Class 3) or not. Enter Section 14 transport classification manually.",
			},
		}
	}

	fp := *flashPt

	// Branch 1 — flammable liquid (flash point at or below the Class 3 ceiling).
	// Flammability dominates: the product ships as a Class 3 flammable liquid,
	// with an additional environmentally-hazardous mark when it is also
	// aquatic-toxic. SP375 does NOT exempt flammable liquids.
	if fp <= flammableUpperC {
		return flammableResult(fp, productType, envHazard, innerPackageLitres), nil
	}

	// Branch 2 — non-flammable but environmentally hazardous → UN 3082 Class 9,
	// UNLESS ADR special provision SP375 (IATA equivalent) exempts it: an
	// environmentally hazardous substance (UN 3082 / UN 3077) in inner
	// packagings ≤ 5 L (liquids) / ≤ 5 kg (solids) is NOT subject to
	// dangerous-goods regulation. Consumer fragrance ships in small retail
	// bottles, so the exemption is the norm.
	if envHazard {
		if sp375Exempts(innerPackageLitres) {
			return sp375NotRegulatedResult(innerPackageLitres)
		}
		return environmentallyHazardousResult(innerPackageLitres), nil
	}

	// Branch 3 — low-hazard water-based product: not regulated for transport.
	return TransportResult{Regulated: false}, nil
}

// flammableResult builds the Class 3 flammable-liquid outcome, selecting UN 1197
// for botanical extracts/flavourings, UN 1170 for an alcohol/ethanol carrier, and
// UN 1266 for finished perfumery products (the default when the product type is
// unknown). The road and air legs are built independently (buildADREntry /
// buildIATAEntry); the aquatic env mark is carried as an additional mark when
// applicable. SP375 never exempts a flammable liquid.
func flammableResult(flashPt float64, productType string, envHazard bool, innerL *float64) TransportResult {
	unNumber, psn := flammableUNAndName(productType)
	pg := flammablePackingGroup(flashPt)

	// Air is more restrictive than road for Class 3: surface that on the air leg
	// (rendered in §14.6) so the road LQ/EQ regime is never assumed to apply.
	airNote := "Air transport (IATA DGR): flammable liquids carry stricter passenger/cargo-aircraft net-quantity limits than road; confirm the packing instruction and limits against the current edition."

	return TransportResult{
		Regulated:  true,
		ADR:        buildADREntry(unNumber, "3", pg, psn, innerL, ""),
		IATA:       buildIATAEntry(unNumber, "3", pg, psn, innerL, "", airNote),
		EnvMark:    envHazard, // additional fish+tree mark when also aquatic-toxic
		LqEligible: true,
		EqEligible: true,
	}
}

// flammableUNAndName resolves the UN number and proper shipping name for a
// flammable fragrance liquid. Extracts and flavourings (botanical origin, with
// alcohol as carrier) take UN 1197; an alcohol/ethanol carrier that is not a
// finished perfumery product takes UN 1170; finished perfumery products take
// UN 1266. When the product type is unrecognised the finished-fragrance default
// (UN 1266) is used — the "prefer 1266 when in doubt for finished consumer
// fragrance" rule from the SKILL table.
func flammableUNAndName(productType string) (unNumber, properShippingName string) {
	pt := strings.ToLower(strings.TrimSpace(productType))

	if strings.Contains(pt, productTypeExtract) || strings.Contains(pt, productTypeFlavouring) {
		return "1197", psnUN1197
	}

	// An alcohol/ethanol carrier shipped as such → UN 1170, unless it is a finished
	// perfumery product (those stay UN 1266 even though they are alcohol-based).
	if !strings.Contains(pt, productTypePerfumery) && !strings.Contains(pt, productTypeFinished) {
		for _, tok := range alcoholTokens {
			if strings.Contains(pt, tok) {
				return "1170", psnUN1170
			}
		}
	}

	// productTypePerfumery, productTypeFinished, empty, or anything unknown.
	return "1266", psnUN1266
}

// flammablePackingGroup maps a known flash point to its packing group. PG I
// additionally requires an initial boiling point ≤ 35 °C, which the transport
// core does not receive, so a flash point below 23 °C is assigned the
// conservative PG II; 23 °C up to the Class 3 ceiling is PG III.
func flammablePackingGroup(flashPt float64) string {
	if flashPt < pgIICeilingC {
		return "II"
	}
	return "III"
}

// environmentallyHazardousResult builds the UN 3082 Class 9 PG III outcome for a
// non-flammable, aquatic-toxic mixture above the SP375 size limit. The
// environmentally-hazardous mark is always required for UN 3082.
func environmentallyHazardousResult(innerL *float64) TransportResult {
	return TransportResult{
		Regulated:  true,
		ADR:        buildADREntry("3082", "9", "III", psnUN3082, innerL, ""),
		IATA:       buildIATAEntry("3082", "9", "III", psnUN3082, innerL, "", ""),
		EnvMark:    true, // mandatory for UN 3082
		LqEligible: true,
		EqEligible: true,
	}
}

// classifySolidTransport is the §14 decision for a SOLID product — the solid-wax
// path (candles, wax melts, snap bars). A finished solid wax is NOT a flammable
// liquid, so the flash-point / Class 3 branch never runs and no flash point is
// consulted (that is the whole point of routing solids here rather than through
// Classify). The only dangerous-goods outcome a solid home-fragrance product
// realistically reaches is environmentally hazardous SOLID (UN 3077, Class 9),
// and even that is exempt under ADR SP375 / IATA A197 when the inner packaging
// is ≤ 5 kg — which consumer wax products always are. Otherwise a finished solid
// wax is an article, not bulk dangerous goods, and is not regulated for
// transport.
//
// The inner-package value is reused from the liquid path's litre field; for a
// solid it is interpreted as kilograms (SP375 reads ≤ 5 L for liquids / ≤ 5 kg
// for solids — the same numeric threshold), and the LQ/EQ limits on the built
// ADR/IATA legs are likewise kilogram/gram magnitudes for UN 3077.
func classifySolidTransport(aquaticClass string, innerPackage *float64) (TransportResult, []Flag) {
	if !isTransportEnvHazard(aquaticClass) {
		return TransportResult{Regulated: false}, nil
	}
	if sp375Exempts(innerPackage) {
		return sp375NotRegulatedSolidResult(innerPackage)
	}
	return environmentallyHazardousSolidResult(innerPackage), nil
}

// environmentallyHazardousSolidResult is the UN 3077 (Class 9, PG III) leg for a
// non-exempt environmentally hazardous solid — the solid analogue of
// environmentallyHazardousResult's UN 3082.
func environmentallyHazardousSolidResult(innerKg *float64) TransportResult {
	return TransportResult{
		Regulated:  true,
		ADR:        buildADREntry("3077", "9", "III", psnUN3077, innerKg, ""),
		IATA:       buildIATAEntry("3077", "9", "III", psnUN3077, innerKg, "", ""),
		EnvMark:    true, // mandatory for UN 3077
		LqEligible: true,
		EqEligible: true,
	}
}

// sp375NotRegulatedSolidResult is the SP375 / A197 not-regulated outcome for a
// solid (≤ 5 kg) — the solid analogue of sp375NotRegulatedResult's ≤ 5 L liquid
// wording.
func sp375NotRegulatedSolidResult(innerKg *float64) (TransportResult, []Flag) {
	res := TransportResult{
		Regulated:            false,
		EnvMark:              false, // SP375 exemption removes the transport marking too
		Sp375Applied:         true,
		NotRegulatedReason:   "Not a flammable solid — not classified as ADR/IATA Class 4.1. Environmentally hazardous (UN 3077, Class 9) but NOT regulated for transport — exempt under ADR special provision SP375 / IATA special provision A197: an environmentally hazardous substance in inner packaging ≤ 5 kg is not subject to dangerous-goods regulation.",
		NotRegulatedReasonEL: "Μη εύφλεκτο στερεό — δεν ταξινομείται ως ADR/IATA Κλάση 4.1. Επικίνδυνο για το περιβάλλον (UN 3077, Κλάση 9) αλλά ΜΗ ρυθμιζόμενο για μεταφορά — εξαιρείται βάσει της ειδικής διάταξης ADR SP375 / IATA A197: ουσία επικίνδυνη για το περιβάλλον σε εσωτερική συσκευασία ≤ 5 kg δεν υπόκειται σε ρύθμιση επικίνδυνων εμπορευμάτων.",
	}
	if innerKg != nil {
		return res, nil // size known and ≤ 5 kg: certain, no review flag
	}
	return res, []Flag{
		{
			Section:  "14",
			Code:     FlagDataMissing,
			Severity: SeverityWarn,
			Message:  "Section 14: SP375 applied — the solid product is treated as NOT regulated for transport assuming the inner (retail) packaging is ≤ 5 kg, which holds for consumer wax products. Confirm the package size; inner packaging > 5 kg ships as UN 3077, Class 9, PG III, with the environmentally-hazardous mark.",
		},
	}
}

// dgLimitsFor returns the ADR limited-quantity (litres) and excepted-quantity
// limits for a Class 3 / Class 9 fragrance entry by packing group: PG II → LQ 1 L,
// EQ E2 (30 ml inner / 500 ml outer); PG III → LQ 5 L, EQ E1 (30 ml inner /
// 1000 ml outer). The EQ values are harmonised with the UN Model Regulations, so
// they hold for air (IATA) as well; the builders apply the LQ value per mode.
func dgLimitsFor(pg string) (lqLitres, eqInnerMl, eqOuterMl float64, eqCode string) {
	if pg == "II" {
		return 1.0, 30, 500, "E2"
	}
	return 5.0, 30, 1000, "E1"
}

// dgRegime resolves which regime applies for an inner-packaging size against the
// LQ (litres) and EQ (inner ml) limits: unknown size → full_dg (conservative, so
// the operator must confirm a smaller package to claim LQ/EQ relief); ≤ EQ inner →
// excepted_quantity; ≤ LQ → limited_quantity; otherwise full_dg.
func dgRegime(innerL *float64, lqLitres, eqInnerMl float64) string {
	if innerL == nil {
		return regimeFullDG
	}
	innerMl := *innerL * 1000
	switch {
	case innerMl <= eqInnerMl:
		return regimeExcepted
	case *innerL <= lqLitres:
		return regimeLimited
	default:
		return regimeFullDG
	}
}

// buildADREntry assembles the road (ADR) leg: classification plus the PG-specific
// LQ/EQ limits and the regime that applies for the given inner-packaging size.
func buildADREntry(un, class, pg, psn string, innerL *float64, sp string) *ADREntry {
	lq, eqIn, eqOut, eqCode := dgLimitsFor(pg)
	return &ADREntry{
		UNNumber:                un,
		Class:                   class,
		PackingGroup:            pg,
		ProperShippingName:      psn,
		LimitedQuantityLitres:   lq,
		Regime:                  dgRegime(innerL, lq, eqIn),
		ExceptedQuantityCode:    eqCode,
		ExceptedQuantityInnerMl: eqIn,
		ExceptedQuantityOuterMl: eqOut,
		SpecialProvision:        sp,
	}
}

// buildIATAEntry assembles the air (IATA) leg independently of the road leg. The
// classification (UN/class/PG/PSN) is the same and the EQ limits are harmonised,
// but the regime is computed for air and any air-specific caveat is carried in
// notes so the road result is never assumed to apply to air.
func buildIATAEntry(un, class, pg, psn string, innerL *float64, sp, notes string) *IATAEntry {
	lq, eqIn, eqOut, eqCode := dgLimitsFor(pg)

	// The excepted-quantity codes and volumes come from the UN Model Regulations
	// and are genuinely harmonised across modes. The LIMITED-quantity threshold is
	// not: the IATA DGR sets its own per-UN air limits, which are typically far
	// smaller than the ADR road limit, and this engine does not carry that table.
	// The air leg therefore reports the road threshold and says so, rather than
	// presenting an unverified air figure as authoritative.
	airCaveat := "Air (IATA) limited-quantity threshold and regime are shown from the ADR road table; " +
		"confirm against the current IATA DGR entry for this UN number before shipping by air."
	if notes != "" {
		airCaveat = notes + " " + airCaveat
	}

	return &IATAEntry{
		UNNumber:                un,
		Class:                   class,
		PackingGroup:            pg,
		ProperShippingName:      psn,
		LimitedQuantityLitres:   lq,
		Regime:                  dgRegime(innerL, lq, eqIn),
		ExceptedQuantityCode:    eqCode,
		ExceptedQuantityInnerMl: eqIn,
		ExceptedQuantityOuterMl: eqOut,
		SpecialProvision:        sp,
		Notes:                   airCaveat,
	}
}

// sp375MaxLitres is the ADR special-provision SP375 inner-packaging ceiling for
// LIQUIDS (≤ 5 L); the equivalent solid limit is 5 kg. At or below it an
// environmentally hazardous substance (UN 3082 / UN 3077) is not subject to
// dangerous-goods regulation; above it it ships as full UN 3082 Class 9 PG III.
const sp375MaxLitres = 5.0

// sp375Exempts reports whether SP375 exempts an environmentally hazardous
// substance from transport regulation. Per the operator decision, an UNKNOWN
// inner-packaging size is treated as exempt — consumer fragrance ships in small
// retail bottles ≤ 5 L — and the caller raises a confirm-packaging review flag
// so the assumption is visible.
func sp375Exempts(innerPackageLitres *float64) bool {
	if innerPackageLitres == nil {
		return true
	}
	return *innerPackageLitres <= sp375MaxLitres
}

// sp375NotRegulatedResult builds the not-regulated outcome for an SP375-exempt
// environmentally hazardous substance. When the inner-packaging size is unknown
// it attaches a warn review flag asking the operator to confirm it is ≤ 5 L;
// when the size is known and within the limit the call is certain and no flag is
// raised.
func sp375NotRegulatedResult(innerPackageLitres *float64) (TransportResult, []Flag) {
	res := TransportResult{
		Regulated:            false,
		EnvMark:              false, // SP375 exemption removes the transport marking too
		Sp375Applied:         true,
		NotRegulatedReason:   "Not a flammable liquid — not classified as ADR/IATA Class 3. Environmentally hazardous (UN 3082, Class 9) but NOT regulated for transport — exempt under ADR special provision SP375 / IATA special provision A197: an environmentally hazardous substance in inner packaging ≤ 5 L is not subject to dangerous-goods regulation.",
		NotRegulatedReasonEL: "Μη εύφλεκτο υγρό — δεν ταξινομείται ως ADR/IATA Κλάση 3. Επικίνδυνο για το περιβάλλον (UN 3082, Κλάση 9) αλλά ΜΗ ρυθμιζόμενο για μεταφορά — εξαιρείται βάσει της ειδικής διάταξης ADR SP375 / IATA A197: ουσία επικίνδυνη για το περιβάλλον σε εσωτερική συσκευασία ≤ 5 L δεν υπόκειται σε ρύθμιση επικίνδυνων εμπορευμάτων.",
	}
	if innerPackageLitres != nil {
		return res, nil // size known and ≤ 5 L: certain, no review flag
	}
	return res, []Flag{
		{
			Section:  "14",
			Code:     FlagDataMissing,
			Severity: SeverityWarn,
			Message:  "Section 14: SP375 applied — the product is treated as NOT regulated for transport assuming the inner (retail) packaging is ≤ 5 L, which holds for consumer fragrance. Confirm the package size; inner packaging > 5 L ships as UN 3082, Class 9, PG III, with the environmentally-hazardous mark.",
		},
	}
}

// isTransportEnvHazard reports whether an aquatic classification string meets
// the transport environmentally-hazardous criteria. Only Aquatic Acute 1,
// Chronic 1, and Chronic 2 trigger the transport mark (and UN 3082); Chronic 3
// and Chronic 4 do not. Matching is case-insensitive and tolerant of the exact
// label formatting the classifier emits.
func isTransportEnvHazard(aquaticClass string) bool {
	c := strings.ToLower(strings.TrimSpace(aquaticClass))
	if c == "" {
		return false
	}

	if strings.Contains(c, "acute 1") {
		return true
	}
	if strings.Contains(c, "chronic 1") || strings.Contains(c, "chronic 2") {
		return true
	}
	return false
}

// transportSummary renders a short human-readable description of a transport
// result, used in FLAG messages and log lines. It is exported-adjacent helper
// kept unexported; the dash renders the structured fields directly.
func transportSummary(r TransportResult) string {
	if !r.Regulated || r.ADR == nil {
		return "Not regulated for transport"
	}
	return fmt.Sprintf("UN %s, Class %s, PG %s", r.ADR.UNNumber, r.ADR.Class, r.ADR.PackingGroup)
}
