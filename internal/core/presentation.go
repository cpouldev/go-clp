package core

// presentation.go derives the bilingual, render-ready view of the data-driven
// SDS sections (§2 label, §3 composition) from the deterministic Classification
// block, resolving every CLP code to its statement text through the phrase
// library (clp_phrases.go). It runs at generate time for BOTH languages so the
// print route renders stored text and never needs a phrase library of its own.
//
// Concentrations are shown as standard CLP disclosure bands, never the exact
// trade-secret % w/w the engine classified on.

import (
	"strconv"
	"strings"
)

// BuildPresentation resolves a language-neutral classification into the Greek
// and English label, composition, and glossary views.
func BuildPresentation(classification SdsClassification) SdsPresentation {
	components := compositionRows(classification.Components)
	return SdsPresentation{
		EL: presentationLang(classification, LangEL, components),
		EN: presentationLang(classification, LangEN, append([]CompositionRow{}, components...)),
	}
}

// presentationLang resolves one language's label rows and composition rows.
func presentationLang(cls SdsClassification, lang Lang, components []CompositionRow) SdsPresentationLang {
	l := cls.Label
	return SdsPresentationLang{
		SignalWord:     presentationSignalWord(l.SignalWord, lang),
		Pictograms:     pictogramViews(l.Pictograms, lang),
		HStatements:    codeTexts(l.HCodes, lang, hStatementText),
		EuhStatements:  euhCodeTexts(l, lang),
		PStatements:    codeTexts(l.PCodes, lang, pStatementText),    // pruned label set
		PStatementsSds: codeTexts(l.PCodesSds, lang, pStatementText), // full §2.2 set
		Components:     components,
		Glossary:       buildGlossary(l, cls.Components, lang),
	}
}

// euhCodeTexts resolves the label EUH codes to statement text, naming the
// EUH208 sensitisers ("Contains <names>. May produce an allergic reaction.")
// from the label's Euh208Substances instead of the generic "contains a
// component" wording. All other EUH codes resolve through the phrase library.
func euhCodeTexts(l SdsLabel, lang Lang) []CodeText {
	out := make([]CodeText, 0, len(l.EuhCodes))
	for _, code := range l.EuhCodes {
		if normKey(code) == "EUH208" {
			out = append(out, CodeText{Code: code, Text: euh208NamedText(l.Euh208Substances, lang)})
			continue
		}
		text, _ := euhStatementText(code, lang)
		out = append(out, CodeText{Code: code, Text: text})
	}
	return out
}

// buildGlossary collects every CLP code shown anywhere in the document — the §2
// mixture-label codes AND the §3 component classification codes — into the
// end-of-document glossary, so every code rendered inline has an authoritative
// entry to link to. Codes are de-duplicated by normalised code and ordered H,
// then EUH, then P (component substances contribute only H-codes); each entry is
// paired with its official text in lang and — for H-codes that carry one — its
// GHS pictogram code (CLP Annex I, via pictogramForHCode). EUH and P codes never
// carry a pictogram. An unknown code keeps an empty Text (the gap is then visible
// in the glossary rather than silently dropped), matching codeTexts' contract.
func buildGlossary(l SdsLabel, comps []ClassificationComponent, lang Lang) []GlossaryEntry {
	// Merge component H-codes into the label H-codes (first-appearance order) so the
	// H→EUH→P grouping holds across both sources and a code disclosed only in §3
	// (e.g. H302) still earns an entry. The dedup below drops the overlap.
	hCodes := append([]string{}, l.HCodes...)
	for _, c := range comps {
		for _, h := range c.Hazards {
			hCodes = append(hCodes, h.HCodes...)
		}
	}

	out := make([]GlossaryEntry, 0, len(hCodes)+len(l.EuhCodes)+len(l.PCodes))
	seen := make(map[string]bool)

	add := func(codes []string, resolve func(string, Lang) (string, bool), withPictogram bool) {
		for _, c := range codes {
			key := normKey(c)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			text, _ := resolve(c, lang)
			e := GlossaryEntry{Code: c, Text: text}
			if withPictogram {
				if p, ok := pictogramForHCode(c); ok {
					e.Pictogram = p
				}
			}
			out = append(out, e)
		}
	}

	add(hCodes, hStatementText, true)
	add(
		l.EuhCodes, func(code string, lg Lang) (string, bool) {
			if normKey(code) == "EUH208" {
				return euh208NamedText(l.Euh208Substances, lg), true
			}
			return euhStatementText(code, lg)
		}, false,
	)
	// Glossary covers the full §2.2 P-set plus any label-only codes; dedup in add
	// keeps first-seen order, so PCodesSds (when populated) drives the ordering.
	add(append(append([]string{}, l.PCodesSds...), l.PCodes...), pStatementText, false)
	return out
}

// presentationSignalWord localises the label signal word ("" when unclassified).
func presentationSignalWord(word string, lang Lang) string {
	if strings.TrimSpace(word) == "" {
		return ""
	}
	if t, ok := signalWordText(word, lang); ok {
		return t
	}
	return word
}

// pictogramViews pairs each pictogram code with its localised alt text.
func pictogramViews(codes []string, lang Lang) []PictogramView {
	out := make([]PictogramView, 0, len(codes))
	for _, code := range codes {
		alt, _ := pictogramAlt(code, lang)
		out = append(out, PictogramView{Code: code, Alt: alt})
	}
	return out
}

// codeTexts pairs each statement code with its resolved text via the supplied
// phrase-library resolver. Unknown codes carry empty text rather than dropping
// the code, so a gap is visible on the rendered label.
func codeTexts(codes []string, lang Lang, resolve func(string, Lang) (string, bool)) []CodeText {
	out := make([]CodeText, 0, len(codes))
	for _, c := range codes {
		text, _ := resolve(c, lang)
		out = append(out, CodeText{Code: c, Text: text})
	}
	return out
}

// compositionRows builds the Section 3 rows from the hazardous substances only
// (carriers and inerts with no hazard entries are not listed in Section 3).
func compositionRows(comps []ClassificationComponent) []CompositionRow {
	rows := make([]CompositionRow, 0, len(comps))
	for _, c := range comps {
		if len(c.Hazards) == 0 {
			continue
		}
		rows = append(
			rows, CompositionRow{
				Name:               c.Name,
				CasNumber:          c.CasNumber,
				EcNumber:           c.EcNumber,
				InciName:           inciSynonym(c.Name, c.InciName),
				ConcentrationRange: concentrationBand(c.ConcentrationPct),
				Classification:     formatClassification(c.Hazards),
			},
		)
	}
	return rows
}

// inciSynonym returns the INCI name to show as a §3 synonym, or "" when it would
// be redundant. A synonym is only worth showing when it differs from the
// substance name already in the row (case- and space-insensitive); when the
// supplier name already IS the INCI name, repeating it adds noise.
func inciSynonym(name, inci string) string {
	inci = strings.TrimSpace(inci)
	if inci == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(name), inci) {
		return ""
	}
	return inci
}

// concentrationBand maps an exact % w/w to the standard CLP disclosure band used
// in Section 3, so the published SDS never exposes the exact formula. Bands are
// numeric and identical in both languages.
func concentrationBand(pct float64) string {
	switch {
	case pct <= 0:
		return ""
	case pct < 0.1:
		return "< 0.1 %"
	case pct < 1:
		return "≥ 0.1 - < 1 %"
	case pct < 5:
		return "≥ 1 - < 5 %"
	case pct < 10:
		return "≥ 5 - < 10 %"
	case pct < 25:
		return "≥ 10 - < 25 %"
	case pct < 50:
		return "≥ 25 - < 50 %"
	case pct < 100:
		return "≥ 50 - < 100 %"
	default:
		return "100 %"
	}
}

// formatClassification renders a substance's hazard list for the Section 3
// classification cell in the CANONICAL CLP short form — "<abbrev> <category>
// (<H-codes>)" per hazard, joined with "; " (e.g. "Skin Sens. 1B (H317); Aquatic
// Chronic 2 (H411)"). The CLP class abbreviation is language-neutral (used
// untranslated in every EU SDS), so the same value renders in the EL and EN
// sheets.
//
// Two clean-ups are applied to the supplier extraction, which is inconsistent:
//  1. the class NAME is normalised to its canonical abbreviation (the supplier's
//     long form "Skin irritation" → "Skin Irrit."), while the extracted CATEGORY
//     is preserved verbatim so 1A/1B precision is not lost; and
//  2. the H-codes are filtered to those that belong to the hazard's class family,
//     because suppliers often repeat a substance's whole code list on every
//     hazard row — matching by family collapses it to one code per class.
//
// Duplicate classes are merged. Unknown classes/codes are kept verbatim so no
// data is silently dropped.
func formatClassification(hazards []ComponentHazard) string {
	var order []string
	codesByLabel := map[string][]string{}
	seenCode := map[string]map[string]bool{}
	mByLabel := map[string]string{}

	for _, h := range hazards {
		abbrev, known := canonicalClassAbbrev(h.Class)
		cat := strings.TrimSpace(h.Category)
		// An empty class text falls back to the family implied by the H-codes, so
		// a coded hazard is never silently dropped from the §3 cell. (A non-empty
		// but unrecognised class is kept verbatim.)
		if !known && strings.TrimSpace(h.Class) == "" {
			for _, raw := range h.HCodes {
				if fam, ok := familyForHCode(raw); ok {
					abbrev, known = fam, true
					break
				}
			}
		}
		base := abbrev
		if !known {
			base = strings.TrimSpace(h.Class)
		}
		label := base
		if cat != "" && !strings.HasSuffix(strings.ToLower(base), strings.ToLower(cat)) {
			label = strings.TrimSpace(base + " " + cat)
		}
		if label == "" {
			continue
		}

		if _, ok := codesByLabel[label]; !ok {
			order = append(order, label)
			seenCode[label] = map[string]bool{}
		}
		// REACH Annex II §3.2: state the M-factor for an aquatic Acute 1 / Chronic 1
		// substance (the multiplier used in the mixture summation). A category-1
		// aquatic hazard always has one — the supplier default is 1 when unassigned.
		if m := aquaticMFactor(h); m != "" {
			mByLabel[label] = m
		}
		// Keep the codes whose family matches this class; if none match (unknown
		// class, or codes the family table does not cover) keep them all so the
		// cell never loses a stated code.
		matched := false
		for _, raw := range h.HCodes {
			if fam, ok := familyForHCode(raw); ok && known && fam == abbrev {
				addCode(codesByLabel, seenCode, label, raw)
				matched = true
			}
		}
		if !matched {
			for _, raw := range h.HCodes {
				addCode(codesByLabel, seenCode, label, raw)
			}
		}
	}

	parts := make([]string, 0, len(order))
	for _, label := range order {
		inner := strings.Join(codesByLabel[label], ", ")
		if m := mByLabel[label]; m != "" {
			if inner != "" {
				inner += ", " + m
			} else {
				inner = m
			}
		}
		if inner != "" {
			parts = append(parts, label+" ("+inner+")")
		} else {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, "; ")
}

// aquaticMFactor returns the CLP M-factor annotation ("M=1", "M=10", …) for an
// aquatic Acute 1 / Chronic 1 hazard so Section 3 states the multiplier used in
// the mixture summation (REACH Annex II §3.2). A category-1 aquatic hazard always
// carries an M-factor — the default is 1 when the supplier assigned none. Returns
// "" for any other hazard (no M-factor applies / is shown).
func aquaticMFactor(h ComponentHazard) string {
	c := strings.ToLower(h.Class)
	if !strings.Contains(c, "aquatic") || strings.TrimSpace(h.Category) != "1" {
		return ""
	}
	m := 1
	switch {
	case strings.Contains(c, "acute"):
		if h.MFactorAcute > 0 {
			m = h.MFactorAcute
		}
	case strings.Contains(c, "chronic"):
		if h.MFactorChronic > 0 {
			m = h.MFactorChronic
		}
	default:
		return ""
	}
	return "M=" + strconv.Itoa(m)
}

// addCode appends an H-code to a label's code list, de-duplicating within that
// label. The code is DISPLAYED verbatim (whitespace stripped) so sub-class
// suffixes survive (H361fd, H350i), while de-duplication keys on the three-digit
// base so a repeated code in suffixed/bare forms is not listed twice.
func addCode(codesByLabel map[string][]string, seen map[string]map[string]bool, label, raw string) {
	disp := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	if disp == "" {
		return
	}
	base := hCodeBase(disp)
	if seen[label][base] {
		return
	}
	seen[label][base] = true
	codesByLabel[label] = append(codesByLabel[label], disp)
}

// canonicalClassAbbrev maps a hazard-class name (the supplier's long form, or an
// already-abbreviated string) to its canonical CLP family abbreviation WITHOUT a
// category — "Skin irritation"/"Skin Irrit. 2" → "Skin Irrit.". Matching is by
// keyword so it is robust to phrasing variants. Returns ("", false) when no class
// is recognised, so the caller keeps the original text. Order matters: the more
// specific class is tested before the more general one (eye damage before eye
// irritation; aquatic before acute toxicity).
func canonicalClassAbbrev(class string) (string, bool) {
	c := strings.ToLower(class)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if !strings.Contains(c, s) {
				return false
			}
		}
		return true
	}
	switch {
	case c == "":
		return "", false
	case has("aquatic", "acute"):
		return "Aquatic Acute", true
	case has("aquatic", "chronic"):
		return "Aquatic Chronic", true
	case has("ozone"):
		return "Ozone", true
	case has("respiratory", "sens"):
		return "Resp. Sens.", true
	case has("skin", "sens"):
		return "Skin Sens.", true
	case has("skin", "corros"):
		return "Skin Corr.", true
	case has("skin", "irrit"):
		return "Skin Irrit.", true
	case has("eye", "damage"), has("eye", "dam."):
		return "Eye Dam.", true
	case has("eye"):
		return "Eye Irrit.", true
	case has("aspiration"), has("asp. tox"):
		return "Asp. Tox.", true
	case has("carcinogen"), has("carc."):
		return "Carc.", true
	case has("mutagen"), has("muta."):
		return "Muta.", true
	case has("reproduct"), has("repr."):
		return "Repr.", true
	case has("lactation"):
		return "Lact.", true
	case has("specific target organ", "single"), has("stot se"):
		return "STOT SE", true
	case has("specific target organ", "repeated"), has("stot re"):
		return "STOT RE", true
	case has("acute", "tox"):
		return "Acute Tox.", true
	case has("flammable", "liquid"), has("flam. liq"):
		return "Flam. Liq.", true
	case has("flammable", "solid"), has("flam. sol"):
		return "Flam. Sol.", true
	case has("corrosive", "metal"), has("met. corr"):
		return "Met. Corr.", true
	case has("aerosol"):
		return "Aerosol", true
	}
	return "", false
}

// classComponentsToStored converts the engine's pure-core components into the
// stored classification breakdown (the deterministic source for Section 3),
// replacing the previously model-authored components list.
func classComponentsToStored(ccs []ClassComponent) []ClassificationComponent {
	out := make([]ClassificationComponent, 0, len(ccs))
	for _, cc := range ccs {
		low := cc.ConcentrationLowPct
		if !cc.FromRange {
			low = 0 // omit the redundant low bound for exact concentrations
		}
		out = append(
			out, ClassificationComponent{
				Name:                cc.Name,
				CasNumber:           cc.CasNumber,
				EcNumber:            cc.EcNumber,
				InciName:            cc.InciName,
				ConcentrationPct:    cc.ConcentrationPct,
				ConcentrationLowPct: low,
				FromRange:           cc.FromRange,
				Hazards:             cc.Hazards,
			},
		)
	}
	return out
}
