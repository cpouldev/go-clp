package core

import (
	"strings"
	"testing"
)

// scenario_matrix_test.go is the archetype "golden" harness. The per-function
// unit tests (classifier_test, classify_mixture_test, transport_test,
// validation_test) each lock ONE rule in isolation; this file locks the
// END-TO-END outcome (label + review flags + transport) for realistic product
// archetypes, so a change that silently regresses one rule against another —
// e.g. EUH208 surviving on an H317-classified mixture, or a pictogram-precedence
// rule firing in the wrong combination — fails here even when every isolated
// unit test still passes.
//
// Each scenario asserts ONLY the invariants it is built to lock (a zero-valued
// expectation is skipped), so the matrix stays readable and each row documents
// the rule(s) it guards. Add a scenario whenever a new code/criterion/edge is
// worth pinning across product types, not just for the recipe that surfaced it.
//
// Helpers reused (same package): hz / comp (component builders), hasStr /
// sameStrSet (slice predicates), fp / tfp (*float64), hasFlag (flag presence).

type txCase struct {
	aquaticClass  string
	productType   string
	innerLitres   *float64
	wantRegulated bool
	summaryHas    string // substring of transportSummary(result)
}

type matrixScenario struct {
	name    string
	comps   []ClassComponent
	flashPt *float64

	// Label expectations (each list is "must contain"; *Absent is "must not").
	hPresent   []string
	hAbsent    []string
	euhPresent []string
	euhAbsent  []string
	// Exact names that must / must not appear in label.Euh208Substances — used to
	// lock CLP Annex II §2.8: OTHER sub-threshold sensitisers are named (sentence 2)
	// while the classifying substance is not (sentence 3).
	euh208NamesPresent []string
	euh208NamesAbsent  []string
	pictExact          []string // when set, the pictogram set must equal this exactly
	pictPresent        []string
	pictAbsent         []string
	signal             string
	assertSig          bool

	// Review-flag expectations from validateClassification.
	flagsPresent []FlagCode
	flagsAbsent  []FlagCode

	// Transport expectations from Classify (optional).
	tx *txCase
}

func matrixScenarios() []matrixScenario {
	return []matrixScenario{
		{
			// CLP Annex II §2.8 (mandatory case): a sub-threshold skin sensitiser in
			// a mixture that is NOT itself classified → EUH208 named, no pictogram.
			name: "euh208 emitted when mixture not classified",
			comps: []ClassComponent{
				{
					Name: "Coumarin", CasNumber: "91-64-5", ConcentrationPct: 0.5, SkinSensCategory: "1B",
				},
			},
			hAbsent:    []string{"H317"},
			euhPresent: []string{"EUH208"},
			pictExact:  []string{},
			signal:     "",
			assertSig:  true,
		},
		{
			// CLP Annex II §2.8 sentence 2: when the mixture is itself Skin Sens. 1
			// (H317), an OTHER sub-threshold sensitiser ("band" — below its own H317
			// limit but in the EUH208 band) must STILL be named on the label. Only the
			// EUH208 statement for the CLASSIFYING substance is omitted (sentence 3),
			// which the band's `< limit` upper bound already enforces. (This scenario
			// guards against the regression where EUH208 was wrongly suppressed
			// entirely on any H317-classified mixture.)
			name: "euh208 names OTHER sub-threshold sensitiser even under H317",
			comps: []ClassComponent{
				{Name: "classifier", ConcentrationPct: 2, SkinSensCategory: "1B"},
				{Name: "band", ConcentrationPct: 0.5, SkinSensCategory: "1B"},
			},
			hPresent:           []string{"H317"},
			euhPresent:         []string{"EUH208"},
			euh208NamesPresent: []string{"band"},
			euh208NamesAbsent:  []string{"classifier"},
			pictPresent:        []string{"GHS07"},
			signal:             "Warning",
			assertSig:          true,
		},
		{
			// Art. 26(d): GHS08 from respiratory sensitisation removes GHS07; and the
			// H334 classification omits EUH208 (§2.5). Danger from H334.
			name: "respiratory sens GHS08 suppresses GHS07, omits EUH208",
			comps: []ClassComponent{
				{Name: "skin", ConcentrationPct: 2, SkinSensCategory: "1B"},
				comp("resp", 2, hz("Respiratory sensitisation", "1B", "H334")),
			},
			hPresent:    []string{"H317", "H334"},
			euhAbsent:   []string{"EUH208"},
			pictPresent: []string{"GHS08"},
			pictAbsent:  []string{"GHS07"},
			signal:      "Danger",
			assertSig:   true,
		},
		{
			// Flammable alcohol-based room spray: flash point 30 °C → Flam. Liq. 3
			// (H226, GHS02). Ships as ADR Class 3 — SP375 never exempts a flammable.
			name:        "flammable room spray ships Class 3",
			comps:       []ClassComponent{{Name: "fragrance", ConcentrationPct: 5, SkinSensCategory: "1B"}},
			flashPt:     fp(30),
			hPresent:    []string{"H226", "H317"},
			pictPresent: []string{"GHS02", "GHS07"},
			signal:      "Warning",
			assertSig:   true,
			tx: &txCase{
				productType: "perfumery product", innerLitres: tfp(0.1),
				wantRegulated: true, summaryHas: "Class 3",
			},
		},
		{
			// Flagship diffuser archetype — the bundle this whole investigation
			// surfaced: H317 from a classifying sensitiser, an OTHER sub-threshold
			// sensitiser that must still be NAMED via EUH208 (CLP §2.8 sentence 2),
			// a Repr. 2 component whose upper bound resolves to exactly 3.0% — an
			// inclusive/at-limit worst case that meets the 3% GCL (range-flip +
			// near-threshold flags). A strict "< 3%" upper would stay below and NOT
			// classify; see TestBuildClassComponents_StrictUpperBound. Plus an aquatic
			// Chronic 1 (M=10) → GHS09. Non-flammable (FP 75) + small pkg → SP375 clear.
			name: "non-flammable aquatic diffuser: EUH208 names other sensitiser, range+near flags, SP375",
			comps: []ClassComponent{
				{Name: "Skin sensitiser", ConcentrationPct: 2, SkinSensCategory: "1B"},
				{Name: "Sub-threshold allergen", ConcentrationPct: 0.5, SkinSensCategory: "1B"},
				{
					Name: "Iso bornyl cyclohexanol", ConcentrationPct: 3, ConcentrationLowPct: 1, FromRange: true,
					Hazards: []ComponentHazard{hz("Reproductive toxicity", "2", "H361")},
				},
				{Name: "Aquatic toxin", ConcentrationPct: 5, AquaticChronicCategory: "1", MChronic: 10},
			},
			flashPt:            fp(75),
			hPresent:           []string{"H317", "H361", "H410"},
			euhPresent:         []string{"EUH208"},
			euh208NamesPresent: []string{"Sub-threshold allergen"},
			euh208NamesAbsent:  []string{"Skin sensitiser"},
			pictPresent:        []string{"GHS07", "GHS08", "GHS09"},
			signal:             "Warning",
			assertSig:          true,
			flagsPresent:       []FlagCode{FlagRangeBased, FlagNearThreshold},
			tx: &txCase{
				aquaticClass: "Aquatic Chronic 1", productType: "perfumery product", innerLitres: tfp(0.1),
				wantRegulated: false, summaryHas: "Not regulated",
			},
		},
		{
			// Aquatic summation cascade: two Aquatic Chronic 2 at 13% sum to 26% ≥ 25%
			// → mixture Aquatic Chronic 2 (H411) with NO GHS09 pictogram and NO signal.
			name: "aquatic cascade to Chronic 2 carries no pictogram or signal",
			comps: []ClassComponent{
				{Name: "a", ConcentrationPct: 13, AquaticChronicCategory: "2"},
				{Name: "b", ConcentrationPct: 13, AquaticChronicCategory: "2"},
			},
			hPresent:  []string{"H411"},
			pictExact: []string{},
			signal:    "",
			assertSig: true,
		},
		{
			// Undisclosed-remainder guard: only 15% w/w of the mixture is disclosed,
			// so the large-remainder review flag must fire (the operator must confirm
			// the base is accounted for).
			name:         "large undisclosed remainder raises the review flag",
			comps:        []ClassComponent{{Name: "only disclosed", ConcentrationPct: 15, SkinSensCategory: "1B"}},
			flagsPresent: []FlagCode{FlagUndisclosedRemainder},
		},
		{
			// Repr. 1B at 0.5% ≥ 0.3% GCL → H360 (GHS08) and signal Danger — the
			// heavier reproductive-hazard label, distinct from the Repr. 2 / Warning
			// case in the flagship scenario.
			name:        "Repr. 1 drives H360 + Danger",
			comps:       []ClassComponent{comp("repr1", 0.5, hz("Reproductive toxicity", "1B", "H360"))},
			hPresent:    []string{"H360"},
			pictPresent: []string{"GHS08"},
			signal:      "Danger",
			assertSig:   true,
		},
		{
			// Additive skin + eye irritation (Tables 3.2.3 / 3.3.3): two Irrit. 2
			// components at 6% each sum to 12% ≥ 10% on both endpoints → H315 + H319
			// (GHS07, Warning). No single component reaches the limit alone — the
			// summation rule is what classifies.
			name: "additive irritation summation → H315 + H319",
			comps: []ClassComponent{
				comp("irrA", 6, hz("Skin Irrit. 2", "2", "H315"), hz("Eye Irrit. 2", "2", "H319")),
				comp("irrB", 6, hz("Skin Irrit. 2", "2", "H315"), hz("Eye Irrit. 2", "2", "H319")),
			},
			hPresent:    []string{"H315", "H319"},
			pictPresent: []string{"GHS07"},
			signal:      "Warning",
			assertSig:   true,
		},
		{
			// Art. 26(a): acute toxicity GHS06 unconditionally removes GHS07 (here the
			// GHS07 would stand for skin sensitisation). Both H-codes stay on the
			// label; only the pictogram is dropped. Signal Danger (Acute Tox. 2).
			name: "acute tox GHS06 suppresses GHS07",
			comps: []ClassComponent{
				comp("tox", 50, hz("Acute Tox. 2 (oral)", "2", "H300")),
				{Name: "sens", ConcentrationPct: 5, SkinSensCategory: "1B"},
			},
			hPresent:    []string{"H300", "H317"},
			pictPresent: []string{"GHS06"},
			pictAbsent:  []string{"GHS07"},
			signal:      "Danger",
			assertSig:   true,
		},
		{
			// STOT SE 3 (Table 3.8.3): a single-exposure respiratory-irritant at 25% ≥
			// the 20% GCL → H335 (GHS07, Warning) — a per-component-limit class with no
			// additivity, distinct from the irritation summation above.
			name: "STOT SE 3 → H335",
			comps: []ClassComponent{
				comp(
					"stot",
					25,
					hz("Specific target organ toxicity (single exposure)", "3", "H335"),
				),
			},
			hPresent:    []string{"H335"},
			pictPresent: []string{"GHS07"},
			signal:      "Warning",
			assertSig:   true,
		},
	}
}

func TestScenarioMatrix(t *testing.T) {
	for _, sc := range matrixScenarios() {
		t.Run(
			sc.name, func(t *testing.T) {
				mix := ClassifyMixture(sc.comps, sc.flashPt, false)
				lab := mix.Label

				for _, h := range sc.hPresent {
					if !hasStr(lab.HCodes, h) {
						t.Errorf("H-codes %v missing %s", lab.HCodes, h)
					}
				}
				for _, h := range sc.hAbsent {
					if hasStr(lab.HCodes, h) {
						t.Errorf("H-codes %v must NOT contain %s", lab.HCodes, h)
					}
				}
				for _, e := range sc.euhPresent {
					if !hasStr(lab.EuhCodes, e) {
						t.Errorf("EUH codes %v missing %s", lab.EuhCodes, e)
					}
				}
				for _, e := range sc.euhAbsent {
					if hasStr(lab.EuhCodes, e) {
						t.Errorf("EUH codes %v must NOT contain %s", lab.EuhCodes, e)
					}
				}
				for _, n := range sc.euh208NamesPresent {
					if !hasStr(lab.Euh208Substances, n) {
						t.Errorf("Euh208Substances %v missing %q", lab.Euh208Substances, n)
					}
				}
				for _, n := range sc.euh208NamesAbsent {
					if hasStr(lab.Euh208Substances, n) {
						t.Errorf("Euh208Substances %v must NOT contain %q", lab.Euh208Substances, n)
					}
				}
				if sc.pictExact != nil && !sameStrSet(lab.Pictograms, sc.pictExact) {
					t.Errorf("pictograms = %v, want exactly %v", lab.Pictograms, sc.pictExact)
				}
				for _, p := range sc.pictPresent {
					if !hasStr(lab.Pictograms, p) {
						t.Errorf("pictograms %v missing %s", lab.Pictograms, p)
					}
				}
				for _, p := range sc.pictAbsent {
					if hasStr(lab.Pictograms, p) {
						t.Errorf("pictograms %v must NOT contain %s", lab.Pictograms, p)
					}
				}
				if sc.assertSig && lab.SignalWord != sc.signal {
					t.Errorf("signal word = %q, want %q", lab.SignalWord, sc.signal)
				}

				if len(sc.flagsPresent) > 0 || len(sc.flagsAbsent) > 0 {
					flags := validateClassification(sc.comps, sc.flashPt, false)
					for _, code := range sc.flagsPresent {
						if !hasFlag(flags, code) {
							t.Errorf("expected review flag %q to fire; got %+v", code, flags)
						}
					}
					for _, code := range sc.flagsAbsent {
						if hasFlag(flags, code) {
							t.Errorf("review flag %q must NOT fire", code)
						}
					}
				}

				if sc.tx != nil {
					res, _ := Classify(sc.flashPt, sc.tx.aquaticClass, sc.tx.productType, sc.tx.innerLitres)
					if res.Regulated != sc.tx.wantRegulated {
						t.Errorf("transport regulated = %v, want %v", res.Regulated, sc.tx.wantRegulated)
					}
					if got := transportSummary(res); sc.tx.summaryHas != "" && !strings.Contains(
						got,
						sc.tx.summaryHas,
					) {
						t.Errorf("transport summary %q missing %q", got, sc.tx.summaryHas)
					}
				}
			},
		)
	}
}
