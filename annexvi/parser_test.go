package annexvi

import (
	"reflect"
	"testing"
)

func TestParseSpecificLimits(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSCLs    []SCL
		wantAcute   *int
		wantChronic *int
	}{
		{
			name:     "simple skin sensitisation",
			input:    "Skin Sens. 1; H317: C ≥ 1 %",
			wantSCLs: []SCL{{HCode: "H317", Pct: 1}},
		},
		{
			name:     "European decimal comma",
			input:    "Repr. 1B; H360D: C ≥ 0,3 %",
			wantSCLs: []SCL{{HCode: "H360D", Pct: 0.3}},
		},
		{
			name:     "range uses lower bound",
			input:    "Skin Irrit. 2; H315: 1 % ≤ C < 5 %",
			wantSCLs: []SCL{{HCode: "H315", Pct: 1}},
		},
		{
			name:  "qualified acute and chronic factors",
			input: "M (acute) = 100; M (chronic) = 10",
			wantAcute: func() *int {
				v := 100
				return &v
			}(),
			wantChronic: func() *int {
				v := 10
				return &v
			}(),
		},
		{
			name:  "class-keyed distinct factors",
			input: "Aquatic Acute 1: M = 10; Aquatic Chronic 1: M = 1",
			wantAcute: func() *int {
				v := 10
				return &v
			}(),
			wantChronic: func() *int {
				v := 1
				return &v
			}(),
		},
		{
			name:     "SCL and chronic factor",
			input:    "Skin Sens. 1; H317: C ≥ 0,1 %; Aquatic Chronic 1; H410: M = 10",
			wantSCLs: []SCL{{HCode: "H317", Pct: 0.1}},
			wantChronic: func() *int {
				v := 10
				return &v
			}(),
		},
		{name: "ATE only is ignored", input: "oral: ATE = 890,0 mg/kg bw"},
		{name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSCLs, gotAcute, gotChronic := ParseSpecificLimits(tt.input)
			if !reflect.DeepEqual(gotSCLs, tt.wantSCLs) {
				t.Errorf("SCLs = %#v, want %#v", gotSCLs, tt.wantSCLs)
			}
			if !equalInt(gotAcute, tt.wantAcute) {
				t.Errorf("acute M-factor = %v, want %v", intValue(gotAcute), intValue(tt.wantAcute))
			}
			if !equalInt(gotChronic, tt.wantChronic) {
				t.Errorf("chronic M-factor = %v, want %v", intValue(gotChronic), intValue(tt.wantChronic))
			}
		})
	}
}

func TestParseSpecificLimitsMultipleSCLs(t *testing.T) {
	got, _, _ := ParseSpecificLimits("Skin Corr. 1B; H314: C ≥ 5 %; Eye Dam. 1; H318: C ≥ 5 %; Eye Irrit. 2; H319: 1 % ≤ C < 5 %")
	want := []SCL{
		{HCode: "H314", Pct: 5},
		{HCode: "H318", Pct: 5},
		{HCode: "H319", Pct: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSpecificLimits = %#v, want %#v", got, want)
	}
}

func TestParseSpecificLimitsEURLEXParagraphs(t *testing.T) {
	got, _, _ := ParseSpecificLimits("Eye Dam. 1; H318: C ≥ 22,0 %\nEye Irrit. 2; H319: 14,0 % ≤ C < 22,0 %")
	want := []SCL{{HCode: "H318", Pct: 22}, {HCode: "H319", Pct: 14}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSpecificLimits = %#v, want %#v", got, want)
	}
}

func TestParseHazards(t *testing.T) {
	tests := []struct {
		name       string
		classes    string
		statements string
		want       []HarmonisedHazard
	}{
		{
			name:       "category variants",
			classes:    "Carc. 1B; Repr. 2; STOT SE 3",
			statements: "H350; H361; H335",
			want: []HarmonisedHazard{
				{HCode: "H350", Class: "Carc.", Category: "1B"},
				{HCode: "H361", Class: "Repr.", Category: "2"},
				{HCode: "H335", Class: "STOT SE", Category: "3"},
			},
		},
		{
			name:       "newline separated and uneven",
			classes:    "Flam. Gas 1\nPress. Gas",
			statements: "H220",
			// Press. Gas has no hazard statement of its own; it is still part of
			// the harmonised classification and must not be truncated away.
			want: []HarmonisedHazard{
				{HCode: "H220", Class: "Flam. Gas", Category: "1"},
				{Class: "Press. Gas"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseHazards(tt.classes, tt.statements); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseHazards = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCleanCASFootnotes(t *testing.T) {
	tests := map[string]string{
		"10043-35-3 [1]":    "10043-35-3",
		"7440-50-8 [1] [B]": "7440-50-8",
		" 1333-74-0 ":       "1333-74-0",
		"-":                 "-",
		"[1]":               "",
	}
	for input, want := range tests {
		if got := cleanCAS(input); got != want {
			t.Errorf("cleanCAS(%q) = %q, want %q", input, got, want)
		}
	}
}

func equalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func intValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
