package core

import (
	"testing"
)

// harmonised_test.go guards the CLP Annex VI override layer: a matched CAS must
// override M-factors and per-hazard SCLs (taking precedence over a supplier
// value), an unmatched CAS must leave the component untouched and unmarked, and
// the missing-lookup validation flag must fire only when a registry was actually
// consulted and only for hazards where the lookup could change the outcome.

func TestApplyHarmonised_OverridesMFactorAndSCL(t *testing.T) {
	cc := ClassComponent{
		Name:             "Substance A",
		CasNumber:        "  90-15-3 ", // whitespace exercises normCAS
		SkinSensCategory: "1",
		Hazards: []ComponentHazard{
			{Class: "Skin sensitisation", Category: "1", HCodes: []string{"H317"}},
			{Class: "Hazardous to the aquatic environment", Category: "1", HCodes: []string{"H410"}},
		},
	}
	reg := map[string]Harmonised{
		"90-15-3": {
			MFactorAcute:   10,
			MFactorChronic: 100,
			SCLs:           []HarmonisedSCL{{HCode: "H317", Pct: 0.5}},
			Source:         "annex_vi",
		},
	}

	applyHarmonised(&cc, reg)

	if !cc.HarmonisedFound {
		t.Fatal("HarmonisedFound = false, want true after a CAS match")
	}
	if cc.MAcute != 10 || cc.MChronic != 100 {
		t.Errorf("M-factors = (%d,%d), want (10,100)", cc.MAcute, cc.MChronic)
	}
	if cc.SkinSensSCLPct != 0.5 {
		t.Errorf("SkinSensSCLPct = %g, want 0.5", cc.SkinSensSCLPct)
	}
	// The H317 hazard gets the SCL; the H410 hazard has no harmonised SCL.
	if cc.Hazards[0].SCLPct != 0.5 {
		t.Errorf("H317 hazard SCLPct = %g, want 0.5", cc.Hazards[0].SCLPct)
	}
	if cc.Hazards[1].SCLPct != 0 {
		t.Errorf("H410 hazard SCLPct = %g, want 0 (no harmonised SCL)", cc.Hazards[1].SCLPct)
	}
}

func TestApplyHarmonised_SCLTakesPrecedenceOverSupplier(t *testing.T) {
	cc := ClassComponent{
		Name:      "Substance B",
		CasNumber: "100-00-1",
		Hazards: []ComponentHazard{
			{Class: "Skin sensitisation", Category: "1", HCodes: []string{"H317"}, SCLPct: 0.2}, // supplier value
		},
	}
	reg := map[string]Harmonised{"100-00-1": {SCLs: []HarmonisedSCL{{HCode: "H317", Pct: 0.8}}}}

	applyHarmonised(&cc, reg)

	if cc.Hazards[0].SCLPct != 0.8 {
		t.Errorf("SCLPct = %g, want 0.8 (harmonised overrides supplier 0.2)", cc.Hazards[0].SCLPct)
	}
}

func TestApplyHarmonised_PartialDataPreservesSupplierMFactor(t *testing.T) {
	// Harmonised entry has an SCL but no M-factor: the supplier M-factor survives.
	cc := ClassComponent{
		Name:      "Substance C",
		CasNumber: "200-00-2",
		MAcute:    5,
		Hazards:   []ComponentHazard{{Class: "Skin sensitisation", Category: "1", HCodes: []string{"H317"}}},
	}
	reg := map[string]Harmonised{"200-00-2": {SCLs: []HarmonisedSCL{{HCode: "H317", Pct: 0.3}}}}

	applyHarmonised(&cc, reg)

	if cc.MAcute != 5 {
		t.Errorf("MAcute = %d, want 5 preserved (no harmonised M-factor)", cc.MAcute)
	}
	if !cc.HarmonisedFound {
		t.Error("HarmonisedFound = false, want true")
	}
}

func TestApplyHarmonised_NoMatchOrNilRegistryIsNoop(t *testing.T) {
	base := ClassComponent{
		Name:             "Substance D",
		CasNumber:        "300-00-3",
		SkinSensCategory: "1",
		Hazards:          []ComponentHazard{{Class: "Skin sensitisation", Category: "1", HCodes: []string{"H317"}}},
	}

	for _, tc := range []struct {
		name string
		reg  map[string]Harmonised
	}{
		{"nil registry", nil},
		{"empty registry", map[string]Harmonised{}},
		{"unmatched CAS", map[string]Harmonised{"999-99-9": {MFactorAcute: 10}}},
	} {
		cc := base
		cc.Hazards = append([]ComponentHazard(nil), base.Hazards...)
		applyHarmonised(&cc, tc.reg)
		if cc.HarmonisedFound {
			t.Errorf("%s: HarmonisedFound = true, want false", tc.name)
		}
		if cc.SkinSensSCLPct != 0 || cc.Hazards[0].SCLPct != 0 || cc.MAcute != 0 {
			t.Errorf("%s: component mutated by a no-op apply", tc.name)
		}
	}
}

func TestValidateHarmonisedLookups_FlagsOnlyUnmatchedHazardous(t *testing.T) {
	components := []ClassComponent{
		// Unmatched + a relevant hazard → flagged.
		{Name: "Unmatched sensitiser", CasNumber: "1-1-1", SkinSensCategory: "1"},
		// Matched → not flagged even with a relevant hazard.
		{Name: "Matched sensitiser", CasNumber: "2-2-2", SkinSensCategory: "1", HarmonisedFound: true},
		// Unmatched but no harmonisable hazard (plain irritant) → not flagged.
		{
			Name: "Plain irritant", CasNumber: "3-3-3",
			Hazards: []ComponentHazard{{Class: "Skin irritation", Category: "2", HCodes: []string{"H315"}}},
		},
	}

	flags := validateHarmonisedLookups(components)

	if len(flags) != 1 {
		t.Fatalf("got %d no_harmonised_lookup flags, want 1: %+v", len(flags), flags)
	}
	if flags[0].Code != FlagNoHarmonisedLookup || flags[0].Severity != SeverityInfo {
		t.Errorf("flag = %s/%s, want %s/info", flags[0].Code, flags[0].Severity, FlagNoHarmonisedLookup)
	}
}

func TestValidateClassification_HarmonisedGatedOnRegistry(t *testing.T) {
	components := []ClassComponent{
		{
			Name: "Unmatched CMR", CasNumber: "4-4-4",
			Hazards: []ComponentHazard{{Class: "Carcinogenicity", Category: "2", HCodes: []string{"H351"}}},
		},
	}

	if got := countCode(validateClassification(components, nil, false), FlagNoHarmonisedLookup); got != 0 {
		t.Errorf("registryAvailable=false raised %d no_harmonised_lookup flags, want 0", got)
	}
	if got := countCode(validateClassification(components, nil, true), FlagNoHarmonisedLookup); got != 1 {
		t.Errorf("registryAvailable=true raised %d no_harmonised_lookup flags, want 1", got)
	}
}

func countCode(flags []Flag, code FlagCode) int {
	n := 0
	for _, f := range flags {
		if f.Code == code {
			n++
		}
	}
	return n
}

// A substance-specific concentration limit is set FOR one hazard class and
// applies to that class alone (CLP Annex I §1.2.1). This is not automatic here,
// because substanceHazards stamps the substance's WHOLE H-code list onto every
// one of its hazard entries — so matching an SCL against that list alone lets a
// limit set for one endpoint be read as the limit for an unrelated one on the
// same substance. The dangerous direction is an SCL LOOSER than the generic
// limit: it silently switches the other classification off.
func TestApplyHarmonised_SCLDoesNotBleedAcrossEndpoints(t *testing.T) {
	// Shape produced by substanceHazards: both entries carry both H-codes.
	both := []string{"H317", "H351"}
	cc := ClassComponent{
		Name:             "Substance B",
		CasNumber:        "1234-56-7",
		ConcentrationPct: 2.0,
		SkinSensCategory: "1",
		Hazards: []ComponentHazard{
			{Class: "Skin Sens.", Category: "1", HCodes: both},
			{Class: "Carc.", Category: "2", HCodes: both},
		},
	}
	reg := map[string]Harmonised{
		"1234-56-7": {SCLs: []HarmonisedSCL{{HCode: "H317", Pct: 5.0}}, Source: "annex_vi"},
	}

	applyHarmonised(&cc, reg)

	if cc.Hazards[0].SCLPct != 5.0 {
		t.Errorf("skin-sens hazard SCLPct = %g, want 5.0 (the SCL's own endpoint)", cc.Hazards[0].SCLPct)
	}
	if cc.Hazards[1].SCLPct != 0 {
		t.Fatalf(
			"Carc. 2 hazard SCLPct = %g, want 0 — an H317 SCL must not set the carcinogenicity limit",
			cc.Hazards[1].SCLPct,
		)
	}

	// End-to-end: Carc. 2 must therefore keep its generic 1.0% limit, so the
	// substance at 2% is classified. With the SCL bled across, the limit would
	// read 5.0% and H351 would vanish from the label.
	limit, ok := effectiveComponentLimit(cc.Hazards[1], kindCarc)
	if !ok || limit != gclCarc2 {
		t.Errorf("Carc. 2 effective limit = (%g, %v), want (%g, true)", limit, ok, gclCarc2)
	}
	if mix := ClassifyMixture([]ClassComponent{cc}, nil, false); !hasStr(mix.Label.HCodes, "H351") {
		t.Errorf("substance at 2%% with Carc. 2 (limit 1%%): want H351, got %v", mix.Label.HCodes)
	}
}

// hCodeKind must resolve the suffixed CMR statements to the same endpoint as
// their base code — Annex VI writes H350i, H360Df, H361d and friends, and the
// differentiation suffix never changes which hazard class the limit belongs to.
func TestHCodeKind_SuffixedCMRCodesResolveToBaseEndpoint(t *testing.T) {
	tests := map[string]hazardKind{
		"H350": kindCarc, "H350i": kindCarc, "H351": kindCarc,
		"H360": kindRepr, "H360D": kindRepr, "H360Df": kindRepr, "H361d": kindRepr,
		"H361fd": kindRepr,
		"H340":   kindMuta, "H341": kindMuta,
		"H317": kindSkinSens, "H334": kindRespSens, "H362": kindLactation,
	}
	for code, want := range tests {
		got, known := hCodeKind(code)
		if !known || got != want {
			t.Errorf("hCodeKind(%q) = (%d, %v), want (%d, true)", code, got, known, want)
		}
	}
	// An unrecognised statement must report "unknown", never a wrong endpoint —
	// callers treat unknown as "cannot confirm", which is the safe reading.
	if _, known := hCodeKind("EUH208"); known {
		t.Error("hCodeKind(EUH208) reported a known endpoint; supplemental statements have none")
	}
}
