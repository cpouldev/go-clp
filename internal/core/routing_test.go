package core

import (
	"testing"
)

func TestProductClassString(t *testing.T) {
	cases := map[ProductClass]string{
		ClassLiquid:   "liquid",
		ClassSolidWax: "solid_wax",
		ClassCosmetic: "cosmetic",
		ClassSet:      "set",
		ClassUnknown:  "unknown",
	}
	for cls, want := range cases {
		if got := cls.String(); got != want {
			t.Errorf("ProductClass(%d).String() = %q, want %q", cls, got, want)
		}
	}
}

// TestRunEngineSolidWaxPath proves the routing layer's central safety property:
// for an identical recipe (a flammable, skin-sensitising fragrance), the solid-wax
// path drops the liquid-only flammable classification and Class 3 transport while
// keeping the skin-sensitisation classification, whereas the liquid path keeps
// both.
func TestRunEngineSolidWaxPath(t *testing.T) {
	// One fragrance component: flammable (flash point 40 °C → Flam. Liq. 3) AND a
	// skin sensitiser (Skin Sens. 1 at 100 % → H317).
	flash := 40.0
	fragrance := tSub(
		"Fragrance Oil", "100-00-1", "200-000-0", "100%",
		[]string{"H226", "H317"},
		tHaz("Flammable Liquid", "3"),
		tHaz("Skin Sens.", "1"),
	)
	lines := []RecipeLine{tLine("rm-frag", 100, "g", "Fragrance Oil", "FRG-1")}
	extractions := map[string]ParsedExtraction{
		"rm-frag": {FlashPointCelsius: &flash, Substances: []ParsedSubstance{fragrance}},
	}

	liquid := RunEngine(
		EngineInput{
			Recipe:       &Recipe{Name: "Reed Diffuser", FormulationNumber: 1},
			ProductClass: ClassLiquid, Lines: lines, Extractions: extractions,
		},
	)
	solid := RunEngine(
		EngineInput{
			Recipe:       &Recipe{Name: "Scented Candle", FormulationNumber: 2},
			ProductClass: ClassSolidWax, Lines: lines, Extractions: extractions,
		},
	)

	// Liquid keeps BOTH the flammable and skin-sens classification.
	if !tHasStr(liquid.Classification.Label.HCodes, "H226") {
		t.Errorf("liquid: expected H226 (Flam. Liq.), got HCodes=%v", liquid.Classification.Label.HCodes)
	}
	if !tHasStr(liquid.Classification.Label.HCodes, "H317") {
		t.Errorf("liquid: expected H317 (Skin Sens.), got HCodes=%v", liquid.Classification.Label.HCodes)
	}
	if !tHasStr(liquid.Classification.Label.Pictograms, "GHS02") {
		t.Errorf("liquid: expected GHS02 pictogram, got %v", liquid.Classification.Label.Pictograms)
	}
	if liquid.Classification.Transport.ADR == nil || liquid.Classification.Transport.ADR.Class != "3" {
		t.Errorf("liquid: expected Class 3 transport, got %+v", liquid.Classification.Transport.ADR)
	}
	if liquid.FlashPointC == nil || *liquid.FlashPointC != 40.0 {
		t.Errorf("liquid: expected FlashPointC=40, got %v", liquid.FlashPointC)
	}

	// Solid drops the flammable classification entirely but keeps skin-sens.
	if tHasStr(solid.Classification.Label.HCodes, "H226") {
		t.Errorf("solid: must NOT classify Flam. Liq. (H226), got HCodes=%v", solid.Classification.Label.HCodes)
	}
	if tHasStr(solid.Classification.Label.Pictograms, "GHS02") {
		t.Errorf("solid: must NOT carry the flammable pictogram GHS02, got %v", solid.Classification.Label.Pictograms)
	}
	if !tHasStr(solid.Classification.Label.HCodes, "H317") {
		t.Errorf("solid: expected H317 (Skin Sens.) retained, got HCodes=%v", solid.Classification.Label.HCodes)
	}
	// Solid transport is not a flammable liquid; with no aquatic hazard it is not
	// regulated — never Class 3.
	if solid.Classification.Transport.ADR != nil {
		t.Errorf("solid: expected not-regulated transport, got ADR=%+v", solid.Classification.Transport.ADR)
	}
	// A solid has no flash point to be "missing": no data_missing block flag and
	// no liquid flash-point value on the result.
	if solid.FlashPointC != nil {
		t.Errorf("solid: expected nil FlashPointC, got %v", *solid.FlashPointC)
	}
	for _, f := range solid.Flags {
		if f.Code == FlagDataMissing && (f.Section == "9.1" || f.Section == "14") {
			t.Errorf("solid: unexpected missing-flash-point flag %q in §%s: %s", f.Code, f.Section, f.Message)
		}
	}
}

// ── local fixture helpers (mirror the e2e harness builders) ─────────────────

func tLine(rmID string, qty float64, unit, name, code string) RecipeLine {
	return RecipeLine{
		Quantity:     qty,
		Unit:         unit,
		MaterialID:   rmID,
		MaterialName: name,
		MaterialCode: code,
	}
}

func tSub(name, cas, ec, concRange string, hCodes []string, hazards ...ParsedHazard) ParsedSubstance {
	return ParsedSubstance{
		Name:               name,
		CasNumber:          cas,
		EcNumber:           ec,
		ConcentrationRange: concRange,
		HCodes:             hCodes,
		Hazards:            hazards,
	}
}

func tHaz(class, category string) ParsedHazard {
	return ParsedHazard{Class: class, Category: category}
}

func tHasStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
