package core

import (
	"strings"
	"testing"
)

// TestHStatementText covers the canonical lookup, case/trim-insensitivity, the
// EL/EN language split, the EL fallback for an unknown Lang, and the unknown-code
// contract (returns false so the caller FLAGs for manual entry).
func TestHStatementText(t *testing.T) {
	// H317 Greek skin-sensitisation text, copied verbatim from the reference SDS.
	if got, ok := hStatementText(
		"H317",
		LangEL,
	); !ok || got != "Μπορεί να προκαλέσει αλλεργική δερματική αντίδραση." {
		t.Fatalf("hStatementText(H317, EL) = %q, %v; want the Greek skin-sens text and true", got, ok)
	}
	// English counterpart.
	if got, ok := hStatementText("H317", LangEN); !ok || got != "May cause an allergic skin reaction." {
		t.Fatalf("hStatementText(H317, EN) = %q, %v", got, ok)
	}
	// Lowercase + surrounding whitespace must still resolve.
	if got, ok := hStatementText("  h317 ", LangEL); !ok || got == "" {
		t.Fatalf("hStatementText with messy casing/space failed: %q, %v", got, ok)
	}
	// Unknown Lang falls back to Greek.
	if got, _ := hStatementText("H315", Lang("fr")); got != "Προκαλεί ερεθισμό του δέρματος." {
		t.Fatalf("hStatementText(H315, fr) fallback = %q; want Greek", got)
	}
	// Unknown code → ("", false).
	if got, ok := hStatementText("H999", LangEL); ok || got != "" {
		t.Fatalf("hStatementText(H999) = %q, %v; want \"\", false", got, ok)
	}
}

// TestPStatementCombinedSpacing verifies that a combined P-code with spaces
// normalises to the no-space canonical key.
func TestPStatementCombinedSpacing(t *testing.T) {
	want := "ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΠΑΦΗΣ ΜΕ ΤΑ ΜΑΤΙΑ: Ξεπλύνετε προσεκτικά με νερό για αρκετά λεπτά. Αν υπάρχουν φακοί επαφής, αφαιρέστε τους, αν είναι εύκολο. Συνεχίστε να ξεπλένετε."
	canonical, ok := pStatementText("P305+P351+P338", LangEL)
	if !ok {
		t.Fatalf("pStatementText(P305+P351+P338) not found")
	}
	if canonical != want {
		t.Fatalf("pStatementText(P305+P351+P338, EL) = %q; want reference text", canonical)
	}
	// Spaced form must resolve to the identical string.
	spaced, ok := pStatementText("P305 + P351 + P338", LangEL)
	if !ok || spaced != canonical {
		t.Fatalf("spaced P-code normalisation failed: %q, %v", spaced, ok)
	}
}

// TestEuhStatementText spot-checks the EUH208 verbatim header text.
func TestEuhStatementText(t *testing.T) {
	if got, ok := euhStatementText(
		"euh208",
		LangEL,
	); !ok || got != "Περιέχει ένα συστατικό που μπορεί να προκαλέσει αλλεργική αντίδραση." {
		t.Fatalf("euhStatementText(EUH208, EL) = %q, %v", got, ok)
	}
	if _, ok := euhStatementText("EUH000", LangEN); ok {
		t.Fatalf("euhStatementText(EUH000) should be unknown")
	}
}

// TestHazardClassDisplay checks an abbreviated class name (the form the
// classifier emits) resolves in both languages and that whitespace is ignored.
func TestHazardClassDisplay(t *testing.T) {
	if got, ok := hazardClassDisplay("Skin Sens. 1B", LangEL); !ok || got != "Ευαισθητοποίηση του δέρματος 1B" {
		t.Fatalf("hazardClassDisplay(Skin Sens. 1B, EL) = %q, %v", got, ok)
	}
	if got, ok := hazardClassDisplay(
		"aquatic chronic 2",
		LangEN,
	); !ok || got != "Hazardous to the aquatic environment - Chronic 2" {
		t.Fatalf("hazardClassDisplay(Aquatic Chronic 2, EN) = %q, %v", got, ok)
	}
	if _, ok := hazardClassDisplay("Made Up 9", LangEL); ok {
		t.Fatalf("hazardClassDisplay(Made Up 9) should be unknown")
	}
}

// TestSignalWordText checks the case-insensitive Danger/Warning mapping.
func TestSignalWordText(t *testing.T) {
	if got, ok := signalWordText("warning", LangEL); !ok || got != "Προσοχή" {
		t.Fatalf("signalWordText(warning, EL) = %q, %v; want Προσοχή", got, ok)
	}
	if got, ok := signalWordText("DANGER", LangEN); !ok || got != "Danger" {
		t.Fatalf("signalWordText(DANGER, EN) = %q, %v", got, ok)
	}
	if _, ok := signalWordText("caution", LangEL); ok {
		t.Fatalf("signalWordText(caution) should be unknown")
	}
}

// TestPictogramAlt checks a couple of GHS pictogram alt-text lookups.
func TestPictogramAlt(t *testing.T) {
	if got, ok := pictogramAlt("GHS07", LangEL); !ok || got != "Θαυμαστικό" {
		t.Fatalf("pictogramAlt(GHS07, EL) = %q, %v; want Θαυμαστικό", got, ok)
	}
	if got, ok := pictogramAlt("ghs09", LangEN); !ok || got != "Environment" {
		t.Fatalf("pictogramAlt(GHS09, EN) = %q, %v", got, ok)
	}
	if _, ok := pictogramAlt("GHS10", LangEL); ok {
		t.Fatalf("pictogramAlt(GHS10) should be unknown")
	}
}

// TestPictogramForHCode locks the CLP Annex I H-code → GHS pictogram
// correspondence the directive renderer relies on: one representative code per
// pictogram, statements that legitimately carry NO pictogram, and
// case-insensitive matching.
func TestPictogramForHCode(t *testing.T) {
	withPicto := map[string]string{
		"H201":   "GHS01", // explosive
		"H225":   "GHS02", // highly flammable liquid
		"H271":   "GHS03", // strong oxidiser
		"H280":   "GHS04", // gas under pressure
		"H314":   "GHS05", // skin corrosion 1
		"H318":   "GHS05", // serious eye damage 1
		"H301":   "GHS06", // acute tox 3 oral
		"H302":   "GHS07", // acute tox 4 oral
		"H317":   "GHS07", // skin sensitisation 1
		"H319":   "GHS07", // eye irritation 2
		"H335":   "GHS07", // STOT SE 3 respiratory irritation
		"H304":   "GHS08", // aspiration
		"H334":   "GHS08", // respiratory sensitisation
		"H350":   "GHS08", // carcinogenicity
		"H360FD": "GHS08", // reproductive toxicity sub-variant
		"H400":   "GHS09", // aquatic acute 1
		"H410":   "GHS09", // aquatic chronic 1

		"H207":           "GHS02", // desensitised explosive — flame, not bomb
		"H300+H310":      "GHS06", // combined fatal oral+dermal
		"H301+H311+H331": "GHS06", // combined toxic oral+dermal+inhalation
		"H302+H312":      "GHS07", // combined harmful oral+dermal
	}
	for code, want := range withPicto {
		got, ok := pictogramForHCode(code)
		if !ok || got != want {
			t.Errorf("pictogramForHCode(%q) = (%q, %t), want (%q, true)", code, got, ok, want)
		}
	}

	// Statements that carry no pictogram, and non-H codes, must report none.
	for _, code := range []string{
		"H411", // aquatic chronic 2 — no pictogram (CLP Annex V)
		"H412", // aquatic chronic 3 — no pictogram
		"H413", // aquatic chronic 4 — no pictogram
		"H420", // ozone — no pictogram
		"H362", // effects on/via lactation — no pictogram
		"H229", // aerosol container statement — flame comes from H222/H223
		"EUH208",
		"P305+P351+P338",
		"",
		"not-a-code",
	} {
		if got, ok := pictogramForHCode(code); ok {
			t.Errorf("pictogramForHCode(%q) = (%q, true), want no pictogram", code, got)
		}
	}

	// Case- and whitespace-insensitive, including the lower-cased carc/repr suffixes.
	for _, code := range []string{"h317", " H317 ", "H350I", "h350i", "H360fd"} {
		if _, ok := pictogramForHCode(code); !ok {
			t.Errorf("pictogramForHCode(%q) should match case-insensitively", code)
		}
	}
}

// TestPictogramValuesAreKnownGHS asserts every pictogram the map points to is a
// real GHS code defined in the alt-text table, so a directive can always resolve
// the icon and its localised alt text.
func TestPictogramValuesAreKnownGHS(t *testing.T) {
	for code, picto := range hCodePictograms {
		if _, ok := pictogramAlt(picto, LangEN); !ok {
			t.Errorf("%s maps to %q which is not a known GHS pictogram", code, picto)
		}
	}
}

// TestEveryHazardHCodeHasPictogramDecision is the completeness guard for ask (b):
// every H-code in the embedded CLP catalogue must have an explicit pictogram
// decision — either mapped in hCodePictograms, or listed here as a known
// no-pictogram class. A data refresh that adds an H-code we have not classified
// fails here rather than silently rendering a code chip with no icon.
func TestEveryHazardHCodeHasPictogramDecision(t *testing.T) {
	// H-codes that legitimately carry NO GHS pictogram (Warning-only / no-signal).
	noPicto := map[string]bool{
		"H229": true, // pressurised container: may burst if heated
		"H362": true, // may cause harm to breast-fed children
		"H411": true, // toxic to aquatic life (chronic 2) — no pictogram (CLP Annex V)
		"H412": true, // harmful to aquatic life (chronic 3)
		"H413": true, // may cause long lasting harmful effects (chronic 4)
		"H420": true, // harms public health/environment (ozone)
	}
	for code := range mhchemPhrases() {
		u := strings.ToUpper(code)
		if !strings.HasPrefix(u, "H") || strings.HasPrefix(u, "EUH") {
			continue
		}
		_, mapped := pictogramForHCode(code)
		// Good iff exactly one of {mapped, allowlisted} holds.
		if mapped == noPicto[code] {
			if noPicto[code] {
				t.Errorf("H-code %q is in the no-pictogram allowlist but is also mapped in hCodePictograms", code)
			} else {
				t.Errorf(
					"H-code %q has no pictogram decision: add it to hCodePictograms or the no-pictogram allowlist",
					code,
				)
			}
		}
	}
}

// TestPictogramForHCode_MatchesMixtureLabel is the load-bearing cross-check: for
// the H-codes the deterministic mixture engine emits on a finished label, the
// per-code pictogram must agree with the pictogram the engine assigns to that
// hazard class. A divergence would print a code chip with an icon that
// contradicts the §2 label.
func TestPictogramForHCode_MatchesMixtureLabel(t *testing.T) {
	cases := []struct {
		name    string
		comps   []ClassComponent
		flashPt *float64
		hCode   string
		picto   string
	}{
		{
			"skin sens H317", []ClassComponent{{Name: "s", ConcentrationPct: 5, SkinSensCategory: "1B"}}, nil,
			"H317",
			"GHS07",
		},
		{
			"aquatic chronic 1 H410",
			[]ClassComponent{{Name: "a", ConcentrationPct: 30, AquaticChronicCategory: "1", MChronic: 1}}, nil,
			"H410",
			"GHS09",
		},
		{"flammable H225", nil, ptrF(15), "H225", "GHS02"},
	}
	for _, c := range cases {
		t.Run(
			c.name, func(t *testing.T) {
				mix := ClassifyMixture(c.comps, c.flashPt, false)
				if !hasStr(mix.Label.HCodes, c.hCode) {
					t.Fatalf("setup: engine did not emit %s, got %v", c.hCode, mix.Label.HCodes)
				}
				picto, ok := pictogramForHCode(c.hCode)
				if !ok || picto != c.picto {
					t.Fatalf("pictogramForHCode(%s) = (%q,%t), want %q", c.hCode, picto, ok, c.picto)
				}
				if !hasStr(mix.Label.Pictograms, picto) {
					t.Errorf(
						"engine label pictograms %v missing %s (the per-code pictogram for %s)",
						mix.Label.Pictograms,
						picto,
						c.hCode,
					)
				}
			},
		)
	}
}

func ptrF(v float64) *float64 { return &v }

// TestMhchemBaseLoaded proves the embedded CC-BY EU catalogue (clp_embed.go) is
// loaded as the base for the H/EUH/P lookups: every code in the catalogue is
// present after init (so the lookups are at least the full base size), and codes
// that are NOT in the curated overlay (P314, P320, the P342+P311 combination, the
// cyanoacrylate EUH202) now resolve in BOTH languages instead of FLAGGING.
func TestMhchemBaseLoaded(t *testing.T) {
	// Base catalogue size by prefix (mhchem clp/hpstatements-{el,en}-latest.json:
	// 91 H, 33 EUH, 128 P). The curated overlay can only add or overwrite keys,
	// never remove, so each lookup must be at least its base count.
	//
	// All 91 survive: the H-statement keys go through normHCode, which preserves
	// the final-letter case that distinguishes H360FD from H360Fd. See
	// TestReprSubvariantsResolveDistinctly.
	if len(hLookup) < 91 {
		t.Errorf("hLookup has %d entries; want >= 91 (the full Annex III base)", len(hLookup))
	}
	if len(euhLookup) < 33 {
		t.Errorf("euhLookup has %d entries; want >= 33 (full EUH base)", len(euhLookup))
	}
	if len(pLookup) < 128 {
		t.Errorf("pLookup has %d entries; want >= 128 (full Annex IV base incl. combined)", len(pLookup))
	}

	// Codes absent from the curated maps must now resolve in EL and EN from the base.
	newCodes := []struct {
		lookup func(string, Lang) (string, bool)
		code   string
		el, en string
	}{
		{
			pStatementText, "P314", "Συμβουλευθείτε/Επισκεφθείτε γιατρό εάν αισθανθείτε αδιαθεσία.",
			"Get medical advice/attention if you feel unwell.",
		},
		{
			pStatementText, "P320", "Χρειάζεται επειγόντως ειδική αγωγή (βλέπε … στην ετικέτα).",
			"Specific treatment is urgent (see … on this label).",
		},
		{
			pStatementText, "P342+P311",
			"Εάν παρουσιάζονται αναπνευστικά συμπτώματα: Καλέστε το ΚΕΝΤΡΟ ΔΗΛΗΤΗΡΙΑΣΕΩΝ/γιατρό/…",
			"If experiencing respiratory symptoms: Call a POISON CENTER/doctor/…",
		},
		{
			euhStatementText, "EUH202",
			"Κυανοακρυλική ένωση. Κίνδυνος. Κολλάει στην επιδερμίδα και στα μάτια μέσα σε λίγα δευτερόλεπτα. Να φυλάσσεται μακριά από παιδιά.",
			"Cyanoacrylate. Danger. Bonds skin and eyes in seconds. Keep out of the reach of children.",
		},
	}
	for _, c := range newCodes {
		if got, ok := c.lookup(c.code, LangEL); !ok || got != c.el {
			t.Errorf("%s EL = (%q, %t); want base text and true", c.code, got, ok)
		}
		if got, ok := c.lookup(c.code, LangEN); !ok || got != c.en {
			t.Errorf("%s EN = (%q, %t); want base text and true", c.code, got, ok)
		}
	}

	// Spaced form of a base-only combined code still normalises to the same key.
	if got, ok := pStatementText("P342 + P311", LangEL); !ok || got == "" {
		t.Errorf("spaced base combined code P342 + P311 did not resolve: (%q, %t)", got, ok)
	}
}

// TestCuratedOverridesBase locks overlay precedence: EUH208's curated reference
// header must win over the mhchem Annex II placeholder text ("Περιέχει <όνομα …>")
// for the same code, proving the curated maps overwrite the embedded base rather
// than the other way round.
func TestCuratedOverridesBase(t *testing.T) {
	const curatedEL = "Περιέχει ένα συστατικό που μπορεί να προκαλέσει αλλεργική αντίδραση."
	got, ok := euhStatementText("EUH208", LangEL)
	if !ok {
		t.Fatalf("euhStatementText(EUH208, EL) not found")
	}
	if got != curatedEL {
		t.Fatalf(
			"euhStatementText(EUH208, EL) = %q; want curated header (overlay must beat the mhchem placeholder)",
			got,
		)
	}
	// And the mhchem placeholder form must NOT survive.
	if strings.Contains(got, "<όνομα") {
		t.Fatalf("EUH208 EL still carries the mhchem placeholder %q; curated overlay did not win", got)
	}
}

// TestReprSubvariantsResolveDistinctly guards the one place in the CLP catalogue
// where a code's final-letter CASE encodes confirmed-vs-suspected: H360FD is
// Repr. 1 for both endpoints, H360Fd is Repr. 1 + Repr. 2. Folding case collapses
// them onto one key, and the surviving text is then picked by Go's randomised map
// iteration — the same binary printing different legal wording on different runs.
// The lookup keys therefore go through normHCode, and no two catalogue codes may
// share one.
func TestReprSubvariantsResolveDistinctly(t *testing.T) {
	byKey := map[string][]string{}
	for code := range mhchemPhrases() {
		k := normHCode(code)
		byKey[k] = append(byKey[k], code)
	}
	for k, codes := range byKey {
		if len(codes) > 1 {
			t.Errorf("codes %v share the lookup key %q; one of their statements is unreachable", codes, k)
		}
	}

	// Each sub-variant must return its OWN Annex III statement, every time.
	for _, tc := range []struct {
		code, wantEN string
	}{
		{"H360FD", "May damage fertility. May damage the unborn child."},
		{"H360Fd", "May damage fertility. Suspected of damaging the unborn child."},
		{"H360Df", "May damage the unborn child. Suspected of damaging fertility."},
	} {
		for i := 0; i < 50; i++ {
			got, ok := hStatementText(tc.code, LangEN)
			if !ok {
				t.Fatalf("hStatementText(%q) not found", tc.code)
			}
			if got != tc.wantEN {
				t.Fatalf("hStatementText(%q) = %q, want %q", tc.code, got, tc.wantEN)
			}
		}
	}

	// An ambiguously-cased query must fail closed rather than pick one at random.
	if _, ok := hStatementText("h360fd", LangEN); ok {
		t.Error("an ambiguously-cased repr sub-variant must not resolve to an arbitrary statement")
	}
}
