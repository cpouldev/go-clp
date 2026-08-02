package core

import (
	"strings"
	"testing"
)

// tfp ("transport flash point") returns a pointer to a flash-point literal so
// the table tests can express a known flash point inline. It is named
// distinctly to avoid colliding with sibling pure-core test files in the same
// clp test package (e.g. classifier_test.go).
func tfp(v float64) *float64 { return &v }

// findFlag returns the first FLAG with the given code, plus whether one was
// found. It complements classifier_test.go's hasFlag by exposing the matched
// Flag so transport tests can assert its severity, section, and message.
func findFlag(flags []Flag, code FlagCode) (Flag, bool) {
	for _, f := range flags {
		if f.Code == code {
			return f, true
		}
	}
	return Flag{}, false
}

// TestClassify_TerminalBranches exercises every terminal outcome of the
// fragrance decision tree: UN 1197, UN 1266, UN 3082, SP375-exempt
// not-regulated, plain not-regulated, and the ambiguous (unknown flash point)
// FLAG path.
func TestTransportClassify_TerminalBranches(t *testing.T) {
	tests := []struct {
		name         string
		flashPt      *float64
		aquaticClass string
		productType  string
		innerLitres  *float64 // inner-packaging size for SP375; nil = unknown (assumed ≤ 5 L)

		wantRegulated bool
		wantUN        string // ADR/IATA UN number; "" when not regulated
		wantClass     string
		wantPG        string
		wantEnvMark   bool
		wantLq        bool
		wantEq        bool
		wantFlagCode  FlagCode // "" when no FLAG expected
		wantFlagSev   FlagSeverity
	}{
		{
			name:          "flammable extract -> UN 1197 PG III",
			flashPt:       tfp(40),
			aquaticClass:  "",
			productType:   "extract",
			wantRegulated: true,
			wantUN:        "1197",
			wantClass:     "3",
			wantPG:        "III",
			wantEnvMark:   false,
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "flammable flavouring -> UN 1197 PG III",
			flashPt:       tfp(45),
			aquaticClass:  "",
			productType:   "natural flavouring",
			wantRegulated: true,
			wantUN:        "1197",
			wantClass:     "3",
			wantPG:        "III",
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "flammable perfumery product -> UN 1266 PG III",
			flashPt:       tfp(50),
			aquaticClass:  "",
			productType:   "perfumery product",
			wantRegulated: true,
			wantUN:        "1266",
			wantClass:     "3",
			wantPG:        "III",
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "flammable finished fragrance -> UN 1266 PG III",
			flashPt:       tfp(55),
			aquaticClass:  "",
			productType:   "finished fragrance (EDT)",
			wantRegulated: true,
			wantUN:        "1266",
			wantClass:     "3",
			wantPG:        "III",
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "flammable, unknown product type -> prefer UN 1266",
			flashPt:       tfp(48),
			aquaticClass:  "",
			productType:   "", // ambiguous product type -> finished consumer fragrance default
			wantRegulated: true,
			wantUN:        "1266",
			wantClass:     "3",
			wantPG:        "III",
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "flammable low flash point -> PG II (conservative without BP)",
			flashPt:       tfp(18),
			aquaticClass:  "",
			productType:   "perfumery",
			wantRegulated: true,
			wantUN:        "1266",
			wantClass:     "3",
			wantPG:        "II",
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "flammable AND aquatic-toxic -> Class 3 with env mark (SP375 N/A to flammables)",
			flashPt:       tfp(42),
			aquaticClass:  "Aquatic Chronic 1",
			productType:   "perfumery product",
			innerLitres:   tfp(0.1), // small package, but SP375 never exempts a flammable Class 3
			wantRegulated: true,
			wantUN:        "1266",
			wantClass:     "3",
			wantPG:        "III",
			wantEnvMark:   true, // additional fish+tree mark on the Class 3 entry
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "flammable with only Chronic 3 aquatic -> Class 3, NO env mark",
			flashPt:       tfp(42),
			aquaticClass:  "Aquatic Chronic 3", // below the transport env-hazard threshold
			productType:   "perfumery product",
			wantRegulated: true,
			wantUN:        "1266",
			wantClass:     "3",
			wantPG:        "III",
			wantEnvMark:   false, // Chronic 3 does not add the mark even on the flammable path
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "aquatic non-flammable (Acute 1), package > 5 L -> UN 3082 Class 9 env mark",
			flashPt:       tfp(95),
			aquaticClass:  "Aquatic Acute 1",
			productType:   "perfumery product",
			innerLitres:   tfp(25), // > 5 L: SP375 does not exempt
			wantRegulated: true,
			wantUN:        "3082",
			wantClass:     "9",
			wantPG:        "III",
			wantEnvMark:   true,
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "aquatic non-flammable (Chronic 2), package > 5 L -> UN 3082 Class 9 env mark",
			flashPt:       tfp(80),
			aquaticClass:  "Aquatic Chronic 2",
			productType:   "extract",
			innerLitres:   tfp(10), // > 5 L: SP375 does not exempt
			wantRegulated: true,
			wantUN:        "3082",
			wantClass:     "9",
			wantPG:        "III",
			wantEnvMark:   true,
			wantLq:        true,
			wantEq:        true,
		},
		{
			name:          "aquatic non-flammable (Acute 1), small package (50 ml) -> SP375 not regulated, no flag",
			flashPt:       tfp(95),
			aquaticClass:  "Aquatic Acute 1",
			productType:   "perfumery product",
			innerLitres:   tfp(0.05), // ≤ 5 L: SP375 exempts; size known so no review flag
			wantRegulated: false,
		},
		{
			name:          "aquatic non-flammable (Chronic 2), unknown package -> SP375 not regulated + confirm flag",
			flashPt:       tfp(80),
			aquaticClass:  "Aquatic Chronic 2",
			productType:   "extract",
			innerLitres:   nil, // unknown: assumed ≤ 5 L (consumer fragrance) with a confirm review flag
			wantRegulated: false,
			wantFlagCode:  FlagDataMissing,
			wantFlagSev:   SeverityWarn,
		},
		{
			name:          "low-hazard water-based (no flash, high FP) -> not regulated",
			flashPt:       tfp(120),
			aquaticClass:  "",
			productType:   "perfumery product",
			wantRegulated: false,
		},
		{
			name:          "non-flammable, only Chronic 3 -> not env-hazard, not regulated",
			flashPt:       tfp(90),
			aquaticClass:  "Aquatic Chronic 3",
			productType:   "perfumery product",
			wantRegulated: false, // Chronic 3/4 do NOT trigger the transport env mark
		},
		{
			name:          "non-flammable, only Chronic 4 -> not env-hazard, not regulated",
			flashPt:       tfp(90),
			aquaticClass:  "Aquatic Chronic 4",
			productType:   "perfumery product",
			wantRegulated: false,
		},
		{
			name:          "unknown flash point -> data_missing FLAG, not regulated",
			flashPt:       nil,
			aquaticClass:  "Aquatic Acute 1",
			productType:   "perfumery product",
			wantRegulated: false,
			wantFlagCode:  FlagDataMissing,
			wantFlagSev:   SeverityBlock,
		},
		{
			name:          "unknown flash point, no aquatic -> still data_missing FLAG",
			flashPt:       nil,
			aquaticClass:  "",
			productType:   "extract",
			wantRegulated: false,
			wantFlagCode:  FlagDataMissing,
			wantFlagSev:   SeverityBlock,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, flags := Classify(tt.flashPt, tt.aquaticClass, tt.productType, tt.innerLitres)

				if got.Regulated != tt.wantRegulated {
					t.Fatalf("Regulated = %v, want %v", got.Regulated, tt.wantRegulated)
				}

				// FLAG expectation.
				if tt.wantFlagCode != "" {
					f, ok := findFlag(flags, tt.wantFlagCode)
					if !ok {
						t.Fatalf("expected FLAG %q, got flags %+v", tt.wantFlagCode, flags)
					}
					if f.Severity != tt.wantFlagSev {
						t.Errorf("FLAG severity = %q, want %q", f.Severity, tt.wantFlagSev)
					}
					if f.Section != "14" {
						t.Errorf("FLAG section = %q, want %q", f.Section, "14")
					}
					if f.Message == "" {
						t.Errorf("FLAG message is empty; expected a human-readable explanation")
					}
				} else if len(flags) != 0 {
					t.Errorf("expected no FLAGs, got %+v", flags)
				}

				if !tt.wantRegulated {
					// Not regulated: ADR/IATA must be nil and no env mark.
					if got.ADR != nil || got.IATA != nil {
						t.Errorf("not-regulated result must have nil ADR/IATA, got ADR=%+v IATA=%+v", got.ADR, got.IATA)
					}
					if got.EnvMark {
						t.Errorf("not-regulated result must not set EnvMark")
					}
					return
				}

				// Regulated: ADR and IATA must both be present and agree on UN/class/PG.
				if got.ADR == nil {
					t.Fatalf("regulated result missing ADR entry")
				}
				if got.IATA == nil {
					t.Fatalf("regulated result missing IATA entry")
				}
				if got.ADR.UNNumber != tt.wantUN {
					t.Errorf("ADR UN = %q, want %q", got.ADR.UNNumber, tt.wantUN)
				}
				if got.IATA.UNNumber != tt.wantUN {
					t.Errorf("IATA UN = %q, want %q", got.IATA.UNNumber, tt.wantUN)
				}
				if got.ADR.Class != tt.wantClass {
					t.Errorf("ADR Class = %q, want %q", got.ADR.Class, tt.wantClass)
				}
				if got.IATA.Class != tt.wantClass {
					t.Errorf("IATA Class = %q, want %q", got.IATA.Class, tt.wantClass)
				}
				if got.ADR.PackingGroup != tt.wantPG {
					t.Errorf("ADR PG = %q, want %q", got.ADR.PackingGroup, tt.wantPG)
				}
				if got.IATA.PackingGroup != tt.wantPG {
					t.Errorf("IATA PG = %q, want %q", got.IATA.PackingGroup, tt.wantPG)
				}
				if got.ADR.ProperShippingName == "" {
					t.Errorf("ADR ProperShippingName is empty")
				}
				if got.IATA.ProperShippingName == "" {
					t.Errorf("IATA ProperShippingName is empty")
				}
				if got.EnvMark != tt.wantEnvMark {
					t.Errorf("EnvMark = %v, want %v", got.EnvMark, tt.wantEnvMark)
				}
				if got.LqEligible != tt.wantLq {
					t.Errorf("LqEligible = %v, want %v", got.LqEligible, tt.wantLq)
				}
				if got.EqEligible != tt.wantEq {
					t.Errorf("EqEligible = %v, want %v", got.EqEligible, tt.wantEq)
				}
			},
		)
	}
}

// TestTransportClassify_SP375 pins the SP375 special-provision behaviour for the
// environmentally hazardous (UN 3082) branch: inner packaging ≤ 5 L (or unknown,
// treated as ≤ 5 L) makes the product NOT regulated; > 5 L ships as UN 3082.
func TestTransportClassify_SP375(t *testing.T) {
	t.Run(
		"unknown package -> not regulated, Sp375Applied, confirm flag", func(t *testing.T) {
			got, flags := Classify(tfp(95), "Aquatic Chronic 1", "perfumery product", nil)
			if got.Regulated {
				t.Fatalf("expected not regulated under SP375, got regulated")
			}
			if !got.Sp375Applied {
				t.Errorf("expected Sp375Applied = true")
			}
			if got.NotRegulatedReason == "" {
				t.Errorf("expected a NotRegulatedReason explaining SP375")
			}
			if f, ok := findFlag(
				flags,
				FlagDataMissing,
			); !ok || f.Severity != SeverityWarn || f.Section != "14" {
				t.Errorf("expected a warn confirm-packaging flag in section 14, got %+v", flags)
			}
		},
	)

	t.Run(
		"small package known -> not regulated, no flag", func(t *testing.T) {
			got, flags := Classify(tfp(95), "Aquatic Acute 1", "perfumery product", tfp(0.1))
			if got.Regulated || !got.Sp375Applied {
				t.Fatalf("expected SP375 not-regulated, got %+v", got)
			}
			if len(flags) != 0 {
				t.Errorf("expected no flags when package size is known ≤ 5 L, got %+v", flags)
			}
		},
	)

	t.Run(
		"large package -> UN 3082 Class 9, not SP375", func(t *testing.T) {
			got, _ := Classify(tfp(95), "Aquatic Acute 1", "perfumery product", tfp(5.0001))
			if !got.Regulated || got.Sp375Applied || got.ADR == nil || got.ADR.UNNumber != "3082" {
				t.Fatalf("expected UN 3082 Class 9 above the 5 L SP375 limit, got %+v", got)
			}
		},
	)

	t.Run(
		"exactly 5 L -> SP375 exempts (inclusive boundary)", func(t *testing.T) {
			got, _ := Classify(tfp(95), "Aquatic Acute 1", "perfumery product", tfp(5.0))
			if got.Regulated || !got.Sp375Applied {
				t.Fatalf("expected SP375 exemption at exactly 5 L, got %+v", got)
			}
		},
	)
}

// TestClassify_LimitedQuantityLitres pins the ADR limited-quantity threshold per
// outcome: both the Class 3 fragrance outcomes and the UN 3082 Class 9 outcome
// carry a 5 L LQ allowance. The UN 3082 row uses a > 5 L package so SP375 does
// not exempt it.
func TestTransportClassify_LimitedQuantityLitres(t *testing.T) {
	tests := []struct {
		name         string
		flashPt      *float64
		aquaticClass string
		productType  string
		innerLitres  *float64
		wantUN       string
		wantLqLitres float64
	}{
		{"UN 1197 LQ 5L", tfp(40), "", "extract", nil, "1197", 5},
		{"UN 1266 LQ 5L", tfp(40), "", "perfumery product", nil, "1266", 5},
		{"UN 3082 LQ 5L", tfp(95), "Aquatic Acute 1", "perfumery product", tfp(25), "3082", 5},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, _ := Classify(tt.flashPt, tt.aquaticClass, tt.productType, tt.innerLitres)
				if got.ADR == nil {
					t.Fatalf("expected regulated result with ADR entry")
				}
				if got.ADR.UNNumber != tt.wantUN {
					t.Fatalf("UN = %q, want %q", got.ADR.UNNumber, tt.wantUN)
				}
				if got.ADR.LimitedQuantityLitres != tt.wantLqLitres {
					t.Errorf("LimitedQuantityLitres = %v, want %v", got.ADR.LimitedQuantityLitres, tt.wantLqLitres)
				}
			},
		)
	}
}

// TestClassify_ProperShippingNames verifies each UN number carries its ADR
// proper shipping name on both the road and air entries.
func TestTransportClassify_ProperShippingNames(t *testing.T) {
	tests := []struct {
		name         string
		flashPt      *float64
		aquaticClass string
		productType  string
		innerLitres  *float64
		wantPSN      string
	}{
		{"UN 1197 PSN", tfp(40), "", "extract", nil, psnUN1197},
		{"UN 1266 PSN", tfp(40), "", "perfumery product", nil, psnUN1266},
		{"UN 3082 PSN", tfp(95), "Aquatic Acute 1", "perfumery product", tfp(25), psnUN3082},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, _ := Classify(tt.flashPt, tt.aquaticClass, tt.productType, tt.innerLitres)
				if got.ADR == nil || got.IATA == nil {
					t.Fatalf("expected regulated result with ADR + IATA entries")
				}
				if got.ADR.ProperShippingName != tt.wantPSN {
					t.Errorf("ADR PSN = %q, want %q", got.ADR.ProperShippingName, tt.wantPSN)
				}
				if got.IATA.ProperShippingName != tt.wantPSN {
					t.Errorf("IATA PSN = %q, want %q", got.IATA.ProperShippingName, tt.wantPSN)
				}
			},
		)
	}
}

// TestClassify_FlashPointBoundary pins the Class 3 ceiling and the PG II/III
// boundary so a one-degree drift on either boundary is caught.
func TestTransportClassify_FlashPointBoundary(t *testing.T) {
	tests := []struct {
		name          string
		flashPt       float64
		wantRegulated bool
		wantPG        string
	}{
		{"22.9C just below PG boundary -> PG II", 22.9, true, "II"},
		{"23.0C at PG boundary -> PG III", 23.0, true, "III"},
		{"60.0C at Class 3 ceiling -> still flammable PG III", 60.0, true, "III"},
		{"60.1C just above ceiling -> not flammable, not regulated", 60.1, false, ""},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				// productType "perfumery", no aquatic class -> isolates the flammable boundary.
				got, _ := Classify(tfp(tt.flashPt), "", "perfumery product", nil)
				if got.Regulated != tt.wantRegulated {
					t.Fatalf("flashPt %.1f: Regulated = %v, want %v", tt.flashPt, got.Regulated, tt.wantRegulated)
				}
				if !tt.wantRegulated {
					return
				}
				if got.ADR.PackingGroup != tt.wantPG {
					t.Errorf("flashPt %.1f: PG = %q, want %q", tt.flashPt, got.ADR.PackingGroup, tt.wantPG)
				}
			},
		)
	}
}

// TestIsTransportEnvHazard pins which aquatic classes trigger the transport
// environmentally-hazardous designation (Acute 1 / Chronic 1 / Chronic 2 only),
// and that matching is tolerant of casing and formatting variants the
// classifier may emit.
func TestIsTransportEnvHazard(t *testing.T) {
	tests := []struct {
		aquaticClass string
		want         bool
	}{
		{"", false},
		{"Aquatic Acute 1", true},
		{"aquatic acute 1", true},
		{"Acute 1", true},
		{"Aquatic Chronic 1", true},
		{"Aquatic Chronic 2", true},
		{"Chronic 2", true},
		{"Aquatic Chronic 3", false},
		{"Aquatic Chronic 4", false},
		{"Skin Sens. 1", false},
	}

	for _, tt := range tests {
		t.Run(
			tt.aquaticClass, func(t *testing.T) {
				if got := isTransportEnvHazard(tt.aquaticClass); got != tt.want {
					t.Errorf("isTransportEnvHazard(%q) = %v, want %v", tt.aquaticClass, got, tt.want)
				}
			},
		)
	}
}

// TestClassify_Deterministic confirms the pure core returns identical output for
// identical input across repeated calls.
func TestTransportClassify_Deterministic(t *testing.T) {
	for i := 0; i < 3; i++ {
		got, flags := Classify(tfp(42), "Aquatic Chronic 1", "perfumery product", nil)
		if !got.Regulated || got.ADR == nil || got.ADR.UNNumber != "1266" || !got.EnvMark {
			t.Fatalf("run %d: unexpected result %+v", i, got)
		}
		if len(flags) != 0 {
			t.Fatalf("run %d: unexpected flags %+v", i, flags)
		}
	}
}

// TestTransportSummary documents the short summary rendering used in FLAG
// messages and log lines.
func TestTransportSummary(t *testing.T) {
	regulated, _ := Classify(tfp(40), "", "perfumery product", nil)
	if got, want := transportSummary(regulated), "UN 1266, Class 3, PG III"; got != want {
		t.Errorf("transportSummary(regulated) = %q, want %q", got, want)
	}

	notRegulated, _ := Classify(tfp(120), "", "perfumery product", nil)
	if got, want := transportSummary(notRegulated), "Not regulated for transport"; got != want {
		t.Errorf("transportSummary(notRegulated) = %q, want %q", got, want)
	}
}

// TestTransportClassify_UN1170Alcohol pins the alcohol/ethanol branch: an ethanol
// or alcohol carrier that is NOT a finished perfumery product ships as UN 1170;
// finished/perfumery alcohol-based products stay UN 1266; extracts stay UN 1197.
func TestTransportClassify_UN1170Alcohol(t *testing.T) {
	for _, tc := range []struct {
		productType string
		wantUN      string
	}{
		{"ethanol", "1170"},
		{"ethanol solution", "1170"},
		{"isopropyl alcohol carrier", "1170"},
		{"alcohol base", "1170"},
		{"perfumery product", "1266"},            // alcohol-based but finished perfumery
		{"finished fragrance (alcohol)", "1266"}, // "finished" outranks the alcohol token
		{"botanical extract", "1197"},
	} {
		got, _ := Classify(tfp(30), "", tc.productType, nil)
		if got.ADR == nil {
			t.Fatalf("%q: expected a regulated result", tc.productType)
		}
		if got.ADR.UNNumber != tc.wantUN {
			t.Errorf("%q → UN %s, want %s", tc.productType, got.ADR.UNNumber, tc.wantUN)
		}
		if tc.wantUN == "1170" && got.ADR.ProperShippingName != psnUN1170 {
			t.Errorf("%q: UN 1170 PSN = %q, want %q", tc.productType, got.ADR.ProperShippingName, psnUN1170)
		}
	}
}

// TestTransportClassify_Regime pins the per-mode DG/LQ/EQ regime selection by
// inner-package size for a PG III UN 1266 product (LQ 5 L, EQ inner 30 ml), and
// that an unknown size is conservatively full DG. Both modes are checked.
func TestTransportClassify_Regime(t *testing.T) {
	for _, tc := range []struct {
		name  string
		inner *float64
		want  string
	}{
		{"30 ml → excepted", tfp(0.03), regimeExcepted},
		{"500 ml → limited", tfp(0.5), regimeLimited},
		{"5 L → limited (inclusive)", tfp(5), regimeLimited},
		{"10 L → full DG", tfp(10), regimeFullDG},
		{"unknown size → full DG", nil, regimeFullDG},
	} {
		got, _ := Classify(tfp(40), "", "perfumery product", tc.inner)
		if got.ADR == nil || got.IATA == nil {
			t.Fatalf("%s: expected regulated result with both legs", tc.name)
		}
		if got.ADR.Regime != tc.want {
			t.Errorf("%s: ADR regime = %q, want %q", tc.name, got.ADR.Regime, tc.want)
		}
		if got.IATA.Regime != tc.want {
			t.Errorf("%s: IATA regime = %q, want %q", tc.name, got.IATA.Regime, tc.want)
		}
	}
}

// TestTransportClassify_ExceptedQuantityLimits pins the PG-driven EQ code and
// inner/outer limits, and that PG II carries the stricter 1 L LQ.
func TestTransportClassify_ExceptedQuantityLimits(t *testing.T) {
	pgIII, _ := Classify(tfp(40), "", "perfumery product", nil) // PG III
	if a := pgIII.ADR; a.ExceptedQuantityCode != "E1" || a.ExceptedQuantityInnerMl != 30 || a.ExceptedQuantityOuterMl != 1000 || a.LimitedQuantityLitres != 5 {
		t.Errorf(
			"PG III: EQ %s %g/%g, LQ %g; want E1 30/1000, LQ 5",
			a.ExceptedQuantityCode,
			a.ExceptedQuantityInnerMl,
			a.ExceptedQuantityOuterMl,
			a.LimitedQuantityLitres,
		)
	}
	pgII, _ := Classify(tfp(18), "", "perfumery product", nil) // PG II
	if a := pgII.ADR; a.ExceptedQuantityCode != "E2" || a.ExceptedQuantityOuterMl != 500 || a.LimitedQuantityLitres != 1 {
		t.Errorf(
			"PG II: EQ %s outer %g, LQ %g; want E2 500, LQ 1",
			a.ExceptedQuantityCode,
			a.ExceptedQuantityOuterMl,
			a.LimitedQuantityLitres,
		)
	}
}

// TestTransportClassify_IATAComputedPerMode verifies the air leg is its own entry:
// it carries its own LQ value and, for a flammable liquid, an air-stricter note;
// a non-flammable UN 3082 air leg carries no flammable note. Every air leg
// carries the standing caveat that its limited quantity is shown from the road
// table, because this engine does not hold the IATA DGR air limits.
func TestTransportClassify_IATAComputedPerMode(t *testing.T) {
	flam, _ := Classify(tfp(40), "", "perfumery product", tfp(0.1)) // flammable PG III
	if flam.IATA == nil {
		t.Fatal("expected IATA entry")
	}
	if flam.IATA.LimitedQuantityLitres != 5 {
		t.Errorf("IATA LQ = %g, want 5", flam.IATA.LimitedQuantityLitres)
	}
	if !strings.Contains(flam.IATA.Notes, "flammable liquids carry stricter") {
		t.Errorf("flammable air leg should carry an air-stricter note, got %q", flam.IATA.Notes)
	}
	if !strings.Contains(flam.IATA.Notes, "IATA DGR") {
		t.Errorf("air leg should carry the DGR verification caveat, got %q", flam.IATA.Notes)
	}

	env, _ := Classify(tfp(95), "Aquatic Acute 1", "perfumery product", tfp(25)) // UN 3082
	if env.IATA == nil {
		t.Fatal("expected IATA entry")
	}
	if strings.Contains(env.IATA.Notes, "flammable liquids carry stricter") {
		t.Errorf("UN 3082 air leg should carry no flammable note, got %q", env.IATA.Notes)
	}
	if !strings.Contains(env.IATA.Notes, "confirm against the current IATA DGR") {
		t.Errorf("every air leg must carry the DGR verification caveat, got %q", env.IATA.Notes)
	}
}

// TestTransportClassify_SP375NamesAirProvision pins that the SP375 not-regulated
// reason names the IATA A197 equivalent, so the air mode is not assumed from road.
func TestTransportClassify_SP375NamesAirProvision(t *testing.T) {
	got, _ := Classify(tfp(95), "Aquatic Chronic 1", "perfumery product", tfp(0.1))
	if !got.Sp375Applied {
		t.Fatal("expected SP375 applied")
	}
	if !strings.Contains(got.NotRegulatedReason, "SP375") || !strings.Contains(got.NotRegulatedReason, "A197") {
		t.Errorf("SP375 reason should name both ADR SP375 and IATA A197, got %q", got.NotRegulatedReason)
	}
}

// TestTransportClassify_SP375StatesNotFlammable pins that the §14 SP375
// not-regulated reason plainly states the product is NOT a flammable liquid /
// solid (not Class 3 / 4.1), alongside the environmental-hazard exemption. The
// bare "exempt under SP375" wording read as borderline Class 3 to carriers; the
// explicit not-flammable statement removes that ambiguity.
func TestTransportClassify_SP375StatesNotFlammable(t *testing.T) {
	liquid, _ := Classify(tfp(95), "Aquatic Chronic 1", "perfumery product", tfp(0.1))
	if !liquid.Sp375Applied {
		t.Fatal("expected SP375 applied for the liquid path")
	}
	if !strings.Contains(liquid.NotRegulatedReason, "Class 3") ||
		!strings.Contains(strings.ToLower(liquid.NotRegulatedReason), "not a flammable liquid") {
		t.Errorf(
			"liquid SP375 reason must state not a flammable liquid / not Class 3, got %q",
			liquid.NotRegulatedReason,
		)
	}

	solid, _ := classifySolidTransport("Aquatic Chronic 1", tfp(0.1))
	if !solid.Sp375Applied {
		t.Fatal("expected SP375 applied for the solid path")
	}
	if !strings.Contains(solid.NotRegulatedReason, "Class 4.1") ||
		!strings.Contains(strings.ToLower(solid.NotRegulatedReason), "not a flammable solid") {
		t.Errorf(
			"solid SP375 reason must state not a flammable solid / not Class 4.1, got %q",
			solid.NotRegulatedReason,
		)
	}
}
