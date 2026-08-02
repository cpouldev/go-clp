package core

import (
	"testing"
)

// classify_summation_test.go guards the summation arithmetic itself, as opposed
// to the individual GCL values: that a sum landing EXACTLY on a boundary
// classifies, that a strictly-bounded concentration still does not, that a
// component is counted once however many hazard entries express the same class,
// that pooled endpoints do not double-count, and that a harmonised SCL displaces
// the generic limit in every summation and not only the per-component ones.

// ── Exact boundaries ───────────────────────────────────────────────────────────

// TestSummation_ExactBoundaryFromSeveralComponents pins that accumulated float64
// error cannot drop a mixture below a limit it exactly reaches. 0.01+4.02+0.97
// is 4.999999999999999 in float64, not 5.0.
func TestSummation_ExactBoundaryFromSeveralComponents(t *testing.T) {
	t.Run(
		"skin corrosion at exactly 5%", func(t *testing.T) {
			comps := []ClassComponent{
				comp("a", 0.01, hz("Skin corrosion", "1B", "H314")),
				comp("b", 4.02, hz("Skin corrosion", "1B", "H314")),
				comp("c", 0.97, hz("Skin corrosion", "1B", "H314")),
			}
			label := ClassifyMixture(comps, nil, false).Label
			if !hasStr(label.HCodes, "H314") {
				t.Errorf("Σ = 5.00%% must reach the 5%% Skin Corr. 1 limit; got H-codes %v", label.HCodes)
			}
			if hasStr(label.HCodes, "H315") {
				t.Errorf("Skin Corr. 1 supersedes Skin Irrit. 2; got %v", label.HCodes)
			}
		},
	)

	t.Run(
		"aquatic chronic 1 at exactly 25%", func(t *testing.T) {
			comps := []ClassComponent{
				comp("a", 6.10, hz("Hazardous to the aquatic environment - chronic", "1", "H410")),
				comp("b", 9.95, hz("Hazardous to the aquatic environment - chronic", "1", "H410")),
				comp("c", 8.95, hz("Hazardous to the aquatic environment - chronic", "1", "H410")),
			}
			got, _ := ClassifyAquatic(comps)
			if got != aquaticChron1 {
				t.Errorf("Σ = 25.00%% must reach the 25%% Chronic 1 rung; got %q", got)
			}
		},
	)
}

// TestSummation_StrictUpperBoundStaysBelowLimit is the counterpart guard: the
// boundary tolerance must not swallow a supplier's STRICT "<" upper bound. The
// two live at deliberately separated scales (gclEpsilon vs strictBoundMargin).
func TestSummation_StrictUpperBoundStaysBelowLimit(t *testing.T) {
	atLimit := []ClassComponent{comp("x", 3, hz("Reproductive toxicity", "2", "H361"))}
	if !hasStr(ClassifyMixture(atLimit, nil, false).Label.HCodes, "H361") {
		t.Error("exactly 3.0% must reach the 3% Repr. 2 limit")
	}

	justBelow := []ClassComponent{comp("x", 3-strictBoundMargin, hz("Reproductive toxicity", "2", "H361"))}
	if hasStr(ClassifyMixture(justBelow, nil, false).Label.HCodes, "H361") {
		t.Error("a strict \"<\" bound just below 3.0% must NOT reach the 3% Repr. 2 limit")
	}
}

// ── Counting each component once ───────────────────────────────────────────────

// TestSummation_DuplicateHazardEntriesCountOnce covers the CAS roll-up, which
// concatenates the hazard lists of one substance found in several materials: the
// duplicate entries must not multiply the substance's concentration.
func TestSummation_DuplicateHazardEntriesCountOnce(t *testing.T) {
	t.Run(
		"acute toxicity ATE", func(t *testing.T) {
			single := []ClassComponent{comp("x", 30, hz("Acute toxicity (oral)", "3", "H301"))}
			doubled := []ClassComponent{
				comp("x", 30, hz("Acute toxicity (oral)", "3", "H301"), hz("Acute toxicity (oral)", "3", "H301")),
			}
			want := ClassifyMixture(single, nil, false).Label.HCodes
			got := ClassifyMixture(doubled, nil, false).Label.HCodes
			if !sameStrSet(got, want) {
				t.Errorf("a repeated hazard entry changed the ATE class: %v vs %v", got, want)
			}
		},
	)

	t.Run(
		"STOT SE summation", func(t *testing.T) {
			// 12% against the 20% STOT SE 3 limit: unclassified either way, unless
			// the duplicate entry is counted twice (24%).
			doubled := []ClassComponent{
				comp("x", 12, hz("STOT SE", "3", "H336"), hz("STOT SE", "3", "H336")),
			}
			if codes := ClassifyMixture(doubled, nil, false).Label.HCodes; hasStr(codes, "H336") {
				t.Errorf("12%% is below the 20%% STOT SE 3 limit however often it is listed; got %v", codes)
			}
		},
	)
}

// TestClassifyEye_PoolsSkinCorrOnce covers Table 3.3.3's pooled "Eye Dam. 1 or
// Skin Corr. 1" row. Most Annex VI caustics carry BOTH classifications, and each
// one contributes its concentration to the pool once.
func TestClassifyEye_PoolsSkinCorrOnce(t *testing.T) {
	caustic := []ClassComponent{
		comp("caustic", 2, hz("Skin corrosion", "1A", "H314"), hz("Serious eye damage", "1", "H318")),
	}
	codes := ClassifyMixture(caustic, nil, false).Label.HCodes
	if hasStr(codes, "H318") {
		t.Errorf("one 2%% component in the pooled 3%% Eye Dam. 1 row must not classify H318; got %v", codes)
	}
	if !hasStr(codes, "H319") {
		t.Errorf("the corrosive-to-irritant boost still applies (10×2 ≥ 10); got %v", codes)
	}
}

// ── Specific concentration limits ──────────────────────────────────────────────

// TestSummation_HarmonisedSCLDisplacesGenericLimit covers CLP Annex I 3.2.3.3.4:
// where a component carries a harmonised SCL, the summation compares against
// that limit and not the generic one.
func TestSummation_HarmonisedSCLDisplacesGenericLimit(t *testing.T) {
	withSCL := []ClassComponent{
		{
			Name: "x", ConcentrationPct: 2,
			Hazards: []ComponentHazard{
				{Class: "Skin corrosion", Category: "1B", HCodes: []string{"H314"}, SCLPct: 1},
			},
		},
	}
	label := ClassifyMixture(withSCL, nil, false).Label
	if !hasStr(label.HCodes, "H314") {
		t.Errorf("2%% against a 1%% SCL must classify Skin Corr. 1; got %v", label.HCodes)
	}
	if label.SignalWord != "Danger" {
		t.Errorf("signal word = %q, want Danger", label.SignalWord)
	}
	if !hasStr(label.Pictograms, "GHS05") {
		t.Errorf("pictograms %v missing GHS05", label.Pictograms)
	}

	// Without the SCL the same component sits under the generic 5% limit.
	generic := []ClassComponent{comp("x", 2, hz("Skin corrosion", "1B", "H314"))}
	if codes := ClassifyMixture(generic, nil, false).Label.HCodes; hasStr(codes, "H314") {
		t.Errorf("2%% against the generic 5%% limit must not classify Skin Corr. 1; got %v", codes)
	}
}

// ── Endpoints that must not absorb one another ─────────────────────────────────

// TestClassifyAcuteTox_ReportsEveryRoute covers Annex I 3.1.3.6: acute toxicity
// is classified per exposure route, so a mixture toxic by two routes carries the
// H-code of each rather than only the most severe.
func TestClassifyAcuteTox_ReportsEveryRoute(t *testing.T) {
	comps := []ClassComponent{
		comp("oral", 60, hz("Acute toxicity (oral)", "4", "H302")),
		comp("inhaled", 40, hz("Acute toxicity (inhalation)", "2", "H330")),
	}
	codes := ClassifyMixture(comps, nil, false).Label.HCodes
	if !hasStr(codes, "H302") {
		t.Errorf("the oral route must be reported alongside the inhalation route; got %v", codes)
	}
	inhalation := hasStr(codes, "H330") || hasStr(codes, "H331") || hasStr(codes, "H332")
	if !inhalation {
		t.Errorf("the inhalation route must be reported; got %v", codes)
	}
}

// TestClassifyStotSE_Cat3CoexistsWithSevereCategory covers STOT SE 3 being a
// class in its own right (respiratory-tract irritation / narcosis) rather than a
// lower rung of SE 1-2.
func TestClassifyStotSE_Cat3CoexistsWithSevereCategory(t *testing.T) {
	comps := []ClassComponent{
		comp("severe", 5, hz("STOT SE", "1", "H370")),
		comp("irritant", 30, hz("STOT SE", "3", "H335")),
	}
	codes := ClassifyMixture(comps, nil, false).Label.HCodes
	if !hasStr(codes, "H371") {
		t.Errorf("5%% Cat 1 sits in the 1-<10%% band → STOT SE 2 (H371); got %v", codes)
	}
	if !hasStr(codes, "H335") {
		t.Errorf("30%% Cat 3 reaches the 20%% STOT SE 3 limit and is not absorbed; got %v", codes)
	}
}

// ── Categories the engine cannot apply ─────────────────────────────────────────

// TestClassifyMixture_CategorySpellingsAndFailClosed covers the everyday SDS
// renderings of a category, and requires anything still unresolvable to be
// reported rather than silently dropped.
func TestClassifyMixture_CategorySpellingsAndFailClosed(t *testing.T) {
	for _, spelling := range []string{"1B", "1b", " 1B ", "Cat. 1B", "Cat 1B", "Category 1B"} {
		t.Run(
			"carcinogenicity "+spelling, func(t *testing.T) {
				comps := []ClassComponent{comp("x", 50, hz("Carcinogenicity", spelling, "H350"))}
				if codes := ClassifyMixture(comps, nil, false).Label.HCodes; !hasStr(codes, "H350") {
					t.Errorf("category %q must classify Carc. 1B; got %v", spelling, codes)
				}
			},
		)
	}

	t.Run(
		"unusable category is flagged", func(t *testing.T) {
			comps := []ClassComponent{comp("x", 50, hz("Carcinogenicity", "", "H350"))}
			mix := ClassifyMixture(comps, nil, false)
			if hasStr(mix.Label.HCodes, "H350") {
				t.Errorf("a category the engine cannot apply must not classify; got %v", mix.Label.HCodes)
			}
			if !hasFlag(mix.Flags, FlagEndpointNotEvaluated) {
				t.Errorf("a dropped hazard must be reported; got %+v", mix.Flags)
			}
		},
	)
}

// TestClassifyMixture_HazardsListDrivesSkinSensAndAquatic covers the exported
// API contract: ClassComponent.Hazards is documented as the full per-component
// hazard list, so supplying a class there must classify exactly as setting the
// engine's dedicated category field does.
func TestClassifyMixture_HazardsListDrivesSkinSensAndAquatic(t *testing.T) {
	viaHazards := []ClassComponent{
		comp(
			"x", 90,
			hz("Skin sensitisation", "1A", "H317"),
			hz("Hazardous to the aquatic environment - chronic", "1", "H410"),
		),
	}
	viaFields := []ClassComponent{
		{Name: "x", ConcentrationPct: 90, SkinSensCategory: "1A", AquaticChronicCategory: "1"},
	}

	got := ClassifyMixture(viaHazards, nil, false).Label
	want := ClassifyMixture(viaFields, nil, false).Label
	if !sameStrSet(got.HCodes, want.HCodes) {
		t.Errorf("Hazards list gave H-codes %v, dedicated fields gave %v", got.HCodes, want.HCodes)
	}
	if !sameStrSet(got.Pictograms, want.Pictograms) {
		t.Errorf("Hazards list gave pictograms %v, dedicated fields gave %v", got.Pictograms, want.Pictograms)
	}
}

// TestClassifyMixture_GenericAquaticClassIsClassified covers the untiered
// "Hazardous to the aquatic environment" spelling that ParseHazardClass accepts:
// it must reach the aquatic cascade rather than falling through to nothing.
func TestClassifyMixture_GenericAquaticClassIsClassified(t *testing.T) {
	comps := []ClassComponent{
		{
			Name: "x", ConcentrationPct: 100,
			Hazards: []ComponentHazard{
				{
					Class:    "Hazardous to the aquatic environment",
					Category: "1", MFactorChronic: 10, HCodes: []string{"H410"},
				},
			},
		},
	}
	if got, _ := ClassifyAquatic(comps); got != aquaticChron1 {
		t.Errorf("aquatic class = %q, want %q", got, aquaticChron1)
	}
	if codes := ClassifyMixture(comps, nil, false).Label.HCodes; !hasStr(codes, "H410") {
		t.Errorf("H-codes %v missing H410", codes)
	}
}

// ── Engine entry point ─────────────────────────────────────────────────────────

// TestRunEngine_NilRecipeFailsClosed covers the documented "Recipe must be
// non-nil" precondition: violating it fails closed with a blocking flag, like
// every other invalid input, rather than panicking.
func TestRunEngine_NilRecipeFailsClosed(t *testing.T) {
	res := RunEngine(EngineInput{})
	if !hasFlag(res.Flags, FlagDataMissing) {
		t.Fatalf("a nil recipe must raise a data_missing flag; got %+v", res.Flags)
	}
	for _, f := range res.Flags {
		if f.Code == FlagDataMissing && f.Severity != SeverityBlock {
			t.Errorf("severity = %q, want %q", f.Severity, SeverityBlock)
		}
	}
}
