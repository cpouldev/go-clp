package core

import (
	"testing"
)

// harmonised_lookup_test.go covers how a component is matched to its harmonised
// Annex VI entry, and what is applied once it matches. A missed lookup is not a
// degraded result: the substance silently loses a mandatory classification while
// the audit trail still reports it as self-classified.

// TestNormCAS_SupplierRenderings covers the CAS renderings that reach the engine
// from ERP exports (zero-padded) and PDF-extracted SDS text (Unicode dashes).
func TestNormCAS_SupplierRenderings(t *testing.T) {
	const canonical = "5989-27-5"
	for _, in := range []string{
		"5989-27-5",
		" 5989-27-5 ",
		"005989-27-5",
		"0005989-27-5",
		"5989–27–5", // en-dash
		"5989‑27‑5", // non-breaking hyphen
		"5989 - 27 - 5",
	} {
		if got := normCAS(in); got != canonical {
			t.Errorf("normCAS(%q) = %q, want %q", in, got, canonical)
		}
	}

	// A value that is not a plain CAS rendering is left alone.
	if got := normCAS("  not-a-cas  "); got != "not-a-cas" {
		t.Errorf("normCAS of a non-CAS value = %q, want it trimmed and unchanged", got)
	}

	// An EC number's leading zeros are part of the identifier.
	if got := normEC("020-001-8"); got != "020-001-8" {
		t.Errorf("normEC(%q) = %q, leading zeros must be preserved", "020-001-8", got)
	}
}

// TestApplyHarmonised_MatchesNonCanonicalCASAndECNumber covers both lookup keys.
func TestApplyHarmonised_MatchesNonCanonicalCASAndECNumber(t *testing.T) {
	registry := map[string]Harmonised{
		"5989-27-5": {SCLs: []HarmonisedSCL{{HCode: "H317", Pct: 0.5}}, Source: "annex_vi/test"},
		"200-001-8": {MFactorChronic: 10, Source: "annex_vi/test"},
	}

	t.Run(
		"zero-padded CAS", func(t *testing.T) {
			cc := ClassComponent{
				Name: "Limonene", CasNumber: "005989-27-5", ConcentrationPct: 1,
				Hazards: []ComponentHazard{hz("Skin sensitisation", "1", "H317")},
			}
			applyHarmonised(&cc, registry)
			if !cc.HarmonisedFound {
				t.Fatal("a zero-padded CAS must still match the registry")
			}
			if cc.SkinSensSCLPct != 0.5 {
				t.Errorf("SkinSensSCLPct = %v, want 0.5", cc.SkinSensSCLPct)
			}
		},
	)

	t.Run(
		"EC number when the CAS does not match", func(t *testing.T) {
			cc := ClassComponent{
				Name: "Substance", CasNumber: "", EcNumber: "200-001-8", ConcentrationPct: 1,
				Hazards: []ComponentHazard{
					hz("Hazardous to the aquatic environment - chronic", "1", "H410"),
				},
			}
			applyHarmonised(&cc, registry)
			if !cc.HarmonisedFound {
				t.Fatal("an entry carrying only an EC number must be reachable")
			}
			if cc.MChronic != 10 {
				t.Errorf("MChronic = %d, want 10", cc.MChronic)
			}
		},
	)
}

// TestApplyHarmonised_AppliesMandatoryClassification covers CLP Art. 4(3): the
// harmonised classification may not be omitted, so an endpoint missing from the
// supplier's own data is added from the registry.
func TestApplyHarmonised_AppliesMandatoryClassification(t *testing.T) {
	registry := map[string]Harmonised{
		"121-43-7": {
			Hazards: []HarmonisedHazard{
				{HCode: "H360FD", Class: "Repr.", Category: "1B"},
				{HCode: "H226", Class: "Flam. Liq.", Category: "3"},
			},
			Source: "annex_vi/test",
		},
	}

	// A supplier SDS that declares only the flammability, omitting the repro entry.
	cc := ClassComponent{
		Name: "Trimethyl borate", CasNumber: "121-43-7", ConcentrationPct: 5,
		Hazards: []ComponentHazard{hz("Flammable liquid", "3", "H226")},
	}
	applyHarmonised(&cc, registry)

	if !componentHasHazardClass(cc, "Reproductive toxicity") {
		t.Fatalf("the harmonised Repr. classification was not applied; hazards = %+v", cc.Hazards)
	}
	if got := len(cc.Hazards); got != 2 {
		t.Errorf("hazard count = %d, want 2 (the supplier's own entry must not be duplicated)", got)
	}
	if codes := ClassifyMixture([]ClassComponent{cc}, nil, false).Label.HCodes; !hasStr(codes, "H360") {
		t.Errorf("5%% of a Repr. 1B substance must classify H360; got %v", codes)
	}
}

// TestApplyHarmonised_MFactorReachesHazardEntry pins the M-factor onto the
// component's aquatic hazard entry as well as its cascade field, so the §3
// render reports the factor the classification actually used.
func TestApplyHarmonised_MFactorReachesHazardEntry(t *testing.T) {
	registry := map[string]Harmonised{
		"111-11-1": {MFactorChronic: 10, Source: "annex_vi/test"},
	}
	cc := ClassComponent{
		Name: "Substance", CasNumber: "111-11-1", ConcentrationPct: 3,
		Hazards: []ComponentHazard{hz("Hazardous to the aquatic environment - chronic", "1", "H410")},
	}
	applyHarmonised(&cc, registry)

	if cc.MChronic != 10 {
		t.Errorf("MChronic = %d, want 10", cc.MChronic)
	}
	if got := cc.Hazards[0].MFactorChronic; got != 10 {
		t.Errorf("hazard entry MFactorChronic = %d, want 10 — §3 would print M=1", got)
	}
}
