package core

import (
	"slices"
	"testing"
)

// label_construction_test.go covers the Phase A4 label-pipeline additions: CLP
// H-code/EUH ordering and the EUH trigger table (EUH210 for a not-classified
// mixture supplied to the public, supplier-declared EUH204/205 propagation, and
// the EUH208-authored-here exclusion).

func TestOrderHCodesCLP(t *testing.T) {
	// Mixed bands + a duplicate + an unparseable code → physical, health,
	// environmental ascending, dupes removed, unparseable last.
	got := orderHCodesCLP([]string{"H411", "H317", "H225", "H319", "H317", "HXYZ"})
	want := []string{"H225", "H317", "H319", "H411", "HXYZ"}
	if !slices.Equal(got, want) {
		t.Errorf("orderHCodesCLP = %v, want %v", got, want)
	}
}

func TestOrderEUHCodes(t *testing.T) {
	got := orderEUHCodes([]string{"EUH401", "EUH208", "EUH204", "EUH208"})
	want := []string{"EUH204", "EUH208", "EUH401"}
	if !slices.Equal(got, want) {
		t.Errorf("orderEUHCodes = %v, want %v", got, want)
	}
}

func TestAssembleLabel_EUH210_NotClassifiedWithHazardousSubstance(t *testing.T) {
	// A sub-threshold Sens 1B (0.5% < 1.0% GCL): no H317, so the mixture is not
	// classified — but it carries an allergen, so EUH208 (named) + EUH210.
	comps := []ClassComponent{
		{
			Name: "Hydroxycitronellal", ConcentrationPct: 0.5, SkinSensCategory: "1B",
			Hazards: []ComponentHazard{{Class: "Skin sensitisation", Category: "1B", HCodes: []string{"H317"}}},
		},
	}
	lab := ClassifyMixture(comps, nil, false).Label

	if lab.SignalWord != "" || len(lab.HCodes) != 0 || len(lab.Pictograms) != 0 {
		t.Fatalf(
			"expected a not-classified label, got signal=%q H=%v picts=%v",
			lab.SignalWord,
			lab.HCodes,
			lab.Pictograms,
		)
	}
	if !slices.Contains(lab.EuhCodes, "EUH208") {
		t.Errorf("expected EUH208 (sub-threshold sensitiser), got %v", lab.EuhCodes)
	}
	if !slices.Contains(lab.EuhCodes, "EUH210") {
		t.Errorf("not-classified mixture with an allergen → expected EUH210, got %v", lab.EuhCodes)
	}
}

func TestAssembleLabel_NoEUH210_WhenTrulyNonHazardous(t *testing.T) {
	comps := []ClassComponent{{Name: "Water", ConcentrationPct: 100}}
	lab := ClassifyMixture(comps, nil, false).Label
	if len(lab.EuhCodes) != 0 {
		t.Errorf("non-hazardous mixture → no EUH codes, got %v", lab.EuhCodes)
	}
}

func TestAssembleLabel_Art26d_RespSensSuppressesGHS07(t *testing.T) {
	// CLP Art. 26(d): when GHS08 applies for RESPIRATORY sensitisation, GHS07 that
	// stands for skin sensitisation (or skin/eye irritation) is removed.
	out := assembleLabel(
		nil, []classOutcome{
			{display: "Resp. Sens. 1", category: "1", hCodes: []string{"H334"}, pictogram: "GHS08", danger: true},
			{display: "Skin Sens. 1", category: "1", hCodes: []string{"H317"}, pictogram: "GHS07"},
		},
	)
	if slices.Contains(out.Pictograms, "GHS07") {
		t.Errorf("Art.26(d): GHS07 (skin sens) must be suppressed by a resp-sens GHS08, got %v", out.Pictograms)
	}
	if !slices.Contains(out.Pictograms, "GHS08") {
		t.Errorf("GHS08 must remain, got %v", out.Pictograms)
	}
	// Pictogram suppression is pictogram-only: the H-statements are retained.
	if !slices.Contains(out.HCodes, "H317") || !slices.Contains(out.HCodes, "H334") {
		t.Errorf("H-codes must retain H317 + H334, got %v", out.HCodes)
	}
}

func TestAssembleLabel_Art26d_KeepsGHS07FromAcuteToxOrStot(t *testing.T) {
	// Art. 26(d) removes GHS07 only for skin sens / irritation. GHS07 that also
	// stands for Acute Tox. 4 (or STOT SE 3) is NOT suppressed by a resp-sens GHS08.
	out := assembleLabel(
		nil, []classOutcome{
			{display: "Resp. Sens. 1", category: "1", hCodes: []string{"H334"}, pictogram: "GHS08", danger: true},
			{display: "Acute Tox. 4", category: "4", hCodes: []string{"H302"}, pictogram: "GHS07"},
		},
	)
	if !slices.Contains(out.Pictograms, "GHS07") {
		t.Errorf("GHS07 from Acute Tox. 4 must be retained alongside a resp-sens GHS08, got %v", out.Pictograms)
	}
}

func TestAssembleLabel_Art26d_CMRGHS08DoesNotSuppressGHS07(t *testing.T) {
	// Only a RESPIRATORY-sens GHS08 triggers 26(d). GHS08 from CMR/STOT/aspiration
	// leaves GHS07 (skin sens / irritation) in place.
	out := assembleLabel(
		nil, []classOutcome{
			{display: "Carc. 1", category: "1", hCodes: []string{"H350"}, pictogram: "GHS08", danger: true},
			{display: "Skin Sens. 1", category: "1", hCodes: []string{"H317"}, pictogram: "GHS07"},
		},
	)
	if !slices.Contains(out.Pictograms, "GHS07") {
		t.Errorf("CMR GHS08 must NOT suppress GHS07 (only resp-sens does), got %v", out.Pictograms)
	}
}

func TestAssembleLabel_EUHDisclosurePropagation(t *testing.T) {
	// A classified mixture (H317 from a Sens 1B at 2%) plus a component carrying a
	// supplier EUH204 (isocyanates) and a stray EUH208. EUH204 propagates; EUH208
	// on a component is NOT propagated (the engine authors EUH208 itself, and this
	// substance is not in the EUH208 band); EUH210 does not apply (classified).
	comps := []ClassComponent{
		{
			Name: "Sensitiser", ConcentrationPct: 2, SkinSensCategory: "1B",
			Hazards: []ComponentHazard{{Class: "Skin sensitisation", Category: "1B", HCodes: []string{"H317"}}},
		},
		{Name: "Isocyanate carrier", ConcentrationPct: 1, Euh: []string{"EUH204", "EUH208"}},
	}
	lab := ClassifyMixture(comps, nil, false).Label

	if !slices.Contains(lab.HCodes, "H317") {
		t.Fatalf("expected H317 (classified), got %v", lab.HCodes)
	}
	if !slices.Contains(lab.EuhCodes, "EUH204") {
		t.Errorf("expected supplier EUH204 to propagate, got %v", lab.EuhCodes)
	}
	if slices.Contains(lab.EuhCodes, "EUH208") {
		t.Errorf("component-declared EUH208 must NOT propagate (engine authors it), got %v", lab.EuhCodes)
	}
	if slices.Contains(lab.EuhCodes, "EUH210") {
		t.Errorf("classified mixture must not carry EUH210, got %v", lab.EuhCodes)
	}
}
