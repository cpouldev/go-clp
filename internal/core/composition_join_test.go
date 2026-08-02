package core

import (
	"strings"
	"testing"
)

// composition_join_test.go covers how recipe lines are joined to their parsed
// supplier SDS, and how a supplier concentration RANGE is read — the two places
// where a wrong answer silently changes every downstream classification.

// TestBuildClassComponents_JoinsByMaterialID pins the documented join key.
// Material names are not unique: two lines can carry two supplier batches of one
// trade name, and joining on the name merges them into whichever extraction
// happens to be indexed last.
func TestBuildClassComponents_JoinsByMaterialID(t *testing.T) {
	composition := []Component{
		{MaterialID: "rm1", RawMaterialName: "Fragrance Oil", Pct: 50},
		{MaterialID: "rm2", RawMaterialName: "Fragrance Oil", Pct: 50},
	}
	extractions := map[string]ParsedExtraction{
		"rm1": {
			RawMaterialName: "Fragrance Oil",
			Substances: []ParsedSubstance{
				{
					Name: "Citral", CasNumber: "5392-40-5", ConcentrationRange: "10 %",
					Hazards: []ParsedHazard{{Class: "Skin sensitisation", Category: "1"}},
					HCodes:  []string{"H317"},
				},
			},
		},
		"rm2": {
			RawMaterialName: "Fragrance Oil",
			Substances: []ParsedSubstance{
				{Name: "Dipropylene glycol", CasNumber: "25265-71-8", ConcentrationRange: "10 %"},
			},
		},
	}

	comps, _ := buildClassComponents(composition, extractions, nil)

	byName := make(map[string]ClassComponent, len(comps))
	for _, c := range comps {
		byName[c.Name] = c
	}
	if _, ok := byName["Citral"]; !ok {
		t.Errorf("the sensitiser from rm1 was dropped; got %v", byName)
	}
	if _, ok := byName["Dipropylene glycol"]; !ok {
		t.Errorf("the substance from rm2 was dropped; got %v", byName)
	}
	if got := byName["Citral"].ConcentrationPct; got != 5 {
		t.Errorf("Citral = %v%%, want 5%% (10%% of one 50%% line, not of both)", got)
	}
	if !hasStr(ClassifyMixture(comps, nil, false).Label.HCodes, "H317") {
		t.Error("the sensitiser's hazard must reach the classification")
	}
}

// TestBuildClassComponents_IngredientNameIsNotAComparator covers a bounded
// supplier range whose text happens to contain a comparator word. "jasmine" and
// "cumin" are everyday ingredients that contain "min"; reading either as a
// lower-bound marker turns a bounded range into an open-ended one and carries
// the substance at the material's full percentage.
func TestBuildClassComponents_IngredientNameIsNotAComparator(t *testing.T) {
	t.Run(
		"comparator detection", func(t *testing.T) {
			cases := []struct {
				text              string
				wantLow, wantHigh bool
			}{
				{"1 - 5 % jasmine absolute", false, false},
				{"1 - 5 % cumin oil", false, false},
				{"5 % aluminium powder", false, false},
				{"contains amine derivatives 2 %", false, false},
				{"min 5 %", true, false},
				{"min. 5 %", true, false},
				{"max 5 %", false, true},
				{"≥ 1 - < 5 %", true, true},
				{"at least 3 %", true, false},
				{"up to 3 %", false, true},
			}
			for _, tc := range cases {
				if got := hasLowerBoundComparator(tc.text); got != tc.wantLow {
					t.Errorf("hasLowerBoundComparator(%q) = %v, want %v", tc.text, got, tc.wantLow)
				}
				if got := hasUpperBoundComparator(tc.text); got != tc.wantHigh {
					t.Errorf("hasUpperBoundComparator(%q) = %v, want %v", tc.text, got, tc.wantHigh)
				}
			}
		},
	)

	t.Run(
		"bounded range is apportioned", func(t *testing.T) {
			composition := []Component{{MaterialID: "rm1", RawMaterialName: "Jasmine base", Pct: 20}}
			extractions := map[string]ParsedExtraction{
				"rm1": {
					RawMaterialName: "Jasmine base",
					Substances: []ParsedSubstance{
						{
							Name: "Benzyl acetate", CasNumber: "140-11-4",
							ConcentrationRange: "1 - 5 % jasmine absolute",
						},
					},
				},
			}
			comps, _ := buildClassComponents(composition, extractions, nil)
			if len(comps) != 1 {
				t.Fatalf("want 1 component, got %d", len(comps))
			}
			if got := comps[0].ConcentrationPct; got != 1 {
				t.Errorf("ConcentrationPct = %v%%, want 1%% (5%% of a 20%% material), not the full material", got)
			}
		},
	)
}

// TestBuildClassComponents_PartialConcentrationsAreFlagged covers the common
// partially-quantified supplier SDS. The substances with no stated concentration
// are carried at the material's full percentage, which is the conservative
// bound but overstates them and can push the disclosed composition past 100%, so
// the operator has to be told.
func TestBuildClassComponents_PartialConcentrationsAreFlagged(t *testing.T) {
	composition := []Component{{MaterialID: "rm1", RawMaterialName: "Fragrance", Pct: 30}}
	extractions := map[string]ParsedExtraction{
		"rm1": {
			RawMaterialName: "Fragrance",
			Substances: []ParsedSubstance{
				{Name: "Linalool", CasNumber: "78-70-6", ConcentrationRange: "≥ 1 - < 5 %"},
				{Name: "Some ketone", CasNumber: "111-11-1"},
			},
		},
	}

	comps, flags := buildClassComponents(composition, extractions, nil)
	if !hasFlag(flags, FlagDataMissing) {
		t.Errorf("a partially-quantified SDS must raise data_missing; got %+v", flags)
	}
	var named bool
	for _, f := range flags {
		if f.Code == FlagDataMissing && strings.Contains(f.Message, "Some ketone") {
			named = true
		}
	}
	if !named {
		t.Errorf("the flag must name the unquantified substance; got %+v", flags)
	}
	if len(comps) != 2 {
		t.Fatalf("want 2 components, got %d", len(comps))
	}
}
