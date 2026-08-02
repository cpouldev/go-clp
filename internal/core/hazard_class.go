package core

import (
	"fmt"
	"strings"
)

// HazardClass is a validated CLP hazard-class family. ParsedHazard keeps its
// Class field as a string for straightforward JSON interoperability; callers
// can use ParseHazardClass at their input boundary before constructing it.
type HazardClass string

const (
	HazardClassAcuteToxicityOral        HazardClass = "acute_toxicity_oral"
	HazardClassAcuteToxicityDermal      HazardClass = "acute_toxicity_dermal"
	HazardClassAcuteToxicityInhalation  HazardClass = "acute_toxicity_inhalation"
	HazardClassSkinCorrosion            HazardClass = "skin_corrosion"
	HazardClassSkinIrritation           HazardClass = "skin_irritation"
	HazardClassSeriousEyeDamage         HazardClass = "serious_eye_damage"
	HazardClassEyeIrritation            HazardClass = "eye_irritation"
	HazardClassSkinSensitisation        HazardClass = "skin_sensitisation"
	HazardClassRespiratorySensitisation HazardClass = "respiratory_sensitisation"
	HazardClassCarcinogenicity          HazardClass = "carcinogenicity"
	HazardClassGermCellMutagenicity     HazardClass = "germ_cell_mutagenicity"
	HazardClassReproductiveToxicity     HazardClass = "reproductive_toxicity"
	HazardClassLactation                HazardClass = "lactation"
	HazardClassSTOTSingleExposure       HazardClass = "stot_single_exposure"
	HazardClassSTOTRepeatedExposure     HazardClass = "stot_repeated_exposure"
	HazardClassAspirationHazard         HazardClass = "aspiration_hazard"
	HazardClassAquatic                  HazardClass = "aquatic"
	HazardClassAquaticAcute             HazardClass = "aquatic_acute"
	HazardClassAquaticChronic           HazardClass = "aquatic_chronic"
	HazardClassFlammableLiquid          HazardClass = "flammable_liquid"
	HazardClassPyrophoricSolid          HazardClass = "pyrophoric_solid"
	HazardClassExplosive                HazardClass = "explosive"
	// HazardClassOther is a recognised CLP class outside this engine's v1
	// classification scope, such as oxidising gases or organic peroxides.
	HazardClassOther HazardClass = "other"
)

var hazardClassReplacer = strings.NewReplacer(
	".", " ", ",", " ", "(", " ", ")", " ", "-", " ", "_", " ",
)

// ParseHazardClass validates class and returns its canonical family. Matching
// is case-insensitive and accepts official abbreviations, spelled-out names,
// category suffixes, and documented route qualifiers. It never uses substring
// matching: surrounding or invented text returns an error.
func ParseHazardClass(class string) (HazardClass, error) {
	normalized := normalizeHazardClass(class)
	if normalized == "" {
		return "", fmt.Errorf("clp: empty hazard class")
	}

	if parsed, ok := parseAcuteToxicity(normalized); ok {
		return parsed, nil
	}
	if parsed, ok := parseSTOT(normalized); ok {
		return parsed, nil
	}
	base := stripHazardCategory(normalized)

	switch base {
	case "skin corr", "skin corrosion":
		return HazardClassSkinCorrosion, nil
	case "skin irrit", "skin irritation":
		return HazardClassSkinIrritation, nil
	case "eye dam", "serious eye damage":
		return HazardClassSeriousEyeDamage, nil
	case "eye irrit", "eye irritation":
		return HazardClassEyeIrritation, nil
	case "skin sens", "skin sensitisation", "skin sensitization", "skin sensitising", "skin sensitizing":
		return HazardClassSkinSensitisation, nil
	case "resp sens", "respiratory sens", "respiratory sensitisation", "respiratory sensitization":
		return HazardClassRespiratorySensitisation, nil
	case "carc", "carcinogenicity":
		return HazardClassCarcinogenicity, nil
	case "muta", "mutagenicity", "germ cell mutagenicity":
		return HazardClassGermCellMutagenicity, nil
	case "repr", "reproductive toxicity", "toxicity to reproduction":
		return HazardClassReproductiveToxicity, nil
	case "lact", "lactation", "effects on or via lactation":
		return HazardClassLactation, nil
	case "asp tox", "aspiration", "aspiration hazard":
		return HazardClassAspirationHazard, nil
	case "aquatic acute", "hazardous to aquatic environment acute", "hazardous to the aquatic environment acute":
		return HazardClassAquaticAcute, nil
	case "aquatic chronic", "hazardous to aquatic environment chronic", "hazardous to the aquatic environment chronic":
		return HazardClassAquaticChronic, nil
	case "aquatic", "hazardous to aquatic environment", "hazardous to the aquatic environment":
		return HazardClassAquatic, nil
	case "flam liq", "flammable liquid":
		return HazardClassFlammableLiquid, nil
	case "pyrophoric solid":
		return HazardClassPyrophoricSolid, nil
	case "explosive", "explosives":
		return HazardClassExplosive, nil
	}

	if _, ok := recognisedOtherHazardClasses[base]; ok {
		return HazardClassOther, nil
	}
	return "", fmt.Errorf("clp: unrecognised hazard class %q", class)
}

func normalizeHazardClass(class string) string {
	return strings.Join(strings.Fields(strings.ToLower(hazardClassReplacer.Replace(class))), " ")
}

func parseAcuteToxicity(class string) (HazardClass, bool) {
	route := HazardClassAcuteToxicityOral
	for _, suffix := range []struct {
		text  string
		class HazardClass
	}{
		{" inhalation", HazardClassAcuteToxicityInhalation},
		{" inhal", HazardClassAcuteToxicityInhalation},
		{" dermal", HazardClassAcuteToxicityDermal},
		{" oral", HazardClassAcuteToxicityOral},
	} {
		if strings.HasSuffix(class, suffix.text) {
			route = suffix.class
			class = strings.TrimSuffix(class, suffix.text)
			break
		}
	}
	base := stripHazardCategory(class)
	if base == "acute tox" || base == "acute toxicity" {
		return route, true
	}
	return "", false
}

func parseSTOT(class string) (HazardClass, bool) {
	for _, qualifier := range []string{" respiratory tract irritation", " narcotic effects"} {
		class = strings.TrimSuffix(class, qualifier)
	}
	base := stripHazardCategory(class)
	switch base {
	case "stot se", "stot single exposure", "specific target organ toxicity single exposure":
		return HazardClassSTOTSingleExposure, true
	case "stot re", "stot repeated exposure", "specific target organ toxicity repeated exposure":
		return HazardClassSTOTRepeatedExposure, true
	default:
		return "", false
	}
}

func stripHazardCategory(class string) string {
	fields := strings.Fields(class)
	if len(fields) == 0 || !isHazardCategory(fields[len(fields)-1]) {
		return class
	}
	fields = fields[:len(fields)-1]
	if len(fields) > 0 && (fields[len(fields)-1] == "cat" || fields[len(fields)-1] == "category") {
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}

func isHazardCategory(token string) bool {
	if len(token) < 1 || len(token) > 2 || token[0] < '1' || token[0] > '4' {
		return false
	}
	return len(token) == 1 || token[1] == 'a' || token[1] == 'b' || token[1] == 'c'
}

var recognisedOtherHazardClasses = map[string]struct{}{
	"flammable gas": {}, "flam gas": {}, "aerosol": {},
	"oxidising gas": {}, "oxidizing gas": {}, "gas under pressure": {},
	"flammable solid": {}, "flam sol": {}, "self reactive substance and mixture": {},
	"self reactive substances and mixtures": {}, "pyrophoric liquid": {},
	"self heating substance and mixture": {}, "self heating substances and mixtures": {},
	"substance and mixture which in contact with water emit flammable gas":     {},
	"substances and mixtures which in contact with water emit flammable gases": {},
	"oxidising liquid": {}, "oxidizing liquid": {}, "oxidising solid": {}, "oxidizing solid": {},
	"organic peroxide": {}, "corrosive to metals": {}, "met corr": {},
	"desensitised explosive": {}, "desensitized explosive": {},
	"hazardous to the ozone layer": {}, "ozone": {},
}
