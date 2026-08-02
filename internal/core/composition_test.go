package core

import (
	"math"
	"strings"
	"testing"
)

func TestComputeComposition_PercentagesSumToTotal(t *testing.T) {
	lines := []RecipeLine{
		{Quantity: 25, MaterialName: "Alcohol", MaterialCode: "ALC", Unit: "g"},
		{Quantity: 70, MaterialName: "Water", MaterialCode: "H2O", Unit: "g"},
		{Quantity: 5, MaterialName: "Fragrance", MaterialCode: "FRG", Unit: "g"},
	}

	comps, flags := computeComposition(lines)

	if len(flags) != 0 {
		t.Fatalf("expected no flags for a unit-consistent recipe, got %+v", flags)
	}
	if len(comps) != 3 {
		t.Fatalf("expected 3 components, got %d", len(comps))
	}

	want := map[string]float64{"Alcohol": 25, "Water": 70, "Fragrance": 5}
	sum := 0.0
	for _, c := range comps {
		if !approxEq(c.Pct, want[c.RawMaterialName]) {
			t.Errorf("%s: pct = %v, want %v", c.RawMaterialName, c.Pct, want[c.RawMaterialName])
		}

		if c.Code == "" || c.QtyUnit != "g" {
			t.Errorf("%s: audit fields not carried: %+v", c.RawMaterialName, c)
		}
		sum += c.Pct
	}
	if !approxEq(sum, 100) {
		t.Errorf("percentages should sum to 100, got %v", sum)
	}
}

func TestComputeComposition_NonHundredTotal(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 10, MaterialName: "A", Unit: "ml"},
		{Quantity: 30, MaterialName: "B", Unit: "ml"},
	}

	comps, flags := computeComposition(lines)
	if len(flags) != 0 {
		t.Fatalf("unexpected flags: %+v", flags)
	}
	if !approxEq(comps[0].Pct, 25) {
		t.Errorf("A: pct = %v, want 25", comps[0].Pct)
	}
	if !approxEq(comps[1].Pct, 75) {
		t.Errorf("B: pct = %v, want 75", comps[1].Pct)
	}
}

func TestComputeComposition_ZeroQtyComponentSafe(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 100, MaterialName: "Base", Unit: "g"},
		{Quantity: 0, MaterialName: "Trace", Unit: "g"},
	}

	comps, flags := computeComposition(lines)
	if len(flags) != 0 {
		t.Fatalf("unexpected flags for zero-qty line: %+v", flags)
	}
	if !approxEq(comps[0].Pct, 100) {
		t.Errorf("Base: pct = %v, want 100", comps[0].Pct)
	}
	if !approxEq(comps[1].Pct, 0) {
		t.Errorf("Trace: pct = %v, want 0", comps[1].Pct)
	}
}

func TestComputeComposition_AllZeroTotalNoDivideByZero(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 0, MaterialName: "A", Unit: "g"},
		{Quantity: 0, MaterialName: "B", Unit: "g"},
	}

	comps, _ := computeComposition(lines)
	for _, c := range comps {
		if c.Pct != 0 {
			t.Errorf("%s: pct = %v, want 0 for zero-total recipe", c.RawMaterialName, c.Pct)
		}
	}
}

func TestComputeComposition_EmptyRecipe(t *testing.T) {
	comps, flags := computeComposition(nil)
	if len(comps) != 0 {
		t.Errorf("expected no components for empty recipe, got %d", len(comps))
	}
	if len(flags) != 0 {
		t.Errorf("expected no flags for empty recipe, got %+v", flags)
	}
}

func TestComputeComposition_MixedUnitsRaisesFlag(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 50, MaterialName: "Solid", Unit: "g"},
		{Quantity: 50, MaterialName: "Liquid", Unit: "ml"},
	}

	comps, flags := computeComposition(lines)

	if !hasFlag(flags, FlagUnitInconsistent) {
		t.Fatalf("expected unit_inconsistent flag for mixed mass+volume, got %+v", flags)
	}
	for _, c := range comps {
		if c.Pct != 0 {
			t.Errorf("%s: pct should be 0 (not silently-wrong) under mixed units, got %v", c.RawMaterialName, c.Pct)
		}
	}
}

func TestComputeComposition_SameFamilyDifferentUnitsConsistent(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 1, MaterialName: "A", Unit: "kg"},
		{Quantity: 1, MaterialName: "B", Unit: "g"},
	}

	_, flags := computeComposition(lines)
	if hasFlag(flags, FlagUnitInconsistent) {
		t.Errorf("g + kg are both mass; should not flag unit_inconsistent: %+v", flags)
	}
}

func TestComputeComposition_EmptyUnitDoesNotFlag(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 50, MaterialName: "A", Unit: "g"},
		{Quantity: 50, MaterialName: "B", Unit: ""},
	}

	_, flags := computeComposition(lines)
	if hasFlag(flags, FlagUnitInconsistent) {
		t.Errorf("empty unit should not trigger unit_inconsistent: %+v", flags)
	}
}

func TestComputeComposition_ExcludesArticlesAndRebaselines(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 7, MaterialName: "CM3000 base", MaterialCode: "17", Unit: "GRAM"},
		{Quantity: 3, MaterialName: "BOOKSTORE compound", MaterialCode: "351", Unit: "GRAM"},
		{Quantity: 1, MaterialName: "Card", MaterialCode: "414", Unit: "PIECE"},
		{Quantity: 1, MaterialName: "Label", MaterialCode: "415", Unit: "PIECE"},
		{Quantity: 1, MaterialName: "Bottle", MaterialCode: "422", Unit: "PIECE"},
	}

	comps, flags := computeComposition(lines)

	if len(comps) != 2 {
		t.Fatalf("expected 2 chemical components (articles excluded), got %d: %+v", len(comps), comps)
	}
	want := map[string]float64{"CM3000 base": 70, "BOOKSTORE compound": 30}
	for _, c := range comps {
		if _, ok := want[c.RawMaterialName]; !ok {
			t.Errorf("unexpected component %q — an article should have been excluded", c.RawMaterialName)
		}
		if !approxEq(c.Pct, want[c.RawMaterialName]) {
			t.Errorf("%s: pct = %v, want %v", c.RawMaterialName, c.Pct, want[c.RawMaterialName])
		}
	}
	if !hasFlag(flags, FlagArticleExcluded) {
		t.Errorf("expected article_excluded info flag, got %+v", flags)
	}
	if hasFlag(flags, FlagUnitInconsistent) {
		t.Errorf("all chemical lines are mass; must not flag unit_inconsistent: %+v", flags)
	}
}

func TestComputeComposition_EnumUnitWordsRecognised(t *testing.T) {

	mixed := []RecipeLine{
		{Quantity: 50, MaterialName: "Solid", Unit: "GRAM"},
		{Quantity: 50, MaterialName: "Liquid", Unit: "MILLILITRE"},
	}
	if _, flags := computeComposition(mixed); !hasFlag(flags, FlagUnitInconsistent) {
		t.Fatalf("GRAM + MILLILITRE are mass + volume → expected unit_inconsistent, got %+v", flags)
	}

	massOnly := []RecipeLine{
		{Quantity: 3, MaterialName: "A", Unit: "GRAM"},
		{Quantity: 1, MaterialName: "B", Unit: "KILOGRAM"},
	}
	comps, flags := computeComposition(massOnly)
	if hasFlag(flags, FlagUnitInconsistent) {
		t.Errorf("GRAM + KILOGRAM are both mass; must not flag unit_inconsistent: %+v", flags)
	}
	if len(comps) != 2 {
		t.Fatalf("expected 2 components, got %d", len(comps))
	}
}

func TestUnitFamily(t *testing.T) {
	cases := map[string]string{
		"g": "mass", "kg": "mass", "mg": "mass", "GR": "mass",
		"GRAM": "mass", "KILOGRAM": "mass",
		"ml": "volume", "L": "volume", "cl": "volume", " lt ": "volume",
		"MILLILITRE": "volume", "LITRE": "volume",
		"pcs": "article", "PIECE": "article", "METER": "article", "CENTIMETER": "article",
		"": "", "drop": "",
	}
	for in, want := range cases {
		if got := unitFamily(in); got != want {
			t.Errorf("unitFamily(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsClassified(t *testing.T) {
	if isClassified(SdsLabel{}) {
		t.Error("empty label should be not-classified")
	}
	if !isClassified(SdsLabel{SignalWord: "Warning"}) {
		t.Error("a signal word means classified")
	}
	if !isClassified(SdsLabel{HCodes: []string{"H317"}}) {
		t.Error("an H code means classified")
	}
	if !isClassified(SdsLabel{Pictograms: []string{"GHS07"}}) {
		t.Error("a pictogram means classified")
	}
}

func TestRequiresUFI(t *testing.T) {
	cases := []struct {
		name  string
		label SdsLabel
		want  bool
	}{
		{"not classified", SdsLabel{}, false},
		{
			"environmental-only Chronic 1 (GHS09 + H410)",
			SdsLabel{Pictograms: []string{"GHS09"}, HCodes: []string{"H410"}, SignalWord: "Warning"}, false,
		},
		{
			"environmental-only Chronic 2 (GHS09 + H411)",
			SdsLabel{Pictograms: []string{"GHS09"}, HCodes: []string{"H411"}, SignalWord: "Warning"}, false,
		},
		{
			"environmental-only Chronic 3 (no pictogram, H412)",
			SdsLabel{HCodes: []string{"H412"}, SignalWord: "Warning"}, false,
		},
		{
			"health: skin sens (GHS07 + H317)",
			SdsLabel{Pictograms: []string{"GHS07"}, HCodes: []string{"H317"}, SignalWord: "Warning"}, true,
		},
		{
			"physical: flammable (GHS02 + H225)",
			SdsLabel{Pictograms: []string{"GHS02"}, HCodes: []string{"H225"}, SignalWord: "Danger"}, true,
		},
		{
			"mixed health + environmental",
			SdsLabel{
				Pictograms: []string{"GHS07", "GHS09"}, HCodes: []string{"H317", "H410"}, SignalWord: "Warning",
			},
			true,
		},
	}
	for _, c := range cases {
		if got := requiresUFI(c.label); got != c.want {
			t.Errorf("%s: requiresUFI = %t, want %t", c.name, got, c.want)
		}
	}
}

func TestGenerateUFIUsesIssuerCountry(t *testing.T) {
	got, flags := generateUFI(&Recipe{FormulationNumber: 123456}, "FR", "AB123456789", true)
	if len(flags) != 0 {
		t.Fatalf("generateUFI returned flags: %#v", flags)
	}
	if got != "27W0-KC81-E00T-V314" {
		t.Fatalf("generateUFI = %q, want official ECHA vector", got)
	}
}

func TestMixtureFlashPoint_LowestWins(t *testing.T) {
	lines := []RecipeLine{
		{MaterialID: "rm-1", MaterialName: "A"},
		{MaterialID: "rm-2", MaterialName: "B"},
		{MaterialID: "rm-3", MaterialName: "C"},
	}
	extractions := map[string]ParsedExtraction{
		"rm-1": {FlashPointCelsius: fp(45)},
		"rm-2": {FlashPointCelsius: fp(22)},
		"rm-3": {},
	}

	got := mixtureFlashPoint(lines, extractions)
	if got == nil {
		t.Fatal("expected a flash point, got nil")
	}
	if *got != 22 {
		t.Errorf("mixture flash point = %v, want 22 (lowest component)", *got)
	}
}

func TestMixtureFlashPoint_NoneKnown(t *testing.T) {
	lines := []RecipeLine{{MaterialID: "rm-1"}}
	extractions := map[string]ParsedExtraction{"rm-1": {}}

	if got := mixtureFlashPoint(lines, extractions); got != nil {
		t.Errorf("expected nil when no component reports a flash point, got %v", *got)
	}
}

// TestMixtureFlammabilityText_AboveCeilingIsDeterministic pins the §9 wording for
// a non-flammable mixture (flash point > 60 °C): it states plainly that the
// product is not a flammable liquid / not ADR Class 3 and that all reported
// constituent flash points are above 60 °C, but it must NOT cite a single
// constituent value — that number flips between generations when extraction
// captures a different component's flash point (the v4→v5 70 °C→91 °C drift),
// even though the formulation and the not-flammable conclusion are unchanged.
func TestMixtureFlammabilityText_AboveCeilingIsDeterministic(t *testing.T) {
	flashPointC, el, en := mixtureFlammabilityText(fp(70))
	if flashPointC != "> 60 °C" {
		t.Errorf("flashPointC = %q, want %q", flashPointC, "> 60 °C")
	}
	for _, c := range []struct{ lang, got, want string }{
		{"EN", en, "not classified as ADR/IATA Class 3"},
		{"EN", en, "above 60 °C"},
		{"EL", el, "Κλάση 3"},
		{"EL", el, "60 °C"},
	} {
		if !strings.Contains(c.got, c.want) {
			t.Errorf("%s flammability text %q missing %q", c.lang, c.got, c.want)
		}
	}

	for _, v := range []string{"70 °C", "91 °C"} {
		if strings.Contains(en, v) || strings.Contains(el, v) {
			t.Errorf("flammability text must not cite a specific constituent flash point %q (non-deterministic)", v)
		}
	}

	_, el91, en91 := mixtureFlammabilityText(fp(91))
	if en91 != en || el91 != el {
		t.Errorf("flammability text must be identical for any FP > 60 °C; fp70 EN=%q vs fp91 EN=%q", en, en91)
	}
}

func TestMixtureFlammabilityText_NilUndetermined(t *testing.T) {
	flashPointC, el, en := mixtureFlammabilityText(nil)
	if flashPointC != "" {
		t.Errorf("flashPointC = %q, want empty when undetermined", flashPointC)
	}
	if !strings.Contains(en, "Not determined") || el == "" {
		t.Errorf("nil flash point should read as not determined; got EL=%q EN=%q", el, en)
	}
}

// TestBuildClassComponents_OverrideCollapsesRange verifies that pinning a
// substance's real finished-product % replaces the supplier-range worst case, so
// a range-driven Repr. 2 over-classification drops when the lower real figure is
// supplied — and the range-flip review flag stops firing.
//
// This fixture models a supplier reporting an *inclusive* upper bound (≤ 10 %), so
// the diluted worst case lands exactly on the 3% GCL and Repr. 2 genuinely
// triggers. The real iso bornyl cyclohexanol case (strict < 10 %, which the
// dilution math alone keeps below 3% — no override needed) is covered separately
// by TestBuildClassComponents_StrictUpperBound.
func TestBuildClassComponents_OverrideCollapsesRange(t *testing.T) {
	const cas = "3407-42-9"
	composition := []Component{{MaterialID: "rm1", RawMaterialName: "Fragrance", Pct: 30}}
	extractions := map[string]ParsedExtraction{
		"rm1": {
			RawMaterialName: "Fragrance",
			Substances: []ParsedSubstance{
				{
					Name:               "Iso bornyl cyclohexanol",
					CasNumber:          cas,
					ConcentrationRange: "≥ 3 - ≤ 10 %",
					HCodes:             []string{"H361"},
					Hazards:            []ParsedHazard{{Class: "Reproductive toxicity", Category: "2"}},
				},
			},
		},
	}

	comps, _ := buildClassComponents(composition, extractions, nil)
	if len(comps) != 1 {
		t.Fatalf("want 1 component, got %d", len(comps))
	}
	if got := comps[0].ConcentrationPct; got != 3.0 {
		t.Errorf("worst-case ConcentrationPct = %v, want 3.0 (30%% × 10%%)", got)
	}
	if !comps[0].FromRange {
		t.Error("worst-case component should be FromRange")
	}
	if mix := ClassifyMixture(comps, nil, false); !hasStr(mix.Label.HCodes, "H361") {
		t.Errorf("worst case must classify Repr. 2 (H361), got %v", mix.Label.HCodes)
	}
	if !hasFlag(validateClassification(comps, nil, false), FlagRangeBased) {
		t.Error("worst case (range straddling the GCL) must raise the range-flip flag")
	}

	overrides := map[string]float64{cas: 1.5}
	comps2, _ := buildClassComponents(composition, extractions, overrides)
	if got := comps2[0].ConcentrationPct; got != 1.5 {
		t.Errorf("overridden ConcentrationPct = %v, want 1.5", got)
	}
	if comps2[0].FromRange {
		t.Error("overridden component must NOT be FromRange (concentration is now exact)")
	}
	if mix := ClassifyMixture(comps2, nil, false); hasStr(mix.Label.HCodes, "H361") {
		t.Errorf("override < 3%% must drop Repr. 2 (H361), got %v", mix.Label.HCodes)
	}
	if hasFlag(validateClassification(comps2, nil, false), FlagRangeBased) {
		t.Error("overridden (exact) concentration must not raise the range-flip flag")
	}
}

func TestLookupOverride_CASThenName(t *testing.T) {
	ov := map[string]float64{"100-79-8": 2.0, "iso bornyl cyclohexanol": 1.5}
	for _, tc := range []struct {
		cas, name string
		want      float64
		wantOK    bool
	}{
		{"100-79-8", "Solketal", 2.0, true},
		{"100-79-8", "", 2.0, true},
		{"", "Iso Bornyl Cyclohexanol", 1.5, true},
		{"999-99-9", "Unknown", 0, false},
	} {
		got, ok := lookupOverride(ov, tc.cas, tc.name)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("lookupOverride(%q,%q) = %v,%v; want %v,%v", tc.cas, tc.name, got, ok, tc.want, tc.wantOK)
		}
	}
	if _, ok := lookupOverride(nil, "x", "y"); ok {
		t.Error("nil overrides must return false")
	}
}

// TestBuildClassComponents_StrictUpperBound verifies the engine respects a STRICT
// "<X" supplier upper bound: iso bornyl cyclohexanol disclosed "≥3 - <10%" in a
// fragrance blended at 30% is strictly < 3.0% in the finished product (not exactly
// 3.0%), so it does NOT meet the ≥3% Repr. 2 GCL — H361 must not classify. An
// INCLUSIVE "≤10%" bound can reach 3.0% and does classify. This is the real
// diffuser case (Vioryl BOOKSTORE concentrate × 30% blend).
func TestBuildClassComponents_StrictUpperBound(t *testing.T) {
	composition := []Component{{MaterialID: "rm1", RawMaterialName: "Fragrance", Pct: 30}}
	mk := func(rangeStr string) []ClassComponent {
		ext := map[string]ParsedExtraction{
			"rm1": {
				RawMaterialName: "Fragrance",
				Substances: []ParsedSubstance{
					{
						Name:               "Iso bornyl cyclohexanol",
						CasNumber:          "3407-42-9",
						ConcentrationRange: rangeStr,
						HCodes:             []string{"H361"},
						Hazards:            []ParsedHazard{{Class: "Reproductive toxicity", Category: "2"}},
					},
				},
			},
		}
		comps, _ := buildClassComponents(composition, ext, nil)
		return comps
	}

	strict := mk("≥ 3 - < 10 %")
	if got := strict[0].ConcentrationPct; !(got < 3.0) {
		t.Errorf("strict upper: ConcentrationPct = %v, want strictly < 3.0", got)
	}
	if mix := ClassifyMixture(strict, nil, false); hasStr(mix.Label.HCodes, "H361") {
		t.Errorf("strict <10%% × 30%% = <3.0%% must NOT classify Repr. 2 (H361); got %v", mix.Label.HCodes)
	}

	incl := mk("≥ 3 - ≤ 10 %")
	if got := incl[0].ConcentrationPct; got != 3.0 {
		t.Errorf("inclusive upper: ConcentrationPct = %v, want 3.0", got)
	}
	if mix := ClassifyMixture(incl, nil, false); !hasStr(mix.Label.HCodes, "H361") {
		t.Errorf("inclusive ≤10%% × 30%% = 3.0%% must classify Repr. 2 (H361); got %v", mix.Label.HCodes)
	}
}

func TestUpperBoundIsStrict(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{
		{"≥ 3 - < 10 %", true},
		{"< 5 %", true},
		{"≥ 3 - ≤ 10 %", false},
		{"<= 10 %", false},
		{"1 - 10 %", false},
		{"5 %", false},
	} {
		if got := upperBoundIsStrict(tc.s); got != tc.want {
			t.Errorf("upperBoundIsStrict(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestLowerBoundOnly(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{
		{"≥ 5 %", true},
		{"> 5 %", true},
		{">= 5 %", true},
		{"min 5 %", true},
		{"at least 5 %", true},
		{"≥ 1 - < 5 %", false},
		{"≥ 1 - ≤ 5 %", false},
		{"≤ 5 %", false},
		{"< 5 %", false},
		{"1 - 5 %", false},
		{"5 %", false},
		{"", false},
	} {
		if got := lowerBoundOnly(tc.s); got != tc.want {
			t.Errorf("lowerBoundOnly(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// TestBuildClassComponents_OpenEndedLowerBound verifies that a substance disclosed
// only as "≥ X%" (no upper bound) is apportioned at the FULL component % for its
// worst case, not at X% — because it can legally sit anywhere up to 100% of the
// raw material. Pinning it at X under-classified: a 3% GCL hazard would be missed
// at the low figure but correctly triggers at the true worst case.
func TestBuildClassComponents_OpenEndedLowerBound(t *testing.T) {
	composition := []Component{{MaterialID: "rm1", RawMaterialName: "Fragrance", Pct: 30}}
	extractions := map[string]ParsedExtraction{
		"rm1": {
			RawMaterialName: "Fragrance",
			Substances: []ParsedSubstance{
				{
					Name:               "Open-ended repr substance",
					CasNumber:          "111-11-1",
					ConcentrationRange: "≥ 5 %",
					HCodes:             []string{"H361"},
					Hazards:            []ParsedHazard{{Class: "Reproductive toxicity", Category: "2"}},
				},
			},
		},
	}

	comps, _ := buildClassComponents(composition, extractions, nil)
	if len(comps) != 1 {
		t.Fatalf("want 1 component, got %d", len(comps))
	}
	if got := comps[0].ConcentrationPct; got != 30 {
		t.Errorf("open-ended worst case ConcentrationPct = %v, want 30 (full component)", got)
	}
	if got := comps[0].ConcentrationLowPct; got != 1.5 {
		t.Errorf("open-ended ConcentrationLowPct = %v, want 1.5 (5%% × 30%%)", got)
	}
	if !comps[0].FromRange {
		t.Error("open-ended range must be FromRange")
	}
	if mix := ClassifyMixture(comps, nil, false); !hasStr(mix.Label.HCodes, "H361") {
		t.Errorf("open-ended worst case (30%% ≥ 3%% GCL) must classify Repr. 2 (H361); got %v", mix.Label.HCodes)
	}

	boundedExt := map[string]ParsedExtraction{
		"rm1": {
			RawMaterialName: "Fragrance",
			Substances: []ParsedSubstance{
				{
					Name:               "Open-ended repr substance",
					CasNumber:          "111-11-1",
					ConcentrationRange: "≥ 5 - < 10 %",
					HCodes:             []string{"H361"},
					Hazards:            []ParsedHazard{{Class: "Reproductive toxicity", Category: "2"}},
				},
			},
		},
	}
	bounded, _ := buildClassComponents(composition, boundedExt, nil)
	if got := bounded[0].ConcentrationPct; !(got < 3.0) {
		t.Errorf("bounded strict upper: ConcentrationPct = %v, want strictly < 3.0 (not full component)", got)
	}
}

// A CHEMICAL with no supplier SDS is a blocking gap, not a notice. It stays in
// the physical product but leaves the classification, and computeComposition
// then re-baselines every other component's % w/w over a smaller total — so the
// sheet is not merely incomplete, it actively understates the concentrations it
// does report. That must not be signed off as an advisory.
func TestExcludeUnmappedLines_ChemicalWithoutSdsBlocks(t *testing.T) {
	lines := []RecipeLine{
		{MaterialID: "rm-1", MaterialName: "Alcohol", MaterialCode: "100", Unit: "gram"},
		{MaterialID: "rm-2", MaterialName: "Mystery", MaterialCode: "200", Unit: "gram"},
	}

	sdsRows := []SupplierSDS{
		{MaterialID: "rm-1"},
	}

	mapped, flags := excludeUnmappedLines(lines, sdsRows)

	if len(mapped) != 1 || mapped[0].MaterialID != "rm-1" {
		t.Fatalf("expected only rm-1 to survive, got %+v", mapped)
	}
	if len(flags) != 1 {
		t.Fatalf("expected exactly 1 sds_unmapped flag, got %d (%+v)", len(flags), flags)
	}
	f := flags[0]
	if f.Code != FlagSdsUnmapped || f.Severity != SeverityBlock {
		t.Errorf("expected BLOCKING sds_unmapped for a chemical, got code=%s severity=%s", f.Code, f.Severity)
	}

	if !strings.Contains(f.Message, "Mystery") || !strings.Contains(f.Message, "200") {
		t.Errorf("flag message should name the excluded material, got %q", f.Message)
	}
}

// An ARTICLE (bottle, card, label) is not part of the mixture an SDS describes,
// so no supplier SDS is expected for it and its absence must stay advisory —
// otherwise every recipe blocks on its own packaging and the severity carries no
// signal. A recipe missing BOTH kinds must raise them as two separate flags, so
// a real chemical gap is never buried in a list of stickers.
func TestExcludeUnmappedLines_ArticleStaysAdvisoryAndIsReportedSeparately(t *testing.T) {
	lines := []RecipeLine{
		{MaterialID: "rm-1", MaterialName: "Base", MaterialCode: "1", Unit: "gram"},
		{MaterialID: "rm-2", MaterialName: "Fragrance", MaterialCode: "424", Unit: "gram"},
		{MaterialID: "rm-3", MaterialName: "Bottle", MaterialCode: "262", Unit: "piece"},
		{MaterialID: "rm-4", MaterialName: "Sticker", MaterialCode: "250", Unit: "piece"},
	}
	sdsRows := []SupplierSDS{{MaterialID: "rm-1"}}

	_, flags := excludeUnmappedLines(lines, sdsRows)

	if len(flags) != 2 {
		t.Fatalf("expected the chemical gap and the article gap as separate flags, got %d (%+v)", len(flags), flags)
	}

	var block, info *Flag
	for i := range flags {
		switch flags[i].Severity {
		case SeverityBlock:
			block = &flags[i]
		case SeverityInfo:
			info = &flags[i]
		}
	}
	if block == nil || info == nil {
		t.Fatalf("expected one block and one info flag, got %+v", flags)
	}

	if !strings.Contains(block.Message, "Fragrance") {
		t.Errorf("blocking flag must name the chemical, got %q", block.Message)
	}
	if strings.Contains(block.Message, "Bottle") || strings.Contains(block.Message, "Sticker") {
		t.Errorf("blocking flag must not list articles, got %q", block.Message)
	}
	if !strings.Contains(info.Message, "Bottle") || !strings.Contains(info.Message, "Sticker") {
		t.Errorf("advisory flag should name the articles, got %q", info.Message)
	}
}

func TestExcludeUnmappedLines_AllMappedKeepsLinesNoFlag(t *testing.T) {
	lines := []RecipeLine{
		{MaterialID: "rm-1", MaterialName: "A", MaterialCode: "1"},
		{MaterialID: "rm-2", MaterialName: "B", MaterialCode: "2"},

		{MaterialID: "", MaterialName: "Sub-item", MaterialCode: ""},
	}
	sdsRows := []SupplierSDS{
		{MaterialID: "rm-1"},
		{MaterialID: "rm-2"},
	}

	mapped, flags := excludeUnmappedLines(lines, sdsRows)
	if len(flags) != 0 {
		t.Errorf("every raw material mapped should yield no flags, got %+v", flags)
	}

	if len(mapped) != 3 {
		t.Errorf("expected all 3 lines kept (2 mapped + 1 item-set), got %d (%+v)", len(mapped), mapped)
	}
}

// TestExcludeUnmappedLines_RebaselinesRemainingFraction locks the user-visible
// effect: an excluded material no longer dilutes the mixture. 50 g mapped + 50 g
// unmapped, wired as exclude → computeComposition,
// must re-baseline the survivor to 100 % w/w.
func TestExcludeUnmappedLines_RebaselinesRemainingFraction(t *testing.T) {
	lines := []RecipeLine{
		{MaterialID: "rm-1", MaterialName: "Alcohol", Quantity: 50, Unit: "g"},
		{MaterialID: "rm-2", MaterialName: "Mystery", Quantity: 50, Unit: "g"},
	}
	sdsRows := []SupplierSDS{{MaterialID: "rm-1"}}

	mapped, _ := excludeUnmappedLines(lines, sdsRows)
	comp, _ := computeComposition(mapped)
	if len(comp) != 1 {
		t.Fatalf("expected 1 component after exclusion, got %d (%+v)", len(comp), comp)
	}
	if comp[0].Pct != 100 {
		t.Errorf("survivor should re-baseline to 100%%, got %v", comp[0].Pct)
	}
}

// TestComputeComposition_MixedMassVolume_UnitInconsistentFlag confirms that the
// unit_inconsistent FLAG is raised for a mixed-unit recipe and that the pcts are
// left at zero (not silently wrong). This is a second explicit assertion for the
// acceptance criterion "a recipe that mixes mass and volume units with no density
// raises unit_inconsistent" — the test above (TestComputeComposition_MixedUnitsRaisesFlag)
// already covers it; this variant uses different units (kg + l) to confirm the
// family-detection covers the full unit vocabulary.
func TestComputeComposition_MixedMassVolume_UnitInconsistentFlag(t *testing.T) {

	lines := []RecipeLine{
		{Quantity: 1, MaterialName: "Carrier", Unit: "kg"},
		{Quantity: 1, MaterialName: "Fragrance oil", Unit: "l"},
	}

	comps, flags := computeComposition(lines)

	if !hasFlag(flags, FlagUnitInconsistent) {
		t.Fatalf("kg + l mix must raise unit_inconsistent flag; got %+v", flags)
	}

	for _, c := range comps {
		if c.Pct != 0 {
			t.Errorf("component %q pct must be 0 under mixed units, got %v", c.RawMaterialName, c.Pct)
		}
	}
}

// approxEq reports whether two percentages are within a small tolerance — the
// composition rounding tolerance the acceptance criterion allows.
func approxEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}
