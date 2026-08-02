package core

import (
	"fmt"
	"math"
	"strings"
)

// validation.go is the post-classification review pass. After the deterministic
// engine produces the finished-mixture classification, it re-examines the
// decisions and surfaces — as INFO/WARN review flags, never errors — every call
// an operator should sanity-check:
//
//	(a) range-based decisions that would FLIP between the lower and upper bound
//	    of a supplier concentration range (the trigger used the worst-case upper
//	    bound) → FlagRangeBased (warn);
//	(b) decisions sitting within 10% (relative) of their CLP threshold, where a
//	    small concentration revision could change the outcome → FlagNearThreshold
//	    (info);
//	(c) components classified with no supplier SDS on file — raised upstream as
//	    the sds_unmapped flag (excludeUnmappedLines), so it is not duplicated here.
//	(d) components with a hazard whose outcome can hinge on a harmonised SCL /
//	    M-factor for which no CLP Annex VI entry was found → FlagNoHarmonisedLookup
//	    (info), raised only when a harmonised registry was actually consulted.
//
// The flags are advisory: they tell the operator where to confirm the exact
// % w/w, not that the classification is wrong.

// nearThresholdRelative is the relative band around a CLP threshold within which
// a decision is flagged for review (10% per the requirement).
const nearThresholdRelative = 0.10

// validateClassification runs the review pass over the resolved components and
// returns its advisory flags. flashPt is the mixture flash point used for the
// range-flip re-classification (nil = unknown). registryAvailable reports whether
// a harmonised CLP Annex VI registry was consulted for this run; only then is the
// missing-harmonised-lookup pass run (otherwise the feature is simply not active
// and flagging every component would be noise).
func validateClassification(components []ClassComponent, flashPt *float64, registryAvailable bool) []Flag {
	var flags []Flag
	flags = append(flags, validateRangeFlips(components, flashPt)...)
	flags = append(flags, validateNearThresholds(components)...)
	flags = append(flags, validateUndisclosedRemainder(components)...)
	flags = append(flags, validateAllergenCoverage(components)...)
	if registryAvailable {
		flags = append(flags, validateHarmonisedLookups(components)...)
	}
	return flags
}

// allergenDisclosureThreshold is the % w/w at/above which an undisclosed EU
// Annex III fragrance allergen warrants a coverage review. It is the generic
// EUH208 disclosure limit for a skin sensitiser (Skin Sens. 1 / 1B); a substance
// with a lower specific limit may need disclosure below it, which is exactly what
// the advisory asks the operator to confirm.
const allergenDisclosureThreshold = 0.1

// validateAllergenCoverage flags each component that is on the EU Annex III
// fragrance-allergen list (IsFragranceAllergen) and present at/above the generic
// EUH208 threshold, yet carries NO supplier skin-sensitisation classification —
// so the engine's EUH208 author (which keys off the CLP skin-sens classification)
// had nothing to disclose it with. This is the allergen-list coverage net: it
// does not change the classification or label, it tells the operator to confirm
// whether EUH208 "contains <name>" disclosure is required and whether the supplier
// classification is complete. A component the supplier DID classify as a skin
// sensitiser is left to the existing H317 / EUH208 logic and is not re-flagged.
func validateAllergenCoverage(components []ClassComponent) []Flag {
	var flags []Flag
	for _, c := range components {
		if !c.IsFragranceAllergen || c.SkinSensCategory != "" {
			continue
		}
		if c.ConcentrationPct < allergenDisclosureThreshold {
			continue
		}
		flags = append(
			flags, Flag{
				Section:  "2.2",
				Code:     FlagAllergenCoverage,
				Severity: SeverityWarn,
				Message: fmt.Sprintf(
					"Known EU fragrance allergen not disclosed: %q (CAS %s) is on the Annex III fragrance-allergen list and is present at %.3g%%, but the supplier SDS gives it no skin-sensitisation classification — so no EUH208 statement was authored for it. Confirm whether EUH208 \"contains %s. May produce an allergic reaction.\" disclosure is required and whether the supplier classification is complete.",
					c.Name, casOrUnknown(c.CasNumber), c.ConcentrationPct, c.Name,
				),
			},
		)
	}
	return flags
}

// validateHarmonisedLookups flags, as a SINGLE roll-up advisory, the components
// that carry a hazard whose classification can hinge on a substance-specific
// concentration limit or M-factor (per hasHarmonisableHazard) but for which no
// harmonised CLP Annex VI entry was found (HarmonisedFound is false) — so only the
// generic limit or a supplier value was applied.
//
// The harmonised registry (clp_harmonised) holds the full CLP Annex VI Table 3, so
// "not found" means the substance is self-classified — the normal case for the bulk
// of a fragrance's constituents. Emitting one flag per such substance produced
// 20–30 non-actionable INFO flags on a typical fragrance SDS and buried the genuine
// warnings (range-flip, allergen coverage). One roll-up note, naming the substances,
// keeps the audit trail without the noise.
func validateHarmonisedLookups(components []ClassComponent) []Flag {
	var names []string
	seen := map[string]bool{}
	for _, c := range components {
		if c.HarmonisedFound || !hasHarmonisableHazard(c) {
			continue
		}
		label := strings.TrimSpace(c.Name)
		if label == "" {
			label = "CAS " + casOrUnknown(c.CasNumber)
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		names = append(names, label)
	}
	if len(names) == 0 {
		return nil
	}
	return []Flag{
		{
			Section:  "3.2",
			Code:     FlagNoHarmonisedLookup,
			Severity: SeverityInfo,
			Message: fmt.Sprintf(
				"%d constituent(s) are self-classified with no CLP Annex VI harmonised entry, so the generic concentration limits / supplier classifications were applied (no harmonised SCL or M-factor on file): %s. Confirm none carries a harmonised SCL/M-factor that would change the outcome.",
				len(names), strings.Join(names, ", "),
			),
		},
	}
}

// casOrUnknown renders a CAS for a flag message, falling back to a placeholder.
func casOrUnknown(cas string) string {
	if strings.TrimSpace(cas) == "" {
		return "not stated"
	}
	return cas
}

// validateRangeFlips re-runs the mixture classification with every range-based
// component at its LOWER bound and flags each mixture hazard triggered at the
// upper bound but not at the lower bound — a decision that hinges on the range.
func validateRangeFlips(components []ClassComponent, flashPt *float64) []Flag {
	hasRange := false
	for _, c := range components {
		if c.FromRange {
			hasRange = true
			break
		}
	}
	if !hasRange {
		return nil
	}

	high := ClassifyMixture(components, flashPt, false)
	low := ClassifyMixture(lowerBoundComponents(components), flashPt, false)

	lowClasses := make(map[string]bool, len(low.Hazards))
	for _, h := range low.Hazards {
		lowClasses[h.Class] = true
	}

	var flags []Flag
	for _, h := range high.Hazards {
		if lowClasses[h.Class] {
			continue
		}
		flags = append(
			flags, Flag{
				Section:  "2.1",
				Code:     FlagRangeBased,
				Severity: SeverityWarn,
				Message: fmt.Sprintf(
					"Range-based, confirm exact %%: %s (%s) triggers at the worst-case (upper) bound of a supplier concentration range but NOT at the lower bound. Confirm the exact %% w/w of the contributing component(s).",
					h.Class, strings.Join(h.HCodes, ", "),
				),
			},
		)
	}
	return flags
}

// lowerBoundComponents copies the components with each range-based component's
// concentration set to its lower bound; exact components are unchanged. Only the
// concentration changes — the per-substance hazard categories do not depend on it.
func lowerBoundComponents(components []ClassComponent) []ClassComponent {
	out := make([]ClassComponent, len(components))
	copy(out, components)
	for i := range out {
		if out[i].FromRange {
			out[i].ConcentrationPct = out[i].ConcentrationLowPct
		}
	}
	return out
}

// validateNearThresholds flags decisions whose governing concentration sits
// within nearThresholdRelative of its CLP threshold.
func validateNearThresholds(components []ClassComponent) []Flag {
	var flags []Flag

	// Per-component classes (no additivity): skin sensitisation, CMR, and
	// respiratory sensitisation — each compared to the limit that governs it.
	for _, c := range components {
		if limit, ok := skinSensH317Limit(c); ok && nearThreshold(c.ConcentrationPct, limit) {
			flags = append(flags, nearFlag(c.Name, "Skin Sens. 1 (H317)", c.ConcentrationPct, limit))
		}
		flags = append(flags, nearCMRFlags(c)...)
		flags = append(flags, nearRespSensFlags(c)...)
	}

	// Additive / summation classes (mixture level).
	flags = append(flags, nearAquaticFlags(components)...)
	flags = append(flags, nearIrritationFlags(components)...)
	flags = append(flags, nearStotAspFlags(components)...)
	return flags
}

// nearRespSensFlags flags a component whose concentration is within 10% of its
// respiratory-sensitisation limit (the per-component GCL, or a substance-specific
// limit when one applies). Respiratory sensitisation is per-component, never
// additive, so each component is checked against its own limit.
func nearRespSensFlags(c ClassComponent) []Flag {
	var flags []Flag
	for _, h := range c.Hazards {
		if classKind(h.Class) != kindRespSens {
			continue
		}
		if limit, ok := effectiveComponentLimit(h, kindRespSens); ok && nearThreshold(
			c.ConcentrationPct,
			limit,
		) {
			flags = append(flags, nearFlag(c.Name, "Resp. Sens. 1 (H334)", c.ConcentrationPct, limit))
		}
	}
	return flags
}

// nearStotAspFlags flags the STOT-SE/RE and aspiration summations that sit within
// 10% of their CLP thresholds, mirroring the per-category sums the classifiers use
// (classifyStotSE/RE sum per category; aspiration sums all H304 components).
func nearStotAspFlags(components []ClassComponent) []Flag {
	return summationNearFlags(
		[]nearCheck{
			{"STOT SE 1 summation", sumByKindCat(components, kindStotSE, "1"), mustGCL(kindStotSE, "1")},
			{"STOT SE 2 summation", sumByKindCat(components, kindStotSE, "2"), mustGCL(kindStotSE, "2")},
			{"STOT SE 3 summation", sumByKindCat(components, kindStotSE, "3"), mustGCL(kindStotSE, "3")},
			{"STOT RE 1 summation", sumByKindCat(components, kindStotRE, "1"), mustGCL(kindStotRE, "1")},
			{"STOT RE 2 summation", sumByKindCat(components, kindStotRE, "2"), mustGCL(kindStotRE, "2")},
			{"Aspiration (H304) summation", sumByKind(components, kindAsp), mustGCL(kindAsp, "1")},
		},
	)
}

// sumByKindCat sums the concentrations of components carrying a hazard of the
// given kind AND category. A component is counted once per (kind, category),
// mirroring the per-category summation in classifyStotSE / classifyStotRE.
func sumByKindCat(components []ClassComponent, kind hazardKind, cat string) float64 {
	want := normHazardCategory(cat)
	var sum float64
	for _, c := range components {
		for _, h := range c.Hazards {
			if classKind(h.Class) == kind && normHazardCategory(h.Category) == want {
				sum += c.ConcentrationPct
				break
			}
		}
	}
	return sum
}

// nearThreshold reports whether value sits within nearThresholdRelative of a
// positive threshold (relative distance). A zero/absent value or threshold is
// never "near".
func nearThreshold(value, threshold float64) bool {
	if threshold <= 0 || value <= 0 {
		return false
	}
	return math.Abs(value-threshold)/threshold <= nearThresholdRelative
}

// nearFlag builds a near-threshold review flag for a per-component decision.
func nearFlag(name, decision string, value, threshold float64) Flag {
	return Flag{
		Section:  "2.1",
		Code:     FlagNearThreshold,
		Severity: SeverityInfo,
		Message: fmt.Sprintf(
			"Near threshold (within 10%%): %s — %q sits at %.3g%% versus the %.3g%% limit. A small concentration revision could change the classification; confirm the exact %% w/w.",
			decision, name, value, threshold,
		),
	}
}

// nearCheck is one mixture-level summation metric (name + computed value + CLP
// threshold) tested for proximity to its limit by summationNearFlags.
type nearCheck struct {
	name      string
	value     float64
	threshold float64
}

// summationNearFlags emits a near_threshold review flag for each summation check
// whose value sits within nearThresholdRelative of its threshold. Shared by the
// STOT/aspiration, irritation, and aquatic summation passes.
func summationNearFlags(checks []nearCheck) []Flag {
	var flags []Flag
	for _, ch := range checks {
		if nearThreshold(ch.value, ch.threshold) {
			flags = append(
				flags, Flag{
					Section:  "2.1",
					Code:     FlagNearThreshold,
					Severity: SeverityInfo,
					Message: fmt.Sprintf(
						"Near threshold (within 10%%): %s is %.3g%% versus the %.3g%% limit. Confirm the exact concentrations.",
						ch.name, ch.value, ch.threshold,
					),
				},
			)
		}
	}
	return flags
}

// nearCMRFlags flags a component whose concentration is within 10% of its
// carcinogenicity / mutagenicity / reproductive-toxicity generic limit (these
// classes are per-component, never additive).
func nearCMRFlags(c ClassComponent) []Flag {
	var flags []Flag
	for _, h := range c.Hazards {
		var decision string
		kind := classKind(h.Class)
		switch kind {
		case kindCarc:
			decision = "Carc. (H350/H351)"
		case kindMuta:
			decision = "Muta. (H340/H341)"
		case kindRepr:
			decision = "Repr. (H360/H361)"
		default:
			continue
		}
		threshold, ok := gclFor(kind, h.Category)
		if ok && nearThreshold(c.ConcentrationPct, threshold) {
			flags = append(flags, nearFlag(c.Name, decision, c.ConcentrationPct, threshold))
		}
	}
	return flags
}

// nearAquaticFlags flags the aquatic summation metrics that sit within 10% of
// the 25% threshold, using the same weighted sums and cross-tier coefficients
// as ClassifyAquatic (Chronic 1 weighted 10× at the Chronic 2 rung, 100× at the
// Chronic 3 rung, and the UNWEIGHTED Chronic 1+2+3+4 sum at the Chronic 4
// safety-net rung) so every rung the classifier decides on is also watched here.
func nearAquaticFlags(components []ClassComponent) []Flag {
	var acuteSum, chron1Sum, chron2Sum, chron3Sum, chron4Sum, chron1Raw float64
	for _, c := range components {
		if normHazardCategory(c.AquaticAcuteCategory) == "1" {
			acuteSum += c.ConcentrationPct * mFactor(c.MAcute)
		}
		switch normHazardCategory(c.AquaticChronicCategory) {
		case "1":
			chron1Sum += c.ConcentrationPct * mFactor(c.MChronic)
			chron1Raw += c.ConcentrationPct // unweighted, for the Chronic 4 safety net
		case "2":
			chron2Sum += c.ConcentrationPct
		case "3":
			chron3Sum += c.ConcentrationPct
		case "4":
			chron4Sum += c.ConcentrationPct
		}
	}
	return summationNearFlags(
		[]nearCheck{
			{"Aquatic Acute 1 summation", acuteSum, aquaticThreshld},
			{"Aquatic Chronic 1 summation", chron1Sum, aquaticThreshld},
			{"Aquatic Chronic 2 summation (10×Chronic1 + Chronic2)", 10*chron1Sum + chron2Sum, aquaticThreshld},
			{
				"Aquatic Chronic 3 summation (100×Chronic1 + 10×Chronic2 + Chronic3)",
				100*chron1Sum + 10*chron2Sum + chron3Sum, aquaticThreshld,
			},
			{
				"Aquatic Chronic 4 safety-net summation (Chronic1 + Chronic2 + Chronic3 + Chronic4, unweighted)",
				chron1Raw + chron2Sum + chron3Sum + chron4Sum, aquaticThreshld,
			},
		},
	)
}

// nearIrritationFlags flags the additive skin/eye irritation and corrosion
// summations that sit within 10% of their thresholds.
func nearIrritationFlags(components []ClassComponent) []Flag {
	sumCorr := sumByKind(components, kindSkinCorr)
	sumSkinIrrit := sumByKind(components, kindSkinIrrit)
	sumEyeDam := sumByAnyKind(components, kindEyeDam, kindSkinCorr)
	sumEyeIrrit := sumByKind(components, kindEyeIrrit)

	return summationNearFlags(
		[]nearCheck{
			{"Skin Corr. 1 summation", sumCorr, gclSkinCorr1},
			{"Skin Irrit. 2 summation (10×Corr + Irrit)", corrToIrritBoost*sumCorr + sumSkinIrrit, gclSkinIrrit2},
			{"Eye Dam. 1 summation", sumEyeDam, gclEyeDam1},
			{"Eye Irrit. 2 summation (10×EyeDam + EyeIrrit)", corrToIrritBoost*sumEyeDam + sumEyeIrrit, gclEyeIrrit},
		},
	)
}

// ── Endpoint-not-evaluated (silent non-classification) ───────────────────────

// validateEndpointsEvaluated flags component hazards the engine could NOT turn
// into a classification decision — the silent non-classification failure mode the
// systematic refactor targets. Two cases: (a) a recognised endpoint whose category
// is absent/unusable, so the hazard was dropped from its summation; (b) a hazard
// class the engine does not classify at all. Both warrant operator review. Skin
// sensitisation and the aquatic classes (handled by classifier.go) and mixture-
// level flammability (decided from the flash point) are treated as evaluated.
func validateEndpointsEvaluated(components []ClassComponent) []Flag {
	var flags []Flag
	seen := make(map[string]bool)
	for _, c := range components {
		for _, h := range c.Hazards {
			ok, endpoint := hazardEvaluable(h)
			if ok {
				continue
			}
			var msg string
			if endpoint == "" {
				msg = fmt.Sprintf(
					"Endpoint not evaluated: %q declares hazard class %q, which the engine does not classify; this hazard did not contribute to the mixture classification. Classify it manually.",
					nameOrUnknown(c.Name), strings.TrimSpace(h.Class),
				)
			} else {
				msg = fmt.Sprintf(
					"Endpoint not evaluated: %q declares %s but its category %q could not be applied, so it was silently dropped from the classification. Provide the missing category/data.",
					nameOrUnknown(c.Name), endpoint, strings.TrimSpace(h.Category),
				)
			}
			if seen[msg] {
				continue
			}
			seen[msg] = true
			flags = append(
				flags, Flag{
					Section:  "2.1",
					Code:     FlagEndpointNotEvaluated,
					Severity: SeverityWarn,
					Message:  msg,
				},
			)
		}
	}
	return flags
}

// hazardEvaluable reports whether the engine evaluates a component hazard. When it
// does not, endpoint is the readable endpoint name for a recognised class with an
// unusable category, or "" for a class the engine does not handle at all (so the
// caller can distinguish "missing category" from "unrecognised class").
func hazardEvaluable(h ComponentHazard) (ok bool, endpoint string) {
	kind := classKind(h.Class)
	cat := normHazardCategory(h.Category)
	switch kind {
	case kindAcuteOral, kindAcuteDermal, kindAcuteInhal:
		if _, found := acuteATE(kind, cat); found {
			return true, ""
		}
		return false, "acute toxicity"
	case kindLactation:
		return true, "" // category-independent GCL ("*")
	case kindCarc, kindMuta, kindRepr, kindRespSens,
		kindSkinCorr, kindSkinIrrit, kindEyeDam, kindEyeIrrit,
		kindStotSE, kindStotRE, kindAsp:
		if _, found := gclFor(kind, cat); found {
			return true, ""
		}
		return false, endpointLabel(kind)
	default:
		// kindOther: skin sensitisation and the aquatic classes are evaluated by
		// classifier.go; flammability is decided at mixture level from the flash
		// point. Anything else is genuinely unhandled.
		parsed, err := ParseHazardClass(h.Class)
		if err != nil {
			return false, ""
		}
		switch parsed {
		case HazardClassSkinSensitisation:
			// classifier.go reads the category to pick the 1A vs 1/1B limit; one it
			// cannot resolve drops the component out of the sensitisation check.
			if _, found := gclFor(kindSkinSens, cat); found {
				return true, ""
			}
			return false, "skin sensitisation"
		case HazardClassAquatic, HazardClassAquaticAcute, HazardClassAquaticChronic:
			// The aquatic cascade switches on the bare category, so anything
			// outside 1-4 contributes to no rung.
			switch cat {
			case "1", "2", "3", "4":
				return true, ""
			}
			return false, "the aquatic environment"
		case HazardClassFlammableLiquid:
			return true, ""
		}
		return false, ""
	}
}

// endpointLabel renders a hazard kind as a readable endpoint name for flags.
func endpointLabel(kind hazardKind) string {
	switch kind {
	case kindCarc:
		return "carcinogenicity"
	case kindMuta:
		return "germ cell mutagenicity"
	case kindRepr:
		return "reproductive toxicity"
	case kindRespSens:
		return "respiratory sensitisation"
	case kindSkinCorr:
		return "skin corrosion"
	case kindSkinIrrit:
		return "skin irritation"
	case kindEyeDam:
		return "serious eye damage"
	case kindEyeIrrit:
		return "eye irritation"
	case kindStotSE:
		return "STOT — single exposure"
	case kindStotRE:
		return "STOT — repeated exposure"
	case kindAsp:
		return "aspiration"
	}
	return "this endpoint"
}

// nameOrUnknown renders a component name for a flag, with a placeholder fallback.
func nameOrUnknown(name string) string {
	if strings.TrimSpace(name) == "" {
		return "(unnamed component)"
	}
	return name
}

// ── Undisclosed remainder ─────────────────────────────────────────────────────

// undisclosedRemainderPP is the size of the unattributed balance (in percentage
// points) above which the validation pass flags that a large remainder is being
// treated as non-hazardous.
const undisclosedRemainderPP = 10.0

// validateUndisclosedRemainder flags a mixture whose disclosed component
// concentrations sum to well under 100%, so a large remainder is being treated as
// a non-hazardous balance. A zero disclosed sum is the degenerate unit-inconsistent
// / empty-recipe case (already covered by a block flag) and is not re-reported.
func validateUndisclosedRemainder(components []ClassComponent) []Flag {
	var disclosed float64
	for _, c := range components {
		disclosed += c.ConcentrationPct
	}
	remainder := 100 - disclosed
	if disclosed <= 0 || remainder <= undisclosedRemainderPP {
		return nil
	}
	return []Flag{
		{
			Section:  "3.2",
			Code:     FlagUndisclosedRemainder,
			Severity: SeverityInfo,
			Message: fmt.Sprintf(
				"Disclosed component concentrations sum to %.1f%%; the remaining %.1f%% is treated as a non-hazardous balance (carrier/solvent). Confirm the undisclosed remainder contains no hazardous substance.",
				disclosed, remainder,
			),
		},
	}
}
