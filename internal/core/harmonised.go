package core

import (
	"strings"
	"unicode"
)

// harmonised.go applies CLP Annex VI harmonised classification data — specific
// concentration limits (SCLs) and M-factors — to the resolved components before
// classification. Harmonised data ALWAYS overrides the generic limit, but only
// for the component that carries it (CLP Annex I §1.2). The data is keyed by CAS
// and supplied by the caller through EngineInput.Harmonised; the pure engine
// applies it (applyHarmonised) so the
// classifiers consume already-resolved SCL/M-factor values via the same fields
// the supplier data populates (precedence: harmonised → supplier → generic).
//
// The annexvi subpackage supplies an embedded, date-aware EUR-Lex registry.

// Harmonised is the harmonised classification data for one substance.
type Harmonised struct {
	// MFactorAcute / MFactorChronic are the Annex VI M-factors for Aquatic
	// Acute 1 / Chronic 1. Zero means none is harmonised (the default M=1 or a
	// supplier value applies).
	MFactorAcute   int
	MFactorChronic int

	// SCLs are the substance-specific concentration limits by H-code.
	SCLs []HarmonisedSCL

	// Hazards is the harmonised classification itself: the hazard classes and
	// categories Annex VI makes MANDATORY for the substance. Under CLP Art. 4(3)
	// a supplier may classify for additional hazards but may not omit these, so
	// the engine adds any endpoint the supplier's own data does not cover.
	Hazards []HarmonisedHazard

	// Source is the provenance tag ("annex_vi", "pubchem", "manual").
	Source string
}

// HarmonisedHazard is one hazard class + category from a harmonised
// classification, with the hazard statement it carries where Annex VI pairs one
// unambiguously (HCode may be empty).
type HarmonisedHazard struct {
	HCode    string `json:"hCode,omitempty"`
	Class    string `json:"class"`
	Category string `json:"category"`
}

// HarmonisedSCL is one substance-specific concentration limit: the H-code it
// applies to and the limit in % w/w. JSON tags define the public serialisation
// shape ([{"hCode":"H317","pct":0.5}]).
type HarmonisedSCL struct {
	HCode string  `json:"hCode"`
	Pct   float64 `json:"pct"`
}

// sclForHCode returns the SCL for an H-code, if the substance has one.
func (h Harmonised) sclForHCode(hCode string) (float64, bool) {
	want := strings.ToUpper(strings.TrimSpace(hCode))
	for _, s := range h.SCLs {
		if strings.ToUpper(strings.TrimSpace(s.HCode)) == want && s.Pct > 0 {
			return s.Pct, true
		}
	}
	return 0, false
}

// normCAS normalises a CAS string to the registry key form: canonical
// "NNNNNNN-NN-N" digits and ASCII hyphens, with no leading zeros on the first
// block.
//
// Supplier data reaches this engine from ERP exports, which zero-pad
// ("005989-27-5"), and from PDF-extracted SDS text, which carries en-dashes and
// non-breaking hyphens. Every registry — harmonised, allergen, INCI — is keyed
// on the canonical form, so a non-canonical rendering does not degrade the
// lookup, it misses all three at once: the substance loses its harmonised
// classification while the audit trail still reports it as self-classified.
func normCAS(cas string) string {
	cleaned, numeric := canonicalIdentifier(cas)
	if !numeric {
		return cleaned
	}
	parts := strings.Split(cleaned, "-")
	if len(parts) != 3 {
		return cleaned
	}
	if trimmed := strings.TrimLeft(parts[0], "0"); trimmed != "" {
		parts[0] = trimmed
	}
	return strings.Join(parts, "-")
}

// normEC normalises an EC number to the registry key form. Unlike a CAS number
// an EC number's leading zeros are part of the identifier, so only punctuation
// and spacing are canonicalised.
func normEC(ec string) string {
	cleaned, _ := canonicalIdentifier(ec)
	return cleaned
}

// canonicalIdentifier normalises a numeric chemical identifier's punctuation:
// digits are kept, any Unicode dash becomes an ASCII hyphen, and spacing is
// dropped. numeric is false when the value contains anything else, in which case
// it is returned trimmed and otherwise untouched.
func canonicalIdentifier(value string) (cleaned string, numeric bool) {
	trimmed := strings.TrimSpace(value)
	out := make([]rune, 0, len(trimmed))
	for _, r := range trimmed {
		switch {
		case r >= '0' && r <= '9':
			out = append(out, r)
		case isDashRune(r):
			out = append(out, '-')
		case unicode.IsSpace(r):
			// Interior spacing carries no meaning in a CAS or EC number.
		default:
			return trimmed, false
		}
	}
	return string(out), true
}

// isDashRune reports whether r is any of the Unicode dashes that stand in for
// the ASCII hyphen of a CAS or EC number in copied or PDF-extracted text.
func isDashRune(r rune) bool {
	switch r {
	case '-', '‐', '‑', '‒', '–', '—', '−':
		return true
	}
	return false
}

// applyHarmonised applies the harmonised registry entry for a component's CAS
// (or, failing that, its EC number) and records that a lookup succeeded
// (HarmonisedFound): its M-factors, its per-hazard SCLs, and the harmonised
// classification itself for any endpoint the component does not already carry.
// A nil/empty registry or an unmatched identifier leaves the component untouched
// (HarmonisedFound stays false), so the validation pass can flag it. Harmonised
// values take precedence over any supplier-derived value.
func applyHarmonised(cc *ClassComponent, registry map[string]Harmonised) {
	if len(registry) == 0 {
		return
	}
	h, ok := lookupHarmonised(*cc, registry)
	if !ok {
		return
	}
	cc.HarmonisedFound = true

	// M-factors (aquatic cascade reads MAcute/MChronic).
	if h.MFactorAcute > 0 {
		cc.MAcute = h.MFactorAcute
	}
	if h.MFactorChronic > 0 {
		cc.MChronic = h.MFactorChronic
	}

	// Per-hazard SCLs (effectiveComponentLimit reads ComponentHazard.SCLPct).
	// An SCL is set FOR one hazard class and applies to that class only (CLP
	// Annex I §1.2.1), so it may only land on a hazard of the same endpoint.
	// This has to be checked explicitly because substanceHazards stamps the
	// substance's WHOLE H-code list onto every one of its hazard entries: without
	// the endpoint test, an H317 SCL of 5% would also be read as the limit for a
	// Carc. 2 entry on the same substance (generic limit 1%) and silently switch
	// its H351 off. An unrecognised code is not a match — a limit is only ever
	// applied where its endpoint is confirmed.
	for i := range cc.Hazards {
		kind := hazardEndpoint(cc.Hazards[i])
		for _, code := range cc.Hazards[i].HCodes {
			scl, ok := h.sclForHCode(code)
			if !ok {
				continue
			}
			if k, known := hCodeKind(code); !known || k != kind {
				continue
			}
			cc.Hazards[i].SCLPct = scl
			break
		}

		// Mirror the harmonised M-factor onto the aquatic hazard entries too, so
		// the §3 render reports the factor the cascade actually classified with.
		acute, chronic := aquaticEndpoint(cc.Hazards[i].Class)
		if acute && cc.MAcute > 0 {
			cc.Hazards[i].MFactorAcute = cc.MAcute
		}
		if chronic && cc.MChronic > 0 {
			cc.Hazards[i].MFactorChronic = cc.MChronic
		}
	}

	// The harmonised classification itself is MANDATORY (CLP Art. 4(3)): a
	// supplier may classify for additional hazards, but may not omit or weaken
	// these. Add every harmonised endpoint the component's own hazard list does
	// not already carry — without this, matching Annex VI would contribute
	// nothing while still suppressing the no-harmonised-lookup advisory.
	for _, hh := range h.Hazards {
		// A registry class this engine cannot parse (a physical hazard outside its
		// scope, or a malformed source cell) is skipped rather than injected: it
		// would otherwise surface as an unrecognised-hazard-class BLOCK against
		// the operator, who did not supply it.
		if _, err := ParseHazardClass(hh.Class); err != nil {
			continue
		}
		if componentHasHazardClass(*cc, hh.Class) {
			continue
		}
		hazard := ComponentHazard{Class: hh.Class, Category: hh.Category}
		if code := strings.TrimSpace(hh.HCode); code != "" {
			hazard.HCodes = []string{code}
		}
		if scl, ok := h.sclForHCode(hh.HCode); ok {
			hazard.SCLPct = scl
		}
		if acute, chronic := aquaticEndpoint(hh.Class); acute || chronic {
			if acute {
				hazard.MFactorAcute = h.MFactorAcute
			}
			if chronic {
				hazard.MFactorChronic = h.MFactorChronic
			}
		}
		cc.Hazards = append(cc.Hazards, hazard)
	}

	// Re-derive the dedicated skin-sensitisation and aquatic fields so a
	// harmonised endpoint the supplier omitted reaches those classifiers too.
	applyDedicatedFields(cc)

	// Skin sensitisation reads its SCL from the dedicated field (classifier.go).
	if scl, ok := h.sclForHCode("H317"); ok {
		cc.SkinSensSCLPct = scl
	}
}

// lookupHarmonised finds a component's harmonised entry by CAS, falling back to
// its EC number: several hundred Annex VI entries carry an EC number and no CAS,
// and a CAS-only lookup leaves those permanently unreachable.
func lookupHarmonised(cc ClassComponent, registry map[string]Harmonised) (Harmonised, bool) {
	if cc.CasNumber != "" {
		if h, ok := registry[normCAS(cc.CasNumber)]; ok {
			return h, true
		}
	}
	if cc.EcNumber != "" {
		if h, ok := registry[normEC(cc.EcNumber)]; ok {
			return h, true
		}
	}
	return Harmonised{}, false
}

// componentHasHazardClass reports whether a component already carries a hazard
// of the same CLP class family. It compares parsed families rather than kinds
// because classKind buckets skin sensitisation and the aquatic classes together
// as kindOther.
func componentHasHazardClass(cc ClassComponent, class string) bool {
	want, err := ParseHazardClass(class)
	if err != nil {
		return false
	}
	for _, h := range cc.Hazards {
		if got, err := ParseHazardClass(h.Class); err == nil && got == want {
			return true
		}
	}
	return false
}

// aquaticEndpoint reports which aquatic tier a hazard class names. It parses the
// class rather than reading classKind, which buckets the aquatic classes into
// kindOther because classifier.go owns their cascade.
func aquaticEndpoint(class string) (acute, chronic bool) {
	parsed, err := ParseHazardClass(class)
	if err != nil {
		return false, false
	}
	switch parsed {
	case HazardClassAquaticAcute:
		return true, false
	case HazardClassAquaticChronic:
		return false, true
	case HazardClassAquatic:
		return true, true
	}
	return false, false
}

// applyAllergenFlag marks a component when its CAS is on the EU Annex III
// fragrance-allergen list. A nil/empty set or an unmatched/blank CAS leaves the
// component unmarked. This only annotates the component for the validation pass
// (FlagAllergenCoverage); it never affects the classification or the label.
func applyAllergenFlag(cc *ClassComponent, allergenCAS map[string]bool) {
	if len(allergenCAS) == 0 || cc.CasNumber == "" {
		return
	}
	if allergenCAS[normCAS(cc.CasNumber)] {
		cc.IsFragranceAllergen = true
	}
}

// applyInciName attaches the INCI cosmetic-nomenclature synonym to a component
// when its CAS is in the preloaded registry map. A nil/empty map or an
// unmatched/blank CAS leaves the component's InciName empty. Presentation only —
// it never affects the classification or the label.
func applyInciName(cc *ClassComponent, inciNames map[string]string) {
	if len(inciNames) == 0 || cc.CasNumber == "" {
		return
	}
	if name := strings.TrimSpace(inciNames[normCAS(cc.CasNumber)]); name != "" {
		cc.InciName = name
	}
}

// hasHarmonisableHazard reports whether a component carries a hazard for which a
// harmonised SCL or M-factor could matter — skin/respiratory sensitisation, CMR,
// or the aquatic classes. Those are the endpoints where a missing harmonised
// lookup could change the classification, so only they warrant the review flag.
func hasHarmonisableHazard(c ClassComponent) bool {
	if c.SkinSensCategory != "" || c.AquaticAcuteCategory != "" || c.AquaticChronicCategory != "" {
		return true
	}
	for _, h := range c.Hazards {
		switch classKind(h.Class) {
		case kindCarc, kindMuta, kindRepr, kindRespSens:
			return true
		}
	}
	return false
}
