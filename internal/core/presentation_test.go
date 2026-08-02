package core

import (
	"testing"
)

func TestConcentrationBand_Boundaries(t *testing.T) {
	cases := []struct {
		pct  float64
		want string
	}{
		{0, ""},
		{-1, ""},
		{0.05, "< 0.1 %"},
		{0.1, "≥ 0.1 - < 1 %"},
		{0.99, "≥ 0.1 - < 1 %"},
		{1, "≥ 1 - < 5 %"},
		{4.99, "≥ 1 - < 5 %"},
		{5, "≥ 5 - < 10 %"},
		{9.99, "≥ 5 - < 10 %"},
		{10, "≥ 10 - < 25 %"},
		{25, "≥ 25 - < 50 %"},
		{50, "≥ 50 - < 100 %"},
		{99.99, "≥ 50 - < 100 %"},
		{100, "100 %"},
		{150, "100 %"},
	}
	for _, c := range cases {
		if got := concentrationBand(c.pct); got != c.want {
			t.Errorf("concentrationBand(%v) = %q, want %q", c.pct, got, c.want)
		}
	}
}

func TestFormatClassification(t *testing.T) {
	got := formatClassification(
		[]ComponentHazard{
			{Class: "Skin Sens.", Category: "1B", HCodes: []string{"H317"}},
			{Class: "Aquatic Chronic", Category: "2", HCodes: []string{"H411"}},
		},
	)
	if want := "Skin Sens. 1B (H317); Aquatic Chronic 2 (H411)"; got != want {
		t.Errorf("formatClassification = %q, want %q", got, want)
	}
	if formatClassification(nil) != "" {
		t.Error("nil hazards must format to empty string")
	}
	// Multiple H-codes on one hazard join inside the parentheses.
	if got := formatClassification(
		[]ComponentHazard{
			{
				Class: "Acute Tox.", Category: "4", HCodes: []string{"H302", "H312"},
			},
		},
	); got != "Acute Tox. 4 (H302, H312)" {
		t.Errorf("multi-code format = %q", got)
	}

	// Supplier LONG-form class names are normalised to the canonical CLP
	// abbreviation, with the category preserved.
	if got := formatClassification(
		[]ComponentHazard{
			{Class: "Skin irritation", Category: "2", HCodes: []string{"H315"}},
			{Class: "Eye irritation", Category: "2", HCodes: []string{"H319"}},
			{Class: "Reproductive toxicity", Category: "2", HCodes: []string{"H361"}},
			{Class: "Aspiration hazard", Category: "1", HCodes: []string{"H304"}},
			{Class: "Hazardous to the aquatic environment - chronic", Category: "1", HCodes: []string{"H410"}},
		},
	); got != "Skin Irrit. 2 (H315); Eye Irrit. 2 (H319); Repr. 2 (H361); Asp. Tox. 1 (H304); Aquatic Chronic 1 (H410, M=1)" {
		t.Errorf("long-form normalisation = %q", got)
	}

	// The supplier repeats a substance's whole code list on every hazard row;
	// codes are filtered to their class family so each class shows only its own.
	if got := formatClassification(
		[]ComponentHazard{
			{
				Class: "Hazardous to the aquatic environment - chronic", Category: "1",
				HCodes: []string{"H410", "H315", "H317"},
			},
			{Class: "Skin irritation", Category: "2", HCodes: []string{"H410", "H315", "H317"}},
			{Class: "Skin sensitisation", Category: "1", HCodes: []string{"H410", "H315", "H317"}},
		},
	); got != "Aquatic Chronic 1 (H410, M=1); Skin Irrit. 2 (H315); Skin Sens. 1 (H317)" {
		t.Errorf("family code filtering = %q", got)
	}

	// An unknown class is kept verbatim (no data lost) with its codes.
	if got := formatClassification(
		[]ComponentHazard{
			{
				Class: "Pyrophoric solid", Category: "1", HCodes: []string{"H250"},
			},
		},
	); got != "Pyrophoric solid 1 (H250)" {
		t.Errorf("unknown class passthrough = %q", got)
	}

	// Sub-class H-code suffixes (H361fd, H350i) are preserved in the display
	// while the family match still resolves via the three-digit base.
	if got := formatClassification(
		[]ComponentHazard{
			{
				Class: "Reproductive toxicity", Category: "2", HCodes: []string{"H361fd", "H317"},
			},
		},
	); got != "Repr. 2 (H361fd)" {
		t.Errorf("suffix preservation/family filter = %q", got)
	}

	// A hazard with an empty class but real H-codes must not be dropped — the
	// class is derived from the codes so the §3 cell never loses a stated hazard.
	if got := formatClassification(
		[]ComponentHazard{
			{
				Class: "", Category: "2", HCodes: []string{"H319"},
			},
		},
	); got != "Eye Irrit. 2 (H319)" {
		t.Errorf("empty-class fallback = %q", got)
	}

	// Aquatic category-1 hazards state their M-factor (REACH Annex II §3.2):
	// default M=1 when the supplier assigned none, the explicit value otherwise.
	// Category 2/3/4 (no M-factor) carry none — covered by the Chronic 2 case above.
	if got := formatClassification(
		[]ComponentHazard{
			{Class: "Hazardous to the aquatic environment - acute", Category: "1", HCodes: []string{"H400"}},
		},
	); got != "Aquatic Acute 1 (H400, M=1)" {
		t.Errorf("aquatic acute default M = %q", got)
	}
	if got := formatClassification(
		[]ComponentHazard{
			{
				Class: "Hazardous to the aquatic environment - chronic", Category: "1",
				HCodes: []string{"H410"}, MFactorChronic: 10,
			},
		},
	); got != "Aquatic Chronic 1 (H410, M=10)" {
		t.Errorf("aquatic chronic explicit M = %q", got)
	}
}

func TestInciSynonym(t *testing.T) {
	cases := []struct {
		name string
		inci string
		want string
	}{
		{"Limonene", "", ""},                     // no INCI on file
		{"d-Limonene", "Limonene", "Limonene"},   // differs → show
		{"Linalool", "linalool", ""},             // same (case-insensitive) → redundant
		{"  Citral ", "Citral", ""},              // same after trim → redundant
		{"Benzyl alcohol", "Benzyl Alcohol", ""}, // case-only difference → redundant (EqualFold)
	}
	for _, c := range cases {
		if got := inciSynonym(c.name, c.inci); got != c.want {
			t.Errorf("inciSynonym(%q,%q) = %q, want %q", c.name, c.inci, got, c.want)
		}
	}
}

func TestApplyInciName(t *testing.T) {
	reg := map[string]string{"78-70-6": "Linalool", "5989-27-5": "Limonene"}

	// Matched CAS gets its INCI name.
	cc := ClassComponent{Name: "Linalol", CasNumber: "78-70-6"}
	applyInciName(&cc, reg)
	if cc.InciName != "Linalool" {
		t.Errorf("matched CAS InciName = %q, want Linalool", cc.InciName)
	}

	// Unmatched CAS, blank CAS, and a nil registry all leave InciName empty.
	for _, tc := range []struct {
		label string
		cc    ClassComponent
		reg   map[string]string
	}{
		{"unmatched CAS", ClassComponent{CasNumber: "00-00-0"}, reg},
		{"blank CAS", ClassComponent{CasNumber: ""}, reg},
		{"nil registry", ClassComponent{CasNumber: "78-70-6"}, nil},
	} {
		got := tc.cc
		applyInciName(&got, tc.reg)
		if got.InciName != "" {
			t.Errorf("%s: InciName = %q, want empty", tc.label, got.InciName)
		}
	}
}

func TestCompositionRows_SurfacesInciSynonym(t *testing.T) {
	haz := []ComponentHazard{{Class: "Skin Sens.", Category: "1", HCodes: []string{"H317"}}}
	comps := []ClassificationComponent{
		// Differs from name → synonym shown.
		{Name: "d-Limonene", CasNumber: "5989-27-5", InciName: "Limonene", ConcentrationPct: 2, Hazards: haz},
		// Equals name → no redundant synonym.
		{Name: "Linalool", CasNumber: "78-70-6", InciName: "Linalool", ConcentrationPct: 1, Hazards: haz},
		// No INCI on file.
		{Name: "Geraniol", CasNumber: "106-24-1", ConcentrationPct: 1, Hazards: haz},
		// No hazards → excluded from §3 entirely (so its INCI never renders).
		{Name: "Water", CasNumber: "7732-18-5", InciName: "Aqua", ConcentrationPct: 90},
	}
	rows := compositionRows(comps)
	if len(rows) != 3 {
		t.Fatalf("expected 3 hazardous rows, got %d", len(rows))
	}
	if rows[0].InciName != "Limonene" {
		t.Errorf("row[0] InciName = %q, want Limonene", rows[0].InciName)
	}
	if rows[1].InciName != "" {
		t.Errorf("row[1] (name==inci) InciName = %q, want empty", rows[1].InciName)
	}
	if rows[2].InciName != "" {
		t.Errorf("row[2] (no inci) InciName = %q, want empty", rows[2].InciName)
	}
}

func TestBuildPresentation_ResolvesLabelBothLanguages(t *testing.T) {
	cls := SdsClassification{
		Label: SdsLabel{
			Pictograms: []string{"GHS07", "GHS09"},
			SignalWord: "Warning",
			HCodes:     []string{"H317", "H411"},
			EuhCodes:   []string{"EUH208"},
			PCodes:     []string{"P273"},
		},
		Components: []ClassificationComponent{
			{
				Name:             "Linalool",
				CasNumber:        "78-70-6",
				EcNumber:         "201-134-4",
				ConcentrationPct: 2.5,
				Hazards: []ComponentHazard{
					{
						Class: "Skin Sens.", Category: "1B", HCodes: []string{"H317"},
					},
				},
			},
			{Name: "Water", ConcentrationPct: 60}, // non-hazardous → excluded from §3
		},
	}
	p := BuildPresentation(cls)

	for _, lc := range []struct {
		name string
		v    SdsPresentationLang
	}{{"el", p.EL}, {"en", p.EN}} {
		if len(lc.v.HStatements) != 2 || lc.v.HStatements[0].Code != "H317" {
			t.Fatalf("%s: H statements not resolved: %+v", lc.name, lc.v.HStatements)
		}
		if lc.v.HStatements[0].Text == "" {
			t.Errorf("%s: H317 text empty (phrase library not wired)", lc.name)
		}
		if len(lc.v.EuhStatements) != 1 || lc.v.EuhStatements[0].Code != "EUH208" {
			t.Errorf("%s: EUH not resolved: %+v", lc.name, lc.v.EuhStatements)
		}
		if len(lc.v.PStatements) != 1 || lc.v.PStatements[0].Code != "P273" {
			t.Errorf("%s: P not resolved: %+v", lc.name, lc.v.PStatements)
		}
		if len(lc.v.Pictograms) != 2 || lc.v.Pictograms[0].Code != "GHS07" || lc.v.Pictograms[0].Alt == "" {
			t.Errorf("%s: pictograms not resolved: %+v", lc.name, lc.v.Pictograms)
		}
		if lc.v.SignalWord == "" {
			t.Errorf("%s: signal word empty", lc.name)
		}
		if len(lc.v.Components) != 1 || lc.v.Components[0].Name != "Linalool" {
			t.Fatalf("%s: §3 must list only the hazardous substance, got %+v", lc.name, lc.v.Components)
		}
		row := lc.v.Components[0]
		if row.CasNumber != "78-70-6" || row.EcNumber != "201-134-4" {
			t.Errorf("%s: identity not carried: %+v", lc.name, row)
		}
		if row.ConcentrationRange != "≥ 1 - < 5 %" {
			t.Errorf("%s: band = %q, want banded (never exact %%)", lc.name, row.ConcentrationRange)
		}
	}

	// Resolution is genuinely localised: the H317 statement and the signal word
	// differ between Greek and English.
	if p.EL.HStatements[0].Text == p.EN.HStatements[0].Text {
		t.Errorf("H317 text must differ EL vs EN, both = %q", p.EL.HStatements[0].Text)
	}
	if p.EL.SignalWord == p.EN.SignalWord {
		t.Errorf("signal word must differ EL vs EN (%q == %q)", p.EL.SignalWord, p.EN.SignalWord)
	}
}

func TestBuildPresentation_UnclassifiedIsEmpty(t *testing.T) {
	p := BuildPresentation(SdsClassification{})
	if p.EL.SignalWord != "" || len(p.EL.Pictograms) != 0 || len(p.EL.HStatements) != 0 ||
		len(p.EL.Components) != 0 || len(p.EL.Glossary) != 0 {
		t.Errorf("unclassified mixture → empty presentation, got %+v", p.EL)
	}
}

func TestClassComponentsToStored_CarriesIdentity(t *testing.T) {
	stored := classComponentsToStored(
		[]ClassComponent{
			{
				Name: "Linalool", CasNumber: "78-70-6", EcNumber: "201-134-4", ConcentrationPct: 2.5,
				Hazards: []ComponentHazard{{Class: "Skin Sens.", Category: "1B"}},
			},
		},
	)
	if len(stored) != 1 {
		t.Fatalf("want 1 stored component, got %d", len(stored))
	}
	s := stored[0]
	if s.Name != "Linalool" || s.CasNumber != "78-70-6" || s.EcNumber != "201-134-4" || s.ConcentrationPct != 2.5 || len(s.Hazards) != 1 {
		t.Errorf("identity not preserved: %+v", s)
	}
}

// TestBuildGlossary_OrdersDedupesAndAttachesPictograms locks the end-of-document
// glossary: every code shown anywhere — the §2 label codes AND the §3 component
// codes — in H→EUH→P order, de-duplicated, each with resolved text, and a GHS
// pictogram attached ONLY to H-codes that carry one (EUH/P and no-pictogram
// H-codes carry none). Both languages are built and the text must be localised.
func TestBuildGlossary_OrdersDedupesAndAttachesPictograms(t *testing.T) {
	cls := SdsClassification{
		Label: SdsLabel{
			HCodes:   []string{"H226", "H317", "H412", "H317"}, // H317 duplicated; H412 carries no pictogram
			EuhCodes: []string{"EUH208"},
			PCodes:   []string{"P210", "P273"},
		},
		Components: []ClassificationComponent{
			// A component discloses H302, which is NOT on the mixture label: it must
			// still earn a glossary entry, appended after the label H-codes (before
			// EUH). H317 here is already on the label, so it must NOT duplicate.
			{
				Name: "X", ConcentrationPct: 2, Hazards: []ComponentHazard{
					{Class: "Acute Tox.", Category: "4", HCodes: []string{"H302", "H317"}},
				},
			},
		},
	}
	p := BuildPresentation(cls)

	wantOrder := []string{"H226", "H317", "H412", "H302", "EUH208", "P210", "P273"}
	wantPicto := map[string]string{
		"H226": "GHS02", "H317": "GHS07", "H412": "", "H302": "GHS07", "EUH208": "", "P210": "", "P273": "",
	}

	for _, lc := range []struct {
		name string
		v    SdsPresentationLang
	}{{"el", p.EL}, {"en", p.EN}} {
		g := lc.v.Glossary
		if len(g) != len(wantOrder) {
			t.Fatalf("%s: glossary len = %d, want %d (deduped H317): %+v", lc.name, len(g), len(wantOrder), g)
		}
		for i, code := range wantOrder {
			if g[i].Code != code {
				t.Errorf(
					"%s: glossary[%d].FormulationNumber = %q, want %q (H→EUH→P order)",
					lc.name,
					i,
					g[i].Code,
					code,
				)
			}
			if g[i].Text == "" {
				t.Errorf("%s: glossary[%d] (%s) has empty text", lc.name, i, code)
			}
			if g[i].Pictogram != wantPicto[code] {
				t.Errorf(
					"%s: glossary[%d] (%s) pictogram = %q, want %q",
					lc.name,
					i,
					code,
					g[i].Pictogram,
					wantPicto[code],
				)
			}
		}
	}

	// Glossary text is genuinely localised (H317 differs EL vs EN).
	if p.EL.Glossary[1].Code != "H317" || p.EL.Glossary[1].Text == p.EN.Glossary[1].Text {
		t.Errorf("H317 glossary text must differ EL vs EN (%q vs %q)", p.EL.Glossary[1].Text, p.EN.Glossary[1].Text)
	}
}
