package core

import (
	"fmt"
)

// method_table.go is the single, explicit source of truth mapping every CLP
// hazard class to its CLASSIFICATION METHOD and its generic concentration limits
// (GCLs). It exists so the rule "this endpoint is per-component-vs-limit, that
// one is additive/summation" is declared in one auditable place rather than
// implied by which helper a class happens to call — the root cause this refactor
// targets (a class that should be evaluated per component must never be summed,
// and vice versa).
//
// The per-component and single-threshold summation classifiers read their limits
// through gclFor/mustGCL, so the table is load-bearing, not documentation. The
// scalar values live in the named GCL consts (classify_mixture.go / classifier.go)
// which the table references, so there is exactly one place to change a number
// and the table is its structured, method-annotated view. method_table_test.go
// asserts the table stays complete and consistent with those consts.
//
// gclFor is also the designated hook for substance-specific concentration limits
// (SCLs): a harmonised SCL for a component overrides the GCL for that component
// only (see effectiveComponentLimit), never the table value globally.

// classMethod is how a hazard class decides the mixture classification from its
// components. The distinction is legally load-bearing: summing a per-component
// class (or vice versa) silently mis-classifies.
type classMethod int

const (
	// methodPerComponentGCL — each component is compared to the GCL on its own;
	// the components are NEVER summed (CMR, skin/respiratory sensitisation,
	// effects via lactation). The mixture takes the most severe component's class.
	methodPerComponentGCL classMethod = iota
	// methodSummation — the components' concentrations are added (with any
	// cross-category boost) and the sum is compared to the GCL (skin/eye
	// corrosion+irritation, STOT-SE, STOT-RE, aspiration).
	methodSummation
	// methodATE — acute toxicity via the reciprocal ATE-mix formula
	// (100 / Σ(Ci/ATEi)), per exposure route.
	methodATE
	// methodAquaticCascade — aquatic summation with M-factors and the
	// Chronic 1/2/3 cross-tier multiplier cascade.
	methodAquaticCascade
)

func (m classMethod) String() string {
	switch m {
	case methodPerComponentGCL:
		return "per-component-vs-GCL"
	case methodSummation:
		return "summation"
	case methodATE:
		return "ATE-summation"
	case methodAquaticCascade:
		return "aquatic-cascade"
	}
	return "unknown"
}

// Table-only hazardKind identities for endpoints that classifier.go owns (skin
// sensitisation, aquatic). classKind never returns these — they exist so the
// method table can be the COMPLETE map of class → method/GCL, and so the cross-
// check test ties classifier.go's consts into the same source of truth. Values
// are deliberately outside the classKind iota range.
const (
	kindSkinSens hazardKind = 100 + iota
	kindAquaticAcute
	kindAquaticChronic
)

// endpointSpec is one row of the method table: a hazard class, how it is
// classified, the additivity pool it belongs to, and its GCLs by CLP category.
// gcl is keyed by normalised category ("1", "1A", "1B", "2", "3"); the "*" key
// is a category-independent limit (lactation, aspiration, aquatic 25% threshold).
type endpointSpec struct {
	name   string
	kind   hazardKind
	method classMethod
	pool   string // additivity pool; classes sharing a pool sum together
	gcl    map[string]float64
}

// methodTable is the authoritative class → method/GCL map. Edit a GCL via its
// named const (referenced below); add a class by adding a row here.
var methodTable = []endpointSpec{
	// ── Per-component vs GCL — NEVER summed ──────────────────────────────────
	{
		name: "Skin sensitisation", kind: kindSkinSens, method: methodPerComponentGCL, pool: "skin_sens",
		gcl: map[string]float64{"1": gclSens1bH317, "1B": gclSens1bH317, "1A": gclSens1aH317},
	},
	{
		name: "Respiratory sensitisation", kind: kindRespSens, method: methodPerComponentGCL, pool: "resp_sens",
		gcl: map[string]float64{"1": gclRespSens1b, "1B": gclRespSens1b, "1A": gclRespSens1a},
	},
	{
		name: "Carcinogenicity", kind: kindCarc, method: methodPerComponentGCL, pool: "carc",
		gcl: map[string]float64{"1": gclCarc1, "1A": gclCarc1, "1B": gclCarc1, "2": gclCarc2},
	},
	{
		name: "Germ cell mutagenicity", kind: kindMuta, method: methodPerComponentGCL, pool: "muta",
		gcl: map[string]float64{"1": gclMuta1, "1A": gclMuta1, "1B": gclMuta1, "2": gclMuta2},
	},
	{
		name: "Reproductive toxicity", kind: kindRepr, method: methodPerComponentGCL, pool: "repr",
		gcl: map[string]float64{"1": gclRepr1, "1A": gclRepr1, "1B": gclRepr1, "2": gclRepr2},
	},
	{
		name: "Effects on or via lactation", kind: kindLactation, method: methodPerComponentGCL, pool: "repr",
		gcl: map[string]float64{"*": gclLact},
	},

	// ── Summation ────────────────────────────────────────────────────────────
	{
		// Sub-categories 1A/1B/1C are distinct CLP classifications of the SAME
		// class and all sum against the one Skin Corr. 1 limit (Annex I Table
		// 3.2.3). A supplier states the sub-category far more often than a bare
		// "1", so omitting them here silently drops the component from the
		// corrosion sum — and with it the Skin Irrit. 2 boost and the Eye Dam. 1
		// pool, which both read ΣSkinCorr.
		name: "Skin corrosion", kind: kindSkinCorr, method: methodSummation, pool: "skin_local",
		gcl: map[string]float64{
			"1": gclSkinCorr1, "1A": gclSkinCorr1, "1B": gclSkinCorr1, "1C": gclSkinCorr1,
		},
	},
	{
		name: "Skin irritation", kind: kindSkinIrrit, method: methodSummation, pool: "skin_local",
		gcl: map[string]float64{"2": gclSkinIrrit2},
	},
	{
		name: "Serious eye damage", kind: kindEyeDam, method: methodSummation, pool: "eye_local",
		gcl: map[string]float64{"1": gclEyeDam1},
	},
	{
		name: "Eye irritation", kind: kindEyeIrrit, method: methodSummation, pool: "eye_local",
		gcl: map[string]float64{"2": gclEyeIrrit},
	},
	{
		name: "STOT — single exposure", kind: kindStotSE, method: methodSummation, pool: "stot_se",
		gcl: map[string]float64{"1": gclStot1Hi, "2": gclStot2, "3": gclStotSe3},
	},
	{
		name: "STOT — repeated exposure", kind: kindStotRE, method: methodSummation, pool: "stot_re",
		gcl: map[string]float64{"1": gclStot1Hi, "2": gclStot2},
	},
	{
		name: "Aspiration", kind: kindAsp, method: methodSummation, pool: "asp",
		gcl: map[string]float64{"1": gclAsp1},
	},

	// ── ATE summation (acute toxicity, per route) ────────────────────────────
	{name: "Acute toxicity (oral)", kind: kindAcuteOral, method: methodATE, pool: "acute_oral"},
	{name: "Acute toxicity (dermal)", kind: kindAcuteDermal, method: methodATE, pool: "acute_dermal"},
	{name: "Acute toxicity (inhalation)", kind: kindAcuteInhal, method: methodATE, pool: "acute_inhal"},

	// ── Aquatic cascade (summation + M-factor + chronic multiplier cascade) ──
	{
		name: "Hazardous to the aquatic environment — acute", kind: kindAquaticAcute, method: methodAquaticCascade,
		pool: "aquatic",
		gcl:  map[string]float64{"*": aquaticThreshld},
	},
	{
		name: "Hazardous to the aquatic environment — chronic", kind: kindAquaticChronic, method: methodAquaticCascade,
		pool: "aquatic",
		gcl:  map[string]float64{"*": aquaticThreshld},
	},
}

// endpointByKind indexes methodTable by kind for O(1) lookup.
var endpointByKind = func() map[hazardKind]endpointSpec {
	m := make(map[hazardKind]endpointSpec, len(methodTable))
	for _, e := range methodTable {
		m[e.kind] = e
	}
	return m
}()

// gclFor returns the generic concentration limit for a kind+category, falling
// back to the category-independent "*" entry. ok is false when the kind has no
// table row or no limit for that category (e.g. an ATE endpoint, which has no %
// GCL). Category matching is normalised ("1a" → "1A").
func gclFor(kind hazardKind, category string) (float64, bool) {
	e, ok := endpointByKind[kind]
	if !ok || e.gcl == nil {
		return 0, false
	}
	if v, ok := e.gcl[normHazardCategory(category)]; ok {
		return v, true
	}
	if v, ok := e.gcl["*"]; ok {
		return v, true
	}
	return 0, false
}

// mustGCL returns the GCL for a kind+category and panics when it is absent. It is
// used by the classifiers for limits that MUST exist in the table; a panic is a
// programming error (a table row was removed) caught by method_table_test.go,
// never a runtime condition driven by data.
func mustGCL(kind hazardKind, category string) float64 {
	v, ok := gclFor(kind, category)
	if !ok {
		panic(fmt.Sprintf("clp: method table has no GCL for kind=%d category=%q", kind, category))
	}
	return v
}

// hCodeByEndpoint maps a hazard statement to the endpoint it belongs to. It is
// the inverse of the classifiers' hCodes output and exists so a substance-
// specific concentration limit can be tied to the hazard class it was set FOR
// (CLP Annex I §1.2.1). Suffixed CMR codes (H350i, H360Df, H361d …) resolve via
// the 4-character base, so only the base codes are listed.
var hCodeByEndpoint = map[string]hazardKind{
	"H300": kindAcuteOral, "H301": kindAcuteOral, "H302": kindAcuteOral, "H303": kindAcuteOral,
	"H310": kindAcuteDermal, "H311": kindAcuteDermal, "H312": kindAcuteDermal, "H313": kindAcuteDermal,
	"H330": kindAcuteInhal, "H331": kindAcuteInhal, "H332": kindAcuteInhal, "H333": kindAcuteInhal,
	"H304": kindAsp,
	"H314": kindSkinCorr, "H315": kindSkinIrrit,
	"H318": kindEyeDam, "H319": kindEyeIrrit,
	"H317": kindSkinSens, "H334": kindRespSens,
	"H340": kindMuta, "H341": kindMuta,
	"H350": kindCarc, "H351": kindCarc,
	"H360": kindRepr, "H361": kindRepr, "H362": kindLactation,
	"H370": kindStotSE, "H371": kindStotSE, "H335": kindStotSE, "H336": kindStotSE,
	"H372": kindStotRE, "H373": kindStotRE,
	"H400": kindAquaticAcute,
	"H410": kindAquaticChronic, "H411": kindAquaticChronic,
	"H412": kindAquaticChronic, "H413": kindAquaticChronic,
}

// hCodeKind resolves a hazard statement to its endpoint. known is false for a
// code the table does not cover (an EUH statement, a combined code, or a new
// ATP), which callers must treat as "cannot confirm this belongs to my
// endpoint" rather than as a match.
func hCodeKind(code string) (kind hazardKind, known bool) {
	c := normCategory(code)
	if k, ok := hCodeByEndpoint[c]; ok {
		return k, true
	}
	// H350i / H360Df / H361fd … — the differentiation suffix does not change the
	// endpoint, so fall back to the 4-character base code.
	if len(c) > 4 {
		if k, ok := hCodeByEndpoint[c[:4]]; ok {
			return k, true
		}
	}
	return 0, false
}

// methodFor returns the classification method declared for a kind.
func methodFor(kind hazardKind) (classMethod, bool) {
	e, ok := endpointByKind[kind]
	if !ok {
		return 0, false
	}
	return e.method, true
}

// effectiveComponentLimit is the limit a single component is compared against for
// a per-component-vs-GCL class: a substance-specific concentration limit (SCL)
// on its hazard entry, otherwise the generic limit from the table. SCL always
// overrides the GCL for that hazard (CLP Annex I §1.2.1).
func effectiveComponentLimit(h ComponentHazard, kind hazardKind) (
	limit float64,
	ok bool,
) {
	if h.SCLPct > 0 {
		return h.SCLPct, true
	}
	return gclFor(kind, h.Category)
}
