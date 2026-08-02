package core

import (
	"testing"
)

// classifier_test.go proves the CLP decision boundaries the deterministic
// safety-net guarantees. The load-bearing assertions are the boundary pairs —
// each threshold is tested on BOTH sides (e.g. 0.999% vs 1.0%, 24.9% vs 25.0%)
// so a ">" vs ">=" slip fails immediately. Fixtures follow the ECHA worked-
// example shapes documented in .claude/skills/bom-sds-generator/SKILL.md.

// fp returns a pointer to a flash-point literal so tests can express a known
// flash point inline. It is shared with transport_test.go (same test package),
// which delegates ownership of this helper here.
func fp(v float64) *float64 { return &v }

// hasFlag reports whether any flag in the slice carries the given code.
// (findFlag is the matched-flag variant declared in transport_test.go.)
func hasFlag(flags []Flag, code FlagCode) bool {
	_, ok := findFlag(flags, code)
	return ok
}

// ── Skin sensitisation: generic limits + boundaries ──────────────────────────

func TestSkinSens_Sens1B_GenericBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		pct       float64
		wantClass string
		wantEUH   bool // EUH208 (not_classified info flag) expected
	}{
		// H317 generic limit for Sens 1 / 1B is 1.0%; EUH208 limit is 0.1%.
		{"just below 1.0% → EUH208 only", 0.999, "", true},
		{"exactly 1.0% → Skin Sens. 1", 1.0, skinSensClass1, false},
		{"above 1.0% → Skin Sens. 1", 5.0, skinSensClass1, false},
		{"exactly 0.1% → EUH208", 0.1, "", true},
		{"just below 0.1% → nothing", 0.0999, "", false},
		{"exactly 0.01% → nothing (below EUH208)", 0.01, "", false},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				comps := []ClassComponent{
					{
						Name:             "linalool",
						ConcentrationPct: tt.pct,
						SkinSensCategory: "1B",
					},
				}
				class, flags := ClassifySkinSens(comps)

				if class != tt.wantClass {
					t.Errorf("class = %q, want %q", class, tt.wantClass)
				}
				gotEUH := hasFlag(flags, FlagNotClassified)
				if gotEUH != tt.wantEUH {
					t.Errorf("EUH208 flag = %v, want %v (flags=%+v)", gotEUH, tt.wantEUH, flags)
				}
				// When classified H317, EUH208 must NOT also be emitted.
				if tt.wantClass == skinSensClass1 && len(flags) != 0 {
					t.Errorf("H317-classified mixture must emit no EUH208 flag, got %+v", flags)
				}
			},
		)
	}
}

func TestSkinSens_Sens1_AliasOfSens1B(t *testing.T) {
	// Bare "Sens 1" uses the same generic limits as "1B".
	for _, cat := range []string{"1", "1B", "1b"} {
		t.Run(
			"category="+cat, func(t *testing.T) {
				classified, _ := ClassifySkinSens(
					[]ClassComponent{
						{Name: "x", ConcentrationPct: 1.0, SkinSensCategory: cat},
					},
				)
				if classified != skinSensClass1 {
					t.Errorf("category %q at 1.0%% → %q, want %q", cat, classified, skinSensClass1)
				}
				notYet, _ := ClassifySkinSens(
					[]ClassComponent{
						{Name: "x", ConcentrationPct: 0.999, SkinSensCategory: cat},
					},
				)
				if notYet != "" {
					t.Errorf("category %q at 0.999%% → %q, want not classified", cat, notYet)
				}
			},
		)
	}
}

func TestSkinSens_Sens1A_PotentBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		pct       float64
		wantClass string
		wantEUH   bool
	}{
		// Sens 1A: H317 at ≥ 0.1%, EUH208 at ≥ 0.01%.
		{"just below 0.1% → EUH208 only", 0.0999, "", true},
		{"exactly 0.1% → Skin Sens. 1", 0.1, skinSensClass1, false},
		{"exactly 0.01% → EUH208", 0.01, "", true},
		{"just below 0.01% → nothing", 0.00999, "", false},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				comps := []ClassComponent{
					{
						Name:             "cinnamal",
						ConcentrationPct: tt.pct,
						SkinSensCategory: "1A",
					},
				}
				class, flags := ClassifySkinSens(comps)
				if class != tt.wantClass {
					t.Errorf("class = %q, want %q", class, tt.wantClass)
				}
				if got := hasFlag(flags, FlagNotClassified); got != tt.wantEUH {
					t.Errorf("EUH208 flag = %v, want %v", got, tt.wantEUH)
				}
			},
		)
	}
}

func TestSkinSens_SCLOverridesGCL(t *testing.T) {
	// A Sens 1B substance with a harmonised SCL of 0.5% classifies the mixture
	// at 0.6% — where the generic 1.0% limit would NOT — proving SCL > GCL.
	withSCL := []ClassComponent{
		{
			Name:             "isoeugenol",
			ConcentrationPct: 0.6,
			SkinSensCategory: "1B",
			SkinSensSCLPct:   0.5,
		},
	}
	if class, _ := ClassifySkinSens(withSCL); class != skinSensClass1 {
		t.Errorf("SCL=0.5%% at 0.6%% → %q, want %q (SCL must override GCL)", class, skinSensClass1)
	}

	// Same component WITHOUT the SCL stays unclassified at 0.6% (GCL=1.0%).
	noSCL := []ClassComponent{
		{
			Name:             "isoeugenol",
			ConcentrationPct: 0.6,
			SkinSensCategory: "1B",
		},
	}
	if class, _ := ClassifySkinSens(noSCL); class != "" {
		t.Errorf("no SCL at 0.6%% (GCL=1.0%%) → %q, want not classified", class)
	}

	// EUH208 band scales with the SCL: limit/10 = 0.05%. At 0.05% → EUH208.
	euhAtSCL := []ClassComponent{
		{
			Name:             "isoeugenol",
			ConcentrationPct: 0.05,
			SkinSensCategory: "1B",
			SkinSensSCLPct:   0.5,
		},
	}
	class, flags := ClassifySkinSens(euhAtSCL)
	if class != "" || !hasFlag(flags, FlagNotClassified) {
		t.Errorf("SCL=0.5%% at 0.05%% → class=%q flags=%+v, want EUH208 only", class, flags)
	}
}

func TestSkinSens_HighestClassWins_NoAdditivity(t *testing.T) {
	// Two sub-limit Sens 1B components (0.6% + 0.6% = 1.2%) must NOT add up to
	// classify the mixture — skin sensitisation has no additivity. But each is
	// ≥ 0.1%, so EUH208 applies.
	comps := []ClassComponent{
		{Name: "a", ConcentrationPct: 0.6, SkinSensCategory: "1B"},
		{Name: "b", ConcentrationPct: 0.6, SkinSensCategory: "1B"},
	}
	class, flags := ClassifySkinSens(comps)
	if class != "" {
		t.Errorf("two 0.6%% Sens 1B (no additivity) → %q, want not classified", class)
	}
	if !hasFlag(flags, FlagNotClassified) {
		t.Errorf("expected EUH208 flag for sub-limit sensitisers, got %+v", flags)
	}

	// One classifying component among non-sensitisers → highest class wins.
	mixed := []ClassComponent{
		{Name: "inert", ConcentrationPct: 90, SkinSensCategory: ""},
		{Name: "potent", ConcentrationPct: 0.2, SkinSensCategory: "1A"}, // ≥0.1% → H317
	}
	if class, _ := ClassifySkinSens(mixed); class != skinSensClass1 {
		t.Errorf("mixture with one classifying Sens 1A → %q, want %q", class, skinSensClass1)
	}
}

func TestSkinSens_NonSensitiserIgnored(t *testing.T) {
	comps := []ClassComponent{{Name: "water", ConcentrationPct: 99, SkinSensCategory: ""}}
	class, flags := ClassifySkinSens(comps)
	if class != "" || len(flags) != 0 {
		t.Errorf("non-sensitiser → class=%q flags=%+v, want both empty", class, flags)
	}
}

// ── Aquatic: acute summation + boundary + M-factors ──────────────────────────

func TestAquatic_AcuteSummationBoundary(t *testing.T) {
	tests := []struct {
		name      string
		pct       float64
		wantClass string
	}{
		// Σ(C·M) with M=1: 24.9% NOT Acute 1; 25.0% IS Acute 1.
		{"just below 25% → not classified", 24.9, ""},
		{"exactly 25% → Aquatic Acute 1", 25.0, aquaticAcute1},
		{"above 25% → Aquatic Acute 1", 40.0, aquaticAcute1},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				comps := []ClassComponent{
					{
						Name:                 "d-limonene",
						ConcentrationPct:     tt.pct,
						AquaticAcuteCategory: "1",
					},
				}
				if class, _ := ClassifyAquatic(comps); class != tt.wantClass {
					t.Errorf("acute sum %.1f%% → %q, want %q", tt.pct, class, tt.wantClass)
				}
			},
		)
	}
}

func TestAquatic_MFactorOnlyOnAcute1AndChronic1(t *testing.T) {
	// M-factor on Acute 1: 2.5% × M=10 = 25.0 → exactly Acute 1.
	acuteM := []ClassComponent{
		{
			Name:                 "high-tox",
			ConcentrationPct:     2.5,
			AquaticAcuteCategory: "1",
			MAcute:               10,
		},
	}
	if class, _ := ClassifyAquatic(acuteM); class != aquaticAcute1 {
		t.Errorf("2.5%% × M=10 acute → %q, want %q", class, aquaticAcute1)
	}

	// Just below with the M-factor: 2.49% × 10 = 24.9 → not classified.
	acuteBelow := []ClassComponent{
		{
			Name:                 "high-tox",
			ConcentrationPct:     2.49,
			AquaticAcuteCategory: "1",
			MAcute:               10,
		},
	}
	if class, _ := ClassifyAquatic(acuteBelow); class != "" {
		t.Errorf("2.49%% × M=10 acute → %q, want not classified", class)
	}

	// M-factor must be IGNORED on a Chronic 2 substance (default 1). A Chronic 2
	// at 2.4% with a spurious MChronic=10 stays unclassified: with M ignored and
	// no Chronic 1 present (100×0 term = 0), the Chronic 3 rung is 10×2.4 = 24
	// < 25. If the M-factor were wrongly applied, chron2Sum would be 24 and the
	// Chronic 3 rung 10×24 = 240 ≥ 25 → Chronic 3, so this fixture flips iff the
	// M-factor leaks below Chronic 1.
	chron2SpuriousM := []ClassComponent{
		{
			Name:                   "chronic2",
			ConcentrationPct:       2.4,
			AquaticChronicCategory: "2",
			MChronic:               10, // illegitimate below Chronic 1 — must be ignored
		},
	}
	if class, _ := ClassifyAquatic(chron2SpuriousM); class != "" {
		t.Errorf("Chronic 2 at 2.4%% with ignored M=10 → %q, want not classified", class)
	}
}

func TestAquatic_Chronic1Boundary(t *testing.T) {
	// Chronic 1 summation (with its M-factor) at exactly 25% → Chronic 1.
	at25 := []ClassComponent{
		{
			Name:                   "c1",
			ConcentrationPct:       2.5,
			AquaticChronicCategory: "1",
			MChronic:               10,
		},
	}
	if class, _ := ClassifyAquatic(at25); class != aquaticChron1 {
		t.Errorf("Chronic 1 sum 25%% → %q, want %q", class, aquaticChron1)
	}

	// Just below the Chronic 1 rung (chron1Sum = 24.9) the mixture is NOT
	// Chronic 1 — but the cumulative cascade carries that potent Chronic 1
	// contribution into the Chronic 2 rung (10 × 24.9 = 249 ≥ 25), so the
	// correct CLP outcome is Chronic 2, not "not classified". This proves the
	// boundary is on the Chronic 1 *rung*, while the lower rungs still fold in
	// the Chronic 1 sum at 10×.
	below := []ClassComponent{
		{
			Name:                   "c1",
			ConcentrationPct:       2.49,
			AquaticChronicCategory: "1",
			MChronic:               10,
		},
	}
	if class, _ := ClassifyAquatic(below); class != aquaticChron2 {
		t.Errorf("Chronic 1 sum 24.9%% → %q, want %q (cascades to Chronic 2)", class, aquaticChron2)
	}
}

func TestAquatic_ChronicCascade_Worked(t *testing.T) {
	// ECHA-style worked examples exercising the top-down chronic cascade. The
	// CLP rung formulas (Annex I Table 4.1.0, §4.1.3.5.5.4) fold each more-toxic
	// tier into the lower rungs, with the Chronic-1 coefficient stepping 1×→10×→100×
	// (10× per tier of separation — NOT a uniform 10×):
	//   Chronic 2 rung:  10×Σ(C_chr1·M) +              Σ(C_chr2)              ≥ 25
	//   Chronic 3 rung: 100×Σ(C_chr1·M) + 10×Σ(C_chr2) + Σ(C_chr3)            ≥ 25
	// The Chronic-3 Chronic-1 coefficient is 100×, not 10×; a 10× regression
	// under-classifies (e.g. a lone Chronic 1 @ 0.3% would drop from Chronic 3 to
	// not-classified). The discriminating fixtures below carry Chronic-1 PRESENT
	// and resolve on ONLY the Chronic 3 rung so the suite catches that regression.
	tests := []struct {
		name      string
		comps     []ClassComponent
		wantClass string
	}{
		{
			// Chronic 2 rung at exactly 25: 10×1 + 15 = 25 (stops before Chr3).
			name: "Chronic 2 rung at 25",
			comps: []ClassComponent{
				{Name: "c1", ConcentrationPct: 1, AquaticChronicCategory: "1"},
				{Name: "c2", ConcentrationPct: 15, AquaticChronicCategory: "2"},
			},
			wantClass: aquaticChron2,
		},
		{
			// Chronic 3 rung at exactly 25 with no Chronic 1 present:
			// 100×0 + 10×2 + 5 = 25 → Chronic 3 (Chronic 2 rung = 2 < 25).
			// (Chronic-1 absent here, so this case does not exercise the 100×
			// Chronic-1 coefficient — the two fixtures below do.)
			name: "Chronic 3 rung at 25 (no Chronic 1)",
			comps: []ClassComponent{
				{Name: "c2", ConcentrationPct: 2, AquaticChronicCategory: "2"},
				{Name: "c3", ConcentrationPct: 5, AquaticChronicCategory: "3"},
			},
			wantClass: aquaticChron3,
		},
		{
			// DISCRIMINATING: a single Aquatic Chronic 1 at 0.3% (M=1) lands on
			// ONLY the Chronic 3 rung. With the correct 100× coefficient:
			//   Chr1 rung: 0.3 < 25; Chr2 rung: 10×0.3 = 3 < 25;
			//   Chr3 rung: 100×0.3 = 30 ≥ 25 → Chronic 3.
			// A 10× regression on the Chronic-1 term would give 10×0.3 = 3 < 25 →
			// not-classified — a dangerous-direction under-classification. This is
			// the canonical counterexample that distinguishes 100× from 10×.
			name: "single Chronic 1 @ 0.3% → Chronic 3 (100×0.3=30)",
			comps: []ClassComponent{
				{Name: "c1", ConcentrationPct: 0.3, AquaticChronicCategory: "1"},
			},
			wantClass: aquaticChron3,
		},
		{
			// DISCRIMINATING (panel's case): Chronic 1 @ 2% + Chronic 3 @ 4%.
			//   Chr1 rung: 2 < 25; Chr2 rung: 10×2 + 0 = 20 < 25;
			//   Chr3 rung: 100×2 + 10×0 + 4 = 204 ≥ 25 → Chronic 3.
			// Under a 10× regression: 10×2 + 4 = 24 < 25 → not-classified. The
			// large 204 margin makes the 100× rung unmistakable.
			name: "Chronic 1 @ 2% + Chronic 3 @ 4% → Chronic 3 (100×2+4=204)",
			comps: []ClassComponent{
				{Name: "c1", ConcentrationPct: 2, AquaticChronicCategory: "1"},
				{Name: "c3", ConcentrationPct: 4, AquaticChronicCategory: "3"},
			},
			wantClass: aquaticChron3,
		},
		{
			// A potent Chronic 1 just below its own rung (chron1Sum = 24.9)
			// cascades DOWN: the Chronic 2 rung is 10×24.9 = 249 ≥ 25, so the
			// cascade resolves to Chronic 2 — the documented "more-toxic tiers
			// dominate the lower rungs" behaviour.
			name: "potent Chronic 1 below its rung cascades to Chronic 2",
			comps: []ClassComponent{
				{Name: "c1", ConcentrationPct: 2.49, AquaticChronicCategory: "1", MChronic: 10},
			},
			wantClass: aquaticChron2,
		},
		{
			// All rungs genuinely below 25 → not classified, evaluated under the
			// CORRECT 100× Chronic-1 coefficient:
			// chron1Sum=0.1, chron2Sum=1, chron3Sum=1.
			//   Chr2 rung: 10×0.1 + 1 = 2 < 25;
			//   Chr3 rung: 100×0.1 + 10×1 + 1 = 10 + 10 + 1 = 21 < 25. Both below.
			// NOTE: the previous fixture used chron1Sum=0.5 and asserted "" — but
			// under the correct cascade that is Chr3 rung 100×0.5+10×1+1 = 61 ≥ 25,
			// i.e. Chronic 3. The old value silently encoded the 10× bug; chron1Sum
			// is lowered to 0.1 here so the not-classified intent holds correctly.
			name: "all chronic rungs below 25 → not classified",
			comps: []ClassComponent{
				{Name: "c1", ConcentrationPct: 0.1, AquaticChronicCategory: "1"},
				{Name: "c2", ConcentrationPct: 1, AquaticChronicCategory: "2"},
				{Name: "c3", ConcentrationPct: 1, AquaticChronicCategory: "3"},
			},
			wantClass: "",
		},
		{
			// Chronic 4 safety net (CLP Annex I Table 4.1.0): the UNWEIGHTED sum of
			// ALL chronically-classified components (Chronic 1+2+3+4, no M-factor)
			// ≥ 25%. A single Chronic 4 at exactly 25% → Chronic 4.
			name: "Chronic 4 summation at 25%",
			comps: []ClassComponent{
				{Name: "c4", ConcentrationPct: 25, AquaticChronicCategory: "4"},
			},
			wantClass: aquaticChron4,
		},
		{
			// Just below the 25% safety-net sum → not classified.
			name: "Chronic 4 summation at 24.9%",
			comps: []ClassComponent{
				{Name: "c4", ConcentrationPct: 24.9, AquaticChronicCategory: "4"},
			},
			wantClass: "",
		},
		{
			// REGRESSION LOCK: a lone Chronic 4 at 1% is NOT classified — the safety
			// net is a 25% summation, not the former "any single Chronic 4 ≥ 1% →
			// H413" per-component trigger (which both over-classified a lone 1–24.9%
			// component and under-classified many sub-1% ones).
			name: "lone Chronic 4 at 1% → not classified",
			comps: []ClassComponent{
				{Name: "c4", ConcentrationPct: 1, AquaticChronicCategory: "4"},
			},
			wantClass: "",
		},
		{
			// DISCRIMINATING: Chronic 3 @ 24% + two Chronic 4 @ 0.5% (each < 1%).
			//   Chr3 rung: 100×0 + 10×0 + 24 = 24 < 25 → not Chronic 3;
			//   Chr4 net:  0 + 0 + 24 + 1.0 = 25 ≥ 25 → Chronic 4.
			// Under the old "single Chronic 4 ≥ 1%" rule both 0.5% components were
			// ignored → not-classified: a dangerous-direction under-call this locks out.
			name: "Chr3 24% + 2×Chr4 0.5% → Chronic 4 (summation 25)",
			comps: []ClassComponent{
				{Name: "c3", ConcentrationPct: 24, AquaticChronicCategory: "3"},
				{Name: "c4a", ConcentrationPct: 0.5, AquaticChronicCategory: "4"},
				{Name: "c4b", ConcentrationPct: 0.5, AquaticChronicCategory: "4"},
			},
			wantClass: aquaticChron4,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if class, _ := ClassifyAquatic(tt.comps); class != tt.wantClass {
					t.Errorf("cascade → %q, want %q", class, tt.wantClass)
				}
			},
		)
	}
}

func TestAquatic_TopDownPrecedence(t *testing.T) {
	// A mixture that satisfies BOTH Acute 1 and Chronic 1 returns Acute 1 — the
	// top-down cascade stops at the first rung reached.
	comps := []ClassComponent{
		{Name: "x", ConcentrationPct: 30, AquaticAcuteCategory: "1", AquaticChronicCategory: "1"},
	}
	if class, _ := ClassifyAquatic(comps); class != aquaticAcute1 {
		t.Errorf("acute+chronic both ≥25%% → %q, want %q (acute precedence)", class, aquaticAcute1)
	}
}

func TestAquatic_NotClassified(t *testing.T) {
	comps := []ClassComponent{{Name: "water", ConcentrationPct: 99}}
	if class, flags := ClassifyAquatic(comps); class != "" || flags != nil {
		t.Errorf("non-aquatic → class=%q flags=%+v, want empty/nil", class, flags)
	}
}

// ── Flammable: data-completeness FLAGs ────────────────────────────────────────

func TestCheckFlammable_FlashPointUnknown(t *testing.T) {
	// Flash point unknown → data_missing (block).
	flags := CheckFlammable(false)
	f, ok := findFlag(flags, FlagDataMissing)
	if !ok {
		t.Fatalf("flash point unknown → expected data_missing flag, got %+v", flags)
	}
	if f.Severity != SeverityBlock {
		t.Errorf("data_missing severity = %q, want %q", f.Severity, SeverityBlock)
	}
}

func TestCheckFlammable_FlashPointKnown_NoFlags(t *testing.T) {
	if flags := CheckFlammable(true); flags != nil {
		t.Errorf("flash point known → expected no flags, got %+v", flags)
	}
}

// ── FLAG wire-value contract ──────────────────────────────────────────────────

func TestFlagCodeWireValues(t *testing.T) {
	// The classifier and the service shell share these exact wire values; the
	// classifier deliberately does NOT emit unit_inconsistent (that FLAG is a
	// composition-stage concern in the shell), so we lock its value here to
	// guarantee the shared contract is intact.
	tests := []struct {
		got  FlagCode
		want string
	}{
		{FlagDataMissing, "data_missing"},
		{FlagUnitInconsistent, "unit_inconsistent"},
		{FlagSdsUnmapped, "sds_unmapped"},
		{FlagNotClassified, "not_classified"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("FlagCode = %q, want %q", tt.got, tt.want)
		}
	}

	// The classifier core never raises unit_inconsistent — it is raised by the
	// composition stage, not by these pure functions.
	if hasFlag(CheckFlammable(false), FlagUnitInconsistent) {
		t.Error("classifier must not emit unit_inconsistent")
	}
	_, skinFlags := ClassifySkinSens([]ClassComponent{{Name: "a", ConcentrationPct: 0.5, SkinSensCategory: "1B"}})
	if hasFlag(skinFlags, FlagUnitInconsistent) {
		t.Error("ClassifySkinSens must not emit unit_inconsistent")
	}
}
