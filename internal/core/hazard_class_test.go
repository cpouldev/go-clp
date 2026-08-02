package core

import (
	"testing"
)

func TestParseHazardClassAcceptsSupportedForms(t *testing.T) {
	tests := []struct {
		input string
		want  HazardClass
	}{
		{"Acute Tox. 4", HazardClassAcuteToxicityOral},
		{"Acute Tox. 2 (dermal)", HazardClassAcuteToxicityDermal},
		{"Acute toxicity 3 inhalation", HazardClassAcuteToxicityInhalation},
		{"Skin Corr. 1B", HazardClassSkinCorrosion},
		{"Skin Irrit. 2", HazardClassSkinIrritation},
		{"Eye Dam. 1", HazardClassSeriousEyeDamage},
		{"Eye Irrit. 2", HazardClassEyeIrritation},
		{"Skin Sens. 1A", HazardClassSkinSensitisation},
		{"Respiratory sensitisation", HazardClassRespiratorySensitisation},
		{"Carc. 1B", HazardClassCarcinogenicity},
		{"Germ cell mutagenicity", HazardClassGermCellMutagenicity},
		{"Repr. 2", HazardClassReproductiveToxicity},
		{"Lact.", HazardClassLactation},
		{"STOT SE 3", HazardClassSTOTSingleExposure},
		{"Specific target organ toxicity, Single exposure, Respiratory tract irritation",
			HazardClassSTOTSingleExposure,
		},
		{"STOT-RE 2", HazardClassSTOTRepeatedExposure},
		{"Asp. Tox. 1", HazardClassAspirationHazard},
		{"Aquatic Acute 1", HazardClassAquaticAcute},
		{"Hazardous to the aquatic environment (Chronic)", HazardClassAquaticChronic},
		{"Hazardous to the aquatic environment", HazardClassAquatic},
		{"Flam. Liq. 3", HazardClassFlammableLiquid},
		{"Pyrophoric solid", HazardClassPyrophoricSolid},
		{"Explosive", HazardClassExplosive},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseHazardClass(tt.input)
			if err != nil {
				t.Fatalf("ParseHazardClass(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseHazardClass(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseHazardClassRejectsUnknownInput(t *testing.T) {
	for _, input := range []string{"", "not a hazard", "possibly Skin Corr. 1", "Carcinogenicity-ish"} {
		t.Run(input, func(t *testing.T) {
			if got, err := ParseHazardClass(input); err == nil {
				t.Errorf("ParseHazardClass(%q) = %q, want error", input, got)
			}
		})
	}
}

func TestClassifyMixtureFlagsUnknownHazardClass(t *testing.T) {
	mix := ClassifyMixture([]ClassComponent{{
		Name:             "unknown input",
		ConcentrationPct: 10,
		Hazards: []ComponentHazard{{
			Class:    "possibly Skin Corr. 1",
			Category: "1",
		}},
	}}, nil, false)

	for _, flag := range mix.Flags {
		if flag.Code == FlagInvalidHazardClass && flag.Severity == SeverityBlock {
			return
		}
	}
	t.Fatalf("ClassifyMixture flags = %+v, want invalid-hazard-class BLOCK", mix.Flags)
}
