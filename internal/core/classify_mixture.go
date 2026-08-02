package core

// classify_mixture.go is the deterministic CLP mixture-classification engine.
// It is the single source of truth for the finished mixture's hazard
// classification and GHS/CLP label — the LLM no longer authors the label.
//
// Given the recipe components (each at its authoritative % w/w derived from the
// recipe quantities, carrying the CLP hazard categories extracted from its
// supplier SDS), ClassifyMixture applies the CLP 1272/2008 Annex I generic
// concentration limits, additivity rules, and summation methods to every
// common hazard class, then derives the label (pictograms, signal word, H/EUH
// codes, P codes) from the triggered classes.
//
// Scope: the classes a home-fragrance mixture realistically triggers — acute
// toxicity (ATE summation), skin corrosion/irritation and eye damage/irritation
// (additive), skin + respiratory sensitisation, CMR, STOT SE/RE, aspiration,
// the aquatic environment (summation, reused from classifier.go), and flammable
// liquids (flash-point gated). Where a decisive datum is missing (kinematic
// viscosity for aspiration, boiling point to split Flam. Liq. 1 vs 2) the engine
// classifies conservatively and raises a FLAG rather than silently dropping the
// hazard.
//
// All numeric boundaries are legally load-bearing CLP thresholds: every
// comparison uses ">=" at the boundary value, matching classifier.go.

import (
	"fmt"
	"sort"
	"strings"
)

// ── Generic concentration limits (CLP Annex I), % w/w ─────────────────────────

const (
	// Skin corrosion / irritation (Table 3.2.3, additive).
	gclSkinCorr1     = 5.0  // Σ Skin Corr 1 ≥ 5%  → Skin Corr. 1 (H314)
	gclSkinIrrit2    = 10.0 // 10·ΣCorr1 + ΣIrrit2 ≥ 10% → Skin Irrit. 2 (H315)
	corrToIrritBoost = 10.0 // a corrosive also counts ×10 toward irritation

	// Eye damage / irritation (Table 3.3.3, additive; corrosives count as Eye Dam 1).
	gclEyeDam1  = 3.0  // Σ Eye Dam 1 (+ Skin Corr 1) ≥ 3%  → Eye Dam. 1 (H318)
	gclEyeIrrit = 10.0 // 10·ΣEyeDam1 + ΣEyeIrrit2 ≥ 10% → Eye Irrit. 2 (H319)

	// Respiratory sensitisation (Table 3.4.x, per-component GCL).
	gclRespSens1b = 1.0 // Resp. Sens. 1 / 1B ≥ 1%   → H334
	gclRespSens1a = 0.1 // Resp. Sens. 1A    ≥ 0.1% → H334

	// Carcinogenicity / mutagenicity / reproductive toxicity (per-component GCL).
	gclCarc1 = 0.1 // Carc. 1A/1B ≥ 0.1% → H350
	gclCarc2 = 1.0 // Carc. 2     ≥ 1.0% → H351
	gclMuta1 = 0.1 // Muta. 1A/1B ≥ 0.1% → H340
	gclMuta2 = 1.0 // Muta. 2     ≥ 1.0% → H341
	gclRepr1 = 0.3 // Repr. 1A/1B ≥ 0.3% → H360
	gclRepr2 = 3.0 // Repr. 2     ≥ 3.0% → H361
	gclLact  = 0.3 // Lactation  ≥ 0.3% → H362

	// STOT single / repeated exposure (Table 3.8.3 / 3.9.4).
	gclStot1Hi = 10.0 // Cat 1 ≥ 10%        → STOT … 1 (H370 / H372)
	gclStot1Lo = 1.0  // Cat 1 1–<10%       → STOT … 2 (H371 / H373)
	gclStot2   = 10.0 // Cat 2 ≥ 10%        → STOT … 2 (H371 / H373)
	gclStotSe3 = 20.0 // Cat 3 ≥ 20%        → STOT SE 3 (H335 / H336)

	// Aspiration (Table 3.10.x): Σ Asp. Tox. 1 ≥ 10% AND kinematic viscosity
	// ≤ 20.5 mm²/s at 40 °C. The viscosity gate is enforced by the caller.
	gclAsp1 = 10.0

	// Flammable liquids (Annex I §2.6) — flash-point cut-offs in °C.
	flashCat3Max = 60.0 // 23 ≤ FP ≤ 60 → Flam. Liq. 3 (H226)
	flashCat12   = 23.0 // FP < 23      → Flam. Liq. 1 or 2 (boiling point splits)
)

// ── Public result ─────────────────────────────────────────────────────────────

// MixtureClassification is the deterministic outcome for the finished mixture:
// the GHS/CLP label, the list of triggered mixture hazards, and any FLAGs the
// engine raised (e.g. a conservative aspiration call pending viscosity).
type MixtureClassification struct {
	Label   SdsLabel
	Hazards []MixtureHazard
	Flags   []Flag
}

// ── Engine ──────────────────────────────────────────────────────────────────

// classOutcome is one triggered hazard class with everything the label needs.
type classOutcome struct {
	display   string   // human class label, e.g. "Skin Irrit. 2"
	category  string   // CLP category, e.g. "2"
	hCodes    []string // H-codes this class contributes
	pictogram string   // GHS pictogram code, or "" (e.g. H362, Chronic 2/3/4)
	danger    bool     // true → signal word "Danger"
	noSignal  bool     // true → contributes NO signal word AND no pictogram (Aquatic Chronic 2/3/4, Lact.)
	pCodes    []string // precautionary statements this class recommends
}

// ClassifyMixture is the engine entry point. components are the recipe units at
// their authoritative % w/w (material- or substance-level); flashPt is the
// finished-mixture flash point in °C (nil = unknown); viscosityKnownLow is true
// only when the kinematic viscosity is known to be ≤ 20.5 mm²/s (the aspiration
// gate). It returns the derived label + the triggered mixture hazards + FLAGs.
func ClassifyMixture(components []ClassComponent, flashPt *float64, viscosityKnownLow bool) MixtureClassification {
	var outcomes []classOutcome

	// Reuse the legally-tested cores for the two classes classifier.go owns.
	if skin, _ := ClassifySkinSens(components); skin != "" {
		outcomes = append(
			outcomes, classOutcome{
				display: "Skin Sens. 1", category: "1", hCodes: []string{"H317"},
				pictogram: "GHS07", danger: false,
				pCodes: []string{"P261", "P272", "P280", "P302+P352", "P333+P313", "P362+P364", "P501"},
			},
		)
	}
	if aq, _ := ClassifyAquatic(components); aq != "" {
		outcomes = append(outcomes, aquaticOutcome(aq))
	}

	// Every other class is evaluated here, off the per-component hazard list.
	outcomes = append(outcomes, classifyAcuteTox(components)...)
	outcomes = appendIf(outcomes, classifySkinCorrIrrit(components))
	outcomes = appendIf(outcomes, classifyEye(components))
	outcomes = appendIf(outcomes, classifyRespSens(components))
	outcomes = append(outcomes, classifyCMR(components)...)
	outcomes = append(outcomes, classifyStotSE(components)...)
	outcomes = appendIf(outcomes, classifyStotRE(components))

	aspOutcome, aspFlags := classifyAspiration(components, viscosityKnownLow)
	outcomes = appendIf(outcomes, aspOutcome)

	flamOutcome, flamFlags := classifyFlammable(flashPt)
	outcomes = appendIf(outcomes, flamOutcome)

	label := assembleLabel(components, outcomes)
	flags := append(aspFlags, flamFlags...)
	flags = append(flags, invalidHazardClassFlags(components)...)

	// A hazard whose category the engine cannot apply contributes to nothing, so
	// it is reported HERE rather than in the engine's review pass: ClassifyMixture
	// is exported, and a direct caller must not receive a classification that
	// silently ignored one of the hazards it supplied.
	flags = append(flags, validateEndpointsEvaluated(components)...)

	return MixtureClassification{
		Label:   label,
		Hazards: outcomesToHazards(outcomes),
		Flags:   flags,
	}
}

func invalidHazardClassFlags(components []ClassComponent) []Flag {
	seen := make(map[string]bool)
	var flags []Flag
	for _, component := range components {
		for _, hazard := range component.Hazards {
			if _, err := ParseHazardClass(hazard.Class); err == nil {
				continue
			}
			key := component.Name + "\x00" + hazard.Class
			if seen[key] {
				continue
			}
			seen[key] = true
			flags = append(
				flags, Flag{
					Section:  "2.1",
					Code:     FlagInvalidHazardClass,
					Severity: SeverityBlock,
					Message: fmt.Sprintf(
						"Component %q has unrecognised hazard class %q; correct the input before relying on this classification.",
						nameOrUnknown(component.Name),
						hazard.Class,
					),
				},
			)
		}
	}
	return flags
}

// appendIf appends a non-nil outcome.
func appendIf(dst []classOutcome, o *classOutcome) []classOutcome {
	if o != nil {
		return append(dst, *o)
	}
	return dst
}

// ── Per-class accumulation helpers ────────────────────────────────────────────

// hazardKind is the canonical class family used to bucket the model's free-form
// class strings.
type hazardKind int

const (
	kindOther hazardKind = iota
	kindAcuteOral
	kindAcuteDermal
	kindAcuteInhal
	kindSkinCorr
	kindSkinIrrit
	kindEyeDam
	kindEyeIrrit
	kindRespSens
	kindCarc
	kindMuta
	kindRepr
	kindLactation
	kindStotSE
	kindStotRE
	kindAsp
)

// classKind resolves validated public hazard classes to internal classifier
// buckets. Skin sensitisation and aquatic classes are handled by classifier.go
// and therefore remain kindOther here.
func classKind(class string) hazardKind {
	parsed, err := ParseHazardClass(class)
	if err != nil {
		return kindOther
	}
	switch parsed {
	case HazardClassAspirationHazard:
		return kindAsp
	case HazardClassSkinCorrosion:
		return kindSkinCorr
	case HazardClassSkinIrritation:
		return kindSkinIrrit
	case HazardClassSeriousEyeDamage:
		return kindEyeDam
	case HazardClassEyeIrritation:
		return kindEyeIrrit
	case HazardClassRespiratorySensitisation:
		return kindRespSens
	case HazardClassCarcinogenicity:
		return kindCarc
	case HazardClassGermCellMutagenicity:
		return kindMuta
	case HazardClassLactation:
		return kindLactation
	case HazardClassReproductiveToxicity:
		return kindRepr
	case HazardClassSTOTSingleExposure:
		return kindStotSE
	case HazardClassSTOTRepeatedExposure:
		return kindStotRE
	case HazardClassAcuteToxicityDermal:
		return kindAcuteDermal
	case HazardClassAcuteToxicityInhalation:
		return kindAcuteInhal
	case HazardClassAcuteToxicityOral:
		return kindAcuteOral
	default:
		return kindOther
	}
}

// hazardEndpoint resolves a hazard to its endpoint kind, INCLUDING the three
// endpoints classifier.go owns — skin sensitisation and the two aquatic classes
// — which classKind deliberately reports as kindOther. It exists so an SCL can
// be matched to the endpoint it was set for; use classKind directly when you
// want only the classes classify_mixture.go dispatches on.
func hazardEndpoint(h ComponentHazard) hazardKind {
	if k := classKind(h.Class); k != kindOther {
		return k
	}
	parsed, err := ParseHazardClass(h.Class)
	if err != nil {
		return kindOther
	}
	switch parsed {
	case HazardClassSkinSensitisation:
		return kindSkinSens
	case HazardClassAquaticAcute:
		return kindAquaticAcute
	case HazardClassAquatic, HazardClassAquaticChronic:
		return kindAquaticChronic
	}
	return kindOther
}

// sumByKind sums the component concentrations whose hazard list includes the
// given kind (a component contributes its full concentration once per kind).
func sumByKind(components []ClassComponent, kind hazardKind) float64 {
	return sumByAnyKind(components, kind)
}

// componentHasKind reports whether a component carries any hazard of the kind.
func componentHasKind(c ClassComponent, kind hazardKind) bool {
	for _, h := range c.Hazards {
		if classKind(h.Class) == kind {
			return true
		}
	}
	return false
}

// gclEpsilon absorbs the float64 accumulation error of summing component
// percentages: 0.01 + 4.02 + 0.97 evaluates to 4.999999999999999, which must
// still reach the 5.00 % boundary of Annex I Table 3.2.3.
const gclEpsilon = 1e-9

// reachesLimit reports whether a summation total meets a CLP concentration
// limit. Every threshold comparison goes through it, so a sum landing exactly
// on a boundary always classifies.
func reachesLimit(sum, limit float64) bool {
	return sum >= limit-gclEpsilon
}

// sumByAnyKind totals the concentration of every component carrying at least
// one of kinds.
//
// A component counts ONCE however many of its hazard entries match: the CAS
// roll-up in buildClassComponents concatenates the hazard lists of one
// substance found in several materials, so repeated entries are routine. Where
// several entries match, the strictest concentration limit wins.
func sumByAnyKind(components []ClassComponent, kinds ...hazardKind) float64 {
	var sum float64
	for _, c := range components {
		if w, ok := componentWeight(c, kinds...); ok {
			sum += c.ConcentrationPct * w
		}
	}
	return sum
}

// sumByKindCategory totals the components carrying kind in one category,
// counting each component once and honouring its concentration limit.
func sumByKindCategory(components []ClassComponent, kind hazardKind, category string) float64 {
	want := normHazardCategory(category)
	var sum float64
	for _, c := range components {
		weight, found := 0.0, false
		for _, h := range c.Hazards {
			if classKind(h.Class) != kind || normHazardCategory(h.Category) != want {
				continue
			}
			if w := hazardWeight(h, kind); !found || w > weight {
				weight, found = w, true
			}
		}
		if found {
			sum += c.ConcentrationPct * weight
		}
	}
	return sum
}

// componentWeight returns the multiplier c's concentration carries into a
// summation for any of kinds, and whether c carries one at all.
func componentWeight(c ClassComponent, kinds ...hazardKind) (float64, bool) {
	weight, found := 0.0, false
	for _, h := range c.Hazards {
		for _, kind := range kinds {
			if classKind(h.Class) != kind {
				continue
			}
			if w := hazardWeight(h, kind); !found || w > weight {
				weight, found = w, true
			}
			break
		}
	}
	return weight, found
}

// hazardWeight scales one hazard entry's contribution to a summation.
//
// It is 1 for a component classified at the generic limit. Where a harmonised
// specific concentration limit applies, CLP Annex I (3.2.3.3.4, 3.3.3.3.4)
// replaces the generic comparison with Σ Ci/SCLi ≥ 1; scaling the component by
// GCL/SCL states the same test in the units the summation tables already use,
// so the corrosive-to-irritant boost and the near-threshold advisories keep
// working unchanged.
func hazardWeight(h ComponentHazard, kind hazardKind) float64 {
	gcl, gclOK := gclFor(kind, h.Category)
	limit, limitOK := effectiveComponentLimit(h, kind)
	if gclOK && limitOK && gcl > 0 && limit > 0 {
		return gcl / limit
	}
	return 1
}

// ── Acute toxicity (ATE summation, Table 3.1.x) ───────────────────────────────

// acuteATE returns the converted acute-toxicity estimate (ATE point estimate,
// Table 3.1.2) for a route+category, and whether the category is recognised.
func acuteATE(kind hazardKind, category string) (float64, bool) {
	cat := normHazardCategory(category)
	switch kind {
	case kindAcuteOral:
		switch cat {
		case "1":
			return 0.5, true
		case "2":
			return 5, true
		case "3":
			return 100, true
		case "4":
			return 500, true
		}
	case kindAcuteDermal:
		switch cat {
		case "1":
			return 5, true
		case "2":
			return 50, true
		case "3":
			return 300, true
		case "4":
			return 1100, true
		}
	case kindAcuteInhal:
		// Vapour ATE (mg/l/4h), Table 3.1.2.
		switch cat {
		case "1":
			return 0.05, true
		case "2":
			return 0.5, true
		case "3":
			return 3, true
		case "4":
			return 11, true
		}
	}
	return 0, false
}

// classifyAcuteTox applies the ATE summation method per route and returns the
// most severe route's outcome (oral is the dominant route for fragrances). The
// summation is 100 / Σ(Ci/ATEi); the resulting ATEmix maps to a category by the
// route cut-offs.
func classifyAcuteTox(components []ClassComponent) []classOutcome {
	routes := []struct {
		kind   hazardKind
		hByCat map[string]string
	}{
		{kindAcuteOral, map[string]string{"1": "H300", "2": "H300", "3": "H301", "4": "H302"}},
		{kindAcuteDermal, map[string]string{"1": "H310", "2": "H310", "3": "H311", "4": "H312"}},
		{kindAcuteInhal, map[string]string{"1": "H330", "2": "H330", "3": "H331", "4": "H332"}},
	}

	// Acute toxicity is classified per route (Annex I 3.1.3.6): a mixture that is
	// Acute Tox. 4 orally AND Acute Tox. 2 by inhalation carries both H302 and
	// H330, so every route that reaches a category is reported.
	var outcomes []classOutcome
	for _, r := range routes {
		var inv float64
		for _, c := range components {
			ate, ok := componentATE(c, r.kind)
			if !ok {
				continue
			}
			inv += c.ConcentrationPct / ate
		}
		if inv <= 0 {
			continue
		}
		cat := acuteCategoryFromATE(r.kind, 100.0/inv)
		if cat == "" {
			continue
		}
		outcomes = append(
			outcomes, classOutcome{
				display:   "Acute Tox. " + cat,
				category:  cat,
				hCodes:    []string{r.hByCat[cat]},
				pictogram: acutePictogram(cat),
				danger:    cat != "4",
				pCodes:    acutePCodes(cat),
			},
		)
	}
	return outcomes
}

// componentATE returns the strictest acute-toxicity estimate a component
// carries for one route. The component contributes to the ATEmix sum ONCE
// however many entries express that route — the CAS roll-up readily produces
// duplicates, and Annex I 3.1.3.6.1 sums each component's concentration once.
func componentATE(c ClassComponent, kind hazardKind) (float64, bool) {
	best, found := 0.0, false
	for _, h := range c.Hazards {
		if classKind(h.Class) != kind {
			continue
		}
		if ate, ok := acuteATE(kind, h.Category); ok && ate > 0 && (!found || ate < best) {
			best, found = ate, true
		}
	}
	return best, found
}

// acuteCategoryFromATE maps an ATEmix to an acute-toxicity category using the
// route's OWN CLP Table 3.1.1 cut-off values. The oral scale must NOT be reused
// for dermal or inhalation — their boundaries differ, and reusing oral both
// over-states inhalation (a Cat-4 vapour reads as Cat-2 "fatal") and under-states
// dermal. The inhalation row uses the VAPOUR cut-offs (0.5 / 2 / 10 / 20
// mg/l/4h), matching the vapour ATE point estimates acuteATE assigns; a fragrance
// mixture off-gasses vapour, so that is the applicable physical form. Category 1
// shares its H-statement, GHS06 pictogram and Danger signal word with Category 2
// on every route, so folding the ≤ cat1 boundary into the cat2 label is exact. An
// ATEmix above the Category 4 ceiling returns "" (not classified for acute tox).
func acuteCategoryFromATE(kind hazardKind, atemix float64) string {
	var cat1, cat2, cat3, cat4 float64
	switch kind {
	case kindAcuteOral:
		cat1, cat2, cat3, cat4 = 5, 50, 300, 2000 // mg/kg bw
	case kindAcuteDermal:
		cat1, cat2, cat3, cat4 = 50, 200, 1000, 2000 // mg/kg bw
	case kindAcuteInhal:
		cat1, cat2, cat3, cat4 = 0.5, 2, 10, 20 // mg/l/4h (vapours)
	default:
		return ""
	}
	switch {
	case atemix <= cat1:
		return "1"
	case atemix <= cat2:
		return "2"
	case atemix <= cat3:
		return "3"
	case atemix <= cat4:
		return "4"
	default:
		return ""
	}
}

func acutePictogram(cat string) string {
	if cat == "4" {
		return "GHS07"
	}
	return "GHS06"
}

func acutePCodes(cat string) []string {
	if cat == "4" {
		return []string{"P264", "P270", "P301+P312", "P330", "P501"}
	}
	return []string{"P264", "P270", "P301+P310", "P330", "P405", "P501"}
}

// ── Skin corrosion / irritation (additive) ────────────────────────────────────

func classifySkinCorrIrrit(components []ClassComponent) *classOutcome {
	sumCorr := sumByKind(components, kindSkinCorr)
	sumIrrit := sumByKind(components, kindSkinIrrit)

	if reachesLimit(sumCorr, mustGCL(kindSkinCorr, "1")) {
		return &classOutcome{
			display: "Skin Corr. 1", category: "1", hCodes: []string{"H314"},
			pictogram: "GHS05", danger: true,
			pCodes: []string{"P260", "P280", "P301+P330+P331", "P303+P361+P353", "P305+P351+P338", "P310"},
		}
	}
	if reachesLimit(corrToIrritBoost*sumCorr+sumIrrit, mustGCL(kindSkinIrrit, "2")) {
		return &classOutcome{
			display: "Skin Irrit. 2", category: "2", hCodes: []string{"H315"},
			pictogram: "GHS07", danger: false,
			pCodes: []string{"P264", "P280", "P302+P352", "P332+P313", "P362+P364"},
		}
	}
	return nil
}

// ── Eye damage / irritation (additive; corrosives count as eye damage) ─────────

func classifyEye(components []ClassComponent) *classOutcome {
	// Table 3.3.3 pools "Eye Dam. 1 or Skin Corr. 1" — a component carrying both,
	// the usual Annex VI pairing for a caustic, counts once and not twice.
	sumEyeDam := sumByAnyKind(components, kindEyeDam, kindSkinCorr)
	sumEyeIrrit := sumByKind(components, kindEyeIrrit)

	if reachesLimit(sumEyeDam, mustGCL(kindEyeDam, "1")) {
		return &classOutcome{
			display: "Eye Dam. 1", category: "1", hCodes: []string{"H318"},
			pictogram: "GHS05", danger: true,
			pCodes: []string{"P280", "P305+P351+P338", "P310"},
		}
	}
	if reachesLimit(corrToIrritBoost*sumEyeDam+sumEyeIrrit, mustGCL(kindEyeIrrit, "2")) {
		return &classOutcome{
			display: "Eye Irrit. 2", category: "2", hCodes: []string{"H319"},
			pictogram: "GHS07", danger: false,
			pCodes: []string{"P264", "P280", "P305+P351+P338", "P337+P313"},
		}
	}
	return nil
}

// ── Respiratory sensitisation (per-component GCL) ─────────────────────────────

func classifyRespSens(components []ClassComponent) *classOutcome {
	for _, c := range components {
		for _, h := range c.Hazards {
			if classKind(h.Class) != kindRespSens {
				continue
			}
			limit, ok := effectiveComponentLimit(h, kindRespSens)
			if ok && reachesLimit(c.ConcentrationPct, limit) {
				return &classOutcome{
					display: "Resp. Sens. 1", category: "1", hCodes: []string{"H334"},
					pictogram: "GHS08", danger: true,
					pCodes: []string{"P261", "P284", "P304+P340", "P342+P311"},
				}
			}
		}
	}
	return nil
}

// ── CMR (per-component GCL) ────────────────────────────────────────────────────

// classifyCMR returns the carcinogenicity, mutagenicity, and reproductive
// outcomes (each is an independent class, so a mixture can carry more than one).
func classifyCMR(components []ClassComponent) []classOutcome {
	var out []classOutcome

	if o := cmrClass(
		components, kindCarc, cmrSpec{
			hiCat: "1", hiCode: "H350", loCat: "2", loCode: "H351", display: "Carc.",
		},
	); o != nil {
		out = append(out, *o)
	}
	if o := cmrClass(
		components, kindMuta, cmrSpec{
			hiCat: "1", hiCode: "H340", loCat: "2", loCode: "H341", display: "Muta.",
		},
	); o != nil {
		out = append(out, *o)
	}
	if o := cmrClass(
		components, kindRepr, cmrSpec{
			hiCat: "1", hiCode: "H360", loCat: "2", loCode: "H361", display: "Repr.",
		},
	); o != nil {
		out = append(out, *o)
	}
	// Effects on / via lactation (H362) — no pictogram, no signal word.
	if maxConcentrationByKind(components, kindLactation) >= mustGCL(kindLactation, "*") {
		out = append(
			out, classOutcome{
				display: "Lact.", category: "", hCodes: []string{"H362"},
				pictogram: "", danger: false, noSignal: true,
				pCodes: []string{"P201", "P260", "P263", "P308+P313"},
			},
		)
	}
	return out
}

type cmrSpec struct {
	hiCat   string
	hiCode  string
	loCat   string
	loCode  string
	display string
}

// cmrClass evaluates one CMR family PER COMPONENT (never summed): a Cat-1A/1B
// component at/above its effective limit classifies the mixture Category 1
// (Danger); otherwise a Cat-2 component at/above its effective limit classifies
// Category 2 (Warning). The limit is resolved per component by
// effectiveComponentLimit, so a substance-specific concentration limit overrides
// the generic limit for that component alone (CLP Annex I §1.2.1).
func cmrClass(components []ClassComponent, kind hazardKind, s cmrSpec) *classOutcome {
	cat1Hit, cat2Hit := false, false
	for _, c := range components {
		for _, h := range c.Hazards {
			if classKind(h.Class) != kind {
				continue
			}
			limit, ok := effectiveComponentLimit(h, kind)
			if !ok || !reachesLimit(c.ConcentrationPct, limit) {
				continue
			}
			switch normHazardCategory(h.Category) {
			case "1", "1A", "1B":
				cat1Hit = true
			case "2":
				cat2Hit = true
			}
		}
	}
	if cat1Hit {
		return &classOutcome{
			display: s.display + " " + s.hiCat, category: s.hiCat, hCodes: []string{s.hiCode},
			pictogram: "GHS08", danger: true,
			pCodes: []string{"P201", "P202", "P280", "P308+P313", "P405", "P501"},
		}
	}
	if cat2Hit {
		return &classOutcome{
			display: s.display + " " + s.loCat, category: s.loCat, hCodes: []string{s.loCode},
			pictogram: "GHS08", danger: false,
			pCodes: []string{"P201", "P202", "P280", "P308+P313", "P405", "P501"},
		}
	}
	return nil
}

// maxConcentrationByKind returns the highest concentration of any component
// carrying the kind (used for the category-independent lactation GCL).
func maxConcentrationByKind(components []ClassComponent, kind hazardKind) float64 {
	var max float64
	for _, c := range components {
		if componentHasKind(c, kind) && c.ConcentrationPct > max {
			max = c.ConcentrationPct
		}
	}
	return max
}

// ── STOT single / repeated exposure (GCL) ─────────────────────────────────────

func classifyStotSE(components []ClassComponent) []classOutcome {
	cat1 := sumByKindCategory(components, kindStotSE, "1")
	cat2 := sumByKindCategory(components, kindStotSE, "2")
	cat3 := sumByKindCategory(components, kindStotSE, "3")

	var outcomes []classOutcome
	switch {
	case reachesLimit(cat1, mustGCL(kindStotSE, "1")):
		outcomes = append(
			outcomes, classOutcome{
				display: "STOT SE 1", category: "1", hCodes: []string{"H370"},
				pictogram: "GHS08", danger: true,
				pCodes: []string{"P260", "P264", "P270", "P308+P311", "P405", "P501"},
			},
		)
	case reachesLimit(cat1, gclStot1Lo) || reachesLimit(cat2, mustGCL(kindStotSE, "2")):
		outcomes = append(
			outcomes, classOutcome{
				display: "STOT SE 2", category: "2", hCodes: []string{"H371"},
				pictogram: "GHS08", danger: false,
				pCodes: []string{"P260", "P264", "P270", "P308+P311", "P405", "P501"},
			},
		)
	}

	// STOT SE 3 (H335 respiratory-tract irritation / H336 narcosis) is a class in
	// its own right, not a lower rung of SE 1-2: a mixture over both limits
	// carries both, so this is deliberately not part of the switch above.
	if reachesLimit(cat3, mustGCL(kindStotSE, "3")) {
		outcomes = append(
			outcomes, classOutcome{
				display: "STOT SE 3", category: "3", hCodes: stotSe3Codes(components),
				pictogram: "GHS07", danger: false,
				pCodes: []string{"P261", "P271", "P304+P340", "P403+P233", "P405", "P501"},
			},
		)
	}
	return outcomes
}

// stotSe3Codes returns the H335/H336 codes actually present among the Cat-3
// STOT-SE components (respiratory irritation vs narcotic effects).
func stotSe3Codes(components []ClassComponent) []string {
	has335, has336 := false, false
	for _, c := range components {
		for _, h := range c.Hazards {
			if classKind(h.Class) != kindStotSE || normHazardCategory(h.Category) != "3" {
				continue
			}
			for _, code := range h.HCodes {
				switch normCategory(code) {
				case "H335":
					has335 = true
				case "H336":
					has336 = true
				}
			}
		}
	}
	var codes []string
	if has335 {
		codes = append(codes, "H335")
	}
	if has336 {
		codes = append(codes, "H336")
	}
	if len(codes) == 0 {
		codes = []string{"H335"} // category 3 with no specific code defaults to respiratory irritation
	}
	return codes
}

func classifyStotRE(components []ClassComponent) *classOutcome {
	cat1 := sumByKindCategory(components, kindStotRE, "1")
	cat2 := sumByKindCategory(components, kindStotRE, "2")

	switch {
	case reachesLimit(cat1, mustGCL(kindStotRE, "1")):
		return &classOutcome{
			display: "STOT RE 1", category: "1", hCodes: []string{"H372"},
			pictogram: "GHS08", danger: true,
			pCodes: []string{"P260", "P264", "P270", "P314", "P501"},
		}
	case reachesLimit(cat1, gclStot1Lo) || reachesLimit(cat2, mustGCL(kindStotRE, "2")):
		return &classOutcome{
			display: "STOT RE 2", category: "2", hCodes: []string{"H373"},
			pictogram: "GHS08", danger: false,
			pCodes: []string{"P260", "P314", "P501"},
		}
	}
	return nil
}

// ── Aspiration (Σ ≥ 10% + viscosity gate) ─────────────────────────────────────

func classifyAspiration(components []ClassComponent, viscosityKnownLow bool) (*classOutcome, []Flag) {
	sum := sumByKind(components, kindAsp)
	if !reachesLimit(sum, mustGCL(kindAsp, "1")) {
		return nil, nil
	}

	outcome := &classOutcome{
		display: "Asp. Tox. 1", category: "1", hCodes: []string{"H304"},
		pictogram: "GHS08", danger: true,
		pCodes: []string{"P301+P310", "P331", "P405", "P501"},
	}

	// CLP requires kinematic viscosity ≤ 20.5 mm²/s for the class to apply. When
	// the viscosity is unconfirmed we classify conservatively (the class drives a
	// Danger label) and FLAG the missing datum for the reviewer.
	if !viscosityKnownLow {
		return outcome, []Flag{
			{
				Section:  "9.1",
				Code:     FlagDataMissing,
				Severity: SeverityWarn,
				Message: fmt.Sprintf(
					"Aspiration: components carrying H304 sum to %.1f%% (≥ 10%%), so Asp. Tox. 1 is asserted conservatively. Confirm the kinematic viscosity (≤ 20.5 mm²/s at 40 °C means the class applies; above it the class does not).",
					sum,
				),
			},
		}
	}
	return outcome, nil
}

// ── Flammable liquid (flash-point gated) ──────────────────────────────────────

func classifyFlammable(flashPt *float64) (*classOutcome, []Flag) {
	if flashPt == nil {
		return nil, nil // the missing-flash-point block FLAG is raised by CheckFlammable
	}
	fp := *flashPt
	if fp > flashCat3Max {
		return nil, nil // not a flammable liquid
	}
	if fp >= flashCat12 {
		return &classOutcome{
			display: "Flam. Liq. 3", category: "3", hCodes: []string{"H226"},
			pictogram: "GHS02", danger: false,
			pCodes: []string{"P210", "P233", "P240", "P241", "P280", "P370+P378", "P403+P235", "P501"},
		}, nil
	}
	// FP < 23 °C: Cat 1 vs Cat 2 needs the boiling point. Classify Cat 2
	// conservatively and FLAG the missing datum.
	return &classOutcome{
			display: "Flam. Liq. 2", category: "2", hCodes: []string{"H225"},
			pictogram: "GHS02", danger: true,
			pCodes: []string{"P210", "P233", "P240", "P241", "P280", "P370+P378", "P403+P235", "P501"},
		}, []Flag{
			{
				Section:  "9.1",
				Code:     FlagDataMissing,
				Severity: SeverityWarn,
				Message:  "Flash point is below 23 °C; the initial boiling point is needed to split Flam. Liq. 1 (BP ≤ 35 °C) from Flam. Liq. 2. Classified as Flam. Liq. 2 conservatively pending the boiling point.",
			},
		}
}

// ── Aquatic outcome mapping ────────────────────────────────────────────────────

// aquaticOutcome maps an aquatic class verdict from ClassifyAquatic to a label
// outcome. Acute 1 and Chronic 1 carry GHS09; Chronic 2 carries GHS09 with no
// signal word; Chronic 3/4 carry no pictogram and no signal word.
func aquaticOutcome(class string) classOutcome {
	switch class {
	case aquaticAcute1:
		return classOutcome{
			display: aquaticAcute1, category: "1", hCodes: []string{"H400"}, pictogram: "GHS09",
			pCodes: []string{"P273", "P391", "P501"},
		}
	case aquaticChron1:
		return classOutcome{
			display: aquaticChron1, category: "1", hCodes: []string{"H410"}, pictogram: "GHS09",
			pCodes: []string{"P273", "P391", "P501"},
		}
	case aquaticChron2:
		// Aquatic Chronic 2 (H411) carries NO pictogram and NO signal word
		// (CLP 1272/2008 Annex I Table 4.1.0 / Annex V): only the H411 statement
		// and its P-codes appear on the label.
		return classOutcome{
			display: aquaticChron2, category: "2", hCodes: []string{"H411"}, pictogram: "", noSignal: true,
			pCodes: []string{"P273", "P391", "P501"},
		}
	case aquaticChron3:
		return classOutcome{
			display: aquaticChron3, category: "3", hCodes: []string{"H412"}, pictogram: "", noSignal: true,
			pCodes: []string{"P273", "P501"},
		}
	default: // aquaticChron4
		return classOutcome{
			display: aquaticChron4, category: "4", hCodes: []string{"H413"}, pictogram: "", noSignal: true,
			pCodes: []string{"P273", "P501"},
		}
	}
}

// ── Label assembly ─────────────────────────────────────────────────────────────

// assembleLabel derives the GHS/CLP label from the triggered outcomes: the
// union of H-codes and pictograms (with CLP precedence suppression of GHS07),
// the signal word (Danger if any outcome is a Danger class), the EUH codes
// (EUH208 when a sensitiser sits in its disclosure band), and a deduplicated,
// ordered P-code set.
func assembleLabel(components []ClassComponent, outcomes []classOutcome) SdsLabel {
	var hCodes, pCodes []string
	danger := false
	warning := false
	picts := map[string]bool{}

	// Track whether GHS07 was added for a non-irritation reason (acute tox cat 4,
	// skin sensitisation, STOT SE 3) so CLP precedence can suppress it correctly.
	ghs07FromOther := false
	// Art. 26(d) inputs: whether GHS08 was raised specifically by respiratory
	// sensitisation (the ONLY GHS08 source that suppresses GHS07), and whether
	// GHS07 was raised by a source 26(d) does NOT suppress — Acute Tox. 4 or
	// STOT SE 3 — which must keep GHS07 even alongside a respiratory-sens GHS08.
	ghs08FromRespSens := false
	ghs07From26dProtected := false

	for _, o := range outcomes {
		hCodes = append(hCodes, o.hCodes...)
		pCodes = append(pCodes, o.pCodes...)
		switch {
		case o.danger:
			danger = true
		case !o.noSignal:
			warning = true
		}
		if o.pictogram == "" {
			continue
		}
		if o.pictogram == "GHS07" && o.display != "Skin Irrit. 2" && o.display != "Eye Irrit. 2" {
			ghs07FromOther = true
		}
		if o.pictogram == "GHS07" && (o.display == "Acute Tox. 4" || o.display == "STOT SE 3") {
			ghs07From26dProtected = true
		}
		if o.pictogram == "GHS08" && strings.HasPrefix(o.display, "Resp. Sens.") {
			ghs08FromRespSens = true
		}
		picts[o.pictogram] = true
	}

	// CLP Annex I §1.3.4 pictogram precedence (Article 26):
	//   (a) if GHS06 (skull) applies, GHS07 (exclamation mark) shall not appear —
	//       unconditionally, whatever reason added GHS07.
	//   (b) if GHS05 (corrosion) applies, GHS07 is dropped only where it stands for
	//       skin/eye irritation (kept when it also covers acute tox / skin sens / STOT).
	//   (d) if GHS08 (health) applies FOR RESPIRATORY SENSITISATION, GHS07 is dropped
	//       where it stands for skin sensitisation or skin/eye irritation — but kept
	//       when GHS07 also covers Acute Tox. 4 or STOT SE 3 (which 26(d) does not
	//       suppress). GHS08 from CMR / STOT / aspiration does NOT suppress GHS07.
	if picts["GHS07"] {
		suppressedByCorr := picts["GHS05"] && !ghs07FromOther
		suppressedByAcute := picts["GHS06"]
		suppressedByRespSens := ghs08FromRespSens && !ghs07From26dProtected
		if suppressedByCorr || suppressedByAcute || suppressedByRespSens {
			delete(picts, "GHS07")
		}
	}

	// Signal word (CLP Annex I §1.4): Danger if any class requires it, else
	// Warning if any signal-bearing class triggers, else none. Classes that
	// carry no signal word (Aquatic Chronic 2/3/4, lactation H362) never raise it.
	signalWord := ""
	switch {
	case danger:
		signalWord = "Danger"
	case warning:
		signalWord = "Warning"
	}

	// H-codes and pictograms in CLP statement order (physical → health →
	// environmental). Whether the mixture is classified at all also gates EUH210.
	orderedH := orderHCodesCLP(hCodes)
	pictsList := sortedPictograms(picts)
	classified := signalWord != "" || len(orderedH) > 0 || len(pictsList) > 0

	// EUH codes via the trigger table: EUH208 (authored here, with names),
	// supplier-declared constituent disclosures (EUH204/205/401…), and EUH210 for
	// a not-classified mixture supplied to the public that still contains a
	// hazardous substance.
	euh208Names := euh208Substances(components)
	euhCodes := assembleEUH(components, classified, euh208Names)

	// The SDS Section 2.2 may carry every triggered P-statement; the physical
	// label is pruned to the most relevant set (~6) per CLP Annex IV §6.3.
	fullP := dedupeOrdered(pCodes)

	return SdsLabel{
		Pictograms:       pictsList,
		SignalWord:       signalWord,
		HCodes:           orderedH,
		EuhCodes:         euhCodes,
		Euh208Substances: euh208Names,
		PCodes:           pruneLabelPStatements(fullP),
		PCodesSds:        fullP,
	}
}

// euhDisclosureCodes are the supplier-declared "Contains <constituent>" EUH codes
// that legitimately propagate from a single component to the mixture label (they
// name a constituent the consumer must be warned about, CLP Annex II / Annex III).
// EUH208 is deliberately excluded — the engine authors it itself with the
// substance names. The self-classification EUH0xx codes (EUH014, EUH018, EUH019,
// EUH066, EUH070…) describe MIXTURE behaviour and are not auto-propagated from one
// component without a mixture-level assessment.
// Restricted to codes the CLP phrase library can render in full (§2.2 / §16); a
// propagated code with no text would print bare. Extend both together.
var euhDisclosureCodes = map[string]bool{
	"EUH204": true, // contains isocyanates — may produce an allergic reaction
	"EUH205": true, // contains epoxy constituents — may produce an allergic reaction
	"EUH401": true, // comply with the instructions for use (plant-protection products)
}

// assembleEUH builds the mixture label's EUH-code set from the trigger table:
//   - EUH208 when a sub-threshold sensitiser sits in its disclosure band (authored
//     by euh208Substances with names; H317 suppresses it for the same substance);
//   - the supplier-declared constituent-disclosure codes carried on the components
//     (euhDisclosureCodes), e.g. EUH204/205/401;
//   - EUH210 ("Safety data sheet available on request") when the mixture is NOT
//     classified but still contains a hazardous substance and is supplied to the
//     public (CLP Article 25(6) / Annex II).
//
// The result is de-duplicated and ordered numerically (EUH204 < … < EUH210 < …).
func assembleEUH(components []ClassComponent, classified bool, euh208Names []string) []string {
	var codes []string
	if len(euh208Names) > 0 {
		codes = append(codes, "EUH208")
	}
	for _, c := range components {
		for _, code := range c.Euh {
			if euhDisclosureCodes[strings.ToUpper(strings.TrimSpace(code))] {
				codes = append(codes, code)
			}
		}
	}
	if !classified && containsHazardousSubstance(components) {
		codes = append(codes, "EUH210")
	}
	return orderEUHCodes(codes)
}

// containsHazardousSubstance reports whether any component carries hazard data —
// a CLP hazard class/category, a skin-sens or aquatic classification, or a
// supplier-declared EUH code. It gates EUH210: a not-classified mixture only needs
// "SDS available on request" when it actually contains something hazardous.
func containsHazardousSubstance(components []ClassComponent) bool {
	for _, c := range components {
		if len(c.Hazards) > 0 || c.SkinSensCategory != "" ||
			c.AquaticAcuteCategory != "" || c.AquaticChronicCategory != "" || len(c.Euh) > 0 {
			return true
		}
	}
	return false
}

// orderHCodesCLP de-duplicates and orders H-codes in CLP statement order: physical
// (H2xx) → health (H3xx) → environmental (H4xx), ascending within each band. The
// integer after "H" yields exactly that order; codes with no parseable number sort
// last in first-seen order.
func orderHCodesCLP(codes []string) []string {
	deduped := dedupeOrdered(codes)
	sort.SliceStable(
		deduped, func(i, j int) bool {
			ki, kj := hStatementNumber(deduped[i]), hStatementNumber(deduped[j])
			if (ki == 0) != (kj == 0) {
				return kj == 0 // a parseable code sorts before an unparseable one
			}
			return ki < kj
		},
	)
	return deduped
}

// orderEUHCodes de-duplicates and orders EUH codes numerically (EUH204 < EUH205 <
// … < EUH210 < EUH401), matching the way they are listed after the H-statements.
func orderEUHCodes(codes []string) []string {
	deduped := dedupeOrdered(codes)
	sort.SliceStable(
		deduped, func(i, j int) bool {
			return euhStatementNumber(deduped[i]) < euhStatementNumber(deduped[j])
		},
	)
	return deduped
}

// hStatementNumber returns the integer after the leading "H" in an H-code (e.g.
// "H315" → 315), or 0 when there is none.
func hStatementNumber(code string) int { return leadingNumber(code, "H") }

// euhStatementNumber returns the integer after the leading "EUH" in an EUH-code
// (e.g. "EUH210" → 210; a trailing letter like "EUH209A" is ignored → 209).
func euhStatementNumber(code string) int { return leadingNumber(code, "EUH") }

// leadingNumber strips a case-insensitive prefix then reads the leading run of
// digits as an int (0 when none), used to order H/EUH statements numerically.
func leadingNumber(code, prefix string) int {
	s := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(code)), prefix)
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// euh208Substances returns the NAMES of the skin-sensitising components that
// trigger EUH208: those present BELOW their active H317 limit but at/above the
// EUH208 disclosure band [limit/10, limit). A sensitiser at/above its H317 limit
// instead drives the mixture's Skin Sens. classification (H317) and is NOT named
// by EUH208 (CLP Annex II §2.8) — the band's upper bound enforces that
// suppression, so a classified sensitiser never carries both H317 and EUH208.
// Names are returned in component order, de-duplicated case-insensitively, so
// the label can read "Contains <names>. May produce an allergic reaction."
func euh208Substances(components []ClassComponent) []string {
	var names []string
	seen := map[string]bool{}
	for _, c := range components {
		limit, ok := skinSensH317Limit(c)
		if !ok {
			continue
		}
		if c.ConcentrationPct >= limit/euh208Divisor && c.ConcentrationPct < limit {
			key := strings.ToLower(strings.TrimSpace(c.Name))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			names = append(names, c.Name)
		}
	}
	return names
}

// outcomesToHazards renders the triggered outcomes as the stored MixtureHazard
// list (the "as a whole" classification shown in Section 2.1).
func outcomesToHazards(outcomes []classOutcome) []MixtureHazard {
	out := make([]MixtureHazard, 0, len(outcomes))
	for _, o := range outcomes {
		out = append(
			out, MixtureHazard{
				Class:    o.display,
				Category: o.category,
				HCodes:   o.hCodes,
			},
		)
	}
	return out
}

// sortedPictograms returns the pictogram codes in ascending GHS-number order.
func sortedPictograms(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// dedupeOrdered returns the input with duplicates removed, preserving first-seen
// order (so the label reads in the order the classes were evaluated).
func dedupeOrdered(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		key := strings.ToUpper(strings.TrimSpace(v))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

// labelPStatementCap is the soft cap on the number of P-statements shown on the
// PHYSICAL LABEL. CLP Annex IV §6.3 advises selecting the most relevant
// statements (normally no more than six); the full set still appears in SDS
// Section 2.2 via SdsLabel.PCodesSds.
const labelPStatementCap = 6

// pStatementLabelPriority ranks P-statements for label selection (lower number
// = kept first). The order favours what matters most on a small consumer label:
// essential PPE/avoidance prevention, the highest-value response actions for the
// likely exposure routes, environmental release, then disposal and storage.
// Codes absent from the table sort after every ranked code (priority 1000) in
// their original order, so an unranked statement is only deprioritised, never
// lost from the full SDS set.
var pStatementLabelPriority = map[string]int{
	"P280":           1,  // wear protective equipment (near-universal prevention)
	"P261":           2,  // avoid breathing vapours/spray
	"P305+P351+P338": 3,  // eye exposure — the severe, common response
	"P333+P313":      4,  // skin allergic reaction response (sensitiser)
	"P302+P352":      5,  // skin contact — wash
	"P501":           6,  // disposal
	"P308+P313":      7,  // CMR exposure response
	"P273":           8,  // avoid release to the environment
	"P337+P313":      9,  // persistent eye irritation
	"P332+P313":      10, // skin irritation response
	"P362+P364":      11, // remove and wash contaminated clothing
	"P201":           12, // CMR — obtain instructions
	"P202":           13,
	"P301+P310":      14, // swallowed — severe
	"P301+P312":      15, // swallowed — harmful
	"P304+P340":      16, // inhaled
	"P342+P311":      17, // respiratory sensitiser response
	"P264":           18, // wash hands after handling
	"P272":           19, // contaminated work clothing
	"P270":           20,
	"P330":           21,
	"P391":           22, // collect spillage
	"P403+P235":      23,
	"P403+P233":      24,
	"P405":           25, // store locked up
	"P210":           26, // flammable prevention
	"P233":           27,
	"P240":           28,
	"P241":           29,
	"P370+P378":      30, // fire response
	"P260":           31,
	"P263":           32,
}

// dropRedundantSingles removes single P-codes whose code is a constituent of a
// combined P-code present in the same list (e.g. drop "P305" when
// "P305+P351+P338" is present), de-duplicating combined statements per CLP
// Annex IV §6.3. Order is preserved.
func dropRedundantSingles(codes []string) []string {
	covered := map[string]bool{}
	for _, c := range codes {
		if strings.Contains(c, "+") {
			for _, part := range strings.Split(c, "+") {
				covered[normKey(part)] = true
			}
		}
	}
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if !strings.Contains(c, "+") && covered[normKey(c)] {
			continue // single code already carried by a combined statement
		}
		out = append(out, c)
	}
	return out
}

// pruneLabelPStatements selects the physical-label P-statement set from the full
// triggered list: it first de-duplicates combined statements (dropRedundantSingles),
// then — if still over the cap — keeps the highest-priority statements up to
// labelPStatementCap, restoring the original reading order among the selected
// codes. The full set is preserved separately for SDS Section 2.2.
func pruneLabelPStatements(full []string) []string {
	pruned := dropRedundantSingles(full)
	if len(pruned) <= labelPStatementCap {
		return pruned
	}

	type rankedP struct {
		code string
		idx  int
	}
	ranked := make([]rankedP, len(pruned))
	for i, c := range pruned {
		ranked[i] = rankedP{code: c, idx: i}
	}
	sort.SliceStable(
		ranked, func(i, j int) bool {
			pi, oki := pStatementLabelPriority[normKey(ranked[i].code)]
			if !oki {
				pi = 1000
			}
			pj, okj := pStatementLabelPriority[normKey(ranked[j].code)]
			if !okj {
				pj = 1000
			}
			if pi != pj {
				return pi < pj
			}
			return ranked[i].idx < ranked[j].idx
		},
	)

	selected := make(map[string]bool, labelPStatementCap)
	for _, r := range ranked[:labelPStatementCap] {
		selected[normKey(r.code)] = true
	}
	out := make([]string, 0, labelPStatementCap)
	for _, c := range pruned { // restore original label reading order
		if selected[normKey(c)] {
			out = append(out, c)
		}
	}
	return out
}
