package core

import (
	"fmt"
	"strings"
	"testing"
)

// validation_test.go covers the always-on validation pass added in Phase A3:
// undisclosed-remainder, endpoint-not-evaluated (the silent non-classification
// guard), and the near-threshold extension to respiratory sensitisation, STOT,
// and aspiration. countCode is defined in harmonised_test.go (same package).

func TestValidateUndisclosedRemainder(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pcts     []float64
		wantFlag bool
	}{
		{"large remainder", []float64{40, 20}, true},  // 60% disclosed, 40% remainder
		{"small remainder", []float64{60, 35}, false}, // 95% disclosed, 5% remainder
		{"exactly full", []float64{50, 50}, false},    // 100% disclosed
		{"over 100", []float64{70, 40}, false},        // 110% (range upper bounds) — no remainder
		{"degenerate zero", []float64{0, 0}, false},   // unit-inconsistent case, flagged elsewhere
	} {
		var comps []ClassComponent
		for i, p := range tc.pcts {
			comps = append(comps, ClassComponent{Name: fmt.Sprintf("c%d", i), ConcentrationPct: p})
		}
		got := countCode(validateUndisclosedRemainder(comps), FlagUndisclosedRemainder) > 0
		if got != tc.wantFlag {
			t.Errorf("%s: undisclosed_remainder flag=%v, want %v", tc.name, got, tc.wantFlag)
		}
	}
}

func TestValidateEndpointsEvaluated(t *testing.T) {
	comps := []ClassComponent{
		// Recognised endpoints with an unusable/absent category → flagged.
		{Name: "AcuteNoCat", Hazards: []ComponentHazard{{Class: "Acute toxicity (oral)", Category: ""}}},
		{Name: "CarcNoCat", Hazards: []ComponentHazard{{Class: "Carcinogenicity", Category: ""}}},
		// A class the engine does not classify at all → flagged.
		{Name: "Weird", Hazards: []ComponentHazard{{Class: "Explosive", Category: "1.1"}}},
		// Evaluated cases → NOT flagged.
		{Name: "CarcOK", Hazards: []ComponentHazard{{Class: "Carcinogenicity", Category: "1B"}}},
		{
			Name: "Skin", SkinSensCategory: "1",
			Hazards: []ComponentHazard{{Class: "Skin sensitisation", Category: "1", HCodes: []string{"H317"}}},
		},
		{
			Name:    "Aq",
			Hazards: []ComponentHazard{{Class: "Hazardous to the aquatic environment (Chronic)", Category: "1"}},
		},
		{Name: "Flam", Hazards: []ComponentHazard{{Class: "Flam. Liq.", Category: "2"}}},
		{Name: "AcuteOK", Hazards: []ComponentHazard{{Class: "Acute Tox. (dermal)", Category: "3"}}},
		{
			// Abbreviated CLP form "Skin Sens." (the canonical short form the model
			// emits) must read as EVALUATED — skin sens is classified via the
			// dedicated SkinSensCategory field. Regression lock for the predicate
			// drift ("sensit" vs "skin sens") that produced spurious
			// endpoint_not_evaluated flags on real finalised SDSs.
			Name: "SkinAbbrev", SkinSensCategory: "1B",
			Hazards: []ComponentHazard{{Class: "Skin Sens.", Category: "1B", HCodes: []string{"H317"}}},
		},
	}
	flags := validateEndpointsEvaluated(comps)
	if n := countCode(flags, FlagEndpointNotEvaluated); n != 3 {
		t.Fatalf("endpoint_not_evaluated count = %d, want 3: %+v", n, flags)
	}
	for _, f := range flags {
		if f.Severity != SeverityWarn {
			t.Errorf("endpoint_not_evaluated severity = %s, want warn", f.Severity)
		}
	}
}

func TestValidateEndpointsEvaluated_Dedupes(t *testing.T) {
	comps := []ClassComponent{
		{Name: "Same", Hazards: []ComponentHazard{{Class: "Explosive"}, {Class: "Explosive"}}},
	}
	if n := countCode(validateEndpointsEvaluated(comps), FlagEndpointNotEvaluated); n != 1 {
		t.Errorf("identical hazards should dedupe to one flag, got %d", n)
	}
}

func TestValidateNearThresholds_RespSensStotAspiration(t *testing.T) {
	respLimit := mustGCL(kindRespSens, "1")
	stotSE1 := mustGCL(kindStotSE, "1")
	aspLimit := mustGCL(kindAsp, "1")

	comps := []ClassComponent{
		{
			Name: "Resp", ConcentrationPct: respLimit * 0.95,
			Hazards: []ComponentHazard{
				{
					Class: "Respiratory sensitisation", Category: "1", HCodes: []string{"H334"},
				},
			},
		},
		{
			Name: "Stot", ConcentrationPct: stotSE1 * 0.95,
			Hazards: []ComponentHazard{{Class: "STOT SE", Category: "1"}},
		},
		{
			Name: "Asp", ConcentrationPct: aspLimit * 0.95,
			Hazards: []ComponentHazard{{Class: "Aspiration", Category: "1", HCodes: []string{"H304"}}},
		},
	}
	if n := countCode(validateNearThresholds(comps), FlagNearThreshold); n < 3 {
		t.Errorf("near_threshold count = %d, want >= 3 (resp sens + STOT SE 1 + aspiration)", n)
	}
}

// TestValidateNearThresholds_AquaticChronic4SafetyNet locks the near-threshold
// review onto the Chronic 4 safety-net rung (unweighted Σ Chronic 1+2+3+4 vs
// 25%), mirroring ClassifyAquatic. Without it, a mixture classifying via the
// safety net would be the one aquatic rung the reviewer is never warned about.
func TestValidateNearThresholds_AquaticChronic4SafetyNet(t *testing.T) {
	// 12 + 11 = 23, within 10% of 25 → flagged. No other aquatic rung is near
	// (Chronic 3 rung: 10×0 + 12 = 12; Chronic 2 rung: 0).
	near := []ClassComponent{
		{Name: "c3", ConcentrationPct: 12, AquaticChronicCategory: "3"},
		{Name: "c4", ConcentrationPct: 11, AquaticChronicCategory: "4"},
	}
	found := false
	for _, f := range validateNearThresholds(near) {
		if f.Code == FlagNearThreshold && strings.Contains(f.Message, "Chronic 4") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a near_threshold flag for the Chronic 4 safety-net summation")
	}

	// Far from the threshold → no Chronic 4 near flag.
	far := []ClassComponent{
		{Name: "c4", ConcentrationPct: 5, AquaticChronicCategory: "4"},
	}
	for _, f := range validateNearThresholds(far) {
		if strings.Contains(f.Message, "Chronic 4") {
			t.Fatalf("5%% Chronic 4 must not be near the 25%% safety net, got %+v", f)
		}
	}
}

// TestValidateClassification_AlwaysOnPassesRun verifies the new passes run from
// validateClassification even with no harmonised registry (registryAvailable=false).
func TestValidateClassification_AlwaysOnPassesRun(t *testing.T) {
	comps := []ClassComponent{
		{
			Name: "OnlyHazard", ConcentrationPct: 5,
			Hazards: []ComponentHazard{{Class: "Acute toxicity (oral)", Category: ""}},
		}, // unevaluated + 95% remainder
	}
	flags := validateClassification(comps, nil, false)

	// The endpoint-not-evaluated pass runs inside ClassifyMixture rather than the
	// review pass, so that a direct ClassifyMixture caller is told when a hazard
	// it supplied was dropped. RunEngine folds in both sets, so its output is
	// unchanged.
	if countCode(ClassifyMixture(comps, nil, false).Flags, FlagEndpointNotEvaluated) == 0 {
		t.Error("ClassifyMixture did not run the endpoint-not-evaluated pass")
	}
	if countCode(flags, FlagUndisclosedRemainder) == 0 {
		t.Error("validateClassification did not run the undisclosed-remainder pass")
	}
	if countCode(flags, FlagNoHarmonisedLookup) != 0 {
		t.Error("no_harmonised_lookup must not fire when registryAvailable=false")
	}
}

// TestValidateHarmonisedLookups_RollsUpIntoOneFlag verifies the no-harmonised
// advisory is a single roll-up naming the affected substances, not one flag per
// substance, and that it excludes components that were found or carry no
// harmonisable-endpoint hazard.
func TestValidateHarmonisedLookups_RollsUpIntoOneFlag(t *testing.T) {
	comps := []ClassComponent{
		{Name: "Alpha", CasNumber: "1-1-1", SkinSensCategory: "1B"}, // harmonisable, not found
		{
			Name: "Beta", CasNumber: "2-2-2", AquaticChronicCategory: "1",
		}, // harmonisable, not found
		{
			Name: "Gamma", CasNumber: "3-3-3", SkinSensCategory: "1", HarmonisedFound: true,
		}, // found → excluded
		{
			Name: "Delta", CasNumber: "4-4-4",
			Hazards: []ComponentHazard{{Class: "Eye irritation", Category: "2"}},
		}, // not harmonisable → excluded
	}
	flags := validateHarmonisedLookups(comps)
	if len(flags) != 1 {
		t.Fatalf("want exactly 1 roll-up flag, got %d: %+v", len(flags), flags)
	}
	f := flags[0]
	if f.Code != FlagNoHarmonisedLookup || f.Severity != SeverityInfo {
		t.Errorf("flag code/severity = %q/%q", f.Code, f.Severity)
	}
	if !strings.Contains(f.Message, "Alpha") || !strings.Contains(f.Message, "Beta") {
		t.Errorf("roll-up must name Alpha + Beta, got %q", f.Message)
	}
	if strings.Contains(f.Message, "Gamma") || strings.Contains(f.Message, "Delta") {
		t.Errorf("roll-up must exclude found / non-harmonisable substances, got %q", f.Message)
	}
	if !strings.Contains(f.Message, "2 constituent") {
		t.Errorf("roll-up should report the count (2), got %q", f.Message)
	}
}

func TestValidateAllergenCoverage(t *testing.T) {
	comps := []ClassComponent{
		// On the allergen list, no supplier skin-sens classification, ≥0.1% → flag.
		{Name: "Linalool", CasNumber: "78-70-6", IsFragranceAllergen: true, ConcentrationPct: 0.5},
		// Exactly at the 0.1% threshold → flag (≥).
		{Name: "Citral", CasNumber: "5392-40-5", IsFragranceAllergen: true, ConcentrationPct: 0.1},
		// Allergen but BELOW threshold → no flag.
		{Name: "Geraniol", CasNumber: "106-24-1", IsFragranceAllergen: true, ConcentrationPct: 0.05},
		// Allergen but supplier DID classify it as a skin sensitiser → existing
		// H317/EUH208 logic owns it, not re-flagged here.
		{
			Name: "Limonene", CasNumber: "5989-27-5", IsFragranceAllergen: true, ConcentrationPct: 2,
			SkinSensCategory: "1B",
		},
		// Not an allergen → never flagged regardless of concentration.
		{Name: "Water", CasNumber: "7732-18-5", ConcentrationPct: 60},
	}
	flags := validateAllergenCoverage(comps)
	if n := countCode(flags, FlagAllergenCoverage); n != 2 {
		t.Fatalf("allergen_coverage count = %d, want 2 (Linalool, Citral): %+v", n, flags)
	}
	for _, f := range flags {
		if f.Severity != SeverityWarn {
			t.Errorf("allergen_coverage severity = %s, want warn", f.Severity)
		}
		if f.Section != "2.2" {
			t.Errorf("allergen_coverage section = %s, want 2.2", f.Section)
		}
	}
}
