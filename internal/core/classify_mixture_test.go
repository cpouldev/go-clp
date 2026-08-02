package core

import (
	"sort"
	"testing"
)

// classify_mixture_test.go locks the deterministic CLP engine to its
// legally-load-bearing boundaries. Every threshold below is a generic
// concentration limit (GCL), additivity cut-off, ATE point estimate, or
// flash-point boundary from CLP 1272/2008 Annex I; the engine compares with
// ">=" at the boundary, so each class is probed at the limit (must classify)
// and just below (must not).

// ── fixture helpers ────────────────────────────────────────────────────────

// hz builds a per-component hazard entry (class + category + H-codes).
func hz(class, category string, hCodes ...string) ComponentHazard {
	return ComponentHazard{Class: class, Category: category, HCodes: hCodes}
}

// comp builds a component at a concentration carrying a free-form hazard list
// (the path ClassifyMixture reads for every class except skin-sens/aquatic).
func comp(name string, pct float64, hazards ...ComponentHazard) ClassComponent {
	return ClassComponent{Name: name, ConcentrationPct: pct, Hazards: hazards}
}

func hasStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func sameStrSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	for i := range g {
		if g[i] != w[i] {
			return false
		}
	}
	return true
}

// ── Acute toxicity (ATE summation, Table 3.1.x) ──────────────────────────────

func TestClassifyMixture_AcuteTox_ATESummationBoundaries(t *testing.T) {
	// Oral ATE point estimates: Cat4=500, Cat3=100, Cat2=5. ATEmix = 100/Σ(Ci/ATEi);
	// acuteCategoryFromATE: ≤5→"1", ≤50→"2", ≤300→"3", ≤2000→"4", else not classified.
	tests := []struct {
		name     string
		comps    []ClassComponent
		wantH    string // "" = not classified
		wantPict string
		wantDng  bool
	}{
		{
			// 25% oral Cat4: ATEmix = 100/(25/500) = 2000 → exactly Cat 4.
			name:     "oral cat4 at 25pct hits cat4 boundary",
			comps:    []ClassComponent{comp("a", 25, hz("Acute Tox. 4 (oral)", "4", "H302"))},
			wantH:    "H302",
			wantPict: "GHS07",
			wantDng:  false,
		},
		{
			// 24% oral Cat4: ATEmix = 2083 > 2000 → not classified.
			name:  "oral cat4 just below boundary not classified",
			comps: []ClassComponent{comp("a", 24, hz("Acute Tox. 4 (oral)", "4", "H302"))},
			wantH: "",
		},
		{
			// 50% oral Cat3: ATEmix = 100/(50/100) = 200 → Cat 3.
			name:     "oral cat3 maps to cat3 with skull",
			comps:    []ClassComponent{comp("a", 50, hz("Acute Tox. 3 (oral)", "3", "H301"))},
			wantH:    "H301",
			wantPict: "GHS06",
			wantDng:  true,
		},
		{
			// 50% oral Cat2: ATEmix = 100/(50/5) = 10 → Cat 2 (H300, skull).
			name:     "oral cat2 maps to cat2 with skull",
			comps:    []ClassComponent{comp("a", 50, hz("Acute Tox. 2 (oral)", "2", "H300"))},
			wantH:    "H300",
			wantPict: "GHS06",
			wantDng:  true,
		},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				if tc.wantH == "" {
					if hasStr(mix.Label.HCodes, "H300") || hasStr(mix.Label.HCodes, "H301") ||
						hasStr(mix.Label.HCodes, "H302") {
						t.Fatalf("expected no acute H-code, got %v", mix.Label.HCodes)
					}
					return
				}
				if !hasStr(mix.Label.HCodes, tc.wantH) {
					t.Fatalf("HCodes = %v, want %s", mix.Label.HCodes, tc.wantH)
				}
				if !hasStr(mix.Label.Pictograms, tc.wantPict) {
					t.Fatalf("Pictograms = %v, want %s", mix.Label.Pictograms, tc.wantPict)
				}
				wantWord := "Warning"
				if tc.wantDng {
					wantWord = "Danger"
				}
				if mix.Label.SignalWord != wantWord {
					t.Fatalf("SignalWord = %q, want %q", mix.Label.SignalWord, wantWord)
				}
			},
		)
	}
}

// TestClassifyMixture_AcuteTox_RouteSpecificCutoffs proves the ATEmix→category
// mapping uses each route's OWN CLP Table 3.1.1 cut-offs, not the oral scale for
// all three. Every case lands on a DIFFERENT category (and label) under its
// route's cut-offs than it would under the oral cut-offs, so it fails if the
// route mapping ever regresses to oral-only.
func TestClassifyMixture_AcuteTox_RouteSpecificCutoffs(t *testing.T) {
	tests := []struct {
		name     string
		comps    []ClassComponent
		wantH    string
		wantPict string
		wantDng  bool
	}{
		{
			// Inhalation Cat 4 (vapour ATE 11) at 100% → ATEmix 11.0.
			// Inhalation cut-offs: 10 < 11 ≤ 20 → Cat 4 → H332 (Warning, GHS07).
			// Oral cut-offs would WRONGLY read 11 ≤ 50 → Cat 2 → H330 "fatal".
			name:     "inhalation cat4 is harmful (H332), not fatal (H330)",
			comps:    []ClassComponent{comp("a", 100, hz("Acute Tox. 4 (inhalation)", "4", "H332"))},
			wantH:    "H332",
			wantPict: "GHS07",
			wantDng:  false,
		},
		{
			// Inhalation Cat 3 (vapour ATE 3) at 100% → ATEmix 3.0.
			// Inhalation cut-offs: 2 < 3 ≤ 10 → Cat 3 → H331. Oral cut-offs would
			// give 3 ≤ 5 → Cat 1 → H330 (two categories too severe).
			name:     "inhalation cat3 is H331, not oral-scale H330",
			comps:    []ClassComponent{comp("a", 100, hz("Acute Tox. 3 (inhalation)", "3", "H331"))},
			wantH:    "H331",
			wantPict: "GHS06",
			wantDng:  true,
		},
		{
			// Dermal Cat 2 (ATE 50) at 60% → ATEmix 100/(60/50) = 83.3.
			// Dermal cut-offs: 50 < 83.3 ≤ 200 → Cat 2 → H310 (fatal). Oral cut-offs
			// would WRONGLY read 83.3 ≤ 300 → Cat 3 → H311 (toxic): an
			// under-classification in the dangerous direction.
			name:     "dermal cat2 is fatal (H310), not toxic (H311)",
			comps:    []ClassComponent{comp("a", 60, hz("Acute Tox. 2 (dermal)", "2", "H310"))},
			wantH:    "H310",
			wantPict: "GHS06",
			wantDng:  true,
		},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				if !hasStr(mix.Label.HCodes, tc.wantH) {
					t.Fatalf("HCodes = %v, want %s", mix.Label.HCodes, tc.wantH)
				}
				if !hasStr(mix.Label.Pictograms, tc.wantPict) {
					t.Fatalf("Pictograms = %v, want %s", mix.Label.Pictograms, tc.wantPict)
				}
				wantWord := "Warning"
				if tc.wantDng {
					wantWord = "Danger"
				}
				if mix.Label.SignalWord != wantWord {
					t.Fatalf("SignalWord = %q, want %q", mix.Label.SignalWord, wantWord)
				}
			},
		)
	}
}

// ── Skin corrosion / irritation (additive, Table 3.2.3) ──────────────────────

func TestClassifyMixture_SkinCorrIrrit_AdditiveBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		comps    []ClassComponent
		wantH    string
		wantPict string
		wantDng  bool
	}{
		{
			name:     "corrosive sum at 5pct triggers Skin Corr 1",
			comps:    []ClassComponent{comp("a", 5.0, hz("Skin Corr. 1", "1", "H314"))},
			wantH:    "H314",
			wantPict: "GHS05",
			wantDng:  true,
		},
		{
			// 1.0% corrosive: below the Corr1 GCL and below the Eye Dam 1 GCL (3%),
			// but the ×10 boost (10×1.0 = 10) exactly reaches the Skin Irrit 2
			// cut-off, so H315 classifies on its own from a sub-corrosive amount.
			name:     "corrosive at 1pct still irritates via x10 boost",
			comps:    []ClassComponent{comp("a", 1.0, hz("Skin Corr. 1", "1", "H314"))},
			wantH:    "H315",
			wantPict: "GHS07",
			wantDng:  false,
		},
		{
			name:     "pure irritant sum at 10pct triggers Skin Irrit 2",
			comps:    []ClassComponent{comp("a", 10.0, hz("Skin Irrit. 2", "2", "H315"))},
			wantH:    "H315",
			wantPict: "GHS07",
			wantDng:  false,
		},
		{
			name:  "pure irritant just below 10pct not classified",
			comps: []ClassComponent{comp("a", 9.9, hz("Skin Irrit. 2", "2", "H315"))},
			wantH: "",
		},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				if tc.wantH == "" {
					if hasStr(mix.Label.HCodes, "H314") || hasStr(mix.Label.HCodes, "H315") {
						t.Fatalf("expected no skin H-code, got %v", mix.Label.HCodes)
					}
					return
				}
				if !hasStr(mix.Label.HCodes, tc.wantH) {
					t.Fatalf("HCodes = %v, want %s", mix.Label.HCodes, tc.wantH)
				}
				if !hasStr(mix.Label.Pictograms, tc.wantPict) {
					t.Fatalf("Pictograms = %v, want %s", mix.Label.Pictograms, tc.wantPict)
				}
			},
		)
	}
}

// ── Eye damage / irritation (additive; corrosives count as eye damage) ───────

func TestClassifyMixture_Eye_AdditiveBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		comps    []ClassComponent
		wantH    string
		wantPict string
	}{
		{
			name:     "eye damage sum at 3pct triggers Eye Dam 1",
			comps:    []ClassComponent{comp("a", 3.0, hz("Eye Dam. 1", "1", "H318"))},
			wantH:    "H318",
			wantPict: "GHS05",
		},
		{
			// 2.9% eye damage: 10×2.9 = 29 ≥ 10 → Eye Irrit 2.
			name:     "eye damage below 3pct irritates via x10 boost",
			comps:    []ClassComponent{comp("a", 2.9, hz("Eye Dam. 1", "1", "H318"))},
			wantH:    "H319",
			wantPict: "GHS07",
		},
		{
			name:     "pure eye irritant at 10pct triggers Eye Irrit 2",
			comps:    []ClassComponent{comp("a", 10.0, hz("Eye Irrit. 2", "2", "H319"))},
			wantH:    "H319",
			wantPict: "GHS07",
		},
		{
			name:  "pure eye irritant below 10pct not classified",
			comps: []ClassComponent{comp("a", 9.9, hz("Eye Irrit. 2", "2", "H319"))},
			wantH: "",
		},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				if tc.wantH == "" {
					if hasStr(mix.Label.HCodes, "H318") || hasStr(mix.Label.HCodes, "H319") {
						t.Fatalf("expected no eye H-code, got %v", mix.Label.HCodes)
					}
					return
				}
				if !hasStr(mix.Label.HCodes, tc.wantH) {
					t.Fatalf("HCodes = %v, want %s", mix.Label.HCodes, tc.wantH)
				}
				if !hasStr(mix.Label.Pictograms, tc.wantPict) {
					t.Fatalf("Pictograms = %v, want %s", mix.Label.Pictograms, tc.wantPict)
				}
			},
		)
	}
}

// ── Respiratory sensitisation (per-component GCL) ────────────────────────────

func TestClassifyMixture_RespSens_GCLBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		comps    []ClassComponent
		wantH334 bool
	}{
		{
			"1B at 1.0pct classifies",
			[]ClassComponent{comp("a", 1.0, hz("Respiratory sensitisation", "1", "H334"))},
			true,
		},
		{
			"1B at 0.9pct not classified",
			[]ClassComponent{comp("a", 0.9, hz("Respiratory sensitisation", "1", "H334"))}, false,
		},
		{
			"1A at 0.1pct classifies",
			[]ClassComponent{comp("a", 0.1, hz("Respiratory sensitisation", "1A", "H334"))},
			true,
		},
		{
			"1A at 0.09pct not classified",
			[]ClassComponent{comp("a", 0.09, hz("Respiratory sensitisation", "1A", "H334"))}, false,
		},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				got := hasStr(mix.Label.HCodes, "H334")
				if got != tc.wantH334 {
					t.Fatalf("H334 present = %v, want %v (codes %v)", got, tc.wantH334, mix.Label.HCodes)
				}
				if tc.wantH334 {
					if !hasStr(mix.Label.Pictograms, "GHS08") {
						t.Fatalf("expected GHS08, got %v", mix.Label.Pictograms)
					}
					if mix.Label.SignalWord != "Danger" {
						t.Fatalf("SignalWord = %q, want Danger", mix.Label.SignalWord)
					}
				}
			},
		)
	}
}

// ── CMR (per-component GCL; classes are independent) ─────────────────────────

func TestClassifyMixture_CMR_GCLBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		comps   []ClassComponent
		wantH   string
		wantDng bool
	}{
		{
			"Carc 1A at 0.1pct -> H350", []ClassComponent{comp("a", 0.1, hz("Carcinogenicity", "1A", "H350"))},
			"H350",
			true,
		},
		{
			"Carc 2 at 1.0pct -> H351", []ClassComponent{comp("a", 1.0, hz("Carcinogenicity", "2", "H351"))},
			"H351",
			false,
		},
		{
			"Carc 2 at 0.99pct not classified",
			[]ClassComponent{comp("a", 0.99, hz("Carcinogenicity", "2", "H351"))},
			"", false,
		},
		{
			"Muta 1B at 0.1pct -> H340",
			[]ClassComponent{comp("a", 0.1, hz("Germ cell mutagenicity", "1B", "H340"))},
			"H340", true,
		},
		{
			"Repr 1B at 0.3pct -> H360",
			[]ClassComponent{comp("a", 0.3, hz("Reproductive toxicity", "1B", "H360"))},
			"H360", true,
		},
		{
			"Repr 2 at 3.0pct -> H361", []ClassComponent{comp("a", 3.0, hz("Reproductive toxicity", "2", "H361"))},
			"H361", false,
		},
		{
			"Repr 2 at 2.9pct not classified",
			[]ClassComponent{comp("a", 2.9, hz("Reproductive toxicity", "2", "H361"))}, "", false,
		},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				if tc.wantH == "" {
					for _, c := range []string{"H350", "H351", "H340", "H341", "H360", "H361"} {
						if hasStr(mix.Label.HCodes, c) {
							t.Fatalf("expected no CMR code, got %v", mix.Label.HCodes)
						}
					}
					return
				}
				if !hasStr(mix.Label.HCodes, tc.wantH) {
					t.Fatalf("HCodes = %v, want %s", mix.Label.HCodes, tc.wantH)
				}
				if !hasStr(mix.Label.Pictograms, "GHS08") {
					t.Fatalf("expected GHS08, got %v", mix.Label.Pictograms)
				}
				wantWord := "Warning"
				if tc.wantDng {
					wantWord = "Danger"
				}
				if mix.Label.SignalWord != wantWord {
					t.Fatalf("SignalWord = %q, want %q", mix.Label.SignalWord, wantWord)
				}
			},
		)
	}
}

func TestClassifyMixture_CMR_MultiClassAndCat1Dominates(t *testing.T) {
	// Independent CMR classes co-exist: Carc 2 (1%) + Repr 2 (3%) → both codes.
	multi := ClassifyMixture(
		[]ClassComponent{
			comp("carc", 1.0, hz("Carcinogenicity", "2", "H351")),
			comp("repr", 3.0, hz("Reproductive toxicity", "2", "H361")),
		}, nil, false,
	)
	if !hasStr(multi.Label.HCodes, "H351") || !hasStr(multi.Label.HCodes, "H361") {
		t.Fatalf("expected both H351 and H361, got %v", multi.Label.HCodes)
	}
	if multi.Label.SignalWord != "Warning" {
		t.Fatalf("two Cat-2 CMR → Warning, got %q", multi.Label.SignalWord)
	}

	// Within a family the Cat-1 path wins outright (no Cat-2 echo).
	dom := ClassifyMixture(
		[]ClassComponent{
			comp("c1", 0.1, hz("Carcinogenicity", "1A", "H350")),
			comp("c2", 50, hz("Carcinogenicity", "2", "H351")),
		}, nil, false,
	)
	if !hasStr(dom.Label.HCodes, "H350") || hasStr(dom.Label.HCodes, "H351") {
		t.Fatalf("Carc 1 must dominate Carc 2, got %v", dom.Label.HCodes)
	}
	if dom.Label.SignalWord != "Danger" {
		t.Fatalf("Carc 1 → Danger, got %q", dom.Label.SignalWord)
	}
}

// ── STOT single exposure (Table 3.8.3) ───────────────────────────────────────

func TestClassifyMixture_StotSE_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		comps    []ClassComponent
		wantH    string
		wantPict string
		wantDng  bool
	}{
		{
			"cat1 at 10pct -> SE1 H370", []ClassComponent{comp("a", 10, hz("STOT SE", "1", "H370"))}, "H370",
			"GHS08",
			true,
		},
		{
			"cat1 at 9.9pct -> SE2 H371", []ClassComponent{comp("a", 9.9, hz("STOT SE", "1", "H370"))}, "H371",
			"GHS08",
			false,
		},
		{
			"cat1 at 1.0pct -> SE2 H371", []ClassComponent{comp("a", 1.0, hz("STOT SE", "1", "H370"))}, "H371",
			"GHS08",
			false,
		},
		{
			"cat2 at 10pct -> SE2 H371", []ClassComponent{comp("a", 10, hz("STOT SE", "2", "H371"))}, "H371",
			"GHS08",
			false,
		},
		{
			"cat3 at 20pct -> SE3 H335", []ClassComponent{comp("a", 20, hz("STOT SE", "3", "H335"))}, "H335",
			"GHS07",
			false,
		},
		{
			"cat3 at 19.9pct not classified", []ClassComponent{comp("a", 19.9, hz("STOT SE", "3", "H335"))}, "", "",
			false,
		},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				if tc.wantH == "" {
					for _, c := range []string{"H370", "H371", "H335", "H336"} {
						if hasStr(mix.Label.HCodes, c) {
							t.Fatalf("expected no STOT-SE code, got %v", mix.Label.HCodes)
						}
					}
					return
				}
				if !hasStr(mix.Label.HCodes, tc.wantH) {
					t.Fatalf("HCodes = %v, want %s", mix.Label.HCodes, tc.wantH)
				}
				if !hasStr(mix.Label.Pictograms, tc.wantPict) {
					t.Fatalf("Pictograms = %v, want %s", mix.Label.Pictograms, tc.wantPict)
				}
			},
		)
	}
}

func TestClassifyMixture_StotSE3_NarcoticCodeSelected(t *testing.T) {
	// A Cat-3 component carrying H336 (narcotic) must surface H336, not the H335 default.
	mix := ClassifyMixture([]ClassComponent{comp("a", 25, hz("STOT SE", "3", "H336"))}, nil, false)
	if !hasStr(mix.Label.HCodes, "H336") || hasStr(mix.Label.HCodes, "H335") {
		t.Fatalf("expected H336 only, got %v", mix.Label.HCodes)
	}
}

// ── STOT repeated exposure (Table 3.9.4) ─────────────────────────────────────

func TestClassifyMixture_StotRE_Boundaries(t *testing.T) {
	tests := []struct {
		name    string
		comps   []ClassComponent
		wantH   string
		wantDng bool
	}{
		{"cat1 at 10pct -> RE1 H372", []ClassComponent{comp("a", 10, hz("STOT RE", "1", "H372"))}, "H372", true},
		{"cat1 at 1.0pct -> RE2 H373", []ClassComponent{comp("a", 1.0, hz("STOT RE", "1", "H372"))}, "H373", false},
		{"cat2 at 10pct -> RE2 H373", []ClassComponent{comp("a", 10, hz("STOT RE", "2", "H373"))}, "H373", false},
		{"cat1 at 0.9pct not classified", []ClassComponent{comp("a", 0.9, hz("STOT RE", "1", "H372"))}, "", false},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(tc.comps, nil, false)
				if tc.wantH == "" {
					if hasStr(mix.Label.HCodes, "H372") || hasStr(mix.Label.HCodes, "H373") {
						t.Fatalf("expected no STOT-RE code, got %v", mix.Label.HCodes)
					}
					return
				}
				if !hasStr(mix.Label.HCodes, tc.wantH) {
					t.Fatalf("HCodes = %v, want %s", mix.Label.HCodes, tc.wantH)
				}
				if !hasStr(mix.Label.Pictograms, "GHS08") {
					t.Fatalf("expected GHS08, got %v", mix.Label.Pictograms)
				}
			},
		)
	}
}

// ── Aspiration (Σ ≥ 10% + viscosity gate) ────────────────────────────────────

func TestClassifyMixture_Aspiration_ViscosityGateAndFlag(t *testing.T) {
	comps := []ClassComponent{comp("a", 10, hz("Aspiration hazard", "1", "H304"))}

	// Viscosity unconfirmed → classify conservatively AND raise a data-missing FLAG.
	pending := ClassifyMixture(comps, nil, false)
	if !hasStr(pending.Label.HCodes, "H304") || !hasStr(pending.Label.Pictograms, "GHS08") {
		t.Fatalf("expected Asp Tox 1 (H304/GHS08), got %v / %v", pending.Label.HCodes, pending.Label.Pictograms)
	}
	if pending.Label.SignalWord != "Danger" {
		t.Fatalf("Asp Tox 1 → Danger, got %q", pending.Label.SignalWord)
	}
	if len(pending.Flags) != 1 || pending.Flags[0].Code != FlagDataMissing || pending.Flags[0].Section != "9.1" {
		t.Fatalf("expected one data_missing FLAG on 9.1, got %+v", pending.Flags)
	}

	// Viscosity confirmed low → same classification, no FLAG.
	confirmed := ClassifyMixture(comps, nil, true)
	if !hasStr(confirmed.Label.HCodes, "H304") {
		t.Fatalf("expected H304, got %v", confirmed.Label.HCodes)
	}
	if len(confirmed.Flags) != 0 {
		t.Fatalf("viscosity known → no FLAG, got %+v", confirmed.Flags)
	}

	// Below 10% → no aspiration outcome and no FLAG, regardless of viscosity.
	below := ClassifyMixture([]ClassComponent{comp("a", 9.9, hz("Aspiration hazard", "1", "H304"))}, nil, false)
	if hasStr(below.Label.HCodes, "H304") || len(below.Flags) != 0 {
		t.Fatalf("below 10%% must not classify or flag, got %v / %+v", below.Label.HCodes, below.Flags)
	}
}

// ── Flammable liquid (flash-point gated, Annex I §2.6) ───────────────────────

func TestClassifyMixture_Flammable_FlashPointBoundaries(t *testing.T) {
	fp := func(v float64) *float64 { return &v }
	tests := []struct {
		name      string
		flash     *float64
		wantH     string
		wantPict  string
		wantDng   bool
		wantFlags int
	}{
		{"unknown flash point → engine silent", nil, "", "", false, 0},
		{"flash 70C → not flammable", fp(70), "", "", false, 0},
		{"flash 60C → Flam Liq 3 H226", fp(60), "H226", "GHS02", false, 0},
		{"flash 23C → Flam Liq 3 (boundary)", fp(23), "H226", "GHS02", false, 0},
		{"flash 22.9C → Flam Liq 2 H225 + FLAG", fp(22.9), "H225", "GHS02", true, 1},
	}
	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				mix := ClassifyMixture(nil, tc.flash, false)
				if len(mix.Flags) != tc.wantFlags {
					t.Fatalf("flags = %+v, want %d", mix.Flags, tc.wantFlags)
				}
				if tc.wantH == "" {
					if hasStr(mix.Label.HCodes, "H225") || hasStr(mix.Label.HCodes, "H226") {
						t.Fatalf("expected no flammable code, got %v", mix.Label.HCodes)
					}
					return
				}
				if !hasStr(mix.Label.HCodes, tc.wantH) || !hasStr(mix.Label.Pictograms, tc.wantPict) {
					t.Fatalf("got %v / %v, want %s / %s", mix.Label.HCodes, mix.Label.Pictograms, tc.wantH, tc.wantPict)
				}
				wantWord := "Warning"
				if tc.wantDng {
					wantWord = "Danger"
				}
				if mix.Label.SignalWord != wantWord {
					t.Fatalf("SignalWord = %q, want %q", mix.Label.SignalWord, wantWord)
				}
			},
		)
	}
}

// ── Aquatic outcome mapping (delegated to ClassifyAquatic) ───────────────────

func TestClassifyMixture_Aquatic_OutcomeMapping(t *testing.T) {
	acute := ClassifyMixture(
		[]ClassComponent{{Name: "a", ConcentrationPct: 25, AquaticAcuteCategory: "1"}},
		nil,
		false,
	)
	if !hasStr(acute.Label.HCodes, "H400") || !hasStr(acute.Label.Pictograms, "GHS09") {
		t.Fatalf("Aquatic Acute 1 → H400/GHS09, got %v / %v", acute.Label.HCodes, acute.Label.Pictograms)
	}

	// Aquatic Chronic 2 carries the H411 statement but NO pictogram and NO
	// signal word (CLP Annex I Table 4.1.0 / Annex V) — only Acute 1 / Chronic 1
	// carry GHS09.
	chron2 := ClassifyMixture(
		[]ClassComponent{{Name: "a", ConcentrationPct: 25, AquaticChronicCategory: "2"}},
		nil,
		false,
	)
	if !hasStr(chron2.Label.HCodes, "H411") {
		t.Fatalf("Aquatic Chronic 2 → expected H411, got %v", chron2.Label.HCodes)
	}
	if len(chron2.Label.Pictograms) != 0 {
		t.Fatalf("Aquatic Chronic 2 → no pictogram (no GHS09), got %v", chron2.Label.Pictograms)
	}
	if chron2.Label.SignalWord != "" {
		t.Fatalf("Aquatic Chronic 2 alone → no signal word, got %q", chron2.Label.SignalWord)
	}

	// Chronic 3 carries no pictogram and no signal word.
	chron3 := ClassifyMixture(
		[]ClassComponent{{Name: "a", ConcentrationPct: 25, AquaticChronicCategory: "3"}},
		nil,
		false,
	)
	if !hasStr(chron3.Label.HCodes, "H412") {
		t.Fatalf("expected H412, got %v", chron3.Label.HCodes)
	}
	if len(chron3.Label.Pictograms) != 0 {
		t.Fatalf("Chronic 3 → no pictogram, got %v", chron3.Label.Pictograms)
	}
}

// ── EUH208 sub-threshold disclosure ──────────────────────────────────────────

func TestClassifyMixture_EUH208_SubThresholdDisclosure(t *testing.T) {
	// Skin sensitiser 1B at 0.5%: below the 1% H317 limit but at/above the EUH208
	// band (limit/10 = 0.1%). The mixture is NOT Skin Sens classified; EUH208 is
	// disclosed and the label carries no H317 and no pictogram.
	mix := ClassifyMixture(
		[]ClassComponent{{Name: "a", ConcentrationPct: 0.5, SkinSensCategory: "1B"}},
		nil,
		false,
	)
	if hasStr(mix.Label.HCodes, "H317") {
		t.Fatalf("0.5%% sensitiser must not classify H317, got %v", mix.Label.HCodes)
	}
	if !hasStr(mix.Label.EuhCodes, "EUH208") {
		t.Fatalf("expected EUH208 disclosure, got %v", mix.Label.EuhCodes)
	}
	if len(mix.Label.Pictograms) != 0 || mix.Label.SignalWord != "" {
		t.Fatalf(
			"sub-threshold sensitiser → no pictogram/signal, got %v / %q",
			mix.Label.Pictograms,
			mix.Label.SignalWord,
		)
	}
}

func TestClassifyMixture_EUH208_SuppressedWhenClassified(t *testing.T) {
	// A sensitiser at or above its H317 limit classifies the mixture Skin Sens. 1
	// (H317). EUH208 must NOT also appear for that same substance — H317
	// supersedes the supplementary statement (CLP Annex II §2.8). A label bearing
	// both H317 and EUH208 for the one sensitiser is not a valid CLP label.
	mix := ClassifyMixture(
		[]ClassComponent{
			{Name: "linalool", ConcentrationPct: 2.0, SkinSensCategory: "1B"},
		}, nil, false,
	)
	if !hasStr(mix.Label.HCodes, "H317") {
		t.Fatalf("2%% sensitiser must classify H317, got %v", mix.Label.HCodes)
	}
	if hasStr(mix.Label.EuhCodes, "EUH208") {
		t.Fatalf("EUH208 must be suppressed when H317 applies for the same substance, got %v", mix.Label.EuhCodes)
	}
}

func TestClassifyMixture_EUH208_NamesOtherBandSensitiserAlongsideH317(t *testing.T) {
	// One sensitiser at/above its limit classifies the mixture (H317); a DIFFERENT
	// sensitiser sitting in the [limit/10, limit) band is still disclosed via
	// EUH208. The two statements coexist because they refer to different substances.
	mix := ClassifyMixture(
		[]ClassComponent{
			{Name: "classifier", ConcentrationPct: 2.0, SkinSensCategory: "1B"},
			{Name: "band", ConcentrationPct: 0.5, SkinSensCategory: "1B"},
		}, nil, false,
	)
	if !hasStr(mix.Label.HCodes, "H317") {
		t.Fatalf("expected H317 from the classifying sensitiser, got %v", mix.Label.HCodes)
	}
	if !hasStr(mix.Label.EuhCodes, "EUH208") {
		t.Fatalf("expected EUH208 naming the sub-threshold sensitiser, got %v", mix.Label.EuhCodes)
	}
}

// ── Pictogram precedence (CLP Annex I §1.3.4) ────────────────────────────────

func TestClassifyMixture_Precedence_GHS06SuppressesGHS07(t *testing.T) {
	// Acute Tox 2 (oral, GHS06) co-occurring with a skin sensitiser (GHS07).
	// §1.3.4(a): GHS06 unconditionally removes GHS07, even when GHS07 stands for
	// skin sensitisation. H-codes are unaffected — only the pictogram is dropped.
	mix := ClassifyMixture(
		[]ClassComponent{
			{Name: "tox", ConcentrationPct: 50, Hazards: []ComponentHazard{hz("Acute Tox. 2 (oral)", "2", "H300")}},
			{Name: "sens", ConcentrationPct: 5, SkinSensCategory: "1B"},
		}, nil, false,
	)
	if hasStr(mix.Label.Pictograms, "GHS07") {
		t.Fatalf("GHS06 must suppress GHS07, got %v", mix.Label.Pictograms)
	}
	if !hasStr(mix.Label.Pictograms, "GHS06") {
		t.Fatalf("expected GHS06 retained, got %v", mix.Label.Pictograms)
	}
	if !hasStr(mix.Label.HCodes, "H300") || !hasStr(mix.Label.HCodes, "H317") {
		t.Fatalf("expected both H300 and H317 on label, got %v", mix.Label.HCodes)
	}
}

func TestClassifyMixture_Precedence_GHS05SuppressesIrritationGHS07Only(t *testing.T) {
	// §1.3.4(b): GHS05 drops GHS07 only where it stands for skin/eye irritation.

	// Eye Dam 1 (GHS05) + Skin Irrit 2 (GHS07 for irritation) → GHS07 dropped.
	dropped := ClassifyMixture(
		[]ClassComponent{
			comp("dam", 3.0, hz("Eye Dam. 1", "1", "H318")),
			comp("irr", 10.0, hz("Skin Irrit. 2", "2", "H315")),
		}, nil, false,
	)
	if hasStr(dropped.Label.Pictograms, "GHS07") {
		t.Fatalf("GHS05 must suppress irritation GHS07, got %v", dropped.Label.Pictograms)
	}
	if !hasStr(dropped.Label.Pictograms, "GHS05") {
		t.Fatalf("expected GHS05, got %v", dropped.Label.Pictograms)
	}
	if !hasStr(dropped.Label.HCodes, "H315") || !hasStr(dropped.Label.HCodes, "H318") {
		t.Fatalf("expected H315 and H318 retained, got %v", dropped.Label.HCodes)
	}

	// Eye Dam 1 (GHS05) + skin sensitiser (GHS07 for sensitisation) → GHS07 kept,
	// because (b) only suppresses irritation GHS07.
	kept := ClassifyMixture(
		[]ClassComponent{
			comp("dam", 3.0, hz("Eye Dam. 1", "1", "H318")),
			{Name: "sens", ConcentrationPct: 5, SkinSensCategory: "1B"},
		}, nil, false,
	)
	if !sameStrSet(kept.Label.Pictograms, []string{"GHS05", "GHS07"}) {
		t.Fatalf("expected GHS05+GHS07, got %v", kept.Label.Pictograms)
	}
}

// ── End-to-end: reproduce the reference SDS label shape ──────────────────────

func TestClassifyMixture_ReferenceLabel_Integration(t *testing.T) {
	// A single fragrance compound at 30% that is skin-sensitising, skin/eye
	// irritating, and chronically aquatic-toxic (Chronic 2) — the classic
	// finished-fragrance label: GHS07, Warning, H317 + H315 + H319 + H411.
	// Aquatic Chronic 2 contributes the H411 statement but NO pictogram (no
	// GHS09) and no signal word of its own.
	frag := ClassComponent{
		Name:                   "Fragrance compound",
		ConcentrationPct:       30,
		SkinSensCategory:       "1B",
		AquaticChronicCategory: "2",
		Hazards: []ComponentHazard{
			hz("Skin Irrit. 2", "2", "H315"),
			hz("Eye Irrit. 2", "2", "H319"),
		},
	}
	mix := ClassifyMixture([]ClassComponent{frag}, nil, false)

	if !sameStrSet(mix.Label.Pictograms, []string{"GHS07"}) {
		t.Fatalf("Pictograms = %v, want GHS07 only (Aquatic Chronic 2 carries no GHS09)", mix.Label.Pictograms)
	}
	if mix.Label.SignalWord != "Warning" {
		t.Fatalf("SignalWord = %q, want Warning", mix.Label.SignalWord)
	}
	if !sameStrSet(mix.Label.HCodes, []string{"H317", "H315", "H319", "H411"}) {
		t.Fatalf("HCodes = %v, want H317/H315/H319/H411", mix.Label.HCodes)
	}
	if len(mix.Label.PCodes) == 0 {
		t.Fatalf("expected precautionary statements, got none")
	}
}

// ── Degenerate inputs ────────────────────────────────────────────────────────

func TestClassifyMixture_NonHazardousProducesEmptyLabel(t *testing.T) {
	for _, comps := range [][]ClassComponent{
		nil,
		{comp("water", 80), comp("inert", 20)},
	} {
		mix := ClassifyMixture(comps, nil, false)
		if len(mix.Label.Pictograms) != 0 || len(mix.Label.HCodes) != 0 ||
			len(mix.Label.EuhCodes) != 0 || mix.Label.SignalWord != "" || len(mix.Flags) != 0 {
			t.Fatalf("non-hazardous mixture must yield empty label, got %+v", mix)
		}
	}
}

// ── Regression: CLP sub-categories and the STOT differentiation ──────────────

// CLP Annex I Table 3.2.3 classifies skin corrosion as 1A/1B/1C, and all three
// sum against the single Skin Corr. 1 limit (5%). The summation itself reads
// only the KIND (sumByKind/componentHasKind never consult the category), so the
// classification was always correct — but the method table's GCL map was keyed
// on "1" alone, and validateEndpointsEvaluated looks the component's ACTUAL
// category up via gclFor. A supplier stating "Skin Corr." / "1B" (which is what
// the live production data carries) therefore drew an endpoint_not_evaluated
// flag asserting the hazard "was silently dropped from the classification" when
// it had not been. This guards the table against that false alarm — a review
// flag that cries wolf is how a real one gets clicked through.
func TestSkinCorrosionSubCategoriesResolveAndRaiseNoFalseFlag(t *testing.T) {
	for _, cat := range []string{"1", "1A", "1B", "1C"} {
		if limit, ok := gclFor(kindSkinCorr, cat); !ok || limit != gclSkinCorr1 {
			t.Errorf("gclFor(Skin Corr., %q) = (%v, %v), want (%v, true)", cat, limit, ok, gclSkinCorr1)
		}

		c := comp("corrosive", 6, hz("Skin Corr.", cat, "H314"))
		if flags := validateEndpointsEvaluated([]ClassComponent{c}); len(flags) != 0 {
			t.Errorf("Skin Corr. %s must be reported as evaluated, got %+v", cat, flags)
		}
		// The classification is unaffected by the table key (summation is by kind),
		// so it must keep working for every sub-category: 6% ≥ the 5% limit → H314.
		if mix := ClassifyMixture([]ClassComponent{c}, nil, false); !hasStr(mix.Label.HCodes, "H314") {
			t.Errorf("Skin Corr. %s at 6%%: want H314 (Σ ≥ 5%%), got %v", cat, mix.Label.HCodes)
		}
	}
}

// classKind must tell the single-exposure differentiation of STOT from the
// repeated-exposure one on the phrasings suppliers actually write. The failure
// that motivated this: a bare " re" substring test also matches " respiratory",
// and "respiratory tract irritation" is the standard qualifier on STOT SE 3 —
// so H335 was routed into the repeated-exposure table, which has no Cat 3, and
// vanished from the classification.
func TestClassKind_StotSingleVsRepeatedExposure(t *testing.T) {
	tests := []struct {
		class string
		want  hazardKind
	}{
		// The exact string in production data that exposed the defect.
		{"Specific target organ toxicity, Single exposure, Respiratory tract irritation", kindStotSE},
		{"STOT SE 3 - respiratory tract irritation", kindStotSE},
		{"Specific target organ toxicity - single exposure", kindStotSE},
		{"Specific target organ toxicity, single exposure", kindStotSE},
		{"STOT SE", kindStotSE},
		{"STOT SE 3", kindStotSE},
		// Repeated exposure, including the hyphenated abbreviation.
		{"STOT RE 2", kindStotRE},
		{"STOT-RE 2", kindStotRE},
		{"Specific target organ toxicity - repeated exposure", kindStotRE},
		{"Specific target organ toxicity (repeated exposure)", kindStotRE},
	}
	for _, tt := range tests {
		if got := classKind(tt.class); got != tt.want {
			t.Errorf("classKind(%q) = %d, want %d", tt.class, got, tt.want)
		}
	}
}

// End-to-end: a Cat-3 respiratory irritant over the 20% STOT SE 3 limit must
// reach the label. Before the fix this produced an empty label, because the
// class string was bucketed as repeated exposure where no Cat 3 limit exists.
func TestClassifyMixture_StotSE3RespiratoryIrritantReachesLabel(t *testing.T) {
	mix := ClassifyMixture(
		[]ClassComponent{
			comp(
				"irritant", 25,
				hz("Specific target organ toxicity, Single exposure, Respiratory tract irritation", "3", "H335"),
			),
		}, nil, false,
	)
	if !hasStr(mix.Label.HCodes, "H335") {
		t.Fatalf("STOT SE 3 respiratory irritant at 25%%: want H335 on the label, got %v", mix.Label.HCodes)
	}
}

// Every canonical CLP abbreviation must resolve to a real endpoint. These are
// the forms Annex VI, supplier SDSs, and this engine's OWN classifier display
// strings use, so any that falls through to kindOther is a hazard the engine
// emits but cannot read back — it contributes to no summation and reaches no
// limit, and the only trace is an endpoint_not_evaluated flag. Three of these
// (Carc., Asp. Tox., Lact.) were live in production data before this guard.
func TestClassKind_CanonicalCLPAbbreviationsAllResolve(t *testing.T) {
	abbrevs := []string{
		"Acute Tox. 4", "Skin Corr. 1B", "Skin Irrit. 2", "Eye Dam. 1", "Eye Irrit. 2",
		"Skin Sens. 1", "Skin Sens. 1A", "Resp. Sens. 1", "Carc. 1B", "Carc. 2",
		"Muta. 2", "Repr. 2", "Lact.", "STOT SE 3", "STOT RE 2", "Asp. Tox. 1",
		"Aquatic Acute 1", "Aquatic Chronic 2",
	}
	for _, a := range abbrevs {
		if got := hazardEndpoint(ComponentHazard{Class: a}); got == kindOther {
			t.Errorf("hazardEndpoint(%q) = kindOther — the engine cannot read back an abbreviation it emits", a)
		}
	}
}

// End-to-end for the abbreviated CMR form: a supplier writing the canonical
// "Carc. 1B" must classify exactly as one writing "Carcinogenicity". 0.5% is
// above the 0.1% Cat-1 limit, so H350 is required either way.
func TestClassifyMixture_AbbreviatedCarcinogenIsClassified(t *testing.T) {
	for _, class := range []string{"Carc.", "Carcinogenicity"} {
		mix := ClassifyMixture(
			[]ClassComponent{comp("cmr", 0.5, hz(class, "1B", "H350"))}, nil, false,
		)
		if !hasStr(mix.Label.HCodes, "H350") {
			t.Errorf("%q 1B at 0.5%% (limit 0.1%%): want H350, got %v", class, mix.Label.HCodes)
		}
		if mix.Label.SignalWord != "Danger" {
			t.Errorf("%q 1B: want Danger signal word, got %q", class, mix.Label.SignalWord)
		}
	}
}
