package core

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cpouldev/go-clp/ufi"
)

// Recipe identifies the finished mixture and supplies its UFI formulation
// number.
type Recipe struct {
	// Name is the finished mixture's human-readable product name. It is used
	// only as a transport product-type hint.
	Name string

	// FormulationNumber is the mixture-specific number used to generate a UFI.
	// It must be in the inclusive range 0..268,435,455 when a UFI is required.
	FormulationNumber int
}

// RecipeLine is one material contribution to a finished mixture. The engine
// uses recipe-line ratios to compute composition and detects incompatible units.
type RecipeLine struct {
	// MaterialID joins this line to its parsed Supplier SDS extraction.
	// Leave it empty for a non-chemical article.
	MaterialID string

	// MaterialName is the human-readable component name used in audit output
	// and validation messages.
	MaterialName string

	// MaterialCode is an optional internal or supplier material code.
	MaterialCode string

	// Quantity is the amount of material in Unit. Ratios, not absolute amounts,
	// determine the computed composition.
	Quantity float64

	// Unit identifies a mass, volume, or article unit such as "g", "ml", or "pcs".
	Unit string
}

// SupplierSDS records that supplier safety data is available for a material.
type SupplierSDS struct {
	// MaterialID identifies the recipe material covered by a supplier safety
	// data sheet. It joins to RecipeLine.MaterialID.
	MaterialID string
}

// excludeUnmappedLines partitions the recipe lines into those that contribute to
// the documented mixture and those that cannot. A raw-material line whose
// material has no primary SDS on file is dropped: an SDS describes only what
// supplier data can substantiate, so an undocumented material must neither appear
// as a component nor dilute the % w/w (computeComposition then re-baselines over
// what remains). The dropped materials are named once, by name + code, in a
// single informational sds_unmapped FLAG so nothing disappears silently and the
// operator can attach an SDS to include one.
//
// Article lines (no MaterialID) are not chemical materials and pass through
// unchanged. A recipe whose every raw material has a primary SDS returns the
// lines untouched and no FLAG.
func excludeUnmappedLines(lines []RecipeLine, sdsRows []SupplierSDS) ([]RecipeLine, []Flag) {
	haveSDS := make(map[string]bool, len(sdsRows))
	for _, row := range sdsRows {
		haveSDS[row.MaterialID] = true
	}

	mapped := make([]RecipeLine, 0, len(lines))
	var excludedChemical, excludedArticle []string
	seen := make(map[string]bool)
	for _, line := range lines {
		rmID := strings.TrimSpace(line.MaterialID)
		if rmID == "" || haveSDS[rmID] {
			mapped = append(mapped, line)
			continue
		}

		if seen[rmID] {
			continue
		}
		seen[rmID] = true
		label := strings.TrimSpace(line.MaterialName)
		if line.MaterialCode != "" {
			label = strings.TrimSpace(fmt.Sprintf("%s (%s)", label, line.MaterialCode))
		}
		if label == "" {
			label = "(unnamed material)"
		}

		if unitFamily(line.Unit) == unitArticle {
			excludedArticle = append(excludedArticle, label)
			continue
		}
		excludedChemical = append(excludedChemical, label)
	}

	var flags []Flag
	if len(excludedChemical) > 0 {
		flags = append(
			flags, Flag{
				Section:  "3.2",
				Code:     FlagSdsUnmapped,
				Severity: SeverityBlock,
				Message: fmt.Sprintf(
					"%d chemical material(s) in this recipe have no supplier SDS on file: %s. They contribute mass to the product but no hazard data, so their hazards are absent from the classification and every other component's %% w/w has been re-baselined over a smaller total. This SDS understates the product until a primary SDS is attached to each of them and it is regenerated.",
					len(excludedChemical), strings.Join(excludedChemical, ", "),
				),
			},
		)
	}
	if len(excludedArticle) > 0 {
		flags = append(
			flags, Flag{
				Section:  "3.2",
				Code:     FlagSdsUnmapped,
				Severity: SeverityInfo,
				Message: fmt.Sprintf(
					"Excluded %d non-chemical article(s) with no SDS on file: %s. Articles are not part of the mixture an SDS describes, so no supplier SDS is expected for them.",
					len(excludedArticle), strings.Join(excludedArticle, ", "),
				),
			},
		)
	}
	return mapped, flags
}

// ParsedExtraction is the structured result parsed from one supplier SDS.
type ParsedExtraction struct {
	RawMaterialName   string            `json:"rawMaterialName"`
	Code              string            `json:"code"`
	FlashPointCelsius *float64          `json:"flashPointCelsius,omitempty"`
	Substances        []ParsedSubstance `json:"substances"`
	Issues            []string          `json:"issues"`
}

// ParsedSubstance is one substance entry from a supplier SDS extraction.
type ParsedSubstance struct {
	Name               string         `json:"name"`
	CasNumber          string         `json:"casNumber"`
	EcNumber           string         `json:"ecNumber"`
	ConcentrationRange string         `json:"concentrationRange"`
	HCodes             []string       `json:"hCodes"`
	EuhCodes           []string       `json:"euhCodes"`
	Hazards            []ParsedHazard `json:"hazards"`
}

// ParsedHazard is one hazard class+category entry extracted for a substance.
type ParsedHazard struct {
	Class          string  `json:"class"`
	Category       string  `json:"category"`
	MFactorAcute   int     `json:"mFactorAcute,omitempty"`
	MFactorChronic int     `json:"mFactorChronic,omitempty"`
	SCLPercent     float64 `json:"sclPercent,omitempty"`
}

// generateUFI generates the product UFI from the UFI issuer's VAT and the
// recipe's formulation number, but ONLY when the mixture needs one. Under CLP
// Annex VIII a UFI / Poison Centre Notification is required for a mixture
// classified for a HEALTH or PHYSICAL hazard; a mixture classified solely for
// environmental hazards (and a not-classified mixture) carries no UFI
// obligation, so the caller passes requiresUFI(label) here.
//
// vat is the ISO-prefixed VAT of the duty holder placing the mixture on the
// market (e.g. "EL123456789"); an empty vat yields a data-missing FLAG rather
// than a UFI built from a zero VAT, which would collide across companies.
// formulationNo must satisfy 0 ≤ n ≤ 268,435,455 (2^28-1); the range-guard
// yields a FLAG rather than a silently-wrong UFI.
//
// Returns the UFI string ("" when not generated) and any FLAGs.
func generateUFI(recipe *Recipe, country, vat string, ufiRequired bool) (string, []Flag) {
	if !ufiRequired {
		return "", nil
	}

	if strings.TrimSpace(country) == "" {
		return "", []Flag{
			{
				Section:  "1.1",
				Code:     FlagDataMissing,
				Severity: SeverityWarn,
				Message: "No UFI issuer country was supplied, so a UFI cannot be generated. " +
					"Set EngineInput.CompanyCountry to the VAT-issuing country code.",
			},
		}
	}

	if strings.TrimSpace(vat) == "" {
		return "", []Flag{
			{
				Section:  "1.1",
				Code:     FlagDataMissing,
				Severity: SeverityWarn,
				Message: "No UFI issuer VAT was supplied, so a UFI cannot be generated. " +
					"Set EngineInput.CompanyVAT to the ISO-prefixed VAT of the duty holder placing the mixture on the market.",
			},
		}
	}

	formulationNo := recipe.FormulationNumber
	if formulationNo < 0 || formulationNo > ufi.MaxFormulationNumber {
		return "", []Flag{
			{
				Section:  "1.1",
				Code:     FlagDataMissing,
				Severity: SeverityWarn,
				Message: fmt.Sprintf(
					"Formulation number %d is out of range [0, %d]; a UFI cannot be generated. Correct the recipe's formulation number.",
					formulationNo, ufi.MaxFormulationNumber,
				),
			},
		}
	}

	generated, err := ufi.Generate(country, vat, formulationNo)
	if err != nil {
		return "", []Flag{
			{
				Section:  "1.1",
				Code:     FlagDataMissing,
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("UFI generation failed: %s.", err.Error()),
			},
		}
	}
	return generated, nil
}

// Unit families returned by unitFamily.
const (
	unitMass    = "mass"
	unitVolume  = "volume"
	unitArticle = "article"
)

// massUnits / volumeUnits enumerate the chemical recipe-line unit families
// (% w/w can be computed within one family but NOT across mass and volume
// without a density — mixing the two yields unit_inconsistent). Keys cover both
// canonical names (GRAM, KILOGRAM, MILLILITRE, LITRE — lower-cased) and common
// abbreviations. articleUnits enumerate count and
// length units (PIECE, METER, CENTIMETER) that denote a physical ARTICLE —
// bottle, card, label, wick, ribbon — never a chemical mixture component; those
// lines are dropped from the composition so packaging cannot dilute the % w/w.
var (
	massUnits = map[string]bool{
		"g": true, "kg": true, "mg": true, "gr": true,
		"gram": true, "kilogram": true,
	}
	volumeUnits = map[string]bool{
		"ml": true, "l": true, "cl": true, "lt": true,
		"millilitre": true, "milliliter": true, "litre": true, "liter": true,
	}
	articleUnits = map[string]bool{
		"piece": true, "pieces": true, "pcs": true, "pc": true,
		"meter": true, "metre": true, "m": true,
		"centimeter": true, "centimetre": true, "cm": true,
	}
)

// computeComposition computes each recipe line's % w/w (qty ÷ recipe total × 100)
// and returns the audit Components plus any FLAGs. It is the deterministic
// composition core (pure, no I/O):
//
//   - Non-chemical ARTICLE lines (counted by piece or measured by length —
//     bottle, card, label, wick, ribbon; see articleUnits) are excluded entirely
//     and re-baseline the remaining chemical fraction, so packaging never appears
//     as a component nor dilutes the % w/w. An informational article_excluded
//     FLAG lists what was dropped so nothing disappears silently.
//   - Lines whose quantity is zero contribute 0 to the total and get pct 0
//     (zero-qty components are carried safely — the Step 9 carry-forward).
//   - A recipe that mixes mass and volume units with no density raises a single
//     unit_inconsistent FLAG (the percentages cannot be trusted, so they are left
//     at 0 rather than computed wrongly).
//   - A zero or negative total (e.g. an empty recipe) yields pct 0 for every line
//     with no division by zero.
//
// Lines with an empty or unrecognised unit are treated as chemical (kept) and as
// belonging to the dominant family, so a missing unit on an otherwise-consistent
// recipe is never silently dropped nor spuriously flagged.
func computeComposition(lines []RecipeLine) ([]Component, []Flag) {
	components := make([]Component, 0, len(lines))
	var flags []Flag

	chemical := make([]RecipeLine, 0, len(lines))
	var excluded []string
	total := 0.0
	sawMass := false
	sawVolume := false
	for _, l := range lines {
		family := unitFamily(l.Unit)
		if family == unitArticle {
			excluded = append(excluded, l.MaterialName)
			continue
		}
		chemical = append(chemical, l)
		total += l.Quantity
		switch family {
		case unitMass:
			sawMass = true
		case unitVolume:
			sawVolume = true
		}
	}
	if len(excluded) > 0 {
		flags = append(
			flags, Flag{
				Section:  "3.2",
				Code:     FlagArticleExcluded,
				Severity: SeverityInfo,
				Message: fmt.Sprintf(
					"Excluded %d non-chemical article line(s) from the mixture composition (counted by piece/length — e.g. bottle, card, label): %s. Percentages are re-baselined over the chemical (mass/volume) fraction only.",
					len(excluded), strings.Join(excluded, ", "),
				),
			},
		)
	}

	mixedUnits := sawMass && sawVolume

	for _, l := range chemical {
		c := Component{
			MaterialID:      l.MaterialID,
			RawMaterialName: l.MaterialName,
			Code:            l.MaterialCode,
			Qty:             l.Quantity,
			QtyUnit:         l.Unit,
		}

		if !mixedUnits && total > 0 {
			c.Pct = l.Quantity / total * 100
		}
		components = append(components, c)
	}

	if mixedUnits {
		flags = append(
			flags, Flag{
				Section:  "3.2",
				Code:     FlagUnitInconsistent,
				Severity: SeverityBlock,
				Message:  "Recipe lines mix mass and volume units with no density available; component percentages cannot be computed. Express all quantities in one family (mass or volume) or supply a density.",
			},
		)
	}

	return components, flags
}

// unitFamily classifies a recipe-line unit string as "mass", "volume",
// "article" (a counted/measured physical item — see articleUnits), or ""
// (unknown / empty). Matching is case-insensitive on the trimmed value.
func unitFamily(unit string) string {
	u := strings.ToLower(strings.TrimSpace(unit))
	switch {
	case u == "":
		return ""
	case massUnits[u]:
		return unitMass
	case volumeUnits[u]:
		return unitVolume
	case articleUnits[u]:
		return unitArticle
	default:
		return ""
	}
}

// buildClassComponents rolls the deterministic composition + parsed supplier
// hazard data up to the SUBSTANCE level the classifier needs. Each supplier SDS
// component is expanded into its declared substances at their final % w/w in the
// finished mixture (component % w/w × the substance's declared upper-bound range),
// so additive classes (acute toxicity, irritation, the aquatic environment) sum
// real substance concentrations rather than treating a whole fragrance compound
// as one worst-case lump. Substances are aggregated across components by CAS
// (else name) so a substance present in two compounds is summed once before the
// GCL test. It returns the classifier units plus any FLAGs.
//
// Fallbacks stay conservative and honest:
//   - a component with no supplier extraction is carried as one non-hazardous
//     unit at its % w/w (it contributes mass to the denominator, no hazard);
//   - a multi-substance component whose supplier states NO concentrations cannot
//     be apportioned, so it is carried at its full % w/w with its worst categories
//     and a data_missing FLAG asks for the concentrations a precise call needs.
func buildClassComponents(
	composition []Component,
	extractions map[string]ParsedExtraction,
	overrides map[string]float64,
) ([]ClassComponent, []Flag) {
	type agg struct {
		name      string
		cas       string
		ec        string
		pct       float64 // worst-case (upper) % w/w used for triggering
		pctLow    float64 // lower-bound % w/w (for the range-flip validation)
		fromRange bool
		hazards   []ComponentHazard
		euh       []string // supplier-declared EUH codes (propagated to the label)
	}
	byKey := make(map[string]*agg)
	var order []string
	var flags []Flag

	add := func(name, cas, ec string, pct, pctLow float64, fromRange bool, hz []ComponentHazard, euh []string) {
		key := strings.ToLower(strings.TrimSpace(cas))
		if key == "" {
			key = "name:" + strings.ToLower(strings.TrimSpace(name))
		}
		a, ok := byKey[key]
		if !ok {
			a = &agg{name: name, cas: cas, ec: ec}
			byKey[key] = a
			order = append(order, key)
		}
		a.pct += pct
		a.pctLow += pctLow
		if fromRange {
			a.fromRange = true
		}
		a.hazards = append(a.hazards, hz...)
		a.euh = append(a.euh, euh...)
		if a.cas == "" && cas != "" {
			a.cas = cas
		}
		if a.ec == "" && ec != "" {
			a.ec = ec
		}
	}

	for _, c := range composition {
		matPct := c.Pct
		ext, ok := extractions[c.MaterialID]
		if !ok || len(ext.Substances) == 0 {
			add(c.RawMaterialName, "", "", matPct, matPct, false, nil, nil)
			continue
		}

		anyRange := false
		for _, sub := range ext.Substances {
			if _, ok := parseRangeUpperPct(sub.ConcentrationRange); ok {
				anyRange = true
				break
			}
		}

		if !anyRange && len(ext.Substances) > 1 {
			add(
				c.RawMaterialName,
				primaryCAS(ext),
				primaryEC(ext),
				matPct,
				matPct,
				false,
				flattenSubstanceHazards(ext),
				flattenSubstanceEUH(ext),
			)
			flags = append(
				flags, Flag{
					Section:  "3.2",
					Code:     FlagDataMissing,
					Severity: SeverityWarn,
					Message: fmt.Sprintf(
						"Supplier SDS for %q lists hazardous substances without their concentrations; %q is classified conservatively at its full %.1f%% rather than per substance. Provide the substance concentrations for a precise mixture call.",
						c.RawMaterialName, c.RawMaterialName, matPct,
					),
				},
			)
			continue
		}

		var unquantified []string
		for _, sub := range ext.Substances {

			frac := 1.0
			fracLow := 1.0
			fromRange := false
			strictUpper := false
			if up, ok := parseRangeUpperPct(sub.ConcentrationRange); ok {
				if lowerBoundOnly(sub.ConcentrationRange) {

					frac = 1.0
					fromRange = true
					if low, okLow := parseRangeLowerPct(sub.ConcentrationRange); okLow {
						fracLow = low / 100
					} else {
						fracLow = up / 100
					}
				} else {
					frac = up / 100
					fracLow = frac
					strictUpper = upperBoundIsStrict(sub.ConcentrationRange)
					if low, okLow := parseRangeLowerPct(sub.ConcentrationRange); okLow {
						fracLow = low / 100
						fromRange = low != up
					}
				}
			} else if anyRange {
				// A partially-quantified SDS: siblings declared a range and this
				// substance did not, so it keeps frac = 1.0 and is carried at the
				// material's FULL percentage. That is the conservative bound, but
				// it silently inflates the mixture past 100% and can trigger a
				// limit on its own, so it must not pass unreported.
				unquantified = append(unquantified, substanceDisplayName(sub, c.RawMaterialName))
			}

			upper := matPct * frac
			if strictUpper {
				upper = math.Max(0, upper-strictBoundMargin)
			}
			add(
				substanceDisplayName(sub, c.RawMaterialName),
				sub.CasNumber,
				sub.EcNumber,
				upper,
				matPct*fracLow,
				fromRange,
				substanceHazards(sub),
				sub.EuhCodes,
			)
		}

		if len(unquantified) > 0 {
			flags = append(
				flags, Flag{
					Section:  "3.2",
					Code:     FlagDataMissing,
					Severity: SeverityWarn,
					Message: fmt.Sprintf(
						"Supplier SDS for %q declares concentrations for some substances but not for %s; each of those is classified conservatively at the material's full %.1f%%, which overstates its contribution and can push the disclosed composition past 100%%. Provide the missing concentrations for a precise mixture call.",
						c.RawMaterialName, strings.Join(unquantified, ", "), matPct,
					),
				},
			)
		}
	}

	out := make([]ClassComponent, 0, len(order))
	for _, k := range order {
		a := byKey[k]
		pct, pctLow, fromRange := a.pct, a.pctLow, a.fromRange

		if ov, ok := lookupOverride(overrides, a.cas, a.name); ok {
			pct, pctLow, fromRange = ov, ov, false
		}
		cc := ClassComponent{
			Name:                a.name,
			CasNumber:           a.cas,
			EcNumber:            a.ec,
			ConcentrationPct:    pct,
			ConcentrationLowPct: pctLow,
			FromRange:           fromRange,
			Hazards:             a.hazards,
			Euh:                 dedupeOrdered(a.euh),
		}
		applyDedicatedFields(&cc)
		out = append(out, cc)
	}
	return out, flags
}

// lookupOverride finds an operator-supplied finished-product concentration (% w/w)
// for a substance, matching by CAS first (case-insensitive) then by name. It lets
// the operator pin the real figure when the supplier only declared a range.
func lookupOverride(overrides map[string]float64, cas, name string) (float64, bool) {
	if len(overrides) == 0 {
		return 0, false
	}
	if cas != "" {
		if v, ok := overrides[strings.ToLower(strings.TrimSpace(cas))]; ok {
			return v, true
		}
	}
	if n := strings.ToLower(strings.TrimSpace(name)); n != "" {
		if v, ok := overrides[n]; ok {
			return v, true
		}
	}
	return 0, false
}

// substanceDisplayName returns the substance's own name, falling back to the
// component (material) name when the supplier did not name the substance.
func substanceDisplayName(s ParsedSubstance, materialName string) string {
	if n := strings.TrimSpace(s.Name); n != "" {
		return n
	}
	return materialName
}

// substanceHazards maps one parsed substance's hazard entries to the stored
// ComponentHazard list, carrying the substance's H-codes and SCL onto each entry
// so the classifier (which keys some checks on specific H-codes) sees them.
func substanceHazards(s ParsedSubstance) []ComponentHazard {
	out := make([]ComponentHazard, 0, len(s.Hazards))
	for _, h := range s.Hazards {
		out = append(
			out, ComponentHazard{
				Class:          h.Class,
				Category:       h.Category,
				HCodes:         s.HCodes,
				MFactorAcute:   h.MFactorAcute,
				MFactorChronic: h.MFactorChronic,
				SCLPct:         h.SCLPercent,
			},
		)
	}
	return out
}

// flattenSubstanceHazards collects every substance's hazards for a component
// whose substances cannot be apportioned (the conservative material-level path).
func flattenSubstanceHazards(ext ParsedExtraction) []ComponentHazard {
	var out []ComponentHazard
	for _, s := range ext.Substances {
		out = append(out, substanceHazards(s)...)
	}
	return out
}

// flattenSubstanceEUH collects every substance's supplier-declared EUH codes for
// the conservative material-level path (companion of flattenSubstanceHazards).
func flattenSubstanceEUH(ext ParsedExtraction) []string {
	var out []string
	for _, s := range ext.Substances {
		out = append(out, s.EuhCodes...)
	}
	return out
}

// applyDedicatedFields populates the dedicated skin-sensitisation and aquatic
// fields the legacy classifier cores (ClassifySkinSens / ClassifyAquatic) read,
// from the component's full hazard list. The worst category per class wins
// (1A over 1/1B for skin sens; the most toxic aquatic rung). Respiratory
// sensitisation is intentionally NOT folded into the skin-sens field.
func applyDedicatedFields(cc *ClassComponent) {
	for _, h := range cc.Hazards {
		class, err := ParseHazardClass(h.Class)
		if err != nil {
			continue
		}
		cat := normHazardCategory(h.Category)
		switch class {
		case HazardClassSkinSensitisation:
			cc.SkinSensCategory = strongerSkinSens(cc.SkinSensCategory, cat)
			if h.SCLPct > 0 {
				cc.SkinSensSCLPct = h.SCLPct
			}
		case HazardClassAquaticAcute:
			if cat == "1" {
				cc.AquaticAcuteCategory = "1"
				if h.MFactorAcute > 0 {
					cc.MAcute = h.MFactorAcute
				}
			}
		case HazardClassAquaticChronic:
			cc.AquaticChronicCategory = strongerChronic(cc.AquaticChronicCategory, cat)
			if h.MFactorChronic > 0 {
				cc.MChronic = h.MFactorChronic
			}
		case HazardClassAquatic:
			// ParseHazardClass accepts the generic "hazardous to the aquatic
			// environment" with no tier. Disambiguate from the H-codes — H400 is
			// the acute endpoint, H410-H413 the chronic ones — and fall back to
			// chronic, whose cascade has a rung for every category, so an
			// untyped entry can never drop out of the classification entirely.
			acute, chronic := aquaticTiersFromHCodes(h.HCodes)
			if !acute && !chronic {
				chronic = true
			}
			if acute && cat == "1" {
				cc.AquaticAcuteCategory = "1"
				if h.MFactorAcute > 0 {
					cc.MAcute = h.MFactorAcute
				}
			}
			if chronic {
				cc.AquaticChronicCategory = strongerChronic(cc.AquaticChronicCategory, cat)
				if h.MFactorChronic > 0 {
					cc.MChronic = h.MFactorChronic
				}
			}
		}
	}
}

// aquaticTiersFromHCodes reports which aquatic endpoints a hazard's H-codes name.
func aquaticTiersFromHCodes(hCodes []string) (acute, chronic bool) {
	for _, code := range hCodes {
		switch normCategory(code) {
		case "H400":
			acute = true
		case "H410", "H411", "H412", "H413":
			chronic = true
		}
	}
	return acute, chronic
}

// withDedicatedFields returns c with its skin-sensitisation and aquatic fields
// derived from its Hazards list.
//
// RunEngine fills these in while building the composition, but ClassifySkinSens,
// ClassifyAquatic and ClassifyMixture are exported and documented to read the
// per-component Hazards list, so a caller that supplies only Hazards must
// classify identically to the engine. Deriving is idempotent: an already-populated
// component keeps the stronger of the two values, which is what it already holds.
func withDedicatedFields(c ClassComponent) ClassComponent {
	applyDedicatedFields(&c)
	return c
}

// parseRangeUpperPct extracts the upper-bound percentage from a supplier-declared
// concentration range string (e.g. ">= 1 - < 5%" → 5, "< 1%" → 1, "0,1-1%" → 1).
// It returns the largest number found (the conservative upper bound) and false
// when the string contains no number.
func parseRangeUpperPct(s string) (float64, bool) {
	matches := rangeNumberRe.FindAllString(s, -1)
	found := false
	var max float64
	for _, m := range matches {
		v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", "."), 64)
		if err != nil {
			continue
		}
		if !found || v > max {
			max = v
			found = true
		}
	}
	return max, found
}

// parseRangeLowerPct returns the LOWER bound (% w/w) of a supplier concentration
// range string, and whether a numeric bound was found. For a two-number range
// ("1 - 10 %") it returns the smaller number; for an upper-bounded range
// ("≤ 10 %", "< 5 %", "up to 10 %") the lower bound is 0; for a single value
// ("5 %") it returns that value. It is the companion of parseRangeUpperPct used
// to detect classification decisions that flip between a range's ends.
func parseRangeLowerPct(s string) (float64, bool) {
	matches := rangeNumberRe.FindAllString(s, -1)
	vals := make([]float64, 0, len(matches))
	for _, m := range matches {
		v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", "."), 64)
		if err != nil {
			continue
		}
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		return 0, false
	}
	if len(vals) == 1 {

		if hasUpperBoundComparator(s) {
			return 0, true
		}
		return vals[0], true
	}
	min := vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
	}
	return min, true
}

// upperBoundIsStrict reports whether a supplier concentration range uses a STRICT
// "<" upper bound — the standard CLP band edge (e.g. "≥3 - <10%", where the next
// band starts AT the stated value) — as opposed to an inclusive "≤"/"<=" bound or a
// bare closed range ("1 - 10 %"). A strict upper means the substance is strictly
// below the ceiling, which the classifier must respect at a GCL boundary: a diluted
// "<10%" worst case of exactly 3.0% does NOT meet the ≥3% Repr. 2 limit. A bare
// range with no comparator is treated as inclusive (conservative — keeps the
// worst case).
func upperBoundIsStrict(s string) bool {
	if strings.Contains(s, "≤") || strings.Contains(s, "≦") || strings.Contains(s, "<=") {
		return false
	}
	return strings.Contains(s, "<")
}

// strictBoundMargin is how far below a stated ceiling a STRICT "<" upper bound
// is carried, so that a diluted "<10 %" worst case of exactly 3.0 % does not
// meet the ≥3 % Repr. 2 limit.
//
// The size matters. It has to survive being summed with other components, so it
// must sit well above float64 accumulation noise (~1e-15 at these magnitudes)
// AND well above gclEpsilon (1e-9), the tolerance that lets a sum landing on a
// boundary still classify. 1e-6 % w/w is 1000× gclEpsilon yet chemically
// meaningless (0.01 ppm), which keeps both invariants true at once. Encoding
// strictness as a single ULP does NOT work: it is indistinguishable from the
// rounding error of adding three percentages together.
const strictBoundMargin = 1e-6

// hasUpperBoundComparator reports whether a concentration string expresses an
// upper bound with no stated lower bound ("≤ 10 %", "< 5 %", "max 10 %",
// "up to 10 %", Greek "έως"/"μέχρι"), so a single parsed number is the ceiling
// of a [0, n] range rather than an exact value.
func hasUpperBoundComparator(s string) bool {
	l := strings.ToLower(s)
	for _, sym := range []string{"≤", "≦", "<"} {
		if strings.Contains(l, sym) {
			return true
		}
	}
	for _, word := range []string{"max", "up to", "έως", "μέχρι"} {
		if containsWord(l, word) {
			return true
		}
	}
	return false
}

// containsWord reports whether word occurs in s as a standalone word rather than
// as a fragment of a longer one.
//
// The comparator words are short and extremely common inside ingredient names —
// "jasmine", "cumin", "amine" and "aluminium" all contain "min" — and a false
// positive here turns a bounded supplier range into an open-ended one, which
// carries the substance at the material's FULL percentage instead of its
// declared upper bound.
func containsWord(s, word string) bool {
	for offset := 0; offset <= len(s)-len(word); {
		idx := strings.Index(s[offset:], word)
		if idx < 0 {
			return false
		}
		start := offset + idx
		if !letterAdjacent(s, start, start+len(word)) {
			return true
		}
		offset = start + 1
	}
	return false
}

// letterAdjacent reports whether s[start:end] is flanked by a letter on either
// side, i.e. whether it sits inside a longer word.
func letterAdjacent(s string, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); unicode.IsLetter(r) {
			return true
		}
	}
	if end < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[end:]); unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// hasLowerBoundComparator reports whether s carries a lower-bound comparator
// (≥ / > / "min" / "at least" / Greek "τουλάχιστον"). The ">" token also
// matches ">=".
func hasLowerBoundComparator(s string) bool {
	l := strings.ToLower(s)
	for _, sym := range []string{"≥", "≧", ">"} {
		if strings.Contains(l, sym) {
			return true
		}
	}
	for _, word := range []string{"min", "at least", "τουλάχιστον"} {
		if containsWord(l, word) {
			return true
		}
	}
	return false
}

// lowerBoundOnly reports whether s declares only a lower bound and no upper one
// (e.g. "≥ 5 %"). Such an open-ended range can reach 100% of the raw material,
// so its apportioned worst case must be the full component %, not the stated
// number — otherwise an open-ended disclosure under-classifies.
func lowerBoundOnly(s string) bool {
	return hasLowerBoundComparator(s) && !hasUpperBoundComparator(s)
}

// rangeNumberRe matches decimal numbers (comma or dot decimal separator) inside a
// concentration-range string.
var rangeNumberRe = regexp.MustCompile(`[0-9]+(?:[.,][0-9]+)?`)

// strongerSkinSens returns the more potent of two skin-sens categories: 1A
// (most potent) outranks 1 / 1B. An empty incoming value keeps the existing one.
func strongerSkinSens(existing, incoming string) string {
	if incoming == "" {
		return existing
	}
	if existing == "1A" || incoming == "1A" {
		return "1A"
	}
	return incoming
}

// strongerChronic returns the most toxic (lowest-numbered) aquatic chronic
// category of two. Chronic 1 is most toxic; "" means none seen yet.
func strongerChronic(existing, incoming string) string {
	rank := func(c string) int {
		switch c {
		case "1":
			return 1
		case "2":
			return 2
		case "3":
			return 3
		case "4":
			return 4
		default:
			return 99
		}
	}
	if rank(incoming) < rank(existing) {
		return incoming
	}
	return existing
}

// mixtureFlashPoint derives the finished mixture's flash point for the
// flammability check + transport tree. The conservative rule for a fragrance
// mixture is the LOWEST component flash point (the most flammable component
// dominates ignitability); nil when no component reported a flash point.
func mixtureFlashPoint(lines []RecipeLine, extractions map[string]ParsedExtraction) *float64 {
	var lowest *float64
	for _, l := range lines {
		ext, ok := extractions[l.MaterialID]
		if !ok || ext.FlashPointCelsius == nil {
			continue
		}
		fp := *ext.FlashPointCelsius
		if lowest == nil || fp < *lowest {
			v := fp
			lowest = &v
		}
	}
	return lowest
}

// mixtureFlammabilityText derives the ENGINE-OWNED Section 9.1 flash-point and
// flammability text for the finished mixture from the lowest constituent flash
// point. It deliberately never echoes a single constituent's flash-point number
// as the mixture's value — a mixture flash point must be measured for the
// mixture — and instead states a determination:
//
//   - no constituent flash point known → "Not determined" (the field is blank,
//     rendered as the localised "not available");
//   - lowest constituent flash point > 60 °C → the mixture is NOT a flammable
//     liquid: flash point reported as "> 60 °C" with an explicit not-flammable note;
//   - a constituent flash point ≤ 60 °C → a flammable constituent is present, so
//     the mixture flash point must be measured before a value is stated; the
//     field is left "Not determined" and the note records the open measurement.
//
// flashPt is the lowest constituent flash point in °C (nil = none reported).
// flashPointC is language-neutral (a value/symbol); flammability text is bilingual.
func mixtureFlammabilityText(flashPt *float64) (flashPointC, flammabilityEL, flammabilityEN string) {
	switch {
	case flashPt == nil:
		return "", "Δεν έχει προσδιοριστεί για το μείγμα", "Not determined for the mixture"
	case *flashPt > flammableUpperC:

		return "> 60 °C",
			"Μη εύφλεκτο υγρό — δεν ταξινομείται ως ADR/IATA Κλάση 3 (όλα τα αναφερόμενα σημεία ανάφλεξης των συστατικών είναι άνω των 60 °C· το τελικό μείγμα δεν έχει μετρηθεί χωριστά).",
			"Not a flammable liquid — not classified as ADR/IATA Class 3 (all reported constituent flash points are above 60 °C; the finished mixture has not been separately tested)."
	default:
		return "",
			"Δεν έχει προσδιοριστεί για το μείγμα· περιέχει εύφλεκτο/-α συστατικό/-ά — απαιτείται μέτρηση του σημείου ανάφλεξης του μείγματος",
			"Not determined for the mixture; contains flammable constituent(s) — the mixture flash point must be measured"
	}
}

// productTypeHint derives a short natural product-type description from the recipe
// name to orient the compose pass + transport tree. It is a hint only (never an
// ID); an empty recipe name yields the neutral default the transport tree treats as
// a finished consumer fragrance (UN 1266 when flammable).
func productTypeHint(recipe *Recipe) string {
	return strings.TrimSpace(recipe.Name)
}

// primaryCAS / primaryEC return the first non-empty CAS / EC number across a
// file's substances (the lead substance's identity for the composition table).
func primaryCAS(ext ParsedExtraction) string {
	for _, s := range ext.Substances {
		if s.CasNumber != "" {
			return s.CasNumber
		}
	}
	return ""
}

func primaryEC(ext ParsedExtraction) string {
	for _, s := range ext.Substances {
		if s.EcNumber != "" {
			return s.EcNumber
		}
	}
	return ""
}

// isClassified reports whether the finished-mixture label asserts any hazard classification
// (a signal word, any H code, or any pictogram). A not-classified mixture has an
// empty signal word and no codes/pictograms and carries no UFI obligation.
func isClassified(label SdsLabel) bool {
	return strings.TrimSpace(label.SignalWord) != "" ||
		len(label.HCodes) > 0 ||
		len(label.Pictograms) > 0
}

// isEnvironmentalHCode reports whether an H-code denotes an environmental
// (aquatic / ozone) hazard. CLP Annex VIII treats these as the only hazards
// that do NOT, on their own, create a UFI / Poison Centre Notification
// obligation.
func isEnvironmentalHCode(h string) bool {
	switch strings.ToUpper(strings.TrimSpace(h)) {
	case "H400", "H401", "H402", "H410", "H411", "H412", "H413", "H420":
		return true
	default:
		return false
	}
}

// requiresUFI reports whether a classified mixture needs a UFI. Under CLP
// Annex VIII the UFI / Poison Centre Notification obligation arises only from a
// HEALTH or PHYSICAL hazard classification; a mixture classified solely for
// environmental hazards (aquatic / ozone) carries no UFI obligation. The label
// is authoritative: any pictogram other than the environment pictogram (GHS09),
// or any non-environmental H-code, signals a health/physical hazard.
func requiresUFI(label SdsLabel) bool {
	for _, p := range label.Pictograms {
		if strings.ToUpper(strings.TrimSpace(p)) != "GHS09" {
			return true
		}
	}
	for _, h := range label.HCodes {
		if !isEnvironmentalHCode(h) {
			return true
		}
	}
	return false
}
